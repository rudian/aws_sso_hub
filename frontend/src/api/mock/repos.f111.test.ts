import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { toUiError } from '../errors'
import { BindingSource, ErrorCode, RepoProvider, SuggestionKind } from '../models'
import { mock } from './index'
import { folderNameReason } from './repos'
import { getWorld, resetWorld } from './world'

async function call<T>(p: Promise<T>, ms = 2000): Promise<T> {
  const settled = p.then(
    (v) => ({ ok: true as const, v }),
    (e: unknown) => ({ ok: false as const, e }),
  )
  await vi.advanceTimersByTimeAsync(ms)
  const r = await settled
  if (!r.ok) throw r.e
  return r.v
}
const code = async (p: Promise<unknown>) => {
  try {
    await call(p)
    return null
  } catch (e) {
    return toUiError(e).code
  }
}

describe('mock repository service (F1-11)', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-11-04T04:00:00Z'))
    resetWorld('fresh')
  })
  afterEach(() => vi.useRealTimers())

  it('a scan finds new repositories; they wait in the review set and the counts do not change', async () => {
    const before = await call(mock.repos.GetState())
    expect(before.found).toEqual([])
    const st = await call(mock.repos.Scan(), 3000)
    expect(st.found?.length).toBe(4)
    expect(st.counts).toMatchObject({ all: 16, found: 4, hidden: 0 })
    expect(st.repos).toHaveLength(16)
    const gh = st.found?.find((r) => r.localName === 'dotfiles')
    expect(gh).toMatchObject({ provider: RepoProvider.ProviderGitHub, pending: true, remoteName: 'dotfiles' })
    expect(gh?.binding.source).toBe(BindingSource.BindingMachine)
    expect(gh?.lastCommitAt).toBeTruthy()
    // A second scan finds nothing more.
    expect((await call(mock.repos.Scan(), 3000)).found).toHaveLength(4)
  })

  it('review: show some, hide the rest; hidden ones can come back; unknown IDs are refused', async () => {
    const st = await call(mock.repos.Scan(), 3000)
    const [a, b, c, d] = (st.found ?? []).map((r) => r.id)
    const after = await call(mock.repos.ResolveFound([a, b], [c, d]))
    expect(after.found).toEqual([])
    expect(after.counts).toMatchObject({ all: 18, found: 0, hidden: 2 })
    expect(after.hiddenRepos?.map((r) => r.id).sort()).toEqual([c, d].sort())
    const back = await call(mock.repos.SetHidden(c, false))
    expect(back.counts).toMatchObject({ all: 19, hidden: 1 })
    expect(await code(mock.repos.ResolveFound(['nope'], null))).toBe(ErrorCode.ErrRepoNotFound)
  })

  it('hidden repositories get no status and no fetch', async () => {
    const id = getWorld().repos[0].id
    await call(mock.repos.SetHidden(id, true))
    const st = await call(mock.repos.RefreshStatus(null))
    expect(st.repos?.every((r) => r.status)).toBe(true)
    expect(st.hiddenRepos?.[0].status).toBeUndefined()
    expect(await code(mock.repos.FetchNow(id))).toBe(ErrorCode.ErrInvalidInput)
  })

  it('projects: create with members, move, rename, collapse, remove; suggestions come and go', async () => {
    const st0 = await call(mock.repos.GetState())
    const sug = st0.suggestions?.find((s) => s.kind === SuggestionKind.SuggestPrefix && s.label === 'initech')
    expect(sug?.repoIds?.length).toBeGreaterThanOrEqual(3)
    const st1 = await call(mock.repos.CreateProjectWith(' initech ', sug?.repoIds ?? []))
    expect(st1.projects).toHaveLength(1)
    expect(st1.projects?.[0]).toMatchObject({ name: 'initech', repoCount: sug?.repoIds?.length })
    expect(st1.suggestions?.some((s) => s.key === sug?.key)).toBe(false)
    const pid = st1.projects?.[0].id as string
    const first = (sug?.repoIds ?? [])[0]
    expect((await call(mock.repos.MoveToProject([first], ''))).repos?.find((r) => r.id === first)?.projectId).toBeUndefined()
    expect((await call(mock.repos.RenameProject(pid, 'Initech'))).projects?.[0].name).toBe('Initech')
    expect((await call(mock.repos.SetProjectCollapsed(pid, true))).projects?.[0].collapsed).toBe(true)
    expect((await call(mock.repos.RemoveProject(pid))).projects).toEqual([])
    expect(await code(mock.repos.MoveToProject([first], 'p_nope'))).toBe(ErrorCode.ErrProjectNotFound)
    expect(await code(mock.repos.CreateProject('  '))).toBe(ErrorCode.ErrInvalidInput)
    const dismissed = await call(mock.repos.DismissSuggestion('prefix:acme'))
    expect(dismissed.suggestions?.some((s) => s.key === 'prefix:acme')).toBe(false)
  })

  it('add folder: GitHub and plain repositories are listed with the machine default; CodeCommit goes the old way', async () => {
    const res = await call(mock.repos.AddFolder('/Users/alex/work/github-tool'))
    expect(res.added).toBe(true)
    const row = res.state.repos?.find((r) => r.id === res.repoId)
    expect(row).toMatchObject({ provider: RepoProvider.ProviderGitHub, localName: 'github-tool', remoteName: 'github-tool', extra: true, pending: false })
    expect(row?.binding.source).toBe(BindingSource.BindingMachine)
    expect(await code(mock.repos.AddFolder('/Users/alex/work/notrepo'))).toBe(ErrorCode.ErrRepoNotFound)
  })

  it('moved-folder answers map to their own codes', async () => {
    const w = getWorld()
    const r = w.repos[0]
    r.movedFrom = { oldId: 'r_old', name: 'old_name' }
    const st = await call(mock.repos.DeclineMoved(r.id))
    expect(st.repos?.find((x) => x.id === r.id)?.movedFrom).toBeUndefined()
    expect(await code(mock.repos.DeclineMoved(r.id))).toBe(ErrorCode.ErrMovedGone) // out of date, not unknown
    expect(await code(mock.repos.DeclineMoved('r_nope'))).toBe(ErrorCode.ErrRepoNotFound)
    r.movedFrom = { oldId: 'r_old', name: 'old_name' }
    r.projectId = 'p_x'
    expect(await code(mock.repos.ReattachMoved(r.id))).toBe(ErrorCode.ErrMovedHasData)
  })

  it('clone folder names: optional, validated like the Go engine', async () => {
    const req = { profile: 'acme', region: 'us-east-1', repoName: 'acme_api_x', parentDir: '/Users/alex/work', folderName: 'api-main' }
    expect((await call(mock.repos.PreviewClone(req))).dest).toBe('/Users/alex/work/api-main')
    expect((await call(mock.repos.PreviewClone({ ...req, folderName: '' }))).folderName).toBe('acme_api_x')
    for (const bad of ['CON', 'nul.txt', 'x.', 'a/b', '..']) expect(await code(mock.repos.PreviewClone({ ...req, folderName: bad }))).toBe(ErrorCode.ErrInvalidInput)
    const cloned = await call(mock.repos.Clone(req), 4000)
    expect(cloned.repo).toMatchObject({ localName: 'api-main', remoteName: 'acme_api_x', provider: RepoProvider.ProviderCodeCommit })
  })

  it('folderNameReason table', () => {
    const table: Array<[string, string]> = [
      ['api-main', ''], ['console', ''], ['COM0', ''], ['', 'empty'], ['.', 'dots'], ['a\\b', 'separator'], ['CON', 'reserved'],
      ['com1.old', 'reserved'], ['LPT9', 'reserved'], ['COM¹', 'reserved'], ['x ', 'trailing'], ['a:b', 'bad-char'], ['a'.repeat(256), 'too-long'],
    ]
    for (const [name, reason] of table) expect(folderNameReason(name), name).toBe(reason)
  })

  it('non-CodeCommit rows never need fixing, hiding a found row takes it out of the review, names count code points', async () => {
    const st = await call(mock.repos.Scan(), 3000)
    expect(st.found?.length).toBe(4)
    const first = st.found?.[0]?.id as string
    const after = await call(mock.repos.ResolveFound([st.found?.[1]?.id as string], []))
    expect(after.repos?.filter((r) => r.provider !== RepoProvider.ProviderCodeCommit).every((r) => !r.toFix)).toBe(true)
    const hid = await call(mock.repos.SetHidden(first, true))
    expect(hid.found?.some((r) => r.id === first)).toBe(false)
    expect(hid.hiddenRepos?.some((r) => r.id === first)).toBe(true)
    expect(hid.counts.found).toBe(hid.found?.length)
    const back = await call(mock.repos.SetHidden(first, false))
    expect(back.repos?.some((r) => r.id === first)).toBe(true)
    expect(back.found?.some((r) => r.id === first)).toBe(false)
    // 100 Chinese characters are fine (the Go engine counts characters), 101 are not
    expect((await call(mock.repos.CreateProject('仓'.repeat(100)))).projects?.length).toBe(1)
    expect(await code(mock.repos.CreateProject('仓'.repeat(101)))).toBe(ErrorCode.ErrInvalidInput)
  })

  it('background fetch runs only for bound or opened repositories', async () => {
    const w = getWorld()
    const plain = w.repos[0] // global default binding: not bound by the person
    expect(await code(mock.repos.FetchNow(plain.id))).toBe(ErrorCode.ErrInvalidInput)
    await call(mock.repos.MarkOpened(plain.id))
    expect(await code(mock.repos.FetchNow(plain.id))).toBeNull()
  })

  it('the done scenario shows projects, a hidden row and the moved-folder question', async () => {
    resetWorld('done')
    const st = await call(mock.repos.GetState())
    expect(st.projects?.map((p) => p.name)).toEqual(['acme', 'globex', 'umbrella'])
    expect(st.counts.hidden).toBe(1)
    const moved = st.repos?.find((r) => r.movedFrom)
    expect(moved?.movedFrom).toMatchObject({ name: 'initech_dashboard' })
    const answered = await call(mock.repos.ReattachMoved(moved?.id as string))
    expect(answered.repos?.some((r) => r.movedFrom)).toBe(false)
  })
})
