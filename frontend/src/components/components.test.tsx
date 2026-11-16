import { act, createElement } from 'react'
import type { ReactElement } from 'react'
import { createRoot } from 'react-dom/client'
import type { Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { SessionState } from '../api/models'
import type { SessionStatus } from '../api/models'
import { I18nProvider } from '../i18n'
import type { Lang } from '../i18n'
import { CodeText, codeMark, splitCodeText } from './CodeText'
import { Countdown } from './Countdown'
import { Menu } from './Menu'
import { Modal } from './Modal'
import { DiffView } from './DiffView'
import { Segmented } from './Segmented'
import { Tabs } from './Tabs'
import { AcctChip } from './AcctChip'
import { Button } from './Button'
import { Field } from './Field'
import { Input } from './Input'
import { DiffKind } from '../api/models'

;(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true

const NOW = Date.parse('2026-11-04T04:00:00Z')
const iso = (min: number) => new Date(NOW + min * 60_000).toISOString().replace(/\.\d{3}Z$/, 'Z')

let host: HTMLDivElement
let root: Root

function render(ui: ReactElement, lang: Lang = 'en') {
  act(() => root.render(createElement(I18nProvider, { lang }, ui)))
}

function session(over: Partial<SessionStatus> = {}): SessionStatus {
  return {
    portal: 'initech',
    state: SessionState.SessionActive,
    expiresAt: iso(42),
    issuedAt: iso(-18),
    sessionKnown: true,
    loginAt: iso(-108),
    sessionEndsAt: iso(372),
    sessionEndExact: false,
    sessionMinutes: 480,
    sessionOverrun: false,
    progressPct: 78,
    canRefresh: true,
    refreshing: false,
    ...over,
  }
}

beforeEach(() => {
  vi.useFakeTimers({ toFake: ['Date', 'setTimeout', 'clearTimeout', 'setInterval', 'clearInterval'] })
  vi.setSystemTime(NOW)
  host = document.createElement('div')
  document.body.appendChild(host)
  root = createRoot(host)
})
afterEach(() => {
  act(() => root.unmount())
  host.remove()
  vi.useRealTimers()
})

describe('Countdown', () => {
  it('known session: label, bar at remaining/sessionMinutes, normal colour', () => {
    render(<Countdown session={session()} slot={3} label="Signed in, 6 h 12 min left" />)
    expect(host.textContent).toContain('Signed in, 6 h 12 min left')
    const bar = host.querySelector('[role="progressbar"]')
    expect(bar?.getAttribute('aria-valuenow')).toBe('78')
    expect(host.querySelector('.countdown--low')).toBeNull()
    expect(host.querySelector('.countdown--danger')).toBeNull()
    expect((host.querySelector('.countdown__fill') as HTMLElement).style.getPropertyValue('--pct')).toBe('78%')
    expect(host.querySelector('.countdown')?.getAttribute('data-acct')).toBe('3')
  })

  it('colour steps: warn under 60 min, danger under 15 min', () => {
    render(<Countdown session={session({ sessionEndsAt: iso(48) })} label="48 min" />)
    expect(host.querySelector('.countdown--low')).not.toBeNull()
    render(<Countdown session={session({ sessionEndsAt: iso(9), state: SessionState.SessionExpiring })} label="9 min" />)
    expect(host.querySelector('.countdown--danger')).not.toBeNull()
    expect(host.querySelector('.countdown__fill')).not.toBeNull()
  })

  it('overrun: the overrun wording, nothing to adjust, and no bar', () => {
    render(<Countdown session={session({ sessionOverrun: true, progressPct: 0, sessionEndsAt: iso(-5) })} label="ignored" />)
    expect(host.textContent).toContain('Past the expected end, still signed in')
    expect(host.textContent).not.toContain('ignored')
    expect(host.querySelector('.countdown__fill')).toBeNull()
    expect(host.querySelectorAll('button')).toHaveLength(1) // only the label that opens the details
  })

  it('unknown session: the unknown wording, an info icon, no bar', () => {
    render(<Countdown session={session({ sessionKnown: false, loginAt: undefined, sessionEndsAt: undefined, progressPct: 0 })} label="ignored" />)
    expect(host.textContent).toContain('Signed in · start time unknown')
    expect(host.querySelector('.countdown__info')).not.toBeNull()
    expect(host.querySelector('.countdown__fill')).toBeNull()
  })

  it('clicking the label opens the sign-in details popover: an estimated end, set by the administrator', () => {
    render(<Countdown session={session()} label="Signed in" />, 'en')
    expect(document.body.textContent).not.toContain('Sign-in details')
    act(() => (host.querySelector('.countdown__label') as HTMLButtonElement).click())
    const pop = document.body.querySelector('.session-details')
    expect(pop?.textContent).toContain('Sign-in details')
    expect(pop?.textContent).toContain('Signed in at')
    expect(pop?.textContent).toContain('Expected end')
    expect(pop?.textContent).toContain('8 hours · estimate')
    expect(pop?.textContent).toContain('renews automatically')
    expect(pop?.textContent).toContain('Your AWS administrator sets the session length in IAM Identity Center.')
    expect(pop?.textContent).toContain("Quitting the app doesn't sign you out".replace("'", '’'))
    expect(pop?.querySelector('button')).toBeNull()
  })

  it('details: an end that AWS showed is the session end, not an estimate', () => {
    render(<Countdown session={session({ sessionEndExact: true, sessionEndsAt: iso(12), sessionMinutes: 120 })} label="x" />, 'zh-CN')
    act(() => (host.querySelector('.countdown__label') as HTMLButtonElement).click())
    const pop = document.body.querySelector('.session-details')
    const keys = Array.from(document.body.querySelectorAll('.session-details .kv__key')).map((e) => e.textContent)
    expect(keys).toContain('会话结束')
    expect(keys).not.toContain('预计结束')
    expect(pop?.textContent).toContain('2小时')
    expect(pop?.textContent).not.toContain('估计')
    expect(pop?.textContent).toContain('会话时长由 AWS 管理员在 IAM Identity Center 中设定')
  })

  it('details show dashes when the session start is unknown', () => {
    render(<Countdown session={session({ sessionKnown: false, loginAt: undefined, sessionEndsAt: undefined })} label="x" />)
    act(() => (host.querySelector('.countdown__label') as HTMLButtonElement).click())
    const vals = Array.from(document.body.querySelectorAll('.session-details .kv__val')).map((e) => e.textContent)
    expect(vals[0]).toBe('—')
    expect(vals[1]).toBe('—')
  })

  it('ticks: the bar shrinks as time passes', () => {
    render(<Countdown session={session({ sessionEndsAt: iso(240) })} label="x" />)
    expect(host.querySelector('[role="progressbar"]')?.getAttribute('aria-valuenow')).toBe('50')
    act(() => {
      vi.setSystemTime(NOW + 120 * 60_000)
      vi.advanceTimersByTime(30_000)
    })
    expect(host.querySelector('[role="progressbar"]')?.getAttribute('aria-valuenow')).toBe('25')
  })
})

describe('Modal', () => {
  it('closes on Esc, and not when dismissible is false', () => {
    const onClose = vi.fn()
    render(
      <Modal open title="T" onClose={onClose}>
        body
      </Modal>,
    )
    act(() => {
      window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    })
    expect(onClose).toHaveBeenCalledOnce()
    const onClose2 = vi.fn()
    render(
      <Modal open title="T" onClose={onClose2} dismissible={false}>
        body
      </Modal>,
    )
    act(() => {
      window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    })
    expect(onClose2).not.toHaveBeenCalled()
    expect(document.body.querySelector('[aria-label="Close"]')).toBeNull()
  })

  it('onRequestClose keeps the close button while the modal is not dismissible', () => {
    const onClose = vi.fn()
    const onRequest = vi.fn()
    render(
      <Modal open title="T" onClose={onClose} dismissible={false} onRequestClose={onRequest}>
        body
      </Modal>,
    )
    const x = document.body.querySelector('[aria-label="Close"]') as HTMLButtonElement
    expect(x).not.toBeNull()
    act(() => x.click())
    expect(onRequest).toHaveBeenCalledOnce()
    expect(onClose).not.toHaveBeenCalled()
  })

  it('has an accessible name and focuses inside; the backdrop ignores clicks when not dismissible', () => {
    const onClose = vi.fn()
    render(
      <Modal open title="Sign in" onClose={onClose} dismissible={false} footer={<Button>OK</Button>}>
        <Input />
      </Modal>,
    )
    const dialog = document.body.querySelector('[role="dialog"]') as HTMLElement
    expect(dialog.getAttribute('aria-modal')).toBe('true')
    expect(document.getElementById(dialog.getAttribute('aria-labelledby') as string)?.textContent).toBe('Sign in')
    act(() => {
      vi.advanceTimersByTime(10)
    })
    expect(dialog.contains(document.activeElement)).toBe(true)
    act(() => {
      ;(document.body.querySelector('.modal-backdrop') as HTMLElement).dispatchEvent(new MouseEvent('mousedown', { bubbles: true }))
    })
    expect(onClose).not.toHaveBeenCalled()
  })
})

describe('Menu', () => {
  it('opens from the trigger, moves with arrows, selects with click and closes', () => {
    const a = vi.fn()
    const b = vi.fn()
    render(
      <Menu
        trigger={<Button>More</Button>}
        items={[
          { id: 'a', label: 'Alpha', onSelect: a },
          { id: 'b', label: 'Beta', onSelect: b },
        ]}
      />,
    )
    act(() => (host.querySelector('button') as HTMLButtonElement).click())
    const items = Array.from(document.body.querySelectorAll('[role="menuitem"]')) as HTMLButtonElement[]
    expect(items.map((i) => i.textContent)).toEqual(['Alpha', 'Beta'])
    act(() => {
      vi.advanceTimersByTime(0)
    })
    act(() => {
      items[0].dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true }))
    })
    expect(document.activeElement).toBe(items[1])
    act(() => items[1].click())
    expect(b).toHaveBeenCalledOnce()
    expect(document.body.querySelector('[role="menu"]')).toBeNull()
  })
})

describe('small components', () => {
  it('Segmented radiogroup exposes aria-checked and arrow keys', () => {
    const onChange = vi.fn()
    render(<Segmented role="radiogroup" ariaLabel="Theme" value="a" onChange={onChange} items={[{ value: 'a', label: 'A' }, { value: 'b', label: 'B' }]} />)
    const radios = Array.from(host.querySelectorAll('[role="radio"]'))
    expect(radios.map((r) => r.getAttribute('aria-checked'))).toEqual(['true', 'false'])
    act(() => {
      host.querySelector('.segmented')!.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowRight', bubbles: true }))
    })
    expect(onChange).toHaveBeenCalledWith('b')
  })

  it('Segmented moves focus on the next frame without reading a cleared currentTarget', () => {
    const frames: FrameRequestCallback[] = []
    const spy = vi.spyOn(window, 'requestAnimationFrame').mockImplementation((cb) => frames.push(cb))
    try {
      render(<Segmented role="radiogroup" ariaLabel="Theme" value="b" onChange={() => undefined} items={[{ value: 'a', label: 'A' }, { value: 'b', label: 'B' }]} />)
      act(() => {
        host.querySelector('.segmented')!.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowLeft', bubbles: true }))
      })
      expect(frames).toHaveLength(1)
      expect(() => frames[0](0)).not.toThrow()
    } finally {
      spy.mockRestore()
    }
  })

  it('Segmented focuses the target even when the value arrives late', () => {
    const frames: FrameRequestCallback[] = []
    const spy = vi.spyOn(window, 'requestAnimationFrame').mockImplementation((cb) => frames.push(cb))
    try {
      render(<Segmented role="radiogroup" ariaLabel="Motion" value="a" onChange={() => undefined} items={[{ value: 'a', label: 'A' }, { value: 'b', label: 'B' }]} />)
      act(() => {
        host.querySelector('.segmented')!.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowRight', bubbles: true }))
      })
      frames[0](0) // the value is still 'a' here, as during an async save
      expect(document.activeElement).toBe(host.querySelectorAll('[role="radio"]')[1])
    } finally {
      spy.mockRestore()
    }
  })

  it('Tabs skip disabled tabs', () => {
    const onChange = vi.fn()
    render(<Tabs value="a" onChange={onChange} tabs={[{ id: 'a', label: 'A' }, { id: 'b', label: 'B', disabled: true }, { id: 'c', label: 'C' }]} />)
    act(() => {
      host.querySelector('.tabs')!.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowRight', bubbles: true }))
    })
    expect(onChange).toHaveBeenCalledWith('c')
  })

  it('DiffView renders add/del/ctx lines and the fold label', () => {
    render(
      <DiffView
        preview={{
          file: '~/.aws/config',
          path: '/x',
          baseHash: 'h',
          backupDir: '',
          added: 1,
          removed: 1,
          lines: [
            { kind: DiffKind.DiffContext, text: '[profile acme]' },
            { kind: DiffKind.DiffAdded, text: 'sso_session = initech' },
            { kind: DiffKind.DiffRemoved, text: 'old = 1' },
            { kind: DiffKind.DiffFolded, text: '', count: 18 },
          ],
        }}
      />,
    )
    expect(host.querySelectorAll('.diff-add')).toHaveLength(1)
    expect(host.querySelectorAll('.diff-del')).toHaveLength(1)
    expect(host.querySelectorAll('.diff-ctx')).toHaveLength(1)
    expect(host.querySelector('.diff-com')?.textContent).toBe('⋯ 18 unchanged lines ⋯')
    expect(host.textContent).toContain('1 added, 1 removed')
  })

  it('Field wires label, hint and error to the input', () => {
    render(
      <Field label="Name" hint="Letters only" error="Taken">
        <Input />
      </Field>,
    )
    const input = host.querySelector('input') as HTMLInputElement
    const label = host.querySelector('label') as HTMLLabelElement
    expect(label.htmlFor).toBe(input.id)
    expect(input.getAttribute('aria-invalid')).toBe('true')
    expect(input.getAttribute('aria-describedby')).toContain(`${input.id}-hint`)
  })

  it('Button loading disables and hides the label', () => {
    render(<Button loading>Save</Button>)
    const b = host.querySelector('button') as HTMLButtonElement
    expect(b.disabled).toBe(true)
    expect(b.className).toContain('is-loading')
  })

  it('Button loading with keepEnabled shows the spinner and stays focusable; disabled still disables', () => {
    render(<Button loading keepEnabled>Console</Button>)
    const b = host.querySelector('button') as HTMLButtonElement
    expect(b.hasAttribute('disabled')).toBe(false)
    expect(b.getAttribute('aria-busy')).toBe('true')
    expect(b.className).toContain('is-loading')
    act(() => b.focus())
    expect(document.activeElement).toBe(b)
    render(<Button keepEnabled>Console</Button>)
    expect(b.getAttribute('aria-busy')).toBeNull()
    expect(b.className).not.toContain('is-loading')
    render(<Button loading keepEnabled disabled>Console</Button>)
    expect(b.disabled).toBe(true)
  })
})

describe('CodeText', () => {
  it('splits marked values into code parts and leaves the rest as text', () => {
    const text = `Back up ${codeMark('~/.aws/config')} now`
    expect(splitCodeText(text)).toEqual([
      { code: false, text: 'Back up ' },
      { code: true, text: '~/.aws/config' },
      { code: false, text: ' now' },
    ])
  })
  it('never lets a marked value smuggle markers in', () => {
    expect(splitCodeText(codeMark('a\u0003b'))).toEqual([{ code: true, text: 'ab' }])
  })
  it('renders code elements', () => {
    render(<CodeText text={`use ${codeMark('--profile acme')}`} />)
    expect(host.querySelector('code')?.textContent).toBe('--profile acme')
    expect(host.textContent).toBe('use --profile acme')
  })
})

describe('AcctChip', () => {
  it('renders an ellipsis-able name and a title with the full name', () => {
    const name = 'a-very-long-profile-name-that-would-overflow-the-cell'
    render(<AcctChip slot={2} monogram="AV" name={name} />)
    expect(host.querySelector('.acct-chip')?.hasAttribute('title')).toBe(false)
    const nameEl = host.querySelector('.acct-chip__name')
    expect(nameEl?.getAttribute('title')).toBe(name)
    expect(nameEl?.textContent).toBe(name)
  })
})
