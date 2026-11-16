// How many AI tasks are in the running state (not waiting for the person, done or stopped). The AI-task engine
// (milestone 2) calls setRunningTasks whenever that number changes; until then it is 0, and the mock can start with
// ?tasks=2. The update dialog and the reopen watcher read it.
import { useSyncExternalStore } from 'react'

let count = 0
const listeners = new Set<() => void>()

export function getRunningTasks(): number {
  return count
}

export function setRunningTasks(n: number): void {
  const next = Math.max(0, Math.floor(n))
  if (next === count) return
  count = next
  for (const l of [...listeners]) l()
}

export function subscribeRunningTasks(cb: () => void): () => void {
  listeners.add(cb)
  return () => listeners.delete(cb)
}

export function useRunningTasks(): number {
  return useSyncExternalStore(subscribeRunningTasks, getRunningTasks, getRunningTasks)
}

/** Mock mode only: ?tasks=2 pretends two tasks run, so the warning and the waiting flow can be seen. */
export function seedRunningTasksFromQuery(search: string): void {
  const v = new URLSearchParams(search).get('tasks')
  if (v && /^\d{1,3}$/.test(v)) setRunningTasks(Number(v))
}
