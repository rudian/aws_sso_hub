import { act, createElement } from 'react'
import type { ReactElement } from 'react'
import { createRoot } from 'react-dom/client'
import type { Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { Accounts } from '../../api/services'
import type { Portal, Profile } from '../../api/models'
import { buildScenario } from '../../api/mock/scenarios'
import { accountsState, resetWorld } from '../../api/mock/world'
import { I18nProvider } from '../../i18n'
import { ActionsProvider } from '../../shell/AppActions'
import { AccountsProvider } from '../../state/accounts'
import { PassCard } from './PassCard'

;(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true

let host: HTMLDivElement
let root: Root

function mount(ui: ReactElement) {
  act(() => root.render(createElement(I18nProvider, { lang: 'en' }, createElement(AccountsProvider, null, createElement(ActionsProvider, null, ui)))))
}

beforeEach(() => {
  resetWorld('mid')
  host = document.createElement('div')
  document.body.appendChild(host)
  root = createRoot(host)
})
afterEach(() => {
  act(() => root.unmount())
  host.remove()
  document.body.innerHTML = ''
})

function initech(): { portal: Portal; profile: Profile } {
  const s = accountsState(buildScenario('mid'))
  const portal = (s.portals ?? []).find((p) => p.name === 'initech') as Portal
  const profile = (s.profiles ?? []).find((p) => p.portal === 'initech') as Profile
  return { portal, profile }
}

function menuLabels(): string[] {
  const trigger = host.querySelector<HTMLButtonElement>('.pass__head button[aria-haspopup="menu"]')
  expect(trigger).not.toBeNull()
  act(() => trigger?.click())
  return Array.from(document.querySelectorAll('[role="menuitem"]')).map((el) => el.textContent ?? '')
}

describe('PassCard "Copy export AWS_PROFILE"', () => {
  const noop = () => undefined

  it('is offered for a normal profile name', () => {
    const { portal, profile } = initech()
    mount(<PassCard profile={profile} portal={portal} readOnly={false} onDialog={noop} />)
    expect(menuLabels()).toContain('Copy export AWS_PROFILE')
  })

  for (const name of ['acme$(touch x)', 'dev;rm -rf', "a'b", 'a’b', 'a&calc', '%PATH%']) {
    it(`is hidden for the unsafe name ${JSON.stringify(name)}`, () => {
      const { portal, profile } = initech()
      mount(<PassCard profile={{ ...profile, name }} portal={portal} readOnly={false} onDialog={noop} />)
      const labels = menuLabels()
      expect(labels.length).toBeGreaterThan(0)
      expect(labels).not.toContain('Copy export AWS_PROFILE')
      expect(labels).toContain('Copy profile name')
    })
  }
})

describe('PassCard repos fact', () => {
  const noop = () => undefined
  const dtTexts = () => Array.from(host.querySelectorAll('.pass__facts dt')).map((e) => e.textContent)

  it('shows the bound count with its sr-only labels', () => {
    const { portal, profile } = initech()
    mount(<PassCard profile={{ ...profile, boundRepos: 3 }} portal={portal} readOnly={false} onDialog={noop} />)
    expect(host.querySelector('.pass__repos')?.textContent).toBe('3 repos bound')
    expect(dtTexts()).toContain('Repos')
    expect(dtTexts()).toContain('Account')
    const dts = Array.from(host.querySelectorAll('.pass__facts dt'))
    expect(dts.length).toBeGreaterThan(0)
    for (const dt of dts) expect(dt.classList.contains('sr-only')).toBe(true)
  })

  it('says no repos are bound for zero', () => {
    const { portal, profile } = initech()
    mount(<PassCard profile={{ ...profile, boundRepos: 0 }} portal={portal} readOnly={false} onDialog={noop} />)
    expect(host.querySelector('.pass__repos')?.textContent).toBe('No repos bound')
    expect(host.querySelector('.pass__repos button')).toBeNull()
    expect(dtTexts()).toContain('Repos')
  })
})

describe('PassCard Console button', () => {
  const noop = () => undefined
  const consoleButton = () => [...host.querySelectorAll<HTMLButtonElement>('.pass__actions button')].find((b) => b.textContent === 'Console') as HTMLButtonElement
  const flush = async (ms: number) => {
    await act(async () => {
      await vi.advanceTimersByTimeAsync(ms)
    })
  }

  beforeEach(() => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout', 'setInterval', 'clearInterval'] })
  })
  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  it('shows the loading state while OpenConsole runs and ignores repeat clicks', async () => {
    const spy = vi.spyOn(Accounts, 'OpenConsole')
    const { portal, profile } = initech()
    mount(<PassCard profile={profile} portal={portal} readOnly={false} onDialog={noop} />)
    const btn = consoleButton()
    act(() => {
      btn.click()
      btn.click()
    })
    expect(spy).toHaveBeenCalledOnce()
    expect(spy).toHaveBeenCalledWith(profile.name)
    expect(btn.getAttribute('aria-busy')).toBe('true')
    expect(btn.classList.contains('is-loading')).toBe(true)
    await flush(1000)
    expect(btn.getAttribute('aria-busy')).toBeNull()
    expect(btn.classList.contains('is-loading')).toBe(false)
    act(() => btn.click())
    expect(spy).toHaveBeenCalledTimes(2)
  })

  it('stays enabled while loading, so keyboard focus stays on it; a second click meanwhile is ignored', async () => {
    const spy = vi.spyOn(Accounts, 'OpenConsole')
    const { portal, profile } = initech()
    mount(<PassCard profile={profile} portal={portal} readOnly={false} onDialog={noop} />)
    const btn = consoleButton()
    act(() => btn.focus())
    act(() => btn.click())
    // a browser moves focus off a control that becomes disabled (to <body>); this one must not get the attribute
    expect(btn.getAttribute('aria-busy')).toBe('true')
    expect(btn.hasAttribute('disabled')).toBe(false)
    expect(document.activeElement).toBe(btn)
    // jsdom leaves focus on a disabled control, so also check that the busy button can still take focus
    act(() => btn.blur())
    act(() => btn.focus())
    expect(document.activeElement).toBe(btn)
    // the button is enabled now, so this click (Enter or Space again) reaches the handler, which ignores it
    act(() => btn.click())
    expect(spy).toHaveBeenCalledOnce()
    await flush(1000)
    expect(btn.getAttribute('aria-busy')).toBeNull()
    expect(document.activeElement).toBe(btn)
    act(() => btn.click())
    expect(spy).toHaveBeenCalledTimes(2)
  })

  it('a failure goes to a toast and ends the loading state', async () => {
    const { portal, profile } = initech()
    // the mock only opens the console for an SSO profile it knows
    mount(<PassCard profile={{ ...profile, name: 'not-in-config' }} portal={portal} readOnly={false} onDialog={noop} />)
    const btn = consoleButton()
    act(() => btn.click())
    expect(btn.getAttribute('aria-busy')).toBe('true')
    await flush(1000)
    expect(btn.getAttribute('aria-busy')).toBeNull()
    expect(btn.disabled).toBe(false)
    expect(document.querySelector('.toast--danger')?.textContent).toContain('Check this field')
  })
})
