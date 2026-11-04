import { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import { System } from '../api/services'
import { onEvent } from '../api/events'
import type { Settings, SettingsPatch } from '../api/models'

interface SettingsValue {
  settings: Settings | null
  /** Rejects with the backend error (use toUiError); the settings:changed event updates `settings`. */
  update(p: SettingsPatch): Promise<void>
}

const Ctx = createContext<SettingsValue>({ settings: null, update: async () => undefined })

export function SettingsProvider({ children }: { children?: ReactNode }) {
  const [settings, setSettings] = useState<Settings | null>(null)
  useEffect(() => {
    let alive = true
    System.GetSettings().then(
      (v) => alive && setSettings(v),
      () => undefined,
    )
    const off = onEvent('settings:changed', (s) => setSettings(s))
    return () => {
      alive = false
      off()
    }
  }, [])
  const update = useCallback(async (p: SettingsPatch) => {
    const next = await System.UpdateSettings(p)
    setSettings(next)
  }, [])
  const value = useMemo(() => ({ settings, update }), [settings, update])
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>
}

export function useSettings(): SettingsValue {
  return useContext(Ctx)
}
