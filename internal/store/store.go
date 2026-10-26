// Package store persists notifications, tombstones, PR state and settings in
// SQLite (ARCHITECTURE §5).
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"time"

	_ "modernc.org/sqlite" // database/sql driver "sqlite"

	"prism/internal/settings"
)

// Store is PRism's local database. All GitHub data stays here (CLAUDE.md §5).
type Store struct {
	db  *sql.DB
	now func() time.Time
}

// DataDir is os.UserConfigDir()/PRism (ARCHITECTURE §6).
func DataDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("user config dir: %w", err)
	}
	return filepath.Join(dir, "PRism"), nil
}

// Open opens (and migrates) the database at path, creating its directory.
func Open(ctx context.Context, path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	dsn := "file:" + (&url.URL{Path: path}).EscapedPath() +
		"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)&_txlock=immediate"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	db.SetMaxOpenConns(1) // one writer; keeps SQLite simple and serialized
	s := &Store{db: db, now: time.Now}
	if err := s.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

// migrations[i] moves the schema from version i to i+1.
var migrations = []string{
	`CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);
	CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT NOT NULL);
	CREATE TABLE pr_state (
		pr_id      TEXT PRIMARY KEY,
		repo       TEXT NOT NULL,
		number     INTEGER NOT NULL,
		role       TEXT NOT NULL,
		snapshot   TEXT NOT NULL,
		updated_at TEXT NOT NULL
	);
	CREATE TABLE notifications (
		id           INTEGER PRIMARY KEY,
		dedup_key    TEXT NOT NULL UNIQUE,
		type         TEXT NOT NULL,
		pr_id        TEXT NOT NULL,
		repo         TEXT NOT NULL,
		pr_number    INTEGER NOT NULL,
		pr_title     TEXT NOT NULL,
		actor_login  TEXT,
		actor_avatar TEXT,
		summary      TEXT NOT NULL,
		excerpt      TEXT,
		url          TEXT NOT NULL,
		occurred_at  TEXT NOT NULL,
		created_at   TEXT NOT NULL
	);
	CREATE INDEX notifications_pr ON notifications(pr_id);
	CREATE TABLE tombstones (
		dedup_key  TEXT PRIMARY KEY,
		pr_id      TEXT NOT NULL,
		created_at TEXT NOT NULL
	);
	CREATE INDEX tombstones_pr ON tombstones(pr_id);
	CREATE TABLE avatars (login TEXT PRIMARY KEY, path TEXT NOT NULL, etag TEXT, fetched_at TEXT NOT NULL);`,
	// v2: a muted PR records no events until it is unmuted or leaves the watch set.
	`ALTER TABLE pr_state ADD COLUMN muted INTEGER NOT NULL DEFAULT 0;`,
}

func (s *Store) migrate(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var version int
	if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	var v string
	err = tx.QueryRowContext(ctx, `SELECT value FROM meta WHERE key = 'schema_version'`).Scan(&v)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		// fresh database; the first migration creates meta again, so drop the bootstrap table
		if _, err := tx.ExecContext(ctx, `DROP TABLE meta`); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	case err != nil:
		return fmt.Errorf("migrate: read version: %w", err)
	default:
		if version, err = strconv.Atoi(v); err != nil {
			return fmt.Errorf("migrate: bad schema_version %q", v)
		}
	}
	if version > len(migrations) {
		return fmt.Errorf("database schema v%d is newer than this PRism (v%d)", version, len(migrations))
	}
	for ; version < len(migrations); version++ {
		if _, err := tx.ExecContext(ctx, migrations[version]); err != nil {
			return fmt.Errorf("migrate to v%d: %w", version+1, err)
		}
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO meta(key, value) VALUES('schema_version', ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		strconv.Itoa(version)); err != nil {
		return fmt.Errorf("migrate: write version: %w", err)
	}
	return tx.Commit()
}

// Meta keys (ARCHITECTURE §5).
const (
	MetaNotificationsLastModified = "notifications_last_modified"
	MetaMeLogin                   = "me_login"
	MetaBaselineDone              = "baseline_done"
)

// Meta returns a meta value, or "" when unset.
func (s *Store) Meta(ctx context.Context, key string) (string, error) {
	var v string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read meta %s: %w", key, err)
	}
	return v, nil
}

// SetMeta stores a meta value.
func (s *Store) SetMeta(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO meta(key, value) VALUES(?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	if err != nil {
		return fmt.Errorf("write meta %s: %w", key, err)
	}
	return nil
}

const settingsKey = "app"

// Settings returns the stored settings merged over the defaults.
func (s *Store) Settings(ctx context.Context) (settings.Settings, error) {
	var v string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, settingsKey).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return settings.Default(), nil
	}
	if err != nil {
		return settings.Settings{}, fmt.Errorf("read settings: %w", err)
	}
	return settings.Decode([]byte(v))
}

// SaveSettings validates and stores the settings.
func (s *Store) SaveSettings(ctx context.Context, st settings.Settings) error {
	if err := st.Validate(); err != nil {
		return err
	}
	b, err := jsonMarshal(st)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO settings(key, value) VALUES(?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, settingsKey, b)
	if err != nil {
		return fmt.Errorf("write settings: %w", err)
	}
	return nil
}

func ts(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

func parseTS(s string) time.Time {
	t, _ := time.Parse(time.RFC3339Nano, s)
	return t
}

// Reset deletes all local data except the schema: notifications, tombstones,
// PR state, avatars metadata, settings and meta ("Reset local data").
func (s *Store) Reset(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("reset: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, q := range []string{
		`DELETE FROM notifications`, `DELETE FROM tombstones`, `DELETE FROM pr_state`,
		`DELETE FROM avatars`, `DELETE FROM settings`, `DELETE FROM meta WHERE key <> 'schema_version'`,
	} {
		if _, err := tx.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("reset: %w", err)
		}
	}
	return tx.Commit()
}
