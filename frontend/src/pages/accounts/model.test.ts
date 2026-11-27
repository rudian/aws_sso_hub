import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ErrorCode, SessionState } from '../../api/models'
import type { Portal, Profile } from '../../api/models'
import { buildScenario } from '../../api/mock/scenarios'
import { accountsState } from '../../api/mock/world'
import { makeT } from '../../i18n'
import { formatDateTime } from '../../lib/time'
import {
  activeLabel,
  cardStateText,
  impactLine,
  isSignedIn,
  loginErrorText,
  loginSuccessText,
  pillState,
  pillText,
  previewMonogram,
  primaryMigrateProfile,
  profileNameProblem,
  sharedLine,
  splitFirstSentence,
  staticProfiles,
  validMonogram,
} from './model'

const NOW = Date.parse('2026-11-04T04:00:00Z')
const en = makeT('en')
const zh = makeT('zh-CN')

function world(id: 'fresh' | 'mid') {
  const s = accountsState(buildScenario(id))
  const profile = (n: string) => (s.profiles ?? []).find((p) => p.name === n) as Profile
  const portal = (n: string) => (s.portals ?? []).find((p) => p.name === n) as Portal
  return { s, profile, portal }
}

describe('accounts model', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date(NOW))
  })
  afterEach(() => vi.useRealTimers())

  it('pillState maps every session state', () => {
    const base = { portal: 'x', sessionKnown: false, sessionEndExact: false, sessionMinutes: 480, sessionOverrun: false, progressPct: 0, canRefresh: false, refreshing: false, signInWaiting: false }
    const states: Array<[SessionState, string]> = [
      [SessionState.SessionActive, 'active'],
      [SessionState.SessionExpiring, 'expiring'],
      [SessionState.SessionRefreshable, 'refreshable'],
      [SessionState.SessionExpired, 'expired'],
      [SessionState.SessionSignedOut, 'signed-out'],
      [SessionState.SessionSigningIn, 'signing-in'],
      [SessionState.SessionError, 'error'],
    ]
    for (const [state, want] of states) expect(pillState({ ...base, state })).toBe(want)
  })

  it('pillState on the mock portals: mid is signed in, fresh is refreshable', () => {
    const mid = world('mid')
    expect(pillState(mid.portal('initech').session)).toBe('active')
    expect(isSignedIn(mid.portal('initech').session)).toBe(true)
    expect(pillState(mid.portal('globex').session)).toBe('signed-out')
    const fresh = world('fresh')
    expect(pillState(fresh.portal('initech').session)).toBe('refreshable')
  })

  it('sharedLine: two profiles name the other one, three or more count', () => {
    const { profile, portal } = world('mid')
    const initech = portal('initech')
    expect(sharedLine(profile('initech-admin'), initech, true, en)).toBe('Shares the initech sign-in with initech-readonly')
    expect(sharedLine(profile('initech-readonly'), initech, false, en)).toBe('One sign-in also unlocks initech-admin')
    expect(sharedLine(profile('initech-admin'), initech, true, zh)).toBe('与 initech-readonly 共用 initech 的登录')
    const many: Portal = { ...initech, profiles: ['a', 'b', 'c'] }
    expect(sharedLine(profile('initech-admin'), many, true, en)).toBe('Shares sign-in with 3 other profiles')
    const alone: Portal = { ...portal('acme') }
    expect(sharedLine(profile('acme'), alone, true, en)).toBeNull()
  })

  it('primaryMigrateProfile: global default first, else most bound repos, else by name', () => {
    const fresh = world('fresh')
    const statics = staticProfiles(fresh.s.profiles ?? [])
    expect(statics.map((p) => p.name).sort()).toEqual(['acme', 'globex'])
    expect(primaryMigrateProfile(statics)?.name).toBe('acme')
    const noDefault = statics.map((p) => ({ ...p, isGlobalDefault: false, boundRepos: p.name === 'globex' ? 5 : 1 }))
    expect(primaryMigrateProfile(noDefault)?.name).toBe('globex')
    const flat = statics.map((p) => ({ ...p, isGlobalDefault: false, boundRepos: 0 }))
    expect(primaryMigrateProfile(flat)?.name).toBe('acme')
    expect(primaryMigrateProfile([])).toBeUndefined()
    const mid = world('mid')
    expect(primaryMigrateProfile(staticProfiles(mid.s.profiles ?? []))?.name).toBe('globex')
  })

  it('impactLine: global default, bound, none', () => {
    const fresh = world('fresh')
    const acme = fresh.profile('acme')
    expect(acme.isGlobalDefault).toBe(true)
    expect(impactLine(acme, en)).toBe(`Git uses this account by default for ${acme.globalDefaultRepos} repositories`)
    expect(impactLine(fresh.profile('globex'), en)).toBe('No repositories are bound to this account')
    expect(impactLine({ ...fresh.profile('globex'), boundRepos: 3 }, en)).toBe('3 repositories are bound here')
    expect(impactLine({ ...fresh.profile('globex'), boundRepos: 1 }, en)).toBe('1 repository is bound here')
    expect(impactLine({ ...fresh.profile('globex'), boundRepos: 3 }, zh)).toBe('3 个仓库绑定到此账号')
  })

  it('texts follow the session: remaining for a known session, wording for the others', () => {
    const mid = world('mid')
    const s = mid.portal('initech').session
    expect(activeLabel(s, NOW, en)).toBe('Signed in, 6 h 12 min left')
    expect(pillText(s, NOW, zh)).toBe('已登录，还剩 6 小时 12 分')
    expect(pillText({ ...s, sessionKnown: false }, NOW, en)).toBe('Signed in · start time unknown')
    expect(pillText({ ...s, sessionOverrun: true }, NOW, en)).toBe('Past the expected end, still signed in')
    expect(pillText({ ...s, sessionOverrun: true }, NOW, zh)).toBe('已过预计结束时间，仍可使用')
    const fresh = world('fresh').portal('initech').session
    expect(pillText(fresh, NOW, en)).toBe('Expired, can renew')
    expect(cardStateText({ ...fresh, state: SessionState.SessionExpired }, NOW, 'en', en)).toMatch(/^Expired /)
    expect(cardStateText({ ...fresh, state: SessionState.SessionSignedOut }, NOW, 'en', en)).toBe('Not signed in yet')
    expect(pillText({ ...s, state: SessionState.SessionExpiring, sessionEndsAt: new Date(NOW + 9 * 60_000).toISOString() }, NOW, en)).toBe('Expires in 9 min')
  })

  it('an expired card never names a time in the future: an estimated session end that has not come yet gives way to the token expiry', () => {
    const at = (min: number) => new Date(NOW + min * 60_000).toISOString()
    const expiredAt = (iso: string) => en('session.expiredAt', { time: formatDateTime(iso, 'en') })
    const fresh = world('fresh').portal('initech').session
    // a known session whose estimated end (3 h from now) was wrong: the sign-in already ended, the token 20 min ago
    const expired = { ...fresh, state: SessionState.SessionExpired, sessionKnown: true, sessionEndExact: false, sessionEndsAt: at(180), expiresAt: at(-20) }
    expect(cardStateText(expired, NOW, 'en', en)).toBe(expiredAt(at(-20)))
    expect(cardStateText(expired, NOW, 'en', en)).not.toContain(formatDateTime(at(180), 'en'))
    // a session end that has passed is when the sign-in expired (an exact end, or an estimate that came true)
    expect(cardStateText({ ...expired, sessionEndsAt: at(-5), sessionEndExact: true }, NOW, 'en', en)).toBe(expiredAt(at(-5)))
    expect(cardStateText({ ...expired, sessionEndsAt: at(-5) }, NOW, 'en', en)).toBe(expiredAt(at(-5)))
    // the start is unknown: the token expiry, as before
    expect(cardStateText({ ...expired, sessionKnown: false }, NOW, 'en', en)).toBe(expiredAt(at(-20)))
    // no usable time: no time at all
    expect(cardStateText({ ...expired, expiresAt: undefined }, NOW, 'en', en)).toBe('Sign-in expired')
    expect(cardStateText({ ...expired, expiresAt: undefined }, NOW, 'zh-CN', zh)).toBe('登录已过期')
  })

  it('name and monogram validation', () => {
    expect(profileNameProblem('acme-admin', new Set(['acme']))).toBeNull()
    expect(profileNameProblem('acme', new Set(['acme']))).toBe('taken')
    expect(profileNameProblem('bad name', new Set())).toBe('invalid')
    expect(profileNameProblem('', new Set())).toBe('invalid')
    expect(previewMonogram('initech-readonly')).toBe('IR')
    expect(previewMonogram('acme')).toBe('AC')
    expect(validMonogram('')).toBe(true)
    expect(validMonogram('ab')).toBe(true)
    expect(validMonogram('a')).toBe(false)
    expect(validMonogram('账')).toBe(true)
    expect(validMonogram('账号')).toBe(false)
  })
})

describe('sign-in modal texts', () => {
  it('uses the singular sentence for one profile and lists several', () => {
    expect(loginSuccessText(['acme'], 'Today 18:55', 'en', en)).toBe('Valid until Today 18:55. acme is ready to use.')
    expect(loginSuccessText(['acme'], '今天 18:55', 'zh-CN', zh)).toBe('有效期到 今天 18:55。acme 现在可以使用了。')
    expect(loginSuccessText(['acme', 'globex'], 'Tomorrow 03:20', 'en', en)).toBe('Valid until Tomorrow 03:20. acme, globex are ready to use.')
    expect(loginSuccessText(['acme', 'globex'], '明天 03:20', 'zh-CN', zh)).toBe('有效期到 明天 03:20。acme、globex 现在都可以使用了。')
    expect(loginSuccessText([], 'Today 18:55', 'en', en)).toBe('Valid until Today 18:55.')
  })
  it('gives a network failure its own title and body, not the generic sentence', () => {
    const e = { code: ErrorCode.ErrNetwork, message: 'dial tcp: timeout', params: {} }
    expect(loginErrorText(e, en)).toMatchObject({ title: 'Can’t reach AWS', body: 'Check your network or proxy, then try again.' })
    expect(loginErrorText(e, zh).body).toBe('检查网络或代理设置后重试。')
  })
  it('splits the waiting text after its first sentence', () => {
    expect(splitFirstSentence('授权页面已在默认浏览器中打开。登录后点击“允许访问”，这里会自动继续。')).toEqual(['授权页面已在默认浏览器中打开。', '登录后点击“允许访问”，这里会自动继续。'])
    expect(splitFirstSentence('We opened the page. Sign in and choose Allow.')).toEqual(['We opened the page.', 'Sign in and choose Allow.'])
    expect(splitFirstSentence('One sentence only.')).toEqual(['One sentence only.', ''])
  })
})
