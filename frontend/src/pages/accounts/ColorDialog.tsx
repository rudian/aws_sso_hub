import clsx from 'clsx'
import { Check } from 'lucide-react'
import { useRef, useState } from 'react'
import { toUiError } from '../../api/errors'
import type { UiError } from '../../api/errors'
import { Accounts } from '../../api/services'
import type { Profile } from '../../api/models'
import { AcctTile } from '../../components/AcctTile'
import { Button } from '../../components/Button'
import { ErrorBanner } from '../../components/ErrorBanner'
import { Field } from '../../components/Field'
import { Input } from '../../components/Input'
import { Modal } from '../../components/Modal'
import { useI18n } from '../../i18n'
import { useActions } from '../../shell/AppActions'
import { validMonogram } from './model'
import './accounts.css'

/** Palette names. They are proper names, so they are the same in both languages. */
export const COLOR_NAMES = ['Cobalt', 'Jade', 'Iris', 'Orchid', 'Lagoon', 'Copper', 'Olive', 'Ruby'] as const

/** Change the account colour (shared by every profile of the account) and the profile's initials. */
export function ColorDialog({ profile, onClose }: { profile: Profile; onClose: () => void }) {
  const { t } = useI18n()
  const { toast } = useActions()
  const [slot, setSlot] = useState(profile.colorSlot || 1)
  const [mono, setMono] = useState(profile.monogram)
  const [touched, setTouched] = useState(false)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<UiError | null>(null)
  const selected = useRef<HTMLButtonElement>(null)

  const clean = mono.trim()
  const monoOk = validMonogram(clean)
  const shown = clean && monoOk ? clean.toUpperCase() : profile.monogram

  const save = async () => {
    if (!monoOk) return
    setSaving(true)
    setError(null)
    try {
      await Accounts.SetProfileIdentity({ profile: profile.name, colorSlot: slot, monogram: clean })
      toast({ kind: 'ok', title: t('card.colorSaved', { name: profile.name }) })
      onClose()
    } catch (e) {
      setError(toUiError(e))
      setSaving(false)
    }
  }

  return (
    <Modal
      open
      title={t('color.title')}
      subtitle={<span className="mono">{profile.name}</span>}
      onClose={onClose}
      initialFocus={selected}
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button variant="primary" loading={saving} disabled={!monoOk} onClick={() => void save()}>
            {t('common.save')}
          </Button>
        </>
      }
    >
      <div className="color-dialog">
        <div className="color-dialog__preview" aria-label={t('color.preview')} role="group">
          <AcctTile slot={slot} monogram={shown} size="lg" />
          <div>
            <div className="pass__name">{profile.name}</div>
            <div className="pass__role">{profile.role ?? ''}</div>
          </div>
        </div>
        <div>
          <div className="field__label" id="color-swatches">
            {t('color.swatches')}
          </div>
          <div className="swatches" role="radiogroup" aria-labelledby="color-swatches">
            {COLOR_NAMES.map((name, i) => {
              const n = i + 1
              const on = n === slot
              return (
                <button
                  key={name}
                  ref={on ? selected : undefined}
                  type="button"
                  role="radio"
                  aria-checked={on}
                  aria-label={name}
                  title={name}
                  data-acct={n}
                  className={clsx('swatch', on && 'is-on')}
                  tabIndex={on ? 0 : -1}
                  onClick={() => setSlot(n)}
                  onKeyDown={(e) => {
                    const step = e.key === 'ArrowRight' || e.key === 'ArrowDown' ? 1 : e.key === 'ArrowLeft' || e.key === 'ArrowUp' ? -1 : 0
                    if (!step) return
                    e.preventDefault()
                    const next = ((n - 1 + step + 8) % 8) + 1
                    setSlot(next)
                    requestAnimationFrame(() => (e.currentTarget.parentElement?.querySelector('[aria-checked="true"]') as HTMLElement | null)?.focus())
                  }}
                >
                  {on && <Check aria-hidden="true" />}
                </button>
              )
            })}
          </div>
          <div className="field__hint" style={{ marginTop: 6 }}>
            {t('tip.colorKey')}
          </div>
        </div>
        <Field label={t('color.monogram')} hint={t('color.monogram.hint')} error={touched && !monoOk ? t('color.monogram.invalid') : undefined}>
          <Input
            value={mono}
            maxLength={2}
            placeholder={profile.monogram}
            style={{ maxWidth: 120 }}
            onChange={(e) => setMono(e.target.value)}
            onBlur={() => setTouched(true)}
          />
        </Field>
        {error && <ErrorBanner error={error} />}
      </div>
    </Modal>
  )
}
