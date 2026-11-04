import { useRef, useState } from 'react'
import { toUiError } from '../../api/errors'
import type { UiError } from '../../api/errors'
import { Accounts } from '../../api/services'
import type { Profile } from '../../api/models'
import { Banner } from '../../components/Banner'
import { Button } from '../../components/Button'
import { DiffView } from '../../components/DiffView'
import { ErrorBanner } from '../../components/ErrorBanner'
import { Modal } from '../../components/Modal'
import { Skeleton } from '../../components/Skeleton'
import { useI18n } from '../../i18n'
import { useAsync } from '../../lib/useAsync'
import { useActions } from '../../shell/AppActions'
import './accounts.css'

/**
 * Delete a profile: diff preview, a prompt when repositories (or the global Git default) still use it, danger confirm.
 * Focus starts on Cancel.
 */
export function RemoveProfileDialog({ profile, onMove, onClose }: { profile: Profile; onMove: () => void; onClose: () => void }) {
  const { t } = useI18n()
  const { toast, navigate } = useActions()
  const cancelRef = useRef<HTMLButtonElement>(null)
  const preview = useAsync(() => Accounts.PreviewRemoveProfile(profile.name), [profile.name])
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<UiError | null>(null)

  const remove = async () => {
    if (!preview.data) return
    setBusy(true)
    setError(null)
    try {
      await Accounts.RemoveProfile(profile.name, preview.data.baseHash)
      toast({ kind: 'ok', title: t('card.removed', { name: profile.name }) })
      onClose()
    } catch (e) {
      setError(toUiError(e))
      setBusy(false)
      void preview.reload()
    }
  }

  return (
    <Modal
      open
      size="lg"
      title={t('card.remove.confirm.title', { name: profile.name })}
      onClose={onClose}
      initialFocus={cancelRef}
      footer={
        <>
          <Button ref={cancelRef} variant="secondary" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button variant="danger" loading={busy} disabled={!preview.data} onClick={() => void remove()}>
            {t('card.remove.confirm.action')}
          </Button>
        </>
      }
    >
      <div className="stack" style={{ ['--gap' as string]: 'var(--sp-4)' }}>
        <p style={{ margin: 0 }}>{t('card.remove.confirm.body', { name: profile.name, n: profile.boundRepos })}</p>
        {profile.boundRepos > 0 && (
          <Banner
            kind="warn"
            actions={
              <Button
                size="sm"
                onClick={() => {
                  onClose()
                  onMove()
                }}
              >
                {t('card.remove.moveFirst')}
              </Button>
            }
          >
            {t('card.remove.boundRepos', { n: profile.boundRepos, name: profile.name })}
          </Banner>
        )}
        {profile.isGlobalDefault && (
          <Banner
            kind="warn"
            actions={
              <Button
                size="sm"
                onClick={() => {
                  onClose()
                  navigate({ page: 'git' })
                }}
              >
                {t('card.remove.openGit')}
              </Button>
            }
          >
            {t('card.remove.globalDefault', { name: profile.name })}
          </Banner>
        )}
        {preview.error && <ErrorBanner error={preview.error} onRetry={() => void preview.reload()} />}
        {!preview.data && !preview.error && <Skeleton variant="block" height={120} />}
        {preview.data && <DiffView preview={preview.data} />}
        {error && <ErrorBanner error={error} />}
      </div>
    </Modal>
  )
}
