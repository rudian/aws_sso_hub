import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

// The leave motion animates `.app`'s grid columns. A from-keyframe whose track types differ from the grid's own
// (`minmax(0, 1fr)` against `1fr`) is interpolated wrongly by Chromium, which is the engine behind WebView2.
const read = (...p: string[]) => readFileSync(resolve(process.cwd(), 'src', ...p), 'utf8').replace(/\/\*[\s\S]*?\*\//g, '')
const workspace = read('workspace', 'workspace.css')
const tokens = read('styles', 'tokens.css')

/** Split a track list at top-level spaces ("var(--a) minmax(0, 1fr)" -> ["var(--a)", "minmax(0, 1fr)"]). */
function tracks(list: string): string[] {
  const out: string[] = []
  let depth = 0
  let cur = ''
  for (const ch of list.trim()) {
    if (ch === '(') depth++
    if (ch === ')') depth--
    if (ch === ' ' && depth === 0) {
      if (cur) out.push(cur)
      cur = ''
    } else cur += ch
  }
  if (cur) out.push(cur)
  return out
}
const kind = (track: string) => (track.startsWith('var(') || /^[\d.]+px$/.test(track) ? 'length' : track)

describe('workspace motion css', () => {
  it('ws-widen starts from the same track types as the grid of .app', () => {
    const from = /@keyframes ws-widen\s*\{\s*from\s*\{\s*grid-template-columns:\s*([^;}]+);/.exec(workspace)?.[1]
    const grid = /\n\.app\s*\{[^}]*?grid-template-columns:\s*([^;}]+);/.exec(tokens)?.[1]
    expect(from).toBeTruthy()
    expect(grid).toBeTruthy()
    expect(tracks(from!).map(kind)).toEqual(tracks(grid!).map(kind))
  })

  it('clips the sidebar and fades its content while it widens', () => {
    expect(workspace).toMatch(/\.app--from-ws \.app__sidebar\s*\{[^}]*overflow:\s*hidden/)
    expect(workspace).toMatch(/\.app--from-ws \.app__sidebar > \*\s*\{[^}]*min-width:\s*var\(--sidebar-w\)[^}]*fade-in/)
  })

  it('the bottom panel never draws taller than 60% of the window, whatever the window size became', () => {
    expect(workspace).toMatch(/\.bpanel\s*\{[^}]*max-height:\s*60vh/)
  })
})
