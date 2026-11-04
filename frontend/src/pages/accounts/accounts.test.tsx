import { act, createElement } from 'react'
import type { ReactElement } from 'react'
import { createRoot } from 'react-dom/client'
import type { Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { SessionState } from '../../api/models'
import type { Portal, Profile, SessionStatus } from '../../api/models'
import { buildScenario } from '../../api/mock/scenarios'
import { accountsState, resetWorld } from '../../api/mock/world'
import { I18nProvider } from '../../i18n'
import { ActionsProvider, useActions } from '../../shell/AppActions'
import { AccountsProvider } from '../../state/accounts'
import { PortalSection } from './PortalSection'
import { SessionLengthDialog } from './SessionLengthDialog'

;(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true

const NOW = Date.parse('2026-11-04T04:00:00Z')
const iso = (min: number) => new Date(NOW + min * 60_000).toISOString().replace(/\.\d{3}Z$/, 'Z')

let host: HTMLDivElement
let root: Root

function mount(ui: ReactElement) {
  act(() => root.render(createElement(I18nProvider, { lang: 'en' }, createElement(AccountsProvider, null, createElement(ActionsProvider, null, ui)))))
}

async function flush(ms: number) {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(ms)
  })
}

beforeEach(() => {
  vi.useFakeTimers({ toFake: ['Date', 'setTimeout', 'clearTimeout', 'setInterval', 'clearInterval'] })
  vi.setSystemTime(NOW)
  resetWorld('fresh')
  host = document.createElement('div')
  document.body.appendChild(host)
  root = createRoot(host)
})
afterEach(() => {
  act(() => root.unmount())
  host.remove()
  vi.useRealTimers()
})

function midInitech(over: Partial<SessionStatus> = {}): { portal: Portal; profiles: Profile[] } {
  const s = accountsState(buildScenario('mid'))
  const portal = (s.portals ?? []).find((p) => p.name === 'initech') as Portal
  return { portal: { ...portal, session: { ...portal.session, ...over } }, profiles: s.profiles ?? [] }
}

describe('PortalSection session states', () => {
  const noop = () => undefined

  it('active and known: pill with the remaining time, countdown bar on every card, sign-out button', () => {
    const { portal, profiles } = midInitech()
    mount(<PortalSection portal={portal} profiles={profiles} readOnly={false} onDialog={noop} />)
    expect(host.querySelector('.session-pill')?.textContent).toContain('Signed in, 6 h 12 min left')
    expect(host.querySelectorAll('.pass').length).toBe(2)
    expect(host.querySelectorAll('.countdown__fill').length).toBe(2)
    expect(host.textContent).toContain('Shares the initech sign-in with initech-readonly')
    expect(host.textContent).toContain('Sign out')
    expect(host.textContent).toContain('Sign in again')
  })

  it('unknown start: wording instead of a countdown, no bar', () => {
    const { portal, profiles } = midInitech({ sessionKnown: false, loginAt: undefined, sessionEndsAt: undefined, progressPct: 0 })
    mount(<PortalSection portal={portal} profiles={profiles} readOnly={false} onDialog={noop} />)
    expect(host.querySelector('.session-pill')?.textContent).toContain('Signed in · start time unknown')
    expect(host.querySelector('.countdown__fill')).toBeNull()
    expect(host.querySelector('.countdown__info')).not.toBeNull()
  })

  it('overrun: wording and an adjust button that opens the length dialog', () => {
    const { portal, profiles } = midInitech({ sessionOverrun: true, sessionEndsAt: iso(-10), progressPct: 0 })
    const onDialog = vi.fn()
    mount(<PortalSection portal={portal} profiles={profiles} readOnly={false} onDialog={onDialog} />)
    expect(host.querySelector('.session-pill')?.textContent).toContain('Signed in longer than the set session')
    expect(host.querySelector('.countdown__fill')).toBeNull()
    act(() => (host.querySelector('.countdown__action') as HTMLButtonElement).click())
    expect(onDialog).toHaveBeenCalledWith({ kind: 'length', portal: 'initech' })
  })

  it('ending soon: warn pill and the danger colour step', () => {
    const { portal, profiles } = midInitech({ state: SessionState.SessionExpiring, sessionEndsAt: iso(9) })
    mount(<PortalSection portal={portal} profiles={profiles} readOnly={false} onDialog={noop} />)
    expect(host.querySelector('.session-pill')?.textContent).toContain('Expires in 9 min')
    expect(host.querySelector('.session-pill--warn')).not.toBeNull()
    expect(host.querySelector('.countdown--danger')).not.toBeNull()
  })

  it('ended early: expired with a known session that had time left offers to shorten the length', () => {
    const { portal, profiles } = midInitech({ state: SessionState.SessionExpired, expiresAt: iso(-5), sessionEndsAt: iso(300), canRefresh: false })
    const onDialog = vi.fn()
    mount(<PortalSection portal={portal} profiles={profiles} readOnly={false} onDialog={onDialog} />)
    expect(host.textContent).toContain('The session ended earlier than the set length. Shorten it?')
    expect(host.textContent).toContain('SSO sign-in')
    const adjust = [...host.querySelectorAll('.banner button')].find((b) => b.textContent === 'Adjust length') as HTMLButtonElement
    act(() => adjust.click())
    expect(onDialog).toHaveBeenCalledWith({ kind: 'length', portal: 'initech' })
  })

  it('expired normally: no ended-early banner', () => {
    const { portal, profiles } = midInitech({ state: SessionState.SessionExpired, expiresAt: iso(-5), sessionEndsAt: iso(10), canRefresh: false })
    mount(<PortalSection portal={portal} profiles={profiles} readOnly={false} onDialog={noop} />)
    expect(host.textContent).not.toContain('ended earlier')
  })

  it('read-only config disables the writes that touch ~/.aws/config', () => {
    const { portal, profiles } = midInitech()
    mount(<PortalSection portal={portal} profiles={profiles} readOnly onDialog={noop} />)
    const add = host.querySelector('.pass-add') as HTMLButtonElement
    expect(add.disabled).toBe(true)
  })
})

describe('SessionLengthDialog', () => {
  it('saves a preset through SetSessionLength and shows the toast', async () => {
    const onClose = vi.fn()
    mount(<SessionLengthDialog portal="initech" minutes={480} onClose={onClose} />)
    const preset = [...document.body.querySelectorAll('.segmented button')].find((b) => b.textContent === '12 h') as HTMLButtonElement
    act(() => preset.click())
    const save = [...document.body.querySelectorAll('.modal__footer button')].find((b) => b.textContent === 'Save') as HTMLButtonElement
    expect(save.disabled).toBe(false)
    act(() => save.click())
    await flush(500)
    expect(onClose).toHaveBeenCalledOnce()
    expect(document.body.textContent).toContain('Session length updated')
  })

  it('rejects a custom value outside 15 minutes to 90 days', () => {
    mount(<SessionLengthDialog portal="initech" minutes={480} onClose={() => undefined} />)
    const input = document.body.querySelector('input[type="number"]') as HTMLInputElement
    const set = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')?.set as (v: string) => void
    act(() => {
      set.call(input, '3000')
      input.dispatchEvent(new Event('input', { bubbles: true }))
    })
    expect(document.body.textContent).toContain('Enter a number of hours between 0.25 and 2160')
    const save = [...document.body.querySelectorAll('.modal__footer button')].find((b) => b.textContent === 'Save') as HTMLButtonElement
    expect(save.disabled).toBe(true)
  })
})

describe('LoginModal', () => {
  function Opener({ onResult }: { onResult: (r: unknown) => void }) {
    const { openLogin } = useActions()
    return createElement('button', { id: 'go', onClick: () => void openLogin('initech').then(onResult) }, 'go')
  }

  it('walks starting, waiting, success and closes itself with the result and a toast', async () => {
    const results: unknown[] = []
    mount(createElement(Opener, { onResult: (r) => results.push(r) }))
    act(() => (host.querySelector('#go') as HTMLButtonElement).click())
    await flush(100)
    expect(document.body.textContent).toContain('Sign in to initech')
    expect(document.body.textContent).toContain('Opening your browser…')
    await flush(700)
    expect(document.body.textContent).toContain('Finish signing in in your browser')
    expect(document.body.textContent).toContain('initech-admin')
    expect(document.body.textContent).toContain('Waiting. Expires in')
    await flush(2000)
    expect(document.body.textContent).toContain('You’re signed in')
    expect(document.body.querySelector('.success-mark')).not.toBeNull()
    await flush(1400)
    expect(document.body.querySelector('.login-modal')).toBeNull()
    expect(results).toHaveLength(1)
    expect((results[0] as { portal: string }).portal).toBe('initech')
    expect(document.body.textContent).toContain('Signed in to initech')
  })

  it('cancel asks first and "Keep waiting" keeps the flow', async () => {
    const results: unknown[] = []
    mount(createElement(Opener, { onResult: (r) => results.push(r) }))
    act(() => (host.querySelector('#go') as HTMLButtonElement).click())
    await flush(900)
    const cancel = [...document.body.querySelectorAll('.login-modal .modal__footer button')].find((b) => b.textContent === 'Cancel') as HTMLButtonElement
    act(() => cancel.click())
    await flush(50)
    expect(document.body.textContent).toContain('Cancel this sign-in?')
    const keep = [...document.body.querySelectorAll('.modal--sm button')].find((b) => b.textContent === 'Keep waiting') as HTMLButtonElement
    act(() => keep.click())
    await flush(50)
    expect(document.body.querySelector('.login-modal')).not.toBeNull()
    expect(results).toHaveLength(0)
    act(() => cancel.click())
    await flush(50)
    const yes = [...document.body.querySelectorAll('.modal--sm button')].find((b) => b.textContent === 'Cancel sign-in') as HTMLButtonElement
    act(() => yes.click())
    await flush(100)
    expect(document.body.querySelector('.login-modal')).toBeNull()
    expect(results).toEqual([null])
  })
})
