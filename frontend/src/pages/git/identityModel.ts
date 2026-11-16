import { IdentitySourceKind, RepoAction, RepoIdentityMode } from '../../api/models'
import type { Author, IdentitySource, ProjectIdentity, RepoChangePlan, RepoIdentity } from '../../api/models'
import type { MessageKey } from '../../i18n'

/** One source tag of the effective identity table (design 5.12). */
export interface SourceTag {
  key: MessageKey
  /** neutral = grey, proj = blue, repo = amber, warn = amber with emphasis (the terminal environment). */
  tone: 'neutral' | 'proj' | 'repo' | 'warn'
  params?: Record<string, string>
  /** Plain text for the tooltip: the file or rule behind the tag. */
  title?: { key: MessageKey; params: Record<string, string> }
}

/** Map a backend source to the tag a row shows. Unknown kinds fall back to a neutral "from global" tag. */
export function sourceTag(src: IdentitySource): SourceTag {
  const file = src.file ?? ''
  switch (src.kind) {
    case IdentitySourceKind.IdentitySrcNone:
      return { key: 'identity.src.none', tone: 'neutral' }
    case IdentitySourceKind.IdentitySrcGlobal:
      return { key: 'identity.src.global', tone: 'neutral', title: file ? { key: 'identity.src.title.file', params: { file } } : undefined }
    case IdentitySourceKind.IdentitySrcSystem:
      return { key: 'identity.src.system', tone: 'neutral', title: file ? { key: 'identity.src.title.file', params: { file } } : undefined }
    case IdentitySourceKind.IdentitySrcProject:
      return { key: 'identity.src.project', tone: 'proj' }
    case IdentitySourceKind.IdentitySrcFromProject:
      return { key: 'identity.src.fromProject', tone: 'proj' }
    case IdentitySourceKind.IdentitySrcRepo:
      return { key: src.external ? 'identity.src.repoExternal' : 'identity.src.repo', tone: 'repo', title: file ? { key: 'identity.src.title.file', params: { file } } : undefined }
    case IdentitySourceKind.IdentitySrcConditional: {
      const where = src.ruleFile || ''
      return {
        key: where ? 'identity.src.conditional' : 'identity.src.conditionalNoFile',
        tone: 'neutral',
        params: where ? { file: where } : undefined,
        title: src.rule ? { key: 'identity.src.title.rule', params: { rule: src.rule } } : undefined,
      }
    }
    case IdentitySourceKind.IdentitySrcInclude:
      return { key: 'identity.src.include', tone: 'neutral', title: file ? { key: 'identity.src.title.file', params: { file } } : undefined }
    case IdentitySourceKind.IdentitySrcWorktree:
      return { key: 'identity.src.worktree', tone: 'repo' }
    case IdentitySourceKind.IdentitySrcCommand:
      return { key: 'identity.src.command', tone: 'neutral' }
    case IdentitySourceKind.IdentitySrcEnv:
      return { key: 'identity.src.env', tone: 'warn', title: src.var ? { key: 'identity.src.title.var', params: { var: src.var } } : undefined }
    default:
      return { key: 'identity.src.global', tone: 'neutral' }
  }
}

const NAME_BAD = /[<>\u0000-\u001f]/

/** The same rules as internal/identity.Validate: a name without < > or control characters, an email like a@b. */
export function validateAuthor(a: Author): { name: boolean; email: boolean } {
  const name = a.name.trim()
  const email = a.email.trim()
  return {
    name: name !== '' && name.length <= 200 && !NAME_BAD.test(name),
    email: email.length <= 254 && /^[^\s@<>]+@[^\s@<>]+$/.test(email),
  }
}

export const sameAuthor = (a?: Author | null, b?: Author | null): boolean => (!a && !b) || (!!a && !!b && a.name.trim() === b.name.trim() && a.email.trim() === b.email.trim())

/** What a repository follows today. */
export type RepoFollow = 'project' | 'global' | 'own'

/** A value in the repository's own config: recorded by the app, or set outside it (a repo or worktree source). */
const ownSource = (row: RepoIdentity): boolean => {
  const k = row.setting?.source?.kind
  return k === IdentitySourceKind.IdentitySrcRepo || k === IdentitySourceKind.IdentitySrcWorktree
}

/** The option a repository is on today: its own values (recorded or set outside the app), the follow-global marker, or its project. */
export function repoFollow(row: RepoIdentity, project?: ProjectIdentity): RepoFollow {
  if (row.override || ownSource(row)) return 'own'
  if (row.followsGlobal) return 'global'
  return project ? 'project' : 'global'
}

/** The values the "own" fields start with: the app's record, else what the repository's config says now (also when set outside the app), else global. */
export function repoOwnStart(row: RepoIdentity, global: { name: string; email: string }): Author {
  if (row.override) return { name: row.override.name, email: row.override.email }
  if (ownSource(row) && row.setting) return { name: row.setting.name, email: row.setting.email }
  return { name: row.setting?.name || global.name, email: row.setting?.email || global.email }
}

/** The options a repository's editor offers (design 5.12): follow its project (project members), follow global, its own. */
export function repoOptions(project?: ProjectIdentity): { project: boolean; global: boolean } {
  return { project: !!project, global: true }
}

/**
 * The request for a chosen option. A project member that follows global writes the sticky marker (project saves
 * skip it); an ungrouped repository just drops its own values.
 */
export function repoRequest(row: RepoIdentity, choice: RepoFollow, own: Author, project?: ProjectIdentity) {
  if (choice === 'own') return { repoId: row.repoId, mode: RepoIdentityMode.RepoModeOwn, identity: { name: own.name.trim(), email: own.email.trim() } }
  if (choice === 'global') return { repoId: row.repoId, mode: row.projectId && project ? RepoIdentityMode.RepoModeGlobal : RepoIdentityMode.RepoModeProject }
  return { repoId: row.repoId, mode: RepoIdentityMode.RepoModeProject }
}

/** Show a field error once the field was touched (or a save was tried), or while its text is wrong. */
export const fieldError = (value: string, ok: boolean, touched: boolean): boolean => !ok && (touched || value !== '')

/** A plan row of the project sheet's list. */
export type PlanKind = 'change' | 'unchanged' | 'keeps-own' | 'keeps-external' | 'missing' | 'error'

export function planKind(p: RepoChangePlan): PlanKind {
  switch (p.action) {
    case RepoAction.RepoActionSet:
    case RepoAction.RepoActionRemove:
      return 'change'
    case RepoAction.RepoActionKeepOwn:
      return 'keeps-own'
    case RepoAction.RepoActionKeepExternal:
      return 'keeps-external'
    case RepoAction.RepoActionSkipMissing:
      return 'missing'
    case RepoAction.RepoActionError:
      return 'error'
    default:
      return 'unchanged'
  }
}

/** Repositories that change come first; the ones that keep their own setting come last (design 5.12). */
export function sortPlans(plans: RepoChangePlan[]): RepoChangePlan[] {
  const rank: Record<PlanKind, number> = { error: 0, change: 1, unchanged: 2, missing: 3, 'keeps-own': 4, 'keeps-external': 4 }
  return plans.map((p, i) => ({ p, i })).sort((a, b) => rank[planKind(a.p)] - rank[planKind(b.p)] || a.i - b.i).map((x) => x.p)
}

/** "What was there before" for a struck-through value: the old local email, else what the repository used (global, a rule), else "". */
export function wasText(p: RepoChangePlan, rows?: Map<string, RepoIdentity>): string {
  return p.before?.email || rows?.get(p.repoId)?.setting?.email || ''
}

/** The names of an apply's failed repositories. */
export function failedNames(outcomes: { name: string; ok: boolean }[] | null | undefined): string[] {
  return (outcomes ?? []).filter((o) => !o.ok).map((o) => o.name)
}
