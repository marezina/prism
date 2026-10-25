package model

import "time"

// Actor is a GitHub user, bot or mannequin. A deleted account ("ghost") has an
// empty Login.
type Actor struct {
	Login     string `json:"login"`
	AvatarURL string `json:"avatarUrl,omitempty"`
	IsBot     bool   `json:"isBot,omitempty"` // __typename == "Bot" or login ends in [bot]
}

// PRState is the pull request's state.
type PRState string

const (
	PROpen   PRState = "OPEN"
	PRClosed PRState = "CLOSED"
	PRMerged PRState = "MERGED"
)

// ReviewState mirrors GraphQL PullRequestReviewState.
type ReviewState string

const (
	ReviewApproved         ReviewState = "APPROVED"
	ReviewChangesRequested ReviewState = "CHANGES_REQUESTED"
	ReviewCommented        ReviewState = "COMMENTED"
	ReviewDismissed        ReviewState = "DISMISSED"
	ReviewPending          ReviewState = "PENDING"
)

// Review is a submitted (or pending) pull request review.
type Review struct {
	ID          string      `json:"id"`
	DatabaseID  int64       `json:"databaseId"`
	State       ReviewState `json:"state"`
	Body        string      `json:"body,omitempty"`
	URL         string      `json:"url"`
	SubmittedAt time.Time   `json:"submittedAt,omitzero"` // zero for PENDING
	Author      Actor       `json:"author"`
	CommitOID   string      `json:"commitOid,omitempty"`
}

// Comment is a conversation (issue) comment on the PR.
type Comment struct {
	ID         string    `json:"id"`
	DatabaseID int64     `json:"databaseId"`
	Body       string    `json:"body"`
	URL        string    `json:"url"`
	CreatedAt  time.Time `json:"createdAt"`
	Author     Actor     `json:"author"`
}

// ThreadComment is a review-thread (inline) comment.
type ThreadComment struct {
	Comment
	ReplyToID        string `json:"replyToId,omitempty"`        // node id of the comment replied to
	ReviewDatabaseID int64  `json:"reviewDatabaseId,omitempty"` // review the comment was submitted with
}

// Thread is a review thread on a diff line.
type Thread struct {
	ID         string          `json:"id"`
	IsResolved bool            `json:"isResolved,omitempty"`
	Path       string          `json:"path"`
	Comments   []ThreadComment `json:"comments"` // oldest first
}

// Reviewer is a requested reviewer: a user login or a team "org/slug".
type Reviewer struct {
	Login string `json:"login,omitempty"`
	Team  string `json:"team,omitempty"`
}

// TimelineType names the timeline items PRism fetches.
type TimelineType string

const (
	TimelineReviewRequested    TimelineType = "REVIEW_REQUESTED"
	TimelineHeadRefForcePushed TimelineType = "HEAD_REF_FORCE_PUSHED"
)

// TimelineEvent is a review request or force push.
type TimelineEvent struct {
	Type              TimelineType `json:"type"`
	ID                string       `json:"id"`
	CreatedAt         time.Time    `json:"createdAt"`
	Actor             Actor        `json:"actor"`
	RequestedReviewer *Reviewer    `json:"requestedReviewer,omitempty"` // REVIEW_REQUESTED
	AfterOID          string       `json:"afterOid,omitempty"`          // HEAD_REF_FORCE_PUSHED
}

// CheckState is the normalized state of one check or the whole rollup
// (GraphQL StatusState: SUCCESS, FAILURE, ERROR, PENDING, EXPECTED).
type CheckState string

const (
	CheckSuccess  CheckState = "SUCCESS"
	CheckFailure  CheckState = "FAILURE"
	CheckError    CheckState = "ERROR"
	CheckPending  CheckState = "PENDING"
	CheckExpected CheckState = "EXPECTED"
)

// Check is one CheckRun or StatusContext of the head commit.
type Check struct {
	Name  string     `json:"name"`
	State CheckState `json:"state"`
	URL   string     `json:"url,omitempty"`
	At    time.Time  `json:"at,omitzero"` // completedAt (CheckRun) / createdAt (StatusContext)
}

// CI is the head commit's statusCheckRollup. A nil *CI means the PR has no CI.
type CI struct {
	State  CheckState `json:"state"`
	Checks []Check    `json:"checks,omitempty"`
}

// PRSnapshot is everything PRism knows about one PR at one poll. The previous
// snapshot is stored as JSON in pr_state.snapshot (ARCHITECTURE §4–5).
type PRSnapshot struct {
	ID        string     `json:"id"` // GraphQL node id
	Repo      string     `json:"repo"`
	Number    int        `json:"number"`
	Title     string     `json:"title"`
	URL       string     `json:"url"`
	Author    Actor      `json:"author"`
	IsDraft   bool       `json:"isDraft,omitempty"`
	State     PRState    `json:"state"`
	CreatedAt time.Time  `json:"createdAt,omitzero"`
	UpdatedAt time.Time  `json:"updatedAt,omitzero"`
	ClosedAt  *time.Time `json:"closedAt,omitempty"`

	HeadOID          string `json:"headOid"`
	BaseOID          string `json:"baseOid"`
	Mergeable        string `json:"mergeable"`        // MERGEABLE | CONFLICTING | UNKNOWN
	MergeStateStatus string `json:"mergeStateStatus"` // CLEAN, DIRTY, BLOCKED, …
	CommitsTotal     int    `json:"commitsTotal"`
	CI               *CI    `json:"ci,omitempty"`

	Reviews        []Review        `json:"reviews"`        // oldest first
	Comments       []Comment       `json:"comments"`       // oldest first
	Threads        []Thread        `json:"threads"`        // oldest first
	ReviewRequests []Reviewer      `json:"reviewRequests"` // pending requests now
	Timeline       []TimelineEvent `json:"timeline"`       // oldest first
}
