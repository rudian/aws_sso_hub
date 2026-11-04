import clsx from 'clsx'

export type DotKind = 'ok' | 'warn' | 'danger' | 'off' | 'busy'

/** Status dot. Always put text next to it: colour is never the only signal. */
export function Dot({ kind, className }: { kind: DotKind; className?: string }) {
  return <span className={clsx('dot', `dot--${kind}`, className)} aria-hidden="true" />
}
