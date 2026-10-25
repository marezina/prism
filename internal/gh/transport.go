package gh

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// maxRetries is the number of retries after the first attempt for 502/503/504.
const maxRetries = 3

// transport adds auth and User-Agent, retries gateway errors with jittered
// backoff, records rate limits and turns auth / SSO / rate-limit responses into
// typed errors. It is shared by the REST and GraphQL clients.
type transport struct {
	base      http.RoundTripper
	token     string
	userAgent string
	log       *slog.Logger
	limits    *RateLimits

	now   func() time.Time
	sleep func(ctx context.Context, d time.Duration) error
}

func (t *transport) RoundTrip(req *http.Request) (*http.Response, error) {
	r := req.Clone(req.Context())
	r.Header.Set("User-Agent", t.userAgent)
	if t.token != "" {
		r.Header.Set("Authorization", "Bearer "+t.token)
	}

	for attempt := 0; ; attempt++ {
		if attempt > 0 {
			if err := rewind(r); err != nil {
				return nil, err
			}
			if err := t.sleep(r.Context(), backoff(attempt)); err != nil {
				return nil, err
			}
		}

		start := t.now()
		resp, err := t.base.RoundTrip(r)
		if err != nil {
			return nil, err
		}
		t.limits.observe(resp.Header, t.log)
		t.log.Debug("github request",
			"method", r.Method, "path", r.URL.Path, "status", resp.StatusCode,
			"attempt", attempt+1, "duration", t.now().Sub(start))

		if retryable(resp.StatusCode) && attempt < maxRetries && (r.Body == nil || r.GetBody != nil) {
			discard(resp)
			continue
		}
		if typed := classify(resp, t.now()); typed != nil {
			discard(resp)
			return nil, typed
		}
		return resp, nil
	}
}

func retryable(status int) bool {
	return status == http.StatusBadGateway || status == http.StatusServiceUnavailable || status == http.StatusGatewayTimeout
}

// backoff is 0.5 s, 1 s, 2 s … plus up to 250 ms of jitter.
func backoff(attempt int) time.Duration {
	return time.Duration(500<<(attempt-1))*time.Millisecond + rand.N(250*time.Millisecond)
}

func rewind(r *http.Request) error {
	if r.GetBody == nil {
		return nil
	}
	body, err := r.GetBody()
	if err != nil {
		return fmt.Errorf("rewind request body: %w", err)
	}
	r.Body = body
	return nil
}

func discard(resp *http.Response) {
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	_ = resp.Body.Close()
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// RateLimit is the last seen quota for one resource ("core", "graphql", …).
type RateLimit struct {
	Limit     int
	Remaining int
	ResetAt   time.Time
}

// RateLimits tracks quotas per resource from X-RateLimit-* headers.
type RateLimits struct {
	mu     sync.Mutex
	byRes  map[string]RateLimit
	warned map[string]time.Time // resource → reset we already warned about
}

func newRateLimits() *RateLimits {
	return &RateLimits{byRes: map[string]RateLimit{}, warned: map[string]time.Time{}}
}

// Get returns the last seen quota for resource.
func (l *RateLimits) Get(resource string) (RateLimit, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	rl, ok := l.byRes[resource]
	return rl, ok
}

// observe records the headers and warns once per window below 20 % left.
func (l *RateLimits) observe(h http.Header, log *slog.Logger) {
	res := h.Get("X-RateLimit-Resource")
	limit, err1 := strconv.Atoi(h.Get("X-RateLimit-Limit"))
	remaining, err2 := strconv.Atoi(h.Get("X-RateLimit-Remaining"))
	reset, err3 := strconv.ParseInt(h.Get("X-RateLimit-Reset"), 10, 64)
	if res == "" || err1 != nil || err2 != nil || err3 != nil {
		return
	}
	rl := RateLimit{Limit: limit, Remaining: remaining, ResetAt: time.Unix(reset, 0)}

	l.mu.Lock()
	l.byRes[res] = rl
	warn := limit > 0 && remaining*5 < limit && !l.warned[res].Equal(rl.ResetAt)
	if warn {
		l.warned[res] = rl.ResetAt
	}
	l.mu.Unlock()

	if warn {
		log.Warn("github rate limit below 20 %", "resource", res, "remaining", remaining, "limit", limit, "reset", rl.ResetAt)
	}
}
