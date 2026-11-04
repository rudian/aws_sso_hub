import { act, createElement } from 'react'
import type { ReactElement } from 'react'
import { createRoot } from 'react-dom/client'
import type { Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { System } from '../../api/services'
import { resetWorld } from '../../api/mock/world'
import { ErrorCode } from '../../api/models'
import { I18nProvider } from '../../i18n'
import { ActionsProvider } from '../../shell/AppActions'
import { RestoreModal } from './RestoreModal'

;(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true

let host: HTMLDivElement
let root: Root

async function settle(until: () => boolean) {
  for (let i = 0; i < 100 && !until(); i++) {
    await act(async () => {
      await new Promise((r) => setTimeout(r, 20))
    })
  }
}

async function mount(ui: ReactElement) {
  await act(async () => {
    root.render(createElement(I18nProvider, { lang: 'en' }, createElement(ActionsProvider, null, ui)))
  })
}

const find = (label: string) => Array.from(document.querySelectorAll<HTMLButtonElement>('button')).find((b) => b.textContent?.trim() === label)

beforeEach(() => {
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

describe('RestoreModal', () => {
  it('a changed config file shows the error and "Review again" previews the restore again', async () => {
    const w = resetWorld('mid')
    const id = w.backups[0].id
    const previewSpy = vi.spyOn(System, 'PreviewRestore')
    const restoreSpy = vi.spyOn(System, 'RestoreBackup').mockRejectedValue({ code: ErrorCode.ErrConfigChanged, message: 'changed', params: { file: 'config' } })

    await mount(<RestoreModal backupId={id} onClose={() => undefined} />)
    await settle(() => find('Restore backup')?.disabled === false)
    expect(previewSpy).toHaveBeenCalledTimes(1)

    await act(async () => find('Restore backup')?.click())
    await settle(() => !!find('Review again'))
    expect(restoreSpy).toHaveBeenCalledTimes(1)
    expect(document.body.textContent).toContain('config changed outside the app')
    expect(find('Review again')).toBeDefined()
    expect(find('Restore backup')?.disabled).toBe(true)

    await act(async () => find('Review again')?.click())
    await settle(() => previewSpy.mock.calls.length >= 2 && !find('Review again') && find('Restore backup')?.disabled === false)
    expect(previewSpy).toHaveBeenCalledTimes(2)
    expect(find('Review again')).toBeUndefined()
    expect(find('Restore backup')?.disabled).toBe(false)
  })

  it('keeps Restore disabled and hides the old diff while "Review again" is still loading', async () => {
    const w = resetWorld('mid')
    const id = w.backups[0].id
    const previewSpy = vi.spyOn(System, 'PreviewRestore')
    vi.spyOn(System, 'RestoreBackup').mockRejectedValue({ code: ErrorCode.ErrConfigChanged, message: 'changed', params: { file: 'config' } })

    await mount(<RestoreModal backupId={id} onClose={() => undefined} />)
    await settle(() => find('Restore backup')?.disabled === false)
    await act(async () => find('Restore backup')?.click())
    await settle(() => !!find('Review again'))

    let release: () => void = () => undefined
    const first = await previewSpy.mock.results[0].value
    previewSpy.mockImplementationOnce(() => new Promise((resolve) => (release = () => resolve(first))) as never)
    await act(async () => find('Review again')?.click())
    expect(find('Restore backup')?.disabled).toBe(true)
    expect(document.querySelector('[aria-busy="true"]')).not.toBeNull()

    await act(async () => release())
    await settle(() => find('Restore backup')?.disabled === false)
    expect(find('Restore backup')?.disabled).toBe(false)
  })

  it('shows the error and keeps Restore disabled when the second preview fails', async () => {
    const w = resetWorld('mid')
    const id = w.backups[0].id
    const previewSpy = vi.spyOn(System, 'PreviewRestore')
    const restoreSpy = vi.spyOn(System, 'RestoreBackup').mockRejectedValue({ code: ErrorCode.ErrConfigChanged, message: 'changed', params: { file: 'config' } })

    await mount(<RestoreModal backupId={id} onClose={() => undefined} />)
    await settle(() => find('Restore backup')?.disabled === false)
    await act(async () => find('Restore backup')?.click())
    await settle(() => !!find('Review again'))

    previewSpy.mockRejectedValueOnce({ code: ErrorCode.ErrFileIO, message: 'cannot read', params: { file: 'config' } })
    await act(async () => find('Review again')?.click())
    await settle(() => !!document.querySelector('[role="alert"]') || !!find('Retry'))
    expect(find('Restore backup')?.disabled).toBe(true)
    expect(document.body.textContent).toMatch(/Retry|Try again/)
    expect(restoreSpy).toHaveBeenCalledTimes(1)
  })
})
