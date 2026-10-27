---
name: release
description: Building, versioning, packaging and distributing PRism — CI workflows, Wails v3 build tasks, Windows NSIS installer with WebView2 and AppUserModelID shortcut, code signing, Linux AppImage and .deb, GitHub Releases in the internal repo, checksums and the in-app update check. Use this skill for anything in .github/workflows, build/, installer scripts, version bumps, or when preparing a release or beta for colleagues.
---

# Release

## Versioning
- SemVer tags `vMAJOR.MINOR.PATCH`; pre-releases `v0.x.y-beta.N` until 1.0.
- Version injected via `-ldflags "-X main.version=$TAG"`; shown in Settings → Advanced
  and in the User-Agent. The Taskfile reads it from `PRISM_VERSION` (CI sets the tag);
  without it builds are `dev`, which never registers autostart or checks for updates.
- Build-time settings (root `Taskfile.yml` → ldflags of production builds):
  `PRISM_VERSION`, `PRISM_CLIENT_ID` (→ `auth.ClientID`, repo variable),
  `PRISM_UPDATE_REPO` (→ `update.Repo`, CI uses `github.repository`).
- CHANGELOG.md, Keep-a-Changelog format; every user-visible PR adds a line.

## CI
- `ci.yml`: on PR/push — lint, `go test ./...` on ubuntu + windows, frontend
  `npm ci && npm run check && npm run build`.
- `release.yml`: on tag `v*` — build Windows (windows-latest) and Linux (ubuntu-24.04,
  GTK4/WebKitGTK 6.0 — see ADR 0001), package, sign, upload to GitHub Release with
  `SHA256SUMS.txt`. Tags with a `-` (`-beta.N`) publish as pre-releases; notes come
  from the matching `CHANGELOG.md` section. Artifacts:
  `PRism-<v>-windows-setup.exe` (one NSIS installer carrying amd64 + arm64 exes; Go
  cross-compiles arm64 with CGO off on windows-latest, CI also tests on windows-11-arm), `PRism-<v>-x86_64.AppImage`, `prism_<v>_amd64.deb`.
- Signing secrets (optional): `WINDOWS_CERT_PFX_BASE64`, `WINDOWS_CERT_PASSWORD`; the
  exe is signed before NSIS packs it, then the installer is signed.
- Linux build deps: `build-essential pkg-config libgtk-4-dev libwebkitgtk-6.0-dev` (re-verify on every Wails bump).

## Windows
- `build/windows/nsis/project.nsi` (customized, done) —
  - check/install WebView2 Runtime (Evergreen bootstrapper),
  - create a Start-menu shortcut (toasts get their AppUserModelID "PRism" and COM
    activator from the app at runtime, ADR 0002; the uninstaller removes both),
  - optional "Start PRism when I sign in" checkbox,
  - per-user install (no admin) into `%LOCALAPPDATA%\Programs\PRism`.
- Code signing: Authenticode with the company certificate (or Azure Trusted Signing).
  Unsigned builds trigger SmartScreen — acceptable only for internal beta.
- Alternative later: MSIX (package identity simplifies toasts) or Intune/winget distribution.

## Linux
- AppImage (primary, any distro) + `.deb` (Ubuntu/Debian).
- Desktop file `prism.desktop` + icons 16–512 px in hicolor theme.
- Document: GNOME needs AppIndicator extension; Secret Service needed for keyring.

## Update check
- Daily + on start: `GET /repos/<org>/<repo>/releases/latest` with the user's token
  (works for the private internal repo thanks to `repo` scope).
- If newer: tray menu item + one toast "PRism x.y.z is available" → opens release page.
- Auto-download/install is a later feature; when added, verify SHA256 before running.

## Release checklist
- [ ] CHANGELOG updated, version tag created from main
- [ ] CI green, release workflow artifacts present for both OSes
- [ ] Install fresh on clean Windows 11 VM and Ubuntu VM; login; receive a test event
- [ ] Upgrade over previous version keeps login, settings and unread items
- [ ] Release notes posted in the team channel
