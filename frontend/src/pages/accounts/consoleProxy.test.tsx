import { act, createElement } from 'react'
import type { ReactElement } from 'react'
import { createRoot } from 'react-dom/client'
import type { Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { Call } from '@wailsio/runtime'
import type { Portal, Profile } from '../../api/models'
import { buildScenario } from '../../api/mock/scenarios'
import { accountsState, resetWorld } from '../../api/mock/world'
import { I18nProvider } from '../../i18n'
import { ActionsProvider } from '../../shell/AppActions'
import { AccountsProvider } from '../../state/accounts'
import { AccountsPage } from './AccountsPage'
import { ConsoleProxyDialog } from './ConsoleProxyDialog'
import { PortalSection } from './PortalSection'

const settingsState: { settings: { consoleProxies?: Record<string, string> | null } | null; update: ReturnType<typeof vi.fn> } = {
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
  settingsState.settings = { consoleProxies: {} }
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

const legacy = (p: Portal): Portal => ({ ...p, name: 'legacy:d-1234567890.awsapps.com/start', legacyFormat: true })

function menuItems(): HTMLButtonElement[] {
  act(() => (host.querySelector('.portal__tools button[aria-haspopup="menu"]') as HTMLButtonElement).click())
  return [...document.querySelectorAll<HTMLButtonElement>('[role="menuitem"]')]
}

describe('PortalSection: console proxy', () => {
  it('the portal menu opens the dialog, also for a legacy portal and with a read-only config', () => {
    const { portal, profiles } = initech()
    for (const p of [portal, legacy(portal)]) {
      const onDialog = vi.fn()
      mount(<PortalSection portal={p} profiles={profiles} readOnly onDialog={onDialog} />)
      const item = menuItems().find((b) => b.textContent === 'Open console through a proxy…')
      expect(item, p.name).toBeDefined()
      expect(item?.disabled).toBe(false)
      act(() => item?.click())
      expect(onDialog).toHaveBeenCalledWith({ kind: 'consoleProxy', portal: p.name })
    }
  })

  it('the header shows the proxy of this portal only', () => {
    const { portal, profiles } = initech()
    settingsState.settings = { consoleProxies: { other: '198.51.100.7:3128' } }
    mount(<PortalSection portal={portal} profiles={profiles} readOnly={false} onDialog={() => undefined} />)
    expect(host.querySelector('.portal__proxy')).toBeNull()

    settingsState.settings = { consoleProxies: { initech: '192.0.2.10:3128' } }
    mount(<PortalSection portal={portal} profiles={profiles} readOnly={false} onDialog={() => undefined} collapsed />)
    const label = host.querySelector('.portal__header .portal__meta .portal__proxy')
    expect(label?.textContent).toBe('Console via proxy 192.0.2.10:3128')
    expect(label?.getAttribute('title')).toBe('Console via proxy 192.0.2.10:3128')
  })

  it('settings that are not loaded yet show no label', () => {
    const { portal, profiles } = initech()
    settingsState.settings = null
    mount(<PortalSection portal={portal} profiles={profiles} readOnly={false} onDialog={() => undefined} />)
    expect(host.querySelector('.portal__proxy')).toBeNull()
  })
})

describe('ConsoleProxyDialog', () => {
  const dialog = () => document.querySelector('[role="dialog"]') as HTMLElement
  const input = () => dialog().querySelector('input') as HTMLInputElement
  const button = (label: string) => [...dialog().querySelectorAll<HTMLButtonElement>('button')].find((b) => b.textContent === label)
  const alertText = () => dialog().querySelector('.field__error')?.textContent ?? null

  function type(value: string) {
    const set = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')?.set
    act(() => {
      set?.call(input(), value)
      input().dispatchEvent(new Event('input', { bubbles: true }))
    })
  }
  async function click(label: string) {
    await act(async () => button(label)?.click())
  }
  function open(onClose = vi.fn()) {
    mount(<ConsoleProxyDialog portal={initech().portal} onClose={onClose} />)
    return onClose
  }

  it('shows what it does, an example and only Save and Cancel when no proxy is set', () => {
    open()
    expect(dialog().getAttribute('aria-modal')).toBe('true')
    expect(dialog().textContent).toContain('Open console through a proxy')
    expect(dialog().textContent).toContain('initech')
    expect(dialog().textContent).toContain('fixed egress IP')
    expect(dialog().textContent).toContain('macOS only')
    expect(input().placeholder).toBe('10.0.0.1:3128')
    expect(input().value).toBe('')
    expect(dialog().querySelector('label')?.textContent).toBe('HTTP proxy')
    expect(button('Save')).toBeDefined()
    expect(button('Cancel')).toBeDefined()
    expect(button('Turn off')).toBeUndefined()
  })

  it('checks the value inline and never saves an invalid one', async () => {
    open()
    const cases: [string, string][] = [
      ['http://192.0.2.10:3128', 'Enter host:port only, without http://, a path or spaces'],
      ['192.0.2.10:3128/path', 'Enter host:port only, without http://, a path or spaces'],
      ['192.0.2.10 :3128', 'Enter host:port only, without http://, a path or spaces'],
      ['192.0.2.10', 'Add the port, for example 10.0.0.1:3128'],
      ['192.0.2.10:70000', 'The port must be a number from 1 to 65535'],
      ['192.0.2.10:0', 'The port must be a number from 1 to 65535'],
      ['300.0.2.10:3128', 'The host must be an IPv4 address or a domain name'],
    ]
    for (const [value, message] of cases) {
      type(value)
      await click('Save')
      expect(alertText(), value).toBe(message)
      expect(dialog().querySelector('.field__error')?.getAttribute('role')).toBe('alert')
      expect(input().getAttribute('aria-invalid')).toBe('true')
      expect(input().getAttribute('aria-describedby')).toContain(dialog().querySelector('.field__error')?.id)
    }
    expect(settingsState.update).not.toHaveBeenCalled()
    // fixing the value clears the message as you type
    type('192.0.2.10:3128')
    expect(alertText()).toBeNull()
    expect(input().getAttribute('aria-invalid')).toBeNull()
  })

  it('shows a problem only after leaving the field', () => {
    open()
    type('192.0.2.10')
    expect(alertText()).toBeNull()
    act(() => {
      input().focus()
      input().blur()
    })
    expect(alertText()).toBe('Add the port, for example 10.0.0.1:3128')
  })

  it('saves the proxy in canonical form, confirms and closes', async () => {
    const onClose = open()
    type('  Proxy.Example.com:03128 ')
    await click('Save')
    expect(settingsState.update).toHaveBeenCalledTimes(1)
    expect(settingsState.update).toHaveBeenCalledWith({ consoleProxies: { initech: 'proxy.example.com:3128' } })
    expect(onClose).toHaveBeenCalledTimes(1)
    expect(document.querySelector('.toast--ok')?.textContent).toContain('Saved. The initech console opens through proxy.example.com:3128')
  })

  it('Enter in the field saves', async () => {
    const onClose = open()
    type('192.0.2.10:3128')
    await act(async () => (dialog().querySelector('form') as HTMLFormElement).requestSubmit())
    expect(settingsState.update).toHaveBeenCalledWith({ consoleProxies: { initech: '192.0.2.10:3128' } })
    expect(onClose).toHaveBeenCalled()
  })

  it('Turn off removes a proxy that is set; so does saving an empty field', async () => {
    settingsState.settings = { consoleProxies: { initech: '192.0.2.10:3128' } }
    let onClose = open()
    expect(input().value).toBe('192.0.2.10:3128')
    await click('Turn off')
    expect(settingsState.update).toHaveBeenLastCalledWith({ consoleProxies: { initech: '' } })
    expect(onClose).toHaveBeenCalledTimes(1)
    expect(document.querySelector('.toast--ok')?.textContent).toContain('Turned off. The initech console opens in your default browser')

    act(() => root.unmount())
    root = createRoot(host)
    onClose = open()
    type('   ')
    await click('Save')
    expect(settingsState.update).toHaveBeenCalledTimes(2)
    expect(settingsState.update).toHaveBeenLastCalledWith({ consoleProxies: { initech: '' } })
    expect(onClose).toHaveBeenCalledTimes(1)
  })

  it('Cancel, or saving what is already set, closes without writing', async () => {
    let onClose = open()
    type('192.0.2.10:3128')
    await click('Cancel')
    expect(onClose).toHaveBeenCalledTimes(1)

    act(() => root.unmount())
    root = createRoot(host)
    settingsState.settings = { consoleProxies: { initech: '192.0.2.10:3128' } }
    onClose = open()
    await click('Save')
    expect(onClose).toHaveBeenCalledTimes(1)

    act(() => root.unmount())
    root = createRoot(host)
    settingsState.settings = { consoleProxies: {} }
    onClose = open()
    await click('Save') // empty and nothing set
    expect(onClose).toHaveBeenCalledTimes(1)
    expect(settingsState.update).not.toHaveBeenCalled()
  })

  it('a rejected save keeps the dialog open and shows why', async () => {
    settingsState.update = vi.fn(async () => {
      throw new Call.RuntimeError('E_INVALID_INPUT', { cause: { code: 'E_INVALID_INPUT', message: 'invalid value for consoleProxies', params: { field: 'consoleProxies' } } })
    })
    const onClose = open()
    type('192.0.2.10:3128')
    await click('Save')
    expect(onClose).not.toHaveBeenCalled()
    expect(dialog().querySelector('.banner')?.textContent).toContain('Check this field: the console proxy must be host:port')
    expect(button('Save')?.disabled).toBe(false)
  })
})

describe('AccountsPage: console proxy dialog', () => {
  it('opens from the portal menu', async () => {
    mount(<AccountsPage />)
    for (let i = 0; i < 50 && !host.querySelector('[data-portal="initech"] .portal__header'); i++) {
      await act(async () => {
        await new Promise((r) => setTimeout(r, 40))
      })
    }
    act(() => (host.querySelector('[data-portal="initech"] .portal__tools button[aria-haspopup="menu"]') as HTMLButtonElement).click())
    const item = [...document.querySelectorAll<HTMLButtonElement>('[role="menuitem"]')].find((b) => b.textContent === 'Open console through a proxy…')
    act(() => item?.click())
    const d = document.querySelector('[role="dialog"]')
    expect(d?.textContent).toContain('Open console through a proxy')
    expect(d?.querySelector('input')?.placeholder).toBe('10.0.0.1:3128')
  })
})
