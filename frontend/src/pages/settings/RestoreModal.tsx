import { useState } from 'react'
import { toUiError } from '../../api/errors'
import type { UiError } from '../../api/errors'
import { System } from '../../api/services'
import { ErrorCode } from '../../api/models'
import type { BackupEntry } from '../../api/models'
import { Banner } from '../../components/Banner'
import { Button } from '../../components/Button'
import { DiffView } from '../../components/DiffView'
import { ErrorBanner } from '../../components/ErrorBanner'
import { Modal } from '../../components/Modal'
import { Skeleton } from '../../components/Skeleton'
import { errorText, useI18n } from '../../i18n'
import { formatDateTime } from '../../lib/time'
import { useAsync } from '../../lib/useAsync'
import { useActions } from '../../shell/AppActions'
import './settings.css'

/** Preview a backup against the current file, then restore it. The current file is backed up first by the backend. */
export function RestoreModal({ backupId, onClose }: { backupId: string; onClose: (restored: BackupEntry | null) => void }) {
  const { t, lang } = useI18n()
  const { toast } = useActions()
  const preview = useAsync(() => System.PreviewRestore(backupId), [backupId])
  const [busy, setBusy] = useState(false)
  // Set when the config file changed after the preview was taken: the user has to look at the new diff first.
  const [changed, setChanged] = useState<UiError | null>(null)
  const changedText = changed ? errorText(changed, t) : null

  // useAsync keeps the previous result while it reloads. Only a result that finished loading without an error is
  // shown and can be restored; during "Review again" the old diff and the old list of lost keys are hidden.
  const fresh = !preview.loading && !preview.error ? preview.data : null
  const entry = preview.data?.entry
  const lost = fresh?.lostProfiles ?? []
  const unchanged = !!fresh && fresh.preview.added === 0 && fresh.preview.removed === 0

  const restore = async () => {
    if (!entry || !fresh) return
    setBusy(true)
    try {
      const safety = await System.RestoreBackup(entry.id, lost.length > 0)
      toast({ kind: 'ok', title: t('settings.backups.restore.done', { file: entry.file }) })
      onClose(safety)
    } catch (e) {
      const ue = toUiError(e)
      if (ue.code === ErrorCode.ErrConfigChanged) {
        setChanged(ue)
      } else {
        const text = errorText(ue, t)
        toast({ kind: 'danger', title: text.title, body: text.body })
      }
      setBusy(false)
    }
  }

  const reviewAgain = () => {
    setChanged(null)
    void preview.reload()
  }

  return (
    <Modal
      open
      size="lg"
      dismissible={!busy}
      onClose={() => onClose(null)}
      title={entry ? t('settings.backups.restore.title', { time: formatDateTime(entry.createdAt, lang), file: entry.file }) : t('settings.backups.restore.action')}
      footer={
        <>
          <Button variant="ghost" onClick={() => onClose(null)} disabled={busy}>
            {t('common.cancel')}
          </Button>
          <Button variant={lost.length > 0 ? 'danger' : 'primary'} loading={busy} disabled={!fresh || !!changed} onClick={() => void restore()}>
            {t('settings.backups.restore.action')}
          </Button>
        </>
      }
    >
      <p className="restore__none">{t('settings.backups.restore.body')}</p>
      {changed && changedText && (
        <Banner
          kind="danger"
          title={changedText.title}
          className="restore__warn"
          actions={
            <Button size="sm" onClick={reviewAgain}>
              {t('common.reviewAgain')}
            </Button>
          }
        >
          {changedText.body && changedText.body !== changedText.title && <div>{changedText.body}</div>}
        </Banner>
      )}
      {lost.length > 0 && (
        <Banner kind="warn" className="restore__warn">
          {t('settings.backups.redactedWarn', { profiles: lost.join(', ') })}
        </Banner>
      )}
      {preview.error ? (
        <ErrorBanner error={preview.error} onRetry={() => void preview.reload()} />
      ) : !fresh ? (
        <div aria-busy="true">
          <Skeleton variant="block" height={120} />
        </div>
      ) : unchanged ? (
        <p className="restore__none">{t('settings.backups.noChanges')}</p>
      ) : (
        <DiffView preview={fresh.preview} />
      )}
    </Modal>
  )
}
