// The only way the UI talks to Go: generated Wails bindings and pushed events
// (prism-ui skill §Talking to Go). Never poll from JS.
export * as Api from '../../bindings/prism/cmd/prism/appservice'
export type { AuthState, MutedPR, Notification, Org, PRStatus, SyncStatus } from '../../bindings/prism/internal/app/models'
export type { Settings, Flags } from '../../bindings/prism/internal/settings/models'
export { Events } from '@wailsio/runtime'

/** Human-readable message for a rejected binding call. */
export function errorText(err: unknown): string {
  if (err instanceof Error) return err.message
  if (typeof err === 'string') return err
  if (err && typeof err === 'object' && 'message' in err) return String((err as { message: unknown }).message)
  return 'Something went wrong'
}
