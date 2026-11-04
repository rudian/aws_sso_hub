import { Fragment } from 'react'

const OPEN = '\u0002'
const CLOSE = '\u0003'

/** Mark a value so that CodeText renders it in the monospace style when it is interpolated into a translated sentence. */
export function codeMark(value: string | number): string {
  return `${OPEN}${String(value).replace(/[\u0002\u0003]/g, '')}${CLOSE}`
}

/** Split a sentence that contains codeMark()ed values into plain text and code parts. */
export function splitCodeText(text: string): { code: boolean; text: string }[] {
  const out: { code: boolean; text: string }[] = []
  let rest = text
  while (rest.length > 0) {
    const a = rest.indexOf(OPEN)
    const b = a >= 0 ? rest.indexOf(CLOSE, a + 1) : -1
    if (a < 0 || b < 0) {
      out.push({ code: false, text: rest.replace(/[\u0002\u0003]/g, '') })
      break
    }
    if (a > 0) out.push({ code: false, text: rest.slice(0, a) })
    out.push({ code: true, text: rest.slice(a + 1, b) })
    rest = rest.slice(b + 1)
  }
  return out
}

/** A translated sentence whose codeMark()ed values (paths, profile names, flags) are shown in monospace. */
export function CodeText({ text }: { text: string }) {
  return (
    <>
      {splitCodeText(text).map((p, i) =>
        p.code ? (
          <code key={i} className="mono nowrap">
            {p.text}
          </code>
        ) : (
          <Fragment key={i}>{p.text}</Fragment>
        ),
      )}
    </>
  )
}
