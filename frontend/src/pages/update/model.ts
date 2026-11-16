// Pure mapping from an UpdateState to what the update screens draw (design 5.13). No I/O, no React.
import { UpdatePhase } from '../../api/models'
import type { UpdateState } from '../../api/models'
import type { MessageKey } from '../../i18n'
import { zhAll } from '../../i18n'

export type StepId = 'download' | 'verify' | 'replace' | 'reopen'
export type StepStatus = 'todo' | 'now' | 'done' | 'error'
export interface StepView {
  id: StepId
  status: StepStatus
}

export const STEP_IDS: StepId[] = ['download', 'verify', 'replace', 'reopen']
const STEP_KEY: Record<StepId, MessageKey> = {
  download: 'update.step.download',
  verify: 'update.step.verify',
  replace: 'update.step.replace',
  reopen: 'update.step.reopen',
}
export const stepKey = (id: StepId): MessageKey => STEP_KEY[id]

/** Which of the four steps a failure at `step` stands on. A failed check never reaches the steps. */
function failedStep(s: UpdateState): StepId {
  if (s.step === 'download') return 'download'
  if (s.step === 'restart') return 'replace'
  return 'verify' // a refusal that names no step is a verification failure
}

export function stepsFor(s: UpdateState): StepView[] {
  const status = (at: number, failed?: number): StepStatus[] =>
    STEP_IDS.map((_, i) => (failed === i ? 'error' : i < (failed ?? at) ? 'done' : i === at && failed === undefined ? 'now' : 'todo'))
  let st: StepStatus[]
  switch (s.phase) {
    case UpdatePhase.UpdateDownloading:
      st = status(0)
      break
    case UpdatePhase.UpdateVerifying:
      st = status(1)
      break
    case UpdatePhase.UpdateReady:
      st = status(2).map((x, i) => (i === 2 ? 'todo' : x)) // staged and verified; the helper does the rest after Reopen
      break
    case UpdatePhase.UpdateRestarting:
      st = status(2)
      break
    case UpdatePhase.UpdateError:
    case UpdatePhase.UpdateRefused:
      st = status(0, STEP_IDS.indexOf(failedStep(s)))
      break
    default:
      st = status(-1)
  }
  return STEP_IDS.map((id, i) => ({ id, status: st[i] }))
}

/** A check would discard a downloaded copy while a flow is on its way; "Check now" stays off in these phases. */
export function canCheck(s: UpdateState): boolean {
  return s.phase !== UpdatePhase.UpdateDownloading && s.phase !== UpdatePhase.UpdateVerifying && s.phase !== UpdatePhase.UpdateReady && s.phase !== UpdatePhase.UpdateRestarting
}

/** The Settings dot: a release is offered, downloading or ready. A failure the person has seen does not keep it lit. */
export function showsDot(s: UpdateState): boolean {
  return (
    s.phase === UpdatePhase.UpdateAvailable ||
    s.phase === UpdatePhase.UpdateDownloading ||
    s.phase === UpdatePhase.UpdateVerifying ||
    s.phase === UpdatePhase.UpdateReady
  )
}

export function failed(s: UpdateState): boolean {
  return s.phase === UpdatePhase.UpdateError || s.phase === UpdatePhase.UpdateRefused
}

/** 0..100 for the bar; 0 when the size is unknown. */
export function percent(s: UpdateState): number {
  if (!s.total || s.total <= 0) return 0
  return Math.max(0, Math.min(100, Math.floor(((s.written ?? 0) / s.total) * 100)))
}

export function mb(n: number | undefined): string {
  return ((n ?? 0) / 1_000_000).toFixed(1)
}

/** Seconds left from the bytes still to come and a measured speed in bytes per second; null while unknown. */
export function etaSeconds(s: UpdateState, bytesPerSec: number): number | null {
  if (!s.total || bytesPerSec <= 0) return null
  return Math.max(1, Math.ceil((s.total - (s.written ?? 0)) / bytesPerSec))
}

/**
 * Release notes are plain text from the signed update file: one line per entry, a leading list marker dropped,
 * blank lines skipped. The result is only ever rendered as React text, never as markup.
 */
export function noteLines(notes: string | undefined): string[] {
  return (notes ?? '')
    .split(/\r?\n/)
    .map((l) => l.replace(/^\s*(?:[-*•]|\d+[.)])\s+/, '').trim())
    .filter((l) => l.length > 0)
}

const KNOWN_CODES = new Set(Object.keys(zhAll).filter((k) => k.startsWith('update.code.')).map((k) => k.slice('update.code.'.length)))

/** The message key of a failure code; unknown codes get the generic line. */
export function codeKey(code: string | undefined): MessageKey {
  return (code && KNOWN_CODES.has(code) ? `update.code.${code}` : 'update.code.other') as MessageKey
}

export type Compare = -1 | 0 | 1
/** Semver precedence: numeric core, then a release outranks its pre-releases, then pre-release identifiers one by one. */
export function compareVersions(a: string, b: string): Compare {
  const parse = (v: string) => {
    const [core, pre = ''] = v.replace(/^v/, '').split('+')[0].split(/-(.*)/s)
    return { nums: core.split('.').map((x) => Number.parseInt(x, 10) || 0), pre: pre ? pre.split('.') : [] }
  }
  const x = parse(a)
  const y = parse(b)
  for (let i = 0; i < Math.max(x.nums.length, y.nums.length); i++) {
    const d = (x.nums[i] ?? 0) - (y.nums[i] ?? 0)
    if (d !== 0) return d < 0 ? -1 : 1
  }
  if (x.pre.length === 0 || y.pre.length === 0) return x.pre.length === y.pre.length ? 0 : x.pre.length === 0 ? 1 : -1
  for (let i = 0; i < Math.max(x.pre.length, y.pre.length); i++) {
    const p = x.pre[i]
    const q = y.pre[i]
    if (p === undefined) return -1
    if (q === undefined) return 1
    const pn = /^\d+$/.test(p)
    const qn = /^\d+$/.test(q)
    if (pn && qn) {
      const d = Number(p) - Number(q)
      if (d !== 0) return d < 0 ? -1 : 1
    } else if (pn !== qn) return pn ? -1 : 1
    else if (p !== q) return p < q ? -1 : 1
  }
  return 0
}

export type BannerKind = 'available' | 'downloading' | 'ready' | 'restarting' | 'failed'

/** What the banner under the Projects header says; null when there is nothing to say. */
export function bannerKind(s: UpdateState): BannerKind | null {
  switch (s.phase) {
    case UpdatePhase.UpdateAvailable:
      return 'available'
    case UpdatePhase.UpdateDownloading:
    case UpdatePhase.UpdateVerifying:
      return 'downloading'
    case UpdatePhase.UpdateReady:
      return 'ready'
    case UpdatePhase.UpdateRestarting:
      return 'restarting'
    case UpdatePhase.UpdateError:
    case UpdatePhase.UpdateRefused:
      return s.version && s.step !== 'check' ? 'failed' : null
    default:
      return null
  }
}
