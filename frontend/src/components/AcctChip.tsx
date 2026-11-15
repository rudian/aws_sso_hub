import clsx from 'clsx'
import { useI18n } from '../i18n'
import { AcctTile } from './AcctTile'

export interface AcctChipProps {
  slot?: number
  monogram?: string
  name: string
  /** dashed border: a candidate that is not bound yet. */
  dashed?: boolean
  /** "Not bound" chip without identity colour. */
  none?: boolean
  /** Used in the accessible label: "账号 acme，111122223333". */
  accountId?: string
  className?: string
}

export function AcctChip({ slot, monogram, name, dashed, none, accountId, className }: AcctChipProps) {
  const { t } = useI18n()
  const label = accountId ? t('acct.label', { name, accountId }) : undefined
  return (
    <span
      className={clsx('acct-chip', none && 'acct-chip--none', dashed && 'acct-chip--dashed', className)}
      data-acct={none ? undefined : slot || undefined}
      aria-label={label}
      role={label ? 'img' : undefined}
    >
      {!none && monogram && <AcctTile slot={slot} monogram={monogram} />}
      <span className="acct-chip__name" title={name}>{name}</span>
    </span>
  )
}
