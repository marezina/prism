// Package assets embeds PRism's icons and sounds so product code can use them
// without touching the file system. See README.md for what each file is for.
package assets

import "embed"

// TrayPNG holds the pre-rendered tray bitmaps (png/tray/<variant>-<size>.png).
//
//go:embed icons/png/tray/*.png
var TrayPNG embed.FS

// AppPNG holds the app icon at 16–512 px (icons/png/app/prism-<size>.png).
//
//go:embed icons/png/app/*.png
var AppPNG embed.FS

// Sounds holds the notification sounds (sounds/<name>.wav).
//
//go:embed sounds/*.wav
var Sounds embed.FS
