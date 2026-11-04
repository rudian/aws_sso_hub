import clsx from 'clsx'
import type { InputHTMLAttributes, Ref } from 'react'
import { useFieldProps } from './Field'

export interface InputProps extends InputHTMLAttributes<HTMLInputElement> {
  /** Monospace for literal values (URLs, paths, IDs). */
  mono?: boolean
  ref?: Ref<HTMLInputElement>
}

export function Input({ mono, className, ref, ...rest }: InputProps) {
  const f = useFieldProps()
  return <input ref={ref} className={clsx('input', mono && 'input--mono', className)} {...f} {...rest} />
}
