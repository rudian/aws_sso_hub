import type { CancellablePromise } from '@wailsio/runtime'
import { mockEmit } from '../events'
import { ErrorCode } from '../models'
import { fail, READ, run, WRITE } from './util'

// (start) hand-written types, until A2-W registers KeepAwakeService and the event and regenerates the bindings.
// They mirror internal/api/keepawake.go. A2-W: replace this block with
//   import * as KeepAwakeReal from '../../../bindings/github.com/rudian/codepass/internal/app/keepawakeservice'
//   import type { KeepAwakeConfig, KeepAwakeState } from '../models'
// and type the mock as `typeof KeepAwakeReal`.
export interface KeepAwakeConfig {
  enabled: boolean
  screenToo: boolean
  graceMinutes: number
  ceilingHours: number
}
export interface KeepAwakeState {
  held: boolean
  active: number
  running: number
  waiting: number
  leftover: number
  /** Programs the closed sessions (leftover) still run: "2 个后台程序". */
  programs: number
  screen: boolean
  /** Unix milliseconds; 0 = none. */
  releaseAt: number
  ceilingHit: boolean
  /** '' = none, 'partial' = some assertions could not be taken, 'none' = none could be taken. */
  error: '' | 'partial' | 'none'
  config: KeepAwakeConfig
}
interface KeepAwakeServiceShape {
  GetState(): CancellablePromise<KeepAwakeState>
  GetConfig(): CancellablePromise<KeepAwakeConfig>
  SetConfig(c: KeepAwakeConfig): CancellablePromise<KeepAwakeState>
}
// (end)

// The demo backend of keep-awake (A2-03, plan 4.10). The real manager counts running and waiting AI tasks and the
// programs closed sessions left behind; the demo has no sessions, so the counts are set from the URL or by the other
// mocks through setMockKeepAwakeActivity.
//
// URL switch (mock mode only), for the screenshots of the milestone gate:
//   ?awake=<running>[,<waiting>[,<leftover>[,<programs>]]]   for example ?awake=2 (two tasks running), ?awake=0,1
//                                (one waiting), ?awake=0,0,1,2 (one closed session with 2 programs still running;
//                                programs defaults to the number of closed sessions)
//   ?awake=off                   turns the setting off
//   a word after the numbers:    ceiling (a task was dropped at the hard ceiling), error (no assertion could be
//                                taken), for example ?awake=0,0,0,ceiling or ?awake=2,error
// The default is no activity.

// The event is typed here until A2-W adds it to EventPayloads in ../events.ts (identical members merge, so both can stand).
declare module '../events' {
  interface EventPayloads {
    'keepawake:changed': KeepAwakeState
  }
}

const DEFAULTS: KeepAwakeConfig = { enabled: true, screenToo: true, graceMinutes: 15, ceilingHours: 8 }

let config: KeepAwakeConfig = { ...DEFAULTS }
let counts = { running: 0, waiting: 0, leftover: 0, programs: 0 }
let flags = { ceiling: false, error: false }
let loadedAt = Date.now()

/** Back to a fresh page load, reading the URL switch (tests pass the query string). */
export function resetMockKeepAwake(search = ''): void {
  const q = new URLSearchParams(search)
  config = { ...DEFAULTS }
  counts = { running: 0, waiting: 0, leftover: 0, programs: 0 }
  flags = { ceiling: false, error: false }
  loadedAt = Date.now()
  const v = q.get('awake')
  if (v === 'off') config.enabled = false
  else if (v) {
    const parts = v.split(',')
    const nums = parts.filter((x) => /^\d+$/.test(x)).map((x) => Number.parseInt(x, 10))
    const [r, w, l, p] = nums
    counts = { running: r ?? 0, waiting: w ?? 0, leftover: l ?? 0, programs: p ?? l ?? 0 }
    flags = { ceiling: parts.includes('ceiling'), error: parts.includes('error') }
  }
}

/** When the hold ends if nothing changes, as the real manager reports it: only while nothing runs and something waits
 *  or is left behind (a waiting task ends at the grace time, "no limit" and closed sessions at the ceiling). */
function releaseAt(): number {
  if (counts.running > 0 || counts.waiting + counts.leftover === 0) return 0
  const ceiling = config.ceilingHours * 3_600_000
  const grace = config.graceMinutes > 0 ? Math.min(config.graceMinutes * 60_000, ceiling) : ceiling
  const ms = Math.max(counts.waiting > 0 ? grace : 0, counts.leftover > 0 ? ceiling : 0)
  return loadedAt + ms
}

/** The state the real manager would report for these settings and counts. */
export function mockKeepAwakeState(): KeepAwakeState {
  const active = counts.running + counts.waiting + counts.leftover
  const held = config.enabled && active > 0 && !flags.error
  return {
    held,
    active,
    running: counts.running,
    waiting: counts.waiting,
    leftover: counts.leftover,
    programs: counts.leftover > 0 ? Math.max(counts.programs, counts.leftover) : 0,
    screen: held && config.screenToo,
    releaseAt: releaseAt(),
    ceilingHit: flags.ceiling,
    error: config.enabled && active > 0 && flags.error ? 'none' : '',
    config: { ...config },
  }
}

/** Other mocks (the terminal sessions, the AI tasks) call this when their sessions change; the event goes out. */
export function setMockKeepAwakeActivity(next: { running?: number; waiting?: number; leftover?: number; programs?: number }): void {
  counts = { ...counts, ...next }
  mockEmit('keepawake:changed', mockKeepAwakeState())
}

resetMockKeepAwake(typeof window !== 'undefined' ? window.location.search : '')

export const mockKeepAwakeService: KeepAwakeServiceShape = {
  GetState: () => run(READ, mockKeepAwakeState),
  GetConfig: () => run(READ, () => ({ ...config })),
  SetConfig: (c) =>
    run(WRITE, () => {
      if (![0, 5, 15, 60].includes(c.graceMinutes)) throw fail(ErrorCode.ErrInvalidInput, 'grace time must be 5, 15 or 60 minutes, or 0 for no limit', { field: 'graceMinutes' })
      if (![4, 8, 12].includes(c.ceilingHours)) throw fail(ErrorCode.ErrInvalidInput, 'longest hold must be 4, 8 or 12 hours', { field: 'ceilingHours' })
      config = { ...c }
      const st = mockKeepAwakeState()
      mockEmit('keepawake:changed', st)
      return st
    }),
}
