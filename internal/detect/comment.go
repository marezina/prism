package detect

import (
	"fmt"

	"prism/internal/model"
)

const commentedSummary = "commented on your PR"

// detectComment: new top-level comment on my PR (SPEC §3.1 my_pr.comment) —
// a conversation comment, a COMMENTED review (body and/or new threads folded
// into it), or a new thread root that belongs to no new review.
func detectComment(d *diff) []candidate {
	if !d.mine {
		return nil
	}
	var out []candidate
	for _, cm := range d.newComments() {
		out = append(out, candidate{
			source: commentKey(cm.DatabaseID),
			event: d.event(model.MyPRComment, commentKey(cm.DatabaseID), cm.Author, commentedSummary,
				Excerpt(cm.Body), cm.URL, cm.CreatedAt),
		})
	}
	for _, r := range d.newReviews() {
		if r.State != model.ReviewCommented {
			continue
		}
		roots := d.rootsOf(r)
		if r.Body == "" && len(roots) == 0 {
			continue // only replies: they get their own events
		}
		summary := commentedSummary
		if len(roots) >= 2 {
			summary = fmt.Sprintf("left %d comments on your PR", len(roots))
		}
		out = append(out, candidate{source: reviewKey(r), event: d.reviewEvent(model.MyPRComment, r, summary)})
	}
	for _, tc := range d.newThreadComments() {
		if !tc.isRoot() {
			continue
		}
		if _, folded := d.foldedInto(tc); folded {
			continue
		}
		out = append(out, candidate{
			source: commentKey(tc.DatabaseID),
			event: d.event(model.MyPRComment, commentKey(tc.DatabaseID), tc.Author, commentedSummary,
				Excerpt(tc.Body), tc.URL, tc.CreatedAt),
		})
	}
	return out
}
