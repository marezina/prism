package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Avatar is a cached avatar file (ARCHITECTURE §5 avatars).
type Avatar struct {
	Login     string
	Path      string
	ETag      string
	FetchedAt time.Time
}

// Avatar returns the cached avatar of login; ok is false when none is cached.
func (s *Store) Avatar(ctx context.Context, login string) (Avatar, bool, error) {
	var (
		a       Avatar
		etag    sql.NullString
		fetched string
	)
	err := s.db.QueryRowContext(ctx, `SELECT login, path, etag, fetched_at FROM avatars WHERE login = ?`, login).
		Scan(&a.Login, &a.Path, &etag, &fetched)
	if errors.Is(err, sql.ErrNoRows) {
		return Avatar{}, false, nil
	}
	if err != nil {
		return Avatar{}, false, fmt.Errorf("read avatar %s: %w", login, err)
	}
	a.ETag, a.FetchedAt = etag.String, parseTS(fetched)
	return a, true, nil
}

// PutAvatar records a cached avatar.
func (s *Store) PutAvatar(ctx context.Context, a Avatar) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO avatars(login, path, etag, fetched_at) VALUES(?, ?, ?, ?)
		ON CONFLICT(login) DO UPDATE SET path = excluded.path, etag = excluded.etag, fetched_at = excluded.fetched_at`,
		a.Login, a.Path, a.ETag, ts(a.FetchedAt))
	if err != nil {
		return fmt.Errorf("write avatar %s: %w", a.Login, err)
	}
	return nil
}
