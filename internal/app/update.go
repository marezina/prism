package app

import (
	"context"
	"errors"
	"time"

	"prism/internal/gh"
	"prism/internal/notify"
	"prism/internal/update"
)

// UpdateInfo describes a newer PRism release.
type UpdateInfo struct {
	Version string `json:"version"`
	URL     string `json:"url"`
}

const (
	updateEvery         = 24 * time.Hour
	metaUpdateNotified  = "update_notified" // newest version a toast was shown for
	firstUpdateCheckLag = time.Minute       // let the first poll go first
)

// checkUpdates looks for a newer release on start and daily while signed in
// (release skill §Update check). Dev builds and builds without a release repo
// never check.
func (c *Core) checkUpdates(ctx context.Context, client *gh.Client) {
	if c.opts.UpdateRepo == "" || c.opts.Version == "dev" {
		return
	}
	wait := firstUpdateCheckLag
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		wait = updateEvery
		if err := c.checkUpdateOnce(ctx, client); err != nil && !errors.Is(err, context.Canceled) {
			c.log.Debug("update check", "err", err)
		}
	}
}

func (c *Core) checkUpdateOnce(ctx context.Context, client *gh.Client) error {
	rel, err := update.Latest(ctx, client, c.opts.UpdateRepo)
	if err != nil {
		return err
	}
	if !update.Newer(c.opts.Version, rel.Tag) {
		return nil
	}
	info := UpdateInfo{Version: rel.Tag, URL: rel.URL}
	c.mu.Lock()
	c.update = info
	c.mu.Unlock()
	c.log.Info("update available", "version", rel.Tag)
	if h := c.opts.Hooks.UpdateAvailable; h != nil {
		h(info)
	}

	notified, _ := c.opts.Store.Meta(ctx, metaUpdateNotified)
	if notified == rel.Tag || c.opts.Dispatcher == nil || c.opts.Dispatcher.Notifier == nil {
		return nil
	}
	err = c.opts.Dispatcher.Notifier.Show(ctx, notify.Toast{
		ID:      "prism-update-" + rel.Tag,
		Title:   "PRism " + rel.Tag + " is available",
		Body:    "Open the release page to download it.",
		Actions: []notify.Action{{ID: notify.ActionOpen, Title: "Open release"}},
		Data:    map[string]string{notify.DataURL: rel.URL},
	})
	if err != nil {
		return err
	}
	return c.opts.Store.SetMeta(ctx, metaUpdateNotified, rel.Tag)
}

// Update returns the newer release found by the last check, if any.
func (c *Core) Update() UpdateInfo {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.update
}
