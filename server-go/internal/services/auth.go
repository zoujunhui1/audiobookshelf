package services

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"audiobookshelf-go/internal/db"
)

type LoginPayloadProvider interface {
	LoginPayload(context.Context, string) (map[string]any, error)
}

type Auth struct {
	database                    *db.Manager
	payload                     LoginPayloadProvider
	secret                      string
	accessExpiry, refreshExpiry time.Duration
}

func NewAuth(database *db.Manager, payload LoginPayloadProvider, secret string, accessExpiry, refreshExpiry time.Duration) *Auth {
	return &Auth{database: database, payload: payload, secret: secret, accessExpiry: accessExpiry, refreshExpiry: refreshExpiry}
}

type LoginResult struct {
	Payload          map[string]any
	RefreshToken     string
	RefreshExpiresAt time.Time
}

func (a *Auth) Login(ctx context.Context, username, password, ip, userAgent string, returnTokens bool) (LoginResult, error) {
	settings, err := a.database.ReadAuthSettings(ctx)
	if err != nil {
		return LoginResult{}, err
	}
	if !slices.Contains(settings.Methods, "local") {
		return LoginResult{}, db.ErrInvalidCredentials
	}
	user, err := a.database.CheckCredentials(ctx, username, password)
	if err != nil {
		return LoginResult{}, err
	}
	secret := a.secret
	if secret == "" {
		secret = settings.TokenSecret
	}
	if secret == "" {
		return LoginResult{}, errors.New("shared JWT secret is missing")
	}
	now := time.Now().UTC()
	accessToken, err := signToken(user, "access", secret, now, a.accessExpiry)
	if err != nil {
		return LoginResult{}, err
	}
	refreshToken, err := signToken(user, "refresh", secret, now, a.refreshExpiry)
	if err != nil {
		return LoginResult{}, err
	}
	// Node supplies the existing business-data payload, using only Go's access
	// token. It never receives the submitted username or password.
	payload, err := a.payload.LoginPayload(ctx, accessToken)
	if err != nil {
		return LoginResult{}, err
	}
	userPayload, ok := payload["user"].(map[string]any)
	if !ok || userPayload["id"] != user.ID {
		return LoginResult{}, errors.New("invalid Node login payload")
	}
	sessionID, err := uuid()
	if err != nil {
		return LoginResult{}, err
	}
	expiresAt := now.Add(a.refreshExpiry)
	err = a.database.CreateSession(ctx, db.Session{ID: sessionID, UserID: user.ID, IPAddress: ip, UserAgent: userAgent,
		RefreshToken: refreshToken, CreatedAt: now, ExpiresAt: expiresAt})
	if err != nil {
		return LoginResult{}, err
	}
	userPayload["accessToken"] = accessToken
	userPayload["refreshToken"] = nil
	if returnTokens {
		userPayload["refreshToken"] = refreshToken
	}
	return LoginResult{Payload: payload, RefreshToken: refreshToken, RefreshExpiresAt: expiresAt}, nil
}

func signToken(user db.User, tokenType, secret string, now time.Time, expiry time.Duration) (string, error) {
	id, err := uuid()
	if err != nil {
		return "", err
	}
	claims := jwt.MapClaims{"userId": user.ID, "username": user.Username, "jti": id, "type": tokenType,
		"iat": now.Unix(), "exp": now.Add(expiry).Unix()}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
}

func uuid() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", value[:4], value[4:6], value[6:8], value[8:10], value[10:]), nil
}
