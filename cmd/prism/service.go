package main

import (
	"context"
	"errors"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"prism/internal/app"
	"prism/internal/autostart"
	"prism/internal/settings"
	"prism/internal/sound"
)

// AppService is bound to the frontend (ARCHITECTURE §7). Every method is a
// thin call into internal/app.
type AppService struct {
	core    *app.Core
	app     *application.App
	tray    *trayController
	sounds  *sound.OtoPlayer
	dataDir string
	log     *slog.Logger
}

// Unread returns the unread notifications, newest first.
func (s *AppService) Unread(ctx context.Context) ([]app.Notification, error) {
	return s.core.Unread(ctx)
}

// MarkRead marks notifications read (deletes them).
func (s *AppService) MarkRead(ctx context.Context, ids []int64) error {
	return s.core.MarkRead(ctx, ids)
}

// MarkPRRead marks every notification of a PR read.
func (s *AppService) MarkPRRead(ctx context.Context, prID string) error {
	return s.core.MarkPRRead(ctx, prID)
}

// MarkAllRead marks everything read.
func (s *AppService) MarkAllRead(ctx context.Context) error { return s.core.MarkAllRead(ctx) }

// Open opens a notification's link and marks it read.
func (s *AppService) Open(ctx context.Context, id int64) error { return s.core.Open(ctx, id) }

// OpenURL opens a link in the browser (PR page, SSO authorize page).
func (s *AppService) OpenURL(link string) error {
	u, err := url.Parse(link)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return errors.New("only web links can be opened")
	}
	return s.core.OpenURL(link)
}

// Settings returns the current settings.
func (s *AppService) Settings(ctx context.Context) (settings.Settings, error) {
	return s.core.Settings(ctx)
}

// SaveSettings validates and applies settings.
func (s *AppService) SaveSettings(ctx context.Context, st settings.Settings) error {
	if err := s.core.SaveSettings(ctx, st); err != nil {
		return err
	}
	applyAutostart(st.Autostart, s.log)
	return nil
}

// applyAutostart registers PRism to start at login. Dev builds never touch
// the login items, so running from the repo does not leave traces.
func applyAutostart(enabled bool, log *slog.Logger) {
	if version == "dev" {
		log.Debug("autostart not applied in dev builds", "enabled", enabled)
		return
	}
	if err := autostart.Set(enabled); err != nil {
		log.Warn("autostart", "enabled", enabled, "err", err)
	}
}

// SoundNames lists the embedded sounds for the settings picker.
func (s *AppService) SoundNames() []string { return sound.Names() }

// PreviewSound plays a sound now (settings), ignoring the rate limit.
func (s *AppService) PreviewSound(name string, volume float64) error {
	if s.sounds == nil {
		return errors.New("no audio output available")
	}
	return s.sounds.Play(name, volume)
}

// Auth returns the current sign-in state.
func (s *AppService) Auth() app.AuthState { return s.core.Auth() }

// StartLogin starts the device flow and returns the code to show.
func (s *AppService) StartLogin(ctx context.Context) (app.AuthState, error) {
	return s.core.StartLogin(ctx)
}

// CancelLogin abandons the device flow.
func (s *AppService) CancelLogin() { s.core.CancelLogin() }

// LoginWithToken signs in with a classic personal access token.
func (s *AppService) LoginWithToken(ctx context.Context, token string) (app.AuthState, error) {
	return s.core.LoginWithToken(ctx, token)
}

// Orgs lists the organization picker rows.
func (s *AppService) Orgs(ctx context.Context) ([]app.Org, error) { return s.core.Orgs(ctx) }

// ChooseOrgs saves the organization selection and starts watching.
func (s *AppService) ChooseOrgs(ctx context.Context, owners []string, includePersonal bool) error {
	return s.core.ChooseOrgs(ctx, owners, includePersonal)
}

// Logout signs out.
func (s *AppService) Logout(ctx context.Context) error { return s.core.Logout(ctx) }

// Status returns the last sync status.
func (s *AppService) Status() app.SyncStatus { return s.core.Status() }

// RefreshNow polls GitHub immediately.
func (s *AppService) RefreshNow() { s.core.RefreshNow() }

// Paused returns the end of a notification pause as RFC3339, "" when not paused.
func (s *AppService) Paused() string {
	if t := s.core.PausedUntil(); !t.IsZero() {
		return t.UTC().Format(time.RFC3339)
	}
	return ""
}

// Update returns a newer release found by the update check (empty when none).
func (s *AppService) Update() app.UpdateInfo { return s.core.Update() }

// Version is the app version for Settings → Advanced.
func (s *AppService) Version() string { return version }

// OpenLogsFolder opens the log directory in the file manager.
func (s *AppService) OpenLogsFolder() error {
	return s.app.Browser.OpenFile(filepath.Join(s.dataDir, "logs"))
}

// ResetLocalData deletes local notifications, settings and cached avatars,
// keeping the sign-in.
func (s *AppService) ResetLocalData(ctx context.Context) error {
	if err := s.core.ResetLocalData(ctx); err != nil {
		return err
	}
	if err := os.RemoveAll(filepath.Join(s.dataDir, "avatars")); err != nil {
		s.log.Warn("remove avatar cache", "err", err)
	}
	return nil
}

// HideWindow hides the popup (e.g. after opening a link).
func (s *AppService) HideWindow() {
	if s.tray != nil {
		s.tray.tray.HideWindow()
	}
}
