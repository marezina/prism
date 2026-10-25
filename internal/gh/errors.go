package gh

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ErrUnauthorized means the token is missing, revoked or expired (HTTP 401).
// Callers clear the keyring and send the UI back to login.
var ErrUnauthorized = errors.New("github: unauthorized (token revoked or invalid)")

// SSORequiredError means an org enforces SAML SSO and the token is not
// authorized for it (HTTP 403 + X-GitHub-SSO: required; url=...).
type SSORequiredError struct {
	URL string // where the user authorizes the token; may be empty
}

func (e *SSORequiredError) Error() string {
	if e.URL == "" {
		return "github: SAML SSO authorization required"
	}
	return "github: SAML SSO authorization required: " + e.URL
}

// RateLimitedError means GitHub refused the request because a primary or
// secondary rate limit is exhausted. Back off until ResetAt.
type RateLimitedError struct {
	ResetAt time.Time
}

func (e *RateLimitedError) Error() string {
	return fmt.Sprintf("github: rate limited until %s", e.ResetAt.Format(time.RFC3339))
}

// classify maps an HTTP response to a typed error, or nil when the response
// should be handed to the caller unchanged (github-api skill §Error mapping).
func classify(resp *http.Response, now time.Time) error {
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return ErrUnauthorized
	case http.StatusForbidden, http.StatusTooManyRequests:
		if sso := resp.Header.Get("X-GitHub-SSO"); strings.HasPrefix(sso, "required") {
			return &SSORequiredError{URL: ssoURL(sso)}
		}
		if reset, ok := rateLimitReset(resp.Header, now); ok {
			return &RateLimitedError{ResetAt: reset}
		}
	}
	return nil
}

// ssoURL extracts url=... from "required; url=https://github.com/orgs/acme/sso?...".
func ssoURL(header string) string {
	for _, part := range strings.Split(header, ";") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(part), "url="); ok {
			return v
		}
	}
	return ""
}

// rateLimitReset reports whether h signals an exhausted rate limit and when it
// resets. Retry-After (secondary limits) wins over X-RateLimit-Reset.
func rateLimitReset(h http.Header, now time.Time) (time.Time, bool) {
	if ra := h.Get("Retry-After"); ra != "" {
		if secs, err := strconv.Atoi(ra); err == nil {
			return now.Add(time.Duration(secs) * time.Second), true
		}
	}
	if h.Get("X-RateLimit-Remaining") == "0" {
		if unix, err := strconv.ParseInt(h.Get("X-RateLimit-Reset"), 10, 64); err == nil {
			return time.Unix(unix, 0), true
		}
		return now.Add(time.Minute), true
	}
	return time.Time{}, false
}

// partialSSOOrgs parses "partial-results; organizations=21955855,20582480",
// which GitHub sends when it hid orgs the token is not SSO-authorized for.
func partialSSOOrgs(header string) []int64 {
	if !strings.HasPrefix(header, "partial-results") {
		return nil
	}
	var ids []int64
	for _, part := range strings.Split(header, ";") {
		v, ok := strings.CutPrefix(strings.TrimSpace(part), "organizations=")
		if !ok {
			continue
		}
		for _, s := range strings.Split(v, ",") {
			if id, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64); err == nil {
				ids = append(ids, id)
			}
		}
	}
	return ids
}
