# ADR 0002 — Tray, popup window and toasts

- Status: proposed — Linux (KDE Plasma 6, Wayland) verified; Windows 11 and GNOME pending
- Date: 2026-10-24

## Context
Spike A (tray + popup attached to tray, hide on blur) and Spike B (toast with
image, two action buttons, click → Go callback) from PLAN Phase 0. PRism is an
unpackaged app on Windows, so toasts need an AppUserModelID and a COM activator.

## Options considered
1. Wails v3 built-ins: `app.SystemTray` (systray v2: `AttachWindow`,
   `HideOnFocusLost`, `HideOnEscape`) and `pkg/services/notifications`.
2. Separate libraries: `go-toast` (Windows) and `godbus` (Linux) behind our own
   interface, plus a third-party systray.

## Decision
Option 1, all from `github.com/wailsapp/wails/v3 v3.0.0-beta.28`:
- Tray: `app.SystemTray.New()`, `SetIcon` / `SetDarkModeIcon` with PNGs from
  `internal/tray`, `SetLabel("PRism")`, `AttachWindow(popup)` + `SetMenu(menu)`.
- Popup: frameless 440×640, `Hidden`, `AlwaysOnTop`, `HideOnFocusLost`,
  `HideOnEscape`, `Windows.HiddenOnTaskbar`; the `WindowClosing` hook hides instead
  of closing.
- Toasts: `notifications.New()` as a Wails service; one category `prism.event`
  with actions `open` ("Open PR") and `read` ("Mark as read"); `Sound.Silent`;
  image via `Attachments[0].Path` (local file); routing data in `Data`
  (comes back as `Response.UserInfo`). `OnNotificationResponse` receives
  `ActionIdentifier` (`open`, `read`, or `default` for a click on the toast body).
- Windows: Wails uses `Options.Name` as the AUMID and registers its own COM
  activator (`HKCU\Software\Classes\CLSID\<guid>\LocalServer32` → the exe) via
  go-toast v2. No Start-menu shortcut is needed for toasts in dev builds.

Platform code stays behind the `Notifier` interface from the desktop-integration
skill; only `internal/notify/*_{windows,linux}.go` import the Wails service.

## Verified on Linux (Debian 13, KDE Plasma, Wayland)
- Tray registers as a StatusNotifierItem; title is "PRism" (without `SetLabel`
  it reads "Wails").
- `Notify` carries app name `PRism`, actions `default/open/read`, `image-path`,
  `suppress-sound=true`, and our ID/data as hints.
- An `ActionInvoked(id, "read")` signal reaches `OnNotificationResponse` with
  the right toast ID and `UserInfo.url`.
- Manual check by the author (2026-10-24): left click toggles the popup, right
  click opens the menu, popup hides on blur and Esc, toast shows the image and
  both real button clicks work, sounds are audible. Everything as expected.

## Workarounds
- Rounded, bordered popup: the window is frameless with `BackgroundTypeTransparent` and
  the UI draws the frame (`#app` border + `--radius-lg`). Wails beta.28's `setTransparent`
  is a no-op on GTK4, so `cmd/prism/window_linux.go` adds a CSS class that clears the
  GtkWindow background. Re-check on every Wails bump; remove once Wails implements it.

## Open / to verify
- Windows 11: toast with image, both buttons with the window hidden, activation
  when PRism was not running (does the COM activator relaunch the exe and deliver
  the action?), AUMID display name. If the AUMID must be `PRism.Desktop`
  (desktop-integration skill), either set `Options.Name` to it or keep "PRism"
  and update the skill: decide after the Windows run.
- Popup placement: acceptable on KDE Wayland; check X11 and GNOME (AppIndicator
  extension) too.
- Linux tray tooltip: the SNI `ToolTip` property uses the label, so a per-count
  tooltip ("3 unread") is not shown on Linux. The badge carries the count.
- `SetIcon` takes one bitmap; we pass 32 px. Check sharpness at 100 %/125 %/150 %
  scaling on Windows.
