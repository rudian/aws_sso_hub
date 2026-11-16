import clsx from 'clsx'
import type { KeyboardEvent, ReactNode } from 'react'

export interface SegmentedItem<T> {
  value: T
  label: ReactNode
  count?: number
  warn?: boolean
}

export interface SegmentedProps<T extends string> {
  items: SegmentedItem<T>[]
  value: T
  onChange: (v: T) => void
  ariaLabel: string
  /** group: buttons with aria-pressed (filters). radiogroup: one choice, arrow keys move (settings). */
  role?: 'group' | 'radiogroup'
}

export function Segmented<T extends string>({ items, value, onChange, ariaLabel, role = 'group' }: SegmentedProps<T>) {
  const radio = role === 'radiogroup'
  const onKey = (e: KeyboardEvent<HTMLDivElement>) => {
    if (!radio) return
    // The next item comes from the radio that has focus, not from the value prop: the value arrives late (async save),
    // so two quick arrow presses would otherwise land on the same item.
    const radios = Array.from(e.currentTarget.querySelectorAll<HTMLElement>('[role="radio"]'))
    const at = radios.indexOf((e.target as HTMLElement).closest('[role="radio"]') as HTMLElement)
    const i = at >= 0 ? at : items.findIndex((it) => it.value === value)
    let next = -1
    if (e.key === 'ArrowRight' || e.key === 'ArrowDown') next = (i + 1) % items.length
    else if (e.key === 'ArrowLeft' || e.key === 'ArrowUp') next = (i - 1 + items.length) % items.length
    if (next >= 0) {
      e.preventDefault()
      onChange(items[next].value)
      // Focus by position, not by checked state: the new value may arrive later than the next frame (async save).
      // React clears currentTarget once the handler returns, so the target is taken now.
      const target = e.currentTarget.querySelectorAll<HTMLElement>('[role="radio"]')[next]
      requestAnimationFrame(() => target?.focus())
    }
  }
  return (
    <div className="segmented" role={role} aria-label={ariaLabel} onKeyDown={onKey}>
      {items.map((it) => {
        const active = it.value === value
        return (
          <button
            key={it.value}
            type="button"
            role={radio ? 'radio' : undefined}
            aria-checked={radio ? active : undefined}
            aria-pressed={radio ? undefined : active}
            tabIndex={radio ? (active ? 0 : -1) : undefined}
            className={clsx('segmented__item', active && 'is-active')}
            onClick={() => onChange(it.value)}
          >
            {it.label}
            {it.count !== undefined && <span className={clsx('nav__count', it.warn && 'nav__count--warn')}>{it.count}</span>}
          </button>
        )
      })}
    </div>
  )
}
