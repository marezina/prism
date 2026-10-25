# PRism — project guide for Claude

PRism is a cross-platform (Windows 11 + Linux) system-tray app written in Go that
shows desktop notifications for GitHub pull-request activity, in the spirit of
Azure DevOps PR e-mails. UI language: English. Users: the author and colleagues
inside the same GitHub organization.

## Read before working
- `docs/SPEC.md` — product spec: event catalog, detection rules, lifecycle, settings.
- `docs/ARCHITECTURE.md` — packages, data flow, DB schema, polling model.
- `docs/PLAN.md` — phased implementation plan. Work phase by phase; do not start a
  phase before the previous phase's "Definition of done" is met.
- Skills in `.claude/skills/` — load the relevant one before touching that area:
  - `github-api` — any GitHub REST/GraphQL/auth code
  - `event-detector` — adding or changing an event type / detection logic
  - `prism-ui` — anything in `frontend/`
  - `desktop-integration` — tray, toasts, sound, autostart, single instance
  - `release` — builds, CI, packaging, signing, auto-update

## Non-negotiables
1. The five CORE events (see SPEC §3.1) must never regress. Every change to
   `internal/detect` runs `go test ./internal/detect/...` and all golden tests pass.
2. `internal/detect` is pure: no network, no DB, no clock (time is passed in).
3. Never emit an event for the signed-in user's own actions. Bots are filtered
   per settings.
4. Never log tokens, Authorization headers, or full comment bodies at info level.
5. All GitHub data stays local. No telemetry.
6. Pin dependency versions (especially Wails v3 beta, see ADR 0001). Upgrades are
   deliberate, one per PR, with a note in `docs/adr/`.

## Conventions
- Go: standard layout, `internal/` only, `context.Context` first arg for IO,
  errors wrapped with `%w`, `log/slog` for logging.
- Tests: table-driven; detection tests use golden fixtures in `testdata/`.
- Frontend: Svelte + TypeScript, design tokens from `prism-ui` skill only —
  no ad-hoc colors or font sizes.
- Commits: conventional commits (`feat(detect): ...`).

## Commands
- `go test ./...` — unit + golden tests
- `go run ./cmd/prism-cli login [--pat]` / `whoami` / `orgs` / `logout` — account (dev)
- `go run ./cmd/prism-cli verify-search` — check search qualifier semantics (ADR 0005)
- `go run ./cmd/prism-cli watch` — headless poller printing events (dev)
- `go run ./cmd/prism-cli record --pr owner/repo#123` — record a sanitized fixture
- `go run ./cmd/prism-spike <badge|keyring|sound>` — Phase 0 platform spikes
- `wails3 dev` — run the app with hot reload
- `wails3 task build` — production build for current OS
