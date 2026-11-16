import { useEffect, useMemo, useState } from 'react'
import { toUiError } from '../../api/errors'
import type { BindItem, BindOutcome, Repo } from '../../api/models'
import { Repos } from '../../api'
import { AcctChip, Banner, Button, Dot, Modal } from '../../components'
import { RepoName } from '../../components/RepoName'
import { errorText, useI18n } from '../../i18n'
import { useAccounts } from '../../state/accounts'
import { ProfileSelect } from './ProfileSelect'
import { bindableProfiles, bulkSummary, suggestedProfile } from './model'
import { useReportError } from './hooks'

export interface BulkBindModalProps {
  /** review: every row has its own profile chooser. suggestions: fixed list, one confirm (bind all suggestions). */
  mode: 'review' | 'suggestions'
  repos: Repo[]
  /** suggestions mode: the plan to confirm. */
  items?: BindItem[]
  /** suggestions mode: repositories left out because several accounts have them. */
  skipped?: Repo[]
  onClose: () => void
}

/** Binds several repositories in one go, then lists the result of every row. */
export function BulkBindModal({ mode, repos, items: planned, skipped, onClose }: BulkBindModalProps) {
  const { t } = useI18n()
  const { state: accounts } = useAccounts()
  const report = useReportError()
  const profiles = accounts?.profiles ?? []
  const choices = useMemo(() => bindableProfiles(profiles), [profiles])
  const [pick, setPick] = useState<Record<string, string>>(() => {
    const init: Record<string, string> = {}
    for (const r of repos) {
      const fixed = planned?.find((i) => i.repoId === r.id)?.profile
      const s = fixed ?? suggestedProfile(r)
      if (s) init[r.id] = s
    }
    return init
  })
  // Owner detection may finish while the dialog is open: fill in suggestions for rows without a choice.
  useEffect(() => {
    setPick((cur) => {
      let next = cur
      for (const r of repos) {
        const s = suggestedProfile(r)
        if (s && !cur[r.id]) next = { ...next, [r.id]: s }
      }
      return next
    })
  }, [repos])
  const [busy, setBusy] = useState(false)
  const [results, setResults] = useState<BindOutcome[] | null>(null)

  const rows = mode === 'suggestions' ? repos.filter((r) => planned?.some((i) => i.repoId === r.id)) : repos
  const items: BindItem[] = rows.filter((r) => pick[r.id]).map((r) => ({ repoId: r.id, profile: pick[r.id] }))
  const missing = rows.length - items.length

  const submit = async () => {
    setBusy(true)
    try {
      setResults((await Repos.BindMany(items)) ?? [])
    } catch (e) {
      report(e)
    } finally {
      setBusy(false)
    }
  }

  const resultOf = (id: string) => results?.find((o) => o.repoId === id)
  const title = results ? t('repos.bulk.title') : mode === 'suggestions' ? t('repos.bindAll.confirm.title', { n: rows.length }) : t('repos.bulk.title')

  return (
    <Modal
      open
      size="lg"
      title={title}
      dismissible={!busy}
      onClose={onClose}
      footer={
        results ? (
          <Button variant="primary" onClick={onClose}>
            {t('common.done')}
          </Button>
        ) : (
          <>
            <Button variant="secondary" onClick={onClose} disabled={busy}>
              {t('common.cancel')}
            </Button>
            <Button variant="primary" loading={busy} disabled={items.length === 0 || missing > 0} onClick={() => void submit()}>
              {t('repos.bulk.action', { n: items.length })}
            </Button>
          </>
        )
      }
    >
      <div className="stack" style={{ '--gap': 'var(--sp-4)' } as React.CSSProperties}>
        {!results && items.length > 0 && <div style={{ fontWeight: 500 }}>{bulkSummary(items, profiles, t)}</div>}
        {!results && missing > 0 && <Banner kind="warn">{t('repos.bulk.needChoice', { n: items.length, missing })}</Banner>}
        {!results && mode === 'suggestions' && (skipped?.length ?? 0) > 0 && <Banner kind="info">{t('repos.bindAll.confirm.skipped', { n: skipped?.length ?? 0 })}</Banner>}
        <div className="card list" role="list">
          {rows.map((r) => {
            const p = profiles.find((x) => x.name === pick[r.id])
            const out = resultOf(r.id)
            return (
              <div key={r.id} className="list__item" role="listitem" style={{ alignItems: 'center' }}>
                <div style={{ minWidth: 0, flex: 1 }}>
                  <div className="row__primary truncate">
                    <RepoName localName={r.localName} remoteName={r.remoteName} />
                  </div>
                  <div className="row__secondary mono truncate">{r.displayPath}</div>
                </div>
                {results ? (
                  <div className="hint" style={{ display: 'flex', gap: 8, alignItems: 'center', maxWidth: 260 }}>
                    <Dot kind={out?.ok ? 'ok' : 'danger'} />
                    <span>{out?.ok ? t('repos.bulk.rowDone', { name: pick[r.id] }) : out?.error ? errorText(toUiError(out.error), t).title : ''}</span>
                  </div>
                ) : mode === 'suggestions' ? (
                  p ? <AcctChip slot={p.colorSlot} monogram={p.monogram} name={p.name} accountId={p.accountId} /> : null
                ) : (
                  <div style={{ width: 260, display: 'flex', flexDirection: 'column', gap: 4 }}>
                    <ProfileSelect aria-label={r.name} profiles={choices} value={pick[r.id] ?? ''} onChange={(v) => setPick((cur) => ({ ...cur, [r.id]: v }))} placeholder={t('repos.bind.choose')} />
                  </div>
                )}
              </div>
            )
          })}
        </div>
      </div>
    </Modal>
  )
}
