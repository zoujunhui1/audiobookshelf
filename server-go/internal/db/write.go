package db

import (
	"context"
	"database/sql"
	"time"
)

type Session struct {
	ID, UserID, IPAddress, UserAgent, RefreshToken string
	CreatedAt, ExpiresAt                           time.Time
}

func (m *Manager) CreateSession(ctx context.Context, session Session) error {
	return m.access(ctx, func(connection *sql.DB) error {
		// Match Sequelize's SQLite DATE representation so Node can compare dates.
		format := "2006-01-02 15:04:05.000 +00:00"
		now := session.CreatedAt.UTC().Format(format)
		result, err := connection.ExecContext(ctx, `
			INSERT INTO sessions (id, userId, ipAddress, userAgent, refreshToken, expiresAt, createdAt, updatedAt)
			SELECT ?, id, ?, ?, ?, ?, ?, ? FROM users WHERE id = ? AND isActive = 1`,
			session.ID, session.IPAddress, session.UserAgent, session.RefreshToken,
			session.ExpiresAt.UTC().Format(format), now, now, session.UserID)
		if err != nil {
			return err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if count != 1 {
			return ErrInvalidCredentials
		}
		return nil
	})
}
