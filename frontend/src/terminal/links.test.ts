import { describe, expect, it } from 'vitest'
import { classifyLink, findPathLinks, lineText } from './links'
import { countLines, needsPasteConfirm, sanitizePaste } from './paste'

describe('classifyLink', () => {
  it('opens https', () => {
    expect(classifyLink('https://example.com/a?b=1')).toEqual({ kind: 'open', url: 'https://example.com/a?b=1' })
  })
  it('asks once per port for local http', () => {
    expect(classifyLink('http://localhost:5173/')).toEqual({ kind: 'confirm-local', url: 'http://localhost:5173/', port: '5173' })
    expect(classifyLink('http://127.0.0.1:8080/x')).toMatchObject({ kind: 'confirm-local', port: '8080' })
    expect(classifyLink('http://localhost/')).toMatchObject({ kind: 'confirm-local', port: '80' })
  })
  it('copies other http links and lookalike hosts', () => {
    expect(classifyLink('http://example.com/')).toEqual({ kind: 'copy', url: 'http://example.com/' })
    expect(classifyLink('http://localhost.example.com:3000/')).toMatchObject({ kind: 'copy' })
    expect(classifyLink('http://127.0.0.1.example.com/')).toMatchObject({ kind: 'copy' })
    expect(classifyLink('http://localhost@evil.example.com/')).toMatchObject({ kind: 'copy' })
  })
  it('ignores everything else', () => {
    for (const u of ['javascript:alert(1)', 'file:///etc/passwd', 'ftp://example.com', 'data:text/html,x', 'not a url', '']) {
      expect(classifyLink(u)).toEqual({ kind: 'ignore' })
    }
  })
})

describe('findPathLinks', () => {
  it('finds paths with a line number', () => {
    const m = findPathLinks('error in src/pages/App.tsx:42:7 and ./a/b.go:9')
    expect(m.map((x) => [x.path, x.line, x.column])).toEqual([
      ['src/pages/App.tsx', 42, 7],
      ['./a/b.go', 9, undefined],
    ])
    expect('error in src/pages/App.tsx:42:7'.slice(m[0].index, m[0].index + m[0].length)).toBe('src/pages/App.tsx:42:7')
  })
  it('does not take times, ratios or URL parts', () => {
    expect(findPathLinks('at 10:30 ratio 3:4 https://example.com/a.js:10 x')).toEqual([])
  })
})

describe('paste confirm', () => {
  it('asks for more than 5 lines only', () => {
    expect(countLines('a\nb\nc\nd\ne')).toBe(5)
    expect(needsPasteConfirm('a\nb\nc\nd\ne')).toBe(false)
    expect(needsPasteConfirm('a\nb\nc\nd\ne\nf')).toBe(true)
    expect(needsPasteConfirm('a\r\nb\r\nc\r\nd\r\ne\r\nf\r\n')).toBe(true)
    expect(needsPasteConfirm('a\nb\nc\nd\ne\n')).toBe(false)
    expect(countLines('')).toBe(0)
  })
})

describe('sanitizePaste', () => {
  it('removes ESC and other control characters but keeps tab, CR and LF', () => {
    expect(sanitizePaste('ls\x1b[201~\rcurl x\r')).toBe('ls[201~\rcurl x\r')
    expect(sanitizePaste('a\tb\r\nc\x00\x07\x7f\x9b')).toBe('a\tb\r\nc')
  })
})

describe('lineText', () => {
  it('maps string indices to cell columns across wide characters', () => {
    // 已(2 cells) 修(2) 改(2) space s r c
    const cells = [['已', 2], ['', 0], ['修', 2], ['', 0], ['改', 2], ['', 0], [' ', 1], ['s', 1], ['r', 1], ['c', 1], ['', 1]] as const
    const line = { length: cells.length, getCell: (x: number) => ({ getChars: () => cells[x][0], getWidth: () => cells[x][1] }) }
    const { text, cols } = lineText(line)
    expect(text).toBe('已修改 src')
    expect(text.indexOf('src')).toBe(4)
    expect(cols[4]).toBe(7)
  })
})
