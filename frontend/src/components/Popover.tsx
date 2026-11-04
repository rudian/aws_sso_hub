import clsx from 'clsx'
import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import type { ReactNode, RefObject } from 'react'
import { createPortal } from 'react-dom'

export interface PopoverProps {
  anchorRef: RefObject<HTMLElement | null>
  open: boolean
  onClose: () => void
  /** Align the popover's left (start) or right (end) edge with the anchor. */
  align?: 'start' | 'end'
  ariaLabel?: string
  className?: string
  children?: ReactNode
}

/** Floating panel under an anchor element. Closes on Esc and on outside click. Rendered in a portal. */
export function Popover({ anchorRef, open, onClose, align = 'start', ariaLabel, className, children }: PopoverProps) {
  const ref = useRef<HTMLDivElement>(null)
  const [pos, setPos] = useState<{ top: number; left: number } | null>(null)

  useLayoutEffect(() => {
    if (!open) {
      setPos(null)
      return
    }
    const a = anchorRef.current
    const p = ref.current
    if (!a) return
    const r = a.getBoundingClientRect()
    const w = p?.offsetWidth ?? 280
    let left = align === 'end' ? r.right - w : r.left
    left = Math.max(8, Math.min(left, window.innerWidth - w - 8))
    let top = r.bottom + 6
    const h = p?.offsetHeight ?? 0
    if (h && top + h > window.innerHeight - 8) top = Math.max(8, r.top - h - 6)
    setPos({ top, left })
  }, [open, align, anchorRef, children])

  useEffect(() => {
    if (!open) return
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.stopPropagation()
        onClose()
        anchorRef.current?.focus()
      }
    }
    const onDown = (e: MouseEvent) => {
      const t = e.target as Node
      if (ref.current?.contains(t) || anchorRef.current?.contains(t)) return
      onClose()
    }
    window.addEventListener('keydown', onKey, true)
    window.addEventListener('mousedown', onDown)
    return () => {
      window.removeEventListener('keydown', onKey, true)
      window.removeEventListener('mousedown', onDown)
    }
  }, [open, onClose, anchorRef])

  if (!open) return null
  return createPortal(
    <div ref={ref} role="dialog" aria-label={ariaLabel} className={clsx('popover', className)} style={{ position: 'fixed', top: pos?.top ?? -9999, left: pos?.left ?? -9999 }}>
      {children}
    </div>,
    document.body,
  )
}
