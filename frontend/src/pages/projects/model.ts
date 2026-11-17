// Pure helpers for the Projects page (design 5.2): grouping, filters, search, suggestions, the found chooser and
// the identity side of a move. No React in here, so the rules are tested on their own.
import { RepoAction, RepoProvider } from '../../api/models'
import type { MoveIdentityPreview, Portal, Profile, ProjectInfo, ProjectSuggestion, Repo, ReposState } from '../../api/models'
import type { ProviderKind } from '../../components'
import { rankRepo, searchRepos } from '../../lib/repoSearch'
import { canBind, portalOf, rowKind, sessionNeedsLogin } from '../repos/model'

export type ProjectFilter = 'all' | 'todo' | 'changes'
/** 'all', or one provider; 'hidden' lists the hidden repositories instead of the listed ones. */
export type SourceFilter = 'all' | RepoProvider.ProviderCodeCommit | RepoProvider.ProviderGitHub | RepoProvider.ProviderBitbucket | RepoProvider.ProviderOther

export const UNGROUPED = ''
export const HIDDEN_SECTION = '__hidden__'
/** Repositories with a commit from the last 90 days are pre-ticked in the found chooser. */
export const RECENT_DAYS = 90

export interface Section {
  /** Project ID; '' = Ungrouped; HIDDEN_SECTION = the list of hidden repositories. */
  id: string
  project?: ProjectInfo
  /** Repositories that pass the filters, in display order. */
  repos: Repo[]
  /** Listed repositories in the section before any filter. */
  total: number
}

export interface GroupOptions {
  filter: ProjectFilter
  source: SourceFilter
  query: string
  showHidden: boolean
  /** Accounts, for the rows whose sign-in expired; without them only the binding counts. */
  profiles?: Profile[]
  portals?: Portal[]
}

export function hasChanges(r: Repo): boolean {
  return (r.status?.changes ?? 0) > 0
}

export function isCodeCommit(r: Repo): boolean {
  return r.provider === RepoProvider.ProviderCodeCommit
}

/**
 * True where the account chip opens the bind popover: a CodeCommit repository that can be bound and whose owner is
 * neither being looked up nor ambiguous. Elsewhere the chip is a plain chip (the Projects row and the workspace header share this).
 */
export function chipBindable(r: Repo): boolean {
  if (!isCodeCommit(r) || !canBind(r)) return false
  const k = rowKind(r)
  return k !== 'checking' && k !== 'ambiguous'
}

/** A repository is "to fix" for CodeCommit rows (the binding is wrong or missing); other providers never are. */
export function needsFix(r: Repo): boolean {
  return isCodeCommit(r) && r.toFix
}

/**
 * A row that needs an action: the binding is wrong or missing, or it is bound to a portal whose sign-in expired
 * (the row shows "Sign-in expired; pushes will fail" with its Sign in action). One rule for the "To fix" filter, its
 * number and the sidebar count.
 */
export function needsAction(r: Repo, profiles: Profile[], portals: Portal[]): boolean {
  if (!isCodeCommit(r)) return false
  if (r.toFix) return true
  if (rowKind(r) !== 'bound' || !r.binding.profileKnown) return false
  const profile = profiles.find((p) => p.name === r.binding.profile)
  const portal = portalOf(profile, portals)
  return !!portal && sessionNeedsLogin(portal.session.state)
}

export function actionCount(repos: Repo[] | null | undefined, profiles: Profile[], portals: Portal[]): number {
  return (repos ?? []).filter((r) => needsAction(r, profiles, portals)).length
}

/** The home folder implied by a scan root and its display form ("/Users/alex/Documents/code" and "~/Documents/code"); '' when unknown. */
export function homeFromRoot(root: string | undefined, display: string | undefined): string {
  if (!root || !display || !display.startsWith('~')) return ''
  const rest = display.slice(1)
  const flat = (v: string) => v.replace(/\\/g, '/')
  if (!flat(root).endsWith(flat(rest))) return ''
  return root.slice(0, root.length - rest.length).replace(/[\\/]+$/, '')
}

/** The home folder from the first scan root whose display form starts with "~"; '' when none does. */
export function homeFromRoots(roots: readonly string[] | null | undefined, displays: readonly string[] | null | undefined): string {
  for (let i = 0; i < (roots?.length ?? 0); i++) {
    const h = homeFromRoot(roots?.[i], displays?.[i])
    if (h) return h
  }
  return ''
}

/** A path with the home folder shown as "~", as the Projects rows show it; other paths stay as they are. */
export function tildePath(path: string, home: string): string {
  if (!home) return path
  const h = home.replace(/[\\/]+$/, '')
  if (path === h) return '~'
  const next = path.charAt(h.length)
  return path.startsWith(h) && (next === '/' || next === '\\') ? `~${path.slice(h.length)}` : path
}

/** Search matches the folder name, the remote name, the CodeCommit name and the path. */
export function repoMatches(r: Repo, query: string): boolean {
  return rankRepo(r, query) !== null
}

export function providerKind(r: Repo): ProviderKind | null {
  switch (r.provider) {
    case RepoProvider.ProviderCodeCommit:
      return 'aws'
    case RepoProvider.ProviderGitHub:
      return 'github'
    case RepoProvider.ProviderBitbucket:
      return 'bitbucket'
    default:
      return null
  }
}

function passes(r: Repo, o: GroupOptions): boolean {
  if (o.filter === 'todo' && !needsAction(r, o.profiles ?? [], o.portals ?? [])) return false
  if (o.filter === 'changes' && !hasChanges(r)) return false
  if (o.source !== 'all' && r.provider !== o.source) return false
  return true
}

/**
 * The sections of the page. Every project is a section in the order the backend gives; Ungrouped comes last.
 * With a filter or a search active, sections without a match are dropped; a search that matches a project's own
 * name keeps all of that project's repositories (the other filters still apply).
 */
export function groupRepos(state: ReposState | null, o: GroupOptions, ungroupedName = ''): Section[] {
  if (!state) return []
  if (o.showHidden) {
    const hidden = searchRepos((state.hiddenRepos ?? []).filter((r) => o.source === 'all' || r.provider === o.source), o.query)
    return [{ id: HIDDEN_SECTION, repos: hidden, total: hidden.length }]
  }
  const repos = state.repos ?? []
  const constrained = o.filter !== 'all' || o.source !== 'all' || o.query.trim() !== ''
  const q = o.query.trim().toLowerCase()
  const known = new Set((state.projects ?? []).map((p) => p.id))
  const byProject = new Map<string, Repo[]>()
  for (const r of repos) {
    const id = r.projectId && known.has(r.projectId) ? r.projectId : UNGROUPED
    const list = byProject.get(id) ?? []
    list.push(r)
    byProject.set(id, list)
  }
  const out: Section[] = []
  const build = (id: string, project: ProjectInfo | undefined, name: string) => {
    const all = byProject.get(id) ?? []
    const nameHit = q !== '' && name.toLowerCase().includes(q)
    // Better matches first (design 5.2); a repository kept only by its project's name follows the real matches.
    const passing = all.filter((r) => passes(r, o))
    const hits = searchRepos(passing, o.query)
    const shown = nameHit ? [...hits, ...passing.filter((r) => !hits.includes(r))] : hits
    if (constrained && shown.length === 0) return
    if (!project && all.length === 0) return
    out.push({ id, project, repos: shown, total: all.length })
  }
  for (const p of state.projects ?? []) build(p.id, p, p.name)
  build(UNGROUPED, undefined, ungroupedName)
  return out
}

/** How many listed repositories have local changes, for the "With changes" count. */
export function changesCount(repos: Repo[] | null | undefined): number {
  return (repos ?? []).filter(hasChanges).length
}

/** Repository IDs in the order the rows appear (collapsed sections have no rows on screen). */
export function displayOrder(sections: Section[], collapsed: ReadonlySet<string>): string[] {
  const out: string[] = []
  for (const s of sections) if (!collapsed.has(s.id)) for (const r of s.repos) out.push(r.id)
  return out
}

/** The suggestion to show: one at a time, the first the backend lists whose repositories are all still ungrouped. */
export function activeSuggestion(state: ReposState | null): ProjectSuggestion | null {
  if (!state) return null
  const ungrouped = new Set((state.repos ?? []).filter((r) => !r.projectId).map((r) => r.id))
  for (const s of state.suggestions ?? []) {
    const ids = (s.repoIds ?? []).filter((id) => ungrouped.has(id))
    if (ids.length >= 2) return { ...s, repoIds: ids }
  }
  return null
}

/** Account chips peeked under a collapsed project: the distinct bound profiles, in order of first use. */
export function peekProfiles(repos: Repo[]): Repo[] {
  const seen = new Set<string>()
  const out: Repo[] = []
  for (const r of repos) {
    const p = r.binding.profile
    if (!p || !r.binding.profileKnown || seen.has(p)) continue
    seen.add(p)
    out.push(r)
  }
  return out
}

/** Pre-ticked in the found chooser: the last commit is within 90 days of `now`. */
export function recentlyActive(r: Repo, now: number): boolean {
  if (!r.lastCommitAt) return false
  const t = Date.parse(r.lastCommitAt)
  return !Number.isNaN(t) && now - t <= RECENT_DAYS * 24 * 3600 * 1000
}

/** The found repositories to tick when the chooser opens. */
export function defaultFoundSelection(found: Repo[], now: number): Set<string> {
  return new Set(found.filter((r) => recentlyActive(r, now)).map((r) => r.id))
}

/** The ungrouped repositories to tick in the New project dialog: those whose name starts with the typed project name. */
export function prefixMatches(name: string, repos: Repo[]): string[] {
  const n = name.trim().toLowerCase()
  if (n.length < 2) return []
  return repos.filter((r) => !r.projectId && r.localName.toLowerCase().startsWith(n)).map((r) => r.id)
}

// ---------------------------------------------------------------- moving

export interface MovePreviewItem {
  repo: Repo
  /** Undefined when the identity service does not know the repository (the plain move is used). */
  preview?: MoveIdentityPreview
}

/** The move writes the repository's git config (set or remove the author). */
export function writesIdentity(it: MovePreviewItem): boolean {
  const a = it.preview?.plan.action
  return a === RepoAction.RepoActionSet || a === RepoAction.RepoActionRemove
}

/** The repository keeps a value of its own (the app's own setting, or one set outside the app). */
export function keepsOwnIdentity(it: MovePreviewItem): boolean {
  const a = it.preview?.plan.action
  return a === RepoAction.RepoActionKeepOwn || a === RepoAction.RepoActionKeepExternal
}

export interface MoveSplit {
  /** Their author identity will change. */
  change: MovePreviewItem[]
  /** They keep their own. */
  keep: MovePreviewItem[]
  /** Everything else: nothing changes in the config. */
  rest: MovePreviewItem[]
}

export function splitMove(items: MovePreviewItem[]): MoveSplit {
  const out: MoveSplit = { change: [], keep: [], rest: [] }
  for (const it of items) (writesIdentity(it) ? out.change : keepsOwnIdentity(it) ? out.keep : out.rest).push(it)
  return out
}

/** A confirm is needed when a config file will be written, or the destination has an identity that some repositories keep out of. */
export function needsMoveConfirm(split: MoveSplit, destinationHasIdentity: boolean): boolean {
  return split.change.length > 0 || (destinationHasIdentity && split.keep.length > 0)
}

/** The repositories to move: the dragged or focused row, or the whole selection when that row is part of it. */
export function moveTargets(rowId: string, selected: ReadonlySet<string>, order: readonly string[]): string[] {
  if (!selected.has(rowId)) return [rowId]
  return order.filter((id) => selected.has(id))
}

/** Repositories that cannot be moved or ticked: hidden rows, and rows that wait for the review. */
export function movable(r: Repo): boolean {
  return !r.hidden && !r.pending
}
