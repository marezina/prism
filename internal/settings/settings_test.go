package settings

import (
	"strings"
	"testing"

	"prism/internal/model"
)

func TestDefaultsMatchSpec(t *testing.T) {
	s := Default()
	if err := s.Validate(); err != nil {
		t.Fatalf("defaults invalid: %v", err)
	}
	if len(s.Events) != len(model.AllEventTypes) {
		t.Fatalf("defaults cover %d of %d events", len(s.Events), len(model.AllEventTypes))
	}
	tests := []struct {
		typ  model.EventType
		want Flags
	}{
		{model.MyPRApproved, Flags{true, true, true}},
		{model.CommentReplyToMe, Flags{true, true, true}},
		{model.CIPassed, Flags{true, true, false}},
		{model.CommitsAfterMyReview, Flags{true, true, false}},
		{model.MyPRClosed, Flags{true, false, false}},
		{model.MyPROpened, Flags{true, false, false}},
	}
	for _, tt := range tests {
		if got := s.FlagsFor(tt.typ); got != tt.want {
			t.Errorf("%s = %+v, want %+v", tt.typ, got, tt.want)
		}
	}
	if s.PollInterval().Seconds() != 120 || !s.IgnoreBots || !s.IncludeDrafts || s.Theme != "system" {
		t.Errorf("defaults = %+v", s)
	}
}

func TestDecodeMergesOverDefaults(t *testing.T) {
	stored := `{"owners":["acme"],"pollIntervalSec":60,"ignoreBots":false,
		"events":{"my_pr.approved":{"list":true,"toast":false,"sound":false},"gone.event":{"list":true}},
		"sound":{"enabled":false,"volume":0.2,"name":"ding","alertName":"alert"}}`
	s, err := Decode([]byte(stored))
	if err != nil {
		t.Fatal(err)
	}
	if s.PollIntervalSec != 60 || s.IgnoreBots || len(s.Owners) != 1 || s.Sound.Name != "ding" {
		t.Errorf("stored values lost: %+v", s)
	}
	if !s.IncludeDrafts || s.Theme != "system" || !s.BurstGrouping {
		t.Errorf("missing fields must keep defaults: %+v", s)
	}
	if got := s.FlagsFor(model.MyPRApproved); got != (Flags{List: true}) {
		t.Errorf("stored event flags lost: %+v", got)
	}
	if got := s.FlagsFor(model.CIFailed); got != (Flags{true, true, true}) {
		t.Errorf("event missing from storage must get its default: %+v", got)
	}
	if _, ok := s.Events["gone.event"]; ok {
		t.Error("unknown stored event must be dropped")
	}
	if err := s.Validate(); err != nil {
		t.Errorf("decoded settings invalid: %v", err)
	}

	empty, err := Decode(nil)
	if err != nil || empty.PollIntervalSec != 120 || empty.Owners == nil {
		t.Errorf("Decode(nil) = %+v, %v", empty, err)
	}
	if _, err := Decode([]byte("{")); err == nil {
		t.Error("want error for bad JSON")
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Settings)
		want   string
	}{
		{"interval", func(s *Settings) { s.PollIntervalSec = 30 }, "pollIntervalSec"},
		{"volume", func(s *Settings) { s.Sound.Volume = 1.5 }, "volume"},
		{"sound name", func(s *Settings) { s.Sound.Name = "" }, "sound names"},
		{"quiet hours", func(s *Settings) { s.QuietHours.From = "25:00" }, "quiet hours"},
		{"theme", func(s *Settings) { s.Theme = "pink" }, "theme"},
		{"event", func(s *Settings) { s.Events["nope"] = Flags{} }, "unknown event"},
	}
	for _, tt := range tests {
		s := Default()
		tt.mutate(&s)
		if err := s.Validate(); err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: err = %v, want %q", tt.name, err, tt.want)
		}
	}
}
