import type { Profile } from '../../api/models'
import { Select } from '../../components'
import { useI18n } from '../../i18n'
import { profileLabel } from './model'

export interface ProfileSelectProps {
  profiles: Profile[]
  value: string
  onChange: (name: string) => void
  placeholder: string
  /** Names that are listed but cannot be picked. */
  disabledNames?: ReadonlySet<string>
  id?: string
  'aria-label'?: string
}

/** A native select of profiles: "name · account id". Used wherever the user picks which AWS profile to bind. */
export function ProfileSelect({ profiles, value, onChange, placeholder, disabledNames, id, 'aria-label': ariaLabel }: ProfileSelectProps) {
  const { t } = useI18n()
  return (
    <Select {...(id ? { id } : {})} {...(ariaLabel ? { 'aria-label': ariaLabel } : {})} value={value} onChange={(e) => onChange(e.target.value)}>
      <option value="" disabled={value !== ''}>
        {placeholder}
      </option>
      {profiles.map((p) => (
        <option key={p.name} value={p.name} disabled={disabledNames?.has(p.name)}>
          {profileLabel(p, t('repos.profile.notIdentified'))}
        </option>
      ))}
    </Select>
  )
}
