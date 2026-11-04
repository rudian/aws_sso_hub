import clsx from 'clsx'
import { CircleCheck, Info, OctagonAlert, TriangleAlert } from 'lucide-react'
import type { ReactNode } from 'react'

export interface BannerProps {
  kind: 'info' | 'warn' | 'danger' | 'ok'
  title?: ReactNode
  actions?: ReactNode
  className?: string
  children?: ReactNode
}

const ICONS = { info: Info, warn: TriangleAlert, danger: OctagonAlert, ok: CircleCheck }

export function Banner({ kind, title, actions, className, children }: BannerProps) {
  const Icon = ICONS[kind]
  return (
    <div className={clsx('banner', kind !== 'info' && `banner--${kind}`, className)} role={kind === 'danger' ? 'alert' : 'status'}>
      <Icon className="banner__icon" aria-hidden="true" />
      <div className="banner__body">
        {title && <div className="banner__title">{title}</div>}
        {children}
      </div>
      {actions && <div className="banner__actions">{actions}</div>}
    </div>
  )
}
