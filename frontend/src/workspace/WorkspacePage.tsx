// The workspace (design 4.2): the frame around one repository. Tab bodies, the terminal and the sync button are slots
// that later tasks fill (slots.tsx); this file owns the layout, the keyboard map and the per-folder state wiring.
import { useEffect, useMemo, useRef, useState } from 'react'
import { ArrowLeft } from 'lucide-react'
import { Repos } from '../api'
import type { Repo } from '../api/models'
import { Button, EmptyState, Skeleton } from '../components'
import { useI18n } from '../i18n'
import { takeOpen } from '../lib/initialUI'
import { guessPlatform } from '../lib/platform'
import type { Platform } from '../lib/platform'
import { setRoute, useRoute } from '../lib/route'
import { useAppInfo } from '../state/appInfo'
import { useReportError } from '../pages/repos/hooks'
import { useRepos } from '../state/repos'
import { MAIN_FOLDER, cycleFolder, getWorkspace, initialFolder, useWorkspace } from '../state/workspace'
import { BottomPanel } from './BottomPanel'
import { inTerminal, wsKeyAction } from './keymap'
import { Rail } from './Rail'
import { RepoSettingsSheet } from './RepoSettingsSheet'
import { StatusBar } from './StatusBar'
import { TAB_VIEWS } from './slots'
import { TabBar, panelId, tabId } from './TabBar'
import { workTreesOf, findRepo } from './trees'
import { useBindPopover } from './useBind'
import { WorkspaceHeader } from './WorkspaceHeader'
import './workspace.css'

// True while a Workspace is mounted: the next Workspace (another repository, same page) skips the sidebar-narrows motion.
let workspaceOnScreen = false

const SOON = { code: 'ws.soon.code', changes: 'ws.soon.changes', history: 'ws.soon.history', branches: 'ws.soon.branches', prs: 'ws.soon.prs', ai: 'ws.soon.ai' } as const

export function WorkspacePage() {
  const route = useRoute()
  const { state, loading } = useRepos()
  const { t } = useI18n()
  const repo = findRepo(state?.repos, route.repo)

  if (!repo) {
    return (
      <div className="ws-gone" role="main">
        {loading && !state ? (
          <Skeleton variant="block" width={320} height={96} />
        ) : (
          <EmptyState
            title={t('ws.missing.title')}
            text={t('ws.missing.body')}
            actions={
              <Button icon={ArrowLeft} onClick={() => setRoute({ page: 'projects' })}>
                {t('ws.back')}
              </Button>
            }
          />
        )}
      </div>
    )
  }
  // key: another repository starts with its own state, scroll and focus
  return <Workspace key={repo.id} repo={repo} />
}

function Workspace({ repo }: { repo: Repo }) {
  const { t } = useI18n()
  const info = useAppInfo()
  const route = useRoute()
  const platform: Platform = info?.platform === 'windows' ? 'windows' : info?.platform === 'darwin' ? 'darwin' : guessPlatform()
  const report = useReportError()
  const [still] = useState(() => workspaceOnScreen)
  useEffect(() => {
    workspaceOnScreen = true
    return () => {
      workspaceOnScreen = false
    }
  }, [])
  const ws = useWorkspace(repo.id)
  const trees = useMemo(() => workTreesOf(repo), [repo])
  // A folder that is gone (a task's workspace removed after Merge or Discard) falls back to the main workspace
  const shownId = trees.some((x) => x.id === ws.folderId) ? ws.folderId : MAIN_FOLDER
  const folderId = shownId
  const folder = shownId === ws.folderId ? ws.folder : (getWorkspace(repo.id).folders[shownId] ?? initialFolder())
  const tab = ws.tab
  useEffect(() => {
    if (shownId !== ws.folderId) ws.selectFolder(shownId)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [shownId, ws.folderId])

  // Every way into a workspace records the opening (it allows the background fetch) and reads the git status
  useEffect(() => {
    void Repos.MarkOpened(repo.id).catch(report)
    if (!repo.status) void Repos.RefreshStatus([repo.id]).catch(() => undefined)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [repo.id])
  const bind = useBindPopover(repo)
  // ?open=repo-settings (mock links): the sheet starts open
  const [settingsOpen, setSettingsOpen] = useState(() => takeOpen('repo-settings') !== null)
  const settingsRef = useRef<HTMLButtonElement>(null)

  // A link that names a tab (mock URL, "open the AI tab of this repository") applies once on arrival.
  const wantedTab = useRef(route.tab)
  useEffect(() => {
    if (wantedTab.current) ws.setTab(wantedTab.current)
    wantedTab.current = undefined
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // Keyboard map (design 4.3): Ctrl/Alt+1..6 tabs, Ctrl+` panel, Ctrl+Tab working trees
  const stateRef = useRef({ ws, trees })
  stateRef.current = { ws, trees }
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.defaultPrevented) return
      // a sheet, a dialog or an open menu has the keyboard: the workspace behind it stays as it is
      if (document.querySelector('[aria-modal="true"], [role="menu"]')) return
      const a = wsKeyAction(e, platform, inTerminal(e.target))
      if (!a) return
      e.preventDefault()
      const { ws: w, trees: tr } = stateRef.current
      if (a.kind === 'tab') w.setTab(a.tab)
      else if (a.kind === 'panel') w.togglePanel()
      else w.selectFolder(cycleFolder(tr.map((x) => x.id), w.folderId, a.dir))
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [platform])

  const tree = trees.find((x) => x.id === folderId) ?? trees[0]
  const unborn = !!repo.status?.unborn
  const detached = !!repo.status?.detached
  const context = tree.kind === 'main' ? tree.branch || (unborn ? t('projects.git.unborn') : detached ? t('projects.git.detached') : '') : tree.branch || tree.title || tree.id
  const View = TAB_VIEWS[tab]
  const changes = repo.status?.changes ?? 0

  return (
    <div className={still ? 'ws ws--still' : 'ws'} role="region" aria-label={t('ws.aria')}>
      <WorkspaceHeader
        repo={repo}
        trees={trees}
        folderId={folderId}
        onSelectFolder={ws.selectFolder}
        onBind={bind.open}
        onSettings={() => setSettingsOpen(true)}
        settingsRef={settingsRef}
      />
      <Rail />
      <div className="ws__main">
        <TabBar value={tab} onChange={ws.setTab} counts={{ changes }} platform={platform} />
        {/* key: a new tab's content fades in and rises 6px (design 3); the old one goes at once */}
        <div className="ws__body" key={`${folderId}:${tab}`} role="tabpanel" id={panelId(tab)} aria-labelledby={tabId(tab)}>
          {View ? (
            <View repo={repo} folderId={folderId} />
          ) : (
            <EmptyState art={<></>} title={t('ws.soon.title')} text={t(SOON[tab])} />
          )}
        </div>
        {folder.panel.open && <BottomPanel repo={repo} folderId={folderId} panel={folder.panel} onChange={ws.setPanel} />}
      </div>
      <StatusBar panel={folder.panel} context={context} platform={platform} onTogglePanel={ws.togglePanel} onOpenPanel={(tab) => ws.setPanel({ open: true, tab })} onOpenBranches={() => ws.setTab('branches')} />
      <RepoSettingsSheet repo={repo} open={settingsOpen} onClose={() => setSettingsOpen(false)} onBind={bind.open} />
      {bind.element}
    </div>
  )
}
