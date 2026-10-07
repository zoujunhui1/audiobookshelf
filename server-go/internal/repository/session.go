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

// sequelizeDateTimeLayout matches exactly what Sequelize's SQLite dialect
// writes (verified against real rows in config/absdatabase.sqlite, e.g.
// "2026-09-25 01:24:18.531 +00:00"): UTC, millisecond precision, a "+00:00"
// offset rather than Go's default "+0000 UTC" suffix. This isn't cosmetic —
// Node and Go share this database (see server-go/ai/workflow.md's
// transitional architecture), so a session row Go writes must be exactly as
// readable to Sequelize as one Node wrote, and a row Node wrote (including
// every pre-existing row from before this rewrite) must be exactly as
// readable to Go. The sqlite driver's default time.Time scan/bind does
// neither: it round-trips its own format fine but can't parse Sequelize's.
const sequelizeDateTimeLayout = "2006-01-02 15:04:05.000 -07:00"

func sequelizeDateTime(t time.Time) string {
	return t.UTC().Format(sequelizeDateTimeLayout)
}

func parseSequelizeDateTime(raw string) (time.Time, error) {
	return time.Parse(sequelizeDateTimeLayout, raw)
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
	// createdAt/updatedAt are NOT NULL with no default in the real schema
	// (Sequelize sets them application-side on every model, same as here) —
	// omitting them fails the INSERT outright against real data.
	now := sequelizeDateTime(time.Now())
	_, err := r.db.Exec(
		`INSERT INTO sessions (id, userId, ipAddress, userAgent, refreshToken, expiresAt, createdAt, updatedAt) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		s.ID, s.UserID, s.IPAddress, s.UserAgent, s.RefreshToken, sequelizeDateTime(s.ExpiresAt), now, now,
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
	var expiresAtRaw string
	var lastExpiresAtRaw sql.NullString
	if err := row.Scan(&s.ID, &s.UserID, &s.IPAddress, &s.UserAgent, &s.RefreshToken, &expiresAtRaw, &s.LastRefreshToken, &lastExpiresAtRaw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("querying session by refresh token: %w", err)
	}

	expiresAt, err := parseSequelizeDateTime(expiresAtRaw)
	if err != nil {
		return nil, fmt.Errorf("parsing session %s expiresAt %q: %w", s.ID, expiresAtRaw, err)
	}
	s.ExpiresAt = expiresAt

	if lastExpiresAtRaw.Valid {
		t, err := parseSequelizeDateTime(lastExpiresAtRaw.String)
		if err != nil {
			return nil, fmt.Errorf("parsing session %s lastRefreshTokenExpiresAt %q: %w", s.ID, lastExpiresAtRaw.String, err)
		}
		s.LastRefreshTokenExpiresAt = &t
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
	// updatedAt is bumped here because Sequelize's Session.update() does so on
	// every call by default (timestamps aren't disabled on this model) —
	// matched for parity, not because any NOT NULL constraint requires it.
	var graceExpires *string
	if lastRefreshTokenExpiresAt != nil {
		v := sequelizeDateTime(*lastRefreshTokenExpiresAt)
		graceExpires = &v
	}
	result, err := r.db.Exec(
		`UPDATE sessions SET refreshToken = ?, expiresAt = ?, lastRefreshToken = ?, lastRefreshTokenExpiresAt = ?, updatedAt = ? WHERE id = ? AND refreshToken = ?`,
		newRefreshToken, sequelizeDateTime(newExpiresAt), lastRefreshToken, graceExpires, sequelizeDateTime(time.Now()), id, previousRefreshToken,
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
