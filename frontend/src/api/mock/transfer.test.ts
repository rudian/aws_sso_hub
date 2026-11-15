import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { toUiError } from '../errors'
import { BackupReason, ErrorCode, ImportAction } from '../models'
import type { ExportRequest, ImportRequest } from '../models'
import { mock } from './index'
import { getWorld, resetWorld } from './world'

async function call<T>(p: Promise<T>, ms = 2000): Promise<T> {
  const settled = p.then(
    (v) => ({ ok: true as const, v }),
    (e: unknown) => ({ ok: false as const, e }),
  )
  await vi.advanceTimersByTimeAsync(ms)
  const r = await settled
  if (!r.ok) throw r.e
  return r.v
}
const code = async (p: Promise<unknown>) => {
  try {
    await call(p)
    return null
  } catch (e) {
    return toUiError(e).code
  }
}

const everything = (): ExportRequest => {
  const w = getWorld()
  return {
    portals: (w.accounts.portals ?? []).filter((p) => !p.draft).map((p) => p.name),
    profiles: (w.accounts.profiles ?? []).map((p) => p.name),
    includeExtras: true,
  }
}

const doc = (portals: unknown[], profiles: unknown[]) =>
  JSON.stringify({ format: 'aws-sso-hub.accounts', version: 1, exportedAt: '2026-11-04T10:00:00Z', exportedBy: 'Codepass 0.0.1', portals, profiles })

describe('mock accounts export and import', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-11-04T04:00:00Z'))
  })
  afterEach(() => vi.useRealTimers())

  it('exports SSO portals and profiles only, never keys', async () => {
    resetWorld('fresh')
    const res = await call(mock.accounts.BuildExport(everything()))
    const parsed = JSON.parse(res.json)
    expect(parsed.format).toBe('aws-sso-hub.accounts')
    expect(parsed.version).toBe(1)
    expect(parsed.portals.map((p: { name: string }) => p.name)).toEqual(['initech'])
    expect(parsed.profiles.map((p: { name: string }) => p.name)).toEqual(['initech-admin', 'initech-readonly'])
    expect(res.json).not.toMatch(/A(KI|SI)A[0-9A-Z]{16}|secret|AKIA/i)
    const skipped = (res.items ?? []).filter((i) => !i.included).map((i) => `${i.name}:${i.reasonKey}`)
    expect(skipped).toEqual(['acme:export.static', 'globex:export.static'])
    const bare = await call(mock.accounts.BuildExport({ ...everything(), includeExtras: false }))
    expect(bare.json).not.toMatch(/"color"|"monogram"|"accountName"|"sessionMinutes"/)
  })

  it('SaveExport returns the path of the chosen file and refuses an empty selection', async () => {
    resetWorld('fresh')
    const res = await call(mock.accounts.SaveExport(everything()))
    expect(res.path).toBe('/Users/alex/Desktop/codepass-accounts-20261104.json')
    expect(await code(mock.accounts.SaveExport({ portals: [], profiles: [], includeExtras: true }))).toBe(ErrorCode.ErrInvalidInput)
  })

  it('an export imports back as all "same"', async () => {
    resetWorld('mid')
    const res = await call(mock.accounts.BuildExport(everything()))
    const p = await call(mock.accounts.PreviewImport({ json: res.json, importExtras: true, include: {}, choices: {}, renames: {} }))
    expect((p.items ?? []).length).toBeGreaterThan(2)
    for (const it of p.items ?? []) expect(it.action).toBe(ImportAction.ImportActionSame)
    expect(p.toWrite).toBe(0)
    expect(p.extrasCount).toBe(0)
  })

  it('the sample file shows same, new, conflict and rejected entries', async () => {
    resetWorld('fresh')
    const src = await call(mock.accounts.OpenImportFile())
    expect(src.name).toMatch(/\.json$/)
    const req: ImportRequest = { json: src.json, include: {}, choices: {}, renames: {}, importExtras: true }
    const p = await call(mock.accounts.PreviewImport(req))
    const by = Object.fromEntries((p.items ?? []).map((i) => [i.key, i]))
    expect(by['portal:initech'].action).toBe(ImportAction.ImportActionSame)
    expect(by['portal:globex'].action).toBe(ImportAction.ImportActionNew)
    expect(by['profile:globex-admin'].action).toBe(ImportAction.ImportActionNew)
    expect(by['profile:initech-admin'].action).toBe(ImportAction.ImportActionConflict)
    expect(by['profile:initech-admin'].choice).toBe('skip')
    expect(by['profile:initech-admin'].suggested).toBe('initech-admin-2')
    expect(by['profile:initech-legacy'].action).toBe(ImportAction.ImportActionRejected)
    expect(by['profile:initech-legacy'].reasonKey).toBe('import.rejected.secret')
    expect(p.preview.added).toBeGreaterThan(0)
    expect(p.toWrite).toBe(2)
  })

  it('applies choices, creates a backup and reports the portals that need a sign-in', async () => {
    resetWorld('fresh')
    const src = await call(mock.accounts.OpenImportFile())
    const req: ImportRequest = { json: src.json, include: {}, choices: { 'profile:initech-admin': 'rename' }, renames: {}, importExtras: true }
    const p = await call(mock.accounts.PreviewImport(req))
    const backupsBefore = getWorld().backups.length
    const r = await call(mock.accounts.ApplyImport(req, p.baseHash))
    expect(r).toMatchObject({ added: 2, renamed: 1, rejected: 1, replaced: 0 })
    // initech still has a renewable session in this scenario; only the new portal needs a sign-in
    expect(r.needSignIn).toEqual(['globex'])
    const w = getWorld()
    expect(w.backups.length).toBe(backupsBefore + 1)
    expect(w.backups.some((b) => b.reason === BackupReason.ReasonImport)).toBe(true)
    const names = (w.accounts.profiles ?? []).map((x) => x.name)
    expect(names).toContain('globex-admin')
    expect(names).toContain('initech-admin-2')
    expect(w.accounts.portals?.some((x) => x.name === 'globex')).toBe(true)
    expect(w.portalModels.globex.sessionMinutes).toBe(600)
    expect(w.accounts.profiles?.find((x) => x.name === 'globex-admin')?.monogram).toBe('GX')
    // importing the same file again with the default choices writes nothing (the conflict is skipped)
    const again = await call(mock.accounts.PreviewImport({ ...req, choices: {} }))
    expect(again.toWrite).toBe(0)
  })

  it('rejects unsafe values and foreign files', async () => {
    resetWorld('fresh')
    const entry = { name: 'evil', portal: 'initech', accountId: '777788889999', roleName: 'R', region: 'us-east-1\n[profile evil]\ncredential_process = x' }
    const p = await call(mock.accounts.PreviewImport({ json: doc([], [entry]), include: {}, choices: {}, renames: {}, importExtras: false }))
    expect(p.items?.[0]).toMatchObject({ action: ImportAction.ImportActionRejected, code: ErrorCode.ErrUnsafeValue, reasonKey: 'import.rejected.invalid' })
    const newer = JSON.stringify({ format: 'aws-sso-hub.accounts', version: 2 })
    expect(await code(mock.accounts.PreviewImport({ json: newer, include: {}, choices: {}, renames: {}, importExtras: false }))).toBe(ErrorCode.ErrImportFormat)
    expect(await code(mock.accounts.PreviewImport({ json: 'not json', include: {}, choices: {}, renames: {}, importExtras: false }))).toBe(ErrorCode.ErrImportFormat)
  })

  it('maps a portal with a known URL onto the existing portal', async () => {
    resetWorld('fresh')
    const portal = { name: 'corp', startUrl: 'https://d-1234567890.awsapps.com/start', region: 'us-west-2' }
    const profile = { name: 'corp-ro', portal: 'corp', accountId: '777788889999', roleName: 'ReadOnlyAccess' }
    const p = await call(mock.accounts.PreviewImport({ json: doc([portal], [profile]), include: {}, choices: {}, renames: {}, importExtras: false }))
    const by = Object.fromEntries((p.items ?? []).map((i) => [i.key, i]))
    expect(by['portal:corp']).toMatchObject({ action: ImportAction.ImportActionMapped, target: 'initech' })
    expect(by['profile:corp-ro']).toMatchObject({ action: ImportAction.ImportActionNew, portal: 'initech' })
  })
})
