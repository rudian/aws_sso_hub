import clsx from 'clsx'
import type { InputHTMLAttributes, ReactNode, Ref } from 'react'

export interface RadioProps extends Omit<InputHTMLAttributes<HTMLInputElement>, 'type'> {
  label?: ReactNode
  ref?: Ref<HTMLInputElement>
}

export function Radio({ label, className, ref, ...rest }: RadioProps) {
  const input = <input ref={ref} type="radio" className={clsx('radio', !label && className)} {...rest} />
  if (!label) return input
  return (
    <label className={clsx('cluster', className)} style={{ gap: 8, flexWrap: 'nowrap' }}>
      {input}
      <span>{label}</span>
    </label>
  )
}
