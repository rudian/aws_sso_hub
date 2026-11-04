import clsx from 'clsx'
import type { CSSProperties } from 'react'

export function Progress({ pct, indeterminate, className, label }: { pct?: number; indeterminate?: boolean; className?: string; label?: string }) {
  const style = { '--pct': `${Math.max(0, Math.min(100, pct ?? 0))}%` } as CSSProperties
  return (
    <div
      className={clsx('progress', indeterminate && 'progress--indeterminate', className)}
      role="progressbar"
      aria-label={label}
      aria-valuemin={0}
      aria-valuemax={100}
      aria-valuenow={indeterminate ? undefined : Math.round(pct ?? 0)}
    >
      <div className="progress__bar" style={style} />
    </div>
  )
}
