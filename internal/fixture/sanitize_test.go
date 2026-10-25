package fixture

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"prism/internal/model"
)

func TestText(t *testing.T) {
	m := NewMapping("Mona")
	tests := []struct{ in, want string }{
		{"Why 3 retries?", "w1 w2 w3?"},
		{"> Why 3 retries?\n\nBecause", "> w1 w2 w3?\n\nw4"},
		{"@Mona please check, @octo-cat", "@me w5 w6, @alice"},
		{"cc @acme/platform-team", "w7 @org-a/team-a"},
		{"mail me@example.com", "w8 w9@w10.w11"},
		{"```go\nx := retry(3)\n```", "```w12\nw13 := w14(w2)\n```"},
		{"see https://github.com/acme/api/pull/7#issuecomment-9 and https://ci.acme.io/run/1).",
			"w15 https://github.example/org-a/repo-a/pull/7#issuecomment-9 w16 https://example.invalid/1)."},
		{"- **bold** _it_ `code`", "- **w17** _w18_ `w19`"},
		{"Čćž naïve", "w20 w21"},
		{"why", "w22"}, // case-sensitive: different word than "Why"
	}
	for _, tt := range tests {
		if got := m.Text(tt.in); got != tt.want {
			t.Errorf("Text(%q)\n got %q\nwant %q", tt.in, got, tt.want)
		}
	}
	// The same text always maps the same way (quote-replies keep matching).
	if a, b := m.Text("Why 3 retries?"), m.Text("> Why 3 retries?"); "> "+a != b {
		t.Errorf("quote mapping drifted: %q vs %q", a, b)
	}
}

func TestLettersAndLogins(t *testing.T) {
	for i, want := range map[int]string{0: "a", 25: "z", 26: "aa", 27: "ab", 701: "zz", 702: "aaa"} {
		if got := letters(i); got != want {
			t.Errorf("letters(%d) = %q, want %q", i, got, want)
		}
	}
	m := NewMapping("me-orig")
	got := []string{
		m.login("x1"), m.login("dependabot[bot]"), m.login("X1"), m.loginAs("netlify", true),
		m.login("x2"), m.login("ME-ORIG"),
	}
	want := []string{"alice", "bot-1[bot]", "alice", "bot-2[bot]", "bob", "me"}
	if !cmp.Equal(got, want) {
		t.Errorf("logins = %v, want %v", got, want)
	}
	for i := 0; i < 30; i++ {
		m.login("filler" + string(rune('a'+i)))
	}
	if v := m.login("late"); !strings.HasPrefix(v, "user-") {
		t.Errorf("after the name list runs out: %q", v)
	}
}

func snapshot() *model.PRSnapshot {
	at := time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)
	return &model.PRSnapshot{
		ID: "PR_kwDOAbc", Repo: "acme/api", Number: 482, Title: "Add retry to webhook",
		URL:    "https://github.com/acme/api/pull/482",
		Author: model.Actor{Login: "mona", AvatarURL: "https://avatars.githubusercontent.com/u/1"},
		State:  model.PROpen, HeadOID: "abc123", BaseOID: "def456", Mergeable: "MERGEABLE",
		CI: &model.CI{State: model.CheckFailure, Checks: []model.Check{{Name: "build (ubuntu)", State: model.CheckFailure, URL: "https://ci.acme.io/1"}}},
		Reviews: []model.Review{{ID: "PRR_x", DatabaseID: 9, State: model.ReviewApproved, Body: "LGTM @mona",
			URL: "https://github.com/acme/api/pull/482#pullrequestreview-9", SubmittedAt: at, Author: model.Actor{Login: "octocat"}}},
		Comments: []model.Comment{{ID: "IC_x", DatabaseID: 10, Body: "Deploy ready", URL: "https://github.com/acme/api/pull/482#issuecomment-10",
			CreatedAt: at, Author: model.Actor{Login: "netlify", IsBot: true}}},
		Threads: []model.Thread{{ID: "PRRT_x", Path: "delivery/retry.go", Comments: []model.ThreadComment{
			{Comment: model.Comment{ID: "PRRC_a", DatabaseID: 11, Body: "Why?", CreatedAt: at, Author: model.Actor{Login: "octocat"}}, ReviewDatabaseID: 9},
			{Comment: model.Comment{ID: "PRRC_b", DatabaseID: 12, Body: "> Why?\n\nSLA", CreatedAt: at, Author: model.Actor{Login: "mona"}}, ReplyToID: "PRRC_a"},
		}}},
		ReviewRequests: []model.Reviewer{{Login: "hubot"}, {Team: "acme/platform"}},
		Timeline: []model.TimelineEvent{{Type: model.TimelineReviewRequested, ID: "RRE_x", CreatedAt: at,
			Actor: model.Actor{Login: "mona"}, RequestedReviewer: &model.Reviewer{Team: "acme/platform"}}},
	}
}

func TestSnapshotSanitized(t *testing.T) {
	m := NewMapping("mona")
	got := m.Snapshot(snapshot())

	b, _ := json.Marshal(got)
	for _, leak := range []string{"acme", "octocat", "mona", "netlify", "hubot", "github.com", "retry", "platform", "kwDO", "ubuntu", "avatars."} {
		if strings.Contains(strings.ToLower(string(b)), leak) {
			t.Errorf("sanitized snapshot still contains %q:\n%s", leak, b)
		}
	}
	checks := []struct{ name, got, want string }{
		{"id", got.ID, "PR_1"},
		{"repo", got.Repo, "org-a/repo-a"},
		{"author", got.Author.Login, "me"},
		{"avatar", got.Author.AvatarURL, "avatar://me"},
		{"url", got.URL, "https://github.example/org-a/repo-a/pull/482"},
		{"review url keeps anchor", got.Reviews[0].URL, "https://github.example/org-a/repo-a/pull/482#pullrequestreview-9"},
		{"mention of me", got.Reviews[0].Body, "w5 @me"},
		{"bot", got.Comments[0].Author.Login, "bot-1[bot]"},
		{"reply target id", got.Threads[0].Comments[1].ReplyToID, got.Threads[0].Comments[0].ID},
		{"quote still quotes", strings.SplitN(got.Threads[0].Comments[1].Body, "\n", 2)[0], "> " + got.Threads[0].Comments[0].Body},
		{"path keeps extension", got.Threads[0].Path, "p1/p2.go"},
		{"team", got.ReviewRequests[1].Team, "org-a/team-a"},
		{"timeline team", got.Timeline[0].RequestedReviewer.Team, "org-a/team-a"},
		{"check", got.CI.Checks[0].Name, "check-1"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.name, c.got, c.want)
		}
	}
	if got.HeadOID != "abc123" || got.Reviews[0].DatabaseID != 9 || !got.Reviews[0].SubmittedAt.Equal(snapshot().Reviews[0].SubmittedAt) {
		t.Error("OIDs, database ids and timestamps must be kept")
	}

	// Re-sanitizing with the same mapping (next recording step) is stable.
	if again := m.Snapshot(snapshot()); !cmp.Equal(got, again) {
		t.Errorf("mapping not stable:\n%s", cmp.Diff(got, again))
	}
}

func TestMappingFilesAndCheck(t *testing.T) {
	dir := t.TempDir()
	m, err := LoadMapping(dir, "approved__basic", "mona")
	if err != nil {
		t.Fatal(err)
	}
	clean := m.Snapshot(snapshot())
	if err := SaveMapping(dir, "approved__basic", m); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(mapPath(dir, "approved__basic")); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("mapping file perms: %v %v", info, err)
	}

	m2, err := LoadMapping(dir, "approved__basic", "MONA")
	if err != nil {
		t.Fatal(err)
	}
	if !cmp.Equal(clean, m2.Snapshot(snapshot())) {
		t.Error("reloaded mapping sanitizes differently")
	}
	if _, err := LoadMapping(dir, "approved__basic", "someone-else"); err == nil {
		t.Error("want error when another user records the same scenario")
	}

	fixtures := t.TempDir()
	if err := WriteJSON(filepath.Join(fixtures, "s1", "curr.json"), clean); err != nil {
		t.Fatal(err)
	}
	all, err := LoadAllMappings(dir)
	if err != nil || len(all) != 1 {
		t.Fatalf("LoadAllMappings = %v, %v", all, err)
	}
	if f, err := Check(fixtures, all[0].Originals()); err != nil || len(f) != 0 {
		t.Fatalf("clean fixture flagged: %v %v", f, err)
	}

	if err := WriteJSON(filepath.Join(fixtures, "s2", "curr.json"), snapshot()); err != nil {
		t.Fatal(err)
	}
	f, err := Check(fixtures, all[0].Originals())
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, x := range f {
		found[strings.ToLower(x.Match)] = true
	}
	for _, want := range []string{"github.com", "acme", "octocat", "mona"} {
		if !found[want] {
			t.Errorf("raw fixture: %q not reported; findings %v", want, f)
		}
	}
}
