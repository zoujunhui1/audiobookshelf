package repository

import (
	"database/sql"
	"errors"
	"fmt"
)

// ErrNotFound is returned when a lookup finds no matching row.
var ErrNotFound = errors.New("not found")

// User mirrors the subset of the "users" table (server/models/User.js) the
// login module needs. Column name for the password hash is "pash", not
// "password" — see the go-rewrite-dev skill. Type is needed to replicate
// LocalAuthStrategy's root-without-a-password-yet login path (a freshly
// initialized server's root user has Pash == "" and must still be allowed
// to log in with an empty password to reach the setup wizard).
type User struct {
	ID       string
	Username string
	Pash     string
	Type     string
	IsActive bool
	IsLocked bool
}

type UserRepository struct {
	db *sql.DB
}

func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) GetByUsername(username string) (*User, error) {
	return r.queryOne(`SELECT id, username, pash, type, isActive, isLocked FROM users WHERE username = ?`, username)
}

func (r *UserRepository) GetByID(id string) (*User, error) {
	return r.queryOne(`SELECT id, username, pash, type, isActive, isLocked FROM users WHERE id = ?`, id)
}

func (r *UserRepository) queryOne(query string, arg string) (*User, error) {
	var u User
	row := r.db.QueryRow(query, arg)
	if err := row.Scan(&u.ID, &u.Username, &u.Pash, &u.Type, &u.IsActive, &u.IsLocked); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("querying user: %w", err)
	}
	return &u, nil
}
