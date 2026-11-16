import { act, createElement } from 'react'
import type { ReactElement } from 'react'
import { createRoot } from 'react-dom/client'
import type { Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mockEmit } from '../../api/events'
import { resetUpdateMock } from '../../api/mock/update'
import { UpdatePhase } from '../../api/models'
import type { UpdateState } from '../../api/models'
import { I18nProvider } from '../../i18n'
import type { Lang } from '../../i18n'
import { ActionsProvider } from '../../shell/AppActions'
import { Notes, UpdatePanel } from './UpdatePanel'
import { UpdateBanner } from './UpdateBanner'
import { UpdateDialog, UpdateFooter } from './UpdateDialog'
import { UpdatePage } from './UpdatePage'
import { cancelReopen, getReopenState } from './reopenWatch'
import { setRunningTasks } from './runningTasks'
import { openUpdateDialog, resetUpdateStore, updateActions } from './useUpdate'

;(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true

let host: HTMLDivElement
let root: Root

function mount(ui: ReactElement, lang: Lang = 'en') {
  act(() => root.render(createElement(I18nProvider, { lang }, createElement(ActionsProvider, null, ui))))
}
const st = (p: Partial<UpdateState>): UpdateState => ({ phase: UpdatePhase.UpdateAvailable, current: '1.0.0', version: '1.1.0', notes: 'one', ...p })
const button = (name: string) => Array.from(document.querySelectorAll('button')).find((b) => b.textContent?.trim() === name)

beforeEach(() => {
  host = document.createElement('div')
  document.body.appendChild(host)
  root = createRoot(host)
  setRunningTasks(0)
  try {
    localStorage.clear()
  } catch {
    /* no storage here */
  }
})
afterEach(() => {
  act(() => root.unmount())
  host.remove()
  cancelReopen()
  setRunningTasks(0)
  resetUpdateStore()
})

describe('release notes', () => {
  it('shows a note that contains HTML as text, never as markup', () => {
    mount(<Notes notes={'<img src=x onerror="alert(1)">\n- <b>bold</b> and <script>x()</script>'} />)
    const ul = host.querySelector('.notes') as HTMLElement
    expect(ul.querySelector('img, b, script')).toBeNull()
    const items = Array.from(ul.querySelectorAll('li')).map((l) => l.textContent)
    expect(items).toEqual(['<img src=x onerror="alert(1)">', '<b>bold</b> and <script>x()</script>'])
  })

  it('scrolls after eight lines and says so when there are none', () => {
    mount(<Notes notes={Array.from({ length: 10 }, (_, i) => `line ${i}`).join('\n')} />)
    expect(host.querySelector('.notes')?.getAttribute('tabindex')).toBe('0')
    mount(<Notes notes="" />)
    expect(host.textContent).toContain('no notes')
  })
})

describe('panel', () => {
  it('warns about running AI tasks while downloading', () => {
    setRunningTasks(2)
    mount(<UpdatePanel s={st({ phase: UpdatePhase.UpdateDownloading, written: 31_200_000, total: 48_600_000 })} />)
    expect(host.textContent).toContain('2 AI tasks are still running')
    expect(host.textContent).toContain('31.2 / 48.6 MB')
    expect(host.querySelector('[role=progressbar]')?.getAttribute('aria-valuenow')).toBe('64')
  })

  it('names the failed step and says the app was not changed when verification fails', () => {
    mount(<UpdatePanel s={st({ phase: UpdatePhase.UpdateRefused, step: 'verify', code: 'codesign' })} />)
    expect(host.textContent).toContain('The download did not pass verification and was discarded. The app was not changed.')
    expect(host.textContent).toContain('Step that failed: Verify signature and version')
    expect(host.querySelector('.usteps .is-error')).not.toBeNull()
    expect(host.querySelector('[role=alert]')).not.toBeNull()
  })

  it('tells the person to move the app when it cannot replace itself', () => {
    mount(<UpdatePanel s={{ phase: UpdatePhase.UpdateBlocked, current: '1.0.0', block: 'translocated', folder: '/Volumes/Codepass' }} />)
    expect(host.textContent).toContain('Move Codepass to the Applications folder, then update')
  })
})

describe('footer: reopen', () => {
  it('has one primary Reopen when no task runs', () => {
    mount(<UpdateFooter s={st({ phase: UpdatePhase.UpdateReady })} onClose={() => undefined} />)
    expect(button('Reopen')).toBeDefined()
    expect(button('Reopen when no task is running')).toBeUndefined()
  })

  it('offers "Reopen now" and "Reopen when no task is running" while tasks run, and the second one waits', () => {
    setRunningTasks(2)
    mount(<UpdateFooter s={st({ phase: UpdatePhase.UpdateReady })} onClose={() => undefined} />)
    expect(button('Reopen now')).toBeDefined()
    const wait = button('Reopen when no task is running') as HTMLButtonElement
    expect(wait.disabled).toBe(false)
    act(() => wait.click())
    expect(getReopenState().mode).toBe('waiting')
    expect((button('Reopen when no task is running') as HTMLButtonElement).disabled).toBe(true)
  })

  it('lets "Reopen when no task is running" be set while downloading; Reopen now waits for the verified copy', () => {
    setRunningTasks(1)
    mount(<UpdateFooter s={st({ phase: UpdatePhase.UpdateDownloading })} onClose={() => undefined} />)
    expect((button('Reopen when no task is running') as HTMLButtonElement).disabled).toBe(false)
    expect((button('Reopen now') as HTMLButtonElement).disabled).toBe(true)
  })

  it('on the page there is no Not now / Close button to press', () => {
    mount(<UpdateFooter s={st({})} />)
    expect(button('Not now')).toBeUndefined()
    mount(<UpdateFooter s={st({ phase: UpdatePhase.UpdateError, step: 'download', code: 'network' })} />)
    expect(button('Close')).toBeUndefined()
    expect(button('Try again')).toBeDefined()
  })

  it('failed update: Try again and the browser link', () => {
    mount(<UpdateFooter s={st({ phase: UpdatePhase.UpdateError, step: 'download', code: 'network' })} onClose={() => undefined} />)
    expect(button('Try again')).toBeDefined()
    expect(button('Download in the browser instead')).toBeDefined()
  })
})

describe('banner', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    resetUpdateMock('none')
    resetUpdateStore()
  })
  afterEach(() => vi.useRealTimers())

  it('shows for an available release, opens no dialog by itself, and is dismissible for that version', async () => {
    mount(<UpdateBanner />)
    expect(host.querySelector('.update-banner')).toBeNull()
    await act(async () => {
      mockEmit('update:state', st({}))
    })
    expect(host.querySelector('.update-banner')?.textContent).toContain('Codepass 1.1.0 is available')
    expect(document.querySelector('[role=dialog]')).toBeNull()
    act(() => (document.querySelector('.update-banner button[aria-label="Do not remind me about this version"]') as HTMLButtonElement).click())
    expect(host.querySelector('.update-banner')).toBeNull()
    await act(async () => {
      mockEmit('update:state', st({ version: '1.2.0' }))
    })
    expect(host.querySelector('.update-banner')?.textContent).toContain('1.2.0')
  })

  it('shows the download percentage after the dialog is closed', async () => {
    mount(<UpdateBanner />)
    await act(async () => {
      mockEmit('update:state', st({ phase: UpdatePhase.UpdateDownloading, written: 31_200_000, total: 48_600_000 }))
    })
    expect(host.querySelector('.update-banner')?.textContent).toContain('Downloading 64%')
  })

  it('"Update" opens the dialog and starts the download', async () => {
    mount(<UpdateBanner />)
    await act(async () => {
      mockEmit('update:state', st({}))
    })
    await act(async () => {
      ;(button('Update') as HTMLButtonElement).click()
    })
    expect(document.querySelector('[role=dialog]')).not.toBeNull()
    await act(async () => {
      await vi.advanceTimersByTimeAsync(500)
    })
    expect(document.querySelector('.modal')?.textContent).toContain('Downloading')
  })
})

describe('dialog focus and the armed watch', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    resetUpdateMock('none')
    resetUpdateStore()
  })
  afterEach(() => vi.useRealTimers())

  it('opens a ready update with tasks running on "Reopen when no task is running", not on "Reopen now"', async () => {
    setRunningTasks(2)
    mount(<UpdateDialog />)
    await act(async () => {
      mockEmit('update:state', st({ phase: UpdatePhase.UpdateReady }))
      openUpdateDialog()
      await vi.advanceTimersByTimeAsync(50)
    })
    expect(document.activeElement?.textContent?.trim()).toBe('Reopen when no task is running')
  })

  it('keeps focus inside the dialog after Update is pressed', async () => {
    mount(<UpdateDialog />)
    await act(async () => {
      mockEmit('update:state', st({}))
      openUpdateDialog()
      await vi.advanceTimersByTimeAsync(50)
    })
    expect(document.activeElement?.textContent?.trim()).toBe('Update')
    await act(async () => {
      mockEmit('update:state', st({ phase: UpdatePhase.UpdateDownloading, written: 1, total: 10 }))
      await vi.advanceTimersByTimeAsync(50)
    })
    const box = document.querySelector('[role=dialog]') as HTMLElement
    expect(box.contains(document.activeElement)).toBe(true)
    expect(document.activeElement).not.toBe(document.body)
  })

  it('an armed watch does not start its clock before the copy is verified, and ends when the flow leaves ready', async () => {
    mount(<UpdateDialog />)
    await act(async () => {
      await vi.advanceTimersByTimeAsync(500) // the first State() answer arrives
      mockEmit('update:state', st({ phase: UpdatePhase.UpdateDownloading, written: 1, total: 10 }))
      updateActions.reopenWhenIdle()
      await vi.advanceTimersByTimeAsync(120_000)
    })
    expect(getReopenState().mode).toBe('waiting') // no countdown while still downloading
    await act(async () => {
      mockEmit('update:state', st({ phase: UpdatePhase.UpdateReady }))
      await vi.advanceTimersByTimeAsync(61_000)
    })
    expect(getReopenState().mode).toBe('countdown')
    await act(async () => {
      mockEmit('update:state', st({ phase: UpdatePhase.UpdateAvailable }))
    })
    expect(getReopenState().mode).toBe('off')
  })
})

describe('Update page', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    resetUpdateMock('none')
    resetUpdateStore()
  })
  afterEach(() => vi.useRealTimers())

  it('checks once when it opens, and its Check now is off while an update is ready', async () => {
    mount(<UpdatePage />)
    await act(async () => {
      await vi.advanceTimersByTimeAsync(2000)
    })
    expect(host.textContent).toContain('Up to date: 1.0.0')
    await act(async () => {
      mockEmit('update:state', st({ phase: UpdatePhase.UpdateReady }))
    })
    expect((button('Check now') as HTMLButtonElement).disabled).toBe(true)
    expect(button('Not now')).toBeUndefined()
  })
})
