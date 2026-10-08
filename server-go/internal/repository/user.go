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
// "password" — see the go-rewrite-dev skill. Type is needed both for
// LocalAuthStrategy's root-without-a-password-yet login path, and because
// the frontend's post-login redirect checks user.type === 'root' before
// falling back to an error page when there's no default library (see
// client/pages/login.vue's `user` watcher). Permissions is the raw JSON
// column (server/models/User.js's `permissions` field) — callers that need
// accessAllLibraries/librariesAccessible unmarshal it themselves.
type User struct {
	ID          string
	Username    string
	Pash        string
	Type        string
	Permissions string
	IsActive    bool
	IsLocked    bool
}

type UserRepository struct {
	db *sql.DB
}

func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) GetByUsername(username string) (*User, error) {
	return r.queryOne(`SELECT id, username, pash, type, permissions, isActive, isLocked FROM users WHERE username = ?`, username)
}

func (r *UserRepository) GetByID(id string) (*User, error) {
	return r.queryOne(`SELECT id, username, pash, type, permissions, isActive, isLocked FROM users WHERE id = ?`, id)
}

func (r *UserRepository) queryOne(query string, arg string) (*User, error) {
	var u User
	var permissions sql.NullString
	row := r.db.QueryRow(query, arg)
	if err := row.Scan(&u.ID, &u.Username, &u.Pash, &u.Type, &permissions, &u.IsActive, &u.IsLocked); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("querying user: %w", err)
	}
	u.Permissions = permissions.String
	return &u, nil
}
