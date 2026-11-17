import { beforeEach, describe, expect, it } from 'vitest'
import { MAIN_FOLDER, PANEL_DEFAULT, PANEL_MIN, clampPanelHeight, cycleFolder, getWorkspace, resetWorkspaceStore, selectFolder, setPanel, setTab, togglePanel, updateFolder } from './workspace'

beforeEach(() => {
  window.localStorage.clear()
  resetWorkspaceStore()
})

describe('workspace state', () => {
  it('starts on the Code tab of the main workspace with the panel closed', () => {
    const ws = getWorkspace('r1')
    expect(ws.folder).toBe(MAIN_FOLDER)
    expect(ws.tab).toBe('code')
    expect(ws.folders[MAIN_FOLDER]).toMatchObject({ openFiles: [], activeFile: null, panel: { open: false, tab: 'terminal', height: PANEL_DEFAULT } })
  })

  it('keeps a folder\'s files and panel when you switch away and back; the tab stays (per-folder model)', () => {
    setTab('r1', 'changes')
    updateFolder('r1', (f) => ({ ...f, openFiles: ['a.go'], activeFile: 'a.go' }))
    togglePanel('r1')
    selectFolder('r1', 'task-1')
    // the new folder starts clean
    expect(getWorkspace('r1').tab).toBe('changes')
    expect(getWorkspace('r1').folders['task-1']).toMatchObject({ activeFile: null, panel: { open: false } })
    setTab('r1', 'history')
    selectFolder('r1', MAIN_FOLDER)
    const main = getWorkspace('r1').folders[MAIN_FOLDER]
    expect(getWorkspace('r1').tab).toBe('history')
    expect(main).toMatchObject({ openFiles: ['a.go'], activeFile: 'a.go', panel: { open: true, tab: 'terminal' } })
  })

  it('keeps repositories apart', () => {
    setTab('r1', 'prs')
    expect(getWorkspace('r2').tab).toBe('code')
  })

  it('returns the same object until something changes (stable for useSyncExternalStore)', () => {
    const a = getWorkspace('r1')
    expect(getWorkspace('r1')).toBe(a)
    setTab('r1', 'code')
    expect(getWorkspace('r1')).toBe(a)
    setTab('r1', 'ai')
    expect(getWorkspace('r1')).not.toBe(a)
  })

  it('Ctrl+` opens the panel on the Terminal tab, and closes it', () => {
    setPanel('r1', { open: true, tab: 'terminal' })
    togglePanel('r1')
    expect(getWorkspace('r1').folders[MAIN_FOLDER].panel.open).toBe(false)
    togglePanel('r1')
    expect(getWorkspace('r1').folders[MAIN_FOLDER].panel).toMatchObject({ open: true, tab: 'terminal' })
  })

  it('Ctrl+` on Problems or Find shows the terminal instead of closing the panel', () => {
    setPanel('r1', { open: true, tab: 'find' })
    togglePanel('r1')
    expect(getWorkspace('r1').folders[MAIN_FOLDER].panel).toMatchObject({ open: true, tab: 'terminal' })
  })

  it('remembers the panel per repository and folder (a new session of the store reads it back)', () => {
    setPanel('r1', { open: true, tab: 'find', height: 300 })
    resetWorkspaceStore()
    expect(getWorkspace('r1').folders[MAIN_FOLDER].panel).toEqual({ open: true, tab: 'find', height: 300 })
    expect(getWorkspace('r2').folders[MAIN_FOLDER].panel.open).toBe(false)
  })

  it('works without localStorage (private window)', () => {
    const real = window.localStorage
    Object.defineProperty(window, 'localStorage', { configurable: true, get: () => { throw new Error('blocked') } })
    try {
      setPanel('r9', { open: true })
      expect(getWorkspace('r9').folders[MAIN_FOLDER].panel.open).toBe(true)
    } finally {
      Object.defineProperty(window, 'localStorage', { configurable: true, value: real })
    }
  })

  it('clamps the panel height between the minimum and 60% of the window', () => {
    expect(clampPanelHeight(10, 900)).toBe(PANEL_MIN)
    expect(clampPanelHeight(2000, 900)).toBe(540)
    expect(clampPanelHeight(250.4, 900)).toBe(250)
    setPanel('r1', { height: 5 })
    expect(getWorkspace('r1').folders[MAIN_FOLDER].panel.height).toBe(PANEL_MIN)
  })

  it('cycles working trees forwards and backwards, and stays with one tree', () => {
    expect(cycleFolder(['main', 'a', 'b'], 'main', 1)).toBe('a')
    expect(cycleFolder(['main', 'a', 'b'], 'b', 1)).toBe('main')
    expect(cycleFolder(['main', 'a', 'b'], 'main', -1)).toBe('b')
    expect(cycleFolder(['main'], 'main', 1)).toBe('main')
    expect(cycleFolder([], 'main', 1)).toBe('main')
    expect(cycleFolder(['main', 'a'], 'gone', 1)).toBe('main')
  })
})
