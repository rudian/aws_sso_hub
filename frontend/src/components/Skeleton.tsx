import clsx from 'clsx'
import type { CSSProperties } from 'react'

export function Skeleton({ variant, width, height, className }: { variant: 'text' | 'block'; width?: number | string; height?: number | string; className?: string }) {
  const style: CSSProperties = { width, height }
  return <div className={clsx('skeleton', `skeleton--${variant}`, className)} style={style} aria-hidden="true" />
}
