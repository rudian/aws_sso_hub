// Small helpers shared by the Accounts and Migration pages.
import { SessionState } from '../../api/models'
import { motionReduced } from '../../lib/motion'
import type { Profile } from '../../api/models'

/** Cancel a bound-method promise without raising "unhandled rejection" for the cancellation itself. */
export function cancelQuietly(p: { cancel?: () => Promise<unknown> } | null | undefined): void {
  try {
    void p?.cancel?.().catch(() => undefined)
  } catch {
    /* already settled */
  }
}

/** Scroll a portal section (rendered with data-portal) into view. */
export function scrollToPortal(name: string): void {
  window.setTimeout(() => {
    document.querySelector(`[data-portal="${CSS.escape(name)}"]`)?.scrollIntoView({ block: 'start', behavior: motionReduced() ? 'auto' : 'smooth' })
  }, 60)
}

export function profileByName(profiles: Profile[] | null | undefined, name: string): Profile | undefined {
  return (profiles ?? []).find((p) => p.name === name)
}

export function needsSignIn(state: SessionState): boolean {
  return state === SessionState.SessionExpired || state === SessionState.SessionSignedOut || state === SessionState.SessionError
}

/** "mm:ss" for a countdown. */
export function clock(ms: number): string {
  const s = Math.max(0, Math.ceil(ms / 1000))
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`
}
