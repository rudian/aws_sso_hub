import clsx from 'clsx'
import { Info } from 'lucide-react'
import { useRef, useState } from 'react'
import type { CSSProperties, ReactNode } from 'react'
import { SessionState } from '../api/models'
import type { SessionStatus } from '../api/models'
import { useI18n } from '../i18n'
import { formatDateTime, formatRelative, formatSessionLength } from '../lib/time'
import { useNow } from '../lib/useNow'
import { Button } from './Button'
import { Dot } from './Dot'
import { Popover } from './Popover'
import { Tooltip } from './Tooltip'
import type { DotKind } from './Dot'

const MIN = 60_000

export interface CountdownProps {
  session: SessionStatus
  /** Identity colour slot of the card (bar colour). */
  slot?: number
  /** The page passes the state-line text (e.g. "已登录，还剩 6 小时 12 分"). It is replaced by the overrun/unknown wording when those apply. */
  label: ReactNode
  /** Opens the session-length dialog (owned by the accounts page). When omitted, the "Adjust length" / "Change" controls are hidden. */
  onChangeLength?: () => void
  className?: string
}

/**
 * Session countdown. Remaining = sessionEndsAt - now, never the ~1 h access token.
 * - sessionKnown && !sessionOverrun: label + bar (width = remaining / sessionMinutes), colour steps normal >= 60 min, warn < 60, danger < 15.
 * - sessionOverrun: wording `session.overrunLong` + link button, no bar.
 * - signed in but not sessionKnown: wording `session.unknown` + info icon with tooltip, no bar.
 * Click the label for the "Sign-in details" popover. Ticks every 30 s and every second in the last minute.
 */
export function Countdown({ session, slot, label, onChangeLength, className }: CountdownProps) {
  const { t, lang } = useI18n()
  const anchor = useRef<HTMLButtonElement>(null)
  const [open, setOpen] = useState(false)

  const endsMs = session.sessionEndsAt ? Date.parse(session.sessionEndsAt) : NaN
  const known = session.sessionKnown && !Number.isNaN(endsMs)
  const lastMinute = known && !session.sessionOverrun && endsMs - Date.now() <= MIN && endsMs - Date.now() > -MIN
  const now = useNow(lastMinute ? 1000 : 30_000)

  const signedIn = session.state === SessionState.SessionActive || session.state === SessionState.SessionExpiring || session.state === SessionState.SessionRefreshable
  const overrun = signedIn && known && session.sessionOverrun
  const unknown = signedIn && !known
  const showBar = known && !session.sessionOverrun && signedIn
  const leftMs = known ? endsMs - now : Infinity
  const level: 'normal' | 'warn' | 'danger' = !known || session.sessionOverrun ? 'normal' : leftMs < 15 * MIN ? 'danger' : leftMs < 60 * MIN ? 'warn' : 'normal'
  const dot: DotKind = signedIn ? (level === 'normal' ? 'ok' : level === 'warn' ? 'warn' : 'danger') : session.state === SessionState.SessionSigningIn ? 'busy' : session.state === SessionState.SessionSignedOut ? 'off' : 'warn'

  const total = Math.max(1, session.sessionMinutes) * MIN
  const pct = showBar ? Math.min(100, Math.max(0, Math.round((Math.max(0, Math.ceil(leftMs / MIN) * MIN) / total) * 100))) : 0
  const text: ReactNode = overrun ? t('session.overrunLong') : unknown ? t('session.unknown') : label

  const dash = '—'
  const fmt = (iso?: string) => (iso ? formatDateTime(iso, lang) : dash)

  return (
    <div className={clsx('countdown', level === 'warn' && 'countdown--low', level === 'danger' && 'countdown--danger', className)} data-acct={slot || undefined}>
      <div className="pass__state">
        <Dot kind={dot} />
        <button ref={anchor} type="button" className="countdown__label" aria-haspopup="dialog" aria-expanded={open} onClick={() => setOpen((o) => !o)}>
          {text}
        </button>
        {unknown && (
          <Tooltip content={t('session.unknown.hint')}>
            <span tabIndex={0} className="countdown__info" role="img" aria-label={t('session.unknown.hint')}>
              <Info size={14} aria-hidden="true" />
            </span>
          </Tooltip>
        )}
      </div>
      {overrun && onChangeLength && (
        <div className="countdown__action-row">
          <button type="button" className="link countdown__action" onClick={onChangeLength}>
            {t('session.overrun.action')}
          </button>
        </div>
      )}
      {showBar && (
        <div className="countdown__track" role="progressbar" aria-valuemin={0} aria-valuemax={100} aria-valuenow={pct} aria-hidden="false" aria-label={typeof label === 'string' ? label : undefined}>
          <div className="countdown__fill" style={{ '--pct': `${pct}%` } as CSSProperties} />
        </div>
      )}
      <span className="sr-only" role="status" aria-live="polite">
        {session.state === SessionState.SessionExpiring ? text : ''}
      </span>
      <Popover anchorRef={anchor} open={open} onClose={() => setOpen(false)} ariaLabel={t('session.details.title')} className="session-details">
        <div className="session-details__title">{t('session.details.title')}</div>
        <dl className="kv">
          <dt className="kv__key">{t('session.details.loginAt')}</dt>
          <dd className="kv__val">{known ? fmt(session.loginAt) : dash}</dd>
          <dt className="kv__key">{t('session.details.endsAt')}</dt>
          <dd className="kv__val">{known ? fmt(session.sessionEndsAt) : dash}</dd>
          <dt className="kv__key">{t('session.details.length')}</dt>
          <dd className="kv__val">
            {formatSessionLength(session.sessionMinutes, lang)}
            {onChangeLength && (
              <Button
                variant="ghost"
                size="sm"
                onClick={() => {
                  setOpen(false)
                  onChangeLength()
                }}
              >
                {t('session.details.change')}
              </Button>
            )}
          </dd>
          <dt className="kv__key">{t('session.details.tokenExpires')}</dt>
          <dd className="kv__val">
            {fmt(session.expiresAt)}
            {session.canRefresh && <span className="muted"> · {t('session.details.autoRenews')}</span>}
          </dd>
          <dt className="kv__key">{t('session.details.lastRenewed')}</dt>
          <dd className="kv__val">{session.issuedAt ? formatRelative(session.issuedAt, now, t) : dash}</dd>
        </dl>
        <p className="session-details__note">{t('session.details.note')}</p>
      </Popover>
    </div>
  )
}
