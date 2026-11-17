import type { Repo } from '../api/models'
import { MAIN_FOLDER } from '../state/workspace'
import type { WorkTree } from '../state/workspace'

/** The working trees of a repository, main workspace first. Milestone 3 appends one row per AI task that has its own folder. */
export function workTreesOf(repo: Repo): WorkTree[] {
  return [{ id: MAIN_FOLDER, kind: 'main', branch: repo.status?.branch ?? '' }]
}

/** Find the repository a route names: by ID, and (mock links and tests) by folder name. */
export function findRepo(repos: readonly Repo[] | null | undefined, key: string | undefined): Repo | undefined {
  if (!key || !repos) return undefined
  return repos.find((r) => r.id === key) ?? repos.find((r) => r.localName === key)
}
