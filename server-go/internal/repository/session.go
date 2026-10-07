package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Session mirrors the "sessions" table (server/models/Session.js). Sequelize
// generates the id application-side (UUIDV4), this repository does the same.
type Session struct {
	ID                        string
	UserID                    string
	IPAddress                 string
	UserAgent                 string
	RefreshToken              string
	ExpiresAt                 time.Time
	LastRefreshToken          *string
	LastRefreshTokenExpiresAt *time.Time
}

type SessionRepository struct {
	db *sql.DB
}

func NewSessionRepository(db *sql.DB) *SessionRepository {
	return &SessionRepository{db: db}
}

// Create inserts a new session row for a freshly issued refresh token.
func (r *SessionRepository) Create(userID, ipAddress, userAgent, refreshToken string, expiresAt time.Time) (*Session, error) {
	s := &Session{
		ID:           uuid.NewString(),
		UserID:       userID,
		IPAddress:    ipAddress,
		UserAgent:    userAgent,
		RefreshToken: refreshToken,
		ExpiresAt:    expiresAt,
	}
	_, err := r.db.Exec(
		`INSERT INTO sessions (id, userId, ipAddress, userAgent, refreshToken, expiresAt) VALUES (?, ?, ?, ?, ?, ?)`,
		s.ID, s.UserID, s.IPAddress, s.UserAgent, s.RefreshToken, s.ExpiresAt,
	)
	if err != nil {
		return nil, fmt.Errorf("creating session for user %s: %w", userID, err)
	}
	return s, nil
}

// FindByRefreshToken looks up a session where token matches either the
// current refreshToken or the still-valid lastRefreshToken (grace period).
// The caller decides which case it is by comparing the token against the
// returned session's own RefreshToken field — see the go-rewrite-dev skill's
// refresh-rotation steps.
func (r *SessionRepository) FindByRefreshToken(token string) (*Session, error) {
	row := r.db.QueryRow(
		`SELECT id, userId, ipAddress, userAgent, refreshToken, expiresAt, lastRefreshToken, lastRefreshTokenExpiresAt
		 FROM sessions WHERE refreshToken = ? OR lastRefreshToken = ?`,
		token, token,
	)
	var s Session
	if err := row.Scan(&s.ID, &s.UserID, &s.IPAddress, &s.UserAgent, &s.RefreshToken, &s.ExpiresAt, &s.LastRefreshToken, &s.LastRefreshTokenExpiresAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("querying session by refresh token: %w", err)
	}
	return &s, nil
}

// RotateTokens updates a session's refreshToken (and the grace-period fields)
// after a successful rotation, but only if the row still has
// previousRefreshToken — an optimistic lock matching Node's
// rotateTokensForSession (TokenManager.js), which guards against two
// concurrent/retried refreshes racing each other. Pass
// lastRefreshToken/lastRefreshTokenExpiresAt as nil to clear the grace
// period instead of setting one.
//
// Returns false (no error) if another rotation already won the race — the
// caller must then re-read the session and use its current refreshToken
// instead of minting a second one, exactly like Node's numUpdated===0 path.
func (r *SessionRepository) RotateTokens(id, previousRefreshToken, newRefreshToken string, newExpiresAt time.Time, lastRefreshToken *string, lastRefreshTokenExpiresAt *time.Time) (bool, error) {
	result, err := r.db.Exec(
		`UPDATE sessions SET refreshToken = ?, expiresAt = ?, lastRefreshToken = ?, lastRefreshTokenExpiresAt = ? WHERE id = ? AND refreshToken = ?`,
		newRefreshToken, newExpiresAt, lastRefreshToken, lastRefreshTokenExpiresAt, id, previousRefreshToken,
	)
	if err != nil {
		return false, fmt.Errorf("rotating session %s: %w", id, err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("checking rotation result for session %s: %w", id, err)
	}
	return n > 0, nil
}

// Delete removes a single session (single-device logout).
func (r *SessionRepository) Delete(id string) error {
	if _, err := r.db.Exec(`DELETE FROM sessions WHERE id = ?`, id); err != nil {
		return fmt.Errorf("deleting session %s: %w", id, err)
	}
	return nil
}

// DeleteAllForUser removes every session for a user (logout ?allDevices=1).
func (r *SessionRepository) DeleteAllForUser(userID string) error {
	if _, err := r.db.Exec(`DELETE FROM sessions WHERE userId = ?`, userID); err != nil {
		return fmt.Errorf("deleting all sessions for user %s: %w", userID, err)
	}
	return nil
}
