import { CancellablePromise } from '@wailsio/runtime'
import { initialUI } from '../../lib/initialUI'
import * as AccountReal from '../../../bindings/github.com/rudian/codepass/internal/app/accountservice'
import { BackupKind, BackupReason, ErrorCode, LoginMode, LoginPhase, ProfileKind, SessionState } from '../models'
import type { DiffLine, LoginResult, PortalInputCheck } from '../models'
import { mockEmit } from '../events'
import { HOUR, MIN, deepCopy, fromNow, iso, now } from './clock'
import {
  ACCT,
  AUTO_SLOT_ORDER,
  HOME,
  accountsState,
  add,
  addBackup,
  autoMonogram,
  ctx,
  del,
  discoverFor,
  emitSession,
  findPortal,
  findProfile,
  getWorld,
  mkPortal,
  mkPreview,
  newSessionModel,
  recompute,
  fold,
  identifyStaticProfile,
  sortPortals,
} from './world'
import type { World } from './world'
import { MockError, READ, SLOW, WRITE, fail, run, toRuntimeError } from './util'
import { applyImport, buildExport, exportFileName, previewImport, sampleImportFile } from './transfer'

const AUTHORIZE_URL =
  'https://oidc.us-east-1.amazonaws.com/authorize?response_type=code&client_id=MOCKCLIENTID&redirect_uri=http%3A%2F%2F127.0.0.1%3A53412%2Foauth%2Fcallback&state=00000000-0000-4000-8000-000000000000&code_challenge_method=S256&scopes=sso%3Aaccount%3Aaccess&code_challenge=MOCKCHALLENGE'

const HOST_SUFFIXES = ['.awsapps.com', '.app.aws', '.awsapps.cn', '.portal.amazonaws.com', '.portal.amazonaws.com.cn', '.app.amazonwebservices.com.cn', '.portal.amazonaws.eu', '.api.amazonwebservices.eu']
const HOSTS_EXACT = ['identitycenter.amazonaws.com', 'identitycenter.amazonaws.com.cn', 'identitycenter.amazonaws.eu', 'identitycenter.us-gov.amazonaws.com']
const SIGN_IN_DEVICE_HOST_RE = /^(device\.sso|oidc)\.[a-z]{2,4}(-[a-z]+)+-\d+\.amazonaws\.(com|com\.cn|eu)$/
const SESSION_NAME_RE = /^[A-Za-z0-9_.-]+$/
const REGION_RE = /^[a-z]{2}(-gov|-iso[a-z]*)?-[a-z]+-[0-9]{1,2}$/

function normalizeStartUrl(raw: string): { url: string; ok: boolean } {
  let s = raw.trim()
  s = s.replace(/#\/?$/, '')
  let u: URL
  try {
    u = new URL(s)
  } catch {
    return { url: s, ok: false }
  }
  const host = u.hostname.toLowerCase()
  const okHost = HOSTS_EXACT.includes(host) || HOST_SUFFIXES.some((x) => host.endsWith(x))
  if (u.protocol !== 'https:' || !okHost) return { url: s, ok: false }
  let path = u.pathname
  if (host.endsWith('.awsapps.com') && (path === '' || path === '/')) path = '/start'
  return { url: `https://${host}${path === '/' ? '' : path}`, ok: true }
}

export function checkPortalInput(w: World, input: { name: string; startUrl: string; region: string }): PortalInputCheck {
  const errors: Record<string, ErrorCode> = {}
  const n = normalizeStartUrl(input.startUrl)
  if (!n.ok) errors.startUrl = ErrorCode.ErrInvalidStartURL
  let region = input.region.trim()
  const m = /^[^.]+\.portal\.([a-z0-9-]+)\.app\.aws$/.exec(n.ok ? new URL(n.url).hostname : '')
  if (!region && m) region = m[1]
  if (region && !REGION_RE.test(region)) errors.region = ErrorCode.ErrInvalidRegion
  let name = input.name.trim()
  if (!name && n.ok) name = new URL(n.url).hostname.split('.')[0].toLowerCase()
  if (name && !SESSION_NAME_RE.test(name)) errors.name = ErrorCode.ErrInvalidName
  else if (name && (w.accounts.portals ?? []).some((p) => p.name === name)) errors.name = ErrorCode.ErrPortalExists
  return { name, startUrl: n.url, region, errors }
}

function tokenState(w: World, portal: string): SessionState {
  const p = findPortal(w, portal)
  return p ? p.session.state : SessionState.SessionSignedOut
}

function requireSignedIn(w: World, portal: string): void {
  const st = tokenState(w, portal)
  if (st === SessionState.SessionSignedOut || st === SessionState.SessionError) throw fail(ErrorCode.ErrNotSignedIn, `${portal} is not signed in`, { portal })
  if (st === SessionState.SessionExpired) throw fail(ErrorCode.ErrSessionExpired, `${portal} session expired`, { portal })
}

function requirePortal(w: World, portal: string) {
  const p = findPortal(w, portal)
  if (!p) throw fail(ErrorCode.ErrPortalNotFound, `portal ${portal} not found`, { portal })
  return p
}

function requireWritable(w: World): void {
  if (w.accounts.readOnly) throw fail(ErrorCode.ErrReadOnly, 'config file has a parse issue')
}

function nextSlot(w: World): number {
  const used = new Set((w.accounts.profiles ?? []).map((p) => p.colorSlot))
  return AUTO_SLOT_ORDER.find((s) => !used.has(s)) ?? AUTO_SLOT_ORDER[(w.accounts.profiles ?? []).length % AUTO_SLOT_ORDER.length]
}

function profileNames(w: World): Set<string> {
  return new Set((w.accounts.profiles ?? []).map((p) => p.name))
}

function createPreviewLines(w: World, portal: string, specs: Array<{ name: string; accountId: string; role: string; region: string }>): DiffLine[] {
  const p = requirePortal(w, portal)
  const lines: DiffLine[] = [fold(12)]
  if (p.draft) {
    lines.push(add(`[sso-session ${p.name}]`), add(`sso_start_url = ${p.startUrl}`), add(`sso_region = ${p.region}`), add('sso_registration_scopes = sso:account:access'), add(''))
  }
  for (const s of specs) {
    lines.push(add(`[profile ${s.name}]`), add(`sso_session    = ${p.name}`), add(`sso_account_id = ${s.accountId}`), add(`sso_role_name  = ${s.role}`), add(`region         = ${s.region}`), add(''))
  }
  return lines
}

export function applyLoginSuccess(w: World, portal: string): void {
  const t = now()
  const m = w.portalModels[portal] ?? (w.portalModels[portal] = newSessionModel())
  m.file = 'present'
  m.known = true
  m.loginAt = t
  m.tokenExpiresAt = t + HOUR
  m.issuedAt = t
  m.canRefresh = true
  m.refreshFailed = false
  m.refreshing = false
  m.signingIn = false
  m.error = undefined
}

const login = (portal: string, mode: LoginMode): CancellablePromise<LoginResult> => {
  const w = getWorld()
  const timers: Array<ReturnType<typeof setTimeout>> = []
  const clear = () => timers.forEach(clearTimeout)
  const at = (ms: number, fn: () => void) => {
    timers.push(setTimeout(fn, ms))
  }
  return new CancellablePromise<LoginResult>(
    (resolve, reject) => {
      const p = findPortal(w, portal)
      if (!p) {
        reject(toRuntimeError(fail(ErrorCode.ErrPortalNotFound, `portal ${portal} not found`, { portal })))
        return
      }
      const m = w.portalModels[portal] ?? (w.portalModels[portal] = newSessionModel())
      if (m.signingIn) {
        reject(toRuntimeError(fail(ErrorCode.ErrLoginInProgress, `already signing in to ${portal}`, { portal })))
        return
      }
      m.signingIn = true
      emitSession(w, portal)
      const device = mode === LoginMode.LoginModeDevice || initialUI.mockLogin === 'device'
      const outcome = initialUI.mockLogin
      const endSigningIn = () => {
        m.signingIn = false
        emitSession(w, portal)
      }
      const failWith = (e: MockError) => {
        endSigningIn()
        reject(toRuntimeError(e))
      }
      at(0, () => mockEmit('login:progress', { portal, phase: LoginPhase.LoginPhaseStarting, browserOpened: false, privateWindow: false, isolatedWindow: false }))
      if (outcome === 'port') {
        at(200, () => failWith(fail(ErrorCode.ErrLoginPort, 'cannot listen on a local port')))
        return
      }
      at(400, () => {
        if (device) {
          mockEmit('login:progress', {
            portal,
            phase: LoginPhase.LoginPhaseDevice,
            browserOpened: false,
            privateWindow: false,
            isolatedWindow: false,
            userCode: 'WDJB-MJHT',
            verificationUri: 'https://device.sso.us-east-1.amazonaws.com/',
            verificationUriComplete: 'https://device.sso.us-east-1.amazonaws.com/?user_code=WDJB-MJHT',
            expiresAt: fromNow(10 * MIN),
          })
        } else {
          // the mock's private window is always an isolated browser
          mockEmit('login:progress', { portal, phase: LoginPhase.LoginPhaseWaiting, authorizeUrl: AUTHORIZE_URL, browserOpened: true, privateWindow: w.settings.privateSignIn, isolatedWindow: w.settings.privateSignIn, expiresAt: fromNow(10 * MIN) })
        }
      })
      if (outcome === 'denied') at(1500, () => failWith(fail(ErrorCode.ErrLoginDenied, 'access denied')))
      else if (outcome === 'timeout') at(1500, () => failWith(fail(ErrorCode.ErrLoginTimeout, 'login timed out')))
      else if (outcome === 'network') at(1500, () => failWith(fail(ErrorCode.ErrNetwork, 'cannot reach AWS', undefined, 'oidc.us-east-1.amazonaws.com')))
      else if (outcome === 'success') {
        at(2200, () => mockEmit('login:progress', { portal, phase: LoginPhase.LoginPhaseExchanging, browserOpened: !device, privateWindow: !device && w.settings.privateSignIn, isolatedWindow: !device && w.settings.privateSignIn }))
        at(2500, () => {
          applyLoginSuccess(w, portal)
          emitSession(w, portal)
          const status = findPortal(w, portal)?.session
          resolve({ portal, expiresAt: status?.expiresAt ?? iso(now() + HOUR), profiles: [...(findPortal(w, portal)?.profiles ?? [])] })
        })
      }
      // waiting / device: never resolves; cancelling rejects.
    },
    () => {
      clear()
      const m = w.portalModels[portal]
      if (m) {
        m.signingIn = false
        emitSession(w, portal)
      }
    },
  )
}

export const mockAccountService: typeof AccountReal = {
  GetState: () => run(READ, () => accountsState(getWorld())),
  Reload: () => run(READ, () => accountsState(getWorld())),

  IdentifyStatic: (profile) =>
    run(SLOW, () => {
      const w = getWorld()
      const p = findProfile(w, profile)
      if (!p) throw fail(ErrorCode.ErrProfileNotFound, `profile ${profile} not found`, { name: profile })
      if (!p.hasStaticKeys) throw fail(ErrorCode.ErrNoStaticKey, `${profile} has no access key`, { name: profile })
      return identifyStaticProfile(w, profile)
    }),

  CheckPortalInput: (input) => run(READ, () => checkPortalInput(getWorld(), input)),

  AddPortalDraft: (input) =>
    run(WRITE, () => {
      const w = getWorld()
      const c = checkPortalInput(w, input)
      const first = Object.entries(c.errors ?? {}).find(([, v]) => !!v)
      if (first) throw fail(first[1] as ErrorCode, `invalid portal input: ${first[0]}`, { field: first[0] })
      const portal = mkPortal(c.name, c.startUrl, c.region || 'us-east-1', true)
      w.accounts.portals = sortPortals([...(w.accounts.portals ?? []), portal])
      w.portalModels[portal.name] = newSessionModel()
      recompute(w)
      mockEmit('accounts:changed', deepCopy(w.accounts))
      return portal
    }),

  DiscardPortalDraft: (name) =>
    run(WRITE, () => {
      const w = getWorld()
      const p = findPortal(w, name)
      if (!p || !p.draft) throw fail(ErrorCode.ErrPortalNotFound, `draft ${name} not found`, { portal: name })
      w.accounts.portals = (w.accounts.portals ?? []).filter((x) => x.name !== name)
      delete w.portalModels[name]
      recompute(w)
      mockEmit('accounts:changed', deepCopy(w.accounts))
    }),

  Login: login,

  RefreshSession: (portal) =>
    run(SLOW, () => {
      const w = getWorld()
      requirePortal(w, portal)
      const m = w.portalModels[portal]
      if (!m || m.file !== 'present') throw fail(ErrorCode.ErrNotSignedIn, `${portal} is not signed in`, { portal })
      if (!m.canRefresh || m.refreshFailed) throw fail(ErrorCode.ErrSessionExpired, `${portal} session expired`, { portal })
      const t = now()
      m.tokenExpiresAt = t + HOUR
      m.issuedAt = t
      m.refreshing = false
      recompute(w)
      const status = deepCopy(findPortal(w, portal)!.session)
      mockEmit('session:renewed', status)
      emitSession(w, portal)
      return status
    }),

  Logout: (portal) =>
    run(WRITE, () => {
      const w = getWorld()
      const p = requirePortal(w, portal)
      const m = w.portalModels[portal] ?? (w.portalModels[portal] = newSessionModel())
      const n = p.profiles?.length ?? 0
      m.file = 'missing'
      m.known = false
      m.loginAt = undefined
      m.tokenExpiresAt = undefined
      m.issuedAt = undefined
      m.canRefresh = false
      m.refreshFailed = false
      m.refreshing = false
      emitSession(w, portal)
      return { portal, deletedRoleCaches: n, serverLogout: true }
    }),

  DiscoverAccounts: (portal) =>
    run(SLOW, () => {
      const w = getWorld()
      requirePortal(w, portal)
      requireSignedIn(w, portal)
      return discoverFor(w, portal)
    }),

  LoadAccountRoles: (portal, accountID) =>
    run(SLOW, () => {
      const w = getWorld()
      requirePortal(w, portal)
      requireSignedIn(w, portal)
      const a = discoverFor(w, portal).find((x) => x.accountId === accountID)
      if (!a) throw fail(ErrorCode.ErrInvalidInput, `account ${accountID} not in portal`, { field: 'accountId' })
      return a
    }),

  PreviewCreateProfiles: (req) =>
    run(READ, () => {
      const w = getWorld()
      requireWritable(w)
      const specs = req.profiles ?? []
      const names = profileNames(w)
      for (const s of specs) if (names.has(s.name)) throw fail(ErrorCode.ErrProfileExists, `profile ${s.name} exists`, { name: s.name })
      return mkPreview(w, '~/.aws/config', `${HOME}/.aws/config`, createPreviewLines(w, req.portal, specs))
    }),

  CreateProfiles: (req, _baseHash) =>
    run(WRITE, () => {
      const w = getWorld()
      requireWritable(w)
      const portal = requirePortal(w, req.portal)
      const specs = req.profiles ?? []
      const names = profileNames(w)
      for (const s of specs) if (names.has(s.name)) throw fail(ErrorCode.ErrProfileExists, `profile ${s.name} exists`, { name: s.name })
      const taken = new Set((w.accounts.profiles ?? []).map((p) => p.monogram))
      for (const s of specs) {
        const existing = (w.accounts.profiles ?? []).find((p) => p.accountId === s.accountId)
        const slot = existing?.colorSlot ?? Object.values(ACCT).find((a) => a.id === s.accountId)?.slot ?? nextSlot(w)
        const monogram = autoMonogram(s.name, taken)
        taken.add(monogram)
        w.accounts.profiles = [
          ...(w.accounts.profiles ?? []),
          {
            name: s.name,
            kind: ProfileKind.ProfileKindSSO,
            portal: req.portal,
            accountId: s.accountId,
            accountName: s.accountName,
            role: s.role,
            region: s.region,
            colorSlot: slot,
            monogram,
            isRootKey: false,
            hasStaticKeys: false,
            boundRepos: 0,
            isGlobalDefault: false,
            globalDefaultRepos: 0,
            migrationInProgress: false,
          },
        ]
      }
      portal.draft = false
      const backup = addBackup(w, BackupKind.BackupAWSConfig, BackupReason.ReasonAddProfiles, { n: String(specs.length) })
      recompute(w)
      mockEmit('accounts:changed', deepCopy(w.accounts))
      return { backupId: backup.id, profiles: specs.map((s) => s.name) }
    }),

  PreviewRemoveProfile: (name) =>
    run(READ, () => {
      const w = getWorld()
      const p = findProfile(w, name)
      if (!p) throw fail(ErrorCode.ErrProfileNotFound, `profile ${name} not found`, { name })
      const lines: DiffLine[] = [fold(10), del(`[profile ${name}]`)]
      if (p.kind === ProfileKind.ProfileKindSSO) lines.push(del(`sso_session    = ${p.portal}`), del(`sso_account_id = ${p.accountId}`), del(`sso_role_name  = ${p.role}`))
      if (p.region) lines.push(del(`region         = ${p.region}`))
      lines.push(fold(8))
      return mkPreview(w, '~/.aws/config', `${HOME}/.aws/config`, lines)
    }),

  RemoveProfile: (name, _baseHash) =>
    run(WRITE, () => {
      const w = getWorld()
      requireWritable(w)
      if (!findProfile(w, name)) throw fail(ErrorCode.ErrProfileNotFound, `profile ${name} not found`, { name })
      w.accounts.profiles = (w.accounts.profiles ?? []).filter((p) => p.name !== name)
      const b = addBackup(w, BackupKind.BackupAWSConfig, BackupReason.ReasonRemoveProfile, { name })
      recompute(w)
      mockEmit('accounts:changed', deepCopy(w.accounts))
      return { file: '~/.aws/config', backupId: b.id }
    }),

  SetProfileIdentity: (u) =>
    run(WRITE, () => {
      const w = getWorld()
      const p = findProfile(w, u.profile)
      if (!p) throw fail(ErrorCode.ErrProfileNotFound, `profile ${u.profile} not found`, { name: u.profile })
      if (!Number.isInteger(u.colorSlot) || u.colorSlot < 1 || u.colorSlot > 8) throw fail(ErrorCode.ErrInvalidInput, 'colour slot out of range', { field: 'colorSlot' })
      const raw = u.monogram.trim()
      if (raw && !/^[A-Za-z0-9]{2}$/.test(raw) && !/^[\u4e00-\u9fff]$/.test(raw)) throw fail(ErrorCode.ErrInvalidInput, 'invalid monogram', { field: 'monogram' })
      const sameAccount = (x: typeof p) => (p.accountId ? x.accountId === p.accountId : x.name === p.name)
      for (const x of w.accounts.profiles ?? []) if (sameAccount(x)) x.colorSlot = u.colorSlot
      const taken = new Set((w.accounts.profiles ?? []).filter((x) => x.name !== p.name).map((x) => x.monogram))
      p.monogram = raw ? raw.toUpperCase() : autoMonogram(p.name, taken)
      recompute(w)
      mockEmit('accounts:changed', deepCopy(w.accounts))
      return accountsState(w)
    }),

  OpenPortal: (portal) =>
    run(READ, () => {
      const w = getWorld()
      const p = requirePortal(w, portal)
      console.info('[mock] open portal', p.startUrl)
    }),

  // "Open browser" (device code) and "Open again": reports how the page opened, like the first open.
  OpenSignInPage: (url) =>
    run(READ, () => {
      let u: URL | undefined
      try {
        u = new URL(url)
      } catch {
        /* rejected below */
      }
      const host = u?.hostname.toLowerCase() ?? ''
      const okHost = HOSTS_EXACT.includes(host) || HOST_SUFFIXES.some((x) => host.endsWith(x) && host.length > x.length) || SIGN_IN_DEVICE_HOST_RE.test(host)
      if (!u || u.protocol !== 'https:' || u.username || u.password || !okHost)
        throw fail(ErrorCode.ErrInvalidInput, 'not a sign-in page URL', { field: 'url' })
      console.info('[mock] open sign-in page', url)
      const { privateSignIn } = getWorld().settings
      return { private: privateSignIn, isolated: privateSignIn }
    }),

  // two network calls in the app (about 1 s)
  OpenConsole: (profile) =>
    run(SLOW, () => {
      const w = getWorld()
      const p = findProfile(w, profile)
      if (!p || p.kind !== ProfileKind.ProfileKindSSO) throw fail(ErrorCode.ErrInvalidInput, 'console needs an SSO profile', { field: 'profile' })
      console.info('[mock] open console', profile)
    }),

  PreviewRenamePortal: (oldName, newName) =>
    run(READ, () => {
      const w = getWorld()
      requirePortal(w, oldName)
      if (!/^[A-Za-z0-9_.-]+$/.test(newName)) throw fail(ErrorCode.ErrInvalidName, 'invalid name', { name: newName })
      if (findPortal(w, newName)) throw fail(ErrorCode.ErrPortalExists, 'portal exists', { portal: newName })
      const lines: DiffLine[] = [fold(4), del(`[sso-session ${oldName}]`), add(`[sso-session ${newName}]`), ctx('sso_start_url = ...'), fold(10)]
      return mkPreview(w, '~/.aws/config', `${HOME}/.aws/config`, lines)
    }),

  RenamePortal: (oldName, newName, _baseHash) =>
    run(WRITE, () => {
      const w = getWorld()
      const p = requirePortal(w, oldName)
      if (findPortal(w, newName)) throw fail(ErrorCode.ErrPortalExists, 'portal exists', { portal: newName })
      for (const pr of w.accounts.profiles ?? []) if (pr.portal === oldName) pr.portal = newName
      w.portalModels[newName] = w.portalModels[oldName]
      delete w.portalModels[oldName]
      w.discovery[newName] = w.discovery[oldName]
      delete w.discovery[oldName]
      p.name = newName
      const b = addBackup(w, BackupKind.BackupAWSConfig, BackupReason.ReasonPortal, { portal: newName })
      recompute(w)
      mockEmit('accounts:changed', deepCopy(w.accounts))
      return { file: '~/.aws/config', backupId: b.id }
    }),

  PreviewRemovePortal: (name) =>
    run(READ, () => {
      const w = getWorld()
      const p = requirePortal(w, name)
      const n = p.profiles?.length ?? 0
      if (n > 0) throw fail(ErrorCode.ErrPortalInUse, `${n} profiles use ${name}`, { portal: name, n: String(n) })
      return mkPreview(w, '~/.aws/config', `${HOME}/.aws/config`, [fold(4), del(`[sso-session ${name}]`), del(`sso_start_url = ${p.startUrl}`), del(`sso_region = ${p.region}`), fold(6)])
    }),

  RemovePortal: (name, _baseHash) =>
    run(WRITE, () => {
      const w = getWorld()
      const p = requirePortal(w, name)
      const n = p.profiles?.length ?? 0
      if (n > 0) throw fail(ErrorCode.ErrPortalInUse, `${n} profiles use ${name}`, { portal: name, n: String(n) })
      w.accounts.portals = (w.accounts.portals ?? []).filter((x) => x.name !== name)
      delete w.portalModels[name]
      const b = addBackup(w, BackupKind.BackupAWSConfig, BackupReason.ReasonPortal, { portal: name })
      recompute(w)
      mockEmit('accounts:changed', deepCopy(w.accounts))
      return { file: '~/.aws/config', backupId: b.id }
    }),

  BuildExport: (req) => run(READ, () => buildExport(getWorld(), req)),

  SaveExport: (req) =>
    run(WRITE, () => {
      const res = buildExport(getWorld(), req)
      if (!(res.items ?? []).some((i) => i.included)) throw fail(ErrorCode.ErrInvalidInput, 'nothing to export', { field: 'portals' })
      return { ...res, path: `${HOME}/Desktop/${exportFileName()}` }
    }),

  OpenImportFile: () => run(READ, () => sampleImportFile()),

  PreviewImport: (req) => run(READ, () => previewImport(getWorld(), req)),

  ApplyImport: (req, _baseHash) =>
    run(WRITE, () => {
      const w = getWorld()
      requireWritable(w)
      return applyImport(w, req)
    }),
}
