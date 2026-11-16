import { act, createElement } from 'react'
import type { ReactElement } from 'react'
import { createRoot } from 'react-dom/client'
import type { Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { Identity, Repos } from '../../api'
import { RepoAction } from '../../api/models'
import { mkOtherRepo } from '../../api/mock/repos'
import { getWorld, resetWorld } from '../../api/mock/world'
import { I18nProvider } from '../../i18n'
import { ActionsProvider } from '../../shell/AppActions'
import { AccountsProvider } from '../../state/accounts'
import { ReposProvider } from '../../state/repos'
import { ProjectsPage } from './ProjectsPage'

;(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true

let host: HTMLDivElement
let root: Root

// The mock API answers after 120 to 2500 ms; every timer runs 20x faster so the file takes seconds.
const TIME_SCALE = 20
const realSetTimeout = globalThis.setTimeout.bind(globalThis)

function ui(): ReactElement {
  return createElement(I18nProvider, { lang: 'en' }, createElement(AccountsProvider, null, createElement(ReposProvider, null, createElement(ActionsProvider, null, createElement(ProjectsPage)))))
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
const settle = (ms = 80) => act(async () => new Promise((r) => realSetTimeout(r, ms)))
async function mount() {
  await act(async () => {
    root.render(ui())
  })
  await waitFor(() => !!document.querySelector('[data-repo-id]'))
  await settle(150)
}

const row = (name: string) => Array.from(document.querySelectorAll<HTMLElement>('[role="row"][data-repo-id]')).find((r) => r.getAttribute('aria-label') === name) as HTMLElement
const section = (id: string) => document.querySelector<HTMLElement>(`[data-section="${id}"]`) as HTMLElement
const rowsIn = (id: string) => Array.from(section(id)?.querySelectorAll<HTMLElement>('[role="row"][data-repo-id]') ?? []).map((r) => r.getAttribute('aria-label'))
const live = () => document.querySelector('[role="status"][aria-live="polite"][data-live]')?.textContent ?? ''
const click = (el: Element | null | undefined) => act(async () => (el as HTMLElement | undefined)?.click())
const buttonByText = (re: RegExp, scope: ParentNode = document) => Array.from(scope.querySelectorAll<HTMLElement>('button, [role="menuitem"]')).find((b) => re.test(b.textContent ?? '') || re.test(b.getAttribute('aria-label') ?? '')) as HTMLElement | undefined
async function key(el: HTMLElement, k: string) {
  await act(async () => {
    el.dispatchEvent(new KeyboardEvent('keydown', { key: k, bubbles: true, cancelable: true }))
  })
}
const tabbable = (id: string) => Array.from(section(id).querySelectorAll<HTMLElement>('[role="row"][data-repo-id]')).filter((r) => r.tabIndex === 0).length

beforeEach(() => {
  const scaled = ((fn: TimerHandler, ms?: number, ...args: unknown[]) => realSetTimeout(fn as () => void, ms ? Math.round(ms / TIME_SCALE) : ms, ...args)) as unknown as typeof setTimeout
  vi.spyOn(globalThis, 'setTimeout').mockImplementation(scaled)
  if (window.setTimeout !== globalThis.setTimeout) vi.spyOn(window, 'setTimeout').mockImplementation(scaled)
  resetWorld('done')
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

describe('Projects page', () => {
  it('draws the projects of the done scenario, Ungrouped last, with the moved-folder question', async () => {
    await mount()
    const titles = Array.from(document.querySelectorAll('.proj .portal__title')).map((e) => e.textContent)
    expect(titles).toEqual(['acme', 'globex', 'umbrella', 'Ungrouped'])
    expect(rowsIn('p_umbrella')).toEqual(['initech_infra', 'initech_site'])
    expect(document.body.textContent).toContain('Is this initech_dashboard, moved here?')
    // The project's author identity comes from the identity service.
    await waitFor(() => /Alex <alex@acme\.example\.com>/.test(section('p_acme').textContent ?? ''))
  })

  it('answering "Yes" gives the folder its old project back', async () => {
    await mount()
    expect(rowsIn('')).toEqual(['dashboard-main'])
    await click(buttonByText(/Yes, keep its records/))
    await waitFor(() => rowsIn('p_umbrella').includes('dashboard-main'))
    expect(rowsIn('')).toEqual([]) // Ungrouped is empty, so its section is gone
    expect(document.body.textContent).not.toContain('moved here?')
  })

  it('has one tab stop per table and the arrow keys move between rows (roving grid)', async () => {
    await mount()
    expect(tabbable('p_acme')).toBe(1)
    expect(tabbable('p_umbrella')).toBe(1)
    const first = row('acme_agent_portal')
    expect(first.tabIndex).toBe(0)
    first.focus()
    await key(first, 'ArrowDown')
    expect(document.activeElement).toBe(row('acme_api'))
    await settle(20)
    expect(row('acme_api').tabIndex).toBe(0)
    expect(row('acme_agent_portal').tabIndex).toBe(-1)
    // Down at the end of a table goes on to the next table.
    const last = row('acme_search')
    last.focus()
    await key(last, 'ArrowDown')
    expect(document.activeElement).toBe(row('globex_backend'))
    await key(document.activeElement as HTMLElement, 'End')
    expect(document.activeElement).toBe(row('globex_backend'))
    // Right enters the row's controls, Left leaves them, and the controls are out of the tab order until the row is active.
    const r = row('acme_api')
    r.focus()
    await key(r, 'ArrowRight')
    expect(r.contains(document.activeElement) && document.activeElement !== r).toBe(true)
    await key(document.activeElement as HTMLElement, 'Escape')
    expect(document.activeElement).toBe(r)
    const other = row('acme_auth').querySelector<HTMLElement>('[data-grid-control]')
    expect(other?.tabIndex).toBe(-1)
  })

  it('Enter opens the repository and marks it opened; Space selects the row', async () => {
    const mark = vi.spyOn(Repos, 'MarkOpened')
    vi.spyOn(Repos, 'OpenFolder').mockResolvedValue(undefined as never)
    await mount()
    const r = row('acme_api')
    r.focus()
    await key(r, ' ')
    expect(r.getAttribute('aria-selected')).toBe('true')
    expect(document.querySelector('.bulkbar')?.textContent).toContain('1 selected')
    await key(r, ' ')
    expect(r.getAttribute('aria-selected')).toBe('false')
    await key(r, 'Enter')
    expect(mark).toHaveBeenCalledWith(r.getAttribute('data-repo-id'))
  })

  it('M opens the move menu, a pick moves the row, announces it and returns focus to the row', async () => {
    await mount()
    const r = row('initech_site')
    r.focus()
    await key(r, 'm')
    expect(live()).toBe('Picked up initech_site')
    const menu = document.querySelector('[role="menu"]') as HTMLElement
    expect(menu).toBeTruthy()
    const items = Array.from(menu.querySelectorAll('[role="menuitem"]')).map((e) => e.textContent)
    expect(items).toEqual(['acme', 'globex', 'umbrella', 'Ungrouped', 'New project...'])
    // The project it is in already is marked and cannot be picked.
    expect((buttonByText(/^umbrella$/, menu) as HTMLButtonElement).disabled).toBe(true)
    await click(buttonByText(/^globex$/, menu))
    await waitFor(() => rowsIn('p_globex').includes('initech_site'))
    expect(rowsIn('p_umbrella')).toEqual(['initech_infra'])
    await waitFor(() => live() === 'Moved to globex')
    await waitFor(() => document.activeElement === row('initech_site'))
    expect(document.body.textContent).toContain('Moved initech_site to globex')
  })

  it('Escape closes the move menu without moving', async () => {
    await mount()
    const r = row('initech_site')
    r.focus()
    await key(r, 'm')
    const menu = document.querySelector('[role="menu"]') as HTMLElement
    await act(async () => {
      menu.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
      window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    })
    expect(document.querySelector('[role="menu"]')).toBeNull()
    expect(rowsIn('p_umbrella')).toContain('initech_site')
  })

  it('a move can be undone from its toast', async () => {
    await mount()
    const r = row('initech_site')
    r.focus()
    await key(r, 'm')
    await click(buttonByText(/^globex$/, document.querySelector('[role="menu"]') as HTMLElement))
    await waitFor(() => rowsIn('p_globex').includes('initech_site'))
    await waitFor(() => !!buttonByText(/^Undo$/))
    await click(buttonByText(/^Undo$/))
    await waitFor(() => rowsIn('p_umbrella').includes('initech_site'))
  })

  it('dragging a row by its grip onto a project moves it; the target gets the ring and the live region speaks', async () => {
    await mount()
    const r = row('initech_site')
    const grip = r.querySelector('.grip') as HTMLElement
    const target = section('p_globex')
    const head = target.querySelector('.proj__head') as HTMLElement
    const elementFromPoint = vi.fn(() => head)
    ;(document as unknown as { elementFromPoint: unknown }).elementFromPoint = elementFromPoint
    const pe = (type: string, x: number, y: number) => new MouseEvent(type, { bubbles: true, cancelable: true, button: 0, clientX: x, clientY: y })
    await act(async () => {
      grip.dispatchEvent(pe('pointerdown', 10, 10))
    })
    await act(async () => {
      window.dispatchEvent(pe('pointermove', 12, 11))
    }) // under the 4px threshold: still a click
    expect(document.querySelector('.drag-ghost')).toBeNull()
    await act(async () => {
      window.dispatchEvent(pe('pointermove', 60, 80))
    })
    expect(document.querySelector('.drag-ghost')?.textContent).toContain('initech_site')
    expect(target.classList.contains('is-drop')).toBe(true)
    expect(live()).toBe('Over globex')
    await act(async () => {
      window.dispatchEvent(pe('pointerup', 60, 80))
    })
    await waitFor(() => rowsIn('p_globex').includes('initech_site'))
    expect(document.querySelector('.drag-ghost')).toBeNull()
    await waitFor(() => live() === 'Moved to globex')
  })

  it('Escape during a drag cancels it', async () => {
    await mount()
    const grip = row('initech_site').querySelector('.grip') as HTMLElement
    ;(document as unknown as { elementFromPoint: unknown }).elementFromPoint = () => section('p_globex').querySelector('.proj__head')
    const pe = (type: string, x: number, y: number) => new MouseEvent(type, { bubbles: true, cancelable: true, button: 0, clientX: x, clientY: y })
    await act(async () => {
      grip.dispatchEvent(pe('pointerdown', 10, 10))
      window.dispatchEvent(pe('pointermove', 60, 80))
    })
    await act(async () => {
      window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true }))
    })
    expect(document.querySelector('.drag-ghost')).toBeNull()
    expect(live()).toBe('Move cancelled')
    expect(rowsIn('p_umbrella')).toContain('initech_site')
  })

  it('selecting rows and using the bulk bar moves them all', async () => {
    await mount()
    for (const n of ['initech_infra', 'initech_site']) await click(row(n).querySelector('input[type="checkbox"]'))
    expect(document.querySelector('.bulkbar')?.textContent).toContain('2 selected')
    await click(buttonByText(/Move to project/, document.querySelector('.bulkbar') as HTMLElement))
    await click(buttonByText(/^Ungrouped$/, document.querySelector('[role="menu"]') as HTMLElement))
    await waitFor(() => rowsIn('').includes('initech_site') && rowsIn('').includes('initech_infra'))
    await waitFor(() => (document.body.textContent ?? '').includes('Moved 2 repositories to Ungrouped'))
  })

  it('hiding a row moves it to the hidden list and Undo brings it back', async () => {
    await mount()
    const r = row('initech_infra')
    await click(buttonByText(/^More$/, r))
    await click(buttonByText(/^Hide$/, document.querySelector('[role="menu"]') as HTMLElement))
    await waitFor(() => !row('initech_infra'))
    await waitFor(() => live() === 'Hid initech_infra')
    expect(document.body.textContent).toContain('Hid initech_infra')
    // Focus goes to a neighbouring row, not to the page.
    await waitFor(() => !!document.activeElement?.closest('[data-repo-id]'))
    await waitFor(() => !!buttonByText(/^Undo$/))
    await click(buttonByText(/^Undo$/))
    await waitFor(() => !!row('initech_infra'))
  })

  it('collapsing a project is remembered by the backend and hides its rows from the tab order', async () => {
    await mount()
    const toggle = section('p_globex').querySelector('.portal__toggle') as HTMLElement
    expect(toggle.getAttribute('aria-expanded')).toBe('true')
    await click(toggle)
    expect(toggle.getAttribute('aria-expanded')).toBe('false')
    expect(section('p_globex').classList.contains('is-collapsed')).toBe(true)
    expect(section('p_globex').querySelector('.proj__body')?.hasAttribute('inert')).toBe(true)
    await settle(150)
    expect(getWorld().projects.find((p) => p.id === 'p_globex')?.collapsed).toBe(true)
  })

  it('renaming: Enter saves, Esc cancels', async () => {
    await mount()
    await click(buttonByText(/Menu for globex/))
    await click(buttonByText(/^Rename$/, document.querySelector('[role="menu"]') as HTMLElement))
    const input = section('p_globex').querySelector('input.proj__rename') as HTMLInputElement
    expect(input).toBeTruthy()
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')?.set?.call(input, 'globex-eu')
      input.dispatchEvent(new Event('input', { bubbles: true }))
    })
    await key(input, 'Enter')
    await waitFor(() => section('p_globex').querySelector('.portal__title')?.textContent === 'globex-eu')
    await waitFor(() => live() === 'Renamed to globex-eu')
    // Esc leaves the name alone.
    await click(buttonByText(/Menu for globex-eu/))
    await click(buttonByText(/^Rename$/, document.querySelector('[role="menu"]') as HTMLElement))
    const again = section('p_globex').querySelector('input.proj__rename') as HTMLInputElement
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')?.set?.call(again, 'zzz')
      again.dispatchEvent(new Event('input', { bubbles: true }))
    })
    await key(again, 'Escape')
    expect(section('p_globex').querySelector('input.proj__rename')).toBeNull()
    expect(section('p_globex').querySelector('.portal__title')?.textContent).toBe('globex-eu')
  })

  it('removing a project asks first, then sends its repositories back to Ungrouped', async () => {
    await mount()
    await click(buttonByText(/Menu for umbrella/))
    await click(buttonByText(/^Remove project$/, document.querySelector('[role="menu"]') as HTMLElement))
    await waitFor(() => !!document.querySelector('.modal'))
    expect(document.querySelector('.modal')?.textContent).toContain('Remove project umbrella?')
    expect(document.querySelector('.modal')?.textContent).toContain('Its 2 repositories go back to Ungrouped. Nothing on disk is touched.')
    await click(buttonByText(/^Remove project$/, document.querySelector('.modal') as HTMLElement))
    await waitFor(() => !section('p_umbrella'))
    expect(rowsIn('')).toEqual(expect.arrayContaining(['initech_infra', 'initech_site']))
    expect(getWorld().projects.map((p) => p.name)).toEqual(['acme', 'globex'])
  })

  it('creating a project from the dialog ticks the ungrouped repositories that start with its name', async () => {
    await mount()
    await click(buttonByText(/^New project$/, document.querySelector('.page__actions') as HTMLElement))
    await waitFor(() => !!document.querySelector('.modal'))
    const input = document.querySelector('.modal input.input') as HTMLInputElement
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')?.set?.call(input, 'dashboard')
      input.dispatchEvent(new Event('input', { bubbles: true }))
    })
    const ticked = Array.from(document.querySelectorAll<HTMLInputElement>('.modal .proj-check__row input')).filter((i) => i.checked)
    expect(ticked.map((i) => i.getAttribute('aria-label'))).toEqual(['Select dashboard-main'])
    await click(buttonByText(/^Create project$/, document.querySelector('.modal') as HTMLElement))
    await waitFor(() => !!Array.from(document.querySelectorAll('.proj .portal__title')).find((e) => e.textContent === 'dashboard'))
    await waitFor(() => rowsIn(getWorld().projects.find((p) => p.name === 'dashboard')?.id ?? 'x').includes('dashboard-main'))
  })

  it('shows the newly found banner after a scan and the chooser pre-ticks the recent repositories', async () => {
    await mount()
    expect(document.querySelector('[aria-label="Newly found repositories"]')).toBeNull()
    await click(document.querySelector('.page__actions button[aria-label="Rescan"]'))
    await waitFor(() => !!document.querySelector('[aria-label="Newly found repositories"]'), 8000)
    expect(document.querySelector('[aria-label="Newly found repositories"]')?.textContent).toContain('Found 4 more repositories')
    // A found row is not in the list.
    expect(row('dotfiles')).toBeUndefined()
    await click(buttonByText(/Choose which to show/))
    await waitFor(() => !!document.querySelector('.modal .proj-check'))
    const boxes = Array.from(document.querySelectorAll<HTMLInputElement>('.modal .proj-check__row input'))
    expect(boxes).toHaveLength(4)
    const found = getWorld().repos.filter((r) => r.pending)
    const recent = found.filter((r) => Date.now() - Date.parse(r.lastCommitAt ?? '') <= 90 * 24 * 3600 * 1000).length
    expect(boxes.filter((b) => b.checked)).toHaveLength(recent)
    await click(buttonByText(/^Show \d+ repositor/, document.querySelector('.modal') as HTMLElement))
    await waitFor(() => !document.querySelector('[aria-label="Newly found repositories"]'))
    expect(getWorld().repos.filter((r) => r.pending)).toHaveLength(0)
    expect(getWorld().repos.filter((r) => r.hidden).length).toBe(1 + (4 - recent)) // the one hidden before, plus those left unticked
  })

  it('"Hide them all" hides the found repositories', async () => {
    await mount()
    await click(document.querySelector('.page__actions button[aria-label="Rescan"]'))
    await waitFor(() => !!document.querySelector('[aria-label="Newly found repositories"]'), 8000)
    await click(buttonByText(/Hide them all/))
    await waitFor(() => !document.querySelector('[aria-label="Newly found repositories"]'))
    expect(getWorld().repos.filter((r) => r.hidden)).toHaveLength(5)
  })

  it('the filter menu lists the hidden repositories, dimmed, with Unhide', async () => {
    await mount()
    await click(buttonByText(/All providers/))
    await click(buttonByText(/^Hidden 1$/, document.querySelector('[role="menu"]') as HTMLElement))
    await waitFor(() => !!section('__hidden__'))
    expect(rowsIn('__hidden__')).toEqual(['acme_handbook'])
    expect(row('acme_handbook').classList.contains('is-dim')).toBe(true)
    await click(buttonByText(/^Unhide$/, row('acme_handbook')))
    await waitFor(() => !section('__hidden__'))
    expect(rowsIn('p_acme')).not.toContain('acme_handbook') // it had no project; it is listed under Ungrouped
    expect(rowsIn('')).toContain('acme_handbook')
  })

  it('search keeps matching rows under their project and hides the other projects', async () => {
    await mount()
    const input = document.querySelector('.toolbar input.input') as HTMLInputElement
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')?.set?.call(input, 'initech_site')
      input.dispatchEvent(new Event('input', { bubbles: true }))
    })
    expect(Array.from(document.querySelectorAll('.proj .portal__title')).map((e) => e.textContent)).toEqual(['umbrella'])
    expect(rowsIn('p_umbrella')).toEqual(['initech_site'])
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')?.set?.call(input, 'no-such-thing')
      input.dispatchEvent(new Event('input', { bubbles: true }))
    })
    expect(document.body.textContent).toContain('No matching repositories')
    await click(buttonByText(/^Clear search$/))
    expect(Array.from(document.querySelectorAll('.proj .portal__title')).length).toBeGreaterThan(1)
  })

  it('a "projects.json not writable" warning from the backend is shown', async () => {
    getWorld().repoWarnings = [{ code: 'projects-not-writable', count: 3, file: 'projects.json' } as never]
    await mount()
    expect(document.body.textContent).toContain('Project information could not be saved')
    expect(document.body.textContent).toContain('3 folders are not listed')
  })
})

describe('Projects page: review round 1', () => {
  const setValue = (input: HTMLInputElement, v: string) =>
    act(async () => {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')?.set?.call(input, v)
      input.dispatchEvent(new Event('input', { bubbles: true }))
    })
  const setPlan = (repoId: string, name: string) => ({
    repoId,
    name,
    path: `/x/${name}`,
    action: RepoAction.RepoActionSet,
    before: null,
    after: { name: 'Alex', email: 'alex@acme.example.com' },
  })

  it('M > New project... on a grouped row moves it into the new project', async () => {
    await mount()
    const r = row('initech_site')
    r.focus()
    await key(r, 'm')
    await click(buttonByText(/^New project/, document.querySelector('[role="menu"]') as HTMLElement))
    await waitFor(() => !!document.querySelector('.modal'))
    const ticked = Array.from(document.querySelectorAll<HTMLInputElement>('.modal .proj-check__row input')).filter((i) => i.checked)
    expect(ticked.map((i) => i.getAttribute('aria-label'))).toEqual(['Select initech_site'])
    await setValue(document.querySelector('.modal input.input') as HTMLInputElement, 'split')
    await click(buttonByText(/^Create project$/, document.querySelector('.modal') as HTMLElement))
    await waitFor(() => {
      const p = getWorld().projects.find((x) => x.name === 'split')
      return !!p && rowsIn(p.id).includes('initech_site')
    })
    expect(rowsIn('p_umbrella')).toEqual(['initech_infra'])
  })

  it('Esc in the rename box leaves focus on the project header toggle', async () => {
    await mount()
    await click(buttonByText(/Menu for globex/))
    await click(buttonByText(/^Rename$/, document.querySelector('[role="menu"]') as HTMLElement))
    const input = section('p_globex').querySelector('input.proj__rename') as HTMLInputElement
    await key(input, 'Escape')
    await waitFor(() => document.activeElement === section('p_globex').querySelector('.portal__toggle'))
  })

  it('the bind popover opens from the control that was pressed and gives focus back to it', async () => {
    await mount()
    const chip = document.querySelector<HTMLElement>('.proj-chip')
    expect(chip).toBeTruthy()
    chip?.focus()
    await click(chip)
    await waitFor(() => !!document.querySelector('.popover'))
    await act(async () => {
      window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    })
    await waitFor(() => !document.querySelector('.popover'))
    await waitFor(() => document.activeElement === chip)
  })

  it('ArrowDown on a menu trigger opens the menu and does not move to the next row', async () => {
    await mount()
    const more = buttonByText(/^More$/, row('acme_api')) as HTMLElement
    more.focus()
    await key(more, 'ArrowDown')
    const at = document.activeElement?.closest('[data-repo-id]')
    expect(!at || at.getAttribute('aria-label') === 'acme_api').toBe(true)
  })

  it('every button in the status and actions cells is a grid control (one tab stop per table)', async () => {
    await mount()
    const stray = Array.from(document.querySelectorAll<HTMLElement>('[role="row"][data-repo-id] button')).filter((b) => !b.hasAttribute('data-grid-control') && !b.closest('[data-no-open]') && b.tabIndex === 0)
    expect(stray.map((b) => b.textContent)).toEqual([])
  })

  it('a session that ends within 15 minutes shows "Sign-in ends"', async () => {
    const w = getWorld()
    for (const m of Object.values(w.portalModels)) m.loginAt = Date.now() - (m.sessionMinutes - 9) * 60_000
    await mount()
    expect(document.body.textContent).toContain('Sign-in ends: 9 min left')
    expect(document.body.textContent).toContain('Sign in again')
  })

  it('a move that changes author identities asks first, applies with the plan hash, and Undo calls Identity.Undo', async () => {
    const prev = vi.spyOn(Identity, 'PreviewMove').mockImplementation((async (repoId: string, to: string) => ({ repoId, toProjectId: to, plan: setPlan(repoId, 'initech_site'), planHash: 'h1' })) as never)
    const apply = vi.spyOn(Identity, 'ApplyMove').mockResolvedValue({ outcomes: [{ repoId: 'x', name: 'initech_site', action: RepoAction.RepoActionSet, ok: true }], undoId: 'u1', recordUpdated: true } as never)
    const undo = vi.spyOn(Identity, 'Undo').mockResolvedValue(undefined as never)
    await mount()
    const id = row('initech_site').getAttribute('data-repo-id')
    const r = row('initech_site')
    r.focus()
    await key(r, 'm')
    await click(buttonByText(/^globex$/, document.querySelector('[role="menu"]') as HTMLElement))
    await waitFor(() => !!document.querySelector('.modal'))
    expect(prev).toHaveBeenCalledWith(id, 'p_globex')
    expect(document.querySelector('.modal')?.textContent).toContain('The author identity of this repository will change')
    expect(apply).not.toHaveBeenCalled()
    await click(buttonByText(/^Move$/, document.querySelector('.modal') as HTMLElement))
    await waitFor(() => apply.mock.calls.length === 1)
    expect(apply).toHaveBeenCalledWith(id, 'p_globex', 'h1')
    await waitFor(() => !!buttonByText(/^Undo$/))
    await click(buttonByText(/^Undo$/))
    await waitFor(() => undo.mock.calls.length === 1)
    expect(undo).toHaveBeenCalledWith('u1')
  })

  it('removing a project lists the repositories that fall back to the global identity even when the overview failed', async () => {
    vi.spyOn(Identity, 'GetOverview').mockRejectedValue(new Error('no overview'))
    const prev = vi.spyOn(Identity, 'PreviewMove').mockImplementation((async (repoId: string) => ({ repoId, plan: setPlan(repoId, 'x'), planHash: `h-${repoId}` })) as never)
    const apply = vi.spyOn(Identity, 'ApplyMove').mockResolvedValue({ outcomes: [], undoId: '', recordUpdated: true } as never)
    const remove = vi.spyOn(Repos, 'RemoveProject')
    await mount()
    await click(buttonByText(/Menu for umbrella/))
    await click(buttonByText(/^Remove project$/, document.querySelector('[role="menu"]') as HTMLElement))
    await waitFor(() => !!document.querySelector('.modal'))
    expect(prev).toHaveBeenCalledTimes(2)
    expect(document.querySelector('.modal')?.textContent).toContain('fall back to the global identity')
    expect(apply).not.toHaveBeenCalled()
    await click(buttonByText(/^Remove project$/, document.querySelector('.modal') as HTMLElement))
    await waitFor(() => remove.mock.calls.length === 1)
    expect(apply).toHaveBeenCalledTimes(2)
    for (const c of apply.mock.calls) expect(c[1]).toBe('')
    expect(Math.max(...apply.mock.invocationCallOrder)).toBeLessThan(remove.mock.invocationCallOrder[0])
  })

  it('a new project with an author identity shows the plan first and warns when it was not saved', async () => {
    const preview = vi.spyOn(Identity, 'PreviewProject').mockResolvedValue({ projectId: 'p', name: 'idp', repos: [setPlan('r1', 'dashboard-main')], writes: 1, keepsOwn: 0, planHash: 'pp1' } as never)
    const apply = vi.spyOn(Identity, 'ApplyProject').mockResolvedValue({ outcomes: [{ repoId: 'r1', name: 'dashboard-main', action: RepoAction.RepoActionError, ok: false }], undoId: '', recordUpdated: false } as never)
    await mount()
    await click(buttonByText(/^New project$/, document.querySelector('.page__actions') as HTMLElement))
    await waitFor(() => !!document.querySelector('.modal'))
    await setValue(document.querySelector('.modal input.input') as HTMLInputElement, 'idp')
    await click(document.querySelector('.proj-new__disclose'))
    const inputs = Array.from(document.querySelectorAll<HTMLInputElement>('.modal .proj-new__fields input'))
    await setValue(inputs[0], 'Alex')
    await setValue(inputs[1], 'alex@acme.example.com')
    await click(buttonByText(/^Create project$/, document.querySelector('.modal') as HTMLElement))
    await waitFor(() => preview.mock.calls.length === 1)
    await waitFor(() => (document.body.textContent ?? '').includes('Set the author identity of idp?'))
    expect(document.body.textContent).toContain('dashboard-main')
    expect(apply).not.toHaveBeenCalled()
    await click(buttonByText(/^Set$/))
    await waitFor(() => apply.mock.calls.length === 1)
    expect(apply.mock.calls[0][1]).toBe('pp1')
    await waitFor(() => (document.body.textContent ?? '').includes('was created, but its author identity was not saved'))
    expect(document.body.textContent).toContain('dashboard-main')
  })
})

describe('Projects page motion', () => {
  const stubRects = () => {
    // Every keyed row gets a top that follows its position in the document, so a move changes the rows' places.
    vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function (this: HTMLElement) {
      const all = Array.from(document.querySelectorAll('[data-flip-key]'))
      const i = all.indexOf(this)
      return { top: i * 50, left: 0, right: 100, bottom: i * 50 + 50, width: 100, height: 50, x: 0, y: i * 50, toJSON: () => ({}) } as DOMRect
    })
  }

  it('the row travels to its new place (FLIP) in a list of normal size', async () => {
    const animate = vi.fn(() => ({}))
    ;(Element.prototype as unknown as { animate: unknown }).animate = animate
    await mount()
    stubRects()
    const r = row('initech_site')
    r.focus()
    await key(r, 'm')
    await click(buttonByText(/^acme$/, document.querySelector('[role="menu"]') as HTMLElement))
    await waitFor(() => rowsIn('p_acme').includes('initech_site'))
    expect(animate).toHaveBeenCalled()
    delete (Element.prototype as unknown as { animate?: unknown }).animate
  })

  it('skips the motion above 200 rows', async () => {
    const w = getWorld()
    for (let i = 0; i < 205; i++) w.repos.push(mkOtherRepo(`/Users/alex/Documents/code/bulk_${String(i).padStart(3, '0')}`, `https://github.com/someone/bulk_${i}.git`))
    const animate = vi.fn(() => ({}))
    ;(Element.prototype as unknown as { animate: unknown }).animate = animate
    await mount()
    stubRects()
    const r = row('initech_site')
    r.focus()
    await key(r, 'm')
    await click(buttonByText(/^acme$/, document.querySelector('[role="menu"]') as HTMLElement))
    await waitFor(() => rowsIn('p_acme').includes('initech_site'), 8000)
    expect(animate).not.toHaveBeenCalled()
    delete (Element.prototype as unknown as { animate?: unknown }).animate
  })
})
