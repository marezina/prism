package app

import (
	"strconv"
	"time"

	"prism/internal/store"
)

// Auth states for the UI (SPEC §8).
const (
	StateSignedOut    = "signed_out"    // welcome screen, optional Message
	StateAwaitingUser = "awaiting_user" // device code shown, waiting for the browser
	StateChooseOrgs   = "choose_orgs"   // signed in, organization picker
	StateSignedIn     = "signed_in"
)

// AuthState is pushed to the UI as "auth:changed".
type AuthState struct {
	State     string `json:"state"`
	Login     string `json:"login,omitempty"`
	AvatarURL string `json:"avatarUrl,omitempty"`
	Message   string `json:"message,omitempty"` // error or hint to show

	// Device flow (StateAwaitingUser).
	UserCode        string `json:"userCode,omitempty"`
	VerificationURI string `json:"verificationUri,omitempty"`
	ExpiresAt       string `json:"expiresAt,omitempty"` // RFC3339
}

// SyncStatus is pushed to the UI as "sync:status".
type SyncStatus struct {
	LastSync      string       `json:"lastSync,omitempty"` // RFC3339 of the last successful poll
	Offline       bool         `json:"offline"`
	Syncing       bool         `json:"syncing"` // a full poll is running
	Error         string       `json:"error,omitempty"`
	SSOURL        string       `json:"ssoUrl,omitempty"`
	Watched       int          `json:"watched"`
	RateRemaining int          `json:"rateRemaining"`
	Skipped       []SkippedOrg `json:"skipped"`
}

// SkippedOrg is an organization PRism could not read in the last poll.
type SkippedOrg struct {
	Login  string `json:"login"`
	SSOURL string `json:"ssoUrl,omitempty"`
	Error  string `json:"error"`
}

// Notification is one unread item for the popup.
type Notification struct {
	ID          int64  `json:"id"`
	Type        string `json:"type"`
	PRID        string `json:"prId"`
	Repo        string `json:"repo"`
	PRNumber    int    `json:"prNumber"`
	PRTitle     string `json:"prTitle"`
	PRURL       string `json:"prUrl"`
	Mine        bool   `json:"mine"` // the PR is authored by me
	ActorLogin  string `json:"actorLogin,omitempty"`
	ActorAvatar string `json:"actorAvatar,omitempty"`
	Summary     string `json:"summary"`
	Excerpt     string `json:"excerpt,omitempty"`
	URL         string `json:"url"`
	OccurredAt  string `json:"occurredAt"` // RFC3339
}

// Org is a row of the organization picker.
type Org struct {
	Login       string `json:"login"`
	AvatarURL   string `json:"avatarUrl,omitempty"`
	Personal    bool   `json:"personal"`
	Selected    bool   `json:"selected"`
	SSORequired bool   `json:"ssoRequired"`
	SSOURL      string `json:"ssoUrl,omitempty"`
}

func (c *Core) toDTO(n store.Notification, mine map[string]bool) Notification {
	return Notification{
		ID: n.ID, Type: string(n.Type), PRID: n.PRID, Repo: n.Repo, PRNumber: n.PRNumber, PRTitle: n.PRTitle,
		PRURL: c.prURL(n.Repo, n.PRNumber), Mine: mine[n.PRID] || isMyPRType(string(n.Type)),
		ActorLogin: n.Actor.Login, ActorAvatar: n.Actor.AvatarURL, Summary: n.Summary, Excerpt: n.Excerpt,
		URL: n.URL, OccurredAt: n.OccurredAt.UTC().Format(time.RFC3339),
	}
}

// isMyPRType: events that only ever happen on my PRs (SPEC §3).
func isMyPRType(t string) bool {
	switch t {
	case "my_pr.approved", "my_pr.changes_requested", "my_pr.comment", "my_pr.thread_reply",
		"ci.failed", "ci.passed", "merge.conflict", "my_pr.closed", "my_pr.opened":
		return true
	}
	return false
}

func itoa(n int) string { return strconv.Itoa(n) }
