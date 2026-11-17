import { act, createElement } from 'react'
import { createRoot } from 'react-dom/client'
import type { Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { App } from './App'
import { getWorld, resetWorld } from './api/mock/world'
import { Language } from './api/models'
import { setRoute } from './lib/route'

;(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true

let host: HTMLDivElement
let root: Root

async function until(cond: () => boolean) {
  for (let i = 0; i < 80 && !cond(); i++) {
    await act(async () => {
      await new Promise((r) => setTimeout(r, 40))
    })
  }
}

beforeEach(() => {
  resetWorld('mid')
  getWorld().settings.language = Language.LanguageEn
  Element.prototype.scrollIntoView = vi.fn()
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

describe('leaving the workspace', () => {
  it('draws the shell with app--from-ws on the page that follows (the rail widens back to the sidebar)', async () => {
    const repo = getWorld().repos[0]
    setRoute({ page: 'workspace', repo: repo.id })
    act(() => root.render(createElement(App)))
    await until(() => !!document.querySelector('.ws'))
    expect(document.querySelector('.ws')).toBeTruthy()
    act(() => setRoute({ page: 'projects' }))
    await until(() => !!document.querySelector('.app'))
    expect(document.querySelector('.app.app--from-ws')).toBeTruthy()
  })
})
