import { act, createElement } from 'react'
import type { ReactElement } from 'react'
import { createRoot } from 'react-dom/client'
import type { Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { System } from '../../api/services'
import type { Portal, Profile } from '../../api/models'
import { buildScenario } from '../../api/mock/scenarios'
import { accountsState, getWorld, resetWorld } from '../../api/mock/world'
import { I18nProvider } from '../../i18n'
import { ActionsProvider } from '../../shell/AppActions'
import { AccountsProvider } from '../../state/accounts'
import { SettingsProvider } from '../../state/settings'
import { PassCard } from '../accounts/PassCard'
import { SettingsPage } from './SettingsPage'

;(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true

let host: HTMLDivElement
let root: Root

function mount(ui: ReactElement) {
  act(() => root.render(createElement(I18nProvider, { lang: 'en' }, createElement(SettingsProvider, null, createElement(AccountsProvider, null, createElement(ActionsProvider, null, ui))))))
}
async function flush(ms: number) {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(ms)
  })
}

beforeEach(() => {
  vi.useFakeTimers({ toFake: ['Date', 'setTimeout', 'clearTimeout', 'setInterval', 'clearInterval'] })
  vi.setSystemTime(Date.parse('2026-11-04T04:00:00Z'))
  resetWorld('mid')
  host = document.createElement('div')
  document.body.appendChild(host)
  root = createRoot(host)
})
afterEach(() => {
  act(() => root.unmount())
  host.remove()
  document.body.innerHTML = ''
  vi.useRealTimers()
  vi.restoreAllMocks()
})

const SWITCH = 'button[role="switch"][aria-label="Open the sign-in page before the session ends"]'

describe('Settings: open the sign-in page before the session ends', () => {
  it('is on by default with 10 minutes, and offers 10, 15, 30 and 60 minutes', async () => {
    mount(createElement(SettingsPage))
    await flush(200)
    expect(document.querySelector(SWITCH)?.getAttribute('aria-checked')).toBe('true')
    const lead = document.querySelector<HTMLSelectElement>('select[aria-label="How early"]')
    expect(lead?.value).toBe('10')
    expect(Array.from(lead?.options ?? []).map((o) => o.value)).toEqual(['10', '15', '30', '60'])
    expect(document.body.textContent).toContain('The browser first shows a page that names the portal and its profiles')
  })

  it('writes autoSignIn and turns the lead off with it', async () => {
    const spy = vi.spyOn(System, 'UpdateSettings')
    mount(createElement(SettingsPage))
    await flush(200)
    act(() => document.querySelector<HTMLButtonElement>(SWITCH)?.click())
    await flush(1500)
    expect(spy).toHaveBeenCalledWith({ reminders: { autoSignIn: false, leadMinutes: 10, autoRenew: true } })
    expect(getWorld().settings.reminders.autoSignIn).toBe(false)
    expect(document.querySelector<HTMLSelectElement>('select[aria-label="How early"]')?.disabled).toBe(true)
  })

  it('says it needs automatic renewal while that is off', async () => {
    getWorld().settings.reminders.autoRenew = false
    mount(createElement(SettingsPage))
    await flush(200)
    expect(document.body.textContent).toContain('Works only with “Renew sign-in automatically” below turned on.')
  })
})

describe('PassCard: a sign-in page the app opened by itself', () => {
  function initech(waiting: boolean): { portal: Portal; profile: Profile } {
    const s = accountsState(buildScenario('mid'))
    const portal = (s.portals ?? []).find((p) => p.name === 'initech') as Portal
    const profile = (s.profiles ?? []).find((p) => p.portal === 'initech') as Profile
    return { portal: { ...portal, session: { ...portal.session, signInWaiting: waiting } }, profile }
  }

  it('shows a marker next to the countdown while it waits', () => {
    const { portal, profile } = initech(true)
    mount(createElement(PassCard, { profile, portal, readOnly: false, onDialog: () => undefined }))
    expect(host.querySelector('.pass__waiting')?.textContent).toBe('Sign-in page open')
    expect(host.querySelector('.countdown, [class*="countdown"]')).not.toBeNull()
  })

  it('shows nothing otherwise', () => {
    const { portal, profile } = initech(false)
    mount(createElement(PassCard, { profile, portal, readOnly: false, onDialog: () => undefined }))
    expect(host.querySelector('.pass__waiting')).toBeNull()
  })
})
