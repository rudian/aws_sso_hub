import { createContext, useContext, useEffect, useState } from 'react'
import type { ReactNode } from 'react'
import { System } from '../api/services'
import type { AppInfo } from '../api/models'

const Ctx = createContext<AppInfo | null>(null)

export function AppInfoProvider({ children }: { children?: ReactNode }) {
  const [info, setInfo] = useState<AppInfo | null>(null)
  useEffect(() => {
    let alive = true
    System.GetAppInfo().then(
      (v) => alive && setInfo(v),
      () => undefined,
    )
    return () => {
      alive = false
    }
  }, [])
  return <Ctx.Provider value={info}>{children}</Ctx.Provider>
}

/** null until the first GetAppInfo call has returned. */
export function useAppInfo(): AppInfo | null {
  return useContext(Ctx)
}
