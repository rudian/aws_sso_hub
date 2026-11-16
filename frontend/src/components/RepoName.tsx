import clsx from 'clsx'
import { useI18n } from '../i18n'
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
  className?: string
}

export function RepoName({ localName, remoteName, className }: RepoNameProps) {
  const { t } = useI18n()
  const differs = !!remoteName && remoteName !== localName
  return (
    <span className={clsx('repo-nm', className)}>
      <span className="repo-nm__local">{localName}</span>
      {differs && (
        <>
          <span className="sr-only">{t('repoName.remote.sr', { name: remoteName })}</span>
          <span className="remote-name" aria-hidden="true">
            {t('repoName.remote', { name: remoteName })}
          </span>
        </>
      )}
    </span>
  )
}
