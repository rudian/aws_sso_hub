import clsx from 'clsx'
import { ProfileKind, SessionState } from '../api/models'
import type { Portal } from '../api/models'
import { Dot } from '../components/Dot'
import type { DotKind } from '../components/Dot'
import { Tooltip } from '../components/Tooltip'
import { useI18n } from '../i18n'
import { useNavigate } from '../lib/route'
import { motionReduced } from '../lib/motion'
import { formatKeyringTime, formatUntil, remainingMs } from '../lib/time'
import { useNow } from '../lib/useNow'
import { useAccounts } from '../state/accounts'
import { useActions } from './AppActions'

interface Row {
  key: string
  name: string
  dot: DotKind
  time?: string
  warn?: boolean
  tooltip?: string
  onClick: () => void
}

function dotFor(state: SessionState): DotKind {
  switch (state) {
    case SessionState.SessionActive:
      return 'ok'
    case SessionState.SessionSignedOut:
      return 'off'
    case SessionState.SessionSigningIn:
      return 'busy'
    case SessionState.SessionError:
      return 'danger'
    default:
      return 'warn'
  }
}

/** One row per portal plus one per static-key profile. Time = session remaining (h:mm / 48m). */
export function Keyring() {
  const { t, lang } = useI18n()
  const { state } = useAccounts()
  const { openLogin } = useActions()
  const navigate = useNavigate()
  const now = useNow(30_000)

  const portals: Portal[] = (state?.portals ?? []).filter((p) => !p.draft && !p.legacyFormat)
  const statics = (state?.profiles ?? []).filter((p) => p.kind === ProfileKind.ProfileKindStatic)

  const rows: Row[] = portals.map((p): Row => {
    const s = p.session
    const signedIn = s.state === SessionState.SessionActive || s.state === SessionState.SessionExpiring || s.state === SessionState.SessionRefreshable
    const needsLogin = s.state === SessionState.SessionExpired || s.state === SessionState.SessionSignedOut || s.state === SessionState.SessionError
    const row: Row = {
      key: `portal:${p.name}`,
      name: p.name,
      dot: dotFor(s.state),
      onClick: () => {
        if (needsLogin) void openLogin(p.name)
        else {
          navigate({ page: 'accounts' })
          window.setTimeout(() => document.querySelector(`[data-portal="${CSS.escape(p.name)}"]`)?.scrollIntoView({ block: 'start', behavior: motionReduced() ? 'auto' : 'smooth' }), 50)
        }
      },
    }
    if (signedIn && s.sessionKnown && s.sessionEndsAt && !s.sessionOverrun) {
      row.time = formatKeyringTime(s.sessionEndsAt, now)
      row.warn = remainingMs(s.sessionEndsAt, now) < 60 * 60_000
      row.tooltip = t('keyring.tooltip.active', { time: formatUntil(s.sessionEndsAt, now, lang) })
    } else if (signedIn && s.sessionKnown && s.sessionOverrun) {
      row.tooltip = t('session.overrunLong')
    } else if (signedIn) {
      // session length unknown: a dot with a tooltip, no countdown.
      row.tooltip = t('session.unknown')
    } else if (s.state === SessionState.SessionExpired) {
      row.time = t('keyring.expired')
      row.warn = true
      row.tooltip = t('keyring.tooltip.expired')
    } else if (s.state === SessionState.SessionSignedOut) {
      row.time = t('keyring.signedOut')
      row.tooltip = t('keyring.tooltip.signedOut')
    } else if (s.state === SessionState.SessionSigningIn) {
      row.time = t('keyring.signingIn')
    } else if (s.state === SessionState.SessionError) {
      row.time = t('keyring.error')
      row.warn = true
    }
    return row
  })

  for (const p of statics) {
    rows.push({
      key: `static:${p.name}`,
      name: p.name,
      dot: 'warn',
      time: t('keyring.key'),
      warn: true,
      tooltip: t('keyring.tooltip.key'),
      onClick: () => navigate({ page: 'migrate', profile: p.name }),
    })
  }

  return (
    <div className="keyring" role="list">
      {rows.map((r) => (
        <Tooltip key={r.key} content={r.tooltip}>
          <button type="button" role="listitem" className="keyring__item" onClick={r.onClick}>
            <Dot kind={r.dot} />
            <span className="truncate">{r.name}</span>
            <span className={clsx('keyring__time', r.warn && 'keyring__time--warn')}>{r.time ?? ''}</span>
          </button>
        </Tooltip>
      ))}
    </div>
  )
}
