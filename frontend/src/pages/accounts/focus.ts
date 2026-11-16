import { useEffect } from 'react'
import { animateInsert } from '../../lib/motion'

// "Open Accounts at a portal" (plan 4.15, design 6): a click on a sign-in notification shows the Accounts page
// scrolled to that portal's section, with the section highlighted once (the insert motion; none under reduced
// motion). A collapsed portal is shown open for this view only; the saved fold state is not changed.

const EVENT = 'codepass:portal-focus'
let pending: string | null = null

/** Ask the Accounts page to show a portal. Works before the page has mounted: the page takes the request when it does. */
export function requestPortalFocus(name: string): void {
  pending = name
  window.dispatchEvent(new Event(EVENT))
}

function take(): string | null {
  const v = pending
  pending = null
  return v
}

/** Called by the Accounts page: `onFocus` runs for a request made before it mounted and for every later one. */
export function usePortalFocus(onFocus: (name: string) => void): void {
  useEffect(() => {
    const run = () => {
      const name = take()
      if (name) onFocus(name)
    }
    run()
    window.addEventListener(EVENT, run)
    return () => window.removeEventListener(EVENT, run)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])
}

/** Scroll the portal's section into view and play the highlight once. Returns false when the section is not on the page. */
export function showPortalOnce(name: string): boolean {
  const el = document.querySelector<HTMLElement>(`[data-portal="${CSS.escape(name)}"]`)
  if (!el) return false
  el.scrollIntoView({ block: 'start', behavior: 'auto' })
  animateInsert(el, 1)
  return true
}
