import { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import { Accounts } from '../api/services'
import { onEvent } from '../api/events'
import { toUiError } from '../api/errors'
import type { UiError } from '../api/errors'
import type { AccountsState, SessionStatus } from '../api/models'

interface AccountsValue {
  state: AccountsState | null
  loading: boolean
  error: UiError | null
  reload(): Promise<void>
}

const Ctx = createContext<AccountsValue>({ state: null, loading: true, error: null, reload: async () => undefined })

function mergeSession(s: AccountsState | null, st: SessionStatus): AccountsState | null {
  if (!s) return s
  return { ...s, portals: (s.portals ?? []).map((p) => (p.name === st.portal ? { ...p, session: st } : p)) }
}

export function AccountsProvider({ children }: { children?: ReactNode }) {
  const [state, setState] = useState<AccountsState | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<UiError | null>(null)

  const reload = useCallback(async () => {
    try {
      const s = await Accounts.Reload()
      setState(s)
      setError(null)
    } catch (e) {
      setError(toUiError(e))
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    let alive = true
    Accounts.GetState().then(
      (s) => {
        if (!alive) return
        setState((cur) => cur ?? s)
        setError(null)
        setLoading(false)
      },
      (e) => {
        if (!alive) return
        setError(toUiError(e))
        setLoading(false)
      },
    )
    const offs = [
      onEvent('accounts:changed', (s) => setState(s)),
      onEvent('session:changed', (st) => setState((cur) => mergeSession(cur, st))),
      onEvent('session:renewed', (st) => setState((cur) => mergeSession(cur, st))),
    ]
    return () => {
      alive = false
      offs.forEach((f) => f())
    }
  }, [])

  const value = useMemo(() => ({ state, loading, error, reload }), [state, loading, error, reload])
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>
}

export function useAccounts(): AccountsValue {
  return useContext(Ctx)
}
