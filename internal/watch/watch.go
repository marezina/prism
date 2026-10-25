// Package watch builds the set of pull requests PRism watches from the user's
// settings (ARCHITECTURE §2 "Watch set").
package watch

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"prism/internal/gh"
)

// Role is why a PR is in the watch set.
type Role string

const (
	RoleMine            Role = "mine"             // authored by me
	RoleReviewRequested Role = "review_requested" // I or my team am requested
	RoleParticipated    Role = "participated"     // I reviewed, commented or was mentioned
)

// Config is the part of the settings the watch set depends on.
type Config struct {
	Me              string
	Orgs            []string // selected organizations
	IncludePersonal bool     // also watch user:<Me>
	IncludeDrafts   bool
	Teams           []string // "org/team" I belong to

	// CombineOwners puts all owner qualifiers into one search. Only valid if
	// GitHub OR-s them (ADR 0005, prism-cli verify-search); otherwise one
	// search per owner is made.
	CombineOwners bool
}

// Query is one search to run.
type Query struct {
	Role  Role
	Owner string // owner qualifier this query is scoped to; "" when combined
	Q     string
}

// Queries returns the searches for c. The literal login is used instead of
// @me so the queries do not depend on @me support in GraphQL search.
func Queries(c Config) []Query {
	type template struct {
		role Role
		q    string
	}
	me := c.Me
	templates := []template{
		{RoleMine, "author:" + me},
		{RoleReviewRequested, "review-requested:" + me},
		{RoleParticipated, "reviewed-by:" + me + " -author:" + me},
		{RoleParticipated, "commenter:" + me + " -author:" + me},
		{RoleParticipated, "mentions:" + me + " -author:" + me},
	}

	owners := ownerQualifiers(c)
	var out []Query
	add := func(role Role, owner, q string) {
		out = append(out, Query{Role: role, Owner: owner, Q: "is:pr is:open " + q + " " + owner})
	}
	if c.CombineOwners && len(owners) > 0 {
		all := strings.Join(owners, " ")
		for _, t := range templates {
			out = append(out, Query{Role: t.role, Q: "is:pr is:open " + t.q + " " + all})
		}
	} else {
		for _, o := range owners {
			for _, t := range templates {
				add(t.role, o, t.q)
			}
		}
	}
	for _, team := range c.Teams {
		org, _, _ := strings.Cut(team, "/")
		if slices.Contains(c.Orgs, org) {
			add(RoleReviewRequested, "org:"+org, "team-review-requested:"+team)
		}
	}
	return out
}

func ownerQualifiers(c Config) []string {
	var owners []string
	for _, o := range c.Orgs {
		owners = append(owners, "org:"+o)
	}
	if c.IncludePersonal {
		owners = append(owners, "user:"+c.Me)
	}
	return owners
}

// PR is one watched pull request.
type PR struct {
	ID      string
	Repo    string
	Number  int
	Title   string
	IsDraft bool
	Roles   []Role // sorted, unique
}

// Has reports whether the PR has role r.
func (p *PR) Has(r Role) bool { return slices.Contains(p.Roles, r) }

// Set is the union of all searches.
type Set struct {
	PRs       map[string]*PR   // by node id
	Failed    map[string]error // owner qualifier → error; that owner's PRs are missing
	Cost      int              // GraphQL points
	Remaining int              // GraphQL points left after the last search
}

// Searcher runs one GraphQL PR search (gh.Client implements it).
type Searcher interface {
	SearchPRs(ctx context.Context, q string) (gh.SearchResult, error)
}

// Build runs every query and merges the results. An owner whose search fails
// with SSO or a GraphQL error is recorded in Failed and skipped; the others
// still count (github-api skill). Auth, rate-limit and network errors abort.
func Build(ctx context.Context, s Searcher, c Config) (Set, error) {
	set := Set{PRs: map[string]*PR{}, Failed: map[string]error{}}
	for _, q := range Queries(c) {
		if _, failed := set.Failed[q.Owner]; failed && q.Owner != "" {
			continue
		}
		res, err := s.SearchPRs(ctx, q.Q)
		if err != nil {
			if ownerScoped(err) && q.Owner != "" {
				set.Failed[q.Owner] = err
				continue
			}
			return set, fmt.Errorf("watch set: %w", err)
		}
		set.Cost += res.Cost
		set.Remaining = res.Remaining
		for _, h := range res.Hits {
			if h.IsDraft && !c.IncludeDrafts {
				continue
			}
			pr := set.PRs[h.ID]
			if pr == nil {
				pr = &PR{ID: h.ID, Repo: h.Repo, Number: h.Number, Title: h.Title, IsDraft: h.IsDraft}
				set.PRs[h.ID] = pr
			}
			if !pr.Has(q.Role) {
				pr.Roles = append(pr.Roles, q.Role)
				slices.Sort(pr.Roles)
			}
		}
	}
	return set, nil
}

func ownerScoped(err error) bool {
	var sso *gh.SSORequiredError
	return errors.As(err, &sso) || gh.IsGraphQLError(err)
}

// IDs returns the node ids in the set, sorted.
func (s Set) IDs() []string {
	ids := make([]string, 0, len(s.PRs))
	for id := range s.PRs {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}
