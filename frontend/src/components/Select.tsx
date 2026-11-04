import clsx from 'clsx'
import type { Ref, SelectHTMLAttributes } from 'react'
import { useFieldProps } from './Field'

export function Select({ className, ref, children, ...rest }: SelectHTMLAttributes<HTMLSelectElement> & { ref?: Ref<HTMLSelectElement> }) {
  const f = useFieldProps()
  return (
    <select ref={ref} className={clsx('select', className)} {...f} {...rest}>
      {children}
    </select>
  )
}
