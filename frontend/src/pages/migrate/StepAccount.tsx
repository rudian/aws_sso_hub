import { useEffect, useMemo, useState } from 'react'
import { Accounts, Migration } from '../../api/services'
import { ErrorCode, MigrationStep } from '../../api/models'
import type { DiscoveredAccount } from '../../api/models'
import { AcctTile } from '../../components/AcctTile'
import { Badge } from '../../components/Badge'
import { Banner } from '../../components/Banner'
import { Button } from '../../components/Button'
import { Checkbox } from '../../components/Checkbox'
import { ErrorBanner } from '../../components/ErrorBanner'
import { Input } from '../../components/Input'
import { Radio } from '../../components/Radio'
import { Skeleton } from '../../components/Skeleton'
import { useI18n } from '../../i18n'
import { AWS_REGIONS } from '../../lib/regions'
import { useAsync } from '../../lib/useAsync'
import { useActions } from '../../shell/AppActions'
import { previewMonogram } from '../accounts/model'
import { StepHead, WizardFooter } from './wizard'
import type { WizardCtx } from './wizard'
import { CodeText, codeMark } from '../../components/CodeText'

const keyOf = (accountId: string, role: string) => `${accountId}|${role}`

/** Step 3: choose the account and role the profile will point at. The account of the old key is pinned on top. */
export function StepAccount({ w }: { w: WizardCtx }) {
  const { t } = useI18n()
  const { openLogin, navigate } = useActions()
  const { st } = w
  const portal = st.portal ?? ''
  const discovered = useAsync(() => Accounts.DiscoverAccounts(portal), [portal])
  const keyAccount = st.identity.accountId

  const accounts = useMemo(() => {
    const list = discovered.data ?? []
    return [...list].sort((a, b) => Number(b.accountId === keyAccount) - Number(a.accountId === keyAccount))
  }, [discovered.data, keyAccount])
  const match = accounts.find((a) => a.accountId === keyAccount)

  const [sel, setSel] = useState<string | null>(st.accountId && st.role ? keyOf(st.accountId, st.role) : null)
  const [confirmOther, setConfirmOther] = useState(st.confirmOther)
  const [region, setRegion] = useState(st.region || w.profile?.region || '')
  const [editRegion, setEditRegion] = useState(false)

  // a single role on the account of the old key is chosen for the user
  useEffect(() => {
    if (sel) return
    // the account of the old key, or the only account when the key could not be identified
    const only = !keyAccount && accounts.length === 1 ? accounts[0] : undefined
    const target = match ?? only
    const roles = target?.roles ?? []
    if (target && roles.length === 1) setSel(keyOf(target.accountId, roles[0].name))
  }, [match, accounts, keyAccount, sel])

  const chosen = useMemo(() => {
    if (!sel) return null
    for (const a of accounts) for (const r of a.roles ?? []) if (keyOf(a.accountId, r.name) === sel) return { account: a, role: r.name }
    return null
  }, [sel, accounts])

  const other = !!chosen && !!keyAccount && chosen.account.accountId !== keyAccount
  const loaded = !!discovered.data
  const canNext = !!chosen && (!other || confirmOther)

  const next = async () => {
    if (!chosen) return
    w.setError(null)
    await w.run(async () => {
      await Migration.ChooseAccount(w.name, { accountId: chosen.account.accountId, accountName: chosen.account.accountName, role: chosen.role, region, confirmOther })
      return Migration.Preview(w.name)
    })
  }

  const signedOut = discovered.error?.code === ErrorCode.ErrNotSignedIn || discovered.error?.code === ErrorCode.ErrSessionExpired

  return (
    <>
      <StepHead title={t('migrate.s3.title')} />
      <div className="wizard__body">
        {discovered.loading && !loaded && (
          <div className="account-list" aria-busy="true" aria-label={t('migrate.s3.loading')}>
            {[0, 1].map((i) => (
              <Skeleton key={i} variant="block" height={88} />
            ))}
          </div>
        )}
        {discovered.error && (
          <>
            <ErrorBanner error={discovered.error} onRetry={signedOut ? undefined : () => void discovered.reload()} />
            {signedOut && (
              <div>
                <Button variant="primary" onClick={() => void openLogin(portal).then((r) => r && discovered.reload())}>
                  {t('migrate.s2.signIn')}
                </Button>
              </div>
            )}
          </>
        )}
        {loaded && keyAccount && !match && (
          <Banner
            kind="warn"
            actions={
              <>
                <Button size="sm" onClick={() => w.setView(MigrationStep.MigrationPortal)}>
                  {t('migrate.s3.otherPortal')}
                </Button>
                <Button size="sm" variant="ghost" onClick={() => navigate({ page: 'tutorial', section: 'assign' })}>
                  {t('migrate.s3.tutorial')}
                </Button>
              </>
            }
          >
            {t('migrate.s3.noMatch', { id: keyAccount })}
          </Banner>
        )}
        {loaded && accounts.length > 0 && (
          <div className="account-list" role="radiogroup" aria-label={t('migrate.s3.role')}>
            {accounts.map((a) => (
              <AccountCard key={a.accountId} account={a} match={a.accountId === keyAccount} selected={chosen?.account.accountId === a.accountId} sel={sel} onSelect={setSel} />
            ))}
          </div>
        )}
        {loaded && accounts.length === 0 && <p className="muted">{t('addPortal.roles.empty')}</p>}
        {other && (
          <Checkbox checked={confirmOther} onChange={(e) => setConfirmOther(e.target.checked)} label={<CodeText text={t('migrate.s3.confirmOther', { name: codeMark(w.name) })} />} />
        )}
        <div className="cluster">
          {editRegion ? (
            <>
              <label className="muted" htmlFor="migrate-region">
                {t('migrate.s3.regionLabel')}
              </label>
              <Input id="migrate-region" mono list="aws-regions" value={region} onChange={(e) => setRegion(e.target.value)} style={{ width: 200 }} />
              <datalist id="aws-regions">
                {AWS_REGIONS.map((r) => (
                  <option key={r} value={r} />
                ))}
              </datalist>
            </>
          ) : (
            <>
              <span className="muted">{t('migrate.s3.region', { region: region || '—' })}</span>
              <button type="button" className="link-button" onClick={() => setEditRegion(true)}>
                {t('migrate.s3.changeRegion')}
              </button>
            </>
          )}
        </div>
      </div>
      <WizardFooter
        w={w}
        primary={
          <Button variant="primary" loading={w.busy} disabled={!canNext} onClick={() => void next()}>
            {t('common.next')}
          </Button>
        }
      />
    </>
  )
}

function AccountCard({ account, match, selected, sel, onSelect }: { account: DiscoveredAccount; match: boolean; selected: boolean; sel: string | null; onSelect: (k: string) => void }) {
  const { t } = useI18n()
  const roles = account.roles ?? []
  return (
    <section className={`account-card${match ? ' is-match' : ''}${selected ? ' is-selected' : ''}`} data-acct={account.colorSlot || undefined}>
      <header className="account-card__head">
        <AcctTile slot={account.colorSlot} monogram={previewMonogram(account.accountName)} size="sm" />
        <strong>{account.accountName}</strong>
        <span className="mono muted">{account.accountId}</span>
        {match && <Badge kind="ok">{t('migrate.s3.match')}</Badge>}
      </header>
      <div className="account-card__roles">
        {roles.length === 0 && <span className="muted">{t('migrate.s3.noRoles')}</span>}
        {roles.map((r) => (
          <Radio
            key={r.name}
            name="migrate-role"
            checked={sel === keyOf(account.accountId, r.name)}
            onChange={() => onSelect(keyOf(account.accountId, r.name))}
            label={<span className="mono">{r.name}</span>}
          />
        ))}
      </div>
    </section>
  )
}
