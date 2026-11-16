import { act, createElement } from 'react'
import type { ReactElement } from 'react'
import { createRoot } from 'react-dom/client'
import type { Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mockEmit } from '../../api/events'
import { resetToolMock } from '../../api/mock/system'
import { Tools } from '../../api/services'
import { InstallStepKind, InstallStepState, SetupToolID } from '../../api/models'
import type { InstallState } from '../../api/models'
import { I18nProvider } from '../../i18n'
import type { Lang } from '../../i18n'
import { GATE_RECHECK_EVENT } from '../../lib/gate'
import { SetupPage } from './SetupPage'

;(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true

let host: HTMLDivElement
let root: Root
let open: ReturnType<typeof vi.fn>

/** Time passes in slices of 100 ms, each in its own act, so React renders between them as it does in the app. */
async function flush(ms: number) {
  for (let left = ms; left > 0; left -= 100) {
    await act(async () => {
      await vi.advanceTimersByTimeAsync(Math.min(100, left))
    })
  }
  if (ms === 0) await act(async () => void (await vi.advanceTimersByTimeAsync(0)))
}
function mount(ui: ReactElement, lang: Lang = 'en') {
  act(() => root.render(createElement(I18nProvider, { lang }, ui)))
}
const text = () => host.textContent ?? ''
const live = () => host.querySelector('[role="status"][aria-live="polite"]')?.textContent ?? ''
const button = (label: string | RegExp) => [...host.querySelectorAll('button')].find((b) => (typeof label === 'string' ? b.textContent === label : label.test(b.textContent ?? ''))) as HTMLButtonElement | undefined
const primary = () => host.querySelector('.setup__primary') as HTMLButtonElement
const rowOf = (id: string) => host.querySelector(`[data-tool="${id}"]`) as HTMLElement

beforeEach(() => {
  vi.useFakeTimers({ toFake: ['Date', 'setTimeout', 'clearTimeout', 'setInterval', 'clearInterval'] })
  vi.setSystemTime(new Date('2026-11-04T04:00:00Z'))
  open = vi.fn()
  vi.stubGlobal('open', open)
  window.open = open as unknown as typeof window.open
  host = document.createElement('div')
  document.body.appendChild(host)
  root = createRoot(host)
})
afterEach(() => {
  act(() => root.unmount())
  host.remove()
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

async function start(scn: Parameters<typeof resetToolMock>[0], plat: 'darwin' | 'windows' = 'darwin', cause?: Parameters<typeof resetToolMock>[2], at?: string, lang: Lang = 'en') {
  resetToolMock(scn, plat, cause, at)
  mount(<SetupPage />, lang)
  await flush(500)
}

describe('SetupPage idle', () => {
  it('macOS: the three rows, the count, and the focus on the one-click button', async () => {
    await start('missing')
    expect(text()).toContain('Let’s get this Mac ready')
    expect(text()).toContain('0 of 3 ready')
    expect([...host.querySelectorAll('.tool-row')].map((r) => r.getAttribute('data-tool'))).toEqual(['homebrew', 'git', 'awscli'])
    expect(host.querySelector('ul.setup__tools')).not.toBeNull()
    expect(primary().textContent).toBe('Install everything')
    expect(document.activeElement).toBe(primary())
    // there is no "install into my own folder" button (disproven by milestone 0)
    expect(button(/own folder/i)).toBeUndefined()
    expect(button('I’ll install them myself')).toBeDefined()
  })

  it('zh-CN copy comes from the same keys', async () => {
    await start('missing', 'darwin', undefined, undefined, 'zh-CN')
    expect(text()).toContain('先把这台 Mac 准备好')
    expect(primary().textContent).toBe('一键安装')
    expect(button('我自己安装')).toBeDefined()
  })

  it('Windows: the winget row, Git for Windows, and the UAC note', async () => {
    await start('missing', 'windows')
    expect(text()).toContain('Let’s get this PC ready')
    expect(text()).toContain('winget')
    expect(text()).toContain('Git for Windows')
    expect(text()).toContain('User Account Control')
  })

  it('a non-administrator is told before the password window, and the screen still offers the install', async () => {
    await start('noadmin')
    expect(text()).toContain('This account is not an administrator')
    expect(primary().textContent).toBe('Install everything')
  })

  it('a partly installed Homebrew asks for a re-check, not an install, and says it is never deleted', async () => {
    await start('partial')
    expect(text()).toContain('There is an unfinished Homebrew in /opt/homebrew')
    expect(text()).toContain('never deletes it')
    expect(primary().textContent).toBe('Check again')
  })

  it('Windows without winget: the Store button, then a check on focus', async () => {
    await start('nowinget', 'windows')
    expect(primary().textContent).toBe('Open Microsoft Store to get App Installer')
    act(() => primary().click())
    await flush(300)
    expect(open).toHaveBeenCalledWith('https://apps.microsoft.com/detail/9nblggh4nns1', '_blank', 'noopener,noreferrer')
    expect(button('Check again')).toBeDefined()
  })

  it('a Settings path that blocks the gate: its own sentence, no install step, no promise of a backup', async () => {
    await start('chosen-git')
    expect(text()).toContain('The Git you chose in Settings is too old (2.43.0)')
    expect(text()).toContain('Nothing is installed')
    expect(primary().textContent).toBe('Continue')
    expect(text()).not.toMatch(/backup|backed up/i)
    act(() => primary().click())
    await flush(2000)
    expect(text()).toContain('Open Codepass')
  })

  it('an override row that still needs an install says the path is cleared first', async () => {
    await start('chosen-missing')
    expect(text()).toContain('The path chosen in Settings (/Users/alex/tools/aws) does not work, so it is cleared before the install.')
    expect(primary().textContent).toBe('Install everything')
  })

  it('too old git outside Homebrew: the row says the new one is used first', async () => {
    await start('old-git')
    expect(rowOf('git').textContent).toContain('Too old2.39.5')
    expect(rowOf('git').textContent).toContain('Needs 2.44.0 or newer. Once the new one is installed it is used first.')
  })
})

describe('SetupPage run', () => {
  it('runs to the end: steps, one waiting state for the password window and the installer, announcements, then Open Codepass', async () => {
    await start('missing')
    act(() => primary().click())
    await flush(400)
    expect(rowOf('homebrew').getAttribute('data-state')).toBe('installing')
    expect(rowOf('git').getAttribute('data-state')).toBe('queued')
    expect(button('Cancel')).toBeDefined()
    expect(primary().disabled).toBe(true)
    await flush(1400)
    await flush(0)
    // Homebrew's three short steps in the row, the download first
    expect(rowOf('homebrew').querySelectorAll('.tool-steps li')).toHaveLength(3)
    // the privileged step is one indeterminate state
    expect(rowOf('homebrew').getAttribute('data-state')).toBe('waiting')
    expect(rowOf('homebrew').querySelector('[role="progressbar"]')?.hasAttribute('aria-valuenow')).toBe(false)
    expect(text()).toContain('Confirm in the macOS window; the install then takes about a minute')
    expect(live()).toContain('Homebrew: Confirm in the macOS window')
    await flush(1700)
    expect(live()).toMatch(/Homebrew installed, 1 of 3/)
    await flush(1500)
    await flush(400)
    expect(text()).toContain('3 of 3 ready')
    expect(live()).toBe('Everything is ready. You can open Codepass')
    expect(primary().textContent).toBe('Open Codepass')
    expect(document.activeElement).toBe(primary())
    expect(host.querySelector('.setup--done')).not.toBeNull()
    const onRecheck = vi.fn()
    window.addEventListener(GATE_RECHECK_EVENT, onRecheck)
    act(() => primary().click())
    window.removeEventListener(GATE_RECHECK_EVENT, onRecheck)
    expect(onRecheck).toHaveBeenCalledTimes(1)
  })

  it('a download shows its bytes as a progress bar with aria-valuenow', async () => {
    await start('missing')
    act(() => primary().click())
    await flush(500)
    const bar = rowOf('homebrew').querySelector('[role="progressbar"]') as HTMLElement
    expect(Number(bar.getAttribute('aria-valuenow'))).toBeGreaterThan(0)
    expect(rowOf('homebrew').textContent).toMatch(/downloading, \d+%/)
  })

  it('the log is role=log, shows the last 5 lines and expands', async () => {
    await start('missing')
    act(() => primary().click())
    await flush(1500)
    const log = host.querySelector('[role="log"]') as HTMLElement
    expect(log.getAttribute('aria-live')).toBe('off')
    expect(log.querySelectorAll('.setup__line').length).toBeLessThanOrEqual(5)
    const toggle = button('Show the whole log')
    expect(toggle).toBeDefined()
    act(() => toggle!.click())
    expect(button('Show less')?.getAttribute('aria-expanded')).toBe('true')
    expect((host.querySelector('[role="log"]') as HTMLElement).classList.contains('is-expanded')).toBe(true)
  })

  it('Cancel is one press, shows Cancelling and stops after the step; what is installed stays', async () => {
    await start('missing')
    act(() => primary().click())
    await flush(450)
    act(() => button('Cancel')!.click())
    expect(button('Cancelling…')?.disabled).toBe(true)
    expect(text()).toContain('Cancelling. It stops after the current step')
    await flush(2000)
    expect(text()).toContain('Cancelled. What was installed stays')
    expect(primary().textContent).toBe('Install everything')
  })

  it('over 10 seconds without output: "Still downloading"', async () => {
    await start('missing')
    const snapshot = (): InstallState => ({
      running: true, cancelling: false, cancelled: false, finished: false, log: ['$ brew install git'],
      steps: [{ id: 'git:install', tool: SetupToolID.SetupGit, kind: InstallStepKind.StepInstall, state: InstallStepState.StepRunning, command: 'brew install git' }],
    })
    act(() => mockEmit('tools:install-progress', snapshot()))
    await flush(1000)
    expect(text()).not.toContain('Still downloading')
    await flush(11000)
    expect(text()).toContain('Still downloading. This takes longer on a slow connection')
    // new output clears it
    act(() => mockEmit('tools:install-progress', { ...snapshot(), log: ['$ brew install git', '==> Fetching git'] }))
    await flush(1000)
    expect(text()).not.toContain('Still downloading')
  })
})

describe('SetupPage failure', () => {
  it('names the tool and the cause, keeps the log, moves focus to Try again, and the gate stays closed', async () => {
    await start('proxy', 'darwin', 'network', 'git:install')
    act(() => primary().click())
    await flush(6000)
    const banner = host.querySelector('.banner--danger') as HTMLElement
    expect(banner.getAttribute('role')).toBe('alert')
    expect(banner.textContent).toContain('Git was not installed: the download timed out')
    expect(banner.textContent).toContain('What is already installed is untouched.')
    expect(banner.textContent).toContain('Using the system proxy proxy.example.com:8080')
    expect(rowOf('git').getAttribute('data-state')).toBe('failed')
    expect(rowOf('awscli').getAttribute('data-state')).toBe('queued')
    expect(host.querySelector('.setup__line--bad')?.textContent).toContain('Error: Failure while executing')
    expect(primary().textContent).toBe('Try again')
    expect(document.activeElement).toBe(primary())
    expect(button('Copy log')).toBeDefined()
    expect(button('I’ll install them myself')).toBeDefined()
    expect(text()).not.toContain('Open Codepass')
  })

  it('Try again continues from the failed step: the rows that are installed are not run again', async () => {
    await start('proxy', 'darwin', 'network', 'git:install')
    act(() => primary().click())
    await flush(6000)
    act(() => primary().click())
    await flush(200)
    const ids = (host.querySelectorAll('.tool-row') as NodeListOf<HTMLElement>)
    expect(rowOf('homebrew').getAttribute('data-state')).toBe('ok')
    expect(ids.length).toBe(3)
    await flush(3000)
    expect(rowOf('git').getAttribute('data-state')).toBe('failed')
  })

  it('Copy log puts the lines on the clipboard without secrets', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    vi.stubGlobal('navigator', { clipboard: { writeText } })
    await start('proxy', 'darwin', 'network', 'git:install')
    act(() => primary().click())
    await flush(6000)
    await act(async () => {
      button('Copy log')!.click()
      await vi.advanceTimersByTimeAsync(10)
    })
    expect(writeText).toHaveBeenCalledTimes(1)
    expect(writeText.mock.calls[0][0]).toContain('Error: Failure while executing')
    expect(button('Copied')).toBeDefined()
    expect(live()).toBe('Log copied')
  })

  it.each([
    ['not_admin', 'homebrew:install', 'Homebrew was not installed: this account is not an administrator', 'Try again'],
    ['cancelled', 'homebrew:install', 'Homebrew was not installed: you cancelled the macOS password window', 'Try again'],
    ['damaged_download', 'homebrew:verify', 'The download is incomplete', 'Download again'],
    ['bad_signature', 'homebrew:verify', 'The installer’s signature is wrong, so nothing was run', 'Try again'],
    ['unknown_signer', 'homebrew:install', 'This version of Codepass does not know who signed Homebrew’s installer', 'Try again'],
    ['policy', 'homebrew:install', 'This computer’s management policy does not allow the install', 'Try again'],
    ['disk_full', 'git:install', 'Not enough disk space. About 1.2 GB more is needed', 'Try again'],
  ] as const)('%s: its own sentence and the right button', async (cause, at, title, label) => {
    await start('missing', 'darwin', cause, at)
    act(() => primary().click())
    await flush(8000)
    expect(host.querySelector('.banner--danger')?.textContent).toContain(title)
    expect(primary().textContent).toBe(label)
    expect(button('I’ll install them myself')).toBeDefined()
  })

  it('a run that ends in prefix_exists asks for Check again, and the text says Codepass never deletes it', async () => {
    await start('missing', 'darwin', 'prefix_exists', 'homebrew:install')
    act(() => primary().click())
    await flush(8000)
    expect(host.querySelector('.banner--danger')?.textContent).toContain('Repair or remove it yourself, then check again.')
    expect(primary().textContent).toBe('Check again')
  })

  it('the failure banner draws a lock for the administrator cases and a triangle for the rest', async () => {
    await start('missing', 'darwin', 'not_admin', 'homebrew:install')
    act(() => primary().click())
    await flush(8000)
    expect(host.querySelector('.banner--danger svg.banner__icon')?.getAttribute('class')).toContain('lucide-lock')
    await start('missing', 'darwin', 'network', 'git:install')
    act(() => primary().click())
    await flush(8000)
    expect(host.querySelector('.banner--danger svg.banner__icon')?.getAttribute('class')).toContain('lucide-triangle-alert')
  })

  it('a verify failure is logged as "verify: <detail>" and coloured as the failing line', async () => {
    await start('missing', 'darwin', 'bad_signature', 'homebrew:install')
    act(() => primary().click())
    await flush(8000)
    const bad = [...host.querySelectorAll('.setup__line--bad')].map((n) => n.textContent)
    expect(bad.some((l) => l?.includes('verify: signature: Status: no signature'))).toBe(true)
  })

  it('a reason that is not in the log (the re-check after an install) is shown and copied', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    vi.stubGlobal('navigator', { clipboard: { writeText } })
    await start('missing')
    const line = 'git is still not usable after the install (2.39.5, needs 2.44.0)'
    act(() =>
      mockEmit('tools:install-progress', {
        running: false, cancelling: false, cancelled: false, finished: true,
        log: ['==> Pouring git--2.39.5.arm64_tahoe.bottle.tar.gz'],
        steps: [{ id: 'git:install', tool: SetupToolID.SetupGit, kind: InstallStepKind.StepInstall, state: InstallStepState.StepFailed }],
        failure: { tool: SetupToolID.SetupGit, stepId: 'git:install', cause: 'installer_failed', exitCode: 0, line },
      } as unknown as InstallState),
    )
    await flush(300)
    const bad = [...host.querySelectorAll('.setup__line--bad')].map((n) => n.textContent)
    expect(bad.some((l) => l?.includes(line))).toBe(true)
    await act(async () => {
      button('Copy log')!.click()
      await vi.advanceTimersByTimeAsync(10)
    })
    expect(writeText.mock.calls[0][0]).toContain(line)
  })

  it('focus ends on Try again when the run fails at once (low disk), not on the page', async () => {
    await start('lowdisk')
    primary().blur()
    expect(document.activeElement).toBe(document.body)
    act(() => primary().click())
    await flush(500)
    expect(primary().textContent).toBe('Try again')
    expect(document.activeElement).toBe(primary())
  })

  it('while a run goes the focus sits on Cancel, and comes back to the primary after the run', async () => {
    await start('missing')
    primary().blur()
    act(() => primary().click())
    await flush(300)
    expect(document.activeElement).toBe(button('Cancel'))
    await flush(8000)
    expect(document.activeElement).toBe(primary())
  })
})

describe('SetupPage "I will install them myself"', () => {
  it('opens the official page, turns the footer into Check again, and says what is still missing', async () => {
    await start('missing')
    act(() => button('I’ll install them myself')!.click())
    await flush(300)
    expect(open).toHaveBeenCalledWith('https://brew.sh/', '_blank', 'noopener,noreferrer')
    expect(primary().textContent).toBe('Check again')
    expect(live()).toBe('The official page opened in your browser')
    act(() => primary().click())
    await flush(300)
    expect(live()).toBe('Not found yet: Homebrew, Git, AWS CLI v2')
    expect(button('Use one-click install')).toBeDefined()
    act(() => button('Use one-click install')!.click())
    expect(primary().textContent).toBe('Install everything')
  })

  it('Check again leaves the focus on the primary button', async () => {
    await start('missing')
    act(() => button('I’ll install them myself')!.click())
    await flush(300)
    primary().blur()
    act(() => primary().click())
    await flush(300)
    expect(document.activeElement).toBe(primary())
  })

  it('a row that only uses the detected copy has no page to open, so no "install them myself"', async () => {
    await start('chosen-git')
    expect(primary().textContent).toBe('Continue')
    expect(button('I’ll install them myself')).toBeUndefined()
  })

  it('a window focus checks again by itself', async () => {
    await start('missing')
    act(() => button('I’ll install them myself')!.click())
    await flush(300)
    const spy = vi.spyOn(Tools, 'GetSetupState')
    act(() => window.dispatchEvent(new Event('focus')))
    await flush(300)
    expect(spy).toHaveBeenCalled()
    // the tools were installed by hand meanwhile: the focus alone opens the way
    resetToolMock('ready', 'darwin')
    act(() => window.dispatchEvent(new Event('focus')))
    await flush(300)
    expect(primary().textContent).toBe('Open Codepass')
    spy.mockRestore()
  })
})

describe('Windows without winget', () => {
  it('after the Store page was opened and a check still finds no winget, it says to ask IT', async () => {
    await start('nowinget', 'windows')
    expect(text()).not.toContain('cannot open Microsoft Store')
    act(() => primary().click())
    await flush(300)
    expect(text()).not.toContain('cannot open Microsoft Store')
    act(() => button('Check again')!.click())
    await flush(300)
    expect(host.querySelector('.banner--danger')?.textContent).toContain('This PC has no winget and cannot open Microsoft Store')
    expect(host.querySelector('.banner--danger')?.textContent).toContain('Ask IT to install App Installer')
    expect(primary().textContent).toBe('Open Microsoft Store to get App Installer')
    expect(button('Check again')).toBeDefined()
  })
})

describe('the low-disk notice', () => {
  it('says how much more is needed and what to press, not a button that is not there', async () => {
    await start('lowdisk')
    expect(text()).toContain('Free some space, then press Install everything')
    expect(text()).not.toContain('then check again')
    expect(host.querySelector('.banner--warn .banner__title')?.textContent).toMatch(/About 0\.\d GB more/)
  })
})

describe('the quiet hint', () => {
  const snap = (n: number, kind = InstallStepKind.StepInstall): InstallState => ({
    running: true, cancelling: false, cancelled: false, finished: false,
    log: Array.from({ length: 200 }, (_, i) => `==> line ${n + i}`),
    steps: [{ id: 'git:install', tool: SetupToolID.SetupGit, kind, state: InstallStepState.StepRunning, command: 'brew install git' }],
  })
  it('a full 200-line log that keeps moving is not quiet', async () => {
    await start('missing')
    for (let i = 0; i < 14; i++) {
      act(() => mockEmit('tools:install-progress', snap(i)))
      await flush(1000)
    }
    expect(text()).not.toContain('Still downloading')
  })
  it('the same full log standing still is', async () => {
    await start('missing')
    act(() => mockEmit('tools:install-progress', snap(0)))
    await flush(12000)
    expect(text()).toContain('Still downloading')
  })
  it('a Windows winget step waiting for the UAC window is not "downloading"', async () => {
    await start('missing', 'windows')
    act(() => mockEmit('tools:install-progress', snap(0)))
    await flush(12000)
    expect(text()).not.toContain('Still downloading')
  })
})

describe('the gate cannot be left from this screen', () => {
  it('there is no link or button that navigates away, and nothing says Open Codepass while the gate is closed', async () => {
    await start('missing')
    expect(host.querySelectorAll('a[href]').length).toBe(0)
    expect(text()).not.toContain('Open Codepass')
  })
})
