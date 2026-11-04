import clsx from 'clsx'
import type { ReactNode } from 'react'
import type { LucideIcon } from 'lucide-react'

export interface BadgeProps {
  kind?: 'ok' | 'warn' | 'danger' | 'info' | 'neutral' | 'outline'
  icon?: LucideIcon
  className?: string
  children?: ReactNode
}

export function Badge({ kind = 'neutral', icon: Icon, className, children }: BadgeProps) {
  return (
    <span className={clsx('badge', `badge--${kind}`, className)}>
      {Icon && <Icon aria-hidden="true" />}
      {children}
    </span>
  )
}
