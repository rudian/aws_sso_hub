import { useMemo, useState } from 'react'
import { toUiError } from '../../api/errors'
import type { UiError } from '../../api/errors'
import { Repos } from '../../api/services'
import { BindingSource, ProfileKind } from '../../api/models'
import type { Profile } from '../../api/models'
import { AcctTile } from '../../components/AcctTile'
import { Badge } from '../../components/Badge'
import { Banner } from '../../components/Banner'
import { Button } from '../../components/Button'
import { EmptyState } from '../../components/EmptyState'
import { ErrorBanner } from '../../components/ErrorBanner'
import { Modal } from '../../components/Modal'
import { Radio } from '../../components/Radio'
import { useI18n } from '../../i18n'
import { useActions } from '../../shell/AppActions'
import { useAccounts } from '../../state/accounts'
import { useRepos } from '../../state/repos'
import './accounts.css'

/** Repositories whose own git config points at this profile. */
export function reposBoundTo(repos: ReturnType<typeof useRepos>['state'], profile: string) {
  return (repos?.repos ?? []).filter((r) => r.binding.source === BindingSource.BindingLocal && r.binding.profile === profile)
}

/** Target order: same AWS account first, SSO before access keys, then by name. */
export function moveTargets(all: Profile[], from: Profile): Profile[] {
  const usable = all.filter((p) => p.name !== from.name && (p.kind === ProfileKind.ProfileKindSSO || p.kind === ProfileKind.ProfileKindStatic))
  const rank = (p: Profile) => (from.accountId && p.accountId === from.accountId ? 0 : 2) + (p.kind === ProfileKind.ProfileKindSSO ? 0 : 1)
  return [...usable].sort((a, b) => rank(a) - rank(b) || a.name.localeCompare(b.name))
}

/** Move the repositories bound to one profile to another profile (existing BindMany). */
export function MoveReposDialog({ from, onClose }: { from: Profile; onClose: () => void }) {
  const { t } = useI18n()
  const { toast } = useActions()
  const { state: accounts } = useAccounts()
  const { state: repoState } = useRepos()
  const repos = useMemo(() => reposBoundTo(repoState, from.name), [repoState, from.name])
  const targets = useMemo(() => moveTargets(accounts?.profiles ?? [], from), [accounts, from])
  const [target, setTarget] = useState<string>(() => targets[0]?.name ?? '')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<UiError | null>(null)

  const chosen = targets.find((p) => p.name === target)
  const otherAccount = !!chosen && !!from.accountId && !!chosen.accountId && chosen.accountId !== from.accountId

  const move = async () => {
    if (!chosen || repos.length === 0) return
    setBusy(true)
    setError(null)
    try {
      const out = await Repos.BindMany(repos.map((r) => ({ repoId: r.id, profile: chosen.name })))
      const ok = (out ?? []).filter((o) => o.ok).length
      const failed = repos.length - ok
      if (ok > 0) toast({ kind: failed ? 'warn' : 'ok', title: t('card.moveRepos.done', { n: ok, to: chosen.name }), body: failed ? t('card.moveRepos.partial', { n: failed }) : undefined })
      else toast({ kind: 'danger', title: t('card.moveRepos.partial', { n: failed }) })
      onClose()
    } catch (e) {
      setError(toUiError(e))
      setBusy(false)
    }
  }

  return (
    <Modal
      open
      size="lg"
      title={t('card.moveRepos.title', { from: from.name, n: repos.length })}
      onClose={onClose}
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button variant="primary" loading={busy} disabled={!chosen || repos.length === 0} onClick={() => void move()}>
            {t('card.moveRepos.action', { n: repos.length })}
          </Button>
        </>
      }
    >
      {targets.length === 0 ? (
        <EmptyState title={t('card.moveRepos.none')} />
      ) : (
        <div className="move-repos">
          <div>
            <div className="section-label" id="move-target">
              {t('card.moveRepos.target')}
            </div>
            <div className="move-repos__targets" role="radiogroup" aria-labelledby="move-target">
              {targets.map((p) => {
                const same = !!from.accountId && p.accountId === from.accountId
                return (
                  <label key={p.name} className="move-repos__target" data-acct={p.colorSlot || undefined}>
                    <Radio name="move-target" checked={target === p.name} onChange={() => setTarget(p.name)} />
                    <AcctTile slot={p.colorSlot} monogram={p.monogram} size="sm" />
                    <span className="mono">{p.name}</span>
                    <span className="muted">{p.accountId ?? ''}</span>
                    <span className="spacer" />
                    {same && <Badge kind="ok">{t('card.moveRepos.sameAccount')}</Badge>}
                    <Badge kind={p.kind === ProfileKind.ProfileKindSSO ? 'info' : 'warn'}>{p.kind === ProfileKind.ProfileKindSSO ? 'SSO' : t('legacy.badge')}</Badge>
                  </label>
                )
              })}
            </div>
          </div>
          {otherAccount && (
            <Banner kind="warn">
              {t('card.moveRepos.otherAccount')}
            </Banner>
          )}
          <div>
            <div className="section-label">{t('card.moveRepos.repos')}</div>
            <ul className="move-repos__list">
              {repos.map((r) => (
                <li key={r.id}>
                  <span>{r.name}</span>
                  <span className="mono muted">{r.displayPath}</span>
                </li>
              ))}
            </ul>
          </div>
          <p className="muted" style={{ margin: 0, fontSize: 'var(--fs-12)' }}>
            {t('card.moveRepos.note')}
          </p>
          {error && <ErrorBanner error={error} />}
        </div>
      )}
    </Modal>
  )
}
