import type { Platform } from '../lib/platform'

export interface KeyLike {
  type: string
  key: string
  shiftKey: boolean
  ctrlKey: boolean
  metaKey: boolean
  altKey: boolean
  /** IME composition in progress */
  isComposing?: boolean
  /** 229 while an IME owns the key (WKWebView) */
  keyCode?: number
}

/**
 * What the terminal does with a key event:
 * - send: write these bytes to the child, and tell xterm to skip the event
 * - copy / search: app actions on the terminal itself
 * - paste: let the browser's paste event through (Windows Ctrl+V / Ctrl+Shift+V); the paste handler confirms and sanitizes
 * - app: leave the event to the app's shortcuts (xterm skips it, the page still sees it)
 * - swallow: xterm skips it, nothing else happens (the key-up and key-press of a key already handled)
 * - native: xterm handles it as usual
 */
export type KeyAction =
  | { kind: 'send'; data: string }
  | { kind: 'copy' }
  | { kind: 'search' }
  | { kind: 'paste' }
  | { kind: 'app' }
  | { kind: 'swallow' }
  | { kind: 'native' }

/** Shift+Enter sends ESC CR: a new line in tools that support multi-line input (Claude Code). */
export const SHIFT_ENTER = '\x1b\r'

export function mapKey(e: KeyLike, ctx: { platform: Platform; hasSelection: boolean }): KeyAction {
  const mac = ctx.platform === 'darwin'
  const down = e.type === 'keydown'
  const k = e.key

  // An IME owns the key: xterm's composition handling must see it, or the new line lands before the composed text.
  if (e.isComposing || e.keyCode === 229) return { kind: 'native' }

  if (k === 'Enter' && e.shiftKey && !e.ctrlKey && !e.metaKey && !e.altKey) {
    return down ? { kind: 'send', data: SHIFT_ENTER } : { kind: 'swallow' }
  }
  if (!down) return { kind: 'native' }

  const mod = mac ? e.metaKey && !e.ctrlKey : e.ctrlKey && !e.metaKey
  const lower = k.length === 1 ? k.toLowerCase() : k

  // Keys that belong to the app while a terminal has focus (ui-design 5.7b): Ctrl+`, Cmd/Ctrl+1..5, quick capture,
  // the session switcher, and on macOS only Open repository.
  if (k === '`' && e.ctrlKey && !e.metaKey && !e.altKey) return { kind: 'app' }
  if (mod && !e.altKey && !e.shiftKey && /^[1-5]$/.test(k)) return { kind: 'app' }
  if (mod && e.shiftKey && !e.altKey && lower === 'n') return { kind: 'app' }
  if (mac && e.metaKey && !e.ctrlKey && !e.shiftKey && !e.altKey && (lower === 'j' || lower === 'o')) return { kind: 'app' }
  if (!mac && e.ctrlKey && e.shiftKey && !e.metaKey && !e.altKey && lower === 'j') return { kind: 'app' }

  // Copy: Cmd+C on macOS and Ctrl+Shift+C on Windows always copy a selection; Ctrl+C on Windows only when something is
  // selected, otherwise it is the interrupt and goes to the process.
  if (lower === 'c' && !e.altKey) {
    if (mac && e.metaKey && !e.ctrlKey && !e.shiftKey) return ctx.hasSelection ? { kind: 'copy' } : { kind: 'swallow' }
    if (!mac && e.ctrlKey && !e.metaKey) {
      if (e.shiftKey) return ctx.hasSelection ? { kind: 'copy' } : { kind: 'swallow' }
      if (ctx.hasSelection) return { kind: 'copy' }
    }
  }
  // Windows paste: Ctrl+V and Ctrl+Shift+V paste (like Windows Terminal) instead of sending ^V to the program.
  if (!mac && lower === 'v' && e.ctrlKey && !e.metaKey && !e.altKey) return { kind: 'paste' }
  if (lower === 'f' && mod && !e.shiftKey && !e.altKey) return { kind: 'search' }
  return { kind: 'native' }
}
