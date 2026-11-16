import { describe, expect, it } from 'vitest'
import { RepoAction, RepoProvider } from '../../api/models'
import type { MoveIdentityPreview, ProjectInfo, Repo, ReposState } from '../../api/models'
import { activeSuggestion, changesCount, defaultFoundSelection, displayOrder, groupRepos, hasChanges, moveTargets, needsFix, needsMoveConfirm, peekProfiles, prefixMatches, recentlyActive, repoMatches, splitMove } from './model'
import type { GroupOptions, MovePreviewItem } from './model'

const NOW = Date.parse('2026-11-04T04:00:00Z')
const DAY = 24 * 3600 * 1000

function repo(name: string, over: Partial<Repo> = {}): Repo {
  return {
    id: `id_${name}`,
    path: `/Users/alex/Documents/code/${name}`,
    displayPath: `~/Documents/code/${name}`,
    name,
    localName: name,
    remoteName: name,
    remote: 'origin',
    remoteUrl: '',
    remoteKind: 'https',
    hasUserinfo: false,
    binding: { source: '', profileKnown: false, needsRepair: false, legacyPlain: false },
    owner: { status: 'unknown', candidates: [], checkedWith: 0, skipped: 0 },
    toFix: false,
    extra: false,
    provider: RepoProvider.ProviderCodeCommit,
    hidden: false,
    pending: false,
    ...over,
  } as unknown as Repo
}
const withStatus = (changes: number): Repo['status'] => ({ branch: 'main', changes, ahead: 0, behind: 0 }) as unknown as Repo['status']
const proj = (id: string, name: string, order = 0): ProjectInfo => ({ id, name, collapsed: false, order, repoCount: 0 })

function state(repos: Repo[], projects: ProjectInfo[], extra: Partial<ReposState> = {}): ReposState {
  return { repos, projects, suggestions: [], found: [], hiddenRepos: [], counts: { all: repos.length, toFix: 0, bound: 0, found: 0, hidden: 0 }, ...extra } as unknown as ReposState
}
const opts = (o: Partial<GroupOptions> = {}): GroupOptions => ({ filter: 'all', source: 'all', query: '', showHidden: false, ...o })
const ids = (s: ReturnType<typeof groupRepos>) => s.map((x) => `${x.id || '-'}:${x.repos.map((r) => r.localName).join(',')}`)

describe('groupRepos', () => {
  const projects = [proj('p_acme', 'acme', 0), proj('p_globex', 'globex', 1), proj('p_empty', 'umbrella', 2)]
  const list = [
    repo('acme_api', { projectId: 'p_acme', status: withStatus(2) }),
    repo('acme_auth', { projectId: 'p_acme', status: withStatus(0), toFix: true }),
    repo('globex_app', { projectId: 'p_globex', provider: RepoProvider.ProviderGitHub, status: withStatus(0), remoteName: 'app-remote' }),
    repo('scratch', { status: withStatus(1) }),
  ]
  const st = state(list, projects)

  it('every project is a section in order, empty ones included, Ungrouped last', () => {
    expect(ids(groupRepos(st, opts()))).toEqual(['p_acme:acme_api,acme_auth', 'p_globex:globex_app', 'p_empty:', '-:scratch'])
  })

  it('a repository whose project is unknown falls into Ungrouped', () => {
    const orphan = state([repo('lost', { projectId: 'p_gone' })], projects)
    expect(ids(groupRepos(orphan, opts()))).toEqual(['p_acme:', 'p_globex:', 'p_empty:', '-:lost'])
  })

  it('"To fix" keeps CodeCommit rows to fix only and drops the sections left empty', () => {
    expect(ids(groupRepos(st, opts({ filter: 'todo' })))).toEqual(['p_acme:acme_auth'])
  })

  it('"With changes" uses the git status', () => {
    expect(ids(groupRepos(st, opts({ filter: 'changes' })))).toEqual(['p_acme:acme_api', '-:scratch'])
    expect(changesCount(list)).toBe(2)
    expect(hasChanges(list[3])).toBe(true)
  })

  it('the provider filter', () => {
    expect(ids(groupRepos(st, opts({ source: RepoProvider.ProviderGitHub })))).toEqual(['p_globex:globex_app'])
  })

  it('search matches the folder name, the remote name or the path, and keeps matches under their project', () => {
    expect(ids(groupRepos(st, opts({ query: 'AUTH' })))).toEqual(['p_acme:acme_auth'])
    expect(ids(groupRepos(st, opts({ query: 'app-remote' })))).toEqual(['p_globex:globex_app'])
    expect(ids(groupRepos(st, opts({ query: 'documents/code/scratch' })))).toEqual(['-:scratch'])
    expect(repoMatches(list[0], '')).toBe(true)
    expect(ids(groupRepos(st, opts({ query: 'zzz' })))).toEqual([])
  })

  it('a search that names a project keeps all of its repositories', () => {
    expect(ids(groupRepos(st, opts({ query: 'globex' })))).toEqual(['p_globex:globex_app'])
    expect(ids(groupRepos(st, opts({ query: 'acme' })))).toEqual(['p_acme:acme_api,acme_auth'])
  })

  it('the hidden list is one flat section', () => {
    const h = state(list, projects, { hiddenRepos: [repo('old_fork', { hidden: true })] })
    expect(ids(groupRepos(h, opts({ showHidden: true })))).toEqual(['__hidden__:old_fork'])
  })

  it('needsFix is for CodeCommit rows only', () => {
    expect(needsFix(repo('a', { toFix: true }))).toBe(true)
    expect(needsFix(repo('a', { toFix: true, provider: RepoProvider.ProviderGitHub }))).toBe(false)
  })

  it('display order skips collapsed sections', () => {
    const s = groupRepos(st, opts())
    expect(displayOrder(s, new Set())).toEqual(['id_acme_api', 'id_acme_auth', 'id_globex_app', 'id_scratch'])
    expect(displayOrder(s, new Set(['p_acme']))).toEqual(['id_globex_app', 'id_scratch'])
  })

  it('peek lists each bound account once', () => {
    const b = (profile: string) => ({ ...list[0].binding, profile, profileKnown: true })
    const peeked = peekProfiles([repo('a', { binding: b('acme') }), repo('b', { binding: b('acme') }), repo('c', { binding: b('globex') }), repo('d')])
    expect(peeked.map((r) => r.binding.profile)).toEqual(['acme', 'globex'])
  })
})

describe('suggestions', () => {
  it('shows one at a time, only for repositories that are still ungrouped', () => {
    const a = repo('initech_a')
    const b = repo('initech_b')
    const c = repo('initech_c')
    const d = repo('initech_d', { projectId: 'p1' })
    const st = state([a, b, c, d], [proj('p1', 'x')], {
      suggestions: [
        { key: 'prefix:initech', kind: 'prefix', label: 'initech', repoIds: [a.id, b.id, c.id, d.id] },
        { key: 'folder:x', kind: 'folder', label: 'x', repoIds: [a.id, b.id, c.id] },
      ],
    } as Partial<ReposState>)
    const s = activeSuggestion(st)
    expect(s?.key).toBe('prefix:initech')
    expect(s?.repoIds).toEqual([a.id, b.id, c.id])
    expect(activeSuggestion(state([a], []))).toBeNull()
    expect(activeSuggestion(null)).toBeNull()
  })

  it('a suggestion left with fewer than two ungrouped repositories is dropped', () => {
    const a = repo('x_a')
    const b = repo('x_b', { projectId: 'p1' })
    const st = state([a, b], [proj('p1', 'p')], { suggestions: [{ key: 'prefix:x', kind: 'prefix', label: 'x', repoIds: [a.id, b.id] }] } as Partial<ReposState>)
    expect(activeSuggestion(st)).toBeNull()
  })

  it('the New project dialog pre-ticks ungrouped repositories whose name starts with the project name', () => {
    const list = [repo('initech_a'), repo('Initech-b'), repo('acme_x'), repo('initech_c', { projectId: 'p' })]
    expect(prefixMatches('initech', list)).toEqual(['id_initech_a', 'id_Initech-b'])
    expect(prefixMatches('i', list)).toEqual([])
    expect(prefixMatches('', list)).toEqual([])
  })
})

describe('the found chooser', () => {
  it('pre-ticks repositories with a commit in the last 90 days', () => {
    const fresh = repo('fresh', { lastCommitAt: new Date(NOW - 10 * DAY).toISOString() })
    const edge = repo('edge', { lastCommitAt: new Date(NOW - 90 * DAY).toISOString() })
    const old = repo('old', { lastCommitAt: new Date(NOW - 91 * DAY).toISOString() })
    const none = repo('none')
    expect(recentlyActive(old, NOW)).toBe(false)
    expect([...defaultFoundSelection([fresh, edge, old, none], NOW)]).toEqual([fresh.id, edge.id])
  })
})

describe('moving', () => {
  const preview = (action: RepoAction): MoveIdentityPreview => ({ repoId: 'x', plan: { action } as MoveIdentityPreview['plan'], planHash: 'h' })
  const item = (name: string, action?: RepoAction): MovePreviewItem => ({ repo: repo(name), preview: action ? preview(action) : undefined })

  it('splits the plans into change, keep and rest', () => {
    const s = splitMove([item('a', RepoAction.RepoActionSet), item('b', RepoAction.RepoActionRemove), item('c', RepoAction.RepoActionKeepOwn), item('d', RepoAction.RepoActionKeepExternal), item('e', RepoAction.RepoActionUnchanged), item('f')])
    expect(s.change.map((i) => i.repo.name)).toEqual(['a', 'b'])
    expect(s.keep.map((i) => i.repo.name)).toEqual(['c', 'd'])
    expect(s.rest.map((i) => i.repo.name)).toEqual(['e', 'f'])
  })

  it('confirms when a config will be written, or the destination has an identity and some repositories keep their own', () => {
    expect(needsMoveConfirm(splitMove([item('a', RepoAction.RepoActionSet)]), false)).toBe(true)
    expect(needsMoveConfirm(splitMove([item('a', RepoAction.RepoActionKeepOwn)]), true)).toBe(true)
    expect(needsMoveConfirm(splitMove([item('a', RepoAction.RepoActionKeepOwn)]), false)).toBe(false)
    expect(needsMoveConfirm(splitMove([item('a', RepoAction.RepoActionUnchanged), item('b')]), true)).toBe(false)
  })

  it('a row that is part of the selection moves the whole selection, in display order', () => {
    const order = ['a', 'b', 'c', 'd']
    expect(moveTargets('b', new Set(['d', 'b']), order)).toEqual(['b', 'd'])
    expect(moveTargets('c', new Set(['d', 'b']), order)).toEqual(['c'])
  })
})
