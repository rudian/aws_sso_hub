import { cloneElement, useCallback, useEffect, useId, useState } from 'react'
import type { ReactElement, ReactNode } from 'react'
import { createPortal } from 'react-dom'

interface ChildProps {
  onMouseEnter?: (e: React.MouseEvent<HTMLElement>) => void
  onMouseLeave?: (e: React.MouseEvent<HTMLElement>) => void
  onFocus?: (e: React.FocusEvent<HTMLElement>) => void
  onBlur?: (e: React.FocusEvent<HTMLElement>) => void
  'aria-describedby'?: string
}

/** Hover/focus tooltip for a single child element. Rendered in a portal, so it is never clipped by overflow. */
export function Tooltip({ content, children }: { content: ReactNode; children: ReactElement<ChildProps> }) {
  const id = useId()
  const [pos, setPos] = useState<{ x: number; y: number } | null>(null)
  const show = useCallback((el: HTMLElement) => {
    const r = el.getBoundingClientRect()
    setPos({ x: r.left + r.width / 2, y: r.bottom + 6 })
  }, [])
  useEffect(() => {
    if (!pos) return
    const hide = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setPos(null)
    }
    window.addEventListener('keydown', hide)
    return () => window.removeEventListener('keydown', hide)
  }, [pos])

  if (content === undefined || content === null || content === '' || content === false) return children
  const p = children.props
  return (
    <>
      {cloneElement(children, {
        onMouseEnter: (e: React.MouseEvent<HTMLElement>) => {
          p.onMouseEnter?.(e)
          show(e.currentTarget)
        },
        onMouseLeave: (e: React.MouseEvent<HTMLElement>) => {
          p.onMouseLeave?.(e)
          setPos(null)
        },
        onFocus: (e: React.FocusEvent<HTMLElement>) => {
          p.onFocus?.(e)
          if (e.currentTarget.matches(':focus-visible')) show(e.currentTarget)
        },
        onBlur: (e: React.FocusEvent<HTMLElement>) => {
          p.onBlur?.(e)
          setPos(null)
        },
        'aria-describedby': pos ? id : p['aria-describedby'],
      })}
      {pos &&
        createPortal(
          <div role="tooltip" id={id} className="tooltip" style={{ position: 'fixed', left: pos.x, top: pos.y, transform: 'translateX(-50%)' }}>
            {content}
          </div>,
          document.body,
        )}
    </>
  )
}
