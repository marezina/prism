package avatar

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"prism/internal/store"
)

func TestCache(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, err := store.Open(ctx, filepath.Join(dir, "prism.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()

	hits, status := 0, http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.Header.Get("User-Agent") != "PRism/test" {
			t.Errorf("UA = %q", r.Header.Get("User-Agent"))
		}
		if status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		if r.Header.Get("If-None-Match") == `"v1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("ETag", `"v1"`)
		_, _ = w.Write([]byte("PNGDATA"))
	}))
	defer srv.Close()

	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	c := New(filepath.Join(dir, "avatars"), st, "test", slog.New(slog.DiscardHandler))
	c.Now = func() time.Time { return now }

	p := c.Path(ctx, "dependabot[bot]", srv.URL+"/u/1")
	if filepath.Base(p) != "dependabot_bot_.png" {
		t.Fatalf("path = %q", p)
	}
	if b, _ := os.ReadFile(p); string(b) != "PNGDATA" {
		t.Errorf("file = %q", b)
	}

	if c.Path(ctx, "dependabot[bot]", srv.URL+"/u/1") != p || hits != 1 {
		t.Errorf("fresh cache must not refetch (hits %d)", hits)
	}

	now = now.Add(MaxAge + time.Minute)
	if c.Path(ctx, "dependabot[bot]", srv.URL+"/u/1") != p || hits != 2 {
		t.Errorf("stale cache must revalidate with ETag (hits %d)", hits)
	}
	if a, _, _ := st.Avatar(ctx, "dependabot[bot]"); !a.FetchedAt.Equal(now) {
		t.Errorf("304 must refresh fetched_at: %v", a.FetchedAt)
	}

	now = now.Add(MaxAge + time.Minute)
	status = http.StatusInternalServerError
	if c.Path(ctx, "dependabot[bot]", srv.URL+"/u/1") != p {
		t.Error("failed refresh must fall back to the stale file")
	}
	if c.Path(ctx, "nobody", srv.URL+"/u/2") != "" {
		t.Error("failed first fetch must give no path")
	}
	if c.Path(ctx, "", srv.URL) != "" || c.Path(ctx, "x", "") != "" {
		t.Error("empty login or url must give no path")
	}
}
