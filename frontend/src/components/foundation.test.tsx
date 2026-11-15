import { act, createElement } from 'react'
import type { ReactElement } from 'react'
import { createRoot } from 'react-dom/client'
import type { Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { I18nProvider } from '../i18n'
import { applyMotionMode } from '../lib/motion'
import { Popover } from './Popover'
import { ProviderMark } from './ProviderMark'
import { Sheet } from './Sheet'
import { ToolMark } from './ToolMark'

;(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true

let host: HTMLDivElement
let root: Root

function render(ui: ReactElement) {
  act(() => root.render(createElement(I18nProvider, { lang: 'en' }, ui)))
}

beforeEach(() => {
  vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
  applyMotionMode('full')
  host = document.createElement('div')
  document.body.appendChild(host)
  root = createRoot(host)
})
afterEach(() => {
  act(() => root.unmount())
  host.remove()
  document.documentElement.removeAttribute('data-motion')
  vi.useRealTimers()
})

describe('Sheet', () => {
  it('renders a labelled dialog with body and footer, and focuses inside it', () => {
    render(
      createElement(Sheet, { open: true, onClose: () => {}, title: 'Author identity', subtitle: 'for acme', footer: createElement('button', null, 'Save') }, createElement('input', { 'aria-label': 'name' })),
    )
    const dlg = document.querySelector('aside.sheet') as HTMLElement
    expect(dlg.getAttribute('role')).toBe('dialog')
    expect(dlg.getAttribute('aria-modal')).toBe('true')
    const label = document.getElementById(dlg.getAttribute('aria-labelledby')!)
    expect(label?.textContent).toBe('Author identity')
    expect(dlg.querySelector('.sheet__sub')?.textContent).toBe('for acme')
    expect(dlg.querySelector('.sheet__foot')?.textContent).toBe('Save')
    act(() => vi.advanceTimersByTime(5))
    // jsdom has no layout (offsetParent is always null), so focus falls back to the dialog itself.
    expect(dlg.contains(document.activeElement)).toBe(true)
  })

  it('closes on Esc and on a backdrop press, but not when not dismissible', () => {
    const onClose = vi.fn()
    render(createElement(Sheet, { open: true, onClose, title: 'T' }, createElement('input')))
    act(() => {
      window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true }))
    })
    expect(onClose).toHaveBeenCalledTimes(1)
    act(() => {
      document.querySelector('.sheet-backdrop')!.dispatchEvent(new MouseEvent('mousedown', { bubbles: true }))
    })
    expect(onClose).toHaveBeenCalledTimes(2)

    const blocked = vi.fn()
    render(createElement(Sheet, { open: true, onClose: blocked, title: 'T', dismissible: false }, createElement('input')))
    act(() => {
      window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true }))
      document.querySelector('.sheet-backdrop')!.dispatchEvent(new MouseEvent('mousedown', { bubbles: true }))
    })
    expect(blocked).not.toHaveBeenCalled()
    expect(document.querySelector('.sheet [aria-label="Close"], .sheet .icon-btn')).toBeNull()
  })

  describe('focus trap', () => {
    const desc = Object.getOwnPropertyDescriptor(HTMLElement.prototype, 'offsetParent')
    beforeEach(() => {
      Object.defineProperty(HTMLElement.prototype, 'offsetParent', {
        configurable: true,
        get() {
          return (this as HTMLElement).parentNode
        },
      })
    })
    afterEach(() => {
      if (desc) Object.defineProperty(HTMLElement.prototype, 'offsetParent', desc)
      else delete (HTMLElement.prototype as unknown as Record<string, unknown>).offsetParent
    })
    const tab = (shiftKey = false) => {
      const ev = new KeyboardEvent('keydown', { key: 'Tab', shiftKey, bubbles: true, cancelable: true })
      act(() => {
        window.dispatchEvent(ev)
      })
      return ev
    }
    const mountTwo = () => {
      render(createElement(Sheet, { open: true, onClose: () => {}, title: 'T', dismissible: false }, createElement('input', { id: 'a' }), createElement('input', { id: 'b' })))
      return [document.getElementById('a') as HTMLElement, document.getElementById('b') as HTMLElement] as const
    }

    it('wraps Tab from the last element to the first', () => {
      const [a, b] = mountTwo()
      b.focus()
      const ev = tab()
      expect(ev.defaultPrevented).toBe(true)
      expect(document.activeElement).toBe(a)
    })

    it('wraps Shift+Tab from the first element to the last', () => {
      const [a, b] = mountTwo()
      a.focus()
      const ev = tab(true)
      expect(ev.defaultPrevented).toBe(true)
      expect(document.activeElement).toBe(b)
    })

    it('skips visibility:hidden content (a closed expand) when wrapping', () => {
      render(
        createElement(
          Sheet,
          { open: true, onClose: () => {}, title: 'T', dismissible: false },
          createElement('input', { id: 'a' }),
          createElement('input', { id: 'b' }),
          createElement('div', { style: { visibility: 'hidden' } }, createElement('button', { id: 'h' }, 'hidden')),
        ),
      )
      const a = document.getElementById('a') as HTMLElement
      const b = document.getElementById('b') as HTMLElement
      b.focus()
      expect(tab().defaultPrevented).toBe(true)
      expect(document.activeElement).toBe(a)
      a.focus()
      expect(tab(true).defaultPrevented).toBe(true)
      expect(document.activeElement).toBe(b)
    })

    it('returns stray focus to the field the user was in, not the Close button', () => {
      const onClose = vi.fn()
      render(createElement(Sheet, { open: true, onClose, title: 'T' }, createElement('input', { id: 'f' })))
      const field = document.getElementById('f') as HTMLElement
      const outside = document.createElement('button')
      document.body.appendChild(outside)
      field.focus()
      act(() => outside.focus())
      expect(document.activeElement).toBe(field)
      expect(document.activeElement?.classList.contains('icon-btn')).toBe(false)
      outside.remove()
    })

    it('pulls focus back in when it lands outside the sheet', () => {
      const [a] = mountTwo()
      const out = document.createElement('button')
      document.body.appendChild(out)
      out.focus()
      expect(document.activeElement).toBe(a)
      out.remove()
    })

    it('returns focus to the opener on close', () => {
      const opener = document.createElement('button')
      document.body.appendChild(opener)
      opener.focus()
      render(createElement(Sheet, { open: true, onClose: () => {}, title: 'T' }, createElement('input')))
      act(() => {
        vi.advanceTimersByTime(5)
      })
      render(createElement(Sheet, { open: false, onClose: () => {}, title: 'T' }, createElement('input')))
      act(() => {
        vi.advanceTimersByTime(400)
      })
      expect(document.activeElement).toBe(opener)
      opener.remove()
    })

    it('leaves Tab alone inside a popover opened from the sheet', () => {
      const anchor = { current: document.createElement('button') }
      const ui = (open: boolean) =>
        createElement(
          Sheet,
          { open: true, onClose: () => {}, title: 'T' },
          createElement('input'),
          createElement(Popover, { anchorRef: anchor, open, onClose: () => {}, ariaLabel: 'p' }, createElement('input', { id: 'p1' }), createElement('input', { id: 'p2' })),
        )
      render(ui(false))
      render(ui(true))
      const p1 = document.getElementById('p1') as HTMLElement
      p1.focus()
      const ev = tab()
      expect(ev.defaultPrevented).toBe(false)
      expect(document.activeElement).toBe(p1)
    })

    it('wraps Tab inside a popover opened from the sheet', () => {
      const anchor = { current: document.createElement('button') }
      const ui = (open: boolean) =>
        createElement(
          Sheet,
          { open: true, onClose: () => {}, title: 'T' },
          createElement('input'),
          createElement(Popover, { anchorRef: anchor, open, onClose: () => {}, ariaLabel: 'p' }, createElement('input', { id: 'p1' }), createElement('input', { id: 'p2' })),
        )
      render(ui(false))
      render(ui(true))
      const p1 = document.getElementById('p1') as HTMLElement
      const p2 = document.getElementById('p2') as HTMLElement
      p2.focus()
      expect(tab().defaultPrevented).toBe(true)
      expect(document.activeElement).toBe(p1)
      expect(tab(true).defaultPrevented).toBe(true)
      expect(document.activeElement).toBe(p2)
    })

    it('does nothing to Tab while a dialog opened later covers the sheet', () => {
      const [, b] = mountTwo()
      b.focus()
      const later = document.createElement('div')
      later.className = 'modal-backdrop'
      document.body.appendChild(later)
      const ev = tab()
      expect(ev.defaultPrevented).toBe(false)
      expect(document.activeElement).toBe(b)
      later.remove()
    })
  })

  it('Esc closes a popover opened from inside the sheet, not the sheet', () => {
    const onClose = vi.fn()
    const onPop = vi.fn()
    const anchor = { current: document.createElement('button') }
    const ui = (open: boolean) =>
      createElement(
        Sheet,
        { open: true, onClose, title: 'T' },
        createElement('input'),
        createElement(Popover, { anchorRef: anchor, open, onClose: onPop, ariaLabel: 'p' }, createElement('span', null, 'x')),
      )
    render(ui(false))
    render(ui(true)) // opened later, so it is a later body child than the sheet
    expect(document.querySelector('.popover')).not.toBeNull()
    act(() => {
      window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true }))
    })
    expect(onPop).toHaveBeenCalledTimes(1)
    expect(onClose).not.toHaveBeenCalled()
  })

  it('stays mounted for the 160ms exit animation, then unmounts', () => {
    render(createElement(Sheet, { open: true, onClose: () => {}, title: 'T' }))
    render(createElement(Sheet, { open: false, onClose: () => {}, title: 'T' }))
    expect(document.querySelector('.sheet')?.classList.contains('is-leaving')).toBe(true)
    expect(document.querySelector('.sheet-backdrop')?.classList.contains('is-leaving')).toBe(true)
    act(() => vi.advanceTimersByTime(159))
    expect(document.querySelector('.sheet')).not.toBeNull()
    act(() => vi.advanceTimersByTime(2))
    expect(document.querySelector('.sheet')).toBeNull()
  })

  it('unmounts at once under Reduced motion', () => {
    applyMotionMode('reduce')
    render(createElement(Sheet, { open: true, onClose: () => {}, title: 'T' }))
    render(createElement(Sheet, { open: false, onClose: () => {}, title: 'T' }))
    expect(document.querySelector('.sheet')).toBeNull()
  })

  it('reopening during the exit cancels it', () => {
    render(createElement(Sheet, { open: true, onClose: () => {}, title: 'T' }))
    render(createElement(Sheet, { open: false, onClose: () => {}, title: 'T' }))
    render(createElement(Sheet, { open: true, onClose: () => {}, title: 'T' }))
    act(() => vi.advanceTimersByTime(500))
    const s = document.querySelector('.sheet')
    expect(s).not.toBeNull()
    expect(s?.classList.contains('is-leaving')).toBe(false)
  })

  it('ignores Esc while a dialog opened later sits above it', () => {
    const onClose = vi.fn()
    render(createElement(Sheet, { open: true, onClose, title: 'T' }))
    const later = document.createElement('div')
    later.className = 'modal-backdrop'
    document.body.appendChild(later)
    act(() => {
      window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true }))
    })
    expect(onClose).not.toHaveBeenCalled()
    later.remove()
  })
})

describe('marks', () => {
  it('ProviderMark names the provider and sizes up', () => {
    render(createElement('div', null, createElement(ProviderMark, { provider: 'github' }), createElement(ProviderMark, { provider: 'aws', large: true })))
    const marks = host.querySelectorAll('.prov')
    expect(marks[0].getAttribute('aria-label')).toBe('GitHub')
    expect(marks[1].getAttribute('aria-label')).toBe('AWS CodeCommit')
    expect(marks[1].classList.contains('prov--lg')).toBe(true)
    expect(marks[0].querySelector('svg')).not.toBeNull()
  })

  it('ToolMark draws a neutral glyph for each tool', () => {
    render(createElement('div', null, createElement(ToolMark, { tool: 'claude' }), createElement(ToolMark, { tool: 'codex', large: true }), createElement(ToolMark, { tool: 'opencode' })))
    const marks = Array.from(host.querySelectorAll('.tool'))
    expect(marks.map((m) => m.getAttribute('aria-label'))).toEqual(['Claude Code', 'Codex', 'OpenCode'])
    expect(marks[1].classList.contains('tool--lg')).toBe(true)
    for (const m of marks) expect(m.querySelectorAll('svg path, svg circle').length).toBeGreaterThan(0)
  })
})
