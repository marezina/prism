package poller

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"prism/internal/gh"
	"prism/internal/model"
)

// fakeClock fires After channels when Advance passes their deadline.
type fakeClock struct {
	mu      sync.Mutex
	now     time.Time
	waiters []waiter
}

type waiter struct {
	at time.Time
	ch chan time.Time
}

func newFakeClock(now time.Time) *fakeClock { return &fakeClock{now: now} }

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) After(d time.Duration) <-chan time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	ch := make(chan time.Time, 1)
	if d <= 0 {
		ch <- c.now
		return ch
	}
	c.waiters = append(c.waiters, waiter{at: c.now.Add(d), ch: ch})
	return ch
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
	kept := c.waiters[:0]
	for _, w := range c.waiters {
		if !w.at.After(c.now) {
			w.ch <- c.now
		} else {
			kept = append(kept, w)
		}
	}
	c.waiters = kept
}

func TestBackoff(t *testing.T) {
	want := []time.Duration{30 * time.Second, time.Minute, 2 * time.Minute, 4 * time.Minute, 8 * time.Minute, 10 * time.Minute, 10 * time.Minute}
	for i, w := range want {
		if got := backoff(i + 1); got != w {
			t.Errorf("backoff(%d) = %v, want %v", i+1, got, w)
		}
	}
	now := t0
	if d := retryDelay(&gh.RateLimitedError{ResetAt: now.Add(7 * time.Minute)}, 1, now); d != 7*time.Minute+5*time.Second {
		t.Errorf("rate-limit delay = %v", d)
	}
	if d := retryDelay(&gh.RateLimitedError{ResetAt: now.Add(-time.Minute)}, 3, now); d != 2*time.Minute {
		t.Errorf("past reset falls back to backoff: %v", d)
	}
}

func TestWoke(t *testing.T) {
	if woke(t0, t0.Add(30*time.Second), 2*time.Minute) {
		t.Error("normal tick reported as wake")
	}
	if !woke(t0, t0.Add(5*time.Minute), 2*time.Minute) {
		t.Error("5 min gap with 2 min interval not reported")
	}
}

// statusWaiter turns OnStatus into a channel the test can block on.
func runHarness(t *testing.T) (*harness, chan Status, context.CancelFunc, chan error) {
	h := newHarness(t)
	statuses := make(chan Status, 16)
	h.p.onStatus = func(s Status) { statuses <- s }
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- h.p.Run(ctx) }()
	t.Cleanup(cancel)
	return h, statuses, cancel, done
}

func waitStatus(t *testing.T, ch chan Status) Status {
	t.Helper()
	select {
	case s := <-ch:
		return s
	case <-time.After(5 * time.Second):
		t.Fatal("no cycle ran")
		return Status{}
	}
}

// settle lets the Run goroutine re-arm its timers before the clock moves.
func settle() { time.Sleep(20 * time.Millisecond) }

func TestRunSchedule(t *testing.T) {
	h, statuses, cancel, done := runHarness(t)
	h.gh.mu.Lock()
	h.gh.prs["PR_1"], h.gh.inSet["PR_1"] = myPR("PR_1", "acme/api", 1), true
	h.gh.mu.Unlock()

	waitStatus(t, statuses) // immediate cycle on start
	settle()

	h.clock.Advance(119 * time.Second)
	settle()
	select {
	case <-statuses:
		t.Fatal("cycled before the 120 s interval")
	default:
	}
	h.clock.Advance(time.Second)
	waitStatus(t, statuses)

	h.p.Trigger()
	waitStatus(t, statuses)

	// Failure → back off 30 s instead of 120 s.
	h.gh.mu.Lock()
	h.gh.searchErr = errors.New("offline")
	h.gh.mu.Unlock()
	settle()
	h.clock.Advance(120 * time.Second)
	if s := waitStatus(t, statuses); s.Err == nil {
		t.Fatal("want failing cycle")
	}
	h.gh.mu.Lock()
	h.gh.searchErr = nil
	h.gh.mu.Unlock()
	settle()
	h.clock.Advance(30 * time.Second)
	if s := waitStatus(t, statuses); s.Err != nil {
		t.Fatalf("retry after backoff failed: %v", s.Err)
	}

	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Errorf("Run = %v", err)
	}
}

func TestRunStopsOnUnauthorized(t *testing.T) {
	h, statuses, _, done := runHarness(t)
	waitStatus(t, statuses)
	h.gh.mu.Lock()
	h.gh.searchErr = gh.ErrUnauthorized
	h.gh.mu.Unlock()
	h.p.Trigger()
	select {
	case err := <-done:
		if !errors.Is(err, gh.ErrUnauthorized) {
			t.Fatalf("Run = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run kept going after ErrUnauthorized")
	}
}

func TestRunFastTierTriggersRefresh(t *testing.T) {
	h, statuses, _, _ := runHarness(t)
	h.gh.mu.Lock()
	h.gh.prs["PR_1"], h.gh.inSet["PR_1"] = myPR("PR_1", "acme/api", 1), true
	h.gh.mu.Unlock()
	waitStatus(t, statuses) // baseline

	// A new PR of a watched owner shows up in the inbox: full cycle requested.
	h.gh.mu.Lock()
	h.gh.notif = gh.NotificationsResult{LastModified: "lm-1", PRs: []gh.PRRef{{Repo: "acme/api", Number: 2}}}
	h.gh.mu.Unlock()
	settle()
	h.clock.Advance(60 * time.Second)
	waitStatus(t, statuses)
	if v, _ := h.store.Meta(context.Background(), "notifications_last_modified"); v != "lm-1" {
		t.Errorf("Last-Modified not stored: %q", v)
	}

	// A known PR changes: refreshed right away, before the slow tier.
	h.gh.edit("PR_1", func(p *model.PRSnapshot) { p.Reviews = []model.Review{approve(1, t0)} })
	h.gh.mu.Lock()
	h.gh.notif = gh.NotificationsResult{LastModified: "lm-2", PRs: []gh.PRRef{{Repo: "acme/api", Number: 1}}}
	h.gh.mu.Unlock()
	settle()
	h.clock.Advance(60 * time.Second)
	deadline := time.Now().Add(5 * time.Second)
	for {
		n, _ := h.store.UnreadCount(context.Background())
		if n == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("fast-tier hint did not refresh the PR")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestRunRechecksUnknownMergeable(t *testing.T) {
	h, statuses, _, _ := runHarness(t)
	pr := myPR("PR_1", "acme/api", 1)
	pr.Mergeable = "UNKNOWN"
	h.gh.mu.Lock()
	h.gh.prs["PR_1"], h.gh.inSet["PR_1"] = pr, true
	h.gh.mu.Unlock()
	waitStatus(t, statuses) // baseline; PR_1 queued for a re-poll
	h.gh.mu.Lock()
	before := len(h.gh.snapped)
	h.gh.mu.Unlock()

	h.gh.edit("PR_1", func(p *model.PRSnapshot) { p.Mergeable = "CONFLICTING" })
	settle()
	h.clock.Advance(30 * time.Second)
	deadline := time.Now().Add(5 * time.Second)
	for {
		if ids := h.gh.lastSnapshotIDs(); len(ids) == 1 && ids[0] == "PR_1" {
			h.gh.mu.Lock()
			n := len(h.gh.snapped)
			h.gh.mu.Unlock()
			if n > before {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("UNKNOWN mergeable was not re-polled after 30 s")
		}
		time.Sleep(10 * time.Millisecond)
	}
	for {
		if n, _ := h.store.UnreadCount(context.Background()); n == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("conflict from the re-poll not recorded")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if h.p.hasRecheck() {
		t.Error("recheck not cleared once mergeable is known")
	}
}
