package auth

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zalando/go-keyring"

	"prism/internal/gh"
)

// fakeUser serves GET /user; tokens maps a bearer token to its X-OAuth-Scopes
// (missing → 401).
func fakeUser(t *testing.T, tokens map[string]string) NewClient {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		scopes, ok := tokens[r.Header.Get("Authorization")[len("Bearer "):]]
		if !ok || r.URL.Path != "/user" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("X-OAuth-Scopes", scopes)
		writeJSON(w, map[string]any{"login": "octocat", "id": 1})
	}))
	t.Cleanup(srv.Close)
	return func(token string) (*gh.Client, error) {
		return gh.New(gh.Options{
			Token: token, Version: "test", Logger: slog.New(slog.DiscardHandler),
			Endpoints: gh.Endpoints{Web: srv.URL, API: srv.URL + "/", GraphQL: srv.URL + "/graphql"},
		})
	}
}

func TestKeyringStore(t *testing.T) {
	keyring.MockInit()
	s := NewKeyringStore()

	if _, _, err := s.Load(); !errors.Is(err, ErrNoToken) {
		t.Fatalf("empty Load = %v, want ErrNoToken", err)
	}
	if err := s.Delete(); err != nil {
		t.Fatalf("Delete on empty store = %v", err)
	}
	if err := s.Save("octocat", "gho_1"); err != nil {
		t.Fatal(err)
	}
	login, token, err := s.Load()
	if err != nil || login != "octocat" || token != "gho_1" {
		t.Fatalf("Load = %q, %q, %v", login, token, err)
	}
	if err := s.Delete(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Load(); !errors.Is(err, ErrNoToken) {
		t.Fatalf("Load after Delete = %v, want ErrNoToken", err)
	}
}

func TestKeyringUnavailable(t *testing.T) {
	keyring.MockInitWithError(errors.New("org.freedesktop.secrets was not provided"))
	defer keyring.MockInit()
	s := NewKeyringStore()
	if err := s.Save("octocat", "gho_1"); !errors.Is(err, ErrKeyringUnavailable) {
		t.Errorf("Save = %v, want ErrKeyringUnavailable", err)
	}
	if _, _, err := s.Load(); !errors.Is(err, ErrKeyringUnavailable) {
		t.Errorf("Load = %v, want ErrKeyringUnavailable", err)
	}
}

func TestSessionCompleteAndRestore(t *testing.T) {
	keyring.MockInit()
	tokens := map[string]string{"gho_good": "repo, notifications, read:org"}
	s := &Session{Store: NewKeyringStore(), NewClient: fakeUser(t, tokens)}
	ctx := context.Background()

	_, id, err := s.Complete(ctx, "  gho_good\n")
	if err != nil || id.Login != "octocat" {
		t.Fatalf("Complete = %+v, %v", id, err)
	}
	if _, token, _ := s.Store.Load(); token != "gho_good" {
		t.Fatalf("stored token = %q (whitespace must be trimmed)", token)
	}

	if _, id, err = s.Restore(ctx); err != nil || id.Login != "octocat" {
		t.Fatalf("Restore = %+v, %v", id, err)
	}

	// Token revoked on GitHub: Restore surfaces ErrUnauthorized and clears the keyring.
	delete(tokens, "gho_good")
	if _, _, err := s.Restore(ctx); !errors.Is(err, gh.ErrUnauthorized) {
		t.Fatalf("Restore after revoke = %v, want ErrUnauthorized", err)
	}
	if _, _, err := s.Store.Load(); !errors.Is(err, ErrNoToken) {
		t.Fatalf("keyring after revoke = %v, want ErrNoToken", err)
	}
}

func TestSessionRejectsBadTokens(t *testing.T) {
	keyring.MockInit()
	tokens := map[string]string{"ghp_noorg": "repo", "ghp_ok": "repo, read:org"}
	s := &Session{Store: NewKeyringStore(), NewClient: fakeUser(t, tokens)}
	ctx := context.Background()

	var ms *MissingScopesError
	if _, _, err := s.CompletePAT(ctx, "ghp_noorg"); !errors.As(err, &ms) {
		t.Errorf("missing scope: got %v", err)
	}
	if _, _, err := s.CompletePAT(ctx, "github_pat_x"); !errors.Is(err, ErrFineGrainedPAT) {
		t.Errorf("fine-grained: got %v", err)
	}
	if _, _, err := s.CompletePAT(ctx, "ghp_revoked"); !errors.Is(err, gh.ErrUnauthorized) {
		t.Errorf("revoked: got %v", err)
	}
	if _, _, err := s.CompletePAT(ctx, "   "); err == nil {
		t.Error("empty token accepted")
	}
	if _, _, err := s.Store.Load(); !errors.Is(err, ErrNoToken) {
		t.Fatalf("nothing may be stored after failures, got %v", err)
	}

	if _, _, err := s.CompletePAT(ctx, "ghp_ok"); err != nil {
		t.Fatal(err)
	}
	if err := s.Logout(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Store.Load(); !errors.Is(err, ErrNoToken) {
		t.Fatalf("after Logout = %v, want ErrNoToken", err)
	}
}
