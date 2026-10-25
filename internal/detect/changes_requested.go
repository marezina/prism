package detect

import "prism/internal/model"

// detectChangesRequested: new CHANGES_REQUESTED review on my PR
// (SPEC §3.1 my_pr.changes_requested).
func detectChangesRequested(d *diff) []candidate {
	return reviewsWithState(d, model.ReviewChangesRequested, model.MyPRChangesRequested, "requested changes on your PR")
}
