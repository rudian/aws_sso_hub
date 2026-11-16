import { act, createElement } from 'react'
import { createRoot } from 'react-dom/client'
import type { Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { I18nProvider } from '../../i18n'
import { SetupLog } from './SetupRows'

;(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true

let host: HTMLDivElement
let root: Root

beforeEach(() => {
  host = document.createElement('div')
  document.body.appendChild(host)
  root = createRoot(host)
})
afterEach(() => {
  act(() => root.unmount())
  host.remove()
})

function render(lines: string[], expanded: boolean) {
  act(() => root.render(createElement(I18nProvider, { lang: 'en' }, createElement(SetupLog, { lines, expanded, onToggle: () => {} }))))
}

describe('SetupLog follows the newest line', () => {
  for (const [name, expanded, size] of [['collapsed at 5 lines', false, 5], ['expanded at 200 lines', true, 200]] as const) {
    it(name, () => {
      let height = 100
      Object.defineProperty(HTMLElement.prototype, 'scrollHeight', { configurable: true, get: () => height })
      try {
        const lines = Array.from({ length: size }, (_, i) => `line ${i}`)
        render(lines, expanded)
        const pre = host.querySelector('pre') as HTMLPreElement
        expect(pre.scrollTop).toBe(100)
        height = 250
        render([...lines.slice(1), 'line new'], expanded) // same length, new last line
        expect(pre.scrollTop).toBe(250)
      } finally {
        delete (HTMLElement.prototype as { scrollHeight?: number }).scrollHeight
      }
    })
  }
})
