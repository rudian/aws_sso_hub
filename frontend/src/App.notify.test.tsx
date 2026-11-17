import { act, createElement } from 'react'
import { createRoot } from 'react-dom/client'
import type { Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { App } from './App'
import { mockEmit } from './api/events'
import { resetMockNotify } from './api/mock/notify'
import { getWorld, resetWorld } from './api/mock/world'
import { Language, NotifyTone } from './api/models'
import { Notify } from './api/services'
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
  setRoute({ page: 'projects' })
  Element.prototype.scrollIntoView = vi.fn()
  host = document.createElement('div')
  document.body.appendChild(host)
  root = createRoot(host)
})
afterEach(() => {
  act(() => root.unmount())
  host.remove()
  document.body.innerHTML = ''
  vi.restoreAllMocks()
})

const toasts = () => [...document.querySelectorAll('.toast')]

describe('notifications in the window', () => {
  it('tells Go when clicks may run: the tools gate is open and the accounts are loaded', async () => {
    const ready = vi.spyOn(Notify, 'SetReady')
    act(() => root.render(createElement(App)))
    await until(() => ready.mock.calls.length > 0)
    expect(ready).toHaveBeenCalledWith(true)
  })

  it('a notify:toast shows the title, the body and the button; the button goes back to Go', async () => {
    act(() => root.render(createElement(App)))
    await until(() => !!document.querySelector('.app'))
    const respond = vi.spyOn(Notify, 'RespondToast')
    act(() =>
      mockEmit('notify:toast', { id: 'signin:acme#1', kind: 'signin.ended', title: 'Sign-in expired: acme', body: 'acme-prod. Sign in again to continue.', button: 'Sign in again', tone: NotifyTone.NotifyToneDanger, sticky: true }),
    )
    await until(() => toasts().length > 0)
    const t = toasts()[0]
    expect(t.classList.contains('toast--danger')).toBe(true)
    expect(t.textContent).toContain('Sign-in expired: acme')
    expect(t.textContent).toContain('acme-prod. Sign in again to continue.')
    const btn = [...t.querySelectorAll('button')].find((b) => b.textContent === 'Sign in again')
    await act(async () => btn?.click())
    expect(respond).toHaveBeenCalledWith('signin:acme#1', 'button')
  })

  it('a warning toast has no button when the notice has none', async () => {
    act(() => root.render(createElement(App)))
    await until(() => !!document.querySelector('.app'))
    act(() => mockEmit('notify:toast', { id: 'x#1', kind: 'other', title: 'Hello', body: '', tone: NotifyTone.NotifyToneWarn, sticky: false }))
    await until(() => toasts().length > 0)
    expect(toasts()[0].classList.contains('toast--warn')).toBe(true)
    expect(toasts()[0].querySelector('button.link')).toBeNull()
  })

  it('app:navigate "show" opens Accounts at the portal; "signed-in" adds "Already signed in"', async () => {
    act(() => root.render(createElement(App)))
    await until(() => !!document.querySelector('.app'))
    act(() => mockEmit('app:navigate', { page: 'accounts', action: 'show', arg: 'initech' }))
    await until(() => !!document.querySelector('[data-portal="initech"]'))
    expect(document.querySelector('[data-portal="initech"]')).not.toBeNull()
    expect(toasts().some((t) => t.textContent?.includes('Already signed in'))).toBe(false)
    act(() => mockEmit('app:navigate', { page: 'accounts', action: 'signed-in', arg: 'initech' }))
    await until(() => toasts().some((t) => t.textContent?.includes('Already signed in')))
    expect(toasts().some((t) => t.textContent?.includes('Already signed in'))).toBe(true)
  })
})
