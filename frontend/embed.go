// Package frontend embeds the built UI (frontend/dist) for the Wails app.
// The app lives in cmd/prism, and go:embed cannot reach parent directories,
// so the embed sits next to the files it serves.
package frontend

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// Dist returns the built frontend rooted at dist/.
func Dist() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err) // the embed pattern guarantees dist/ exists
	}
	return sub
}
