// Pure helpers for the repositories page: row classification, filtering, bulk bind planning, profile choices.
import { BindingSource, OwnerStatus, ProfileKind, RemoteKind, SessionState, StaticState, TestStatus } from '../../api/models'
import type { BindItem, OwnerCandidate, Portal, Profile, Repo, RepoCounts, TestResult } from '../../api/models'
import type { MessageKey, T } from '../../i18n'
import type { RepoFilter } from '../../lib/route'

export type RowKind =
  | 'checking'
  | 'ssh'
  | 'grc'
  | 'repair'
  | 'legacy'
  | 'bound'
  | 'ambiguous'
  | 'url-rule'
  | 'not-found'
  | 'partial'
  | 'global-default'
  | 'guard'
  | 'none'

/**
 * Precedence: checking > ssh > grc > repair > legacy > bound > ambiguous > url-rule > not-found / partial >
 * global-default / guard / none.
 */
export function rowKind(repo: Repo): RowKind {
  const b = repo.binding
  if (repo.owner.status === OwnerStatus.OwnerChecking) return 'checking'
  if (repo.remoteKind === RemoteKind.RemoteSSH) return 'ssh'
  if (repo.remoteKind === RemoteKind.RemoteGRC || b.source === BindingSource.BindingRemoteURL) return 'grc'
  if (b.source === BindingSource.BindingLocal) {
    if (b.needsRepair) return 'repair'
    if (b.legacyPlain) return 'legacy'
    return 'bound'
  }
  if (repo.owner.status === OwnerStatus.OwnerAmbiguous) return 'ambiguous'
  if (b.source === BindingSource.BindingGlobalURL) return 'url-rule'
  if (repo.owner.status === OwnerStatus.OwnerNone) return 'not-found'
  if (repo.owner.status === OwnerStatus.OwnerPartial) return 'partial'
  if (b.source === BindingSource.BindingGlobalDefault) return 'global-default'
  if (b.source === BindingSource.BindingGuard) return 'guard'
  return 'none'
}

/** "To fix" while something is left to do, otherwise everything. */
export function defaultFilter(counts: RepoCounts): RepoFilter {
  return counts.toFix > 0 ? 'todo' : 'all'
}

export function filterRepos(repos: Repo[], filter: RepoFilter, profile: string | undefined, query: string): Repo[] {
  const q = query.trim().toLowerCase()
  return repos.filter((r) => {
    if (filter === 'todo' && !r.toFix) return false
    if (filter === 'bound' && r.toFix) return false
    if (profile && r.binding.profile !== profile) return false
    if (q && !r.name.toLowerCase().includes(q) && !r.displayPath.toLowerCase().includes(q)) return false
    return true
  })
}

/** A repository whose remote and binding allow a bind: HTTPS, not named by git-remote-codecommit. */
export function canBind(repo: Repo): boolean {
  return repo.remoteKind === RemoteKind.RemoteHTTPS && repo.binding.source !== BindingSource.BindingRemoteURL && repo.owner.status !== OwnerStatus.OwnerChecking
}

/** Rows with a checkbox: HTTPS remotes that are not being looked up. */
export function canSelect(repo: Repo): boolean {
  return canBind(repo) && repo.owner.status !== OwnerStatus.OwnerChecking
}

/** Rows the bulk bind acts on: bindable and not already bound correctly. */
export function needsBinding(repo: Repo): boolean {
  return canBind(repo) && repo.toFix
}

export function isSSO(p: Profile | undefined): boolean {
  return p?.kind === ProfileKind.ProfileKindSSO
}

/** The candidate that is safe to pick without asking: the only one, or the single one whose HEAD matches the local clone. */
export function resolvedCandidate(repo: Repo): OwnerCandidate | undefined {
  const cands = repo.owner.candidates ?? []
  if (repo.owner.status === OwnerStatus.OwnerSingle && cands.length === 1) return cands[0]
  if (repo.owner.status === OwnerStatus.OwnerAmbiguous) {
    const hits = cands.filter((c) => c.headMatch)
    if (hits.length === 1) return hits[0]
  }
  return undefined
}

/** The profile to preselect for a repository, or undefined when the user has to choose. */
export function suggestedProfile(repo: Repo): string | undefined {
  const b = repo.binding
  const kind = rowKind(repo)
  if ((kind === 'url-rule' || kind === 'legacy' || kind === 'repair') && b.profile && b.profileKnown) return b.profile
  return resolvedCandidate(repo)?.profile
}

export interface BindAllPlan {
  items: BindItem[]
  /** Rows listed as skipped: several possible accounts and no unique match. */
  ambiguous: Repo[]
}

/** Owner is single (or uniquely HEAD-matched) and the suggested profile is an SSO profile. */
export function bindAllPlan(repos: Repo[], profiles: Profile[]): BindAllPlan {
  const byName = new Map(profiles.map((p) => [p.name, p]))
  const items: BindItem[] = []
  const ambiguous: Repo[] = []
  for (const r of repos) {
    if (!needsBinding(r)) continue
    const cand = resolvedCandidate(r)
    if (cand) {
      if (isSSO(byName.get(cand.profile))) items.push({ repoId: r.id, profile: cand.profile })
    } else if (r.owner.status === OwnerStatus.OwnerAmbiguous) {
      ambiguous.push(r)
    }
  }
  return { items, ambiguous }
}

/** "acme 1 个，globex 1 个": profile names in config order, then unknown names alphabetically. */
export function bulkBreakdown(items: BindItem[], profiles: Profile[], t: T): string {
  const counts = new Map<string, number>()
  for (const it of items) counts.set(it.profile, (counts.get(it.profile) ?? 0) + 1)
  const order = profiles.map((p) => p.name).filter((n) => counts.has(n))
  const rest = [...counts.keys()].filter((n) => !order.includes(n)).sort()
  return [...order, ...rest].map((name) => t('repos.bulk.breakdownItem', { name, n: counts.get(name) ?? 0 })).join(t('repos.bulk.breakdownSep'))
}

/** "将绑定 2 个仓库：acme 1 个，globex 1 个" */
export function bulkSummary(items: BindItem[], profiles: Profile[], t: T): string {
  return t('repos.bulk.summary', { n: items.length, breakdown: bulkBreakdown(items, profiles, t) })
}

// ---------------------------------------------------------------- profiles

/** Profiles a repository can be bound to: SSO first (config order), then access-key profiles. */
export function bindableProfiles(profiles: Profile[]): Profile[] {
  const rank = (p: Profile) => (p.kind === ProfileKind.ProfileKindStatic ? 1 : 0)
  return profiles.filter((p) => p.kind === ProfileKind.ProfileKindSSO || p.kind === ProfileKind.ProfileKindLegacySSO || p.kind === ProfileKind.ProfileKindStatic).map((p, i) => ({ p, i })).sort((a, b) => rank(a.p) - rank(b.p) || a.i - b.i).map((x) => x.p)
}

export function sessionUsable(state: SessionState | undefined): boolean {
  return state === SessionState.SessionActive || state === SessionState.SessionExpiring
}

/** Sign-in is needed (or failed): expired, signed out or unreadable. A token that can still be refreshed counts as signed in. */
export function sessionNeedsLogin(state: SessionState | undefined): boolean {
  return state === SessionState.SessionExpired || state === SessionState.SessionSignedOut || state === SessionState.SessionError
}

export function portalOf(profile: Profile | undefined, portals: Portal[]): Portal | undefined {
  if (!profile?.portal) return undefined
  return portals.find((p) => p.name === profile.portal)
}

/** A profile that can call AWS right now: an access-key profile that AWS accepted, or an SSO profile whose portal is signed in. */
export function profileUsable(profile: Profile, portals: Portal[]): boolean {
  if (profile.kind === ProfileKind.ProfileKindStatic) return profile.staticState !== StaticState.StaticInvalid
  if (profile.kind === ProfileKind.ProfileKindSSO) return sessionUsable(portalOf(profile, portals)?.session.state)
  return false
}

// ---------------------------------------------------------------- test results

const TEST_KEYS: Record<TestStatus, MessageKey> = {
  [TestStatus.$zero]: 'repos.test.error',
  [TestStatus.TestOK]: 'repos.test.ok',
  [TestStatus.TestAuth]: 'repos.test.auth',
  [TestStatus.TestSignedOut]: 'repos.test.signedOut',
  [TestStatus.TestNotFound]: 'repos.test.notFound',
  [TestStatus.TestNetwork]: 'repos.test.network',
  [TestStatus.TestTimeout]: 'repos.test.timeout',
  [TestStatus.TestError]: 'repos.test.error',
}

export function testText(res: TestResult, t: T): string {
  return t(TEST_KEYS[res.status] ?? 'repos.test.error', { name: res.profile ?? '', portal: res.portal ?? '' })
}

/** Label for one profile in a select: "acme · 111122223333", or "acme · not identified" while the account is unknown. */
export function profileLabel(p: Profile, notIdentified: string): string {
  return `${p.name} · ${p.accountId ? p.accountId : notIdentified}`
}

/**
 * Split a long path or remote into a shrinkable head and a fixed tail so CSS can ellipsise the middle.
 * The tail is the last path segment (with its separator) when that is short enough, otherwise the last `tail` characters.
 */
export function splitMiddle(value: string, tail = 18): { head: string; tail: string } {
  if (value.length <= tail) return { head: '', tail: value }
  const cut = Math.max(value.lastIndexOf('/'), value.lastIndexOf('\\'), value.lastIndexOf(':'))
  if (cut > 0 && value.length - cut <= tail + 6) return { head: value.slice(0, cut), tail: value.slice(cut) }
  return { head: value.slice(0, value.length - tail), tail: value.slice(value.length - tail) }
}

/**
 * The value with its middle replaced by an ellipsis, only when the whole value does not fit `avail` (in the units of
 * `measure`). The tail from `splitMiddle` is kept whole and the ellipsis sits right against it: no space between them.
 */
export function fitMiddle(value: string, avail: number, measure: (s: string) => number): string {
  if (avail <= 0 || measure(value) <= avail) return value
  const { head, tail } = splitMiddle(value)
  if (!head) return value
  let lo = 0
  let hi = head.length - 1
  while (lo < hi) {
    const mid = Math.ceil((lo + hi) / 2)
    if (measure(head.slice(0, mid) + '…' + tail) <= avail) lo = mid
    else hi = mid - 1
  }
  return head.slice(0, lo) + '…' + tail
}
