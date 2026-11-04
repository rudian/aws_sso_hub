// Pure helpers for the Accounts page and the dialogs around it. No React, no API calls.
import { ProfileKind, SessionState, StaticState } from '../../api/models'
import type { Portal, Profile, SessionStatus } from '../../api/models'
import { ErrorCode } from '../../api/models'
import type { UiError } from '../../api/errors'
import { errorText } from '../../i18n'
import type { Lang, T } from '../../i18n'
import { formatDateTime, formatRemaining, remainingMs } from '../../lib/time'

export type PillState = 'active' | 'expiring' | 'refreshable' | 'expired' | 'signed-out' | 'signing-in' | 'error'

const MIN = 60_000

export function pillState(s: SessionStatus): PillState {
  switch (s.state) {
    case SessionState.SessionActive:
      return 'active'
    case SessionState.SessionExpiring:
      return 'expiring'
    case SessionState.SessionRefreshable:
      return 'refreshable'
    case SessionState.SessionExpired:
      return 'expired'
    case SessionState.SessionSigningIn:
      return 'signing-in'
    case SessionState.SessionError:
      return 'error'
    default:
      return 'signed-out'
  }
}

/** The portal can serve credentials right now (a refreshable token renews itself on first use). */
export function isSignedIn(s: SessionStatus): boolean {
  const p = pillState(s)
  return p === 'active' || p === 'expiring' || p === 'refreshable'
}

/** The moment the interactive session (or, when its start is unknown, the access token) ends. */
export function endIso(s: SessionStatus): string | undefined {
  return s.sessionKnown && s.sessionEndsAt ? s.sessionEndsAt : s.expiresAt
}

/** "Session ended early": expired although the app knew the sign-in and the set length had more than 30 minutes left. */
export function endedEarly(s: SessionStatus, now: number): boolean {
  if (s.state !== SessionState.SessionExpired || !s.sessionKnown || !s.sessionEndsAt) return false
  return Date.parse(s.sessionEndsAt) > now + 30 * MIN
}

/** Other profiles that share the portal's sign-in: 2 profiles name the other one, 3 or more count the others. */
export function sharedLine(profile: Profile, portal: Portal, active: boolean, t: T): string | null {
  const others = (portal.profiles ?? []).filter((n) => n !== profile.name)
  if (others.length === 0) return null
  if (others.length === 1) return t(active ? 'card.shared.active' : 'card.shared.expired', { other: others[0], portal: portal.name })
  return t('card.shared.many', { n: others.length })
}

/** The static-key profile to migrate first: the global default, else the one with the most bound repos, else the first by name. */
export function primaryMigrateProfile(profiles: Profile[]): Profile | undefined {
  if (profiles.length === 0) return undefined
  const byName = [...profiles].sort((a, b) => a.name.localeCompare(b.name))
  const def = byName.find((p) => p.isGlobalDefault)
  if (def) return def
  let best = byName[0]
  for (const p of byName) if (p.boundRepos > best.boundRepos) best = p
  return best
}

export function impactLine(profile: Profile, t: T): string {
  if (profile.isGlobalDefault) return t('legacy.impact.globalDefault', { n: profile.globalDefaultRepos })
  if (profile.boundRepos > 0) return t('legacy.impact.bound', { n: profile.boundRepos })
  return t('legacy.impact.none')
}

export function staticProfiles(profiles: Profile[]): Profile[] {
  return profiles.filter((p) => p.kind === ProfileKind.ProfileKindStatic)
}

/** An SSO profile whose old key is still in ~/.aws/credentials. */
export function hasKeysLeft(p: Profile): boolean {
  return p.kind === ProfileKind.ProfileKindSSO && (p.staticState === StaticState.StaticKeysLeft || p.hasStaticKeys)
}

/** Remaining time text for the countdown label ("Signed in, 6 h 12 min left"). */
export function activeLabel(s: SessionStatus, now: number, t: T): string {
  const end = endIso(s)
  return t('session.active', { remaining: end ? formatRemaining(end, now, t) : '' })
}

/** The one-line session text of the portal pill. */
export function pillText(s: SessionStatus, now: number, t: T): string {
  switch (pillState(s)) {
    case 'active':
      if (s.sessionKnown && s.sessionOverrun) return t('session.overrunLong')
      if (!s.sessionKnown) return t('session.unknown')
      return activeLabel(s, now, t)
    case 'expiring': {
      const end = endIso(s)
      return t('session.expiring', { m: end ? Math.max(1, Math.ceil(remainingMs(end, now) / MIN)) : 1 })
    }
    case 'refreshable':
      return s.refreshing ? t('session.refreshing') : t('session.refreshable')
    case 'expired':
      return t('session.expired')
    case 'signing-in':
      return t('session.signingIn')
    case 'error':
      return t('session.error')
    default:
      return t('session.signedOut')
  }
}

/** The state line of a card that is not signed in (a signed-in card uses the Countdown). */
export function cardStateText(s: SessionStatus, lang: Lang, t: T): string {
  switch (pillState(s)) {
    case 'expired': {
      const at = s.sessionKnown && s.sessionEndsAt ? s.sessionEndsAt : s.expiresAt
      return at ? t('session.expiredAt', { time: formatDateTime(at, lang) }) : t('session.expired')
    }
    case 'signing-in':
      return t('session.signingIn.card')
    case 'error':
      return t('session.error')
    case 'refreshable':
      return s.refreshing ? t('session.refreshing') : t('session.refreshable')
    default:
      return t('session.signedOut.card')
  }
}

// ------------------------------------------------------------------ names and initials

export const PROFILE_NAME_RE = /^[A-Za-z0-9_.@+=,-]{1,64}$/
export const SESSION_NAME_RE = /^[A-Za-z0-9_.-]{1,64}$/

/** Why a profile name cannot be used, or null. */
export function profileNameProblem(name: string, taken: ReadonlySet<string>): 'invalid' | 'taken' | null {
  if (!PROFILE_NAME_RE.test(name)) return 'invalid'
  if (taken.has(name)) return 'taken'
  return null
}

/** Two-letter preview for a new profile (the backend picks the final one): first letters of the first and last segment. */
export function previewMonogram(name: string): string {
  const segs = name.split(/[-_.]/).filter(Boolean)
  const first = (segs[0] ?? name)[0] ?? '?'
  const second = segs.length >= 2 ? (segs[segs.length - 1][0] ?? '') : ((segs[0] ?? name)[1] ?? '')
  return (first + second).toUpperCase()
}

/** Exactly 2 letters/digits, or one CJK character. The empty string means "automatic". */
export function validMonogram(m: string): boolean {
  if (m === '') return true
  return /^[A-Za-z0-9]{2}$/.test(m) || /^[一-鿿]$/.test(m)
}

// ------------------------------------------------------------------ session length

export const SESSION_LENGTH_PRESETS = [60, 240, 480, 720, 1440, 10080, 43200, 129600] as const
export const SESSION_MIN = 15
export const SESSION_MAX = 129600

/** Hours typed by the user -> whole minutes, or null when outside 15 min .. 90 days. */
export function hoursToMinutes(hours: number): number | null {
  if (!Number.isFinite(hours)) return null
  const m = Math.round(hours * 60)
  return m >= SESSION_MIN && m <= SESSION_MAX ? m : null
}

export function minutesToHours(minutes: number): string {
  return String(Math.round((minutes / 60) * 100) / 100)
}

// ------------------------------------------------------------------ sign-in modal

/** Body of the success state: one profile gets its own sentence, several are listed. */
export function loginSuccessText(names: string[], time: string, lang: Lang, t: T): string {
  if (names.length === 0) return t('login.success.bodyShort', { time })
  if (names.length === 1) return t('login.success.bodyOne', { time, profile: names[0] })
  return t('login.success.body', { time, profiles: names.join(lang === 'zh-CN' ? '、' : ', ') })
}

/** Error text of the sign-in modal. A network failure gets its own title and body instead of a body that repeats the title. */
export function loginErrorText(e: UiError, t: T): { title: string; body?: string; detail?: string } {
  if (e.code === ErrorCode.ErrNetwork) return { title: t('login.error.network.title'), body: t('login.error.network.body'), detail: e.detail }
  return errorText(e, t)
}

/** Split a paragraph after its first sentence ("。" or ". ") so the two halves can sit on separate lines. */
export function splitFirstSentence(text: string): [string, string] {
  const m = /^(.+?[。！？]|.+?[.!?](?=\s))\s*(.*)$/s.exec(text)
  return m && m[2] ? [m[1], m[2]] : [text, '']
}
