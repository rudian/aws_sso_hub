import clsx from 'clsx'
import { useId, useState } from 'react'
import type { ReactNode } from 'react'
import { ChevronRight, Clock, Globe, LogOut, MoreHorizontal, Plus } from 'lucide-react'
import { copyText } from '../../api/platform'
import { Accounts } from '../../api/services'
import type { Portal, Profile } from '../../api/models'
import { AcctTile } from '../../components/AcctTile'
import { Badge } from '../../components/Badge'
import { Button } from '../../components/Button'
import { Dot } from '../../components/Dot'
import { IconButton } from '../../components/IconButton'
import { Menu } from '../../components/Menu'
import { useI18n } from '../../i18n'
import { formatRemaining } from '../../lib/time'
import { useNow } from '../../lib/useNow'
import { useActions } from '../../shell/AppActions'
import { Gated, useErrorToast, useSignOut } from './hooks'
import type { DialogState } from './hooks'
import { endIso, isSignedIn, pillState, pillText } from './model'
import { PassCard } from './PassCard'
import './accounts.css'

/** The session pill in a portal header (7 states). */
export function SessionPill({ portal }: { portal: Portal }) {
  const { t } = useI18n()
  const now = useNow(30_000)
  const s = portal.session
  const state = pillState(s)
  const warn = state === 'expiring' || state === 'refreshable' || state === 'expired'
  const danger = state === 'error'
  // "Signed in, **6 h 12 min left**": only the remaining time is emphasised
  const end = endIso(s)
  let boldRemaining: ReactNode = null
  if (state === 'active' && s.sessionKnown && !s.sessionOverrun && end) {
    const [before, after] = t('session.active', { remaining: '\u0001' }).split('\u0001')
    // One element, so the pill's flex gap only separates the dot from the text.
    boldRemaining = (
      <span>
        {before}
        <b>{formatRemaining(end, now, t)}</b>
        {after}
      </span>
    )
  }
  return (
    <span className={clsx('session-pill', warn && 'session-pill--warn', danger && 'session-pill--danger')} role="status">
      {state === 'active' && <Dot kind="ok" />}
      {state === 'expiring' && <Dot kind="warn" />}
      {state === 'refreshable' && <Dot kind="warn" />}
      {state === 'expired' && <Clock size={14} aria-hidden="true" />}
      {state === 'signed-out' && <Dot kind="off" />}
      {state === 'signing-in' && <Dot kind="busy" />}
      {state === 'error' && <Dot kind="danger" />}
      {boldRemaining ?? pillText(s, now, t)}
    </span>
  )
}

interface PortalSectionProps {
  portal: Portal
  profiles: Profile[]
  readOnly: boolean
  onDialog: (d: DialogState) => void
  collapsed?: boolean
  onToggle?: () => void
}

const PEEK = 8

/** One portal: header with session pill, sign-out and menu, a grid of passes, and the dashed "add account or role" tile. */
export function PortalSection({ portal, profiles, readOnly, onDialog, collapsed = false, onToggle }: PortalSectionProps) {
  const gridId = useId()
  const titleId = useId()
  const [revealed, setRevealed] = useState(false) // animate only after the user toggled
  const toggle = () => {
    setRevealed(true)
    onToggle?.()
  }
  const { t } = useI18n()
  const { toast } = useActions()
  const failToast = useErrorToast()
  const signOut = useSignOut()
  const s = portal.session
  const mine = (portal.profiles ?? []).map((n) => profiles.find((p) => p.name === n)).filter((p): p is Profile => !!p)

  const copyUrl = async () => {
    try {
      await copyText(portal.startUrl)
      toast({ kind: 'ok', title: t('portal.urlCopied') })
    } catch {
      /* clipboard unavailable */
    }
  }
  const items = [
    { id: 'copyUrl', label: t('portal.menu.copyUrl'), onSelect: () => void copyUrl() },
    { id: 'open', label: t('portal.menu.openPortal'), onSelect: () => void Accounts.OpenPortal(portal.name).catch(failToast) },
    ...(portal.legacyFormat
      ? []
      : [
          { id: 'addRole', label: t('portal.menu.addRole'), separatorBefore: true, disabled: readOnly, onSelect: () => onDialog({ kind: 'addRole', portal: portal.name }) },
          { id: 'rename', label: t('portal.menu.rename'), disabled: readOnly, onSelect: () => onDialog({ kind: 'rename', portal: portal.name }) },
          { id: 'remove', label: t('portal.menu.remove'), danger: true, separatorBefore: true, disabled: readOnly, onSelect: () => onDialog({ kind: 'removePortal', portal: portal.name }) },
        ]),
  ]

  return (
    <section className={clsx('portal-panel', collapsed && 'is-collapsed')} id={`portal-${portal.name}`} data-portal={portal.name} aria-label={t('accounts.section.portalAria', { name: portal.name })}>
      <div className="portal__header">
        <button type="button" className="portal__toggle" aria-expanded={!collapsed} aria-controls={gridId} aria-labelledby={titleId} onClick={toggle}>
          <ChevronRight className="portal__chevron" aria-hidden="true" />
          <span className="portal__icon">
            <Globe aria-hidden="true" />
          </span>
        </button>
        <div className="portal__id">
          <h2 className="portal__title" id={titleId}>
            {portal.legacyFormat ? portal.startUrlHost.split('/')[0] : portal.name}
            {portal.legacyFormat && (
              <Badge kind="neutral" className="portal__legacy">
                {t('portal.legacyFormat')}
              </Badge>
            )}
          </h2>
          <div className="portal__meta">
            <span className="mono">{portal.startUrlHost}</span>
            <span>{portal.region}</span>
          </div>
        </div>
        <div className="portal__tools">
          <SessionPill portal={portal} />
          {isSignedIn(s) && (
            <Button variant="ghost" size="sm" icon={LogOut} onClick={() => void signOut(portal)}>
              {t('session.signOut')}
            </Button>
          )}
          <Menu align="end" trigger={<IconButton label={t('tip.portalOptions')} icon={MoreHorizontal} />} items={items} />
        </div>
      </div>
      {collapsed && (
        <div className="portal__summary">
          {mine.length > 0 && (
            <span className="portal__peek" aria-hidden="true">
              {mine.slice(0, PEEK).map((p) => (
                <span key={p.name} title={p.name}>
                  <AcctTile size="sm" slot={p.colorSlot} monogram={p.monogram} />
                </span>
              ))}
              {mine.length > PEEK && <span className="portal__peek-more">+{mine.length - PEEK}</span>}
            </span>
          )}
          <span>{t('portal.summary', { n: mine.length })}</span>
        </div>
      )}
      <div className={clsx('portal__grid', revealed && 'is-revealed')} id={gridId} hidden={collapsed}>
        {!collapsed && (
          <>
            {mine.map((p) => (
              <PassCard key={p.name} profile={p} portal={portal} readOnly={readOnly} onDialog={onDialog} />
            ))}
            {!portal.legacyFormat && (
              <Gated blocked={readOnly} fill>
                <button type="button" className="pass-add" onClick={() => onDialog({ kind: 'addRole', portal: portal.name })}>
                  <span className="icon-btn icon-btn--bordered">
                    <Plus aria-hidden="true" />
                  </span>
                  <strong>{t('card.add.title')}</strong>
                  <span>{t('card.add.body', { portal: portal.name })}</span>
                </button>
              </Gated>
            )}
          </>
        )}
      </div>
    </section>
  )
}

