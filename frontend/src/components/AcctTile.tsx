import clsx from 'clsx'

export interface AcctTileProps {
  /** Identity colour slot 1..8 (0 or undefined: neutral). */
  slot?: number
  monogram: string
  size?: 'sm' | 'lg'
  hollow?: boolean
  className?: string
}

export function AcctTile({ slot, monogram, size, hollow, className }: AcctTileProps) {
  return (
    <span className={clsx('acct-tile', size && `acct-tile--${size}`, (hollow || !slot) && 'acct-tile--hollow', className)} data-acct={slot || undefined} aria-hidden="true">
      {monogram}
    </span>
  )
}
