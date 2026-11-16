import type { Repo } from '../../api/models'
import { RepoName } from '../../components/RepoName'
import { useI18n } from '../../i18n'
import type { MessageKey } from '../../i18n'

/**
 * A translated sentence with a {repo} placeholder where the repository is named by RepoName: the local folder name
 * first and the remote name as its tag (plan 4.4a). `plain` is the same sentence as text for aria-labels.
 */
export function useRepoSentence() {
  const { t } = useI18n()
  const mark = '\u0000'
  return {
    plain: (key: MessageKey, repo: Pick<Repo, 'localName'>, params: Record<string, string | number> = {}) => t(key, { ...params, repo: repo.localName }),
    node: (key: MessageKey, repo: Pick<Repo, 'localName' | 'remoteName'>, params: Record<string, string | number> = {}) => {
      const [pre, post = ''] = t(key, { ...params, repo: mark }).split(mark)
      return (
        <>
          {pre}
          <RepoName localName={repo.localName} remoteName={repo.remoteName} />
          {post}
        </>
      )
    },
  }
}
