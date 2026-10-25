package detect

import "prism/internal/model"

const conflicting = "CONFLICTING"

// detectConflict: my PR's mergeable becomes CONFLICTING for this head and base
// (SPEC §3.2 merge.conflict). UNKNOWN is GitHub still computing: it never
// emits and never clears a conflict; the poller re-polls such PRs after 30 s.
func detectConflict(d *diff) []candidate {
	if !d.mine || d.curr.Mergeable != conflicting {
		return nil
	}
	if d.prev.Mergeable == conflicting && d.prev.HeadOID == d.curr.HeadOID && d.prev.BaseOID == d.curr.BaseOID {
		return nil
	}
	key := "conflict:" + d.curr.ID + ":" + d.curr.HeadOID + ":" + d.curr.BaseOID
	return []candidate{{source: key, event: d.event(model.MergeConflict, key, model.Actor{}, "has merge conflicts", "", d.curr.URL, d.curr.UpdatedAt)}}
}
