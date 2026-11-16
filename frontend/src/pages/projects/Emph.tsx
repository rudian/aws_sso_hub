import { Fragment } from 'react'
import { splitCodeText } from '../../components/CodeText'

/** A sentence whose codeMark()ed values are shown in bold (the prefix in a suggestion, the count in the found strip). */
export function Emph({ text }: { text: string }) {
  return (
    <>
      {splitCodeText(text).map((p, i) => (p.code ? <b key={i}>{p.text}</b> : <Fragment key={i}>{p.text}</Fragment>))}
    </>
  )
}
