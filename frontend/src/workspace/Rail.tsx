import clsx from 'clsx'
import { Bot, GitPullRequest, IdCard, Layers, Settings, ShieldCheck, SlidersHorizontal } from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import { ToolStatus } from '../api/models'
import { Tooltip } from '../components'
import { useI18n } from '../i18n'
import type { MessageKey } from '../i18n'
import { setRoute } from '../lib/route'
import type { PageId } from '../lib/route'
import { actionCount } from '../pages/projects/model'
import { useUpdateIndicator } from '../pages/update/useUpdate'
import { useAccounts } from '../state/accounts'
import { useEnv } from '../state/env'
import { useRepos } from '../state/repos'

interface RailDef {
  id: string
  icon: LucideIcon
  label: MessageKey
  page?: PageId
}

// Design 4.2: the sidebar's destinations as icon buttons. AI tasks and Pull requests have no page yet (A2-W wires them).
const TOP: RailDef[] = [
  { id: 'projects', icon: Layers, label: 'nav.projects', page: 'projects' },
  { id: 'ai', icon: Bot, label: 'ws.rail.ai' },
  { id: 'prs', icon: GitPullRequest, label: 'ws.rail.prs' },
  { id: 'accounts', icon: IdCard, label: 'nav.accounts', page: 'accounts' },
]
const MORE: RailDef[] = [
  { id: 'git', icon: SlidersHorizontal, label: 'nav.gitGlobal', page: 'git' },
  { id: 'health', icon: ShieldCheck, label: 'nav.health', page: 'health' },
]

/** The icon rail (`.ws__rail`, 52px). Projects is active while a repository is open; Settings carries the update pip. */
export function Rail() {
  const { t } = useI18n()
  const updatePip = useUpdateIndicator()
  const { state: accounts } = useAccounts()
  const { state: repos } = useRepos()
  const { report } = useEnv()
  // the same counts the sidebar shows, as pips
  const toFix = actionCount(repos?.repos, accounts?.profiles ?? [], accounts?.portals ?? [])
  const toMigrate = accounts?.staticCount ?? 0
  const envBad =
    !!report && (report.awsCli.status !== ToolStatus.ToolOK || report.git.status !== ToolStatus.ToolOK || (report.files ?? []).some((f) => !f.readable || !f.writable || !!f.issue))
  const pips: Record<string, string | undefined> = {
    projects: toFix > 0 ? t('nav.count.toFix', { n: toFix }) : undefined,
    accounts: toMigrate > 0 ? t('nav.count.toMigrate', { n: toMigrate }) : undefined,
    health: envBad ? t('ws.rail.pip.env') : undefined,
    settings: updatePip ? t('update.dot') : undefined,
  }

  const btn = (d: RailDef, active: boolean, pip?: string) => {
    const soon = !d.page
    // the label carries the pip's meaning: a button's aria-label replaces its content
    const name = pip ? `${t(d.label)}, ${pip}` : t(d.label)
    return (
      <Tooltip key={d.id} content={soon ? `${t(d.label)} · ${t('ws.rail.soon')}` : name}>
        <button
          type="button"
          className={clsx('rail__btn', active && 'is-active')}
          aria-label={name}
          aria-current={active ? 'page' : undefined}
          aria-disabled={soon || undefined}
          onClick={() => d.page && setRoute({ page: d.page })}
        >
          <d.icon aria-hidden="true" />
          {pip && <span className="pip" aria-hidden="true" />}
        </button>
      </Tooltip>
    )
  }

  return (
    <nav className="ws__rail" aria-label={t('nav.aria')}>
      {TOP.map((d) => btn(d, d.id === 'projects', pips[d.id]))}
      <span className="rail__sep" />
      {MORE.map((d) => btn(d, false, pips[d.id]))}
      <span className="rail__spacer" />
      {/* the keep-awake cup (A2-03) sits here while a hold is on */}
      {btn({ id: 'settings', icon: Settings, label: 'nav.settings', page: 'settings' }, false, pips.settings)}
    </nav>
  )
}
