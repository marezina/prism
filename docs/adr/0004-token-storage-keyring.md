# ADR 0004 — Token storage in the OS keyring

- Status: proposed — Linux verified; Windows 11 pending; fallback needs sign-off
- Date: 2026-10-24

## Context
Spike D. The GitHub token must live in the OS keyring (service `PRism`,
account = login), never in the DB, logs or settings (CLAUDE.md §4,
github-api skill). Linux needs a running Secret Service.

## Options considered
1. `github.com/zalando/go-keyring` — Windows Credential Manager (wincred),
   Linux Secret Service over D-Bus (godbus, no cgo).
2. `github.com/99designs/keyring` — more backends (incl. encrypted file), heavier.

## Decision
Option 1: `github.com/zalando/go-keyring v0.2.8`.

When no Secret Service is available (`keyring.Set` fails on Linux): **no file
fallback in v1**. PRism shows a clear error ("PRism needs a keyring: install and
unlock gnome-keyring or KWallet") and stays on the login screen. An encrypted file
would need a key stored next to it or a user password, which adds UX and
security work for a rare setup.

## Verified on Linux (Debian 13, KDE, gnome-keyring as Secret Service)
`prism-spike keyring`: set → get (round-trip matches) → delete → get returns
`keyring.ErrNotFound`.

## Consequences
- Release notes for Linux list "Secret Service (gnome-keyring or KWallet)" as a
  requirement.
- Verify on Windows with `prism-spike keyring`.
- Revisit the fallback only if a colleague's setup actually lacks a Secret Service.
