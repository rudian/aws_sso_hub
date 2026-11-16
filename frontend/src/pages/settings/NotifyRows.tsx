import { useCallback, useEffect, useState } from 'react'
import { toUiError } from '../../api/errors'
import { errorText, useI18n } from '../../i18n'
import { onEvent } from '../../api/events'
import { Notify } from '../../api/services'
import { NotifyPermission } from '../../api/models'
import type { NotifyState } from '../../api/models'
import { Button } from '../../components/Button'
import { Dot } from '../../components/Dot'
import { useActions } from '../../shell/AppActions'

/** The system's permission to notify. It is read again when the answer arrives and when the window gets focus back (the person may have changed it in System Settings). */
export function useNotifyState(): NotifyState | null {
  const [state, setState] = useState<NotifyState | null>(null)
  const read = useCallback(() => {
    Notify.GetState().then(
      (s) => setState(s),
      () => undefined,
    )
  }, [])
  useEffect(() => {
    read()
    const off = onEvent('notify:state', (s) => setState(s))
    window.addEventListener('focus', read)
    return () => {
      off()
      window.removeEventListener('focus', read)
    }
  }, [read])
  return state
}

/** Design 5.14, "System notifications": one status row, shown only on macOS and only when there is something to say. Allowed: nothing. */
export function NotifyStatusCard({ state, platform }: { state: NotifyState | null; platform: string }) {
  const { t } = useI18n()
  const { toast } = useActions()
  const [busy, setBusy] = useState(false)
  if (!state || platform === 'windows' || !state.available) return null
  if (state.permission === NotifyPermission.NotifyAllowed) return null
  const off = state.permission === NotifyPermission.NotifyOff
  const act = async () => {
    setBusy(true)
    try {
      if (off) await Notify.OpenSystemSettings()
      else await Notify.RequestPermission()
    } catch (e) {
      const text = errorText(toUiError(e), t)
      toast({ kind: 'danger', title: text.title, body: text.body })
    } finally {
      setBusy(false)
    }
  }
  return (
    <section className="card notify-status" aria-labelledby="set-notify">
      <div className="card__header">
        <h2 className="card__title" id="set-notify">
          {t('settings.notify.row.title')}
        </h2>
      </div>
      <div className="srow">
        <div>
          <div className="srow__label notify-status__label">
            <Dot kind={off ? 'warn' : 'off'} />
            <span>{off ? t('settings.notify.off') : t('settings.notify.notAsked')}</span>
          </div>
          {off && <div className="srow__desc">{t('settings.notify.off.hint')}</div>}
        </div>
        <div className="srow__control">
          <Button size="sm" loading={busy} onClick={() => void act()}>
            {off ? t('settings.notify.off.button') : t('settings.notify.notAsked.button')}
          </Button>
        </div>
      </div>
    </section>
  )
}

/** Windows cannot tell whether notifications are off, so a muted help line stands in for the state. */
export function NotifyWindowsHelp({ platform }: { platform: string }) {
  const { t } = useI18n()
  const { toast } = useActions()
  if (platform !== 'windows') return null
  const open = () => {
    Notify.OpenSystemSettings().catch((e) => {
      const text = errorText(toUiError(e), t)
      toast({ kind: 'danger', title: text.title, body: text.body })
    })
  }
  return (
    <p className="notify-help">
      {t('settings.notify.win.help')}{' '}
      <button type="button" className="link-button" onClick={open}>
        {t('settings.notify.win.link')}
      </button>
    </p>
  )
}
