// Package app is PRism's application core: session, polling, delivery and
// the operations the UI calls (ARCHITECTURE §7). It does not import Wails;
// cmd/prism binds it to the window, tray and toasts.
package app

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"prism/internal/auth"
	"prism/internal/gh"
	"prism/internal/notify"
	"prism/internal/poller"
	"prism/internal/settings"
	"prism/internal/store"
)

// Hooks are how the core tells the UI about changes. All may be nil.
type Hooks struct {
	UnreadChanged func(count int)
	AuthChanged   func(AuthState)
	SyncStatus    func(SyncStatus)
	OpenURL       func(url string) error
	ShowWindow    func()
	// UpdateAvailable fires once per check that finds a newer release.
	UpdateAvailable func(UpdateInfo)
}

// Options configure a Core.
type Options struct {
	Version    string
	ClientID   string
	Endpoints  gh.Endpoints
	Store      *store.Store
	Session    *auth.Session
	Dispatcher *notify.Dispatcher
	Hooks      Hooks
	Log        *slog.Logger

	// UpdateRepo is "owner/name" of the release repo; empty disables update checks.
	UpdateRepo string

	// NewPoller is replaced in tests.
	NewPoller func(poller.Options) Poller
}

// Poller is the part of *poller.Poller the core drives.
type Poller interface {
	Run(ctx context.Context) error
	Trigger()
}

// Core is the running application.
type Core struct {
	opts Options
	log  *slog.Logger

	mu       sync.Mutex
	client   *gh.Client
	identity gh.Identity
	auth     AuthState
	poller   Poller
	stop     context.CancelFunc
	pollDone chan struct{}
	sync     SyncStatus
	login    *loginAttempt
	update   UpdateInfo
}

// MetaOrgsChosen marks that the user finished the organization picker once.
const MetaOrgsChosen = "orgs_chosen"

// New returns a Core; call Start.
func New(o Options) *Core {
	if o.Log == nil {
		o.Log = slog.Default()
	}
	if o.Endpoints == (gh.Endpoints{}) {
		o.Endpoints = gh.DotCom
	}
	if o.NewPoller == nil {
		o.NewPoller = func(po poller.Options) Poller { return poller.New(po) }
	}
	return &Core{opts: o, log: o.Log, auth: AuthState{State: StateSignedOut}}
}

// Start restores the session and starts polling when possible.
func (c *Core) Start(ctx context.Context) {
	client, id, err := c.opts.Session.Restore(ctx)
	switch {
	case errors.Is(err, auth.ErrNoToken):
		c.setAuth(AuthState{State: StateSignedOut})
	case errors.Is(err, gh.ErrUnauthorized):
		c.setAuth(AuthState{State: StateSignedOut, Message: "Your GitHub session expired or was revoked. Sign in again."})
	case errors.Is(err, auth.ErrKeyringUnavailable):
		c.setAuth(AuthState{State: StateSignedOut, Message: err.Error()})
	case err != nil:
		// Offline at start: keep the token, retry by polling later.
		c.log.Warn("restore session", "err", err)
		c.setAuth(AuthState{State: StateSignedOut, Message: "Could not reach GitHub: " + err.Error()})
	default:
		c.signedIn(ctx, client, id)
	}
	c.notifyUnread(ctx)
}

// Stop stops polling and waits for it to end.
func (c *Core) Stop() {
	c.mu.Lock()
	stop, done := c.stop, c.pollDone
	c.stop, c.pollDone, c.poller = nil, nil, nil
	c.mu.Unlock()
	if stop != nil {
		stop()
		<-done
	}
}

// Shutdown stops polling and any sign-in in progress (app quit).
func (c *Core) Shutdown() {
	c.cancelLogin()
	c.Stop()
}

// signedIn records the identity and either asks for organizations or polls.
func (c *Core) signedIn(ctx context.Context, client *gh.Client, id gh.Identity) {
	c.mu.Lock()
	c.client, c.identity = client, id
	c.mu.Unlock()
	if err := c.opts.Store.SetMeta(ctx, store.MetaMeLogin, id.Login); err != nil {
		c.log.Warn("store login", "err", err)
	}
	chosen, _ := c.opts.Store.Meta(ctx, MetaOrgsChosen)
	if chosen != "1" {
		c.setAuth(AuthState{State: StateChooseOrgs, Login: id.Login, AvatarURL: id.AvatarURL})
		return
	}
	c.setAuth(AuthState{State: StateSignedIn, Login: id.Login, AvatarURL: id.AvatarURL})
	c.startPolling(client, id)
}

func (c *Core) startPolling(client *gh.Client, id gh.Identity) {
	c.Stop()
	ctx, cancel := context.WithCancel(context.Background())
	teams, err := client.Teams(ctx)
	if err != nil {
		c.log.Warn("list teams; team review requests and mentions are off until restart", "err", err)
	}
	p := c.opts.NewPoller(poller.Options{
		GitHub:   client,
		Store:    c.opts.Store,
		Me:       poller.Identity{Login: id.Login, Teams: teams},
		Log:      c.log,
		OnEvents: c.onEvents,
		OnStatus: c.onStatus,
	})
	done := make(chan struct{})
	c.mu.Lock()
	c.poller, c.stop, c.pollDone = p, cancel, done
	c.mu.Unlock()

	go c.checkUpdates(ctx, client)
	go func() {
		defer close(done)
		err := p.Run(ctx)
		if errors.Is(err, gh.ErrUnauthorized) {
			c.log.Warn("token rejected by GitHub; signing out")
			_ = c.opts.Session.Logout()
			c.mu.Lock()
			if c.pollDone == done {
				c.client, c.poller, c.stop, c.pollDone = nil, nil, nil, nil
			}
			c.mu.Unlock()
			c.setAuth(AuthState{State: StateSignedOut, Message: "Your GitHub session expired or was revoked. Sign in again."})
		}
	}()
}

func (c *Core) onEvents(fresh []store.Fresh) {
	ctx := context.Background()
	if c.opts.Dispatcher != nil {
		if st, err := c.opts.Store.Settings(ctx); err == nil {
			c.opts.Dispatcher.Deliver(ctx, fresh, st)
		}
	}
	c.notifyUnread(ctx)
}

func (c *Core) onStatus(s poller.Status) {
	st := SyncStatus{Watched: s.Watched, RateRemaining: s.RateRemaining, Skipped: []SkippedOrg{}}
	for _, sk := range s.Skipped {
		st.Skipped = append(st.Skipped, SkippedOrg{Login: sk.Owner, SSOURL: sk.SSOURL, Error: sk.Err})
	}
	if !s.LastSync.IsZero() {
		st.LastSync = s.LastSync.UTC().Format(time.RFC3339)
	}
	if s.Err != nil {
		st.Error = s.Err.Error()
		st.Offline = true
		var sso *gh.SSORequiredError
		if errors.As(s.Err, &sso) {
			st.SSOURL = sso.URL
		}
	}
	c.mu.Lock()
	c.sync = st
	c.mu.Unlock()
	if h := c.opts.Hooks.SyncStatus; h != nil {
		h(st)
	}
}

// Status returns the last sync status.
func (c *Core) Status() SyncStatus {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sync
}

// RefreshNow polls immediately.
func (c *Core) RefreshNow() {
	c.mu.Lock()
	p := c.poller
	c.mu.Unlock()
	if p != nil {
		p.Trigger()
	}
}

// Pause mutes toasts and sounds for d (0 resumes).
func (c *Core) Pause(d time.Duration) {
	if c.opts.Dispatcher == nil {
		return
	}
	until := time.Time{}
	if d > 0 {
		until = time.Now().Add(d)
	}
	c.opts.Dispatcher.Pause(until)
}

// PausedUntil reports the end of a pause, zero when not paused.
func (c *Core) PausedUntil() time.Time {
	if c.opts.Dispatcher == nil {
		return time.Time{}
	}
	return c.opts.Dispatcher.PausedUntil()
}

// Settings returns the current settings.
func (c *Core) Settings(ctx context.Context) (settings.Settings, error) {
	return c.opts.Store.Settings(ctx)
}

// SaveSettings validates, stores and applies new settings: the watch set is
// rebuilt and PRs of unselected owners are purged on the next cycle.
func (c *Core) SaveSettings(ctx context.Context, s settings.Settings) error {
	if err := c.opts.Store.SaveSettings(ctx, s); err != nil {
		return err
	}
	c.RefreshNow()
	return nil
}

func (c *Core) setAuth(s AuthState) {
	c.mu.Lock()
	c.auth = s
	c.mu.Unlock()
	if h := c.opts.Hooks.AuthChanged; h != nil {
		h(s)
	}
}

// Auth returns the current auth state.
func (c *Core) Auth() AuthState {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.auth
}

func (c *Core) notifyUnread(ctx context.Context) {
	if h := c.opts.Hooks.UnreadChanged; h != nil {
		n, err := c.opts.Store.UnreadCount(ctx)
		if err != nil {
			c.log.Warn("count unread", "err", err)
			return
		}
		h(n)
	}
}

// prURL is the PR's page on the web endpoint.
func (c *Core) prURL(repo string, number int) string {
	return strings.TrimSuffix(c.opts.Endpoints.Web, "/") + "/" + repo + "/pull/" + itoa(number)
}

// ResetLocalData deletes every local notification, PR state, setting and
// cached avatar, keeping the sign-in. A signed-in user goes back to the
// organization picker; the next poll baselines silently again.
func (c *Core) ResetLocalData(ctx context.Context) error {
	c.Stop()
	if err := c.opts.Store.Reset(ctx); err != nil {
		return err
	}
	c.notifyUnread(ctx)
	c.mu.Lock()
	client, id := c.client, c.identity
	c.mu.Unlock()
	if client == nil {
		c.setAuth(AuthState{State: StateSignedOut})
		return nil
	}
	c.signedIn(ctx, client, id)
	return nil
}
