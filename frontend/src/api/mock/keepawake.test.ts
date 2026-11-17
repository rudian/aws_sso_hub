import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { onEvent } from '../events'
import { ErrorCode } from '../models'
import type { KeepAwakeState } from './keepawake'
import { mockKeepAwakeService as svc, mockKeepAwakeState, resetMockKeepAwake, setMockKeepAwakeActivity } from './keepawake'

beforeEach(() => {
  vi.useFakeTimers()
  resetMockKeepAwake('')
})
afterEach(() => vi.useRealTimers())

async function call<T>(p: Promise<T>): Promise<T> {
  const done = p.then(
    (v) => ({ v }),
    (e) => ({ e }),
  )
  await vi.advanceTimersByTimeAsync(2000)
  const r = (await done) as { v?: T; e?: unknown }
  if (r.e) throw r.e
  return r.v as T
}

describe('mock keep-awake service', () => {
  it('starts idle with the defaults of plan 4.10', async () => {
    const st = await call(svc.GetState())
    expect(st).toMatchObject({ held: false, active: 0, screen: false, config: { enabled: true, screenToo: true, graceMinutes: 15, ceilingHours: 8 } })
  })

  it('holds while a task runs, and the screen follows its setting', async () => {
    setMockKeepAwakeActivity({ running: 1 })
    expect(mockKeepAwakeState()).toMatchObject({ held: true, screen: true, running: 1 })
    const cfg = await call(svc.GetConfig())
    const st = await call(svc.SetConfig({ ...cfg, screenToo: false }))
    expect(st).toMatchObject({ held: true, screen: false })
  })

  it('turning the setting off releases at once but keeps the count', async () => {
    setMockKeepAwakeActivity({ running: 2, waiting: 1 })
    const cfg = await call(svc.GetConfig())
    const st = await call(svc.SetConfig({ ...cfg, enabled: false }))
    expect(st).toMatchObject({ held: false, active: 3, running: 2, waiting: 1 })
  })

  it('a program a closed session left behind counts', () => {
    setMockKeepAwakeActivity({ leftover: 1 })
    expect(mockKeepAwakeState()).toMatchObject({ held: true, leftover: 1, active: 1 })
    setMockKeepAwakeActivity({ leftover: 0 })
    expect(mockKeepAwakeState().held).toBe(false)
  })

  it('emits keepawake:changed on every change', async () => {
    const seen: KeepAwakeState[] = []
    const off = onEvent('keepawake:changed', (s) => seen.push(s))
    setMockKeepAwakeActivity({ waiting: 1 })
    const cfg = await call(svc.GetConfig())
    await call(svc.SetConfig({ ...cfg, graceMinutes: 60 }))
    off()
    expect(seen.map((s) => [s.held, s.config.graceMinutes])).toEqual([
      [true, 15],
      [true, 60],
    ])
  })

  it('rejects values the card does not offer', async () => {
    const cfg = await call(svc.GetConfig())
    await expect(call(svc.SetConfig({ ...cfg, graceMinutes: 7 }))).rejects.toMatchObject({
      cause: { code: ErrorCode.ErrInvalidInput, params: { field: 'graceMinutes' } },
    })
    await expect(call(svc.SetConfig({ ...cfg, ceilingHours: 3 }))).rejects.toMatchObject({
      cause: { code: ErrorCode.ErrInvalidInput, params: { field: 'ceilingHours' } },
    })
  })

  it('reads the URL switch', () => {
    resetMockKeepAwake('?awake=2,1,3')
    expect(mockKeepAwakeState()).toMatchObject({ running: 2, waiting: 1, leftover: 3, held: true })
    resetMockKeepAwake('?awake=off')
    expect(mockKeepAwakeState().config.enabled).toBe(false)
  })

  it('counts the programs of the closed sessions', () => {
    resetMockKeepAwake('?awake=0,0,1,2')
    expect(mockKeepAwakeState()).toMatchObject({ leftover: 1, programs: 2 })
    resetMockKeepAwake('?awake=0,0,2')
    expect(mockKeepAwakeState()).toMatchObject({ leftover: 2, programs: 2 })
    setMockKeepAwakeActivity({ leftover: 1, programs: 3 })
    expect(mockKeepAwakeState().programs).toBe(3)
    setMockKeepAwakeActivity({ leftover: 0 })
    expect(mockKeepAwakeState().programs).toBe(0)
  })

  it('reports when the hold ends, like the real manager', async () => {
    vi.setSystemTime(new Date('2026-11-17T09:00:00Z'))
    const t0 = Date.now()
    resetMockKeepAwake('?awake=0,1')
    expect(mockKeepAwakeState().releaseAt).toBe(t0 + 15 * 60_000)
    resetMockKeepAwake('?awake=0,0,1')
    expect(mockKeepAwakeState().releaseAt).toBe(t0 + 8 * 3_600_000)
    resetMockKeepAwake('?awake=1,1')
    expect(mockKeepAwakeState().releaseAt).toBe(0) // a running task renews its own ceiling
    resetMockKeepAwake('?awake=0,1')
    const cfg = await call(svc.GetConfig())
    const st = await call(svc.SetConfig({ ...cfg, graceMinutes: 0 }))
    expect(st.releaseAt).toBe(t0 + 8 * 3_600_000)
  })

  it('has switches for the ceiling and the error', () => {
    resetMockKeepAwake('?awake=0,0,0,ceiling')
    expect(mockKeepAwakeState()).toMatchObject({ ceilingHit: true, held: false, active: 0 })
    resetMockKeepAwake('?awake=2,error')
    expect(mockKeepAwakeState()).toMatchObject({ error: 'none', held: false, screen: false, running: 2 })
    resetMockKeepAwake('?awake=0,0,1,error')
    expect(mockKeepAwakeState()).toMatchObject({ error: 'none', leftover: 1 })
  })
})
