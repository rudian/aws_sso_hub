import { describe, expect, it } from 'vitest'
import { inTerminal, wsKeyAction } from './keymap'
import type { KeyLike } from './keymap'

const k = (init: Partial<KeyLike>): KeyLike => ({ key: '', code: '', ctrlKey: false, metaKey: false, altKey: false, shiftKey: false, ...init })

describe('workspace keyboard map (design 4.3)', () => {
  it('macOS: Ctrl+1..6 pick the tabs in order', () => {
    const tabs = ['code', 'changes', 'history', 'branches', 'prs', 'ai']
    tabs.forEach((tab, i) => expect(wsKeyAction(k({ ctrlKey: true, key: String(i + 1), code: `Digit${i + 1}` }), 'darwin', false)).toEqual({ kind: 'tab', tab }))
  })
  it('macOS: Cmd+digits belong to the app (pages), Alt+digits to nobody', () => {
    expect(wsKeyAction(k({ metaKey: true, key: '1', code: 'Digit1' }), 'darwin', false)).toBeNull()
    expect(wsKeyAction(k({ altKey: true, key: '1', code: 'Digit1' }), 'darwin', false)).toBeNull()
  })
  it('Windows: Alt+1..6 pick the tabs; Ctrl+digits belong to the app', () => {
    expect(wsKeyAction(k({ altKey: true, key: '6', code: 'Digit6' }), 'windows', false)).toEqual({ kind: 'tab', tab: 'ai' })
    expect(wsKeyAction(k({ ctrlKey: true, key: '2', code: 'Digit2' }), 'windows', false)).toBeNull()
  })
  it('Windows: Alt+Numpad digits type Alt codes and are not taken', () => {
    expect(wsKeyAction(k({ altKey: true, key: '1', code: 'Numpad1' }), 'windows', false)).toBeNull()
  })
  it('there is no seventh tab', () => {
    expect(wsKeyAction(k({ ctrlKey: true, key: '7', code: 'Digit7' }), 'darwin', false)).toBeNull()
    expect(wsKeyAction(k({ altKey: true, key: '0', code: 'Digit0' }), 'windows', false)).toBeNull()
  })
  it('Ctrl+` toggles the panel on both platforms, also while a terminal has focus', () => {
    for (const p of ['darwin', 'windows'] as const) {
      expect(wsKeyAction(k({ ctrlKey: true, key: '`', code: 'Backquote' }), p, false)).toEqual({ kind: 'panel' })
      expect(wsKeyAction(k({ ctrlKey: true, key: '`', code: 'Backquote' }), p, true)).toEqual({ kind: 'panel' })
    }
    expect(wsKeyAction(k({ ctrlKey: true, shiftKey: true, key: '~', code: 'Backquote' }), 'darwin', false)).toBeNull()
  })
  it('Ctrl+Tab and Ctrl+Shift+Tab cycle the working trees', () => {
    expect(wsKeyAction(k({ ctrlKey: true, key: 'Tab', code: 'Tab' }), 'darwin', false)).toEqual({ kind: 'tree', dir: 1 })
    expect(wsKeyAction(k({ ctrlKey: true, shiftKey: true, key: 'Tab', code: 'Tab' }), 'windows', false)).toEqual({ kind: 'tree', dir: -1 })
  })
  it('while a terminal has focus the tab and tree keys go to the process', () => {
    expect(wsKeyAction(k({ ctrlKey: true, key: '1', code: 'Digit1' }), 'darwin', true)).toBeNull()
    expect(wsKeyAction(k({ altKey: true, key: '1', code: 'Digit1' }), 'windows', true)).toBeNull()
    expect(wsKeyAction(k({ ctrlKey: true, key: 'Tab', code: 'Tab' }), 'darwin', true)).toBeNull()
  })
  it('plain keys and other chords are not taken', () => {
    expect(wsKeyAction(k({ key: '1', code: 'Digit1' }), 'darwin', false)).toBeNull()
    expect(wsKeyAction(k({ ctrlKey: true, key: 'c', code: 'KeyC' }), 'darwin', false)).toBeNull()
    expect(wsKeyAction(k({ key: 'Escape', code: 'Escape' }), 'darwin', false)).toBeNull()
  })
  it('finds a terminal by its container', () => {
    document.body.innerHTML = '<div class="xterm"><textarea id="a"></textarea></div><div data-terminal><input id="b"></div><input id="c">'
    expect(inTerminal(document.getElementById('a'))).toBe(true)
    expect(inTerminal(document.getElementById('b'))).toBe(true)
    expect(inTerminal(document.getElementById('c'))).toBe(false)
    expect(inTerminal(null)).toBe(false)
    document.body.innerHTML = ''
  })
})
