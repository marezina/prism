package detect

import "prism/internal/model"

// detectOpened: a new open PR authored by me appears in the watch set
// (SPEC §3.2 my_pr.opened). Runs only on baseline.
func detectOpened(d *diff) []candidate {
	if !d.mine || d.curr.State != model.PROpen {
		return nil
	}
	key := "opened:" + d.curr.ID
	return []candidate{{source: key, event: d.event(model.MyPROpened, key, d.curr.Author, "you opened a PR", "", d.curr.URL, d.curr.CreatedAt)}}
}
