import { useEffect } from 'react'
import { Events, Window } from '@wailsio/runtime'
import { useMocks } from '../api/env'
import { guessPlatform, hasModifier, isMac } from '../lib/platform'
import { setRoute } from '../lib/route'
import type { Route } from '../lib/route'

/** Window event the pages listen to for commands only they can run (editor, palette, terminal). */
export const COMMAND_EVENT = 'codepass:command'

export type Layout = 'management' | 'workspace'

// Hooks the editor and the workspace shell register later (A1-xx / C3-xx). Defaults: no tabs, management layout.
let tabsHook: () => number = () => 0
let layoutHook: () => Layout = () => 'management'

/** The editor reports how many tabs are open; Cmd+W closes a tab while this is above zero. */
export function registerEditorTabsHook(fn: () => number): () => void {
  tabsHook = fn
  return () => {
    if (tabsHook === fn) tabsHook = () => 0
  }
}
export function registerLayoutHook(fn: () => Layout): () => void {
  layoutHook = fn
  return () => {
    if (layoutHook === fn) layoutHook = () => 'management'
  }
}

let terminalFocusHook: () => boolean = () => false

/** The terminal (A2-07) may register a check for "a terminal has focus"; the .xterm test below works without it. */
export function registerTerminalFocusHook(fn: () => boolean): () => void {
  terminalFocusHook = fn
  return () => {
    if (terminalFocusHook === fn) terminalFocusHook = () => false
  }
}

/** True while the key event comes from a terminal (design 4.3: the app then takes very few keys). */
export function inTerminal(target: EventTarget | null): boolean {
  if (terminalFocusHook()) return true
  return target instanceof Element && target.closest('.xterm') !== null
}

export type CommandAction =
  | { kind: 'route'; route: Route }
  | { kind: 'close-window' }
  | { kind: 'dispatch'; id: string }
  | { kind: 'none' }

const ROUTES: Record<string, Route> = {
  settings: { page: 'settings' },
  'check-updates': { page: 'update' },
  'nav-projects': { page: 'projects' },
  'nav-accounts': { page: 'accounts' },
  tutorial: { page: 'tutorial' },
}

/**
 * What a menu command does. Cmd+W (close-tab) never closes the window while a tab is open; with no tab it does
 * nothing in the workspace layout and closes the window in the management layout (design 4.4).
 */
export function resolveCommand(id: string, tabs: number, layout: Layout): CommandAction {
  if (id === 'close-tab') {
    if (tabs > 0) return { kind: 'dispatch', id }
    return layout === 'management' ? { kind: 'close-window' } : { kind: 'none' }
  }
  if (id === 'close-window') return { kind: 'close-window' }
  const route = ROUTES[id]
  if (route) return { kind: 'route', route }
  return { kind: 'dispatch', id }
}

/** Windows has no menu bar, so these shortcuts are handled here. Digits, Ctrl+, and Ctrl+F (focus search) stay in App.tsx. */
const WINDOWS_KEYS: Record<string, string> = {
  t: 'new-task', p: 'quick-open', s: 'save', w: 'close-tab', j: 'switch-session', '`': 'toggle-terminal',
}

/**
 * The command a Windows shortcut runs. While a terminal has focus the app takes only Ctrl+` and the session
 * switcher (Ctrl+Shift+J); Ctrl+1..4 live in App.tsx. Everything else (Ctrl+W delete word, Ctrl+R history,
 * Ctrl+P, Ctrl+F, Ctrl+S, Ctrl+J line feed) belongs to the process.
 */
export function commandForKey(e: Pick<KeyboardEvent, 'key' | 'shiftKey'>, terminal = false): string | null {
  const k = e.key.toLowerCase()
  if (terminal) {
    if (k === '`') return 'toggle-terminal'
    return e.shiftKey && k === 'j' ? 'switch-session' : null
  }
  if (e.shiftKey) return k === 'w' ? 'close-window' : k === 'f' ? 'find-in-files' : k === 'j' ? 'switch-session' : null
  return WINDOWS_KEYS[k] ?? null
}

/** True for the keys that would reload the page: Cmd/Ctrl+R (with or without Shift) and F5. */
export function isReloadKey(e: Pick<KeyboardEvent, 'key' | 'metaKey' | 'ctrlKey'>): boolean {
  const k = e.key.toLowerCase()
  return k === 'f5' || ((e.metaKey || e.ctrlKey) && k === 'r')
}

export function runCommand(id: string): void {
  const a = resolveCommand(id, tabsHook(), layoutHook())
  switch (a.kind) {
    case 'route':
      setRoute(a.route)
      break
    case 'close-window':
      if (!useMocks) void Window.Close() // the backend's close hook decides: hide or quit flow
      break
    case 'dispatch':
      window.dispatchEvent(new CustomEvent(COMMAND_EVENT, { detail: { id: a.id } }))
      break
  }
}

/** Mount once inside the app. Renders nothing. */
export function AppCommands() {
  useEffect(() => {
    // Menu clicks arrive as "app:command" events (macOS menu, tray, second window).
    const off = useMocks ? () => {} : Events.On('app:command', (ev: { data: unknown }) => {
      if (typeof ev.data === 'string') runCommand(ev.data)
    })
    const platform = guessPlatform()
    const onKey = (e: KeyboardEvent) => {
      // The page is never reloaded by a shortcut (the key still reaches a terminal: no stopPropagation).
      // Cmd+R rescans where a page registered a handler (App.tsx).
      if (isReloadKey(e)) {
        e.preventDefault()
        return
      }
      if (isMac(platform) || !hasModifier(e, platform) || e.altKey) return // macOS: the menu does this
      const id = commandForKey(e, inTerminal(e.target))
      if (id) {
        e.preventDefault()
        runCommand(id)
      }
    }
    window.addEventListener('keydown', onKey, true)
    return () => {
      off()
      window.removeEventListener('keydown', onKey, true)
    }
  }, [])
  return null
}
