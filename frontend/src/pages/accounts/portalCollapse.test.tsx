import { act, createElement } from 'react'
import type { ReactElement } from 'react'
import { createRoot } from 'react-dom/client'
import type { Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Portal, Profile } from '../../api/models'
import { buildScenario } from '../../api/mock/scenarios'
import { accountsState, resetWorld } from '../../api/mock/world'
import { I18nProvider } from '../../i18n'
import { ActionsProvider } from '../../shell/AppActions'
import { AccountsProvider } from '../../state/accounts'
import { useCollapsedPortals } from './hooks'
import { AccountsPage } from './AccountsPage'
import { PortalSection } from './PortalSection'

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

function mount(ui: ReactElement) {
  act(() => root.render(createElement(I18nProvider, { lang: 'en' }, createElement(AccountsProvider, null, createElement(ActionsProvider, null, ui)))))
}

beforeEach(() => {
  resetWorld('mid')
  settingsState.settings = null
  settingsState.update = vi.fn(async () => undefined)
  host = document.createElement('div')
  document.body.appendChild(host)
  root = createRoot(host)
})
afterEach(() => {
  act(() => root.unmount())
  host.remove()
  document.body.innerHTML = ''
})

function initech(): { portal: Portal; profiles: Profile[] } {
  const s = accountsState(buildScenario('mid'))
  const portal = (s.portals ?? []).find((p) => p.name === 'initech') as Portal
  return { portal, profiles: s.profiles ?? [] }
}

describe('PortalSection collapse', () => {
  const noop = () => undefined

  it('toggle button exposes aria state and calls onToggle', () => {
    const { portal, profiles } = initech()
    const onToggle = vi.fn()
    mount(<PortalSection portal={portal} profiles={profiles} readOnly={false} onDialog={noop} onToggle={onToggle} />)
    const btn = host.querySelector<HTMLButtonElement>('.portal__toggle')
    expect(btn).not.toBeNull()
    expect(btn?.getAttribute('aria-expanded')).toBe('true')
    const controls = btn?.getAttribute('aria-controls') as string
    expect(controls).toBeTruthy()
    expect(document.getElementById(controls)).not.toBeNull()
    expect(host.querySelectorAll('.pass').length).toBeGreaterThan(0)
    act(() => btn?.click())
    expect(onToggle).toHaveBeenCalledTimes(1)
  })

  it('collapsed shows the summary, no cards, and keeps the header tools', () => {
    const { portal, profiles } = initech()
    mount(<PortalSection portal={portal} profiles={profiles} readOnly={false} onDialog={noop} collapsed onToggle={noop} />)
    const btn = host.querySelector<HTMLButtonElement>('.portal__toggle')
    expect(btn?.getAttribute('aria-expanded')).toBe('false')
    expect(host.querySelector('.portal__summary')?.textContent).toMatch(/profiles?/)
    expect(host.querySelector('.pass')).toBeNull()
    expect(document.getElementById(btn?.getAttribute('aria-controls') as string)).not.toBeNull()
    expect(host.querySelector('.session-pill')).not.toBeNull()
    expect(host.textContent).toContain('Sign out')
    expect(host.querySelector('.portal__tools button[aria-haspopup="menu"]')).not.toBeNull()
  })
})

function Probe({ names }: { names: string[] }) {
  const f = useCollapsedPortals(names)
  return createElement(
    'div',
    null,
    createElement('span', { id: 'state' }, names.filter(f.isCollapsed).join(',')),
    createElement('button', { id: 'toggle', onClick: () => f.toggle('a') }),
  )
}

describe('useCollapsedPortals', () => {
  it('treats null settings as nothing collapsed', () => {
    mount(<Probe names={['a', 'b']} />)
    expect(host.querySelector('#state')?.textContent).toBe('')
  })

  it('toggle writes collapsedPortals', async () => {
    mount(<Probe names={['a', 'b']} />)
    await act(async () => host.querySelector<HTMLButtonElement>('#toggle')?.click())
    expect(settingsState.update).toHaveBeenCalledWith({ collapsedPortals: ['a'] })
    expect(host.querySelector('#state')?.textContent).toBe('a')
  })

  it('rolls back when the update fails', async () => {
    settingsState.update = vi.fn(async () => {
      throw new Error('boom')
    })
    mount(<Probe names={['a', 'b']} />)
    await act(async () => host.querySelector<HTMLButtonElement>('#toggle')?.click())
    expect(host.querySelector('#state')?.textContent).toBe('')
    expect(document.querySelector('.toast--danger')).not.toBeNull()
  })
})

describe('AccountsPage collapse all', () => {
  async function mountPage(scenario: 'fresh' | 'done') {
    resetWorld(scenario)
    mount(<AccountsPage />)
    for (let i = 0; i < 50 && !host.querySelector('.portal__header'); i++) {
      await act(async () => {
        await new Promise((r) => setTimeout(r, 40))
      })
    }
  }
  const button = () => host.querySelector<HTMLButtonElement>('.page__actions button[aria-label$="all portals"]')

  it('is hidden with a single portal', async () => {
    await mountPage('fresh')
    expect(host.querySelectorAll('.portal-panel').length).toBe(1)
    expect(button()).toBeNull()
  })

  it('collapses every portal and flips its label', async () => {
    await mountPage('done')
    expect(host.querySelectorAll('.portal-panel').length).toBeGreaterThan(1)
    expect(host.querySelectorAll('.pass').length).toBeGreaterThan(0)
    expect(button()?.getAttribute('aria-label')).toBe('Collapse all portals')
    await act(async () => button()?.click())
    expect(host.querySelectorAll('.pass').length).toBe(0)
    expect(button()?.getAttribute('aria-label')).toBe('Expand all portals')
  })
})
