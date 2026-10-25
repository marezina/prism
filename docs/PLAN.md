# PRism — Implementation plan

Work strictly phase by phase. Each phase ends with its **Definition of done (DoD)**
checked off in this file. Phases 0–4 deliberately build a headless, fully tested
core before any UI: notifications that are wrong or duplicated cannot be fixed by a
nice UI.

Legend: ⚠ = risk to resolve early · 🧪 = needs tests · 📄 = write an ADR in `docs/adr/`

---

## Phase 0 — Scaffold and technical spikes

Goal: prove every risky platform capability on BOTH Windows 11 and Linux before
writing product code.

Tasks
- [x] Repo, `go.mod`, layout from ARCHITECTURE §3, `CLAUDE.md`, skills, docs.
- [x] Wails v3 project (Svelte + TS template). **Pin the exact alpha version.** 📄 — beta.28, ADR 0001
- [x] CI (`.github/workflows/ci.yml`): `go vet`, `golangci-lint`, `go test` on
      `ubuntu-latest` + `windows-latest`; app build job.
- [ ] Spike A ⚠: tray icon + popup window attached to tray, opens near the tray,
      hides on blur (Win + Linux KDE + GNOME with AppIndicator).
      _Linux KDE/Wayland verified by author; Windows + GNOME pending (ADR 0002)._
- [ ] Spike B ⚠: toast with image, two action buttons, click → Go callback, on Windows
      (unpackaged app, AppUserModelID) and Linux (D-Bus actions). First try Wails v3
      notifications service; fallback `go-toast` (Win) / `godbus` (Linux). 📄
      _Linux verified incl. real clicks (ADR 0002); Windows pending._
- [ ] Spike C: play an embedded WAV (oto/beep) on both OSes; Linux needs ALSA headers in CI.
      _oto v3.5.1, no ALSA headers needed; Linux OK (audible), Windows pending (ADR 0003)._
- [ ] Spike D: `go-keyring` store/read/delete on Windows Credential Manager and Linux
      Secret Service. Decide fallback when no Secret Service is running. 📄
      _Linux OK; no file fallback proposed; Windows pending (ADR 0004)._
- [x] Spike E: runtime-generated tray icon with numeric badge (light + dark).

DoD
- [ ] CI green on both OSes.
- [ ] Spike results recorded as ADRs: chosen libraries + versions, known limitations.
- [ ] Hello-world tray app with a working toast button callback on both OSes.

---

## Phase 1 — Auth and GitHub client

Tasks
- [ ] Org-owned OAuth App with Device Flow (see `docs/github-oauth-app-setup.md`).
      _Needs an org admin; set `auth.ClientID` afterwards. Until then `prism-cli login --pat`._
- [x] `internal/auth`: device flow (own loop over net/http instead of x/oauth2, ADR 0005),
      handle `authorization_pending`, `slow_down`, `expired_token`, `access_denied`. 🧪
- [x] PAT fallback (validate token + scopes via `GET /user`, header `X-OAuth-Scopes`).
- [x] Token in keyring; `Logout` deletes it.
- [x] `internal/gh`: REST (`go-github`) + GraphQL (`githubv4`) with shared HTTP client,
      User-Agent `PRism/<version>`, rate-limit tracking, typed errors
      (`ErrUnauthorized`, `ErrSSORequired` from `X-GitHub-SSO` header, `ErrRateLimited`). 🧪
- [x] `/user/orgs` + SSO detection per org; `/user/teams` for team review requests.
- [x] `prism-cli login` and `prism-cli whoami`.
- [ ] ⚠ Verify search qualifiers: multiple `org:`/`user:` OR-ed? `@me` in GraphQL search? 📄
      _Tool ready: `prism-cli verify-search`; record result in ADR 0005._

DoD
- [ ] Fresh login via device flow works on both OSes; token survives restart.
- [ ] Revoked token → `ErrUnauthorized` surfaced, keyring cleared.

---

## Phase 2 — Snapshot model, watch set, fixtures

Tasks
- [x] `internal/model`: `PRSnapshot` and friends (ARCHITECTURE §4).
- [x] GraphQL queries in `internal/gh/queries/` (see `github-api` skill reference):
      watch-set search, PR snapshot fragment, batch `nodes(ids:)`.
- [x] Pagination for >50 reviews/comments/threads (rare, but must be correct). 🧪
- [x] `internal/watch`: build watch set from settings; drafts filter.
- [x] `prism-cli record --pr owner/repo#N --out testdata/fixtures/<name>/<step>.json`
      with **sanitizer** (replace logins/bodies/urls with stable fakes, keep structure).
- [ ] Record fixtures for real scenarios (see event-detector skill §Scenarios).
      _Needs a test repo + a colleague; `prism-cli record` and `sanitize-check` are ready._

DoD
- [ ] `prism-cli watch --dry` prints the watch set for the user's orgs.
- [ ] ≥ 15 sanitized fixture sequences committed.

---

## Phase 3 — CORE detectors (the heart)

Tasks 🧪 (follow the `event-detector` skill for every item)
- [x] `detect.Diff` skeleton, global rules (SPEC §4: self, bots, pending, baseline).
- [x] `my_pr.approved`, `my_pr.changes_requested`.
- [x] `my_pr.comment` (conversation comment, review body, new thread root).
- [x] `my_pr.thread_reply`.
- [x] `comment.reply_to_me` — threads (`replyTo`, my earlier comment in thread),
      quote-reply matcher, @mention-after-my-comment rule.
- [x] Precedence resolver (one activity → one event).
- [x] Excerpt builder: strip markdown, quotes and code fences, 200 chars.
- [x] Deep links: comment `url`, review `url`.

DoD
- [x] Golden tests for every core event incl. negative cases (self, bot, edit,
      pending review, baseline, precedence). Coverage of `internal/detect` ≥ 90 %.
      _39 hand-written scenarios, coverage 99.6 %; behaviour clarified in SPEC §3.1._
- [ ] Each core event verified once against real GitHub activity (do it with a colleague
      on a test repo) and recorded as a fixture.
      _Open: needs a test repo + colleague (`prism-cli record`)._

---

## Phase 4 — Store, poller, lifecycle (headless product)

Tasks
- [x] `internal/store`: migrations, schema (ARCHITECTURE §5), tombstones, queries. 🧪
- [x] `internal/poller`: slow tier (watch set → snapshots → Diff → store),
      fast tier (`/notifications` + `If-Modified-Since` + `X-Poll-Interval`)
      as trigger, startup poll, sleep/wake detection, backoff. 🧪 (fake clock + fake gh)
- [x] PR enter/leave handling; merged/closed fetch; purge + `my_pr.closed`.
      _`my_pr.closed` is a detector (`closed.go`): the poller diffs the final snapshot, so last-minute comments are reported too._
- [x] First-run baseline (SPEC §4.5).
      _Silent, plus quiet list-only items for review requests pending at first run._
- [x] Settings model with SPEC §7 defaults.
- [x] `prism-cli watch` prints live events, `prism-cli unread`, `prism-cli read <id|all>`.

DoD
- [ ] Run `prism-cli watch` for one full working day: every core event that happened on
      GitHub appears exactly once; no events after restart; closed PRs purge correctly.
- [ ] Rate-limit usage logged; < 10 % of hourly GraphQL budget.

---

## Phase 5 — Secondary detectors

Tasks 🧪 (event-detector skill)
- [x] `review.requested` (incl. team requests, who requested via timeline event).
- [x] `mention` (user + team).
- [x] `ci.failed`, `ci.passed` (statusCheckRollup; failing check names in summary).
- [x] `merge.conflict` (ignore UNKNOWN; re-poll that PR after 30 s when UNKNOWN).
- [x] `commits.after_my_review` (head change after my latest review; force-push).
- [x] `my_pr.opened`, `my_pr.closed`.

DoD
- [x] Golden tests for each (85 scenarios in total, coverage 99 %).
- [ ] One-day run passes again with all events enabled. _(Done with the app, not the CLI.)_

---

## Phase 6 — Notifications, sound, tray

Tasks
- [x] `internal/notify`: `Notifier` interface; Windows + Linux implementations from
      the Phase 0 spike; toast actions wired to store (Open PR → open URL + mark read).
      _Pure dispatcher in `internal/notify`; Wails adapter in `cmd/prism/toasts.go`; app core in `internal/app`._
- [x] `internal/avatar`: download `avatarUrl(size: 96)`, disk cache, ETag; local file
      path used in toasts (remote images are unreliable in toasts).
- [x] Burst grouping, quiet hours, DND respect, per-event flags. 🧪 (pure, fake clock)
- [x] `internal/sound`: embedded sounds, volume, 5 s rate limit.
- [x] `internal/tray`: badge icon (0, 1–99, 99+), offline state, menu: Open, Mark all
      as read, Pause 1 h, Settings, Quit.

DoD
- [ ] On both OSes: new event → toast with avatar + sound + badge; action buttons work
      with app in background; DND suppresses toast/sound.

---

## Phase 7 — UI (follow `prism-ui` skill)

Tasks
- [x] Design tokens + Inter (bundled via @fontsource, offline) + light/dark.
- [x] Popup window: tabs (All, My PRs, Reviews, Mentions, CI), groups per PR,
      item rows (avatar/icon, actor, action, excerpt, time, unread dot), Mark PR read,
      Mark all read, empty state, offline banner, last-sync footer.
- [x] Keyboard: ↑/↓ move, Enter open, `E` mark read, Esc close.
- [x] Login flow screens (SPEC §8) incl. errors.
      _Device flow needs `auth.ClientID` (org OAuth App); PAT sign-in works now._
- [x] Settings: all 7 sections; Events table drives `settings.events`.
- [x] Relative time updates every 30 s.

DoD
- [ ] Matches canvas mockups; usable with keyboard only; contrast AA in both themes.

---

## Phase 8 — Robustness and polish

Tasks
- [x] Single instance (second launch focuses the first).
- [x] Autostart: Windows `HKCU\...\Run`, Linux `~/.config/autostart/prism.desktop`.
      _Applied for release builds only; dev builds never register._
- [x] Logging with rotation; "Open logs folder"; "Reset local data".
- [x] Error UX: token revoked → login screen; SSO missing → banner per org; offline.
- [x] Settings change → watch set rebuilt, unwatched owners purged.
- [ ] Memory/CPU check: idle < 1 % CPU, < 80 MB RAM (incl. WebView, window closed).
      _Measured 2026-10-25 on KDE/Wayland, window closed: CPU 0 %, PSS ≈ 255 MB
      (WebKit web process ≈ 120 MB, network process ≈ 36 MB, main ≈ 100 MB). 80 MB is not
      reachable while WebKitGTK lives; destroying the WebView on hide (recreate on open)
      would save ≈ 150 MB at the cost of a slower popup. Decision pending._

DoD
- [ ] 1 week of daily use by the author without a single wrong/duplicate core event.

---

## Phase 9 — Packaging, distribution, beta

Tasks (follow `release` skill)
- [x] Windows: NSIS installer (WebView2 bootstrapper check), Start-menu shortcut with
      AppUserModelID (needed for toasts), code signing.
      _Per-user, autostart option, clean uninstall; AUMID comes from the app (ADR 0002).
      Signing runs when the certificate secrets exist. First real build happens in CI._
- [x] Linux: AppImage + .deb; README for GNOME AppIndicator.
- [x] Release workflow on tag `v*` → GitHub Release in the internal repo with SHA256 sums.
- [x] Update check: latest release of the internal repo via user's token → notify
      "Update available" (auto-install can come later).
- [ ] Beta with 2–3 colleagues; feedback issue template.
      _Template done (`.github/ISSUE_TEMPLATE/beta-feedback.yml`); beta itself open._

DoD
- [ ] Colleague installs from a release link without help and gets their first
      notification within 5 minutes.

---

## Working with Claude Code — kickoff prompts

Use one session per phase. Start each with:

> Read CLAUDE.md, docs/SPEC.md, docs/ARCHITECTURE.md and docs/PLAN.md.
> We are doing Phase N. Load the relevant skills. First list the tasks you will do
> and any open questions, then implement task by task, running tests after each.
> Tick the checkboxes in PLAN.md as you finish them.

For detector work:

> Using the event-detector skill, implement `<event id>` exactly as in SPEC §3.
> Start with failing golden tests from the fixtures, then the code.
