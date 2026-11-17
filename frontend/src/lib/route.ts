import { useSyncExternalStore } from 'react'
import type { MigrationStep } from '../api/models'
import { MigrationStep as MS } from '../api/models'
import { initialUI } from './initialUI'

export type PageId = 'accounts' | 'migrate' | 'projects' | 'repos' | 'setup' | 'update' | 'git' | 'terminal' | 'health' | 'tutorial' | 'settings' | 'onboarding' | 'workspace'
export type TutorialSectionId = 'why' | 'enable' | 'assign' | 'portal' | 'migrate' | 'bind' | 'daily' | 'faq'
export type RepoFilter = 'all' | 'todo' | 'bound'
/** The six tabs of the workspace (design 4.2), in order. */
export const WORKSPACE_TABS = ['code', 'changes', 'history', 'branches', 'prs', 'ai'] as const
export type WorkspaceTabId = (typeof WORKSPACE_TABS)[number]
export interface Route {
  page: PageId
  profile?: string
  step?: MigrationStep
  section?: TutorialSectionId
  filter?: RepoFilter
  /** page 'workspace': the repository (its ID; in mock mode also its folder name) and the tab to show. */
  repo?: string
  tab?: WorkspaceTabId
}

const SECTIONS: TutorialSectionId[] = ['why', 'enable', 'assign', 'portal', 'migrate', 'bind', 'daily', 'faq']
const FILTERS: RepoFilter[] = ['all', 'todo', 'bound']
const STEPS = new Set<string>(Object.values(MS).filter((v) => v !== ''))

function initialRoute(): Route {
  const r: Route = { page: initialUI.page ?? 'accounts' }
  if (initialUI.profile) r.profile = initialUI.profile
  if (initialUI.step && STEPS.has(initialUI.step)) r.step = initialUI.step as MigrationStep
  if (initialUI.section && (SECTIONS as string[]).includes(initialUI.section)) r.section = initialUI.section as TutorialSectionId
  if (initialUI.filter && (FILTERS as string[]).includes(initialUI.filter)) r.filter = initialUI.filter as RepoFilter
  if (initialUI.repo) r.repo = initialUI.repo
  if (initialUI.tab && (WORKSPACE_TABS as readonly string[]).includes(initialUI.tab)) r.tab = initialUI.tab as WorkspaceTabId
  return r
}

let current: Route = initialRoute()
const listeners = new Set<() => void>()

export function getRoute(): Route {
  return current
}

export function setRoute(r: Route): void {
  current = r
  for (const l of [...listeners]) l()
}

function subscribe(cb: () => void): () => void {
  listeners.add(cb)
  return () => listeners.delete(cb)
}

export function useRoute(): Route {
  return useSyncExternalStore(subscribe, getRoute, getRoute)
}

export function useNavigate(): (r: Route) => void {
  return setRoute
}
