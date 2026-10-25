package model

import "time"

// EventType is a stable event id used in code, settings and the DB (SPEC §3).
type EventType string

// CORE events (SPEC §3.1) — must never regress.
const (
	MyPRApproved         EventType = "my_pr.approved"
	MyPRChangesRequested EventType = "my_pr.changes_requested"
	MyPRComment          EventType = "my_pr.comment"
	MyPRThreadReply      EventType = "my_pr.thread_reply"
	CommentReplyToMe     EventType = "comment.reply_to_me"
)

// Secondary events (SPEC §3.2).
const (
	ReviewRequested      EventType = "review.requested"
	Mention              EventType = "mention"
	CIFailed             EventType = "ci.failed"
	CIPassed             EventType = "ci.passed"
	MergeConflict        EventType = "merge.conflict"
	CommitsAfterMyReview EventType = "commits.after_my_review"
	MyPRClosed           EventType = "my_pr.closed"
	MyPROpened           EventType = "my_pr.opened"
)

// AllEventTypes lists every event in SPEC order.
var AllEventTypes = []EventType{
	MyPRApproved, MyPRChangesRequested, MyPRComment, MyPRThreadReply, CommentReplyToMe,
	ReviewRequested, Mention, CIFailed, CIPassed, MergeConflict, CommitsAfterMyReview,
	MyPRClosed, MyPROpened,
}

// Event is one user-visible notification (ARCHITECTURE §4).
type Event struct {
	Type       EventType `json:"type"`
	DedupKey   string    `json:"dedupKey"`
	PRID       string    `json:"prId"`
	Repo       string    `json:"repo"`
	PRNumber   int       `json:"prNumber"`
	PRTitle    string    `json:"prTitle"`
	Actor      Actor     `json:"actor"`
	Summary    string    `json:"summary"`           // "approved your PR"
	Excerpt    string    `json:"excerpt,omitempty"` // ≤ 200 chars, markdown stripped
	URL        string    `json:"url"`               // deep link to the comment / review
	OccurredAt time.Time `json:"occurredAt"`        // GitHub timestamp
}

// DetectContext is everything detect.Diff needs besides the two snapshots.
type DetectContext struct {
	Me         string   `json:"me"`
	MyTeams    []string `json:"teams,omitempty"` // "org/team"
	IgnoreBots bool     `json:"ignoreBots"`
	FirstSeen  bool     `json:"firstSeen"` // PR just entered the watch set → baseline
}
