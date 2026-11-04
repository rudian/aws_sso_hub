import { createContext, useCallback, useContext, useMemo, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { LoginMode } from '../api/models'
import type { LoginResult } from '../api/models'
import { LoginModal } from '../pages/accounts/LoginModal'
import { setRoute } from '../lib/route'
import type { Route } from '../lib/route'
import { ConfirmHost } from './ConfirmHost'
import type { PendingConfirm } from './ConfirmHost'
import { ToastRegion } from './ToastRegion'
import type { ToastEntry } from './ToastRegion'

export interface ToastInput {
  kind: 'ok' | 'warn' | 'danger' | 'info'
  title: string
  body?: string
  action?: { label: string; onClick: () => void }
}
export interface ConfirmInput {
  title: string
  body: ReactNode
  confirmLabel: string
  danger?: boolean
  cancelLabel?: string
}
export interface AppActions {
  openLogin(portal: string, opts?: { mode?: LoginMode }): Promise<LoginResult | null>
  toast(t: ToastInput): void
  confirm(c: ConfirmInput): Promise<boolean>
  navigate(r: Route): void
}

const Ctx = createContext<AppActions | null>(null)

interface LoginHost {
  portal: string
  mode: LoginMode
  resolve: (r: LoginResult | null) => void
}

/** Owns the toast list, the confirm modal and the login modal host. */
export function ActionsProvider({ children }: { children?: ReactNode }) {
  const [toasts, setToasts] = useState<ToastEntry[]>([])
  const [confirmPending, setConfirmPending] = useState<PendingConfirm | null>(null)
  const [login, setLogin] = useState<LoginHost | null>(null)
  const nextId = useRef(1)
  const loginRef = useRef<LoginHost | null>(null)
  loginRef.current = login

  const toast = useCallback((t: ToastInput) => {
    setToasts((list) => [...list, { ...t, id: nextId.current++ }])
  }, [])
  const dismiss = useCallback((id: number) => setToasts((list) => list.filter((t) => t.id !== id)), [])

  const confirm = useCallback(
    (input: ConfirmInput) =>
      new Promise<boolean>((resolve) => {
        setConfirmPending({
          input,
          resolve: (ok) => {
            setConfirmPending(null)
            resolve(ok)
          },
        })
      }),
    [],
  )

  const openLogin = useCallback(
    (portal: string, opts?: { mode?: LoginMode }) =>
      new Promise<LoginResult | null>((resolve) => {
        // One login modal at a time: a second request for the same portal returns null immediately.
        if (loginRef.current) {
          resolve(null)
          return
        }
        setLogin({ portal, mode: opts?.mode ?? LoginMode.LoginModeBrowser, resolve })
      }),
    [],
  )

  const value = useMemo<AppActions>(() => ({ openLogin, toast, confirm, navigate: setRoute }), [openLogin, toast, confirm])

  return (
    <Ctx.Provider value={value}>
      {children}
      {login && (
        <LoginModal
          portal={login.portal}
          initialMode={login.mode}
          onClose={(result) => {
            login.resolve(result)
            setLogin(null)
          }}
        />
      )}
      <ConfirmHost pending={confirmPending} />
      <ToastRegion toasts={toasts} onDismiss={dismiss} />
    </Ctx.Provider>
  )
}

export function useActions(): AppActions {
  const v = useContext(Ctx)
  if (!v) throw new Error('useActions must be used inside <ActionsProvider>')
  return v
}
