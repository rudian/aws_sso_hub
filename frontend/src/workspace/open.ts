import { setRoute } from '../lib/route'
import type { WorkspaceTabId } from '../lib/route'
import { setTab } from '../state/workspace'

/** Go to a repository's workspace (design 4.2). The tab is set in the workspace store, so it also applies when that workspace is already on screen. */
export function openWorkspace(repoId: string, tab?: WorkspaceTabId): void {
  if (tab) setTab(repoId, tab)
  setRoute({ page: 'workspace', repo: repoId })
}
