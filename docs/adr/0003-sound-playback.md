# ADR 0003 — Sound playback

- Status: proposed — Linux verified; Windows 11 pending
- Date: 2026-10-24

## Context
Spike C: play the embedded WAVs (`assets/sounds/`, 44.1 kHz 16-bit mono) on
Windows 11 and Linux, with volume, without extra system packages if possible.

## Options considered
1. `github.com/ebitengine/oto/v3` directly, with our own minimal WAV decoder.
2. `github.com/gopxl/beep` on top of oto (decoders, mixer) — more than we need.
3. OS players (PowerShell `SoundPlayer`, `paplay`) — process spawn per sound,
   no volume control on Windows.

## Decision
Option 1: `github.com/ebitengine/oto/v3 v3.5.1`.
- `internal/sound`: `Player` interface, `decodeWAV` (PCM 16-bit only), volume by
  sample scaling, `OtoPlayer` that opens the device lazily on first `Play` with
  the sounds' format (all sounds must share rate and channel count, checked at load).
- Windows: WASAPI, pure Go. Linux: PulseAudio client in pure Go
  (`jfreymuth/pulse`), ALSA fallback loaded at runtime via `purego`.

## Consequences
- No cgo and no `libasound2-dev` for audio; removed from CI.
- The 5 s rate limit and burst priority (alert > soft-chime) are Phase 6 work in
  `internal/notify`/`internal/sound`, not part of the spike.
- Verify on Windows: `prism-spike sound` plays all four sounds at 0.7 volume.
