package gh

import (
	"context"
	"fmt"
	"time"

	"github.com/shurcooL/githubv4"
)

// SearchHit is one PR returned by SearchPRs.
type SearchHit struct {
	ID      string
	Repo    string
	Number  int
	Title   string
	IsDraft bool
}

// SearchResult is one full SearchPRs run.
type SearchResult struct {
	IssueCount int
	Hits       []SearchHit
	Cost       int // GraphQL points spent, summed over pages
	Remaining  int
	ResetAt    time.Time
}

// searchQuery mirrors a richer form of query 1 (WatchSet) in the github-api
// skill's references/queries.graphql.
type searchQuery struct {
	RateLimit struct {
		Cost      int
		Remaining int
		ResetAt   githubv4.DateTime
	}
	Search struct {
		IssueCount int
		PageInfo   struct {
			HasNextPage bool
			EndCursor   githubv4.String
		}
		Nodes []struct {
			PullRequest struct {
				ID         string
				Number     int
				Title      string
				IsDraft    bool
				Repository struct{ NameWithOwner string }
			} `graphql:"... on PullRequest"`
		}
	} `graphql:"search(query: $q, type: ISSUE, first: 100, after: $after)"`
}

// SearchPRs runs a GraphQL issue search and returns every PR hit (all pages).
func (c *Client) SearchPRs(ctx context.Context, q string) (SearchResult, error) {
	var (
		res   SearchResult
		after *githubv4.String
	)
	for {
		var query searchQuery
		vars := map[string]any{"q": githubv4.String(q), "after": after}
		if err := c.GraphQL.Query(ctx, &query, vars); err != nil {
			return SearchResult{}, fmt.Errorf("graphql search %q: %w", q, err)
		}
		res.IssueCount = query.Search.IssueCount
		res.Cost += query.RateLimit.Cost
		res.Remaining = query.RateLimit.Remaining
		res.ResetAt = query.RateLimit.ResetAt.Time
		for _, n := range query.Search.Nodes {
			pr := n.PullRequest
			if pr.ID == "" {
				continue // not a PR (search type ISSUE also returns issues)
			}
			res.Hits = append(res.Hits, SearchHit{
				ID: pr.ID, Repo: pr.Repository.NameWithOwner, Number: pr.Number, Title: pr.Title, IsDraft: pr.IsDraft,
			})
		}
		if !query.Search.PageInfo.HasNextPage {
			return res, nil
		}
		cursor := query.Search.PageInfo.EndCursor
		after = &cursor
	}
}
