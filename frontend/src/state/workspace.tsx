// The workspace's per-folder state (design 4.2). A repository has one or more working trees ("folders": the main
// workspace now, an AI task's own folder from milestone 3). Open editor tabs, the selected file and the bottom panel
// belong to a folder: switching away and back restores them. The selected tab (Code, Changes, ...) belongs to the
// repository: choosing another folder re-points the same tab at it (design 4.2, 5.5). The store lives outside React so the
// state survives leaving the workspace and coming back (the page unmounts, the state stays).
import { useCallback, useSyncExternalStore } from 'react'
import { initialUI } from '../lib/initialUI'
import { WORKSPACE_TABS } from '../lib/route'
import type { WorkspaceTabId } from '../lib/route'

export type WsTab = WorkspaceTabId
export const WS_TABS: readonly WsTab[] = WORKSPACE_TABS
export type PanelTab = 'terminal' | 'problems' | 'find'
export const PANEL_TABS: readonly PanelTab[] = ['terminal', 'problems', 'find']
export const MAIN_FOLDER = 'main'

export const PANEL_DEFAULT = 196
export const PANEL_MIN = 120
export const PANEL_STEP = 16

export interface PanelState {
  open: boolean
  tab: PanelTab
  /** px */
  height: number
}

export interface FolderState {
  /** Editor tabs (file paths relative to the folder), filled by the Code tab (milestone 3). */
  openFiles: string[]
  activeFile: string | null
  panel: PanelState
}

export type WorkTreeKind = 'main' | 'task'

/** One entry of the working-tree switcher. Milestone 3 adds the task rows (tool, title, branch, state). */
export interface WorkTree {
  id: string
  kind: WorkTreeKind
  /** Branch shown in the pill and the row; '' when unknown or detached. */
  branch: string
  /** Task title for kind 'task'. */
  title?: string
}

export interface RepoWorkspace {
  /** The folder on screen. */
  folder: string
  /** The selected workspace tab, shared by the folders of the repository. */
  tab: WsTab
  folders: Readonly<Record<string, FolderState>>
}

export function initialFolder(): FolderState {
  return { openFiles: [], activeFile: null, panel: { open: false, tab: 'terminal', height: PANEL_DEFAULT } }
}

export function clampPanelHeight(h: number, windowHeight = typeof window !== 'undefined' ? window.innerHeight : 900): number {
  const max = Math.max(PANEL_MIN, Math.floor(windowHeight * 0.6))
  return Math.min(max, Math.max(PANEL_MIN, Math.round(h)))
}

/** The next working tree when Ctrl+Tab cycles (dir 1) or Ctrl+Shift+Tab (dir -1). One tree: stays. */
export function cycleFolder(ids: readonly string[], current: string, dir: 1 | -1): string {
  if (ids.length === 0) return current
  const i = ids.indexOf(current)
  if (i < 0) return ids[0]
  return ids[(i + dir + ids.length) % ids.length]
}

// ---------------------------------------------------------------- the store

const STORE_KEY = 'codepass.ws.panel.'
const spaces = new Map<string, RepoWorkspace>()
const listeners = new Set<() => void>()
const FRESH: RepoWorkspace = { folder: MAIN_FOLDER, tab: 'code', folders: { [MAIN_FOLDER]: initialFolder() } }

function emit(): void {
  for (const l of [...listeners]) l()
}

function readPanel(repoId: string, folder: string): Partial<PanelState> | null {
  try {
    const raw = window.localStorage.getItem(`${STORE_KEY}${repoId}.${folder}`)
    if (!raw) return null
    const v = JSON.parse(raw) as Partial<PanelState>
    return v && typeof v === 'object' ? v : null
  } catch {
    return null
  }
}

function writePanel(repoId: string, folder: string, p: PanelState): void {
  try {
    window.localStorage.setItem(`${STORE_KEY}${repoId}.${folder}`, JSON.stringify(p))
  } catch {
    /* private window or blocked site data: the panel just is not remembered */
  }
}

function seedFolder(repoId: string, folder: string): FolderState {
  const f = initialFolder()
  const saved = readPanel(repoId, folder)
  if (saved) {
    if (typeof saved.open === 'boolean') f.panel.open = saved.open
    if (saved.tab && (PANEL_TABS as readonly string[]).includes(saved.tab)) f.panel.tab = saved.tab
    if (typeof saved.height === 'number') f.panel.height = clampPanelHeight(saved.height)
  }
  // Mock links only (?panel=): the panel starts open on that tab
  if (initialUI.panel && (PANEL_TABS as readonly string[]).includes(initialUI.panel)) f.panel = { ...f.panel, open: true, tab: initialUI.panel as PanelTab }
  return f
}

/** The repository's state; the first read seeds it (the panel is restored from what was remembered). Stable afterwards. */
export function getWorkspace(repoId: string): RepoWorkspace {
  let ws = spaces.get(repoId)
  if (!ws) {
    ws = ensure(repoId, MAIN_FOLDER)
    spaces.set(repoId, ws)
  }
  return ws
}

/** The folder state, seeded from what was remembered when first used. */
function ensure(repoId: string, folder: string): RepoWorkspace {
  let ws = spaces.get(repoId)
  if (!ws) ws = { folder: MAIN_FOLDER, tab: 'code', folders: {} }
  if (!ws.folders[folder]) ws = { ...ws, folders: { ...ws.folders, [folder]: seedFolder(repoId, folder) } }
  return ws
}

/** Change the folder on screen (the state of the folder you leave is kept as it is). */
export function selectFolder(repoId: string, folder: string): void {
  const ws = ensure(repoId, folder)
  spaces.set(repoId, { ...ws, folder })
  emit()
}

/** Change one folder's state (default: the folder on screen). */
export function updateFolder(repoId: string, patch: (f: FolderState) => FolderState, folder?: string): void {
  const base = ensure(repoId, folder ?? spaces.get(repoId)?.folder ?? MAIN_FOLDER)
  const id = folder ?? base.folder
  const before = base.folders[id]
  const after = patch(before)
  if (after === before && spaces.get(repoId) === base) return
  spaces.set(repoId, { ...base, folders: { ...base.folders, [id]: after } })
  if (after.panel !== before.panel) writePanel(repoId, id, after.panel)
  emit()
}

export function setTab(repoId: string, tab: WsTab): void {
  const base = ensure(repoId, spaces.get(repoId)?.folder ?? MAIN_FOLDER)
  if (base.tab === tab && spaces.get(repoId) === base) return
  spaces.set(repoId, { ...base, tab })
  emit()
}

export function setPanel(repoId: string, patch: Partial<PanelState>): void {
  updateFolder(repoId, (f) => {
    const next = { ...f.panel, ...patch }
    if (patch.height !== undefined) next.height = clampPanelHeight(patch.height)
    return next.open === f.panel.open && next.tab === f.panel.tab && next.height === f.panel.height ? f : { ...f, panel: next }
  })
}

/** Ctrl+` : opens the panel on the Terminal tab, or closes it when the terminal is what it shows (Problems or Find: switches to the terminal). */
export function togglePanel(repoId: string): void {
  updateFolder(repoId, (f) => ({ ...f, panel: f.panel.open && f.panel.tab === 'terminal' ? { ...f.panel, open: false } : { ...f.panel, open: true, tab: 'terminal' } }))
}

/** Test helper: forget everything. */
export function resetWorkspaceStore(): void {
  spaces.clear()
  emit()
}

function subscribe(cb: () => void): () => void {
  listeners.add(cb)
  return () => listeners.delete(cb)
}

export interface WorkspaceApi {
  /** The folder on screen. */
  folderId: string
  folder: FolderState
  /** The selected workspace tab of the repository. */
  tab: WsTab
  folders: readonly string[]
  selectFolder(id: string): void
  setTab(tab: WsTab): void
  setPanel(patch: Partial<PanelState>): void
  togglePanel(): void
}

/** The workspace state of one repository: the folder on screen and its state. */
export function useWorkspace(repoId: string): WorkspaceApi {
  const ws = useSyncExternalStore(subscribe, () => getWorkspace(repoId), () => FRESH)
  const folder = ws.folders[ws.folder] ?? initialFolder()
  const doSelect = useCallback((id: string) => selectFolder(repoId, id), [repoId])
  const doTab = useCallback((tab: WsTab) => setTab(repoId, tab), [repoId])
  const doPanel = useCallback((p: Partial<PanelState>) => setPanel(repoId, p), [repoId])
  const doToggle = useCallback(() => togglePanel(repoId), [repoId])
  return { folderId: ws.folder, folder, tab: ws.tab, folders: Object.keys(ws.folders), selectFolder: doSelect, setTab: doTab, setPanel: doPanel, togglePanel: doToggle }
}
