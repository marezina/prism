package app

import (
	"context"
	"slices"
	"strings"

	"prism/internal/model"
)

// PRStatus is a PR's state at the last poll, shown in the popup's group header.
type PRStatus struct {
	Draft            bool   `json:"draft"`
	Approvals        int    `json:"approvals"`
	ChangesRequested int    `json:"changesRequested"`
	CI               string `json:"ci,omitempty"` // passing | failing | pending; "" without CI
	Conflict         bool   `json:"conflict"`
}

// CI values of PRStatus.
const (
	CIPassing = "passing"
	CIFailing = "failing"
	CIPending = "pending"
)

// prStatus summarizes a snapshot. Each reviewer counts once, by their latest
// approving or blocking review; a dismissal clears it, comments don't change it.
func prStatus(s *model.PRSnapshot) *PRStatus {
	if s == nil {
		return nil
	}
	st := &PRStatus{Draft: s.IsDraft, Conflict: s.Mergeable == "CONFLICTING"}
	latest := map[string]model.ReviewState{}
	for _, r := range s.Reviews { // oldest first
		who := strings.ToLower(r.Author.Login)
		switch r.State {
		case model.ReviewApproved, model.ReviewChangesRequested:
			latest[who] = r.State
		case model.ReviewDismissed:
			delete(latest, who)
		}
	}
	for _, v := range latest {
		if v == model.ReviewApproved {
			st.Approvals++
		} else {
			st.ChangesRequested++
		}
	}
	if s.CI != nil {
		switch s.CI.State {
		case model.CheckSuccess:
			st.CI = CIPassing
		case model.CheckFailure, model.CheckError:
			st.CI = CIFailing
		case model.CheckPending, model.CheckExpected:
			st.CI = CIPending
		}
	}
	return st
}

// MutedPR is a row of Settings → Muted PRs.
type MutedPR struct {
	PRID   string `json:"prId"`
	Repo   string `json:"repo"`
	Number int    `json:"number"`
	Title  string `json:"title"`
	URL    string `json:"url"`
}

// MutePR silences a PR until it is closed or unmuted: its notifications are
// marked read and its new activity is not recorded.
func (c *Core) MutePR(ctx context.Context, prID string) error {
	err := c.opts.Store.SetMuted(ctx, prID, true)
	c.notifyUnread(ctx)
	return err
}

// UnmutePR lets a PR notify again, starting with its next new activity.
func (c *Core) UnmutePR(ctx context.Context, prID string) error {
	return c.opts.Store.SetMuted(ctx, prID, false)
}

// MutedPRs lists the muted PRs by repository and number.
func (c *Core) MutedPRs(ctx context.Context) ([]MutedPR, error) {
	states, err := c.opts.Store.PRStates(ctx)
	if err != nil {
		return nil, err
	}
	out := []MutedPR{}
	for id, s := range states {
		if s.Muted {
			out = append(out, MutedPR{PRID: id, Repo: s.Repo, Number: s.Number, Title: s.Snapshot.Title,
				URL: c.prURL(s.Repo, s.Number)})
		}
	}
	slices.SortFunc(out, func(a, b MutedPR) int {
		if r := strings.Compare(a.Repo, b.Repo); r != 0 {
			return r
		}
		return a.Number - b.Number
	})
	return out, nil
}
