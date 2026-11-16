// The setup screen's state machine (design 5.1): what the card shows for a ToolService snapshot. Pure functions, no React.
import { FailureCause, InstallStepKind, InstallStepState, SetupFix, SetupToolID, ToolStatus, Tri } from '../../api/models'
import type { InstallFailure, InstallState, InstallStep, Preflight, SetupState, SetupTool } from '../../api/models'
import type { MessageKey } from '../../i18n'

export type Platform = 'darwin' | 'windows'

/** idle: nothing running, the gate is closed. running: a setup run is going. failed: the last run stopped on an error. done: the gate is open. */
export type Phase = 'idle' | 'running' | 'failed' | 'done'

/** `done` wins over a stale failure: the user may have installed the tool by hand and checked again. */
export function phaseOf(setup: SetupState | null, install: InstallState | null): Phase {
  if (install?.running) return 'running'
  if (setup && !setup.blocked) return 'done'
  if (install?.finished && install.failure) return 'failed'
  return 'idle'
}

// ---- rows --------------------------------------------------------------------------------------------------

/** Product names, not UI copy. */
export function toolLabel(id: SetupToolID, platform: Platform): string {
  switch (id) {
    case SetupToolID.SetupHomebrew:
      return 'Homebrew'
    case SetupToolID.SetupWinget:
      return 'winget'
    case SetupToolID.SetupGit:
      return platform === 'windows' ? 'Git for Windows' : 'Git'
    default:
      return 'AWS CLI v2'
  }
}

/** The name of what is installed, as the user would ask IT for it (the exact package of design 5.1 "company policy"). */
export function packageName(id: SetupToolID, platform: Platform): string {
  if (platform === 'windows') return id === SetupToolID.SetupAWSCLI ? 'Amazon.AWSCLI' : id === SetupToolID.SetupGit ? 'Git.Git' : 'Microsoft.AppInstaller'
  return id === SetupToolID.SetupHomebrew ? 'Homebrew.pkg' : id === SetupToolID.SetupGit ? 'git' : 'awscli'
}

export function whyKey(id: SetupToolID, platform: Platform): MessageKey {
  switch (id) {
    case SetupToolID.SetupHomebrew:
      return 'setup.why.homebrew'
    case SetupToolID.SetupWinget:
      return platform === 'windows' ? 'setup.why.winget' : 'setup.why.homebrew'
    case SetupToolID.SetupGit:
      return 'setup.why.git'
    default:
      return 'setup.why.awscli'
  }
}

export type RowKind =
  | 'ok' // found, or installed by this run
  | 'missing'
  | 'too-old'
  | 'v1' // AWS CLI v1
  | 'error' // found but its version could not be read
  | 'detected' // fix use-detected: the path chosen in Settings is unusable, another copy is found
  | 'queued'
  | 'installing'
  | 'waiting' // the OS password / UAC window is up (macOS: one state with the installer)
  | 'failed'

export interface RowView {
  tool: SetupTool
  kind: RowKind
  /** The version to show on the right (ok rows only). */
  version?: string
  /** This run's steps of the row, in order. */
  steps: InstallStep[]
  /** The step in progress (installing / waiting rows). */
  current?: InstallStep
}

function baseKind(t: SetupTool): RowKind {
  if (t.fix === SetupFix.FixUseDetected) return 'detected'
  switch (t.status) {
    case ToolStatus.ToolOK:
    case ToolStatus.ToolOutdated:
      return 'ok'
    case ToolStatus.ToolTooOld:
      return 'too-old'
    case ToolStatus.ToolV1:
      return 'v1'
    case ToolStatus.ToolError:
      return 'error'
    default:
      return 'missing'
  }
}

/** What one row of the card shows, from the gate row and the run's steps. */
export function rowView(tool: SetupTool, install: InstallState | null): RowView {
  const steps = (install?.steps ?? []).filter((s) => s.tool === tool.id)
  const view = (kind: RowKind, current?: InstallStep): RowView => ({ tool, kind, steps, current, version: kind === 'ok' ? tool.version : undefined })
  const base = baseKind(tool)
  if (steps.length === 0 || !install) return view(base)
  const failed = steps.find((s) => s.state === InstallStepState.StepFailed)
  if (failed) return view('failed', failed)
  if (steps.every((s) => s.state === InstallStepState.StepDone)) return view('ok')
  const live = steps.find((s) => s.state === InstallStepState.StepWaitingOS || s.state === InstallStepState.StepRunning)
  if (live) return view(live.state === InstallStepState.StepWaitingOS ? 'waiting' : 'installing', live)
  // queued, or skipped. After a failure the rest wait for the retry; after Cancel they are simply not installed.
  const waiting = steps.some((s) => s.state === InstallStepState.StepQueued) || (install.finished && !!install.failure && steps.some((s) => s.state === InstallStepState.StepSkipped))
  return view(waiting ? 'queued' : base)
}

export function rowViews(setup: SetupState, install: InstallState | null): RowView[] {
  return (setup.tools ?? []).map((t) => rowView(t, install))
}

/** "n of 3 ready": counted from what the rows show, so a tool this run just installed counts before the state is re-read. */
export function readyCount(rows: RowView[]): number {
  return rows.filter((r) => r.kind === 'ok').length
}

/** Bytes done and total as a percentage; undefined when there is nothing to measure. */
export function stepPercent(s: InstallStep): number | undefined {
  if (s.kind !== InstallStepKind.StepDownload || !s.total || s.total <= 0) return undefined
  return Math.max(0, Math.min(100, Math.round(((s.done ?? 0) * 100) / s.total)))
}

export function stepLabelKey(kind: InstallStepKind): MessageKey {
  return kind === InstallStepKind.StepDownload ? 'setup.step.download' : kind === InstallStepKind.StepVerify ? 'setup.step.verify' : 'setup.step.install'
}

// ---- failures ----------------------------------------------------------------------------------------------

export interface FailureView {
  titleKey: MessageKey
  bodyKey: MessageKey
  params: Record<string, string | number>
  /** A second sentence: the proxy in use. */
  proxyKey?: MessageKey
  /** The banner icon: a lock for the administrator cases, a triangle otherwise. */
  icon: 'alert' | 'lock'
}

/** How much more space is needed: the total need minus what is free (the total when the free space is unknown or already enough). */
export function missingBytes(pre: Preflight): number {
  return pre.freeBytes >= 0 && pre.freeBytes < pre.needBytes ? pre.needBytes - pre.freeBytes : pre.needBytes
}

function gb(bytes: number): string {
  return (Math.max(0, bytes) / 1e9).toFixed(1)
}

/** One sentence per cause (design 5.1). The retry button is chosen by `primaryAction`. */
export function failureView(f: InstallFailure, platform: Platform, pre?: Preflight): FailureView {
  const mac = platform === 'darwin'
  const params: Record<string, string | number> = {
    tool: toolLabel(f.tool, platform),
    pkg: packageName(f.tool, platform),
    os: mac ? 'macOS' : 'Windows',
    proxy: f.proxy ?? '',
    need: pre ? gb(missingBytes(pre)) : '1.2',
    free: pre && pre.freeBytes >= 0 ? gb(pre.freeBytes) : '?',
    prefix: '/opt/homebrew',
  }
  const v = (titleKey: MessageKey, bodyKey: MessageKey, icon: FailureView['icon'] = 'alert', proxyKey?: MessageKey): FailureView => ({ titleKey, bodyKey, params, icon, proxyKey })
  switch (f.cause) {
    case FailureCause.CauseNotAdmin:
      return v('setup.fail.notAdmin.title', 'setup.fail.notAdmin.body', 'lock')
    case FailureCause.CauseCancelled:
      return v(mac ? 'setup.fail.cancelled.title.mac' : 'setup.fail.cancelled.title.win', 'setup.fail.cancelled.body', 'lock')
    case FailureCause.CauseAuthDenied:
      return v('setup.fail.authDenied.title', 'setup.fail.authDenied.body', 'lock')
    case FailureCause.CauseNoSession:
      return v('setup.fail.noSession.title', 'setup.fail.noSession.body')
    case FailureCause.CauseNetwork:
      return v('setup.fail.network.title', 'setup.fail.network.body', 'alert', f.proxy ? 'setup.fail.proxyUsed' : undefined)
    case FailureCause.CauseProxyAuth:
      return v('setup.fail.proxyAuth.title', 'setup.fail.proxyAuth.body', 'alert', f.proxy ? 'setup.fail.proxyUsed' : undefined)
    case FailureCause.CausePolicy:
      return v('setup.fail.policy.title', 'setup.fail.policy.body')
    case FailureCause.CauseDiskFull:
      return v('setup.fail.disk.title', 'setup.fail.disk.body')
    case FailureCause.CauseDamagedDownload:
      return v('setup.fail.damaged.title', 'setup.fail.damaged.body')
    case FailureCause.CauseBadSignature:
      return v('setup.fail.signature.title', 'setup.fail.signature.body')
    case FailureCause.CauseNotNotarized:
      return v('setup.fail.notarized.title', 'setup.fail.notarized.body')
    case FailureCause.CauseUnknownSigner:
      return v('setup.fail.signer.title', 'setup.fail.signer.body')
    case FailureCause.CausePrefixExists:
      return v('setup.fail.prefix.title', 'setup.fail.prefix.body')
    case FailureCause.CauseWingetMissing:
      return v('setup.fail.winget.title', 'setup.fail.winget.body')
    case FailureCause.CauseNeedsAdmin:
      return v('setup.fail.needsAdmin.title', 'setup.fail.needsAdmin.body', 'lock')
    case FailureCause.CauseReboot:
      return v('setup.fail.reboot.title', 'setup.fail.reboot.body')
    default:
      return v('setup.fail.generic.title', 'setup.fail.generic.body')
  }
}

// ---- the footer's buttons ----------------------------------------------------------------------------------

/**
 * install: "Install everything". continue: only rows whose Settings path is unusable (nothing is installed).
 * retry / redownload: continue the run from the failed step. recheck: nothing the app can do, the user acts, then checks again.
 * open-store: Windows without winget. enter: all rows ready. busy: a run is going.
 */
export type PrimaryKind = 'install' | 'continue' | 'retry' | 'redownload' | 'recheck' | 'open-store' | 'enter' | 'busy'

/** Causes where pressing "Try again" cannot help: the user has to change something on the computer first. */
const CHECK_AGAIN_CAUSES: FailureCause[] = [FailureCause.CausePrefixExists, FailureCause.CauseWingetMissing, FailureCause.CauseReboot]

export function primaryKind(phase: Phase, setup: SetupState | null, failure: InstallFailure | null | undefined, selfInstall: boolean): PrimaryKind {
  if (phase === 'done') return 'enter'
  if (phase === 'running') return 'busy'
  if (selfInstall) return 'recheck'
  if (phase === 'failed' && failure) {
    if (CHECK_AGAIN_CAUSES.includes(failure.cause)) return 'recheck'
    return failure.cause === FailureCause.CauseDamagedDownload ? 'redownload' : 'retry'
  }
  const blocking = (setup?.tools ?? []).filter((t) => t.blocking)
  if (blocking.some((t) => t.fix === SetupFix.FixRepairPrefix)) return 'recheck'
  if (blocking.some((t) => t.fix === SetupFix.FixOpenStore)) return 'open-store'
  if (blocking.length > 0 && blocking.every((t) => t.fix === SetupFix.FixUseDetected)) return 'continue'
  return 'install'
}

/** The official pages behind "I'll install them myself": one per blocking row; with Homebrew missing only Homebrew's page (brew then installs the others). */
export function installPageTargets(setup: SetupState): SetupToolID[] {
  const blocking = (setup.tools ?? []).filter((t) => t.blocking && t.fix !== SetupFix.FixUseDetected)
  const brew = blocking.find((t) => t.id === SetupToolID.SetupHomebrew)
  if (brew) return [brew.id]
  return blocking.map((t) => t.id)
}

/** Which notice the idle card shows before anything runs (design 5.1: "shown before the prompt"). */
export type IdleNotice = 'not-admin' | 'managed' | 'prefix' | 'low-disk' | 'proxy-pac' | 'proxy-socks' | null

export function idleNotice(setup: SetupState): IdleNotice {
  const p = setup.preflight
  const brewBlocking = (setup.tools ?? []).some((t) => t.id === SetupToolID.SetupHomebrew && t.blocking)
  if (p.prefixExists && brewBlocking) return 'prefix'
  if (setup.platform === 'darwin' && brewBlocking && p.admin === Tri.TriNo) return 'not-admin'
  if (!p.enoughSpace) return 'low-disk'
  if (p.managed === Tri.TriYes && brewBlocking) return 'managed'
  if (p.proxy.pacOnly) return 'proxy-pac'
  if (p.proxy.unsupported === 'socks') return 'proxy-socks'
  return null
}

// ---- the log -----------------------------------------------------------------------------------------------

/** Defence in depth: the backend already removes secrets from what it reports; a URL's user:password part and token-looking values never reach the screen or the clipboard. */
export function redactLine(line: string): string {
  return line
    .replace(/(\b[a-z][a-z0-9+.-]*:\/\/)[^\s/@]+@/gi, '$1')
    .replace(/\b(authorization)(\s*[=:]\s*)(?:(?:bearer|basic|token)\s+)?\S+/gi, '$1$2***')
    .replace(/\b(token|password|passwd|secret)(\s*[=:]\s*)\S+/gi, '$1$2***')
    .replace(/\bbearer\s+\S+/gi, 'bearer ***')
    .replace(/\b(gh[pousr]_|github_pat_|AKIA|ASIA)[A-Za-z0-9_]{8,}/g, '***')
}

export type LogTone = 'ok' | 'dim' | 'bad' | 'plain'

/** The log line is the failure's own line, or Go's "verify: <detail>" form of it. */
function endsWithFailure(line: string, failureLine: string): boolean {
  const want = redactLine(failureLine)
  return line === want || line.endsWith(want)
}

/**
 * The log as the card shows it. Go's fail() paths (a re-check that still finds the tool unusable, the repair-prefix
 * refusal) never write their reason to the log, so when no line carries failure.line it is added as the last line.
 */
export function logLines(install: InstallState | null): string[] {
  const lines = install?.log ?? []
  const fl = install?.failure?.line
  if (!fl || lines.some((l) => endsWithFailure(redactLine(l), fl))) return lines
  return [...lines, fl]
}

/** Colour of one log line: failing lines are danger once the run has failed; `$` and `==>` lines are dim. */
export function logTone(line: string, failure?: InstallFailure | null): LogTone {
  if (failure) {
    if (failure.line && endsWithFailure(line, failure.line)) return 'bad'
    if (/^(error|curl:|fatal|codepass-pkg:)/i.test(line)) return 'bad'
  }
  if (line.startsWith('✓')) return 'ok'
  if (line.startsWith('$ ') || line.startsWith('==>')) return 'dim'
  return 'plain'
}

/** The log as plain text for "Copy log": the lines, then the failure's own facts, nothing else. */
export function logText(install: InstallState | null): string {
  if (!install) return ''
  const lines = logLines(install).map(redactLine)
  const f = install.failure
  if (f) lines.push(`# ${f.tool}: ${f.cause}${f.stepId ? ` at ${f.stepId}` : ''}, exit code ${f.exitCode}`)
  return lines.join('\n')
}
