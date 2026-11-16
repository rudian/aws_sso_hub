import clsx from 'clsx'
import type { ButtonHTMLAttributes, ReactNode, Ref } from 'react'
import type { LucideIcon } from 'lucide-react'

export interface ButtonProps extends Omit<ButtonHTMLAttributes<HTMLButtonElement>, 'children'> {
  variant?: 'primary' | 'secondary' | 'ghost' | 'danger' | 'danger-ghost' | 'acct'
  size?: 'sm' | 'lg'
  block?: boolean
  loading?: boolean
  /** With `loading`: stay enabled, so keyboard focus stays on the button. The click handler must ignore clicks while it loads. */
  keepEnabled?: boolean
  icon?: LucideIcon
  children?: ReactNode
  ref?: Ref<HTMLButtonElement>
}

/**
 * `.btn` with one of the design variants. `loading` hides the label, shows a spinner and disables the button
 * (width preserved); with `keepEnabled` it only shows the spinner and aria-busy.
 */
export function Button({ variant = 'secondary', size, block, loading, keepEnabled, icon: Icon, className, children, disabled, type = 'button', ref, ...rest }: ButtonProps) {
  return (
    <button
      ref={ref}
      type={type}
      className={clsx('btn', `btn--${variant}`, size && `btn--${size}`, block && 'btn--block', loading && 'is-loading', className)}
      disabled={disabled || (loading && !keepEnabled)}
      aria-busy={loading || undefined}
      {...rest}
    >
      {Icon && <Icon className="btn__icon" aria-hidden="true" />}
      {children}
    </button>
  )
}
