package repository

import (
	"database/sql"
	"strings"
	"time"
)

// User maps the existing SQLite `users` table (Sequelize model in server/models/User.js).
type User struct {
	ID          string
	Username    string
	Email       sql.NullString
	Pash        sql.NullString
	Type        string
	Token       sql.NullString
	IsActive    bool
	IsLocked    bool
	LastSeen    sql.NullTime
	Permissions []byte
	Bookmarks   []byte
	ExtraData   []byte
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type UserRepo struct {
	db *sql.DB
}

func NewUserRepo(db *sql.DB) *UserRepo {
	return &UserRepo{db: db}
}

// GetUserByUsername loads a user by username (case-insensitive), matching
// User.getUserByUsername in server/models/User.js. Returns nil, nil when not found.
func (r *UserRepo) GetUserByUsername(username string) (*User, error) {
	if username == "" {
		return nil, nil
	}

	const q = `
SELECT id, username, email, pash, type, token, isActive, isLocked, lastSeen,
       permissions, bookmarks, extraData, createdAt, updatedAt
FROM users
WHERE lower(username) = ?
LIMIT 1`

	u := &User{}
	err := r.db.QueryRow(q, strings.ToLower(username)).Scan(
		&u.ID,
		&u.Username,
		&u.Email,
		&u.Pash,
		&u.Type,
		&u.Token,
		&u.IsActive,
		&u.IsLocked,
		&u.LastSeen,
		&u.Permissions,
		&u.Bookmarks,
		&u.ExtraData,
		&u.CreatedAt,
		&u.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return u, nil
}
