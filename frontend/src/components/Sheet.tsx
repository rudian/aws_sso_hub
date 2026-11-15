import clsx from 'clsx'
import { X } from 'lucide-react'
import { useEffect, useId, useRef, useState } from 'react'
import type { ReactNode, RefObject } from 'react'
import { createPortal } from 'react-dom'
import { useI18n } from '../i18n'
import { DUR, exitDelay } from '../lib/motion'
import { IconButton } from './IconButton'

export interface SheetProps {
  open: boolean
  onClose: () => void
  title: ReactNode
  subtitle?: ReactNode
  /** Small element before the title (an icon tile). */
  icon?: ReactNode
  footer?: ReactNode
  /** false blocks the backdrop click and Esc (an action in progress). Default true. */
  dismissible?: boolean
  /** Element to focus on open (default: first focusable element in the body). */
  initialFocus?: RefObject<HTMLElement | null>
  className?: string
  children?: ReactNode
}

const stack: symbol[] = []

/** Rendered and not hidden through `visibility` (closed .expand / .collapse content stays laid out). */
function visible(el: HTMLElement): boolean {
  if (el.offsetParent === null) return false
  return getComputedStyle(el).visibility !== 'hidden'
}

const FOCUSABLE = 'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'

/** A dialog or another sheet opened later sits above this sheet (same layer, later in the document). */
// Focus inside a menu or popover opened after this sheet keeps its own tab order.
function laterLayerHas(el: HTMLElement | null, node: Node | null): boolean {
  if (!el || !node) return false
  return Array.from(document.querySelectorAll('.menu, .popover')).some((o) => (el.compareDocumentPosition(o) & Node.DOCUMENT_POSITION_FOLLOWING) !== 0 && o.contains(node))
}

function coveredByLaterOverlay(el: HTMLElement | null): boolean {
  if (!el) return false
  const later = document.querySelectorAll('.modal-backdrop, .sheet')
  for (const o of Array.from(later)) {
    if (o !== el && (el.compareDocumentPosition(o) & Node.DOCUMENT_POSITION_FOLLOWING) !== 0) return true
  }
  return false
}

/** A menu or popover opened later (from inside this sheet) is closed by Esc before the sheet is. */
function menuOpenAbove(el: HTMLElement | null): boolean {
  if (!el) return false
  for (const o of Array.from(document.querySelectorAll('.menu, .popover'))) {
    if ((el.compareDocumentPosition(o) & Node.DOCUMENT_POSITION_FOLLOWING) !== 0) return true
  }
  return false
}

/**
 * Right-hand side sheet: slides in 24px with a fade (220ms), out in 160ms, backdrop fades.
 * Focus trap, Esc, focus returns to the opener. Under reduced motion it unmounts at once.
 */
export function Sheet({ open, onClose, title, subtitle, icon, footer, dismissible = true, initialFocus, className, children }: SheetProps) {
  const { t } = useI18n()
  const ref = useRef<HTMLElement>(null)
  const titleId = useId()
  const closeRef = useRef(onClose)
  closeRef.current = onClose
  const dismissRef = useRef(dismissible)
  dismissRef.current = dismissible

  // Stay mounted while the exit animation runs.
  const [mounted, setMounted] = useState(open)
  const [leaving, setLeaving] = useState(false)
  useEffect(() => {
    if (open) {
      setMounted(true)
      setLeaving(false)
      return
    }
    if (!mounted) return
    const delay = exitDelay(DUR.leave)
    if (delay === 0) {
      setMounted(false)
      return
    }
    setLeaving(true)
    const id = window.setTimeout(() => {
      setMounted(false)
      setLeaving(false)
    }, delay)
    return () => window.clearTimeout(id)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open])

  const active = open && mounted
  useEffect(() => {
    if (!active) return
    const me = Symbol('sheet')
    stack.push(me)
    const opener = document.activeElement as HTMLElement | null
    const focusables = () => Array.from(ref.current?.querySelectorAll<HTMLElement>(FOCUSABLE) ?? []).filter((el) => visible(el) || el === document.activeElement)
    const t0 = window.setTimeout(() => {
      const target = initialFocus?.current ?? focusables().find((el) => !el.classList.contains('icon-btn')) ?? focusables()[0] ?? ref.current
      target?.focus()
    }, 0)
    const onKey = (e: KeyboardEvent) => {
      if (stack[stack.length - 1] !== me || coveredByLaterOverlay(ref.current)) return
      if (e.key === 'Escape') {
        if (menuOpenAbove(ref.current)) return
        e.stopPropagation()
        if (dismissRef.current) {
          e.preventDefault()
          closeRef.current()
        }
      } else if (e.key === 'Tab') {
        if (laterLayerHas(ref.current, document.activeElement)) {
          // A popover opened from the sheet wraps its own Tab order (the host would otherwise move focus out of the page); menus close themselves on Tab.
          const pop = Array.from(document.querySelectorAll<HTMLElement>('.popover')).find((o) => (ref.current!.compareDocumentPosition(o) & Node.DOCUMENT_POSITION_FOLLOWING) !== 0 && o.contains(document.activeElement))
          if (!pop) return
          const pf = Array.from(pop.querySelectorAll<HTMLElement>(FOCUSABLE)).filter((x) => visible(x) || x === document.activeElement)
          const cur = document.activeElement
          if (pf.length === 0) e.preventDefault()
          else if (e.shiftKey && cur === pf[0]) {
            e.preventDefault()
            pf[pf.length - 1].focus()
          } else if (!e.shiftKey && cur === pf[pf.length - 1]) {
            e.preventDefault()
            pf[0].focus()
          }
          return
        }
        const f = focusables()
        if (f.length === 0) {
          e.preventDefault()
          return
        }
        const first = f[0]
        const last = f[f.length - 1]
        const cur = document.activeElement
        if (!ref.current?.contains(cur)) {
          e.preventDefault()
          first.focus()
        } else if (e.shiftKey && cur === first) {
          e.preventDefault()
          last.focus()
        } else if (!e.shiftKey && cur === last) {
          e.preventDefault()
          first.focus()
        }
      }
    }
    // Backstop: focus that lands outside this sheet (and outside a later menu/popover/dialog/sheet) comes back in.
    const onFocusIn = (e: FocusEvent) => {
      const el = ref.current
      const node = e.target as Node | null
      if (stack[stack.length - 1] !== me || !el || !node || el.contains(node)) return
      if (laterLayerHas(el, node) || coveredByLaterOverlay(el)) return
      const prev = e.relatedTarget
      if (prev instanceof HTMLElement && el.contains(prev) && visible(prev)) {
        prev.focus()
        return
      }
      const f = focusables()
      ;(f.find((x) => !x.classList.contains('icon-btn')) ?? f[0] ?? el).focus()
    }
    window.addEventListener('keydown', onKey, true)
    document.addEventListener('focusin', onFocusIn)
    return () => {
      document.removeEventListener('focusin', onFocusIn)
      window.clearTimeout(t0)
      window.removeEventListener('keydown', onKey, true)
      const i = stack.indexOf(me)
      if (i >= 0) stack.splice(i, 1)
      if (opener && document.contains(opener)) opener.focus()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [active])

  if (!mounted) return null
  const out = leaving || !open
  return createPortal(
    <>
      <div
        className={clsx('sheet-backdrop', out && 'is-leaving')}
        onMouseDown={(e) => {
          if (e.target === e.currentTarget && dismissRef.current) closeRef.current()
        }}
      />
      <aside ref={ref} className={clsx('sheet', out && 'is-leaving', className)} role="dialog" aria-modal="true" aria-labelledby={titleId} tabIndex={-1}>
        <div className="sheet__head">
          {icon}
          <div style={{ flex: 1, minWidth: 0 }}>
            <div className="sheet__title" id={titleId}>
              {title}
            </div>
            {subtitle && <div className="sheet__sub">{subtitle}</div>}
          </div>
          {dismissible && <IconButton label={t('common.close')} icon={X} size="sm" onClick={onClose} />}
        </div>
        <div className="sheet__body">{children}</div>
        {footer && <div className="sheet__foot">{footer}</div>}
      </aside>
    </>,
    document.body,
  )
}
