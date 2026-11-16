import { useMemo, useState } from 'react'
import { GitGlobal } from '../../api'
import type { BindOutcome, URLRulePlan } from '../../api/models'
import { Banner, Button, DiffView, Modal } from '../../components'
import { useI18n } from '../../i18n'
import { RepoName } from '../../components/RepoName'
import { useAccounts } from '../../state/accounts'
import { useActions } from '../../shell/AppActions'
import { ProfileSelect } from '../repos/ProfileSelect'
import { bindableProfiles } from '../repos/model'
import { useReportError } from '../repos/hooks'

export interface UrlRuleModalProps {
  plan: URLRulePlan
  onClose: () => void
  onDone: () => void
}

/** Moves every local clone matched by a global URL rule to its own binding, then removes the rule. */
export function UrlRuleModal({ plan, onClose, onDone }: UrlRuleModalProps) {
  const { t } = useI18n()
  const { state: accounts } = useAccounts()
  const { toast } = useActions()
  const report = useReportError()
  const choices = useMemo(() => bindableProfiles(accounts?.profiles ?? []), [accounts])
  const repos = plan.repos ?? []
  // Several clones of one URL: the user decides for each. Otherwise the rule's own profile is the default.
  const [pick, setPick] = useState<Record<string, string>>(() => Object.fromEntries(repos.map((r) => [r.id, plan.duplicateUrls ? '' : plan.profile])))
  const [busy, setBusy] = useState(false)
  const missing = repos.filter((r) => !pick[r.id]).length

  const submit = async () => {
    setBusy(true)
    try {
      const res = await GitGlobal.MigrateURLRule({ pattern: plan.pattern, assignments: repos.map((r) => ({ repoId: r.id, profile: pick[r.id] })) }, plan.preview.baseHash)
      const outcomes: BindOutcome[] = res.outcomes ?? []
      if (res.ruleRemoved) {
        toast({ kind: 'ok', title: t('git.url.done', { n: outcomes.length }) })
      } else {
        toast({ kind: 'warn', title: t('git.url.partial', { ok: outcomes.filter((o) => o.ok).length, failed: outcomes.filter((o) => !o.ok).length }) })
      }
      onDone()
    } catch (e) {
      report(e)
    } finally {
      setBusy(false)
    }
  }

  return (
    <Modal
      open
      size="lg"
      title={t('git.url.modal.title')}
      subtitle={<span className="mono">{plan.pattern}</span>}
      dismissible={!busy}
      onClose={onClose}
      footer={
        <>
          <Button variant="secondary" onClick={onClose} disabled={busy}>
            {t('common.cancel')}
          </Button>
          <Button variant="primary" loading={busy} disabled={missing > 0} onClick={() => void submit()}>
            {t('git.url.modal.action')}
          </Button>
        </>
      }
    >
      <div className="stack" style={{ '--gap': 'var(--sp-4)' } as React.CSSProperties}>
        <p style={{ margin: 0 }}>{t('git.url.toLocal.confirm', { n: repos.length })}</p>
        {plan.duplicateUrls && <Banner kind="warn">{t('git.url.modal.duplicate')}</Banner>}
        {repos.length === 0 ? (
          <Banner kind="info">{t('git.url.modal.noRepos')}</Banner>
        ) : (
          <div>
            <div className="field__label" style={{ marginBottom: 6 }}>
              {t('git.url.modal.assign')}
            </div>
            <div className="card list" role="list">
              {repos.map((r) => (
                <div key={r.id} className="list__item" role="listitem">
                  <div style={{ minWidth: 0, flex: 1 }}>
                    <div className="row__primary truncate">
                      <RepoName localName={r.localName} remoteName={r.remoteName} />
                    </div>
                    <div className="row__secondary mono truncate">{r.displayPath}</div>
                  </div>
                  <div style={{ width: 260 }}>
                    <ProfileSelect aria-label={r.name} profiles={choices} value={pick[r.id] ?? ''} onChange={(v) => setPick((cur) => ({ ...cur, [r.id]: v }))} placeholder={t('git.url.modal.choose')} />
                  </div>
                </div>
              ))}
            </div>
          </div>
        )}
        <div>
          <div className="field__label" style={{ marginBottom: 6 }}>
            {t('git.url.modal.removal')}
          </div>
          <DiffView preview={plan.preview} />
        </div>
      </div>
    </Modal>
  )
}
