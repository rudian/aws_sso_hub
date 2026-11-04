import { useCallback, useEffect, useRef, useState } from 'react'
import { toUiError } from '../api/errors'
import type { UiError } from '../api/errors'

/** Run `fn` on mount and whenever `deps` change. Stale results are dropped. `reload` re-runs without clearing the data. */
export function useAsync<T>(fn: () => Promise<T>, deps: unknown[]): { data: T | null; error: UiError | null; loading: boolean; reload: () => Promise<void> } {
  const [data, setData] = useState<T | null>(null)
  const [error, setError] = useState<UiError | null>(null)
  const [loading, setLoading] = useState(true)
  const seq = useRef(0)
  const fnRef = useRef(fn)
  fnRef.current = fn

  const run = useCallback(async () => {
    const my = ++seq.current
    setLoading(true)
    try {
      const v = await fnRef.current()
      if (my !== seq.current) return
      setData(v)
      setError(null)
    } catch (e) {
      if (my !== seq.current) return
      setError(toUiError(e))
    } finally {
      if (my === seq.current) setLoading(false)
    }
  }, [])

  useEffect(() => {
    void run()
    return () => {
      seq.current++
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, deps)

  return { data, error, loading, reload: run }
}
