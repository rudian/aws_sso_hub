import clsx from 'clsx'
import { useI18n } from '../i18n'
import { matchRange } from '../lib/repoSearch'
import './RepoName.css'

/**
 * A repository's name: the local folder name first and, only when it differs, the remote name as secondary
 * text (design 5.2 "Two names"). Screen readers hear "api-main, remote name acme_api".
 */
export interface RepoNameProps {
  /** The folder's name on disk. */
  localName: string
  /** The last path segment of the origin URL without `.git`; empty when there is no remote. */
  remoteName?: string
  /** The search text: the letters that match are bold in the name (and in the remote tag when it matched). */
  query?: string
  className?: string
}

/** `text` with the part that matches `query` in bold. */
function Marked({ text, query }: { text: string; query?: string }) {
  const r = query ? matchRange(text, query) : null
  if (!r) return <>{text}</>
  return (
    <>
      {text.slice(0, r[0])}
      <b className="repo-nm__hit">{text.slice(r[0], r[1])}</b>
      {text.slice(r[1])}
    </>
  )
}

export function RepoName({ localName, remoteName, query, className }: RepoNameProps) {
  const { t } = useI18n()
  const differs = !!remoteName && remoteName !== localName
  // the tag's words around the name, so the matched letters of the name can be bold
  const tagParts = differs ? t('repoName.remote', { name: '\u0000' }).split('\u0000') : []
  return (
    <span className={clsx('repo-nm', className)}>
      <span className="repo-nm__local">
        <Marked text={localName} query={query} />
      </span>
      {differs && (
        <>
          <span className="sr-only">{t('repoName.remote.sr', { name: remoteName })}</span>
          <span className="remote-name" aria-hidden="true">
            {tagParts[0]}
            <Marked text={remoteName} query={query} />
            {tagParts[1]}
          </span>
        </>
      )}
    </span>
  )
}
