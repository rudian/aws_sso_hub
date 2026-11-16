// The "Updates" card of Settings (design 5.14): current version, when it was last checked, "Check now", and the
// daily-check switch. SettingsPage renders <UpdatesCard /> in its card grid.
import { RefreshCw } from 'lucide-react'
import { useEffect, useState } from 'react'
import { toUiError } from '../../api/errors'
import { UpdatePhase } from '../../api/models'
import { Updates } from '../../api/services'
import { Button } from '../../components/Button'
import { Switch } from '../../components/Switch'
import { errorText, useI18n } from '../../i18n'
import { formatRelative } from '../../lib/time'
import { useNow } from '../../lib/useNow'
import { useActions } from '../../shell/AppActions'
import { UpdateDialog } from '../update/UpdateDialog'
import { openUpdateDialog, updateActions, useUpdateState } from '../update/useUpdate'
import { bannerKind, canCheck } from '../update/model'
import { blockedText, failureText } from '../update/UpdatePanel'
import { guessPlatform } from '../../lib/platform'
import { useAppInfo } from '../../state/appInfo'

export function UpdatesCard() {
  const { t } = useI18n()
  const { toast } = useActions()
  const s = useUpdateState()
  const now = useNow(30_000)
  const info = useAppInfo()
  const [auto, setAuto] = useState<boolean | null>(null)

  useEffect(() => {
    let alive = true
    Updates.GetPreferences().then(
      (p) => alive && setAuto(p.autoCheck),
      () => alive && setAuto(true),
    )
    return () => {
      alive = false
    }
  }, [])

  const save = async (on: boolean) => {
    const before = auto
    setAuto(on)
    try {
      setAuto((await Updates.SetAutoCheck(on)).autoCheck)
    } catch (e) {
      setAuto(before)
      toast({ kind: 'danger', title: t('update.settings.saveFailed'), body: errorText(toUiError(e), t).title })
    }
  }

  const checking = s.phase === UpdatePhase.UpdateChecking
  const offered = bannerKind(s) !== null
  const blocked = s.phase === UpdatePhase.UpdateBlocked
  const failedCheck = s.phase === UpdatePhase.UpdateError || s.phase === UpdatePhase.UpdateRefused
  // A blocked copy stopped before any network call, so it has no "checked ... from GitHub" time.
  const when = s.checkedAt && !blocked ? t('update.settings.versionHelp', { when: formatRelative(s.checkedAt, now, t) }) : blocked ? '' : t('update.settings.versionNever')
  let status = ''
  let tone: 'danger' | 'warn' | undefined
  if (s.phase === UpdatePhase.UpdateUpToDate) status = t('update.status.upToDate', { current: s.current })
  else if (failedCheck) {
    status = failureText(s, t).title
    tone = 'danger'
  } else if (blocked) {
    status = [blockedText(s, t, info?.platform ?? guessPlatform()), s.block === 'translocated' && s.folder ? t('update.blocked.folder', { folder: s.folder }) : ''].filter(Boolean).join(' ')
    tone = 'warn'
  } else if (offered && s.version) status = t('update.status.available', { version: s.version })

  return (
    <section className="card" aria-labelledby="set-updates">
      <div className="card__header">
        <h2 className="card__title" id="set-updates">
          {t('update.settings.title')}
        </h2>
      </div>
      <div className="srow">
        <div>
          <div className="srow__label">
            {t('update.settings.version')} <span className="mono">{s.current}</span>
          </div>
          <div className="srow__desc">
            {/* only the status text is a live region: the relative "Last checked" time changes every minute */}
            <span role="status" style={tone ? { color: `var(--${tone})` } : undefined}>
              {status}
            </span>
            {status && when ? ' · ' : ''}
            {when}
          </div>
        </div>
        <div className="srow__control">
          {offered && (
            <Button variant="primary" onClick={openUpdateDialog}>
              {t('update.action.details')}
            </Button>
          )}
          <Button icon={RefreshCw} loading={checking} disabled={!canCheck(s)} onClick={() => void updateActions.check()}>
            {t('update.action.check')}
          </Button>
        </div>
      </div>
      <div className="srow">
        <div>
          <div className="srow__label">{t('update.settings.auto')}</div>
          <div className="srow__desc">{t('update.settings.autoHelp')}</div>
        </div>
        <div className="srow__control">
          <Switch checked={auto ?? true} disabled={auto === null} onChange={(on) => void save(on)} label={t('update.settings.autoLabel')} />
        </div>
      </div>
      <UpdateDialog />
    </section>
  )
}
