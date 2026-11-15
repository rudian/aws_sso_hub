import { CancellablePromise } from '@wailsio/runtime'
import * as RepoReal from '../../../bindings/github.com/rudian/codepass/internal/app/reposervice'
import { BackupKind, BackupReason, BindWarning, BindingSource, ErrorCode, OwnerStatus, ProfileKind, RemoteKind, SessionState, StaticState, TestStatus } from '../models'
import type { BindOutcome, OwnerResult, Repo, TestResult } from '../models'
import { mockEmit } from '../events'
import { deepCopy, iso, now } from './clock'
import { ACCT, CC_HOST, HOME, REGION_OF_ACCT, SCAN_ROOT_DISPLAY, accountIdOfProfile, accountsState, acctKeyOfId, addBackup, allProfiles, candidateFor, emptyBinding, emptyOwner, findPortal, findProfile, getWorld, httpsUrl, isUsable, mkRepo, recompute, reposState } from './world'
import type { World } from './world'
import { SCAN, SLOW, WRITE, fail, run, toRuntimeError } from './util'

const REGION_RE = /^[a-z]{2}(-gov|-iso[a-z]*)?-[a-z]+-[0-9]{1,2}$/

function publish(w: World, accountsToo = true): void {
  mockEmit('repos:changed', reposState(w))
  if (accountsToo) {
    recompute(w)
    mockEmit('accounts:changed', accountsState(w))
  }
}

function needRepo(w: World, id: string): Repo {
  const r = w.repos.find((x) => x.id === id)
  if (!r) throw fail(ErrorCode.ErrRepoNotFound, `repository ${id} not found`, { path: id })
  return r
}

function needCli(w: World): void {
  if (!w.awsCliReady) throw fail(ErrorCode.ErrAWSCLIMissing, 'AWS CLI v2 not found')
}

function needBindable(r: Repo): void {
  if (r.remoteKind === RemoteKind.RemoteSSH) throw fail(ErrorCode.ErrRemoteSSH, 'remote uses SSH')
  if (r.remoteKind === RemoteKind.RemoteGRC) throw fail(ErrorCode.ErrRemoteGRC, 'remote URL names the account')
  if (r.remoteKind === RemoteKind.RemoteOther) throw fail(ErrorCode.ErrNotCodeCommit, 'not a CodeCommit remote')
}

// ------------------------------------------------------------------ owner detection

interface Group {
  accountId: string
  profiles: string[]
  usable: boolean
  portals: string[]
}

function groups(w: World): Group[] {
  const byAcct = new Map<string, Group>()
  for (const p of allProfiles(w)) {
    const id = accountIdOfProfile(p)
    if (!id) continue
    const g = byAcct.get(id) ?? { accountId: id, profiles: [], usable: false, portals: [] }
    g.profiles.push(p.name)
    if (p.kind === ProfileKind.ProfileKindStatic) {
      if (p.staticState !== StaticState.StaticInvalid) g.usable = true
    } else if (p.kind === ProfileKind.ProfileKindSSO && p.portal) {
      const st = findPortal(w, p.portal)?.session.state
      if (st && isUsable(st)) g.usable = true
      else if (!g.portals.includes(p.portal)) g.portals.push(p.portal)
    }
    byAcct.set(id, g)
  }
  return [...byAcct.values()]
}

export function detectOwner(w: World, r: Repo): OwnerResult {
  const gs = groups(w)
  const checked = gs.filter((g) => g.usable)
  const skipped = gs.filter((g) => !g.usable)
  const truth = w.truth[r.id]?.accounts ?? []
  const hits = checked.filter((g) => truth.includes(g.accountId))
  const portals = [...new Set(skipped.flatMap((g) => g.portals))].sort()
  let status: OwnerStatus
  if (hits.length === 1) status = OwnerStatus.OwnerSingle
  else if (hits.length >= 2) status = OwnerStatus.OwnerAmbiguous
  else status = skipped.length > 0 ? OwnerStatus.OwnerPartial : OwnerStatus.OwnerNone
  return {
    status,
    candidates: hits.map((g) => candidateFor(w, g.accountId, { repo: r })),
    checkedWith: checked.length,
    skipped: skipped.length,
    skippedPortals: portals,
    checkedAt: iso(now()),
  }
}

// ------------------------------------------------------------------ binding

function sq(v: string): string {
  return `'${v.replace(/'/g, `'\\''`)}'`
}

function fallbackText(w: World, profile: string): string {
  const zh = w.appInfo.platform === 'darwin' && w.appInfo.uiLanguage === 'zh-CN'
  return zh
    ? `!f() { test "$1" = get || return 0; echo "AWS SSO Hub: 无法获取 AWS 配置文件 '${profile}' 的 CodeCommit 凭证（未登录、登录已过期或缺少 AWS CLI）。请在 AWS SSO Hub 中点击该配置文件的“SSO 登录”，或运行: aws sso login --profile ${profile}" >&2; echo quit=1; }; f`
    : `!f() { test "$1" = get || return 0; echo "AWS SSO Hub: cannot get CodeCommit credentials for AWS profile '${profile}' (not logged in, session expired, or AWS CLI missing). Click 'SSO sign-in' for this profile in AWS SSO Hub, or run: aws sso login --profile ${profile}" >&2; echo quit=1; }; f`
}

export function bindCommands(w: World, r: Repo, profile: string): string[] {
  const H = `https://${r.host ?? CC_HOST}`
  const aws = w.env.awsCli.path ?? '/opt/homebrew/bin/aws'
  const p = findProfile(w, profile)
  const accountId = p ? accountIdOfProfile(p) : undefined
  const cmds = [
    `git config --local ${sq(`credential.${H}.helper`)} ''`,
    `git config --local --add ${sq(`credential.${H}.helper`)} ${sq(`!"${aws}" --profile ${sq(profile)} codecommit credential-helper $@`)}`,
    `git config --local --add ${sq(`credential.${H}.helper`)} ${sq(fallbackText(w, profile))}`,
    `git config --local ${sq(`credential.${H}.useHttpPath`)} 'true'`,
    `git config --local ${sq(`credential.${H}.interactive`)} 'false'`,
    'git config --local --remove-section awsssohub',
    `git config --local awsssohub.profile ${sq(profile)}`,
  ]
  if (accountId) cmds.push(`git config --local awsssohub.accountId ${sq(accountId)}`)
  cmds.push("git config --local awsssohub.version '1'")
  return cmds
}

export function bindCore(w: World, r: Repo, profile: string, reason: BackupReason = BackupReason.ReasonBind) {
  const p = findProfile(w, profile)
  if (!p) throw fail(ErrorCode.ErrProfileNotFound, `profile ${profile} not found`, { name: profile })
  const b = addBackup(w, BackupKind.BackupGitLocal, reason, { repo: r.name }, { path: `${r.path}/.git/config` })
  r.binding = emptyBinding(BindingSource.BindingLocal, profile)
  recompute(w)
  return b
}

export function outcomeOk(r: Repo, backupId?: string): BindOutcome {
  return { repoId: r.id, ok: true, repo: deepCopy(r), backupId }
}

export function outcomeFail(id: string, e: unknown): BindOutcome {
  const rt = toRuntimeError(e) as { cause?: BindOutcome['error'] }
  return { repoId: id, ok: false, error: rt.cause ?? { code: ErrorCode.ErrInternal, message: String(e) } }
}

function unboundBinding(w: World, r: Repo) {
  if (r.remoteKind !== RemoteKind.RemoteHTTPS) return emptyBinding(BindingSource.BindingNone)
  if (w.git.globalDefault) return emptyBinding(BindingSource.BindingGlobalDefault, w.git.globalDefault.profile)
  if (w.git.guardInstalled) return emptyBinding(BindingSource.BindingGuard)
  return emptyBinding(BindingSource.BindingNone)
}

function testStatusFor(w: World, r: Repo): TestResult {
  const at = iso(now())
  const base = { repoId: r.id, at }
  if (r.binding.source !== BindingSource.BindingLocal || !r.binding.profile) return { ...base, status: TestStatus.TestAuth }
  const p = findProfile(w, r.binding.profile)
  if (!p) return { ...base, status: TestStatus.TestError, profile: r.binding.profile }
  if (p.kind === ProfileKind.ProfileKindStatic) {
    return p.staticState === StaticState.StaticInvalid ? { ...base, status: TestStatus.TestAuth, profile: p.name } : { ...base, status: TestStatus.TestOK, profile: p.name }
  }
  const st = p.portal ? findPortal(w, p.portal)?.session.state : undefined
  if (st && isUsable(st)) return { ...base, status: TestStatus.TestOK, profile: p.name }
  return { ...base, status: TestStatus.TestSignedOut, profile: p.name, portal: p.portal }
}

export const mockRepoService: typeof RepoReal = {
  GetState: () => run(120, () => reposState(getWorld())),

  Scan: () => {
    const w = getWorld()
    const timers: Array<ReturnType<typeof setTimeout>> = []
    return new CancellablePromise<ReturnType<typeof reposState>>(
      (resolve) => {
        w.repoMeta.scanning = true
        mockEmit('repos:changed', reposState(w))
        const total = w.repos.length
        const prog = (found: number, done: boolean) => mockEmit('repos:scan-progress', { root: SCAN_ROOT_DISPLAY, found, done })
        timers.push(setTimeout(() => prog(Math.min(total, 5), false), SCAN * 0.3))
        timers.push(setTimeout(() => prog(Math.min(total, 10), false), SCAN * 0.6))
        timers.push(setTimeout(() => prog(total, false), SCAN * 0.9))
        timers.push(
          setTimeout(() => {
            prog(total, true)
            w.repoMeta.scanning = false
            w.repoMeta.scannedAt = iso(now())
            mockEmit('repos:changed', reposState(w))
            resolve(reposState(w))
          }, SCAN),
        )
      },
      () => {
        timers.forEach(clearTimeout)
        w.repoMeta.scanning = false
        mockEmit('repos:changed', reposState(w))
      },
    )
  },

  DetectOwners: (ids) => {
    const w = getWorld()
    const want = new Set(ids ?? [])
    const targets = w.repos.filter((r) => (want.size > 0 ? want.has(r.id) : r.toFix && (r.remoteKind === RemoteKind.RemoteHTTPS || r.remoteKind === RemoteKind.RemoteSSH)))
    for (const r of targets) r.owner = { ...emptyOwner(), status: OwnerStatus.OwnerChecking }
    w.repoMeta.detecting = true
    mockEmit('repos:changed', reposState(w))
    return run(SLOW, () => {
      for (const r of targets) r.owner = detectOwner(w, r)
      w.repoMeta.detecting = false
      mockEmit('repos:changed', reposState(w))
      return reposState(w)
    })
  },

  CompareCommits: (repoID) =>
    run(SLOW, () => {
      const w = getWorld()
      needCli(w)
      const r = needRepo(w, repoID)
      const cands = r.owner.candidates ?? []
      if (cands.length < 2) return r
      const head = w.truth[r.id]?.headMatch
      r.owner = {
        ...r.owner,
        candidates: cands.map((c) => ({ ...c, headMatch: !!head && c.accountId === head, matchScore: head && c.accountId === head ? 3 : 0 })),
      }
      mockEmit('repos:changed', reposState(w))
      return r
    }),

  PreviewBind: (repoID, profile) =>
    run(120, () => {
      const w = getWorld()
      const r = needRepo(w, repoID)
      const p = findProfile(w, profile)
      if (!p) throw fail(ErrorCode.ErrProfileNotFound, `profile ${profile} not found`, { name: profile })
      const warnings: BindWarning[] = []
      if (r.binding.source === BindingSource.BindingLocal || r.binding.legacyPlain) warnings.push(BindWarning.BindWarnReplaces)
      if (p.kind === ProfileKind.ProfileKindStatic) warnings.push(BindWarning.BindWarnStatic)
      if (p.kind === ProfileKind.ProfileKindSSO && p.portal) {
        const st = findPortal(w, p.portal)?.session.state
        if (!st || !(isUsable(st) || st === SessionState.SessionRefreshable)) warnings.push(BindWarning.BindWarnSignedOut)
      }
      const acct = accountIdOfProfile(p)
      if (r.owner.status === OwnerStatus.OwnerSingle && acct && r.owner.candidates?.[0] && r.owner.candidates[0].accountId !== acct) warnings.push(BindWarning.BindWarnOwnerMismatch)
      return { repoId: r.id, profile, commands: bindCommands(w, r, profile), warnings }
    }),

  Bind: (repoID, profile) =>
    run(WRITE, () => {
      const w = getWorld()
      needCli(w)
      const r = needRepo(w, repoID)
      needBindable(r)
      const b = bindCore(w, r, profile)
      publish(w)
      return outcomeOk(r, b.id)
    }),

  BindMany: (items) =>
    run(WRITE * 2, () => {
      const w = getWorld()
      const out: BindOutcome[] = []
      for (const it of items ?? []) {
        try {
          needCli(w)
          const r = needRepo(w, it.repoId)
          needBindable(r)
          const b = bindCore(w, r, it.profile)
          out.push(outcomeOk(r, b.id))
        } catch (e) {
          out.push(outcomeFail(it.repoId, e))
        }
      }
      publish(w)
      return out
    }),

  Unbind: (repoID) =>
    run(WRITE, () => {
      const w = getWorld()
      const r = needRepo(w, repoID)
      const b = addBackup(w, BackupKind.BackupGitLocal, BackupReason.ReasonUnbind, { repo: r.name }, { path: `${r.path}/.git/config` })
      r.binding = unboundBinding(w, r)
      r.lastTest = undefined
      publish(w)
      return outcomeOk(r, b.id)
    }),

  TestConnection: (repoID) =>
    run(SLOW, () => {
      const w = getWorld()
      needCli(w)
      const r = needRepo(w, repoID)
      const res = testStatusFor(w, r)
      r.lastTest = res
      mockEmit('repos:changed', reposState(w))
      return res
    }),

  TestMany: (repoIDs) =>
    run(SLOW, () => {
      const w = getWorld()
      needCli(w)
      const out: TestResult[] = []
      for (const id of repoIDs ?? []) {
        const r = needRepo(w, id)
        const res = testStatusFor(w, r)
        r.lastTest = res
        out.push(res)
      }
      mockEmit('repos:changed', reposState(w))
      return out
    }),

  ConvertToHTTPS: (repoID, profile) =>
    run(WRITE, () => {
      const w = getWorld()
      needCli(w)
      const r = needRepo(w, repoID)
      if (r.remoteKind === RemoteKind.RemoteSSH) {
        addBackup(w, BackupKind.BackupGitLocal, BackupReason.ReasonHTTPS, { repo: r.name }, { path: `${r.path}/.git/config` })
        r.remoteKind = RemoteKind.RemoteHTTPS
        r.remoteUrl = r.httpsUrl ?? httpsUrl(r.name)
        r.binding = unboundBinding(w, r)
      }
      let backupId: string | undefined
      if (profile) backupId = bindCore(w, r, profile).id
      publish(w)
      return outcomeOk(r, backupId)
    }),

  RepairBindings: () =>
    run(WRITE * 2, () => {
      const w = getWorld()
      needCli(w)
      const out: BindOutcome[] = []
      for (const r of w.repos) {
        if (r.binding.source !== BindingSource.BindingLocal || !(r.binding.needsRepair || r.binding.legacyPlain) || !r.binding.profileKnown) continue
        const b = bindCore(w, r, r.binding.profile as string, BackupReason.ReasonRepair)
        out.push(outcomeOk(r, b.id))
      }
      publish(w)
      return out
    }),

  AddExisting: (path) =>
    run(SLOW, () => {
      const w = getWorld()
      const clean = path.trim().replace(/\/+$/, '')
      const lower = clean.toLowerCase()
      if (!clean.startsWith('/') || lower.includes('notrepo')) throw fail(ErrorCode.ErrRepoNotFound, 'not a git repository', { path: clean })
      if (lower.includes('github')) throw fail(ErrorCode.ErrNotCodeCommit, 'no CodeCommit remote', undefined, 'github.com')
      const listed = w.repos.find((x) => x.path === clean)
      if (listed) return { state: reposState(w), repoId: listed.id, added: false, autoBound: false }

      const name = clean.split('/').pop() || 'repo'
      const r = mkRepo(clean, name, emptyBinding(BindingSource.BindingNone))
      r.extra = true
      r.binding = unboundBinding(w, r)
      const ambiguous = lower.includes('ambiguous')
      w.truth[r.id] = { accounts: ambiguous ? [ACCT.acme.id, ACCT.globex.id] : [ACCT.acme.id] }
      w.repos.push(r)
      recompute(w)
      r.owner = ambiguous
        ? { status: OwnerStatus.OwnerAmbiguous, candidates: [ACCT.acme.id, ACCT.globex.id].map((id) => candidateFor(w, id, { repo: r })), checkedWith: 2, skipped: 0, skippedPortals: [], checkedAt: iso(now()) }
        : detectOwner(w, r)

      // Bind straight away only when one SSO profile is a safe choice and the repository has no binding of its own.
      let outcome: BindOutcome | undefined
      const cand = r.owner.status === OwnerStatus.OwnerSingle ? r.owner.candidates?.[0] : undefined
      const suggested = cand ? findProfile(w, cand.profile) : undefined
      if (w.settings.autoBindOnAdd && w.awsCliReady && cand && suggested?.kind === ProfileKind.ProfileKindSSO && r.remoteKind === RemoteKind.RemoteHTTPS) {
        const b = bindCore(w, r, suggested.name)
        outcome = outcomeOk(r, b.id)
      }
      publish(w, !!outcome)
      return { state: reposState(w), repoId: r.id, added: true, autoBound: !!outcome, outcome }
    }),

  RemoveExtra: (repoID) =>
    run(WRITE, () => {
      const w = getWorld()
      const r = needRepo(w, repoID)
      if (!r.extra) throw fail(ErrorCode.ErrInvalidInput, 'the repository was not added by hand', { field: 'repoId' })
      w.repos = w.repos.filter((x) => x.id !== repoID)
      delete w.truth[repoID]
      publish(w, false)
      return reposState(w)
    }),

  OpenFolder: (repoID) =>
    run(120, () => {
      const r = needRepo(getWorld(), repoID)
      console.info('[mock] reveal folder', r.path.startsWith('~/') ? `${HOME}/${r.path.slice(2)}` : r.path)
    }),

  ListRemoteRepos: (profile, region) =>
    run(SLOW, () => {
      const w = getWorld()
      const p = findProfile(w, profile)
      if (!p) throw fail(ErrorCode.ErrProfileNotFound, `profile ${profile} not found`, { name: profile })
      if (p.kind === ProfileKind.ProfileKindSSO && p.portal) {
        const st = findPortal(w, p.portal)?.session.state
        if (!st || !isUsable(st)) throw fail(ErrorCode.ErrNotSignedIn, `${p.portal} is not signed in`, { portal: p.portal })
      }
      if (p.kind === ProfileKind.ProfileKindStatic && p.staticState === StaticState.StaticInvalid) throw fail(ErrorCode.ErrAWSInvalidKey, 'AWS rejected this key', undefined, p.staticError)
      const acct = accountIdOfProfile(p)
      const list = (acct ? w.remote[acct] : undefined) ?? []
      const own = acct ? acctKeyOfId(acct) : undefined
      if (region && own && region !== REGION_OF_ACCT[own]) return []
      return [...list].sort((a, b) => a.name.localeCompare(b.name))
    }),

  PreviewClone: (req) =>
    run(120, () => {
      const w = getWorld()
      const dest = `${req.parentDir.replace(/\/+$/, '')}/${req.folderName}`
      const tld = req.region.startsWith('cn-') ? 'amazonaws.com.cn' : 'amazonaws.com'
      const url = `https://git-codecommit.${req.region}.${tld}/v1/repos/${req.repoName}`
      return { dest, exists: w.repos.some((r) => r.path === dest), command: `git clone ${url}`, httpsUrl: url }
    }),

  Clone: (req) => {
    const w = getWorld()
    const timers: Array<ReturnType<typeof setTimeout>> = []
    const lines = [
      `Cloning into '${req.folderName}'...`,
      'remote: Counting objects: 100% (1532/1532), done.',
      'Receiving objects:  45% (689/1532)',
      'Receiving objects: 100% (1532/1532), 2.31 MiB | 4.10 MiB/s, done.',
      'Resolving deltas: 100% (812/812), done.',
    ]
    return new CancellablePromise<BindOutcome>(
      (resolve, reject) => {
        let dest = ''
        try {
          needCli(w)
          const p = findProfile(w, req.profile)
          if (!p) throw fail(ErrorCode.ErrProfileNotFound, `profile ${req.profile} not found`, { name: req.profile })
          if (!REGION_RE.test(req.region)) throw fail(ErrorCode.ErrInvalidRegion, 'invalid region')
          if (!/^[\w.-]{1,100}$/.test(req.repoName)) throw fail(ErrorCode.ErrInvalidInput, 'invalid repository name', { field: 'repoName' })
          if (!req.folderName || /[\\/]/.test(req.folderName) || req.folderName === '.' || req.folderName === '..') throw fail(ErrorCode.ErrInvalidInput, 'invalid folder name', { field: 'folderName' })
          if (!req.parentDir) throw fail(ErrorCode.ErrInvalidInput, 'parent folder required', { field: 'parentDir' })
          dest = `${req.parentDir.replace(/\/+$/, '')}/${req.folderName}`
          if (w.repos.some((r) => r.path === dest)) throw fail(ErrorCode.ErrDestExists, 'destination exists', { path: dest })
        } catch (e) {
          reject(toRuntimeError(e))
          return
        }
        lines.forEach((line, i) => timers.push(setTimeout(() => mockEmit('clone:progress', { line }), 250 * (i + 1))))
        timers.push(
          setTimeout(() => {
            const r = mkRepo(dest, req.repoName, emptyBinding(BindingSource.BindingNone), req.region)
            const acct = accountIdOfProfile(findProfile(w, req.profile)!)
            w.truth[r.id] = { accounts: acct ? [acct] : [] }
            w.repos.push(r)
            const b = bindCore(w, r, req.profile)
            r.owner = { status: OwnerStatus.OwnerSingle, candidates: acct ? [candidateFor(w, acct, { repo: r })] : [], checkedWith: 1, skipped: 0, skippedPortals: [], checkedAt: iso(now()) }
            publish(w)
            resolve(outcomeOk(r, b.id))
          }, 250 * (lines.length + 1)),
        )
      },
      () => timers.forEach(clearTimeout),
    )
  },
}
