// Package tray renders the tray icon, including the runtime unread badge.
package tray

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"strconv"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"

	"prism/assets"
)

// Theme is the taskbar theme the icon is drawn for.
type Theme int

const (
	ThemeLight Theme = iota // light taskbar → dark glyph
	ThemeDark               // dark taskbar → light glyph
)

// State is the tray icon state (desktop-integration skill §Tray).
type State int

const (
	StateIdle State = iota
	StateUnread
	StateError
)

// Sizes are the bitmap sizes PRism ships (Windows DPI steps).
var Sizes = []int{16, 20, 24, 32}

var (
	badgeLight = color.NRGBA{0xC4, 0x32, 0x1F, 0xFF}
	badgeDark  = color.NRGBA{0xD9, 0x45, 0x2F, 0xFF}
)

// MaxBadge is the largest count shown as digits; above it the badge reads "99+".
const MaxBadge = 99

type cacheKey struct {
	state State
	label string
	theme Theme
	size  int
}

var (
	cacheMu sync.Mutex
	cache   = map[cacheKey][]byte{}

	fontOnce sync.Once
	fontErr  error
	badgeFnt *opentype.Font
)

// Icon returns the PNG-encoded tray icon. count is only used for StateUnread;
// a count ≤ 0 renders the idle icon. Results are cached.
func Icon(state State, count int, theme Theme, size int) ([]byte, error) {
	label := ""
	if state == StateUnread {
		if count <= 0 {
			state = StateIdle
		} else {
			label = BadgeLabel(count)
		}
	}
	key := cacheKey{state, label, theme, size}

	cacheMu.Lock()
	defer cacheMu.Unlock()
	if b, ok := cache[key]; ok {
		return b, nil
	}
	img, err := render(key)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("encode tray icon: %w", err)
	}
	cache[key] = buf.Bytes()
	return cache[key], nil
}

// BadgeLabel formats an unread count for the badge.
func BadgeLabel(count int) string {
	if count > MaxBadge {
		return strconv.Itoa(MaxBadge) + "+"
	}
	return strconv.Itoa(count)
}

func render(k cacheKey) (*image.NRGBA, error) {
	variant := "light"
	if k.theme == ThemeDark {
		variant = "dark"
	}
	if k.state == StateError {
		variant += "-error"
	}
	base, err := loadBase(fmt.Sprintf("icons/png/tray/tray-%s-%d.png", variant, k.size))
	if err != nil {
		return nil, err
	}
	if k.label == "" {
		return base, nil
	}
	fill := badgeLight
	if k.theme == ThemeDark {
		fill = badgeDark
	}
	if err := drawBadge(base, k.label, fill); err != nil {
		return nil, err
	}
	return base, nil
}

func loadBase(name string) (*image.NRGBA, error) {
	f, err := assets.TrayPNG.Open(name)
	if err != nil {
		return nil, fmt.Errorf("open tray base %s: %w", name, err)
	}
	defer func() { _ = f.Close() }() // read-only embed file
	src, err := png.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decode tray base %s: %w", name, err)
	}
	dst := image.NewNRGBA(src.Bounds())
	draw.Draw(dst, dst.Bounds(), src, src.Bounds().Min, draw.Src)
	return dst, nil
}

// drawBadge paints a pill in the top-right corner (height ≈ 60 % of the icon),
// clears a 1 px gap around it so it separates from the glyph, and centres the
// label in white.
func drawBadge(img *image.NRGBA, label string, fill color.NRGBA) error {
	size := img.Bounds().Dx()
	h := math.Round(float64(size) * 0.6)

	face, err := badgeFace(h * 0.62 / 0.73) // Go Bold cap height ≈ 0.73 em
	if err != nil {
		return err
	}
	defer func() { _ = face.Close() }() // releases glyph caches only
	textW := fixedToFloat(font.MeasureString(face, label))
	w := math.Min(float64(size), math.Max(h, textW+h*0.5))
	x0 := float64(size) - w

	gap := 1.0
	forEachCoverage(img, x0-gap, -gap, w+2*gap, h+2*gap, func(x, y int, cov float64) {
		c := img.NRGBAAt(x, y)
		c.A = uint8(float64(c.A) * (1 - cov))
		img.SetNRGBA(x, y, c)
	})
	forEachCoverage(img, x0, 0, w, h, func(x, y int, cov float64) {
		img.SetNRGBA(x, y, over(img.NRGBAAt(x, y), fill, cov))
	})

	b, _ := font.BoundString(face, label)
	textH := fixedToFloat(b.Max.Y - b.Min.Y)
	d := font.Drawer{
		Dst:  img,
		Src:  image.White,
		Face: face,
		Dot: fixed.Point26_6{
			X: floatToFixed(x0 + (w-textW)/2),
			Y: floatToFixed((h+textH)/2) - b.Max.Y,
		},
	}
	d.DrawString(label)
	return nil
}

func badgeFace(px float64) (font.Face, error) {
	fontOnce.Do(func() {
		badgeFnt, fontErr = opentype.Parse(gobold.TTF)
	})
	if fontErr != nil {
		return nil, fmt.Errorf("parse badge font: %w", fontErr)
	}
	face, err := opentype.NewFace(badgeFnt, &opentype.FaceOptions{Size: px, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		return nil, fmt.Errorf("badge font face: %w", err)
	}
	return face, nil
}

// forEachCoverage calls fn for every pixel overlapped by the stadium (pill)
// shape at (x, y, w, h), with the anti-aliased coverage in (0, 1].
func forEachCoverage(img *image.NRGBA, x, y, w, h float64, fn func(px, py int, cov float64)) {
	const ss = 4 // supersampling per axis
	r := h / 2
	bounds := img.Bounds()
	for py := int(math.Floor(y)); py < int(math.Ceil(y+h)); py++ {
		for px := int(math.Floor(x)); px < int(math.Ceil(x+w)); px++ {
			if !(image.Point{px, py}).In(bounds) {
				continue
			}
			hits := 0
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					fx := float64(px) + (float64(sx)+0.5)/ss
					fy := float64(py) + (float64(sy)+0.5)/ss
					cx := math.Max(x+r, math.Min(x+w-r, fx))
					if math.Hypot(fx-cx, fy-(y+r)) <= r {
						hits++
					}
				}
			}
			if hits > 0 {
				fn(px, py, float64(hits)/ss/ss)
			}
		}
	}
}

// over composites src with coverage cov over dst (non-premultiplied).
func over(dst, src color.NRGBA, cov float64) color.NRGBA {
	sa := float64(src.A) / 255 * cov
	da := float64(dst.A) / 255
	oa := sa + da*(1-sa)
	if oa == 0 {
		return color.NRGBA{}
	}
	mix := func(s, d uint8) uint8 {
		return uint8(math.Round((float64(s)*sa + float64(d)*da*(1-sa)) / oa))
	}
	return color.NRGBA{mix(src.R, dst.R), mix(src.G, dst.G), mix(src.B, dst.B), uint8(math.Round(oa * 255))}
}

func fixedToFloat(v fixed.Int26_6) float64 { return float64(v) / 64 }
func floatToFixed(v float64) fixed.Int26_6 { return fixed.Int26_6(math.Round(v * 64)) }
