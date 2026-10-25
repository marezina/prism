package sound

import (
	"sync"
	"time"
)

// MinGap is the minimum time between two sounds (SPEC §6).
const MinGap = 5 * time.Second

// Limited plays at most one sound per MinGap; extra requests are dropped.
type Limited struct {
	Player Player
	Now    func() time.Time

	mu   sync.Mutex
	last time.Time
}

// NewLimited wraps p with the SPEC §6 rate limit.
func NewLimited(p Player) *Limited { return &Limited{Player: p, Now: time.Now} }

// Play plays name unless a sound played less than MinGap ago. It reports
// whether the sound was played.
func (l *Limited) Play(name string, volume float64) (bool, error) {
	l.mu.Lock()
	now := l.Now()
	if !l.last.IsZero() && now.Sub(l.last) < MinGap {
		l.mu.Unlock()
		return false, nil
	}
	l.last = now
	l.mu.Unlock()
	return true, l.Player.Play(name, volume)
}
