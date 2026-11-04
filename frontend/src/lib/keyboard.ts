import { useEffect, useRef } from 'react'
import type { RefObject } from 'react'

// Module-level registries so the app shell can route Cmd/Ctrl+R and Cmd/Ctrl+F to whichever page is mounted.
const refreshHandlers: Array<() => void> = []
const searchRefs: Array<RefObject<HTMLInputElement | null>> = []

/** Cmd/Ctrl+R on the current page. The most recently mounted handler wins. */
export function useRefreshHandler(fn: () => void): void {
  const ref = useRef(fn)
  ref.current = fn
  useEffect(() => {
    const h = () => ref.current()
    refreshHandlers.push(h)
    return () => {
      const i = refreshHandlers.indexOf(h)
      if (i >= 0) refreshHandlers.splice(i, 1)
    }
  }, [])
}

/** Cmd/Ctrl+F focuses this field. */
export function useSearchShortcut(ref: RefObject<HTMLInputElement | null>): void {
  useEffect(() => {
    searchRefs.push(ref)
    return () => {
      const i = searchRefs.indexOf(ref)
      if (i >= 0) searchRefs.splice(i, 1)
    }
  }, [ref])
}

export function runRefreshHandler(): boolean {
  const h = refreshHandlers[refreshHandlers.length - 1]
  if (!h) return false
  h()
  return true
}

export function focusSearch(): boolean {
  const r = searchRefs[searchRefs.length - 1]
  if (!r?.current) return false
  r.current.focus()
  r.current.select()
  return true
}
