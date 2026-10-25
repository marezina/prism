# PRism — Architecture

## 1. Overview

```
            ┌──────────── Wails v3 app (cmd/prism) ─────────────┐
            │  tray  ·  popup window (Svelte)  ·  settings/login │
            └───────▲──────────────────────────────▲────────────┘
                    │ events / unread list          │ user actions
┌───────────┐  ┌────┴─────┐  ┌──────────┐  ┌───────┴──────┐  ┌──────────┐
│ watch     │→ │ poller   │→ │ detect   │→ │ store        │→ │ notify   │
│ (search)  │  │ (sched.) │  │ (pure)   │  │ (SQLite)     │  │ toast+snd│
└───────────┘  └────▲─────┘  └──────────┘  └──────────────┘  └──────────┘
                    │
               ┌────┴────┐   ┌──────┐
               │ gh      │←──│ auth │ (device flow, keyring)
               └─────────┘   └──────┘
```

## 2. Polling model (two tiers)

| Tier | What | Interval | Purpose |
|---|---|---|---|
| Fast | `GET /notifications` with `If-Modified-Since` | `X-Poll-Interval` (≈60 s) | Cheap change hint. 304 → nothing. 200 with a thread for a watched PR → schedule an immediate state poll of that PR. |
| Slow | GraphQL watch-set search + PR snapshots | `pollIntervalSec` (default 120 s) | Source of truth for all events, including CI / conflicts which never appear in the inbox. |

Also: poll immediately on startup, on resume from sleep (detect wall-clock jump
> 2× interval), and on "Refresh" in the UI. Exponential backoff on errors
(max 10 min); tray shows offline state.

### Watch set (GraphQL `search`, type ISSUE, per owner qualifier)
- Mine: `is:pr is:open author:@me <owners>`
- Review requested: `is:pr is:open review-requested:@me <owners>` (+ `team-review-requested:<org>/<team>` per team)
- Participated: `is:pr is:open reviewed-by:@me -author:@me <owners>`,
  `is:pr is:open commenter:@me -author:@me <owners>`, `is:pr is:open mentions:@me -author:@me <owners>`
- `<owners>` = `org:acme org:acme-labs user:<me>` — VERIFY in P1 that several owner
  qualifiers are OR-ed; otherwise run one search per owner.
- Drafts filtered client-side by `isDraft` when `includeDrafts=false`.

Union of node IDs = watch set. Snapshot query fetches PRs by node id in batches
(`nodes(ids: [...])`, ≤ 50 per request).

## 3. Packages

| Package | Responsibility | Depends on |
|---|---|---|
| `cmd/prism` | Wails v3 entry: window, tray, toasts adapter, `AppService` bound to the frontend | app + Wails |
| `internal/app` | Application core without Wails: session lifecycle, org picker gate, poller start/stop, delivery, UI operations and DTOs | auth, gh, poller, notify, store |
| `cmd/prism-cli` | Dev tool: `login`, `watch` (print events), `record` (fixtures) | auth, gh, watch, poller, detect, store |
| `internal/auth` | OAuth device flow, PAT fallback, keyring storage (ADR 0005) | gh, `go-keyring` |
| `internal/gh` | REST + GraphQL clients, queries, rate-limit + ETag handling, error types (`ErrUnauthorized`, `ErrSSORequired`) | `go-github`, `githubv4` |
| `internal/model` | `PRSnapshot`, `Comment`, `Review`, `Thread`, `Event`, `EventType` | — |
| `internal/watch` | Builds watch set from settings | gh, model |
| `internal/detect` | **Pure** `Diff(prev, curr *PRSnapshot, ctx DetectContext) []Event` + reply/quote/mention helpers | model |
| `internal/poller` | Scheduler, tiers, sleep/wake, backoff, PR enter/leave handling | gh, watch, detect, store |
| `internal/store` | SQLite (modernc), migrations, tombstones, queries | `modernc.org/sqlite` |
| `internal/notify` | `Notifier` interface, Windows + Linux impls, burst grouping, quiet hours | platform libs |
| `internal/sound` | Embedded sounds, rate limiting | `oto` / `beep` |
| `internal/tray` | Badge icon rendering (Go `image`), menu | Wails tray |
| `internal/settings` | Typed settings, defaults from SPEC §7, validation | store |
| `internal/fixture` | Fixture sanitizer + mapping files (outside the repo), sanitize-check | model |
| `internal/autostart` | Start at login: HKCU Run key / XDG autostart file | — |
| `internal/avatar` | Download + disk cache of avatars (needed for toast images) | net/http |
| `frontend/` | Svelte + TS UI: popup list, login, settings | Wails bindings |

## 4. Core types (sketch)

```go
type EventType string // "my_pr.approved", ... (SPEC §3)

type Event struct {
    Type        EventType
    DedupKey    string
    PRID        string    // GraphQL node id
    Repo        string    // owner/name
    PRNumber    int
    PRTitle     string
    Actor       Actor     // login, avatarURL, isBot
    Summary     string    // "approved your PR"
    Excerpt     string    // first 200 chars, markdown stripped
    URL         string    // deep link to comment/review
    OccurredAt  time.Time // GitHub timestamp
}

type DetectContext struct {
    Me          string
    MyTeams     []string // "org/team"
    IgnoreBots  bool
    FirstSeen   bool     // PR just entered the watch set → baseline
}
```

`PRSnapshot` holds: ids, repo, number, title, url, author, isDraft, state, headOid,
baseOid, mergeable, mergeStateStatus, ciRollup, commitsTotal, reviews, conversation
comments, review threads (with comments, replyTo and the review each comment was
submitted with), review requests and relevant timeline events (REVIEW_REQUESTED_EVENT,
HEAD_REF_FORCE_PUSHED_EVENT). "My latest review oid" is derived from reviews in detect.
Connections are paginated to completion (≤ 20 extra pages each), so snapshots are full.
The previous snapshot is stored as JSON in `pr_state.snapshot`.

## 5. Database schema (v1)

```sql
CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);
-- schema_version, notifications_last_modified, me_login

CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT NOT NULL); -- JSON values

CREATE TABLE pr_state (
  pr_id        TEXT PRIMARY KEY,
  repo         TEXT NOT NULL,
  number       INTEGER NOT NULL,
  role         TEXT NOT NULL,          -- mine | other
  snapshot     TEXT NOT NULL,          -- JSON PRSnapshot (previous)
  updated_at   TEXT NOT NULL
);

CREATE TABLE notifications (
  id           INTEGER PRIMARY KEY,
  dedup_key    TEXT NOT NULL UNIQUE,
  type         TEXT NOT NULL,
  pr_id        TEXT NOT NULL,
  repo         TEXT NOT NULL,
  pr_number    INTEGER NOT NULL,
  pr_title     TEXT NOT NULL,
  actor_login  TEXT,
  actor_avatar TEXT,
  summary      TEXT NOT NULL,
  excerpt      TEXT,
  url          TEXT NOT NULL,
  occurred_at  TEXT NOT NULL,
  created_at   TEXT NOT NULL
);
CREATE INDEX notifications_pr ON notifications(pr_id);

CREATE TABLE tombstones (
  dedup_key    TEXT PRIMARY KEY,
  pr_id        TEXT NOT NULL,
  created_at   TEXT NOT NULL
);
CREATE INDEX tombstones_pr ON tombstones(pr_id);

CREATE TABLE avatars (login TEXT PRIMARY KEY, path TEXT NOT NULL, etag TEXT, fetched_at TEXT NOT NULL);
```

Insert rule: an event is stored only if its `dedup_key` exists in neither
`notifications` nor `tombstones`. Every emitted key (even list-disabled) gets a tombstone.

## 6. Paths
- Data: `os.UserConfigDir()/PRism` → `%APPDATA%\PRism` / `~/.config/PRism`
  (db: `prism.db`, avatars: `avatars/`, logs: `logs/prism.log` rotated 5×5 MB).
- Token: OS keyring, service `PRism`, account = GitHub login.

## 7. Frontend ↔ Go
Wails services (bound methods): `Unread() []NotificationDTO`, `MarkRead(ids)`,
`MarkPRRead(prID)`, `MarkAllRead()`, `Open(id)`, `Settings()`, `SaveSettings(s)`,
`StartLogin()`, `LoginStatus()`, `Orgs()`, `Logout()`, `RefreshNow()`.
Events pushed to frontend: `unread:changed`, `auth:changed`, `sync:status`.
