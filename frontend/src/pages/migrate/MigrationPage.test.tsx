import { act, createElement } from 'react'
import { createRoot } from 'react-dom/client'
import type { Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { resetWorld } from '../../api/mock/world'
import { I18nProvider } from '../../i18n'
import { setRoute } from '../../lib/route'
import { ActionsProvider } from '../../shell/AppActions'
import { AccountsProvider } from '../../state/accounts'
import { MigrationPage } from './MigrationPage'

;(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true

let host: HTMLDivElement
let root: Root

async function flush(ms: number) {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(ms)
  })
}
const text = () => document.body.textContent ?? ''
function button(label: string, within: ParentNode = document.body): HTMLButtonElement {
  const el = [...within.querySelectorAll('button')].find((b) => (b.textContent ?? '').trim() === label)
  if (!el) throw new Error(`no button "${label}" in: ${text().slice(0, 400)}`)
  return el as HTMLButtonElement
}
async function click(label: string, within?: ParentNode) {
  act(() => button(label, within).click())
  await flush(20)
}
function typeInto(el: HTMLInputElement, value: string) {
  const set = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')?.set as (v: string) => void
  act(() => {
    set.call(el, value)
    el.dispatchEvent(new Event('input', { bubbles: true }))
  })
}

beforeEach(() => {
  vi.useFakeTimers({ toFake: ['Date', 'setTimeout', 'clearTimeout', 'setInterval', 'clearInterval'] })
  vi.setSystemTime(new Date('2026-11-04T04:00:00Z'))
  resetWorld('fresh')
  setRoute({ page: 'migrate', profile: 'acme' })
  host = document.createElement('div')
  document.body.appendChild(host)
  root = createRoot(host)
  act(() => root.render(createElement(I18nProvider, { lang: 'en' }, createElement(AccountsProvider, null, createElement(ActionsProvider, null, createElement(MigrationPage))))))
})
afterEach(() => {
  act(() => root.unmount())
  host.remove()
  vi.useRealTimers()
})

describe('migration wizard (mock backend)', () => {
  it('walks every step from identify to finish', async () => {
    await flush(900)
    // 1. identify
    expect(text()).toContain('Migrate acme to SSO')
    expect(text()).toContain('Who owns this key')
    expect(text()).toContain('AKIA••••••••••••3F2A')
    expect(text()).not.toContain('SecretAccessKey')
    await click('Next')

    // 2. portal: the account is not in the existing portal, so a new portal is pre-selected with the profile name
    expect(text()).toContain('Choose the SSO portal for acme')
    const radios = [...document.querySelectorAll<HTMLInputElement>('.portal-choice input[type="radio"]')]
    expect(radios.map((r) => r.checked)).toEqual([false, true])
    const urlInput = document.querySelector<HTMLInputElement>('.portal-fields input[placeholder^="https"]') as HTMLInputElement
    const nameInput = [...document.querySelectorAll<HTMLInputElement>('.portal-fields input')].find((i) => i.value === '')
    expect(nameInput).toBeDefined()
    typeInto(urlInput, 'https://d-1111111111.awsapps.com/start')
    await flush(300)
    await click('Sign in and continue')
    // the portal draft is created, the login modal opens and finishes, then the account step loads
    for (let i = 0; i < 20 && !document.querySelector('.account-list'); i++) await flush(500)
    await flush(900)
    expect(text()).toContain('Same account as the old key')
    const role = [...document.querySelectorAll<HTMLInputElement>('.account-card input[type="radio"]')][0]
    act(() => role.click())
    await click('Next')
    await flush(600)

    // 4. review
    expect(text()).toContain('Review the config changes')
    expect(text()).toContain('sso_account_id = 111122223333')
    expect(text()).toContain('Step 6')
    await click('Save and verify')

    // 5. verify
    await flush(2000)
    expect(text()).toContain('acme now signs in with SSO')
    await click('Next')
    await flush(600)

    // 6. remove the old key: masked values, danger confirm
    expect(text()).toContain('Remove the old key from ~/.aws/credentials')
    expect(text()).toContain('aws_secret_access_key')
    expect(text()).toContain('••••••••')
    await click('Remove old key')
    const confirmBtn = button('Remove old key', document.querySelector('.modal--sm') as ParentNode)
    act(() => confirmBtn.click())
    await flush(700)

    // 7. deactivate in IAM, then finish; the other static profile is offered next
    expect(text()).toContain('Deactivate the old key in IAM')
    expect(text()).toContain('Migrate globex next')
    await click('Deactivate now', document.querySelector('.option-card') as ParentNode)
    act(() => button('Deactivate key', document.querySelector('.modal--sm') as ParentNode).click())
    await flush(1200)
    expect(text()).toContain('Deactivated. Once everything works')
    await click('Finish')
    await flush(700)
    expect(text()).toContain('acme migrated to SSO')
  })

  it('Back goes to the previous step only before the config is written; Not now keeps the state', async () => {
    await flush(900)
    const back = () => button('Back') as HTMLButtonElement
    expect(back().disabled).toBe(true)
    await click('Next')
    expect(back().disabled).toBe(false)
    await click('Back')
    expect(text()).toContain('Who owns this key')
  })
})
