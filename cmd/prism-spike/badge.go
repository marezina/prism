package main

import (
	"bytes"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"

	"prism/internal/tray"
)

// runBadge writes a contact sheet of every tray icon variant: rows are
// (theme, size), columns are idle, 1, 7, 42, 99, 100+, error. Each icon is
// upscaled 4× with nearest-neighbour so individual pixels are visible.
func runBadge(args []string) error {
	fs := flag.NewFlagSet("badge", flag.ExitOnError)
	out := fs.String("out", "spike-out/badges.png", "output PNG")
	_ = fs.Parse(args)

	type col struct {
		state tray.State
		count int
	}
	cols := []col{{tray.StateIdle, 0}, {tray.StateUnread, 1}, {tray.StateUnread, 7},
		{tray.StateUnread, 42}, {tray.StateUnread, 99}, {tray.StateUnread, 100}, {tray.StateError, 0}}
	const scale, cell = 4, 32*4 + 16

	themes := []struct {
		theme tray.Theme
		bg    color.Color
	}{{tray.ThemeLight, color.NRGBA{0xF0, 0xF0, 0xF0, 0xFF}}, {tray.ThemeDark, color.NRGBA{0x20, 0x20, 0x20, 0xFF}}}

	sheet := image.NewNRGBA(image.Rect(0, 0, len(cols)*cell, len(themes)*len(tray.Sizes)*cell))
	for ti, th := range themes {
		for si, size := range tray.Sizes {
			row := ti*len(tray.Sizes) + si
			draw.Draw(sheet, image.Rect(0, row*cell, sheet.Bounds().Dx(), (row+1)*cell), image.NewUniform(th.bg), image.Point{}, draw.Src)
			for ci, c := range cols {
				b, err := tray.Icon(c.state, c.count, th.theme, size)
				if err != nil {
					return err
				}
				icon, err := png.Decode(bytes.NewReader(b))
				if err != nil {
					return fmt.Errorf("decode icon: %w", err)
				}
				ox, oy := ci*cell+8, row*cell+8
				for y := 0; y < size*scale; y++ {
					for x := 0; x < size*scale; x++ {
						src := icon.At(x/scale, y/scale)
						draw.Draw(sheet, image.Rect(ox+x, oy+y, ox+x+1, oy+y+1), image.NewUniform(src), image.Point{}, draw.Over)
					}
				}
			}
		}
	}

	if err := os.MkdirAll("spike-out", 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}
	f, err := os.Create(*out)
	if err != nil {
		return fmt.Errorf("create %s: %w", *out, err)
	}
	if err := png.Encode(f, sheet); err != nil {
		_ = f.Close()
		return fmt.Errorf("encode sheet: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close %s: %w", *out, err)
	}
	fmt.Println("wrote", *out)
	return nil
}
