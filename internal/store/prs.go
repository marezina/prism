package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"prism/internal/model"
)

// Role of a watched PR in pr_state.
const (
	RoleMine  = "mine"
	RoleOther = "other"
)

// PRState is a row of pr_state: the previous snapshot of a watched PR.
type PRState struct {
	PRID      string
	Repo      string
	Number    int
	Role      string
	Snapshot  *model.PRSnapshot
	UpdatedAt time.Time
}

func jsonMarshal(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("encode: %w", err)
	}
	return string(b), nil
}

// PRStates returns every watched PR by node id.
func (s *Store) PRStates(ctx context.Context) (map[string]PRState, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT pr_id, repo, number, role, snapshot, updated_at FROM pr_state`)
	if err != nil {
		return nil, fmt.Errorf("read pr_state: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]PRState{}
	for rows.Next() {
		var (
			st       PRState
			snap, up string
		)
		if err := rows.Scan(&st.PRID, &st.Repo, &st.Number, &st.Role, &snap, &up); err != nil {
			return nil, fmt.Errorf("scan pr_state: %w", err)
		}
		st.UpdatedAt = parseTS(up)
		if err := json.Unmarshal([]byte(snap), &st.Snapshot); err != nil {
			return nil, fmt.Errorf("decode snapshot of %s: %w", st.PRID, err)
		}
		out[st.PRID] = st
	}
	return out, rows.Err()
}

// PutPRState stores the latest snapshot of a PR.
func (s *Store) PutPRState(ctx context.Context, st PRState) error {
	snap, err := jsonMarshal(st.Snapshot)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO pr_state(pr_id, repo, number, role, snapshot, updated_at) VALUES(?, ?, ?, ?, ?, ?)
		ON CONFLICT(pr_id) DO UPDATE SET repo = excluded.repo, number = excluded.number,
			role = excluded.role, snapshot = excluded.snapshot, updated_at = excluded.updated_at`,
		st.PRID, st.Repo, st.Number, st.Role, snap, ts(s.now()))
	if err != nil {
		return fmt.Errorf("write pr_state %s: %w", st.PRID, err)
	}
	return nil
}

// PurgePR deletes a PR's state, notifications and tombstones (SPEC §5: the PR
// left the open set or its owner was unselected).
func (s *Store) PurgePR(ctx context.Context, prID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("purge %s: %w", prID, err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, q := range []string{
		`DELETE FROM pr_state WHERE pr_id = ?`,
		`DELETE FROM notifications WHERE pr_id = ?`,
		`DELETE FROM tombstones WHERE pr_id = ?`,
	} {
		if _, err := tx.ExecContext(ctx, q, prID); err != nil {
			return fmt.Errorf("purge %s: %w", prID, err)
		}
	}
	return tx.Commit()
}
