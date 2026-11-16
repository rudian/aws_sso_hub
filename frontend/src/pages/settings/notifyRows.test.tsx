import { act, createElement } from 'react'
import type { ReactElement } from 'react'
import { createRoot } from 'react-dom/client'
import type { Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { resetMockNotify } from '../../api/mock/notify'
import { getWorld, resetWorld } from '../../api/mock/world'
import { Notify } from '../../api/services'
import { I18nProvider } from '../../i18n'
import type { Lang } from '../../i18n'
import { ActionsProvider } from '../../shell/AppActions'
import { AccountsProvider } from '../../state/accounts'
import { SettingsProvider } from '../../state/settings'
import { NotifyWindowsHelp } from './NotifyRows'
import { SettingsPage } from './SettingsPage'

;(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true

let host: HTMLDivElement
let root: Root

function mount(ui: ReactElement, lang: Lang = 'en') {
  act(() => root.render(createElement(I18nProvider, { lang }, createElement(SettingsProvider, null, createElement(AccountsProvider, null, createElement(ActionsProvider, null, ui))))))
}
async function until(cond: () => boolean) {
  for (let i = 0; i < 60 && !cond(); i++) {
    await act(async () => {
      await new Promise((r) => setTimeout(r, 40))
    })
  }
}

beforeEach(() => {
  resetWorld('fresh')
  resetMockNotify('')
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

const text = () => document.body.textContent ?? ''
const card = () => document.getElementById('set-notify')?.closest('section') ?? null

async function settings(search = '', lang: Lang = 'en') {
  resetMockNotify(search)
  mount(<SettingsPage />, lang)
  await until(() => !!document.getElementById('set-reminders'))
}

describe('Settings: sign-in reminders and system notifications (design 5.14)', () => {
  it('the reminders card: the new help line, the "ended" switch and the administrator note, in English', async () => {
    await settings()
    expect(text()).toContain('Sign-in reminders')
    expect(text()).toContain('Sends a system notification. You can sign in again from it.')
    expect(text()).toContain('Notify me when a sign-in expires')
    expect(text()).toContain('One notification at the moment it expires, naming the sign-in.')
    expect(text()).toContain('Your administrator decides how long a sign-in lasts. Codepass cannot extend it; it can only sign in again.')
  })

  it('... and in Chinese', async () => {
    await settings('', 'zh-CN')
    expect(text()).toContain('登录提醒')
    expect(text()).toContain('用系统通知提醒，通知上可以直接重新登录。')
    expect(text()).toContain('登录过期时通知我')
    expect(text()).toContain('登录能保持多久由管理员决定，Codepass 不能延长，只能重新登录。')
  })

  it('the "ended" switch is on by default and stores endedNoticeOff when turned off', async () => {
    await settings()
    const sw = document.querySelector<HTMLElement>('[aria-label="Notify me when a sign-in expires"]')
    expect(sw?.getAttribute('aria-checked') ?? (sw as HTMLInputElement | null)?.checked?.toString()).toBe('true')
    await act(async () => sw?.click())
    await until(() => getWorld().settings.reminders.endedNoticeOff === true)
    expect(getWorld().settings.reminders).toMatchObject({ endedNoticeOff: true, expiryNotice: true, leadMinutes: 15 })
  })

  it('allowed: no status row', async () => {
    await settings('?notify=allowed')
    await act(async () => {
      await new Promise((r) => setTimeout(r, 200))
    })
    expect(card()).toBeNull()
  })

  it('not asked yet: neutral row with "Allow notifications", which asks and then the row goes', async () => {
    await settings('?notify=notasked')
    await until(() => !!card())
    expect(card()?.textContent).toContain('Codepass has not asked for permission to notify you yet')
    expect(card()?.querySelector('.dot--off')).not.toBeNull()
    const ask = vi.spyOn(Notify, 'RequestPermission')
    const btn = [...(card()?.querySelectorAll('button') ?? [])].find((b) => b.textContent === 'Allow notifications')
    await act(async () => btn?.click())
    expect(ask).toHaveBeenCalled()
    await until(() => card() === null) // the mock answers "allow" and says so with notify:state
    expect(card()).toBeNull()
  })

  it('off: warning row with the hint and "Open System Settings"', async () => {
    await settings('?notify=off')
    await until(() => !!card())
    expect(card()?.textContent).toContain('macOS does not allow notifications from Codepass')
    expect(card()?.textContent).toContain('Reminders about expired sign-ins and waiting AI tasks will not appear.')
    expect(card()?.querySelector('.dot--warn')).not.toBeNull()
    const open = vi.spyOn(Notify, 'OpenSystemSettings')
    const btn = [...(card()?.querySelectorAll('button') ?? [])].find((b) => b.textContent === 'Open System Settings')
    await act(async () => btn?.click())
    expect(open).toHaveBeenCalled()
    // the switches stay usable in every state
    expect(document.querySelector('[aria-label="Notify me when a sign-in expires"]')?.hasAttribute('disabled')).toBe(false)
  })

  it('off, in Chinese', async () => {
    await settings('?notify=off', 'zh-CN')
    await until(() => !!card())
    expect(card()?.textContent).toContain('macOS 没有允许 Codepass 发通知')
    expect(card()?.textContent).toContain('打开系统设置')
  })

  it('the row asks again when the window gets focus back', async () => {
    await settings('?notify=off')
    await until(() => !!card())
    resetMockNotify('?notify=allowed') // the person allowed it in System Settings
    await act(async () => {
      window.dispatchEvent(new Event('focus'))
    })
    await until(() => card() === null)
    expect(card()).toBeNull()
  })
})

describe('Windows help line', () => {
  it('is shown on Windows only, with the link', async () => {
    mount(<NotifyWindowsHelp platform="darwin" />)
    expect(text()).toBe('')
    mount(<NotifyWindowsHelp platform="windows" />)
    expect(text()).toContain('No notifications? Check Codepass under Windows Settings → System → Notifications.')
    const open = vi.spyOn(Notify, 'OpenSystemSettings')
    await act(async () => document.querySelector<HTMLButtonElement>('.notify-help button')?.click())
    expect(open).toHaveBeenCalled()
  })

  it('in Chinese', () => {
    mount(<NotifyWindowsHelp platform="windows" />, 'zh-CN')
    expect(text()).toContain('没看到通知？请检查 Windows 设置 → 系统 → 通知 里的 Codepass。')
    expect(text()).toContain('打开 Windows 设置')
  })
})
