package repository

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"audiobookshelf-go/internal/database"
)

func TestGetByUsername(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absdatabase.sqlite")
	fixture, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.Exec(`CREATE TABLE users (
		id UUID PRIMARY KEY, username VARCHAR(255), pash VARCHAR(255),
		type VARCHAR(255), isActive TINYINT(1)
	);
	INSERT INTO users VALUES ('user-id', 'Ryan', 'stored-bcrypt-hash', 'user', 1);
	INSERT INTO users VALUES ('root-id', 'root', NULL, 'root', 1);
	INSERT INTO users VALUES ('inactive-id', 'inactive', 'inactive-hash', 'user', 0);`); err != nil {
		fixture.Close()
		t.Fatal(err)
	}
	if err := fixture.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := database.OpenSQLite(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repo := NewUserRepository(db)

	for _, tc := range []struct {
		name string
		want *User
	}{
		{"rYaN", &User{"user-id", "Ryan", "stored-bcrypt-hash", "user", true}},
		{"ROOT", &User{"root-id", "root", "", "root", true}},
		{"inactive", &User{"inactive-id", "inactive", "inactive-hash", "user", false}},
		{"missing", nil},
		{"", nil},
		{" Ryan ", nil},
		{"' OR 1=1 --", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := repo.GetByUsername(context.Background(), tc.name)
			if err != nil {
				t.Fatal(err)
			}
			if tc.want == nil {
				if got != nil {
					t.Fatalf("expected no user, got %+v", got)
				}
				return
			}
			if got == nil || *got != *tc.want {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if user, err := repo.GetByUsername(context.Background(), "Ryan"); err == nil || user != nil {
		t.Fatalf("database failure must not become a missing user: user=%+v err=%v", user, err)
	}
}
