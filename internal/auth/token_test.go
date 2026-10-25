package auth

import (
	"errors"
	"slices"
	"testing"
)

func TestCheckPATFormat(t *testing.T) {
	tests := []struct {
		token string
		want  error
	}{
		{"ghp_abcdef", nil},
		{"github_pat_11ABC", ErrFineGrainedPAT},
		{"gho_oauthtoken", ErrUnknownTokenFormat},
		{"ghs_apptoken", ErrUnknownTokenFormat},
		{"plain", ErrUnknownTokenFormat},
	}
	for _, tt := range tests {
		if err := CheckPATFormat(tt.token); !errors.Is(err, tt.want) {
			t.Errorf("CheckPATFormat(%q) = %v, want %v", tt.token, err, tt.want)
		}
	}
}

func TestCheckScopes(t *testing.T) {
	tests := []struct {
		name    string
		granted []string
		missing []string
	}{
		{"exact", []string{"notifications", "read:org", "repo"}, nil},
		{"repo covers notifications, admin:org covers read:org", []string{"admin:org", "repo"}, nil},
		{"write:org covers read:org", []string{"notifications", "repo", "write:org"}, nil},
		{"nothing", nil, []string{"repo", "notifications", "read:org"}},
		{"public_repo is not repo", []string{"notifications", "public_repo", "read:org"}, []string{"repo"}},
		{"no org", []string{"repo"}, []string{"read:org"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckScopes(tt.granted)
			var ms *MissingScopesError
			switch {
			case tt.missing == nil && err != nil:
				t.Errorf("got %v, want nil", err)
			case tt.missing != nil && (!errors.As(err, &ms) || !slices.Equal(ms.Missing, tt.missing)):
				t.Errorf("got %v, want missing %v", err, tt.missing)
			}
		})
	}
}
