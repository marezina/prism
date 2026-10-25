// Package fixture records and sanitizes PR snapshots for the detector's golden
// tests (event-detector skill, references/fixtures.md).
package fixture

import (
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strings"
	"unicode"

	"prism/internal/model"
)

// Mapping is the stable original → fake mapping for one fixture scenario. It
// contains original names, so it is stored outside the repo (MapPath) and
// reused when recording the next step of the same scenario.
type Mapping struct {
	Me     string            `json:"me"`     // original login of the signed-in user
	Logins map[string]string `json:"logins"` // login → alice, bob, …
	Owners map[string]string `json:"owners"` // org → org-a, …
	Repos  map[string]string `json:"repos"`  // repo name (no owner) → repo-a, …
	Teams  map[string]string `json:"teams"`  // team slug → team-a, …
	Words  map[string]string `json:"words"`  // word → w1, …
	IDs    map[string]string `json:"ids"`    // node id → <prefix>_<n>
	Paths  map[string]string `json:"paths"`  // path segment → p1, …
	Checks map[string]string `json:"checks"` // check name → check-1, …
	Links  map[string]string `json:"links"`  // non-GitHub URL → https://example.invalid/<n>
}

var people = []string{"alice", "bob", "carol", "dave", "erin", "frank", "grace", "heidi",
	"ivan", "judy", "mallory", "niaj", "olivia", "peggy", "rupert", "sybil", "trent", "victor", "walter"}

// NewMapping starts an empty mapping; me becomes "me".
func NewMapping(me string) *Mapping {
	m := &Mapping{Me: me}
	m.init()
	return m
}

func (m *Mapping) init() {
	for _, p := range []*map[string]string{&m.Logins, &m.Owners, &m.Repos, &m.Teams, &m.Words, &m.IDs, &m.Paths, &m.Checks, &m.Links} {
		if *p == nil {
			*p = map[string]string{}
		}
	}
}

// FakeHost replaces github.com in every URL.
const FakeHost = "github.example"

func (m *Mapping) login(l string) string { return m.loginAs(l, false) }

// loginAs maps a login; bots (by flag or "[bot]" suffix) become bot-N[bot].
func (m *Mapping) loginAs(l string, isBot bool) string {
	switch {
	case l == "":
		return ""
	case strings.EqualFold(l, m.Me):
		return "me"
	}
	key := strings.ToLower(l)
	if v, ok := m.Logins[key]; ok {
		return v
	}
	var v string
	if isBot || strings.HasSuffix(l, "[bot]") {
		v = fmt.Sprintf("bot-%d[bot]", countSuffix(m.Logins, "[bot]")+1)
	} else if n := len(m.Logins) - countSuffix(m.Logins, "[bot]"); n < len(people) {
		v = people[n]
	} else {
		v = fmt.Sprintf("user-%d", n+1)
	}
	m.Logins[key] = v
	return v
}

func countSuffix(mp map[string]string, suffix string) int {
	n := 0
	for _, v := range mp {
		if strings.HasSuffix(v, suffix) {
			n++
		}
	}
	return n
}

func (m *Mapping) owner(o string) string {
	if strings.EqualFold(o, m.Me) {
		return "me"
	}
	return seq(m.Owners, strings.ToLower(o), "org-", letters)
}

func (m *Mapping) repo(nameWithOwner string) string {
	owner, name, ok := strings.Cut(nameWithOwner, "/")
	if !ok {
		return nameWithOwner
	}
	return m.owner(owner) + "/" + seq(m.Repos, strings.ToLower(name), "repo-", letters)
}

func (m *Mapping) team(orgSlash string) string {
	org, slug, ok := strings.Cut(orgSlash, "/")
	if !ok {
		return orgSlash
	}
	return m.owner(org) + "/" + seq(m.Teams, strings.ToLower(slug), "team-", letters)
}

func (m *Mapping) id(id string) string {
	if id == "" {
		return ""
	}
	prefix, _, ok := strings.Cut(id, "_")
	if !ok {
		prefix = "N"
	}
	if v, ok := m.IDs[id]; ok {
		return v
	}
	v := fmt.Sprintf("%s_%d", prefix, len(m.IDs)+1)
	m.IDs[id] = v
	return v
}

func (m *Mapping) word(w string) string { return seq(m.Words, w, "w", numbers) }

func (m *Mapping) check(name string) string { return seq(m.Checks, name, "check-", numbers) }

// seq returns mp[key], assigning prefix+next label on first use.
func seq(mp map[string]string, key, prefix string, label func(int) string) string {
	if v, ok := mp[key]; ok {
		return v
	}
	v := prefix + label(len(mp))
	mp[key] = v
	return v
}

func numbers(i int) string { return fmt.Sprint(i + 1) }

func letters(i int) string {
	s := ""
	for i++; i > 0; i = (i - 1) / 26 {
		s = string(rune('a'+(i-1)%26)) + s
	}
	return s
}

// path maps "src/delivery/retry.go" to "p1/p2/p3.go", keeping extensions.
func (m *Mapping) path(p string) string {
	segs := strings.Split(p, "/")
	for i, s := range segs {
		ext := path.Ext(s)
		segs[i] = seq(m.Paths, strings.TrimSuffix(s, ext), "p", numbers) + ext
	}
	return strings.Join(segs, "/")
}

var githubURL = regexp.MustCompile(`^https?://(?:www\.)?github\.com/([^/\s]+)/([^/\s#?]+)(.*)$`)

// url rewrites github.com/<owner>/<repo>/… keeping the rest (PR numbers and
// anchors carry no names); any other URL becomes an opaque placeholder.
func (m *Mapping) url(u string) string {
	if u == "" {
		return ""
	}
	if g := githubURL.FindStringSubmatch(u); g != nil {
		repo := m.repo(g[1] + "/" + g[2])
		return "https://" + FakeHost + "/" + repo + g[3]
	}
	if pu, err := url.Parse(u); err == nil && pu.Host != "" {
		return seq(m.Links, u, "https://example.invalid/", numbers)
	}
	return u
}

func (m *Mapping) actor(a model.Actor) model.Actor {
	login := m.loginAs(a.Login, a.IsBot)
	out := model.Actor{Login: login, IsBot: a.IsBot}
	if a.AvatarURL != "" {
		out.AvatarURL = "avatar://" + login
	}
	return out
}

// Text sanitizes markdown: words, numbers, @mentions and URLs are mapped,
// everything else (quote markers, list markers, code fences, punctuation,
// whitespace, line breaks) is kept, so quote-replies and mentions still match.
func (m *Mapping) Text(s string) string {
	var b strings.Builder
	r := []rune(s)
	for i := 0; i < len(r); {
		c := r[i]
		switch {
		case c == '@' && (i == 0 || !isWord(r[i-1])) && i+1 < len(r) && isLoginRune(r[i+1]):
			j := i + 1
			for j < len(r) && (isLoginRune(r[j]) || (r[j] == '/' && j+1 < len(r) && isLoginRune(r[j+1]))) {
				j++
			}
			name := string(r[i+1 : j])
			if strings.Contains(name, "/") {
				b.WriteString("@" + m.team(name))
			} else {
				b.WriteString("@" + m.login(name))
			}
			i = j
		case hasURLPrefix(r[i:]):
			j := i
			for j < len(r) && !unicode.IsSpace(r[j]) && !strings.ContainsRune(")]>\"'", r[j]) {
				j++
			}
			b.WriteString(m.url(string(r[i:j])))
			i = j
		case isWord(c):
			j := i
			for j < len(r) && isWord(r[j]) {
				j++
			}
			b.WriteString(m.word(string(r[i:j])))
			i = j
		default:
			b.WriteRune(c)
			i++
		}
	}
	return b.String()
}

func isWord(c rune) bool      { return unicode.IsLetter(c) || unicode.IsDigit(c) }
func isLoginRune(c rune) bool { return c == '-' || c == '_' || c < unicode.MaxASCII && isWord(c) }

func hasURLPrefix(r []rune) bool {
	s := string(r[:min(len(r), 8)])
	return strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "http://")
}

func (m *Mapping) reviewer(r model.Reviewer) model.Reviewer {
	if r.Team != "" {
		return model.Reviewer{Team: m.team(r.Team)}
	}
	return model.Reviewer{Login: m.login(r.Login)}
}

func (m *Mapping) comment(c model.Comment) model.Comment {
	return model.Comment{
		ID: m.id(c.ID), DatabaseID: c.DatabaseID, Body: m.Text(c.Body), URL: m.url(c.URL),
		CreatedAt: c.CreatedAt, Author: m.actor(c.Author),
	}
}

// Snapshot returns a sanitized deep copy of s. Commit OIDs, database ids,
// timestamps and states are kept: they carry no names.
func (m *Mapping) Snapshot(s *model.PRSnapshot) *model.PRSnapshot {
	m.init()
	out := &model.PRSnapshot{
		ID: m.id(s.ID), Repo: m.repo(s.Repo), Number: s.Number, Title: m.Text(s.Title), URL: m.url(s.URL),
		Author: m.actor(s.Author), IsDraft: s.IsDraft, State: s.State, ClosedAt: s.ClosedAt,
		CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt,
		HeadOID: s.HeadOID, BaseOID: s.BaseOID, Mergeable: s.Mergeable, MergeStateStatus: s.MergeStateStatus,
		CommitsTotal: s.CommitsTotal,
		Reviews:      []model.Review{}, Comments: []model.Comment{}, Threads: []model.Thread{},
		ReviewRequests: []model.Reviewer{}, Timeline: []model.TimelineEvent{},
	}
	if s.CI != nil {
		out.CI = &model.CI{State: s.CI.State}
		for _, c := range s.CI.Checks {
			out.CI.Checks = append(out.CI.Checks, model.Check{Name: m.check(c.Name), State: c.State, URL: m.url(c.URL), At: c.At})
		}
	}
	for _, r := range s.Reviews {
		out.Reviews = append(out.Reviews, model.Review{
			ID: m.id(r.ID), DatabaseID: r.DatabaseID, State: r.State, Body: m.Text(r.Body), URL: m.url(r.URL),
			SubmittedAt: r.SubmittedAt, Author: m.actor(r.Author), CommitOID: r.CommitOID,
		})
	}
	for _, c := range s.Comments {
		out.Comments = append(out.Comments, m.comment(c))
	}
	for _, t := range s.Threads {
		th := model.Thread{ID: m.id(t.ID), IsResolved: t.IsResolved, Path: m.path(t.Path), Comments: []model.ThreadComment{}}
		for _, c := range t.Comments {
			th.Comments = append(th.Comments, model.ThreadComment{
				Comment: m.comment(c.Comment), ReplyToID: m.id(c.ReplyToID), ReviewDatabaseID: c.ReviewDatabaseID,
			})
		}
		out.Threads = append(out.Threads, th)
	}
	for _, r := range s.ReviewRequests {
		out.ReviewRequests = append(out.ReviewRequests, m.reviewer(r))
	}
	for _, e := range s.Timeline {
		te := model.TimelineEvent{Type: e.Type, ID: m.id(e.ID), CreatedAt: e.CreatedAt, Actor: m.actor(e.Actor), AfterOID: e.AfterOID}
		if e.RequestedReviewer != nil {
			r := m.reviewer(*e.RequestedReviewer)
			te.RequestedReviewer = &r
		}
		out.Timeline = append(out.Timeline, te)
	}
	return out
}

// TeamNames sanitizes "org/team" names for context.json.
func (m *Mapping) TeamNames(teams []string) []string {
	out := make([]string, 0, len(teams))
	for _, t := range teams {
		out = append(out, m.team(t))
	}
	return out
}

// Originals lists the names that must never appear in a committed fixture
// (used by sanitize-check). Short names (< 3 chars) are skipped as too noisy.
func (m *Mapping) Originals() []string {
	var out []string
	add := func(s string) {
		if len(s) >= 3 {
			out = append(out, s)
		}
	}
	add(m.Me)
	for _, mp := range []map[string]string{m.Logins, m.Owners, m.Repos, m.Teams} {
		for k := range mp {
			add(strings.TrimSuffix(k, "[bot]"))
		}
	}
	return out
}
