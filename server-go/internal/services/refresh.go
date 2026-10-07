package services

import (
	"context"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrInvalidRefreshToken = errors.New("Invalid refresh token")
	ErrExpiredRefreshToken = errors.New("Refresh token expired")
	ErrInvalidTokenType    = errors.New("Invalid token type")
	ErrInactiveRefreshUser = errors.New("User not found or inactive")
)

func (s *AuthService) Refresh(ctx context.Context, value string) (*LoginResult, error) {
	token, err := jwt.Parse(value, func(token *jwt.Token) (any, error) {
		return s.secret, nil
	}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired())
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) && !errors.Is(err, jwt.ErrTokenSignatureInvalid) {
			return nil, ErrExpiredRefreshToken
		}
		return nil, ErrInvalidRefreshToken
	}
	if !token.Valid {
		return nil, ErrInvalidRefreshToken
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, ErrInvalidRefreshToken
	}
	if claims["type"] != "refresh" {
		return nil, ErrInvalidTokenType
	}
	userID, ok := claims["userId"].(string)
	if !ok || userID == "" {
		return nil, ErrInvalidRefreshToken
	}
	session, err := s.sessions.GetByRefreshToken(ctx, value)
	if err != nil {
		return nil, err
	}
	if session == nil || session.UserID != userID {
		return nil, ErrInvalidRefreshToken
	}
	now := time.Now().UTC()
	if !session.ExpiresAt.After(now) {
		return nil, ErrExpiredRefreshToken
	}
	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user == nil || !user.IsActive {
		return nil, ErrInactiveRefreshUser
	}
	accessToken, err := s.sign(user, "access", now, s.accessExpiry)
	if err != nil {
		return nil, err
	}
	refreshToken, err := s.sign(user, "refresh", now, s.refreshExpiry)
	if err != nil {
		return nil, err
	}
	expiresAt := now.Add(s.refreshExpiry)
	rotated, err := s.sessions.Rotate(ctx, *session, refreshToken, expiresAt, now)
	if err != nil {
		return nil, err
	}
	if !rotated {
		return nil, ErrInvalidRefreshToken
	}
	return &LoginResult{User: user, AccessToken: accessToken, RefreshToken: refreshToken,
		IssuedAt: now, RefreshExpiresAt: expiresAt}, nil
}
