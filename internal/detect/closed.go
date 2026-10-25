package detect

import (
	"time"

	"prism/internal/model"
)

// detectClosed: my PR went from open to merged or closed (SPEC §3.2
// my_pr.closed). The poller diffs the final snapshot of a PR that left the
// open set; the actor is unknown, so the event has none.
func detectClosed(d *diff) []candidate {
	if !d.mine || d.prev.State != model.PROpen {
		return nil
	}
	summary := ""
	switch d.curr.State {
	case model.PRMerged:
		summary = "your PR was merged"
	case model.PRClosed:
		summary = "your PR was closed"
	default:
		return nil
	}
	var at time.Time
	if d.curr.ClosedAt != nil {
		at = *d.curr.ClosedAt
	}
	key := "closed:" + d.curr.ID
	return []candidate{{source: key, event: d.event(model.MyPRClosed, key, model.Actor{}, summary, "", d.curr.URL, at)}}
}
