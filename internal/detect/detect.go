// Package detect turns two PR snapshots into user-visible events. It is pure:
// no network, no database, no clock (CLAUDE.md non-negotiable 2).
package detect

import (
	"cmp"
	"slices"
	"strconv"
	"strings"
	"time"

	"prism/internal/model"
)

// candidate is one possible event for one source object. Candidates with the
// same source collapse to the highest-precedence type (SPEC §3.1).
type candidate struct {
	source string // "review:<databaseId>" or "comment:<databaseId>"
	event  model.Event
}

// detector proposes candidates for one event type.
type detector func(d *diff) []candidate

// detectors run in this order; order does not affect the result.
var detectors = []detector{
	detectApproved,
	detectChangesRequested,
	detectComment,
	detectThreadReply,
	detectReplyToMe,
	detectClosed,
	detectReviewRequested,
	detectMention,
	detectCI,
	detectConflict,
	detectCommitsAfterMyReview,
}

// baselineDetectors are the only ones that run when a PR is first seen
// (SPEC §4.4): the request that made it enter, and my new PR.
var baselineDetectors = []detector{
	detectReviewRequestedBaseline,
	detectOpened,
}

// precedence: higher wins when candidates share a source (SPEC §3.1). Review
// events outrank everything that can share their review:<id> source.
var precedence = map[model.EventType]int{
	model.MyPRApproved:         5,
	model.MyPRChangesRequested: 5,
	model.CommentReplyToMe:     4,
	model.MyPRThreadReply:      3,
	model.MyPRComment:          2,
	model.Mention:              1,
}

// selfAllowed are the only events allowed for my own actions (SPEC §3.2).
var selfAllowed = map[model.EventType]bool{model.MyPROpened: true, model.MyPRClosed: true}

// Diff returns the events between two snapshots of one PR, sorted by
// OccurredAt then DedupKey. prev == nil or c.FirstSeen baselines the PR.
func Diff(prev, curr *model.PRSnapshot, c model.DetectContext) []model.Event {
	if curr == nil {
		return nil
	}
	dets := detectors
	if prev == nil || c.FirstSeen {
		prev, dets = &model.PRSnapshot{}, baselineDetectors
	}
	d := newDiff(prev, curr, c)

	best := map[string]candidate{}
	for _, det := range dets {
		for _, cand := range det(d) {
			if !d.allowed(cand.event) {
				continue
			}
			if cur, ok := best[cand.source]; !ok || precedence[cand.event.Type] > precedence[cur.event.Type] {
				best[cand.source] = cand
			}
		}
	}

	events := make([]model.Event, 0, len(best))
	for _, cand := range best {
		events = append(events, cand.event)
	}
	slices.SortFunc(events, func(a, b model.Event) int {
		if c := a.OccurredAt.Compare(b.OccurredAt); c != 0 {
			return c
		}
		return cmp.Compare(a.DedupKey, b.DedupKey)
	})
	return events
}

// diff holds both snapshots plus indexes the detectors share.
type diff struct {
	prev, curr *model.PRSnapshot
	c          model.DetectContext
	mine       bool // curr is my PR

	prevReviews   map[string]bool // submitted review ids
	prevComments  map[string]bool
	prevThreadCms map[string]bool
	newReviewByDB map[int64]model.Review // new submitted reviews by databaseId
	myBodies      []string               // my conversation comments and review bodies
	myTimes       []time.Time            // when I commented anywhere on the PR, sorted
}

func newDiff(prev, curr *model.PRSnapshot, c model.DetectContext) *diff {
	d := &diff{
		prev: prev, curr: curr, c: c,
		mine:          strings.EqualFold(curr.Author.Login, c.Me),
		prevReviews:   map[string]bool{},
		prevComments:  map[string]bool{},
		prevThreadCms: map[string]bool{},
		newReviewByDB: map[int64]model.Review{},
	}
	for _, r := range prev.Reviews {
		if submitted(r) {
			d.prevReviews[r.ID] = true
		}
	}
	for _, cm := range prev.Comments {
		d.prevComments[cm.ID] = true
	}
	for _, t := range prev.Threads {
		for _, cm := range t.Comments {
			d.prevThreadCms[cm.ID] = true
		}
	}
	for _, r := range d.newReviews() {
		d.newReviewByDB[r.DatabaseID] = r
	}

	for _, cm := range curr.Comments {
		if d.isMe(cm.Author) {
			d.myBodies = append(d.myBodies, cm.Body)
			d.myTimes = append(d.myTimes, cm.CreatedAt)
		}
	}
	for _, r := range curr.Reviews {
		if d.isMe(r.Author) && submitted(r) {
			if r.Body != "" {
				d.myBodies = append(d.myBodies, r.Body)
			}
			d.myTimes = append(d.myTimes, r.SubmittedAt)
		}
	}
	for _, t := range curr.Threads {
		for _, cm := range t.Comments {
			if d.isMe(cm.Author) {
				d.myTimes = append(d.myTimes, cm.CreatedAt)
			}
		}
	}
	slices.SortFunc(d.myTimes, time.Time.Compare)
	return d
}

func submitted(r model.Review) bool {
	return r.State != model.ReviewPending && r.State != ""
}

func (d *diff) isMe(a model.Actor) bool {
	return a.Login != "" && strings.EqualFold(a.Login, d.c.Me)
}

// myTeam reports whether "org/team" is one of my teams.
func (d *diff) myTeam(team string) bool {
	return slices.ContainsFunc(d.c.MyTeams, func(t string) bool { return strings.EqualFold(t, team) })
}

// allowed applies the global rules (SPEC §4.1–4.2).
func (d *diff) allowed(e model.Event) bool {
	if d.isMe(e.Actor) && !selfAllowed[e.Type] {
		return false
	}
	return !d.c.IgnoreBots || !e.Actor.IsBot
}

// newReviews are submitted reviews not seen as submitted before (SPEC §4.3).
func (d *diff) newReviews() []model.Review {
	var out []model.Review
	for _, r := range d.curr.Reviews {
		if submitted(r) && !d.prevReviews[r.ID] {
			out = append(out, r)
		}
	}
	return out
}

func (d *diff) newComments() []model.Comment {
	var out []model.Comment
	for _, cm := range d.curr.Comments {
		if !d.prevComments[cm.ID] {
			out = append(out, cm)
		}
	}
	return out
}

// threadComment is a thread comment with its position in the thread.
type threadComment struct {
	model.ThreadComment
	thread *model.Thread
	index  int
}

func (tc threadComment) isRoot() bool { return tc.index == 0 }

// newThreadComments are thread comments not seen before, in thread order.
func (d *diff) newThreadComments() []threadComment {
	var out []threadComment
	for ti := range d.curr.Threads {
		t := &d.curr.Threads[ti]
		for i, cm := range t.Comments {
			if !d.prevThreadCms[cm.ID] {
				out = append(out, threadComment{ThreadComment: cm, thread: t, index: i})
			}
		}
	}
	return out
}

// foldedInto returns the new review a thread root belongs to, if any
// (SPEC §3.1 clarification "one review submission → one review event").
func (d *diff) foldedInto(tc threadComment) (model.Review, bool) {
	if !tc.isRoot() || tc.ReviewDatabaseID == 0 {
		return model.Review{}, false
	}
	r, ok := d.newReviewByDB[tc.ReviewDatabaseID]
	return r, ok
}

// rootsOf returns the new thread roots folded into review r, oldest first.
func (d *diff) rootsOf(r model.Review) []threadComment {
	var out []threadComment
	for _, tc := range d.newThreadComments() {
		if fr, ok := d.foldedInto(tc); ok && fr.ID == r.ID {
			out = append(out, tc)
		}
	}
	slices.SortStableFunc(out, func(a, b threadComment) int { return a.CreatedAt.Compare(b.CreatedAt) })
	return out
}

// iCommentedBefore reports whether I commented on the PR before t.
func (d *diff) iCommentedBefore(t time.Time) bool {
	return len(d.myTimes) > 0 && d.myTimes[0].Before(t)
}

// event fills the PR fields of an event.
func (d *diff) event(typ model.EventType, key string, actor model.Actor, summary, excerpt, url string, at time.Time) model.Event {
	return model.Event{
		Type: typ, DedupKey: key, PRID: d.curr.ID, Repo: d.curr.Repo, PRNumber: d.curr.Number,
		PRTitle: d.curr.Title, Actor: actor, Summary: summary, Excerpt: excerpt, URL: url, OccurredAt: at,
	}
}

func reviewKey(r model.Review) string { return "review:" + strconv.FormatInt(r.DatabaseID, 10) }

func commentKey(id int64) string { return "comment:" + strconv.FormatInt(id, 10) }

// reviewEvent builds the single event for a review submission: excerpt and
// link come from the body, else from the first folded thread root.
func (d *diff) reviewEvent(typ model.EventType, r model.Review, summary string) model.Event {
	excerpt, url := Excerpt(r.Body), r.URL
	if roots := d.rootsOf(r); excerpt == "" && len(roots) > 0 {
		excerpt, url = Excerpt(roots[0].Body), roots[0].URL
	}
	return d.event(typ, reviewKey(r), r.Author, summary, excerpt, url, r.SubmittedAt)
}
