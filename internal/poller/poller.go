// Package poller runs PRism's two polling tiers (ARCHITECTURE §2): the slow
// tier (watch set → snapshots → detect → store) is the source of truth; the
// fast tier (GET /notifications) only triggers early snapshot polls.
package poller

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"prism/internal/detect"
	"prism/internal/gh"
	"prism/internal/model"
	"prism/internal/settings"
	"prism/internal/store"
	"prism/internal/watch"
)

// GitHub is what the poller needs from internal/gh (faked in tests).
type GitHub interface {
	SearchPRs(ctx context.Context, q string) (gh.SearchResult, error)
	Snapshots(ctx context.Context, ids []string) (gh.SnapshotResult, error)
	Notifications(ctx context.Context, ifModifiedSince string) (gh.NotificationsResult, error)
}

// Identity is the signed-in user.
type Identity struct {
	Login string
	Teams []string // "org/team"
}

// Status is reported after every slow-tier cycle.
type Status struct {
	LastSync      time.Time // last successful cycle
	Err           error     // last error; nil when healthy
	Watched       int       // PRs in the watch set
	RateRemaining int       // GraphQL points left
	Skipped       []Skipped // owners whose search failed this cycle (SSO, permissions)
}

// Skipped is an owner the last cycle could not search; its PRs are kept as-is.
type Skipped struct {
	Owner  string // org or user login
	SSOURL string // authorize link when SAML SSO is the reason
	Err    string
}

// Options configure a Poller.
type Options struct {
	GitHub   GitHub
	Store    *store.Store
	Me       Identity
	Log      *slog.Logger
	OnEvents func([]store.Fresh) // new events, already stored; called from the poll goroutine
	OnStatus func(Status)
	Clock    Clock // nil: real clock
}

// Poller polls GitHub and records events.
type Poller struct {
	gh       GitHub
	store    *store.Store
	me       Identity
	log      *slog.Logger
	onEvents func([]store.Fresh)
	onStatus func(Status)
	clock    Clock

	mu      sync.Mutex // serializes cycles
	trigger chan struct{}
	status  Status
	recheck map[string]bool // my PRs whose mergeable was UNKNOWN; re-polled after 30 s

	usageStart time.Time // hourly GraphQL usage log
	usageCost  int
}

// New returns a Poller. Call Run to start polling.
func New(o Options) *Poller {
	if o.Log == nil {
		o.Log = slog.Default()
	}
	if o.Clock == nil {
		o.Clock = realClock{}
	}
	if o.OnEvents == nil {
		o.OnEvents = func([]store.Fresh) {}
	}
	if o.OnStatus == nil {
		o.OnStatus = func(Status) {}
	}
	return &Poller{
		gh: o.GitHub, store: o.Store, me: o.Me, log: o.Log,
		onEvents: o.OnEvents, onStatus: o.OnStatus, clock: o.Clock,
		trigger: make(chan struct{}, 1),
		recheck: map[string]bool{},
	}
}

// Trigger asks Run for an immediate slow-tier cycle (Refresh button,
// settings change). It never blocks.
func (p *Poller) Trigger() {
	select {
	case p.trigger <- struct{}{}:
	default:
	}
}

// Cycle runs one slow-tier poll: build the watch set, snapshot every watched
// PR, diff against the stored snapshot, record events, and handle PRs that
// left the set (SPEC §4–5).
func (p *Poller) Cycle(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	err := p.cycle(ctx)
	p.status.Err = err
	if err == nil {
		p.status.LastSync = p.clock.Now()
	}
	p.onStatus(p.status)
	return err
}

func (p *Poller) cycle(ctx context.Context) error {
	st, err := p.store.Settings(ctx)
	if err != nil {
		return err
	}
	firstRun, err := p.firstRun(ctx)
	if err != nil {
		return err
	}
	set, err := watch.Build(ctx, p.gh, p.watchConfig(st))
	if err != nil {
		return err
	}
	p.status.Skipped = nil
	for q, ferr := range set.Failed {
		p.log.Warn("owner skipped this cycle", "owner", q, "err", ferr)
		_, owner, _ := strings.Cut(q, ":")
		sk := Skipped{Owner: owner, Err: ferr.Error()}
		var sso *gh.SSORequiredError
		if errors.As(ferr, &sso) {
			sk.SSOURL = sso.URL
		}
		p.status.Skipped = append(p.status.Skipped, sk)
	}
	slices.SortFunc(p.status.Skipped, func(a, b Skipped) int { return strings.Compare(a.Owner, b.Owner) })
	states, err := p.store.PRStates(ctx)
	if err != nil {
		return err
	}

	cost := set.Cost
	p.status.RateRemaining = set.Remaining
	n, err := p.refresh(ctx, set.IDs(), states, st, firstRun, &cost)
	if err != nil {
		return err
	}
	if err := p.handleLeft(ctx, set, states, st, firstRun, &cost); err != nil {
		return err
	}
	if firstRun {
		// SPEC §4.5: the first run is silent except quiet, list-only items for
		// review requests pending right now (kept by refresh).
		if err := p.store.SetMeta(ctx, store.MetaBaselineDone, "1"); err != nil {
			return err
		}
		p.log.Info("baseline done", "prs", len(set.PRs))
	}
	p.status.Watched = len(set.PRs)
	p.logUsage(cost)
	p.log.Debug("cycle done", "prs", len(set.PRs), "events", n, "cost", cost, "remaining", p.status.RateRemaining)
	return nil
}

// RefreshPRs snapshots known PRs early (fast-tier hint). It returns true when
// a ref is not a known PR, i.e. a full cycle is needed to pick it up.
func (p *Poller) RefreshPRs(ctx context.Context, refs []gh.PRRef) (needFull bool, err error) {
	states, err := p.store.PRStates(ctx)
	if err != nil {
		return false, err
	}
	st, err := p.store.Settings(ctx)
	if err != nil {
		return false, err
	}
	byRef := map[gh.PRRef]string{}
	for id, s := range states {
		byRef[gh.PRRef{Repo: strings.ToLower(s.Repo), Number: s.Number}] = id
	}
	owners := p.watchedOwners(st)
	var ids []string
	for _, ref := range refs {
		if !owners[ownerOf(ref.Repo)] {
			continue
		}
		if id, ok := byRef[gh.PRRef{Repo: strings.ToLower(ref.Repo), Number: ref.Number}]; ok {
			ids = append(ids, id)
		} else {
			needFull = true
		}
	}
	return needFull, p.RefreshIDs(ctx, ids)
}

// RefreshIDs snapshots and diffs the given known PRs now.
func (p *Poller) RefreshIDs(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	st, err := p.store.Settings(ctx)
	if err != nil {
		return err
	}
	states, err := p.store.PRStates(ctx)
	if err != nil {
		return err
	}
	known := ids[:0:0]
	for _, id := range ids {
		if _, ok := states[id]; ok {
			known = append(known, id)
		}
	}
	firstRun, err := p.firstRun(ctx)
	if err != nil {
		return err
	}
	cost := 0
	_, err = p.refresh(ctx, known, states, st, firstRun, &cost)
	p.logUsage(cost)
	return err
}

// takeRecheck returns and clears the PRs waiting for a mergeable re-poll.
func (p *Poller) takeRecheck() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	ids := make([]string, 0, len(p.recheck))
	for id := range p.recheck {
		ids = append(ids, id)
	}
	clear(p.recheck)
	return ids
}

func (p *Poller) hasRecheck() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.recheck) > 0
}

// refresh snapshots ids, diffs them against states, records the events and
// then stores the new snapshots. Recording first means a crash in between
// re-detects the same dedup keys next time instead of losing events.
func (p *Poller) refresh(ctx context.Context, ids []string, states map[string]store.PRState,
	st settings.Settings, firstRun bool, cost *int) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	res, err := p.gh.Snapshots(ctx, ids)
	if err != nil {
		return 0, err
	}
	*cost += res.Cost
	p.status.RateRemaining = res.Remaining
	for id, ferr := range res.Failed {
		p.log.Warn("snapshot failed", "pr", id, "err", ferr)
	}

	var events []model.Event
	for _, curr := range res.PRs {
		var prev *model.PRSnapshot
		if s, ok := states[curr.ID]; ok {
			prev = s.Snapshot
		}
		for _, e := range detect.Diff(prev, curr, p.detectContext(st, prev == nil)) {
			if !firstRun || e.Type == model.ReviewRequested {
				events = append(events, e)
			}
		}
		if curr.Mergeable == "UNKNOWN" && strings.EqualFold(curr.Author.Login, p.me.Login) {
			p.recheck[curr.ID] = true
		} else {
			delete(p.recheck, curr.ID)
		}
	}
	n, err := p.record(ctx, events, st, firstRun)
	if err != nil {
		return 0, err
	}
	for _, curr := range res.PRs {
		role := store.RoleOther
		if strings.EqualFold(curr.Author.Login, p.me.Login) {
			role = store.RoleMine
		}
		if err := p.store.PutPRState(ctx, store.PRState{
			PRID: curr.ID, Repo: curr.Repo, Number: curr.Number, Role: role, Snapshot: curr,
		}); err != nil {
			return n, err
		}
		if err := p.dropDeleted(ctx, curr); err != nil {
			return n, err
		}
	}
	return n, nil
}

// handleLeft deals with PRs that were watched but are not in the set now
// (SPEC §5): unselected owner → purge silently; otherwise fetch once — merged
// or closed → purge and diff (my_pr.closed); still open or gone → purge
// silently. PRs of owners whose search failed this cycle are left alone.
func (p *Poller) handleLeft(ctx context.Context, set watch.Set, states map[string]store.PRState,
	st settings.Settings, firstRun bool, cost *int) error {
	failed := map[string]bool{}
	for q := range set.Failed {
		_, owner, _ := strings.Cut(q, ":")
		failed[strings.ToLower(owner)] = true
	}
	owners := p.watchedOwners(st)

	var left []string
	for id, s := range states {
		if _, in := set.PRs[id]; in {
			continue
		}
		owner := ownerOf(s.Repo)
		switch {
		case failed[owner]:
			continue
		case !owners[owner]:
			if err := p.store.PurgePR(ctx, id); err != nil {
				return err
			}
			p.log.Info("purged PR of unselected owner", "repo", s.Repo, "number", s.Number)
		default:
			left = append(left, id)
		}
	}
	if len(left) == 0 {
		return nil
	}

	res, err := p.gh.Snapshots(ctx, left)
	if err != nil {
		return err
	}
	*cost += res.Cost
	var events []model.Event
	for _, id := range left {
		if _, failedNow := res.Failed[id]; failedNow {
			continue // try again next cycle
		}
		final := res.PRs[id]
		if final != nil && final.State != model.PROpen && !firstRun {
			events = append(events, detect.Diff(states[id].Snapshot, final, p.detectContext(st, false))...)
		}
		// Purge before recording: purge deletes the PR's tombstones, and the
		// closed event's tombstone must survive.
		if err := p.store.PurgePR(ctx, id); err != nil {
			return err
		}
		p.log.Debug("PR left the watch set", "repo", states[id].Repo, "number", states[id].Number)
	}
	_, err = p.record(ctx, events, st, false)
	return err
}

// dropDeleted silently removes notifications whose comment or review no
// longer exists (SPEC §4.6).
func (p *Poller) dropDeleted(ctx context.Context, curr *model.PRSnapshot) error {
	stored, err := p.store.ForPR(ctx, curr.ID)
	if err != nil || len(stored) == 0 {
		return err
	}
	exists := map[string]bool{}
	for _, r := range curr.Reviews {
		exists["review:"+strconv.FormatInt(r.DatabaseID, 10)] = true
	}
	for _, c := range curr.Comments {
		exists["comment:"+strconv.FormatInt(c.DatabaseID, 10)] = true
	}
	for _, t := range curr.Threads {
		for _, c := range t.Comments {
			exists["comment:"+strconv.FormatInt(c.DatabaseID, 10)] = true
		}
	}
	var gone []int64
	for _, n := range stored {
		if (strings.HasPrefix(n.DedupKey, "review:") || strings.HasPrefix(n.DedupKey, "comment:")) && !exists[n.DedupKey] {
			gone = append(gone, n.ID)
		}
	}
	_, err = p.store.MarkRead(ctx, gone...)
	return err
}

func (p *Poller) record(ctx context.Context, events []model.Event, st settings.Settings, quiet bool) (int, error) {
	list := func(t model.EventType) bool { return quiet || st.FlagsFor(t).List }
	fresh, err := p.store.Record(ctx, events, list)
	if err != nil {
		return 0, err
	}
	for i := range fresh {
		fresh[i].Quiet = quiet
	}
	if len(fresh) > 0 {
		p.onEvents(fresh)
	}
	return len(fresh), nil
}

func (p *Poller) firstRun(ctx context.Context) (bool, error) {
	v, err := p.store.Meta(ctx, store.MetaBaselineDone)
	return v != "1", err
}

func (p *Poller) watchConfig(st settings.Settings) watch.Config {
	return watch.Config{
		Me: p.me.Login, Orgs: st.Owners, IncludePersonal: st.IncludePersonal,
		IncludeDrafts: st.IncludeDrafts, Teams: p.me.Teams,
	}
}

func (p *Poller) detectContext(st settings.Settings, firstSeen bool) model.DetectContext {
	return model.DetectContext{Me: p.me.Login, MyTeams: p.me.Teams, IgnoreBots: st.IgnoreBots, FirstSeen: firstSeen}
}

// watchedOwners are the lower-cased owners the settings select.
func (p *Poller) watchedOwners(st settings.Settings) map[string]bool {
	owners := map[string]bool{}
	for _, o := range st.Owners {
		owners[strings.ToLower(o)] = true
	}
	if st.IncludePersonal {
		owners[strings.ToLower(p.me.Login)] = true
	}
	return owners
}

func ownerOf(repo string) string {
	owner, _, _ := strings.Cut(repo, "/")
	return strings.ToLower(owner)
}

// logUsage logs GraphQL points spent per hour (PLAN Phase 4 DoD: < 10 % of 5 000).
func (p *Poller) logUsage(cost int) {
	now := p.clock.Now()
	if p.usageStart.IsZero() {
		p.usageStart = now
	}
	p.usageCost += cost
	if now.Sub(p.usageStart) >= time.Hour {
		p.log.Info("graphql usage", "points", p.usageCost, "since", p.usageStart.Format(time.RFC3339),
			"budget_pct", fmt.Sprintf("%.1f", float64(p.usageCost)/50))
		p.usageStart, p.usageCost = now, 0
	}
}
