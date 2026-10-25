// Package avatar downloads and caches GitHub avatars on disk: toasts need a
// local image file (desktop-integration skill).
package avatar

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"prism/internal/store"
)

const (
	// MaxAge is how long a cached avatar is used before revalidating it.
	MaxAge = 24 * time.Hour
	// fetchTimeout keeps a slow avatar from delaying a toast for long.
	fetchTimeout = 5 * time.Second
	maxBytes     = 1 << 20
)

// Cache keeps avatars in Dir and their metadata in the store.
type Cache struct {
	Dir       string
	Store     *store.Store
	HTTP      *http.Client
	UserAgent string
	Now       func() time.Time
	Log       *slog.Logger
}

// New returns a cache in dir.
func New(dir string, st *store.Store, version string, log *slog.Logger) *Cache {
	return &Cache{
		Dir: dir, Store: st, HTTP: &http.Client{Timeout: fetchTimeout},
		UserAgent: "PRism/" + version, Now: time.Now, Log: log,
	}
}

var unsafeChars = regexp.MustCompile(`[^A-Za-z0-9_-]`)

// Path returns a local image for login, downloading or revalidating it when
// needed. On failure it falls back to a stale file, or "" when there is none.
func (c *Cache) Path(ctx context.Context, login, url string) string {
	if login == "" || url == "" {
		return ""
	}
	cached, ok, err := c.Store.Avatar(ctx, login)
	if err != nil {
		c.Log.Debug("avatar lookup", "login", login, "err", err)
	}
	if ok && fileExists(cached.Path) && c.Now().Sub(cached.FetchedAt) < MaxAge {
		return cached.Path
	}
	path, err := c.fetch(ctx, login, url, cached, ok && fileExists(cached.Path))
	if err != nil {
		c.Log.Debug("avatar fetch", "login", login, "err", err)
		if ok && fileExists(cached.Path) {
			return cached.Path
		}
		return ""
	}
	return path
}

func (c *Cache) fetch(ctx context.Context, login, url string, cached store.Avatar, haveFile bool) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", c.UserAgent)
	if haveFile && cached.ETag != "" {
		req.Header.Set("If-None-Match", cached.ETag)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	switch {
	case resp.StatusCode == http.StatusNotModified && haveFile:
		cached.FetchedAt = c.Now()
		return cached.Path, c.Store.PutAvatar(ctx, cached)
	case resp.StatusCode != http.StatusOK:
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	ext := ".png"
	if mt, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type")); mt == "image/jpeg" {
		ext = ".jpg"
	}
	if err := os.MkdirAll(c.Dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(c.Dir, unsafeChars.ReplaceAllString(login, "_")+ext)
	tmp, err := os.CreateTemp(c.Dir, ".avatar-*")
	if err != nil {
		return "", err
	}
	_, err = io.Copy(tmp, io.LimitReader(resp.Body, maxBytes))
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), path)
	}
	if err != nil {
		_ = os.Remove(tmp.Name())
		return "", fmt.Errorf("save avatar: %w", err)
	}
	if haveFile && cached.Path != path {
		_ = os.Remove(cached.Path) // format changed
	}
	return path, c.Store.PutAvatar(ctx, store.Avatar{Login: login, Path: path, ETag: resp.Header.Get("ETag"), FetchedAt: c.Now()})
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}
