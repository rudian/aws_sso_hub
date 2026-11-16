import clsx from 'clsx'
import { Clock, ExternalLink, KeyRound, Link as LinkIcon, LogIn, MoreHorizontal, Terminal } from 'lucide-react'
import { useRef, useState } from 'react'
import { copyText } from '../../api/platform'
import { Accounts, System } from '../../api/services'
import { MigrationStep } from '../../api/models'
import type { Portal, Profile } from '../../api/models'
import { AcctTile } from '../../components/AcctTile'
import { Badge } from '../../components/Badge'
import { Button } from '../../components/Button'
import { Countdown } from '../../components/Countdown'
import { Dot } from '../../components/Dot'
import { IconButton } from '../../components/IconButton'
import { Menu } from '../../components/Menu'
import { Spinner } from '../../components/Spinner'
import { Tooltip } from '../../components/Tooltip'
import { errorText, useI18n } from '../../i18n'
import { useNavigate } from '../../lib/route'
import { useNow } from '../../lib/useNow'
import { terminalCommands } from '../terminal/commands'
import { useActions } from '../../shell/AppActions'
import { useAccounts } from '../../state/accounts'
import { Gated, useErrorToast } from './hooks'
import type { DialogState } from './hooks'
import { activeLabel, cardStateText, hasKeysLeft, isSignedIn, pillState, sharedLine } from './model'
import './accounts.css'

interface PassCardProps {
  profile: Profile
  portal: Portal
  readOnly: boolean
  onDialog: (d: DialogState) => void
}

/** A card shows a countdown while the interactive session runs (also for a token that is waiting to be renewed). */
export function showsCountdown(portal: Portal): boolean {
  const s = portal.session
  const p = pillState(s)
  return p === 'active' || p === 'expiring' || (p === 'refreshable' && s.sessionKnown)
}

/** SSO profile card: identity, facts, countdown or state line, shared-sign-in line and the action row. */
export function PassCard({ profile, portal, readOnly, onDialog }: PassCardProps) {
  const { t, lang } = useI18n()
  const { openLogin, toast } = useActions()
  const navigate = useNavigate()
  const { reload } = useAccounts()
  const failToast = useErrorToast()
  const now = useNow(30_000)
  const session = portal.session
  const state = pillState(session)
  const counting = showsCountdown(portal)
  const signedIn = isSignedIn(session)
  const shared = sharedLine(profile, portal, counting || signedIn, t)
  const keysLeft = hasKeysLeft(profile)
  const [consoleOpening, setConsoleOpening] = useState(false)
  const consoleBusy = useRef(false)

  // OpenConsole makes two network calls (about 1 s); clicks meanwhile are ignored. The button stays enabled
  // while it loads (keepEnabled), so keyboard focus is not dropped to <body>.
  const openConsole = async () => {
    if (consoleBusy.current) return
    consoleBusy.current = true
    setConsoleOpening(true)
    try {
      await Accounts.OpenConsole(profile.name)
    } catch (e) {
      failToast(e)
    } finally {
      consoleBusy.current = false
      setConsoleOpening(false)
    }
  }

  const copy = async (text: string) => {
    try {
      await copyText(text)
      toast({ kind: 'ok', title: t('common.copied') })
    } catch {
      /* clipboard unavailable */
    }
  }

  // null when the profile name is not safe to put into a shell command (names come from ~/.aws/config)
  const exportCommand = terminalCommands(profile, 'posix').useHere

  const items = [
    { id: 'copyName', label: t('card.menu.copyName'), onSelect: () => void copy(profile.name) },
    ...(profile.accountId ? [{ id: 'copyId', label: t('card.menu.copyId'), onSelect: () => void copy(profile.accountId as string) }] : []),
    ...(exportCommand !== null ? [{ id: 'copyExport', label: t('card.menu.copyExport'), onSelect: () => void copy(exportCommand) }] : []),
    { id: 'color', label: t('card.menu.color'), separatorBefore: true, onSelect: () => onDialog({ kind: 'color', profile: profile.name }) },
    { id: 'config', label: t('card.menu.showInConfig'), onSelect: () => void System.OpenAWSConfig().catch(failToast) },
    ...(profile.boundRepos > 0 ? [{ id: 'move', label: t('card.moveRepos'), onSelect: () => onDialog({ kind: 'move', profile: profile.name }) }] : []),
    { id: 'remove', label: t('card.menu.remove'), danger: true, separatorBefore: true, disabled: readOnly, onSelect: () => onDialog({ kind: 'remove', profile: profile.name }) },
  ]

  const signIn = () => void openLogin(portal.name)
  const signInVariant = state === 'active' ? 'ghost' : state === 'expiring' || (state === 'refreshable' && counting) ? 'secondary' : 'primary'
  const signInLabel = state === 'active' ? t('card.signInAgain') : t('card.ssoLogin')

  return (
    <article className={clsx('pass', counting ? 'pass--active' : 'pass--expired')} data-acct={profile.colorSlot || undefined} data-profile={profile.name}>
      <span className="pass__stripe" />
      <div className="pass__head">
        <AcctTile slot={profile.colorSlot} monogram={profile.monogram} />
        <div style={{ flex: 1, minWidth: 0 }}>
          <div className="pass__name" title={profile.name}>{profile.name}</div>
          <div className="pass__role" title={profile.role}>{profile.role}</div>
        </div>
        <Menu align="end" trigger={<IconButton label={t('card.moreAria', { name: profile.name })} icon={MoreHorizontal} size="sm" />} items={items} />
      </div>
      <div className="pass__block">
        <dl className="pass__facts">
          {profile.accountId && (
            <div className="pass__fact">
              <dt className="sr-only">{t('card.label.account')}</dt>
              <dd>{profile.accountId}</dd>
            </div>
          )}
          {profile.region && (
            <div className="pass__fact">
              <dt className="sr-only">{t('card.label.region')}</dt>
              <dd>{profile.region}</dd>
            </div>
          )}
          <div className="pass__fact">
            <dt className="sr-only">{t('card.label.repos')}</dt>
            <dd className="pass__repos">
              {profile.boundRepos > 0 ? (
                <button type="button" className="link" aria-label={t('card.reposLink', { name: profile.name })} onClick={() => navigate({ page: 'repos', profile: profile.name })}>
                  {t('card.repos.count', { n: profile.boundRepos })}
                </button>
              ) : (
                <span className="muted">{t('card.repos.zero')}</span>
              )}
            </dd>
          </div>
        </dl>
        {keysLeft && (
          <div className="pass__keys">
            <Badge kind="warn" icon={KeyRound}>
              {t('legacy.keysLeft')}
            </Badge>
            <Gated blocked={readOnly}>
              <Button size="sm" variant="secondary" onClick={() => navigate({ page: 'migrate', profile: profile.name, step: MigrationStep.MigrationRemoveKey })}>
                {t('legacy.keysLeft.action')}
              </Button>
            </Gated>
          </div>
        )}
        {counting ? (
          <Countdown session={session} slot={profile.colorSlot} label={activeLabel(session, now, t)} />
        ) : (
          <div className="stack" style={{ ['--gap' as string]: '4px' }}>
            <div className="pass__state">
              {state === 'expired' && <Clock size={15} aria-hidden="true" />}
              {state === 'signing-in' && <Dot kind="busy" />}
              {state === 'signed-out' && <Dot kind="off" />}
              {state === 'error' && <Dot kind="danger" />}
              {state === 'refreshable' && (session.refreshing ? <Spinner /> : <Dot kind="warn" />)}
              <span>{cardStateText(session, now, lang, t)}</span>
            </div>
            {state === 'error' && session.error && <ErrorDetails error={session.error} />}
          </div>
        )}
        {shared && (
          <span className="pass__shared" title={shared}>
            <LinkIcon aria-hidden="true" />
            {shared}
          </span>
        )}
      </div>
      <div className="pass__actions">
        {counting && (
          <>
            <Button size="sm" icon={ExternalLink} loading={consoleOpening} keepEnabled onClick={() => void openConsole()}>
              {t('card.console')}
            </Button>
            <IconButton size="sm" label={t('card.terminal')} icon={Terminal} onClick={() => navigate({ page: 'terminal', profile: profile.name })} />
          </>
        )}
        <span className="spacer" />
        {state === 'error' ? (
          <Button size="sm" variant="primary" onClick={() => void reload()}>
            {t('common.retry')}
          </Button>
        ) : (
          <Tooltip content={state === 'active' ? t('card.signInAgain.tooltip') : undefined}>
            <Button size="sm" variant={signInVariant} icon={LogIn} loading={state === 'signing-in'} onClick={signIn}>
              {signInLabel}
            </Button>
          </Tooltip>
        )}
      </div>
    </article>
  )
}

function ErrorDetails({ error }: { error: NonNullable<Portal['session']['error']> }) {
  const { t } = useI18n()
  const text = errorText({ code: error.code, message: error.message, detail: error.detail, params: (error.params ?? {}) as Record<string, string> }, t)
  return (
    <details className="pass__details">
      <summary>{t('common.details')}</summary>
      <div>{text.title}</div>
      {text.detail && <pre className="mono">{text.detail}</pre>}
    </details>
  )
}

