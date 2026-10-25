# PRism — Product specification

Status: agreed v1 scope. Source of truth for behaviour. If code and spec
disagree, fix one of them in the same PR.

## 1. Product decisions

| # | Topic | Decision |
|---|---|---|
| D1 | Scope | Only selected GitHub organizations + (optionally) the user's personal repositories. Chosen right after login, editable in Settings. |
| D2 | Read = delete | Marking a notification read deletes it. No archive. |
| D3 | PR lifecycle | Unread notifications are kept while the PR is open. When the PR is closed or merged, all its notifications are purged; one `my_pr.closed` notification remains (if enabled) until read. |
| D4 | GitHub inbox sync | Not in v1. PRism never writes to GitHub. |
| D5 | Platforms | Windows 11 and Linux from day one. |
| D6 | Audience | Colleagues in the same org. OAuth App is owned by the organization. |
| D7 | UI language | English. |
| D8 | Name / brand | PRism. Primary color `#2F5D8A`, prism glyph. |
| D9 | Bots / drafts | Bots ignored by default; draft PRs watched by default. Both configurable. |

## 2. Glossary
- **Me** — the signed-in GitHub user.
- **My PR** — open PR authored by me in a watched owner.
- **Other PR** — open PR not authored by me where I commented, reviewed,
  am requested as reviewer (directly or via team), or am mentioned.
- **Watched owner** — selected org or my user account.
- **Event** — one user-visible notification. One GitHub activity → at most one event.

## 3. Event catalog

Each event has a stable ID (used in code, settings and DB), a dedup key and
default delivery flags `list / toast / sound`.

### 3.1 CORE events (must never regress)

| ID | Trigger | Dedup key | Defaults |
|---|---|---|---|
| `my_pr.approved` | New review with state `APPROVED` on my PR, author ≠ me | `review:<databaseId>` | ✔ ✔ ✔ |
| `my_pr.changes_requested` | New review with state `CHANGES_REQUESTED` on my PR | `review:<databaseId>` | ✔ ✔ ✔ |
| `my_pr.comment` | New top-level comment on my PR: conversation comment, non-empty `COMMENTED` review body, or first comment of a new review thread | `comment:<databaseId>` / `review:<databaseId>` | ✔ ✔ ✔ |
| `my_pr.thread_reply` | New reply in a review thread on my PR where the thread has no earlier comment by me (otherwise it is `comment.reply_to_me`) | `comment:<databaseId>` | ✔ ✔ ✔ |
| `comment.reply_to_me` | On ANY watched PR: (a) review-thread comment whose `replyTo` is mine or whose thread contains an earlier comment by me; (b) conversation comment that quote-replies my comment (a `> ` block matching ≥ 1 full non-empty line of one of my comments) or @mentions me, posted after my last comment on that PR | `comment:<databaseId>` | ✔ ✔ ✔ |

Precedence (one activity → one event): `comment.reply_to_me` > `my_pr.thread_reply` > `my_pr.comment` > `mention`.
Review events (`my_pr.approved`, `my_pr.changes_requested`) outrank all of them for their `review:<id>`.
An approve / changes-requested review with a body produces only the review event; its
body is shown as the excerpt.

Clarifications (agreed for Phase 3; change here first if the behaviour should change):
- **One review submission → one review event.** New threads started by a review (thread
  roots whose comment belongs to that review) are folded into the review's event —
  `my_pr.approved`, `my_pr.changes_requested`, or `my_pr.comment` keyed `review:<id>` for a
  `COMMENTED` review. Excerpt: the review body, else the first folded root. Summary for a
  `COMMENTED` review with ≥ 2 folded roots: "left N comments on your PR".
  A `COMMENTED` review with an empty body and no new roots (only replies) emits no review
  event. Replies inside a review are not folded: each is its own
  `my_pr.thread_reply` / `comment.reply_to_me` event.
- **New** = id not present in `prev`; for reviews, not present as a submitted
  (non-`PENDING`) review, so a pending review that gets submitted counts once.
  `DISMISSED` reviews never emit.
- **Thread replies:** a thread comment that is not the first in its thread. "Earlier
  comment by me" means a comment by me before it in the same thread.
- **Conversation reply rule (b):** "posted after my last comment" is read as *posted after
  at least one earlier comment of mine on that PR* (conversation comment, review body or
  thread comment, compared by GitHub timestamps). Quote-reply targets are my conversation
  comments and review bodies on that PR; my own quoted lines don't count as mine. Mentions
  inside code spans, code fences and quoted blocks are ignored.

### 3.2 Secondary events

| ID | Trigger | Dedup key | Defaults |
|---|---|---|---|
| `review.requested` | I (or a team I belong to) am newly requested as reviewer | `revreq:<timelineEventId>` | ✔ ✔ ✔ |
| `mention` | New comment in a watched PR that @mentions me or my team, not classified higher | `comment:<databaseId>` | ✔ ✔ ✔ |
| `ci.failed` | Rollup state of my PR's head commit becomes `FAILURE` or `ERROR` | `ci:<prId>:<headOid>:fail` | ✔ ✔ ✔ |
| `ci.passed` | Rollup becomes `SUCCESS` for this head (after pending or failure) | `ci:<prId>:<headOid>:ok` | ✔ ✔ ✘ |
| `merge.conflict` | My PR's `mergeable` becomes `CONFLICTING` (ignore `UNKNOWN`) | `conflict:<prId>:<headOid>:<baseOid>` | ✔ ✔ ✔ |
| `commits.after_my_review` | Head of an Other PR moves after my latest review (incl. force-push) | `commits:<prId>:<newHeadOid>` | ✔ ✔ ✘ |
| `my_pr.closed` | My PR merged or closed (by anyone, including me) | `closed:<prId>` | ✔ ✘ ✘ |
| `my_pr.opened` | A new PR authored by me appears in the watch set | `opened:<prId>` | ✔ ✘ ✘ |

`my_pr.opened` and `my_pr.closed` are the only events allowed for my own actions.

## 4. Detection rules (global)
1. Ignore activity whose author is me (except the two rows above).
2. Ignore bot authors when `ignoreBots` (GraphQL `__typename == "Bot"` or login ending in `[bot]`).
3. Ignore `PENDING` reviews and edits of existing comments (dedup by id).
4. A PR entering the watch set for the first time is **baselined**: its current state
   becomes the cursor and no historic events are emitted — except `review.requested`
   (when the request is why it entered) and `my_pr.opened`.
5. First run after login baselines everything silently, then creates list-only
   (no toast/sound) items for currently pending review requests.
6. Deleted comments: if a stored notification's comment no longer exists, delete
   the notification silently.
7. "Is new" is decided by cursors and IDs, never by the local clock.

## 5. Lifecycle
- New event → stored (if `list` enabled) → toast/sound per flags, quiet hours and DND.
- Read (single / per PR / all) → row deleted; dedup key kept as tombstone so it never re-appears.
- PR leaves the open set → fetched once to learn merged/closed → rows, tombstones and
  pr_state purged; `my_pr.closed` emitted if it was my PR.
- Owner unselected in settings → its PRs purged the same way, no `closed` events.

## 6. Delivery rules
- Burst grouping: ≥ 3 events within a 10 s window → one summary toast
  ("5 new notifications" + type breakdown), one sound.
- Quiet hours and OS Do-Not-Disturb: no toast, no sound; items still listed.
- Sound plays at most once per 5 s.
- Toast actions: **Open PR** (opens the exact comment URL, marks read) and **Mark as read**.
  Summary toast: **Show all** (opens window) and **Mark as read** (marks the burst).
- Tray badge = unread count (99+ cap). Icon states: idle / unread / offline-or-error.

## 7. Settings (persisted)
```jsonc
{
  "owners": ["acme", "acme-labs"],
  "includePersonal": true,
  "ignoreBots": true,
  "includeDrafts": true,
  "pollIntervalSec": 120,               // 60 | 120 | 300 (state poll)
  "events": { "<eventId>": { "list": true, "toast": true, "sound": true } },
  "sound": { "enabled": true, "volume": 0.7, "name": "soft-chime", "alertName": "alert" },
  "quietHours": { "enabled": false, "from": "18:00", "to": "08:00", "weekends": true },
  "burstGrouping": true,
  "autostart": true,
  "theme": "system"                    // system | light | dark
}
```
Settings sections: Account, Organizations, Events, Sound, Behavior, Appearance,
Advanced (open logs folder, reset local data).

## 8. Login flow
1. Welcome → "Sign in with GitHub" (+ "Use a personal access token instead").
2. Device code: code, Copy, Open GitHub again, spinner, expiry countdown (15 min).
3. Organization picker: personal repositories row first (checked), orgs from
   `/user/orgs`, SSO-unauthorized orgs flagged with an "Authorize" link. "Start watching".
4. Initial sync (baseline) → app goes to tray.
Errors: expired code → new code; access denied → retry; org restriction → explain
admin approval; offline → retry with backoff.

## 9. Out of scope for v1
Writing to GitHub, multiple accounts, GitHub Enterprise Server (keep base URL
configurable in code), macOS, issue (non-PR) notifications.
