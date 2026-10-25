package detect

import "prism/internal/model"

// detectApproved: new APPROVED review on my PR (SPEC §3.1 my_pr.approved).
func detectApproved(d *diff) []candidate {
	return reviewsWithState(d, model.ReviewApproved, model.MyPRApproved, "approved your PR")
}

// reviewsWithState emits one review event per new review in state on my PR.
func reviewsWithState(d *diff, state model.ReviewState, typ model.EventType, summary string) []candidate {
	if !d.mine {
		return nil
	}
	var out []candidate
	for _, r := range d.newReviews() {
		if r.State == state {
			out = append(out, candidate{source: reviewKey(r), event: d.reviewEvent(typ, r, summary)})
		}
	}
	return out
}
