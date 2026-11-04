import { useEffect, useRef } from 'react'
import type { ReactNode } from 'react'
import { onEvent } from './api/events'
import { setWindowTitle } from './api/platform'
import { System } from './api/services'
import { SessionState } from './api/models'
import { I18nProvider, systemLang, useI18n } from './i18n'
import { initialUI, takeOpen } from './lib/initialUI'
import { focusSearch, runRefreshHandler } from './lib/keyboard'
import { guessPlatform, hasModifier } from './lib/platform'
import { setRoute, useRoute } from './lib/route'
import type { PageId } from './lib/route'
import { applyLang, applyMotion, applyPlatform, applyTheme, effectiveLang } from './lib/theme'
import { formatRemaining } from './lib/time'
import { now as clockNow } from './lib/clock'
import { AccountsPage } from './pages/accounts/AccountsPage'
import { GitGlobalPage } from './pages/git/GitGlobalPage'
import { HealthPage } from './pages/health/HealthPage'
import { MigrationPage } from './pages/migrate/MigrationPage'
import { OnboardingPage } from './pages/onboarding/OnboardingPage'
import { ReposPage } from './pages/repos/ReposPage'
import { SettingsPage } from './pages/settings/SettingsPage'
import { TerminalPage } from './pages/terminal/TerminalPage'
import { TutorialPage } from './pages/tutorial/TutorialPage'
import { AccountsProvider, useAccounts } from './state/accounts'
import { AppInfoProvider, useAppInfo } from './state/appInfo'
import { EnvProvider } from './state/env'
import { ExternalChangeProvider } from './state/external'
import { ReposProvider } from './state/repos'
import { SettingsProvider, useSettings } from './state/settings'
import { ActionsProvider, useActions } from './shell/AppActions'
import { Sidebar } from './shell/Sidebar'

/** Theme, motion, platform, language and window title follow the settings (language: settings value or the system language). */
function ThemeAndLanguage({ children }: { children?: ReactNode }) {
  const { settings } = useSettings()
  const info = useAppInfo()
  const lang = settings ? effectiveLang(settings.language) : (initialUI.lang ?? systemLang())

  useEffect(() => {
    if (settings) {
      applyTheme(settings.theme)
      applyMotion(settings.motion)
    }
  }, [settings])
  useEffect(() => applyPlatform(info?.platform ?? guessPlatform()), [info?.platform])
  useEffect(() => {
    applyLang(lang)
    void setWindowTitle('AWS SSO Hub')
    if (settings) void System.SetUILanguage(lang).catch(() => undefined)
  }, [lang, settings === null])

  return <I18nProvider lang={lang}>{children}</I18nProvider>
}

const PAGES: Record<PageId, () => ReactNode> = {
  accounts: () => <AccountsPage />,
  migrate: () => <MigrationPage />,
  repos: () => <ReposPage />,
  git: () => <GitGlobalPage />,
  terminal: () => <TerminalPage />,
  health: () => <HealthPage />,
  tutorial: () => <TutorialPage />,
  settings: () => <SettingsPage />,
  onboarding: () => <OnboardingPage />,
}

const SHORTCUT_PAGES: PageId[] = ['accounts', 'repos', 'git', 'terminal', 'health', 'tutorial']

/** App-level behaviour: first-run routing, ?open=login:, renewed toast, app:navigate, keyboard shortcuts. */
function useAppEffects() {
  const info = useAppInfo()
  const { state: accounts } = useAccounts()
  const { openLogin, toast } = useActions()
  const { t } = useI18n()
  const accountsRef = useRef(accounts)
  accountsRef.current = accounts
  const tRef = useRef(t)
  tRef.current = t

  // first run: no explicit page parameter -> onboarding
  const routed = useRef(false)
  useEffect(() => {
    if (!info || routed.current) return
    routed.current = true
    if (info.firstRun && !initialUI.page) setRoute({ page: 'onboarding' })
  }, [info])

  // ?open=login:<portal>
  const opened = useRef(false)
  useEffect(() => {
    if (!accounts || opened.current) return
    opened.current = true
    const portal = takeOpen('login:')
    if (portal !== null) void openLogin(portal || (accounts.portals?.[0]?.name ?? ''))
  }, [accounts, openLogin])

  // toast.renewed
  useEffect(
    () =>
      onEvent('session:renewed', (s) => {
        const end = s.sessionEndsAt ?? s.expiresAt
        toast({ kind: 'ok', title: tRef.current('toast.renewed', { portal: s.portal, remaining: end ? formatRemaining(end, clockNow(), tRef.current) : '' }) })
      }),
    [toast],
  )

  // tray / second instance
  useEffect(
    () =>
      onEvent('app:navigate', (req) => {
        const page = (req.page as PageId) || 'accounts'
        setRoute({ page })
        if (req.action === 'login' && req.arg) void openLogin(req.arg)
      }),
    [openLogin],
  )

  // keyboard
  useEffect(() => {
    const platform = info?.platform === 'windows' ? 'windows' : info?.platform === 'darwin' ? 'darwin' : guessPlatform()
    const onKey = (e: KeyboardEvent) => {
      if (!hasModifier(e, platform) || e.altKey) return
      const k = e.key.toLowerCase()
      if (k >= '1' && k <= '6') {
        e.preventDefault()
        setRoute({ page: SHORTCUT_PAGES[Number(k) - 1] })
      } else if (k === ',') {
        e.preventDefault()
        setRoute({ page: 'settings' })
      } else if (k === 'r') {
        e.preventDefault()
        runRefreshHandler()
      } else if (k === 'f') {
        if (focusSearch()) e.preventDefault()
      } else if (k === 'l') {
        e.preventDefault()
        const ps = (accountsRef.current?.portals ?? []).filter((p) => !p.draft && !p.legacyFormat)
        const target = ps.find((p) => p.session.state !== SessionState.SessionActive) ?? ps[0]
        if (target) void openLogin(target.name)
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [info?.platform, openLogin])
}

function Shell() {
  const route = useRoute()
  useAppEffects()
  return (
    <div className="app">
      <Sidebar />
      <main className="app__main" id="main" key={route.page}>
        {PAGES[route.page]()}
      </main>
    </div>
  )
}

export function App() {
  return (
    <AppInfoProvider>
      <SettingsProvider>
        <ThemeAndLanguage>
          <ExternalChangeProvider>
            <EnvProvider>
              <AccountsProvider>
                <ReposProvider>
                  <ActionsProvider>
                    <Shell />
                  </ActionsProvider>
                </ReposProvider>
              </AccountsProvider>
            </EnvProvider>
          </ExternalChangeProvider>
        </ThemeAndLanguage>
      </SettingsProvider>
    </AppInfoProvider>
  )
}
