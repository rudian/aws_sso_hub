import { act, createElement } from 'react'
import type { ReactElement } from 'react'
import { createRoot } from 'react-dom/client'
import type { Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { resetToolMock } from '../../api/mock/system'
import { SetupFix, SetupToolID, ToolStatus } from '../../api/models'
import type { SetupTool } from '../../api/models'
import { I18nProvider, makeT } from '../../i18n'
import type { Lang } from '../../i18n'
import { ActionsProvider } from '../../shell/AppActions'
import { ToolRows, canUpdateHere, describeToolRow, updateCommand } from './ToolRows'

;(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true

const t = makeT('en')
const tool = (over: Partial<SetupTool>): SetupTool => ({ id: SetupToolID.SetupAWSCLI, status: ToolStatus.ToolOK, version: '2.40.3', path: '/opt/homebrew/bin/aws', blocking: false, fix: SetupFix.FixNone, ...over })

describe('describeToolRow', () => {
  it('a current tool: ok, version and path, no action', () => {
    expect(describeToolRow(tool({}), t)).toEqual({ status: 'ok', text: 'Installed 2.40.3', meta: '/opt/homebrew/bin/aws', update: false })
  })
  it('older than recommended: a warning with how to update, never danger (it does not block)', () => {
    expect(describeToolRow(tool({ status: ToolStatus.ToolOutdated, version: '2.17.0' }), t)).toMatchObject({ status: 'warn', update: true, action: 'howto', command: 'brew upgrade awscli', text: 'Installed 2.17.0 · A newer version is available' })
  })
  it('the command is named only for a copy Homebrew or winget owns', () => {
    expect(updateCommand(tool({ status: ToolStatus.ToolOutdated, path: '/usr/local/bin/aws' }), 'darwin')).toBeUndefined()
    expect(updateCommand(tool({ status: ToolStatus.ToolOutdated }), 'windows')).toBe('winget upgrade --id Amazon.AWSCLI -e')
    expect(updateCommand(tool({ status: ToolStatus.ToolMissing }), 'windows')).toBe('winget install --id Amazon.AWSCLI -e')
    expect(updateCommand(tool({ id: SetupToolID.SetupGit, status: ToolStatus.ToolOutdated, path: '/opt/homebrew/bin/git' }), 'darwin')).toBe('brew upgrade git')
    expect(updateCommand(tool({ status: ToolStatus.ToolV1 }), 'darwin')).toBeUndefined()
  })
  it('v1 has its own sentence, not the setup screen\'s "v2 will be installed"', () => {
    const d = describeToolRow(tool({ status: ToolStatus.ToolV1 }), t)
    expect(d.text).toContain('Install v2 separately')
    expect(d.text).not.toContain('will be installed')
  })
  it('below the minimum, v1, unreadable and missing all offer an action', () => {
    expect(describeToolRow(tool({ status: ToolStatus.ToolTooOld, version: '2.39.5' }), t)).toMatchObject({ status: 'danger', update: true, text: 'Too old 2.39.5' })
    expect(describeToolRow(tool({ status: ToolStatus.ToolV1 }), t)).toMatchObject({ status: 'danger', update: true })
    expect(describeToolRow(tool({ status: ToolStatus.ToolError }), t)).toMatchObject({ status: 'warn', update: true })
    expect(describeToolRow(tool({ status: ToolStatus.ToolMissing, version: undefined, path: undefined }), t)).toEqual({ status: 'danger', text: 'Not installed', update: true, action: 'install', command: 'brew install awscli' })
  })
  it('a Settings path that does not work and a detected copy is used: said once, nothing to update', () => {
    expect(describeToolRow(tool({ status: ToolStatus.ToolTooOld, fix: SetupFix.FixUseDetected, override: true }), t)).toMatchObject({ status: 'warn', update: false })
  })
})

describe('ToolRows', () => {
  let host: HTMLDivElement
  let root: Root
  let open: ReturnType<typeof vi.fn>
  const mount = (ui: ReactElement, lang: Lang = 'en') => act(() => root.render(createElement(I18nProvider, { lang }, createElement(ActionsProvider, null, ui))))
  async function flush(ms: number) {
    for (let left = ms; left > 0; left -= 100) await act(async () => void (await vi.advanceTimersByTimeAsync(Math.min(100, left))))
  }
  beforeEach(() => {
    vi.useFakeTimers({ toFake: ['Date', 'setTimeout', 'clearTimeout', 'setInterval', 'clearInterval'] })
    open = vi.fn()
    window.open = open as unknown as typeof window.open
    host = document.createElement('div')
    document.body.appendChild(host)
    root = createRoot(host)
  })
  afterEach(() => {
    act(() => root.unmount())
    host.remove()
    vi.useRealTimers()
  })

  it('shows the manager, Git and the AWS CLI like the setup screen, and Update only on the old one', async () => {
    resetToolMock('outdated', 'darwin')
    mount(<ToolRows />)
    await flush(500)
    const rows = [...host.querySelectorAll('.hrow')]
    expect(rows.map((r) => r.querySelector('.hrow__title')?.textContent)).toEqual(['Homebrew', 'Git', 'AWS CLI v2'])
    const buttons = host.querySelectorAll('button')
    expect(buttons).toHaveLength(1)
    expect(buttons[0].getAttribute('aria-label')).toBe('Update AWS CLI v2')
    expect(host.textContent).toContain('brew upgrade awscli')
    expect(host.querySelector('[role="list"]')).toBeNull()
    expect(buttons[0].textContent).toBe('Update')
    expect(open).not.toHaveBeenCalled()
  })

  it('Update runs the installer: progress and Cancel in the row, then the row is read again and is ok', async () => {
    resetToolMock('outdated', 'darwin')
    mount(<ToolRows />)
    await flush(500)
    act(() => (host.querySelector('button') as HTMLButtonElement).click())
    await flush(200)
    expect(host.textContent).toContain('Updating…')
    const cancel = host.querySelector('button')
    expect(cancel?.textContent).toBe('Cancel')
    expect(cancel?.getAttribute('aria-label')).toBe('Cancel the update of AWS CLI v2')
    await flush(1500)
    expect(host.querySelectorAll('button')).toHaveLength(0)
    expect(host.textContent).toContain('Installed 2.40.3')
    expect(host.textContent).not.toContain('Updating…')
  })

  it('Cancel stops the update at once and the row is as before', async () => {
    resetToolMock('outdated', 'darwin')
    mount(<ToolRows />)
    await flush(500)
    act(() => (host.querySelector('button') as HTMLButtonElement).click())
    await flush(200)
    act(() => (host.querySelector('button') as HTMLButtonElement).click())
    await flush(300)
    expect(host.textContent).not.toContain('Updating…')
    expect(host.textContent).toContain('A newer version is available')
    expect(host.querySelector('button')?.textContent).toBe('Update')
  })

  it('a failed update says why, in words, and keeps the Update button', async () => {
    resetToolMock('outdated', 'darwin', 'network')
    mount(<ToolRows />)
    await flush(500)
    act(() => (host.querySelector('button') as HTMLButtonElement).click())
    await flush(1500)
    expect(host.textContent).toContain('The update did not finish: the download timed out')
    expect(host.textContent).not.toContain('{')
    expect(host.querySelector('[role="status"]')).not.toBeNull()
    expect(host.querySelector('button')?.textContent).toBe('Update')
    expect(host.textContent).toContain('A newer version is available')
  })

  it('the failed update in Chinese has no placeholder and no install wording', async () => {
    resetToolMock('outdated', 'darwin', 'network')
    mount(<ToolRows />, 'zh-CN')
    await flush(500)
    act(() => (host.querySelector('button') as HTMLButtonElement).click())
    await flush(1500)
    expect(host.textContent).toContain('更新没有完成：下载超时')
    expect(host.textContent).not.toContain('{')
    expect(host.textContent).not.toContain('没有装上')
  })

  it('a copy outside Homebrew and winget: How to update opens the official page and runs nothing', async () => {
    resetToolMock('old-git', 'darwin')
    mount(<ToolRows />)
    await flush(500)
    const b = host.querySelector('button') as HTMLButtonElement
    expect(b.textContent).toBe('How to update')
    act(() => b.click())
    await flush(800)
    expect(open).toHaveBeenCalledTimes(1)
    expect(String(open.mock.calls[0][0])).toContain('git-scm.com/download/mac')
  })

  it('coming back to Health while an update runs shows the run, with Cancel', async () => {
    resetToolMock('outdated', 'darwin')
    mount(<ToolRows />)
    await flush(500)
    act(() => (host.querySelector('button') as HTMLButtonElement).click())
    await flush(200)
    act(() => root.unmount())
    root = createRoot(host)
    mount(<ToolRows />)
    await flush(500)
    expect(host.textContent).toContain('Updating…')
    expect(host.querySelector('button')?.textContent).toBe('Cancel')
  })

  it('only a git or AWS CLI copy that Homebrew or winget owns is updated by the app', () => {
    expect(canUpdateHere(tool({ status: ToolStatus.ToolOutdated, path: '/usr/local/bin/aws' }), 'darwin')).toBe(false)
    expect(canUpdateHere(tool({ status: ToolStatus.ToolV1 }), 'darwin')).toBe(false)
    expect(canUpdateHere(tool({ status: ToolStatus.ToolMissing }), 'darwin')).toBe(false)
    expect(canUpdateHere(tool({ id: SetupToolID.SetupHomebrew, status: ToolStatus.ToolOutdated, path: '/opt/homebrew/bin/brew' }), 'darwin')).toBe(false)
    expect(canUpdateHere(tool({ status: ToolStatus.ToolTooOld, fix: SetupFix.FixUseDetected }), 'darwin')).toBe(false)
    expect(canUpdateHere(tool({ status: ToolStatus.ToolOutdated }), 'darwin')).toBe(true)
    expect(canUpdateHere(tool({ status: ToolStatus.ToolTooOld, path: 'C:\\Program Files\\Amazon\\AWSCLIV2\\aws.exe' }), 'windows')).toBe(true)
  })

  it('a ready machine shows no action at all; zh-CN uses the same keys', async () => {
    resetToolMock('ready', 'windows')
    mount(<ToolRows />, 'zh-CN')
    await flush(500)
    expect(host.querySelectorAll('button')).toHaveLength(0)
    expect(host.textContent).toContain('已安装 2.47.1')
    expect(host.textContent).toContain('Git for Windows')
  })
})
