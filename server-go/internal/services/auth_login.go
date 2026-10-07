package services

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"audiobookshelf-go/internal/repository"
)

// Errors returned by Login and Refresh. Handlers map them to HTTP responses.
var (
	// ErrInvalidCredentials means the username/password pair was rejected.
	ErrInvalidCredentials = errors.New("invalid credentials")
	// ErrInvalidTokenType means a JWT that is not a refresh token was presented.
	ErrInvalidTokenType = errors.New("invalid token type")
	// ErrInvalidRefreshToken means the refresh token is malformed, unknown, or past its grace period.
	ErrInvalidRefreshToken = errors.New("invalid refresh token")
	// ErrRefreshTokenExpired means the refresh token or its session has expired.
	ErrRefreshTokenExpired = errors.New("refresh token expired")
	// ErrUserInactive means the token's user no longer exists or is inactive.
	ErrUserInactive = errors.New("user not found or inactive")
)

// LoginUser is the user object inside a login/refresh response. Only the
// fields available from UserRepository are populated; the full
// User.toOldJSONForBrowser() field set is not yet ported.
type LoginUser struct {
	ID           string  `json:"id"`
	Username     string  `json:"username"`
	IsActive     bool    `json:"isActive"`
	IsLocked     bool    `json:"isLocked"`
	AccessToken  string  `json:"accessToken"`
	RefreshToken *string `json:"refreshToken"`
}

// LoginResponse mirrors getUserLoginResponsePayload in server/Auth.js.
type LoginResponse struct {
	User                 LoginUser      `json:"user"`
	UserDefaultLibraryID *string        `json:"userDefaultLibraryId"`
	ServerSettings       map[string]any `json:"serverSettings"`
	EreaderDevices       []any          `json:"ereaderDevices"`
	Source               string         `json:"Source"`
}

// AuthResult is the outcome of a successful Login or Refresh. The handler
// writes Response as the body and sets RefreshToken as the refresh_token
// cookie (when applicable).
type AuthResult struct {
	Response     *LoginResponse
	RefreshToken string
	// CookieMaxAge is the refresh_token cookie lifetime (the refresh token expiry).
	CookieMaxAge time.Duration
}

// Login checks the credentials, creates a session, and issues tokens. When
// returnTokens is true the refresh token is also included in the response body.
func (s *AuthService) Login(_ context.Context, username, password, ipAddress, userAgent string, returnTokens bool) (*AuthResult, error) {
	user, err := s.users.GetByUsername(strings.ToLower(username))
	if errors.Is(err, repository.ErrNotFound) {
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, fmt.Errorf("loading user %q: %w", username, err)
	}
	if !user.IsActive {
		return nil, ErrInvalidCredentials
	}
	// Mirrors LocalAuthStrategy.verifyCredentials exactly: a freshly
	// initialized server's root user has no password set yet, and must be
	// allowed to log in with an empty password to reach the setup wizard —
	// but only with an empty password; any non-empty password is rejected.
	// Any other user with no password set (e.g. OpenID-only accounts) is
	// always rejected, regardless of what password was supplied.
	switch {
	case user.Type == "root" && user.Pash == "":
		if password != "" {
			return nil, ErrInvalidCredentials
		}
	case user.Pash == "":
		return nil, ErrInvalidCredentials
	default:
		if err := bcrypt.CompareHashAndPassword([]byte(user.Pash), []byte(password)); err != nil {
			return nil, ErrInvalidCredentials
		}
	}

	accessToken, err := s.signToken(user, "access", s.cfg.AccessTokenExpiry)
	if err != nil {
		return nil, fmt.Errorf("generating access token: %w", err)
	}
	refreshToken, err := s.signToken(user, "refresh", s.cfg.RefreshTokenExpiry)
	if err != nil {
		return nil, fmt.Errorf("generating refresh token: %w", err)
	}
	expiresAt := time.Now().Add(s.cfg.RefreshTokenExpiry)
	if _, err := s.sessions.Create(user.ID, ipAddress, userAgent, refreshToken, expiresAt); err != nil {
		return nil, fmt.Errorf("creating session: %w", err)
	}

	return &AuthResult{
		Response:     loginResponse(user, accessToken, refreshToken, returnTokens),
		RefreshToken: refreshToken,
		CookieMaxAge: s.cfg.RefreshTokenExpiry,
	}, nil
}

// Refresh validates a refresh token and returns a new access token. Outside
// the grace period the refresh token is rotated; inside it the session's
// already-rotated current token is returned unchanged. When returnToken is
// true the refresh token is also included in the response body.
func (s *AuthService) Refresh(_ context.Context, refreshToken string, returnToken bool) (*AuthResult, error) {
	claims, err := s.parseToken(refreshToken)
	if errors.Is(err, errTokenExpired) {
		s.deleteExpiredSession(refreshToken)
		return nil, ErrRefreshTokenExpired
	}
	if err != nil {
		return nil, ErrInvalidRefreshToken
	}
	if claims.Type != "refresh" {
		return nil, ErrInvalidTokenType
	}

	session, err := s.sessions.FindByRefreshToken(refreshToken)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, ErrInvalidRefreshToken
	}
	if err != nil {
		return nil, fmt.Errorf("finding session: %w", err)
	}

	now := time.Now()
	isGracePeriod := false
	if session.RefreshToken != refreshToken {
		// Token matched lastRefreshToken.
		if session.LastRefreshTokenExpiresAt == nil || !session.LastRefreshTokenExpiresAt.After(now) {
			return nil, ErrInvalidRefreshToken
		}
		isGracePeriod = true
	} else if session.ExpiresAt.Before(now) {
		if err := s.sessions.Delete(session.ID); err != nil {
			return nil, fmt.Errorf("deleting expired session: %w", err)
		}
		return nil, ErrRefreshTokenExpired
	}

	user, err := s.users.GetByID(claims.UserID)
	if errors.Is(err, repository.ErrNotFound) || (err == nil && !user.IsActive) {
		return nil, ErrUserInactive
	}
	if err != nil {
		return nil, fmt.Errorf("loading user %s: %w", claims.UserID, err)
	}

	accessToken, err := s.signToken(user, "access", s.cfg.AccessTokenExpiry)
	if err != nil {
		return nil, fmt.Errorf("generating access token: %w", err)
	}

	currentRefresh := session.RefreshToken
	if !isGracePeriod {
		newRefresh, err := s.signToken(user, "refresh", s.cfg.RefreshTokenExpiry)
		if err != nil {
			return nil, fmt.Errorf("generating refresh token: %w", err)
		}
		graceExpires := now.Add(s.cfg.RefreshGracePeriod)
		rotated, err := s.sessions.RotateTokens(session.ID, session.RefreshToken, newRefresh, now.Add(s.cfg.RefreshTokenExpiry), &session.RefreshToken, &graceExpires)
		if err != nil {
			return nil, fmt.Errorf("rotating session: %w", err)
		}
		if rotated {
			currentRefresh = newRefresh
		} else {
			// Lost the race to a concurrent/retried refresh that already
			// rotated this session — mirrors Node's rotateTokensForSession:
			// hand back the token that already won instead of minting a
			// second, conflicting one.
			latest, err := s.sessions.FindByRefreshToken(session.RefreshToken)
			if err != nil {
				return nil, fmt.Errorf("re-reading session after rotation race: %w", err)
			}
			currentRefresh = latest.RefreshToken
		}
	}

	return &AuthResult{
		Response:     loginResponse(user, accessToken, currentRefresh, returnToken),
		RefreshToken: currentRefresh,
		CookieMaxAge: s.cfg.RefreshTokenExpiry,
	}, nil
}

// deleteExpiredSession removes the session whose current refresh token has
// expired. Failures are ignored: the caller is already rejecting the request,
// and a leftover expired row is harmless (it just won't match future lookups).
func (s *AuthService) deleteExpiredSession(refreshToken string) {
	session, err := s.sessions.FindByRefreshToken(refreshToken)
	if err != nil || session.RefreshToken != refreshToken {
		return
	}
	_ = s.sessions.Delete(session.ID) // best-effort cleanup, matches Node
}

// sourceEnv mirrors index.js's `options.source || process.env.SOURCE || 'debian'`
// (the CLI --source flag isn't ported here, so only the env-var/default part applies).
func sourceEnv() string {
	if v := os.Getenv("SOURCE"); v != "" {
		return v
	}
	return "debian"
}

func loginResponse(user *repository.User, accessToken, refreshToken string, includeRefresh bool) *LoginResponse {
	u := LoginUser{
		ID:          user.ID,
		Username:    user.Username,
		IsActive:    user.IsActive,
		IsLocked:    user.IsLocked,
		AccessToken: accessToken,
	}
	if includeRefresh {
		u.RefreshToken = &refreshToken
	}
	return &LoginResponse{
		User:                 u,
		UserDefaultLibraryID: nil,
		ServerSettings:       map[string]any{},
		EreaderDevices:       []any{},
		Source:               sourceEnv(),
	}
}

// tokenClaims is the JWT payload for both access and refresh tokens.
type tokenClaims struct {
	UserID   string `json:"userId"`
	Username string `json:"username"`
	JTI      string `json:"jti"`
	Type     string `json:"type"`
	IssuedAt int64  `json:"iat"`
	Expires  int64  `json:"exp"`
}

var errTokenExpired = errors.New("token expired")

var jwtHeader = base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))

// signToken issues an HS256 JWT of the given type ("access" or "refresh").
func (s *AuthService) signToken(user *repository.User, tokenType string, expiry time.Duration) (string, error) {
	now := time.Now()
	payload, err := json.Marshal(tokenClaims{
		UserID:   user.ID,
		Username: user.Username,
		JTI:      uuid.NewString(),
		Type:     tokenType,
		IssuedAt: now.Unix(),
		Expires:  now.Add(expiry).Unix(),
	})
	if err != nil {
		return "", fmt.Errorf("marshaling claims: %w", err)
	}
	signingInput := jwtHeader + "." + base64.RawURLEncoding.EncodeToString(payload)
	return signingInput + "." + s.signature(signingInput), nil
}

// parseToken verifies an HS256 JWT's signature and expiry. It returns
// errTokenExpired for a validly signed but expired token.
func (s *AuthService) parseToken(token string) (*tokenClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("malformed token")
	}
	header, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("decoding header: %w", err)
	}
	var h struct {
		Alg string `json:"alg"`
	}
	if err := json.Unmarshal(header, &h); err != nil || h.Alg != "HS256" {
		return nil, errors.New("unsupported algorithm")
	}
	want := s.signature(parts[0] + "." + parts[1])
	if !hmac.Equal([]byte(want), []byte(parts[2])) {
		return nil, errors.New("invalid signature")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decoding payload: %w", err)
	}
	var claims tokenClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("parsing claims: %w", err)
	}
	if claims.Expires != 0 && time.Now().Unix() >= claims.Expires {
		return nil, errTokenExpired
	}
	return &claims, nil
}

func (s *AuthService) signature(signingInput string) string {
	mac := hmac.New(sha256.New, []byte(s.cfg.JWTSecret))
	mac.Write([]byte(signingInput))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
