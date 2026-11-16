import { useMemo, useState } from 'react'
import { Repos } from '../../api'
import type { BindOutcome, Repo } from '../../api/models'
import { Button, Field, Modal } from '../../components'
import { useI18n } from '../../i18n'
import { useAccounts } from '../../state/accounts'
import { ProfileSelect } from './ProfileSelect'
import { bindableProfiles, suggestedProfile } from './model'
import { useReportError } from './hooks'
import { useRepoSentence } from './RepoSentence'

export interface HttpsModalProps {
  repo: Repo
  onClose: () => void
  onDone: (outcome: BindOutcome, profile: string) => void
}

/** SSH -> HTTPS: shows the old and the new origin URL and binds the repository in the same step. */
export function HttpsModal({ repo, onClose, onDone }: HttpsModalProps) {
  const { t } = useI18n()
  const sentence = useRepoSentence()
  const { state: accounts } = useAccounts()
  const report = useReportError()
  const choices = useMemo(() => bindableProfiles(accounts?.profiles ?? []), [accounts])
  const [profile, setProfile] = useState(suggestedProfile(repo) ?? '')
  const [busy, setBusy] = useState(false)

  const submit = async () => {
    setBusy(true)
    try {
      onDone(await Repos.ConvertToHTTPS(repo.id, profile), profile)
    } catch (e) {
      report(e)
    } finally {
      setBusy(false)
    }
  }

  return (
    <Modal
      open
      size="sm"
      title={sentence.node('repos.https.title', repo)}
      dismissible={!busy}
      onClose={onClose}
      footer={
        <>
          <Button variant="secondary" onClick={onClose} disabled={busy}>
            {t('common.cancel')}
          </Button>
          <Button variant="primary" loading={busy} disabled={!profile} onClick={() => void submit()}>
            {t('repos.https.action')}
          </Button>
        </>
      }
    >
      <div className="stack" style={{ '--gap': 'var(--sp-4)' } as React.CSSProperties}>
        <p style={{ margin: 0 }}>{t('repos.https.body')}</p>
        <div className="kv">
          <span className="kv__key">{t('repos.https.old')}</span>
          <span className="kv__val" style={{ wordBreak: 'break-all' }}>
            {repo.remoteUrl}
          </span>
          <span className="kv__key">{t('repos.https.new')}</span>
          <span className="kv__val" style={{ wordBreak: 'break-all' }}>
            {repo.httpsUrl}
          </span>
        </div>
        <Field label={t('repos.https.profile')}>
          <ProfileSelect profiles={choices} value={profile} onChange={setProfile} placeholder={t('repos.bind.choose')} />
        </Field>
      </div>
    </Modal>
  )
}
