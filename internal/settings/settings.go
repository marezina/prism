// Package settings defines typed user settings, their defaults (SPEC §7) and
// validation.
package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"prism/internal/model"
)

// Flags are the per-event delivery switches.
type Flags struct {
	List  bool `json:"list"`
	Toast bool `json:"toast"`
	Sound bool `json:"sound"`
}

// Sound settings.
type Sound struct {
	Enabled   bool    `json:"enabled"`
	Volume    float64 `json:"volume"`
	Name      string  `json:"name"`      // people events
	AlertName string  `json:"alertName"` // CI failed, conflicts
}

// QuietHours mute toasts and sounds; items are still listed.
type QuietHours struct {
	Enabled  bool   `json:"enabled"`
	From     string `json:"from"` // "HH:MM", local time
	To       string `json:"to"`
	Weekends bool   `json:"weekends"` // quiet all weekend
}

// Settings is SPEC §7.
type Settings struct {
	Owners          []string                  `json:"owners"`
	IncludePersonal bool                      `json:"includePersonal"`
	IgnoreBots      bool                      `json:"ignoreBots"`
	IncludeDrafts   bool                      `json:"includeDrafts"`
	PollIntervalSec int                       `json:"pollIntervalSec"`
	Events          map[model.EventType]Flags `json:"events"`
	Sound           Sound                     `json:"sound"`
	QuietHours      QuietHours                `json:"quietHours"`
	BurstGrouping   bool                      `json:"burstGrouping"`
	Autostart       bool                      `json:"autostart"`
	Theme           string                    `json:"theme"` // system | light | dark
}

// PollIntervals are the allowed state-poll intervals in seconds.
var PollIntervals = []int{60, 120, 300}

// Themes are the allowed theme values.
var Themes = []string{"system", "light", "dark"}

// DefaultFlags are the delivery defaults per event (SPEC §3 tables).
func DefaultFlags() map[model.EventType]Flags {
	all := Flags{List: true, Toast: true, Sound: true}
	silent := Flags{List: true, Toast: true}
	listOnly := Flags{List: true}
	return map[model.EventType]Flags{
		model.MyPRApproved:         all,
		model.MyPRChangesRequested: all,
		model.MyPRComment:          all,
		model.MyPRThreadReply:      all,
		model.CommentReplyToMe:     all,
		model.ReviewRequested:      all,
		model.Mention:              all,
		model.CIFailed:             all,
		model.CIPassed:             silent,
		model.MergeConflict:        all,
		model.CommitsAfterMyReview: silent,
		model.MyPRClosed:           listOnly,
		model.MyPROpened:           listOnly,
	}
}

// Default returns SPEC §7 defaults. Owners are empty until the user picks
// organizations after login (SPEC §8).
func Default() Settings {
	return Settings{
		Owners:          []string{},
		IncludePersonal: true,
		IgnoreBots:      true,
		IncludeDrafts:   true,
		PollIntervalSec: 120,
		Events:          DefaultFlags(),
		Sound:           Sound{Enabled: true, Volume: 0.7, Name: "soft-chime", AlertName: "alert"},
		QuietHours:      QuietHours{From: "18:00", To: "08:00", Weekends: true},
		BurstGrouping:   true,
		Autostart:       true,
		Theme:           "system",
	}
}

// Decode reads stored JSON on top of the defaults, so settings saved by an
// older version gain new fields and events with their default values.
func Decode(b []byte) (Settings, error) {
	s := Default()
	if len(b) == 0 {
		return s, nil
	}
	stored := Default()
	stored.Events = nil
	if err := json.Unmarshal(b, &stored); err != nil {
		return s, fmt.Errorf("decode settings: %w", err)
	}
	for typ, f := range stored.Events {
		if _, known := s.Events[typ]; known {
			s.Events[typ] = f
		}
	}
	stored.Events = s.Events
	if stored.Owners == nil {
		stored.Owners = []string{}
	}
	return stored, nil
}

// Validate reports the first invalid value.
func (s Settings) Validate() error {
	var errs []error
	if !slices.Contains(PollIntervals, s.PollIntervalSec) {
		errs = append(errs, fmt.Errorf("pollIntervalSec %d: want one of %v", s.PollIntervalSec, PollIntervals))
	}
	if s.Sound.Volume < 0 || s.Sound.Volume > 1 {
		errs = append(errs, fmt.Errorf("sound.volume %v: want 0–1", s.Sound.Volume))
	}
	if s.Sound.Name == "" || s.Sound.AlertName == "" {
		errs = append(errs, errors.New("sound names must not be empty"))
	}
	for _, hm := range []string{s.QuietHours.From, s.QuietHours.To} {
		if _, err := time.Parse("15:04", hm); err != nil {
			errs = append(errs, fmt.Errorf("quiet hours %q: want HH:MM", hm))
		}
	}
	if !slices.Contains(Themes, s.Theme) {
		errs = append(errs, fmt.Errorf("theme %q: want one of %v", s.Theme, Themes))
	}
	for typ := range s.Events {
		if !slices.Contains(model.AllEventTypes, typ) {
			errs = append(errs, fmt.Errorf("unknown event %q", typ))
		}
	}
	return errors.Join(errs...)
}

// FlagsFor returns the delivery flags for an event type.
func (s Settings) FlagsFor(t model.EventType) Flags { return s.Events[t] }

// PollInterval is PollIntervalSec as a duration.
func (s Settings) PollInterval() time.Duration { return time.Duration(s.PollIntervalSec) * time.Second }
