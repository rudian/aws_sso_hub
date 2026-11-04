import { act, createElement } from 'react'
import type { ReactElement } from 'react'
import { createRoot } from 'react-dom/client'
import type { Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { resetWorld } from '../../api/mock/world'
import { ProfileKind } from '../../api/models'
import { I18nProvider } from '../../i18n'
import { ActionsProvider } from '../../shell/AppActions'
import { AccountsProvider } from '../../state/accounts'
import { AppInfoProvider } from '../../state/appInfo'
import { SettingsProvider } from '../../state/settings'
import { TerminalPage } from './TerminalPage'

;(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true

let host: HTMLDivElement
let root: Root

async function mount(ui: ReactElement) {
  await act(async () => {
    root.render(
      createElement(I18nProvider, { lang: 'en' }, createElement(AppInfoProvider, null, createElement(SettingsProvider, null, createElement(AccountsProvider, null, createElement(ActionsProvider, null, ui))))),
    )
  })
  // the mock services answer after a short delay
  for (let i = 0; i < 100 && !host.querySelector('.code'); i++) {
    await act(async () => {
      await new Promise((r) => setTimeout(r, 20))
    })
  }
}

beforeEach(() => {
  host = document.createElement('div')
  document.body.appendChild(host)
  root = createRoot(host)
})
afterEach(() => {
  act(() => root.unmount())
  host.remove()
})

const buttonLabels = () => Array.from(host.querySelectorAll('button')).map((b) => b.textContent ?? '')

describe('TerminalPage', () => {
  it('shows every command with copy buttons and Open terminal for a normal profile', async () => {
    resetWorld('mid')
    await mount(<TerminalPage />)
    expect(host.querySelectorAll('.code').length).toBe(4)
    expect(host.textContent).not.toContain('not safe to paste')
    expect(buttonLabels().some((l) => l.includes('Open terminal'))).toBe(true)
  })

  it('an unsafe profile name hides the commands that contain it, the copy buttons and Open terminal, and shows one notice', async () => {
    const w = resetWorld('mid')
    const p = (w.accounts.profiles ?? []).find((x) => x.kind === ProfileKind.ProfileKindSSO)
    expect(p).toBeDefined()
    if (p) p.name = 'dev;rm -rf'
    await mount(<TerminalPage />)
    const code = Array.from(host.querySelectorAll('.code__body')).map((e) => e.textContent ?? '')
    expect(code.length).toBe(2)
    expect(code.some((c) => c.includes('rm -rf'))).toBe(false)
    expect(host.querySelectorAll('.banner').length).toBeGreaterThanOrEqual(1)
    expect(host.textContent).toContain('not safe to paste into a shell')
    expect(buttonLabels().some((l) => l.includes('Open terminal'))).toBe(false)
  })
})
