package gh

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// PRRef identifies a PR by repo and number.
type PRRef struct {
	Repo   string // owner/name
	Number int
}

// NotificationsResult is one fast-tier poll (ARCHITECTURE §2).
type NotificationsResult struct {
	NotModified  bool          // 304: nothing changed, did not cost rate limit
	LastModified string        // send back as If-Modified-Since next time
	PollInterval time.Duration // X-Poll-Interval: minimum time until the next poll
	PRs          []PRRef       // PRs with unread inbox threads (a hint, never events)
}

// Notifications polls GET /notifications as a cheap change hint. Fine-grained
// PATs cannot call it; PRism only accepts classic and OAuth tokens.
func (c *Client) Notifications(ctx context.Context, ifModifiedSince string) (NotificationsResult, error) {
	u := c.endpoints.API + "notifications?all=false&participating=false&per_page=50"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return NotificationsResult{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if ifModifiedSince != "" {
		req.Header.Set("If-Modified-Since", ifModifiedSince)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return NotificationsResult{}, fmt.Errorf("get /notifications: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	res := NotificationsResult{LastModified: resp.Header.Get("Last-Modified")}
	if secs, err := strconv.Atoi(resp.Header.Get("X-Poll-Interval")); err == nil {
		res.PollInterval = time.Duration(secs) * time.Second
	}
	switch resp.StatusCode {
	case http.StatusNotModified:
		res.NotModified = true
		if res.LastModified == "" {
			res.LastModified = ifModifiedSince
		}
		return res, nil
	case http.StatusOK:
	default:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return res, fmt.Errorf("get /notifications: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var threads []struct {
		Subject struct {
			Type string `json:"type"`
			URL  string `json:"url"`
		} `json:"subject"`
		Repository struct {
			FullName string `json:"full_name"`
		} `json:"repository"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&threads); err != nil {
		return res, fmt.Errorf("decode /notifications: %w", err)
	}
	seen := map[PRRef]bool{}
	for _, t := range threads {
		if t.Subject.Type != "PullRequest" {
			continue
		}
		i := strings.LastIndex(t.Subject.URL, "/pulls/")
		if i < 0 {
			continue
		}
		n, err := strconv.Atoi(t.Subject.URL[i+len("/pulls/"):])
		if err != nil {
			continue
		}
		ref := PRRef{Repo: t.Repository.FullName, Number: n}
		if !seen[ref] {
			seen[ref] = true
			res.PRs = append(res.PRs, ref)
		}
	}
	return res, nil
}
