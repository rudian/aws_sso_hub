// Mock of UpdateService (design 5.13), typed against the generated updateservice module.
// Scenarios: ?update=available|downloading|ready|uptodate|check-failed|download-failed|verify-failed|blocked
// (default: nothing offered, so every other screen stays free of the banner). ?tasks=2 is read by the page, not here.
import * as UpdateReal from '../../../bindings/github.com/rudian/codepass/internal/app/updateservice'
import { UpdatePhase } from '../models'
import type { UpdatePreferences, UpdateState } from '../models'
import { mockEmit } from '../events'
import { nowISO } from './clock'
import { READ, SLOW, WRITE, run } from './util'

export type UpdateScenario = 'none' | 'available' | 'downloading' | 'ready' | 'uptodate' | 'check-failed' | 'download-failed' | 'verify-failed' | 'blocked'

const SCENARIOS: UpdateScenario[] = ['none', 'available', 'downloading', 'ready', 'uptodate', 'check-failed', 'download-failed', 'verify-failed', 'blocked']
const CURRENT = '1.0.0'
const NEXT = '1.1.0'
const TOTAL = 48_600_000
/** Release notes come from the signed file: plain lines. */
export const MOCK_NOTES = ['历史页可以按文件筛选，并显示每一行最后是谁改的', 'AI 任务结束时可以直接创建拉取请求', '修复：Bitbucket 账号偶尔需要重新验证密钥'].join('\n')

interface UState {
  scenario: UpdateScenario
  state: UpdateState
  prefs: UpdatePreferences
  timers: ReturnType<typeof setTimeout>[]
}

function queryScenario(): UpdateScenario {
  try {
    const v = typeof location === 'undefined' ? null : new URLSearchParams(location.search).get('update')
    return (SCENARIOS as string[]).includes(v ?? '') ? (v as UpdateScenario) : 'none'
  } catch {
    return 'none'
  }
}

const base = (): UpdateState => ({ phase: UpdatePhase.UpdateIdle, current: CURRENT })
const offered = (): UpdateState => ({ phase: UpdatePhase.UpdateAvailable, current: CURRENT, version: NEXT, notes: MOCK_NOTES, total: TOTAL, checkedAt: nowISO() })

function initial(s: UpdateScenario): UpdateState {
  switch (s) {
    case 'available':
      return offered()
    case 'downloading':
      return { ...offered(), phase: UpdatePhase.UpdateDownloading, written: 31_200_000 }
    case 'ready':
      return { ...offered(), phase: UpdatePhase.UpdateReady, total: undefined }
    case 'uptodate':
      return { phase: UpdatePhase.UpdateUpToDate, current: CURRENT, checkedAt: nowISO() }
    default:
      return base()
  }
}

let u: UState

export function resetUpdateMock(scenario: UpdateScenario = queryScenario()): void {
  if (u) u.timers.forEach(clearTimeout)
  u = { scenario, state: initial(scenario), prefs: { autoCheck: true }, timers: [] }
}
resetUpdateMock()

function set(s: UpdateState): void {
  u.state = { ...s, current: CURRENT }
  mockEmit('update:state', u.state)
}
function later(ms: number, fn: () => void): void {
  u.timers.push(setTimeout(fn, ms))
}

export const mockUpdateService: typeof UpdateReal = {
  State: () => run(READ, () => u.state),

  Check: () =>
    run(SLOW, () => {
      set({ phase: UpdatePhase.UpdateChecking, current: CURRENT })
      const sc = u.scenario
      if (sc === 'check-failed') set({ phase: UpdatePhase.UpdateError, current: CURRENT, code: 'network', step: 'check', checkedAt: nowISO() })
      else if (sc === 'blocked') set({ phase: UpdatePhase.UpdateBlocked, current: CURRENT, block: 'translocated', code: 'translocated', step: 'check', folder: '/Volumes/Codepass', checkedAt: nowISO() })
      else if (sc === 'none' || sc === 'uptodate') set({ phase: UpdatePhase.UpdateUpToDate, current: CURRENT, checkedAt: nowISO() })
      else set(offered())
      return u.state
    }),

  Retry: () => mockUpdateService.Check(),

  Download: () =>
    run(WRITE, () => {
      set({ ...offered(), phase: UpdatePhase.UpdateDownloading, written: 0 })
      const steps = 10
      for (let i = 1; i <= steps; i++) {
        later(i * 400, () => {
          if (u.scenario === 'download-failed' && i === 4) {
            set({ ...offered(), phase: UpdatePhase.UpdateError, written: Math.round((TOTAL * 4) / steps), code: 'network', step: 'download' })
          } else if (u.state.phase === UpdatePhase.UpdateDownloading) {
            set({ ...offered(), phase: UpdatePhase.UpdateDownloading, written: Math.round((TOTAL * i) / steps) })
          }
        })
      }
      later((steps + 1) * 400, () => {
        if (u.state.phase !== UpdatePhase.UpdateDownloading) return
        set({ ...offered(), phase: UpdatePhase.UpdateVerifying, step: 'verify' })
        later(900, () => {
          if (u.scenario === 'verify-failed') set({ ...offered(), phase: UpdatePhase.UpdateRefused, code: 'codesign', step: 'verify' })
          else set({ ...offered(), phase: UpdatePhase.UpdateReady, total: undefined })
        })
      })
      return u.state
    }),

  Restart: () =>
    run(WRITE, () => {
      set({ phase: UpdatePhase.UpdateRestarting, current: CURRENT, version: NEXT, step: 'restart' })
      // The real controller gives up after 45 s; the demo does after 6 s so the dialog does not stay locked.
      later(6000, () => {
        if (u.state.phase === UpdatePhase.UpdateRestarting) set({ ...offered(), phase: UpdatePhase.UpdateError, code: 'restart-timeout', step: 'restart' })
      })
      return u.state
    }),

  GetPreferences: () => run(READ, () => u.prefs),

  SetAutoCheck: (on) =>
    run(WRITE, () => {
      u.prefs = { autoCheck: on }
      return u.prefs
    }),

  OpenDownloadPage: () =>
    run(READ, () => {
      /* the real service opens the releases page */
    }),
}
