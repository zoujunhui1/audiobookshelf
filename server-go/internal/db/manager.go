package db

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"sync"

	_ "github.com/mattn/go-sqlite3"
)

// Manager opens the existing Node database lazily. Node owns schema migrations.
// Each access checks the file and connection; replaced files or closed handles
// are reopened. SQL operations are short and serialized within this process.
type Manager struct {
	mu         sync.Mutex
	path       string
	connection *sql.DB
	file       os.FileInfo
	closed     bool
}

func NewManager(path string) (*Manager, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	return &Manager{path: absolute}, nil
}

func (m *Manager) access(ctx context.Context, operation func(*sql.DB) error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return errors.New("database manager is closed")
	}
	info, err := os.Stat(m.path)
	if err != nil {
		return err
	}
	if m.connection != nil && (m.file == nil || !os.SameFile(m.file, info) || m.connection.PingContext(ctx) != nil) {
		_ = m.connection.Close()
		m.connection = nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if m.connection == nil {
		u := url.URL{Scheme: "file", Path: m.path}
		q := u.Query()
		q.Set("mode", "rw") // Do not create a competing, empty database.
		q.Set("_busy_timeout", "5000")
		q.Set("_foreign_keys", "on")
		q.Set("_journal_mode", "WAL")
		q.Set("_txlock", "immediate")
		u.RawQuery = q.Encode()
		connection, err := sql.Open("sqlite3", u.String())
		if err != nil {
			return err
		}
		connection.SetMaxOpenConns(1)
		connection.SetMaxIdleConns(1)
		if err := connection.PingContext(ctx); err != nil {
			connection.Close()
			return err
		}
		m.connection, m.file = connection, info
	}
	return operation(m.connection)
}

func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	if m.connection != nil {
		return m.connection.Close()
	}
	return nil
}
