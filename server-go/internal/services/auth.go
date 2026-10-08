// Package services holds the login-module business logic. AuthService's
// methods are intentionally split across multiple files by task owner (see
// server-go/ai/workflow.md): this file defines the shared struct only, no
// business-logic methods — those live in auth_login.go and auth_logout.go.
package services

import (
	"audiobookshelf-go/internal/config"
	"audiobookshelf-go/internal/repository"
)

type AuthService struct {
	users     *repository.UserRepository
	sessions  *repository.SessionRepository
	libraries *repository.LibraryRepository
	cfg       config.Config
}

func NewAuthService(users *repository.UserRepository, sessions *repository.SessionRepository, libraries *repository.LibraryRepository, cfg config.Config) *AuthService {
	return &AuthService{users: users, sessions: sessions, libraries: libraries, cfg: cfg}
}
