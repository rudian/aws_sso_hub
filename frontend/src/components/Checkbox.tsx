import clsx from 'clsx'
import type { InputHTMLAttributes, ReactNode, Ref } from 'react'
import { useFieldProps } from './Field'

export interface CheckboxProps extends Omit<InputHTMLAttributes<HTMLInputElement>, 'type'> {
  label?: ReactNode
  ref?: Ref<HTMLInputElement>
}

/** Native checkbox styled by `.checkbox`. With `label` it renders a clickable label next to the box. */
export function Checkbox({ label, className, ref, ...rest }: CheckboxProps) {
  const f = useFieldProps()
  const input = <input ref={ref} type="checkbox" className={clsx('checkbox', !label && className)} {...f} {...rest} />
  if (!label) return input
  return (
    <label className={clsx('cluster', className)} style={{ gap: 8, flexWrap: 'nowrap' }}>
      {input}
      <span>{label}</span>
    </label>
  )
}
