import clsx from 'clsx'
import type { ButtonHTMLAttributes, ReactNode, Ref } from 'react'
import type { LucideIcon } from 'lucide-react'

export interface ButtonProps extends Omit<ButtonHTMLAttributes<HTMLButtonElement>, 'children'> {
  variant?: 'primary' | 'secondary' | 'ghost' | 'danger' | 'danger-ghost' | 'acct'
  size?: 'sm' | 'lg'
  block?: boolean
  loading?: boolean
  icon?: LucideIcon
  children?: ReactNode
  ref?: Ref<HTMLButtonElement>
}

/** `.btn` with one of the design variants. `loading` hides the label, shows a spinner and disables the button (width preserved). */
export function Button({ variant = 'secondary', size, block, loading, icon: Icon, className, children, disabled, type = 'button', ref, ...rest }: ButtonProps) {
  return (
    <button
      ref={ref}
      type={type}
      className={clsx('btn', `btn--${variant}`, size && `btn--${size}`, block && 'btn--block', loading && 'is-loading', className)}
      disabled={disabled || loading}
      aria-busy={loading || undefined}
      {...rest}
    >
      {Icon && <Icon className="btn__icon" aria-hidden="true" />}
      {children}
    </button>
  )
}
