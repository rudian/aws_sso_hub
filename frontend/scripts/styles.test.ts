import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

// Static checks on the style foundation (F1-08). They read the CSS files as text because the
// jsdom test environment does not process stylesheets. Run from frontend/ (npm test).
const css = (name: string) => readFileSync(resolve(process.cwd(), 'src/styles', name), 'utf8')
const tokens = css('tokens.css')

describe('motion css', () => {
  // visibility and overflow only flip discretely after a delay (hides closed collapse content); it does not animate.
  const ALLOWED = new Set(['opacity', 'transform', 'grid-template-rows', 'visibility', 'overflow'])

  it('transitions in motion.css only name opacity, transform and grid-template-rows', () => {
    const text = css('motion.css').replace(/\/\*[\s\S]*?\*\//g, '')
    const props = new Set<string>()
    for (const m of text.matchAll(/transition\s*:\s*([^;}]+)/g)) {
      for (const part of m[1].split(/,(?![^(]*\))/)) props.add(part.trim().split(/\s+/)[0])
    }
    expect(props.size).toBeGreaterThan(0)
    for (const p of props) expect(ALLOWED.has(p), p).toBe(true)
  })

  it('keyframes in motion.css only set opacity and transform', () => {
    const text = css('motion.css').replace(/\/\*[\s\S]*?\*\//g, '')
    const blocks = [...text.matchAll(/@keyframes\s+[\w-]+\s*\{((?:[^{}]|\{[^{}]*\})*)\}/g)]
    expect(blocks.length).toBeGreaterThan(5)
    for (const b of blocks) {
      for (const decl of b[1].matchAll(/\{([^{}]*)\}/g)) {
        for (const d of decl[1].split(';')) {
          const prop = d.split(':')[0].trim()
          if (prop) expect(['opacity', 'transform'], prop).toContain(prop)
        }
      }
    }
  })

  it('Reduced collapses every transition and animation to about 1ms, and System follows the OS unless Full', () => {
    const t = css('tokens.css')
    expect(t).toMatch(/:root\[data-motion="reduce"\] \*[\s\S]*?animation-duration: 1ms !important[\s\S]*?transition-duration: 1ms !important/)
    expect(t).toMatch(/@media \(prefers-reduced-motion: reduce\) \{\s*:root:not\(\[data-motion="full"\]\) \*/)
  })

  it('Reduced keeps the needed busy cues turning, in the media query and the data-motion block', () => {
    const t = css('tokens.css')
    for (const sel of ['.icon-btn.is-spinning svg', '.progress--indeterminate .progress__bar']) {
      expect(t, sel).toContain(`:root:not([data-motion="full"]) ${sel}`)
      expect(t, sel).toContain(`:root[data-motion="reduce"] ${sel}`)
    }
  })

  it('closed collapse and expand content is hidden from focus and the accessibility tree', () => {
    const m = css('motion.css')
    expect(m).toMatch(/\.collapse\.is-closed > \.collapse__inner \{[^}]*visibility: hidden/)
    expect(m).toMatch(/\.expand\.is-closed > \.expand__inner \{[^}]*visibility: hidden/)
  })

  it('open expand and collapse content is not clipped; it clips only while closed or leaving', () => {
    const m = css('motion.css').replace(/\/\*[\s\S]*?\*\//g, '')
    expect(m).toMatch(/\.expand > \.expand__inner \{[^}]*overflow: visible/)
    expect(m).toMatch(/\.collapse__inner \{[^}]*overflow: visible/)
    expect(m).toMatch(/\.expand > \.expand__inner \{[^}]*min-width: 0/)
    expect(m).toMatch(/\.collapse__inner \{[^}]*min-width: 0/)
    expect(m).toMatch(/\.expand\.is-closed > \.expand__inner \{[^}]*overflow: hidden/)
    expect(m).toMatch(/\.collapse\.is-leaving > \.collapse__inner, \.collapse\.is-closed > \.collapse__inner \{[^}]*overflow: hidden/)
  })

  it('menus and popovers fade and drop in', () => {
    expect(css('tokens.css')).toMatch(/\.popover, \.menu \{[^}]*animation: pop-in var\(--dur-leave\)/)
  })

  it('a menu or popover opened above a dialog or sheet sits above it', () => {
    expect(css('tokens.css')).toMatch(/\.sheet ~ \.menu, \.sheet ~ \.popover \{ z-index: calc\(var\(--z-modal\) \+ 1\)/)
  })

  it('keeps the motion durations of design 3', () => {
    const t = css('tokens.css')
    expect(t).toMatch(/--dur-page: 200ms/)
    expect(t).toMatch(/--dur-panel: 220ms/)
    expect(t).toMatch(/--dur-list: 180ms/)
    expect(t).toMatch(/--ease-enter: cubic-bezier\(\.2, \.8, \.2, 1\)/)
    expect(t).toMatch(/--ease-exit: cubic-bezier\(\.4, 0, 1, 1\)/)
    expect(t).toMatch(/--ease-move: cubic-bezier\(\.3, \.7, \.2, 1\)/)
  })

  it('components.css pulls in motion.css', () => {
    expect(css('components.css')).toContain("@import './motion.css'")
  })
})

// ---- contrast of the syntax, git and terminal tokens (ui-design.md section 2)
function lum(hex: string): number {
  const n = parseInt(hex.slice(1), 16)
  const ch = (v: number) => {
    const c = v / 255
    return c <= 0.03928 ? c / 12.92 : Math.pow((c + 0.055) / 1.055, 2.4)
  }
  return 0.2126 * ch((n >> 16) & 255) + 0.7152 * ch((n >> 8) & 255) + 0.0722 * ch(n & 255)
}
function ratio(a: string, b: string): number {
  const x = lum(a)
  const y = lum(b)
  return (Math.max(x, y) + 0.05) / (Math.min(x, y) + 0.05)
}

const section = tokens.slice(tokens.indexOf('17. Codepass v1.0 tokens'))
function block(selector: string): Record<string, string> {
  const start = section.indexOf(selector + ' {')
  const body = section.slice(start, section.indexOf('}', start))
  const out: Record<string, string> = {}
  for (const m of body.matchAll(/(--[\w-]+):\s*(#[0-9A-Fa-f]{6})/g)) out[m[1]] = m[2]
  return out
}

describe('syntax token contrast', () => {
  const light = { ...block(':root'), }
  const dark = { ...block(':root[data-theme="dark"]') }
  const TEXT = ['--syn-kw', '--syn-str', '--syn-num', '--syn-com', '--syn-type', '--syn-fn', '--syn-punc', '--git-mod', '--git-add', '--git-del']

  it('every syntax and git colour is at least 4.5:1 on the editor background and the current-line tint', () => {
    for (const [theme, t] of [['light', { ...light }], ['dark', { ...light, ...dark }]] as const) {
      for (const k of TEXT) {
        expect(ratio(t[k], t['--editor-bg']), `${theme} ${k} on editor-bg`).toBeGreaterThanOrEqual(4.5)
        expect(ratio(t[k], t['--editor-line']), `${theme} ${k} on editor-line`).toBeGreaterThanOrEqual(4.5)
      }
    }
  })

  it('the comment colour is #687180 in light and #838D9C in dark', () => {
    expect(light['--syn-com'].toUpperCase()).toBe('#687180')
    expect(dark['--syn-com'].toUpperCase()).toBe('#838D9C')
  })

  it('terminal text is readable on the terminal surface', () => {
    const term = { ...light }
    for (const k of ['--term-fg', '--term-dim', '--term-ok', '--term-warn', '--term-info', '--term-danger', '--term-accent']) {
      expect(ratio(term[k], term['--term-bg']), k).toBeGreaterThanOrEqual(4.5)
    }
  })

  it('the system-dark block repeats the dark values', () => {
    const media = section.slice(section.indexOf('@media (prefers-color-scheme: dark)'))
    for (const [k, v] of Object.entries(dark)) expect(media.toUpperCase(), k).toContain(`${k}: ${v}`.toUpperCase())
  })
})
