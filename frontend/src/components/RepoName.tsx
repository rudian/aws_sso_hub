/**
 * A repository's name: the local folder name first and, only when it differs, the remote name as secondary
 * text (design 5.2 "Two names").
 *
 * Stub until F1-19: it shows the local folder name only. F1-19 replaces this file and keeps these props, so
 * every list can use it now.
 */
export interface RepoNameProps {
  /** The folder's name on disk. */
  localName: string
  /** The last path segment of the origin URL without `.git`; empty when there is no remote. */
  remoteName?: string
  className?: string
}

export function RepoName({ localName, className }: RepoNameProps) {
  return <span className={className}>{localName}</span>
}
