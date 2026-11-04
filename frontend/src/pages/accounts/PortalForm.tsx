import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Accounts } from '../../api/services'
import { ErrorCode } from '../../api/models'
import type { PortalInputCheck } from '../../api/models'
import { Field } from '../../components/Field'
import { Input } from '../../components/Input'
import { Select } from '../../components/Select'
import { useI18n } from '../../i18n'
import { AWS_REGIONS } from '../../lib/regions'
import { useAccounts } from '../../state/accounts'

export interface PortalFormValues {
  startUrl: string
  region: string
  name: string
}

export interface PortalFormApi {
  values: PortalFormValues
  errors: Partial<Record<keyof PortalFormValues, ErrorCode>>
  touched: Partial<Record<keyof PortalFormValues, boolean>>
  setField: (k: keyof PortalFormValues, v: string) => void
  blur: (k: keyof PortalFormValues) => void
  /** Check everything with the backend and mark all fields as touched. Resolves true when the portal can be created. */
  validate: () => Promise<{ ok: boolean; values: PortalFormValues }>
  valid: boolean
}

/** The region used most by existing SSO profiles (fallback for a URL that does not name one). */
function usualRegion(regions: Array<string | undefined>): string {
  const count = new Map<string, number>()
  for (const r of regions) if (r) count.set(r, (count.get(r) ?? 0) + 1)
  let best = ''
  for (const [r, n] of count) if (n > (count.get(best) ?? 0)) best = r
  return best
}

/** State of the portal details form. Used by the add-portal modal and by the migration wizard. */
export function usePortalForm(initial?: Partial<PortalFormValues>): PortalFormApi {
  const { state } = useAccounts()
  const fallbackRegion = useMemo(() => usualRegion((state?.portals ?? []).filter((p) => !p.draft && !p.legacyFormat).map((p) => p.region)), [state])
  const [values, setValues] = useState<PortalFormValues>({ startUrl: initial?.startUrl ?? '', region: initial?.region ?? '', name: initial?.name ?? '' })
  const [errors, setErrors] = useState<PortalFormApi['errors']>({})
  const [touched, setTouched] = useState<PortalFormApi['touched']>({})
  const edited = useRef({ name: !!initial?.name, region: !!initial?.region })
  // Prefill the usual region as soon as the accounts are known, so the field is never empty before the URL is checked.
  useEffect(() => {
    if (fallbackRegion && !edited.current.region) setValues((cur) => (cur.region ? cur : { ...cur, region: fallbackRegion }))
  }, [fallbackRegion])
  const seq = useRef(0)

  const run = useCallback(
    async (v: PortalFormValues): Promise<{ values: PortalFormValues; errors: PortalFormApi['errors'] }> => {
      const my = ++seq.current
      let c: PortalInputCheck
      try {
        c = await Accounts.CheckPortalInput({ name: v.name, startUrl: v.startUrl, region: v.region })
      } catch {
        return { values: v, errors: {} }
      }
      const next: PortalFormValues = {
        startUrl: c.startUrl || v.startUrl,
        name: v.name.trim() || c.name,
        // A region the user did not pick follows the URL; one they picked stays.
        region: edited.current.region ? v.region.trim() || c.region || fallbackRegion : c.region || v.region.trim() || fallbackRegion,
      }
      if (my === seq.current) {
        setValues((cur) => ({ startUrl: cur.startUrl === v.startUrl ? next.startUrl : cur.startUrl, name: cur.name === v.name ? next.name : cur.name, region: cur.region === v.region ? next.region : cur.region }))
        setErrors((c.errors ?? {}) as PortalFormApi['errors'])
      }
      return { values: next, errors: (c.errors ?? {}) as PortalFormApi['errors'] }
    },
    [fallbackRegion],
  )

  const setField = useCallback((k: keyof PortalFormValues, v: string) => {
    if (k === 'name' || k === 'region') edited.current[k] = true
    setValues((cur) => ({ ...cur, [k]: v }))
  }, [])

  const blur = useCallback(
    (k: keyof PortalFormValues) => {
      setTouched((cur) => ({ ...cur, [k]: true }))
      void run({ ...values })
    },
    [run, values],
  )

  const validate = useCallback(async () => {
    setTouched({ startUrl: true, region: true, name: true })
    const r = await run({ ...values })
    const ok = Object.values(r.errors).every((e) => !e) && !!r.values.startUrl.trim() && !!r.values.name.trim() && !!r.values.region.trim()
    return { ok, values: r.values }
  }, [run, values])

  const valid = !!values.startUrl.trim() && !!values.name.trim() && !!values.region.trim() && Object.values(errors).every((e) => !e)
  return { values, errors, touched, setField, blur, validate, valid }
}

/** The known regions, plus the current value when it is not one of them (for example one taken from a pasted URL). */
export function regionOptions(current: string): string[] {
  return current && !AWS_REGIONS.includes(current) ? [current, ...AWS_REGIONS] : AWS_REGIONS
}

function startUrlError(code: ErrorCode | undefined, empty: boolean, t: ReturnType<typeof useI18n>['t']): string | undefined {
  if (code || empty) return t('addPortal.startUrl.invalid')
  return undefined
}

/** The three fields of the portal details step. */
export function PortalFields({ form, autoFocus }: { form: PortalFormApi; autoFocus?: boolean }) {
  const { t } = useI18n()
  const { values, errors, touched } = form
  const nameErr = errors.name === ErrorCode.ErrPortalExists ? t('addPortal.name.taken') : errors.name ? t('addPortal.name.invalid') : undefined
  return (
    <div className="portal-fields">
      <Field label={t('addPortal.startUrl')} hint={t('addPortal.startUrl.hint')} error={touched.startUrl ? startUrlError(errors.startUrl, !values.startUrl.trim(), t) : undefined}>
        <Input
          mono
          autoFocus={autoFocus}
          spellCheck={false}
          autoCapitalize="off"
          autoCorrect="off"
          placeholder={t('addPortal.startUrl.placeholder')}
          value={values.startUrl}
          onChange={(e) => form.setField('startUrl', e.target.value)}
          onBlur={() => form.blur('startUrl')}
        />
      </Field>
      <div className="portal-fields__row">
        <Field label={t('addPortal.region')} hint={t('addPortal.region.hint')} error={touched.region && errors.region ? t('addPortal.region.invalid') : undefined}>
          <Select className="mono" value={values.region} onChange={(e) => form.setField('region', e.target.value)} onBlur={() => form.blur('region')}>
            <option value="" disabled={values.region !== ''}>
              {t('addPortal.region.choose')}
            </option>
            {regionOptions(values.region).map((r) => (
              <option key={r} value={r}>
                {r}
              </option>
            ))}
          </Select>
        </Field>
        <Field label={t('addPortal.name')} hint={t('addPortal.name.hint')} error={touched.name ? nameErr : undefined}>
          <Input mono spellCheck={false} value={values.name} onChange={(e) => form.setField('name', e.target.value)} onBlur={() => form.blur('name')} />
        </Field>
      </div>
    </div>
  )
}
