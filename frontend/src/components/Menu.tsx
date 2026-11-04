import clsx from 'clsx'
import { cloneElement, useCallback, useEffect, useId, useLayoutEffect, useRef, useState } from 'react'
import type { KeyboardEvent as ReactKeyboardEvent, MouseEvent as ReactMouseEvent, ReactElement, ReactNode } from 'react'
import { createPortal } from 'react-dom'
import type { LucideIcon } from 'lucide-react'

export interface MenuItem {
  id: string
  label: ReactNode
  icon?: LucideIcon
  danger?: boolean
  disabled?: boolean
  separatorBefore?: boolean
  onSelect: () => void
}

interface TriggerProps {
  onClick?: (e: ReactMouseEvent<HTMLElement>) => void
  onKeyDown?: (e: ReactKeyboardEvent<HTMLElement>) => void
  ref?: React.Ref<HTMLElement>
  'aria-haspopup'?: 'menu'
  'aria-expanded'?: boolean
  'aria-controls'?: string
}

export interface MenuProps {
  /** A single element (usually an <IconButton> or <Button>) that opens the menu. It must accept ref, onClick, onKeyDown and aria-* props. */
  trigger: ReactElement<TriggerProps>
  items: MenuItem[]
  align?: 'start' | 'end'
}

/** Menu button pattern: ArrowDown/Enter/Space opens, arrows + Home/End + type-ahead move, Enter selects, Esc closes and returns focus. */
export function Menu({ trigger, items, align = 'start' }: MenuProps) {
  const [open, setOpen] = useState(false)
  const [pos, setPos] = useState<{ top: number; left: number } | null>(null)
  const triggerRef = useRef<HTMLElement | null>(null)
  const listRef = useRef<HTMLDivElement>(null)
  const id = useId()
  const typed = useRef({ text: '', at: 0 })

  const close = useCallback((returnFocus: boolean) => {
    setOpen(false)
    if (returnFocus) triggerRef.current?.focus()
  }, [])

  const buttons = () => Array.from(listRef.current?.querySelectorAll<HTMLButtonElement>('button[role="menuitem"]:not(:disabled)') ?? [])

  useLayoutEffect(() => {
    if (!open) return
    const a = triggerRef.current
    const l = listRef.current
    if (!a) return
    const r = a.getBoundingClientRect()
    const w = l?.offsetWidth ?? 200
    let left = align === 'end' ? r.right - w : r.left
    left = Math.max(8, Math.min(left, window.innerWidth - w - 8))
    let top = r.bottom + 4
    const h = l?.offsetHeight ?? 0
    if (h && top + h > window.innerHeight - 8) top = Math.max(8, r.top - h - 4)
    setPos({ top, left })
  }, [open, align])

  useEffect(() => {
    if (!open) return
    buttons()[0]?.focus()
    const onDown = (e: MouseEvent) => {
      const t = e.target as Node
      if (listRef.current?.contains(t) || triggerRef.current?.contains(t)) return
      setOpen(false)
    }
    window.addEventListener('mousedown', onDown)
    return () => window.removeEventListener('mousedown', onDown)
  }, [open])

  const onListKey = (e: ReactKeyboardEvent<HTMLDivElement>) => {
    const bs = buttons()
    const i = bs.indexOf(document.activeElement as HTMLButtonElement)
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      bs[(i + 1) % bs.length]?.focus()
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      bs[(i - 1 + bs.length) % bs.length]?.focus()
    } else if (e.key === 'Home') {
      e.preventDefault()
      bs[0]?.focus()
    } else if (e.key === 'End') {
      e.preventDefault()
      bs[bs.length - 1]?.focus()
    } else if (e.key === 'Escape') {
      e.preventDefault()
      e.stopPropagation()
      close(true)
    } else if (e.key === 'Tab') {
      close(false)
    } else if (e.key.length === 1 && !e.metaKey && !e.ctrlKey && !e.altKey && e.key !== ' ') {
      const t = typed.current
      const nowMs = Date.now()
      t.text = nowMs - t.at > 700 ? e.key.toLowerCase() : t.text + e.key.toLowerCase()
      t.at = nowMs
      const hit = bs.find((b) => (b.textContent ?? '').trim().toLowerCase().startsWith(t.text))
      hit?.focus()
    }
  }

  const tp = trigger.props
  return (
    <>
      {cloneElement(trigger, {
        ref: (el: HTMLElement | null) => {
          triggerRef.current = el
          const r = tp.ref
          if (typeof r === 'function') r(el)
          else if (r && typeof r === 'object') (r as { current: HTMLElement | null }).current = el
        },
        'aria-haspopup': 'menu',
        'aria-expanded': open,
        'aria-controls': open ? id : undefined,
        onClick: (e: ReactMouseEvent<HTMLElement>) => {
          tp.onClick?.(e)
          setOpen((o) => !o)
        },
        onKeyDown: (e: ReactKeyboardEvent<HTMLElement>) => {
          tp.onKeyDown?.(e)
          if (e.key === 'ArrowDown') {
            e.preventDefault()
            setOpen(true)
          }
        },
      })}
      {open &&
        createPortal(
          <div ref={listRef} id={id} role="menu" className="menu" style={{ position: 'fixed', top: pos?.top ?? -9999, left: pos?.left ?? -9999, minWidth: 180 }} onKeyDown={onListKey}>
            {items.map((it) => (
              <div key={it.id}>
                {it.separatorBefore && <div className="menu__sep" role="separator" />}
                <button
                  type="button"
                  role="menuitem"
                  disabled={it.disabled}
                  className={clsx('menu__item', it.danger && 'menu__item--danger')}
                  style={{ width: '100%', border: 0, background: 'transparent', font: 'inherit', textAlign: 'left', opacity: it.disabled ? 0.45 : 1 }}
                  onClick={() => {
                    close(true)
                    it.onSelect()
                  }}
                >
                  {it.icon && <it.icon size={15} aria-hidden="true" />}
                  {it.label}
                </button>
              </div>
            ))}
          </div>,
          document.body,
        )}
    </>
  )
}
