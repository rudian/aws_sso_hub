import { act, createElement } from 'react'
import { createRoot } from 'react-dom/client'
import type { Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { repoIdOf, resetIdentityMock } from '../../api/mock/identity'
import { I18nProvider } from '../../i18n'
import { ActionsProvider } from '../../shell/AppActions'
import { IdentitySection } from './IdentitySection'

;(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true

let host: HTMLDivElement
let root: Root

const flush = (ms = 3000) =>
  act(async () => {
    await vi.advanceTimersByTimeAsync(ms)
  })
const q = (sel: string) => Array.from(document.querySelectorAll<HTMLElement>(sel))
const text = () => document.body.textContent ?? ''
const rowOf = (name: string) => q('.ident-table .row').find((r) => r.textContent?.includes(name))
async function setValue(el: HTMLInputElement, value: string) {
  await act(async () => {
    const set = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!
    set.call(el, value)
    el.dispatchEvent(new Event('input', { bubbles: true }))
  })
}

async function mount() {
  await act(async () => {
    root.render(createElement(I18nProvider, { lang: 'en' }, createElement(ActionsProvider, null, createElement(IdentitySection))))
  })
  await flush()
}

beforeEach(() => {
  vi.useFakeTimers()
  resetIdentityMock('page')
  host = document.createElement('div')
  document.body.appendChild(host)
  root = createRoot(host)
})
afterEach(() => {
  act(() => root.unmount())
  host.remove()
  document.body.innerHTML = ''
  vi.useRealTimers()
})

describe('IdentitySection', () => {
  it('lists projects and repositories with the source tag of what git reports', async () => {
    await mount()
    expect(rowOf('acme_api')?.textContent).toContain('From project')
    expect(rowOf('acme_merchant_portal_v4')?.textContent).toContain('Repository’s own')
    expect(rowOf('globex_backend')?.textContent).toContain('From global')
    expect(rowOf('globex')?.textContent).toContain('Follows global')
    expect(rowOf('acme')?.textContent).toContain('Project setting')
    expect(text()).toContain('2 repositories use it now')
  })

  it('warns about a terminal environment override and does not hide the setting rows', async () => {
    resetIdentityMock('env')
    await mount()
    expect(text()).toContain('Commits made in terminals and AI tasks use this instead of the setting above')
    expect(rowOf('acme_api')?.textContent).toContain('From a terminal environment variable')
  })

  it('the project sheet previews the repositories that change and keeps the ones with their own setting last', async () => {
    await mount()
    const set = q('.ident-table button').find((b) => b.getAttribute('aria-label') === 'Set the author identity of project acme')!
    await act(async () => set.click())
    await flush()
    const email = q('.sheet input[type="email"]')[0] as HTMLInputElement
    await setValue(email, 'alex@acme-new.example.com')
    await flush()
    expect(q('.sheet .field__label').some((l) => l.textContent === 'Saving changes 3 repositories')).toBe(true)
    const items = q('.sheet .affect li')
    expect(items).toHaveLength(4)
    expect(items[3]!.textContent).toContain('acme_merchant_portal_v4')
    expect(items[3]!.textContent).toContain('Keeps its own')
    expect(items[0]!.querySelector('.was')?.textContent).toBe('alex@acme.example.com')
    const save = q('.sheet__foot button').find((b) => b.textContent === 'Save and apply to 3 repositories')!
    expect(save.hasAttribute('disabled')).toBe(false)
    await act(async () => save.click())
    await flush()
    // the sheet plays its exit before it unmounts
    expect(q('.sheet').length).toBeGreaterThan(0)
    await flush(500)
    expect(q('.sheet')).toHaveLength(0)
    // the identity:changed event refetched the table
    expect(rowOf('acme_api')?.textContent).toContain('alex@acme-new.example.com')
    expect(rowOf('acme_merchant_portal_v4')?.textContent).toContain('alex.work@acme.example.com')
  })

  const pencil = (name: string) => q('.ident-table button').find((b) => b.getAttribute('aria-label') === `Edit the author identity of ${name}`)!
  const radios = (pop: Element) => Array.from(pop.querySelectorAll<HTMLInputElement>('input[type="radio"]'))

  it('offers the three options of the design, also while the project has its own identity', async () => {
    await mount()
    await act(async () => pencil('acme_api').click())
    await flush(10)
    let pop = document.querySelector('.ident-pop')!
    expect(radios(pop).map((r) => r.checked)).toEqual([true, false, false])
    expect(pop.textContent).toContain('Follow project acme')
    expect(pop.textContent).toContain('Follow global')
    expect(pop.textContent).toContain('This repository has its own')
    await act(async () => pencil('acme_api').click())
    await flush(10)
    // in the page scenario globex_backend has no marker: it follows its project, and focus is inside the popover
    await act(async () => pencil('globex_backend').click())
    await flush(10)
    pop = document.querySelector('.ident-pop')!
    expect(pop.textContent).toContain('Follow project globex')
    expect(radios(pop).map((r) => r.checked)).toEqual([true, false, false])
    expect(pop.contains(document.activeElement)).toBe(true)
  })

  it('"Follow global" writes the marker; reopening shows it checked with focus inside the popover', async () => {
    await mount()
    await act(async () => pencil('globex_backend').click())
    await flush(10)
    let pop = document.querySelector('.ident-pop')!
    await act(async () => radios(pop)[1]!.click())
    const save = Array.from(pop.querySelectorAll('button')).find((b) => b.textContent === 'Save')!
    await act(async () => save.click())
    await flush()
    expect(document.querySelector('.ident-pop')).toBeNull()
    await act(async () => pencil('globex_backend').click())
    await flush(10)
    pop = document.querySelector('.ident-pop')!
    expect(radios(pop).map((r) => r.checked)).toEqual([false, true, false])
    expect(pop.contains(document.activeElement)).toBe(true)
  })

  it('a repository whose own values were set outside the app can be made to follow its project', async () => {
    resetIdentityMock('default')
    await mount()
    await act(async () => pencil('acme_merchant_portal_v4').click())
    await flush(10)
    const pop = document.querySelector('.ident-pop')!
    expect(radios(pop).map((r) => r.checked)).toEqual([false, false, true])
    expect((pop.querySelector('input[type="email"]') as HTMLInputElement).value).toBe('a.lee@acme.example.com')
    const save = () => Array.from(pop.querySelectorAll('button')).find((b) => b.textContent === 'Save')!
    expect(save().hasAttribute('disabled')).toBe(true)
    await act(async () => radios(pop)[0]!.click())
    expect(save().hasAttribute('disabled')).toBe(false)
    await act(async () => save().click())
    await flush()
    expect(document.querySelector('.ident-pop')).toBeNull()
    expect(rowOf('acme_merchant_portal_v4')?.textContent).toContain('From project')
    expect(rowOf('acme_merchant_portal_v4')?.textContent).toContain('alex@acme.example.com')
  })

  it('the sheet offers the same link for a value set outside the app', async () => {
    resetIdentityMock('default')
    await mount()
    const set = q('.ident-table button').find((b) => b.getAttribute('aria-label') === 'Set the author identity of project acme')!
    await act(async () => set.click())
    await flush()
    const li = q('.sheet .affect li').find((l) => l.textContent?.includes('acme_merchant_portal_v4'))!
    const link = li.querySelector<HTMLElement>('button.link')!
    expect(li.textContent).toContain('Kept, set outside the app')
    await act(async () => link.click())
    await flush()
    expect(rowOf('acme_merchant_portal_v4')?.textContent).toContain('From project')
    // focus stays on that repository's row, not on <body>
    expect(document.activeElement?.getAttribute('data-repo')).toBe(repoIdOf('acme_merchant_portal_v4'))
  })

  it('shows the cannot-write state with the file and Retry when the repository config cannot be read', async () => {
    resetIdentityMock('readerr')
    await mount()
    expect(rowOf('acme_cs_portal')?.textContent).toContain('Cannot read this repository’s settings')
    expect(rowOf('acme_cs_portal')?.textContent).toContain('acme_cs_portal/.git/config')
    // the row's Retry sits outside the clipping .ident element
    const rowRetry = Array.from(rowOf('acme_cs_portal')!.querySelectorAll('button')).find((b) => b.textContent === 'Try again')!
    expect(rowRetry).toBeTruthy()
    expect(rowRetry.closest('.ident')).toBeNull()
    expect(rowOf('acme_cs_portal')!.querySelector('.ident')?.textContent).toContain('Cannot read')
    await act(async () => pencil('acme_cs_portal').click())
    await flush(10)
    const pop = document.querySelector('.ident-pop')!
    expect(pop.textContent).toContain('Cannot write this repository’s config')
    expect(pop.textContent).toContain('acme_cs_portal/.git/config')
    const retry = Array.from(pop.querySelectorAll('button')).find((b) => b.textContent === 'Try again')!
    // Retry reads again; the config is still broken, so the state stays and no generic toast appears
    await act(async () => retry.click())
    await flush()
    expect(document.querySelector('.ident-pop')?.textContent).toContain('acme_cs_portal/.git/config')
    expect(document.querySelector('.toast')).toBeNull()
  })

  it('shows the cannot-write state when saving fails with E_GIT_FAILED', async () => {
    resetIdentityMock('readerr')
    await mount()
    await act(async () => pencil('acme_api').click())
    await flush(10)
    const pop = document.querySelector('.ident-pop')!
    await act(async () => radios(pop)[1]!.click())
    // make the config of this repository unreadable after the table was read
    const { mockIdentityService } = await import('../../api/mock/identity')
    const spy = vi.spyOn(mockIdentityService, 'PreviewRepo').mockRejectedValue(Object.assign(new Error('x'), { cause: { code: 'E_GIT_FAILED', message: 'x', params: { file: '~/code/acme_api/.git/config' } } }))
    const save = Array.from(pop.querySelectorAll('button')).find((b) => b.textContent === 'Save')!
    await act(async () => save.click())
    await flush()
    spy.mockRestore()
    expect(document.querySelector('.ident-pop')?.textContent).toContain('~/code/acme_api/.git/config')
    expect(document.querySelector('.ident-pop')?.textContent).toContain('Try again')
  })

  it('after a failed Save the focus stays in the popover, on Try again (FU-16b)', async () => {
    resetIdentityMock('readerr')
    await mount()
    await act(async () => pencil('acme_api').click())
    await flush(10)
    const pop = document.querySelector('.ident-pop')!
    await act(async () => radios(pop)[1]!.click())
    const { mockIdentityService } = await import('../../api/mock/identity')
    const spy = vi.spyOn(mockIdentityService, 'PreviewRepo').mockRejectedValue(Object.assign(new Error('x'), { cause: { code: 'E_GIT_FAILED', message: 'x', params: { file: '~/code/acme_api/.git/config' } } }))
    const save = Array.from(pop.querySelectorAll('button')).find((b) => b.textContent === 'Save')!
    await act(async () => save.click())
    await flush()
    spy.mockRestore()
    expect(pop.contains(document.activeElement)).toBe(true)
    expect(document.activeElement?.textContent).toBe('Try again')
  })

  it('an E_FILE_IO for projects.json is a toast, not "cannot write this repository\u2019s config" (FU-16b)', async () => {
    await mount()
    await act(async () => pencil('acme_api').click())
    await flush(10)
    const pop = document.querySelector('.ident-pop')!
    await act(async () => radios(pop)[1]!.click())
    const { mockIdentityService } = await import('../../api/mock/identity')
    const spy = vi.spyOn(mockIdentityService, 'PreviewRepo').mockRejectedValue(Object.assign(new Error('x'), { cause: { code: 'E_FILE_IO', message: 'x', params: { file: '~/Library/Application Support/Codepass/projects.json' } } }))
    const save = Array.from(pop.querySelectorAll('button')).find((b) => b.textContent === 'Save')!
    await act(async () => save.click())
    await flush()
    spy.mockRestore()
    expect(document.querySelector('.ident-pop')?.textContent).not.toContain('Cannot write this repository')
    expect(document.querySelector('.toast')).not.toBeNull()
    expect(document.activeElement && document.querySelector('.ident-pop')?.contains(document.activeElement)).toBe(true)
  })

  it('a member that followed global shows the global email struck through in the sheet', async () => {
    await mount()
    const set = q('.ident-table button').find((b) => b.getAttribute('aria-label') === 'Set the author identity of project globex')!
    await act(async () => set.click())
    await flush()
    const own = q('.sheet input[type="radio"]')[1] as HTMLInputElement
    await act(async () => own.click())
    const inputs = q('.sheet input.input') as HTMLInputElement[]
    await setValue(inputs[1]!, 'alex@globex-new.example.com')
    await flush()
    const li = q('.sheet .affect li').find((l) => l.textContent?.includes('globex_backend'))!
    expect(li.querySelector('.was')?.textContent).toBe('alex@example.com')
    expect(li.textContent).not.toContain('Not set')
  })

  it('Save of the sheet waits for the preview of the typed draft', async () => {
    await mount()
    const set = q('.ident-table button').find((b) => b.getAttribute('aria-label') === 'Set the author identity of project acme')!
    await act(async () => set.click())
    await flush()
    const email = q('.sheet input[type="email"]')[0] as HTMLInputElement
    await setValue(email, 'alex@acme-new.example.com')
    await flush(1000)
    const save = () => q('.sheet__foot button').find((b) => b.textContent?.startsWith('Save'))!
    expect(save().hasAttribute('disabled')).toBe(false)
    await setValue(email, 'alex@acme-newer.example.com')
    expect(save().hasAttribute('disabled')).toBe(true)
    expect(q('.sheet .affect')[0]?.getAttribute('aria-busy')).toBe('true')
    await flush(1000)
    expect(save().hasAttribute('disabled')).toBe(false)
    // an invalid draft clears the old list instead of leaving it on screen
    await setValue(email, 'nope')
    await flush(1000)
    expect(q('.sheet .affect')).toHaveLength(0)
    expect(save().hasAttribute('disabled')).toBe(true)
  })

  it('says what is missing when Save of the global card is pressed with an empty field', async () => {
    resetIdentityMock('unset')
    await mount()
    const inputs = q('.ident-hero input') as HTMLInputElement[]
    await setValue(inputs[0]!, 'Alex')
    const save = q('.ident-hero button[type="submit"]')[0]!
    await act(async () => save.click())
    await flush(10)
    expect(q('.ident-hero .field__error').some((e) => e.textContent?.includes('Enter an email'))).toBe(true)
  })

  it('keeps focus on the card after the global identity is saved', async () => {
    await mount()
    const inputs = q('.ident-hero input') as HTMLInputElement[]
    await setValue(inputs[1]!, 'alex@new.example.com')
    await act(async () => q('.ident-hero button[type="submit"]')[0]!.click())
    await flush()
    const apply = q('.modal button').find((b) => b.textContent === 'Save to the Git config' || b.classList.contains('btn--primary'))!
    await act(async () => apply.click())
    await flush()
    expect(document.activeElement?.id).toBe('ident-global-title')
  })

  it('a repository can take its own identity', async () => {
    await mount()
    const edit = q('.ident-table button').find((b) => b.getAttribute('aria-label') === 'Edit the author identity of globex_backend')!
    await act(async () => edit.click())
    await flush(10)
    const pop = document.querySelector('.ident-pop')!
    expect(pop.textContent).toContain('Follow global')
    const own = Array.from(pop.querySelectorAll<HTMLInputElement>('input[type="radio"]'))[2]!
    await act(async () => own.click())
    const inputs = Array.from(pop.querySelectorAll<HTMLInputElement>('input.input'))
    await setValue(inputs[0]!, 'Alex')
    await setValue(inputs[1]!, 'alex@globex-core.example.com')
    const save = Array.from(pop.querySelectorAll('button')).find((b) => b.textContent === 'Save')!
    await act(async () => save.click())
    await flush()
    expect(document.querySelector('.ident-pop')).toBeNull()
    expect(rowOf('globex_backend')?.textContent).toContain('alex@globex-core.example.com')
    expect(rowOf('globex_backend')?.textContent).toContain('Repository’s own')
  })
})
