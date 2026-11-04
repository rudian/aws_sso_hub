import { describe, expect, it } from 'vitest'

// The Wails runtime's Browser.OpenURL and Clipboard.Text can be called by any script in the WebView and do not go
// through the app's own https-only OpenURL. The UI must use System.OpenURL and Clipboard.SetText only.
const modules = import.meta.glob<string>(['../**/*.ts', '../**/*.tsx', '!../**/*.test.ts', '!../**/*.test.tsx'], { query: '?raw', import: 'default', eager: true })

describe('Wails runtime usage', () => {
  const files = Object.keys(modules)
  const read = (f: string) => modules[f]

  it('finds the source tree', () => {
    expect(files.length).toBeGreaterThan(50)
  })

  it('no file imports Browser from @wailsio/runtime', () => {
    const bad = files.filter((f) => {
      const text = read(f)
      for (const m of text.matchAll(/import\s+(?:type\s+)?\{([^}]*)\}\s*from\s*['"]@wailsio\/runtime(?:\/[^'"]*)?['"]/g)) {
        if (/(^|[\s,])Browser(\s+as\s+\w+)?\s*(,|$)/.test(m[1])) return true
      }
      return /import\s+\*\s+as\s+\w+\s+from\s*['"]@wailsio\/runtime['"]/.test(text) || /Browser\.OpenURL/.test(text)
    })
    expect(bad).toEqual([])
  })

  it('nothing calls Clipboard.Text', () => {
    const bad = files.filter((f) => /Clipboard\s*\.\s*Text\s*\(/.test(read(f)))
    expect(bad).toEqual([])
  })

  it('the detector catches the forbidden forms', () => {
    const browserImport = /import\s+(?:type\s+)?\{([^}]*)\}\s*from\s*['"]@wailsio\/runtime(?:\/[^'"]*)?['"]/g
    const sample = "import { Clipboard, Browser as B } from '@wailsio/runtime'"
    const m = [...sample.matchAll(browserImport)][0]
    expect(/(^|[\s,])Browser(\s+as\s+\w+)?\s*(,|$)/.test(m[1])).toBe(true)
    expect(/Clipboard\s*\.\s*Text\s*\(/.test('await Clipboard.Text()')).toBe(true)
    expect(/Clipboard\s*\.\s*Text\s*\(/.test('await Clipboard.SetText(x)')).toBe(false)
  })
})
