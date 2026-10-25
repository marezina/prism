package app

import (
	"context"
	"errors"

	"prism/internal/notify"
	"prism/internal/store"
)

// Unread returns every unread notification, newest first.
func (c *Core) Unread(ctx context.Context) ([]Notification, error) {
	list, err := c.opts.Store.Unread(ctx)
	if err != nil {
		return nil, err
	}
	states, err := c.opts.Store.PRStates(ctx)
	if err != nil {
		return nil, err
	}
	mine := map[string]bool{}
	for id, s := range states {
		mine[id] = s.Role == store.RoleMine
	}
	out := make([]Notification, 0, len(list))
	for _, n := range list {
		out = append(out, c.toDTO(n, mine))
	}
	return out, nil
}

// MarkRead deletes notifications (SPEC D2).
func (c *Core) MarkRead(ctx context.Context, ids []int64) error {
	_, err := c.opts.Store.MarkRead(ctx, ids...)
	c.notifyUnread(ctx)
	return err
}

// MarkPRRead deletes every notification of one PR.
func (c *Core) MarkPRRead(ctx context.Context, prID string) error {
	_, err := c.opts.Store.MarkPRRead(ctx, prID)
	c.notifyUnread(ctx)
	return err
}

// MarkAllRead deletes every notification.
func (c *Core) MarkAllRead(ctx context.Context) error {
	_, err := c.opts.Store.MarkAllRead(ctx)
	c.notifyUnread(ctx)
	return err
}

// Open opens a notification's deep link in the browser and marks it read.
func (c *Core) Open(ctx context.Context, id int64) error {
	n, err := c.opts.Store.Get(ctx, id)
	if err != nil {
		return err
	}
	if err := c.openURL(n.URL); err != nil {
		return err
	}
	return c.MarkRead(ctx, []int64{id})
}

// OpenURL opens any link (PR page, SSO authorize page) in the browser.
func (c *Core) OpenURL(url string) error { return c.openURL(url) }

func (c *Core) openURL(url string) error {
	if h := c.opts.Hooks.OpenURL; h != nil {
		return h(url)
	}
	return errors.New("no browser hook")
}

// HandleToastAction routes a toast button or body click (SPEC §6):
// Open PR → open the link and mark read; Mark as read; Show all → window.
func (c *Core) HandleToastAction(ctx context.Context, action string, data map[string]string) error {
	ids := notify.IDs(data)
	switch action {
	case notify.ActionOpen, notify.ActionDefault:
		if url := data[notify.DataURL]; url != "" {
			if err := c.openURL(url); err != nil {
				return err
			}
			return c.MarkRead(ctx, ids)
		}
		c.showWindow() // summary toast body
	case notify.ActionRead:
		return c.MarkRead(ctx, ids)
	case notify.ActionShowAll:
		c.showWindow()
	}
	return nil
}

func (c *Core) showWindow() {
	if h := c.opts.Hooks.ShowWindow; h != nil {
		h()
	}
}
