// "Reopen when no task is running" (design 5.13). Armed by the person: once no AI task has been in the running
// state for IDLE_MS the watcher counts down COUNTDOWN_S seconds (Cancel stops it) and then calls `fire`. A task
// starting again resets the quiet time. It lives at module level so closing the dialog does not stop it; the banner
// and the dialog both show its state.
import { useSyncExternalStore } from 'react'
import { getRunningTasks, subscribeRunningTasks } from './runningTasks'

export const IDLE_MS = 60_000
export const COUNTDOWN_S = 10

export type WatchState = { mode: 'off' } | { mode: 'waiting' } | { mode: 'countdown'; secondsLeft: number }

let state: WatchState = { mode: 'off' }
let fire: (() => void) | null = null
let idleTimer: ReturnType<typeof setTimeout> | undefined
let tick: ReturnType<typeof setInterval> | undefined
let offTasks: (() => void) | undefined
const listeners = new Set<() => void>()

function set(s: WatchState): void {
  state = s
  for (const l of [...listeners]) l()
}

function clearTimers(): void {
  clearTimeout(idleTimer)
  clearInterval(tick)
  idleTimer = undefined
  tick = undefined
}

// The quiet clock only runs once the update is verified and staged; before that the watch just waits.
let ready = true
/** The update flow tells the watch whether the verified copy is staged. Becoming ready starts the clock of an armed watch. */
export function setReopenReady(r: boolean): void {
  ready = r
  if (r && state.mode === 'waiting' && idleTimer === undefined) startIdleClock()
}

function startIdleClock(): void {
  clearTimers()
  set({ mode: 'waiting' })
  if (!ready || getRunningTasks() > 0) return // the clock starts when the last task stops
  idleTimer = setTimeout(startCountdown, IDLE_MS)
}

function startCountdown(): void {
  clearTimers()
  let left = COUNTDOWN_S
  set({ mode: 'countdown', secondsLeft: left })
  tick = setInterval(() => {
    left -= 1
    if (left <= 0) {
      const f = fire
      cancelReopen()
      f?.()
      return
    }
    set({ mode: 'countdown', secondsLeft: left })
  }, 1000)
}

/** Start watching. `onFire` is what reopens the app. Calling it again restarts the quiet time. */
export function armReopen(onFire: () => void): void {
  fire = onFire
  offTasks?.()
  offTasks = subscribeRunningTasks(() => {
    if (state.mode === 'off') return
    // A task that starts during the countdown cancels the reopen: the person's work is running again.
    startIdleClock()
  })
  startIdleClock()
}

export function cancelReopen(): void {
  clearTimers()
  offTasks?.()
  offTasks = undefined
  fire = null
  set({ mode: 'off' })
}

export function getReopenState(): WatchState {
  return state
}

export function useReopenWatch(): WatchState {
  return useSyncExternalStore(
    (cb) => {
      listeners.add(cb)
      return () => listeners.delete(cb)
    },
    getReopenState,
    getReopenState,
  )
}
