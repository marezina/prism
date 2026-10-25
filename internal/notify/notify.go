// Package notify delivers new events as toasts and sounds: per-event flags,
// burst grouping, quiet hours, Do Not Disturb and pause (SPEC §6). Platform
// toast code sits behind Notifier (desktop-integration skill).
package notify

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"prism/internal/model"
	"prism/internal/settings"
	"prism/internal/store"
)

// Toast action ids. "default" is a click on the toast body.
const (
	ActionOpen    = "open"
	ActionRead    = "read"
	ActionShowAll = "show_all"
	ActionDefault = "default"
)

// Data keys carried by a toast and returned with its action.
const (
	DataIDs = "ids" // comma-separated notification ids
	DataURL = "url"
)

// Action is a toast button.
type Action struct {
	ID    string
	Title string
}

// Toast is one desktop notification.
type Toast struct {
	ID        string
	Title     string
	Body      string
	ImagePath string // local file; remote images are unreliable in toasts
	Actions   []Action
	Data      map[string]string
}

// Notifier shows toasts (Wails notifications service in the app, fake in tests).
type Notifier interface {
	Show(ctx context.Context, t Toast) error
}

// DND reports the OS Do Not Disturb state (best effort per platform).
type DND interface {
	Active() bool
}

// Sounds plays a sound, possibly rate limited (sound.Limited).
type Sounds interface {
	Play(name string, volume float64) (bool, error)
}

// Avatars returns a local image path for a login, "" when unavailable.
type Avatars interface {
	Path(ctx context.Context, login, url string) string
}

const (
	// BurstWindow and BurstSize: ≥ BurstSize toasts within BurstWindow are
	// shown as one summary (SPEC §6).
	BurstWindow = 10 * time.Second
	BurstSize   = 3
)

// Dispatcher turns fresh events into toasts and sounds.
type Dispatcher struct {
	Notifier      Notifier
	Sounds        Sounds
	Avatars       Avatars
	DND           DND
	FallbackImage string // app icon, for events without an avatar
	Now           func() time.Time
	Log           *slog.Logger

	mu          sync.Mutex
	recent      []time.Time // toasts shown within BurstWindow
	pausedUntil time.Time
	seq         int
}

// Pause mutes toasts and sounds until t (tray "Pause 1 h").
func (d *Dispatcher) Pause(until time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.pausedUntil = until
}

// PausedUntil returns the end of the current pause, zero when not paused.
func (d *Dispatcher) PausedUntil() time.Time {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.now().Before(d.pausedUntil) {
		return d.pausedUntil
	}
	return time.Time{}
}

func (d *Dispatcher) now() time.Time {
	if d.Now == nil {
		return time.Now()
	}
	return d.Now()
}

// Muted reports why toasts and sounds are off right now, "" when they are on.
func (d *Dispatcher) Muted(st settings.Settings) string {
	switch {
	case !d.PausedUntil().IsZero():
		return "paused"
	case InQuietHours(st.QuietHours, d.now()):
		return "quiet hours"
	case d.DND != nil && d.DND.Active():
		return "do not disturb"
	}
	return ""
}

// Deliver shows toasts and plays at most one sound for a batch of fresh
// events. Items are already listed by the store; muting never hides them.
func (d *Dispatcher) Deliver(ctx context.Context, fresh []store.Fresh, st settings.Settings) {
	var toastable []store.Fresh
	soundName := ""
	for _, f := range fresh {
		if f.Quiet {
			continue
		}
		flags := st.FlagsFor(f.Event.Type)
		if flags.Toast {
			toastable = append(toastable, f)
		}
		if flags.Sound && st.Sound.Enabled {
			soundName = pickSound(soundName, f.Event.Type, st.Sound)
		}
	}
	if len(toastable) == 0 && soundName == "" {
		return
	}
	if why := d.Muted(st); why != "" {
		d.log().Debug("notifications muted", "reason", why, "events", len(fresh))
		return
	}

	for _, t := range d.toasts(ctx, toastable, st.BurstGrouping) {
		if err := d.Notifier.Show(ctx, t); err != nil {
			d.log().Warn("show toast", "err", err)
		}
	}
	if soundName != "" && d.Sounds != nil {
		if _, err := d.Sounds.Play(soundName, st.Sound.Volume); err != nil {
			d.log().Warn("play sound", "err", err)
		}
	}
}

// toasts builds individual toasts, or one summary when the batch plus the
// toasts of the last BurstWindow reach BurstSize (and the batch has ≥ 2).
func (d *Dispatcher) toasts(ctx context.Context, fresh []store.Fresh, grouping bool) []Toast {
	if len(fresh) == 0 {
		return nil
	}
	d.mu.Lock()
	now := d.now()
	kept := d.recent[:0]
	for _, t := range d.recent {
		if now.Sub(t) < BurstWindow {
			kept = append(kept, t)
		}
	}
	d.recent = kept
	burst := grouping && len(fresh) >= 2 && len(d.recent)+len(fresh) >= BurstSize
	n := 1
	if !burst {
		n = len(fresh)
	}
	for range n {
		d.recent = append(d.recent, now)
	}
	d.seq += n
	seq := d.seq
	d.mu.Unlock()

	if burst {
		return []Toast{summaryToast(fmt.Sprintf("prism-%d", seq), fresh)}
	}
	out := make([]Toast, 0, len(fresh))
	for i, f := range fresh {
		t := d.eventToast(ctx, fmt.Sprintf("prism-%d", seq-len(fresh)+i+1), f)
		out = append(out, t)
	}
	return out
}

func (d *Dispatcher) eventToast(ctx context.Context, id string, f store.Fresh) Toast {
	e := f.Event
	ref := fmt.Sprintf("%s#%d", e.Repo, e.PRNumber)
	t := Toast{ID: id, Data: map[string]string{DataURL: e.URL}, ImagePath: d.FallbackImage}
	if e.Actor.Login != "" && e.Type != model.MyPROpened {
		t.Title = e.Actor.Login + " " + e.Summary
		t.Body = ref + " · " + e.PRTitle
		if e.Excerpt != "" {
			t.Body += "\n“" + e.Excerpt + "”"
		}
		if d.Avatars != nil {
			if p := d.Avatars.Path(ctx, e.Actor.Login, e.Actor.AvatarURL); p != "" {
				t.ImagePath = p
			}
		}
	} else {
		t.Title = ref + " " + e.Summary
		t.Body = e.PRTitle
	}
	t.Actions = []Action{{ID: ActionOpen, Title: "Open PR"}}
	if f.ID != 0 {
		t.Data[DataIDs] = strconv.FormatInt(f.ID, 10)
		t.Actions = append(t.Actions, Action{ID: ActionRead, Title: "Mark as read"})
	}
	return t
}

func summaryToast(id string, fresh []store.Fresh) Toast {
	var ids []string
	for _, f := range fresh {
		if f.ID != 0 {
			ids = append(ids, strconv.FormatInt(f.ID, 10))
		}
	}
	t := Toast{
		ID:      id,
		Title:   fmt.Sprintf("%d new notifications", len(fresh)),
		Body:    Breakdown(fresh),
		Actions: []Action{{ID: ActionShowAll, Title: "Show all"}},
		Data:    map[string]string{DataIDs: strings.Join(ids, ",")},
	}
	if len(ids) > 0 {
		t.Actions = append(t.Actions, Action{ID: ActionRead, Title: "Mark as read"})
	}
	return t
}

// labels are the singular / plural nouns of the summary breakdown.
var labels = map[model.EventType][2]string{
	model.MyPRApproved:         {"approval", "approvals"},
	model.MyPRChangesRequested: {"change request", "change requests"},
	model.MyPRComment:          {"comment", "comments"},
	model.MyPRThreadReply:      {"thread reply", "thread replies"},
	model.CommentReplyToMe:     {"reply to you", "replies to you"},
	model.ReviewRequested:      {"review request", "review requests"},
	model.Mention:              {"mention", "mentions"},
	model.CIFailed:             {"CI failure", "CI failures"},
	model.CIPassed:             {"passing CI run", "passing CI runs"},
	model.MergeConflict:        {"merge conflict", "merge conflicts"},
	model.CommitsAfterMyReview: {"push after your review", "pushes after your review"},
	model.MyPRClosed:           {"closed PR", "closed PRs"},
	model.MyPROpened:           {"opened PR", "opened PRs"},
}

// Breakdown is "2 comments · 1 approval", most frequent first, then SPEC order.
func Breakdown(fresh []store.Fresh) string {
	counts := map[model.EventType]int{}
	for _, f := range fresh {
		counts[f.Event.Type]++
	}
	types := slices.Clone(model.AllEventTypes)
	slices.SortStableFunc(types, func(a, b model.EventType) int { return counts[b] - counts[a] })
	var parts []string
	for _, typ := range types {
		n := counts[typ]
		if n == 0 {
			continue
		}
		l := labels[typ]
		noun := l[1]
		if n == 1 {
			noun = l[0]
		}
		parts = append(parts, fmt.Sprintf("%d %s", n, noun))
	}
	return strings.Join(parts, " · ")
}

// alertEvents use the alert sound (desktop-integration skill §Sound).
var alertEvents = map[model.EventType]bool{model.CIFailed: true, model.MergeConflict: true}

// pickSound keeps the highest-priority sound of a batch: alert > people sound.
func pickSound(current string, t model.EventType, s settings.Sound) string {
	if alertEvents[t] || current == s.AlertName {
		return s.AlertName
	}
	return s.Name
}

// InQuietHours reports whether now (local time) is inside the quiet hours.
// From > To spans midnight; Weekends makes Saturday and Sunday quiet all day.
func InQuietHours(q settings.QuietHours, now time.Time) bool {
	if !q.Enabled {
		return false
	}
	if q.Weekends && (now.Weekday() == time.Saturday || now.Weekday() == time.Sunday) {
		return true
	}
	from, err1 := time.Parse("15:04", q.From)
	to, err2 := time.Parse("15:04", q.To)
	if err1 != nil || err2 != nil {
		return false
	}
	m := now.Hour()*60 + now.Minute()
	f := from.Hour()*60 + from.Minute()
	t := to.Hour()*60 + to.Minute()
	switch {
	case f == t:
		return false
	case f < t:
		return m >= f && m < t
	default:
		return m >= f || m < t
	}
}

// IDs parses a toast's DataIDs value.
func IDs(data map[string]string) []int64 {
	var ids []int64
	for _, s := range strings.Split(data[DataIDs], ",") {
		if id, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64); err == nil {
			ids = append(ids, id)
		}
	}
	return ids
}

func (d *Dispatcher) log() *slog.Logger {
	if d.Log == nil {
		return slog.Default()
	}
	return d.Log
}
