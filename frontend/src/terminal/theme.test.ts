import { describe, expect, it } from 'vitest'
import { mix, readTermTheme } from './theme'

describe('readTermTheme', () => {
  it('gives the 16 ANSI colours distinct values', () => {
    const t = readTermTheme()
    const names = ['black', 'red', 'green', 'yellow', 'blue', 'magenta', 'cyan', 'white', 'brightBlack', 'brightRed', 'brightGreen', 'brightYellow', 'brightBlue', 'brightMagenta', 'brightCyan', 'brightWhite'] as const
    const values = names.map((n) => t[n])
    expect(new Set(values).size).toBe(16)
    expect(t.cyan).not.toBe(t.blue)
    expect(t.brightWhite).toBe('#FFFFFF')
    expect(t.selectionBackground).toMatch(/^rgba\(\d+, \d+, \d+, 0\.16\)$/)
  })
  it('mixes hex colours and leaves others alone', () => {
    expect(mix('#000000', '#FFFFFF', 0.5)).toBe('#808080')
    expect(mix('red', '#FFFFFF', 0.5)).toBe('red')
  })
})
