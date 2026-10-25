// Command prism is the PRism desktop app: tray icon, popup window and toasts
// for GitHub pull-request activity. The product logic lives in internal/app;
// this package binds it to Wails.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"

	"prism/assets"
	"prism/frontend"
	"prism/internal/app"
	"prism/internal/auth"
	"prism/internal/avatar"
	"prism/internal/gh"
	"prism/internal/notify"
	"prism/internal/sound"
	"prism/internal/store"
	"prism/internal/update"
)

// version is set at build time with -ldflags "-X main.version=<tag>".
var version = "dev"

// Events pushed to the frontend (ARCHITECTURE §7).
const (
	eventUnread = "unread:changed"
	eventAuth   = "auth:changed"
	eventSync   = "sync:status"
	eventNav    = "nav" // "settings" | "inbox": the tray asks the UI to switch view
	eventUpdate = "update:available"
)

func init() {
	application.RegisterEvent[int](eventUnread)
	application.RegisterEvent[app.AuthState](eventAuth)
	application.RegisterEvent[app.SyncStatus](eventSync)
	application.RegisterEvent[string](eventNav)
	application.RegisterEvent[app.UpdateInfo](eventUpdate)
}

func main() {
	background := flag.Bool("background", false, "start in the tray without opening the window")
	debug := flag.Bool("debug", false, "debug logging")
	flag.Parse()

	if err := run(*background, *debug); err != nil {
		fmt.Fprintln(os.Stderr, "PRism:", err)
		os.Exit(1)
	}
}

func run(background, debug bool) error {
	ctx := context.Background()
	dataDir, err := store.DataDir()
	if err != nil {
		return err
	}
	log, closeLog, err := newLogger(filepath.Join(dataDir, "logs"), debug)
	if err != nil {
		return err
	}
	defer closeLog()
	log.Info("starting PRism", "version", version, "data", dataDir)

	db, err := store.Open(ctx, filepath.Join(dataDir, "prism.db"))
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	iconPath, err := writeAppIcon(dataDir)
	if err != nil {
		log.Warn("write app icon for toasts", "err", err)
	}
	var sounds notify.Sounds
	player, err := sound.NewOtoPlayer()
	if err != nil {
		log.Warn("sound disabled", "err", err)
	} else {
		sounds = sound.NewLimited(player)
	}

	notifier := notifications.New()
	toasts := &wailsToasts{svc: notifier, log: log}
	dispatcher := &notify.Dispatcher{
		Notifier: toasts, Sounds: sounds, DND: &notify.SystemDND{}, FallbackImage: iconPath, Log: log,
		Avatars: avatar.New(filepath.Join(dataDir, "avatars"), db, version, log),
	}
	session := &auth.Session{
		Store: auth.NewKeyringStore(),
		NewClient: func(token string) (*gh.Client, error) {
			return gh.New(gh.Options{Token: token, Version: version, Logger: log})
		},
	}

	var (
		wapp *application.App
		tray *trayController
	)
	emit := func(name string, data any) {
		if wapp != nil {
			wapp.Event.Emit(name, data)
		}
	}
	core := app.New(app.Options{
		Version: version, ClientID: auth.ClientID, UpdateRepo: update.Repo, Store: db, Session: session,
		Dispatcher: dispatcher, Log: log,
		Hooks: app.Hooks{
			UnreadChanged: func(n int) { tray.setUnread(n); emit(eventUnread, n) },
			AuthChanged: func(s app.AuthState) {
				emit(eventAuth, s)
				tray.setSignedIn(s.State == app.StateSignedIn)
				if s.State == app.StateChooseOrgs {
					tray.show() // back from the browser: show the organization picker
				}
			},
			SyncStatus: func(s app.SyncStatus) { tray.setOffline(s.Offline); emit(eventSync, s) },
			OpenURL:    func(u string) error { return wapp.Browser.OpenURL(u) },
			ShowWindow: func() { tray.show() },
			UpdateAvailable: func(u app.UpdateInfo) {
				tray.setUpdate(u)
				emit(eventUpdate, u)
			},
		},
	})
	service := &AppService{core: core, sounds: player, dataDir: dataDir, log: log}

	appIcon, _ := assets.AppPNG.ReadFile("icons/png/app/prism-256.png")
	wapp = application.New(application.Options{
		// On Windows the notifications service uses Name as the AppUserModelID (ADR 0002).
		Name:        "PRism",
		Description: "GitHub pull-request notifications",
		Icon:        appIcon,
		Logger:      log,
		Services: []application.Service{
			application.NewService(notifier),
			application.NewService(service),
		},
		Assets: application.AssetOptions{Handler: application.AssetFileServerFS(frontend.Dist())},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID:               "desktop.prism",
			OnSecondInstanceLaunch: func(application.SecondInstanceData) { tray.show() },
		},
		OnShutdown: core.Shutdown,
	})
	service.app = wapp

	popup := wapp.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:            "popup",
		Title:           "PRism",
		Width:           440,
		Height:          640,
		Frameless:       true,
		Hidden:          true,
		DisableResize:   true,
		AlwaysOnTop:     true,
		HideOnFocusLost: true,
		HideOnEscape:    true,
		// Transparent so the UI can draw its own rounded, bordered frame.
		BackgroundType:   application.BackgroundTypeTransparent,
		BackgroundColour: application.NewRGBA(0, 0, 0, 0),
		URL:              "/",
		Windows:          application.WindowsWindow{HiddenOnTaskbar: true},
	})
	// Closing a window never quits PRism; only the tray "Quit" does.
	popup.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		popup.Hide()
		e.Cancel()
	})

	tray = newTrayController(wapp, popup, core, log)
	service.tray = tray

	notifier.OnNotificationResponse(func(r notifications.NotificationResult) {
		if r.Error != nil {
			log.Warn("toast response", "err", r.Error)
			return
		}
		data := map[string]string{}
		for k, v := range r.Response.UserInfo {
			if s, ok := v.(string); ok {
				data[k] = s
			}
		}
		if err := core.HandleToastAction(ctx, r.Response.ActionIdentifier, data); err != nil {
			log.Warn("toast action", "action", r.Response.ActionIdentifier, "err", err)
		}
	})

	wapp.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		toasts.registerCategories()
		go func() {
			makeTransparent(popup) // dispatches to the GTK main thread; never call it from there
			if st, err := db.Settings(ctx); err == nil {
				applyAutostart(st.Autostart, log)
			}
			core.Start(ctx)
			if !background && core.Auth().State != app.StateSignedIn {
				tray.show() // first run or signed out: show the welcome screen
			}
		}()
	})

	return wapp.Run()
}

// writeAppIcon puts the app icon on disk: toasts take local image files only.
func writeAppIcon(dataDir string) (string, error) {
	b, err := assets.AppPNG.ReadFile("icons/png/app/prism-128.png")
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return "", err
	}
	p := filepath.Join(dataDir, "prism.png")
	return p, os.WriteFile(p, b, 0o644)
}
