import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { initialUI } from '../../lib/initialUI'
import { toUiError } from '../errors'
import { BindingSource, DefaultAction, ErrorCode, MigrationStep, OwnerStatus } from '../models'
import type { Repo } from '../models'
import { mock } from './index'
import { getWorld, resetWorld } from './world'

/** Await a mock call while advancing fake timers. */
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

describe('mock services', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-11-04T04:00:00Z'))
    initialUI.session = undefined
    initialUI.step = undefined
    initialUI.profile = undefined
  })
  afterEach(() => vi.useRealTimers())

  it('fresh: DetectOwners checks 2 accounts, skips initech, and finds the ambiguous clone', async () => {
    resetWorld('fresh')
    const s = await call(mock.repos.DetectOwners(null))
    const byPath = (p: string) => s.repos?.find((r) => r.displayPath === p) as Repo
    const acmeRow = byPath('~/code/api-server')
    expect(acmeRow.owner.status).toBe(OwnerStatus.OwnerSingle)
    expect(acmeRow.owner.candidates?.[0].profile).toBe('acme')
    expect(acmeRow.owner.checkedWith).toBe(2)
    expect(acmeRow.owner.skippedPortals).toEqual(['initech'])
    expect(byPath('~/code/archive/web-app').owner.status).toBe(OwnerStatus.OwnerAmbiguous)
    expect(byPath('~/code/notification-service').owner.candidates?.[0].profile).toBe('globex')
    expect(byPath('~/code/analytics-dashboard').owner.status).toBe(OwnerStatus.OwnerPartial)
  })

  it('CompareCommits marks the account whose HEAD matches', async () => {
    resetWorld('fresh')
    await call(mock.repos.DetectOwners(null))
    const id = getWorld().repos.find((r) => r.path.includes('archive'))!.id
    const r = await call(mock.repos.CompareCommits(id))
    expect(r.owner.candidates?.find((c) => c.headMatch)?.profile).toBe('globex')
  })

  it('Bind updates the row, counts and backups; Unbind returns to the global default', async () => {
    resetWorld('fresh')
    const id = getWorld().repos.find((r) => r.name === 'notification-service')!.id
    const out = await call(mock.repos.Bind(id, 'globex'))
    expect(out.ok).toBe(true)
    expect(out.repo?.binding).toMatchObject({ source: BindingSource.BindingLocal, profile: 'globex', profileKnown: true, colorSlot: 6 })
    expect((await call(mock.repos.GetState())).counts).toEqual({ all: 16, toFix: 15, bound: 1 })
    expect(getWorld().backups.at(-1)).toMatchObject({ kind: 'git-local', reason: 'bind', reasonParams: { repo: 'notification-service' } })
    const un = await call(mock.repos.Unbind(id))
    expect(un.repo?.binding.source).toBe(BindingSource.BindingGlobalDefault)
    expect(un.repo?.binding.profile).toBe('acme')
  })

  it('Bind rejects SSH and GRC remotes and a missing CLI', async () => {
    resetWorld('edge')
    const w = getWorld()
    expect(await code(mock.repos.Bind(w.repos.find((r) => r.name === 'data-pipeline')!.id, 'acme'))).toBe(ErrorCode.ErrAWSCLIMissing)
    w.awsCliReady = true
    expect(await code(mock.repos.Bind(w.repos.find((r) => r.name === 'data-pipeline')!.id, 'acme'))).toBe(ErrorCode.ErrRemoteSSH)
    expect(await code(mock.repos.Bind(w.repos.find((r) => r.name === 'design-system')!.id, 'acme'))).toBe(ErrorCode.ErrRemoteGRC)
  })

  it('TestConnection is ok for static profiles and signed-out for an expired portal', async () => {
    resetWorld('fresh')
    const w = getWorld()
    const legacyRepo = w.repos.find((r) => r.name === 'design-system')!
    expect((await call(mock.repos.TestConnection(legacyRepo.id))).status).toBe('ok')
    const wl = w.repos.find((r) => r.name === 'infra-terraform')!
    await call(mock.repos.Bind(wl.id, 'initech-admin'))
    const r = await call(mock.repos.TestConnection(wl.id))
    expect(r).toMatchObject({ status: 'signed-out', portal: 'initech' })
  })

  it('Clone emits five progress lines and adds a bound row', async () => {
    resetWorld('mid')
    const { onEvent } = await import('../events')
    const lines: string[] = []
    const off = onEvent('clone:progress', (p) => lines.push(p.line))
    const out = await call(mock.repos.Clone({ profile: 'initech-admin', region: 'us-west-2', repoName: 'cfn-templates', parentDir: '/Users/alex/code', folderName: 'cfn-templates' }))
    off()
    expect(lines).toHaveLength(5)
    expect(lines[0]).toBe("Cloning into 'cfn-templates'...")
    expect(out.repo?.binding).toMatchObject({ source: 'local', profile: 'initech-admin' })
    expect(await code(mock.repos.Clone({ profile: 'initech-admin', region: 'us-west-2', repoName: 'x', parentDir: '/Users/alex/code', folderName: 'cfn-templates' }))).toBe(ErrorCode.ErrDestExists)
  })

  it('GitGlobal: removing the default is blocked while a URL rule exists, then works after migrating the rule', async () => {
    resetWorld('fresh')
    expect(await code(mock.gitGlobal.PreviewDefaultChange({ action: DefaultAction.DefaultRemove }))).toBe(ErrorCode.ErrURLRulesFirst)
    const rule = (await call(mock.gitGlobal.GetState())).urlRules?.[0]
    expect(rule?.matchedRepoIds).toHaveLength(1)
    const plan = await call(mock.gitGlobal.PreviewURLRuleMigration(rule!.pattern))
    expect(plan.duplicateUrls).toBe(false)
    const res = await call(mock.gitGlobal.MigrateURLRule({ pattern: rule!.pattern, assignments: [{ repoId: plan.repos![0].id, profile: 'initech-admin' }] }, plan.preview.baseHash))
    expect(res.ruleRemoved).toBe(true)
    const prev = await call(mock.gitGlobal.PreviewDefaultChange({ action: DefaultAction.DefaultRemove }))
    expect(prev.preview.lines?.some((l) => l.text.includes('git-codecommit.*.amazonaws.com'))).toBe(true)
    expect(prev.unboundBlocked).toBeGreaterThan(0)
    const applied = await call(mock.gitGlobal.ApplyDefaultChange({ action: DefaultAction.DefaultRemove }, prev.preview.baseHash))
    expect(applied.state.guard.installed).toBe(true)
    expect(applied.state.globalDefault).toBeUndefined()
    expect((await call(mock.repos.GetState())).repos?.some((r) => r.binding.source === BindingSource.BindingGuard)).toBe(true)
  })

  it('migration: globex goes review -> verify -> remove-key -> deactivate -> done', async () => {
    resetWorld('mid')
    let st = await call(mock.migration.Get('globex'))
    expect(st.step).toBe(MigrationStep.MigrationReview)
    expect(st.configPreview?.lines?.[0].text).toBe('[profile globex]')
    st = await call(mock.migration.Apply('globex', st.configPreview!.baseHash), 3000)
    expect(st.step).toBe(MigrationStep.MigrationVerify)
    expect(st.verify?.ok).toBe(true)
    const accounts = await call(mock.accounts.GetState())
    expect(accounts.profiles?.find((p) => p.name === 'globex')).toMatchObject({ kind: 'sso', staticState: 'migrated-keys-left', portal: 'globex' })
    expect(accounts.staticCount).toBe(0)
    st = await call(mock.migration.PreviewKeyRemoval('globex'))
    expect(st.credentialsPreview?.lines?.some((l) => l.text.includes('•'))).toBe(true)
    expect(JSON.stringify(st)).not.toMatch(/AKIA[A-Z0-9]{16}/)
    st = await call(mock.migration.RemoveKey('globex', st.credentialsPreview!.baseHash))
    expect(st.step).toBe(MigrationStep.MigrationDeactivate)
    st = await call(mock.migration.Deactivate('globex'))
    expect(st.deactivation).toMatchObject({ status: 'deactivated', method: 'old-key' })
    st = await call(mock.migration.Finish('globex'))
    expect(st.step).toBe(MigrationStep.MigrationDone)
    expect(await code(mock.migration.Get('globex'))).toBe(ErrorCode.ErrMigrateState)
    expect((await call(mock.accounts.GetState())).profiles?.find((p) => p.name === 'globex')?.hasStaticKeys).toBe(false)
  })

  it('migration: calls outside the allowed step fail with E_MIGRATE_STATE; wrong-account choice needs confirmation', async () => {
    resetWorld('fresh')
    await call(mock.accounts.IdentifyStatic('acme'))
    const st = await call(mock.migration.Start('acme'))
    expect(st.step).toBe(MigrationStep.MigrationIdentify)
    expect(st.identity.accountId).toBe('111122223333')
    expect(await code(mock.migration.RemoveKey('acme', 'x'))).toBe(ErrorCode.ErrMigrateState)
    await call(mock.accounts.AddPortalDraft({ name: 'acme', startUrl: 'https://d-0000000000.awsapps.com/start', region: 'us-east-1' }))
    const s2 = await call(mock.migration.ChoosePortal('acme', 'acme'))
    expect(s2).toMatchObject({ step: 'account', portalIsDraft: true })
    expect(await code(mock.migration.ChooseAccount('acme', { accountId: '444455556666', accountName: 'x', role: 'R', region: '', confirmOther: false }))).toBe(ErrorCode.ErrMigrateConfirmOther)
    expect(await code(mock.migration.ChoosePortal('acme', 'nope'))).toBe(ErrorCode.ErrPortalNotFound)
  })

  it('?step prefills the wizard (fresh, review)', async () => {
    resetWorld('fresh')
    initialUI.profile = 'acme'
    initialUI.step = 'review'
    const st = await call(mock.migration.Start('acme'))
    expect(st.step).toBe(MigrationStep.MigrationReview)
    expect(st.portal).toBe('acme')
    expect(st.configPreview?.lines?.some((l) => l.text.startsWith('[sso-session acme]'))).toBe(true)
  })

  it('SetSessionLength validates and recomputes', async () => {
    resetWorld('mid')
    expect(await code(mock.accounts.SetSessionLength('initech', 14))).toBe(ErrorCode.ErrInvalidInput)
    expect(await code(mock.accounts.SetSessionLength('initech', 129601))).toBe(ErrorCode.ErrInvalidInput)
    expect(await code(mock.accounts.SetSessionLength('nope', 480))).toBe(ErrorCode.ErrInvalidInput)
    const s = await call(mock.accounts.SetSessionLength('initech', 60))
    expect(s.sessionMinutes).toBe(60)
    expect(s.state).toBe('active')
    // the login was 1 h 48 min ago, so a 60 min session is already past its set length: overrun, no bar
    expect(s.sessionOverrun).toBe(true)
    expect(s.progressPct).toBe(0)
    const s2 = await call(mock.accounts.SetSessionLength('initech', 480 * 2))
    expect(s2).toMatchObject({ sessionOverrun: false, progressPct: 89 })
  })

  it('session overrides: unknown, overrun, ending', async () => {
    for (const [o, check] of [
      ['unknown', (s: { sessionKnown: boolean; loginAt?: string; state: string }) => { expect(s).toMatchObject({ sessionKnown: false, state: 'active' }); expect(s.loginAt).toBeUndefined() }],
      ['overrun', (s: { sessionOverrun: boolean; progressPct: number; state: string }) => expect(s).toMatchObject({ sessionOverrun: true, progressPct: 0, state: 'active' })],
      ['ending', (s: { state: string; sessionEndsAt?: string }) => {
        expect(s.state).toBe('expiring')
        expect(Date.parse(s.sessionEndsAt!) - Date.now()).toBe(9 * 60_000)
      }],
    ] as const) {
      initialUI.session = o
      const w = resetWorld('fresh')
      check(w.accounts.portals![0].session as never)
    }
  })

  it('SetProfileIdentity changes the colour for every profile of the account', async () => {
    resetWorld('fresh')
    const s = await call(mock.accounts.SetProfileIdentity({ profile: 'initech-admin', colorSlot: 8, monogram: 'xx' }))
    expect(s.profiles?.find((p) => p.name === 'initech-admin')).toMatchObject({ colorSlot: 8, monogram: 'XX' })
    expect(s.profiles?.find((p) => p.name === 'initech-readonly')?.colorSlot).toBe(8)
    expect(await code(mock.accounts.SetProfileIdentity({ profile: 'initech-admin', colorSlot: 3, monogram: 'toolong' }))).toBe(ErrorCode.ErrInvalidInput)
  })

  it('CheckPortalInput normalises and validates', async () => {
    resetWorld('fresh')
    const ok = await call(mock.accounts.CheckPortalInput({ name: '', startUrl: ' https://d-1111111111.awsapps.com#/ ', region: '' }))
    expect(ok.startUrl).toBe('https://d-1111111111.awsapps.com/start')
    expect(ok.name).toBe('d-1111111111')
    expect(ok.errors).toEqual({})
    const bad = await call(mock.accounts.CheckPortalInput({ name: 'initech', startUrl: 'http://example.com', region: 'mars-1' }))
    expect(bad.errors).toMatchObject({ startUrl: 'E_INVALID_START_URL', region: 'E_INVALID_REGION', name: 'E_PORTAL_EXISTS' })
  })

  it('Discover requires a signed-in portal; every portal lists only its own account', async () => {
    const fresh = resetWorld('fresh')
    fresh.portalModels['initech'].refreshFailed = true // what the fresh ticker does at +2.5 s
    expect(await code(mock.accounts.DiscoverAccounts('initech'))).toBe(ErrorCode.ErrSessionExpired)
    resetWorld('mid')
    const list = await call(mock.accounts.DiscoverAccounts('initech'))
    expect(list!.map((a) => a.accountId)).toEqual(['777788889999'])
    const acme = await call(mock.accounts.DiscoverAccounts('acme'))
    expect(acme!.map((a) => a.accountId)).toEqual(['111122223333'])
    expect(acme![0].roles?.[0]).toMatchObject({ name: 'AdministratorAccess', existingProfile: 'acme' })
  })

  it('CreateProfiles adds profiles, makes a draft portal real, and rejects duplicates', async () => {
    resetWorld('mid')
    const req = { portal: 'initech', profiles: [{ name: 'initech-platform-poweruser', accountId: '777788889999', accountName: 'initech-platform', role: 'PowerUserAccess', region: 'us-west-2' }] }
    const prev = await call(mock.accounts.PreviewCreateProfiles(req))
    expect(prev.added).toBeGreaterThan(0)
    const res = await call(mock.accounts.CreateProfiles(req, prev.baseHash))
    expect(res.profiles).toEqual(['initech-platform-poweruser'])
    const st = await call(mock.accounts.GetState())
    expect(st.profiles?.find((p) => p.name === 'initech-platform-poweruser')).toMatchObject({ colorSlot: 3, kind: 'sso' })
    expect(st.portals?.[0].profiles).toContain('initech-platform-poweruser')
    expect(await code(mock.accounts.CreateProfiles(req, prev.baseHash))).toBe(ErrorCode.ErrProfileExists)
  })

  it('Logout clears the login but keeps the session length', async () => {
    resetWorld('mid')
    await call(mock.accounts.SetSessionLength('initech', 240))
    const r = await call(mock.accounts.Logout('initech'))
    expect(r).toMatchObject({ portal: 'initech', serverLogout: true, deletedRoleCaches: 2 })
    const s = (await call(mock.accounts.GetState())).portals!.find((p) => p.name === 'initech')!.session
    expect(s).toMatchObject({ state: 'signed-out', sessionKnown: false, sessionMinutes: 240 })
    expect(s.loginAt).toBeUndefined()
  })

  it('settings: UpdateSettings validates and emits settings:changed', async () => {
    resetWorld('fresh')
    const { onEvent } = await import('../events')
    const seen: string[] = []
    const off = onEvent('settings:changed', (s) => seen.push(s.theme))
    const s = await call(mock.system.UpdateSettings({ theme: 'dark' as never }))
    expect(s.theme).toBe('dark')
    expect(seen).toEqual(['dark'])
    off()
    expect(await code(mock.system.UpdateSettings({ scanRoots: [] }))).toBe(ErrorCode.ErrInvalidInput)
    expect(await code(mock.system.UpdateSettings({ reminders: { expiryNotice: true, leadMinutes: 10, autoRenew: true } }))).toBe(ErrorCode.ErrInvalidInput)
  })

  it('settings: clone and portal fields default, update with copies, and clear', async () => {
    resetWorld('fresh')
    const d = await call(mock.system.GetSettings())
    expect(d).toMatchObject({ collapsedPortals: [], lastCloneProfile: '', lastCloneRegion: '' })
    const arr = ['acme']
    const s = await call(mock.system.UpdateSettings({ collapsedPortals: arr, lastCloneProfile: 'dev', lastCloneRegion: 'eu-west-1' }))
    expect(s).toMatchObject({ collapsedPortals: ['acme'], lastCloneProfile: 'dev', lastCloneRegion: 'eu-west-1' })
    arr.push('x')
    expect((await call(mock.system.GetSettings())).collapsedPortals).toEqual(['acme'])
    expect((await call(mock.system.UpdateSettings({ theme: 'dark' as never }))).collapsedPortals).toEqual(['acme'])
    const c = await call(mock.system.UpdateSettings({ collapsedPortals: [], lastCloneProfile: '', lastCloneRegion: '' }))
    expect(c).toMatchObject({ collapsedPortals: [], lastCloneProfile: '', lastCloneRegion: '' })
  })

  it('restoring a redacted credentials backup needs confirmation', async () => {
    resetWorld('mid')
    const entry = (await call(mock.system.ListBackups()))!.find((b) => b.kind === 'aws-credentials')!
    expect(entry.redacted).toBe(true)
    const prev = await call(mock.system.PreviewRestore(entry.id))
    expect(prev.lostProfiles).toEqual(['acme'])
    expect(await code(mock.system.RestoreBackup(entry.id, false))).toBe(ErrorCode.ErrRestoreNeedsConfirm)
    const safety = await call(mock.system.RestoreBackup(entry.id, true))
    expect(safety.reason).toBe('restore')
  })

  it('OpenURL only accepts https', async () => {
    resetWorld('fresh')
    const open = vi.spyOn(window, 'open').mockReturnValue(null)
    expect(await code(mock.system.OpenURL('http://x'))).toBe(ErrorCode.ErrInvalidInput)
    expect(await code(mock.system.OpenURL('https://example.com'))).toBeNull()
    expect(open).toHaveBeenCalledOnce()
  })

  it('RepairBindings repairs needsRepair and legacy-plain bindings', async () => {
    resetWorld('fresh')
    const w = getWorld()
    const legacyRepo = w.repos.find((r) => r.name === 'design-system')!
    expect(legacyRepo.binding.legacyPlain).toBe(true)
    const out = await call(mock.repos.RepairBindings())
    expect(out).toHaveLength(1)
    expect(out![0].repo?.binding.legacyPlain).toBe(false)
  })

  it('PreviewBind builds the documented commands and warnings', async () => {
    resetWorld('fresh')
    const w = getWorld()
    w.portalModels['initech'].refreshFailed = true
    const r = w.repos.find((x) => x.name === 'infra-terraform')!
    const p = await call(mock.repos.PreviewBind(r.id, 'initech-admin'))
    expect(p.commands?.[0]).toContain("credential.https://git-codecommit.us-west-2.amazonaws.com.helper")
    expect(p.commands?.some((c) => c.includes('awsssohub.accountId') && c.includes('777788889999'))).toBe(true)
    expect(p.warnings).toContain('signed-out')
    const q = await call(mock.repos.PreviewBind(r.id, 'acme'))
    expect(q.warnings).toContain('static-profile')
  })

  it('Discover lists the account of a draft portal by its URL', async () => {
    resetWorld('fresh')
    await call(mock.accounts.AddPortalDraft({ name: 'globex', startUrl: 'https://d-2222222222.awsapps.com/start', region: 'eu-west-1' }))
    getWorld().portalModels['globex'].file = 'present'
    getWorld().portalModels['globex'].tokenExpiresAt = Date.now() + 3_600_000
    const list = await call(mock.accounts.DiscoverAccounts('globex'))
    expect(list!.map((a) => a.accountId)).toEqual(['444455556666'])
  })

  it('done: owner detection reports the signed-out portal', async () => {
    resetWorld('done')
    const w = getWorld()
    const row = w.repos.find((r) => r.name === 'notification-service')!
    const st = await call(mock.repos.DetectOwners([row.id]))
    const owner = st.repos!.find((r) => r.id === row.id)!.owner
    expect(owner.status).toBe(OwnerStatus.OwnerPartial)
    expect(owner.skippedPortals).toEqual(['globex'])
  })

  describe('add an existing repo', () => {
    const add = (path: string) => call(mock.repos.AddExisting(path))

    it('ChooseFolder returns the ?addPath= value, or a default outside the scan folder', async () => {
      resetWorld('mid')
      initialUI.addPath = undefined
      expect(await call(mock.system.ChooseFolder('x'), 500)).toBe('/Users/alex/work/legacy-api')
      initialUI.addPath = '/Users/alex/elsewhere/x'
      expect(await call(mock.system.ChooseFolder('x'), 500)).toBe('/Users/alex/elsewhere/x')
      initialUI.addPath = undefined
    })

    it('mid: a new repo is added and bound to the SSO profile of its account', async () => {
      resetWorld('mid')
      const res = await add('/Users/alex/work/legacy-api')
      expect(res).toMatchObject({ added: true, autoBound: true })
      expect(res.outcome?.ok).toBe(true)
      const r = res.state.repos!.find((x) => x.id === res.repoId)!
      expect(r).toMatchObject({ extra: true, name: 'legacy-api', toFix: false })
      expect(r.binding).toMatchObject({ source: BindingSource.BindingLocal, profile: 'acme', accountId: '111122223333' })
      expect(res.state.counts.all).toBe(17)
      // Unbind is an exact undo
      const out = await call(mock.repos.Unbind(res.repoId))
      expect(out.repo?.binding.source).not.toBe(BindingSource.BindingLocal)
    })

    it('fresh: acme is a static profile, so the repo is added with a suggestion only', async () => {
      resetWorld('fresh')
      const res = await add('/Users/alex/work/legacy-api')
      expect(res).toMatchObject({ added: true, autoBound: false })
      expect(res.outcome).toBeUndefined()
      const r = res.state.repos!.find((x) => x.id === res.repoId)!
      expect(r.owner.status).toBe(OwnerStatus.OwnerSingle)
      expect(r.owner.candidates?.[0].profile).toBe('acme')
      expect(r.binding.source).not.toBe(BindingSource.BindingLocal)
    })

    it('does not bind when the setting is off', async () => {
      resetWorld('mid')
      await call(mock.system.UpdateSettings({ autoBindOnAdd: false }))
      const res = await add('/Users/alex/work/legacy-api')
      expect(res).toMatchObject({ added: true, autoBound: false })
    })

    it('an ambiguous path needs the user to choose', async () => {
      resetWorld('mid')
      const res = await add('/Users/alex/work/ambiguous_api')
      expect(res).toMatchObject({ added: true, autoBound: false })
      const r = res.state.repos!.find((x) => x.id === res.repoId)!
      expect(r.owner.status).toBe(OwnerStatus.OwnerAmbiguous)
      expect(r.owner.candidates).toHaveLength(2)
    })

    it('rejects folders that are not CodeCommit repositories and reports listed ones', async () => {
      resetWorld('mid')
      expect(await code(mock.repos.AddExisting('/Users/alex/work/notrepo'))).toBe(ErrorCode.ErrRepoNotFound)
      expect(await code(mock.repos.AddExisting('/Users/alex/work/github_site'))).toBe(ErrorCode.ErrNotCodeCommit)
      const listed = getWorld().repos[0]
      const res = await add(listed.path)
      expect(res).toMatchObject({ added: false, autoBound: false, repoId: listed.id })
      expect(res.state.counts.all).toBe(16)
    })

    it('RemoveExtra removes only repositories that were added by hand', async () => {
      resetWorld('mid')
      const res = await add('/Users/alex/work/legacy-api')
      expect(await code(mock.repos.RemoveExtra(getWorld().repos[0].id))).toBe(ErrorCode.ErrInvalidInput)
      const st = await call(mock.repos.RemoveExtra(res.repoId))
      expect(st.repos!.some((x) => x.id === res.repoId)).toBe(false)
      expect(st.counts.all).toBe(16)
    })
  })

  it('suggested profile order: bound first, SSO before static, most bound repos, then the name', async () => {
    const w = resetWorld('mid')
    const acctRepo = w.repos.find((r) => r.name === 'web-app')!
    // a second SSO profile for the acme account that carries more bound repositories than acme
    const base = w.accounts.profiles!.find((p) => p.name === 'acme')!
    w.accounts.profiles!.push({ ...base, name: 'acme-ops' })
    const before = (await call(mock.repos.DetectOwners([w.repos.find((r) => r.name === 'team-handbook')!.id]))).repos!.find((r) => r.name === 'team-handbook')!
    // acme has 9 bound repos, acme-ops none: the busier profile wins
    expect(before.owner.candidates?.[0].profile).toBe('acme')
    for (const n of ['api-server', 'web-app-v2', 'mobile-app', 'auth-service', 'search-service', 'data-pipeline', 'design-system', 'docs-site']) {
      const r = w.repos.find((x) => x.name === n)!
      r.binding = { ...r.binding, profile: 'acme-ops' }
    }
    const after = (await call(mock.repos.DetectOwners([w.repos.find((r) => r.name === 'team-handbook')!.id]))).repos!.find((r) => r.name === 'team-handbook')!
    expect(after.owner.candidates?.[0].profile).toBe('acme-ops')
    // the repository that is bound to acme keeps acme even though acme-ops has more bound repositories
    const mine = (await call(mock.repos.DetectOwners([acctRepo.id]))).repos!.find((r) => r.id === acctRepo.id)!
    expect(mine.owner.candidates?.[0].profile).toBe('acme')
  })

  it('RevealPath expands the display form and keeps the home-folder guard', async () => {
    resetWorld('mid')
    await call(mock.system.RevealPath('~/.zshrc'))
    await call(mock.system.RevealPath('/Users/alex/.aws'))
    expect(await code(mock.system.RevealPath('/etc/hosts'))).toBe(ErrorCode.ErrInvalidInput)
    expect(await code(mock.system.RevealPath('~/../elsewhere'))).toBe(ErrorCode.ErrInvalidInput)
    expect(await code(mock.system.RevealPath('relative/path'))).toBe(ErrorCode.ErrInvalidInput)
  })
})
