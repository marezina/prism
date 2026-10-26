---
name: event-detector
description: The workflow for adding, changing or debugging a PRism notification event type (approved, comment, reply to me, CI failed, merge conflict, review requested, etc.) in internal/detect, including golden fixture tests, dedup keys and precedence. Use this skill whenever the task mentions an event, a missed or duplicate notification, detection logic, fixtures, or anything in internal/detect or testdata/fixtures — even if the change looks trivial.
---

# Event detectors

The detector is the product. A missed approve or a duplicated reply destroys trust
faster than any UI bug. Follow this workflow exactly.

## Contract
```go
// internal/detect
func Diff(prev, curr *model.PRSnapshot, c model.DetectContext) []model.Event
```
- `prev == nil` or `c.FirstSeen` → baseline: return only events allowed by SPEC §4.4.
- Pure: no IO, no `time.Now()`, deterministic order (sort by OccurredAt, then DedupKey).
- One detector per event type in its own file: `approved.go`, `reply_to_me.go`, …
  each exposing `func detectX(prev, curr, c) []candidate`.
- `Diff` runs all detectors, applies global filters (self, bots), then the
  **precedence resolver**: candidates sharing the same source object (comment or
  review id) collapse to the highest-precedence type (SPEC §3.1).

## Workflow for a new or changed event
1. Read the event's row in `docs/SPEC.md` §3. If the spec is ambiguous, stop and ask —
   update SPEC first, code second.
2. Add/confirm the `EventType` constant and default flags in `internal/model/events.go`
   and `internal/settings/defaults.go`.
3. Write failing golden tests FIRST (format below): at least
   - one positive case,
   - self-authored → no event,
   - bot-authored → no event (when ignoreBots),
   - baseline (`prev == nil`) → no event,
   - re-run with the same snapshot pair → identical output (idempotent),
   - precedence case if the event shares a source with another type.
4. Implement the detector. Keep helpers (quote matching, mention parsing, excerpt) in
   `text.go` with their own unit tests.
5. `go test ./internal/detect/... -run Golden -update` ONLY to create a new golden
   file; review the produced JSON by hand before committing.
6. Run the full suite. Coverage of `internal/detect` must stay ≥ 90 %.
7. If the event came from a real bug, add the real (sanitized) fixture that reproduced it.

## Golden fixture format (references/fixtures.md has full details)
```
testdata/fixtures/<scenario>/
  context.json     # DetectContext: me, teams, ignoreBots, firstSeen
  prev.json        # PRSnapshot or null
  curr.json        # PRSnapshot
  expected.json    # []Event (golden)
```
Test runner walks all scenario folders; name scenarios `<event>__<case>`, e.g.
`reply_to_me__quote_in_conversation`, `approved__self_ignored`.

## Dedup keys
Exactly as SPEC §3. A key must be derivable from GitHub data alone (never local
counters, never timestamps from the local clock) so restarts are idempotent.

## Tricky cases to always consider
- Review with body + inline comments submitted together: one review event; inline
  thread roots in the same review are part of it (match by review → don't emit
  separate `my_pr.comment` for comments whose createdAt equals the review submit and
  that belong to that review — verify via fixture).
- Reply to my comment on MY PR → `comment.reply_to_me`, not `my_pr.thread_reply`.
- Conversation "replies" aren't threaded: only quote-reply or @mention after my
  last comment counts (SPEC §3.1). Quote matching: normalize whitespace, ignore
  markdown emphasis, require ≥ 1 full non-empty line equality.
- Mentions: `@login` with word boundaries, case-insensitive; ignore inside code
  spans/fences and quoted blocks; team mentions `@org/team`.
- Force-push: head changes but `commits.totalCount` may not grow.
- CI: rollup `PENDING` → `SUCCESS` emits `ci.passed`; `SUCCESS` stays → nothing;
  new head → state resets.
- `mergeable: UNKNOWN` never emits and never clears a previous CONFLICTING state.
- Deleted comment in `curr` that exists in `prev` → no event (store handles cleanup).

## Scenarios to record
approve; changes requested; approve with body; plain comment; review with 3 inline
comments; reply in thread by colleague to colleague on my PR; reply to my inline comment
on my PR; reply to my inline comment on other PR; quote-reply in conversation; @mention
after my comment; bot comment; my own comment; CI fail → pass; conflict; force-push
after my review; review requested (user and team); PR merged; draft → ready.
