package app

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zalando/go-keyring"

	"prism/internal/auth"
	"prism/internal/gh"
	"prism/internal/model"
	"prism/internal/notify"
	"prism/internal/store"
)

// fakeGitHub serves the endpoints the core touches.
type fakeGitHub struct {
	*httptest.Server
	revoked    atomic.Bool
	tokenPolls atomic.Int32
}

func newFakeGitHub(t *testing.T) *fakeGitHub {
	f := &fakeGitHub{}
	mux := http.NewServeMux()
	writeJSON := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	authed := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if f.revoked.Load() || !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer g") {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			h(w, r)
		}
	}
	mux.HandleFunc("GET /user", authed(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-OAuth-Scopes", "repo, notifications, read:org")
		writeJSON(w, map[string]any{"login": "mona", "id": 1, "avatar_url": "https://avatars/mona"})
	}))
	mux.HandleFunc("GET /user/orgs", authed(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, []any{map[string]any{"login": "acme", "id": 2}})
	}))
	mux.HandleFunc("GET /repos/acme/prism/releases", authed(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, []any{map[string]any{"tag_name": "v0.2.0", "html_url": "https://x/releases/v0.2.0"}})
	}))
	mux.HandleFunc("GET /user/teams", authed(func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, []any{}) }))
	mux.HandleFunc("GET /notifications", authed(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotModified)
	}))
	mux.HandleFunc("POST /graphql", authed(func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Query string }
		_ = json.NewDecoder(r.Body).Decode(&req)
		data := map[string]any{"rateLimit": map[string]any{"cost": 1, "remaining": 4999, "resetAt": "2027-01-01T00:00:00Z"}}
		if strings.Contains(req.Query, "search(") {
			data["search"] = map[string]any{"issueCount": 0, "pageInfo": map[string]any{"hasNextPage": false}, "nodes": []any{}}
		} else {
			data["nodes"] = []any{}
		}
		writeJSON(w, map[string]any{"data": data})
	}))
	mux.HandleFunc("POST /login/device/code", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{"device_code": "dc", "user_code": "WXYZ-1234",
			"verification_uri": "https://github.example/login/device", "expires_in": 900, "interval": 1})
	})
	mux.HandleFunc("POST /login/oauth/access_token", func(w http.ResponseWriter, _ *http.Request) {
		if f.tokenPolls.Add(1) < 2 {
			writeJSON(w, map[string]any{"error": "authorization_pending"})
			return
		}
		writeJSON(w, map[string]any{"access_token": "gho_device"})
	})
	f.Server = httptest.NewServer(mux)
	t.Cleanup(f.Close)
	return f
}

func (f *fakeGitHub) endpoints() gh.Endpoints {
	return gh.Endpoints{Web: f.URL, API: f.URL + "/", GraphQL: f.URL + "/graphql"}
}

type hooks struct {
	mu     sync.Mutex
	auth   []AuthState
	unread []int
	opened []string
	shown  int
	syncs  chan SyncStatus
}

func (h *hooks) lastAuth() AuthState {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.auth) == 0 {
		return AuthState{}
	}
	return h.auth[len(h.auth)-1]
}

func (h *hooks) lastUnread() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.unread[len(h.unread)-1]
}

type rig struct {
	gh    *fakeGitHub
	store *store.Store
	hooks *hooks
	core  *Core
	toast *recordingNotifier
}

type recordingNotifier struct {
	mu    sync.Mutex
	shown []notify.Toast
}

func (r *recordingNotifier) Show(_ context.Context, t notify.Toast) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.shown = append(r.shown, t)
	return nil
}

func newRig(t *testing.T, f *fakeGitHub, st *store.Store) *rig {
	t.Helper()
	h := &hooks{syncs: make(chan SyncStatus, 16)}
	ep := f.endpoints()
	log := slog.New(slog.DiscardHandler)
	session := &auth.Session{
		Store: auth.NewKeyringStore(),
		NewClient: func(token string) (*gh.Client, error) {
			return gh.New(gh.Options{Token: token, Version: "test", Endpoints: ep, Logger: log})
		},
	}
	n := &recordingNotifier{}
	c := New(Options{
		Version: "test", ClientID: "client", Endpoints: ep, Store: st, Session: session, Log: log,
		Dispatcher: &notify.Dispatcher{Notifier: n, Log: log},
		Hooks: Hooks{
			AuthChanged:   func(s AuthState) { h.mu.Lock(); h.auth = append(h.auth, s); h.mu.Unlock() },
			UnreadChanged: func(n int) { h.mu.Lock(); h.unread = append(h.unread, n); h.mu.Unlock() },
			SyncStatus:    func(s SyncStatus) { h.syncs <- s },
			OpenURL:       func(u string) error { h.mu.Lock(); h.opened = append(h.opened, u); h.mu.Unlock(); return nil },
			ShowWindow:    func() { h.mu.Lock(); h.shown++; h.mu.Unlock() },
		},
	})
	t.Cleanup(c.Shutdown)
	return &rig{gh: f, store: st, hooks: h, core: c, toast: n}
}

func openStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "prism.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func waitSync(t *testing.T, h *hooks) SyncStatus {
	t.Helper()
	select {
	case s := <-h.syncs:
		return s
	case <-time.After(5 * time.Second):
		t.Fatal("no sync status")
		return SyncStatus{}
	}
}

func waitAuth(t *testing.T, h *hooks, state string) AuthState {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if s := h.lastAuth(); s.State == state {
			return s
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("auth state %q never reached; last %+v", state, h.lastAuth())
	return AuthState{}
}

func TestPATLoginPickerAndRestart(t *testing.T) {
	keyring.MockInit()
	ctx := context.Background()
	f := newFakeGitHub(t)
	st := openStore(t)
	r := newRig(t, f, st)

	r.core.Start(ctx)
	if s := r.core.Auth(); s.State != StateSignedOut {
		t.Fatalf("start without token: %+v", s)
	}
	if _, err := r.core.LoginWithToken(ctx, "github_pat_x"); err == nil {
		t.Fatal("fine-grained PAT accepted")
	}
	s, err := r.core.LoginWithToken(ctx, " ghp_classic ")
	if err != nil || s.State != StateChooseOrgs || s.Login != "mona" {
		t.Fatalf("login = %+v, %v", s, err)
	}

	orgs, err := r.core.Orgs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(orgs) != 2 || !orgs[0].Personal || !orgs[0].Selected || orgs[1].Login != "acme" || !orgs[1].Selected {
		t.Fatalf("orgs = %+v (personal first, all preselected the first time)", orgs)
	}
	if err := r.core.ChooseOrgs(ctx, []string{"acme"}, true); err != nil {
		t.Fatal(err)
	}
	if s := r.core.Auth(); s.State != StateSignedIn {
		t.Fatalf("after picker: %+v", s)
	}
	if sync := waitSync(t, r.hooks); sync.Offline || sync.LastSync == "" || sync.RateRemaining != 4999 {
		t.Errorf("first sync = %+v", sync)
	}
	if v, _ := st.Meta(ctx, store.MetaBaselineDone); v != "1" {
		t.Error("baseline not done after first sync")
	}
	r.core.Shutdown()

	// Restart: signed in straight away, polling resumes.
	r2 := newRig(t, f, st)
	r2.core.Start(ctx)
	if s := r2.core.Auth(); s.State != StateSignedIn {
		t.Fatalf("restart: %+v", s)
	}
	waitSync(t, r2.hooks)
	orgs, _ = r2.core.Orgs(ctx)
	if !orgs[1].Selected {
		t.Error("picker must reflect saved settings after the first time")
	}

	// Token revoked on GitHub: polling stops, keyring cleared, back to sign-in.
	f.revoked.Store(true)
	r2.core.RefreshNow()
	s = waitAuth(t, r2.hooks, StateSignedOut)
	if !strings.Contains(s.Message, "revoked") {
		t.Errorf("message = %q", s.Message)
	}
	if _, _, err := auth.NewKeyringStore().Load(); err == nil {
		t.Error("keyring not cleared after revocation")
	}
}

func TestDeviceLogin(t *testing.T) {
	keyring.MockInit()
	ctx := context.Background()
	r := newRig(t, newFakeGitHub(t), openStore(t))
	r.core.Start(ctx)

	s, err := r.core.StartLogin(ctx)
	if err != nil || s.State != StateAwaitingUser || s.UserCode != "WXYZ-1234" || s.ExpiresAt == "" {
		t.Fatalf("StartLogin = %+v, %v", s, err)
	}
	if r.hooks.opened[0] != "https://github.example/login/device" {
		t.Errorf("verification page not opened: %v", r.hooks.opened)
	}
	s = waitAuth(t, r.hooks, StateChooseOrgs)
	if s.Login != "mona" {
		t.Errorf("signed in as %+v", s)
	}
	if _, token, _ := auth.NewKeyringStore().Load(); token != "gho_device" {
		t.Errorf("stored token = %q", token)
	}

	if err := r.core.Logout(ctx); err != nil {
		t.Fatal(err)
	}
	if s := r.core.Auth(); s.State != StateSignedOut {
		t.Errorf("after logout: %+v", s)
	}
	if v, _ := r.store.Meta(ctx, MetaOrgsChosen); v == "1" {
		t.Error("logout must reset the organization picker")
	}
}

func TestDeviceLoginWithoutClientID(t *testing.T) {
	keyring.MockInit()
	r := newRig(t, newFakeGitHub(t), openStore(t))
	r.core.opts.ClientID = ""
	if _, err := r.core.StartLogin(context.Background()); err == nil || !strings.Contains(err.Error(), "personal access token") {
		t.Fatalf("err = %v", err)
	}
}

func TestNotificationsAndToastActions(t *testing.T) {
	keyring.MockInit()
	ctx := context.Background()
	st := openStore(t)
	r := newRig(t, newFakeGitHub(t), st)
	ev := func(key string, n int) model.Event {
		return model.Event{Type: model.MyPRComment, DedupKey: key, PRID: "PR_1", Repo: "acme/api", PRNumber: n,
			PRTitle: "Fix", Actor: model.Actor{Login: "bob"}, Summary: "commented on your PR",
			URL: "https://github.com/acme/api/pull/7#" + key, OccurredAt: time.Date(2026, 10, 24, 10, 0, 0, 0, time.UTC)}
	}
	fresh, err := st.Record(ctx, []model.Event{ev("comment:1", 7), ev("comment:2", 7), ev("comment:3", 7)},
		func(model.EventType) bool { return true })
	if err != nil {
		t.Fatal(err)
	}

	list, err := r.core.Unread(ctx)
	if err != nil || len(list) != 3 {
		t.Fatalf("unread = %v, %v", list, err)
	}
	if list[0].PRURL != r.gh.URL+"/acme/api/pull/7" || !list[0].Mine || list[0].OccurredAt != "2026-10-24T10:00:00Z" {
		t.Errorf("dto = %+v", list[0])
	}

	// Toast "Open PR": opens the deep link and marks that one read.
	if err := r.core.HandleToastAction(ctx, notify.ActionOpen, map[string]string{
		notify.DataURL: "https://github.com/acme/api/pull/7#comment:1", notify.DataIDs: itoa(int(fresh[0].ID)),
	}); err != nil {
		t.Fatal(err)
	}
	if r.hooks.opened[len(r.hooks.opened)-1] != "https://github.com/acme/api/pull/7#comment:1" || r.hooks.lastUnread() != 2 {
		t.Errorf("open: opened %v, unread %d", r.hooks.opened, r.hooks.lastUnread())
	}
	// Summary toast: "Show all" opens the window; "Mark as read" marks the burst.
	_ = r.core.HandleToastAction(ctx, notify.ActionShowAll, nil)
	_ = r.core.HandleToastAction(ctx, notify.ActionDefault, map[string]string{notify.DataIDs: "1,2"})
	if r.hooks.shown != 2 {
		t.Errorf("window shown %d times", r.hooks.shown)
	}
	if err := r.core.HandleToastAction(ctx, notify.ActionRead, map[string]string{notify.DataIDs: itoa(int(fresh[1].ID))}); err != nil {
		t.Fatal(err)
	}
	if r.hooks.lastUnread() != 1 {
		t.Errorf("unread after Mark as read = %d", r.hooks.lastUnread())
	}

	if err := r.core.Open(ctx, fresh[2].ID); err != nil {
		t.Fatal(err)
	}
	if r.hooks.lastUnread() != 0 {
		t.Errorf("unread after Open = %d", r.hooks.lastUnread())
	}
	if err := r.core.Open(ctx, 999); err == nil {
		t.Error("Open of unknown id must fail")
	}
}

func TestDeliveryThroughCore(t *testing.T) {
	keyring.MockInit()
	ctx := context.Background()
	st := openStore(t)
	r := newRig(t, newFakeGitHub(t), st)
	e := model.Event{Type: model.MyPRApproved, DedupKey: "review:1", PRID: "PR_1", Repo: "acme/api", PRNumber: 7,
		PRTitle: "Fix", Actor: model.Actor{Login: "alice"}, Summary: "approved your PR", URL: "u"}
	fresh, _ := st.Record(ctx, []model.Event{e}, func(model.EventType) bool { return true })
	r.core.onEvents(fresh)
	if len(r.toast.shown) != 1 || r.toast.shown[0].Title != "alice approved your PR" || r.hooks.lastUnread() != 1 {
		t.Errorf("toasts = %+v, unread = %d", r.toast.shown, r.hooks.lastUnread())
	}
	r.core.Pause(time.Hour)
	if r.core.PausedUntil().IsZero() {
		t.Error("not paused")
	}
	r.core.onEvents(fresh)
	if len(r.toast.shown) != 1 {
		t.Error("toast while paused")
	}
	r.core.Pause(0)
	if !r.core.PausedUntil().IsZero() {
		t.Error("resume failed")
	}
}

func TestResetLocalData(t *testing.T) {
	keyring.MockInit()
	ctx := context.Background()
	st := openStore(t)
	r := newRig(t, newFakeGitHub(t), st)
	r.core.Start(ctx)
	if _, err := r.core.LoginWithToken(ctx, "ghp_x"); err != nil {
		t.Fatal(err)
	}
	if err := r.core.ChooseOrgs(ctx, []string{"acme"}, true); err != nil {
		t.Fatal(err)
	}
	waitSync(t, r.hooks)
	_, _ = st.Record(ctx, []model.Event{{Type: model.MyPRComment, DedupKey: "comment:1", PRID: "PR_1", Repo: "a/b",
		PRNumber: 1, PRTitle: "t", Summary: "s", URL: "u"}}, func(model.EventType) bool { return true })

	if err := r.core.ResetLocalData(ctx); err != nil {
		t.Fatal(err)
	}
	if s := r.core.Auth(); s.State != StateChooseOrgs || s.Login != "mona" {
		t.Errorf("after reset: %+v (signed in, back to the picker)", s)
	}
	if n, _ := st.UnreadCount(ctx); n != 0 || r.hooks.lastUnread() != 0 {
		t.Error("notifications survived reset")
	}
	if _, token, _ := auth.NewKeyringStore().Load(); token != "ghp_x" {
		t.Error("reset must keep the sign-in")
	}
}

func TestUpdateCheck(t *testing.T) {
	keyring.MockInit()
	ctx := context.Background()
	f := newFakeGitHub(t)
	st := openStore(t)
	r := newRig(t, f, st)
	var found []UpdateInfo
	r.core.opts.Hooks.UpdateAvailable = func(u UpdateInfo) { found = append(found, u) }
	r.core.opts.UpdateRepo, r.core.opts.Version = "acme/prism", "v0.1.0"

	client, err := gh.New(gh.Options{Token: "ghp_x", Version: "test", Endpoints: f.endpoints(), Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := r.core.checkUpdateOnce(ctx, client); err != nil {
			t.Fatal(err)
		}
	}
	if len(found) != 2 || r.core.Update().Version != "v0.2.0" {
		t.Errorf("found = %+v, Update() = %+v", found, r.core.Update())
	}
	if len(r.toast.shown) != 1 || r.toast.shown[0].Data["url"] != "https://x/releases/v0.2.0" {
		t.Errorf("want exactly one update toast, got %+v", r.toast.shown)
	}

	r.core.opts.Version = "v0.2.0"
	found = nil
	_ = r.core.checkUpdateOnce(ctx, client)
	if len(found) != 0 {
		t.Error("same version reported as an update")
	}

	// Dev builds never check.
	r.core.opts.Version = "dev"
	done := make(chan struct{})
	go func() { r.core.checkUpdates(ctx, client); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Error("checkUpdates must return at once for dev builds")
	}
}
