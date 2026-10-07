package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// LoadJWTSecret reads only the persisted signing key; it never updates settings.
func LoadJWTSecret(ctx context.Context, db *sql.DB) (string, error) {
	var value string
	err := db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, "server-settings").Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("load JWT settings: %w", err)
	}
	var setting struct {
		TokenSecret string `json:"tokenSecret"`
	}
	if err := json.Unmarshal([]byte(value), &setting); err != nil {
		return "", fmt.Errorf("decode JWT settings: %w", err)
	}
	return setting.TokenSecret, nil
}
