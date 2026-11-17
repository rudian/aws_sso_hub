import { act, createElement } from 'react'
import { createRoot } from 'react-dom/client'
import type { Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { Identity, Repos } from '../api'
import { mockEmit } from '../api/events'
import { BindingSource, RemoteKind, RepoProvider } from '../api/models'
import type { Repo } from '../api/models'
import { I18nProvider } from '../i18n'
import type { Lang } from '../i18n'
import { getRoute, setRoute } from '../lib/route'
import { MAIN_FOLDER, getWorkspace, resetWorkspaceStore, selectFolder, setTab } from '../state/workspace'
import { openWorkspace } from './open'
import { WorkspacePage } from './WorkspacePage'

const world: { repos: Repo[]; update: boolean; platform: string } = { repos: [], update: false, platform: 'darwin' }

vi.mock('../state/repos', () => ({ useRepos: () => ({ state: { repos: world.repos, projects: [{ id: 'p1', name: 'acme' }] }, loading: false, reload: async () => undefined }) }))
vi.mock('../state/accounts', () => ({ useAccounts: () => ({ state: { profiles: [], portals: [] } }) }))
vi.mock('../state/appInfo', () => ({ useAppInfo: () => ({ platform: world.platform }) }))
vi.mock('../pages/update/useUpdate', () => ({ useUpdateIndicator: () => world.update }))
vi.mock('../shell/AppActions', () => ({ useActions: () => ({ toast: () => undefined, confirm: async () => true, openLogin: async () => null, requestLogin: () => 'opened', navigate: () => undefined }) }))

;(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true

function repo(name: string, over: Partial<Repo> = {}): Repo {
  return {
    id: `id_${name}`,
    path: `/Users/alex/code/${name}`,
    displayPath: `~/code/${name}`,
    name,
    localName: name,
    remoteName: name,
    remote: 'origin',
    remoteUrl: 'https://git-codecommit.us-east-1.amazonaws.com/v1/repos/' + name,
    remoteKind: 'https',
    hasUserinfo: false,
    binding: { source: '', profile: 'acme', profileKnown: true, colorSlot: 1, monogram: 'AC', needsRepair: false, legacyPlain: false },
    owner: { status: 'unknown', candidates: [], checkedWith: 0, skipped: 0 },
    toFix: false,
    extra: false,
    provider: RepoProvider.ProviderCodeCommit,
    projectId: 'p1',
    hidden: false,
    pending: false,
    status: { branch: 'main', detached: false, unborn: false, upstream: '', syncKnown: false, ahead: 0, behind: 0, staged: 0, unstaged: 0, untracked: 0, conflicted: 0, changes: 5, unverified: 0, renamesSkipped: false, at: '' },
    ...over,
  } as Repo
}

let host: HTMLDivElement
let root: Root
function mount(lang: Lang = 'en') {
  act(() => root.render(createElement(I18nProvider, { lang }, createElement(WorkspacePage))))
}
const q = <T extends Element>(sel: string) => host.querySelector<T>(sel)
const tabs = () => Array.from(host.querySelectorAll<HTMLElement>('[role="tab"]')).filter((t) => t.id.startsWith('ws-tab-'))
const selectedTab = () => tabs().find((t) => t.getAttribute('aria-selected') === 'true')?.id
const press = (init: KeyboardEventInit, target: EventTarget = document.body) =>
  act(() => {
    target.dispatchEvent(new KeyboardEvent('keydown', { bubbles: true, cancelable: true, ...init }))
  })

beforeEach(() => {
  window.localStorage.clear()
  resetWorkspaceStore()
  world.repos = [repo('acme_api'), repo('acme_auth')]
  world.update = false
  world.platform = 'darwin'
  host = document.createElement('div')
  document.body.appendChild(host)
  root = createRoot(host)
  openWorkspace('id_acme_api')
})
afterEach(() => {
  act(() => root.unmount())
  host.remove()
  document.body.innerHTML = ''
})

describe('workspace route', () => {
  it('openWorkspace sets the route (page, repository) and the repository\'s tab in the store', () => {
    openWorkspace('x', 'ai')
    expect(getRoute()).toEqual({ page: 'workspace', repo: 'x' })
    expect(getWorkspace('x').tab).toBe('ai')
    openWorkspace('y')
    expect(getRoute()).toEqual({ page: 'workspace', repo: 'y' })
    expect(getWorkspace('y').tab).toBe('code')
  })

  it('openWorkspace with a tab switches a workspace that is already on screen', () => {
    mount()
    expect(selectedTab()).toBe('ws-tab-code')
    act(() => openWorkspace('id_acme_api', 'ai'))
    expect(selectedTab()).toBe('ws-tab-ai')
  })

  it('every way in records the opening and reads the git status of a repository that has none', () => {
    const mark = vi.spyOn(Repos, 'MarkOpened')
    const refresh = vi.spyOn(Repos, 'RefreshStatus')
    world.repos = [repo('acme_api', { status: undefined })]
    mount()
    expect(mark).toHaveBeenCalledTimes(1)
    expect(mark).toHaveBeenCalledWith('id_acme_api')
    expect(refresh).toHaveBeenCalledTimes(1)
    expect(refresh).toHaveBeenCalledWith(['id_acme_api'])
  })

  it('does not read the status again when the repository has one', () => {
    const refresh = vi.spyOn(Repos, 'RefreshStatus')
    mount()
    expect(refresh).not.toHaveBeenCalled()
  })

  it('opening another repository from a workspace skips the sidebar-narrows motion', () => {
    mount()
    expect(q('.ws')?.className).toBe('ws')
    act(() => setRoute({ page: 'workspace', repo: 'id_acme_auth' }))
    expect(q('.ws__repo')?.textContent).toBe('acme_auth')
    expect(q('.ws')?.className).toContain('ws--still')
  })

  it('draws the frame for the repository the route names', () => {
    mount()
    expect(q('.ws__head')).toBeTruthy()
    expect(q('.ws__rail')).toBeTruthy()
    expect(q('.ws__status')).toBeTruthy()
    expect(q('.ws__repo')?.textContent).toBe('acme_api')
    expect(q('.ws__crumb')?.textContent).toContain('acme')
    expect(q('.tree-switch')?.textContent).toContain('Main workspace')
    expect(q('.tree-switch')?.textContent).toContain('main')
  })

  it('finds the repository by folder name too (mock links)', () => {
    setRoute({ page: 'workspace', repo: 'acme_auth' })
    mount()
    expect(q('.ws__repo')?.textContent).toBe('acme_auth')
  })

  it('says so when the repository is not in the list, and Back goes to Projects', () => {
    setRoute({ page: 'workspace', repo: 'gone' })
    mount()
    expect(host.textContent).toContain('This repository is not in the list')
    const back = Array.from(host.querySelectorAll('button')).find((b) => b.textContent?.includes('Back to Projects')) as HTMLElement
    act(() => back.click())
    expect(getRoute().page).toBe('projects')
  })

  it('the header\'s back button goes to Projects', () => {
    mount()
    act(() => q<HTMLButtonElement>('.ws__head button[aria-label="Back to Projects"]')!.click())
    expect(getRoute().page).toBe('projects')
  })
})

describe('frame parts', () => {
  it('has six tabs in order, Code selected, with the count of changed files', () => {
    mount()
    expect(tabs().map((t) => t.id)).toEqual(['ws-tab-code', 'ws-tab-changes', 'ws-tab-history', 'ws-tab-branches', 'ws-tab-prs', 'ws-tab-ai'])
    expect(selectedTab()).toBe('ws-tab-code')
    expect(q('#ws-tab-changes')?.textContent).toContain('5')
    expect(q('#ws-tab-code .badge')).toBeNull()
  })

  it('a click on a tab shows its placeholder; the route\'s tab applies once on arrival', () => {
    mount()
    act(() => q<HTMLElement>('#ws-tab-history')!.click())
    expect(selectedTab()).toBe('ws-tab-history')
    expect(q('[role="tabpanel"]')?.textContent).toContain('Commit history')
    act(() => root.unmount())
    root = createRoot(host)
    resetWorkspaceStore()
    // a mock link names the tab
    setRoute({ page: 'workspace', repo: 'id_acme_api', tab: 'ai' })
    mount()
    expect(selectedTab()).toBe('ws-tab-ai')
  })

  it('arrow keys move between tabs', () => {
    mount()
    press({ key: 'ArrowRight' }, q('#ws-tab-code')!)
    expect(selectedTab()).toBe('ws-tab-changes')
    press({ key: 'End' }, q('#ws-tab-changes')!)
    expect(selectedTab()).toBe('ws-tab-ai')
  })

  it('the rail marks Projects, holds the update pip on Settings only when there is an update', () => {
    mount()
    expect(q('.ws__rail [aria-current="page"]')?.getAttribute('aria-label')).toBe('Projects')
    expect(q('.ws__rail .pip')).toBeNull()
    act(() => root.unmount())
    world.update = true
    root = createRoot(host)
    mount()
    expect(q('.ws__rail .pip')?.closest('button')?.getAttribute('aria-label')).toBe('Settings, An update is available')
  })

  it('shows the account chip, and none for a repository that has no AWS account', () => {
    mount()
    expect(q('.ws__acct')?.textContent).toContain('acme')
    act(() => root.unmount())
    world.repos = [repo('acme_api', { provider: RepoProvider.ProviderGitHub, remoteKind: RemoteKind.RemoteSSH, binding: { source: '', profileKnown: false, colorSlot: 0, monogram: '', needsRepair: false, legacyPlain: false } as Repo['binding'] })]
    root = createRoot(host)
    mount()
    expect(q('.ws__acct')).toBeNull()
    // GitHub over HTTPS: bind does not apply, so no chip and no bind button in the sheet
    act(() => root.unmount())
    world.repos = [repo('acme_web', { provider: RepoProvider.ProviderGitHub, remoteUrl: 'https://github.com/someone/acme_web.git', binding: { source: '', profileKnown: false, colorSlot: 0, monogram: '', needsRepair: false, legacyPlain: false } as Repo['binding'] })]
    root = createRoot(host)
    setRoute({ page: 'workspace', repo: 'id_acme_web' })
    mount()
    expect(q('.ws__acct')).toBeNull()
  })

  it('a git-remote-codecommit repository gets a plain chip, not a button', () => {
    world.repos = [repo('acme_api', { remoteKind: RemoteKind.RemoteGRC, binding: { source: BindingSource.BindingRemoteURL, profile: 'acme', profileKnown: true, colorSlot: 1, monogram: 'AC', needsRepair: false, legacyPlain: false } as Repo['binding'] })]
    mount()
    expect(q('.ws__acct')?.tagName).toBe('SPAN')
    expect(q('.ws__acct')?.textContent).toContain('acme')
  })

  it('the gear opens the repository settings sheet', async () => {
    mount()
    await act(async () => q<HTMLElement>('.ws__head button[aria-label="Repository settings"]')!.click())
    expect(document.querySelector('.sheet')?.textContent).toContain('Account binding')
    expect(document.querySelector('.sheet')?.textContent).toContain('Hide this repository')
  })

  it('the settings sheet reads the identity again when it changes, and offers no bind button where binding does not apply', async () => {
    const ident = (email: string) => ({ global: { name: 'Alex', email: 'alex@acme.example.com' }, projects: [{ id: 'p1', repos: [{ repoId: 'id_acme_api', effective: { name: 'Alex', email } }] }], ungrouped: [] }) as never
    const get = vi.spyOn(Identity, 'GetOverview').mockResolvedValue(ident('alex@acme.example.com'))
    world.repos = [repo('acme_api', { provider: RepoProvider.ProviderGitHub, remoteUrl: 'https://github.com/someone/acme_api.git' })]
    mount()
    await act(async () => q<HTMLElement>('.ws__head button[aria-label="Repository settings"]')!.click())
    await act(async () => undefined)
    const val = () => document.querySelector('.ws-set__val')?.textContent
    expect(val()).toBe('Alex <alex@acme.example.com>')
    expect(document.querySelector('.sheet')?.textContent).toContain('does not use an AWS account')
    get.mockResolvedValue(ident('alex@initech.example.com'))
    await act(async () => mockEmit('identity:changed', {} as never))
    await act(async () => undefined)
    expect(val()).toBe('Alex <alex@initech.example.com>')
  })

  it('hiding leaves for Projects as soon as the repository is hidden', async () => {
    const hid = vi.spyOn(Repos, 'SetHidden').mockResolvedValue(undefined as never)
    mount()
    await act(async () => q<HTMLElement>('.ws__head button[aria-label="Repository settings"]')!.click())
    const hide = Array.from(document.querySelectorAll<HTMLElement>('.sheet button')).find((b) => b.textContent === 'Hide')!
    await act(async () => hide.click())
    expect(hid).toHaveBeenCalledWith('id_acme_api', true)
    expect(getRoute().page).toBe('projects')
  })

  it('tab keys do nothing behind an open sheet', async () => {
    mount()
    await act(async () => q<HTMLElement>('.ws__head button[aria-label="Repository settings"]')!.click())
    press({ key: '5', code: 'Digit5', ctrlKey: true })
    expect(selectedTab()).toBe('ws-tab-code')
  })
})

describe('bottom panel and keys', () => {
  it('Ctrl+` opens the panel on Terminal and closes it again; the status bar button does the same', () => {
    mount()
    expect(q('.bpanel')).toBeNull()
    press({ key: '`', code: 'Backquote', ctrlKey: true })
    expect(q('.bpanel')).toBeTruthy()
    expect(q('#ws-ptab-terminal')?.getAttribute('aria-selected')).toBe('true')
    expect(q('[data-slot="terminal"]')).toBeTruthy()
    press({ key: '`', code: 'Backquote', ctrlKey: true })
    expect(q('.bpanel')).toBeNull()
    act(() => q<HTMLElement>('.ws__status button')!.click())
    expect(q('.bpanel')).toBeTruthy()
  })

  it('the panel has three tabs, a close button and a separator that the arrow keys resize', () => {
    mount()
    press({ key: '`', code: 'Backquote', ctrlKey: true })
    act(() => q<HTMLElement>('#ws-ptab-problems')!.click())
    expect(q('.bpanel__body')?.textContent).toContain('No problems')
    const grip = q<HTMLElement>('.bpanel__grip')!
    const h0 = Number(grip.getAttribute('aria-valuenow'))
    press({ key: 'ArrowUp' }, grip)
    expect(Number(q('.bpanel__grip')!.getAttribute('aria-valuenow'))).toBeGreaterThan(h0)
    act(() => q<HTMLElement>('.bpanel__tabs .icon-btn')!.click())
    expect(q('.bpanel')).toBeNull()
  })

  it('Ctrl+1..6 pick tabs on macOS; Alt+1..6 on Windows', () => {
    mount()
    press({ key: '3', code: 'Digit3', ctrlKey: true })
    expect(selectedTab()).toBe('ws-tab-history')
    press({ key: '3', code: 'Digit3', altKey: true })
    expect(selectedTab()).toBe('ws-tab-history')
    act(() => root.unmount())
    world.platform = 'windows'
    root = createRoot(host)
    mount()
    press({ key: '6', code: 'Digit6', altKey: true })
    expect(selectedTab()).toBe('ws-tab-ai')
    press({ key: '1', code: 'Digit1', ctrlKey: true })
    expect(selectedTab()).toBe('ws-tab-ai')
  })

  it('tab keys are not taken while a terminal has focus; Ctrl+` still is', () => {
    mount()
    const term = document.createElement('div')
    term.className = 'xterm'
    const ta = document.createElement('textarea')
    term.appendChild(ta)
    document.body.appendChild(term)
    press({ key: '2', code: 'Digit2', ctrlKey: true }, ta)
    expect(selectedTab()).toBe('ws-tab-code')
    press({ key: '`', code: 'Backquote', ctrlKey: true }, ta)
    expect(q('.bpanel')).toBeTruthy()
  })

  it('state is per folder and survives leaving the workspace', () => {
    mount()
    act(() => q<HTMLElement>('#ws-tab-branches')!.click())
    press({ key: '`', code: 'Backquote', ctrlKey: true })
    act(() => root.unmount())
    root = createRoot(host)
    mount()
    expect(selectedTab()).toBe('ws-tab-branches')
    expect(q('.bpanel')).toBeTruthy()
    // another folder keeps the tab and has its own panel; the main one is kept
    act(() => selectFolder('id_acme_api', 'task-1'))
    // (no such working tree in the list: the page falls back to the main workspace)
    expect(getWorkspace('id_acme_api').folder).toBe(MAIN_FOLDER)
    expect(selectedTab()).toBe('ws-tab-branches')
    act(() => setTab('id_acme_api', 'ai'))
    expect(selectedTab()).toBe('ws-tab-ai')
    expect(q('.bpanel')).toBeTruthy()
  })

  it('Ctrl+` on the Find tab shows the terminal', () => {
    mount()
    act(() => q<HTMLElement>('.ws__status button[aria-label^="Problems"]')!.click())
    expect(q('#ws-ptab-problems')?.getAttribute('aria-selected')).toBe('true')
    press({ key: '`', code: 'Backquote', ctrlKey: true })
    expect(q('#ws-ptab-terminal')?.getAttribute('aria-selected')).toBe('true')
  })

  it('Ctrl+Tab with one working tree stays where it is', () => {
    mount()
    press({ key: 'Tab', code: 'Tab', ctrlKey: true })
    expect(q('.tree-switch')?.textContent).toContain('Main workspace')
  })
})

describe('both languages', () => {
  it('draws in zh-CN with no key left over', () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => undefined)
    mount('zh-CN')
    expect(host.textContent).toContain('代码')
    expect(host.textContent).toContain('主工作区')
    expect(host.textContent).not.toMatch(/\bws\.[a-z]/)
    expect(warn).not.toHaveBeenCalled()
    warn.mockRestore()
  })
})
