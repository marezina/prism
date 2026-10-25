package update

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"prism/internal/gh"
)

func TestNewer(t *testing.T) {
	tests := []struct {
		current, tag string
		want         bool
	}{
		{"v0.1.0", "v0.2.0", true},
		{"0.1.0", "v0.1.1", true},
		{"v0.1.0-beta.1", "v0.1.0-beta.2", true},
		{"v0.1.0-beta.2", "v0.1.0", true},
		{"v0.1.0", "v0.1.0-beta.3", false},
		{"v0.2.0", "v0.1.9", false},
		{"v0.1.0", "v0.1.0", false},
		{"dev", "v9.9.9", false},
		{"v0.1.0", "nightly", false},
	}
	for _, tt := range tests {
		if got := Newer(tt.current, tt.tag); got != tt.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", tt.current, tt.tag, got, tt.want)
		}
	}
}

func TestLatest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/acme/prism/releases" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"tag_name":"v0.3.0","draft":true,"html_url":"https://x/draft"},
			{"tag_name":"v0.2.0-beta.1","html_url":"https://x/beta"},
			{"tag_name":"v0.1.0","html_url":"https://x/v0.1.0"}]`))
	}))
	defer srv.Close()
	c, err := gh.New(gh.Options{Version: "test", Logger: slog.New(slog.DiscardHandler),
		Endpoints: gh.Endpoints{Web: srv.URL, API: srv.URL + "/", GraphQL: srv.URL + "/graphql"}})
	if err != nil {
		t.Fatal(err)
	}
	rel, err := Latest(context.Background(), c, "acme/prism")
	if err != nil || rel.Tag != "v0.2.0-beta.1" || rel.URL != "https://x/beta" {
		t.Fatalf("Latest = %+v, %v (drafts skipped, pre-releases count)", rel, err)
	}
	if _, err := Latest(context.Background(), c, ""); !errors.Is(err, ErrDisabled) {
		t.Errorf("empty repo: %v", err)
	}
	if _, err := Latest(context.Background(), c, "acme/missing"); err == nil {
		t.Error("want error for a missing repo")
	}
}
