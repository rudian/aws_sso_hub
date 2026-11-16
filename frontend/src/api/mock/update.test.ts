import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { onEvent } from '../events'
import { UpdatePhase } from '../models'
import type { UpdateState } from '../models'
import { mockUpdateService, resetUpdateMock } from './update'

async function call<T>(p: Promise<T>, ms = 1000): Promise<T> {
  const settled = p.then((v) => ({ ok: true as const, v }), (e: unknown) => ({ ok: false as const, e }))
  await vi.advanceTimersByTimeAsync(ms)
  const r = await settled
  if (!r.ok) throw r.e
  return r.v
}

describe('mock UpdateService', () => {
  beforeEach(() => vi.useFakeTimers())
  afterEach(() => vi.useRealTimers())

  it('offers nothing by default and answers a check with up to date', async () => {
    resetUpdateMock('none')
    expect((await call(mockUpdateService.State())).phase).toBe(UpdatePhase.UpdateIdle)
    expect((await call(mockUpdateService.Check())).phase).toBe(UpdatePhase.UpdateUpToDate)
  })

  it('downloads, verifies and becomes ready, emitting update:state on the way', async () => {
    resetUpdateMock('available')
    const seen: UpdateState[] = []
    const off = onEvent('update:state', (s) => seen.push(s))
    await call(mockUpdateService.Download())
    await vi.advanceTimersByTimeAsync(7000)
    off()
    const phases = seen.map((s) => s.phase)
    expect(phases[0]).toBe(UpdatePhase.UpdateDownloading)
    expect(phases).toContain(UpdatePhase.UpdateVerifying)
    expect(phases[phases.length - 1]).toBe(UpdatePhase.UpdateReady)
    expect(seen.find((s) => s.phase === UpdatePhase.UpdateDownloading && (s.written ?? 0) > 0)?.total).toBe(48_600_000)
  })

  it('verify-failed ends refused at the verify step', async () => {
    resetUpdateMock('verify-failed')
    await call(mockUpdateService.Download())
    await vi.advanceTimersByTimeAsync(7000)
    const s = await call(mockUpdateService.State())
    expect(s).toMatchObject({ phase: UpdatePhase.UpdateRefused, step: 'verify', code: 'codesign' })
  })

  it('keeps the daily-check preference', async () => {
    resetUpdateMock('none')
    expect((await call(mockUpdateService.GetPreferences())).autoCheck).toBe(true)
    await call(mockUpdateService.SetAutoCheck(false))
    expect((await call(mockUpdateService.GetPreferences())).autoCheck).toBe(false)
  })
})
