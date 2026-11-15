import * as GitGlobalReal from '../../../bindings/github.com/rudian/codepass/internal/app/gitglobalservice'
import { BackupKind, BackupReason, BindingSource, DefaultAction, ErrorCode, RemoteKind } from '../models'
import type { DefaultChangeRequest, DiffLine, FilePreview, GitGlobalState } from '../models'
import { mockEmit } from '../events'
import { deepCopy } from './clock'
import { HOME, accountsState, add, addBackup, ctx, del, emptyBinding, findProfile, fold, getWorld, globalDefaultInfo, guardInfo, guardText, helperEntries, mkPreview, recompute, reposState, urlRules } from './world'
import type { World } from './world'
import { bindCore, outcomeFail, outcomeOk } from './repos'
import { READ, SLOW, WRITE, fail, run } from './util'

function state(w: World): GitGlobalState {
  recompute(w)
  const { other, lines } = helperEntries(w)
  const all = w.repos.length
  const toFix = w.repos.filter((r) => r.toFix).length
  const s: GitGlobalState = {
    globalFile: '~/.gitconfig',
    globalPath: `${HOME}/.gitconfig`,
    baseHash: w.git.baseHash,
    urlRules: urlRules(w),
    guard: guardInfo(w),
    otherHelpers: other,
    credentialLines: lines,
    savedCredentialsSupported: true,
    boundRepos: all - toFix,
    unboundRepos: toFix,
  }
  const g = globalDefaultInfo(w)
  if (g) s.globalDefault = g
  return deepCopy(s)
}

function publish(w: World): void {
  w.git.baseHash = `mock-gitconfig-${++w.hashCounter}`
  mockEmit('repos:changed', reposState(w))
  mockEmit('accounts:changed', accountsState(w))
}

function blockedIds(w: World): string[] {
  return w.repos.filter((r) => r.remoteKind === RemoteKind.RemoteHTTPS && (r.binding.source === BindingSource.BindingGlobalDefault || r.binding.source === BindingSource.BindingNone)).map((r) => r.id)
}

function defaultPreview(w: World, req: DefaultChangeRequest): { preview: FilePreview; blocked: string[] } {
  const helper = (profile: string) => `\thelper = !aws codecommit credential-helper $@ --profile ${profile}`
  if (req.action === DefaultAction.DefaultKeep) return { preview: mkPreview(w, '~/.gitconfig', `${HOME}/.gitconfig`, []), blocked: [] }
  const cur = w.git.globalDefault
  if (req.action === DefaultAction.DefaultRemove) {
    if (w.git.urlRules.length > 0) throw fail(ErrorCode.ErrURLRulesFirst, 'URL rules must be migrated first', { n: String(w.git.urlRules.length) })
    const lines: DiffLine[] = [fold(6), ctx('[credential]')]
    if (cur) lines.push(del(helper(cur.profile)))
    lines.push(ctx('\tusehttppath = true'), fold(4), add('[credential "https://git-codecommit.*.amazonaws.com"]'), add('\thelper ='), add(`\thelper = ${guardText(w)}`), add('\tuseHttpPath = true'))
    return { preview: mkPreview(w, '~/.gitconfig', `${HOME}/.gitconfig`, lines), blocked: blockedIds(w) }
  }
  const target = req.profile ?? ''
  if (!findProfile(w, target)) throw fail(ErrorCode.ErrProfileNotFound, `profile ${target} not found`, { name: target })
  const lines: DiffLine[] = [fold(6), ctx('[credential]')]
  if (cur) lines.push(del(helper(cur.profile)))
  lines.push(add(helper(target)), ctx('\tusehttppath = true'), fold(4))
  return { preview: mkPreview(w, '~/.gitconfig', `${HOME}/.gitconfig`, lines), blocked: [] }
}

export const mockGitGlobalService: typeof GitGlobalReal = {
  GetState: () => run(READ, () => state(getWorld())),

  PreviewDefaultChange: (req) =>
    run(SLOW, () => {
      const w = getWorld()
      const { preview, blocked } = defaultPreview(w, req)
      const local = w.repos.filter((r) => r.binding.source === BindingSource.BindingLocal && r.binding.profileKnown).length
      return { preview, boundUnaffected: local, unboundBlocked: blocked.length, blockedRepoIds: blocked }
    }),

  ApplyDefaultChange: (req, _baseHash) =>
    run(WRITE, () => {
      const w = getWorld()
      defaultPreview(w, req) // same validation as the preview
      const b = addBackup(w, BackupKind.BackupGitGlobal, BackupReason.ReasonGlobalDefault, {})
      if (req.action === DefaultAction.DefaultRemove) {
        for (const r of w.repos) {
          if (r.binding.source === BindingSource.BindingGlobalDefault || (r.binding.source === BindingSource.BindingNone && r.remoteKind === RemoteKind.RemoteHTTPS)) {
            r.binding = emptyBinding(BindingSource.BindingGuard)
          }
        }
        w.git.globalDefault = null
        w.git.guardInstalled = true
      } else if (req.action === DefaultAction.DefaultReplace) {
        w.git.globalDefault = { profile: req.profile ?? '' }
      }
      publish(w)
      return { state: state(w), backupId: b.id }
    }),

  PreviewURLRuleMigration: (pattern) =>
    run(SLOW, () => {
      const w = getWorld()
      const rule = w.git.urlRules.find((r) => r.pattern === pattern)
      if (!rule) throw fail(ErrorCode.ErrInvalidInput, 'unknown URL rule', { field: 'pattern' })
      const repos = w.repos.filter((r) => r.binding.source === BindingSource.BindingGlobalURL && r.binding.urlRule === pattern)
      const urls = repos.map((r) => r.httpsUrl)
      const lines: DiffLine[] = [fold(8), del(`[credential "${pattern}"]`), del(`\thelper = !aws codecommit credential-helper $@ --profile ${rule.profile}`), fold(6)]
      return { pattern, profile: rule.profile, repos: deepCopy(repos), duplicateUrls: new Set(urls).size !== urls.length, preview: mkPreview(w, '~/.gitconfig', `${HOME}/.gitconfig`, lines) }
    }),

  MigrateURLRule: (m, _baseHash) =>
    run(WRITE * 2, () => {
      const w = getWorld()
      const rule = w.git.urlRules.find((r) => r.pattern === m.pattern)
      if (!rule) throw fail(ErrorCode.ErrInvalidInput, 'unknown URL rule', { field: 'pattern' })
      const matched = w.repos.filter((r) => r.binding.source === BindingSource.BindingGlobalURL && r.binding.urlRule === m.pattern)
      const items = m.assignments ?? []
      const ids = new Set(items.map((a) => a.repoId))
      if (items.length !== matched.length || ids.size !== items.length || matched.some((r) => !ids.has(r.id))) throw fail(ErrorCode.ErrInvalidInput, 'assignments must cover every matched repository once', { field: 'assignments' })
      const outcomes = items.map((a) => {
        try {
          const r = w.repos.find((x) => x.id === a.repoId)!
          const b = bindCore(w, r, a.profile)
          return outcomeOk(r, b.id)
        } catch (e) {
          return outcomeFail(a.repoId, e)
        }
      })
      let removed = false
      let backupId = ''
      if (outcomes.every((o) => o.ok)) {
        backupId = addBackup(w, BackupKind.BackupGitGlobal, BackupReason.ReasonURLRule, { pattern: m.pattern }).id
        w.git.urlRules = w.git.urlRules.filter((r) => r.pattern !== m.pattern)
        removed = true
      }
      publish(w)
      return { outcomes, ruleRemoved: removed, backupId }
    }),

  ListSavedCredentials: () => run(READ, () => getWorld().git.saved),

  ClearSavedCredentials: (ids) =>
    run(WRITE, () => {
      const w = getWorld()
      const want = new Set(ids ?? [])
      const targets = want.size === 0 ? w.git.saved : w.git.saved.filter((c) => want.has(c.id))
      w.git.saved = w.git.saved.filter((c) => !targets.includes(c))
      mockEmit('repos:changed', reposState(w))
      return targets.length
    }),
}
