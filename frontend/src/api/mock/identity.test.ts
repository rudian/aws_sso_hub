import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { toUiError } from '../errors'
import { ErrorCode, IdentitySourceKind, RepoAction, RepoIdentityMode } from '../models'
import { onEvent } from '../events'
import { mockIdentityService as svc, repoIdOf, resetIdentityMock } from './identity'
import { add, ctx, del } from './world'

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

describe('identity mock', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    resetIdentityMock('default')
  })
  afterEach(() => vi.useRealTimers())

  it('shows every source kind of the effective identity table', async () => {
    const ov = await call(svc.GetOverview())
    expect(ov.global).toMatchObject({ set: true, email: 'alex@acme.example.com' })
    const rows = [...ov.projects!.flatMap((p) => p.repos ?? []), ...ov.ungrouped!]
    const kinds = new Set(rows.map((r) => r.effective.source.kind))
    for (const k of [IdentitySourceKind.IdentitySrcFromProject, IdentitySourceKind.IdentitySrcRepo, IdentitySourceKind.IdentitySrcConditional, IdentitySourceKind.IdentitySrcGlobal]) expect(kinds).toContain(k)
    const ext = rows.find((r) => r.name === 'acme_merchant_portal_v4')!
    expect(ext.effective.source).toMatchObject({ kind: IdentitySourceKind.IdentitySrcRepo, external: true })
    expect(rows.find((r) => r.name === 'initech_infra')!.effective.emailSource.rule).toContain('gitdir:')
  })

  it('reports a terminal environment override in the env scenario', async () => {
    resetIdentityMock('env')
    const ov = await call(svc.GetOverview())
    expect(ov.env.active).toBe(true)
    const row = ov.projects![0]!.repos![0]!
    expect(row.effective.source.kind).toBe(IdentitySourceKind.IdentitySrcEnv)
    expect(row.setting.source.kind).toBe(IdentitySourceKind.IdentitySrcFromProject)
  })

  it('project apply writes what the preview listed, then undo restores', async () => {
    const before = await call(svc.GetOverview())
    const req = { projectId: 'p_acme', identity: { name: 'Alex', email: 'alex@acme-new.example.com' } }
    const pv = await call(svc.PreviewProject(req))
    expect(pv.writes).toBe(2)
    expect(pv.repos!.at(-1)!.action).toMatch(/keep/)
    expect(await code(svc.ApplyProject(req, 'stale'))).toBe(ErrorCode.ErrConfigChanged)
    const res = await call(svc.ApplyProject(req, pv.planHash))
    expect(res.outcomes!.filter((o) => o.ok)).toHaveLength(pv.repos!.length)
    const after = await call(svc.GetOverview())
    const written = pv.repos!.filter((p) => p.action === RepoAction.RepoActionSet).map((p) => p.name)
    for (const n of written) expect(after.projects![0]!.repos!.find((r) => r.name === n)!.effective.email).toBe('alex@acme-new.example.com')
    await call(svc.Undo(res.undoId))
    expect(await call(svc.GetOverview())).toEqual(before)
  })

  it('rejects invalid input and an undo ID it never handed out', async () => {
    expect(await code(svc.PreviewGlobal({ name: 'Alex', email: 'nope' }))).toBe(ErrorCode.ErrInvalidInput)
    expect(await code(svc.PreviewRepo({ repoId: repoIdOf('acme_api'), mode: RepoIdentityMode.RepoModeOwn }))).toBe(ErrorCode.ErrInvalidInput)
    expect(await code(svc.Undo('../../x'))).toBe(ErrorCode.ErrInvalidInput)
  })

  it('follow global is offered under a project with an identity and project saves then skip the repository', async () => {
    const req = { repoId: repoIdOf('acme_api'), mode: RepoIdentityMode.RepoModeGlobal }
    const pv = await call(svc.PreviewRepo(req))
    expect(pv.plan.action).toBe(RepoAction.RepoActionRemove)
    const res = await call(svc.ApplyRepo(req, pv.planHash))
    let ov = await call(svc.GetOverview())
    const row = ov.projects![0]!.repos!.find((r) => r.repoId === repoIdOf('acme_api'))!
    expect(row).toMatchObject({ followsGlobal: true })
    expect(row.effective.source.kind).toBe(IdentitySourceKind.IdentitySrcGlobal)
    const proj = await call(svc.PreviewProject({ projectId: 'p_acme', identity: { name: 'Alex', email: 'alex@acme-new.example.com' } }))
    expect(proj.repos!.find((p) => p.repoId === repoIdOf('acme_api'))!.action).toBe(RepoAction.RepoActionKeepOwn)
    await call(svc.Undo(res.undoId))
    ov = await call(svc.GetOverview())
    expect(ov.projects![0]!.repos!.find((r) => r.repoId === repoIdOf('acme_api'))!.followsGlobal).toBe(false)
    expect(await code(svc.Undo(res.undoId))).toBe(ErrorCode.ErrInvalidInput)
  })

  it('reports committer variables and EMAIL apart from the author variables', async () => {
    resetIdentityMock('default')
    const env = await call(svc.GetTerminalEnv(false))
    expect(env).toMatchObject({ active: false, committerVars: false, fallbackEmail: false, probed: true })
  })

  it('previews are drawn with the shared diff helpers', async () => {
    const pv = await call(svc.PreviewGlobal({ name: 'Alex', email: 'alex@acme-new.example.com' }))
    expect(pv.preview.lines!).toContainEqual(add('\temail = alex@acme-new.example.com'))
    expect(pv.preview.lines!).toContainEqual(del('\temail = alex@acme.example.com'))
    expect(pv.preview.lines![0]).toEqual(ctx('[core]'))
  })

  it('global save and undo', async () => {
    resetIdentityMock('unset')
    const a = { name: 'Alex', email: 'alex@acme.example.com' }
    expect((await call(svc.GetOverview())).global.set).toBe(false)
    const pv = await call(svc.PreviewGlobal(a))
    const res = await call(svc.ApplyGlobal(a, pv.preview.baseHash))
    expect((await call(svc.GetOverview())).global.set).toBe(true)
    await call(svc.Undo(res.undoId))
    expect((await call(svc.GetOverview())).global.set).toBe(false)
  })

  it('move into a project writes its identity, move out removes it', async () => {
    const pv = await call(svc.PreviewMove(repoIdOf('scratch'), 'p_globex'))
    expect(pv.plan.action).toBe(RepoAction.RepoActionSet)
    await call(svc.ApplyMove(repoIdOf('scratch'), 'p_globex', pv.planHash))
    let ov = await call(svc.GetOverview())
    expect(ov.projects!.find((p) => p.id === 'p_globex')!.repos!.find((r) => r.name === 'scratch')!.effective.email).toBe('alex@globex.example.com')
    const out = await call(svc.PreviewMove(repoIdOf('scratch'), ''))
    expect(out.plan.action).toBe(RepoAction.RepoActionRemove)
    await call(svc.ApplyMove(repoIdOf('scratch'), '', out.planHash))
    ov = await call(svc.GetOverview())
    expect(ov.ungrouped!.find((r) => r.name === 'scratch')!.effective.source.kind).toBe(IdentitySourceKind.IdentitySrcGlobal)
  })

  it('emits identity:changed after an apply and after an undo', async () => {
    const seen = vi.fn()
    const off = onEvent('identity:changed', seen)
    const prev = await call(svc.PreviewGlobal({ name: 'Sam', email: 'sam@acme.example.com' }))
    const res = await call(svc.ApplyGlobal({ name: 'Sam', email: 'sam@acme.example.com' }, prev.preview.baseHash))
    expect(seen).toHaveBeenCalledTimes(1)
    await call(svc.Undo(res.undoId))
    expect(seen).toHaveBeenCalledTimes(2)
    off()
  })

  it('an unreadable repository gets an error plan in a project save and in a move, and the project record is kept (FU-16e)', async () => {
    resetIdentityMock('readerr')
    const req = { projectId: 'p_acme', identity: { name: 'Alex', email: 'alex@acme-new.example.com' } }
    const pv = await call(svc.PreviewProject(req))
    const bad = pv.repos!.find((p) => p.name === 'acme_cs_portal')!
    expect(bad.action).toBe(RepoAction.RepoActionError)
    expect(bad.error?.params?.file).toContain('acme_cs_portal/.git/config')
    const res = await call(svc.ApplyProject(req, pv.planHash))
    expect(res.recordUpdated).toBe(false)
    expect(res.outcomes!.find((o) => !o.ok)?.name).toBe('acme_cs_portal')
    // a move reads the repository first, so Go rejects it with the coded error instead of returning a plan
    await expect(call(svc.PreviewMove(repoIdOf('acme_cs_portal'), 'p_globex'))).rejects.toBeTruthy()
    await expect(call(svc.ApplyMove(repoIdOf('acme_cs_portal'), 'p_globex', 'x'))).rejects.toBeTruthy()
  })

  it('the repository IDs are the ones of the repos mock, so a move shows its identity confirm (FU-14i)', async () => {
    const { getWorld, resetWorld } = await import('./world')
    resetWorld('done')
    const world = getWorld().repos.find((r) => r.localName === 'acme_api')!
    expect(repoIdOf('acme_api')).toBe(world.id)
  })
})
