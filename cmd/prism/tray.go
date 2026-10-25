package main

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"prism/internal/app"
	"prism/internal/tray"
)

const pauseFor = time.Hour

// trayController owns the tray icon (badge, offline state) and its menu
// (desktop-integration skill §Tray). Methods are safe on a nil receiver, so
// hooks may fire before the tray exists.
type trayController struct {
	tray  *application.SystemTray
	popup *application.WebviewWindow
	app   *application.App
	core  *app.Core
	log   *slog.Logger

	menu       *application.Menu
	pauseItem  *application.MenuItem
	readItem   *application.MenuItem
	updateItem *application.MenuItem
	updateURL  string

	mu       sync.Mutex
	unread   int
	offline  bool
	signedIn bool
}

func newTrayController(wapp *application.App, popup *application.WebviewWindow, core *app.Core, log *slog.Logger) *trayController {
	t := &trayController{app: wapp, popup: popup, core: core, log: log, tray: wapp.SystemTray.New()}
	t.tray.SetLabel("PRism") // Linux StatusNotifierItem title (default "Wails")

	t.menu = wapp.Menu.New()
	t.updateItem = t.menu.Add("Update available").OnClick(func(*application.Context) {
		t.mu.Lock()
		u := t.updateURL
		t.mu.Unlock()
		if u != "" {
			_ = wapp.Browser.OpenURL(u)
		}
	})
	t.updateItem.SetHidden(true)
	t.menu.Add("Open").OnClick(func(*application.Context) { t.show() })
	t.readItem = t.menu.Add("Mark all as read").OnClick(func(*application.Context) {
		if err := core.MarkAllRead(context.Background()); err != nil {
			log.Warn("mark all read", "err", err)
		}
	})
	t.pauseItem = t.menu.Add("Pause notifications for 1 hour").OnClick(func(*application.Context) { t.togglePause() })
	t.menu.Add("Settings…").OnClick(func(*application.Context) {
		t.show()
		wapp.Event.Emit(eventNav, "settings")
	})
	t.menu.AddSeparator()
	t.menu.Add("Quit PRism").OnClick(func(*application.Context) { wapp.Quit() })

	t.tray.AttachWindow(popup).WindowOffset(8).SetMenu(t.menu)
	t.render()
	return t
}

func (t *trayController) show() {
	if t == nil {
		return
	}
	t.tray.ShowWindow()
	t.popup.Focus()
}

func (t *trayController) setUnread(n int) {
	if t == nil {
		return
	}
	t.mu.Lock()
	t.unread = n
	t.mu.Unlock()
	t.render()
}

func (t *trayController) setOffline(offline bool) {
	if t == nil {
		return
	}
	t.mu.Lock()
	t.offline = offline
	t.mu.Unlock()
	t.render()
}

func (t *trayController) setSignedIn(in bool) {
	if t == nil {
		return
	}
	t.mu.Lock()
	t.signedIn = in
	t.mu.Unlock()
	t.render()
}

// setUpdate shows the "Update available" menu item (release skill §Update check).
func (t *trayController) setUpdate(u app.UpdateInfo) {
	if t == nil {
		return
	}
	t.mu.Lock()
	t.updateURL = u.URL
	t.mu.Unlock()
	t.updateItem.SetLabel("Update available: PRism " + u.Version + "…").SetHidden(false)
	t.menu.Update()
}

func (t *trayController) togglePause() {
	if t.core.PausedUntil().IsZero() {
		t.core.Pause(pauseFor)
		until := time.Now().Add(pauseFor)
		time.AfterFunc(pauseFor, t.render) // relabel when the pause ends
		t.log.Info("notifications paused", "until", until.Format(time.Kitchen))
	} else {
		t.core.Pause(0)
	}
	t.render()
}

// render redraws the icon, tooltip and menu labels from the current state.
func (t *trayController) render() {
	if t == nil {
		return
	}
	t.mu.Lock()
	unread, offline, signedIn := t.unread, t.offline, t.signedIn
	t.mu.Unlock()

	state := tray.StateUnread
	tooltip := fmt.Sprintf("PRism — %d unread", unread)
	switch {
	case !signedIn:
		state, tooltip = tray.StateIdle, "PRism — not signed in"
	case offline:
		state, tooltip = tray.StateError, "PRism — offline, retrying"
	case unread == 0:
		tooltip = "PRism — all caught up"
	}
	light, err := tray.Icon(state, unread, tray.ThemeLight, 32)
	if err != nil {
		t.log.Warn("render tray icon", "err", err)
		return
	}
	dark, err := tray.Icon(state, unread, tray.ThemeDark, 32)
	if err != nil {
		t.log.Warn("render tray icon", "err", err)
		return
	}
	t.tray.SetIcon(light).SetDarkModeIcon(dark)
	t.tray.SetTooltip(tooltip)

	if until := t.core.PausedUntil(); !until.IsZero() {
		t.pauseItem.SetLabel("Resume notifications (paused until " + until.Format("15:04") + ")")
	} else {
		t.pauseItem.SetLabel("Pause notifications for 1 hour")
	}
	t.readItem.SetEnabled(unread > 0)
	t.menu.Update()
}
