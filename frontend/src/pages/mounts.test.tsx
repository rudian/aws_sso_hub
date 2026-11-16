// What group E built and F1-CL mounts: UpdatesCard in Settings, UpdateBanner under the Projects header, ToolRows in Health.
import { resetMockNotify } from '../api/mock/notify'
import { act, createElement } from 'react'
import type { ReactElement } from 'react'
import { createRoot } from 'react-dom/client'
import type { Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { resetUpdateMock } from '../api/mock/update'
import { resetToolMock } from '../api/mock/system'
import { resetWorld } from '../api/mock/world'
import { I18nProvider } from '../i18n'
import { ActionsProvider } from '../shell/AppActions'
import { AccountsProvider } from '../state/accounts'
import { EnvProvider } from '../state/env'
import { ReposProvider } from '../state/repos'
import { SettingsProvider } from '../state/settings'
import { HealthPage } from './health/HealthPage'
import { ProjectsPage } from './projects/ProjectsPage'
import { SettingsPage } from './settings/SettingsPage'
import { resetUpdateStore } from './update/useUpdate'

;(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true

let host: HTMLDivElement
let root: Root

function mount(ui: ReactElement) {
  act(() =>
    root.render(
      createElement(
        I18nProvider,
        { lang: 'en' },
        createElement(SettingsProvider, null, createElement(EnvProvider, null, createElement(AccountsProvider, null, createElement(ReposProvider, null, createElement(ActionsProvider, null, ui))))),
      ),
    ),
  )
}
async function until(cond: () => boolean) {
  for (let i = 0; i < 80 && !cond(); i++) {
    await act(async () => {
      await new Promise((r) => setTimeout(r, 40))
    })
  }
}

beforeEach(() => {
  Element.prototype.scrollIntoView = vi.fn()
  resetWorld('mid')
  resetUpdateMock('available')
  resetUpdateStore()
  resetToolMock()
  host = document.createElement('div')
  document.body.appendChild(host)
  root = createRoot(host)
})
afterEach(() => {
  act(() => root.unmount())
  host.remove()
  document.body.innerHTML = ''
  resetUpdateStore()
  vi.restoreAllMocks()
})

describe('mounted pieces', () => {
  it('Settings has the Updates card below the notifications card', async () => {
    resetMockNotify('?notify=notasked')
    mount(createElement(SettingsPage))
    await until(() => !!document.getElementById('set-updates') && !!document.querySelector('.notify-status'))
    expect(document.getElementById('set-updates')).not.toBeNull()
    const cards = [...document.querySelectorAll('section.card')]
    const notify = cards.findIndex((c) => c.classList.contains('notify-status'))
    expect(notify).toBeGreaterThanOrEqual(0)
    expect(notify).toBeLessThan(cards.findIndex((c) => c.contains(document.getElementById('set-updates'))))
    const ids = [...document.querySelectorAll('section.card h2')].map((h) => h.id)
    expect(ids.indexOf('set-updates')).toBeGreaterThan(ids.indexOf('set-general'))
  })

  it('Projects shows the update banner once, under the header', async () => {
    mount(createElement(ProjectsPage))
    await until(() => !!document.querySelector('.update-banner'))
    expect(document.querySelectorAll('.update-banner')).toHaveLength(1)
    expect(document.querySelector('.update-banner')?.textContent).toContain('1.1.0')
  })

  it('Health draws the tool rows from ToolRows, with no second AWS CLI or Git row', async () => {
    mount(createElement(HealthPage))
    const envText = () => document.querySelector('section[aria-labelledby="health-env"]')?.textContent ?? ''
    await until(() => envText().includes('Installed') && envText().includes('Git'))
    const text = envText()
    expect(text.match(/AWS CLI/g)?.length ?? 0).toBe(1)
    expect(text).toContain('Git')
    expect(text).not.toMatch(/Git \d|AWS CLI \d/)
  })
})
