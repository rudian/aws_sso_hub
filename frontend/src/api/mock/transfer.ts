// Mock of the accounts export / import (AccountService.BuildExport ... ApplyImport). It follows the backend
// rules: an allow-list document, strict validation, rejection of credential-like keys and control characters,
// planning against the mock world (new / same / conflict / mapped / rejected) and one backup per import.
import { BackupKind, BackupReason, ErrorCode, ExportItemKind, ImportAction, ImportItemKind, ProfileKind, SessionState } from '../models'
import type { DiffLine, ExportItem, ExportRequest, ExportResult, ImportItem, ImportPreview, ImportRequest, ImportResult, ImportSource, Portal, Profile } from '../models'
import { mockEmit } from '../events'
import { deepCopy, iso, now } from './clock'
import { HOME, add, addBackup, autoMonogram, ctx, del, findPortal, findProfile, fold, mkPortal, mkPreview, newSessionModel, portalState, recompute, sortPortals } from './world'
import type { World } from './world'
import { fail } from './util'

const FORMAT = 'aws-sso-hub.accounts'
// Same limit as the backend (transfer.MaxFileBytes): the text is sent back with every preview and apply call,
// and the web view splits call bodies above 512 KiB into chunks that the runtime guard refuses.
export const MAX_IMPORT_BYTES = 384 * 1024
const MAX_BYTES = MAX_IMPORT_BYTES
const SESSION_NAME_RE = /^[A-Za-z0-9_.-]{1,64}$/
const PROFILE_NAME_RE = /^[A-Za-z0-9_.@+=,-]{1,64}$/
const REGION_RE = /^[a-z]{2}(-gov|-iso[a-z]*)?-[a-z]+-[0-9]{1,2}$/
const ROLE_RE = /^[A-Za-z0-9_+=,.@-]{1,64}$/
const SENSITIVE_KEY_RE = /secret|token|password|process|command/i
const CREDENTIAL_KEYS = new Set(['awsaccesskeyid', 'awssecretaccesskey', 'awssessiontoken', 'awssecuritytoken', 'credentialprocess', 'credentialsource', 'rolearn', 'sourceprofile', 'webidentitytokenfile'])
const KEY_VALUE_RE = /A(KI|SI)A[0-9A-Z]{16}/
const OUTPUTS = ['json', 'text', 'table', 'yaml', 'yaml-stream']
const DEFAULT_SCOPES = ['sso:account:access']

const normKey = (k: string) => k.toLowerCase().replace(/[_-]/g, '')
// eslint-disable-next-line no-control-regex
const UNSAFE_RE = /[\u0000-\u001f\u007f-\u009f\u2028\u2029\ufeff]/

// ------------------------------------------------------------------ export

export function buildExport(w: World, req: ExportRequest): ExportResult {
  const wantPortal = new Set(req.portals ?? [])
  const wantProfile = new Set(req.profiles ?? [])
  for (const n of wantPortal) if (!findPortal(w, n) || findPortal(w, n)?.draft) throw fail(ErrorCode.ErrPortalNotFound, `portal ${n} not found`, { portal: n })
  for (const n of wantProfile) if (!findProfile(w, n)) throw fail(ErrorCode.ErrProfileNotFound, `profile ${n} not found`, { name: n })

  const items: ExportItem[] = []
  const portals: Array<Record<string, unknown>> = []
  const profiles: Array<Record<string, unknown>> = []
  const realPortals = (w.accounts.portals ?? []).filter((p) => !p.draft && !p.legacyFormat).sort((a, b) => a.name.localeCompare(b.name))
  for (const p of realPortals) {
    const inc = wantPortal.has(p.name)
    items.push({ kind: ExportItemKind.ExportKindPortal, name: p.name, included: inc })
    if (!inc) continue
    const e: Record<string, unknown> = { name: p.name, startUrl: p.startUrl, region: p.region, scopes: [...DEFAULT_SCOPES] }
    if (req.includeExtras) e.sessionMinutes = p.session.sessionMinutes
    portals.push(e)
  }
  const sorted = [...(w.accounts.profiles ?? [])].sort((a, b) => a.name.localeCompare(b.name))
  for (const p of sorted) {
    const item: ExportItem = { kind: ExportItemKind.ExportKindProfile, name: p.name, portal: p.portal, included: false }
    if (p.kind === ProfileKind.ProfileKindSSO) {
      const portalOk = !!p.portal && wantPortal.has(p.portal)
      if (!portalOk) {
        if (wantProfile.has(p.name)) item.reasonKey = 'export.noPortal'
      } else {
        item.included = wantProfile.has(p.name)
      }
    } else if (p.kind === ProfileKind.ProfileKindLegacySSO) item.reasonKey = 'export.legacy'
    else if (p.kind === ProfileKind.ProfileKindStatic) item.reasonKey = 'export.static'
    else continue
    items.push(item)
    if (!item.included) continue
    const e: Record<string, unknown> = { name: p.name, portal: p.portal, accountId: p.accountId, roleName: p.role }
    if (p.region) e.region = p.region
    e.output = 'json'
    if (req.includeExtras) {
      if (p.accountName) e.accountName = p.accountName
      e.color = p.colorSlot
      e.monogram = p.monogram
    }
    profiles.push(e)
  }
  const doc = { format: FORMAT, version: 1, exportedAt: iso(now()), exportedBy: `Codepass ${w.appInfo.version}`, portals, profiles }
  return { json: JSON.stringify(doc, null, 2) + '\n', items }
}

export function exportFileName(): string {
  return `codepass-accounts-${iso(now()).slice(0, 10).replace(/-/g, '')}.json`
}

// ------------------------------------------------------------------ sample file for OpenImportFile

export function sampleImportFile(): ImportSource {
  const doc = {
    format: FORMAT,
    version: 1,
    exportedAt: '2026-11-03T09:30:00Z',
    exportedBy: 'Codepass 0.0.1',
    portals: [
      { name: 'initech', startUrl: 'https://d-1234567890.awsapps.com/start', region: 'us-west-2', scopes: ['sso:account:access'], sessionMinutes: 480 },
      { name: 'globex', startUrl: 'https://d-2222222222.awsapps.com/start', region: 'eu-west-1', scopes: ['sso:account:access'], sessionMinutes: 600 },
    ],
    profiles: [
      { name: 'initech-admin', portal: 'initech', accountId: '777788889999', roleName: 'ReadOnlyAccess', region: 'us-west-2', output: 'json' },
      { name: 'globex-admin', portal: 'globex', accountId: '444455556666', roleName: 'AdministratorAccess', region: 'eu-west-1', output: 'json', accountName: 'globex', color: 6, monogram: 'GX' },
      { name: 'initech-legacy', portal: 'initech', accountId: '777788889999', roleName: 'PowerUserAccess', credential_process: '/usr/local/bin/get-creds --profile initech' },
    ],
  }
  return { name: 'codepass-accounts-20261103.json', json: JSON.stringify(doc, null, 2) + '\n' }
}

// ------------------------------------------------------------------ import: parsing

interface Rej {
  reasonKey: string
  code?: ErrorCode
  detail?: string
}
interface PortalEntry {
  name: string
  startUrl: string
  region: string
  scopes: string[]
  sessionMinutes: number
  warnings: string[]
  rej?: Rej
}
interface ProfileEntry {
  name: string
  portal: string
  accountId: string
  role: string
  region: string
  output: string
  accountName: string
  color: number
  monogram: string
  warnings: string[]
  rej?: Rej
}

const formatErr = (reason: string) => fail(ErrorCode.ErrImportFormat, 'not a Codepass accounts export', { reason })

function hasSensitive(v: unknown): boolean {
  if (Array.isArray(v)) return v.some(hasSensitive)
  if (v && typeof v === 'object') return Object.entries(v as Record<string, unknown>).some(([k, x]) => SENSITIVE_KEY_RE.test(k) || CREDENTIAL_KEYS.has(normKey(k)) || hasSensitive(x))
  return typeof v === 'string' && KEY_VALUE_RE.test(v)
}
function hasUnsafe(v: unknown): boolean {
  if (Array.isArray(v)) return v.some(hasUnsafe)
  if (v && typeof v === 'object') return Object.entries(v as Record<string, unknown>).some(([k, x]) => UNSAFE_RE.test(k) || hasUnsafe(x))
  return typeof v === 'string' && UNSAFE_RE.test(v)
}
const clean = (s: string, max = 64) => (s.length > max ? s.slice(0, max) + '…' : s).replace(UNSAFE_RE, '?')

function normalizeUrl(raw: string): string | null {
  let u: URL
  try {
    u = new URL(raw.trim().replace(/#\/?$/, ''))
  } catch {
    return null
  }
  const host = u.hostname.toLowerCase()
  const suffixes = ['.awsapps.com', '.app.aws', '.awsapps.cn', '.portal.amazonaws.com', '.portal.amazonaws.com.cn', '.app.amazonwebservices.com.cn', '.portal.amazonaws.eu', '.api.amazonwebservices.eu']
  if (u.protocol !== 'https:' || !suffixes.some((s) => host.endsWith(s) && host.length > s.length)) return null
  let path = u.pathname
  if (host.endsWith('.awsapps.com')) path = (path === '' || path === '/' ? '/start' : path).replace(/\/+$/, '')
  else if (path === '/') path = ''
  return `https://${host}${path}`
}

function screen(raw: unknown): { m?: Record<string, unknown>; name: string; rej?: Rej } {
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) return { name: '?', rej: { reasonKey: 'import.rejected.invalid', detail: 'entry' } }
  const m = raw as Record<string, unknown>
  const name = typeof m.name === 'string' && m.name ? clean(m.name) : '?'
  if (hasSensitive(m)) return { name, rej: { reasonKey: 'import.rejected.secret' } }
  if (hasUnsafe(m)) return { name, rej: { reasonKey: 'import.rejected.invalid', code: ErrorCode.ErrUnsafeValue } }
  return { m, name }
}

class Fields {
  rej?: Rej
  used = new Set<string>()
  constructor(readonly m: Record<string, unknown>) {}
  bad(code: ErrorCode, detail: string) {
    this.rej ??= { reasonKey: 'import.rejected.invalid', code, detail }
  }
  str(k: string): string {
    this.used.add(k)
    const v = this.m[k]
    if (v === undefined || v === null) return ''
    if (typeof v !== 'string') {
      this.bad(ErrorCode.ErrInvalidInput, k)
      return ''
    }
    return v.trim()
  }
  int(k: string): number {
    this.used.add(k)
    const v = this.m[k]
    if (v === undefined || v === null) return 0
    if (typeof v !== 'number' || !Number.isInteger(v)) {
      this.bad(ErrorCode.ErrInvalidInput, k)
      return 0
    }
    return v
  }
  list(k: string): string[] {
    this.used.add(k)
    const v = this.m[k]
    if (v === undefined || v === null) return []
    if (!Array.isArray(v) || v.length > 10 || v.some((x) => typeof x !== 'string')) {
      this.bad(ErrorCode.ErrInvalidInput, k)
      return []
    }
    return (v as string[]).map((x) => x.trim())
  }
  unknown(): string[] {
    return Object.keys(this.m)
      .filter((k) => !this.used.has(k))
      .sort()
      .map((k) => `unknown-key:${clean(k, 40)}`)
  }
}

function parsePortal(raw: unknown): PortalEntry {
  const s = screen(raw)
  const e: PortalEntry = { name: s.name, startUrl: '', region: '', scopes: [], sessionMinutes: 0, warnings: [] }
  if (s.rej || !s.m) {
    e.rej = s.rej
    return e
  }
  const f = new Fields(s.m)
  const name = f.str('name')
  if (!SESSION_NAME_RE.test(name)) f.bad(ErrorCode.ErrInvalidName, 'name')
  else e.name = name
  const url = normalizeUrl(f.str('startUrl'))
  if (!url) f.bad(ErrorCode.ErrInvalidStartURL, 'startUrl')
  e.startUrl = url ?? ''
  e.region = f.str('region') || (/^[^.]+\.portal\.([a-z0-9-]+)\.app\.aws$/.exec(url ? new URL(url).hostname : '')?.[1] ?? '')
  if (!REGION_RE.test(e.region)) f.bad(ErrorCode.ErrInvalidRegion, 'region')
  e.scopes = f.list('scopes')
  if (e.scopes.some((x) => !/^[A-Za-z0-9:_./-]{1,128}$/.test(x))) f.bad(ErrorCode.ErrInvalidInput, 'scopes')
  e.sessionMinutes = f.int('sessionMinutes')
  if (e.sessionMinutes !== 0 && (e.sessionMinutes < 15 || e.sessionMinutes > 129600)) f.bad(ErrorCode.ErrInvalidInput, 'sessionMinutes')
  e.warnings = f.unknown()
  e.rej = f.rej
  return e
}

function parseProfile(raw: unknown): ProfileEntry {
  const s = screen(raw)
  const e: ProfileEntry = { name: s.name, portal: '', accountId: '', role: '', region: '', output: '', accountName: '', color: 0, monogram: '', warnings: [] }
  if (s.rej || !s.m) {
    e.rej = s.rej
    return e
  }
  const f = new Fields(s.m)
  const name = f.str('name')
  if (!PROFILE_NAME_RE.test(name)) f.bad(ErrorCode.ErrInvalidName, 'name')
  else e.name = name
  e.portal = f.str('portal')
  if (!SESSION_NAME_RE.test(e.portal)) f.bad(ErrorCode.ErrInvalidName, 'portal')
  e.accountId = f.str('accountId')
  if (!/^[0-9]{12}$/.test(e.accountId)) f.bad(ErrorCode.ErrInvalidInput, 'accountId')
  e.role = f.str('roleName')
  if (!ROLE_RE.test(e.role)) f.bad(ErrorCode.ErrInvalidInput, 'roleName')
  e.region = f.str('region')
  if (e.region && !REGION_RE.test(e.region)) f.bad(ErrorCode.ErrInvalidRegion, 'region')
  e.output = f.str('output')
  if (e.output && !OUTPUTS.includes(e.output)) f.bad(ErrorCode.ErrInvalidInput, 'output')
  e.accountName = f.str('accountName')
  if ([...e.accountName].length > 128) f.bad(ErrorCode.ErrInvalidInput, 'accountName')
  e.color = f.int('color')
  if (e.color !== 0 && (e.color < 1 || e.color > 8)) f.bad(ErrorCode.ErrInvalidInput, 'color')
  e.monogram = f.str('monogram')
  if (e.monogram && !/^[A-Za-z0-9]{2}$/.test(e.monogram) && !/^[一-鿿]$/.test(e.monogram)) f.bad(ErrorCode.ErrInvalidInput, 'monogram')
  e.warnings = f.unknown()
  e.rej = f.rej
  return e
}

function parseDocument(text: string): { exportedBy: string; portals: PortalEntry[]; profiles: ProfileEntry[] } {
  if (text.length > MAX_BYTES) throw formatErr('size')
  let top: unknown
  try {
    top = JSON.parse(text.replace(/^\ufeff/, ''))
  } catch {
    throw formatErr('not-json')
  }
  if (!top || typeof top !== 'object' || Array.isArray(top)) throw formatErr('not-json')
  const t = top as Record<string, unknown>
  if (t.format !== FORMAT) throw formatErr('format')
  if (t.version !== 1) throw formatErr('version')
  const list = (v: unknown, max: number) => {
    if (v === undefined || v === null) return []
    if (!Array.isArray(v)) throw formatErr('format')
    if (v.length > max) throw formatErr('limit')
    return v as unknown[]
  }
  const portals = list(t.portals, 200).map(parsePortal)
  const profiles = list(t.profiles, 5000).map(parseProfile)
  const dup = (es: Array<{ name: string; rej?: Rej }>) => {
    const seen = new Set<string>()
    for (const e of es) {
      if (e.rej) continue
      if (seen.has(e.name)) e.rej = { reasonKey: 'import.rejected.invalid', code: ErrorCode.ErrInvalidName, detail: 'name' }
      seen.add(e.name)
    }
  }
  dup(portals)
  dup(profiles)
  return { exportedBy: typeof t.exportedBy === 'string' ? clean(t.exportedBy, 80) : '', portals, profiles }
}

// ------------------------------------------------------------------ import: planning

interface Plan {
  items: ImportItem[]
  lines: DiffLine[]
  ops: Array<() => void>
  changed: number
  extras: number
  needSign: Set<string>
  source: string
}

const uniqueName = (base: string, taken: Set<string>) => {
  for (let i = 2; i < 1000; i++) {
    const c = `${base.slice(0, 64 - String(i).length - 1)}-${i}`
    if (!taken.has(c)) return c
  }
  return `${base}-x`
}

export function planImport(w: World, req: ImportRequest): Plan {
  const parsed = parseDocument(req.json)
  const choices = req.choices ?? {}
  for (const c of Object.values(choices)) if (!['skip', 'replace', 'rename'].includes(c ?? '')) throw fail(ErrorCode.ErrInvalidInput, 'invalid choice', { field: 'choices' })
  const include = (k: string) => req.include?.[k] !== false
  const realPortals = (w.accounts.portals ?? []).filter((p) => !p.draft && !p.legacyFormat)
  const takenPortal = new Set(realPortals.map((p) => p.name))
  const takenProfile = new Set((w.accounts.profiles ?? []).map((p) => p.name))
  parsed.portals.forEach((e) => !e.rej && takenPortal.add(e.name))
  parsed.profiles.forEach((e) => !e.rej && takenProfile.add(e.name))

  const plan: Plan = { items: [], lines: [fold(10)], ops: [], changed: 0, extras: 0, needSign: new Set(), source: parsed.exportedBy }
  const usedP = new Set<string>()
  const usedQ = new Set<string>()
  const renameTo = (k: string, suggested: string, valid: RegExp, taken: Set<string>, used: Set<string>, code: ErrorCode, param: string) => {
    const nn = (req.renames?.[k] ?? '').trim() || suggested
    if (!valid.test(nn)) throw fail(ErrorCode.ErrInvalidName, 'invalid name', { name: nn })
    if (taken.has(nn) || used.has(nn)) throw fail(code, 'name already in use', { [param]: nn })
    used.add(nn)
    return nn
  }
  const colorsSeen = new Set<string>()
  const extraProfile = (inc: boolean, e: ProfileEntry, target: string, replace: boolean) => {
    if (!req.importExtras || !inc) return
    const existing = (w.accounts.profiles ?? []).find((p) => p.accountId === e.accountId)
    if (e.color && !colorsSeen.has('c' + e.accountId) && (!existing || replace) && existing?.colorSlot !== e.color) {
      colorsSeen.add('c' + e.accountId)
      plan.extras++
      plan.ops.push(() => (w.accounts.profiles ?? []).filter((p) => p.accountId === e.accountId).forEach((p) => (p.colorSlot = e.color)))
    }
    if (e.accountName && !colorsSeen.has('n' + e.accountId) && (!existing?.accountName || replace) && existing?.accountName !== e.accountName) {
      colorsSeen.add('n' + e.accountId)
      plan.extras++
      plan.ops.push(() => (w.accounts.profiles ?? []).filter((p) => p.accountId === e.accountId).forEach((p) => (p.accountName = e.accountName)))
    }
    // an existing monogram is the user's own choice unless this entry replaces the profile
    const cur = findProfile(w, target)
    if (e.monogram && cur?.monogram !== e.monogram.toUpperCase() && (replace || !cur)) {
      plan.extras++
      plan.ops.push(() => {
        const p = findProfile(w, target)
        if (p) p.monogram = e.monogram.toUpperCase()
      })
    }
  }

  // portals
  const res = new Map<string, { ok: boolean; target: string; region: string; differs: boolean }>()
  const rejectedPortals = new Set<string>()
  for (const e of parsed.portals) {
    const k = `portal:${e.name}`
    const it: ImportItem = { key: k, kind: ImportItemKind.ImportKindPortal, name: e.name, action: ImportAction.ImportActionNew, include: true, choice: '', target: e.name, suggested: '', canReplace: false, warnings: [...e.warnings] }
    if (e.rej) {
      rejectedPortals.add(e.name)
      Object.assign(it, { action: ImportAction.ImportActionRejected, include: false, reasonKey: e.rej.reasonKey, code: e.rej.code, detail: e.rej.detail })
      plan.items.push(it)
      continue
    }
    it.startUrl = e.startUrl
    it.region = e.region
    it.include = include(k)
    const scopes = e.scopes.length ? e.scopes : DEFAULT_SCOPES
    const ex = realPortals.find((p) => p.name === e.name)
    const sameAs = ex && ex.startUrl === e.startUrl && ex.region === e.region
    const sessionExtra = (target: string, replace: boolean) => {
      if (!req.importExtras || !it.include || !e.sessionMinutes) return
      const cur = w.portalModels[target]?.sessionMinutes ?? 480
      if (cur !== e.sessionMinutes && (replace || cur === 480)) {
        plan.extras++
        plan.ops.push(() => {
          const m = w.portalModels[target] ?? (w.portalModels[target] = newSessionModel())
          m.sessionMinutes = e.sessionMinutes
        })
      }
    }
    const addPortal = (name: string) => {
      plan.lines.push(add(`[sso-session ${name}]`), add(`sso_start_url           = ${e.startUrl}`), add(`sso_region              = ${e.region}`), add(`sso_registration_scopes = ${scopes.join(',')}`), add(''))
      plan.ops.push(() => {
        const p = mkPortal(name, e.startUrl, e.region)
        w.accounts.portals = sortPortals([...(w.accounts.portals ?? []), p])
        w.portalModels[name] = newSessionModel()
      })
      plan.changed++
      plan.needSign.add(name)
    }
    if (sameAs) {
      it.action = ImportAction.ImportActionSame
      res.set(e.name, { ok: true, target: e.name, region: ex.region, differs: false })
      sessionExtra(e.name, false)
    } else if (ex) {
      it.action = ImportAction.ImportActionConflict
      it.canReplace = true
      it.suggested = uniqueName(e.name, takenPortal)
      it.choice = it.include ? (choices[k] ?? 'skip') : 'skip'
      if (it.choice === 'replace') {
        plan.lines.push(del(`sso_start_url = ${ex.startUrl}`), add(`sso_start_url = ${e.startUrl}`))
        plan.ops.push(() => {
          const p = findPortal(w, e.name)
          if (p) Object.assign(p, { startUrl: e.startUrl, startUrlHost: e.startUrl.replace(/^https:\/\//, ''), region: e.region })
        })
        plan.changed++
        plan.needSign.add(e.name)
        res.set(e.name, { ok: true, target: e.name, region: e.region, differs: false })
        sessionExtra(e.name, true)
      } else if (it.choice === 'rename') {
        const nn = renameTo(k, it.suggested, SESSION_NAME_RE, takenPortal, usedP, ErrorCode.ErrPortalExists, 'portal')
        it.target = nn
        addPortal(nn)
        res.set(e.name, { ok: true, target: nn, region: e.region, differs: false })
        sessionExtra(nn, false)
      } else {
        res.set(e.name, { ok: true, target: e.name, region: ex.region, differs: true })
      }
    } else {
      const mapped = realPortals.find((p) => p.startUrl === e.startUrl && p.region === e.region)
      if (mapped) {
        it.action = ImportAction.ImportActionMapped
        it.target = mapped.name
        res.set(e.name, { ok: true, target: mapped.name, region: mapped.region, differs: false })
        sessionExtra(mapped.name, false)
      } else if (it.include) {
        addPortal(e.name)
        res.set(e.name, { ok: true, target: e.name, region: e.region, differs: false })
        sessionExtra(e.name, false)
      }
    }
    plan.items.push(it)
  }

  // profiles
  for (const e of parsed.profiles) {
    const k = `profile:${e.name}`
    const it: ImportItem = { key: k, kind: ImportItemKind.ImportKindProfile, name: e.name, action: ImportAction.ImportActionNew, include: true, choice: '', target: e.name, suggested: '', canReplace: false, warnings: [...e.warnings] }
    if (e.rej) {
      Object.assign(it, { action: ImportAction.ImportActionRejected, include: false, reasonKey: e.rej.reasonKey, code: e.rej.code, detail: e.rej.detail })
      plan.items.push(it)
      continue
    }
    Object.assign(it, { portal: e.portal, accountId: e.accountId, role: e.role, region: e.region || undefined })
    it.include = include(k)
    let pr = res.get(e.portal)
    if (!pr && !rejectedPortals.has(e.portal)) {
      const ex = realPortals.find((p) => p.name === e.portal)
      if (ex) pr = { ok: true, target: ex.name, region: ex.region, differs: false }
    }
    if (!pr || !pr.ok) {
      Object.assign(it, { action: ImportAction.ImportActionRejected, include: false, reasonKey: 'import.rejected.noPortal', detail: e.portal })
      plan.items.push(it)
      continue
    }
    if (pr.differs) it.warnings = [...(it.warnings ?? []), `portal-differs:${e.portal}`]
    it.portal = pr.target
    const portalTarget = pr.target
    const portalRegion = pr.region
    const ex = findProfile(w, e.name)
    const same = ex && ex.kind === ProfileKind.ProfileKindSSO && ex.portal === portalTarget && ex.accountId === e.accountId && ex.role === e.role && (!e.region || ex.region === e.region)
    const addProfile = (name: string) => {
      plan.lines.push(add(`[profile ${name}]`), add(`sso_session    = ${portalTarget}`), add(`sso_account_id = ${e.accountId}`), add(`sso_role_name  = ${e.role}`), add(`region         = ${e.region || portalRegion}`), add(`output         = ${e.output || 'json'}`), add(''))
      plan.ops.push(() => {
        const taken = new Set((w.accounts.profiles ?? []).map((p) => p.monogram))
        const known = (w.accounts.profiles ?? []).find((p) => p.accountId === e.accountId)
        w.accounts.profiles = [
          ...(w.accounts.profiles ?? []),
          {
            name,
            kind: ProfileKind.ProfileKindSSO,
            portal: portalTarget,
            accountId: e.accountId,
            accountName: known?.accountName ?? '',
            role: e.role,
            region: e.region || portalRegion,
            colorSlot: known?.colorSlot ?? 1,
            monogram: autoMonogram(name, taken),
            isRootKey: false,
            hasStaticKeys: false,
            boundRepos: 0,
            isGlobalDefault: false,
            globalDefaultRepos: 0,
            migrationInProgress: false,
          } as Profile,
        ]
      })
      plan.changed++
      plan.needSign.add(portalTarget)
    }
    if (same) {
      it.action = ImportAction.ImportActionSame
      extraProfile(it.include, e, e.name, false)
    } else if (ex) {
      it.action = ImportAction.ImportActionConflict
      it.canReplace = ex.kind === ProfileKind.ProfileKindSSO
      it.suggested = uniqueName(e.name, takenProfile)
      it.choice = it.include ? (choices[k] ?? 'skip') : 'skip'
      if (it.choice === 'replace' && !it.canReplace) throw fail(ErrorCode.ErrInvalidInput, 'cannot replace this profile', { field: 'choices' })
      if (it.choice === 'replace') {
        plan.lines.push(ctx(`[profile ${e.name}]`), del(`sso_role_name  = ${ex.role}`), add(`sso_role_name  = ${e.role}`))
        plan.ops.push(() => {
          const p = findProfile(w, e.name)
          if (p) Object.assign(p, { portal: portalTarget, accountId: e.accountId, role: e.role, region: e.region || p.region })
        })
        plan.changed++
        plan.needSign.add(portalTarget)
        extraProfile(it.include, e, e.name, true)
      } else if (it.choice === 'rename') {
        const nn = renameTo(k, it.suggested, PROFILE_NAME_RE, takenProfile, usedQ, ErrorCode.ErrProfileExists, 'name')
        it.target = nn
        addProfile(nn)
        extraProfile(it.include, e, nn, false)
      }
    } else if (it.include) {
      addProfile(e.name)
      extraProfile(true, e, e.name, false)
    }
    plan.items.push(it)
  }
  return plan
}


export function previewImport(w: World, req: ImportRequest): ImportPreview {
  const plan = planImport(w, req)
  if (plan.changed > 0) plan.lines.push(fold(6))
  const lines = plan.changed > 0 ? plan.lines : []
  const prev = mkPreview(w, '~/.aws/config', `${HOME}/.aws/config`, lines)
  return { items: plan.items, preview: prev, baseHash: prev.baseHash, extrasCount: plan.extras, source: plan.source, toWrite: plan.changed }
}

export function applyImport(w: World, req: ImportRequest): ImportResult {
  const plan = planImport(w, req)
  const res: ImportResult = { added: 0, replaced: 0, renamed: 0, skipped: 0, rejected: 0, needSignIn: [], write: { file: '~/.aws/config', backupId: '' } }
  for (const it of plan.items) {
    if (it.action === ImportAction.ImportActionRejected) res.rejected++
    else if (it.action === ImportAction.ImportActionNew) it.include ? res.added++ : res.skipped++
    else if (it.action === ImportAction.ImportActionConflict) {
      if (it.choice === 'replace') res.replaced++
      else if (it.choice === 'rename') res.renamed++
      else res.skipped++
    }
  }
  if (plan.changed > 0) {
    if (w.accounts.readOnly) throw fail(ErrorCode.ErrReadOnly, 'config file has a parse issue')
    res.write.backupId = addBackup(w, BackupKind.BackupAWSConfig, BackupReason.ReasonImport, { n: String(plan.changed) }).id
  }
  plan.ops.forEach((op) => op())
  recompute(w)
  res.needSignIn = [...plan.needSign].filter((n) => ![SessionState.SessionActive, SessionState.SessionExpiring, SessionState.SessionRefreshable, SessionState.SessionSigningIn].includes(portalState(w, n))).sort()
  mockEmit('accounts:changed', deepCopy(w.accounts))
  return res
}

export type { Portal }
