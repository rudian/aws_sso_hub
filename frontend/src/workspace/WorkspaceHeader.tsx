import { PanelLeft, Settings } from 'lucide-react'
import { useRef } from 'react'
import type { Repo } from '../api/models'
import { useMocks } from '../api/env'
import { AcctChip, IconButton, ProviderMark } from '../components'
import { RepoName } from '../components/RepoName'
import { useI18n } from '../i18n'
import { guessPlatform } from '../lib/platform'
import { setRoute } from '../lib/route'
import { chipBindable, isCodeCommit, providerKind } from '../pages/projects/model'
import { useAppInfo } from '../state/appInfo'
import { useRepos } from '../state/repos'
import type { WorkTree } from '../state/workspace'
import { SyncSlot } from './slots'
import { TreeSwitch } from './TreeSwitch'

export interface WorkspaceHeaderProps {
  repo: Repo
  trees: readonly WorkTree[]
  folderId: string
  onSelectFolder: (id: string) => void
  onBind: (anchor: HTMLElement | null) => void
  onSettings: () => void
  settingsRef: React.Ref<HTMLButtonElement>
}

/** Header (`.ws__head`, 46px, drag region): back, project > repository, working-tree switcher, sync, account chip, gear. */
export function WorkspaceHeader({ repo, trees, folderId, onSelectFolder, onBind, onSettings, settingsRef }: WorkspaceHeaderProps) {
  const { t } = useI18n()
  const info = useAppInfo()
  const { state } = useRepos()
  const acctRef = useRef<HTMLButtonElement>(null)
  const platform = info?.platform === 'windows' ? 'windows' : info?.platform === 'darwin' ? 'darwin' : guessPlatform()
  const project = repo.projectId ? (state?.projects ?? []).find((p) => p.id === repo.projectId) : undefined
  const prov = providerKind(repo)
  const b = repo.binding
  const bindable = chipBindable(repo)
  const chip = b.profile && b.profileKnown ? <AcctChip slot={b.colorSlot} monogram={b.monogram} name={b.profile} accountId={b.accountId} /> : <AcctChip none dashed={!b.profile} name={b.profile ?? t('ws.acct.unbound')} />
  const isTask = trees.find((x) => x.id === folderId)?.kind === 'task'

  return (
    <header className={`ws__head${isTask ? ' is-task' : ''}`}>
      {useMocks && platform === 'darwin' && (
        <div className="traffic" aria-hidden="true">
          <i />
          <i />
          <i />
        </div>
      )}
      <IconButton label={t('ws.back')} icon={PanelLeft} onClick={() => setRoute({ page: 'projects' })} />
      <nav className="ws__crumb" aria-label={t('ws.crumb.aria')}>
        {project && (
          <>
            <span>{project.name}</span>
            <span aria-hidden="true">›</span>
          </>
        )}
        {prov && <ProviderMark provider={prov} />}
        <RepoName localName={repo.localName} remoteName={repo.remoteName} className="ws__repo" />
      </nav>
      <TreeSwitch trees={trees} current={folderId} onSelect={onSelectFolder} />
      <span className="spacer" />
      <SyncSlot />
      {isCodeCommit(repo) &&
        (bindable ? (
          <button ref={acctRef} type="button" className="ws__acct" aria-label={b.profile ? t('ws.acct.change', { name: b.profile }) : t('ws.acct.bind')} onClick={() => onBind(acctRef.current)}>
            {chip}
          </button>
        ) : (
          <span className="ws__acct">{chip}</span>
        ))}
      <IconButton ref={settingsRef} label={t('ws.settings')} icon={Settings} onClick={onSettings} />
    </header>
  )
}
