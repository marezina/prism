---
name: prism-ui
description: PRism's visual design system and frontend conventions — Notion-like minimal style, Inter font, color/spacing tokens, light and dark theme, notification row, PR group, tabs, toggles, login and settings screens, accessibility and Wails bindings usage. Use this skill for any change under frontend/, any new screen or component, and whenever colors, fonts, spacing, icons or layout are involved.
---

# PRism UI

Look: Notion-like. Calm, white (or near-black) surfaces, warm greys, one blue
accent, generous whitespace, thin dividers, no shadows inside the window, no
gradients, no emoji. Content first.

## Stack
- Svelte 5 + TypeScript + Vite (Wails v3 template). No UI framework.
- Font: Inter via `@fontsource-variable/inter`, bundled (works offline).
  Monospace (device code): `JetBrains Mono` bundled, fallback Cascadia Mono, Consolas.
- Icons: use the SVGs in `assets/icons/ui/` (24×24, `currentColor`, stroke 2, round
  caps/joins) wrapped as `src/lib/icons/*.svelte`. Brand glyph: `assets/icons/app/prism-glyph.svg`.
  Event → icon mapping is in `assets/README.md`. Don't add icons from other sets.
- Tokens: `references/tokens.css` → `src/app.css`. Use only tokens; never raw hex
  in components.

## Theme
`data-theme` on `<html>`: `light` | `dark`, resolved from settings (`system` follows
`prefers-color-scheme`). Both themes defined in tokens.css.

## Type scale
| Use | Size / line | Weight |
|---|---|---|
| Window title | 18/24 | 600, letter-spacing -0.01em |
| Section title (settings) | 20/28 | 600 |
| Body / row title | 14/20 | 400, actor name 600 |
| Secondary / excerpt | 13/19 | 400 |
| Meta (time, repo, labels) | 12/16 | 400–500 |

## Components (see the canvas mockups)
- **PR group**: repo (12, muted) + label pill (`Your PR` grey / `Review` blue),
  title line `#482 Title` (number muted 500, title 600, ellipsis), "Mark PR read"
  outline button right, bell-off icon button (Mute PR) before it while the PR is
  watched. Under the title an optional status line (12 px, gap 4/12): Draft, N approvals
  (green), Changes requested (red), CI passing (green) / failing (red) / running,
  Conflict (orange); 12 px icon + text, tint as text color only. Divider below group.
- **Notification row**: 28 px avatar circle (or 28 px rounded-6 icon tile for system
  events with tinted bg per type), text `**Actor** action`, optional excerpt box
  (`--surface-2`, radius 6, 13 px), right side relative time + 7 px unread dot
  (`--accent-dot`). Whole row is a `<button>`; hover `--hover`.
- **Tabs**: text buttons, active = text color + 2 px bottom border.
- **Toggle**: 30×18 track, 14 px knob, on = `--accent`, `role="switch"` + `aria-checked`.
- **Buttons**: primary (accent bg, white text, radius 8, height 40–44), secondary
  (outline `--border-strong`), ghost (text only, `--text-2`).
- **Avatar fallback**: initials on a pastel pair from tokens (`--av-1..5`), chosen by
  hash(login) so a person always has the same color.

## Event visuals
| Event | Leading | Tint |
|---|---|---|
| approved | avatar | — (action text "approved your PR") |
| changes_requested | avatar | — |
| comment / reply / mention | avatar + excerpt | — |
| review.requested | avatar | label pill Review |
| ci.passed | icon check-circle | green |
| ci.failed | icon x-circle | red |
| merge.conflict | icon triangle | orange |
| commits.after_my_review | icon commit | grey |
| my_pr.closed | icon merge | purple |
| my_pr.opened | icon pr | grey |

## Behaviour
- Popup window ~440×640, opens anchored to tray, closes on blur/Esc.
- Motion: Svelte `slide`/`fade`/`fly` with `motion(ms)` from `src/lib/motion.ts`
  (120–180 ms, cubicOut, 0 under reduced motion). Rows and groups animate when they
  arrive or leave within a tab; the list is `{#key tab}` so tab switches are instant.
- Optimistic UI for mark-read; on error, restore and show inline error.
- Relative times: `now`, `5m`, `2h`, `3d`; refresh every 30 s.
- Empty state: small prism glyph + "You're all caught up".
- Offline: thin banner at top `Offline · retrying in 40s`.

## Accessibility (required)
- Every clickable thing is a `<button>` or `<a>`; icon-only buttons have `aria-label`.
- Text contrast ≥ 4.5:1 in both themes (tokens are chosen to pass — don't lighten them).
- Visible focus ring: `outline: 2px solid var(--focus); outline-offset: 2px`.
- Full keyboard use: ↑/↓, Enter, E, Esc; tabs reachable with Tab.

## Talking to Go
Use generated Wails bindings only (`import { Unread, MarkRead } from '../bindings/...'`).
Subscribe to `unread:changed`, `auth:changed`, `sync:status` events; never poll from JS.
