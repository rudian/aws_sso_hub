import { describe, expect, it } from 'vitest'
import { commandForKey, inTerminal, isReloadKey, registerTerminalFocusHook, resolveCommand } from './AppCommands'

describe('resolveCommand', () => {
  it('Cmd+W never closes the window while a tab is open', () => {
    expect(resolveCommand('close-tab', 2, 'management')).toEqual({ kind: 'dispatch', id: 'close-tab' })
    expect(resolveCommand('close-tab', 1, 'workspace')).toEqual({ kind: 'dispatch', id: 'close-tab' })
  })
  it('with no tab: nothing in the workspace, close the window in the management layout', () => {
    expect(resolveCommand('close-tab', 0, 'workspace')).toEqual({ kind: 'none' })
    expect(resolveCommand('close-tab', 0, 'management')).toEqual({ kind: 'close-window' })
  })
  it('routes and pass-through commands', () => {
    expect(resolveCommand('settings', 0, 'management')).toEqual({ kind: 'route', route: { page: 'settings' } })
    expect(resolveCommand('quick-open', 0, 'management')).toEqual({ kind: 'dispatch', id: 'quick-open' })
  })
})

describe('keys', () => {
  it('Cmd/Ctrl+R and F5 are reload keys', () => {
    expect(isReloadKey({ key: 'r', metaKey: true, ctrlKey: false })).toBe(true)
    expect(isReloadKey({ key: 'R', metaKey: false, ctrlKey: true })).toBe(true)
    expect(isReloadKey({ key: 'F5', metaKey: false, ctrlKey: false })).toBe(true)
    expect(isReloadKey({ key: 'c', metaKey: true, ctrlKey: false })).toBe(false)
  })
  it('copy, paste and undo are never mapped to a command', () => {
    for (const key of ['c', 'v', 'x', 'z', 'a']) expect(commandForKey({ key, shiftKey: false })).toBeNull()
  })
  it('maps the Windows shortcuts', () => {
    expect(commandForKey({ key: 'w', shiftKey: false })).toBe('close-tab')
    expect(commandForKey({ key: 'W', shiftKey: true })).toBe('close-window')
    expect(commandForKey({ key: 'F', shiftKey: true })).toBe('find-in-files')
    expect(commandForKey({ key: 'f', shiftKey: false })).toBeNull() // Ctrl+F stays with App.tsx (focusSearch)
  })
})

describe('terminal focus (design 4.3)', () => {
  it('the app takes only Ctrl+` and Ctrl+Shift+J', () => {
    expect(commandForKey({ key: '`', shiftKey: false }, true)).toBe('toggle-terminal')
    expect(commandForKey({ key: 'J', shiftKey: true }, true)).toBe('switch-session')
    for (const key of ['w', 't', 'p', 's', 'f', 'j', 'r']) expect(commandForKey({ key, shiftKey: false }, true)).toBeNull()
    expect(commandForKey({ key: 'F', shiftKey: true }, true)).toBeNull()
    expect(commandForKey({ key: 'W', shiftKey: true }, true)).toBeNull()
  })
  it('outside a terminal the table is unchanged and Ctrl+Shift+J also switches', () => {
    expect(commandForKey({ key: 'j', shiftKey: false }, false)).toBe('switch-session')
    expect(commandForKey({ key: 'J', shiftKey: true }, false)).toBe('switch-session')
  })
  it('detects a terminal by the registered hook', () => {
    expect(inTerminal(null)).toBe(false)
    const off = registerTerminalFocusHook(() => true)
    expect(inTerminal(null)).toBe(true)
    off()
    expect(inTerminal(null)).toBe(false)
  })
})

describe('menu locale', () => {
  it('zh and en have the same non-empty keys', async () => {
    const { zh, en } = await import('../i18n/locales/menu')
    expect(Object.keys(en).sort()).toEqual(Object.keys(zh).sort())
    for (const v of [...Object.values(zh), ...Object.values(en)]) expect(v).not.toBe('')
  })
})
