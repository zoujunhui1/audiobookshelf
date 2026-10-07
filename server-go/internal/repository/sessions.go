package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

const sqliteDateFormat = "2006-01-02 15:04:05.000 -07:00"

type Session struct {
	ID, UserID, IPAddress, UserAgent, RefreshToken string
	CreatedAt, ExpiresAt                           time.Time
}

type SessionRepository struct {
	db *sql.DB
}

func NewSessionRepository(db *sql.DB) *SessionRepository {
	return &SessionRepository{db: db}
}

func (r *SessionRepository) Create(ctx context.Context, session Session) error {
	// Match Sequelize's UTC SQLite DATE representation.
	createdAt := session.CreatedAt.UTC().Format(sqliteDateFormat)
	_, err := r.db.ExecContext(ctx, `INSERT INTO sessions
		(id, userId, ipAddress, userAgent, refreshToken, expiresAt, createdAt, updatedAt)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		session.ID, session.UserID, session.IPAddress, session.UserAgent, session.RefreshToken,
		session.ExpiresAt.UTC().Format(sqliteDateFormat), createdAt, createdAt,
	)
	if err != nil {
		return fmt.Errorf("create login session: %w", err)
	}
	return nil
}

// GetByRefreshToken matches only the current token; this experiment has no
// previous-token grace period. CAST preserves the original SQLite date text.
func (r *SessionRepository) GetByRefreshToken(ctx context.Context, token string) (*Session, error) {
	var session Session
	var expiry string
	err := r.db.QueryRowContext(ctx, `SELECT id, userId, refreshToken, CAST(expiresAt AS TEXT)
		FROM sessions WHERE refreshToken = ? LIMIT 1`, token).Scan(
		&session.ID, &session.UserID, &session.RefreshToken, &expiry,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get refresh session: %w", err)
	}
	session.ExpiresAt, err = time.Parse(sqliteDateFormat, expiry)
	if err != nil {
		session.ExpiresAt, err = time.Parse(time.RFC3339Nano, expiry)
	}
	if err != nil {
		return nil, fmt.Errorf("decode refresh session expiry: %w", err)
	}
	return &session, nil
}

// Rotate conditionally replaces the token we validated. A concurrent rotation
// or deletion returns false so the caller cannot deliver unstored credentials.
func (r *SessionRepository) Rotate(ctx context.Context, session Session, newToken string,
	expiresAt, updatedAt time.Time) (bool, error) {
	result, err := r.db.ExecContext(ctx, `UPDATE sessions SET refreshToken = ?, expiresAt = ?,
		updatedAt = ?, lastRefreshToken = NULL, lastRefreshTokenExpiresAt = NULL
		WHERE id = ? AND userId = ? AND refreshToken = ?`,
		newToken, expiresAt.UTC().Format(sqliteDateFormat), updatedAt.UTC().Format(sqliteDateFormat),
		session.ID, session.UserID, session.RefreshToken,
	)
	if err != nil {
		return false, fmt.Errorf("rotate refresh session: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("check refresh rotation: %w", err)
	}
	return count == 1, nil
}
