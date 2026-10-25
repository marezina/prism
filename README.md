# PRism

GitHub pull-request notifications in your system tray, for Windows 11 and Linux:
approvals, change requests, comments, replies to you, mentions, review requests,
CI results and merge conflicts — as desktop toasts and a small popup.

PRism only **reads** from GitHub and keeps everything on your computer. No telemetry.

## Install

Download the latest release from this repository's **Releases** page and check the
file against `SHA256SUMS.txt`.

### Windows 11
1. Run `PRism-<version>-windows-amd64-setup.exe`. It installs for your user only
   (no admin rights) into `%LOCALAPPDATA%\Programs\PRism`, adds a Start-menu entry and,
   if you keep the box ticked, starts PRism when you sign in.
2. Beta builds may be unsigned: SmartScreen shows "Windows protected your PC" →
   **More info** → **Run anyway**.

### Linux
- **Debian / Ubuntu 24.04+**: `sudo apt install ./prism_<version>_amd64.deb`
- **Any distro**: `chmod +x PRism-<version>-x86_64.AppImage && ./PRism-<version>-x86_64.AppImage`

Requirements:
- A tray that supports StatusNotifierItem. KDE Plasma works out of the box; on
  **GNOME install the "AppIndicator and KStatusNotifierItem Support" extension**
  (Ubuntu ships it enabled).
- A Secret Service for the token: gnome-keyring (GNOME, most desktops) or KWallet.
- GTK 4 and WebKitGTK 6.0 (Ubuntu 24.04+, Debian 13, Fedora 40+). Ubuntu 22.04 is
  not supported (ADR 0001).

## First run
1. Click **Sign in with GitHub** and enter the code in the browser — or use
   **a personal access token (classic)** with the scopes `repo`, `notifications`
   and `read:org`. Fine-grained tokens can't read notifications.
2. Pick the organizations to watch. If an organization uses SAML SSO, click
   **Authorize** next to it.
3. PRism goes to the tray. The first sync is silent; new activity notifies you from
   then on. Click the tray icon for the list, right-click for the menu.

Data lives in `%APPDATA%\PRism` / `~/.config/PRism` (database, avatars, logs).
**Settings → Advanced → Reset local data** clears it; **Sign out** forgets the token.

## Feedback
Open an issue with the **Beta feedback** template. Include the version
(Settings → Advanced) and, for notification problems, a link to the PR.

## Development
See `CLAUDE.md` (commands), `docs/SPEC.md`, `docs/ARCHITECTURE.md`, `docs/PLAN.md`
and the ADRs in `docs/adr/`.

- `go test ./...` — unit and golden tests
- `wails3 dev` — run with hot reload; `wails3 build` — local build (version `dev`)
- `PRISM_VERSION=v0.1.0 wails3 task linux:package` — AppImage + .deb in `bin/`
- Releases: push a tag `vX.Y.Z` (or `vX.Y.Z-beta.N`); `.github/workflows/release.yml`
  builds, signs (when the certificate secret is set) and publishes. Set the repository
  variable `PRISM_CLIENT_ID` to the org OAuth App's client ID first.

Assets in `assets/` are CC0 (see `assets/README.md`).
