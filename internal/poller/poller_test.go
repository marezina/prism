package poller

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"prism/internal/gh"
	"prism/internal/model"
	"prism/internal/settings"
	"prism/internal/store"
)

var t0 = time.Date(2026, 10, 24, 9, 0, 0, 0, time.UTC)

// fakeGitHub serves PRs from memory. Every PR in inSet matches the
// "author:" search of its owner; owners in sso fail with SSORequiredError.
type fakeGitHub struct {
	mu        sync.Mutex
	prs       map[string]*model.PRSnapshot
	inSet     map[string]bool
	sso       map[string]bool
	searchErr error
	notif     gh.NotificationsResult
	snapped   [][]string // ids per Snapshots call
	searches  int
}

func newFakeGitHub() *fakeGitHub {
	return &fakeGitHub{prs: map[string]*model.PRSnapshot{}, inSet: map[string]bool{}, sso: map[string]bool{}}
}

func (f *fakeGitHub) SearchPRs(_ context.Context, q string) (gh.SearchResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.searches++
	if f.searchErr != nil {
		return gh.SearchResult{}, f.searchErr
	}
	fields := strings.Fields(q)
	_, owner, _ := strings.Cut(fields[len(fields)-1], ":")
	if f.sso[owner] {
		return gh.SearchResult{}, &gh.SSORequiredError{URL: "https://sso"}
	}
	res := gh.SearchResult{Cost: 1}
	if !strings.Contains(q, "author:") {
		return res, nil
	}
	for id := range f.inSet {
		pr := f.prs[id]
		if strings.HasPrefix(pr.Repo, owner+"/") {
			res.Hits = append(res.Hits, gh.SearchHit{ID: id, Repo: pr.Repo, Number: pr.Number, Title: pr.Title, IsDraft: pr.IsDraft})
		}
	}
	return res, nil
}

func (f *fakeGitHub) Snapshots(_ context.Context, ids []string) (gh.SnapshotResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.snapped = append(f.snapped, append([]string(nil), ids...))
	res := gh.SnapshotResult{PRs: map[string]*model.PRSnapshot{}, Failed: map[string]error{}, Cost: 1, Remaining: 4000}
	for _, id := range ids {
		pr, ok := f.prs[id]
		if !ok {
			res.Missing = append(res.Missing, id)
			continue
		}
		var cp model.PRSnapshot // deep copy, as if freshly decoded
		b, _ := json.Marshal(pr)
		_ = json.Unmarshal(b, &cp)
		res.PRs[id] = &cp
	}
	return res, nil
}

func (f *fakeGitHub) Notifications(context.Context, string) (gh.NotificationsResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.notif, nil
}

func (f *fakeGitHub) edit(id string, fn func(*model.PRSnapshot)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f.prs[id])
}

func (f *fakeGitHub) lastSnapshotIDs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.snapped) == 0 {
		return nil
	}
	return f.snapped[len(f.snapped)-1]
}

func myPR(id, repo string, number int) *model.PRSnapshot {
	return &model.PRSnapshot{
		ID: id, Repo: repo, Number: number, Title: "Fix " + id, URL: "https://x/" + id,
		Author: model.Actor{Login: "me"}, State: model.PROpen,
		Reviews: []model.Review{}, Comments: []model.Comment{}, Threads: []model.Thread{},
		ReviewRequests: []model.Reviewer{}, Timeline: []model.TimelineEvent{},
	}
}

func approve(dbID int64, at time.Time) model.Review {
	return model.Review{ID: "PRR_" + strings.Repeat("x", int(dbID%7)), DatabaseID: dbID, State: model.ReviewApproved,
		URL: "https://x/r", SubmittedAt: at, Author: model.Actor{Login: "alice"}}
}

type harness struct {
	t      *testing.T
	gh     *fakeGitHub
	store  *store.Store
	p      *Poller
	events []store.Fresh
	status []Status
	clock  *fakeClock
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "prism.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	s := settings.Default()
	s.Owners = []string{"acme"}
	if err := st.SaveSettings(ctx, s); err != nil {
		t.Fatal(err)
	}
	h := &harness{t: t, gh: newFakeGitHub(), store: st, clock: newFakeClock(t0)}
	h.p = h.newPoller()
	return h
}

func (h *harness) newPoller() *Poller {
	return New(Options{
		GitHub: h.gh, Store: h.store, Me: Identity{Login: "me"}, Clock: h.clock,
		Log:      slog.New(slog.DiscardHandler),
		OnEvents: func(f []store.Fresh) { h.events = append(h.events, f...) },
		OnStatus: func(s Status) { h.status = append(h.status, s) },
	})
}

func (h *harness) cycle() {
	h.t.Helper()
	if err := h.p.Cycle(context.Background()); err != nil {
		h.t.Fatalf("cycle: %v", err)
	}
}

func (h *harness) takeEvents() []model.EventType {
	var out []model.EventType
	for _, f := range h.events {
		out = append(out, f.Event.Type)
	}
	h.events = nil
	return out
}

func (h *harness) unread() []store.Notification {
	n, err := h.store.Unread(context.Background())
	if err != nil {
		h.t.Fatal(err)
	}
	return n
}

func (h *harness) settings(fn func(*settings.Settings)) {
	ctx := context.Background()
	s, _ := h.store.Settings(ctx)
	fn(&s)
	if err := h.store.SaveSettings(ctx, s); err != nil {
		h.t.Fatal(err)
	}
}

func (h *harness) states() map[string]store.PRState {
	s, err := h.store.PRStates(context.Background())
	if err != nil {
		h.t.Fatal(err)
	}
	return s
}

func eq(a []model.EventType, b ...model.EventType) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestFirstRunIsSilentThenEventsFlow(t *testing.T) {
	h := newHarness(t)
	pr := myPR("PR_1", "acme/api", 1)
	pr.Reviews = []model.Review{approve(1, t0)}
	h.gh.prs["PR_1"], h.gh.inSet["PR_1"] = pr, true

	h.cycle()
	if got := h.takeEvents(); len(got) != 0 {
		t.Fatalf("first run emitted %v", got)
	}
	if v, _ := h.store.Meta(context.Background(), store.MetaBaselineDone); v != "1" {
		t.Fatal("baseline not marked done")
	}
	if s := h.states()["PR_1"]; s.Role != store.RoleMine || len(s.Snapshot.Reviews) != 1 {
		t.Fatalf("state = %+v", s)
	}

	h.gh.edit("PR_1", func(p *model.PRSnapshot) {
		r := approve(2, t0.Add(time.Hour))
		r.ID = "PRR_two"
		p.Reviews = append(p.Reviews, r)
	})
	h.cycle()
	if got := h.takeEvents(); !eq(got, model.MyPRApproved) {
		t.Fatalf("events = %v", got)
	}
	h.cycle()
	if got := h.takeEvents(); len(got) != 0 {
		t.Fatalf("repeat cycle emitted %v", got)
	}

	// A restart (new Poller, same DB) emits nothing either.
	h.p = h.newPoller()
	h.cycle()
	if got := h.takeEvents(); len(got) != 0 {
		t.Fatalf("after restart: %v", got)
	}
	if len(h.unread()) != 1 {
		t.Fatalf("unread = %v", h.unread())
	}
	last := h.status[len(h.status)-1]
	if last.Err != nil || last.Watched != 1 || !last.LastSync.Equal(t0) || last.RateRemaining != 4000 {
		t.Errorf("status = %+v", last)
	}
}

func TestEnteringPRIsBaselined(t *testing.T) {
	h := newHarness(t)
	h.cycle() // baseline with nothing

	pr := myPR("PR_2", "acme/api", 2)
	pr.Reviews = []model.Review{approve(1, t0)}
	h.gh.prs["PR_2"], h.gh.inSet["PR_2"] = pr, true
	h.cycle()
	if got := h.takeEvents(); !eq(got, model.MyPROpened) {
		t.Fatalf("entering PR: got %v, want only my_pr.opened (its history is baselined)", got)
	}
	h.gh.edit("PR_2", func(p *model.PRSnapshot) {
		p.Comments = append(p.Comments, model.Comment{ID: "IC_1", DatabaseID: 301, Body: "hi", URL: "u",
			CreatedAt: t0.Add(time.Hour), Author: model.Actor{Login: "bob"}})
	})
	h.cycle()
	if got := h.takeEvents(); !eq(got, model.MyPRComment) {
		t.Fatalf("events = %v", got)
	}
}

func TestDeletedCommentDropsNotification(t *testing.T) {
	h := newHarness(t)
	h.gh.prs["PR_1"], h.gh.inSet["PR_1"] = myPR("PR_1", "acme/api", 1), true
	h.cycle()
	h.gh.edit("PR_1", func(p *model.PRSnapshot) {
		p.Comments = []model.Comment{{ID: "IC_1", DatabaseID: 301, Body: "oops", URL: "u", CreatedAt: t0, Author: model.Actor{Login: "bob"}}}
	})
	h.cycle()
	if len(h.unread()) != 1 {
		t.Fatal("comment not listed")
	}
	h.gh.edit("PR_1", func(p *model.PRSnapshot) { p.Comments = nil })
	h.cycle()
	if n := h.unread(); len(n) != 0 {
		t.Fatalf("deleted comment still listed: %v", n)
	}
}

func TestMergedPRPurgedWithClosedEvent(t *testing.T) {
	h := newHarness(t)
	h.gh.prs["PR_1"], h.gh.inSet["PR_1"] = myPR("PR_1", "acme/api", 1), true
	h.cycle()
	h.gh.edit("PR_1", func(p *model.PRSnapshot) {
		p.Comments = []model.Comment{{ID: "IC_1", DatabaseID: 301, Body: "nit", URL: "u", CreatedAt: t0, Author: model.Actor{Login: "bob"}}}
	})
	h.cycle()
	h.takeEvents()

	closedAt := t0.Add(2 * time.Hour)
	h.gh.edit("PR_1", func(p *model.PRSnapshot) { p.State, p.ClosedAt = model.PRMerged, &closedAt })
	delete(h.gh.inSet, "PR_1") // the open-PR searches no longer find it
	h.cycle()

	if got := h.takeEvents(); !eq(got, model.MyPRClosed) {
		t.Fatalf("events = %v", got)
	}
	if len(h.states()) != 0 {
		t.Error("pr_state not purged")
	}
	n := h.unread()
	if len(n) != 1 || n[0].Type != model.MyPRClosed || n[0].Summary != "your PR was merged" {
		t.Fatalf("unread after merge = %+v (only my_pr.closed may remain)", n)
	}
	h.cycle()
	if got := h.takeEvents(); len(got) != 0 {
		t.Fatalf("closed emitted twice: %v", got)
	}
}

func TestLeftButOpenIsPurgedSilently(t *testing.T) {
	h := newHarness(t)
	h.gh.prs["PR_1"], h.gh.inSet["PR_1"] = myPR("PR_1", "acme/api", 1), true
	h.gh.prs["PR_9"], h.gh.inSet["PR_9"] = myPR("PR_9", "acme/api", 9), true
	h.cycle()
	delete(h.gh.inSet, "PR_1") // e.g. turned draft with includeDrafts=false
	delete(h.gh.prs, "PR_9")   // deleted / no access
	delete(h.gh.inSet, "PR_9")
	h.cycle()
	if got := h.takeEvents(); len(got) != 0 {
		t.Fatalf("events = %v", got)
	}
	if len(h.states()) != 0 {
		t.Errorf("states = %v", h.states())
	}
}

func TestUnselectedOwnerPurgedWithoutFetch(t *testing.T) {
	h := newHarness(t)
	h.gh.prs["PR_1"], h.gh.inSet["PR_1"] = myPR("PR_1", "acme/api", 1), true
	h.cycle()
	h.settings(func(s *settings.Settings) { s.Owners = []string{} })
	h.gh.snapped = nil
	h.cycle()
	if len(h.states()) != 0 {
		t.Error("PR of unselected owner not purged")
	}
	if len(h.gh.snapped) != 0 {
		t.Errorf("unselected owner's PR was fetched: %v", h.gh.snapped)
	}
	if got := h.takeEvents(); len(got) != 0 {
		t.Errorf("events = %v (no closed events when unselecting)", got)
	}
}

func TestFailedOwnerKeepsItsPRs(t *testing.T) {
	h := newHarness(t)
	h.settings(func(s *settings.Settings) { s.Owners = []string{"acme", "locked"} })
	h.gh.prs["PR_1"], h.gh.inSet["PR_1"] = myPR("PR_1", "locked/api", 1), true
	h.cycle()
	h.gh.sso["locked"] = true
	h.cycle()
	if _, ok := h.states()["PR_1"]; !ok {
		t.Fatal("PR of an owner whose search failed must not be purged")
	}
	last := h.status[len(h.status)-1]
	if len(last.Skipped) != 1 || last.Skipped[0].Owner != "locked" || last.Skipped[0].SSOURL != "https://sso" {
		t.Errorf("skipped = %+v", last.Skipped)
	}
	delete(h.gh.sso, "locked")
	h.cycle()
	if last := h.status[len(h.status)-1]; len(last.Skipped) != 0 {
		t.Errorf("skipped not cleared: %+v", last.Skipped)
	}
}

func TestListDisabledEventIsFreshButUnlisted(t *testing.T) {
	h := newHarness(t)
	h.settings(func(s *settings.Settings) {
		s.Events[model.MyPRApproved] = settings.Flags{Toast: true}
	})
	h.gh.prs["PR_1"], h.gh.inSet["PR_1"] = myPR("PR_1", "acme/api", 1), true
	h.cycle()
	h.gh.edit("PR_1", func(p *model.PRSnapshot) { p.Reviews = []model.Review{approve(1, t0)} })
	h.cycle()
	if len(h.events) != 1 || h.events[0].ID != 0 {
		t.Fatalf("fresh = %+v", h.events)
	}
	if len(h.unread()) != 0 {
		t.Error("list-disabled event was listed")
	}
}

func TestCycleErrorReported(t *testing.T) {
	h := newHarness(t)
	h.gh.searchErr = errors.New("network down")
	if err := h.p.Cycle(context.Background()); err == nil {
		t.Fatal("want error")
	}
	if s := h.status[len(h.status)-1]; s.Err == nil || !s.LastSync.IsZero() {
		t.Errorf("status = %+v", s)
	}
	if v, _ := h.store.Meta(context.Background(), store.MetaBaselineDone); v == "1" {
		t.Error("a failed first run must not mark the baseline done")
	}
}

func TestRefreshPRs(t *testing.T) {
	h := newHarness(t)
	h.gh.prs["PR_1"], h.gh.inSet["PR_1"] = myPR("PR_1", "acme/api", 1), true
	h.gh.prs["PR_2"], h.gh.inSet["PR_2"] = myPR("PR_2", "acme/api", 2), true
	h.cycle()
	h.gh.edit("PR_2", func(p *model.PRSnapshot) { p.Reviews = []model.Review{approve(1, t0)} })

	needFull, err := h.p.RefreshPRs(context.Background(), []gh.PRRef{
		{Repo: "ACME/api", Number: 2}, // known (case-insensitive)
		{Repo: "other/x", Number: 5},  // owner not watched: ignored
	})
	if err != nil || needFull {
		t.Fatalf("needFull = %v, err = %v", needFull, err)
	}
	if ids := h.gh.lastSnapshotIDs(); len(ids) != 1 || ids[0] != "PR_2" {
		t.Errorf("snapshotted %v, want only PR_2", ids)
	}
	if got := h.takeEvents(); !eq(got, model.MyPRApproved) {
		t.Errorf("events = %v", got)
	}
	needFull, err = h.p.RefreshPRs(context.Background(), []gh.PRRef{{Repo: "acme/new", Number: 3}})
	if err != nil || !needFull {
		t.Errorf("unknown PR of a watched owner: needFull = %v, err = %v", needFull, err)
	}
}

func TestFirstRunListsPendingReviewRequestsQuietly(t *testing.T) {
	h := newHarness(t)
	other := myPR("PR_5", "acme/web", 5)
	other.Author = model.Actor{Login: "alice"}
	other.ReviewRequests = []model.Reviewer{{Login: "me"}}
	other.Timeline = []model.TimelineEvent{{Type: model.TimelineReviewRequested, ID: "RRE_1", CreatedAt: t0,
		Actor: model.Actor{Login: "alice"}, RequestedReviewer: &model.Reviewer{Login: "me"}}}
	h.gh.prs["PR_5"], h.gh.inSet["PR_5"] = other, true
	h.gh.prs["PR_1"], h.gh.inSet["PR_1"] = myPR("PR_1", "acme/api", 1), true // would be my_pr.opened

	h.cycle()
	if len(h.events) != 1 || h.events[0].Event.Type != model.ReviewRequested || !h.events[0].Quiet {
		t.Fatalf("first run fresh = %+v, want one quiet review.requested", h.events)
	}
	h.events = nil

	// The next request (after the baseline) is a normal, loud event.
	h.gh.edit("PR_5", func(p *model.PRSnapshot) {
		p.Timeline = append(p.Timeline, model.TimelineEvent{Type: model.TimelineReviewRequested, ID: "RRE_2",
			CreatedAt: t0.Add(time.Hour), Actor: model.Actor{Login: "alice"}, RequestedReviewer: &model.Reviewer{Login: "me"}})
	})
	h.cycle()
	if len(h.events) != 1 || h.events[0].Quiet {
		t.Fatalf("second run fresh = %+v", h.events)
	}
}

func TestMutedPRRecordsNothingUntilUnmuted(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.gh.prs["PR_1"], h.gh.inSet["PR_1"] = myPR("PR_1", "acme/api", 1), true
	h.cycle()
	if err := h.store.SetMuted(ctx, "PR_1", true); err != nil {
		t.Fatal(err)
	}
	comment := func(dbID int64) {
		h.gh.edit("PR_1", func(p *model.PRSnapshot) {
			p.Comments = append(p.Comments, model.Comment{ID: "IC_" + strings.Repeat("x", int(dbID)), DatabaseID: dbID,
				Body: "hi", URL: "u", CreatedAt: t0, Author: model.Actor{Login: "bob"}})
		})
	}
	comment(1)
	h.cycle()
	if got := h.takeEvents(); len(got) != 0 {
		t.Fatalf("muted PR emitted %v", got)
	}
	if !h.states()["PR_1"].Muted {
		t.Fatal("a cycle unmuted the PR")
	}

	// Unmuting resumes with new activity only; what happened while muted stays silent.
	if err := h.store.SetMuted(ctx, "PR_1", false); err != nil {
		t.Fatal(err)
	}
	h.cycle()
	if got := h.takeEvents(); len(got) != 0 {
		t.Fatalf("muted-time activity resurfaced: %v", got)
	}
	comment(2)
	h.cycle()
	if got := h.takeEvents(); !eq(got, model.MyPRComment) {
		t.Fatalf("after unmute: %v", got)
	}
}

func TestMutedPRClosesSilently(t *testing.T) {
	h := newHarness(t)
	h.gh.prs["PR_1"], h.gh.inSet["PR_1"] = myPR("PR_1", "acme/api", 1), true
	h.cycle()
	if err := h.store.SetMuted(context.Background(), "PR_1", true); err != nil {
		t.Fatal(err)
	}
	h.gh.edit("PR_1", func(p *model.PRSnapshot) { p.State = model.PRMerged })
	delete(h.gh.inSet, "PR_1")
	h.cycle()
	if got := h.takeEvents(); len(got) != 0 {
		t.Fatalf("muted PR emitted %v on merge", got)
	}
	if _, ok := h.states()["PR_1"]; ok {
		t.Error("merged PR not purged")
	}
}
