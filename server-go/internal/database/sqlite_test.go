package database

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenSQLitePreservesExistingDatabase(t *testing.T) {
	// Spaces and '#' exercise URI escaping, including Windows drive paths.
	path := filepath.Join(t.TempDir(), "existing database #1.sqlite")
	fixture, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.Exec(`CREATE TABLE marker (value TEXT);
		INSERT INTO marker VALUES ('unchanged')`); err != nil {
		fixture.Close()
		t.Fatal(err)
	}
	var before string
	if err := fixture.QueryRow(`SELECT sql FROM sqlite_master WHERE name = 'marker'`).Scan(&before); err != nil {
		fixture.Close()
		t.Fatal(err)
	}
	if err := fixture.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := OpenSQLite(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	var after, value string
	if err := db.QueryRow(`SELECT sql FROM sqlite_master WHERE name = 'marker'`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT value FROM marker`).Scan(&value); err != nil {
		t.Fatal(err)
	}
	var tables int
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type = 'table'`).Scan(&tables); err != nil {
		t.Fatal(err)
	}
	if before != after || value != "unchanged" || tables != 1 {
		t.Fatalf("database changed: schema %q -> %q, value %q, tables %d", before, after, value, tables)
	}
}

func TestOpenSQLiteRejectsMissingDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.sqlite")
	db, err := OpenSQLite(context.Background(), path)
	if db != nil {
		db.Close()
	}
	if err == nil || db != nil {
		t.Fatal("expected an error and no connection for a missing database")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("missing database was created or could not be checked: %v", err)
	}
}
