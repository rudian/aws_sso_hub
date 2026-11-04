import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { Repos } from '../api/services'
import { onEvent } from '../api/events'
import { toUiError } from '../api/errors'
import type { UiError } from '../api/errors'
import type { ReposState, ScanProgress } from '../api/models'
import { useEnv } from './env'

interface ReposValue {
  state: ReposState | null
  loading: boolean
  error: UiError | null
  reload(): Promise<void>
  scan(): Promise<void>
  /** Latest scan-progress event while a scan runs, otherwise null. */
  scanning: ScanProgress | null
}

const Ctx = createContext<ReposValue>({ state: null, loading: true, error: null, reload: async () => undefined, scan: async () => undefined, scanning: null })

export function ReposProvider({ children }: { children?: ReactNode }) {
  const [state, setState] = useState<ReposState | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<UiError | null>(null)
  const [scanning, setScanning] = useState<ScanProgress | null>(null)

  const reload = useCallback(async () => {
    try {
      setState(await Repos.GetState())
      setError(null)
    } catch (e) {
      setError(toUiError(e))
    } finally {
      setLoading(false)
    }
  }, [])

  const scan = useCallback(async () => {
    setScanning({ root: '', found: 0, done: false })
    try {
      setState(await Repos.Scan())
      setError(null)
    } catch (e) {
      setError(toUiError(e))
    } finally {
      setScanning(null)
    }
  }, [])

  useEffect(() => {
    void reload()
    const offs = [
      onEvent('repos:changed', (s) => setState(s)),
      onEvent('repos:scan-progress', (p) => setScanning(p.done ? null : p)),
    ]
    return () => offs.forEach((f) => f())
  }, [reload])

  // The repositories view depends on git and the AWS CLI. After a manual environment re-check (Health, first run,
  // tool paths in Settings) fetch it again so a tool that was just installed or fixed shows up without a restart.
  const { report } = useEnv()
  const lastChecked = useRef<string | undefined>(undefined)
  useEffect(() => {
    const at = report?.checkedAt
    if (!at) return
    const first = lastChecked.current === undefined
    const changed = lastChecked.current !== at
    lastChecked.current = at
    if (!first && changed) void reload()
  }, [report?.checkedAt, reload])

  const value = useMemo(() => ({ state, loading, error, reload, scan, scanning }), [state, loading, error, reload, scan, scanning])
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>
}

export function useRepos(): ReposValue {
  return useContext(Ctx)
}
