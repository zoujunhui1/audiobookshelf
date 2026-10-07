package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// User contains only the data required for local credential verification.
type User struct {
	ID           string
	Username     string
	PasswordHash string
	Type         string
	IsActive     bool
}

type UserRepository struct {
	db *sql.DB
}

func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

// GetByUsername mirrors Node's lower(username) lookup. Missing users return
// (nil, nil); database failures return an error. Active/password checks belong
// in the future authentication service, not in this repository.
func (r *UserRepository) GetByUsername(ctx context.Context, username string) (*User, error) {
	if username == "" {
		return nil, nil
	}
	const query = `SELECT id, username, COALESCE(pash, ''), COALESCE(type, ''),
		COALESCE(isActive, 0)
		FROM users WHERE lower(username) = ? LIMIT 1`
	var user User
	err := r.db.QueryRowContext(ctx, query, strings.ToLower(username)).Scan(
		&user.ID, &user.Username, &user.PasswordHash, &user.Type, &user.IsActive,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get user by username: %w", err)
	}
	return &user, nil
}
