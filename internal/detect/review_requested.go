package detect

import (
	"strings"

	"prism/internal/model"
)

// detectReviewRequested: I (or a team I belong to) am newly requested as
// reviewer (SPEC §3.2 review.requested). A new REVIEW_REQUESTED timeline item
// counts only while the request is still pending, so a request removed again
// between two polls stays silent; a re-request is a new timeline item.
func detectReviewRequested(d *diff) []candidate {
	seen := map[string]bool{}
	for _, e := range d.prev.Timeline {
		seen[e.ID] = true
	}
	pending := d.pendingRequests()
	var out []candidate
	for _, e := range d.curr.Timeline {
		if e.Type != model.TimelineReviewRequested || seen[e.ID] || e.RequestedReviewer == nil {
			continue
		}
		r := *e.RequestedReviewer
		if !d.forMe(r) || !pending[reviewerKey(r)] {
			continue
		}
		out = append(out, d.reviewRequest("revreq:"+e.ID, e.Actor, r, e))
	}
	return out
}

// detectReviewRequestedBaseline: a PR seen for the first time where I am
// requested now — usually the reason it entered the watch set (SPEC §4.4).
// Uses the newest matching timeline item; if the request is older than the
// fetched timeline, the key falls back to revreq:<prId>:<reviewer>.
func detectReviewRequestedBaseline(d *diff) []candidate {
	var out []candidate
	for _, r := range d.curr.ReviewRequests {
		if !d.forMe(r) {
			continue
		}
		var latest *model.TimelineEvent
		for i := range d.curr.Timeline {
			e := &d.curr.Timeline[i]
			if e.Type == model.TimelineReviewRequested && e.RequestedReviewer != nil && reviewerKey(*e.RequestedReviewer) == reviewerKey(r) {
				latest = e
			}
		}
		if latest != nil {
			out = append(out, d.reviewRequest("revreq:"+latest.ID, latest.Actor, r, *latest))
			continue
		}
		key := "revreq:" + d.curr.ID + ":" + reviewerKey(r)
		out = append(out, d.reviewRequest(key, model.Actor{}, r, model.TimelineEvent{CreatedAt: d.curr.UpdatedAt}))
	}
	return out
}

func (d *diff) reviewRequest(key string, actor model.Actor, r model.Reviewer, e model.TimelineEvent) candidate {
	summary := "requested your review"
	if r.Team != "" {
		summary = "requested a review from " + r.Team
	}
	return candidate{source: key, event: d.event(model.ReviewRequested, key, actor, summary, "", d.curr.URL, e.CreatedAt)}
}

func (d *diff) forMe(r model.Reviewer) bool {
	if r.Team != "" {
		return d.myTeam(r.Team)
	}
	return strings.EqualFold(r.Login, d.c.Me)
}

func (d *diff) pendingRequests() map[string]bool {
	out := map[string]bool{}
	for _, r := range d.curr.ReviewRequests {
		out[reviewerKey(r)] = true
	}
	return out
}

func reviewerKey(r model.Reviewer) string {
	if r.Team != "" {
		return strings.ToLower(r.Team)
	}
	return strings.ToLower(r.Login)
}
