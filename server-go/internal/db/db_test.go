package db

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func testDatabase(t *testing.T) *Manager {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.sqlite")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	file.Close()
	m, err := NewManager(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.Close() })
	err = m.access(context.Background(), func(c *sql.DB) error {
		_, err := c.Exec(`CREATE TABLE users (id TEXT PRIMARY KEY, username TEXT, pash TEXT, type TEXT, isActive INTEGER);
		CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT);
		CREATE TABLE sessions (id TEXT PRIMARY KEY, userId TEXT REFERENCES users(id), ipAddress TEXT, userAgent TEXT,
		refreshToken TEXT, expiresAt DATETIME, createdAt DATETIME, updatedAt DATETIME);`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestCredentialRules(t *testing.T) {
	m := testDatabase(t)
	hash, err := bcrypt.GenerateFromPassword([]byte(strings.Repeat("p", 72)), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	err = m.access(context.Background(), func(c *sql.DB) error {
		_, err := c.Exec(`INSERT INTO users VALUES ('active', 'Alice', ?, 'user', 1),
		('inactive', 'disabled', ?, 'user', 0), ('root', 'root', '', 'root', 1), ('oidc', 'oidc', NULL, 'user', 1)`, string(hash), string(hash))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, password string
		valid          bool
	}{
		{"ALICE", strings.Repeat("p", 72), true},
		{"Alice", strings.Repeat("p", 72) + "truncated", true},
		{"Alice", "wrong", false}, {"disabled", strings.Repeat("p", 72), false},
		{"root", "", true}, {"root", "nonempty", false}, {"oidc", "", false},
		{"' OR 1=1 --", "", false}, {"missing", "", false},
	} {
		_, err := m.CheckCredentials(context.Background(), tc.name, tc.password)
		if tc.valid && err != nil || !tc.valid && !errors.Is(err, ErrInvalidCredentials) {
			t.Errorf("credential rule %s: %v", tc.name, err)
		}
	}
}

func TestConnectionRecoveryAndConfiguration(t *testing.T) {
	m := testDatabase(t)
	original := m.connection
	original.Close()
	if _, err := m.ReadAuthSettings(context.Background()); err != nil {
		t.Fatal(err)
	}
	if m.connection == original {
		t.Fatal("closed connection was not reopened")
	}
	err := m.access(context.Background(), func(c *sql.DB) error {
		var mode string
		var timeout, foreignKeys int
		if err := c.QueryRow(`PRAGMA journal_mode`).Scan(&mode); err != nil {
			return err
		}
		if err := c.QueryRow(`PRAGMA busy_timeout`).Scan(&timeout); err != nil {
			return err
		}
		if err := c.QueryRow(`PRAGMA foreign_keys`).Scan(&foreignKeys); err != nil {
			return err
		}
		if mode != "wal" || timeout != 5000 || foreignKeys != 1 {
			t.Fatalf("wrong connection settings: %s %d %d", mode, timeout, foreignKeys)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	m.Close()
	if _, err := m.ReadAuthSettings(context.Background()); err == nil {
		t.Fatal("closed manager reopened")
	}
}

func TestMissingDatabaseIsNotCreated(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.sqlite")
	m, err := NewManager(path)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if _, err := m.ReadAuthSettings(context.Background()); err == nil {
		t.Fatal("missing database accepted")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("Go created a database instead of using Node's")
	}
}

func TestSessionWriteRechecksActiveUser(t *testing.T) {
	m := testDatabase(t)
	err := m.access(context.Background(), func(c *sql.DB) error {
		_, err := c.Exec(`INSERT INTO users VALUES ('disabled', 'disabled', '', 'user', 0)`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	err = m.CreateSession(context.Background(), Session{ID: "session", UserID: "disabled", CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour)})
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("disabled user received session: %v", err)
	}
}
