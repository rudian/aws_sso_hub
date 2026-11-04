import { useMocks } from '../api/env'
import { initialUI } from './initialUI'

export type Platform = 'darwin' | 'windows'

/** The platform the UI should assume before AppInfo has loaded (mock: ?platform=, real: navigator). */
export function guessPlatform(): Platform {
  if (useMocks) return initialUI.platform
  return typeof navigator !== 'undefined' && /win/i.test(navigator.platform ?? '') ? 'windows' : 'darwin'
}

export function isMac(platform: Platform = guessPlatform()): boolean {
  return platform === 'darwin'
}

/** True when the platform's primary modifier (Cmd on macOS, Ctrl on Windows) is held. */
export function hasModifier(e: KeyboardEvent, platform: Platform = guessPlatform()): boolean {
  return isMac(platform) ? e.metaKey && !e.ctrlKey : e.ctrlKey && !e.metaKey
}
