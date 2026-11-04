import { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import { onEvent } from '../api/events'

interface ExternalValue {
  /** Kinds ("aws-config" | "aws-credentials" | "git-global") changed outside the app and not reloaded yet. */
  kinds: string[]
  dismiss(kind: string): void
}

const Ctx = createContext<ExternalValue>({ kinds: [], dismiss: () => undefined })

export function ExternalChangeProvider({ children }: { children?: ReactNode }) {
  const [kinds, setKinds] = useState<string[]>([])
  useEffect(() => onEvent('files:external-change', (c) => setKinds((k) => (k.includes(c.kind) ? k : [...k, c.kind]))), [])
  const dismiss = useCallback((kind: string) => setKinds((k) => k.filter((x) => x !== kind)), [])
  const value = useMemo(() => ({ kinds, dismiss }), [kinds, dismiss])
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>
}

export function useExternalChanges(): ExternalValue {
  return useContext(Ctx)
}
