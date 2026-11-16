// Mock of IdentityService (author identity, plan 3.4 / design 5.12), typed against the generated identityservice module.
// Scenarios: resetIdentityMock('default' | 'unset' | 'env' | 'empty').
import * as IdentityReal from '../../../bindings/github.com/rudian/codepass/internal/app/identityservice'
import { ErrorCode, IdentitySourceKind, RepoAction, RepoIdentityMode } from '../models'
import type { DiffLine, FilePreview, Author, IdentitySource, EffectiveIdentity, EnvVar, TerminalEnvOverride, RepoIdentity, IdentityOverview, RepoChangePlan, GlobalIdentityPreview, ProjectIdentityRequest, ProjectIdentityPreview, MoveIdentityPreview, RepoChangeOutcome, IdentityChangeResult } from '../models'
import { mockEmit } from '../events'
import { add, ctx, del, fold } from './world'
import { READ, SLOW, WRITE, fail, run } from './util'

export type IdentityServiceShape = typeof IdentityReal

// ---- mock state (generic by rule: acme / globex / umbrella, alex@...example.com) ----

const HOME = '/Users/alex'
const CODE = `${HOME}/Documents/code`

interface MRepo {
  id: string
  name: string
  projectId?: string
  local?: Author // user.* in the repository's own config
  override?: Author // what the app recorded as the repository's own
  conditional?: { rule: string; path: string; email: string } // an includeIf file sets the email
  missing?: boolean
  followsGlobal?: boolean // set to follow the global identity on purpose; project saves skip it
}

interface MProject {
  id: string
  name: string
  identity?: Author
}

/** What the Go engine keeps for one Undo (the page only ever holds the ID). */
interface MUndo {
  files: { backupId: string; afterHash: string }[]
  records: { kind: 'project-identity' | 'repo-override' | 'repo-project'; id: string; has: boolean; name: string; email: string; value: string }[]
}

interface MState {
  global?: Author
  globalHash: number
  projects: MProject[]
  repos: MRepo[]
  env: EnvVar[]
  hash: number
  undo: Map<string, string>
  tokens: Map<string, MUndo>
  nextToken: number
}

const A = (name: string, email: string): Author => ({ name, email })
const ALEX = A('Alex', 'alex@acme.example.com')
const ALEX_G = A('Alex', 'alex@globex.example.com')

export type IdentityScenario = 'default' | 'unset' | 'env' | 'empty'

function build(scenario: IdentityScenario): MState {
  const s: MState = {
    global: scenario === 'unset' ? undefined : ALEX,
    globalHash: 1,
    projects: [
      { id: 'p_acme', name: 'acme', identity: scenario === 'default' || scenario === 'env' ? ALEX : undefined },
      { id: 'p_globex', name: 'globex', identity: ALEX_G },
      { id: 'p_umbrella', name: 'umbrella' },
    ],
    repos: [
      { id: 'r_acme_api', name: 'acme_api', projectId: 'p_acme', local: scenario === 'default' || scenario === 'env' ? ALEX : undefined },
      { id: 'r_acme_agent_portal', name: 'acme_agent_portal', projectId: 'p_acme', local: A('Alex', 'alex.agent@acme.example.com'), override: A('Alex', 'alex.agent@acme.example.com') },
      { id: 'r_acme_cs_portal', name: 'acme_cs_portal', projectId: 'p_acme', local: scenario === 'default' || scenario === 'env' ? ALEX : undefined },
      { id: 'r_acme_merchant_portal_v4', name: 'acme_merchant_portal_v4', projectId: 'p_acme', local: A('A. Lee', 'a.lee@acme.example.com') },
      { id: 'r_globex_core', name: 'globex_core', projectId: 'p_globex', local: ALEX_G },
      { id: 'r_umbrella_web', name: 'umbrella_web', projectId: 'p_umbrella' },
      { id: 'r_initech_infra', name: 'initech_infra', conditional: { rule: `gitdir:${CODE}/initech/`, path: `${HOME}/.gitconfig-initech`, email: 'alex@initech.example.com' } },
      { id: 'r_scratch', name: 'scratch' },
    ],
    env: scenario === 'env' ? [{ name: 'GIT_AUTHOR_EMAIL', value: 'alex.shell@example.com' }] : [],
    hash: 1,
    undo: new Map(),
    tokens: new Map(),
    nextToken: 1,
  }
  if (scenario === 'empty') s.repos = []
  return s
}

let st: MState = build('default')

/** Reset the mock (tests and scenario query strings). */
export function resetIdentityMock(scenario: IdentityScenario = 'default'): void {
  st = build(scenario)
}

const same = (a?: Author, b?: Author): boolean => (!a && !b) || (!!a && !!b && a.name === b.name && a.email === b.email)
const clone = <T,>(v: T): T => JSON.parse(JSON.stringify(v)) as T

const NONE: IdentitySource = { kind: IdentitySourceKind.IdentitySrcNone, external: false }
const src = (kind: IdentitySourceKind, extra: Partial<IdentitySource> = {}): IdentitySource => ({ kind, external: false, ...extra })
const GLOBAL_SRC: IdentitySource = src(IdentitySourceKind.IdentitySrcGlobal, { scope: 'global', file: '~/.gitconfig', path: `${HOME}/.gitconfig` })

function project(id?: string): MProject | undefined {
  return id ? st.projects.find((p) => p.id === id) : undefined
}

function settingOf(r: MRepo): EffectiveIdentity {
  const p = project(r.projectId)
  let name = st.global?.name ?? ''
  let email = st.global?.email ?? ''
  let ns: IdentitySource = st.global ? GLOBAL_SRC : NONE
  let es: IdentitySource = st.global ? GLOBAL_SRC : NONE
  if (r.conditional) {
    email = r.conditional.email
    es = src(IdentitySourceKind.IdentitySrcConditional, { scope: 'global', file: r.conditional.path.replace(HOME, '~'), path: r.conditional.path, rule: r.conditional.rule, ruleFile: '~/.gitconfig' })
  }
  if (r.local) {
    let kind: IdentitySourceKind = IdentitySourceKind.IdentitySrcRepo
    let external = true
    if (r.override && same(r.override, r.local)) external = false
    else if (p?.identity && same(p.identity, r.local)) {
      kind = IdentitySourceKind.IdentitySrcFromProject
      external = false
    }
    const s = src(kind, { scope: 'local', file: `${r.name}/.git/config`, path: `${CODE}/${r.name}/.git/config`, external })
    name = r.local.name
    email = r.local.email
    ns = s
    es = s
  }
  const rank = (k: IdentitySourceKind) => (k === IdentitySourceKind.IdentitySrcEnv ? 8 : k === IdentitySourceKind.IdentitySrcRepo || k === IdentitySourceKind.IdentitySrcFromProject ? 5 : k === IdentitySourceKind.IdentitySrcConditional ? 4 : k === IdentitySourceKind.IdentitySrcGlobal ? 2 : 0)
  return { set: !!name && !!email, name, email, nameSource: ns, emailSource: es, source: rank(ns.kind) > rank(es.kind) ? ns : es }
}

function effectiveOf(r: MRepo): EffectiveIdentity {
  const base = settingOf(r)
  const e = st.env.find((v) => v.name === 'GIT_AUTHOR_EMAIL')
  const n = st.env.find((v) => v.name === 'GIT_AUTHOR_NAME')
  if (!e && !n) return base
  const file = { file: '~/.zshrc', path: `${HOME}/.zshrc` }
  const out = clone(base)
  if (n) {
    out.name = n.value
    out.nameSource = src(IdentitySourceKind.IdentitySrcEnv, { var: 'GIT_AUTHOR_NAME', ...file })
  }
  if (e) {
    out.email = e.value
    out.emailSource = src(IdentitySourceKind.IdentitySrcEnv, { var: 'GIT_AUTHOR_EMAIL', ...file })
  }
  out.set = !!out.name && !!out.email
  out.source = out.emailSource.kind === IdentitySourceKind.IdentitySrcEnv ? out.emailSource : out.nameSource
  return out
}

function rowOf(r: MRepo): RepoIdentity {
  const row: RepoIdentity = { repoId: r.id, name: r.name, path: `${CODE}/${r.name}`, missing: !!r.missing, effective: effectiveOf(r), setting: settingOf(r), followsGlobal: !!r.followsGlobal }
  if (r.projectId) row.projectId = r.projectId
  if (r.override) row.override = clone(r.override)
  return row
}

function envReport(): TerminalEnvOverride {
  return {
    active: st.env.some((v) => v.name === 'GIT_AUTHOR_NAME' || v.name === 'GIT_AUTHOR_EMAIL'),
    committerVars: st.env.some((v) => v.name.startsWith('GIT_COMMITTER_')),
    fallbackEmail: st.env.some((v) => v.name === 'EMAIL'),
    vars: clone(st.env),
    files: st.env.length ? [{ file: '~/.zshrc', path: `${HOME}/.zshrc` }] : [],
    probed: true,
    shell: 'zsh',
    configRedirected: false,
  }
}

function overview(): IdentityOverview {
  const live = st.repos
  const usedBy = live.filter((r) => !r.missing && settingOf(r).source.kind === IdentitySourceKind.IdentitySrcGlobal && settingOf(r).set).length
  return {
    global: {
      set: !!st.global,
      name: st.global?.name ?? '',
      email: st.global?.email ?? '',
      file: '~/.gitconfig',
      path: `${HOME}/.gitconfig`,
      baseHash: `mock-global-${st.globalHash}`,
      source: st.global ? GLOBAL_SRC : NONE,
      usedBy,
    },
    projects: st.projects.map((p) => ({ id: p.id, name: p.name, ...(p.identity ? { identity: clone(p.identity) } : {}), repos: live.filter((r) => r.projectId === p.id).map(rowOf) })),
    ungrouped: live.filter((r) => !r.projectId).map(rowOf),
    env: envReport(),
  }
}

// ---- validation and plans (same rules as internal/identity) ----

function validate(a: Author | null | undefined, field: 'identity' | 'name' | 'email' = 'name'): Author {
  if (!a) throw fail(ErrorCode.ErrInvalidInput, 'an identity is required', { field: 'identity' })
  const n = a.name.trim()
  const e = a.email.trim()
  if (!n || n.length > 200 || /[<>]/.test(n) || /[\u0000-\u001f]/.test(n)) throw fail(ErrorCode.ErrInvalidInput, 'invalid author name', { field: 'name' })
  if (!/^[^\s@<>]+@[^\s@<>]+$/.test(e) || e.length > 254) throw fail(ErrorCode.ErrInvalidInput, 'invalid author email', { field: 'email' })
  void field
  return { name: n, email: e }
}

function follow(local: Author | undefined, prev: Author | undefined, next: Author | undefined): { action: RepoAction; after?: Author } {
  if (!local && !next) return { action: RepoAction.RepoActionUnchanged }
  if (!local) return { action: RepoAction.RepoActionSet, after: next }
  if (next && same(local, next)) return { action: RepoAction.RepoActionUnchanged, after: local }
  if (prev && same(local, prev)) return next ? { action: RepoAction.RepoActionSet, after: next } : { action: RepoAction.RepoActionRemove }
  return { action: RepoAction.RepoActionKeepExternal, after: local }
}

const diff = (r: MRepo, before?: Author, after?: Author): FilePreview => {
  const lines: DiffLine[] = [ctx('[core]'), fold(4)]
  if (before) lines.push(del('[user]'), del(`\tname = ${before.name}`), del(`\temail = ${before.email}`))
  if (after) lines.push(add('[user]'), add(`\tname = ${after.name}`), add(`\temail = ${after.email}`))
  return { file: `${CODE.replace(HOME, '~')}/${r.name}/.git/config`, path: `${CODE}/${r.name}/.git/config`, baseHash: `mock-${r.id}-${st.hash}`, lines, added: after ? 3 : 0, removed: before ? 3 : 0, backupDir: '~/Library/Application Support/Codepass/backups' }
}

function planOf(r: MRepo, action: RepoAction, after?: Author): RepoChangePlan {
  const p: RepoChangePlan = { repoId: r.id, name: r.name, path: `${CODE}/${r.name}`, action }
  if (r.local) p.before = clone(r.local)
  if (after) p.after = clone(after)
  if (action === RepoAction.RepoActionSet || action === RepoAction.RepoActionRemove) p.preview = diff(r, r.local, action === RepoAction.RepoActionSet ? after : undefined)
  return p
}

function projectPlan(req: ProjectIdentityRequest): { prev?: Author; next?: Author; plans: RepoChangePlan[]; hash: string } {
  const p = project(req.projectId)
  if (!p) throw fail(ErrorCode.ErrInvalidInput, 'unknown project', { field: 'projectId' })
  const next = req.identity ? validate(req.identity) : undefined
  const plans = st.repos
    .filter((r) => r.projectId === p.id)
    .map((r) => {
      if (r.missing) return planOf(r, RepoAction.RepoActionSkipMissing)
      if (r.override || r.followsGlobal) return planOf(r, RepoAction.RepoActionKeepOwn, r.local)
      const f = follow(r.local, p.identity, next)
      return planOf(r, f.action, f.after)
    })
  const rankOf = (a: RepoAction) => (a === RepoAction.RepoActionSet || a === RepoAction.RepoActionRemove ? 0 : a === RepoAction.RepoActionUnchanged ? 1 : a === RepoAction.RepoActionSkipMissing ? 2 : 3)
  plans.sort((a, b) => rankOf(a.action) - rankOf(b.action))
  return { prev: p.identity, next, plans, hash: `mock-plan-p-${p.id}-${st.hash}-${next ? next.email : '-'}` }
}

/** A result under construction: the lists are never null here (Go sends them non-nil). */
type Res = IdentityChangeResult & { outcomes: RepoChangeOutcome[]; backupIds: string[] }

function result(): Res {
  return { outcomes: [], undoId: '', recordUpdated: false, backupIds: [] }
}

/** Keep what Undo needs and return the ID ("" when nothing was written). */
function remember(res: IdentityChangeResult, undo: MUndo): void {
  if (!undo.files.length && !undo.records.length) return
  res.undoId = `mock-undo-${st.nextToken++}`
  st.tokens.set(res.undoId, undo)
}

function stale(file: string): never {
  throw fail(ErrorCode.ErrConfigChanged, 'the file changed since the preview', { file })
}

/** Remember what a config file held before a write, so the mock's Undo can put it back. */
function file(undo: MUndo, r: MRepo, afterHash: string, before: Author | undefined = r.local): string {
  const backupId = `mock-backup-${r.id}-${st.hash}`
  st.undo.set(backupId, JSON.stringify({ repoId: r.id, before: before ?? null }))
  undo.files.push({ backupId, afterHash })
  return backupId
}

// ---- the service ----

export const mockIdentityService: IdentityServiceShape = {
  GetOverview: () => run(READ, overview),
  GetTerminalEnv: (_refresh) => run(READ, envReport),

  PreviewGlobal: (a) =>
    run(SLOW, () => {
      const after = validate(a)
      const lines: DiffLine[] = [ctx('[core]'), fold(3)]
      if (st.global) lines.push(del(`\tname = ${st.global.name}`), del(`\temail = ${st.global.email}`))
      lines.push(add(`\tname = ${after.name}`), add(`\temail = ${after.email}`))
      const unchanged = same(st.global, after)
      const out: GlobalIdentityPreview = {
        preview: { file: '~/.gitconfig', path: `${HOME}/.gitconfig`, baseHash: `mock-global-${st.globalHash}`, lines: unchanged ? [] : lines, added: unchanged ? 0 : 2, removed: unchanged || !st.global ? 0 : 2, backupDir: '~/Library/Application Support/Codepass/backups' },
        after,
        unchanged,
      }
      if (st.global) out.before = clone(st.global)
      return out
    }),

  ApplyGlobal: (a, baseHash) =>
    run(WRITE, () => {
      const after = validate(a)
      if (baseHash !== `mock-global-${st.globalHash}`) stale('~/.gitconfig')
      const bid = `mock-backup-global-${st.globalHash}`
      st.undo.set(bid, JSON.stringify({ repoId: '', before: st.global ?? null }))
      st.global = after
      st.globalHash++
      const r = result()
      r.recordUpdated = true
      r.backupIds = [bid]
      remember(r, { files: [{ backupId: bid, afterHash: `mock-global-${st.globalHash}` }], records: [] })
      mockEmit('identity:changed', {}) // the Go service emits this after every apply and undo
      return r
    }),

  PreviewProject: (req) =>
    run(SLOW, () => {
      const p = project(req.projectId)
      const { prev, next, plans, hash } = projectPlan(req)
      const out: ProjectIdentityPreview = {
        projectId: req.projectId,
        name: p!.name,
        repos: plans,
        writes: plans.filter((x) => x.action === RepoAction.RepoActionSet || x.action === RepoAction.RepoActionRemove).length,
        keepsOwn: plans.filter((x) => x.action === RepoAction.RepoActionKeepOwn || x.action === RepoAction.RepoActionKeepExternal).length,
        planHash: hash,
      }
      if (next) out.identity = next
      if (prev) out.previous = clone(prev)
      return out
    }),

  ApplyProject: (req, planHash) =>
    run(WRITE, () => {
      const { next, plans, hash } = projectPlan(req)
      if (hash !== planHash) stale(project(req.projectId)!.name)
      const res = result()
      const undo: MUndo = { files: [], records: [] }
      const p = project(req.projectId)!
      for (const pl of plans) {
        const r = st.repos.find((x) => x.id === pl.repoId)!
        if (pl.action === RepoAction.RepoActionSet || pl.action === RepoAction.RepoActionRemove) res.backupIds.push(file(undo, r, `mock-after-${r.id}-${st.hash}`))
        if (pl.action === RepoAction.RepoActionSet) r.local = clone(pl.after!)
        if (pl.action === RepoAction.RepoActionRemove) r.local = undefined
        res.outcomes.push({ repoId: r.id, name: r.name, action: pl.action, ok: true })
      }
      undo.records.push({ kind: 'project-identity', id: p.id, has: !!p.identity, name: p.identity?.name ?? '', email: p.identity?.email ?? '', value: '' })
      p.identity = next
      res.recordUpdated = true
      remember(res, undo)
      st.hash++
      mockEmit('identity:changed', {}) // the Go service emits this after every apply and undo
      return res
    }),

  PreviewRepo: (req) =>
    run(READ, () => {
      const r = st.repos.find((x) => x.id === req.repoId)
      if (!r) throw fail(ErrorCode.ErrInvalidInput, 'unknown repository', { field: 'repoId' })
      const p = project(r.projectId)
      let plan: RepoChangePlan
      if (req.mode === RepoIdentityMode.RepoModeOwn) {
        const a = validate(req.identity)
        plan = planOf(r, same(r.local, a) ? RepoAction.RepoActionUnchanged : RepoAction.RepoActionSet, a)
      } else if (req.mode === RepoIdentityMode.RepoModeProject) {
        const want = p?.identity
        plan = want ? planOf(r, same(r.local, want) ? RepoAction.RepoActionUnchanged : RepoAction.RepoActionSet, want) : planOf(r, r.local ? RepoAction.RepoActionRemove : RepoAction.RepoActionUnchanged)
      } else if (req.mode === RepoIdentityMode.RepoModeGlobal) {
        // follow global whatever the project says: the local values go and project saves skip the repository
        plan = planOf(r, r.local ? RepoAction.RepoActionRemove : RepoAction.RepoActionUnchanged)
      } else throw fail(ErrorCode.ErrInvalidInput, 'unknown mode', { field: 'mode' })
      return { repoId: r.id, mode: req.mode, plan, planHash: `mock-plan-r-${r.id}-${st.hash}-${req.mode}-${req.identity?.email ?? '-'}` }
    }),

  ApplyRepo: (req, planHash) =>
    run(WRITE, () => {
      const r = st.repos.find((x) => x.id === req.repoId)
      if (!r) throw fail(ErrorCode.ErrInvalidInput, 'unknown repository', { field: 'repoId' })
      const p = project(r.projectId)
      if (planHash !== `mock-plan-r-${r.id}-${st.hash}-${req.mode}-${req.identity?.email ?? '-'}`) stale(r.name)
      const res = result()
      const undo: MUndo = { files: [], records: [] }
      // the previous marker is an override with empty values, as in projects.json
      const prev = r.followsGlobal ? { has: true, name: '', email: '' } : { has: !!r.override, name: r.override?.name ?? '', email: r.override?.email ?? '' }
      undo.records.push({ kind: 'repo-override', id: r.id, value: '', ...prev })
      res.backupIds.push(file(undo, r, `mock-after-${r.id}-${st.hash}`))
      r.followsGlobal = false
      if (req.mode === RepoIdentityMode.RepoModeOwn) {
        const a = validate(req.identity)
        r.local = a
        r.override = a
      } else if (req.mode === RepoIdentityMode.RepoModeGlobal) {
        r.override = undefined
        r.local = undefined
        r.followsGlobal = true
      } else {
        r.override = undefined
        r.local = p?.identity ? clone(p.identity) : undefined
      }
      res.outcomes.push({ repoId: r.id, name: r.name, action: RepoAction.RepoActionSet, ok: true })
      res.recordUpdated = true
      remember(res, undo)
      st.hash++
      mockEmit('identity:changed', {}) // the Go service emits this after every apply and undo
      return res
    }),

  PreviewMove: (repoId, toProjectId) =>
    run(READ, () => {
      const r = st.repos.find((x) => x.id === repoId)
      if (!r) throw fail(ErrorCode.ErrInvalidInput, 'unknown repository', { field: 'repoId' })
      if (toProjectId && !project(toProjectId)) throw fail(ErrorCode.ErrInvalidInput, 'unknown project', { field: 'projectId' })
      const from = project(r.projectId)
      const to = project(toProjectId)
      let plan: RepoChangePlan
      if (r.override || r.followsGlobal) plan = planOf(r, RepoAction.RepoActionKeepOwn, r.local)
      else if ((r.projectId ?? '') === toProjectId) plan = planOf(r, RepoAction.RepoActionUnchanged, r.local)
      else {
        const f = follow(r.local, from?.identity, to?.identity)
        plan = planOf(r, f.action, f.after)
      }
      const out: MoveIdentityPreview = { repoId, plan, planHash: `mock-plan-m-${r.id}-${st.hash}-${toProjectId}` }
      if (r.projectId) out.fromProjectId = r.projectId
      if (toProjectId) out.toProjectId = toProjectId
      return out
    }),

  ApplyMove: (repoId, toProjectId, planHash) =>
    run(WRITE, () => {
      const r = st.repos.find((x) => x.id === repoId)
      if (!r) throw fail(ErrorCode.ErrInvalidInput, 'unknown repository', { field: 'repoId' })
      if (planHash !== `mock-plan-m-${r.id}-${st.hash}-${toProjectId}`) stale(r.name)
      const from = project(r.projectId)
      const to = project(toProjectId)
      const res = result()
      const undo: MUndo = { files: [], records: [] }
      let action: RepoAction = RepoAction.RepoActionKeepOwn
      if (!r.override && !r.followsGlobal) {
        const f = follow(r.local, from?.identity, to?.identity)
        action = f.action
        if (f.action === RepoAction.RepoActionSet || f.action === RepoAction.RepoActionRemove) res.backupIds.push(file(undo, r, `mock-after-${r.id}-${st.hash}`))
        if (f.action === RepoAction.RepoActionSet) r.local = clone(f.after!)
        if (f.action === RepoAction.RepoActionRemove) r.local = undefined
      }
      undo.records.push({ kind: 'repo-project', id: r.id, has: false, name: '', email: '', value: r.projectId ?? '' })
      r.projectId = toProjectId || undefined
      res.outcomes.push({ repoId: r.id, name: r.name, action, ok: true })
      res.recordUpdated = true
      remember(res, undo)
      st.hash++
      mockEmit('identity:changed', {}) // the Go service emits this after every apply and undo
      return res
    }),

  Undo: (undoId) =>
    run(WRITE, () => {
      const tok = st.tokens.get(undoId)
      if (!tok) throw fail(ErrorCode.ErrInvalidInput, 'nothing to undo', { field: 'undoId' })
      for (const f of tok.files) {
        if (!st.undo.get(f.backupId)) throw fail(ErrorCode.ErrBackupNotFound, 'backup not found')
        const { repoId } = JSON.parse(st.undo.get(f.backupId)!) as { repoId: string }
        if (repoId === '' && f.afterHash !== `mock-global-${st.globalHash}`) stale('~/.gitconfig')
      }
      for (const f of tok.files) {
        const { repoId, before } = JSON.parse(st.undo.get(f.backupId)!) as { repoId: string; before: Author | null }
        if (repoId === '') {
          st.global = before ?? undefined
          st.globalHash++
        } else {
          const r = st.repos.find((x) => x.id === repoId)
          if (r) r.local = before ?? undefined
        }
      }
      for (const rec of tok.records) {
        if (rec.kind === 'project-identity') {
          const p = project(rec.id)
          if (p) p.identity = rec.has ? A(rec.name, rec.email) : undefined
        } else if (rec.kind === 'repo-override') {
          const r = st.repos.find((x) => x.id === rec.id)
          if (r) {
            r.followsGlobal = rec.has && !rec.name && !rec.email
            r.override = rec.has && !r.followsGlobal ? A(rec.name, rec.email) : undefined
          }
        } else {
          const r = st.repos.find((x) => x.id === rec.id)
          if (r) r.projectId = rec.value || undefined
        }
      }
      st.tokens.delete(undoId)
      st.hash++
      const res = result()
      res.recordUpdated = tok.records.length > 0
      mockEmit('identity:changed', {}) // the Go service emits this after every apply and undo
      return res
    }),
}
