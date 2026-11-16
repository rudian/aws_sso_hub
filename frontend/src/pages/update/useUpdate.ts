// The update store: one UpdateState for the whole window, kept current by the update:state event. Components read it
// with useUpdateState(); the actions call UpdateService. Nothing here opens a dialog by itself (never a modal at launch).
import { useEffect, useSyncExternalStore } from 'react'
import { useMocks } from '../../api/env'
import { onEvent } from '../../api/events'
import { UpdatePhase } from '../../api/models'
import type { UpdateState } from '../../api/models'
import { Updates } from '../../api/services'
import { useI18n } from '../../i18n'
import { useActions } from '../../shell/AppActions'
import { setRoute } from '../../lib/route'
import { compareVersions, showsDot } from './model'
import { armReopen, cancelReopen, setReopenReady } from './reopenWatch'
import { seedRunningTasksFromQuery } from './runningTasks'
import './update.css'

const IDLE: UpdateState = { phase: UpdatePhase.UpdateIdle, current: '' }
let state: UpdateState = IDLE
let started = false
let version = ''
const listeners = new Set<() => void>()
const dialogListeners = new Set<() => void>()
let dialogOpen = false

function set(s: UpdateState): void {
  state = s
  if (s.current) version = s.current
  // An armed "reopen when no task is running" only makes sense while the flow is going on or waiting for Restart.
  const flowing = s.phase === UpdatePhase.UpdateDownloading || s.phase === UpdatePhase.UpdateVerifying || s.phase === UpdatePhase.UpdateReady
  if (!flowing) cancelReopen()
  setReopenReady(s.phase === UpdatePhase.UpdateReady)
  remember(s)
  for (const l of [...listeners]) l()
}

function start(): void {
  if (started) return
  started = true
  if (useMocks) {
    // Mock mode only (screenshots and demos): ?tasks=2 pretends two tasks run; ?updateDialog=1 opens the dialog. The real app never opens it by itself.
    seedRunningTasksFromQuery(location.search)
    if (new URLSearchParams(location.search).get('updateDialog') === '1') queueMicrotask(openUpdateDialog)
  }
  onEvent('update:state', set)
  Updates.State().then(set, () => undefined)
}

export function getUpdateState(): UpdateState {
  return state
}

/** Used by tests and by the mock page between scenarios. */
export function resetUpdateStore(): void {
  state = IDLE
  started = false
  version = ''
  dialogOpen = false
  dismissed = undefined
  cancelReopen()
  setReopenReady(true)
  for (const l of [...listeners]) l()
}

/** The current UpdateState (idle until the first answer). Starts listening on first use. */
export function useUpdateState(): UpdateState {
  useEffect(start, [])
  return useSyncExternalStore(
    (cb) => {
      listeners.add(cb)
      return () => listeners.delete(cb)
    },
    getUpdateState,
    getUpdateState,
  )
}

// ---- the dialog is one shared piece of state, so the banner, the page and the Settings card open the same one ----

export function openUpdateDialog(): void {
  dialogOpen = true
  for (const l of [...dialogListeners]) l()
}
export function closeUpdateDialog(): void {
  dialogOpen = false
  for (const l of [...dialogListeners]) l()
}
export function useUpdateDialogOpen(): boolean {
  return useSyncExternalStore(
    (cb) => {
      dialogListeners.add(cb)
      return () => dialogListeners.delete(cb)
    },
    () => dialogOpen,
    () => dialogOpen,
  )
}

// ---- actions ----

export const updateActions = {
  check: () => Updates.Check().then(set, () => undefined),
  retry: () => Updates.Retry().then(set, () => undefined),
  download: () => Updates.Download().then(set, () => undefined),
  restart: () => Updates.Restart().then(set, () => undefined),
  openBrowser: () => Updates.OpenDownloadPage().catch(() => undefined),
  /** Start the update: open the dialog and begin the download. */
  start: () => {
    openUpdateDialog()
    void updateActions.download()
  },
  /** "Reopen when no task is running": the watcher reopens through restart() when its time comes. */
  reopenWhenIdle: () => armReopen(() => void updateActions.restart()),
  cancelWait: () => cancelReopen(),
}

// ---- per-version dismissal of the banner and the "updated" toast; browser storage is only a convenience ----

const DISMISS_KEY = 'codepass.update.dismissed'
const LASTRUN_KEY = 'codepass.update.lastRun'
const NOTES_KEY = 'codepass.update.notes'

function read(key: string): string | null {
  try {
    return localStorage.getItem(key)
  } catch {
    return null
  }
}
function write(key: string, value: string): void {
  try {
    localStorage.setItem(key, value)
  } catch {
    /* private window or blocked storage: the convenience is simply off */
  }
}

let dismissed: string | null | undefined // undefined = not read yet
const dismissListeners = new Set<() => void>()
const getDismissed = (): string | null => (dismissed === undefined ? (dismissed = read(DISMISS_KEY)) : dismissed)

/** The version whose banner the person dismissed ('' when none); re-renders when it changes. */
export function useDismissedVersion(): string | null {
  return useSyncExternalStore(
    (cb) => {
      dismissListeners.add(cb)
      return () => dismissListeners.delete(cb)
    },
    getDismissed,
    getDismissed,
  )
}
export function dismissBanner(v: string): void {
  dismissed = v
  write(DISMISS_KEY, v)
  for (const l of [...dismissListeners]) l()
}

/** Keep the notes of a staged release, so "What's new" still works after the app has reopened on it. */
function remember(s: UpdateState): void {
  if ((s.phase === UpdatePhase.UpdateReady || s.phase === UpdatePhase.UpdateRestarting) && s.version && s.notes) {
    write(NOTES_KEY, JSON.stringify({ version: s.version, notes: s.notes }))
  }
}

/** The notes kept for `v`, or '' when this window never downloaded that version. */
export function rememberedNotes(v: string): string {
  try {
    const o = JSON.parse(read(NOTES_KEY) ?? 'null') as { version?: string; notes?: string } | null
    return o && o.version === v && typeof o.notes === 'string' ? o.notes : ''
  } catch {
    return ''
  }
}

/**
 * For the sidebar: whether Settings carries the update dot. It also announces a finished update once: when the
 * running version is newer than the one the last run recorded, a toast says "Updated to X" with a link to the notes.
 */
export function useUpdateIndicator(): boolean {
  const s = useUpdateState()
  const { t } = useI18n()
  const { toast } = useActions()
  const current = s.current || version
  useEffect(() => {
    if (!current) return
    const last = read(LASTRUN_KEY)
    write(LASTRUN_KEY, current)
    if (last && compareVersions(current, last) > 0) {
      toast({ kind: 'ok', title: t('update.toast.done', { version: current }), action: { label: t('update.toast.notes'), onClick: () => setRoute({ page: 'update' }) } })
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [current])
  return showsDot(s)
}
