package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

var ErrInvalidCredentials = errors.New("invalid credentials")

type User struct {
	ID       string
	Username string
}

// CheckCredentials queries the same users/pash fields as LocalAuthStrategy.
// Password hashing runs after releasing the database lock.
func (m *Manager) CheckCredentials(ctx context.Context, username, password string) (User, error) {
	var user User
	var hash, userType sql.NullString
	var active bool
	err := m.access(ctx, func(connection *sql.DB) error {
		return connection.QueryRowContext(ctx,
			`SELECT id, username, pash, type, isActive FROM users WHERE lower(username) = ? LIMIT 1`,
			strings.ToLower(username)).Scan(&user.ID, &user.Username, &hash, &userType, &active)
	})
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrInvalidCredentials
	}
	if err != nil {
		return User{}, err
	}
	if !active {
		return User{}, ErrInvalidCredentials
	}
	if hash.String == "" {
		if userType.String == "root" && password == "" {
			return user, nil
		}
		return User{}, ErrInvalidCredentials
	}
	// bcryptjs truncates UTF-8 password bytes at 72 bytes; match existing hashes.
	passwordBytes := []byte(password)
	if len(passwordBytes) > 72 {
		passwordBytes = passwordBytes[:72]
	}
	if bcrypt.CompareHashAndPassword([]byte(hash.String), passwordBytes) != nil {
		return User{}, ErrInvalidCredentials
	}
	return user, ctx.Err()
}

type AuthSettings struct {
	TokenSecret string   `json:"tokenSecret"`
	Methods     []string `json:"authActiveAuthMethods"`
}

func (m *Manager) ReadAuthSettings(ctx context.Context) (AuthSettings, error) {
	settings := AuthSettings{Methods: []string{"local"}}
	var raw string
	err := m.access(ctx, func(connection *sql.DB) error {
		return connection.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = 'server-settings'`).Scan(&raw)
	})
	if errors.Is(err, sql.ErrNoRows) {
		return settings, nil
	}
	if err != nil {
		return settings, err
	}
	err = json.Unmarshal([]byte(raw), &settings)
	return settings, err
}
