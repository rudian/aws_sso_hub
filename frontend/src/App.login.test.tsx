// Login requests in the app shell: a closed tools gate ignores them (and keeps ?open=login for after the setup), and a
// request for another portal while a login modal is open waits behind it (FU-Wa, FU-22b, FU-Wc).
import { act, createElement } from 'react'
import { createRoot } from 'react-dom/client'
import type { Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { App } from './App'
import { mockEmit } from './api/events'
import { resetMockNotify } from './api/mock/notify'
import { resetToolMock } from './api/mock/system'
import { getWorld, resetWorld } from './api/mock/world'
import { Language } from './api/models'
import { Tools } from './api/services'
import { initialUI, setOpen } from './lib/initialUI'
import { GATE_RECHECK_EVENT } from './lib/gate'
import { setRoute } from './lib/route'

;(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true

let host: HTMLDivElement
let root: Root

async function until(cond: () => boolean) {
  for (let i = 0; i < 80 && !cond(); i++) {
    await act(async () => {
      await new Promise((r) => setTimeout(r, 40))
    })
  }
}

beforeEach(() => {
  resetWorld('mid')
  getWorld().settings.language = Language.LanguageEn
  resetMockNotify('')
  resetToolMock('ready')
  initialUI.mockLogin = 'waiting'
  setRoute({ page: 'accounts' })
  Element.prototype.scrollIntoView = vi.fn()
  host = document.createElement('div')
  document.body.appendChild(host)
  root = createRoot(host)
})
afterEach(() => {
  act(() => root.unmount())
  host.remove()
  document.body.innerHTML = ''
  initialUI.mockLogin = 'success'
  resetToolMock('ready')
  setOpen('')
  vi.restoreAllMocks()
})

const modals = () => [...document.querySelectorAll('.modal')]
const loginTitles = () => modals().map((m) => m.querySelector('.modal__title')?.textContent ?? '')
const toasts = () => [...document.querySelectorAll('.toast')].map((t) => t.textContent ?? '')
const portals = () => (getWorld().accounts.portals ?? []).filter((p) => !p.draft && !p.legacyFormat).map((p) => p.name)

describe('login requests from outside the window', () => {
  it('two login navigations: the second portal opens when the first modal closes, and a toast says so', async () => {
    act(() => root.render(createElement(App)))
    await until(() => !!document.querySelector('.app__sidebar'))
    const [a, b] = portals()
    act(() => mockEmit('app:navigate', { page: 'accounts', action: 'login', arg: a }))
    await until(() => loginTitles().some((t) => t.includes(a)))
    expect(loginTitles().some((t) => t.includes(a))).toBe(true)

    act(() => mockEmit('app:navigate', { page: 'accounts', action: 'login', arg: b }))
    await until(() => toasts().some((t) => t.includes(b)))
    expect(toasts().some((t) => t.includes(`${b} opens when it ends`))).toBe(true)
    expect(loginTitles().some((t) => t.includes(b))).toBe(false)

    // Close the first modal (Cancel): the waiting one opens
    const cancel = [...document.querySelectorAll('.modal button')].find((x) => /cancel/i.test(x.textContent ?? '')) as HTMLElement
    await act(async () => cancel.click())
    await until(() => loginTitles().some((t) => t.includes(b)))
    expect(loginTitles().some((t) => t.includes(b))).toBe(true)
  })

  it('three login navigations: the second and the third open in turn, and a repeated request is not queued twice', async () => {
    resetWorld('done')
    getWorld().settings.language = Language.LanguageEn
    act(() => root.render(createElement(App)))
    await until(() => !!document.querySelector('.app__sidebar'))
    const [a, b, c] = portals()
    expect(c).toBeTruthy()
    const cancel = async () => {
      const btn = [...document.querySelectorAll('.modal button')].find((x) => /cancel/i.test(x.textContent ?? '')) as HTMLElement
      await act(async () => btn.click())
    }
    act(() => mockEmit('app:navigate', { page: 'accounts', action: 'login', arg: a }))
    await until(() => loginTitles().some((t) => t.includes(a)))
    act(() => mockEmit('app:navigate', { page: 'accounts', action: 'login', arg: b }))
    act(() => mockEmit('app:navigate', { page: 'accounts', action: 'login', arg: c }))
    act(() => mockEmit('app:navigate', { page: 'accounts', action: 'login', arg: b }))
    await until(() => toasts().some((t) => t.includes(c)))
    await cancel()
    await until(() => loginTitles().some((t) => t.includes(b)))
    expect(loginTitles().some((t) => t.includes(b))).toBe(true)
    await cancel()
    await until(() => loginTitles().some((t) => t.includes(c)))
    expect(loginTitles().some((t) => t.includes(c))).toBe(true)
    await cancel()
    await act(async () => {
      await new Promise((r) => setTimeout(r, 150))
    })
    expect(modals().length).toBe(0)
  })

  it('a second request for the portal that is already signing in only says so', async () => {
    act(() => root.render(createElement(App)))
    await until(() => !!document.querySelector('.app__sidebar'))
    const [a] = portals()
    act(() => mockEmit('app:navigate', { page: 'accounts', action: 'login', arg: a }))
    await until(() => loginTitles().some((t) => t.includes(a)))
    act(() => mockEmit('app:navigate', { page: 'accounts', action: 'login', arg: a }))
    await until(() => toasts().length > 0)
    expect(toasts().some((t) => t.includes(`The sign-in for ${a} is already running`))).toBe(true)
    expect(modals().length).toBe(1)
  })

  it('while the tools gate is blocked a login request and Cmd+L do nothing', async () => {
    resetToolMock('missing')
    act(() => root.render(createElement(App)))
    await until(() => !!document.querySelector('.setup'))
    const [a] = portals()
    act(() => mockEmit('app:navigate', { page: 'accounts', action: 'login', arg: a }))
    await act(async () => {
      window.dispatchEvent(new KeyboardEvent('keydown', { key: 'l', metaKey: true, ctrlKey: true, bubbles: true }))
    })
    await act(async () => {
      await new Promise((r) => setTimeout(r, 200))
    })
    expect(loginTitles().some((t) => t.includes(a))).toBe(false)
    expect(document.querySelector('.setup')).not.toBeNull()
  })

  it('?open=login waits for the gate and opens once the setup is done', async () => {
    resetToolMock('missing')
    const [a] = portals()
    setOpen(`login:${a}`)
    act(() => root.render(createElement(App)))
    await until(() => !!document.querySelector('.setup'))
    await act(async () => {
      await new Promise((r) => setTimeout(r, 300))
    })
    expect(loginTitles().some((t) => t.includes(a))).toBe(false)
    // the setup is done: the gate re-checks and opens
    resetToolMock('ready')
    await act(async () => {
      window.dispatchEvent(new Event(GATE_RECHECK_EVENT))
    })
    await until(() => loginTitles().some((t) => t.includes(a)))
    expect(loginTitles().some((t) => t.includes(a))).toBe(true)
  })

  it('while the gate is being checked the window shows the title bar and the brand, not an empty grid', async () => {
    let release: () => void = () => undefined
    const gate = new Promise<void>((r) => (release = r))
    const real = Tools.GetSetupState.bind(Tools)
    vi.spyOn(Tools, 'GetSetupState').mockImplementation((() => gate.then(() => real())) as never)
    act(() => root.render(createElement(App)))
    await act(async () => {
      await new Promise((r) => setTimeout(r, 50))
    })
    expect(document.querySelector('.app .app__titlebar')).not.toBeNull()
    expect(document.querySelector('.app .brand__name')?.textContent).toBe('Codepass')
    await act(async () => release())
    await until(() => !!document.querySelector('.app__sidebar'))
  })
})
