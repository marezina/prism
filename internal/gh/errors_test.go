package gh

import (
	"errors"
	"net/http"
	"slices"
	"testing"
	"time"
)

func TestClassify(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	tests := []struct {
		name    string
		status  int
		headers map[string]string
		check   func(t *testing.T, err error)
	}{
		{"200 passes", 200, nil, isNil},
		{"404 passes", 404, nil, isNil},
		{"401", 401, nil, func(t *testing.T, err error) {
			if !errors.Is(err, ErrUnauthorized) {
				t.Errorf("got %v, want ErrUnauthorized", err)
			}
		}},
		{"403 SSO with url", 403, map[string]string{"X-GitHub-SSO": "required; url=https://github.com/orgs/acme/sso?authorization_request=abc"},
			func(t *testing.T, err error) {
				var sso *SSORequiredError
				if !errors.As(err, &sso) || sso.URL != "https://github.com/orgs/acme/sso?authorization_request=abc" {
					t.Errorf("got %#v", err)
				}
			}},
		{"403 SSO partial results is not an error", 403, map[string]string{"X-GitHub-SSO": "partial-results; organizations=1"}, isNil},
		{"403 primary rate limit", 403, map[string]string{"X-RateLimit-Remaining": "0", "X-RateLimit-Reset": "1800000600"},
			resetAt(time.Unix(1_800_000_600, 0))},
		{"429 retry-after", 429, map[string]string{"Retry-After": "60"}, resetAt(now.Add(time.Minute))},
		{"403 secondary limit retry-after wins", 403, map[string]string{"Retry-After": "30", "X-RateLimit-Remaining": "0", "X-RateLimit-Reset": "1800000600"},
			resetAt(now.Add(30 * time.Second))},
		{"403 remaining 0 without reset", 403, map[string]string{"X-RateLimit-Remaining": "0"}, resetAt(now.Add(time.Minute))},
		{"403 plain permission", 403, map[string]string{"X-RateLimit-Remaining": "4000"}, isNil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &http.Response{StatusCode: tt.status, Header: http.Header{}}
			for k, v := range tt.headers {
				resp.Header.Set(k, v)
			}
			tt.check(t, classify(resp, now))
		})
	}
}

func isNil(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Errorf("got %v, want nil", err)
	}
}

func resetAt(want time.Time) func(*testing.T, error) {
	return func(t *testing.T, err error) {
		t.Helper()
		var rl *RateLimitedError
		if !errors.As(err, &rl) || !rl.ResetAt.Equal(want) {
			t.Errorf("got %v, want RateLimitedError{%v}", err, want)
		}
	}
}

func TestPartialSSOOrgs(t *testing.T) {
	tests := []struct {
		in   string
		want []int64
	}{
		{"", nil},
		{"required; url=https://x", nil},
		{"partial-results; organizations=21955855,20582480", []int64{21955855, 20582480}},
		{"partial-results; organizations=7", []int64{7}},
		{"partial-results; organizations=1, x ,2", []int64{1, 2}},
	}
	for _, tt := range tests {
		if got := partialSSOOrgs(tt.in); !slices.Equal(got, tt.want) {
			t.Errorf("partialSSOOrgs(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestParseScopes(t *testing.T) {
	got := parseScopes(" repo, read:org,notifications ,")
	want := []string{"notifications", "read:org", "repo"}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if parseScopes("") != nil {
		t.Error("empty header must give nil scopes")
	}
}
