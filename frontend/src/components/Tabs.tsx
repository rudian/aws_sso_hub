import clsx from 'clsx'
import type { KeyboardEvent, ReactNode } from 'react'
import { Badge } from './Badge'
import { Tooltip } from './Tooltip'

export interface TabItem {
  id: string
  label: ReactNode
  badge?: ReactNode
  disabled?: boolean
  tooltip?: ReactNode
}

export interface TabsProps {
  tabs: TabItem[]
  value: string
  onChange: (id: string) => void
  ariaLabel?: string
}

export function Tabs({ tabs, value, onChange, ariaLabel }: TabsProps) {
  const enabled = tabs.filter((t) => !t.disabled)
  const onKey = (e: KeyboardEvent<HTMLDivElement>) => {
    const i = enabled.findIndex((t) => t.id === value)
    let next = -1
    if (e.key === 'ArrowRight') next = (i + 1) % enabled.length
    else if (e.key === 'ArrowLeft') next = (i - 1 + enabled.length) % enabled.length
    else if (e.key === 'Home') next = 0
    else if (e.key === 'End') next = enabled.length - 1
    if (next >= 0) {
      e.preventDefault()
      onChange(enabled[next].id)
      requestAnimationFrame(() => (e.currentTarget.querySelector('[aria-selected="true"]') as HTMLElement | null)?.focus())
    }
  }
  return (
    <div className="tabs" role="tablist" aria-label={ariaLabel} onKeyDown={onKey}>
      {tabs.map((t) => (
        <Tooltip key={t.id} content={t.tooltip}>
          <button
            type="button"
            role="tab"
            aria-selected={t.id === value}
            aria-disabled={t.disabled || undefined}
            tabIndex={t.id === value ? 0 : -1}
            className={clsx('tab', t.id === value && 'is-active')}
            style={t.disabled ? { opacity: 0.5 } : undefined}
            onClick={() => !t.disabled && onChange(t.id)}
          >
            {t.label}
            {t.badge !== undefined && <Badge kind="outline">{t.badge}</Badge>}
          </button>
        </Tooltip>
      ))}
    </div>
  )
}
