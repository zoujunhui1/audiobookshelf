package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	"audiobookshelf-go/internal/repository"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

var ErrInvalidCredentials = errors.New("invalid credentials")

type AuthService struct {
	users                       *repository.UserRepository
	sessions                    *repository.SessionRepository
	secret                      []byte
	accessExpiry, refreshExpiry time.Duration
}

func NewAuthService(users *repository.UserRepository, sessions *repository.SessionRepository,
	secret string, accessExpiry, refreshExpiry time.Duration) (*AuthService, error) {
	if secret == "" {
		return nil, errors.New("local login requires JWT_SECRET_KEY or a stored server-settings tokenSecret")
	}
	if accessExpiry < time.Second || refreshExpiry < time.Second {
		return nil, errors.New("token lifetimes must be at least one second")
	}
	return &AuthService{users: users, sessions: sessions, secret: []byte(secret),
		accessExpiry: accessExpiry, refreshExpiry: refreshExpiry}, nil
}

type LoginResult struct {
	User                       *repository.User
	AccessToken, RefreshToken  string
	IssuedAt, RefreshExpiresAt time.Time
}

func (s *AuthService) Login(ctx context.Context, username, password, ipAddress, userAgent string) (*LoginResult, error) {
	user, err := s.users.GetByUsername(ctx, username)
	if err != nil {
		return nil, err
	}
	if user == nil || !user.IsActive {
		return nil, ErrInvalidCredentials
	}
	if user.Type == "root" && user.PasswordHash == "" {
		if password != "" {
			return nil, ErrInvalidCredentials
		}
	} else if user.PasswordHash == "" || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		return nil, ErrInvalidCredentials
	}

	now := time.Now().UTC()
	accessToken, err := s.sign(user, "access", now, s.accessExpiry)
	if err != nil {
		return nil, err
	}
	refreshToken, err := s.sign(user, "refresh", now, s.refreshExpiry)
	if err != nil {
		return nil, err
	}
	id, err := uuid.NewRandom()
	if err != nil {
		return nil, fmt.Errorf("generate session ID: %w", err)
	}
	expiresAt := now.Add(s.refreshExpiry)
	if err := s.sessions.Create(ctx, repository.Session{
		ID: id.String(), UserID: user.ID, IPAddress: ipAddress, UserAgent: userAgent,
		RefreshToken: refreshToken, CreatedAt: now, ExpiresAt: expiresAt,
	}); err != nil {
		return nil, err
	}
	return &LoginResult{User: user, AccessToken: accessToken, RefreshToken: refreshToken,
		IssuedAt: now, RefreshExpiresAt: expiresAt}, nil
}

func (s *AuthService) sign(user *repository.User, tokenType string, now time.Time, lifetime time.Duration) (string, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return "", fmt.Errorf("generate token ID: %w", err)
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"userId": user.ID, "username": user.Username, "jti": id.String(),
		"type": tokenType, "iat": now.Unix(), "exp": now.Add(lifetime).Unix(),
	})
	signed, err := token.SignedString(s.secret)
	if err != nil {
		return "", fmt.Errorf("sign login token: %w", err)
	}
	return signed, nil
}
