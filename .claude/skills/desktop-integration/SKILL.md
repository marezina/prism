---
name: desktop-integration
description: Platform integration for PRism on Windows 11 and Linux — system tray and badge icon, popup window placement, toast notifications with action buttons and activation callbacks, AppUserModelID, Do Not Disturb, sound playback, keyring, autostart, single instance, sleep/wake. Use this skill for any work in internal/notify, internal/tray, internal/sound, autostart or OS-specific files (*_windows.go, *_linux.go), and whenever a feature behaves differently on Windows and Linux.
---

# Desktop integration

Rule: every OS-specific capability sits behind a small Go interface with
`_windows.go` / `_linux.go` implementations and a `fake` for tests. Product code
never imports platform libraries directly.

```go
type Notifier interface {
    Show(ctx context.Context, t Toast) error   // Toast{ID, Title, Body, ImagePath, Actions}
    OnAction(func(toastID, actionID string))
}
type Player interface { Play(name string, volume float64) error }
type Autostart interface { Enable() error; Disable() error; IsEnabled() (bool, error) }
```

The concrete libraries are chosen in Phase 0 spikes and recorded in `docs/adr/`.
Read those ADRs first; this skill lists requirements and known traps.

## Tray
- Base bitmaps: `assets/icons/png/tray/` (light/dark/error × 16/20/24/32). Badge drawn at
  runtime with Go `image/draw`: base glyph + red badge (`#C4321F`,
  white 600-weight digits, `99+` cap). Produce 16/20/24/32 px (Windows DPI) and both
  light/dark glyph variants. Cache by (count, theme, size).
- States: idle (glyph `--text-2`), unread (glyph + badge), error/offline (grey glyph +
  orange dot).
- Left click: toggle popup. Right click: menu (Open, Mark all as read, Pause 1 h,
  Settings…, Quit).
- Linux: StatusNotifierItem; GNOME needs the AppIndicator extension — detect absence
  and show a one-time notification explaining it.

## Popup window
- Frameless, ~440×640, positioned above the tray icon (Windows: bottom-right work area;
  Linux: near pointer/top-right fallback). Hide on blur and Esc; keep process alive.
- Closing any window never quits the app; only "Quit" does.

## Toasts — Windows
- Unpackaged apps need an **AppUserModelID** on a Start-menu shortcut for toasts to
  show and stay in Notification Center. Installer creates it; dev mode registers it on
  startup. Use one constant AUMID: `PRism.Desktop`.
- Activation (button clicks while the app runs in background) requires a COM
  activator or protocol activation (`prism://action?...`). Whichever the spike picked,
  route both into `OnAction`. If the app was not running, the activation must start it
  and still perform the action.
- Images: local file paths only (avatars from `internal/avatar` cache).
- Make the toast silent (`<audio silent="true"/>`) — PRism plays its own sound.
- Respect Focus Assist / Do Not Disturb (Windows suppresses banners itself; PRism must
  also skip its own sound — query notification state, ADR records how).

## Toasts — Linux
- `org.freedesktop.Notifications` via D-Bus: `Notify` with `actions` and
  `image-path`; listen to `ActionInvoked` and `NotificationClosed`.
- Check `GetCapabilities` for `actions`; if missing, clicking the toast body opens the PR.
- DND: GNOME `org.gnome.desktop.notifications show-banners`, KDE "do not disturb" —
  best effort; quiet hours in settings are the reliable fallback.

## Sound
- Embedded WAVs from `assets/sounds/` (see `assets/README.md`): `soft-chime` (people
  events), `alert` (CI failed, conflict), `ding`, `pop`. Volume 0–1. Max one sound per 5 s;
  in a burst play the highest-priority one (alert > soft-chime).
- oto v3.5 needs no audio headers: PulseAudio is pure Go, ALSA is loaded at runtime
  via purego (ADR 0003).

## Keyring
- Service `PRism`, account = login. Linux requires a Secret Service (gnome-keyring,
  KWallet). If unavailable, show a clear error and offer "store token in an encrypted
  file" only if the ADR approved it.

## Autostart
- Windows: `HKCU\Software\Microsoft\Windows\CurrentVersion\Run` value `PRism` = `"<exe>" --background`.
- Linux: `~/.config/autostart/prism.desktop` with `Exec=<path> --background`,
  `X-GNOME-Autostart-enabled=true`.
- `--background` starts in tray without opening the window.

## Single instance & sleep
- Second launch focuses the existing instance (and forwards protocol activation args).
- Detect resume: compare monotonic vs wall clock on a 30 s ticker; jump > 2× poll
  interval → poll now.

## Manual test checklist (run on Win 11, Ubuntu GNOME, KDE)
- [ ] badge updates 0 → 3 → 0
- [ ] toast shows avatar, both buttons work with window closed
- [ ] activation works when app was quit (Windows)
- [ ] DND/quiet hours → no sound, item still listed
- [ ] reboot with autostart → app in tray, no window
