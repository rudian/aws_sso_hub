import clsx from 'clsx'
import { CircleCheck, Info, OctagonAlert, TriangleAlert } from 'lucide-react'
import type { ReactNode } from 'react'
import { Spinner } from '../../components/Spinner'
import { useI18n } from '../../i18n'
import type { MessageKey } from '../../i18n'
import './health.css'

export type RowStatus = 'ok' | 'warn' | 'danger' | 'info' | 'busy'

const ICONS = { ok: CircleCheck, warn: TriangleAlert, danger: OctagonAlert, info: Info }

export interface StatusRowProps {
  status: RowStatus
  title: ReactNode
  text?: ReactNode
  meta?: ReactNode
  action?: ReactNode
  compact?: boolean
}

/** One check: status icon, title, one line of result and an optional action on the right. */
export function StatusRow({ status, title, text, meta, action, compact }: StatusRowProps) {
  const { t } = useI18n()
  const Icon = status === 'busy' ? null : ICONS[status]
  // Spoken status: the colour is never the only signal.
  const statusLabel = status === 'busy' ? undefined : t(`health.severity.${status}` as MessageKey)
  return (
    <div className={clsx('list__item', 'hrow', compact && 'hrow--compact')}>
      <span className={clsx('hrow__icon', `hrow__icon--${status}`)}>
        {Icon ? <Icon aria-hidden="true" /> : <Spinner />}
        {statusLabel && <span className="sr-only">{statusLabel}</span>}
      </span>
      <div className="hrow__body">
        <div className="hrow__title">{title}</div>
        {text && <div className="hrow__text">{text}</div>}
        {meta && <div className="hrow__meta">{meta}</div>}
      </div>
      {action && <div className="hrow__action">{action}</div>}
    </div>
  )
}
