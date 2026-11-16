import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from 'react'
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
  /**
   * A login asked for from outside the window (a notification button, the tray): opens the login modal, or, when one is
   * already open for another portal, keeps the request and opens it when that modal closes. 'running' = the open modal
   * is for the same portal, so there is nothing to add.
   */
  requestLogin(portal: string): 'opened' | 'queued' | 'running'
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

  // Portals waiting behind the open login modal, in the order asked (no duplicates)
  const pendingLogin = useRef<string[]>([])

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

  const requestLogin = useCallback(
    (portal: string): 'opened' | 'queued' | 'running' => {
      const cur = loginRef.current
      if (!cur) {
        void openLogin(portal)
        return 'opened'
      }
      if (cur.portal === portal) return 'running'
      if (!pendingLogin.current.includes(portal)) pendingLogin.current.push(portal)
      return 'queued'
    },
    [openLogin],
  )

  // The login modal closed and another portal was waiting behind it: open that one now.
  useEffect(() => {
    if (login) return
    const next = pendingLogin.current.shift()
    if (next) void openLogin(next)
  }, [login, openLogin])

  const value = useMemo<AppActions>(() => ({ openLogin, requestLogin, toast, confirm, navigate: setRoute }), [openLogin, requestLogin, toast, confirm])

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
