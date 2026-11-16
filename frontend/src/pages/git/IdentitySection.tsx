import { useCallback, useEffect } from 'react'
import { Identity } from '../../api'
import { onEvent } from '../../api/events'
import { RepoProvider } from '../../api/models'
import type { IdentityOverview } from '../../api/models'
import { ErrorBanner, Skeleton } from '../../components'
import type { ProviderKind } from '../../components/ProviderMark'
import { useAsync } from '../../lib/useAsync'
import { useRepos } from '../../state/repos'
import { EnvNotes } from './EnvNotes'
import { GlobalIdentityCard } from './GlobalIdentityCard'
import { IdentityTable } from './IdentityTable'
import './identity.css'

const PROVIDERS: Partial<Record<RepoProvider, ProviderKind>> = {
  [RepoProvider.ProviderCodeCommit]: 'aws',
  [RepoProvider.ProviderGitHub]: 'github',
  [RepoProvider.ProviderBitbucket]: 'bitbucket',
}

export interface RepoMeta {
  provider?: ProviderKind
  remoteName?: string
}

/** The identity part of the Git settings page (design 5.12): global card, ladder, terminal notes and the effective identity table. */
export function IdentitySection() {
  const { state: repos } = useRepos()
  const ov = useAsync<IdentityOverview>(() => Identity.GetOverview(), [])
  const reload = ov.reload

  // Every write and undo emits identity:changed: read the table again.
  useEffect(() => onEvent('identity:changed', () => void reload()), [reload])

  const metaOf = useCallback(
    (repoId: string, name: string): RepoMeta => {
      const r = (repos?.repos ?? []).find((x) => x.id === repoId) ?? (repos?.repos ?? []).find((x) => x.localName === name)
      return r ? { provider: PROVIDERS[r.provider], remoteName: r.remoteName || undefined } : {}
    },
    [repos],
  )

  const o = ov.data
  return (
    <>
      {ov.error && <ErrorBanner error={ov.error} onRetry={() => void reload()} />}
      {!o && ov.loading && (
        <div className="stack" aria-busy="true">
          <Skeleton variant="block" height={150} />
          <Skeleton variant="block" height={240} />
        </div>
      )}
      {o && (
        <>
          <GlobalIdentityCard global={o.global} />
          <EnvNotes env={o.env} />
          <IdentityTable overview={o} metaOf={metaOf} reload={() => void reload()} />
        </>
      )}
    </>
  )
}
