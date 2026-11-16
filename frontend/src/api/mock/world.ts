// The in-memory mock world. Services mutate it like the backend would and call recompute() so the derived
// fields (counts, per-portal profile lists, session status, binding colours) are always consistent.
import { initialUI } from '../../lib/initialUI'
import type { ScenarioId } from '../../lib/initialUI'
import { GIT_MIN_VERSION } from '../../lib/toolVersions'
import {
  BackupKind,
  BackupReason,
  BindingSource,
  DiffKind,
  ErrorCode,
  HealthKind,
  Language,
  MigrationStep,
  MotionPref,
  OwnerStatus,
  ProfileKind,
  RemoteKind,
  RepoProvider,
  SavedCredentialStore,
  SessionState,
  Severity,
  StaticState,
  SuggestionKind,
  TerminalApp,
  TestStatus,
  Theme,
  ToolStatus,
} from '../models'
import type {
  AccountsState,
  AppError,
  AppInfo,
  BackupEntry,
  Binding,
  DiscoveredAccount,
  DiffLine,
  EnvReport,
  FilePreview,
  GlobalDefaultInfo,
  GuardInfo,
  HealthReport,
  HelperEntry,
  MigrationState,
  OwnerCandidate,
  Portal,
  Profile,
  ProjectInfo,
  ProjectSuggestion,
  RemoteRepo,
  Repo,
  ReposState,
  RepoWarning,
  SavedCredential,
  SessionStatus,
  Settings,
  TestResult,
  StaticIdentity,
  URLRule,
} from '../models'
import { mockEmit } from '../events'
import { HOUR, MIN, ago, deepCopy, iso, now } from './clock'

// ------------------------------------------------------------------ constants

export const SCAN_ROOT = '/Users/alex/Documents/code'
export const SCAN_ROOT_DISPLAY = '~/Documents/code'
export const HOME = '/Users/alex'
/** Home region of each fictional organisation (Identity Center instance and profiles). */
export const REGION_ACME = 'us-east-1'
export const REGION_INITECH = 'us-west-2'
export const REGION_GLOBEX = 'eu-west-1'
/** Default CodeCommit region (the acme region); repositories of the other organisations live in their own region. */
export const CC_REGION = REGION_ACME
export const ccHost = (region: string = CC_REGION): string => `git-codecommit.${region}.amazonaws.com`
export const CC_HOST = ccHost()

// Fictional organisations. Account e-mail addresses are left empty on purpose.
export const ACCT = {
  initech: { id: '777788889999', name: 'initech', slot: 3, email: '' },
  acme: { id: '111122223333', name: 'acme-prod', slot: 1, email: '' },
  globex: { id: '444455556666', name: 'globex', slot: 6, email: '' },
} as const
export type AcctKey = keyof typeof ACCT

export const PORTAL_INITECH = { name: 'initech', startUrl: 'https://d-1234567890.awsapps.com/start', host: 'd-1234567890.awsapps.com/start', region: REGION_INITECH }
/** acme and globex each have their own organisation and Identity Center portal. */
export const PORTAL_ACME = { name: 'acme', startUrl: 'https://d-1111111111.awsapps.com/start', host: 'd-1111111111.awsapps.com/start', region: REGION_ACME }
export const PORTAL_GLOBEX = { name: 'globex', startUrl: 'https://d-2222222222.awsapps.com/start', host: 'd-2222222222.awsapps.com/start', region: REGION_GLOBEX }
/** The portal the `fresh` migration wizard creates on the fly for the acme profile. */
export const FRESH_DRAFT_PORTAL = PORTAL_ACME
/** The portal a static profile migrates into (its own organisation). */
export const PORTAL_OF_STATIC: Record<string, { name: string; startUrl: string; region: string }> = { acme: PORTAL_ACME, globex: PORTAL_GLOBEX }

export const STATIC_TRUTH: Record<string, { account: AcctKey; iamUser: string; masked: string; region: string }> = {
  acme: { account: 'acme', iamUser: 'acme-dev', masked: 'AKIA••••••••••••3F2A', region: REGION_ACME },
  globex: { account: 'globex', iamUser: 'globex-ci', masked: 'AKIA••••••••••••9C1D', region: REGION_GLOBEX },
}

/** Home region of an account's CodeCommit repositories. */
export const REGION_OF_ACCT: Record<'acme' | 'initech' | 'globex', string> = { acme: REGION_ACME, initech: REGION_INITECH, globex: REGION_GLOBEX }

export const AUTO_SLOT_ORDER = [1, 3, 2, 6, 5, 4, 7]

// ------------------------------------------------------------------ types

export type SessionFile = 'present' | 'missing' | 'invalid'
export interface SessionModel {
  file: SessionFile
  tokenExpiresAt?: number
  issuedAt?: number
  canRefresh: boolean
  refreshFailed: boolean
  refreshing: boolean
  signingIn: boolean
  known: boolean
  loginAt?: number
  sessionMinutes: number
  error?: AppError
}

export interface OwnerTruth {
  /** account ids that really hold this repository */
  accounts: string[]
  /** account whose HEAD equals the local checkout (CompareCommits) */
  headMatch?: string
}

export interface MovedRecord {
  projectId?: string
  hidden: boolean
  lastTest?: TestResult
  identity?: { name: string; email: string }
}

export interface GitState {
  globalDefault: { profile: string } | null
  urlRules: Array<{ pattern: string; profile: string }>
  guardInstalled: boolean
  baseHash: string
  saved: SavedCredential[]
}

export interface World {
  scenario: ScenarioId
  portalModels: Record<string, SessionModel>
  accounts: AccountsState
  discovery: Record<string, DiscoveredAccount[]>
  draftDiscovery: DiscoveredAccount[]
  repos: Repo[]
  /** Projects of the repository list (F1-11). Rows carry projectId; hidden and pending flags live on the rows too. */
  projects: Array<Omit<ProjectInfo, 'repoCount'>>
  dismissedSuggestions: string[]
  repoWarnings: RepoWarning[]
  /** The mock's scan finds the "newly found" repositories once (F1-11). */
  foundSeeded: boolean
  /** What the record of a vanished folder held (FU-11c), by its old ID: ReattachMoved gives it back to the folder found at the new place. */
  movedRecords: Record<string, MovedRecord>
  /** A repository's own author identity, as the app recorded it (only kept here so a re-attached row gets it back). */
  repoIdentity: Record<string, { name: string; email: string }>
  truth: Record<string, OwnerTruth>
  repoMeta: { scannedAt: string; scanning: boolean; detecting: boolean }
  awsCliReady: boolean
  git: GitState
  settings: Settings
  appInfo: AppInfo
  env: EnvReport
  health: HealthReport
  backups: BackupEntry[]
  migrations: Record<string, MigrationState>
  remote: Record<string, RemoteRepo[]>
  hashCounter: number
}

// ------------------------------------------------------------------ small helpers

export function fakeHash16(s: string): string {
  let h1 = 0x811c9dc5
  let h2 = 0x01000193
  for (let i = 0; i < s.length; i++) {
    h1 = Math.imul(h1 ^ s.charCodeAt(i), 0x01000193) >>> 0
    h2 = Math.imul(h2 + s.charCodeAt(i), 0x85ebca6b) >>> 0
  }
  return (h1.toString(16).padStart(8, '0') + h2.toString(16).padStart(8, '0')).slice(0, 16)
}

export function mkError(code: ErrorCode, message: string, params?: Record<string, string>, detail?: string): AppError {
  const e: AppError = { code, message }
  if (params) e.params = params
  if (detail) e.detail = detail
  return e
}

export function slugRole(role: string): string {
  return role.toLowerCase().replace(/fullaccess$|access$/, '') || role.toLowerCase()
}

/** Two-letter monogram from the name (simplified, enough for fictional data). */
export function autoMonogram(name: string, taken: Set<string> = new Set()): string {
  const segs = name.split(/[-_.]/).filter(Boolean)
  const first = (segs[0] ?? name)[0] ?? 'X'
  const pool = segs.length >= 2 ? [...segs[segs.length - 1]] : [...(segs[0] ?? name)].slice(1)
  for (const c of pool) {
    const m = (first + c).toUpperCase()
    if (!taken.has(m)) return m
  }
  for (let d = 2; d <= 9; d++) {
    const m = (first + String(d)).toUpperCase()
    if (!taken.has(m)) return m
  }
  return (first + '0').toUpperCase()
}

function mkPreview(w: World, file: string, path: string, lines: DiffLine[]): FilePreview {
  w.hashCounter++
  return {
    file,
    path,
    baseHash: `mock-${file.replace(/\W+/g, '')}-${w.hashCounter}`,
    lines,
    added: lines.filter((l) => l.kind === DiffKind.DiffAdded).length,
    removed: lines.filter((l) => l.kind === DiffKind.DiffRemoved).length,
    backupDir: backupDirDisplay(w),
  }
}
export { mkPreview }

export const ctx = (text: string): DiffLine => ({ kind: DiffKind.DiffContext, text })
export const add = (text: string): DiffLine => ({ kind: DiffKind.DiffAdded, text })
export const del = (text: string): DiffLine => ({ kind: DiffKind.DiffRemoved, text })
export const fold = (count: number): DiffLine => ({ kind: DiffKind.DiffFolded, text: '', count })

export function backupDirDisplay(w: World): string {
  return w.appInfo.backupDir
}

// ------------------------------------------------------------------ session model -> SessionStatus

export function newSessionModel(over: Partial<SessionModel> = {}): SessionModel {
  return {
    file: 'missing',
    canRefresh: false,
    refreshFailed: false,
    refreshing: false,
    signingIn: false,
    known: false,
    sessionMinutes: 480,
    ...over,
  }
}

export function computeSession(portal: string, m: SessionModel, at: number = now()): SessionStatus {
  const tokenExpired = m.tokenExpiresAt !== undefined && m.tokenExpiresAt <= at
  const tokenOK = m.file === 'present' && m.tokenExpiresAt !== undefined && (!tokenExpired || (m.canRefresh && !m.refreshFailed))
  const known = m.file === 'present' && m.known && m.loginAt !== undefined
  const endsAt = known ? (m.loginAt as number) + m.sessionMinutes * MIN : undefined
  let overrun = false
  let state: SessionState
  if (m.signingIn) state = SessionState.SessionSigningIn
  else if (m.file === 'missing') state = SessionState.SessionSignedOut
  else if (m.file === 'invalid') state = SessionState.SessionError
  else if (!tokenOK) state = SessionState.SessionExpired
  else if (known && endsAt !== undefined) {
    const left = endsAt - at
    if (left >= 15 * MIN) state = tokenExpired ? SessionState.SessionRefreshable : SessionState.SessionActive
    else if (left > 0) state = SessionState.SessionExpiring
    else {
      overrun = true
      state = tokenExpired ? SessionState.SessionRefreshable : SessionState.SessionActive
    }
  } else if (tokenExpired) state = SessionState.SessionRefreshable
  else if ((m.tokenExpiresAt as number) - at < 15 * MIN && !m.canRefresh) state = SessionState.SessionExpiring
  else state = SessionState.SessionActive

  let progressPct = 0
  if (known && !overrun && endsAt !== undefined) {
    const leftMin = Math.max(0, Math.ceil((endsAt - at) / MIN))
    progressPct = Math.min(100, Math.max(0, Math.round((leftMin / m.sessionMinutes) * 100)))
  }
  const s: SessionStatus = {
    portal,
    state,
    sessionKnown: known,
    sessionEndExact: false, // the mock's end is always the estimate loginAt + sessionMinutes
    sessionMinutes: m.sessionMinutes,
    sessionOverrun: overrun,
    progressPct,
    canRefresh: m.canRefresh && m.file === 'present',
    refreshing: m.refreshing,
  }
  if (m.file !== 'missing' && m.tokenExpiresAt !== undefined) s.expiresAt = iso(m.tokenExpiresAt)
  if (m.issuedAt !== undefined && m.file === 'present') s.issuedAt = iso(m.issuedAt)
  if (known && m.loginAt !== undefined && endsAt !== undefined) {
    s.loginAt = iso(m.loginAt)
    s.sessionEndsAt = iso(endsAt)
  }
  if (state === SessionState.SessionError) s.error = m.error ?? mkError(ErrorCode.ErrFileIO, 'cannot read token file', { file: '~/.aws/sso/cache' })
  return s
}

/** Signed in right now by this app (the sign-in time is known). */
export function signedInModel(over: Partial<SessionModel> = {}): SessionModel {
  const t = now()
  return newSessionModel({
    file: 'present',
    known: true,
    loginAt: t - (HOUR + 48 * MIN),
    tokenExpiresAt: t + 42 * MIN,
    issuedAt: t - 18 * MIN,
    canRefresh: true,
    ...over,
  })
}

export function isUsable(state: SessionState): boolean {
  return state === SessionState.SessionActive || state === SessionState.SessionExpiring
}

// ------------------------------------------------------------------ world accessors

export function findProfile(w: World, name: string): Profile | undefined {
  return w.accounts.profiles?.find((p) => p.name === name)
}
export function allProfiles(w: World): Profile[] {
  return w.accounts.profiles ?? []
}
export function allPortals(w: World): Portal[] {
  return w.accounts.portals ?? []
}
export function findPortal(w: World, name: string): Portal | undefined {
  return allPortals(w).find((p) => p.name === name)
}
export function portalState(w: World, portal: string): SessionState {
  const m = w.portalModels[portal]
  return m ? computeSession(portal, m).state : SessionState.SessionSignedOut
}
export function acctKeyOfId(id: string): AcctKey | undefined {
  return (Object.keys(ACCT) as AcctKey[]).find((k) => ACCT[k].id === id)
}
export function colorSlotForAccount(w: World, accountId: string): number {
  const existing = allProfiles(w).find((p) => p.accountId === accountId)
  if (existing) return existing.colorSlot
  const k = acctKeyOfId(accountId)
  if (k) return ACCT[k].slot
  const used = new Set(allProfiles(w).map((p) => p.colorSlot))
  return AUTO_SLOT_ORDER.find((s) => !used.has(s)) ?? AUTO_SLOT_ORDER[0]
}

// ------------------------------------------------------------------ recompute (derived fields)

/** profile -> account id, including static profiles that were not identified yet (the mock knows the truth). */
export function accountIdOfProfile(p: Profile): string | undefined {
  if (p.accountId) return p.accountId
  const t = STATIC_TRUTH[p.name]
  return t ? ACCT[t.account].id : undefined
}

export function repoToFix(r: Repo): boolean {
  // Only a CodeCommit repository can need a binding; GitHub, Bitbucket and other rows follow the machine's own git setup.
  if (r.provider !== RepoProvider.ProviderCodeCommit) return false
  const b = r.binding
  return !((b.source === BindingSource.BindingLocal && !b.legacyPlain && !b.needsRepair && b.profileKnown) || b.source === BindingSource.BindingRemoteURL)
}

export function recompute(w: World): void {
  const at = now()
  const profiles = w.accounts.profiles ?? []
  const portals = w.accounts.portals ?? []

  // sessions
  for (const p of portals) {
    if (p.legacyFormat) continue
    const m = w.portalModels[p.name] ?? (w.portalModels[p.name] = newSessionModel())
    p.session = computeSession(p.name, m, at)
  }
  // repo bindings
  const def = w.git.globalDefault
  for (const r of w.repos) {
    const b = r.binding
    if (b.source === BindingSource.BindingGlobalDefault) b.profile = def?.profile ?? b.profile
    if (b.profile) {
      const p = profiles.find((x) => x.name === b.profile)
      b.profileKnown = !!p
      b.accountId = p ? accountIdOfProfile(p) : undefined
      b.colorSlot = p?.colorSlot ?? 0
      b.monogram = p?.monogram ?? autoMonogram(b.profile)
    } else {
      b.profileKnown = false
      b.accountId = undefined
      b.colorSlot = 0
      b.monogram = ''
    }
    r.toFix = repoToFix(r)
  }
  // profiles
  for (const p of profiles) {
    p.boundRepos = w.repos.filter((r) => r.binding.source === BindingSource.BindingLocal && r.binding.profile === p.name).length
    p.isGlobalDefault = def?.profile === p.name
    p.globalDefaultRepos = p.isGlobalDefault ? w.repos.filter((r) => r.binding.source === BindingSource.BindingGlobalDefault).length : 0
    p.hasStaticKeys = p.kind === ProfileKind.ProfileKindStatic || p.staticState === StaticState.StaticKeysLeft
    p.migrationInProgress = !!w.migrations[p.name]
  }
  for (const portal of portals) {
    portal.profiles = profiles.filter((p) => p.portal === portal.name).map((p) => p.name)
  }
  w.accounts.staticCount = profiles.filter((p) => p.kind === ProfileKind.ProfileKindStatic).length
}

// ------------------------------------------------------------------ derived views

export function globalDefaultInfo(w: World): GlobalDefaultInfo | undefined {
  const d = w.git.globalDefault
  if (!d) return undefined
  const p = findProfile(w, d.profile)
  return {
    profile: d.profile,
    profileKind: p?.kind,
    colorSlot: p?.colorSlot ?? 0,
    monogram: p?.monogram ?? autoMonogram(d.profile),
    helper: `!aws codecommit credential-helper $@ --profile ${d.profile}`,
    affectedRepos: w.repos.filter((r) => r.binding.source === BindingSource.BindingGlobalDefault).length,
  }
}

/** Name before the first "_", "-" or "." (at least two characters), "" when there is none. Mirrors the Go engine. */
export function namePrefix(name: string): string {
  const i = name.search(/[_.-]/)
  return i < 2 ? '' : name.slice(0, i)
}

const MIN_GROUP = 3

/** Project suggestions for listed repositories that are in no project (three or more share a prefix or a parent folder). */
export function suggestionsFor(w: World, listed: Repo[]): ProjectSuggestion[] {
  const pre = new Map<string, { label: string; ids: string[] }>()
  const dir = new Map<string, { label: string; ids: string[] }>()
  for (const r of listed) {
    if (r.projectId) continue
    const p = namePrefix(r.localName)
    if (p) {
      const g = pre.get(p.toLowerCase()) ?? { label: p, ids: [] }
      g.ids.push(r.id)
      pre.set(p.toLowerCase(), g)
    }
    const parent = r.path.slice(0, Math.max(0, r.path.lastIndexOf('/')))
    if (parent && parent !== SCAN_ROOT && parent.includes('/')) {
      const g = dir.get(parent) ?? { label: parent.split('/').pop() as string, ids: [] }
      g.ids.push(r.id)
      dir.set(parent, g)
    }
  }
  const collect = (m: Map<string, { label: string; ids: string[] }>, kind: SuggestionKind): ProjectSuggestion[] =>
    [...m.entries()]
      .map(([k, g]) => ({ key: `${kind}:${k}`, kind, label: g.label, repoIds: [...g.ids].sort() }))
      .filter((x) => x.repoIds.length >= MIN_GROUP && !w.dismissedSuggestions.includes(x.key))
      .sort((a, b) => b.repoIds.length - a.repoIds.length || a.key.localeCompare(b.key))
  return [...collect(pre, SuggestionKind.SuggestPrefix), ...collect(dir, SuggestionKind.SuggestFolder)]
}

export function reposState(w: World): ReposState {
  recompute(w)
  const byName = (a: Repo, b: Repo) => a.name.localeCompare(b.name) || a.displayPath.localeCompare(b.displayPath)
  const listed = w.repos.filter((r) => !r.hidden && !r.pending)
  const found = w.repos.filter((r) => r.pending && !r.hidden)
  const hiddenRepos = w.repos.filter((r) => r.hidden)
  const all = listed.length
  const cc = listed.filter((r) => r.provider === RepoProvider.ProviderCodeCommit)
  const toFix = cc.filter((r) => r.toFix).length
  const repos = [...listed].sort(byName)
  const known = new Set(w.projects.map((p) => p.id))
  const count = (id: string) => listed.filter((r) => r.projectId === id).length
  const s: ReposState = {
    roots: [SCAN_ROOT],
    rootsDisplay: [SCAN_ROOT_DISPLAY],
    scannedAt: w.repoMeta.scannedAt || undefined,
    scanning: w.repoMeta.scanning,
    detecting: w.repoMeta.detecting,
    repos: deepCopy(repos.map((r) => (r.projectId && !known.has(r.projectId) ? { ...r, projectId: undefined } : r))),
    counts: { all, toFix, bound: cc.length - toFix, found: found.length, hidden: hiddenRepos.length },
    awsCliReady: w.awsCliReady,
    needsRepair: listed.filter((r) => r.binding.source === BindingSource.BindingLocal && r.binding.needsRepair).length,
    found: deepCopy([...found].sort(byName)),
    hiddenRepos: deepCopy([...hiddenRepos].sort(byName)),
    projects: [...w.projects].sort((a, b) => a.order - b.order || a.name.localeCompare(b.name)).map((p) => ({ ...p, repoCount: count(p.id) })),
    suggestions: suggestionsFor(w, listed),
    warnings: deepCopy(w.repoWarnings),
  }
  const g = globalDefaultInfo(w)
  if (g) s.globalDefault = g
  return s as ReposState
}

export function accountsState(w: World): AccountsState {
  recompute(w)
  return deepCopy(w.accounts)
}

export function guardInfo(w: World): GuardInfo {
  return { installed: w.git.guardInstalled, patterns: w.git.guardInstalled ? ['https://git-codecommit.*.amazonaws.com'] : [] }
}

export function urlRules(w: World): URLRule[] {
  return w.git.urlRules.map((r) => {
    const p = findProfile(w, r.profile)
    return {
      pattern: r.pattern,
      profile: r.profile,
      profileKnown: !!p,
      colorSlot: p?.colorSlot ?? 0,
      monogram: p?.monogram ?? autoMonogram(r.profile),
      matchedRepoIds: w.repos.filter((x) => x.binding.source === BindingSource.BindingGlobalURL && x.binding.urlRule === r.pattern).map((x) => x.id),
      extraKeys: false,
    }
  })
}

export function helperEntries(w: World): { other: HelperEntry[]; lines: HelperEntry[] } {
  const other: HelperEntry[] = [
    { scope: 'system', origin: '/opt/homebrew/etc/gitconfig', key: 'credential.helper', value: 'osxkeychain' },
    { scope: 'global', origin: '~/.gitconfig', key: 'credential.https://github.com.helper', value: '!/opt/homebrew/bin/gh auth git-credential' }, // placeholder path
  ]
  const lines: HelperEntry[] = [...other]
  if (w.git.globalDefault) {
    lines.push({ scope: 'global', origin: '~/.gitconfig', key: 'credential.helper', value: `!aws codecommit credential-helper $@ --profile ${w.git.globalDefault.profile}` })
    lines.push({ scope: 'global', origin: '~/.gitconfig', key: 'credential.usehttppath', value: 'true' })
  }
  for (const r of w.git.urlRules) {
    lines.push({ scope: 'global', origin: '~/.gitconfig', key: `credential.${r.pattern}.helper`, value: `!aws codecommit credential-helper $@ --profile ${r.profile}` })
  }
  if (w.git.guardInstalled) {
    lines.push({ scope: 'global', origin: '~/.gitconfig', key: 'credential.https://git-codecommit.*.amazonaws.com.helper', value: guardText(w) })
  }
  return { other, lines }
}

export function guardText(w: World): string {
  const zh = w.appInfo.uiLanguage === 'zh-CN'
  return zh
    ? `!f() { test "$1" = get || return 0; echo "AWS SSO Hub: 这个仓库还没有绑定 AWS 账号。打开 AWS SSO Hub > 仓库，绑定后再试。" >&2; echo quit=1; }; f`
    : `!f() { test "$1" = get || return 0; echo "AWS SSO Hub: this repository is not bound to an AWS account (no CodeCommit credential helper). Open AWS SSO Hub > Repositories and bind it." >&2; echo quit=1; }; f`
}

// ------------------------------------------------------------------ backups

export function addBackup(w: World, kind: BackupKind, reason: BackupReason, params: Record<string, string>, opts: { path?: string; file?: string; createdAt?: string; redacted?: boolean } = {}): BackupEntry {
  const t = opts.createdAt ?? iso(now())
  const stamp = t.replace(/[-:]/g, '')
  const path =
    opts.path ??
    (kind === BackupKind.BackupAWSConfig
      ? `${HOME}/.aws/config`
      : kind === BackupKind.BackupAWSCredentials
        ? `${HOME}/.aws/credentials`
        : kind === BackupKind.BackupGitGlobal
          ? `${HOME}/.gitconfig`
          : `${SCAN_ROOT}/${params.repo ?? 'repo'}/.git/config`)
  const e: BackupEntry = {
    id: `${stamp}-${kind}-${fakeHash16(stamp + kind + w.backups.length).slice(0, 4)}`,
    createdAt: t,
    path,
    file: opts.file ?? path.replace(HOME, '~'),
    kind,
    reason,
    reasonParams: params,
    size: 800 + ((fakeHash16(path).charCodeAt(0) * 37) % 3000),
    redacted: opts.redacted ?? (kind === BackupKind.BackupAWSConfig || kind === BackupKind.BackupAWSCredentials ? kind === BackupKind.BackupAWSCredentials : false),
  }
  w.backups.push(e)
  return e
}

// ------------------------------------------------------------------ repos

export interface RepoDef {
  n: number
  name: string
  dir?: string
  owners: AcctKey[]
  headMatch?: AcctKey
}

export const REPO_DEFS: RepoDef[] = [
  { n: 1, name: 'acme_cs_portal', owners: ['acme'] },
  { n: 2, name: 'acme_cs_portal', dir: 'archive/acme_cs_portal', owners: ['acme', 'globex'], headMatch: 'globex' }, // placeholder path
  { n: 3, name: 'acme_api', owners: ['acme'] },
  { n: 4, name: 'acme_cs_portal_v2', owners: ['acme'] },
  { n: 5, name: 'acme_agent_portal', owners: ['acme'] },
  { n: 6, name: 'acme_auth', owners: ['acme'] },
  { n: 7, name: 'acme_search', owners: ['acme'] },
  { n: 8, name: 'acme_data', owners: ['acme'] },
  { n: 9, name: 'globex_backend', owners: ['globex'] },
  { n: 10, name: 'initech_dashboard', owners: ['initech'] },
  { n: 11, name: 'acme_design', owners: ['acme'] },
  { n: 12, name: 'initech_infra', owners: ['initech'] },
  { n: 13, name: 'acme_docs', owners: ['acme'] },
  { n: 14, name: 'acme_handbook', owners: ['acme'] },
  { n: 15, name: 'initech_site', owners: ['initech'] },
  { n: 16, name: 'acme_changelog', owners: ['acme'] },
]

export function httpsUrl(name: string, region = CC_REGION): string {
  return `https://git-codecommit.${region}.amazonaws.com/v1/repos/${name}`
}

export function emptyBinding(source: BindingSource, profile?: string): Binding {
  return { source, profile, profileKnown: false, colorSlot: 0, monogram: '', legacyPlain: false, needsRepair: false, noProfileFlag: false }
}

export function emptyOwner(): Repo['owner'] {
  return { status: OwnerStatus.OwnerUnknown, candidates: [], checkedWith: 0, skipped: 0, skippedPortals: [] }
}

export function mkRepo(path: string, name: string, binding: Binding, region: string = CC_REGION): Repo {
  return {
    id: fakeHash16(path),
    path,
    displayPath: path.replace(HOME, '~'),
    name,
    localName: path.split('/').pop() || name,
    remoteName: name,
    provider: RepoProvider.ProviderCodeCommit,
    hidden: false,
    pending: false,
    remote: 'origin',
    remoteUrl: httpsUrl(name, region),
    remoteKind: RemoteKind.RemoteHTTPS,
    host: ccHost(region),
    region,
    httpsUrl: httpsUrl(name, region),
    hasUserinfo: false,
    binding,
    owner: emptyOwner(),
    toFix: false,
    extra: false,
  }
}

export function repoByDefN(w: World, n: number): Repo {
  const def = REPO_DEFS.find((d) => d.n === n) as RepoDef
  const path = `${SCAN_ROOT}/${def.dir ?? def.name}`
  return w.repos.find((r) => r.path === path) as Repo
}

export function remoteRepoFor(name: string, offsetDays: number, desc = '', region: string = CC_REGION): RemoteRepo {
  return {
    name,
    description: desc || `${name.replace(/_/g, ' ')} service`,
    lastModified: ago((offsetDays * 24 + 3) * HOUR).replace(/:\d\dZ$/, ':00Z'),
    repositoryId: `${fakeHash16(name).slice(0, 8)}-0000-4000-8000-${fakeHash16(name + 'x').slice(0, 12)}`,
    cloneUrl: httpsUrl(name, region),
  }
}

// ------------------------------------------------------------------ discovery

export function discoveredAccount(w: World, key: AcctKey, roles: string[]): DiscoveredAccount {
  const a = ACCT[key]
  return {
    accountId: a.id,
    accountName: a.name,
    email: a.email,
    colorSlot: colorSlotForAccount(w, a.id),
    roles: roles.map((role) => {
      const existing = allProfiles(w).find((p) => p.kind === ProfileKind.ProfileKindSSO && p.accountId === a.id && p.role === role)
      return { name: role, existingProfile: existing?.name, suggestedProfile: existing?.name ?? `${a.name}-${slugRole(role)}` }
    }),
  }
}

/** Discovery lists are rebuilt on demand so `existingProfile` follows profile creation/removal. */
export function discoverFor(w: World, portal: string): DiscoveredAccount[] {
  const spec = w.discovery[portal]
  if (!spec) {
    // A portal that was typed in: its own organisation holds one account. Unknown URLs list every account.
    const url = findPortal(w, portal)?.startUrl
    const own = (Object.keys(PORTAL_OF_STATIC) as Array<keyof typeof PORTAL_OF_STATIC>).find((k) => PORTAL_OF_STATIC[k].startUrl === url)
    const list = own ? w.draftDiscovery.filter((a) => a.accountId === ACCT[own as AcctKey].id) : w.draftDiscovery
    return deepCopy(list.length > 0 ? list : w.draftDiscovery).map((a) => refreshExisting(w, a))
  }
  return deepCopy(spec).map((a) => refreshExisting(w, a))
}

function refreshExisting(w: World, a: DiscoveredAccount): DiscoveredAccount {
  a.colorSlot = colorSlotForAccount(w, a.accountId)
  for (const r of a.roles ?? []) {
    const existing = allProfiles(w).find((p) => p.kind === ProfileKind.ProfileKindSSO && p.accountId === a.accountId && p.role === r.name)
    r.existingProfile = existing?.name
    if (existing) r.suggestedProfile = existing.name
  }
  return a
}

// ------------------------------------------------------------------ scenario construction

function baseSettings(): Settings {
  return {
    language: initialUI.lang === 'en' ? Language.LanguageEn : Language.LanguageZhCN,
    theme: initialUI.theme === 'dark' ? Theme.ThemeDark : initialUI.theme === 'light' ? Theme.ThemeLight : Theme.ThemeSystem,
    motion: MotionPref.MotionSystem,
    scanRoots: [SCAN_ROOT],
    reminders: { expiryNotice: true, leadMinutes: 15, autoRenew: true, endedNoticeOff: false },
    tray: { enabled: false, keepRunning: false },
    terminalApp: TerminalApp.TerminalAuto,
    backupRetention: 20,
    tutorialDone: [],
    tutorialLang: '',
    awsCliPath: '',
    gitPath: '',
    autoBindOnAdd: true,
    privateSignIn: true,
    updateCheckOff: false,
    collapsedPortals: [],
    lastCloneProfile: '',
    lastCloneRegion: '',
  }
}

function baseAppInfo(firstRun: boolean): AppInfo {
  const win = initialUI.platform === 'windows'
  return {
    version: '0.0.1',
    platform: initialUI.platform,
    arch: win ? 'amd64' : 'arm64',
    sandbox: false,
    dataDir: win ? '%AppData%\\AWS SSO Hub' : '~/Library/Application Support/AWS SSO Hub',
    backupDir: win ? '%AppData%\\AWS SSO Hub\\backups' : '~/Library/Application Support/AWS SSO Hub/backups',
    firstRun,
    uiLanguage: initialUI.lang ?? 'zh-CN',
  }
}

export function baseEnv(cliStatus: ToolStatus = ToolStatus.ToolOK): EnvReport {
  const missing = cliStatus === ToolStatus.ToolMissing
  return {
    awsCli: missing
      ? { name: 'aws', status: ToolStatus.ToolMissing, min: '2.22.0', recommended: '2.38.2' }
      : { name: 'aws', path: '/usr/local/bin/aws', version: '2.40.0', status: cliStatus, min: '2.22.0', recommended: '2.38.2' },
    git: { name: 'git', path: '/usr/bin/git', version: '2.47.1', status: ToolStatus.ToolOK, min: GIT_MIN_VERSION },
    grc: { name: 'git-remote-codecommit', path: '/usr/local/bin/git-remote-codecommit', status: ToolStatus.ToolOK },
    files: [
      { kind: 'aws-config', file: '~/.aws/config', exists: true, readable: true, writable: true },
      { kind: 'aws-credentials', file: '~/.aws/credentials', exists: true, readable: true, writable: true },
      { kind: 'git-global', file: '~/.gitconfig', exists: true, readable: true, writable: true },
    ],
    checkedAt: iso(now()),
  }
}

function ssoProfile(name: string, key: AcctKey, role: string, region: string, portal: string): Profile {
  const a = ACCT[key]
  return {
    name,
    kind: ProfileKind.ProfileKindSSO,
    portal,
    accountId: a.id,
    accountName: a.name,
    role,
    region,
    colorSlot: a.slot,
    monogram: autoMonogram(name),
    isRootKey: false,
    hasStaticKeys: false,
    boundRepos: 0,
    isGlobalDefault: false,
    globalDefaultRepos: 0,
    migrationInProgress: false,
  }
}

function staticProfile(name: string): Profile {
  const t = STATIC_TRUTH[name]
  return {
    name,
    kind: ProfileKind.ProfileKindStatic,
    region: t.region,
    colorSlot: ACCT[t.account].slot, // provisional slot keyed by profile name until identified
    monogram: autoMonogram(name),
    staticState: StaticState.StaticUnverified,
    isRootKey: false,
    hasStaticKeys: true,
    boundRepos: 0,
    isGlobalDefault: false,
    globalDefaultRepos: 0,
    migrationInProgress: false,
  }
}

export function mkPortal(name: string, startUrl: string, region: string, draft = false): Portal {
  return {
    name,
    startUrl,
    startUrlHost: startUrl.replace(/^https:\/\//, ''),
    region,
    legacyFormat: false,
    draft,
    profiles: [],
    session: computeSession(name, newSessionModel()),
  }
}

function emptyWorld(scenario: ScenarioId, firstRun: boolean): World {
  const w: World = {
    scenario,
    portalModels: {},
    accounts: {
      configFile: '~/.aws/config',
      credentialsFile: '~/.aws/credentials',
      readOnly: false,
      issues: [],
      portals: [],
      profiles: [],
      staticCount: 0,
    },
    discovery: {},
    draftDiscovery: [],
    repos: [],
    projects: [],
    dismissedSuggestions: [],
    repoWarnings: [],
    foundSeeded: false,
    movedRecords: {},
    repoIdentity: {},
    truth: {},
    repoMeta: { scannedAt: iso(now() - 30_000), scanning: false, detecting: false },
    awsCliReady: true,
    git: { globalDefault: null, urlRules: [], guardInstalled: false, baseHash: 'mock-gitconfig-0', saved: [] },
    settings: baseSettings(),
    appInfo: baseAppInfo(firstRun),
    env: baseEnv(),
    health: { findings: [], checkedAt: iso(now()) },
    backups: [],
    migrations: {},
    remote: {},
    hashCounter: 0,
  }
  w.settings.language = initialUI.lang === 'en' ? Language.LanguageEn : Language.LanguageZhCN
  return w
}

function addRepos(w: World): void {
  for (const def of REPO_DEFS) {
    const path = `${SCAN_ROOT}/${def.dir ?? def.name}`
    const r = mkRepo(path, def.name, emptyBinding(BindingSource.BindingNone), REGION_OF_ACCT[def.owners[0]])
    w.repos.push(r)
    w.truth[r.id] = { accounts: def.owners.map((k) => ACCT[k].id), headMatch: def.headMatch ? ACCT[def.headMatch].id : undefined }
  }
}

function addRemote(w: World): void {
  const owned = (k: AcctKey) => REPO_DEFS.filter((d) => d.owners.includes(k)).map((d) => d.name)
  const uniq = (xs: string[]) => [...new Set(xs)]
  const mk = (names: string[], region: string) => names.map((n, i) => remoteRepoFor(n, 1 + ((i * 3) % 28), '', region))
  w.remote[ACCT.acme.id] = mk([...uniq(owned('acme')), 'acme_usage'], REGION_ACME).map((r) => (r.name === 'acme_usage' ? { ...r, description: 'Monthly usage reports' } : r))
  w.remote[ACCT.globex.id] = mk(['globex_backend', 'acme_cs_portal', 'mobile-api'], REGION_GLOBEX)
  w.remote[ACCT.initech.id] = mk(['initech_infra', 'initech_dashboard', 'initech_site', 'initech_cfn'], REGION_INITECH)
}

function addGitBase(w: World): void {
  w.git.globalDefault = { profile: 'acme' }
  w.git.urlRules = [{ pattern: httpsUrl('initech_infra', REGION_INITECH), profile: 'initech-admin' }]
  w.git.guardInstalled = false
  w.git.saved = [{ id: `keychain:${CC_HOST}`, host: CC_HOST, store: SavedCredentialStore.StoreKeychain }]
}

function initechProfiles(): Profile[] {
  return [ssoProfile('initech-admin', 'initech', 'AdministratorAccess', REGION_INITECH, PORTAL_INITECH.name), ssoProfile('initech-readonly', 'initech', 'ReadOnlyAccess', REGION_INITECH, PORTAL_INITECH.name)]
}

function initechPortal(): Portal {
  return mkPortal(PORTAL_INITECH.name, PORTAL_INITECH.startUrl, PORTAL_INITECH.region)
}

function portalOf(p: { name: string; startUrl: string; region: string }, draft = false): Portal {
  return mkPortal(p.name, p.startUrl, p.region, draft)
}

export function buildFresh(): World {
  const w = emptyWorld('fresh', true)
  w.accounts.portals = [initechPortal()]
  w.accounts.profiles = [staticProfile('acme'), staticProfile('globex'), ...initechProfiles()]
  w.portalModels[PORTAL_INITECH.name] = newSessionModel({
    file: 'present',
    tokenExpiresAt: now() - 9 * HOUR,
    issuedAt: now() - 10 * HOUR,
    canRefresh: true,
    known: false,
  })
  addRepos(w)
  addRemote(w)
  addGitBase(w)
  for (const r of w.repos) {
    r.binding = emptyBinding(BindingSource.BindingGlobalDefault, 'acme')
  }
  const legacyRepo = repoByDefN(w, 11)
  legacyRepo.binding = { ...emptyBinding(BindingSource.BindingLocal, 'acme'), legacyPlain: true }
  const ruleRepo = repoByDefN(w, 12)
  ruleRepo.binding = { ...emptyBinding(BindingSource.BindingGlobalURL, 'initech-admin'), urlRule: httpsUrl('initech_infra', REGION_INITECH) }
  w.discovery[PORTAL_INITECH.name] = [
    {
      accountId: ACCT.initech.id,
      accountName: ACCT.initech.name,
      email: ACCT.initech.email,
      colorSlot: ACCT.initech.slot,
      roles: [
        { name: 'AdministratorAccess', existingProfile: 'initech-admin', suggestedProfile: 'initech-admin' },
        { name: 'ReadOnlyAccess', existingProfile: 'initech-readonly', suggestedProfile: 'initech-readonly' },
        { name: 'PowerUserAccess', suggestedProfile: 'initech-poweruser' },
      ],
    },
  ]
  w.draftDiscovery = [
    { accountId: ACCT.acme.id, accountName: ACCT.acme.name, email: ACCT.acme.email, colorSlot: ACCT.acme.slot, roles: [{ name: 'AdministratorAccess', suggestedProfile: 'acme-administrator' }, { name: 'PowerUserAccess', suggestedProfile: 'acme-poweruser' }] },
    { accountId: ACCT.globex.id, accountName: ACCT.globex.name, email: ACCT.globex.email, colorSlot: ACCT.globex.slot, roles: [{ name: 'AdministratorAccess', suggestedProfile: 'globex-administrator' }, { name: 'ReadOnlyAccess', suggestedProfile: 'globex-readonly' }] },
  ]
  w.health.findings = [{ id: 'global-static:acme', kind: HealthKind.HealthGlobalStatic, severity: Severity.SeverityWarn, profile: 'acme' }]
  return w
}

export function finishWorld(w: World): World {
  applySessionOverride(w)
  recompute(w)
  return w
}

/** Each portal lists only its own organisation's account (`mid` and `done`). */
function discoveryPerPortal(w: World): Record<string, DiscoveredAccount[]> {
  return {
    [PORTAL_INITECH.name]: [discoveredAccount(w, 'initech', ['AdministratorAccess', 'ReadOnlyAccess', 'PowerUserAccess'])],
    [PORTAL_ACME.name]: [discoveredAccount(w, 'acme', ['AdministratorAccess', 'PowerUserAccess'])],
    [PORTAL_GLOBEX.name]: [discoveredAccount(w, 'globex', ['AdministratorAccess', 'ReadOnlyAccess'])],
  }
}

export function applySessionOverride(w: World): void {
  const o = initialUI.session
  if (!o) return
  const portal = PORTAL_INITECH.name
  const t = now()
  const m = signedInModel({ sessionMinutes: 480 })
  if (o === 'unknown') {
    m.known = false
    m.loginAt = undefined
  } else if (o === 'overrun') {
    m.loginAt = t - 9 * HOUR
  } else if (o === 'ending') {
    m.loginAt = t + 9 * MIN - 480 * MIN
  }
  if (w.portalModels[portal] || w.accounts.portals?.some((p) => p.name === portal)) w.portalModels[portal] = m
}

function localBind(r: Repo, profile: string): void {
  r.binding = emptyBinding(BindingSource.BindingLocal, profile)
}

export function buildMid(): World {
  const w = emptyWorld('mid', false)
  // acme is migrated into its own portal; globex is still a static profile and its new portal is a draft
  w.accounts.portals = [initechPortal(), portalOf(PORTAL_ACME), portalOf(PORTAL_GLOBEX, true)]
  const acme = ssoProfile('acme', 'acme', 'AdministratorAccess', REGION_ACME, PORTAL_ACME.name)
  const globex = staticProfile('globex')
  globex.staticState = StaticState.StaticOK
  w.accounts.profiles = [acme, globex, ...initechProfiles()]
  w.portalModels[PORTAL_INITECH.name] = signedInModel()
  w.portalModels[PORTAL_ACME.name] = signedInModel({ loginAt: now() - (480 * MIN - (3 * HOUR + 5 * MIN)) })
  w.portalModels[PORTAL_GLOBEX.name] = newSessionModel()
  addRepos(w)
  addRemote(w)
  addGitBase(w)
  w.git.saved = []
  for (const r of w.repos) r.binding = emptyBinding(BindingSource.BindingGlobalDefault, 'acme')
  for (const n of [1, 3, 4, 5, 6, 7, 8, 11, 13]) localBind(repoByDefN(w, n), 'acme')
  for (const n of [10, 15]) localBind(repoByDefN(w, n), 'initech-admin')
  const ruleRepo = repoByDefN(w, 12)
  ruleRepo.binding = { ...emptyBinding(BindingSource.BindingGlobalURL, 'initech-admin'), urlRule: httpsUrl('initech_infra', REGION_INITECH) }
  repoByDefN(w, 10).lastTest = { repoId: repoByDefN(w, 10).id, status: TestStatus.TestOK, at: ago(2 * MIN), profile: 'initech-admin' }
  for (const r of w.repos) if (r.binding.source === BindingSource.BindingLocal) presetSingleOwner(w, r)
  w.discovery = discoveryPerPortal(w)
  w.backups = []
  addBackup(w, BackupKind.BackupAWSConfig, BackupReason.ReasonMigrate, { profile: 'acme' }, { createdAt: ago(3 * HOUR), redacted: false })
  addBackup(w, BackupKind.BackupAWSCredentials, BackupReason.ReasonRemoveKey, { profile: 'acme' }, { createdAt: ago(2 * HOUR + 40 * MIN), redacted: true })
  addBackup(w, BackupKind.BackupGitLocal, BackupReason.ReasonBind, { repo: 'acme_cs_portal' }, { createdAt: ago(35 * MIN) })
  w.health.findings = [
    { id: 'pending-deactivation:acme', kind: HealthKind.HealthPendingKey, severity: Severity.SeverityInfo, profile: 'acme', params: { maskedKeyId: STATIC_TRUTH.acme.masked, days: '0' } },
  ]
  w.migrations.globex = midGlobexMigration(w)
  return w
}

function presetSingleOwner(w: World, r: Repo): void {
  const profile = findProfile(w, r.binding.profile ?? '')
  const accountId = profile ? accountIdOfProfile(profile) : undefined
  r.owner = {
    status: OwnerStatus.OwnerSingle,
    candidates: accountId ? [candidateFor(w, accountId, { repo: r })] : [],
    checkedWith: 3,
    skipped: 0,
    skippedPortals: [],
    checkedAt: iso(now() - 10 * MIN),
  }
}

/** The profile a repository is bound to (a local binding or a git-remote-codecommit URL); the global default does not count. */
function boundProfileOf(r: Repo | undefined): string {
  if (!r) return ''
  return r.binding.source === BindingSource.BindingLocal || r.binding.source === BindingSource.BindingRemoteURL ? (r.binding.profile ?? '') : ''
}

/**
 * Suggested profile order for an account: the profile the repository is already bound to, SSO before static keys, the
 * profile with the most bound repositories, then the name. A signed-out SSO profile can still be suggested.
 */
export function candidateFor(w: World, accountId: string, opts: { headMatch?: boolean; score?: number; repo?: Repo } = {}): OwnerCandidate {
  const bound = boundProfileOf(opts.repo)
  const counts = new Map<string, number>()
  for (const r of w.repos) {
    const b = boundProfileOf(r)
    if (b) counts.set(b, (counts.get(b) ?? 0) + 1)
  }
  const rank = (p: Profile) => (p.kind === ProfileKind.ProfileKindSSO ? 0 : 1)
  const profs = allProfiles(w)
    .filter((p) => accountIdOfProfile(p) === accountId)
    .sort(
      (a, b) =>
        Number(b.name === bound) - Number(a.name === bound) ||
        rank(a) - rank(b) ||
        (counts.get(b.name) ?? 0) - (counts.get(a.name) ?? 0) ||
        a.name.localeCompare(b.name),
    )
  const k = acctKeyOfId(accountId)
  const best = profs[0]
  return {
    accountId,
    profile: best?.name ?? (k ? ACCT[k].name : accountId),
    profiles: profs.map((p) => p.name),
    colorSlot: best?.colorSlot ?? (k ? ACCT[k].slot : 0),
    monogram: best?.monogram ?? autoMonogram(k ? ACCT[k].name : accountId),
    headMatch: !!opts.headMatch,
    matchScore: opts.score ?? 0,
    kmsWarning: false,
  }
}

export function buildMigrationConfigPreview(w: World, profile: string, portal: string, accountId: string, role: string, region: string): FilePreview {
  const p = findPortal(w, portal)
  const lines: DiffLine[] = [ctx(`[profile ${profile}]`), add('sso_session    = ' + portal), add('sso_account_id = ' + accountId), add('sso_role_name  = ' + role), ctx('region = ' + region), fold(20)]
  if (p?.draft) {
    lines.push(add(`[sso-session ${portal}]`), add('sso_start_url = ' + p.startUrl), add('sso_region = ' + p.region), add('sso_registration_scopes = sso:account:access'))
  }
  return mkPreview(w, '~/.aws/config', `${HOME}/.aws/config`, lines)
}

function midGlobexMigration(w: World): MigrationState {
  const t = STATIC_TRUTH.globex
  return {
    profile: 'globex',
    step: MigrationStep.MigrationReview,
    identity: { profile: 'globex', accountId: ACCT.globex.id, arn: `arn:aws:iam::${ACCT.globex.id}:user/${t.iamUser}`, iamUser: t.iamUser, maskedKeyId: t.masked, isRoot: false },
    keyInMemory: true,
    portal: PORTAL_GLOBEX.name,
    portalIsDraft: true,
    accountId: ACCT.globex.id,
    accountName: 'globex',
    role: 'AdministratorAccess',
    region: t.region,
    confirmOther: false,
    configPreview: buildMigrationConfigPreview(w, 'globex', PORTAL_GLOBEX.name, ACCT.globex.id, 'AdministratorAccess', t.region),
    keyRemoved: false,
  }
}

/**
 * The projects of the mockup (acme, globex, umbrella), one hidden repository and one repository whose folder was
 * moved by hand (the question "Is this initech_dashboard, moved here?"), as `done` shows them after some use.
 */
function seedProjectsAndReview(w: World): void {
  w.projects = [
    { id: 'p_acme', name: 'acme', collapsed: false, order: 0 },
    { id: 'p_globex', name: 'globex', collapsed: false, order: 1 },
    { id: 'p_umbrella', name: 'umbrella', collapsed: false, order: 2 },
  ]
  for (const r of w.repos) {
    if (r.name.startsWith('acme_')) r.projectId = 'p_acme'
    else if (r.name.startsWith('globex_')) r.projectId = 'p_globex'
    else if (r.name.startsWith('initech_')) r.projectId = 'p_umbrella'
  }
  const hid = repoByDefN(w, 14) // acme_handbook
  hid.hidden = true
  hid.projectId = undefined
  // initech_dashboard was renamed to dashboard-main in Finder: the new folder has no project of its own, the old record has.
  const mv = repoByDefN(w, 10)
  const oldId = mv.id
  // What the old record held, for ReattachMoved to give back (FU-11c).
  w.movedRecords[oldId] = {
    projectId: mv.projectId,
    hidden: false,
    lastTest: mv.lastTest ? { ...mv.lastTest } : undefined,
    identity: { name: 'Alex', email: 'alex@initech.example.com' },
  }
  const path = `${SCAN_ROOT}/dashboard-main`
  mv.path = path
  mv.displayPath = path.replace(HOME, '~')
  mv.localName = 'dashboard-main'
  mv.id = fakeHash16(path)
  mv.projectId = undefined
  mv.movedFrom = { oldId, name: 'initech_dashboard' }
  if (mv.lastTest) mv.lastTest = { ...mv.lastTest, repoId: mv.id }
  if (w.truth[oldId]) {
    w.truth[mv.id] = w.truth[oldId]
    delete w.truth[oldId]
  }
}

export function buildDone(): World {
  const w = emptyWorld('done', false)
  w.accounts.portals = [initechPortal(), portalOf(PORTAL_ACME), portalOf(PORTAL_GLOBEX)]
  w.accounts.profiles = [
    ssoProfile('acme', 'acme', 'AdministratorAccess', REGION_ACME, PORTAL_ACME.name),
    ssoProfile('globex', 'globex', 'AdministratorAccess', REGION_GLOBEX, PORTAL_GLOBEX.name),
    ...initechProfiles(),
  ]
  const t = now()
  w.portalModels[PORTAL_INITECH.name] = signedInModel()
  w.portalModels[PORTAL_ACME.name] = signedInModel({ loginAt: t - (480 * MIN - (3 * HOUR + 5 * MIN)) })
  // globex signed in yesterday: the token is gone and cannot be refreshed, so the card shows "SSO login"
  w.portalModels[PORTAL_GLOBEX.name] = newSessionModel({ file: 'present', tokenExpiresAt: t - 2 * HOUR, issuedAt: t - 3 * HOUR, canRefresh: false })
  addRepos(w)
  addRemote(w)
  w.git = { globalDefault: null, urlRules: [], guardInstalled: true, baseHash: 'mock-gitconfig-0', saved: [] }
  for (const def of REPO_DEFS) {
    const r = repoByDefN(w, def.n)
    const ownerKey = def.n === 2 ? 'globex' : def.owners[0]
    const profile = ownerKey === 'initech' ? 'initech-admin' : ownerKey
    localBind(r, profile)
    r.lastTest = { repoId: r.id, status: TestStatus.TestOK, at: ago((2 + def.n) * MIN), profile }
    presetSingleOwner(w, r)
  }
  seedProjectsAndReview(w)
  w.discovery = discoveryPerPortal(w)
  w.backups = []
  const kinds: Array<[BackupKind, BackupReason, Record<string, string>, boolean]> = [
    [BackupKind.BackupAWSConfig, BackupReason.ReasonAddProfiles, { n: '2' }, false],
    [BackupKind.BackupAWSConfig, BackupReason.ReasonMigrate, { profile: 'acme' }, false],
    [BackupKind.BackupAWSCredentials, BackupReason.ReasonRemoveKey, { profile: 'acme' }, true],
    [BackupKind.BackupAWSConfig, BackupReason.ReasonMigrate, { profile: 'globex' }, false],
    [BackupKind.BackupAWSCredentials, BackupReason.ReasonRemoveKey, { profile: 'globex' }, true],
    [BackupKind.BackupGitLocal, BackupReason.ReasonBind, { repo: 'acme_cs_portal' }, false],
    [BackupKind.BackupGitLocal, BackupReason.ReasonBind, { repo: 'globex_backend' }, false],
    [BackupKind.BackupGitGlobal, BackupReason.ReasonGlobalDefault, {}, false],
  ]
  kinds.forEach(([k, r, p, red], i) => addBackup(w, k, r, p, { createdAt: ago((8 - i) * 3 * HOUR), redacted: red }))
  return w
}

export function buildEmpty(): World {
  const w = emptyWorld('empty', true)
  w.git = { globalDefault: null, urlRules: [], guardInstalled: false, baseHash: 'mock-gitconfig-0', saved: [] }
  w.repoMeta.scannedAt = iso(now() - 5_000)
  return w
}

export function buildEdge(): World {
  const w = buildFresh()
  w.scenario = 'edge'
  w.appInfo.firstRun = false
  w.awsCliReady = false
  w.env = baseEnv(ToolStatus.ToolMissing)
  w.accounts.issues = [{ file: '~/.aws/config', line: 23, message: 'expected key = value' }]
  w.accounts.readOnly = true
  w.portalModels[PORTAL_INITECH.name] = newSessionModel({ file: 'invalid', error: mkError(ErrorCode.ErrFileIO, 'cannot read token file', { file: '~/.aws/sso/cache/' + fakeHash16('initech') + '.json' }) })
  const globex = findProfile(w, 'globex') as Profile
  globex.staticState = StaticState.StaticInvalid
  globex.staticError = 'InvalidClientTokenId'
  // row 8: SSH remote, row 11: GRC remote, row 16: owner none, row 10: needs repair, row 14: owner partial
  const r8 = repoByDefN(w, 8)
  r8.remoteUrl = `ssh://${CC_HOST}/v1/repos/acme_data`
  r8.remoteKind = RemoteKind.RemoteSSH
  r8.binding = emptyBinding(BindingSource.BindingNone)
  const r11 = repoByDefN(w, 11)
  r11.remoteUrl = `codecommit::${CC_REGION}://acme@acme_design`
  r11.remoteKind = RemoteKind.RemoteGRC
  r11.host = undefined
  r11.binding = emptyBinding(BindingSource.BindingRemoteURL, 'acme')
  const r16 = repoByDefN(w, 16)
  r16.owner = { status: OwnerStatus.OwnerNone, candidates: [], checkedWith: 2, skipped: 0, skippedPortals: [], checkedAt: iso(now() - 60_000) }
  w.truth[r16.id] = { accounts: [] }
  const r10 = repoByDefN(w, 10)
  r10.binding = { ...emptyBinding(BindingSource.BindingLocal, 'initech-admin'), needsRepair: true }
  const r14 = repoByDefN(w, 14)
  r14.owner = { status: OwnerStatus.OwnerPartial, candidates: [], checkedWith: 1, skipped: 1, skippedPortals: [PORTAL_INITECH.name], checkedAt: iso(now() - 60_000) }
  const r15 = repoByDefN(w, 15)
  r15.hasUserinfo = false
  const r16u = repoByDefN(w, 16)
  r16u.hasUserinfo = true
  r16u.remoteUrl = `https://alex@${CC_HOST}/v1/repos/acme_changelog`
  w.health.findings = [
    { id: 'shell-export:~/.zshrc:42', kind: HealthKind.HealthShellExport, severity: Severity.SeverityDanger, file: '~/.zshrc', line: 42 },
    { id: 'env-var:AWS_ACCESS_KEY_ID', kind: HealthKind.HealthEnvVar, severity: Severity.SeverityDanger, params: { name: 'AWS_ACCESS_KEY_ID' } },
    { id: 'default-keys', kind: HealthKind.HealthDefaultKeys, severity: Severity.SeverityWarn },
    { id: 'leftover-keys:initech-admin', kind: HealthKind.HealthLeftoverKeys, severity: Severity.SeverityWarn, profile: 'initech-admin' },
    { id: 'url-userinfo:acme_changelog', kind: HealthKind.HealthURLUserinfo, severity: Severity.SeverityWarn, params: { repo: 'acme_changelog' } },
  ]
  return w
}

export function buildScenario(id: ScenarioId): World {
  switch (id) {
    case 'mid':
      return finishWorld(buildMid())
    case 'done':
      return finishWorld(buildDone())
    case 'empty':
      return finishWorld(buildEmpty())
    case 'edge':
      return finishWorld(buildEdge())
    default:
      return finishWorld(buildFresh())
  }
}

// ------------------------------------------------------------------ singleton

let world: World | null = null

/** Present in every bundle that contains the mock layer; scripts/check-dist.mjs fails a production build that has it. */
export const MOCK_LAYER_MARKER = 'awsssohub-mock-layer'

export function getWorld(): World {
  if (!world) {
    if (typeof document !== 'undefined') document.documentElement.dataset.mockLayer = MOCK_LAYER_MARKER
    world = buildScenario(initialUI.scenario)
    if (world.scenario === 'fresh' && !initialUI.session) startFreshTicker(world)
  }
  recompute(world) // session states depend on the clock, so derived fields are refreshed on every access
  return world
}

/** fresh: initech is `refreshable`; the mock ticker shows `refreshing` at +1.5 s and `expired` (refresh failed) at +2.5 s. */
function startFreshTicker(w: World): void {
  const portal = PORTAL_INITECH.name
  setTimeout(() => {
    const m = w.portalModels[portal]
    if (!m || m.file !== 'present' || m.refreshFailed) return
    m.refreshing = true
    emitSession(w, portal)
  }, 1500)
  setTimeout(() => {
    const m = w.portalModels[portal]
    if (!m || !m.refreshing) return
    m.refreshing = false
    m.refreshFailed = true
    emitSession(w, portal)
  }, 2500)
}

/** Recompute and publish session:changed + accounts:changed for one portal. */
export function emitSession(w: World, portal: string): void {
  recompute(w)
  const p = findPortal(w, portal)
  if (p) mockEmit('session:changed', deepCopy(p.session))
  mockEmit('accounts:changed', deepCopy(w.accounts))
}

/** Tests: replace the world with a fresh one for `id`. */
export function resetWorld(id: ScenarioId = initialUI.scenario): World {
  world = buildScenario(id)
  return world
}


/** AccountService.IdentifyStatic logic (shared with the migration wizard). */
export function identifyStaticProfile(w: World, profile: string): StaticIdentity {
  const p = findProfile(w, profile)
  if (!p) throw new Error(`profile ${profile} not found`)
  const t = STATIC_TRUTH[profile]
  if (!t) return { profile, isRoot: false, errorCode: 'InvalidClientTokenId' }
  if (p.staticState === StaticState.StaticInvalid) {
    return { profile, maskedKeyId: t.masked, isRoot: false, errorCode: p.staticError || 'InvalidClientTokenId' }
  }
  const a = ACCT[t.account]
  p.accountId = a.id
  p.accountName = a.name
  p.iamUser = t.iamUser
  p.maskedKeyId = t.masked
  p.colorSlot = a.slot
  if (p.staticState !== StaticState.StaticKeysLeft) p.staticState = StaticState.StaticOK
  recompute(w)
  mockEmit('accounts:changed', deepCopy(w.accounts))
  return { profile, accountId: a.id, arn: `arn:aws:iam::${a.id}:user/${t.iamUser}`, iamUser: t.iamUser, maskedKeyId: t.masked, isRoot: false }
}

/** Portals sorted as the backend does: real portals by name, then drafts, then legacy pseudo-portals. */
export function sortPortals(list: Portal[]): Portal[] {
  const rank = (p: Portal) => (p.legacyFormat ? 2 : p.draft ? 1 : 0)
  return [...list].sort((a, b) => rank(a) - rank(b) || a.name.localeCompare(b.name))
}
