import { Fragment } from 'react'
import type { ReactNode } from 'react'

export type InlinePart = { kind: 'text' | 'bold' | 'code'; text: string }

/** Split text into plain, **bold** and `code` parts. Anything else is plain text: no HTML is ever produced. */
export function parseInline(text: string): InlinePart[] {
  const parts: InlinePart[] = []
  const re = /\*\*([^*]+)\*\*|`([^`]+)`/g
  let last = 0
  let m: RegExpExecArray | null
  while ((m = re.exec(text)) !== null) {
    if (m.index > last) parts.push({ kind: 'text', text: text.slice(last, m.index) })
    if (m[1] !== undefined) parts.push({ kind: 'bold', text: m[1] })
    else parts.push({ kind: 'code', text: m[2] })
    last = m.index + m[0].length
  }
  if (last < text.length) parts.push({ kind: 'text', text: text.slice(last) })
  return parts
}

export function Inline({ text }: { text: string }) {
  return (
    <>
      {parseInline(text).map((p, i) =>
        p.kind === 'bold' ? <b key={i}>{p.text}</b> : p.kind === 'code' ? <code key={i} className="mono">{p.text}</code> : <Fragment key={i}>{p.text}</Fragment>,
      )}
    </>
  )
}

/** A newline starts a new paragraph. */
export function Paragraphs({ text, className }: { text: string; className?: string }): ReactNode {
  return text.split('\n').map((line, i) => (
    <p key={i} className={className}>
      <Inline text={line} />
    </p>
  ))
}
