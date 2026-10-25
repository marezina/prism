// Package queries holds PRism's GraphQL queries as githubv4 structs. They
// mirror .claude/skills/github-api/references/queries.graphql; change both
// together.
package queries

import "github.com/shurcooL/githubv4"

// PageSize is the connection page size used throughout (queries.graphql).
const PageSize = 50

// RateLimit is requested by every query.
type RateLimit struct {
	Cost      int
	Remaining int
	ResetAt   githubv4.DateTime
}

// Actor is fragment Actor.
type Actor struct {
	Typename  string `graphql:"__typename"`
	Login     string
	AvatarURL string `graphql:"avatarUrl(size: 96)"`
}

// BackPage is pageInfo for connections read newest-first with last/before.
type BackPage struct {
	HasPreviousPage bool
	StartCursor     githubv4.String
}

// ForwardPage is pageInfo for connections read with first/after.
type ForwardPage struct {
	HasNextPage bool
	EndCursor   githubv4.String
}

// Reviewer is the RequestedReviewer union (User | Team; Bot/Mannequin ignored).
type Reviewer struct {
	Typename string `graphql:"__typename"`
	User     struct {
		Login string
	} `graphql:"... on User"`
	Team struct {
		Slug         string
		Organization struct{ Login string }
	} `graphql:"... on Team"`
}

type Review struct {
	ID          string
	DatabaseID  int64
	State       string
	Body        string
	URL         string
	SubmittedAt *githubv4.DateTime
	Author      *Actor
	Commit      *struct{ Oid string }
}

type Comment struct {
	ID         string
	DatabaseID int64
	Body       string
	URL        string
	CreatedAt  githubv4.DateTime
	Author     *Actor
}

type ThreadComment struct {
	ID                string
	DatabaseID        int64
	Body              string
	URL               string
	CreatedAt         githubv4.DateTime
	Author            *Actor
	ReplyTo           *struct{ ID string }
	PullRequestReview *struct{ DatabaseID int64 }
}

type ThreadComments struct {
	PageInfo ForwardPage
	Nodes    []ThreadComment
}

type Thread struct {
	ID         string
	IsResolved bool
	Path       string
	Comments   ThreadComments `graphql:"comments(first: 50)"`
}

type CheckContext struct {
	Typename string `graphql:"__typename"`
	CheckRun struct {
		Name        string
		Conclusion  string
		Status      string
		DetailsURL  string `graphql:"detailsUrl"`
		CompletedAt *githubv4.DateTime
	} `graphql:"... on CheckRun"`
	StatusContext struct {
		Context   string
		State     string
		TargetURL string `graphql:"targetUrl"`
		CreatedAt githubv4.DateTime
	} `graphql:"... on StatusContext"`
}

type TimelineItem struct {
	Typename        string `graphql:"__typename"`
	ReviewRequested struct {
		ID                string
		CreatedAt         githubv4.DateTime
		Actor             *Actor
		RequestedReviewer *Reviewer
	} `graphql:"... on ReviewRequestedEvent"`
	ForcePushed struct {
		ID          string
		CreatedAt   githubv4.DateTime
		Actor       *Actor
		AfterCommit *struct{ Oid string }
	} `graphql:"... on HeadRefForcePushedEvent"`
}

// PullRequest is fragment PRSnapshot.
type PullRequest struct {
	ID         string
	Number     int
	Title      string
	URL        string
	State      string
	IsDraft    bool
	CreatedAt  githubv4.DateTime
	UpdatedAt  githubv4.DateTime
	ClosedAt   *githubv4.DateTime
	Repository struct {
		NameWithOwner string
	}
	Author           *Actor
	HeadRefOid       string
	BaseRefOid       string
	Mergeable        string
	MergeStateStatus string
	Commits          struct {
		TotalCount int
		Nodes      []struct {
			Commit struct {
				Oid               string
				StatusCheckRollup *struct {
					State    string
					Contexts struct {
						Nodes []CheckContext
					} `graphql:"contexts(first: 50)"`
				}
			}
		}
	} `graphql:"commits(last: 1)"`
	Reviews struct {
		PageInfo BackPage
		Nodes    []Review
	} `graphql:"reviews(last: 50)"`
	Comments struct {
		PageInfo BackPage
		Nodes    []Comment
	} `graphql:"comments(last: 50)"`
	ReviewThreads struct {
		PageInfo BackPage
		Nodes    []Thread
	} `graphql:"reviewThreads(last: 50)"`
	ReviewRequests struct {
		Nodes []struct {
			RequestedReviewer *Reviewer
		}
	} `graphql:"reviewRequests(first: 20)"`
	TimelineItems struct {
		Nodes []TimelineItem
	} `graphql:"timelineItems(last: 30, itemTypes: [REVIEW_REQUESTED_EVENT, HEAD_REF_FORCE_PUSHED_EVENT])"`
}

// Snapshots is query 2: PRs by node id (≤ 50 ids; variable $ids).
type Snapshots struct {
	RateLimit RateLimit
	Nodes     []*struct {
		PullRequest PullRequest `graphql:"... on PullRequest"`
	} `graphql:"nodes(ids: $ids)"`
}

// PRByNumber resolves owner/repo#n to a node id (variables $owner $name $number).
type PRByNumber struct {
	RateLimit  RateLimit
	Repository *struct {
		PullRequest *struct{ ID string } `graphql:"pullRequest(number: $number)"`
	} `graphql:"repository(owner: $owner, name: $name)"`
}

// The page queries below fetch older items of one connection when a PR has
// more than PageSize of them (variables $id, $before / $after).

type ReviewsPage struct {
	RateLimit RateLimit
	Node      struct {
		PullRequest struct {
			Reviews struct {
				PageInfo BackPage
				Nodes    []Review
			} `graphql:"reviews(last: 50, before: $before)"`
		} `graphql:"... on PullRequest"`
	} `graphql:"node(id: $id)"`
}

type CommentsPage struct {
	RateLimit RateLimit
	Node      struct {
		PullRequest struct {
			Comments struct {
				PageInfo BackPage
				Nodes    []Comment
			} `graphql:"comments(last: 50, before: $before)"`
		} `graphql:"... on PullRequest"`
	} `graphql:"node(id: $id)"`
}

type ThreadsPage struct {
	RateLimit RateLimit
	Node      struct {
		PullRequest struct {
			ReviewThreads struct {
				PageInfo BackPage
				Nodes    []Thread
			} `graphql:"reviewThreads(last: 50, before: $before)"`
		} `graphql:"... on PullRequest"`
	} `graphql:"node(id: $id)"`
}

type ThreadCommentsPage struct {
	RateLimit RateLimit
	Node      struct {
		Thread struct {
			Comments ThreadComments `graphql:"comments(first: 50, after: $after)"`
		} `graphql:"... on PullRequestReviewThread"`
	} `graphql:"node(id: $id)"`
}
