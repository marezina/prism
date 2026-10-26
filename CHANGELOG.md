# Changelog

All notable user-visible changes. Format: [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
versions: [SemVer](https://semver.org/). Before tagging, rename `[Unreleased]` to the
version (e.g. `[1.1.0] - 2026-11-02`); the release workflow uses that section
as the release notes.

## [Unreleased]

## [1.1.0] - 2026-10-26

### Added
- Mute a noisy PR from its group in the list: its notifications are cleared and it
  stays silent until it closes. Muted PRs are listed under Settings → Muted PRs.
- Each PR group shows its current status: draft, approvals, changes requested,
  CI passing / failing / running and merge conflicts.

### Changed
- Notifications and PR groups slide in and out; views and hovers ease. Animations are
  off when the system asks for reduced motion.

## [1.0.0] - 2026-10-26

### Added
- Tray app for Windows 11 and Linux with an unread badge, offline state and a menu
  (Open, Mark all as read, Pause 1 hour, Settings, Quit).
- Sign in with GitHub (device flow) or a classic personal access token; the token is
  kept in the OS keyring.
- Organization picker; personal repositories optional; SSO-protected orgs flagged with
  an "Authorize" link.
- Notifications for your PRs: approved, changes requested, comments, thread replies,
  CI failed / passed, merge conflicts, merged / closed, opened.
- Notifications on other PRs: replies to your comments (threads and quote-replies),
  @mentions of you or your team, review requests, new commits after your review.
- Toasts with avatar and "Open PR" / "Mark as read"; bursts of 3+ become one summary;
  sounds at most every 5 s; quiet hours, Do Not Disturb and a 1-hour pause.
- Popup with tabs (All, My PRs, Reviews, Mentions, CI), PRs grouped, keyboard control
  (↑/↓, Enter, E, Esc), light and dark theme. Clicking a comment, reply or mention opens
  it on GitHub; clicking any other notification marks it read; the PR title opens the PR.
- Refresh button spins while a sync is running.
- Settings for every event (list / toast / sound; grouped by Your PRs, Checks and
  merging, Other PRs), sounds, poll interval, bots, drafts, quiet hours, start at login,
  theme; "Reset local data".
- Update check against this repository's releases.
