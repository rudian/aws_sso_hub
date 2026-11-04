import { Search, TriangleAlert } from 'lucide-react'
import { useMemo, useRef, useState } from 'react'
import { Accounts } from '../../api/services'
import { ErrorCode } from '../../api/models'
import type { DiscoveredAccount, DiscoveredRole, NewProfileSpec } from '../../api/models'
import { AcctTile } from '../../components/AcctTile'
import { Button } from '../../components/Button'
import { Checkbox } from '../../components/Checkbox'
import { EmptyState } from '../../components/EmptyState'
import { ErrorBanner } from '../../components/ErrorBanner'
import { Field } from '../../components/Field'
import { Input } from '../../components/Input'
import { Select } from '../../components/Select'
import { Skeleton } from '../../components/Skeleton'
import { useI18n } from '../../i18n'
import { AWS_REGIONS } from '../../lib/regions'
import { useAsync } from '../../lib/useAsync'
import { useSearchShortcut } from '../../lib/keyboard'
import { useActions } from '../../shell/AppActions'
import { useAccounts } from '../../state/accounts'
import { previewMonogram, profileNameProblem } from './model'
import './accounts.css'

export interface PickedRole extends NewProfileSpec {
  colorSlot: number
}

export const pickKey = (accountId: string, role: string) => `${accountId}|${role}`

/** What `CreateProfiles` takes, in account/role order. */
export function toSpecs(picked: Record<string, PickedRole>): NewProfileSpec[] {
  return Object.values(picked).map((p) => ({ name: p.name.trim(), accountId: p.accountId, accountName: p.accountName, role: p.role, region: p.region }))
}

/** Names that cannot be used: existing profiles, or two picked roles with the same name. */
export function nameProblems(picked: Record<string, PickedRole>, existing: ReadonlySet<string>): Record<string, 'invalid' | 'taken'> {
  const out: Record<string, 'invalid' | 'taken'> = {}
  const counts = new Map<string, number>()
  for (const p of Object.values(picked)) counts.set(p.name.trim(), (counts.get(p.name.trim()) ?? 0) + 1)
  for (const [k, p] of Object.entries(picked)) {
    const n = p.name.trim()
    const problem = profileNameProblem(n, existing) ?? ((counts.get(n) ?? 0) > 1 ? 'taken' : null)
    if (problem) out[k] = problem
  }
  return out
}

export interface RolePickerProps {
  portal: string
  portalRegion: string
  picked: Record<string, PickedRole>
  onPick: (next: Record<string, PickedRole>) => void
  /** Asks the page to sign in to the portal again (expired or missing session). */
  onSignIn: () => void
}

/** Step 2 of the add-portal flow: accounts and roles of a portal, with one profile name per ticked role. */
export function RolePicker({ portal, portalRegion, picked, onPick, onSignIn }: RolePickerProps) {
  const { t } = useI18n()
  const { navigate } = useActions()
  const { state } = useAccounts()
  const search = useRef<HTMLInputElement>(null)
  useSearchShortcut(search)
  const [query, setQuery] = useState('')
  const [retrying, setRetrying] = useState<string | null>(null)
  const [patches, setPatches] = useState<Record<string, DiscoveredAccount>>({})
  const discovered = useAsync(() => Accounts.DiscoverAccounts(portal), [portal])

  const existing = useMemo(() => new Set((state?.profiles ?? []).map((p) => p.name)), [state])
  const accounts = useMemo(() => (discovered.data ?? []).map((a) => patches[a.accountId] ?? a), [discovered.data, patches])
  const problems = useMemo(() => nameProblems(picked, existing), [picked, existing])
  const q = query.trim().toLowerCase()
  const shown = accounts.filter((a) => !q || a.accountName.toLowerCase().includes(q) || a.accountId.includes(q) || a.email.toLowerCase().includes(q))

  const toggle = (a: DiscoveredAccount, r: DiscoveredRole, on: boolean) => {
    const k = pickKey(a.accountId, r.name)
    const next = { ...picked }
    if (!on) delete next[k]
    else next[k] = { name: r.suggestedProfile, accountId: a.accountId, accountName: a.accountName, role: r.name, region: portalRegion, colorSlot: a.colorSlot }
    onPick(next)
  }
  const update = (k: string, patch: Partial<PickedRole>) => onPick({ ...picked, [k]: { ...picked[k], ...patch } })

  const retryAccount = async (a: DiscoveredAccount) => {
    setRetrying(a.accountId)
    try {
      const fresh = await Accounts.LoadAccountRoles(portal, a.accountId)
      setPatches((p) => ({ ...p, [a.accountId]: fresh }))
    } catch {
      /* the row keeps its warning */
    } finally {
      setRetrying(null)
    }
  }

  if (discovered.loading && !discovered.data) {
    return (
      <div className="role-picker" aria-busy="true">
        {[0, 1, 2].map((i) => (
          <Skeleton key={i} variant="block" height={64} />
        ))}
      </div>
    )
  }
  if (discovered.error) {
    const e = discovered.error
    const signIn = e.code === ErrorCode.ErrNotSignedIn || e.code === ErrorCode.ErrSessionExpired
    return (
      <div className="role-picker">
        <ErrorBanner error={e} onRetry={signIn ? undefined : () => void discovered.reload()} />
        {signIn && (
          <div>
            <p className="muted">{t('addPortal.roles.signInFirst', { portal })}</p>
            <Button variant="primary" onClick={onSignIn}>
              {t('addPortal.signIn')}
            </Button>
          </div>
        )}
      </div>
    )
  }
  if (accounts.length === 0) {
    return (
      <EmptyState
        title={t('addPortal.roles.empty')}
        actions={
          <Button variant="secondary" onClick={() => navigate({ page: 'tutorial', section: 'assign' })}>
            {t('accounts.empty.tutorial')}
          </Button>
        }
      />
    )
  }

  return (
    <div className="role-picker">
      <p className="muted role-picker__lead">{t('addPortal.roles.lead')}</p>
      <div className="role-picker__search">
        <Search aria-hidden="true" size={15} />
        <Input ref={search} type="search" placeholder={t('addPortal.roles.search')} aria-label={t('addPortal.roles.search')} value={query} onChange={(e) => setQuery(e.target.value)} />
        <span className="muted">{t('addPortal.roles.selected', { n: Object.keys(picked).length })}</span>
      </div>
      {shown.length === 0 && <p className="muted">{t('addPortal.roles.noMatch')}</p>}
      {shown.map((a) => (
        <section key={a.accountId} className="role-account" data-acct={a.colorSlot || undefined}>
          <header className="role-account__head">
            <AcctTile slot={a.colorSlot} monogram={previewMonogram(a.accountName)} size="sm" />
            <strong>{a.accountName}</strong>
            <span className="mono muted">{a.accountId}</span>
            {a.email && <span className="muted">{a.email}</span>}
          </header>
          {a.rolesError ? (
            <div className="role-account__warn">
              <TriangleAlert size={15} aria-hidden="true" />
              <span>{t('addPortal.roles.partial')}</span>
              <Button size="sm" loading={retrying === a.accountId} onClick={() => void retryAccount(a)}>
                {t('common.retry')}
              </Button>
            </div>
          ) : (
            (a.roles ?? []).map((r) => {
              const k = pickKey(a.accountId, r.name)
              const p = picked[k]
              const disabled = !!r.existingProfile
              return (
                <div key={k} className="role-row">
                  <Checkbox
                    checked={!!p || disabled}
                    disabled={disabled}
                    label={<span className="mono">{r.name}</span>}
                    onChange={(e) => toggle(a, r, e.target.checked)}
                  />
                  {disabled && <span className="muted">{t('addPortal.roles.existing', { name: r.existingProfile as string })}</span>}
                  {p && (
                    <div className="role-row__fields">
                      <Field
                        label={t('addPortal.roles.profileName')}
                        error={problems[k] ? (problems[k] === 'taken' ? t('addPortal.roles.profileTaken') : t('addPortal.name.invalid')) : undefined}
                      >
                        <Input mono spellCheck={false} value={p.name} onChange={(e) => update(k, { name: e.target.value })} />
                      </Field>
                      <Field label={t('addPortal.roles.region')}>
                        <Select value={p.region} onChange={(e) => update(k, { region: e.target.value })}>
                          {[...new Set([p.region, ...AWS_REGIONS])].map((reg) => (
                            <option key={reg} value={reg}>
                              {reg}
                            </option>
                          ))}
                        </Select>
                      </Field>
                      <div className="role-row__tile" title={t('addPortal.roles.colour')}>
                        <AcctTile slot={a.colorSlot} monogram={previewMonogram(p.name)} />
                      </div>
                    </div>
                  )}
                </div>
              )
            })
          )}
        </section>
      ))}
    </div>
  )
}
