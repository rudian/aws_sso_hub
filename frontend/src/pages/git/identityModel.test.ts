import { describe, expect, it } from 'vitest'
import { IdentitySourceKind, RepoAction, RepoIdentityMode } from '../../api/models'
import type { IdentitySource, ProjectIdentity, RepoChangePlan, RepoIdentity } from '../../api/models'
import { fieldError, planKind, repoFollow, repoOptions, repoOwnStart, repoRequest, sortPlans, sourceTag, validateAuthor, wasText } from './identityModel'

const src = (kind: IdentitySourceKind, extra: Partial<IdentitySource> = {}): IdentitySource => ({ kind, external: false, ...extra })

describe('sourceTag', () => {
  it('maps every kind of the backend to its tag', () => {
    const K = IdentitySourceKind
    expect(sourceTag(src(K.IdentitySrcNone)).key).toBe('identity.src.none')
    expect(sourceTag(src(K.IdentitySrcGlobal)).key).toBe('identity.src.global')
    expect(sourceTag(src(K.IdentitySrcSystem)).key).toBe('identity.src.system')
    expect(sourceTag(src(K.IdentitySrcProject))).toMatchObject({ key: 'identity.src.project', tone: 'proj' })
    expect(sourceTag(src(K.IdentitySrcFromProject))).toMatchObject({ key: 'identity.src.fromProject', tone: 'proj' })
    expect(sourceTag(src(K.IdentitySrcRepo))).toMatchObject({ key: 'identity.src.repo', tone: 'repo' })
    expect(sourceTag(src(K.IdentitySrcInclude)).key).toBe('identity.src.include')
    expect(sourceTag(src(K.IdentitySrcWorktree)).key).toBe('identity.src.worktree')
    expect(sourceTag(src(K.IdentitySrcCommand)).key).toBe('identity.src.command')
  })
  it('says "set outside the app" only for a repository value the app did not write', () => {
    expect(sourceTag(src(IdentitySourceKind.IdentitySrcRepo, { external: true })).key).toBe('identity.src.repoExternal')
    expect(sourceTag(src(IdentitySourceKind.IdentitySrcRepo, { external: false })).key).toBe('identity.src.repo')
  })
  it('names the file of a conditional rule and keeps the rule for the tooltip', () => {
    const t = sourceTag(src(IdentitySourceKind.IdentitySrcConditional, { ruleFile: '~/.gitconfig', rule: 'gitdir:~/work/' }))
    expect(t).toMatchObject({ key: 'identity.src.conditional', params: { file: '~/.gitconfig' }, title: { params: { rule: 'gitdir:~/work/' } } })
    expect(sourceTag(src(IdentitySourceKind.IdentitySrcConditional)).key).toBe('identity.src.conditionalNoFile')
  })
  it('flags the terminal environment as a warning', () => {
    expect(sourceTag(src(IdentitySourceKind.IdentitySrcEnv, { var: 'GIT_AUTHOR_EMAIL' }))).toMatchObject({ key: 'identity.src.env', tone: 'warn' })
  })
})

describe('validateAuthor', () => {
  it('accepts a normal identity and rejects markup in the name and a bad email', () => {
    expect(validateAuthor({ name: 'Alex', email: 'alex@example.com' })).toEqual({ name: true, email: true })
    expect(validateAuthor({ name: 'A <b>', email: 'alex@example.com' }).name).toBe(false)
    expect(validateAuthor({ name: '  ', email: 'x' })).toEqual({ name: false, email: false })
    expect(validateAuthor({ name: 'Alex', email: 'a b@example.com' }).email).toBe(false)
  })
})

const proj = (identity?: boolean): ProjectIdentity => ({ id: 'p1', name: 'acme', identity: identity ? { name: 'Alex', email: 'alex@acme.example.com' } : undefined, repos: [] })
const row = (extra: Partial<RepoIdentity> = {}): RepoIdentity => ({ repoId: 'r1', name: 'acme_api', path: '/x/acme_api', projectId: 'p1', missing: false, followsGlobal: false, effective: undefined as never, setting: undefined as never, ...extra })

const ext = (kind: IdentitySourceKind, extra: Partial<RepoIdentity> = {}): RepoIdentity =>
  row({ setting: { set: true, name: 'A. Lee', email: 'a.lee@acme.example.com', nameSource: src(kind), emailSource: src(kind), source: src(kind, { external: true }) }, ...extra })

describe('repository choices', () => {
  it('offers the design options: follow the project for a project member, follow global, its own', () => {
    expect(repoOptions(proj(true))).toEqual({ project: true, global: true })
    expect(repoOptions(proj(false))).toEqual({ project: true, global: true })
    expect(repoOptions(undefined)).toEqual({ project: false, global: true })
  })
  it('reads what a repository follows', () => {
    expect(repoFollow(row({ override: { name: 'A', email: 'a@example.com' } }), proj(true))).toBe('own')
    expect(repoFollow(row({ followsGlobal: true }), proj(true))).toBe('global')
    expect(repoFollow(row({ followsGlobal: true }), proj(false))).toBe('global')
    expect(repoFollow(row(), proj(true))).toBe('project')
    expect(repoFollow(row(), proj(false))).toBe('project')
    expect(repoFollow(row({ projectId: undefined }), undefined)).toBe('global')
  })
  it('a value set outside the app counts as the repository\'s own, so following the project is a change', () => {
    const r = ext(IdentitySourceKind.IdentitySrcRepo)
    expect(repoFollow(r, proj(true))).toBe('own')
    expect(repoFollow(ext(IdentitySourceKind.IdentitySrcWorktree), undefined)).toBe('own')
    expect(repoOwnStart(r, { name: 'G', email: 'g@example.com' })).toEqual({ name: 'A. Lee', email: 'a.lee@acme.example.com' })
    expect(repoOwnStart(row({ override: { name: 'O', email: 'o@example.com' } }), { name: 'G', email: 'g@example.com' })).toEqual({ name: 'O', email: 'o@example.com' })
  })
  it('builds the request: the global marker for a project member, plain removal for an ungrouped repository', () => {
    const own = { name: ' Alex ', email: 'alex@example.com' }
    expect(repoRequest(row(), 'own', own, proj(true))).toMatchObject({ mode: RepoIdentityMode.RepoModeOwn, identity: { name: 'Alex' } })
    expect(repoRequest(row(), 'project', own, proj(true)).mode).toBe(RepoIdentityMode.RepoModeProject)
    expect(repoRequest(row(), 'global', own, proj(false)).mode).toBe(RepoIdentityMode.RepoModeGlobal)
    expect(repoRequest(row(), 'global', own, proj(true)).mode).toBe(RepoIdentityMode.RepoModeGlobal)
    expect(repoRequest(row({ projectId: undefined }), 'global', own, undefined).mode).toBe(RepoIdentityMode.RepoModeProject)
  })
})

describe('fieldError', () => {
  it('shows an empty field once it was touched, and a wrong one while it has text', () => {
    expect(fieldError('', false, false)).toBe(false)
    expect(fieldError('', false, true)).toBe(true)
    expect(fieldError('x', false, false)).toBe(true)
    expect(fieldError('a@b', true, true)).toBe(false)
  })
})

describe('sheet preview list', () => {
  const plan = (name: string, action: RepoAction): RepoChangePlan => ({ repoId: name, name, path: `/x/${name}`, action })
  it('classifies the plan actions', () => {
    expect(planKind(plan('a', RepoAction.RepoActionSet))).toBe('change')
    expect(planKind(plan('a', RepoAction.RepoActionRemove))).toBe('change')
    expect(planKind(plan('a', RepoAction.RepoActionKeepOwn))).toBe('keeps-own')
    expect(planKind(plan('a', RepoAction.RepoActionKeepExternal))).toBe('keeps-external')
    expect(planKind(plan('a', RepoAction.RepoActionSkipMissing))).toBe('missing')
    expect(planKind(plan('a', RepoAction.RepoActionError))).toBe('error')
    expect(planKind(plan('a', RepoAction.RepoActionUnchanged))).toBe('unchanged')
  })
  it('lists the repositories that keep their own setting last and failures first', () => {
    const sorted = sortPlans([plan('own', RepoAction.RepoActionKeepOwn), plan('a', RepoAction.RepoActionSet), plan('same', RepoAction.RepoActionUnchanged), plan('bad', RepoAction.RepoActionError), plan('b', RepoAction.RepoActionRemove)])
    expect(sorted.map((p) => p.name)).toEqual(['bad', 'a', 'b', 'same', 'own'])
  })
})

describe('wasText', () => {
  const plan: RepoChangePlan = { repoId: 'r1', name: 'a', path: '/x/a', action: RepoAction.RepoActionSet }
  it('falls back to what the repository used when it had no local value', () => {
    const rows = new Map([['r1', ext(IdentitySourceKind.IdentitySrcGlobal)]])
    expect(wasText(plan, rows)).toBe('a.lee@acme.example.com')
    expect(wasText({ ...plan, before: { name: 'O', email: 'old@example.com' } }, rows)).toBe('old@example.com')
    expect(wasText(plan)).toBe('')
  })
})
