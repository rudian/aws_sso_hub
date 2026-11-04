import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { initialUI } from '../../lib/initialUI'
import { toUiError } from '../errors'
import { onEvent } from '../events'
import type { LoginProgress, SessionStatus } from '../models'
import { ErrorCode, LoginMode } from '../models'
import { mock } from './index'
import { resetWorld } from './world'

describe('mock Accounts.Login', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-11-04T04:00:00Z'))
    initialUI.mockLogin = 'success'
    resetWorld('fresh')
  })
  afterEach(() => {
    vi.useRealTimers()
    initialUI.mockLogin = 'success'
  })

  it('success emits starting -> waiting -> exchanging, then resolves with a known session', async () => {
    const phases: string[] = []
    const sessions: SessionStatus[] = []
    const off1 = onEvent('login:progress', (p: LoginProgress) => phases.push(p.phase))
    const off2 = onEvent('session:changed', (s) => sessions.push(s))
    const p = mock.accounts.Login('initech', LoginMode.LoginModeBrowser)
    await vi.advanceTimersByTimeAsync(2600)
    const res = await p
    off1()
    off2()
    expect(phases).toEqual(['starting', 'waiting', 'exchanging'])
    expect(res.portal).toBe('initech')
    expect(res.profiles).toEqual(['initech-admin', 'initech-readonly'])
    expect(res.expiresAt).toBe('2026-11-04T05:00:02Z')
    const last = sessions[sessions.length - 1]
    expect(sessions[0].state).toBe('signing-in')
    expect(last).toMatchObject({ state: 'active', sessionKnown: true, sessionMinutes: 480, progressPct: 100, loginAt: '2026-11-04T04:00:02Z' })
    expect(last.sessionEndsAt).toBe('2026-11-04T12:00:02Z')
    const state = await Promise.all([mock.accounts.GetState(), vi.advanceTimersByTimeAsync(200)]).then(([s]) => s)
    expect(state.portals?.[0].session.state).toBe('active')
  })

  it('the waiting authorize URL and device code are fixed', async () => {
    const seen: LoginProgress[] = []
    const off = onEvent('login:progress', (p) => seen.push(p))
    const p = mock.accounts.Login('initech', LoginMode.LoginModeBrowser)
    p.catch(() => undefined)
    await vi.advanceTimersByTimeAsync(500)
    const waiting = seen.find((x) => x.phase === 'waiting')
    expect(waiting?.authorizeUrl).toContain('client_id=MOCKCLIENTID')
    expect(waiting?.authorizeUrl).toContain('code_challenge=MOCKCHALLENGE')
    expect(waiting?.browserOpened).toBe(true)
    expect(Date.parse(waiting?.expiresAt ?? '') - Date.now()).toBeGreaterThan(9 * 60_000)
    p.cancel()
    off()
    const dev: LoginProgress[] = []
    const off2 = onEvent('login:progress', (x) => dev.push(x))
    const p2 = mock.accounts.Login('initech', LoginMode.LoginModeDevice)
    p2.catch(() => undefined)
    await vi.advanceTimersByTimeAsync(500)
    off2()
    p2.cancel()
    expect(dev.find((x) => x.phase === 'device')).toMatchObject({ userCode: 'WDJB-MJHT', verificationUri: 'https://device.sso.us-east-1.amazonaws.com/' })
  })

  it('denied rejects with E_LOGIN_DENIED after 1.5 s and clears signing-in', async () => {
    initialUI.mockLogin = 'denied'
    const p = mock.accounts.Login('initech', LoginMode.LoginModeBrowser)
    const result = p.then(
      () => null,
      (e: unknown) => toUiError(e),
    )
    await vi.advanceTimersByTimeAsync(1600)
    expect((await result)?.code).toBe(ErrorCode.ErrLoginDenied)
    const [state] = await Promise.all([mock.accounts.GetState(), vi.advanceTimersByTimeAsync(200)])
    expect(state.portals?.[0].session.state).not.toBe('signing-in')
  })

  it('timeout, network and port map to their codes', async () => {
    for (const [mode, code, ms] of [
      ['timeout', ErrorCode.ErrLoginTimeout, 1600],
      ['network', ErrorCode.ErrNetwork, 1600],
      ['port', ErrorCode.ErrLoginPort, 300],
    ] as const) {
      initialUI.mockLogin = mode
      const p = mock.accounts.Login('initech', LoginMode.LoginModeBrowser)
      const result = p.then(
        () => null,
        (e: unknown) => toUiError(e),
      )
      await vi.advanceTimersByTimeAsync(ms)
      expect((await result)?.code, mode).toBe(code)
    }
  })

  it('waiting never resolves; cancelling rejects with E_CANCELLED', async () => {
    initialUI.mockLogin = 'waiting'
    const p = mock.accounts.Login('initech', LoginMode.LoginModeBrowser)
    let settled = false
    const result = p.then(
      () => {
        settled = true
        return null
      },
      (e: unknown) => {
        settled = true
        return toUiError(e)
      },
    )
    await vi.advanceTimersByTimeAsync(60_000)
    expect(settled).toBe(false)
    p.cancel()
    expect((await result)?.code).toBe(ErrorCode.ErrCancelled)
    const [state] = await Promise.all([mock.accounts.GetState(), vi.advanceTimersByTimeAsync(200)])
    expect(state.portals?.[0].session.state).not.toBe('signing-in')
  })

  it('a second login for the same portal is rejected while one is running', async () => {
    initialUI.mockLogin = 'waiting'
    const first = mock.accounts.Login('initech', LoginMode.LoginModeBrowser)
    const second = mock.accounts.Login('initech', LoginMode.LoginModeBrowser)
    const code = await second.then(
      () => null,
      (e: unknown) => toUiError(e).code,
    )
    expect(code).toBe(ErrorCode.ErrLoginInProgress)
    first.cancel()
  })
})
