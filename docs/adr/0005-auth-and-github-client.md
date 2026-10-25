# ADR 0005 — Auth flow and GitHub client

- Status: accepted (search-qualifier results pending, see below)
- Date: 2026-10-24

## Context
Phase 1: OAuth device flow, classic-PAT fallback, token storage, and one HTTP
stack for REST + GraphQL with typed errors (github-api skill).

## Decisions
- Libraries: `github.com/google/go-github/v92 v92.0.0` (REST),
  `github.com/shurcooL/githubv4 v0.0.0-20260209031235-2402fdf4a9ed` (GraphQL),
  `golang.org/x/term v0.46.0` (hidden PAT input in the CLI). go-github v92 and
  x/oauth2 v0.37 need Go 1.26, so `go.mod` says `go 1.26.0`.
- **Device flow without x/oauth2.** `x/oauth2`'s `DeviceAccessToken` owns its
  ticker and real clock, so `slow_down` and expiry can only be tested by waiting.
  `internal/auth.DeviceFlow` is ~150 lines over `net/http` with injectable
  `now`/`sleep`; it handles `authorization_pending`, `slow_down` (+5 s, or the
  server's larger `interval`), `expired_token`, `access_denied`, plus local expiry.
  (x/oauth2 is still an indirect dependency via githubv4.)
- **One transport** for REST and GraphQL: Bearer token, `User-Agent: PRism/<version>`,
  30 s timeout, retry on 502/503/504 (3 retries, 0.5/1/2 s + ≤ 250 ms jitter,
  body rewound), rate limits recorded per `X-RateLimit-Resource` with one warning
  per window below 20 %. 401 → `gh.ErrUnauthorized`, 403 + `X-GitHub-SSO: required`
  → `*gh.SSORequiredError{URL}`, 403/429 with `Retry-After` or `Remaining: 0` →
  `*gh.RateLimitedError{ResetAt}`. Errors survive go-github/githubv4 wrapping
  (`errors.Is/As`).
- **Orgs and SSO:** `/user/orgs` hides orgs the token is not SAML-authorized for
  and says so in `X-GitHub-SSO: partial-results; organizations=<ids>`. We resolve
  each id via `/organizations/{id}` and list it with `SSORequired` and an authorize
  URL (GitHub's URL when it sends one, else `<web>/orgs/<org>/sso`).
- **Tokens accepted:** OAuth tokens from the device flow; pasted PATs must be
  classic (`ghp_`). Fine-grained (`github_pat_`) is rejected (no Notifications API).
  Required scopes `repo notifications read:org`; `notifications` is also satisfied
  by `repo`, and `read:org` by `write:org`/`admin:org` (GitHub scope hierarchy).
- **Keyring layout** (amends ADR 0004): service `PRism`, account = login holds the
  token; account `_active-login` holds the signed-in login (not secret) so the
  token can be found at startup before the DB exists. `Logout` deletes both.
  Every non-"not found" keyring error is wrapped as `auth.ErrKeyringUnavailable`.
- **Revocation:** `Session.Restore` verifies the stored token with `GET /user`;
  on `ErrUnauthorized` it deletes the keyring entries before returning the error.

## Search qualifiers (PLAN Phase 1 ⚠) — to fill in
Run `prism-cli verify-search` with a real token and record here:
- several `org:` / `user:` qualifiers in one query OR-ed? → _pending_
- `@me` works in GraphQL `search`? → _pending_

## Consequences
- `auth.ClientID` stays empty until an org admin registers the OAuth App
  (docs/github-oauth-app-setup.md); set it in code or with
  `-ldflags "-X prism/internal/auth.ClientID=…"`. Until then use `login --pat`.
- The SSO authorize fallback URL for hidden orgs needs a real SAML org to verify.
