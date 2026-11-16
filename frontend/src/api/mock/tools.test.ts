import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { onEvent } from '../events'
import { mockToolService, resetToolMock } from './system'
import { SetupToolID } from '../models'
import type { InstallState } from '../models'

async function call<T>(p: Promise<T>, ms = 200): Promise<T> {
  const settled = p.then((v) => ({ ok: true as const, v }), (e: unknown) => ({ ok: false as const, e }))
  await vi.advanceTimersByTimeAsync(ms)
  const r = await settled
  if (!r.ok) throw r.e
  return r.v
}

describe('mock ToolService', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-11-04T04:00:00Z'))
  })
  afterEach(() => vi.useRealTimers())

  it('ready: the gate is open and the rows carry versions', async () => {
    resetToolMock('ready', 'darwin')
    const s = await call(mockToolService.GetSetupState())
    expect(s.blocked).toBe(false)
    expect(s.ready).toBe(3)
    expect(s.tools?.map((t) => t.id)).toEqual(['homebrew', 'git', 'awscli'])
    expect(s.tools?.every((t) => t.version && t.path)).toBe(true)
  })

  it('old-git: names the version found, the minimum and the fix', async () => {
    resetToolMock('old-git', 'darwin')
    const g = (await call(mockToolService.GetSetupState())).tools?.find((t) => t.id === 'git')
    expect(g).toMatchObject({ status: 'too-old', version: '2.39.5', min: '2.44.0', fix: 'upgrade', blocking: true, elsewhere: true })
  })

  it('missing on macOS runs download, verify, install, then brew, and the gate opens', async () => {
    resetToolMock('missing', 'darwin')
    const seen: InstallState[] = []
    const off = onEvent('tools:install-progress', (s) => seen.push(s))
    expect((await call(mockToolService.GetSetupState())).blocked).toBe(true)
    await call(mockToolService.StartSetupInstall())
    await vi.advanceTimersByTimeAsync(10_000)
    off()
    const st = await call(mockToolService.GetInstallState())
    expect(st.finished && !st.running && !st.failure && !st.cancelled).toBe(true)
    expect(st.steps?.map((s) => `${s.id}=${s.state}`)).toEqual([
      'homebrew:download=done', 'homebrew:verify=done', 'homebrew:install=done', 'git:install=done', 'awscli:install=done',
    ])
    expect(st.steps?.[3].command).toBe('brew install git')
    expect(seen.some((s) => s.steps?.some((x) => x.id === 'homebrew:install' && x.state === 'waiting-os' && x.indeterminate))).toBe(true)
    expect(seen.some((s) => s.steps?.some((x) => x.id === 'homebrew:download' && (x.done ?? 0) > 0 && x.total))).toBe(true)
    expect((await call(mockToolService.GetSetupState())).blocked).toBe(false)
  })

  it('a scripted failure stops the run with its cause and skips the rest', async () => {
    resetToolMock('proxy', 'darwin', 'network', 'git:install')
    await call(mockToolService.StartSetupInstall())
    await vi.advanceTimersByTimeAsync(10_000)
    const st = await call(mockToolService.GetInstallState())
    expect(st.failure).toMatchObject({ cause: 'network', stepId: 'git:install', proxy: 'proxy.example.com:8080', command: 'brew install git' })
    expect(st.steps?.map((s) => s.state)).toEqual(['done', 'done', 'done', 'failed', 'skipped'])
    expect((await call(mockToolService.GetSetupState())).blocked).toBe(true)
  })

  it('lowdisk fails at once with disk_full; partial fails with prefix_exists only through its own step', async () => {
    resetToolMock('lowdisk', 'darwin')
    await call(mockToolService.StartSetupInstall())
    expect((await call(mockToolService.GetInstallState())).failure?.cause).toBe('disk_full')
    resetToolMock('partial', 'darwin', 'prefix_exists')
    await call(mockToolService.StartSetupInstall())
    await vi.advanceTimersByTimeAsync(5_000)
    const st = await call(mockToolService.GetInstallState())
    expect(st.failure).toMatchObject({ cause: 'prefix_exists', stepId: 'homebrew:install' })
  })

  it('cancel stops a download at the next tick and keeps what is done', async () => {
    resetToolMock('missing', 'darwin')
    await call(mockToolService.StartSetupInstall())
    await vi.advanceTimersByTimeAsync(300)
    await call(mockToolService.CancelSetupInstall())
    await vi.advanceTimersByTimeAsync(2_000)
    const st = await call(mockToolService.GetInstallState())
    expect(st.cancelled && st.finished && !st.failure).toBe(true)
    expect(st.steps?.every((s) => s.state === 'skipped')).toBe(true)
  })

  it('a second start while running is refused', async () => {
    resetToolMock('missing', 'darwin')
    await call(mockToolService.StartSetupInstall())
    await expect(call(mockToolService.StartSetupInstall())).rejects.toBeTruthy()
  })

  it('windows: winget rows and commands', async () => {
    resetToolMock('missing', 'windows')
    const s = await call(mockToolService.GetSetupState())
    expect(s.tools?.map((t) => t.id)).toEqual(['winget', 'git', 'awscli'])
    await call(mockToolService.StartSetupInstall())
    const st = await call(mockToolService.GetInstallState())
    expect(st.steps?.map((x) => x.command)).toEqual(['winget install --id Git.Git -e', 'winget install --id Amazon.AWSCLI -e --scope machine'])
    resetToolMock('nowinget', 'windows')
    expect((await call(mockToolService.GetSetupState())).tools?.[0]).toMatchObject({ id: 'winget', status: 'missing', fix: 'open-store' })
  })

  it('noadmin and managed set the pre-flight facts', async () => {
    resetToolMock('noadmin', 'darwin')
    expect((await call(mockToolService.GetSetupState())).preflight.admin).toBe('no')
    resetToolMock('managed', 'darwin')
    expect((await call(mockToolService.GetSetupState())).preflight.managed).toBe('yes')
  })

  it('partial always fails with prefix_exists and nowinget with winget_missing, whatever the script says', async () => {
    resetToolMock('partial', 'darwin')
    await call(mockToolService.StartSetupInstall())
    await vi.advanceTimersByTimeAsync(5_000)
    let st = await call(mockToolService.GetInstallState())
    expect(st.failure).toMatchObject({ cause: 'prefix_exists', stepId: 'homebrew:install', exitCode: -1, line: '/opt/homebrew already exists' })
    // the password window is never reached for a repair-prefix row
    expect(st.log).toEqual([])
    expect((await call(mockToolService.GetSetupState())).blocked).toBe(true)
    resetToolMock('nowinget', 'windows')
    await call(mockToolService.StartSetupInstall())
    await vi.advanceTimersByTimeAsync(5_000)
    st = await call(mockToolService.GetInstallState())
    expect(st.failure).toMatchObject({ cause: 'winget_missing', stepId: 'winget:install' })
    expect(st.steps?.map((s) => s.state)).toEqual(['failed', 'skipped', 'skipped'])
  })

  it('install pages follow the platform, and winget has none on macOS', async () => {
    const open = vi.fn()
    vi.stubGlobal('window', { open })
    try {
      resetToolMock('ready', 'darwin')
      await call(mockToolService.OpenInstallPage(SetupToolID.SetupGit))
      await expect(call(mockToolService.OpenInstallPage(SetupToolID.SetupWinget))).rejects.toBeTruthy()
      resetToolMock('ready', 'windows')
      await call(mockToolService.OpenInstallPage(SetupToolID.SetupGit))
      await expect(call(mockToolService.OpenInstallPage(SetupToolID.SetupHomebrew))).rejects.toBeTruthy()
      expect(open.mock.calls.map((c) => c[0])).toEqual(['https://git-scm.com/download/mac', 'https://git-scm.com/download/win'])
    } finally {
      vi.unstubAllGlobals()
    }
  })

  it('verify-step failures are logged as "verify: <detail>" while failure.line is the detail', async () => {
    resetToolMock('missing', 'darwin', 'bad_signature', 'homebrew:install')
    await call(mockToolService.StartSetupInstall())
    await vi.advanceTimersByTimeAsync(5_000)
    const st = await call(mockToolService.GetInstallState())
    expect(st.failure?.line).toBe('signature: Status: no signature')
    expect(st.log?.at(-1)).toBe('verify: signature: Status: no signature')
  })

  it('needs_admin reports the unsigned exit code, as Go does', async () => {
    resetToolMock('missing', 'windows', 'needs_admin', 'git:install')
    await call(mockToolService.StartSetupInstall())
    await vi.advanceTimersByTimeAsync(5_000)
    expect((await call(mockToolService.GetInstallState())).failure?.exitCode).toBe(2316632089)
  })

  it('chosen-git: the run clears the chosen path and installs nothing', async () => {
    resetToolMock('chosen-git', 'darwin')
    const s = await call(mockToolService.GetSetupState())
    expect(s.tools?.[1]).toMatchObject({ fix: 'use-detected', override: true, blocking: true })
    await call(mockToolService.StartSetupInstall())
    await vi.advanceTimersByTimeAsync(2_000)
    const st = await call(mockToolService.GetInstallState())
    expect(st.finished && !st.failure && (st.steps ?? []).length === 0).toBe(true)
    expect((await call(mockToolService.GetSetupState())).blocked).toBe(false)
  })

  it('failure lines carry no osascript prefix and no code tail, as the Go installer reports them', async () => {
    for (const cause of ['cancelled', 'auth_denied', 'not_admin', 'unknown_signer', 'damaged_download'] as const) {
      resetToolMock('missing', 'darwin', cause, 'homebrew:install')
      await call(mockToolService.StartSetupInstall())
      await vi.advanceTimersByTimeAsync(5_000)
      const line = (await call(mockToolService.GetInstallState())).failure?.line ?? ''
      expect(line, cause).not.toMatch(/execution error/)
      expect(line, cause).not.toMatch(/\(-?\d+\)$/)
    }
    resetToolMock('missing', 'darwin', 'cancelled', 'homebrew:install')
    await call(mockToolService.StartSetupInstall())
    await vi.advanceTimersByTimeAsync(5_000)
    expect((await call(mockToolService.GetInstallState())).failure?.line).toBe('User canceled.')
  })

  it('a scripted prefix_exists on the pkg script keeps its exit code and the script prefix', async () => {
    resetToolMock('missing', 'darwin', 'prefix_exists', 'homebrew:install')
    await call(mockToolService.StartSetupInstall())
    await vi.advanceTimersByTimeAsync(5_000)
    expect((await call(mockToolService.GetInstallState())).failure).toMatchObject({ exitCode: 20, line: 'codepass-pkg: /opt/homebrew already exists' })
  })

  it('chosen-missing: the run clears the chosen path (override off, no path) and still installs the tool', async () => {
    resetToolMock('chosen-missing', 'darwin')
    expect((await call(mockToolService.GetSetupState())).tools?.[2]).toMatchObject({ id: 'awscli', status: 'missing', fix: 'install', override: true, blocking: true })
    await call(mockToolService.StartSetupInstall())
    const during = (await call(mockToolService.GetSetupState())).tools?.[2]
    expect(during?.override).toBeFalsy()
    expect(during?.path).toBeUndefined()
    await vi.advanceTimersByTimeAsync(5_000)
    const st = await call(mockToolService.GetInstallState())
    expect(st.steps?.map((x) => x.id)).toEqual(['awscli:install'])
    expect(st.finished && !st.failure).toBe(true)
    expect((await call(mockToolService.GetSetupState())).blocked).toBe(false)
  })

  it('every blocking override row is cleared at start, not only the first', async () => {
    resetToolMock('chosen-git', 'darwin')
    await call(mockToolService.StartSetupInstall())
    expect((await call(mockToolService.GetSetupState())).tools?.some((t) => t.override)).toBe(false)
  })

  it('outdated: the gate is open and the AWS CLI is older than recommended', async () => {
    resetToolMock('outdated', 'darwin')
    const s = await call(mockToolService.GetSetupState())
    expect(s.blocked).toBe(false)
    expect(s.ready).toBe(3)
    expect(s.tools?.[2]).toMatchObject({ id: 'awscli', status: 'outdated', version: '2.17.0', blocking: false })
  })
})
