import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Identity } from '../api/services'
import { onEvent } from '../api/events'
import type { Author, IdentityOverview } from '../api/models'

/** What the Projects page needs from the identity service: the author identity each project sets (design 5.2). */
export interface ProjectIdentities {
  /** Project ID -> its own identity; a project that follows the global one is absent. */
  byProject: ReadonlyMap<string, Author>
  loaded: boolean
  reload(): Promise<void>
}

export function identitiesOf(o: IdentityOverview | null): Map<string, Author> {
  const m = new Map<string, Author>()
  for (const p of o?.projects ?? []) if (p.identity) m.set(p.id, p.identity)
  return m
}

/**
 * Loads the identity overview once the page has drawn, and again after every 'identity:changed'. A failure leaves the
 * map empty: the header line is an extra, never a reason to block the page.
 */
export function useProjectIdentities(): ProjectIdentities {
  const [overview, setOverview] = useState<IdentityOverview | null>(null)
  const [loaded, setLoaded] = useState(false)
  const alive = useRef(true)

  // Reloads coalesce: one runs at a time, and any number of requests during it become one trailing run.
  const inflight = useRef<Promise<void> | null>(null)
  const dirty = useRef(false)
  const reload = useCallback((): Promise<void> => {
    if (inflight.current) {
      dirty.current = true
      return inflight.current
    }
    const run = (async () => {
      do {
        dirty.current = false
        try {
          const o = await Identity.GetOverview()
          if (alive.current) setOverview(o)
        } catch {
          // keep the last value
        } finally {
          if (alive.current) setLoaded(true)
        }
      } while (dirty.current && alive.current)
    })().finally(() => {
      inflight.current = null
    })
    inflight.current = run
    return run
  }, [])

  useEffect(() => {
    alive.current = true
    void reload()
    const off = onEvent('identity:changed', () => void reload())
    return () => {
      alive.current = false
      off()
    }
  }, [reload])

  const byProject = useMemo(() => identitiesOf(overview), [overview])
  return useMemo(() => ({ byProject, loaded, reload }), [byProject, loaded, reload])
}
