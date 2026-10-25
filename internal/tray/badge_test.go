package tray

import (
	"bytes"
	"image/png"
	"testing"
)

func TestBadgeLabel(t *testing.T) {
	tests := []struct {
		count int
		want  string
	}{
		{1, "1"},
		{9, "9"},
		{42, "42"},
		{99, "99"},
		{100, "99+"},
		{12345, "99+"},
	}
	for _, tt := range tests {
		if got := BadgeLabel(tt.count); got != tt.want {
			t.Errorf("BadgeLabel(%d) = %q, want %q", tt.count, got, tt.want)
		}
	}
}

func TestIcon(t *testing.T) {
	tests := []struct {
		name      string
		state     State
		count     int
		wantBadge bool
	}{
		{"idle", StateIdle, 0, false},
		{"unread zero is idle", StateUnread, 0, false},
		{"unread one", StateUnread, 1, true},
		{"unread two digits", StateUnread, 42, true},
		{"unread capped", StateUnread, 250, true},
		{"error", StateError, 5, false},
	}
	for _, tt := range tests {
		for _, theme := range []Theme{ThemeLight, ThemeDark} {
			for _, size := range Sizes {
				b, err := Icon(tt.state, tt.count, theme, size)
				if err != nil {
					t.Fatalf("%s/%d/%d: %v", tt.name, theme, size, err)
				}
				img, err := png.Decode(bytes.NewReader(b))
				if err != nil {
					t.Fatalf("%s/%d/%d: decode: %v", tt.name, theme, size, err)
				}
				if d := img.Bounds().Dx(); d != size || img.Bounds().Dy() != size {
					t.Errorf("%s/%d/%d: got %dx%d", tt.name, theme, size, d, img.Bounds().Dy())
				}
				// A badge fills a good part of the top-right quadrant with red.
				red := 0
				for y := 0; y < size/2; y++ {
					for x := size / 2; x < size; x++ {
						r, g, _, a := img.At(x, y).RGBA()
						if a > 0xC000 && r > 0xB000 && g < 0x6000 {
							red++
						}
					}
				}
				hasBadge := red > size*size/16
				if hasBadge != tt.wantBadge {
					t.Errorf("%s/%d/%d: badge present=%v (%d red px), want %v", tt.name, theme, size, hasBadge, red, tt.wantBadge)
				}
			}
		}
	}
}

func TestIconCappedCountsShareCache(t *testing.T) {
	a, _ := Icon(StateUnread, 100, ThemeLight, 24)
	b, _ := Icon(StateUnread, 999, ThemeLight, 24)
	if !bytes.Equal(a, b) {
		t.Error("counts above MaxBadge must render identically")
	}
	idle, _ := Icon(StateIdle, 0, ThemeLight, 24)
	zero, _ := Icon(StateUnread, 0, ThemeLight, 24)
	if !bytes.Equal(idle, zero) {
		t.Error("unread with count 0 must equal idle")
	}
}
