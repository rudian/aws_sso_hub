import { act, createElement } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { Identity } from '../api'
import { mockEmit } from '../api/events'
import { useProjectIdentities } from './projects'

;(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true

function Probe() {
  useProjectIdentities()
  return null
}

describe('useProjectIdentities', () => {
  afterEach(() => vi.restoreAllMocks())

  it('coalesces a burst of identity:changed events into one trailing overview read', async () => {
    let release: () => void = () => undefined
    const gate = new Promise<void>((r) => (release = r))
    const get = vi.spyOn(Identity, 'GetOverview').mockImplementation((async () => {
      await gate
      return { projects: [] }
    }) as never)
    const host = document.createElement('div')
    const root = createRoot(host)
    await act(async () => root.render(createElement(Probe)))
    expect(get).toHaveBeenCalledTimes(1) // the first load is in flight
    await act(async () => {
      for (let i = 0; i < 20; i++) mockEmit('identity:changed', {})
    })
    expect(get).toHaveBeenCalledTimes(1)
    await act(async () => {
      release()
      await new Promise((r) => setTimeout(r, 30))
    })
    expect(get).toHaveBeenCalledTimes(2) // one trailing run for all twenty
    act(() => root.unmount())
  })
})
