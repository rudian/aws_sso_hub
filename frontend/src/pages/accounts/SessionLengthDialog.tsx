import { useMemo, useState } from 'react'
import { toUiError } from '../../api/errors'
import type { UiError } from '../../api/errors'
import { Accounts } from '../../api/services'
import { Button } from '../../components/Button'
import { ErrorBanner } from '../../components/ErrorBanner'
import { Field } from '../../components/Field'
import { Input } from '../../components/Input'
import { Modal } from '../../components/Modal'
import { Segmented } from '../../components/Segmented'
import { useI18n } from '../../i18n'
import type { T } from '../../i18n'
import { useActions } from '../../shell/AppActions'
import { SESSION_LENGTH_PRESETS, hoursToMinutes, minutesToHours } from './model'
import './accounts.css'

export function presetLabel(minutes: number, t: T): string {
  return minutes >= 1440 ? t('session.length.preset.d', { n: minutes / 1440 }) : t('session.length.preset.h', { n: minutes / 60 })
}

/** How long the IAM Identity Center user session is. Only changes the countdown, never AWS. */
export function SessionLengthDialog({ portal, minutes, onClose }: { portal: string; minutes: number; onClose: () => void }) {
  const { t } = useI18n()
  const { toast } = useActions()
  const isPreset = (SESSION_LENGTH_PRESETS as readonly number[]).includes(minutes)
  const [preset, setPreset] = useState<string>(isPreset ? String(minutes) : '')
  const [custom, setCustom] = useState<string>(isPreset ? '' : minutesToHours(minutes))
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<UiError | null>(null)

  const items = useMemo(() => SESSION_LENGTH_PRESETS.map((m) => ({ value: String(m), label: presetLabel(m, t) })), [t])
  const customMinutes = custom.trim() === '' ? null : hoursToMinutes(Number(custom))
  const customInvalid = custom.trim() !== '' && customMinutes === null
  const chosen = custom.trim() !== '' ? customMinutes : preset ? Number(preset) : null

  const save = async () => {
    if (chosen === null) return
    setSaving(true)
    setError(null)
    try {
      await Accounts.SetSessionLength(portal, chosen)
      toast({ kind: 'ok', title: t('session.length.saved') })
      onClose()
    } catch (e) {
      setError(toUiError(e))
      setSaving(false)
    }
  }

  return (
    <Modal
      open
      title={`${t('session.length.title')} · ${portal}`}
      onClose={onClose}
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button variant="primary" loading={saving} disabled={chosen === null || chosen === minutes} onClick={() => void save()}>
            {t('common.save')}
          </Button>
        </>
      }
    >
      <div className="stack" style={{ ['--gap' as string]: 'var(--sp-4)' }}>
        <p className="muted" style={{ margin: 0 }}>
          {t('session.length.body')}
        </p>
        <Segmented
          items={items}
          value={preset}
          ariaLabel={t('session.length.title')}
          onChange={(v) => {
            setPreset(v)
            setCustom('')
          }}
        />
        <Field label={t('session.length.custom')} error={customInvalid ? t('session.length.invalid') : undefined}>
          <Input
            type="number"
            inputMode="decimal"
            min={0.25}
            max={2160}
            step={0.25}
            value={custom}
            style={{ maxWidth: 160 }}
            onChange={(e) => {
              setCustom(e.target.value)
              if (e.target.value.trim() !== '') setPreset('')
            }}
          />
        </Field>
        {error && <ErrorBanner error={error} />}
      </div>
    </Modal>
  )
}
