import { describe, expect, it } from 'vitest'

// jsdom has no layout, so this reads projects.css and checks the arithmetic: at every window width from the 980px
// minimum up, the active column template's minimums, gaps and row padding must fit the table row.

const { readFileSync } = (await import(/* @vite-ignore */ 'node:' + 'fs')) as { readFileSync(path: string, enc: string): string }
const css = readFileSync('src/pages/projects/projects.css', 'utf8')

const SIDEBAR = 232
const PAGE_PADDING = 64 // 2 x --sp-8
const SCROLLBAR = 10 // .app__main ::-webkit-scrollbar width
const TABLE_BORDER = 2
const ROW_PADDING = 32

function minimum(cols: string): number {
  return cols
    .split(/\s+(?![^()]*\))/)
    .map((c) => parseInt(/minmax\((\d+)px/.exec(c)?.[1] ?? c, 10))
    .reduce((a, b) => a + b, 0)
}

function rule(media: string | null): { cols: string; gap: number } | null {
  const scope = media === null ? css.slice(0, css.indexOf('@media')) : css.slice(css.indexOf(media))
  const block = media === null ? scope : scope.slice(0, scope.indexOf('\n}'))
  const cols = /\.proj-table \{ --cols: ([^;]+);/.exec(block)
  if (!cols) return null
  const gap = /\.row \{ column-gap: (\d+)px/.exec(block)
  return { cols: cols[1], gap: gap ? Number(gap[1]) : 16 }
}

describe('Projects table fits the window', () => {
  const wide = rule(null)!
  const narrow = rule('@media (max-width: 1320px)')!

  function active(w: number) {
    return w <= 1320 ? narrow : wide
  }

  it('finds both templates', () => {
    expect(wide && narrow).toBeTruthy()
  })

  it('every width from 980 to 1600 has a template whose minimums fit the row', () => {
    for (let w = 980; w <= 1600; w++) {
      const t = active(w)
      const tracks = t.cols.split(/\s+(?![^()]*\))/).length
      const need = minimum(t.cols) + t.gap * (tracks - 1) + ROW_PADDING
      const have = w - SIDEBAR - PAGE_PADDING - SCROLLBAR - TABLE_BORDER
      expect(need, `window ${w}`).toBeLessThanOrEqual(have)
    }
  })
})
