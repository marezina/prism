# ADR 0001 — Wails v3 version and Linux toolkit

- Status: proposed (confirm after Spike A/B pass on Windows 11 and Linux)
- Date: 2026-10-24

## Context
PRism is a Wails v3 app. Wails v3 has left alpha: at the start of Phase 0 the
newest tags are `v3.0.0-beta.29` … `beta.34`, released almost daily. CLAUDE.md
requires a pinned version and deliberate upgrades.

Since `v3.0.0-alpha.93` Wails builds Linux against **GTK4 + WebKitGTK 6.0** by
default; GTK3 + WebKit2GTK 4.1 is kept as a legacy path behind `-tags gtk3`.
The original CI/release docs assumed GTK3 on `ubuntu-22.04`, where WebKitGTK 6.0
is not available.

## Options considered
1. Latest beta (`beta.34`, 4 days old) — newest fixes, but no time to settle.
2. `v3.0.0-beta.28` (2026-10-05, ~3 weeks old) — recent beta with systray v2
   (attached window, `HideOnFocusLost`, `HideOnEscape`) and the notifications
   service (Windows: go-toast v2 COM activator; Linux: D-Bus with actions).
3. Linux toolkit: GTK4 (default) vs GTK3 (`-tags gtk3`, legacy).

## Decision
- Pin `github.com/wailsapp/wails/v3 v3.0.0-beta.28` in `go.mod`, and the CLI with
  `go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.28`.
- Use the default **GTK4 / WebKitGTK 6.0** build. No `gtk3` tag.
- Linux CI and release builds move to `ubuntu-24.04`.
  Build deps: `build-essential pkg-config libgtk-4-dev libwebkitgtk-6.0-dev` (no audio
  headers, see ADR 0003).

## Consequences
- Minimum Linux runtime: distros with WebKitGTK 6.0 and glibc ≥ 2.39 for binaries
  built on 24.04 (Ubuntu 24.04+, Debian 13, Fedora 40+). Colleagues on Ubuntu
  22.04 are not supported; if that becomes a problem, switch to `-tags gtk3` and
  build on 22.04 (one ADR, no code changes outside build config).
- The `release` skill's "ubuntu-22.04 for older glibc" note is superseded by this ADR.
- Upgrades: one Wails bump per PR, with a new ADR or an amendment here.
