import clsx from 'clsx'
import { CircleCheck, Info, OctagonAlert, TriangleAlert, X } from 'lucide-react'
import { useEffect, useRef } from 'react'
import { IconButton } from '../components/IconButton'
import { useI18n } from '../i18n'
import type { ToastInput } from './AppActions'

export interface ToastEntry extends ToastInput {
  id: number
}

const ICONS = { ok: CircleCheck, info: Info, warn: TriangleAlert, danger: OctagonAlert }
const DELAY: Record<ToastInput['kind'], number | null> = { ok: 5000, info: 5000, warn: 8000, danger: null }

function ToastItem({ toast, onDismiss }: { toast: ToastEntry; onDismiss: (id: number) => void }) {
  const { t } = useI18n()
  const remaining = useRef<number | null>(DELAY[toast.kind])
  const startedAt = useRef(0)
  const timer = useRef<number | undefined>(undefined)

  const start = () => {
    if (remaining.current === null) return
    startedAt.current = Date.now()
    timer.current = window.setTimeout(() => onDismiss(toast.id), remaining.current)
  }
  const pause = () => {
    if (remaining.current === null) return
    window.clearTimeout(timer.current)
    remaining.current = Math.max(500, remaining.current - (Date.now() - startedAt.current))
  }
  useEffect(() => {
    start()
    return () => window.clearTimeout(timer.current)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const Icon = ICONS[toast.kind]
  return (
    <div className={clsx('toast', `toast--${toast.kind}`)} role={toast.kind === 'danger' ? 'alert' : 'status'} onMouseEnter={pause} onMouseLeave={start}>
      <Icon className="toast__icon" aria-hidden="true" />
      <div className="toast__body">
        <div className="toast__title">{toast.title}</div>
        {toast.body && <div>{toast.body}</div>}
        {toast.action && (
          <button
            type="button"
            className="link"
            style={{ marginTop: 4, background: 'none', border: 0, padding: 0, font: 'inherit', fontWeight: 500 }}
            onClick={() => {
              toast.action?.onClick()
              onDismiss(toast.id)
            }}
          >
            {toast.action.label}
          </button>
        )}
      </div>
      <IconButton label={t('common.close')} icon={X} size="sm" className="toast__close" onClick={() => onDismiss(toast.id)} />
    </div>
  )
}

/** Bottom-right, 360 px, newest at the bottom, max 3 visible. */
export function ToastRegion({ toasts, onDismiss }: { toasts: ToastEntry[]; onDismiss: (id: number) => void }) {
  const visible = toasts.slice(-3)
  return (
    <div className="toast-region">
      {visible.map((t) => (
        <ToastItem key={t.id} toast={t} onDismiss={onDismiss} />
      ))}
    </div>
  )
}
