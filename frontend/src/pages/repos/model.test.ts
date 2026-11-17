import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { BindingSource, OwnerStatus, RemoteKind, TestStatus } from '../../api/models'
import type { Repo } from '../../api/models'
import { buildScenario } from '../../api/mock/scenarios'
import { detectOwner } from '../../api/mock/repos'
import { accountsState, emptyBinding, mkRepo, reposState } from '../../api/mock/world'
import type { World } from '../../api/mock/world'
import { makeT } from '../../i18n'
import { bindAllPlan, bindableProfiles, bulkSummary, defaultFilter, filterRepos, fitMiddle, profileLabel, profileUsable, rowKind, splitMiddle, suggestedProfile, testText } from './model'

function repos(w: World): Repo[] {
  return reposState(w).repos ?? []
}
function byName(list: Repo[], name: string, dir?: string): Repo {
  const r = list.find((x) => x.name === name && (!dir || x.displayPath.includes(dir)))
  if (!r) throw new Error(`no repo ${name}`)
  return r
}
function detectAll(w: World): void {
  for (const r of w.repos) if (r.toFix) r.owner = detectOwner(w, r)
}

describe('rowKind', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-11-04T04:00:00Z'))
  })
  afterEach(() => vi.useRealTimers())

  it('fresh: legacy binding, URL rule and global defaults before detection', () => {
    const w = buildScenario('fresh')
    const list = repos(w)
    expect(rowKind(byName(list, 'acme_design'))).toBe('legacy')
    expect(rowKind(byName(list, 'initech_infra'))).toBe('url-rule')
    expect(rowKind(byName(list, 'acme_docs'))).toBe('global-default')
  })

  it('fresh: after detection the duplicate clone is ambiguous and the signed-out portal leaves a partial row', () => {
    const w = buildScenario('fresh')
    detectAll(w)
    const list = repos(w)
    expect(rowKind(byName(list, 'acme_cs_portal', 'archive'))).toBe('ambiguous')
    expect(rowKind(byName(list, 'acme_cs_portal', '~/Documents/code/acme_cs_portal'))).toBe('global-default')
    // initech_dashboard lives in the initech account, whose portal is not usable: nothing found, one account skipped
    expect(rowKind(byName(list, 'initech_dashboard'))).toBe('partial')
  })

  it('edge: ssh, grc and repair rows', () => {
    const w = buildScenario('edge')
    const list = repos(w)
    expect(rowKind(byName(list, 'acme_data'))).toBe('ssh')
    expect(rowKind(byName(list, 'acme_design'))).toBe('grc')
    expect(rowKind(byName(list, 'initech_dashboard'))).toBe('repair')
  })

  it('done: every row is bound', () => {
    const w = buildScenario('done')
    expect(repos(w).every((r) => rowKind(r) === 'bound')).toBe(true)
  })

  it('precedence', () => {
    const w = buildScenario('done')
    const r = mkRepo('/Users/alex/Documents/code/x', 'x', emptyBinding(BindingSource.BindingLocal, 'acme'))
    r.binding = { ...r.binding, needsRepair: true, legacyPlain: true }
    r.owner = { ...r.owner, status: OwnerStatus.OwnerAmbiguous }
    expect(rowKind(r)).toBe('repair')
    r.binding.needsRepair = false
    expect(rowKind(r)).toBe('legacy')
    r.binding.legacyPlain = false
    expect(rowKind(r)).toBe('bound')
    r.binding = emptyBinding(BindingSource.BindingGlobalURL, 'acme')
    expect(rowKind(r)).toBe('ambiguous')
    r.owner.status = OwnerStatus.OwnerNone
    expect(rowKind(r)).toBe('url-rule')
    r.binding = emptyBinding(BindingSource.BindingGuard)
    expect(rowKind(r)).toBe('not-found')
    r.owner.status = OwnerStatus.OwnerPartial
    expect(rowKind(r)).toBe('partial')
    r.owner.status = OwnerStatus.OwnerUnknown
    expect(rowKind(r)).toBe('guard')
    r.binding = emptyBinding(BindingSource.BindingNone)
    expect(rowKind(r)).toBe('none')
    r.remoteKind = RemoteKind.RemoteSSH
    expect(rowKind(r)).toBe('ssh')
    r.remoteKind = RemoteKind.RemoteGRC
    expect(rowKind(r)).toBe('grc')
    r.owner.status = OwnerStatus.OwnerChecking
    expect(rowKind(r)).toBe('checking')
    expect(w.repos.length).toBeGreaterThan(0)
  })
})

describe('filters', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-11-04T04:00:00Z'))
  })
  afterEach(() => vi.useRealTimers())

  it('defaultFilter picks "todo" only when something is left', () => {
    expect(defaultFilter({ all: 16, toFix: 5, bound: 11, found: 0, hidden: 0 })).toBe('todo')
    expect(defaultFilter({ all: 16, toFix: 0, bound: 16, found: 0, hidden: 0 })).toBe('all')
  })

  it('filters by state, account and query (name or display path, case-insensitive)', () => {
    const w = buildScenario('mid')
    const list = repos(w)
    expect(filterRepos(list, 'all', undefined, '')).toHaveLength(16)
    expect(filterRepos(list, 'todo', undefined, '')).toHaveLength(list.filter((r) => r.toFix).length)
    expect(filterRepos(list, 'bound', undefined, '').every((r) => !r.toFix)).toBe(true)
    const initech = filterRepos(list, 'all', 'initech-admin', '')
    expect(initech.map((r) => r.name).sort()).toEqual(['initech_dashboard', 'initech_infra', 'initech_site'])
    expect(filterRepos(list, 'all', undefined, 'ACME_CS_PORTAL').map((r) => r.name)).toEqual(['acme_cs_portal', 'acme_cs_portal', 'acme_cs_portal_v2'])
    expect(filterRepos(list, 'all', undefined, 'archive/')).toHaveLength(1)
    expect(filterRepos(list, 'all', undefined, 'no-such-thing')).toHaveLength(0)
  })
})

describe('bulk bind', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-11-04T04:00:00Z'))
  })
  afterEach(() => vi.useRealTimers())

  it('bulkSummary lists profiles in config order', () => {
    const w = buildScenario('mid')
    const profiles = w.accounts.profiles ?? []
    const items = [
      { repoId: 'a', profile: 'globex' },
      { repoId: 'b', profile: 'acme' },
      { repoId: 'c', profile: 'acme' },
    ]
    expect(bulkSummary(items, profiles, makeT('zh-CN'))).toBe('将绑定 3 个仓库：acme 2 个，globex 1 个')
    expect(bulkSummary(items.slice(0, 2), profiles, makeT('en'))).toBe('Binding 2 repositories: acme 1, globex 1')
    expect(bulkSummary(items.slice(0, 1), profiles, makeT('en'))).toBe('Binding 1 repository: globex 1')
  })

  it('bindAllPlan takes single owners with an SSO suggestion and lists ambiguous rows as skipped', () => {
    const w = buildScenario('mid')
    detectAll(w)
    const plan = bindAllPlan(repos(w), w.accounts.profiles ?? [])
    const names = plan.items.map((i) => repos(w).find((r) => r.id === i.repoId)?.name).sort()
    // acme_handbook and acme_changelog belong to acme (SSO) and initech_infra to initech (SSO);
    // globex_backend belongs to globex, which is still an access-key profile, so it is not suggested
    expect(names).toEqual(['acme_changelog', 'acme_handbook', 'initech_infra'])
    expect(plan.items.map((i) => i.profile).sort()).toEqual(['acme', 'acme', 'initech-admin'])
    expect(plan.ambiguous.map((r) => r.displayPath)).toEqual(['~/Documents/code/archive/acme_cs_portal'])
  })

  it('a unique HEAD match resolves an ambiguous owner', () => {
    const w = buildScenario('mid')
    detectAll(w)
    const dup = byName(repos(w), 'acme_cs_portal', 'archive')
    expect(suggestedProfile(dup)).toBeUndefined()
    dup.owner = { ...dup.owner, candidates: (dup.owner.candidates ?? []).map((c) => ({ ...c, headMatch: c.accountId === '444455556666' })) }
    expect(suggestedProfile(dup)).toBe('globex')
  })

  it('suggestedProfile keeps the profile of a URL rule, a legacy binding or a repair', () => {
    const w = buildScenario('fresh')
    const list = repos(w)
    expect(suggestedProfile(byName(list, 'initech_infra'))).toBe('initech-admin')
    expect(suggestedProfile(byName(list, 'acme_design'))).toBe('acme')
    expect(suggestedProfile(byName(list, 'acme_docs'))).toBeUndefined()
  })
})

describe('profiles and test text', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-11-04T04:00:00Z'))
  })
  afterEach(() => vi.useRealTimers())

  it('bindableProfiles puts SSO before access keys; profileUsable follows the portal session', () => {
    const w = buildScenario('mid')
    const profiles = w.accounts.profiles ?? []
    expect(bindableProfiles(profiles).map((p) => p.name)).toEqual(['acme', 'initech-admin', 'initech-readonly', 'globex'])
    const portals = w.accounts.portals ?? []
    expect(profiles.filter((p) => profileUsable(p, portals)).map((p) => p.name)).toContain('initech-admin')
    const fresh = buildScenario('fresh')
    const fp = fresh.accounts.profiles ?? []
    const names = fp.filter((p) => profileUsable(p, fresh.accounts.portals ?? [])).map((p) => p.name)
    expect(names).toEqual(['acme', 'globex'])
  })

  it('testText covers every status', () => {
    const t = makeT('en')
    const base = { repoId: 'x', at: '2026-11-04T04:00:00Z', profile: 'acme', portal: 'initech' }
    expect(testText({ ...base, status: TestStatus.TestOK }, t)).toBe('Connected')
    expect(testText({ ...base, status: TestStatus.TestAuth }, t)).toContain('acme')
    expect(testText({ ...base, status: TestStatus.TestSignedOut }, t)).toBe('initech isn’t signed in')
    expect(testText({ ...base, status: TestStatus.TestNotFound }, t)).toBe('Repository not found')
    expect(testText({ ...base, status: TestStatus.TestTimeout }, t)).toBe('No response in 20 seconds')
    expect(testText({ ...base, status: TestStatus.TestNetwork }, t)).toContain('CodeCommit')
    expect(testText({ ...base, status: TestStatus.TestError }, t)).toBe('The test didn’t pass')
  })
})

describe('splitMiddle', () => {
  it('keeps short values whole', () => {
    expect(splitMiddle('~/Documents/api')).toEqual({ head: '', tail: '~/Documents/api' })
  })
  it('keeps the last path segment as the tail', () => {
    expect(splitMiddle('~/Documents/code/archive/acme_cs_portal')).toEqual({ head: '~/Documents/code/archive', tail: '/acme_cs_portal' })
  })
  it('keeps the host-and-path tail of a remote', () => {
    const { head, tail } = splitMiddle('ssh://git-codecommit.us-west-2.amazonaws.com/v1/repos/initech_infra')
    expect(head + tail).toBe('ssh://git-codecommit.us-west-2.amazonaws.com/v1/repos/initech_infra')
    expect(tail).toBe('/initech_infra')
  })
  it('falls back to a character tail when the last segment is long', () => {
    const v = '/a/b/' + 'x'.repeat(60)
    const r = splitMiddle(v, 10)
    expect(r.tail).toHaveLength(10)
    expect(r.head + r.tail).toBe(v)
  })
})

describe('profileLabel', () => {
  it('always names the account or says it is not identified', () => {
    const p = (accountsState(buildScenario('mid')).profiles ?? [])[0]
    expect(profileLabel({ ...p, name: 'acme', accountId: '111122223333' }, 'not identified')).toBe('acme · 111122223333')
    expect(profileLabel({ ...p, name: 'globex', accountId: undefined }, 'not identified')).toBe('globex · not identified')
  })
})

describe('fitMiddle', () => {
  const chars = (s: string) => s.length
  it('keeps the whole path when it fits', () => {
    expect(fitMiddle('~/Documents/code/acme_agent_portal', 34, chars)).toBe('~/Documents/code/acme_agent_portal')
    expect(fitMiddle('~/Documents/code/acme_agent_portal', 40, chars)).toBe('~/Documents/code/acme_agent_portal')
  })
  it('cuts the middle with the ellipsis against the tail and fills the width', () => {
    const r = fitMiddle('~/Documents/code/acme_agent_portal', 30, chars)
    expect(r).toBe('~/Documents…/acme_agent_portal')
    expect(r).not.toMatch(/… /)
    expect(fitMiddle('~/Documents/code/acme_agent_portal', 33, chars)).toBe('~/Documents/co…/acme_agent_portal')
  })
  it('never loses the tail', () => {
    expect(fitMiddle('~/Documents/code/acme_agent_portal', 5, chars)).toBe('…/acme_agent_portal')
    expect(fitMiddle('short', 3, chars)).toBe('short')
  })
})
