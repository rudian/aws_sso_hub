import type { ITheme } from '@xterm/xterm'

function tok(cs: CSSStyleDeclaration, name: string, fallback: string): string {
  const v = cs.getPropertyValue(name).trim()
  return v || fallback
}

function rgb(hex: string): [number, number, number] | null {
  const m = /^#([0-9a-f]{6})$/i.exec(hex.trim())
  if (!m) return null
  const n = parseInt(m[1], 16)
  return [(n >> 16) & 255, (n >> 8) & 255, n & 255]
}

/** `a` mixed with `b`; `t` is how much of `b` (0..1). Not a hex colour: `a` comes back as it is. */
export function mix(a: string, b: string, t: number): string {
  const x = rgb(a)
  const y = rgb(b)
  if (!x || !y) return a
  const c = x.map((v, i) => Math.round(v + (y[i] - v) * t))
  return '#' + c.map((v) => v.toString(16).padStart(2, '0')).join('').toUpperCase()
}

function alpha(a: string, t: number, fallback: string): string {
  const x = rgb(a)
  return x ? `rgba(${x[0]}, ${x[1]}, ${x[2]}, ${t})` : fallback
}

/** The xterm theme from the --term-* tokens (F1-08; tokens.css). Re-read it when data-theme changes. */
export function readTermTheme(el: Element = document.documentElement): ITheme {
  const cs = getComputedStyle(el)
  const bg = tok(cs, '--term-bg', '#151A21')
  const fg = tok(cs, '--term-fg', '#D5DBE4')
  const dim = tok(cs, '--term-dim', '#8691A1')
  const line = tok(cs, '--term-line', '#2A323D')
  const ok = tok(cs, '--term-ok', '#5FD3A2')
  const warn = tok(cs, '--term-warn', '#F0B35C')
  const info = tok(cs, '--term-info', '#8DB3FF')
  const danger = tok(cs, '--term-danger', '#F58A80')
  const accent = tok(cs, '--term-accent', '#C9BBFF')
  const bright = (c: string) => mix(c, '#FFFFFF', 0.25)
  // The tokens have no cyan, no bright colours and no pure white: derive them so tools that tell blue from cyan, or
  // normal from bright, keep that information. The selection is info at 16%, as in the mockup.
  const cyan = mix(info, ok, 0.5)
  return {
    background: bg,
    foreground: fg,
    cursor: fg,
    cursorAccent: bg,
    selectionBackground: alpha(info, 0.16, line),
    black: line,
    red: danger,
    green: ok,
    yellow: warn,
    blue: info,
    magenta: accent,
    cyan,
    white: fg,
    brightBlack: dim,
    brightRed: bright(danger),
    brightGreen: bright(ok),
    brightYellow: bright(warn),
    brightBlue: bright(info),
    brightMagenta: bright(accent),
    brightCyan: bright(cyan),
    brightWhite: '#FFFFFF',
  }
}
