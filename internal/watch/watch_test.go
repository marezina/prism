package watch

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"prism/internal/gh"
)

func TestQueries(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want []string
	}{
		{
			name: "per owner, with team in a watched org",
			cfg:  Config{Me: "me", Orgs: []string{"acme"}, IncludePersonal: true, Teams: []string{"acme/api", "other/x"}},
			want: []string{
				"is:pr is:open author:me org:acme",
				"is:pr is:open review-requested:me org:acme",
				"is:pr is:open reviewed-by:me -author:me org:acme",
				"is:pr is:open commenter:me -author:me org:acme",
				"is:pr is:open mentions:me -author:me org:acme",
				"is:pr is:open author:me user:me",
				"is:pr is:open review-requested:me user:me",
				"is:pr is:open reviewed-by:me -author:me user:me",
				"is:pr is:open commenter:me -author:me user:me",
				"is:pr is:open mentions:me -author:me user:me",
				"is:pr is:open team-review-requested:acme/api org:acme",
			},
		},
		{
			name: "combined owners",
			cfg:  Config{Me: "me", Orgs: []string{"a", "b"}, CombineOwners: true},
			want: []string{
				"is:pr is:open author:me org:a org:b",
				"is:pr is:open review-requested:me org:a org:b",
				"is:pr is:open reviewed-by:me -author:me org:a org:b",
				"is:pr is:open commenter:me -author:me org:a org:b",
				"is:pr is:open mentions:me -author:me org:a org:b",
			},
		},
		{
			name: "nothing selected",
			cfg:  Config{Me: "me"},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			for _, q := range Queries(tt.cfg) {
				got = append(got, q.Q)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("(-want +got):\n%s", diff)
			}
		})
	}
}

// fakeSearcher returns hits for queries containing a key, or an error.
type fakeSearcher struct {
	hits  map[string][]gh.SearchHit
	errs  map[string]error
	calls []string
}

func (f *fakeSearcher) SearchPRs(_ context.Context, q string) (gh.SearchResult, error) {
	f.calls = append(f.calls, q)
	for key, err := range f.errs {
		if strings.Contains(q, key) {
			return gh.SearchResult{}, err
		}
	}
	res := gh.SearchResult{Cost: 1}
	for key, hits := range f.hits {
		if strings.Contains(q, key) {
			res.Hits = append(res.Hits, hits...)
		}
	}
	return res, nil
}

func TestBuild(t *testing.T) {
	mine := gh.SearchHit{ID: "PR_mine", Repo: "acme/api", Number: 1}
	draft := gh.SearchHit{ID: "PR_draft", Repo: "acme/api", Number: 2, IsDraft: true}
	review := gh.SearchHit{ID: "PR_rev", Repo: "acme/web", Number: 3}
	s := &fakeSearcher{
		hits: map[string][]gh.SearchHit{
			"author:me org:acme":               {mine, draft},
			"review-requested:me org:acme":     {review},
			"commenter:me -author:me org:acme": {review},
			"team-review-requested:acme/api":   {review},
		},
		errs: map[string]error{
			"org:locked": &gh.SSORequiredError{URL: "https://sso"},
		},
	}
	cfg := Config{Me: "me", Orgs: []string{"acme", "locked"}, Teams: []string{"acme/api"}}

	set, err := Build(context.Background(), s, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if got := set.IDs(); !cmp.Equal(got, []string{"PR_mine", "PR_rev"}) {
		t.Errorf("ids = %v (drafts must be filtered)", got)
	}
	if got := set.PRs["PR_rev"].Roles; !cmp.Equal(got, []Role{RoleParticipated, RoleReviewRequested}) {
		t.Errorf("roles = %v", got)
	}
	if _, ok := set.Failed["org:locked"]; !ok || len(set.Failed) != 1 {
		t.Errorf("failed = %v", set.Failed)
	}
	lockedCalls := 0
	for _, q := range s.calls {
		if strings.Contains(q, "org:locked") {
			lockedCalls++
		}
	}
	if lockedCalls != 1 {
		t.Errorf("a failed owner must be skipped after its first error; got %d calls", lockedCalls)
	}

	cfg.IncludeDrafts = true
	set, _ = Build(context.Background(), s, cfg)
	if _, ok := set.PRs["PR_draft"]; !ok {
		t.Error("draft missing with IncludeDrafts")
	}
}

func TestBuildAbortsOnAuthError(t *testing.T) {
	s := &fakeSearcher{errs: map[string]error{"is:pr": gh.ErrUnauthorized}}
	_, err := Build(context.Background(), s, Config{Me: "me", Orgs: []string{"acme"}})
	if !errors.Is(err, gh.ErrUnauthorized) {
		t.Fatalf("got %v, want ErrUnauthorized", err)
	}
	if len(s.calls) != 1 {
		t.Errorf("calls = %d, want 1", len(s.calls))
	}
}
