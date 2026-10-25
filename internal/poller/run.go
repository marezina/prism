package poller

import (
	"context"
	"errors"
	"time"

	"prism/internal/gh"
	"prism/internal/store"
)

// Clock is the time source of the scheduler (faked in tests).
type Clock interface {
	Now() time.Time
	After(d time.Duration) <-chan time.Time
}

type realClock struct{}

func (realClock) Now() time.Time                         { return time.Now() }
func (realClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

const (
	minFastInterval = 60 * time.Second
	wakeCheckEvery  = 30 * time.Second
	maxBackoff      = 10 * time.Minute
	firstBackoff    = 30 * time.Second
	// mergeableRecheck: GitHub computes mergeability lazily (github-api skill).
	mergeableRecheck = 30 * time.Second
)

// backoff is 30 s, 1 min, 2 min, … capped at 10 min (ARCHITECTURE §2).
func backoff(failures int) time.Duration {
	d := firstBackoff
	for i := 1; i < failures && d < maxBackoff; i++ {
		d *= 2
	}
	return min(d, maxBackoff)
}

// retryDelay is how long to wait after a failed cycle.
func retryDelay(err error, failures int, now time.Time) time.Duration {
	var rl *gh.RateLimitedError
	if errors.As(err, &rl) {
		if d := rl.ResetAt.Sub(now) + 5*time.Second; d > 0 {
			return d
		}
	}
	return backoff(failures)
}

// woke reports a resume from sleep: the wall clock jumped by more than twice
// the poll interval between two wake checks (desktop-integration skill).
func woke(prev, now time.Time, interval time.Duration) bool {
	return now.Round(0).Sub(prev.Round(0)) > 2*interval
}

// Run polls until ctx is done. It returns gh.ErrUnauthorized when the token
// stops working (the app then goes back to login); other errors only back off.
func (p *Poller) Run(ctx context.Context) error {
	failures := 0
	slow := p.clock.After(0) // poll immediately on start
	fast := p.clock.After(minFastInterval)
	wake := p.clock.After(wakeCheckEvery)
	var recheck <-chan time.Time // armed while PRs wait for a mergeable re-poll
	lastWake := p.clock.Now()
	lastModified, _ := p.store.Meta(ctx, store.MetaNotificationsLastModified)

	runCycle := func() error {
		err := p.Cycle(ctx)
		if errors.Is(err, gh.ErrUnauthorized) {
			return err
		}
		if err != nil {
			failures++
			delay := retryDelay(err, failures, p.clock.Now())
			p.log.Warn("poll failed; backing off", "err", err, "retry_in", delay)
			slow = p.clock.After(delay)
			return nil
		}
		failures = 0
		slow = p.clock.After(p.interval(ctx))
		return nil
	}

	for {
		if recheck == nil && p.hasRecheck() {
			recheck = p.clock.After(mergeableRecheck)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()

		case <-recheck:
			recheck = nil
			if err := p.RefreshIDs(ctx, p.takeRecheck()); errors.Is(err, gh.ErrUnauthorized) {
				return err
			} else if err != nil {
				p.log.Debug("mergeable re-poll failed", "err", err)
			}

		case <-slow:
			if err := runCycle(); err != nil {
				return err
			}

		case <-p.trigger:
			if err := runCycle(); err != nil {
				return err
			}

		case <-wake:
			now := p.clock.Now()
			if woke(lastWake, now, p.interval(ctx)) {
				p.log.Info("resumed from sleep; polling now", "gap", now.Round(0).Sub(lastWake.Round(0)))
				if err := runCycle(); err != nil {
					return err
				}
			}
			lastWake = now
			wake = p.clock.After(wakeCheckEvery)

		case <-fast:
			next := minFastInterval
			if failures == 0 {
				res, err := p.gh.Notifications(ctx, lastModified)
				switch {
				case errors.Is(err, gh.ErrUnauthorized):
					return err
				case err != nil:
					p.log.Debug("notifications poll failed", "err", err)
				default:
					next = max(next, res.PollInterval)
					if res.LastModified != "" && res.LastModified != lastModified {
						lastModified = res.LastModified
						if err := p.store.SetMeta(ctx, store.MetaNotificationsLastModified, lastModified); err != nil {
							p.log.Warn("store Last-Modified", "err", err)
						}
					}
					if len(res.PRs) > 0 {
						needFull, err := p.RefreshPRs(ctx, res.PRs)
						if errors.Is(err, gh.ErrUnauthorized) {
							return err
						}
						if err != nil {
							p.log.Debug("early refresh failed", "err", err)
						}
						if needFull {
							p.Trigger()
						}
					}
				}
			}
			fast = p.clock.After(next)
		}
	}
}

// interval is the user's slow-tier poll interval.
func (p *Poller) interval(ctx context.Context) time.Duration {
	st, err := p.store.Settings(ctx)
	if err != nil {
		return 2 * time.Minute
	}
	return st.PollInterval()
}
