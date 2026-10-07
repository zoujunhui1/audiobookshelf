// Logout and Authorize methods on *AuthService — see server-go/ai/workflow.md
// for the file-ownership split (this file is Dev 2's).
//
// Ground truth: server/Auth.js (the /logout route, ~line 486),
// server/auth/TokenManager.js (session invalidation methods, generateTempAccessToken),
// server/routers/ApiRouter.js + server/controllers/MiscController.js (authorize handler).
package services

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"audiobookshelf-go/internal/repository"
)

// ErrInvalidToken is returned by Authorize when the supplied access token is
// missing, malformed, signed with the wrong secret, expired, or is actually
// a refresh token (refresh tokens are only valid at POST /auth/refresh — see
// TokenManager#jwtAuthCheck).
var ErrInvalidToken = errors.New("invalid or expired access token")

// ErrUserInactive is returned by Authorize when the access token's user no
// longer exists or has been deactivated (TokenManager#jwtAuthCheck checks
// `!user?.isActive`).
var ErrUserInactive = errors.New("user not found or inactive")

// LogoutResult mirrors the body Auth.js's /logout route sends back:
// `res.send({ redirect_url: logoutUrl })`. server-go does not implement the
// OIDC auth strategy (see server-go/ai/workflow.md "Open items not yet
// decided"), so there is never an end-session URL to redirect to and this
// field is always nil.
type LogoutResult struct {
	RedirectURL *string `json:"redirect_url"`
}

// Logout invalidates the refresh-token session(s) for this device, or every
// session belonging to the user when allDevices is true. refreshToken may be
// empty — that mirrors Auth.js's own "No refresh token on request" no-op
// case for the single-device branch, and TokenManager#invalidateAllSessionsForRefreshToken's
// own empty-token guard for the allDevices branch.
func (s *AuthService) Logout(_ context.Context, refreshToken string, allDevices bool) (*LogoutResult, error) {
	switch {
	case allDevices:
		if err := s.invalidateAllSessionsForRefreshToken(refreshToken); err != nil {
			return nil, err
		}
	case refreshToken != "":
		if err := s.invalidateRefreshToken(refreshToken); err != nil {
			return nil, err
		}
	default:
		// No refresh token on request and not an allDevices logout — nothing
		// to invalidate.
	}

	return &LogoutResult{RedirectURL: nil}, nil
}

// invalidateRefreshToken replicates TokenManager#invalidateRefreshToken,
// which deletes the session row whose refreshToken column exactly equals
// token. A token that only matches a session's lastRefreshToken (i.e. it was
// already rotated out and is now just inside its grace period) does not
// match that Sequelize WHERE clause either, so it is left alone here too.
func (s *AuthService) invalidateRefreshToken(token string) error {
	session, err := s.sessions.FindByRefreshToken(token)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil
		}
		return fmt.Errorf("invalidating refresh token: %w", err)
	}

	if session.RefreshToken != token {
		return nil
	}

	if err := s.sessions.Delete(session.ID); err != nil {
		return fmt.Errorf("invalidating refresh token: %w", err)
	}
	return nil
}

// invalidateAllSessionsForRefreshToken replicates
// TokenManager#invalidateAllSessionsForRefreshToken: find the session owning
// this refresh token (current or grace-period value), then delete every
// session for that session's user.
func (s *AuthService) invalidateAllSessionsForRefreshToken(token string) error {
	if token == "" {
		return nil
	}

	session, err := s.sessions.FindByRefreshToken(token)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil
		}
		return fmt.Errorf("invalidating all sessions for refresh token: %w", err)
	}

	if err := s.sessions.DeleteAllForUser(session.UserID); err != nil {
		return fmt.Errorf("invalidating all sessions for refresh token: %w", err)
	}
	return nil
}

// AuthorizedUser is the subset of User#toOldJSONForBrowser() that server-go
// can currently populate from the shared "users" table. The Node payload
// also carries email, type, permissions, token, and more, but those live on
// parts of the User model this rewrite round does not touch (see the
// go-rewrite-dev skill's file-ownership table) — they are intentionally
// omitted here rather than guessed.
type AuthorizedUser struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	IsActive bool   `json:"isActive"`
	IsLocked bool   `json:"isLocked"`
}

// AuthorizeResult mirrors the shape built by
// Auth.js#getUserLoginResponsePayload. ServerSettings, EreaderDevices,
// UserDefaultLibraryID, and Source depend on the Library, Setting, and
// EmailSettings subsystems, none of which exist in server-go yet — they are
// kept here for wire-shape parity but always carry a stub zero value until
// those repositories land in a later round.
type AuthorizeResult struct {
	User                 AuthorizedUser `json:"user"`
	UserDefaultLibraryID *string        `json:"userDefaultLibraryId"`
	ServerSettings       map[string]any `json:"serverSettings"`
	EreaderDevices       []any          `json:"ereaderDevices"`
	Source               string         `json:"Source"`
}

// Authorize verifies accessToken and, if valid, returns the same login
// response payload shape POST /login and POST /auth/refresh return (see
// MiscController#authorize: `this.auth.getUserLoginResponsePayload(req.user)`).
// accessToken must be a non-expired, HS256-signed access token (not a
// refresh token) for an active user.
func (s *AuthService) Authorize(_ context.Context, accessToken string) (*AuthorizeResult, error) {
	if accessToken == "" {
		return nil, ErrInvalidToken
	}

	claims, err := verifyAccessToken(s.cfg.JWTSecret, accessToken)
	if err != nil {
		return nil, ErrInvalidToken
	}

	// TokenManager#isBearerAccessTokenPayload: must carry a userId and must
	// not be a refresh token (refresh tokens are only valid at /auth/refresh).
	if claims.UserID == "" || claims.Type == "refresh" {
		return nil, ErrInvalidToken
	}

	if claims.Exp != 0 && time.Now().Unix() > claims.Exp {
		return nil, ErrInvalidToken
	}

	user, err := s.users.GetByID(claims.UserID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrUserInactive
		}
		return nil, fmt.Errorf("authorizing user %s: %w", claims.UserID, err)
	}

	if !user.IsActive {
		return nil, ErrUserInactive
	}

	return &AuthorizeResult{
		User: AuthorizedUser{
			ID:       user.ID,
			Username: user.Username,
			IsActive: user.IsActive,
			IsLocked: user.IsLocked,
		},
		UserDefaultLibraryID: nil,
		ServerSettings:       map[string]any{},
		EreaderDevices:       []any{},
		Source:               "",
	}, nil
}

// accessTokenClaims is the JWT payload shape TokenManager generates for
// access tokens: `{ userId, username, jti, type: "access" }` (see the
// go-rewrite-dev skill), plus the standard "exp" claim jsonwebtoken adds
// from the expiresIn sign option.
type accessTokenClaims struct {
	UserID   string `json:"userId"`
	Username string `json:"username"`
	JTI      string `json:"jti"`
	Type     string `json:"type"`
	Exp      int64  `json:"exp"`
}

// verifyAccessToken checks the HS256 signature of a compact JWT (the
// "jsonwebtoken" default algorithm) against secret and decodes its payload.
// It does not itself check expiry or claim contents — callers do that.
func verifyAccessToken(secret, token string) (*accessTokenClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("malformed token")
	}

	header, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("decoding token header: %w", err)
	}
	var h struct {
		Alg string `json:"alg"`
	}
	if err := json.Unmarshal(header, &h); err != nil || h.Alg != "HS256" {
		return nil, errors.New("unsupported algorithm")
	}

	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, fmt.Errorf("decoding token signature: %w", err)
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(parts[0] + "." + parts[1]))
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return nil, errors.New("signature mismatch")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decoding token payload: %w", err)
	}

	var claims accessTokenClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("decoding token claims: %w", err)
	}

	return &claims, nil
}
