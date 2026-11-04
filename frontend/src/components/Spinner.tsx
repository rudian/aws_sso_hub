import clsx from 'clsx'

export function Spinner({ className }: { className?: string }) {
  return <span className={clsx('spinner', className)} aria-hidden="true" />
}
