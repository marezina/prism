package notify

import (
	"context"
	"slices"
	"testing"
	"time"

	"prism/internal/model"
	"prism/internal/settings"
	"prism/internal/store"
)

type fakeNotifier struct{ shown []Toast }

func (f *fakeNotifier) Show(_ context.Context, t Toast) error {
	f.shown = append(f.shown, t)
	return nil
}

type fakeSounds struct{ played []string }

func (f *fakeSounds) Play(name string, _ float64) (bool, error) {
	f.played = append(f.played, name)
	return true, nil
}

type fakeDND bool

func (f fakeDND) Active() bool { return bool(f) }

type fakeAvatars struct{}

func (fakeAvatars) Path(_ context.Context, login, _ string) string {
	if login == "alice" {
		return "/cache/alice.png"
	}
	return ""
}

// Wednesday noon local time: outside default quiet hours.
var noon = time.Date(2026, 10, 21, 12, 0, 0, 0, time.Local)

type rig struct {
	d     *Dispatcher
	n     *fakeNotifier
	s     *fakeSounds
	now   time.Time
	dnd   fakeDND
	sttgs settings.Settings
}

func newRig() *rig {
	r := &rig{n: &fakeNotifier{}, s: &fakeSounds{}, now: noon, sttgs: settings.Default()}
	r.d = &Dispatcher{Notifier: r.n, Sounds: r.s, Avatars: fakeAvatars{}, FallbackImage: "/app.png",
		Now: func() time.Time { return r.now }}
	r.d.DND = &r.dnd
	return r
}

func (r *rig) deliver(fresh ...store.Fresh) { r.d.Deliver(context.Background(), fresh, r.sttgs) }

func fresh(id int64, typ model.EventType, actor string) store.Fresh {
	return store.Fresh{ID: id, Event: model.Event{
		Type: typ, DedupKey: "k", PRID: "PR_1", Repo: "acme/api", PRNumber: 7, PRTitle: "Fix retries",
		Actor: model.Actor{Login: actor, AvatarURL: "https://a"}, Summary: "approved your PR",
		Excerpt: "LGTM", URL: "https://github.com/acme/api/pull/7#r1",
	}}
}

func TestSingleToast(t *testing.T) {
	r := newRig()
	r.deliver(fresh(42, model.MyPRApproved, "alice"))
	if len(r.n.shown) != 1 {
		t.Fatalf("shown = %v", r.n.shown)
	}
	got := r.n.shown[0]
	if got.Title != "alice approved your PR" || got.Body != "acme/api#7 · Fix retries\n“LGTM”" || got.ImagePath != "/cache/alice.png" {
		t.Errorf("toast = %+v", got)
	}
	if !slices.Equal(got.Actions, []Action{{ActionOpen, "Open PR"}, {ActionRead, "Mark as read"}}) {
		t.Errorf("actions = %v", got.Actions)
	}
	if got.Data[DataURL] != "https://github.com/acme/api/pull/7#r1" || !slices.Equal(IDs(got.Data), []int64{42}) {
		t.Errorf("data = %v", got.Data)
	}
	if !slices.Equal(r.s.played, []string{"soft-chime"}) {
		t.Errorf("sounds = %v", r.s.played)
	}
}

func TestSystemToastAndUnlisted(t *testing.T) {
	r := newRig()
	e := fresh(0, model.CIFailed, "")
	e.Event.Summary = "CI failed: build"
	r.deliver(e)
	got := r.n.shown[0]
	if got.Title != "acme/api#7 CI failed: build" || got.Body != "Fix retries" || got.ImagePath != "/app.png" {
		t.Errorf("toast = %+v", got)
	}
	if len(got.Actions) != 1 || got.Data[DataIDs] != "" {
		t.Errorf("unlisted event must not offer Mark as read: %+v", got)
	}
	if !slices.Equal(r.s.played, []string{"alert"}) {
		t.Errorf("sounds = %v", r.s.played)
	}
	// Avatar missing for a people event → fallback image.
	r.deliver(fresh(1, model.MyPRComment, "bob"))
	if r.n.shown[1].ImagePath != "/app.png" {
		t.Errorf("image = %q", r.n.shown[1].ImagePath)
	}
}

func TestBurst(t *testing.T) {
	r := newRig()
	r.deliver(
		fresh(1, model.MyPRComment, "alice"),
		fresh(2, model.MyPRComment, "bob"),
		fresh(3, model.MyPRApproved, "carol"),
		fresh(0, model.CIFailed, ""),
	)
	if len(r.n.shown) != 1 {
		t.Fatalf("want one summary, got %d toasts", len(r.n.shown))
	}
	s := r.n.shown[0]
	if s.Title != "4 new notifications" || s.Body != "2 comments · 1 approval · 1 CI failure" {
		t.Errorf("summary = %+v", s)
	}
	if !slices.Equal(IDs(s.Data), []int64{1, 2, 3}) || s.Actions[0].ID != ActionShowAll || s.Actions[1].ID != ActionRead {
		t.Errorf("summary data/actions = %+v", s)
	}
	if !slices.Equal(r.s.played, []string{"alert"}) {
		t.Errorf("one sound, alert wins: %v", r.s.played)
	}
}

func TestBurstAcrossBatches(t *testing.T) {
	r := newRig()
	r.deliver(fresh(1, model.MyPRComment, "alice"))
	r.now = r.now.Add(3 * time.Second)
	r.deliver(fresh(2, model.MyPRComment, "bob"), fresh(3, model.MyPRComment, "carol"))
	if len(r.n.shown) != 2 || r.n.shown[1].Title != "2 new notifications" {
		t.Fatalf("toasts = %+v", r.n.shown)
	}
	// After the window a single event is a normal toast again.
	r.now = r.now.Add(11 * time.Second)
	r.deliver(fresh(4, model.MyPRComment, "alice"))
	if r.n.shown[2].Title != "alice approved your PR" {
		t.Errorf("after window: %+v", r.n.shown[2])
	}
	// A single event never becomes a "1 new notifications" summary.
	r.deliver(fresh(5, model.MyPRComment, "alice"))
	r.deliver(fresh(6, model.MyPRComment, "alice"))
	if r.n.shown[4].Title == "1 new notifications" {
		t.Error("single-event summary")
	}

	r = newRig()
	r.sttgs.BurstGrouping = false
	r.deliver(fresh(1, model.MyPRComment, "a"), fresh(2, model.MyPRComment, "b"), fresh(3, model.MyPRComment, "c"))
	if len(r.n.shown) != 3 {
		t.Errorf("grouping off: %d toasts", len(r.n.shown))
	}
}

func TestFlagsQuietAndMuting(t *testing.T) {
	r := newRig()
	q := fresh(1, model.ReviewRequested, "alice")
	q.Quiet = true
	r.deliver(q, fresh(0, model.MyPRClosed, "")) // quiet; closed is list-only by default
	if len(r.n.shown) != 0 || len(r.s.played) != 0 {
		t.Fatalf("quiet/list-only events delivered: %v %v", r.n.shown, r.s.played)
	}

	r.deliver(fresh(2, model.CIPassed, "")) // toast, no sound by default
	if len(r.n.shown) != 1 || len(r.s.played) != 0 {
		t.Errorf("ci.passed: %v %v", r.n.shown, r.s.played)
	}

	r.sttgs.Sound.Enabled = false
	r.deliver(fresh(3, model.MyPRApproved, "alice"))
	if len(r.s.played) != 0 {
		t.Error("sound played while disabled")
	}
	r.sttgs.Sound.Enabled = true

	for name, mute := range map[string]func(){
		"dnd":         func() { r.dnd = true },
		"quiet hours": func() { r.sttgs.QuietHours = settings.QuietHours{Enabled: true, From: "11:00", To: "13:00"} },
		"pause":       func() { r.d.Pause(r.now.Add(time.Hour)) },
	} {
		r.n.shown, r.s.played = nil, nil
		mute()
		if r.d.Muted(r.sttgs) == "" {
			t.Errorf("%s: not muted", name)
		}
		r.deliver(fresh(9, model.MyPRApproved, "alice"))
		if len(r.n.shown) != 0 || len(r.s.played) != 0 {
			t.Errorf("%s: delivered anyway", name)
		}
		r.dnd, r.sttgs.QuietHours, r.d.pausedUntil = false, settings.Default().QuietHours, time.Time{}
	}
	r.d.Pause(r.now.Add(time.Hour))
	r.now = r.now.Add(61 * time.Minute)
	if !r.d.PausedUntil().IsZero() || r.d.Muted(r.sttgs) != "" {
		t.Error("pause did not expire")
	}
}

func TestInQuietHours(t *testing.T) {
	at := func(day, h, m int) time.Time { return time.Date(2026, 10, day, h, m, 0, 0, time.Local) }
	overnight := settings.QuietHours{Enabled: true, From: "18:00", To: "08:00"}
	daytime := settings.QuietHours{Enabled: true, From: "12:00", To: "13:00"}
	tests := []struct {
		name string
		q    settings.QuietHours
		now  time.Time
		want bool
	}{
		{"disabled", settings.QuietHours{From: "00:00", To: "23:59"}, at(21, 12, 0), false},
		{"overnight evening", overnight, at(21, 19, 0), true},
		{"overnight morning", overnight, at(21, 7, 59), true},
		{"overnight end exclusive", overnight, at(21, 8, 0), false},
		{"overnight day", overnight, at(21, 12, 0), false},
		{"daytime inside", daytime, at(21, 12, 30), true},
		{"daytime outside", daytime, at(21, 13, 0), false},
		{"weekend all day", settings.QuietHours{Enabled: true, From: "18:00", To: "08:00", Weekends: true}, at(24, 12, 0), true},
		{"weekend off", overnight, at(24, 12, 0), false},
		{"from == to", settings.QuietHours{Enabled: true, From: "10:00", To: "10:00"}, at(21, 10, 0), false},
		{"bad time", settings.QuietHours{Enabled: true, From: "x", To: "y"}, at(21, 10, 0), false},
	}
	for _, tt := range tests {
		if got := InQuietHours(tt.q, tt.now); got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestBreakdownAndIDs(t *testing.T) {
	got := Breakdown([]store.Fresh{
		fresh(1, model.Mention, "a"), fresh(2, model.CommentReplyToMe, "b"), fresh(3, model.CommentReplyToMe, "c"),
		fresh(4, model.MyPRThreadReply, "d"),
	})
	if want := "2 replies to you · 1 thread reply · 1 mention"; got != want {
		t.Errorf("Breakdown = %q, want %q", got, want)
	}
	if ids := IDs(map[string]string{DataIDs: "1, 2,x,3"}); !slices.Equal(ids, []int64{1, 2, 3}) {
		t.Errorf("IDs = %v", ids)
	}
	if ids := IDs(map[string]string{}); ids != nil {
		t.Errorf("IDs(empty) = %v", ids)
	}
}
