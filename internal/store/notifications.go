package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"prism/internal/model"
)

// Notification is a stored, unread event.
type Notification struct {
	ID int64
	model.Event
	CreatedAt time.Time
}

// Fresh is an event that was new to the store. ID is 0 when the event's
// "list" flag is off (it is tombstoned but not listed). Quiet events are
// listed only: no toast, no sound (first run, SPEC §4.5).
type Fresh struct {
	ID    int64
	Event model.Event
	Quiet bool
}

// ErrNotFound is returned for an unknown notification id.
var ErrNotFound = errors.New("store: notification not found")

// Record applies the insert rule (ARCHITECTURE §5): an event is new only if
// its dedup key is in neither notifications nor tombstones; every new key is
// tombstoned; it is listed only when list(type) is true. One transaction.
func (s *Store) Record(ctx context.Context, events []model.Event, list func(model.EventType) bool) ([]Fresh, error) {
	if len(events) == 0 {
		return nil, nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("record events: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := ts(s.now())
	var fresh []Fresh
	for _, e := range events {
		res, err := tx.ExecContext(ctx,
			`INSERT INTO tombstones(dedup_key, pr_id, created_at) VALUES(?, ?, ?) ON CONFLICT(dedup_key) DO NOTHING`,
			e.DedupKey, e.PRID, now)
		if err != nil {
			return nil, fmt.Errorf("tombstone %s: %w", e.DedupKey, err)
		}
		if n, _ := res.RowsAffected(); n == 0 {
			continue // seen before (listed, read, or list-disabled)
		}
		f := Fresh{Event: e}
		if list(e.Type) {
			res, err := tx.ExecContext(ctx, `
				INSERT INTO notifications(dedup_key, type, pr_id, repo, pr_number, pr_title, actor_login,
					actor_avatar, summary, excerpt, url, occurred_at, created_at)
				VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				e.DedupKey, string(e.Type), e.PRID, e.Repo, e.PRNumber, e.PRTitle, e.Actor.Login,
				e.Actor.AvatarURL, e.Summary, e.Excerpt, e.URL, ts(e.OccurredAt), now)
			if err != nil {
				return nil, fmt.Errorf("insert %s: %w", e.DedupKey, err)
			}
			if f.ID, err = res.LastInsertId(); err != nil {
				return nil, err
			}
		}
		fresh = append(fresh, f)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("record events: %w", err)
	}
	return fresh, nil
}

const notificationCols = `id, dedup_key, type, pr_id, repo, pr_number, pr_title, actor_login, actor_avatar,
	summary, excerpt, url, occurred_at, created_at`

func scanNotification(row interface{ Scan(...any) error }) (Notification, error) {
	var (
		n                   Notification
		typ, occurred, made string
		login, avatar, exc  sql.NullString
	)
	err := row.Scan(&n.ID, &n.DedupKey, &typ, &n.PRID, &n.Repo, &n.PRNumber, &n.PRTitle, &login, &avatar,
		&n.Summary, &exc, &n.URL, &occurred, &made)
	if err != nil {
		return n, err
	}
	n.Type = model.EventType(typ)
	n.Actor = model.Actor{Login: login.String, AvatarURL: avatar.String}
	n.Excerpt = exc.String
	n.OccurredAt = parseTS(occurred)
	n.CreatedAt = parseTS(made)
	return n, nil
}

func (s *Store) queryNotifications(ctx context.Context, where string, args ...any) ([]Notification, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+notificationCols+` FROM notifications `+where+` ORDER BY occurred_at DESC, id DESC`, args...)
	if err != nil {
		return nil, fmt.Errorf("read notifications: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []Notification
	for rows.Next() {
		n, err := scanNotification(rows)
		if err != nil {
			return nil, fmt.Errorf("scan notification: %w", err)
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// Unread returns every listed notification, newest first.
func (s *Store) Unread(ctx context.Context) ([]Notification, error) {
	return s.queryNotifications(ctx, "")
}

// ForPR returns the notifications of one PR, newest first.
func (s *Store) ForPR(ctx context.Context, prID string) ([]Notification, error) {
	return s.queryNotifications(ctx, "WHERE pr_id = ?", prID)
}

// Get returns one notification.
func (s *Store) Get(ctx context.Context, id int64) (Notification, error) {
	n, err := scanNotification(s.db.QueryRowContext(ctx, `SELECT `+notificationCols+` FROM notifications WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return n, ErrNotFound
	}
	return n, err
}

// UnreadCount is the tray badge number.
func (s *Store) UnreadCount(ctx context.Context) (int, error) {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM notifications`).Scan(&n); err != nil {
		return 0, fmt.Errorf("count notifications: %w", err)
	}
	return n, nil
}

// MarkRead deletes notifications (read = delete, SPEC D2). Their tombstones
// stay, so they never come back.
func (s *Store) MarkRead(ctx context.Context, ids ...int64) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	q := `DELETE FROM notifications WHERE id IN (?` + strings.Repeat(",?", len(ids)-1) + `)`
	return s.exec(ctx, q, args...)
}

// MarkPRRead deletes every notification of a PR.
func (s *Store) MarkPRRead(ctx context.Context, prID string) (int, error) {
	return s.exec(ctx, `DELETE FROM notifications WHERE pr_id = ?`, prID)
}

// MarkAllRead deletes every notification.
func (s *Store) MarkAllRead(ctx context.Context) (int, error) {
	return s.exec(ctx, `DELETE FROM notifications`)
}

func (s *Store) exec(ctx context.Context, q string, args ...any) (int, error) {
	res, err := s.db.ExecContext(ctx, q, args...)
	if err != nil {
		return 0, fmt.Errorf("mark read: %w", err)
	}
	n, err := res.RowsAffected()
	return int(n), err
}
