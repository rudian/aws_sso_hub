import { SquareTerminal } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { ProfileKind } from '../../api/models'
import type { Profile } from '../../api/models'
import { toUiError } from '../../api/errors'
import { System } from '../../api/services'
import { AcctChip } from '../../components/AcctChip'
import { Banner } from '../../components/Banner'
import { Button } from '../../components/Button'
import { CodeBlock } from '../../components/CodeBlock'
import { EmptyState } from '../../components/EmptyState'
import { ErrorBanner } from '../../components/ErrorBanner'
import { Skeleton } from '../../components/Skeleton'
import { Tabs } from '../../components/Tabs'
import { errorText, useI18n } from '../../i18n'
import { guessPlatform } from '../../lib/platform'
import { useNavigate, useRoute } from '../../lib/route'
import { useActions } from '../../shell/AppActions'
import { GlobalBanner } from '../../shell/GlobalBanner'
import { useAccounts } from '../../state/accounts'
import { useAppInfo } from '../../state/appInfo'
import { useSettings } from '../../state/settings'
import { defaultShell, hasUnsafeName, isSafeProfileName, terminalCommands } from './commands'
import type { ShellId } from './commands'
import { TerminalAppSelect } from './TerminalAppSelect'
import './terminal.css'

const USABLE: ProfileKind[] = [ProfileKind.ProfileKindSSO, ProfileKind.ProfileKindLegacySSO, ProfileKind.ProfileKindStatic]

export function TerminalPage() {
  const { t } = useI18n()
  const info = useAppInfo()
  const { state, loading, error, reload } = useAccounts()
  const { settings, update } = useSettings()
  const route = useRoute()
  const navigate = useNavigate()
  const { toast } = useActions()
  const platform = info?.platform ?? guessPlatform()

  const profiles = useMemo<Profile[]>(() => (state?.profiles ?? []).filter((p) => USABLE.includes(p.kind)), [state])
  const [selected, setSelected] = useState<string | null>(route.profile ?? null)
  const [shell, setShell] = useState<ShellId>(defaultShell(platform))
  const [opening, setOpening] = useState(false)

  // The platform is only known after AppInfo has loaded.
  useEffect(() => {
    if (info) setShell(defaultShell(info.platform))
  }, [info?.platform])

  const profile = profiles.find((p) => p.name === selected) ?? profiles.find((p) => p.kind !== ProfileKind.ProfileKindStatic) ?? profiles[0]
  const cmds = profile ? terminalCommands(profile, shell) : null
  const unsafeName = !!profile && hasUnsafeName(profile)
  const portal = profile?.portal ? state?.portals?.find((p) => p.name === profile.portal) : undefined

  const fail = (e: unknown) => {
    const text = errorText(toUiError(e), t)
    toast({ kind: 'danger', title: text.title, body: text.body })
  }

  const openTerminal = async () => {
    if (!profile) return
    setOpening(true)
    try {
      await System.OpenTerminal(profile.name, '')
      toast({ kind: 'ok', title: t('term.open.done', { name: profile.name }) })
    } catch (e) {
      fail(e)
    } finally {
      setOpening(false)
    }
  }

  const shellTabs = [
    { id: 'posix', label: t('term.shell.posix') },
    { id: 'powershell', label: t('term.shell.powershell') },
    { id: 'cmd', label: t('term.shell.cmd') },
  ]

  return (
    <div className="page">
      <header className="page__header">
        <div>
          <h1 className="page__title">{t('term.title')}</h1>
          <p className="page__subtitle">{t('term.subtitle')}</p>
        </div>
        <div className="page__actions">
          {settings && (
            <label className="term__app">
              <span>{t('term.app')}</span>
              <TerminalAppSelect platform={platform} value={settings.terminalApp} onChange={(v) => void update({ terminalApp: v }).catch(fail)} />
            </label>
          )}
          {(!profile || isSafeProfileName(profile.name)) && (
            <Button variant="primary" icon={SquareTerminal} loading={opening} disabled={!profile} onClick={() => void openTerminal()}>
              {t('term.open')}
            </Button>
          )}
        </div>
      </header>
      <GlobalBanner />

      {error && !state ? (
        <ErrorBanner error={error} onRetry={() => void reload()} />
      ) : loading && !state ? (
        <div className="card">
          <div className="card__body term__section">
            <Skeleton variant="block" height={36} />
            <Skeleton variant="block" height={120} />
          </div>
        </div>
      ) : !profile || !cmds ? (
        <div className="card">
          <EmptyState
            title={t('term.empty.title')}
            text={t('term.empty.body')}
            actions={
              <Button variant="primary" onClick={() => navigate({ page: 'accounts' })}>
                {t('term.empty.action')}
              </Button>
            }
          />
        </div>
      ) : (
        <div className="page__body">
          <div className="card">
            <div className="card__body">
              <div className="term__label" id="term-profile-label" style={{ marginBottom: 8 }}>
                {t('term.profile')}
              </div>
              <div className="term__chips" role="group" aria-labelledby="term-profile-label">
                {profiles.map((p) => (
                  <button key={p.name} type="button" className="chip-btn" aria-pressed={p.name === profile.name} aria-label={p.accountId ? t('acct.label', { name: p.name, accountId: p.accountId }) : p.name} onClick={() => setSelected(p.name)}>
                    <AcctChip slot={p.colorSlot} monogram={p.monogram} name={p.name} />
                  </button>
                ))}
              </div>
              <div className="term__meta">
                {profile.accountId && <span className="mono">{profile.accountId}</span>}
                {profile.role && <span>{profile.role}</span>}
                {profile.region && <span className="mono">{profile.region}</span>}
                {portal && <span className="mono">{portal.name}</span>}
              </div>
            </div>
          </div>

          <div className="card">
            <div className="card__header">
              <Tabs tabs={shellTabs} value={shell} onChange={(id) => setShell(id as ShellId)} ariaLabel={t('term.shell')} />
            </div>
            <div className="card__body term__section">
              {unsafeName && <Banner kind="warn">{t('term.unsafe.notice')}</Banner>}
              {cmds.useHere !== null && <CodeBlock label={t('term.useHere')} code={cmds.useHere} />}
              {cmds.signIn !== null ? (
                <CodeBlock label={t('term.signIn')} code={cmds.signIn} />
              ) : (
                profile.kind === ProfileKind.ProfileKindStatic && (
                  <div className="term__block">
                    <div className="term__label">{t('term.signIn')}</div>
                    <div className="term__hint">{t('term.signIn.static')}</div>
                  </div>
                )
              )}
              {cmds.whoami !== null && <CodeBlock label={t('term.whoami')} code={cmds.whoami} />}
              <div className="term__block">
                <CodeBlock label={t('term.clone')} code={cmds.clone} />
                <div className="term__hint">{t('term.clone.hint')}</div>
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
