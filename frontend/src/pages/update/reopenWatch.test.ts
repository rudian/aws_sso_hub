import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { armReopen, cancelReopen, COUNTDOWN_S, getReopenState, IDLE_MS } from './reopenWatch'
import { setRunningTasks } from './runningTasks'

describe('Reopen when no task is running', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    setRunningTasks(0)
  })
  afterEach(() => {
    cancelReopen()
    setRunningTasks(0)
    vi.useRealTimers()
  })

  it('waits 60 quiet seconds, counts down 10, then reopens once', () => {
    const fire = vi.fn()
    armReopen(fire)
    expect(getReopenState()).toEqual({ mode: 'waiting' })
    vi.advanceTimersByTime(IDLE_MS - 1)
    expect(getReopenState().mode).toBe('waiting')
    vi.advanceTimersByTime(1)
    expect(getReopenState()).toEqual({ mode: 'countdown', secondsLeft: COUNTDOWN_S })
    vi.advanceTimersByTime(3000)
    expect(getReopenState()).toEqual({ mode: 'countdown', secondsLeft: COUNTDOWN_S - 3 })
    expect(fire).not.toHaveBeenCalled()
    vi.advanceTimersByTime(7000)
    expect(fire).toHaveBeenCalledTimes(1)
    expect(getReopenState().mode).toBe('off')
  })

  it('does not start the quiet time while a task runs, and restarts it when one starts again', () => {
    const fire = vi.fn()
    setRunningTasks(2)
    armReopen(fire)
    vi.advanceTimersByTime(IDLE_MS * 3)
    expect(getReopenState().mode).toBe('waiting')
    setRunningTasks(0)
    vi.advanceTimersByTime(IDLE_MS - 1000)
    setRunningTasks(1) // a task starts: the quiet time is gone
    vi.advanceTimersByTime(IDLE_MS)
    expect(getReopenState().mode).toBe('waiting')
    setRunningTasks(0)
    vi.advanceTimersByTime(IDLE_MS)
    expect(getReopenState().mode).toBe('countdown')
    expect(fire).not.toHaveBeenCalled()
  })

  it('a task that starts during the countdown cancels the reopen', () => {
    const fire = vi.fn()
    armReopen(fire)
    vi.advanceTimersByTime(IDLE_MS + 4000)
    setRunningTasks(1)
    expect(getReopenState().mode).toBe('waiting')
    vi.advanceTimersByTime(IDLE_MS)
    expect(fire).not.toHaveBeenCalled()
  })

  it('Cancel stops everything', () => {
    const fire = vi.fn()
    armReopen(fire)
    vi.advanceTimersByTime(IDLE_MS + 2000)
    cancelReopen()
    vi.advanceTimersByTime(IDLE_MS * 2)
    expect(fire).not.toHaveBeenCalled()
    expect(getReopenState().mode).toBe('off')
  })
})
