import { act, createElement } from 'react'
import type { ReactElement } from 'react'
import { createRoot } from 'react-dom/client'
import type { Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { Accounts, Repos } from '../../api'
import { resetWorld } from '../../api/mock/world'
import { ErrorCode } from '../../api/models'
import { LoginMode } from '../../api/models'
import type { BindOutcome } from '../../api/models'
import { I18nProvider } from '../../i18n'
import { ActionsProvider } from '../../shell/AppActions'
import { AccountsProvider } from '../../state/accounts'
import { CloneModal, folderNameReason } from './CloneModal'

interface FakeSettings {
  lastCloneProfile: string
  lastCloneRegion: string
}
const settingsState: { settings: FakeSettings | null; update: ReturnType<typeof vi.fn> } = { settings: null, update: vi.fn() }
vi.mock('../../state/settings', () => ({
  useSettings: () => settingsState,
}))

;(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true

let host: HTMLDivElement
let root: Root

// acme (us-east-1) lists, in order: acme_api, acme_auth, acme_data, acme_design, acme_docs, ...

function ui(props: Partial<Parameters<typeof CloneModal>[0]> = {}): ReactElement {
  return createElement(
    I18nProvider,
    { lang: 'en' },
    createElement(AccountsProvider, null, createElement(ActionsProvider, null, createElement(CloneModal, { defaultParent: '/work', cliReady: true, onClose: () => undefined, ...props }))),
  )
}
async function mount(props: Partial<Parameters<typeof CloneModal>[0]> = {}) {
  await act(async () => {
    root.render(ui(props))
  })
  await waitFor(() => !!document.querySelector('.clone-row'))
}
async function waitFor(cond: () => boolean, ms = 4000) {
  const end = Date.now() + ms
  while (!cond() && Date.now() < end) {
    await act(async () => {
      await new Promise((r) => realSetTimeout(r, 10))
    })
  }
  if (!cond()) throw new Error('waitFor timed out')
}
// The mock API answers after 120 to 2500 ms; every timer runs 20x faster here so the file takes seconds, not half a minute.
const TIME_SCALE = 20
const realSetTimeout = globalThis.setTimeout.bind(globalThis)
const settle = () => act(async () => new Promise((r) => realSetTimeout(r, 60)))

const rowCheck = (name: string) => document.querySelector<HTMLInputElement>(`input[aria-label="Select ${name}"]`) as HTMLInputElement
const rowPick = (name: string) => {
  const row = Array.from(document.querySelectorAll('.clone-row')).find((r) => r.querySelector('.row__primary')?.textContent === name)
  return row?.querySelector('button') as HTMLButtonElement
}
const selected = () => Array.from(document.querySelectorAll<HTMLInputElement>('.clone-row input')).filter((i) => i.checked).map((i) => i.getAttribute('aria-label')?.replace('Select ', ''))
const bar = () => document.querySelector<HTMLInputElement>('.clone-pick__bar input') as HTMLInputElement
const button = (label: string | RegExp) =>
  Array.from(document.querySelectorAll<HTMLButtonElement>('.modal button')).find((b) => (typeof label === 'string' ? b.textContent === label : label.test(b.textContent ?? ''))) as HTMLButtonElement | undefined
const click = (el: HTMLElement | undefined) => act(async () => el?.click())
async function setSelect(el: HTMLSelectElement, value: string) {
  await act(async () => {
    Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, 'value')?.set?.call(el, value)
    el.dispatchEvent(new Event('change', { bubbles: true }))
  })
}
async function setInput(el: HTMLInputElement, value: string) {
  await act(async () => {
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')?.set?.call(el, value)
    el.dispatchEvent(new Event('input', { bubbles: true }))
  })
}
const accountSelect = () => document.querySelectorAll<HTMLSelectElement>('.modal select')[0]
const regionSelect = () => document.querySelectorAll<HTMLSelectElement>('.modal select')[1]
const cloneBtn = () => document.querySelector<HTMLButtonElement>('.modal__footer .btn--primary, .modal__footer button:last-child') as HTMLButtonElement
const text = () => document.querySelector('.modal')?.textContent ?? ''

const ok = (name: string): BindOutcome => ({ repoId: name, ok: true, repo: { id: `id-${name}`, name, path: `/work/${name}` } as BindOutcome['repo'] })
const appErr = (code: ErrorCode, message = 'boom') => ({ code, message })
type CloneFn = typeof Repos.Clone
const spyClone = (impl: (name: string) => Promise<BindOutcome>) =>
  vi.spyOn(Repos, 'Clone').mockImplementation(((req: { repoName: string }) => impl(req.repoName)) as unknown as CloneFn)
const names = (spy: ReturnType<typeof spyClone>) => spy.mock.calls.map((c) => (c[0] as { repoName: string }).repoName)

async function pickAndStart(...n: string[]) {
  for (const x of n) await click(rowCheck(x))
  await settle()
  await click(cloneBtn())
}

beforeEach(() => {
  const scaled = ((fn: TimerHandler, ms?: number, ...args: unknown[]) => realSetTimeout(fn as () => void, ms ? Math.round(ms / TIME_SCALE) : ms, ...args)) as unknown as typeof setTimeout
  vi.spyOn(globalThis, 'setTimeout').mockImplementation(scaled)
  if (window.setTimeout !== globalThis.setTimeout) vi.spyOn(window, 'setTimeout').mockImplementation(scaled)
  resetWorld('done')
  settingsState.settings = null
  settingsState.update = vi.fn(async () => undefined)
  host = document.createElement('div')
  document.body.appendChild(host)
  root = createRoot(host)
})
afterEach(() => {
  vi.restoreAllMocks()
  act(() => root.unmount())
  host.remove()
  document.body.innerHTML = ''
})

describe('CloneModal selection', () => {
  it('a row click with 0 or 1 selected picks only that repository, and shows the single form', async () => {
    await mount({ initialProfile: 'acme' })
    await click(rowPick('acme_api'))
    expect(selected()).toEqual(['acme_api'])
    await click(rowPick('acme_auth'))
    expect(selected()).toEqual(['acme_auth'])
    await settle()
    expect(Array.from(document.querySelectorAll<HTMLInputElement>('.modal input')).some((i) => i.type === 'text' && i.value === 'acme_auth' && !i.disabled)).toBe(true)
    expect(text()).toContain('git clone')
    expect(cloneBtn().textContent).toBe('Clone')
  })

  it('a run that clones nothing is neutral', async () => {
    spyClone(async () => {
      throw { code: ErrorCode.ErrDestExists, message: 'exists' }
    })
    await mount({ initialProfile: 'acme' })
    await pickAndStart('acme_api', 'acme_docs')
    await waitFor(() => !!document.querySelector('.clone-sum'))
    expect(document.querySelector('.clone-sum__icon--neutral')).not.toBeNull()
    expect(document.querySelector('.clone-sum__icon--ok')).toBeNull()
    expect(document.querySelector('.clone-sum__title')?.textContent).toBe('No repositories were cloned')
  })

  it('keeps focus inside the dialog while cloning, also after Stop is pressed', async () => {
    let release: (() => void) | undefined
    spyClone((n) => (n === 'acme_api' ? new Promise((res) => (release = () => res(ok(n)))) : Promise.resolve(ok(n))))
    await mount({ initialProfile: 'acme' })
    await pickAndStart('acme_api', 'acme_auth')
    await waitFor(() => !!button('Stop after this one'))
    expect(document.activeElement).toBe(button('Stop after this one'))
    await click(button('Stop after this one'))
    expect(document.activeElement).toBe(document.querySelector('.modal'))
    await act(async () => release?.())
    await waitFor(() => !!button('Done'))
  })

  it('checkboxes build a multi selection; with 2+ selected a row click toggles that row', async () => {
    await mount({ initialProfile: 'acme' })
    await click(rowCheck('acme_api'))
    await click(rowCheck('acme_docs'))
    expect(selected()).toEqual(['acme_api', 'acme_docs'])
    await click(rowPick('acme_auth'))
    expect(selected()).toEqual(['acme_api', 'acme_auth', 'acme_docs'])
    await click(rowPick('acme_auth'))
    expect(selected()).toEqual(['acme_api', 'acme_docs'])
    await settle()
    expect(text()).toContain('named above')
    expect(cloneBtn().textContent).toBe('Clone 2 repositories')
  })

  it('select all acts on the filtered rows only', async () => {
    await mount({ initialProfile: 'acme' })
    await click(rowCheck('acme_api'))
    await setInput(document.querySelector<HTMLInputElement>('input[aria-label="Search repositories"]') as HTMLInputElement, 'doc')
    expect(document.querySelectorAll('.clone-row').length).toBe(1)
    expect(text()).toContain('1 not in results')
    await click(bar())
    expect(selected()).toEqual(['acme_docs'])
    await setInput(document.querySelector<HTMLInputElement>('input[aria-label="Search repositories"]') as HTMLInputElement, '')
    expect(selected()).toEqual(['acme_api', 'acme_docs'])
    await setInput(document.querySelector<HTMLInputElement>('input[aria-label="Search repositories"]') as HTMLInputElement, 'doc')
    await click(bar())
    await setInput(document.querySelector<HTMLInputElement>('input[aria-label="Search repositories"]') as HTMLInputElement, '')
    expect(selected()).toEqual(['acme_api'])
  })

  it('changing the account or the region clears the selection', async () => {
    await mount({ initialProfile: 'acme' })
    await click(rowCheck('acme_api'))
    await click(rowCheck('acme_docs'))
    await setSelect(regionSelect(), 'eu-west-1')
    await waitFor(() => selected().length === 0)
    await setSelect(regionSelect(), 'us-east-1')
    await waitFor(() => !!document.querySelector('.clone-row'))
    await click(rowCheck('acme_api'))
    expect(selected()).toEqual(['acme_api'])
    await setSelect(accountSelect(), 'initech-admin')
    await waitFor(() => !!document.querySelector('.clone-row') && selected().length === 0)
    expect(selected()).toEqual([])
  })

  it('existing folders are skipped and not counted', async () => {
    await mount({ initialProfile: 'acme', defaultParent: '/Users/alex/Documents/code' })
    await click(rowCheck('acme_api'))
    await click(rowCheck('acme_auth'))
    await click(rowCheck('acme_usage'))
    await settle()
    expect(text()).toContain('already exist')
    expect(cloneBtn().textContent).toBe('Clone 1 repository')
  })
})

describe('CloneModal multi clone', () => {
  it('runs one Clone per repository in list order and ends on the summary', async () => {
    const spy = spyClone(async (n) => ok(n))
    await mount({ initialProfile: 'acme' })
    await pickAndStart('acme_docs', 'acme_api', 'acme_auth')
    await waitFor(() => text().includes('Cloned 3 repositories'))
    expect(names(spy)).toEqual(['acme_api', 'acme_auth', 'acme_docs'])
    expect(document.querySelectorAll('.clone-result .icon-btn').length).toBe(3)
  })

  it('a failure continues with the rest and the summary lists the failure first', async () => {
    const spy = spyClone(async (n) => {
      if (n === 'acme_auth') throw appErr(ErrorCode.ErrInternal, 'disk full')
      return ok(n)
    })
    await mount({ initialProfile: 'acme' })
    await pickAndStart('acme_api', 'acme_auth', 'acme_docs')
    await waitFor(() => text().includes('Cloned 2 of 3'))
    expect(names(spy)).toEqual(['acme_api', 'acme_auth', 'acme_docs'])
    expect(document.querySelector('.clone-result .clone-q')?.className).toContain('clone-q--failed')
    expect(text()).toContain('Try 1 again')
  })

  it.each([ErrorCode.ErrSessionExpired, ErrorCode.ErrNotSignedIn])('%s marks the rest not started without calling Clone', async (code) => {
    const spy = spyClone(async (n) => {
      if (n === 'acme_auth') throw appErr(code)
      return ok(n)
    })
    await mount({ initialProfile: 'acme' })
    await pickAndStart('acme_api', 'acme_auth', 'acme_data', 'acme_docs')
    await waitFor(() => text().includes('Cloned 1 of 4'))
    expect(names(spy)).toEqual(['acme_api', 'acme_auth'])
    expect(document.querySelectorAll('.clone-q--stopped').length).toBe(2)
    expect(document.querySelectorAll('.clone-q--failed').length).toBe(1)
  })

  it('E_DEST_EXISTS counts as skipped, not failed', async () => {
    spyClone(async (n) => {
      if (n === 'acme_auth') throw appErr(ErrorCode.ErrDestExists)
      return ok(n)
    })
    await mount({ initialProfile: 'acme' })
    await pickAndStart('acme_api', 'acme_auth')
    await waitFor(() => !!document.querySelector('.clone-result'))
    expect(document.querySelectorAll('.clone-q--exists').length).toBe(1)
    expect(document.querySelectorAll('.clone-q--failed').length).toBe(0)
    expect(text()).toContain('Cloned and bound to acme')
    expect(text()).not.toContain('Try')
  })

  it('Stop after this one finishes the current repository, skips the rest, and retry runs only those', async () => {
    let release: (() => void) | undefined
    const spy = spyClone((n) => (n === 'acme_api' ? new Promise((res) => (release = () => res(ok(n)))) : Promise.resolve(ok(n))))
    await mount({ initialProfile: 'acme' })
    await pickAndStart('acme_api', 'acme_auth', 'acme_docs')
    await waitFor(() => !!button('Stop after this one'))
    await click(button('Stop after this one'))
    expect(button('Stopping…')?.disabled).toBe(true)
    await act(async () => release?.())
    await waitFor(() => text().includes('Cloned 1 of 3'))
    expect(names(spy)).toEqual(['acme_api'])
    expect(document.querySelectorAll('.clone-q--stopped').length).toBe(2)
    await click(button('Try 2 again'))
    await waitFor(() => text().includes('Cloned 3 repositories'))
    expect(names(spy)).toEqual(['acme_api', 'acme_auth', 'acme_docs'])
  })

  it('unmounting during a multi clone stops the queue after the in-flight repository', async () => {
    let release: (() => void) | undefined
    const spy = spyClone((n) => (n === 'acme_api' ? new Promise((res) => (release = () => res(ok(n)))) : Promise.resolve(ok(n))))
    await mount({ initialProfile: 'acme' })
    await pickAndStart('acme_api', 'acme_auth', 'acme_docs')
    await waitFor(() => !!button('Stop after this one'))
    act(() => root.unmount())
    root = createRoot(host)
    await act(async () => release?.())
    await act(async () => new Promise((r) => realSetTimeout(r, 60)))
    expect(names(spy)).toEqual(['acme_api'])
    expect(settingsState.update).toHaveBeenCalledTimes(1)
  })

  it('the account becoming unusable mid-run marks the rest not started and disables retry', async () => {
    const spy = spyClone(async (n) => {
      if (n === 'initech_cfn') {
        await Accounts.Logout('initech')
        await new Promise((r) => realSetTimeout(r, 60)) // let the signed-out state reach the modal
      }
      return ok(n)
    })
    await mount({ initialProfile: 'initech-admin' })
    await pickAndStart('initech_cfn', 'initech_dashboard', 'initech_infra')
    await waitFor(() => text().includes('Cloned 1 of 3'))
    expect(names(spy)).toEqual(['initech_cfn'])
    expect(document.querySelectorAll('.clone-q--stopped').length).toBe(2)
    expect(button('Try 2 again')?.disabled).toBe(true)
    expect(text()).toContain('Sign in')
  })

  it('a failed row shows the error detail in the Details block', async () => {
    spyClone(async (n) => {
      if (n === 'acme_docs') throw { code: ErrorCode.ErrInternal, message: 'boom', detail: 'fatal: disk quota exceeded' }
      return ok(n)
    })
    await mount({ initialProfile: 'acme' })
    await pickAndStart('acme_api', 'acme_docs')
    await waitFor(() => !!document.querySelector('.clone-result'))
    const details = document.querySelector('.clone-q--failed details')
    expect(details?.querySelector('summary')?.textContent).toBe('Details')
    expect(details?.querySelector('pre')?.textContent).toBe('fatal: disk quota exceeded')
  })

  it('a successful multi run labels the folder buttons with the repository name and focuses Done', async () => {
    spyClone(async (n) => ok(n))
    await mount({ initialProfile: 'acme' })
    await pickAndStart('acme_api', 'acme_docs')
    await waitFor(() => text().includes('Cloned 2 repositories'))
    expect(document.querySelector('.clone-result button[aria-label="Open folder of acme_api"]')).not.toBeNull()
    expect(document.activeElement).toBe(button('Done'))
  })

  it('every selected folder existing shows the plain Clone label', async () => {
    await mount({ initialProfile: 'acme', defaultParent: '/Users/alex/Documents/code' })
    await click(rowCheck('acme_api'))
    await click(rowCheck('acme_auth'))
    await settle()
    expect(text()).toContain('already has a folder')
    expect(cloneBtn().textContent).toBe('Clone')
  })

  it('retry re-runs only the failed rows', async () => {
    let fail = true
    const spy = spyClone(async (n) => {
      if (n === 'acme_docs' && fail) throw appErr(ErrorCode.ErrInternal)
      return ok(n)
    })
    await mount({ initialProfile: 'acme' })
    await pickAndStart('acme_api', 'acme_docs')
    await waitFor(() => !!button('Try 1 again'))
    fail = false
    await click(button('Try 1 again'))
    await waitFor(() => text().includes('Cloned 2 repositories'))
    expect(names(spy)).toEqual(['acme_api', 'acme_docs', 'acme_docs'])
  })

  it('a single failed repository returns to the form with the error and the selection', async () => {
    spyClone(async () => {
      throw appErr(ErrorCode.ErrInternal, 'disk full')
    })
    await mount({ initialProfile: 'acme' })
    await pickAndStart('acme_api')
    await waitFor(() => !!document.querySelector('.banner'))
    expect(selected()).toEqual(['acme_api'])
    expect(document.querySelector('.clone-q-list')).toBeNull()
  })

  it('a single success shows the one-repository screen with Open folder', async () => {
    spyClone(async (n) => ok(n))
    await mount({ initialProfile: 'acme' })
    await pickAndStart('acme_api')
    await waitFor(() => text().includes('Cloned and bound to acme'))
    expect(button('Open folder')).toBeDefined()
  })
})

describe('CloneModal remembers the last account', () => {
  it('saves profile and region after a run with a success', async () => {
    spyClone(async (n) => ok(n))
    await mount({ initialProfile: 'acme' })
    await pickAndStart('acme_api', 'acme_docs')
    await waitFor(() => text().includes('Cloned 2'))
    expect(settingsState.update).toHaveBeenCalledTimes(1)
    expect(settingsState.update).toHaveBeenCalledWith({ lastCloneProfile: 'acme', lastCloneRegion: 'us-east-1' })
  })

  it('does not save after a run with no success', async () => {
    spyClone(async () => {
      throw appErr(ErrorCode.ErrInternal)
    })
    await mount({ initialProfile: 'acme' })
    await pickAndStart('acme_api', 'acme_docs')
    await waitFor(() => text().includes('No repositories were cloned'))
    expect(settingsState.update).not.toHaveBeenCalled()
  })

  it('does not save when nothing changed from the remembered values', async () => {
    settingsState.settings = { lastCloneProfile: 'acme', lastCloneRegion: 'us-east-1' }
    spyClone(async (n) => ok(n))
    await mount()
    await pickAndStart('acme_api', 'acme_docs')
    await waitFor(() => text().includes('Cloned 2'))
    expect(settingsState.update).not.toHaveBeenCalled()
  })

  it('preselect priority: initialProfile, then the last clone, then the first usable', async () => {
    settingsState.settings = { lastCloneProfile: 'initech-admin', lastCloneRegion: 'us-west-2' }
    await mount({ initialProfile: 'acme' })
    expect(accountSelect().value).toBe('acme')
    act(() => root.unmount())
    root = createRoot(host)
    await mount()
    expect(accountSelect().value).toBe('initech-admin')
    expect(regionSelect().value).toBe('us-west-2')
    act(() => root.unmount())
    root = createRoot(host)
    settingsState.settings = null
    await mount()
    expect(accountSelect().value).toBe('acme')
  })

  it('a remembered profile that is not usable is ignored', async () => {
    settingsState.settings = { lastCloneProfile: 'globex', lastCloneRegion: 'eu-west-1' }
    await mount()
    expect(accountSelect().value).toBe('acme')
    expect(text()).toContain('used for your last clone')
  })

  it('applies late when settings arrive after mount and nothing was touched', async () => {
    await mount()
    expect(accountSelect().value).toBe('acme')
    settingsState.settings = { lastCloneProfile: 'initech-admin', lastCloneRegion: 'us-west-2' }
    await act(async () => root.render(ui()))
    await waitFor(() => accountSelect().value === 'initech-admin')
    expect(regionSelect().value).toBe('us-west-2')
  })

  it('a remembered profile that is signed out is hinted first, and applied (account and region) once it signs in', async () => {
    settingsState.settings = { lastCloneProfile: 'globex', lastCloneRegion: 'eu-west-1' }
    await mount()
    expect(accountSelect().value).toBe('acme')
    expect(document.querySelector('.modal .hint')?.textContent).toContain('used for your last clone')
    await act(async () => {
      await Accounts.Login('globex', LoginMode.LoginModeBrowser)
    })
    await waitFor(() => accountSelect().value === 'globex')
    expect(regionSelect().value).toBe('eu-west-1')
  })

  it('a sign-in does not switch the account once a row is ticked', async () => {
    settingsState.settings = { lastCloneProfile: 'globex', lastCloneRegion: 'eu-west-1' }
    await mount()
    await click(rowCheck('acme_api'))
    await act(async () => {
      await Accounts.Login('globex', LoginMode.LoginModeBrowser)
    })
    await settle()
    expect(accountSelect().value).toBe('acme')
    expect(regionSelect().value).toBe('us-east-1')
    expect(selected()).toEqual(['acme_api'])
  })

  it('signing another portal in does not switch an auto-picked account when nothing is remembered', async () => {
    await act(async () => {
      await Accounts.Logout('acme')
    })
    await mount()
    const first = accountSelect().value
    await act(async () => {
      await Accounts.Login('acme', LoginMode.LoginModeBrowser)
    })
    await waitFor(() => accountSelect().querySelector('option[value="acme"]')?.hasAttribute('disabled') === false)
    await settle()
    expect(accountSelect().value).toBe(first)
  })

  it('does not apply late once the user touched something', async () => {
    await mount()
    await click(rowCheck('acme_api'))
    settingsState.settings = { lastCloneProfile: 'initech-admin', lastCloneRegion: 'us-west-2' }
    await act(async () => root.render(ui()))
    await settle()
    expect(accountSelect().value).toBe('acme')
    expect(selected()).toEqual(['acme_api'])
  })
})

describe('CloneModal local folder name', () => {
  const nameInput = (repo: string) => document.querySelector<HTMLInputElement>(`input[aria-label="Local folder name for ${repo}"]`) as HTMLInputElement
  const startBtnDisabled = () => cloneBtn().disabled

  it('a ticked row has a folder-name field pre-filled with the remote name, and the destination line follows it', async () => {
    await mount({ initialProfile: 'acme' })
    await click(rowCheck('acme_api'))
    await settle()
    expect(nameInput('acme_api').value).toBe('acme_api')
    await setInput(nameInput('acme_api'), 'api-main')
    await settle()
    expect(text()).toContain('Clones into /work/api-main')
    expect(text()).toContain('differs from remote')
  })

  it('passes the typed name as the folder argument, per repository', async () => {
    const spy = vi.spyOn(Repos, 'Clone').mockImplementation((async (req: { repoName: string }) => ok(req.repoName)) as unknown as typeof Repos.Clone)
    await mount({ initialProfile: 'acme' })
    await click(rowCheck('acme_api'))
    await click(rowCheck('acme_auth'))
    await setInput(nameInput('acme_api'), 'api-main')
    await settle()
    await click(cloneBtn())
    await settle()
    const calls = spy.mock.calls.map((c) => c[0] as { repoName: string; folderName: string })
    expect(calls.find((c) => c.repoName === 'acme_api')?.folderName).toBe('api-main')
    expect(calls.find((c) => c.repoName === 'acme_auth')?.folderName).toBe('')
  })

  it('shows inline errors and blocks the button before anything is cloned', async () => {
    const spy = spyClone(async (n) => ok(n))
    await mount({ initialProfile: 'acme' })
    await click(rowCheck('acme_api'))
    await click(rowCheck('acme_auth'))
    await settle()
    await setInput(nameInput('acme_api'), 'a/b')
    expect(text()).toContain('The name cannot contain / or \\')
    expect(startBtnDisabled()).toBe(true)
    await setInput(nameInput('acme_api'), 'x')
    await setInput(nameInput('acme_auth'), 'x')
    expect(text()).toContain('Two repositories cannot use the same folder name')
    expect(startBtnDisabled()).toBe(true)
    await setInput(nameInput('acme_auth'), 'CON')
    expect(text()).toContain('reserved Windows name')
    await setInput(nameInput('acme_auth'), 'y')
    await settle()
    expect(startBtnDisabled()).toBe(false)
    await setInput(nameInput('acme_api'), 'a/b')
    expect(startBtnDisabled()).toBe(true)
    await click(cloneBtn())
    expect(spy).not.toHaveBeenCalled()
  })

  it('treats names that differ only by case as the same folder', async () => {
    await mount({ initialProfile: 'acme' })
    await click(rowCheck('acme_api'))
    await click(rowCheck('acme_auth'))
    await settle()
    await setInput(nameInput('acme_api'), 'Shared')
    await setInput(nameInput('acme_auth'), 'shared')
    expect(text()).toContain('Two repositories cannot use the same folder name')
    expect(startBtnDisabled()).toBe(true)
  })

  it('shows the server\'s refusal of a folder name inline, before cloning', async () => {
    const spy = spyClone(async (n) => ok(n))
    vi.spyOn(Repos, 'PreviewClone').mockRejectedValue({ code: ErrorCode.ErrInvalidInput, message: 'bad', params: { field: 'folderName', reason: 'too-long' } })
    await mount({ initialProfile: 'acme' })
    await click(rowCheck('acme_api'))
    await settle()
    await settle()
    expect(text()).toContain('The name is too long')
    expect(startBtnDisabled()).toBe(true)
    expect(spy).not.toHaveBeenCalled()
  })

  it('single select: a taken name is an inline error and blocks the button', async () => {
    await mount({ initialProfile: 'acme', defaultParent: '/Users/alex/Documents/code' })
    await click(rowCheck('acme_changelog'))
    await settle()
    await setInput(nameInput('acme_changelog'), 'acme_api')
    await settle()
    expect(text()).toContain('This folder already exists')
    expect(nameInput('acme_changelog').getAttribute('aria-invalid')).toBe('true')
    expect(startBtnDisabled()).toBe(true)
  })

  it('multi select: a typed name that exists is an inline error and blocks the button', async () => {
    await mount({ initialProfile: 'acme', defaultParent: '/Users/alex/Documents/code' })
    await click(rowCheck('acme_handbook'))
    await click(rowCheck('acme_usage'))
    await settle()
    await setInput(nameInput('acme_handbook'), 'acme_api')
    await settle()
    expect(document.getElementById('clone-name-err-acme_handbook')?.textContent).toBe('This folder already exists')
    expect(nameInput('acme_handbook').getAttribute('aria-invalid')).toBe('true')
    expect(startBtnDisabled()).toBe(true)
  })

  it('multi select: renaming a taken row to a free name shows no stale error before the preview returns', async () => {
    await mount({ initialProfile: 'acme', defaultParent: '/Users/alex/Documents/code' })
    await click(rowCheck('acme_handbook'))
    await click(rowCheck('acme_usage'))
    await settle()
    const err = () => document.getElementById('clone-name-err-acme_handbook')?.textContent ?? ''
    await setInput(nameInput('acme_handbook'), 'acme_handbook-2')
    expect(err()).toBe('')
    expect(nameInput('acme_handbook').getAttribute('aria-invalid')).not.toBe('true')
    await settle()
    expect(err()).toBe('')
    expect(nameInput('acme_handbook').getAttribute('aria-invalid')).not.toBe('true')
  })

  it('single select: the command block stays while a new name is being previewed', async () => {
    await mount({ initialProfile: 'acme', defaultParent: '/Users/alex/Documents/code' })
    await click(rowCheck('acme_auth'))
    await settle()
    expect(text()).toContain('Command to run')
    await setInput(nameInput('acme_auth'), 'auth-2')
    expect(text()).toContain('Command to run')
    await settle()
    expect(text()).toContain('Command to run')
  })
})

describe('folderNameReason', () => {
  const cases: [string, string][] = [
    ['api-main', ''], ['acme_api', ''], ['a.b', ''], ['.hidden', ''], ['con-sole', ''], ['console', ''], ['nulled', ''],
    ['COM0', ''], ['COM10', ''], ['LPT0', ''], ['com', ''], ['lpt', ''], ['données', ''], ['仓库', ''], ['a b', ''],
    ['', 'empty'], ['   ', 'empty'], ['.', 'dots'], ['..', 'dots'],
    ['a/b', 'separator'], ['a\\b', 'separator'], ['/abs', 'separator'], ['../x', 'separator'],
    ['CON', 'reserved'], ['con', 'reserved'], ['Prn', 'reserved'], ['AUX', 'reserved'], ['NUL', 'reserved'], ['COM1', 'reserved'],
    ['com9', 'reserved'], ['LPT1', 'reserved'], ['lpt9', 'reserved'], ['CON.txt', 'reserved'], ['nul.tar.gz', 'reserved'],
    ['com1.old', 'reserved'], ['COM\u00b9', 'reserved'], ['LPT\u00b2', 'reserved'], ['CONIN$', 'reserved'],
    ['aux.', 'trailing'], ['api.', 'trailing'], ['api ', 'trailing'], ['api. ', 'trailing'], ['x..', 'trailing'],
    ['a:b', 'badChar'], ['a*b', 'badChar'], ['a?b', 'badChar'], ['a|b', 'badChar'], ['a<b', 'badChar'], ['a>b', 'badChar'],
    ['a"b', 'badChar'], ['a\x00b', 'badChar'], ['a\nb', 'badChar'], ['a\x7fb', 'badChar'],
    ['a'.repeat(255), ''], ['a'.repeat(256), 'tooLong'],
    ['\u4ed3'.repeat(85), ''], ['\u4ed3'.repeat(86), 'tooLong'], ['\u00e9'.repeat(127) + 'a', ''], ['\u00e9'.repeat(128), 'tooLong'],
  ]
  it.each(cases)('%j -> %j', (name, reason) => {
    expect(folderNameReason(name)).toBe(reason)
  })
})
