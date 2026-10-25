---
name: github-api
description: How PRism talks to GitHub — OAuth device flow, token storage, REST and GraphQL clients, the watch-set search and PR snapshot queries, Notifications API polling, rate limits, SSO and error handling. Use this skill whenever you touch internal/auth, internal/gh, internal/watch, the poller's network code, or write/modify any GraphQL query or REST call, even for small changes.
---

# GitHub API in PRism

PRism only READS from GitHub. Never add a call that writes (mark read, comment,
approve) — out of scope for v1 (SPEC §9).

## Clients
- One shared `*http.Client` with: OAuth2 transport, `User-Agent: PRism/<version>`,
  30 s timeout, retry on 502/503/504 with jitter (max 3), no retry on 4xx.
- REST: `github.com/google/go-github` (pin major version in go.mod).
- GraphQL: `github.com/shurcooL/githubv4`. Queries as Go structs live in
  `internal/gh/queries/`. The canonical shape is in `references/queries.graphql` —
  keep the Go structs in sync with it.
- Base URLs are configurable (future GHES), never hard-code `api.github.com` outside
  `internal/gh/config.go`.

## Auth (details: references/auth.md)
- OAuth App (org-owned) + Device Flow. Client ID only, never a secret.
- Scopes: `repo notifications read:org`. After login, read `X-OAuth-Scopes`
  from `GET /user` and fail clearly if one is missing.
- Token in OS keyring (service `PRism`, account = login). Never in the DB, logs or settings.
- OAuth tokens don't expire on a timer; handle 401 anywhere → `ErrUnauthorized`
  → clear keyring → UI goes to login.

## Error mapping (internal/gh/errors.go)
| Signal | Error |
|---|---|
| 401 | `ErrUnauthorized` |
| 403 + header `X-GitHub-SSO: required; url=...` | `ErrSSORequired{Org, URL}` (show Authorize link) |
| 403/429 + `X-RateLimit-Remaining: 0` or `Retry-After` | `ErrRateLimited{ResetAt}` |
| GraphQL `errors[].type == "FORBIDDEN"` on an org resource | treat as SSO/permission for that org, skip org, keep others working |
| Network | wrap, poller backs off |

One failing org must never stop polling of the others.

## Watch set and snapshots
See ARCHITECTURE §2. Rules:
- Use GraphQL `search(type: ISSUE)` for the watch set; collect node IDs; then fetch
  snapshots with `nodes(ids:)` in batches of ≤ 50.
- Keep connection page sizes as in `references/queries.graphql`. If `pageInfo.hasNextPage`
  for comments/reviews/threads, paginate that connection for that PR only.
- Ask for `avatarUrl(size: 96)`.
- `mergeable` may be `UNKNOWN` — never treat as conflict; re-poll that PR after ~30 s.
- `statusCheckRollup` can be null (no CI) — that's "no CI", not failure.

## Notifications API (fast tier)
- `GET /notifications?participating=false&all=false` with `If-Modified-Since: <Last-Modified>`.
- Store `Last-Modified` in `meta`. 304 → nothing to do (doesn't count against rate limit).
- Respect `X-Poll-Interval` as the minimum interval.
- Only use it as a trigger: for threads with `subject.type == "PullRequest"` whose repo
  owner is watched, schedule an immediate snapshot poll of that PR. Never create events
  directly from the inbox.
- Requires a classic OAuth/PAT token — fine-grained PATs and GitHub App tokens can't
  call it. If the user pastes a fine-grained PAT, explain and reject.

## Rate limits
- Read `rateLimit { cost remaining resetAt }` in every GraphQL query; log at debug,
  warn below 20 %. Budget: < 10 % of 5 000 points/h in normal use.
- Back off until `resetAt` on exhaustion; tray shows the offline/error state.

## Testing
- Never hit real GitHub in `go test`. Use `httptest.Server` with recorded responses
  (`testdata/http/`) or the fixture snapshots.
- `prism-cli` is the place for manual live testing.
