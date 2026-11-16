import { act, createElement } from 'react'
import type { ReactElement } from 'react'
import { createRoot } from 'react-dom/client'
import type { Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { Accounts, System } from '../../api/services'
import { mockEmit } from '../../api/events'
import { getWorld, resetWorld } from '../../api/mock/world'
import { LoginMode, LoginPhase } from '../../api/models'
import { I18nProvider } from '../../i18n'
import { ActionsProvider } from '../../shell/AppActions'
import { AccountsProvider } from '../../state/accounts'
import { applyMotion } from '../../lib/theme'
import { SettingsProvider, useSettings } from '../../state/settings'
import { LoginModal } from '../accounts/LoginModal'
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
  resetWorld('fresh')
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

const text = () => document.body.textContent ?? ''
const ISOLATED = 'separate, fresh browser window'
const PRIVATE = 'private browser window'
const FALLBACK = 'a normal window was used'

describe('Settings: private sign-in switch', () => {
  it('shows the card on by default and writes privateSignIn', async () => {
    const spy = vi.spyOn(System, 'UpdateSettings')
    mount(createElement(SettingsPage))
    await flush(200)
    const sw = document.querySelector<HTMLButtonElement>('button[role="switch"][aria-label="Sign in with a private browser window"]')
    expect(sw).not.toBeNull()
    expect(sw?.getAttribute('aria-checked')).toBe('true')
    act(() => sw?.click())
    await flush(1500)
    expect(spy).toHaveBeenCalledWith({ privateSignIn: false })
    expect(getWorld().settings.privateSignIn).toBe(false)
    expect(document.querySelector('button[role="switch"][aria-label="Sign in with a private browser window"]')?.getAttribute('aria-checked')).toBe('false')
  })
})

describe('LoginModal: private window hints', () => {
  const open = (mode: LoginMode = LoginMode.LoginModeBrowser) => mount(createElement(LoginModal, { portal: 'initech', initialMode: mode, onClose: () => undefined }))

  it('isolated window (the default): a fresh browser that closes by itself, no fallback line', async () => {
    open()
    await flush(700)
    expect(text()).toContain('We opened the sign-in page in a separate, fresh browser window')
    expect(text()).toContain('The window closes by itself when you’re done.')
    expect(text()).not.toContain(PRIVATE)
    expect(text()).not.toContain('your default browser')
    expect(text()).not.toContain(FALLBACK)
  })

  it('private but not isolated: private body that says to close all private windows to switch users', async () => {
    open()
    await flush(700)
    act(() => mockEmit('login:progress', { portal: 'initech', phase: LoginPhase.LoginPhaseWaiting, browserOpened: true, privateWindow: true, isolatedWindow: false }))
    expect(text()).toContain('We opened the sign-in page in a private browser window')
    expect(text()).toContain('To sign in as a different user, close all private windows first.')
    expect(text()).not.toContain(ISOLATED)
    expect(text()).not.toContain(FALLBACK)
  })

  it('fell back to a normal window: usual body plus the fallback line', async () => {
    open()
    await flush(700)
    act(() => mockEmit('login:progress', { portal: 'initech', phase: LoginPhase.LoginPhaseWaiting, browserOpened: true, privateWindow: false, isolatedWindow: false }))
    expect(text()).toContain('default browser')
    expect(text()).toContain(FALLBACK)
    expect(text()).not.toContain(PRIVATE)
    expect(text()).not.toContain(ISOLATED)
  })

  it('setting off: nothing about private windows', async () => {
    getWorld().settings.privateSignIn = false
    open()
    await flush(700)
    expect(text()).toContain('default browser')
    expect(text()).not.toContain(PRIVATE)
    expect(text()).not.toContain(ISOLATED)
    expect(text()).not.toContain(FALLBACK)
  })

  it('browser did not open: only the manual-open line, no private hint', async () => {
    open()
    await flush(700)
    act(() => mockEmit('login:progress', { portal: 'initech', phase: LoginPhase.LoginPhaseWaiting, browserOpened: false, privateWindow: false, isolatedWindow: false }))
    expect(text()).not.toContain(FALLBACK)
    expect(text()).not.toContain(PRIVATE)
  })

  it('device code: hint when on, "Open browser" calls OpenSignInPage and shows the fallback if it was not private', async () => {
    const spy = vi.spyOn(Accounts, 'OpenSignInPage').mockResolvedValue({ private: false, isolated: false })
    open(LoginMode.LoginModeDevice)
    await flush(700)
    expect(text()).toContain('uses a private window where it can')
    // the same switch-user warning as the browser flow's private body
    expect(text()).toContain('Close that window after you allow access. To sign in as a different user, close all private windows first.')
    const btn = [...document.querySelectorAll<HTMLButtonElement>('.login-modal .modal__footer button')].find((b) => b.textContent === 'Open browser')
    act(() => btn?.click())
    await flush(50)
    expect(spy).toHaveBeenCalledWith(expect.stringContaining('device.sso'))
    expect(text()).toContain(FALLBACK)
  })

  it('device code: an isolated window replaces the hint with "closes by itself"', async () => {
    open(LoginMode.LoginModeDevice)
    await flush(700)
    const btn = [...document.querySelectorAll<HTMLButtonElement>('.login-modal .modal__footer button')].find((b) => b.textContent === 'Open browser')
    act(() => btn?.click())
    await flush(200)
    expect(text()).toContain('Opened in a separate, fresh browser window. It closes by itself when you’re done.')
    expect(text()).not.toContain('uses a private window where it can')
    expect(text()).not.toContain('To sign in as a different user')
    expect(text()).not.toContain(FALLBACK)
  })

  it('device code, setting off: no hint', async () => {
    getWorld().settings.privateSignIn = false
    open(LoginMode.LoginModeDevice)
    await flush(700)
    expect(text()).not.toContain('private window')
  })
})

describe('LoginModal: Open again', () => {
  it('goes through the backend open, not the plain browser open, and its result decides the text', async () => {
    const spy = vi.spyOn(Accounts, 'OpenSignInPage').mockResolvedValue({ private: false, isolated: false })
    const plain = vi.spyOn(System, 'OpenURL')
    mount(createElement(LoginModal, { portal: 'initech', initialMode: LoginMode.LoginModeBrowser, onClose: () => undefined }))
    await flush(700)
    const url = 'https://oidc.us-east-1.amazonaws.com/authorize?client_id=x'
    act(() => mockEmit('login:progress', { portal: 'initech', phase: LoginPhase.LoginPhaseWaiting, browserOpened: true, privateWindow: true, isolatedWindow: true, authorizeUrl: url }))
    expect(text()).toContain(ISOLATED)
    const btn = Array.from(document.querySelectorAll('button')).find((b) => b.textContent?.includes('Open again'))
    expect(btn).toBeDefined()
    await act(async () => {
      btn?.click()
    })
    expect(spy).toHaveBeenCalledWith(url)
    expect(plain).not.toHaveBeenCalled()
    expect(text()).toContain(FALLBACK)
    expect(text()).not.toContain(ISOLATED)
  })
})

describe('Settings: motion', () => {
  function Apply() {
    const { settings } = useSettings()
    if (settings) applyMotion(settings.motion)
    return null
  }
  afterEach(() => document.documentElement.removeAttribute('data-motion'))

  it('offers System / Full / Reduced; Full is saved and written to data-motion', async () => {
    const spy = vi.spyOn(System, 'UpdateSettings')
    mount(createElement('div', null, createElement(Apply), createElement(SettingsPage)))
    await flush(200)
    const group = document.querySelector('[role="radiogroup"][aria-label="Motion"]')
    expect(group).not.toBeNull()
    const radios = [...(group?.querySelectorAll<HTMLElement>('[role="radio"]') ?? [])]
    expect(radios.map((r) => r.textContent)).toEqual(['System', 'Full', 'Reduced'])
    act(() => radios[1].click())
    await flush(1500)
    expect(spy).toHaveBeenCalledWith({ motion: 'full' })
    expect(getWorld().settings.motion).toBe('full')
    expect(document.documentElement.getAttribute('data-motion')).toBe('full')
    act(() => document.querySelectorAll<HTMLElement>('[role="radiogroup"][aria-label="Motion"] [role="radio"]')[2].click())
    await flush(1500)
    expect(document.documentElement.getAttribute('data-motion')).toBe('reduce')
  })
})
