package gh

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/shurcooL/githubv4"

	"prism/internal/gh/queries"
	"prism/internal/model"
)

// SnapshotBatch is how many PRs one Snapshots query asks for (≤ 50).
const SnapshotBatch = 25

// maxExtraPages bounds back-pagination per connection, so one huge PR cannot
// eat the rate limit. 20 × 50 = 1 000 items per connection.
const maxExtraPages = 20

// SnapshotResult is the outcome of Snapshots.
type SnapshotResult struct {
	PRs       map[string]*model.PRSnapshot // by node id
	Missing   []string                     // GitHub returned null without an error (deleted, no access)
	Failed    map[string]error             // null because of a GraphQL error (e.g. FORBIDDEN org)
	Cost      int
	Remaining int
}

// Snapshots fetches full PR snapshots by node id in batches and completes any
// connection with more than one page. GraphQL errors (one forbidden org, …)
// fail only the PRs they null out; transport errors (unauthorized, rate
// limited, network) abort the whole call.
func (c *Client) Snapshots(ctx context.Context, ids []string) (SnapshotResult, error) {
	res := SnapshotResult{PRs: map[string]*model.PRSnapshot{}, Failed: map[string]error{}}
	for start := 0; start < len(ids); start += SnapshotBatch {
		if err := c.snapshotBatch(ctx, ids[start:min(start+SnapshotBatch, len(ids))], &res); err != nil {
			return res, err
		}
	}
	return res, nil
}

// PRNodeID resolves owner/repo#number to the PR's node id.
func (c *Client) PRNodeID(ctx context.Context, repo string, number int) (string, error) {
	owner, name, ok := strings.Cut(repo, "/")
	if !ok {
		return "", fmt.Errorf("repo %q: want owner/name", repo)
	}
	var q queries.PRByNumber
	vars := map[string]any{
		"owner":  githubv4.String(owner),
		"name":   githubv4.String(name),
		"number": githubv4.Int(number),
	}
	if err := c.GraphQL.Query(ctx, &q, vars); err != nil {
		return "", fmt.Errorf("look up %s#%d: %w", repo, number, err)
	}
	if q.Repository == nil || q.Repository.PullRequest == nil {
		return "", fmt.Errorf("%s#%d not found", repo, number)
	}
	return q.Repository.PullRequest.ID, nil
}

// IsGraphQLError reports whether err came from a GraphQL "errors" array.
// shurcooL/graphql returns those as its unexported type graphql.errors, after
// decoding any partial data into the query struct.
func IsGraphQLError(err error) bool {
	for ; err != nil; err = errors.Unwrap(err) {
		t := reflect.TypeOf(err)
		if t.PkgPath() == "github.com/shurcooL/graphql" && t.Name() == "errors" {
			return true
		}
	}
	return false
}

func (c *Client) snapshotBatch(ctx context.Context, ids []string, res *SnapshotResult) error {
	gids := make([]githubv4.ID, len(ids))
	for i, id := range ids {
		gids[i] = githubv4.ID(id)
	}
	var q queries.Snapshots
	err := c.GraphQL.Query(ctx, &q, map[string]any{"ids": gids})
	if err != nil && (!IsGraphQLError(err) || len(q.Nodes) == 0) {
		return fmt.Errorf("graphql snapshots: %w", err)
	}
	res.Cost += q.RateLimit.Cost
	res.Remaining = q.RateLimit.Remaining
	c.log.Debug("snapshots", "prs", len(ids), "cost", q.RateLimit.Cost, "remaining", q.RateLimit.Remaining)

	for i, n := range q.Nodes {
		if n == nil || n.PullRequest.ID == "" {
			if i < len(ids) {
				if err != nil {
					res.Failed[ids[i]] = err
				} else {
					res.Missing = append(res.Missing, ids[i])
				}
			}
			continue
		}
		pr := &n.PullRequest
		if perr := c.completePages(ctx, pr, res); perr != nil {
			if !IsGraphQLError(perr) {
				return perr
			}
			res.Failed[pr.ID] = perr
			continue
		}
		res.PRs[pr.ID] = toSnapshot(pr)
	}
	return nil
}

// completePages fetches older reviews, comments and threads, and later thread
// comments, when the first page was full.
func (c *Client) completePages(ctx context.Context, pr *queries.PullRequest, res *SnapshotResult) error {
	for i := 0; pr.Reviews.PageInfo.HasPreviousPage && i < maxExtraPages; i++ {
		var q queries.ReviewsPage
		if err := c.page(ctx, &q, pr.ID, "before", pr.Reviews.PageInfo.StartCursor); err != nil {
			return err
		}
		res.Cost += q.RateLimit.Cost
		page := q.Node.PullRequest.Reviews
		pr.Reviews.Nodes = append(page.Nodes, pr.Reviews.Nodes...)
		pr.Reviews.PageInfo = page.PageInfo
	}
	for i := 0; pr.Comments.PageInfo.HasPreviousPage && i < maxExtraPages; i++ {
		var q queries.CommentsPage
		if err := c.page(ctx, &q, pr.ID, "before", pr.Comments.PageInfo.StartCursor); err != nil {
			return err
		}
		res.Cost += q.RateLimit.Cost
		page := q.Node.PullRequest.Comments
		pr.Comments.Nodes = append(page.Nodes, pr.Comments.Nodes...)
		pr.Comments.PageInfo = page.PageInfo
	}
	for i := 0; pr.ReviewThreads.PageInfo.HasPreviousPage && i < maxExtraPages; i++ {
		var q queries.ThreadsPage
		if err := c.page(ctx, &q, pr.ID, "before", pr.ReviewThreads.PageInfo.StartCursor); err != nil {
			return err
		}
		res.Cost += q.RateLimit.Cost
		page := q.Node.PullRequest.ReviewThreads
		pr.ReviewThreads.Nodes = append(page.Nodes, pr.ReviewThreads.Nodes...)
		pr.ReviewThreads.PageInfo = page.PageInfo
	}
	for ti := range pr.ReviewThreads.Nodes {
		t := &pr.ReviewThreads.Nodes[ti]
		for i := 0; t.Comments.PageInfo.HasNextPage && i < maxExtraPages; i++ {
			var q queries.ThreadCommentsPage
			if err := c.page(ctx, &q, t.ID, "after", t.Comments.PageInfo.EndCursor); err != nil {
				return err
			}
			res.Cost += q.RateLimit.Cost
			page := q.Node.Thread.Comments
			t.Comments.Nodes = append(t.Comments.Nodes, page.Nodes...)
			t.Comments.PageInfo = page.PageInfo
		}
	}
	return nil
}

func (c *Client) page(ctx context.Context, q any, id, cursorVar string, cursor githubv4.String) error {
	vars := map[string]any{"id": githubv4.ID(id), cursorVar: cursor}
	if err := c.GraphQL.Query(ctx, q, vars); err != nil {
		return fmt.Errorf("graphql page %s of %s: %w", cursorVar, id, err)
	}
	return nil
}

// toSnapshot converts the GraphQL shape into the stored model.
func toSnapshot(pr *queries.PullRequest) *model.PRSnapshot {
	s := &model.PRSnapshot{
		ID:               pr.ID,
		Repo:             pr.Repository.NameWithOwner,
		Number:           pr.Number,
		Title:            pr.Title,
		URL:              pr.URL,
		Author:           actor(pr.Author),
		IsDraft:          pr.IsDraft,
		State:            model.PRState(pr.State),
		CreatedAt:        pr.CreatedAt.UTC(),
		UpdatedAt:        pr.UpdatedAt.UTC(),
		HeadOID:          pr.HeadRefOid,
		BaseOID:          pr.BaseRefOid,
		Mergeable:        pr.Mergeable,
		MergeStateStatus: pr.MergeStateStatus,
		CommitsTotal:     pr.Commits.TotalCount,
		Reviews:          []model.Review{},
		Comments:         []model.Comment{},
		Threads:          []model.Thread{},
		ReviewRequests:   []model.Reviewer{},
		Timeline:         []model.TimelineEvent{},
	}
	if pr.ClosedAt != nil {
		t := pr.ClosedAt.UTC()
		s.ClosedAt = &t
	}
	if n := pr.Commits.Nodes; len(n) > 0 && n[0].Commit.StatusCheckRollup != nil {
		roll := n[0].Commit.StatusCheckRollup
		s.CI = &model.CI{State: model.CheckState(roll.State)}
		for _, ctx := range roll.Contexts.Nodes {
			s.CI.Checks = append(s.CI.Checks, check(ctx))
		}
	}
	for _, r := range pr.Reviews.Nodes {
		rv := model.Review{
			ID: r.ID, DatabaseID: r.DatabaseID, State: model.ReviewState(r.State), Body: r.Body,
			URL: r.URL, Author: actor(r.Author), SubmittedAt: timeOf(r.SubmittedAt),
		}
		if r.Commit != nil {
			rv.CommitOID = r.Commit.Oid
		}
		s.Reviews = append(s.Reviews, rv)
	}
	for _, cm := range pr.Comments.Nodes {
		s.Comments = append(s.Comments, model.Comment{
			ID: cm.ID, DatabaseID: cm.DatabaseID, Body: cm.Body, URL: cm.URL,
			CreatedAt: cm.CreatedAt.UTC(), Author: actor(cm.Author),
		})
	}
	for _, t := range pr.ReviewThreads.Nodes {
		th := model.Thread{ID: t.ID, IsResolved: t.IsResolved, Path: t.Path, Comments: []model.ThreadComment{}}
		for _, cm := range t.Comments.Nodes {
			tc := model.ThreadComment{Comment: model.Comment{
				ID: cm.ID, DatabaseID: cm.DatabaseID, Body: cm.Body, URL: cm.URL,
				CreatedAt: cm.CreatedAt.UTC(), Author: actor(cm.Author),
			}}
			if cm.ReplyTo != nil {
				tc.ReplyToID = cm.ReplyTo.ID
			}
			if cm.PullRequestReview != nil {
				tc.ReviewDatabaseID = cm.PullRequestReview.DatabaseID
			}
			th.Comments = append(th.Comments, tc)
		}
		s.Threads = append(s.Threads, th)
	}
	for _, rr := range pr.ReviewRequests.Nodes {
		if r := reviewer(rr.RequestedReviewer); r != nil {
			s.ReviewRequests = append(s.ReviewRequests, *r)
		}
	}
	for _, it := range pr.TimelineItems.Nodes {
		switch it.Typename {
		case "ReviewRequestedEvent":
			e := it.ReviewRequested
			s.Timeline = append(s.Timeline, model.TimelineEvent{
				Type: model.TimelineReviewRequested, ID: e.ID, CreatedAt: e.CreatedAt.UTC(),
				Actor: actor(e.Actor), RequestedReviewer: reviewer(e.RequestedReviewer),
			})
		case "HeadRefForcePushedEvent":
			e := it.ForcePushed
			te := model.TimelineEvent{
				Type: model.TimelineHeadRefForcePushed, ID: e.ID, CreatedAt: e.CreatedAt.UTC(), Actor: actor(e.Actor),
			}
			if e.AfterCommit != nil {
				te.AfterOID = e.AfterCommit.Oid
			}
			s.Timeline = append(s.Timeline, te)
		}
	}
	return s
}

func actor(a *queries.Actor) model.Actor {
	if a == nil {
		return model.Actor{} // deleted account ("ghost")
	}
	return model.Actor{
		Login:     a.Login,
		AvatarURL: a.AvatarURL,
		IsBot:     a.Typename == "Bot" || strings.HasSuffix(a.Login, "[bot]"),
	}
}

func reviewer(r *queries.Reviewer) *model.Reviewer {
	switch {
	case r == nil:
		return nil
	case r.Typename == "User":
		return &model.Reviewer{Login: r.User.Login}
	case r.Typename == "Team":
		return &model.Reviewer{Team: r.Team.Organization.Login + "/" + r.Team.Slug}
	}
	return nil
}

// check normalizes a CheckRun or StatusContext to a StatusState-like value.
func check(c queries.CheckContext) model.Check {
	if c.Typename == "StatusContext" {
		return model.Check{Name: c.StatusContext.Context, State: model.CheckState(c.StatusContext.State),
			URL: c.StatusContext.TargetURL, At: c.StatusContext.CreatedAt.UTC()}
	}
	r := c.CheckRun
	state := model.CheckPending
	if r.Status == "COMPLETED" {
		switch r.Conclusion {
		case "SUCCESS", "NEUTRAL", "SKIPPED":
			state = model.CheckSuccess
		default: // FAILURE, TIMED_OUT, CANCELLED, ACTION_REQUIRED, STARTUP_FAILURE, STALE
			state = model.CheckFailure
		}
	}
	return model.Check{Name: r.Name, State: state, URL: r.DetailsURL, At: timeOf(r.CompletedAt)}
}

func timeOf(t *githubv4.DateTime) time.Time {
	if t == nil {
		return time.Time{}
	}
	return t.UTC()
}
