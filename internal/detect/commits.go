package detect

import (
	"time"

	"prism/internal/model"
)

// detectCommitsAfterMyReview: the head of someone else's PR moved after my
// latest submitted review, including force-pushes (SPEC §3.2
// commits.after_my_review). The link shows the changes since my review.
func detectCommitsAfterMyReview(d *diff) []candidate {
	if d.mine || d.prev.HeadOID == "" || d.prev.HeadOID == d.curr.HeadOID {
		return nil
	}
	var mine *model.Review
	for i := range d.curr.Reviews {
		r := &d.curr.Reviews[i]
		if d.isMe(r.Author) && submitted(*r) && (mine == nil || r.SubmittedAt.After(mine.SubmittedAt)) {
			mine = r
		}
	}
	if mine == nil || mine.CommitOID == d.curr.HeadOID {
		return nil
	}

	actor, summary, at := d.curr.Author, "pushed new commits after your review", time.Time{}
	for _, e := range d.curr.Timeline {
		if e.Type == model.TimelineHeadRefForcePushed && e.AfterOID == d.curr.HeadOID {
			actor, summary, at = e.Actor, "force-pushed after your review", e.CreatedAt
		}
	}
	key := "commits:" + d.curr.ID + ":" + d.curr.HeadOID
	url := d.curr.URL + "/files/" + mine.CommitOID + ".." + d.curr.HeadOID
	return []candidate{{source: key, event: d.event(model.CommitsAfterMyReview, key, actor, summary, "", url, d.orUpdated(at))}}
}
