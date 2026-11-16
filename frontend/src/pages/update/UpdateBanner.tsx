// The quiet banner under the Projects header (design 5.13): "Codepass 1.1.0 is available · you are on 1.0.0".
// Dismissible for this version. The Projects page renders <UpdateBanner /> once; it also hosts the dialog.
import { ArrowUp, X } from 'lucide-react'
import { Button } from '../../components/Button'
import { IconButton } from '../../components/IconButton'
import { Spinner } from '../../components/Spinner'
import { useI18n } from '../../i18n'
import { bannerKind, percent } from './model'
import { UpdateDialog } from './UpdateDialog'
import { useReopenWatch } from './reopenWatch'
import { dismissBanner, openUpdateDialog, updateActions, useDismissedVersion, useUpdateState } from './useUpdate'

export function UpdateBanner() {
  const { t } = useI18n()
  const s = useUpdateState()
  const watch = useReopenWatch()
  const dismissedVersion = useDismissedVersion()
  const kind = bannerKind(s)
  const dismissed = kind === 'available' && !!s.version && dismissedVersion === s.version
  if (!kind || dismissed) return <UpdateDialog />
  const v = s.version ?? ''

  let text: React.ReactNode
  let actions: React.ReactNode
  switch (kind) {
    case 'available':
      text = (
        <>
          <b>{t('update.banner.available', { version: v })}</b> · {t('update.banner.current', { current: s.current })}
        </>
      )
      actions = (
        <>
          <Button variant="ghost" size="sm" onClick={openUpdateDialog}>
            {t('update.banner.notes')}
          </Button>
          <Button variant="primary" size="sm" onClick={updateActions.start}>
            {t('update.banner.update')}
          </Button>
          <IconButton label={t('update.banner.dismiss')} icon={X} size="sm" onClick={() => dismissBanner(v)} />
        </>
      )
      break
    case 'downloading':
      text = (
        <>
          <Spinner /> <b>{s.phase === 'verifying' ? t('update.banner.verifying') : t('update.banner.downloading', { pct: percent(s) })}</b>
        </>
      )
      actions = (
        <Button size="sm" onClick={openUpdateDialog}>
          {t('update.banner.view')}
        </Button>
      )
      break
    case 'ready':
      text = (
        <>
          <b>{t('update.banner.ready', { version: v })}</b>
          {watch.mode === 'countdown' ? ` · ${t('update.banner.countdown', { s: watch.secondsLeft })}` : watch.mode === 'waiting' ? ` · ${t('update.banner.waiting')}` : ''}
        </>
      )
      actions =
        watch.mode !== 'off' ? (
          <Button size="sm" onClick={updateActions.cancelWait}>
            {t('update.action.cancelWait')}
          </Button>
        ) : (
          <Button variant="primary" size="sm" onClick={openUpdateDialog}>
            {t('update.banner.reopen')}
          </Button>
        )
      break
    case 'restarting':
      text = <b>{t('update.banner.restarting')}</b>
      break
    case 'failed':
      text = <b>{t('update.banner.failed', { version: v })}</b>
      actions = (
        <Button size="sm" onClick={openUpdateDialog}>
          {t('update.banner.details')}
        </Button>
      )
      break
  }

  return (
    <>
      <div className={kind === 'failed' ? 'update-banner update-banner--failed' : 'update-banner'} role="region" aria-label={t('update.banner.aria')}>
        <span className="update-banner__mark">
          <ArrowUp aria-hidden="true" />
        </span>
        <span className="update-banner__text">{text}</span>
        <span className="spacer" />
        {actions && <span className="update-banner__actions">{actions}</span>}
      </div>
      <UpdateDialog />
    </>
  )
}
