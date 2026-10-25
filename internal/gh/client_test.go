package gh

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeGitHub is an httptest server whose handler the test sets per path.
type fakeGitHub struct {
	*httptest.Server
	mu       sync.Mutex
	handlers map[string]http.HandlerFunc
	calls    map[string]int
	lastReq  *http.Request
	lastBody string
}

func newFake(t *testing.T) *fakeGitHub {
	f := &fakeGitHub{handlers: map[string]http.HandlerFunc{}, calls: map[string]int{}}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		key := r.Method + " " + r.URL.Path
		f.calls[key]++
		f.lastReq, f.lastBody = r, string(body)
		h := f.handlers[key]
		f.mu.Unlock()
		if h == nil {
			http.NotFound(w, r)
			return
		}
		h(w, r)
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeGitHub) handle(key string, h http.HandlerFunc) { f.handlers[key] = h }

func (f *fakeGitHub) callCount(key string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[key]
}

// client returns a Client against f and the list of backoff sleeps it made.
func (f *fakeGitHub) client(t *testing.T) (*Client, *[]time.Duration) {
	t.Helper()
	var sleeps []time.Duration
	c, err := newClient(Options{
		Token:   "gho_testtoken",
		Version: "1.2.3",
		Endpoints: Endpoints{
			Web:     f.URL,
			API:     f.URL + "/",
			GraphQL: f.URL + "/graphql",
		},
		Logger: slog.New(slog.DiscardHandler),
	}, func() time.Time { return time.Unix(1_800_000_000, 0) }, func(_ context.Context, d time.Duration) error {
		sleeps = append(sleeps, d)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return c, &sleeps
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func TestNewRequiresVersion(t *testing.T) {
	if _, err := New(Options{}); err == nil {
		t.Fatal("want error without Version")
	}
}

func TestWhoamiHeadersAndScopes(t *testing.T) {
	f := newFake(t)
	f.handle("GET /user", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-OAuth-Scopes", "repo, notifications, read:org")
		w.Header().Set("X-RateLimit-Resource", "core")
		w.Header().Set("X-RateLimit-Limit", "5000")
		w.Header().Set("X-RateLimit-Remaining", "4999")
		w.Header().Set("X-RateLimit-Reset", "1800003600")
		writeJSON(w, map[string]any{"login": "octocat", "id": 1, "name": "Mona", "avatar_url": "https://a/1"})
	})
	c, _ := f.client(t)

	id, err := c.Whoami(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if id.Login != "octocat" || id.ID != 1 || id.Name != "Mona" {
		t.Errorf("identity = %+v", id)
	}
	if want := []string{"notifications", "read:org", "repo"}; !slices.Equal(id.Scopes, want) {
		t.Errorf("scopes = %v, want %v", id.Scopes, want)
	}
	if ua := f.lastReq.Header.Get("User-Agent"); ua != "PRism/1.2.3" {
		t.Errorf("User-Agent = %q", ua)
	}
	if auth := f.lastReq.Header.Get("Authorization"); auth != "Bearer gho_testtoken" {
		t.Errorf("Authorization = %q", auth)
	}
	rl, ok := c.RateLimits().Get("core")
	if !ok || rl.Remaining != 4999 || rl.Limit != 5000 || !rl.ResetAt.Equal(time.Unix(1_800_003_600, 0)) {
		t.Errorf("rate limit = %+v, %v", rl, ok)
	}
}

func TestUnauthorizedIsTyped(t *testing.T) {
	f := newFake(t)
	f.handle("GET /user", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"message":"Bad credentials"}`, http.StatusUnauthorized)
	})
	c, _ := f.client(t)
	if _, err := c.Whoami(context.Background()); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("got %v, want ErrUnauthorized", err)
	}
}

func TestRetry(t *testing.T) {
	tests := []struct {
		name       string
		statuses   []int // per attempt; last one repeats
		wantCalls  int
		wantSleeps int
		wantErr    bool
	}{
		{"gateway errors then ok", []int{502, 504, 200}, 3, 2, false},
		{"gives up after 3 retries", []int{503}, 4, 3, true},
		{"no retry on 404", []int{404}, 1, 0, true},
		{"no retry on 500", []int{500}, 1, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFake(t)
			n := 0
			f.handle("GET /user", func(w http.ResponseWriter, _ *http.Request) {
				status := tt.statuses[min(n, len(tt.statuses)-1)]
				n++
				if status != 200 {
					w.WriteHeader(status)
					return
				}
				writeJSON(w, map[string]any{"login": "octocat"})
			})
			c, sleeps := f.client(t)
			_, err := c.Whoami(context.Background())
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if got := f.callCount("GET /user"); got != tt.wantCalls {
				t.Errorf("calls = %d, want %d", got, tt.wantCalls)
			}
			if len(*sleeps) != tt.wantSleeps {
				t.Errorf("sleeps = %v, want %d", *sleeps, tt.wantSleeps)
			}
			for i, d := range *sleeps {
				lo := time.Duration(500<<i) * time.Millisecond
				if d < lo || d >= lo+250*time.Millisecond {
					t.Errorf("sleep %d = %v, want [%v, %v)", i, d, lo, lo+250*time.Millisecond)
				}
			}
		})
	}
}

func TestGraphQLRetryResendsBody(t *testing.T) {
	f := newFake(t)
	n := 0
	f.handle("POST /graphql", func(w http.ResponseWriter, _ *http.Request) {
		n++
		if n == 1 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		writeJSON(w, map[string]any{"data": map[string]any{
			"rateLimit": map[string]any{"cost": 1, "remaining": 4999, "resetAt": "2027-01-15T08:00:00Z"},
			"search": map[string]any{
				"issueCount": 2,
				"pageInfo":   map[string]any{"hasNextPage": false, "endCursor": "c1"},
				"nodes": []any{
					map[string]any{"id": "PR_1", "number": 7, "title": "Fix", "isDraft": true, "repository": map[string]any{"nameWithOwner": "acme/api"}},
					map[string]any{}, // an issue: no PR fields
				},
			},
		}})
	})
	c, _ := f.client(t)

	res, err := c.SearchPRs(context.Background(), "is:pr is:open author:@me org:acme")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.lastBody, "author:@me org:acme") {
		t.Errorf("retried body lost the query: %q", f.lastBody)
	}
	want := []SearchHit{{ID: "PR_1", Repo: "acme/api", Number: 7, Title: "Fix", IsDraft: true}}
	if !slices.Equal(res.Hits, want) || res.IssueCount != 2 || res.Cost != 1 || res.Remaining != 4999 {
		t.Errorf("result = %+v", res)
	}
}

func TestSearchPaginates(t *testing.T) {
	f := newFake(t)
	f.handle("POST /graphql", func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Variables map[string]any }
		_ = json.NewDecoder(strings.NewReader(f.lastBody)).Decode(&req)
		page := 1
		if req.Variables["after"] == "c1" {
			page = 2
		}
		writeJSON(w, map[string]any{"data": map[string]any{
			"rateLimit": map[string]any{"cost": 1, "remaining": 100, "resetAt": "2027-01-15T08:00:00Z"},
			"search": map[string]any{
				"issueCount": 2,
				"pageInfo":   map[string]any{"hasNextPage": page == 1, "endCursor": fmt.Sprintf("c%d", page)},
				"nodes": []any{map[string]any{"id": fmt.Sprintf("PR_%d", page), "number": page,
					"repository": map[string]any{"nameWithOwner": "acme/api"}}},
			},
		}})
	})
	c, _ := f.client(t)
	res, err := c.SearchPRs(context.Background(), "is:pr")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Hits) != 2 || res.Hits[1].ID != "PR_2" || res.Cost != 2 {
		t.Errorf("result = %+v", res)
	}
}

func TestOrgsWithSSOHidden(t *testing.T) {
	f := newFake(t)
	f.handle("GET /user/orgs", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") == "2" {
			writeJSON(w, []any{map[string]any{"login": "Beta", "id": 2}})
			return
		}
		w.Header().Set("Link", fmt.Sprintf(`<%s/user/orgs?page=2>; rel="next"`, f.URL))
		w.Header().Set("X-GitHub-SSO", "partial-results; organizations=30,40")
		writeJSON(w, []any{map[string]any{"login": "acme", "id": 1}})
	})
	f.handle("GET /organizations/30", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{"login": "secure-corp", "id": 30})
	})
	f.handle("GET /organizations/40", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-GitHub-SSO", "required; url=https://github.com/orgs/locked/sso?authorization_request=x")
		w.WriteHeader(http.StatusForbidden)
	})
	c, _ := f.client(t)

	orgs, err := c.Orgs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []Org{
		{Login: "acme", ID: 1},
		{Login: "Beta", ID: 2},
		{Login: "org #40", ID: 40, SSORequired: true, SSOURL: "https://github.com/orgs/locked/sso?authorization_request=x"},
		{Login: "secure-corp", ID: 30, SSORequired: true, SSOURL: f.URL + "/orgs/secure-corp/sso"},
	}
	if !slices.Equal(orgs, want) {
		t.Errorf("orgs =\n%+v\nwant\n%+v", orgs, want)
	}
}

func TestTeams(t *testing.T) {
	f := newFake(t)
	f.handle("GET /user/teams", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, []any{
			map[string]any{"slug": "platform", "organization": map[string]any{"login": "acme"}},
			map[string]any{"slug": "api", "organization": map[string]any{"login": "acme"}},
		})
	})
	c, _ := f.client(t)
	teams, err := c.Teams(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"acme/api", "acme/platform"}; !slices.Equal(teams, want) {
		t.Errorf("teams = %v, want %v", teams, want)
	}
}

func TestRateLimitWarnsOncePerWindow(t *testing.T) {
	var buf strings.Builder
	log := slog.New(slog.NewTextHandler(&buf, nil))
	l := newRateLimits()
	h := http.Header{}
	h.Set("X-RateLimit-Resource", "graphql")
	h.Set("X-RateLimit-Limit", "5000")
	h.Set("X-RateLimit-Reset", "1800003600")
	for _, rem := range []string{"1200", "900", "800"} {
		h.Set("X-RateLimit-Remaining", rem)
		l.observe(h, log)
	}
	if n := strings.Count(buf.String(), "rate limit below 20"); n != 1 {
		t.Errorf("warnings = %d, want 1:\n%s", n, buf.String())
	}
	h.Set("X-RateLimit-Reset", "1800007200") // new window
	l.observe(h, log)
	if n := strings.Count(buf.String(), "rate limit below 20"); n != 2 {
		t.Errorf("warnings after new window = %d, want 2", n)
	}
}

func TestNotifications(t *testing.T) {
	f := newFake(t)
	f.handle("GET /notifications", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Poll-Interval", "60")
		if r.Header.Get("If-Modified-Since") == "Thu, 22 Oct 2026 10:00:00 GMT" {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		if r.URL.Query().Get("all") != "false" || r.URL.Query().Get("participating") != "false" {
			t.Errorf("query = %v", r.URL.Query())
		}
		w.Header().Set("Last-Modified", "Thu, 22 Oct 2026 10:00:00 GMT")
		writeJSON(w, []any{
			map[string]any{"subject": map[string]any{"type": "PullRequest", "url": f.URL + "/repos/acme/api/pulls/7"},
				"repository": map[string]any{"full_name": "acme/api"}},
			map[string]any{"subject": map[string]any{"type": "PullRequest", "url": f.URL + "/repos/acme/api/pulls/7"},
				"repository": map[string]any{"full_name": "acme/api"}},
			map[string]any{"subject": map[string]any{"type": "Issue", "url": f.URL + "/repos/acme/api/issues/8"},
				"repository": map[string]any{"full_name": "acme/api"}},
			map[string]any{"subject": map[string]any{"type": "PullRequest", "url": "garbage"},
				"repository": map[string]any{"full_name": "acme/web"}},
		})
	})
	c, _ := f.client(t)
	res, err := c.Notifications(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	want := []PRRef{{Repo: "acme/api", Number: 7}}
	if !slices.Equal(res.PRs, want) || res.NotModified || res.PollInterval != time.Minute ||
		res.LastModified != "Thu, 22 Oct 2026 10:00:00 GMT" {
		t.Errorf("res = %+v", res)
	}

	res, err = c.Notifications(context.Background(), res.LastModified)
	if err != nil || !res.NotModified || res.LastModified != "Thu, 22 Oct 2026 10:00:00 GMT" || len(res.PRs) != 0 {
		t.Errorf("304: res = %+v, err = %v", res, err)
	}

	f.handle("GET /notifications", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	if _, err := c.Notifications(context.Background(), ""); err == nil {
		t.Error("want error on 500")
	}
}
