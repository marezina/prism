package gh

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/google/go-github/v92/github"
)

// Identity is the signed-in user plus the token's classic OAuth scopes.
type Identity struct {
	Login     string
	ID        int64
	Name      string
	AvatarURL string
	Scopes    []string // from X-OAuth-Scopes; empty for fine-grained tokens
}

// Whoami calls GET /user.
func (c *Client) Whoami(ctx context.Context) (Identity, error) {
	u, resp, err := c.REST.Users.Get(ctx, "")
	if err != nil {
		return Identity{}, fmt.Errorf("get /user: %w", err)
	}
	return Identity{
		Login:     u.GetLogin(),
		ID:        u.GetID(),
		Name:      u.GetName(),
		AvatarURL: u.GetAvatarURL(),
		Scopes:    parseScopes(resp.Header.Get("X-OAuth-Scopes")),
	}, nil
}

func parseScopes(h string) []string {
	var scopes []string
	for _, s := range strings.Split(h, ",") {
		if s = strings.TrimSpace(s); s != "" {
			scopes = append(scopes, s)
		}
	}
	sort.Strings(scopes)
	return scopes
}

// Org is an organization the user belongs to.
type Org struct {
	Login       string
	ID          int64
	AvatarURL   string
	SSORequired bool   // token not authorized for this org's SAML SSO
	SSOURL      string // where to authorize, when SSORequired
}

// Orgs lists GET /user/orgs. Orgs that GitHub hid because the token lacks SAML
// SSO authorization (X-GitHub-SSO: partial-results) are included with
// SSORequired set, so the picker can show an "Authorize" link.
func (c *Client) Orgs(ctx context.Context) ([]Org, error) {
	var (
		orgs   []Org
		hidden []int64
	)
	opts := &github.ListOptions{PerPage: 100}
	for {
		page, resp, err := c.REST.Organizations.List(ctx, "", opts)
		if err != nil {
			return nil, fmt.Errorf("list /user/orgs: %w", err)
		}
		for _, o := range page {
			orgs = append(orgs, Org{Login: o.GetLogin(), ID: o.GetID(), AvatarURL: o.GetAvatarURL()})
		}
		hidden = append(hidden, partialSSOOrgs(resp.Header.Get("X-GitHub-SSO"))...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	for _, id := range hidden {
		org, err := c.ssoOrg(ctx, id)
		if err != nil {
			return nil, err
		}
		orgs = append(orgs, org)
	}
	sort.Slice(orgs, func(i, j int) bool { return strings.ToLower(orgs[i].Login) < strings.ToLower(orgs[j].Login) })
	return orgs, nil
}

// ssoOrg resolves an SSO-hidden org id to its login and authorization URL.
func (c *Client) ssoOrg(ctx context.Context, id int64) (Org, error) {
	o, _, err := c.REST.Organizations.GetByID(ctx, id)
	var sso *SSORequiredError
	switch {
	case errors.As(err, &sso):
		// The public org endpoint itself is SSO-guarded; GitHub's URL is the best link.
		return Org{Login: fmt.Sprintf("org #%d", id), ID: id, SSORequired: true, SSOURL: sso.URL}, nil
	case err != nil:
		return Org{}, fmt.Errorf("get /organizations/%d: %w", id, err)
	}
	return Org{
		Login:       o.GetLogin(),
		ID:          id,
		AvatarURL:   o.GetAvatarURL(),
		SSORequired: true,
		SSOURL:      c.endpoints.OrgSSOURL(o.GetLogin()),
	}, nil
}

// Teams lists GET /user/teams as "org/team-slug", for team review requests.
func (c *Client) Teams(ctx context.Context) ([]string, error) {
	var teams []string
	opts := &github.ListOptions{PerPage: 100}
	for {
		page, resp, err := c.REST.Teams.ListUserTeams(ctx, opts)
		if err != nil {
			return nil, fmt.Errorf("list /user/teams: %w", err)
		}
		for _, t := range page {
			teams = append(teams, t.GetOrganization().GetLogin()+"/"+t.GetSlug())
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	sort.Strings(teams)
	return teams, nil
}
