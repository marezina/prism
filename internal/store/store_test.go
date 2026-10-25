package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"prism/internal/model"
	"prism/internal/settings"
)

func open(t *testing.T) *Store {
	t.Helper()
	s, err := Open(context.Background(), filepath.Join(t.TempDir(), "sub", "prism.db"))
	if err != nil {
		t.Fatal(err)
	}
	s.now = func() time.Time { return time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC) }
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func ev(typ model.EventType, key, pr string) model.Event {
	return model.Event{
		Type: typ, DedupKey: key, PRID: pr, Repo: "acme/api", PRNumber: 7, PRTitle: "Fix",
		Actor: model.Actor{Login: "alice", AvatarURL: "https://a"}, Summary: "approved your PR",
		Excerpt: "LGTM", URL: "https://x/" + key, OccurredAt: time.Date(2026, 10, 24, 10, 0, 0, 0, time.UTC),
	}
}

func listAll(model.EventType) bool { return true }

func TestOpenMigratesAndReopens(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "prism.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := s.Meta(ctx, "schema_version"); v != "1" {
		t.Errorf("schema_version = %q", v)
	}
	if err := s.SetMeta(ctx, MetaMeLogin, "mona"); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()

	s, err = Open(ctx, path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer func() { _ = s.Close() }()
	if v, _ := s.Meta(ctx, MetaMeLogin); v != "mona" {
		t.Errorf("meta lost across reopen: %q", v)
	}
	if v, _ := s.Meta(ctx, "missing"); v != "" {
		t.Errorf("missing meta = %q", v)
	}

	if err := s.SetMeta(ctx, "schema_version", "99"); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	if _, err := Open(ctx, path); err == nil {
		t.Error("want error for a newer schema")
	}
}

func TestSettingsRoundTrip(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	got, err := s.Settings(ctx)
	if err != nil || got.PollIntervalSec != 120 {
		t.Fatalf("defaults = %+v, %v", got, err)
	}
	got.Owners = []string{"acme"}
	got.PollIntervalSec = 300
	if err := s.SaveSettings(ctx, got); err != nil {
		t.Fatal(err)
	}
	back, err := s.Settings(ctx)
	if err != nil || !cmp.Equal(back, got) {
		t.Errorf("round trip: %v\n%s", err, cmp.Diff(got, back))
	}
	bad := settings.Default()
	bad.Theme = "pink"
	if err := s.SaveSettings(ctx, bad); err == nil {
		t.Error("invalid settings saved")
	}
}

func TestRecordInsertRule(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	listOnlyApproved := func(t model.EventType) bool { return t == model.MyPRApproved }

	fresh, err := s.Record(ctx, []model.Event{
		ev(model.MyPRApproved, "review:1", "PR_1"),
		ev(model.CIPassed, "ci:PR_1:h:ok", "PR_1"),
	}, listOnlyApproved)
	if err != nil {
		t.Fatal(err)
	}
	if len(fresh) != 2 || fresh[0].ID == 0 || fresh[1].ID != 0 {
		t.Fatalf("fresh = %+v (list-disabled events are fresh but unlisted)", fresh)
	}

	// Same keys again: nothing is fresh, even the unlisted one (tombstone).
	fresh, err = s.Record(ctx, []model.Event{ev(model.MyPRApproved, "review:1", "PR_1"), ev(model.CIPassed, "ci:PR_1:h:ok", "PR_1")}, listAll)
	if err != nil || len(fresh) != 0 {
		t.Fatalf("re-record = %+v, %v", fresh, err)
	}

	unread, _ := s.Unread(ctx)
	if len(unread) != 1 {
		t.Fatalf("unread = %+v", unread)
	}
	want := ev(model.MyPRApproved, "review:1", "PR_1")
	if diff := cmp.Diff(want, unread[0].Event); diff != "" {
		t.Errorf("stored event (-want +got):\n%s", diff)
	}

	// Read = delete; it never comes back.
	if n, err := s.MarkRead(ctx, unread[0].ID); n != 1 || err != nil {
		t.Fatalf("MarkRead = %d, %v", n, err)
	}
	fresh, _ = s.Record(ctx, []model.Event{ev(model.MyPRApproved, "review:1", "PR_1")}, listAll)
	if len(fresh) != 0 {
		t.Error("read notification re-appeared")
	}
	if n, _ := s.UnreadCount(ctx); n != 0 {
		t.Errorf("count = %d", n)
	}
	if _, err := s.Get(ctx, unread[0].ID); err != ErrNotFound {
		t.Errorf("Get deleted = %v", err)
	}
}

func TestMarkReadVariantsAndPurge(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	_, err := s.Record(ctx, []model.Event{
		ev(model.MyPRApproved, "review:1", "PR_1"),
		ev(model.MyPRComment, "comment:2", "PR_1"),
		ev(model.MyPRComment, "comment:3", "PR_2"),
		ev(model.MyPRComment, "comment:4", "PR_2"),
	}, listAll)
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := s.MarkPRRead(ctx, "PR_1"); n != 2 {
		t.Errorf("MarkPRRead = %d", n)
	}
	forPR, _ := s.ForPR(ctx, "PR_2")
	if len(forPR) != 2 {
		t.Fatalf("ForPR = %d", len(forPR))
	}
	if got, err := s.Get(ctx, forPR[0].ID); err != nil || got.DedupKey != forPR[0].DedupKey {
		t.Errorf("Get = %+v, %v", got, err)
	}
	if n, _ := s.MarkRead(ctx); n != 0 {
		t.Error("MarkRead() with no ids must be a no-op")
	}

	// Purge removes PR_2's rows and tombstones, so a re-entering PR starts clean.
	if err := s.PutPRState(ctx, PRState{PRID: "PR_2", Repo: "acme/api", Number: 7, Role: RoleMine,
		Snapshot: &model.PRSnapshot{ID: "PR_2"}}); err != nil {
		t.Fatal(err)
	}
	if err := s.PurgePR(ctx, "PR_2"); err != nil {
		t.Fatal(err)
	}
	if n, _ := s.UnreadCount(ctx); n != 0 {
		t.Errorf("count after purge = %d", n)
	}
	states, _ := s.PRStates(ctx)
	if len(states) != 0 {
		t.Errorf("pr_state after purge = %v", states)
	}
	fresh, _ := s.Record(ctx, []model.Event{ev(model.MyPRComment, "comment:3", "PR_2")}, listAll)
	if len(fresh) != 1 {
		t.Error("tombstones of a purged PR must be gone")
	}
	if n, _ := s.MarkAllRead(ctx); n != 1 {
		t.Errorf("MarkAllRead = %d", n)
	}
	// PR_1 tombstones survive MarkPRRead.
	if fresh, _ := s.Record(ctx, []model.Event{ev(model.MyPRApproved, "review:1", "PR_1")}, listAll); len(fresh) != 0 {
		t.Error("tombstone lost by MarkPRRead")
	}
}

func TestPRStateRoundTrip(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	snap := &model.PRSnapshot{ID: "PR_1", Repo: "acme/api", Number: 7, Title: "Fix", State: model.PROpen,
		Reviews: []model.Review{{ID: "PRR_1", DatabaseID: 1, State: model.ReviewApproved}}}
	st := PRState{PRID: "PR_1", Repo: "acme/api", Number: 7, Role: RoleMine, Snapshot: snap}
	if err := s.PutPRState(ctx, st); err != nil {
		t.Fatal(err)
	}
	st.Snapshot.Title = "Fix v2"
	if err := s.PutPRState(ctx, st); err != nil {
		t.Fatal(err)
	}
	states, err := s.PRStates(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got := states["PR_1"]
	if got.Role != RoleMine || got.Snapshot.Title != "Fix v2" || len(got.Snapshot.Reviews) != 1 || got.UpdatedAt.IsZero() {
		t.Errorf("state = %+v", got)
	}
}

func TestReset(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	_, _ = s.Record(ctx, []model.Event{ev(model.MyPRApproved, "review:1", "PR_1")}, listAll)
	_ = s.PutPRState(ctx, PRState{PRID: "PR_1", Repo: "acme/api", Number: 1, Role: RoleMine, Snapshot: &model.PRSnapshot{ID: "PR_1"}})
	_ = s.PutAvatar(ctx, Avatar{Login: "a", Path: "/p", FetchedAt: s.now()})
	st := settings.Default()
	st.Owners = []string{"acme"}
	_ = s.SaveSettings(ctx, st)
	_ = s.SetMeta(ctx, MetaBaselineDone, "1")

	if err := s.Reset(ctx); err != nil {
		t.Fatal(err)
	}
	if n, _ := s.UnreadCount(ctx); n != 0 {
		t.Error("notifications survived reset")
	}
	if states, _ := s.PRStates(ctx); len(states) != 0 {
		t.Error("pr_state survived reset")
	}
	if _, ok, _ := s.Avatar(ctx, "a"); ok {
		t.Error("avatar survived reset")
	}
	if got, _ := s.Settings(ctx); len(got.Owners) != 0 {
		t.Error("settings survived reset")
	}
	if v, _ := s.Meta(ctx, MetaBaselineDone); v != "" {
		t.Error("meta survived reset")
	}
	if v, _ := s.Meta(ctx, "schema_version"); v != "1" {
		t.Error("schema_version must survive reset")
	}
	if fresh, _ := s.Record(ctx, []model.Event{ev(model.MyPRApproved, "review:1", "PR_1")}, listAll); len(fresh) != 1 {
		t.Error("tombstones survived reset")
	}
}
