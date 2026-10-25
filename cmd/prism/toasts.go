package main

import (
	"context"
	"log/slog"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/services/notifications"

	"prism/internal/notify"
)

// wailsToasts implements notify.Notifier with the Wails notifications
// service (ADR 0002). Each distinct set of buttons is one category.
type wailsToasts struct {
	svc *notifications.NotificationService
	log *slog.Logger
}

// categories are the button sets notify.Dispatcher produces.
var categories = [][]notify.Action{
	{{ID: notify.ActionOpen, Title: "Open PR"}, {ID: notify.ActionRead, Title: "Mark as read"}},
	{{ID: notify.ActionOpen, Title: "Open PR"}},
	{{ID: notify.ActionShowAll, Title: "Show all"}, {ID: notify.ActionRead, Title: "Mark as read"}},
	{{ID: notify.ActionShowAll, Title: "Show all"}},
}

func categoryID(actions []notify.Action) string {
	ids := make([]string, len(actions))
	for i, a := range actions {
		ids[i] = a.ID
	}
	return "prism." + strings.Join(ids, "+")
}

func (w *wailsToasts) registerCategories() {
	for _, actions := range categories {
		cat := notifications.NotificationCategory{ID: categoryID(actions)}
		for _, a := range actions {
			cat.Actions = append(cat.Actions, notifications.NotificationAction{ID: a.ID, Title: a.Title})
		}
		if err := w.svc.RegisterNotificationCategory(cat); err != nil {
			w.log.Warn("register toast category", "id", cat.ID, "err", err)
		}
	}
}

func (w *wailsToasts) Show(_ context.Context, t notify.Toast) error {
	data := make(map[string]any, len(t.Data))
	for k, v := range t.Data {
		data[k] = v
	}
	opts := notifications.NotificationOptions{
		ID:         t.ID,
		Title:      t.Title,
		Body:       t.Body,
		CategoryID: categoryID(t.Actions),
		Data:       data,
		Sound:      &notifications.NotificationSound{Silent: true}, // PRism plays its own sound
	}
	if t.ImagePath != "" {
		opts.Attachments = []notifications.NotificationAttachment{{ID: "image", Path: t.ImagePath, Type: "image/png"}}
	}
	return w.svc.SendNotificationWithActions(opts)
}
