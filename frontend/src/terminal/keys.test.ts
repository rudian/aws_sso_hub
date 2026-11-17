import { describe, expect, it } from 'vitest'
import { mapKey, SHIFT_ENTER, type KeyLike } from './keys'

const k = (key: string, o: Partial<KeyLike> = {}): KeyLike => ({ type: 'keydown', key, shiftKey: false, ctrlKey: false, metaKey: false, altKey: false, ...o })
const mac = { platform: 'darwin' as const, hasSelection: false }
const win = { platform: 'windows' as const, hasSelection: false }

describe('mapKey', () => {
  it('Shift+Enter sends ESC CR, and its key-up is swallowed', () => {
    expect(mapKey(k('Enter', { shiftKey: true }), mac)).toEqual({ kind: 'send', data: SHIFT_ENTER })
    expect(SHIFT_ENTER).toBe('\x1b\r')
    expect(mapKey(k('Enter', { shiftKey: true, type: 'keyup' }), mac)).toEqual({ kind: 'swallow' })
  })
  it('leaves keys to the IME while composing', () => {
    expect(mapKey(k('Enter', { shiftKey: true, keyCode: 229 }), mac)).toEqual({ kind: 'native' })
    expect(mapKey(k('Enter', { shiftKey: true, isComposing: true }), mac)).toEqual({ kind: 'native' })
  })
  it('lets the app keep its keys', () => {
    expect(mapKey(k('1', { metaKey: true }), mac)).toEqual({ kind: 'app' })
    expect(mapKey(k('1', { ctrlKey: true }), win)).toEqual({ kind: 'app' })
  })
  it('copies a selection with Cmd+C, and Ctrl+C is the interrupt on Windows without one', () => {
    expect(mapKey(k('c', { metaKey: true }), { ...mac, hasSelection: true })).toEqual({ kind: 'copy' })
    expect(mapKey(k('c', { ctrlKey: true }), win)).toEqual({ kind: 'native' })
    expect(mapKey(k('c', { ctrlKey: true }), { ...win, hasSelection: true })).toEqual({ kind: 'copy' })
  })
  it('searches with the primary modifier + F', () => {
    expect(mapKey(k('f', { metaKey: true }), mac)).toEqual({ kind: 'search' })
    expect(mapKey(k('f', { ctrlKey: true }), win)).toEqual({ kind: 'search' })
  })
  it('pastes with Ctrl+V and Ctrl+Shift+V on Windows only', () => {
    expect(mapKey(k('v', { ctrlKey: true }), win)).toEqual({ kind: 'paste' })
    expect(mapKey(k('V', { ctrlKey: true, shiftKey: true }), win)).toEqual({ kind: 'paste' })
    expect(mapKey(k('v', { ctrlKey: true }), mac)).toEqual({ kind: 'native' })
  })
})
