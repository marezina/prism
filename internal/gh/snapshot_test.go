package gh

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"prism/internal/model"
)

// graphqlFake answers POST /graphql by matching a substring of the query.
func graphqlFake(t *testing.T, routes map[string]func(vars map[string]any) string) (*Client, *[]string) {
	t.Helper()
	f := newFake(t)
	var queries []string
	f.handle("POST /graphql", func(w http.ResponseWriter, _ *http.Request) {
		var req struct {
			Query     string
			Variables map[string]any
		}
		if err := json.Unmarshal([]byte(f.lastBody), &req); err != nil {
			t.Errorf("bad request body: %v", err)
		}
		queries = append(queries, req.Query)
		for key, h := range routes {
			if strings.Contains(req.Query, key) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(h(req.Variables)))
				return
			}
		}
		t.Errorf("unexpected query: %s", req.Query)
		w.WriteHeader(http.StatusBadRequest)
	})
	c, _ := f.client(t)
	return c, &queries
}

func file(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestSnapshots(t *testing.T) {
	page1 := file(t, "snapshots_page1.json")
	c, queries := graphqlFake(t, map[string]func(map[string]any) string{
		"nodes(ids: $ids)": func(map[string]any) string { return page1 },
		"reviews(last: 50, before: $before)": func(vars map[string]any) string {
			if vars["before"] != "rev-c1" || vars["id"] != "PR_1" {
				t.Errorf("reviews page vars = %v", vars)
			}
			return `{"data":{"rateLimit":{"cost":1,"remaining":4989,"resetAt":"2027-01-15T08:00:00Z"},
				"node":{"reviews":{"pageInfo":{"hasPreviousPage":false,"startCursor":"rev-c0"},"nodes":[
				{"id":"PRR_1","databaseId":2001,"state":"CHANGES_REQUESTED","body":"Needs tests",
				 "url":"https://github.com/acme/api/pull/482#pullrequestreview-2001","submittedAt":"2026-10-19T12:00:00Z",
				 "author":{"__typename":"User","login":"alice","avatarUrl":"https://avatars/alice"},"commit":{"oid":"head0"}}]}}}}`
		},
		"comments(first: 50, after: $after)": func(vars map[string]any) string {
			if vars["after"] != "tc-c1" || vars["id"] != "PRRT_1" {
				t.Errorf("thread comments page vars = %v", vars)
			}
			return `{"data":{"rateLimit":{"cost":1,"remaining":4988,"resetAt":"2027-01-15T08:00:00Z"},
				"node":{"comments":{"pageInfo":{"hasNextPage":false,"endCursor":"tc-c2"},"nodes":[
				{"id":"PRRC_2","databaseId":4002,"body":"> Why 3 retries?\n\nBecause the SLA says so.",
				 "url":"https://github.com/acme/api/pull/482#discussion_r4002","createdAt":"2026-10-20T11:00:00Z",
				 "author":{"__typename":"User","login":"me","avatarUrl":"https://avatars/me"},
				 "replyTo":{"id":"PRRC_1"},"pullRequestReview":{"databaseId":2010}}]}}}}`
		},
	})

	res, err := c.Snapshots(context.Background(), []string{"PR_1", "PR_2", "PR_3"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Cost != 5 || res.Remaining != 4990 {
		t.Errorf("cost = %d, remaining = %d", res.Cost, res.Remaining)
	}
	// Both nulls come with a GraphQL error in the response, so both count as failed.
	if len(res.Failed) != 2 || res.Failed["PR_3"] == nil || len(res.Missing) != 0 {
		t.Errorf("failed = %v, missing = %v", res.Failed, res.Missing)
	}

	got := res.PRs["PR_1"]
	ts := func(s string) time.Time { v, _ := time.Parse(time.RFC3339, s); return v }
	want := &model.PRSnapshot{
		ID: "PR_1", Repo: "acme/api", Number: 482, Title: "Add retry to webhook delivery",
		URL:    "https://github.com/acme/api/pull/482",
		Author: model.Actor{Login: "me", AvatarURL: "https://avatars/me"},
		State:  model.PROpen, HeadOID: "head1", BaseOID: "base1",
		Mergeable: "MERGEABLE", MergeStateStatus: "BLOCKED", CommitsTotal: 4,
		CI: &model.CI{State: model.CheckFailure, Checks: []model.Check{
			{Name: "build", State: model.CheckSuccess, URL: "https://ci/build"},
			{Name: "test", State: model.CheckFailure, URL: "https://ci/test"},
			{Name: "lint", State: model.CheckPending, URL: "https://ci/lint"},
			{Name: "ci/legacy", State: model.CheckError, URL: "https://ci/legacy"},
		}},
		Reviews: []model.Review{
			{ID: "PRR_1", DatabaseID: 2001, State: model.ReviewChangesRequested, Body: "Needs tests",
				URL: "https://github.com/acme/api/pull/482#pullrequestreview-2001", SubmittedAt: ts("2026-10-19T12:00:00Z"),
				Author: model.Actor{Login: "alice", AvatarURL: "https://avatars/alice"}, CommitOID: "head0"},
			{ID: "PRR_2", DatabaseID: 2002, State: model.ReviewApproved, Body: "LGTM",
				URL: "https://github.com/acme/api/pull/482#pullrequestreview-2002", SubmittedAt: ts("2026-10-20T10:00:00Z"),
				Author: model.Actor{Login: "alice", AvatarURL: "https://avatars/alice"}, CommitOID: "head1"},
			{ID: "PRR_3", DatabaseID: 2003, State: model.ReviewPending,
				URL:    "https://github.com/acme/api/pull/482#pullrequestreview-2003",
				Author: model.Actor{Login: "bob", AvatarURL: "https://avatars/bob"}},
		},
		Comments: []model.Comment{
			{ID: "IC_1", DatabaseID: 3001, Body: "Deploy preview ready", URL: "https://github.com/acme/api/pull/482#issuecomment-3001",
				CreatedAt: ts("2026-10-20T09:00:00Z"), Author: model.Actor{Login: "netlify", AvatarURL: "https://avatars/netlify", IsBot: true}},
			{ID: "IC_2", DatabaseID: 3002, Body: "from a deleted account", URL: "https://github.com/acme/api/pull/482#issuecomment-3002",
				CreatedAt: ts("2026-10-20T09:30:00Z")},
		},
		Threads: []model.Thread{{ID: "PRRT_1", Path: "delivery/retry.go", Comments: []model.ThreadComment{
			{Comment: model.Comment{ID: "PRRC_1", DatabaseID: 4001, Body: "Why 3 retries?",
				URL: "https://github.com/acme/api/pull/482#discussion_r4001", CreatedAt: ts("2026-10-20T10:00:00Z"),
				Author: model.Actor{Login: "alice", AvatarURL: "https://avatars/alice"}}, ReviewDatabaseID: 2002},
			{Comment: model.Comment{ID: "PRRC_2", DatabaseID: 4002, Body: "> Why 3 retries?\n\nBecause the SLA says so.",
				URL: "https://github.com/acme/api/pull/482#discussion_r4002", CreatedAt: ts("2026-10-20T11:00:00Z"),
				Author: model.Actor{Login: "me", AvatarURL: "https://avatars/me"}}, ReplyToID: "PRRC_1", ReviewDatabaseID: 2010},
		}}},
		ReviewRequests: []model.Reviewer{{Login: "carol"}, {Team: "acme/platform"}},
		Timeline: []model.TimelineEvent{
			{Type: model.TimelineReviewRequested, ID: "RRE_1", CreatedAt: ts("2026-10-19T08:00:00Z"),
				Actor: model.Actor{Login: "me", AvatarURL: "https://avatars/me"}, RequestedReviewer: &model.Reviewer{Team: "acme/platform"}},
			{Type: model.TimelineHeadRefForcePushed, ID: "HRFPE_1", CreatedAt: ts("2026-10-21T08:00:00Z"),
				Actor: model.Actor{Login: "me", AvatarURL: "https://avatars/me"}, AfterOID: "head1"},
		},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("snapshot mismatch (-want +got):\n%s", diff)
	}

	// The generated query must use GitHub's field names.
	q := (*queries)[0]
	for _, field := range []string{
		"headRefOid", "baseRefOid", "avatarUrl(size: 96)", "detailsUrl", "targetUrl",
		"pullRequestReview{databaseId}", "replyTo{id}", "statusCheckRollup{state",
		"timelineItems(last: 30, itemTypes: [REVIEW_REQUESTED_EVENT, HEAD_REF_FORCE_PUSHED_EVENT])",
		"rateLimit{cost,remaining,resetAt}", "nameWithOwner", "databaseId",
	} {
		if !strings.Contains(q, field) {
			t.Errorf("query lacks %q:\n%s", field, q)
		}
	}
}

func TestSnapshotsMissingAndTransportErrors(t *testing.T) {
	c, _ := graphqlFake(t, map[string]func(map[string]any) string{
		"nodes(ids: $ids)": func(map[string]any) string {
			return `{"data":{"rateLimit":{"cost":1,"remaining":10,"resetAt":"2027-01-15T08:00:00Z"},"nodes":[null]}}`
		},
	})
	res, err := c.Snapshots(context.Background(), []string{"PR_gone"})
	if err != nil || len(res.Missing) != 1 || res.Missing[0] != "PR_gone" || len(res.Failed) != 0 {
		t.Fatalf("res = %+v, err = %v", res, err)
	}

	// A whole-query GraphQL error (no data) is not swallowed.
	c, _ = graphqlFake(t, map[string]func(map[string]any) string{
		"nodes(ids: $ids)": func(map[string]any) string {
			return `{"errors":[{"message":"Field 'bogus' doesn't exist"}]}`
		},
	})
	if _, err := c.Snapshots(context.Background(), []string{"PR_1"}); err == nil {
		t.Fatal("want error for a query without data")
	}
}

func TestSnapshotsBatches(t *testing.T) {
	var batches []int
	c, _ := graphqlFake(t, map[string]func(map[string]any) string{
		"nodes(ids: $ids)": func(vars map[string]any) string {
			ids := vars["ids"].([]any)
			batches = append(batches, len(ids))
			nodes := strings.Repeat("null,", len(ids))
			return `{"data":{"rateLimit":{"cost":1,"remaining":10,"resetAt":"2027-01-15T08:00:00Z"},"nodes":[` +
				strings.TrimSuffix(nodes, ",") + `]}}`
		},
	})
	ids := make([]string, 2*SnapshotBatch+3)
	for i := range ids {
		ids[i] = "PR_x"
	}
	if _, err := c.Snapshots(context.Background(), ids); err != nil {
		t.Fatal(err)
	}
	if want := []int{SnapshotBatch, SnapshotBatch, 3}; !cmp.Equal(batches, want) {
		t.Errorf("batches = %v, want %v", batches, want)
	}
}

func TestPRNodeID(t *testing.T) {
	c, _ := graphqlFake(t, map[string]func(map[string]any) string{
		"pullRequest(number: $number)": func(vars map[string]any) string {
			if vars["owner"] != "acme" || vars["name"] != "api" || vars["number"] != float64(482) {
				t.Errorf("vars = %v", vars)
			}
			return `{"data":{"rateLimit":{"cost":1,"remaining":10,"resetAt":"2027-01-15T08:00:00Z"},
				"repository":{"pullRequest":{"id":"PR_1"}}}}`
		},
	})
	id, err := c.PRNodeID(context.Background(), "acme/api", 482)
	if err != nil || id != "PR_1" {
		t.Fatalf("id = %q, err = %v", id, err)
	}
	if _, err := c.PRNodeID(context.Background(), "nope", 1); err == nil {
		t.Error("want error for repo without owner")
	}
}
