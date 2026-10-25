package auth

import (
	"context"
	"errors"
	"fmt"

	"prism/internal/gh"
)

// NewClient builds a GitHub client for a token. Tests replace it.
type NewClient func(token string) (*gh.Client, error)

// Session ties token storage to the GitHub client.
type Session struct {
	Store     Store
	NewClient NewClient
}

// Complete verifies a freshly obtained token (device flow or PAT), checks its
// scopes and stores it. Nothing is stored when verification fails.
func (s *Session) Complete(ctx context.Context, token string) (*gh.Client, gh.Identity, error) {
	token, err := cleanToken(token)
	if err != nil {
		return nil, gh.Identity{}, err
	}
	c, id, err := s.verify(ctx, token)
	if err != nil {
		return nil, gh.Identity{}, err
	}
	if err := s.Store.Save(id.Login, token); err != nil {
		return nil, gh.Identity{}, err
	}
	return c, id, nil
}

// CompletePAT is Complete for a pasted personal access token.
func (s *Session) CompletePAT(ctx context.Context, token string) (*gh.Client, gh.Identity, error) {
	token, err := cleanToken(token)
	if err != nil {
		return nil, gh.Identity{}, err
	}
	if err := CheckPATFormat(token); err != nil {
		return nil, gh.Identity{}, err
	}
	return s.Complete(ctx, token)
}

// Restore loads the stored token and checks it still works. A revoked token
// (gh.ErrUnauthorized) is deleted from the keyring before the error returns.
func (s *Session) Restore(ctx context.Context) (*gh.Client, gh.Identity, error) {
	_, token, err := s.Store.Load()
	if err != nil {
		return nil, gh.Identity{}, err
	}
	c, id, err := s.verify(ctx, token)
	if errors.Is(err, gh.ErrUnauthorized) {
		if delErr := s.Store.Delete(); delErr != nil {
			return nil, gh.Identity{}, errors.Join(err, delErr)
		}
	}
	if err != nil {
		return nil, gh.Identity{}, err
	}
	return c, id, nil
}

// Logout forgets the stored token. PRism never revokes it on GitHub (read-only).
func (s *Session) Logout() error { return s.Store.Delete() }

func (s *Session) verify(ctx context.Context, token string) (*gh.Client, gh.Identity, error) {
	c, err := s.NewClient(token)
	if err != nil {
		return nil, gh.Identity{}, err
	}
	id, err := c.Whoami(ctx)
	if err != nil {
		return nil, gh.Identity{}, fmt.Errorf("verify token: %w", err)
	}
	if err := CheckScopes(id.Scopes); err != nil {
		return nil, gh.Identity{}, err
	}
	return c, id, nil
}
