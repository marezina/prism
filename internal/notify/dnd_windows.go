package notify

import (
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// SystemDND is the OS Do Not Disturb state, cached for a few seconds.
// Windows: SHQueryUserNotificationState; anything but "accepts notifications"
// (presentation mode, full-screen app, quiet time, busy) counts as DND.
// Focus Assist itself has no public API; Windows suppresses toast banners
// on its own, and PRism's sound follows this best-effort state.
type SystemDND struct {
	mu      sync.Mutex
	checked time.Time
	active  bool
}

const (
	dndCacheFor             = 10 * time.Second
	qunsAcceptsNotification = 5 // QUNS_ACCEPTS_NOTIFICATIONS
)

var procQueryNotificationState = windows.NewLazySystemDLL("shell32.dll").NewProc("SHQueryUserNotificationState")

func (s *SystemDND) Active() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if time.Since(s.checked) < dndCacheFor {
		return s.active
	}
	s.active, s.checked = queryNotificationState(), time.Now()
	return s.active
}

func queryNotificationState() bool {
	if procQueryNotificationState.Find() != nil {
		return false
	}
	var state int32
	hr, _, _ := procQueryNotificationState.Call(uintptr(unsafe.Pointer(&state)))
	return hr == 0 && state != qunsAcceptsNotification
}
