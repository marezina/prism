package notify

import (
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
)

// SystemDND is the OS Do Not Disturb state, cached for a few seconds.
// Linux: KDE Plasma exposes org.freedesktop.Notifications "Inhibited"; GNOME
// keeps it in the gsetting org.gnome.desktop.notifications show-banners.
// Anything else counts as "not in DND"; quiet hours are the reliable fallback.
type SystemDND struct {
	mu      sync.Mutex
	checked time.Time
	active  bool
}

const dndCacheFor = 10 * time.Second

func (s *SystemDND) Active() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if time.Since(s.checked) < dndCacheFor {
		return s.active
	}
	s.active, s.checked = inhibited() || gnomeBannersOff(), time.Now()
	return s.active
}

func inhibited() bool {
	conn, err := dbus.SessionBus()
	if err != nil {
		return false
	}
	v, err := conn.Object("org.freedesktop.Notifications", "/org/freedesktop/Notifications").
		GetProperty("org.freedesktop.Notifications.Inhibited")
	if err != nil {
		return false
	}
	b, ok := v.Value().(bool)
	return ok && b
}

func gnomeBannersOff() bool {
	if !strings.Contains(strings.ToUpper(os.Getenv("XDG_CURRENT_DESKTOP")), "GNOME") {
		return false
	}
	out, err := exec.Command("gsettings", "get", "org.gnome.desktop.notifications", "show-banners").Output()
	return err == nil && strings.TrimSpace(string(out)) == "false"
}
