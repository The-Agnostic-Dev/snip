// Package store persists links and clicks in SQLite.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// ErrNotFound is returned when a link does not exist.
var ErrNotFound = errors.New("not found")

const timeLayout = "2006-01-02T15:04:05Z"

const schema = `
CREATE TABLE IF NOT EXISTS links (
	id          INTEGER PRIMARY KEY,
	alias       TEXT NOT NULL,
	target      TEXT NOT NULL,
	expires_on  TEXT,
	click_count INTEGER NOT NULL DEFAULT 0,
	created_at  TEXT NOT NULL,
	updated_at  TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_links_alias ON links(alias);
CREATE INDEX IF NOT EXISTS idx_links_created ON links(created_at);

CREATE TABLE IF NOT EXISTS clicks (
	id         INTEGER PRIMARY KEY,
	link_id    INTEGER NOT NULL REFERENCES links(id) ON DELETE CASCADE,
	clicked_at TEXT NOT NULL,
	referrer   TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_clicks_link_time ON clicks(link_id, clicked_at);
`

// Store wraps the SQLite database.
type Store struct {
	db *sql.DB
}

// Open opens (or creates) the database at path. Use ":memory:" for a throwaway database.
func Open(ctx context.Context, path string) (*Store, error) {
	dsn := path
	if path != ":memory:" {
		dsn = "file:" + path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	// SQLite allows one writer at a time, and an in-memory database only
	// exists on the connection that created it.
	db.SetMaxOpenConns(1)

	if _, err := db.ExecContext(ctx, schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &Store{db: db}, nil
}

// Close closes the database.
func (s *Store) Close() error {
	return s.db.Close()
}

func formatTime(t time.Time) string {
	return t.UTC().Format(timeLayout)
}

func parseTime(value string) (time.Time, error) {
	return time.Parse(timeLayout, value)
}
