import { act, createElement } from 'react'
import type { ReactElement } from 'react'
import { createRoot } from 'react-dom/client'
import type { Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mockEmit } from '../../api/events'
import { getWorld, resetWorld } from '../../api/mock/world'
import { I18nProvider } from '../../i18n'
import { ActionsProvider } from '../../shell/AppActions'
import { AccountsProvider } from '../../state/accounts'
import { AccountsPage } from './AccountsPage'
import { requestPortalFocus } from './focus'

const settingsState: { settings: { collapsedPortals: string[] | null } | null; update: ReturnType<typeof vi.fn> } = {
  settings: null,
  update: vi.fn(),
}
vi.mock('../../state/settings', () => ({
  useSettings: () => settingsState,
}))

;(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true

let host: HTMLDivElement
let root: Root
const scroll = vi.fn()

function mount(ui: ReactElement) {
  act(() => root.render(createElement(I18nProvider, { lang: 'en' }, createElement(AccountsProvider, null, createElement(ActionsProvider, null, ui)))))
}
async function until(cond: () => boolean) {
  for (let i = 0; i < 60 && !cond(); i++) {
    await act(async () => {
      await new Promise((r) => setTimeout(r, 40))
    })
  }
}

beforeEach(() => {
  resetWorld('done')
  settingsState.settings = { collapsedPortals: ['initech'] }
  settingsState.update = vi.fn(async () => undefined)
  scroll.mockClear()
  Element.prototype.scrollIntoView = scroll
  host = document.createElement('div')
  document.body.appendChild(host)
  root = createRoot(host)
})
afterEach(() => {
  act(() => root.unmount())
  host.remove()
  document.body.innerHTML = ''
})

const section = (name: string) => host.querySelector<HTMLElement>(`[data-portal="${name}"]`)

describe('open Accounts at a portal (a click on a sign-in notification)', () => {
  it('a request made before the page mounted scrolls to the portal and shows a collapsed one open, without saving that', async () => {
    requestPortalFocus('initech') // the click that launched the app: the page is not there yet
    mount(<AccountsPage />)
    await until(() => scroll.mock.calls.length > 0)
    expect(scroll).toHaveBeenCalledTimes(1)
    expect(scroll.mock.instances[0]).toBe(section('initech'))
    expect(section('initech')?.querySelector('.pass')).not.toBeNull() // open for this view
    expect(settingsState.update).not.toHaveBeenCalled() // the saved fold state is untouched
  })

  it('later accounts updates do not scroll to the portal again', async () => {
    requestPortalFocus('initech')
    mount(<AccountsPage />)
    await until(() => scroll.mock.calls.length > 0)
    for (let i = 0; i < 3; i++) {
      await act(async () => {
        mockEmit('accounts:changed', JSON.parse(JSON.stringify(getWorld().accounts)))
        await new Promise((r) => setTimeout(r, 120))
      })
    }
    expect(scroll).toHaveBeenCalledTimes(1)
  })

  it('a later request moves to another portal', async () => {
    settingsState.settings = { collapsedPortals: [] }
    mount(<AccountsPage />)
    await until(() => !!section('acme'))
    act(() => requestPortalFocus('acme'))
    await until(() => scroll.mock.calls.length > 0)
    expect(scroll.mock.instances[0]).toBe(section('acme'))
  })

  it('folding the portal by hand ends the one-time view', async () => {
    requestPortalFocus('initech')
    mount(<AccountsPage />)
    await until(() => scroll.mock.calls.length > 0)
    const toggle = section('initech')?.querySelector<HTMLButtonElement>('.portal__toggle')
    await act(async () => toggle?.click())
    expect(section('initech')?.querySelector('.pass')).toBeNull() // back to its saved, collapsed state
    expect(settingsState.update).not.toHaveBeenCalled()
  })

  it('an unknown portal scrolls nowhere', async () => {
    requestPortalFocus('no-such-portal')
    mount(<AccountsPage />)
    await until(() => !!section('acme'))
    await act(async () => {
      await new Promise((r) => setTimeout(r, 150))
    })
    expect(scroll).not.toHaveBeenCalled()
  })
})
