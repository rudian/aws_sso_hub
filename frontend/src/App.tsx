import { useEffect, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { onEvent } from './api/events'
import { setWindowTitle } from './api/platform'
import { System, Tools } from './api/services'
import { SessionState } from './api/models'
import { I18nProvider, systemLang, useI18n } from './i18n'
import { GATE_RECHECK_EVENT } from './lib/gate'
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
import { ProjectsPage } from './pages/projects/ProjectsPage'
import { SettingsPage } from './pages/settings/SettingsPage'
import { SetupPage } from './pages/setup/SetupPage'
import { TerminalPage } from './pages/terminal/TerminalPage'
import { TutorialPage } from './pages/tutorial/TutorialPage'
import { UpdatePage } from './pages/update/UpdatePage'
import { AccountsProvider, useAccounts } from './state/accounts'
import { AppInfoProvider, useAppInfo } from './state/appInfo'
import { EnvProvider } from './state/env'
import { ExternalChangeProvider } from './state/external'
import { ReposProvider } from './state/repos'
import { SettingsProvider, useSettings } from './state/settings'
import { ActionsProvider, useActions } from './shell/AppActions'
import { AppCommands } from './shell/AppCommands'
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
    void setWindowTitle('Codepass')
    if (settings) void System.SetUILanguage(lang).catch(() => undefined)
  }, [lang, settings === null])

  return <I18nProvider lang={lang}>{children}</I18nProvider>
}

const PAGES: Record<PageId, () => ReactNode> = {
  accounts: () => <AccountsPage />,
  migrate: () => <MigrationPage />,
  projects: () => <ProjectsPage />,
  repos: () => <ProjectsPage />, // alias of projects: old links (Health, account card) land on the same page
  setup: () => <SetupPage />,
  update: () => <UpdatePage />,
  git: () => <GitGlobalPage />,
  terminal: () => <TerminalPage />,
  health: () => <HealthPage />,
  tutorial: () => <TutorialPage />,
  settings: () => <SettingsPage />,
  onboarding: () => <OnboardingPage />,
}

// Cmd/Ctrl+1..4: Projects, AI tasks, Pull requests, Accounts (design 4.3). AI tasks and Pull requests have no page yet.
const SHORTCUT_PAGES: (PageId | null)[] = ['projects', null, null, 'accounts']

/** App-level behaviour: first-run routing, ?open=login:, renewed toast, app:navigate, keyboard shortcuts. */
function useAppEffects(gateOpen: boolean) {
  const info = useAppInfo()
  const { state: accounts } = useAccounts()
  const { openLogin, toast } = useActions()
  const { t } = useI18n()
  const accountsRef = useRef(accounts)
  accountsRef.current = accounts
  const tRef = useRef(t)
  tRef.current = t

  // first run: no explicit page parameter -> onboarding. Decided only once the tools gate is open, so the setup screen wins.
  const routed = useRef(false)
  useEffect(() => {
    if (!info || !gateOpen || routed.current) return
    routed.current = true
    if (info.firstRun && !initialUI.page) setRoute({ page: 'onboarding' })
  }, [info, gateOpen])

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
      if (k >= '1' && k <= '4') {
        e.preventDefault()
        const page = SHORTCUT_PAGES[Number(k) - 1]
        if (page) setRoute({ page })
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

type Gate = 'checking' | 'open' | 'blocked'

/** The tools gate: a missing or too-old git or AWS CLI replaces the whole window with the setup screen (no sidebar, no way around it). */
function useToolsGate(): Gate {
  const [gate, setGate] = useState<Gate>('checking')
  useEffect(() => {
    let alive = true
    const check = () =>
      void Tools.GetSetupState()
        .then((s) => alive && setGate(s.blocked ? 'blocked' : 'open'))
        .catch(() => alive && setGate((g) => (g === 'checking' ? 'open' : g)))
    check()
    window.addEventListener(GATE_RECHECK_EVENT, check)
    return () => {
      alive = false
      window.removeEventListener(GATE_RECHECK_EVENT, check)
    }
  }, [])
  return gate
}

function Shell() {
  const route = useRoute()
  const gate = useToolsGate()
  useAppEffects(gate === 'open')
  if (gate === 'checking') return <div className="app" />
  if (gate === 'blocked') {
    return (
      <div className="app" style={{ gridTemplateColumns: '1fr' }}>
        <main className="app__main page-enter" id="main">
          <SetupPage />
        </main>
      </div>
    )
  }
  return (
    <div className="app">
      <Sidebar />
      <AppCommands />
      <main className="app__main page-enter" id="main" key={route.page}>
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
