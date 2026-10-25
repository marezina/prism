package sound

import (
	"testing"
	"time"
)

type recorder struct{ played []string }

func (r *recorder) Play(name string, _ float64) error {
	r.played = append(r.played, name)
	return nil
}

func TestLimited(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	rec := &recorder{}
	l := &Limited{Player: rec, Now: func() time.Time { return now }}

	steps := []struct {
		after time.Duration
		want  bool
	}{
		{0, true},
		{time.Second, false},
		{3 * time.Second, false}, // 4 s after the first
		{time.Second, true},      // 5 s
		{4999 * time.Millisecond, false},
		{time.Millisecond, true},
	}
	for i, s := range steps {
		now = now.Add(s.after)
		played, err := l.Play("ding", 0.5)
		if err != nil || played != s.want {
			t.Errorf("step %d: played = %v, want %v (err %v)", i, played, s.want, err)
		}
	}
	if len(rec.played) != 3 {
		t.Errorf("played %v", rec.played)
	}
}
