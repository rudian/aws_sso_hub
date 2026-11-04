import clsx from 'clsx'
import { X } from 'lucide-react'
import { useEffect, useId, useRef } from 'react'
import type { ReactNode, RefObject } from 'react'
import { createPortal } from 'react-dom'
import { useI18n } from '../i18n'
import { IconButton } from './IconButton'

export interface ModalProps {
  open: boolean
  onClose: () => void
  size?: 'sm' | 'lg'
  title: ReactNode
  subtitle?: ReactNode
  footer?: ReactNode
  /** false blocks the backdrop click and Esc (flows in progress). Default true. */
  dismissible?: boolean
  /** When set, the header close button is always shown and calls this instead of onClose (flows that ask before closing). */
  onRequestClose?: () => void
  /** Element to focus on open (default: first focusable element). */
  initialFocus?: RefObject<HTMLElement | null>
  className?: string
  children?: ReactNode
}

// Esc / Tab handling is done for the topmost modal only.
const stack: symbol[] = []

const FOCUSABLE = 'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'

/** Dialog with focus trap, Esc to close, focus return to the opener, and a backdrop that ignores clicks while `dismissible` is false. */
export function Modal({ open, onClose, size, title, subtitle, footer, dismissible = true, onRequestClose, initialFocus, className, children }: ModalProps) {
  const { t } = useI18n()
  const ref = useRef<HTMLDivElement>(null)
  const titleId = useId()
  const closeRef = useRef(onClose)
  closeRef.current = onClose
  const dismissRef = useRef(dismissible)
  dismissRef.current = dismissible

  useEffect(() => {
    if (!open) return
    const me = Symbol('modal')
    stack.push(me)
    const opener = document.activeElement as HTMLElement | null
    const focusables = () => Array.from(ref.current?.querySelectorAll<HTMLElement>(FOCUSABLE) ?? []).filter((el) => el.offsetParent !== null || el === document.activeElement)
    const t0 = window.setTimeout(() => {
      const target = initialFocus?.current ?? focusables().find((el) => !el.classList.contains('icon-btn')) ?? focusables()[0] ?? ref.current
      target?.focus()
    }, 0)
    const onKey = (e: KeyboardEvent) => {
      if (stack[stack.length - 1] !== me) return
      if (e.key === 'Escape') {
        e.stopPropagation()
        if (dismissRef.current) {
          e.preventDefault()
          closeRef.current()
        }
      } else if (e.key === 'Tab') {
        const f = focusables()
        if (f.length === 0) {
          e.preventDefault()
          return
        }
        const first = f[0]
        const last = f[f.length - 1]
        const active = document.activeElement
        if (!ref.current?.contains(active)) {
          e.preventDefault()
          first.focus()
        } else if (e.shiftKey && active === first) {
          e.preventDefault()
          last.focus()
        } else if (!e.shiftKey && active === last) {
          e.preventDefault()
          first.focus()
        }
      }
    }
    window.addEventListener('keydown', onKey, true)
    return () => {
      window.clearTimeout(t0)
      window.removeEventListener('keydown', onKey, true)
      const i = stack.indexOf(me)
      if (i >= 0) stack.splice(i, 1)
      if (opener && document.contains(opener)) opener.focus()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open])

  if (!open) return null
  return createPortal(
    <div
      className="modal-backdrop"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget && dismissRef.current) closeRef.current()
      }}
    >
      <div ref={ref} className={clsx('modal', size && `modal--${size}`, className)} role="dialog" aria-modal="true" aria-labelledby={titleId} tabIndex={-1}>
        <div className="modal__header">
          <div style={{ flex: 1, minWidth: 0 }}>
            <h2 className="modal__title" id={titleId}>
              {title}
            </h2>
            {subtitle && <div className="muted" style={{ fontSize: 'var(--fs-12)', marginTop: 2 }}>{subtitle}</div>}
          </div>
          {(dismissible || onRequestClose) && <IconButton label={t('common.close')} icon={X} size="sm" onClick={onRequestClose ?? onClose} />}
        </div>
        <div className="modal__body">{children}</div>
        {footer && <div className="modal__footer">{footer}</div>}
      </div>
    </div>,
    document.body,
  )
}
