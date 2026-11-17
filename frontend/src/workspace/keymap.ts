// The workspace part of the keyboard map (design 4.3). Pure: the page feeds it key events.
import type { Platform } from '../lib/platform'
import { WS_TABS } from '../state/workspace'
import type { WsTab } from '../state/workspace'

export type WsKeyAction = { kind: 'tab'; tab: WsTab } | { kind: 'panel' } | { kind: 'tree'; dir: 1 | -1 }

export type KeyLike = Pick<KeyboardEvent, 'key' | 'code' | 'ctrlKey' | 'metaKey' | 'altKey' | 'shiftKey'>

/** True when the event comes from inside a terminal: only Ctrl+` (and the app-wide keys) are taken there; the rest goes to the process. */
export function inTerminal(target: EventTarget | null): boolean {
  return target instanceof Element && target.closest('.xterm, [data-terminal]') !== null
}

/**
 * Ctrl+1..6 (macOS) / Alt+1..6 (Windows): tabs. Ctrl+` : the bottom panel. Ctrl+Tab / Ctrl+Shift+Tab: the next / previous working tree.
 * Cmd/Ctrl+1..5 (pages), Cmd/Ctrl+, and the rest are the app's own and are handled in App.tsx.
 */
export function wsKeyAction(e: KeyLike, platform: Platform, terminalFocus: boolean): WsKeyAction | null {
  if (e.ctrlKey && !e.metaKey && !e.altKey && !e.shiftKey && (e.code === 'Backquote' || e.key === '`')) return { kind: 'panel' }
  if (terminalFocus) return null
  if (e.ctrlKey && !e.metaKey && !e.altKey && e.key === 'Tab') return { kind: 'tree', dir: e.shiftKey ? -1 : 1 }
  const mac = platform === 'darwin'
  const modOk = mac ? e.ctrlKey && !e.metaKey && !e.altKey : e.altKey && !e.ctrlKey && !e.metaKey
  if (modOk && !e.shiftKey) {
    // the digit row only: Alt+Numpad digits type Alt codes on Windows
    const m = /^Digit([1-6])$/.exec(e.code ?? '')
    if (m) return { kind: 'tab', tab: WS_TABS[Number(m[1]) - 1] }
  }
  return null
}
