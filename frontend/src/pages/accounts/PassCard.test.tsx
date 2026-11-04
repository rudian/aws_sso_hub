import { act, createElement } from 'react'
import type { ReactElement } from 'react'
import { createRoot } from 'react-dom/client'
import type { Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
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
