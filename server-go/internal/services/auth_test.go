package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"audiobookshelf-go/internal/db"
)

type payloadStub struct {
	token string
	fail  bool
}

func (p *payloadStub) LoginPayload(ctx context.Context, token string) (map[string]any, error) {
	p.token = token
	if p.fail {
		return nil, errors.New("Node unavailable")
	}
	return map[string]any{"user": map[string]any{"id": "user-id", "username": "alice"}}, nil
}

func TestLoginTokensAndSession(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.sqlite")
	connection, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	hash, _ := bcrypt.GenerateFromPassword([]byte("password"), bcrypt.MinCost)
	_, err = connection.Exec(`CREATE TABLE users (id TEXT PRIMARY KEY, username TEXT, pash TEXT, type TEXT, isActive INTEGER);
	CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT);
	CREATE TABLE sessions (id TEXT PRIMARY KEY, userId TEXT REFERENCES users(id), ipAddress TEXT, userAgent TEXT,
	refreshToken TEXT, expiresAt DATETIME, createdAt DATETIME, updatedAt DATETIME);
	INSERT INTO settings VALUES ('server-settings', '{"tokenSecret":"stored-secret","authActiveAuthMethods":["local"]}');`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := connection.Exec(`INSERT INTO users VALUES ('user-id','alice',?,'user',1)`, string(hash)); err != nil {
		t.Fatal(err)
	}
	manager, _ := db.NewManager(path)
	defer manager.Close()
	provider := &payloadStub{}
	auth := NewAuth(manager, provider, "", time.Hour, 30*24*time.Hour)
	result, err := auth.Login(context.Background(), "ALICE", "password", "127.0.0.1", "test-agent", true)
	if err != nil {
		t.Fatal(err)
	}
	access, err := jwt.Parse(provider.token, func(*jwt.Token) (any, error) { return []byte("stored-secret"), nil }, jwt.WithValidMethods([]string{"HS256"}))
	if err != nil || !access.Valid {
		t.Fatalf("invalid access token: %v", err)
	}
	claims := access.Claims.(jwt.MapClaims)
	if claims["type"] != "access" || claims["userId"] != "user-id" || claims["jti"] == nil || claims["exp"] == nil {
		t.Fatal("incompatible access claims")
	}
	refresh, err := jwt.Parse(result.RefreshToken, func(*jwt.Token) (any, error) { return []byte("stored-secret"), nil })
	if err != nil || refresh.Claims.(jwt.MapClaims)["type"] != "refresh" {
		t.Fatalf("invalid refresh token: %v", err)
	}
	var stored, agent string
	if err := connection.QueryRow(`SELECT refreshToken, userAgent FROM sessions`).Scan(&stored, &agent); err != nil {
		t.Fatal(err)
	}
	if stored != result.RefreshToken || agent != "test-agent" {
		t.Fatal("session was not saved")
	}
	if result.Payload["user"].(map[string]any)["refreshToken"] != result.RefreshToken {
		t.Fatal("mobile response omitted token")
	}
	provider.fail = true
	if _, err := auth.Login(context.Background(), "alice", "password", "", "", false); err == nil {
		t.Fatal("payload failure accepted")
	}
	var count int
	connection.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&count)
	if count != 1 {
		t.Fatal("failed login created an orphan session")
	}
	provider.token = ""
	if _, err := auth.Login(context.Background(), "alice", "wrong", "", "", false); !errors.Is(err, db.ErrInvalidCredentials) {
		t.Fatal(err)
	}
	if provider.token != "" {
		t.Fatal("invalid credentials reached Node")
	}
	methods, _ := json.Marshal(map[string]any{"authActiveAuthMethods": []string{"openid"}})
	connection.Exec(`UPDATE settings SET value = ?`, string(methods))
	if _, err := auth.Login(context.Background(), "alice", "password", "", "", false); !errors.Is(err, db.ErrInvalidCredentials) {
		t.Fatal("disabled local authentication accepted")
	}
}
