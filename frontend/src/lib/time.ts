// Relative times: now, 5m, 2h, 3d, then a short date (prism-ui skill).
export function relative(iso: string | undefined, now: number): string {
  if (!iso) return ''
  const t = Date.parse(iso)
  if (Number.isNaN(t) || t <= 0) return ''
  const s = Math.max(0, Math.round((now - t) / 1000))
  if (s < 60) return 'now'
  if (s < 3600) return `${Math.floor(s / 60)}m`
  if (s < 86400) return `${Math.floor(s / 3600)}h`
  if (s < 7 * 86400) return `${Math.floor(s / 86400)}d`
  return new Date(t).toLocaleDateString(undefined, { month: 'short', day: 'numeric' })
}

/** "mm:ss" until an RFC3339 time; "0:00" when past. */
export function countdown(iso: string | undefined, now: number): string {
  const left = Math.max(0, Math.round(((iso ? Date.parse(iso) : now) - now) / 1000))
  return `${Math.floor(left / 60)}:${String(left % 60).padStart(2, '0')}`
}

/** A $state-friendly clock that ticks every `ms`. Returns a stop function. */
export function tick(ms: number, fn: (now: number) => void): () => void {
  fn(Date.now())
  const id = setInterval(() => fn(Date.now()), ms)
  return () => clearInterval(id)
}
