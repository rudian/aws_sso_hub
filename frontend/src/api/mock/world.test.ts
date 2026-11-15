import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { BindingSource, ProfileKind, RemoteKind } from '../models'
import { SCENARIO_IDS, buildScenario } from './scenarios'
import { accountsState, recompute, reposState, repoToFix } from './world'

describe('mock worlds', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-11-04T04:00:00Z'))
  })
  afterEach(() => vi.useRealTimers())

  for (const id of SCENARIO_IDS) {
    describe(id, () => {
      it('counts equal an independent recomputation', () => {
        const w = buildScenario(id)
        const s = reposState(w)
        const toFix = s.repos?.filter((r) => {
          const b = r.binding
          const bound = (b.source === BindingSource.BindingLocal && !b.legacyPlain && !b.needsRepair && b.profileKnown) || b.source === BindingSource.BindingRemoteURL
          return !bound
        }).length
        expect(s.counts.all).toBe(s.repos?.length)
        expect(s.counts.toFix).toBe(toFix)
        expect(s.counts.bound).toBe(s.counts.all - s.counts.toFix)
        expect(s.repos?.every((r) => r.toFix === repoToFix(r))).toBe(true)
      })

      it('recompute is idempotent', () => {
        const w = buildScenario(id)
        const a = JSON.stringify(accountsState(w))
        const b = JSON.stringify(reposState(w))
        recompute(w)
        recompute(w)
        expect(JSON.stringify(accountsState(w))).toBe(a)
        expect(JSON.stringify(reposState(w))).toBe(b)
      })

      it('every bound profile exists and staticCount matches', () => {
        const w = buildScenario(id)
        const names = new Set((w.accounts.profiles ?? []).map((p) => p.name))
        for (const r of w.repos) {
          if (r.binding.source === BindingSource.BindingLocal || r.binding.source === BindingSource.BindingRemoteURL) {
            expect(names.has(r.binding.profile as string), `${r.displayPath} -> ${r.binding.profile}`).toBe(true)
            expect(r.binding.profileKnown).toBe(true)
          }
        }
        expect(w.accounts.staticCount).toBe((w.accounts.profiles ?? []).filter((p) => p.kind === ProfileKind.ProfileKindStatic).length)
      })

      it("each portal's profiles match the profiles' portal", () => {
        const w = buildScenario(id)
        for (const p of w.accounts.portals ?? []) {
          const expected = (w.accounts.profiles ?? []).filter((x) => x.portal === p.name).map((x) => x.name)
          expect(p.profiles).toEqual(expected)
        }
        for (const pr of w.accounts.profiles ?? []) {
          if (pr.kind === ProfileKind.ProfileKindSSO) expect((w.accounts.portals ?? []).some((p) => p.name === pr.portal)).toBe(true)
        }
      })

      it('slices are never null and ids are unique', () => {
        const s = reposState(buildScenario(id))
        expect(new Set(s.repos?.map((r) => r.id)).size).toBe(s.repos?.length)
        expect(s.rootsDisplay).toEqual(['~/Documents/code'])
      })
    })
  }

  it('fresh has 16 repos and 2 static profiles', () => {
    const w = buildScenario('fresh')
    expect(w.repos).toHaveLength(16)
    expect(w.accounts.staticCount).toBe(2)
    expect(reposState(w).counts).toEqual({ all: 16, toFix: 16, bound: 0, found: 0, hidden: 0 })
    expect(w.appInfo.firstRun).toBe(true)
  })

  it('fresh: initech is refreshable with a token that expired 9 h ago, and not sessionKnown', () => {
    const s = buildScenario('fresh').accounts.portals?.[0].session
    expect(s?.state).toBe('refreshable')
    expect(s?.expiresAt).toBe('2026-11-03T19:00:00Z') // clock is 2026-11-04T04:00:00Z, minus 9 h
    expect(s?.canRefresh).toBe(true)
    expect(s?.sessionKnown).toBe(false)
  })

  it('mid and done: session of initech (6 h 12 min left, 78 %)', () => {
    for (const id of ['mid', 'done'] as const) {
      const w = buildScenario(id)
      const s = w.accounts.portals?.[0].session
      expect(s?.state).toBe('active')
      expect(s?.sessionKnown).toBe(true)
      expect(s?.sessionMinutes).toBe(480)
      expect(s?.progressPct).toBe(78)
      expect(Date.parse(s?.sessionEndsAt ?? '') - Date.parse('2026-11-04T04:00:00Z')).toBe((6 * 60 + 12) * 60_000)
      expect(Date.parse(s?.expiresAt ?? '') - Date.parse('2026-11-04T04:00:00Z')).toBe(42 * 60_000)
      expect(s?.canRefresh).toBe(true)
    }
  })

  it('mid and done: three portals, each with its own organisation and account', () => {
    for (const id of ['mid', 'done'] as const) {
      const w = buildScenario(id)
      const portals = w.accounts.portals ?? []
      expect(portals.map((p) => p.name)).toEqual(['initech', 'acme', 'globex'])
      expect(portals.map((p) => p.startUrl)).toEqual([
        'https://d-1234567890.awsapps.com/start',
        'https://d-1111111111.awsapps.com/start',
        'https://d-2222222222.awsapps.com/start',
      ])
      const profileOf = (n: string) => w.accounts.profiles?.find((p) => p.name === n)
      expect(profileOf('acme')).toMatchObject({ kind: 'sso', portal: 'acme', accountId: '111122223333', role: 'AdministratorAccess' })
      expect(profileOf('initech-admin')).toMatchObject({ portal: 'initech', accountId: '777788889999' })
      expect(portals[0].profiles).toEqual(['initech-admin', 'initech-readonly'])
      // acme: 3 h 05 min left of the 8 h session
      const acme = portals[1].session
      expect(acme.state).toBe('active')
      expect(Date.parse(acme.sessionEndsAt ?? '') - Date.parse('2026-11-04T04:00:00Z')).toBe((3 * 60 + 5) * 60_000)
    }
  })

  it('mid: globex is still a static profile and its new portal is a draft', () => {
    const w = buildScenario('mid')
    const globex = w.accounts.portals?.find((p) => p.name === 'globex')
    expect(globex?.draft).toBe(true)
    expect(w.accounts.profiles?.find((p) => p.name === 'globex')?.kind).toBe(ProfileKind.ProfileKindStatic)
    expect(w.migrations.globex).toMatchObject({ portal: 'globex', portalIsDraft: true, accountId: '444455556666', region: 'eu-west-1' })
  })

  it('done: globex is signed out of its own portal while the other two are active', () => {
    const w = buildScenario('done')
    const state = (n: string) => w.accounts.portals?.find((p) => p.name === n)?.session.state
    expect([state('initech'), state('acme'), state('globex')]).toEqual(['active', 'active', 'expired'])
    expect(w.accounts.profiles?.find((p) => p.name === 'globex')).toMatchObject({ portal: 'globex', accountId: '444455556666' })
  })

  it('mid: 5 repos to fix, globex is mid-wizard, acme is the global default', () => {
    const w = buildScenario('mid')
    expect(reposState(w).counts).toEqual({ all: 16, toFix: 5, bound: 11, found: 0, hidden: 0 })
    const globex = w.accounts.profiles?.find((p) => p.name === 'globex')
    expect(globex?.migrationInProgress).toBe(true)
    expect(w.migrations.globex.step).toBe('review')
    const acme = w.accounts.profiles?.find((p) => p.name === 'acme')
    expect(acme?.boundRepos).toBe(9)
    expect(acme?.isGlobalDefault).toBe(true)
  })

  it('done: everything bound, guard installed, no default', () => {
    const w = buildScenario('done')
    expect(reposState(w).counts).toEqual({ all: 15, toFix: 0, bound: 15, found: 0, hidden: 1 })
    expect(w.git.globalDefault).toBeNull()
    expect(w.git.guardInstalled).toBe(true)
    expect(w.backups).toHaveLength(8)
    expect(w.repos.every((r) => r.lastTest?.status === 'ok')).toBe(true)
  })

  it('edge: CLI missing, read-only config, SSH and GRC remotes', () => {
    const w = buildScenario('edge')
    expect(w.awsCliReady).toBe(false)
    expect(w.env.awsCli.status).toBe('missing')
    expect(w.accounts.readOnly).toBe(true)
    expect(w.accounts.issues?.[0]).toMatchObject({ file: '~/.aws/config', line: 23 })
    expect(w.accounts.portals?.[0].session.state).toBe('error')
    expect(w.repos.filter((r) => r.remoteKind === RemoteKind.RemoteSSH)).toHaveLength(1)
    expect(w.repos.filter((r) => r.remoteKind === RemoteKind.RemoteGRC)).toHaveLength(1)
    expect(w.health.findings).toHaveLength(5)
  })

  it('empty: nothing at all', () => {
    const w = buildScenario('empty')
    expect(w.repos).toHaveLength(0)
    expect(w.accounts.profiles).toHaveLength(0)
    expect(w.accounts.portals).toHaveLength(0)
    expect(w.appInfo.firstRun).toBe(true)
  })

  it('the two acme_cs_portal clones are ambiguous in the mock truth', () => {
    const w = buildScenario('fresh')
    const dup = w.repos.filter((r) => r.name === 'acme_cs_portal')
    expect(dup).toHaveLength(2)
    expect(w.truth[dup.find((r) => r.path.includes('archive'))!.id].accounts).toHaveLength(2)
  })
})
