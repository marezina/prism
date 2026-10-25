# PRism assets

All assets in this folder are original work made for PRism and released under
**CC0 1.0** (no attribution required). Safe to embed and redistribute.

## Icons (`icons/`)
| File | Use |
|---|---|
| `app/prism-app.svg` | Master app icon (installer, Start menu, Linux hicolor, About). Spectrum rays. |
| `app/prism-app-mono.svg` | Single-colour variant (where the spectrum would be too busy) |
| `app/prism-glyph.svg` | Glyph only, `currentColor` — UI header, empty state, toast header |
| `app/prism.ico` | Windows icon, 16–256 px |
| `png/app/prism-<size>.png` | 16, 24, 32, 48, 64, 128, 256, 512 — Linux hicolor + Wails build |
| `tray/tray-light.svg`, `tray-dark.svg` | Tray glyph for light / dark taskbars (simplified, thicker for 16 px) |
| `tray/tray-*-error.svg` | Offline / error state (dimmed glyph + orange dot) |
| `png/tray/<variant>-<16|20|24|32>.png` | Pre-rendered tray bitmaps; `internal/tray` draws the unread badge on top at runtime |
| `ui/*.svg` | 24 px stroke icons, `currentColor`, stroke 2 |

Event → UI icon: `ci.passed` check-circle · `ci.failed` x-circle · `my_pr.changes_requested`
changes-requested · `merge.conflict` conflict · `commits.after_my_review` commit ·
`my_pr.closed` merge · `my_pr.opened` pull-request · `review.requested` review-requested ·
`mention` mention · replies reply · comments comment. People events show the avatar;
the icon is used as a fallback when the avatar is missing.

Badge (runtime): circle `#C4321F` (dark taskbar `#D9452F`), white digits, weight 600,
diameter ≈ 60 % of icon size, top-right, `99+` cap.

## Sounds (`sounds/`)
44.1 kHz, 16-bit, mono WAV, peaks at −6/−7 dBFS, < 1 s, synthesized.

| File | Character | Default use |
|---|---|---|
| `soft-chime.wav` | two rising bell notes | default for people events (approve, comment, reply, review request) |
| `alert.wav` | two falling notes, still soft | CI failed, merge conflict |
| `ding.wav` | single bell | alternative default |
| `pop.wav` | tiny pop | minimal option |

Embed with `//go:embed sounds/*.wav` in `internal/sound`.
