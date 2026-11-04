import clsx from 'clsx'
import type { ButtonHTMLAttributes, Ref } from 'react'
import type { LucideIcon } from 'lucide-react'
import { Tooltip } from './Tooltip'

export interface IconButtonProps extends Omit<ButtonHTMLAttributes<HTMLButtonElement>, 'children' | 'aria-label'> {
  /** aria-label and tooltip text. */
  label: string
  icon: LucideIcon
  size?: 'sm'
  bordered?: boolean
  ref?: Ref<HTMLButtonElement>
}

export function IconButton({ label, icon: Icon, size, bordered, className, type = 'button', ref, ...rest }: IconButtonProps) {
  return (
    <Tooltip content={label}>
      <button ref={ref} type={type} aria-label={label} className={clsx('icon-btn', size && `icon-btn--${size}`, bordered && 'icon-btn--bordered', className)} {...rest}>
        <Icon aria-hidden="true" />
      </button>
    </Tooltip>
  )
}
