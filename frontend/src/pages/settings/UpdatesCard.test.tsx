import { act, createElement } from 'react'
import { createRoot } from 'react-dom/client'
import type { Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mockEmit } from '../../api/events'
import { resetUpdateMock } from '../../api/mock/update'
import { UpdatePhase } from '../../api/models'
import { I18nProvider } from '../../i18n'
import { ActionsProvider } from '../../shell/AppActions'
import { resetUpdateStore } from '../update/useUpdate'
import { UpdatesCard } from './UpdatesCard'

;(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true

let host: HTMLDivElement
let root: Root

beforeEach(() => {
  vi.useFakeTimers()
  resetUpdateMock('none')
  resetUpdateStore()
  host = document.createElement('div')
  document.body.appendChild(host)
  root = createRoot(host)
})
afterEach(() => {
  act(() => root.unmount())
  host.remove()
  resetUpdateStore()
  vi.useRealTimers()
})

describe('Settings > Updates card', () => {
  it('loads the daily-check setting, saves a change, and checks now', async () => {
    act(() => root.render(createElement(I18nProvider, { lang: 'en' }, createElement(ActionsProvider, null, createElement(UpdatesCard)))))
    await act(async () => {
      await vi.advanceTimersByTimeAsync(300)
    })
    const sw = host.querySelector('[role=switch]') as HTMLButtonElement
    expect(sw.getAttribute('aria-checked')).toBe('true')
    await act(async () => {
      sw.click()
      await vi.advanceTimersByTimeAsync(500)
    })
    expect((host.querySelector('[role=switch]') as HTMLButtonElement).getAttribute('aria-checked')).toBe('false')

    const check = Array.from(host.querySelectorAll('button')).find((b) => b.textContent?.includes('Check now')) as HTMLButtonElement
    await act(async () => {
      check.click()
      await vi.advanceTimersByTimeAsync(1000)
    })
    expect(host.textContent).toContain('Up to date: 1.0.0')
  })

  it('offers the update when a release is available', async () => {
    act(() => root.render(createElement(I18nProvider, { lang: 'en' }, createElement(ActionsProvider, null, createElement(UpdatesCard)))))
    await act(async () => {
      mockEmit('update:state', { phase: UpdatePhase.UpdateAvailable, current: '1.0.0', version: '1.1.0' })
    })
    expect(host.textContent).toContain('Codepass 1.1.0 is available')
    expect(host.textContent).toContain('View update')
  })

  const text = (e: { phase: UpdatePhase; code?: string; step?: string; block?: string; folder?: string }) => async () => {
    act(() => root.render(createElement(I18nProvider, { lang: 'en' }, createElement(ActionsProvider, null, createElement(UpdatesCard)))))
    await act(async () => {
      mockEmit('update:state', { current: '1.0.0', checkedAt: new Date().toISOString(), ...e })
    })
    return host.textContent ?? ''
  }

  it('says a build without an update channel has none, not that GitHub is unreachable', async () => {
    const t = await text({ phase: UpdatePhase.UpdateRefused, code: 'channel', step: 'check' })()
    expect(t).toContain('This build has no update channel set up yet.')
    expect(t).not.toContain('unreachable')
    const n = await text({ phase: UpdatePhase.UpdateError, code: 'network', step: 'check' })()
    expect(n).toContain('GitHub is unreachable')
  })

  it('names the folder fix for a blocked copy and shows no "from GitHub" time', async () => {
    const t = await text({ phase: UpdatePhase.UpdateBlocked, block: 'translocated', folder: '/Volumes/Codepass', step: 'check' })()
    expect(t).toContain('Move Codepass')
    expect(t).toContain('/Volumes/Codepass')
    expect(t).not.toContain('from GitHub')
  })

  it('turns Check now off while an update is ready', async () => {
    await text({ phase: UpdatePhase.UpdateReady, code: undefined })()
    const check = Array.from(host.querySelectorAll('button')).find((b) => b.textContent?.includes('Check now')) as HTMLButtonElement
    expect(check.disabled).toBe(true)
  })
})
