# Fixture details

## Recording
`prism-cli record --pr owner/repo#N --out testdata/fixtures/<scenario>/curr.json`
records one snapshot. Record `prev.json` before the activity and `curr.json` after.

The recorder MUST sanitize:
- logins → `alice`, `bob`, `carol`, … (stable mapping per fixture; `me` stays `me`)
- bodies → keep structure (quotes, @mentions, code fences) but replace words with
  lorem-free neutral text like `text-1 text-2`; keep mention/quote targets consistent
- urls → `https://github.example/<owner>/<repo>/pull/<n>#<anchor>`
- avatar urls → `avatar://<login>`
- repo/org names → `org-a/repo-a`
Run `prism-cli sanitize-check` (fails if any original login/org remains).

## Hand-written fixtures
For edge cases that are hard to reproduce, copy a recorded snapshot and edit it.
Note the edit in `context.json` → `"note": "..."`.

## Golden file
`expected.json` is a sorted JSON array of Event with `occurredAt` as RFC3339.
Don't include volatile fields. Diff output on failure must show per-field changes
(use `github.com/google/go-cmp`).
