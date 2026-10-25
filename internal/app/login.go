package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"prism/internal/auth"
	"prism/internal/gh"
)

type loginAttempt struct {
	cancel context.CancelFunc
}

// StartLogin begins the OAuth device flow (SPEC §8 step 2). The code is
// returned at once; the rest runs in the background and ends in an
// AuthChanged hook (choose_orgs, or signed_out with a message). An expired
// code is replaced by a new one automatically.
func (c *Core) StartLogin(ctx context.Context) (AuthState, error) {
	c.cancelLogin()
	flow := auth.NewDeviceFlow(c.opts.ClientID, c.opts.Endpoints, c.opts.Version)
	dc, err := flow.Start(ctx)
	if err != nil {
		return c.Auth(), loginError(err)
	}
	lctx, cancel := context.WithCancel(context.Background())
	c.mu.Lock()
	c.login = &loginAttempt{cancel: cancel}
	c.mu.Unlock()

	c.setAuth(awaiting(dc))
	if h := c.opts.Hooks.OpenURL; h != nil {
		if err := h(dc.VerificationURI); err != nil {
			c.log.Debug("open verification page", "err", err)
		}
	}
	go c.waitForUser(lctx, flow, dc)
	return c.Auth(), nil
}

func (c *Core) waitForUser(ctx context.Context, flow *auth.DeviceFlow, dc *auth.DeviceCode) {
	for {
		token, err := flow.Wait(ctx, dc)
		switch {
		case errors.Is(err, context.Canceled):
			return
		case errors.Is(err, auth.ErrCodeExpired):
			next, serr := flow.Start(ctx)
			if serr != nil {
				c.setAuth(AuthState{State: StateSignedOut, Message: loginError(serr).Error()})
				return
			}
			dc = next
			c.setAuth(awaiting(dc))
			continue
		case errors.Is(err, auth.ErrAccessDenied):
			c.setAuth(AuthState{State: StateSignedOut, Message: "Access was denied in the browser. Try again when you're ready."})
			return
		case err != nil:
			c.setAuth(AuthState{State: StateSignedOut, Message: loginError(err).Error()})
			return
		}
		client, id, err := c.opts.Session.Complete(ctx, token)
		if err != nil {
			c.setAuth(AuthState{State: StateSignedOut, Message: loginError(err).Error()})
			return
		}
		c.signedIn(ctx, client, id)
		return
	}
}

// CancelLogin abandons a running device flow and returns to the welcome screen.
func (c *Core) CancelLogin() {
	c.cancelLogin()
	c.setAuth(AuthState{State: StateSignedOut})
}

func (c *Core) cancelLogin() {
	c.mu.Lock()
	l := c.login
	c.login = nil
	c.mu.Unlock()
	if l != nil {
		l.cancel()
	}
}

// LoginWithToken signs in with a classic personal access token.
func (c *Core) LoginWithToken(ctx context.Context, token string) (AuthState, error) {
	c.cancelLogin()
	client, id, err := c.opts.Session.CompletePAT(ctx, token)
	if err != nil {
		return c.Auth(), loginError(err)
	}
	c.signedIn(ctx, client, id)
	return c.Auth(), nil
}

// Logout stops polling and forgets the token. Local notifications stay until
// the next sign-in replaces them ("Reset local data" clears everything).
func (c *Core) Logout(ctx context.Context) error {
	c.Stop()
	if err := c.opts.Session.Logout(); err != nil {
		return err
	}
	c.mu.Lock()
	c.client, c.identity = nil, gh.Identity{}
	c.mu.Unlock()
	if err := c.opts.Store.SetMeta(ctx, MetaOrgsChosen, ""); err != nil {
		return err
	}
	c.setAuth(AuthState{State: StateSignedOut})
	return nil
}

// Orgs lists the organization picker rows: personal repositories first, then
// the user's orgs; Selected reflects the settings (SPEC §8 step 3).
func (c *Core) Orgs(ctx context.Context) ([]Org, error) {
	c.mu.Lock()
	client, id := c.client, c.identity
	c.mu.Unlock()
	if client == nil {
		return nil, errors.New("not signed in")
	}
	st, err := c.opts.Store.Settings(ctx)
	if err != nil {
		return nil, err
	}
	chosen, _ := c.opts.Store.Meta(ctx, MetaOrgsChosen)
	firstTime := chosen != "1"

	orgs, err := client.Orgs(ctx)
	if err != nil {
		return nil, err
	}
	out := []Org{{Login: id.Login, AvatarURL: id.AvatarURL, Personal: true, Selected: st.IncludePersonal}}
	for _, o := range orgs {
		selected := slices.ContainsFunc(st.Owners, func(s string) bool { return strings.EqualFold(s, o.Login) })
		if firstTime {
			selected = !o.SSORequired // first run: preselect every usable org
		}
		out = append(out, Org{Login: o.Login, AvatarURL: o.AvatarURL, Selected: selected,
			SSORequired: o.SSORequired, SSOURL: o.SSOURL})
	}
	return out, nil
}

// ChooseOrgs saves the picker selection and starts watching (SPEC §8 step 4).
func (c *Core) ChooseOrgs(ctx context.Context, owners []string, includePersonal bool) error {
	c.mu.Lock()
	client, id := c.client, c.identity
	c.mu.Unlock()
	if client == nil {
		return errors.New("not signed in")
	}
	st, err := c.opts.Store.Settings(ctx)
	if err != nil {
		return err
	}
	st.Owners, st.IncludePersonal = owners, includePersonal
	if st.Owners == nil {
		st.Owners = []string{}
	}
	if err := c.opts.Store.SaveSettings(ctx, st); err != nil {
		return err
	}
	if err := c.opts.Store.SetMeta(ctx, MetaOrgsChosen, "1"); err != nil {
		return err
	}
	if c.Auth().State == StateChooseOrgs {
		c.setAuth(AuthState{State: StateSignedIn, Login: id.Login, AvatarURL: id.AvatarURL})
		c.startPolling(client, id)
		return nil
	}
	c.RefreshNow() // changed from Settings: next cycle rebuilds the watch set
	return nil
}

func awaiting(dc *auth.DeviceCode) AuthState {
	return AuthState{
		State: StateAwaitingUser, UserCode: dc.UserCode, VerificationURI: dc.VerificationURI,
		ExpiresAt: dc.ExpiresAt.UTC().Format(time.RFC3339),
	}
}

// loginError turns auth and GitHub errors into messages for the login screen.
func loginError(err error) error {
	var ms *auth.MissingScopesError
	var sso *gh.SSORequiredError
	switch {
	case errors.Is(err, auth.ErrNoClientID):
		return errors.New("sign-in with GitHub is not configured in this build; use a personal access token")
	case errors.Is(err, auth.ErrFineGrainedPAT), errors.Is(err, auth.ErrUnknownTokenFormat):
		return err
	case errors.As(err, &ms):
		return fmt.Errorf("the token lacks scopes: %s", strings.Join(ms.Missing, ", "))
	case errors.Is(err, gh.ErrUnauthorized):
		return errors.New("GitHub rejected the token")
	case errors.As(err, &sso):
		return fmt.Errorf("your organization requires SSO authorization: %s", sso.URL)
	case errors.Is(err, auth.ErrKeyringUnavailable):
		return err
	}
	return fmt.Errorf("could not sign in: %w", err)
}
