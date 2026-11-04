import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

async function load(search: string) {
  window.history.replaceState({}, '', `/${search}`)
  vi.resetModules()
  return import('./initialUI')
}

describe('initialUI', () => {
  beforeEach(() => vi.resetModules())
  afterEach(() => window.history.replaceState({}, '', '/'))

  it('defaults', async () => {
    const m = await load('')
    expect(m.initialUI.scenario).toBe('fresh')
    expect(m.initialUI.mockLogin).toBe('success')
    expect(m.initialUI.platform).toBe('darwin')
    expect(m.initialUI.page).toBeUndefined()
  })

  it('parses the documented parameters and ignores bad values', async () => {
    const m = await load('?scenario=mid&page=repos&theme=dark&lang=en&mockLogin=denied&platform=windows&addPortalStep=2&filter=todo&session=overrun&scenario2=x')
    expect(m.initialUI).toMatchObject({ scenario: 'mid', page: 'repos', theme: 'dark', lang: 'en', mockLogin: 'denied', platform: 'windows', addPortalStep: 2, filter: 'todo', session: 'overrun' })
    const bad = await load('?scenario=nope&page=nope&theme=purple&now=garbage')
    expect(bad.initialUI.scenario).toBe('fresh')
    expect(bad.initialUI.page).toBeUndefined()
    expect(bad.initialUI.theme).toBeUndefined()
    expect(bad.initialUI.now).toBeUndefined()
  })

  it('takeOpen returns the rest once', async () => {
    const m = await load('?open=login:initech')
    expect(m.takeOpen('addPortal')).toBeNull()
    expect(m.takeOpen('login:')).toBe('initech')
    expect(m.takeOpen('login:')).toBeNull()
  })

  it('takeOpen returns "" when nothing follows the prefix', async () => {
    const m = await load('?open=clone')
    expect(m.takeOpen('clone')).toBe('')
    expect(m.takeOpen('clone')).toBeNull()
  })

  it('parseInitialUI ignores everything when mocks are off', async () => {
    const m = await load('')
    expect(m.parseInitialUI('?scenario=mid&page=repos', false)).toEqual({ scenario: 'fresh', mockLogin: 'success', platform: 'darwin' })
  })

  it('a build without the mock layer ignores every URL parameter and the ?now= clock override', async () => {
    vi.stubEnv('DEV', false)
    try {
      const m = await load('?scenario=mid&page=repos&theme=dark&now=2020-01-01T00:00:00Z&mock')
      expect(m.initialUI).toEqual({ scenario: 'fresh', mockLogin: 'success', platform: 'darwin' })
      expect((await import('../api/env')).useMocks).toBe(false)
      const clock = await import('./clock')
      expect(Math.abs(clock.now() - Date.now())).toBeLessThan(5_000)
    } finally {
      vi.unstubAllEnvs()
    }
  })

  it('the clock starts at ?now= in mock mode', async () => {
    window.history.replaceState({}, '', '/?now=2020-01-01T00:00:00Z')
    vi.resetModules()
    const clock = await import('./clock')
    expect(Math.abs(clock.now() - Date.parse('2020-01-01T00:00:00Z'))).toBeLessThan(5_000)
  })
})
