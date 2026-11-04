import { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import { System } from '../api/services'
import type { EnvReport } from '../api/models'

interface EnvValue {
  report: EnvReport | null
  recheck(): Promise<void>
}

const Ctx = createContext<EnvValue>({ report: null, recheck: async () => undefined })

export function EnvProvider({ children }: { children?: ReactNode }) {
  const [report, setReport] = useState<EnvReport | null>(null)
  const recheck = useCallback(async () => {
    try {
      setReport(await System.CheckEnvironment())
    } catch {
      /* keep the previous report */
    }
  }, [])
  useEffect(() => {
    void recheck()
  }, [recheck])
  const value = useMemo(() => ({ report, recheck }), [report, recheck])
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>
}

export function useEnv(): EnvValue {
  return useContext(Ctx)
}
