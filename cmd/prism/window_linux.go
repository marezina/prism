package main

/*
#cgo linux pkg-config: gtk4
#include <gtk/gtk.h>

// Wails beta.28 leaves BackgroundTypeTransparent unimplemented on GTK4: the
// WebView is transparent but the GtkWindow keeps its themed background. This
// clears it so the UI's rounded, bordered frame shows (ADR 0002).
static void prism_make_transparent(void *window) {
	static gboolean installed = FALSE;
	if (!installed) {
		GtkCssProvider *provider = gtk_css_provider_new();
		gtk_css_provider_load_from_string(provider,
			"window.prism-transparent, window.prism-transparent > * "
			"{ background: transparent; box-shadow: none; }");
		gtk_style_context_add_provider_for_display(gdk_display_get_default(),
			GTK_STYLE_PROVIDER(provider), GTK_STYLE_PROVIDER_PRIORITY_APPLICATION);
		g_object_unref(provider);
		installed = TRUE;
	}
	gtk_widget_add_css_class(GTK_WIDGET(window), "prism-transparent");
}
*/
import "C"

import (
	"github.com/wailsapp/wails/v3/pkg/application"
)

// makeTransparent clears the GTK window background (call after the app started).
func makeTransparent(w *application.WebviewWindow) {
	application.InvokeSync(func() {
		if p := w.NativeWindow(); p != nil {
			C.prism_make_transparent(p)
		}
	})
}
