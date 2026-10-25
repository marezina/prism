package auth

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

var (
	// ErrFineGrainedPAT: fine-grained PATs cannot call the Notifications API.
	ErrFineGrainedPAT = errors.New("fine-grained personal access tokens are not supported: " +
		"GitHub's Notifications API needs a classic token (ghp_…)")
	ErrUnknownTokenFormat = errors.New("not a GitHub classic personal access token (expected ghp_…)")
)

// MissingScopesError lists required scopes a token lacks.
type MissingScopesError struct {
	Missing []string
}

func (e *MissingScopesError) Error() string {
	return "token is missing required scopes: " + strings.Join(e.Missing, ", ")
}

// CheckPATFormat accepts classic PATs only (auth.md §PAT fallback).
func CheckPATFormat(token string) error {
	switch {
	case strings.HasPrefix(token, "github_pat_"):
		return ErrFineGrainedPAT
	case strings.HasPrefix(token, "ghp_"):
		return nil
	default:
		return ErrUnknownTokenFormat
	}
}

// impliedBy lists, per required scope, the granted scopes that also satisfy it
// (GitHub's scope hierarchy; for notifications, the REST docs accept repo too).
var impliedBy = map[string][]string{
	"repo":          {"repo"},
	"notifications": {"notifications", "repo"},
	"read:org":      {"read:org", "write:org", "admin:org"},
}

// CheckScopes reports which of Scopes the granted X-OAuth-Scopes lack.
func CheckScopes(granted []string) error {
	var missing []string
	for _, want := range Scopes {
		ok := slices.ContainsFunc(impliedBy[want], func(s string) bool { return slices.Contains(granted, s) })
		if !ok {
			missing = append(missing, want)
		}
	}
	if len(missing) > 0 {
		return &MissingScopesError{Missing: missing}
	}
	return nil
}

// cleanToken trims whitespace a paste may carry and rejects the empty string.
func cleanToken(token string) (string, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return "", fmt.Errorf("empty token")
	}
	return token, nil
}
