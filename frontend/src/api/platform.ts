import { Clipboard, Window } from '@wailsio/runtime'
import { useMocks } from './env'
import { System } from './services'

/** @wailsio/runtime Clipboard.SetText, or navigator.clipboard (with a textarea fallback) in mock mode. */
export async function copyText(text: string): Promise<void> {
  if (!useMocks) {
    await Clipboard.SetText(text)
    return
  }
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(text)
      return
    }
  } catch {
    /* fall through to the textarea fallback */
  }
  const ta = document.createElement('textarea')
  ta.value = text
  ta.setAttribute('readonly', '')
  ta.style.position = 'fixed'
  ta.style.opacity = '0'
  document.body.appendChild(ta)
  ta.select()
  try {
    document.execCommand('copy')
  } finally {
    document.body.removeChild(ta)
  }
}

/** Runtime Window.SetTitle (inside Wails) + document.title. */
export async function setWindowTitle(title: string): Promise<void> {
  document.title = title
  if (!useMocks) await Window.SetTitle(title)
}

/** All external links go through SystemService.OpenURL (https only). Mock: window.open. */
export async function openExternal(url: string): Promise<void> {
  if (useMocks) {
    window.open(url, '_blank', 'noopener,noreferrer')
    return
  }
  await System.OpenURL(url)
}
