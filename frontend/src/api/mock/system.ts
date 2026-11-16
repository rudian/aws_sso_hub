import * as SystemReal from '../../../bindings/github.com/rudian/codepass/internal/app/systemservice'
import { initialUI } from '../../lib/initialUI'
import { BackupKind, BackupReason, ErrorCode, ProfileKind } from '../models'
import type { BackupEntry, DiffLine, Settings } from '../models'
import * as ToolReal from '../../../bindings/github.com/rudian/codepass/internal/app/toolservice'
import type * as G from '../models'
import { mockEmit } from '../events'
import { deepCopy, iso, now } from './clock'
import { HOME, SCAN_ROOT_DISPLAY, add, addBackup, ctx, del, fold, getWorld, mkPreview } from './world'
import type { World } from './world'
import { READ, WRITE, fail, run } from './util'

function needBackup(w: World, id: string): BackupEntry {
  const e = w.backups.find((b) => b.id === id)
  if (!e) throw fail(ErrorCode.ErrBackupNotFound, `backup ${id} not found`)
  return e
}

export const mockSystemService: typeof SystemReal = {
  GetAppInfo: () => run(READ, () => getWorld().appInfo),

  SetUILanguage: (lang) =>
    run(READ, () => {
      if (lang !== 'zh-CN' && lang !== 'en') throw fail(ErrorCode.ErrInvalidInput, 'unsupported language', { field: 'lang' })
      getWorld().appInfo.uiLanguage = lang
    }),

  GetSettings: () => run(READ, () => getWorld().settings),

  UpdateSettings: (p) =>
    run(WRITE, () => {
      const w = getWorld()
      const s: Settings = w.settings
      if (p.scanRoots !== undefined && p.scanRoots !== null && p.scanRoots.length === 0) throw fail(ErrorCode.ErrInvalidInput, 'at least one scan folder', { field: 'scanRoots' })
      if (p.language != null) s.language = p.language
      if (p.theme != null) s.theme = p.theme
      if (p.motion != null) s.motion = p.motion
      if (p.scanRoots) s.scanRoots = [...p.scanRoots]
      if (p.reminders) {
        if (![15, 30, 60].includes(p.reminders.leadMinutes)) throw fail(ErrorCode.ErrInvalidInput, 'lead minutes must be 15, 30 or 60', { field: 'reminders.leadMinutes' })
        s.reminders = { ...p.reminders }
      }
      if (p.tray) s.tray = { ...p.tray }
      if (p.terminalApp != null) s.terminalApp = p.terminalApp
      if (p.backupRetention != null) {
        if (![0, 20, 50].includes(p.backupRetention)) throw fail(ErrorCode.ErrInvalidInput, 'retention must be 20, 50 or 0', { field: 'backupRetention' })
        s.backupRetention = p.backupRetention
      }
      if (p.tutorialDone) s.tutorialDone = [...p.tutorialDone]
      if (p.tutorialLang != null) s.tutorialLang = p.tutorialLang
      if (p.awsCliPath != null) s.awsCliPath = p.awsCliPath
      if (p.gitPath != null) s.gitPath = p.gitPath
      if (p.autoBindOnAdd != null) s.autoBindOnAdd = p.autoBindOnAdd
      if (p.privateSignIn != null) s.privateSignIn = p.privateSignIn
      if (p.collapsedPortals) s.collapsedPortals = [...p.collapsedPortals]
      if (p.lastCloneProfile != null) s.lastCloneProfile = p.lastCloneProfile
      if (p.lastCloneRegion != null) s.lastCloneRegion = p.lastCloneRegion
      mockEmit('settings:changed', deepCopy(s))
      return s
    }),

  CompleteFirstRun: () =>
    run(WRITE, () => {
      getWorld().appInfo.firstRun = false
    }),

  FirstRunSummary: () =>
    run(READ, () => {
      const w = getWorld()
      const profiles = w.accounts.profiles ?? []
      return {
        profiles: profiles.length,
        portals: (w.accounts.portals ?? []).filter((p) => !p.draft).length,
        static: profiles.filter((p) => p.kind === ProfileKind.ProfileKindStatic).length,
        repos: w.repos.length,
        scanRoots: [SCAN_ROOT_DISPLAY],
      }
    }),

  CheckEnvironment: () =>
    run(700, () => {
      const w = getWorld()
      w.env.checkedAt = iso(now())
      return w.env
    }),

  CheckHealth: () =>
    run(700, () => {
      const w = getWorld()
      w.health.checkedAt = iso(now())
      return w.health
    }),

  ListBackups: () => run(READ, () => getWorld().backups),

  PreviewRestore: (id) =>
    run(READ, () => {
      const w = getWorld()
      const entry = needBackup(w, id)
      const lines: DiffLine[] = [fold(6), del('# current'), add('# from backup'), ctx('...'), fold(8)]
      const lost = entry.redacted && entry.kind === BackupKind.BackupAWSCredentials && entry.reasonParams?.profile ? [entry.reasonParams.profile] : []
      return { entry, preview: mkPreview(w, entry.file, entry.path, lines), lostProfiles: lost }
    }),

  RestoreBackup: (id, confirmLoss) =>
    run(WRITE, () => {
      const w = getWorld()
      const entry = needBackup(w, id)
      const lost = entry.redacted && entry.kind === BackupKind.BackupAWSCredentials && entry.reasonParams?.profile ? [entry.reasonParams.profile] : []
      if (lost.length > 0 && !confirmLoss) throw fail(ErrorCode.ErrRestoreNeedsConfirm, 'restoring loses key values', { profiles: lost.join(', ') })
      const safety = addBackup(w, entry.kind, BackupReason.ReasonRestore, { file: entry.file }, { path: entry.path, file: entry.file })
      mockEmit('accounts:changed', deepCopy(w.accounts))
      return safety
    }),

  RevealBackup: (id) =>
    run(READ, () => {
      console.info('[mock] reveal backup', needBackup(getWorld(), id).id)
    }),
  OpenBackupFolder: () =>
    run(READ, () => {
      console.info('[mock] open backup folder')
    }),

  OpenURL: (url) =>
    run(READ, () => {
      if (!/^https:\/\//i.test(url)) throw fail(ErrorCode.ErrInvalidInput, 'only https:// links can be opened', { field: 'url' })
      window.open(url, '_blank', 'noopener,noreferrer')
    }),
  RevealPath: (path) =>
    run(READ, () => {
      // Like the backend: the display form "~/..." (used by Health findings) is expanded first.
      const abs = path === '~' ? HOME : path.startsWith('~/') ? `${HOME}/${path.slice(2)}` : path
      const inside = abs === HOME || abs.startsWith(`${HOME}/`)
      if (!inside || abs.split('/').includes('..')) throw fail(ErrorCode.ErrInvalidInput, 'path must be under the home folder', { field: 'path' })
      console.info('[mock] reveal path', abs)
    }),
  OpenAWSConfig: () =>
    run(READ, () => {
      console.info('[mock] open ~/.aws/config')
    }),
  ChooseFolder: (_title) => run(READ, () => initialUI.addPath ?? '/Users/alex/work/legacy-api'),
  OpenTerminal: (profile, dir) =>
    run(READ, () => {
      console.info('[mock] open terminal', profile, dir)
    }),
}

export type { World }

// ---- tools gate and installers (F1-12) ---------------------------------------------------------------------
// A scripted ToolService for the setup screen (F1-15) and the Health rows. The scenario comes from the query:
//   ?setup=ready|missing|old-git|chosen-git|chosen-missing|outdated|noadmin|managed|lowdisk|partial|proxy|nowinget   (default ready: the gate is open)
//   partial always ends with prefix_exists and nowinget with winget_missing, as the Go installer does
//   &setupFail=<cause>[&setupFailAt=<step id>]   the run fails with that cause at that step (default: the first install step)
//   &setupRun=1   the run starts by itself when the page first asks for the state (screenshots: the page cannot be clicked)
// Typed against the generated toolservice module. Inside this file the enums are written as their string values
// (Lit turns a generated enum into the union of its values); the service methods hand the values back as the
// generated types, which are the same strings at run time.
type Lit<T> = T extends string ? `${T}` : T extends (infer U)[] ? Lit<U>[] : T extends object ? { [K in keyof T]: Lit<T[K]> } : T
type FailureCause = Lit<G.FailureCause>
type InstallFailure = Lit<G.InstallFailure>
type InstallState = Lit<G.InstallState>
type InstallStep = Lit<G.InstallStep>
type Preflight = Lit<G.Preflight>
type SetupState = Lit<G.SetupState>
type SetupTool = Lit<G.SetupTool>
type SetupToolID = Lit<G.SetupToolID>

export type SetupScenario = 'ready' | 'missing' | 'old-git' | 'chosen-git' | 'chosen-missing' | 'outdated' | 'noadmin' | 'managed' | 'lowdisk' | 'partial' | 'proxy' | 'nowinget'
const SETUP_SCENARIOS: SetupScenario[] = ['ready', 'missing', 'old-git', 'chosen-git', 'chosen-missing', 'outdated', 'noadmin', 'managed', 'lowdisk', 'partial', 'proxy', 'nowinget']

const FAIL_EXIT: Partial<Record<FailureCause, number>> = {
  damaged_download: 15, bad_signature: 12, unknown_signer: 13, not_notarized: 14, prefix_exists: 20, disk_full: 11,
  cancelled: 1, auth_denied: 1, not_admin: 1, network: 1, proxy_auth: 1, policy: 1, installer_failed: 1, needs_admin: 2316632089 /* 0x8A150019 as Go reports it: the uint32 */, winget_missing: -1,
}
// The failure lines the Go installer reports: osascript's "N:M: execution error: " prefix and its "(code)" tail are
// already stripped (osaLines in internal/toolinstall), so none of these carries either.
// Causes of the verify step: Go logs them as "verify: <detail>" while failure.line is just the detail.
const VERIFY_CAUSES: FailureCause[] = ['bad_signature', 'unknown_signer', 'not_notarized', 'damaged_download']
const FAIL_LINE: Partial<Record<FailureCause, string>> = {
  network: 'Error: Failure while executing; `curl --fail https://ghcr.io/v2/homebrew/core/git/blobs/sha256` exited with 28.',
  disk_full: 'Error: No space left on device @ io_write',
  cancelled: 'User canceled.',
  auth_denied: 'The administrator user name or password was incorrect.',
  not_admin: 'The administrator user name or password was incorrect.',
  bad_signature: 'signature: Status: no signature',
  unknown_signer: 'Team ID is ABCDEFGHIJ, expected 927JGANW46',
  not_notarized: 'not notarized',
  damaged_download: 'size 154000000, expected 154292112',
  prefix_exists: 'codepass-pkg: /opt/homebrew already exists',
  policy: 'codepass-pkg: installer failed (exit 1). Last lines: installer: installation is blocked by your administrator',
  installer_failed: 'Error: git: unexpected failure in post_install',
}
/** A row with fix repair-prefix fails before the password window, with no exit code and brew's prefix in the line. */
const REPAIR_PREFIX_LINE = '/opt/homebrew already exists'

interface ToolWorld {
  scenario: SetupScenario
  platform: 'darwin' | 'windows'
  tools: SetupTool[]
  pre: Preflight
  install: InstallState
  failCause?: FailureCause
  failAt?: string
  timers: ReturnType<typeof setTimeout>[]
  cancelAt: boolean
}

function queryParam(name: string): string | null {
  try {
    return typeof location === 'undefined' ? null : new URLSearchParams(location.search).get(name)
  } catch {
    return null
  }
}

const ok = (id: SetupToolID, version: string, path: string, min?: string): SetupTool => ({ id, status: 'ok', version, path, ...(min ? { min } : {}), blocking: false, fix: '' })
const missing = (id: SetupToolID): SetupTool => ({ id, status: 'missing', blocking: true, fix: 'install' })

function buildTools(scn: SetupScenario, platform: 'darwin' | 'windows'): { tools: SetupTool[]; pre: Preflight } {
  const GB = 1_000_000_000
  const pre: Preflight = { admin: platform === 'darwin' ? 'yes' : 'unknown', managed: platform === 'darwin' ? 'no' : 'unknown', freeBytes: 184 * GB, needBytes: 1.2 * GB, enoughSpace: true, prefixExists: false, proxy: { source: 'none' } }
  if (platform === 'windows') {
    const winget = scn === 'nowinget' ? ({ id: 'winget', status: 'missing', blocking: true, fix: 'open-store' } as SetupTool) : ok('winget', '1.9.25200', '%LOCALAPPDATA%\\Microsoft\\WindowsApps\\winget.exe')
    if (scn === 'ready') return { tools: [winget, ok('git', '2.47.1', 'C:\\Program Files\\Git\\cmd\\git.exe', '2.44.0'), ok('awscli', '2.17.0', 'C:\\Program Files\\Amazon\\AWSCLIV2\\aws.exe', '2.9.0')], pre }
    return { tools: [winget, missing('git'), missing('awscli')], pre }
  }
  const brew = ok('homebrew', '5.1.2', '/opt/homebrew/bin/brew')
  const git = ok('git', '2.56.1', '/opt/homebrew/bin/git', '2.44.0')
  const aws = ok('awscli', '2.40.3', '/opt/homebrew/bin/aws', '2.9.0')
  switch (scn) {
    case 'ready':
      return { tools: [brew, git, aws], pre }
    case 'old-git':
      return { tools: [brew, { id: 'git', status: 'too-old', version: '2.39.5', min: '2.44.0', path: '/usr/bin/git', blocking: true, fix: 'upgrade', elsewhere: true }, aws], pre }
    case 'chosen-git':
      // the git path chosen in Settings is too old, a usable git is found by detection: the run clears the chosen path
      return { tools: [brew, { id: 'git', status: 'too-old', version: '2.43.0', min: '2.44.0', path: '/Users/alex/tools/git', blocking: true, fix: 'use-detected', override: true }, aws], pre }
    case 'outdated':
      // older than recommended, not below the minimum: the gate is open and Health offers an Update
      return { tools: [brew, git, { ...aws, status: 'outdated', version: '2.17.0' }], pre }
    case 'chosen-missing':
      // the AWS CLI path chosen in Settings points at nothing and no other copy is found: clearing it does not make the gate open
      return { tools: [brew, git, { id: 'awscli', status: 'missing', path: '/Users/alex/tools/aws', blocking: true, fix: 'install', override: true }], pre }
    case 'noadmin':
      return { tools: [missing('homebrew'), missing('git'), missing('awscli')], pre: { ...pre, admin: 'no' } }
    case 'managed':
      return { tools: [missing('homebrew'), missing('git'), missing('awscli')], pre: { ...pre, managed: 'yes' } }
    case 'lowdisk':
      return { tools: [missing('homebrew'), missing('git'), missing('awscli')], pre: { ...pre, freeBytes: 0.6 * GB, enoughSpace: false } }
    case 'partial':
      return { tools: [{ id: 'homebrew', status: 'missing', blocking: true, fix: 'repair-prefix' }, missing('git'), missing('awscli')], pre: { ...pre, prefixExists: true } }
    case 'proxy':
      return { tools: [missing('homebrew'), missing('git'), missing('awscli')], pre: { ...pre, proxy: { hostPort: 'proxy.example.com:8080', source: 'system' } } }
    default:
      return { tools: [missing('homebrew'), missing('git'), missing('awscli')], pre }
  }
}

const emptyInstall = (): InstallState => ({ running: false, cancelling: false, cancelled: false, steps: [], log: [], finished: false })
let toolWorld: ToolWorld | undefined

/** Resets the scripted gate. Tests call it; the page gets it from the query on first use. */
export function resetToolMock(scenario?: SetupScenario, platform?: 'darwin' | 'windows', failCause?: FailureCause, failAt?: string): void {
  if (toolWorld) toolWorld.timers.forEach(clearTimeout)
  const q = queryParam('setup') as SetupScenario | null
  const scn: SetupScenario = scenario ?? (q !== null && SETUP_SCENARIOS.includes(q) ? q : 'ready')
  const plat = platform ?? initialUI.platform
  const { tools, pre } = buildTools(scn, plat)
  toolWorld = { scenario: scn, platform: plat, tools, pre, install: emptyInstall(), failCause: failCause ?? (queryParam('setupFail') as FailureCause | null) ?? undefined, failAt: failAt ?? queryParam('setupFailAt') ?? undefined, timers: [], cancelAt: false }
}

function tw(): ToolWorld {
  if (!toolWorld) {
    resetToolMock()
    if (queryParam('setupRun') === '1') {
      // the same call the button makes, one tick later so the page has asked for the state first
      const w = toolWorld as ToolWorld | undefined
      w?.timers.push(setTimeout(() => void mockToolService.StartSetupInstall(), 300))
    }
  }
  return toolWorld as ToolWorld
}

function setupState(w: ToolWorld): SetupState {
  const ready = w.tools.filter((t) => t.status === 'ok' || t.status === 'outdated').length
  return { platform: w.platform, tools: deepCopy(w.tools), ready, total: w.tools.length, blocked: w.tools.some((t) => t.blocking), preflight: deepCopy(w.pre), checkedAt: iso(now()) }
}

function planSteps(w: ToolWorld): InstallStep[] {
  const steps: InstallStep[] = []
  for (const t of w.tools) {
    if (!t.blocking || t.fix === 'use-detected') continue
    if (t.id === 'homebrew') {
      if (t.fix === 'repair-prefix') steps.push({ id: 'homebrew:install', tool: 'homebrew', kind: 'install', state: 'queued' })
      else {
        steps.push({ id: 'homebrew:download', tool: 'homebrew', kind: 'download', state: 'queued', command: 'Homebrew.pkg' })
        steps.push({ id: 'homebrew:verify', tool: 'homebrew', kind: 'verify', state: 'queued', command: 'pkgutil --check-signature' })
        steps.push({ id: 'homebrew:install', tool: 'homebrew', kind: 'install', state: 'queued', command: 'installer -pkg Homebrew.pkg', indeterminate: true })
      }
    } else if (t.id === 'winget') {
      steps.push({ id: 'winget:install', tool: 'winget', kind: 'install', state: 'queued' })
    } else {
      const f = t.id === 'awscli' ? 'awscli' : 'git'
      const cmd = w.platform === 'windows' ? `winget install --id ${t.id === 'awscli' ? 'Amazon.AWSCLI -e --scope machine' : 'Git.Git -e'}` : `brew ${t.fix === 'upgrade' && !t.elsewhere ? 'upgrade' : 'install'} ${f}`
      steps.push({ id: `${t.id}:install`, tool: t.id, kind: 'install', state: 'queued', command: cmd })
    }
  }
  return steps
}

function pushInstall(w: ToolWorld): void {
  mockEmit('tools:install-progress', deepCopy(w.install) as unknown as G.InstallState)
}

function logLine(w: ToolWorld, line: string): void {
  const log = w.install.log ?? []
  log.push(line)
  w.install.log = log.slice(-200)
}

const FOUND: Record<string, [string, string]> = {
  homebrew: ['5.1.2', '/opt/homebrew/bin/brew'],
  git: ['2.56.1', '/opt/homebrew/bin/git'],
  awscli: ['2.40.3', '/opt/homebrew/bin/aws'],
  winget: ['1.9.25200', '%LOCALAPPDATA%\\Microsoft\\WindowsApps\\winget.exe'],
}

function finishRun(w: ToolWorld, failure: InstallFailure | undefined, cancelled: boolean): void {
  const steps = w.install.steps ?? []
  for (const s of steps) if (s.state === 'queued' || s.state === 'running' || s.state === 'waiting-os') s.state = 'skipped'
  if (failure) {
    const f = steps.find((s) => s.id === failure.stepId)
    if (f) f.state = 'failed'
  }
  w.install.running = false
  w.install.cancelling = false
  w.install.cancelled = cancelled
  w.install.finished = true
  w.install.failure = failure ?? null
  pushInstall(w)
}

function runStep(w: ToolWorld, i: number): void {
  const steps = w.install.steps ?? []
  if (i >= steps.length) {
    finishRun(w, undefined, w.install.cancelling)
    return
  }
  if (w.install.cancelling) {
    finishRun(w, undefined, true)
    return
  }
  const step = steps[i]
  // The Go installer always fails these two rows, whatever the script says.
  const row0 = w.tools.find((t) => t.id === step.tool)
  const forced: FailureCause | undefined = step.id === 'homebrew:install' && row0?.fix === 'repair-prefix' ? 'prefix_exists' : step.id === 'winget:install' ? 'winget_missing' : undefined
  const scripted = w.failCause !== undefined && (w.failAt ? w.failAt === step.id : step.kind === 'install' || i === steps.length - 1)
  const failCause = forced ?? (scripted ? w.failCause : undefined)
  const failHere = failCause !== undefined
  const repairPrefix = forced === 'prefix_exists' && row0?.fix === 'repair-prefix'
  // the password window is never reached for a repair-prefix row
  step.state = step.id === 'homebrew:install' && !repairPrefix ? 'waiting-os' : 'running'
  pushInstall(w)
  const done = (): void => {
    if (failCause) {
      const cause = failCause
      const line = repairPrefix ? REPAIR_PREFIX_LINE : FAIL_LINE[cause]
      const failure: InstallFailure = {
        tool: step.tool, stepId: step.id, cause, exitCode: repairPrefix ? -1 : (FAIL_EXIT[cause] ?? 1),
        ...(step.command ? { command: step.command } : {}),
        ...(cause === 'network' || cause === 'proxy_auth' ? { proxy: w.pre.proxy.hostPort ?? '' } : {}),
        ...(line ? { line } : {}),
      }
      // Go's fail() paths (the repair-prefix refusal) keep their text in failure.line only; the log stays empty.
      if (line && !repairPrefix) logLine(w, VERIFY_CAUSES.includes(cause) ? `verify: ${line}` : line)
      finishRun(w, failure, false)
      return
    }
    step.state = 'done'
    const row = w.tools.find((t) => t.id === step.tool)
    if (row && step.kind === 'install') {
      const [version, path] = FOUND[row.id] ?? ['1.0.0', '']
      Object.assign(row, { status: 'ok', version, path, blocking: false, fix: '', elsewhere: false })
    }
    pushInstall(w)
    runStep(w, i + 1)
  }
  const later = (ms: number, fn: () => void): void => {
    w.timers.push(setTimeout(fn, ms))
  }
  if (step.kind === 'download') {
    const total = 154_292_112
    step.total = total
    let tick = 0
    const next = (): void => {
      if (w.install.cancelling) return finishRun(w, undefined, true)
      tick++
      step.done = Math.min(total, Math.round((total * tick) / 6))
      logLine(w, `Homebrew.pkg ${Math.round((100 * tick) / 6)}%`)
      pushInstall(w)
      if (tick >= 6 || (failHere && tick >= 3)) return done()
      later(150, next)
    }
    later(150, next)
  } else {
    if (step.command?.startsWith('brew')) logLine(w, `==> Pouring ${step.tool === 'awscli' ? 'awscli--2.40.3' : 'git--2.56.1'}.arm64_tahoe.bottle.tar.gz`)
    pushInstall(w)
    later(step.id === 'homebrew:install' ? 1500 : 600, done)
  }
}

export const mockToolService: typeof ToolReal = {
  GetSetupState: () => run(READ, () => setupState(tw()) as unknown as G.SetupState),

  StartSetupInstall: () =>
    run(READ, () => {
      const w = tw()
      if (w.install.running) throw fail(ErrorCode.ErrInvalidInput, 'a setup run is already going', { field: 'run' })
      // Like the Go service: every blocking row judged on a path chosen in Settings gets that path cleared, then the
      // rows are judged again. Detection finds the tool for use-detected; any other row stays as it is without the path.
      for (const t of w.tools) {
        if (!t.blocking || !t.override) continue
        if (t.fix === 'use-detected') {
          const [version, path] = FOUND[t.id] ?? ['1.0.0', '']
          Object.assign(t, { status: 'ok', version, path, blocking: false, fix: '', elsewhere: false, override: false })
        } else {
          t.override = false
          delete t.path
        }
      }
      w.install = { ...emptyInstall(), running: true, steps: planSteps(w) }
      pushInstall(w)
      if (!w.pre.enoughSpace && (w.install.steps ?? []).length > 0) {
        const first = (w.install.steps ?? [])[0]
        finishRun(w, { tool: first.tool, stepId: first.id, cause: 'disk_full', exitCode: -1 }, false)
        return
      }
      w.timers.push(setTimeout(() => runStep(w, 0), 50))
    }),

  CancelSetupInstall: () =>
    run(READ, () => {
      const w = tw()
      if (!w.install.running) return
      w.install.cancelling = true
      pushInstall(w)
    }),

  GetInstallState: () => run(READ, () => tw().install as unknown as G.InstallState),

  OpenInstallPage: (toolId) =>
    run(READ, () => {
      const tool = toolId as string
      const aws = 'https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html'
      const pages: Record<'darwin' | 'windows', Partial<Record<string, string>>> = {
        darwin: { homebrew: 'https://brew.sh/', git: 'https://git-scm.com/download/mac', awscli: aws },
        windows: { winget: 'https://apps.microsoft.com/detail/9nblggh4nns1', git: 'https://git-scm.com/download/win', awscli: aws },
      }
      const url = pages[tw().platform][tool]
      if (!url) throw fail(ErrorCode.ErrInvalidInput, 'unknown tool', { field: 'tool' })
      window.open(url, '_blank', 'noopener,noreferrer')
    }),
}
