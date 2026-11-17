// Link rules (ui-design 5.7 "Terminal"): https:// opens in the browser (Cmd/Ctrl+click); http://localhost and
// http://127.0.0.1 open after a one-time confirm per port; every other http:// link is copied, not opened.
// Nothing here builds HTML; the result is a decision the caller acts on.

export type LinkDecision =
  | { kind: 'open'; url: string }
  | { kind: 'confirm-local'; url: string; port: string }
  | { kind: 'copy'; url: string }
  | { kind: 'ignore' }

export function classifyLink(raw: string): LinkDecision {
  let u: URL
  try {
    u = new URL(raw)
  } catch {
    return { kind: 'ignore' }
  }
  if (u.username || u.password) return { kind: 'copy', url: raw }
  if (u.protocol === 'https:') return { kind: 'open', url: u.href }
  if (u.protocol === 'http:') {
    if (u.hostname === 'localhost' || u.hostname === '127.0.0.1') {
      return { kind: 'confirm-local', url: u.href, port: u.port || '80' }
    }
    return { kind: 'copy', url: raw }
  }
  return { kind: 'ignore' }
}

export interface PathMatch {
  path: string
  line: number
  column?: number
  /** index of the first character of the match in the text, and its length */
  index: number
  length: number
}

// A relative or absolute file path ending in :line or :line:col. Needs a "/" or a file extension so "10:30" is not a path.
const PATH_LINE = /(?:^|(?<=[\s("'`[]))((?:~\/|\.{1,2}\/|\/)?[\w@.+-]+(?:\/[\w@.+-]+)*\.[A-Za-z][\w]{0,9}|(?:~\/|\.{1,2}\/|\/)[\w@.+-]+(?:\/[\w@.+-]+)*):(\d{1,6})(?::(\d{1,5}))?/g

/** File paths with a line number in one line of terminal text (they open in the Code tab). */
export function findPathLinks(text: string): PathMatch[] {
  const out: PathMatch[] = []
  PATH_LINE.lastIndex = 0
  let m: RegExpExecArray | null
  while ((m = PATH_LINE.exec(text)) !== null) {
    const path = m[1]
    out.push({ path, line: Number(m[2]), column: m[3] ? Number(m[3]) : undefined, index: m.index, length: m[0].length })
  }
  return out
}

/** The part of an xterm buffer line that `lineText` reads (structural, so tests need no xterm). */
export interface CellLine {
  length: number
  getCell(x: number): { getChars(): string; getWidth(): number } | undefined
}

/**
 * The text of a buffer line, and for every UTF-16 index of it the 0-based cell column where that character starts.
 * A wide character takes two cells (the second has width 0 and no text), so string indices are not columns.
 */
export function lineText(line: CellLine): { text: string; cols: number[] } {
  let text = ''
  const cols: number[] = []
  for (let x = 0; x < line.length; x++) {
    const c = line.getCell(x)
    if (!c || c.getWidth() === 0) continue
    const ch = c.getChars() || ' '
    for (let i = 0; i < ch.length; i++) cols.push(x)
    text += ch
  }
  const trimmed = text.replace(/\s+$/, '')
  return { text: trimmed, cols: cols.slice(0, trimmed.length) }
}
