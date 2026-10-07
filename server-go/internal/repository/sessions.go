package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

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
	// Match Sequelize's UTC SQLite DATE representation. The previous-token
	// columns remain NULL until refresh rotation is implemented.
	const dateFormat = "2006-01-02 15:04:05.000 -07:00"
	createdAt := session.CreatedAt.UTC().Format(dateFormat)
	_, err := r.db.ExecContext(ctx, `INSERT INTO sessions
		(id, userId, ipAddress, userAgent, refreshToken, expiresAt, createdAt, updatedAt)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		session.ID, session.UserID, session.IPAddress, session.UserAgent, session.RefreshToken,
		session.ExpiresAt.UTC().Format(dateFormat), createdAt, createdAt,
	)
	if err != nil {
		return fmt.Errorf("create login session: %w", err)
	}
	return nil
}
