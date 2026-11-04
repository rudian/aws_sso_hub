import clsx from 'clsx'
import { BookOpen, GitBranch, IdCard, Settings, ShieldCheck, SlidersHorizontal, SquareTerminal } from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import appicon from '../assets/appicon.svg'
import { useMocks } from '../api/env'
import { ToolStatus } from '../api/models'
import { Dot } from '../components/Dot'
import { useI18n } from '../i18n'
import type { MessageKey } from '../i18n'
import { guessPlatform } from '../lib/platform'
import { useNavigate, useRoute } from '../lib/route'
import type { PageId } from '../lib/route'
import { useAccounts } from '../state/accounts'
import { useAppInfo } from '../state/appInfo'
import { useEnv } from '../state/env'
import { useRepos } from '../state/repos'
import { Keyring } from './Keyring'

interface NavDef {
  page: PageId
  icon: LucideIcon
  label: MessageKey
}

const NAV: NavDef[] = [
  { page: 'accounts', icon: IdCard, label: 'nav.accounts' },
  { page: 'repos', icon: GitBranch, label: 'nav.repos' },
  { page: 'git', icon: SlidersHorizontal, label: 'nav.gitGlobal' },
  { page: 'terminal', icon: SquareTerminal, label: 'nav.terminal' },
  { page: 'health', icon: ShieldCheck, label: 'nav.health' },
  { page: 'tutorial', icon: BookOpen, label: 'nav.tutorial' },
]

export function Sidebar() {
  const { t } = useI18n()
  const route = useRoute()
  const navigate = useNavigate()
  const info = useAppInfo()
  const { state: accounts } = useAccounts()
  const { state: repos } = useRepos()
  const { report } = useEnv()

  const activePage: PageId = route.page === 'migrate' ? 'accounts' : route.page
  const platform = info?.platform ?? guessPlatform()

  const envBad =
    !!report &&
    (report.awsCli.status !== ToolStatus.ToolOK ||
      report.git.status !== ToolStatus.ToolOK ||
      (report.files ?? []).some((f) => !f.readable || !f.writable || !!f.issue))

  const count = (def: NavDef) => {
    if (def.page === 'accounts' && (accounts?.staticCount ?? 0) > 0) return <span className="nav__count nav__count--warn">{t('nav.count.toMigrate', { n: accounts?.staticCount ?? 0 })}</span>
    if (def.page === 'repos' && (repos?.counts.toFix ?? 0) > 0) return <span className="nav__count nav__count--warn">{t('nav.count.toFix', { n: repos?.counts.toFix ?? 0 })}</span>
    if (def.page === 'health' && envBad) return <span className="nav__count"><Dot kind="warn" /></span>
    return null
  }

  return (
    <aside className="app__sidebar">
      <div className="app__titlebar">
        {useMocks && platform === 'darwin' && (
          <div className="traffic" aria-hidden="true">
            <i />
            <i />
            <i />
          </div>
        )}
      </div>
      <div className="brand">
        <img className="brand__mark" src={appicon} alt="" />
        <div>
          <div className="brand__name">{t('app.name')}</div>
          <div className="brand__sub">{t('app.subtitle')}</div>
          {info?.sandbox && <span className="sandbox-badge">{t('app.sandbox')}</span>}
        </div>
      </div>
      <nav className="nav" aria-label={t('nav.aria')}>
        {NAV.map((def) => (
          <button key={def.page} type="button" className={clsx('nav__item', activePage === def.page && 'is-active')} aria-current={activePage === def.page ? 'page' : undefined} onClick={() => navigate({ page: def.page })}>
            <def.icon className="nav__icon" aria-hidden="true" />
            {t(def.label)}
            {count(def)}
          </button>
        ))}
      </nav>
      <div className="nav__section" style={{ paddingLeft: 24, marginTop: 8 }}>
        {t('keyring.title')}
      </div>
      <Keyring />
      <div className="sidebar-foot">
        <nav className="nav">
          <button type="button" className={clsx('nav__item', activePage === 'settings' && 'is-active')} aria-current={activePage === 'settings' ? 'page' : undefined} onClick={() => navigate({ page: 'settings' })}>
            <Settings className="nav__icon" aria-hidden="true" />
            {t('nav.settings')}
          </button>
        </nav>
      </div>
    </aside>
  )
}
