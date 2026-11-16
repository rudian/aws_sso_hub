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
import { ToolRows, describeToolRow, updateCommand } from './ToolRows'

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
    expect(buttons[0].getAttribute('aria-label')).toBe('How to update AWS CLI v2')
    expect(host.textContent).toContain('brew upgrade awscli')
    expect(host.querySelector('[role="list"]')).toBeNull()
    expect(buttons[0].textContent).toBe('How to update')
    act(() => buttons[0].click())
    await flush(300)
    expect(open).toHaveBeenCalledWith('https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html', '_blank', 'noopener,noreferrer')
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
