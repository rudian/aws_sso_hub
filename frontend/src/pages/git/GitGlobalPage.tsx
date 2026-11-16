import { useEffect, useRef } from 'react'
import { GitGlobal } from '../../api'
import type { GitGlobalState } from '../../api/models'
import { ErrorBanner, Skeleton } from '../../components'
import { useI18n } from '../../i18n'
import { takeOpen } from '../../lib/initialUI'
import { useRefreshHandler } from '../../lib/keyboard'
import { useAsync } from '../../lib/useAsync'
import { GlobalBanner } from '../../shell/GlobalBanner'
import { useRepos } from '../../state/repos'
import { DefaultCard } from './DefaultCard'
import { IdentitySection } from './IdentitySection'
import { SavedCard } from './SavedCard'
import { UrlRulesCard } from './UrlRulesCard'

export function GitGlobalPage() {
  const { t } = useI18n()
  const { state: repos } = useRepos()
  const git = useAsync<GitGlobalState>(() => GitGlobal.GetState(), [])
  useRefreshHandler(() => void git.reload())

  // A bind, unbind or repair elsewhere changes the matched repositories and the counts.
  const first = useRef(true)
  useEffect(() => {
    if (first.current) {
      first.current = false
      return
    }
    void git.reload()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [repos])

  // ?open= (mock screenshots): consumed once.
  const open = useRef<{ remove: boolean; urlRule: boolean } | null>(null)
  if (open.current === null) open.current = { remove: takeOpen('gitDefault:remove') !== null, urlRule: takeOpen('urlRule') !== null }

  // Scroll to the URL-rule card and flash it (removal is blocked until the rules are dealt with).
  const showUrlRules = () => {
    window.setTimeout(() => {
      const el = document.getElementById('git-url-rules')
      el?.scrollIntoView({ block: 'center' })
      el?.classList.add('card--flash')
      window.setTimeout(() => el?.classList.remove('card--flash'), 2400)
    }, 0)
  }

  const s = git.data
  return (
    <div className="page">
      <header className="page__header">
        <div>
          <h1 className="page__title">{t('identity.title')}</h1>
          <p className="page__subtitle">{t('identity.subtitle')}</p>
        </div>
      </header>
      <GlobalBanner />
      <div className="page__body">
        <IdentitySection />
        {git.error && <ErrorBanner error={git.error} onRetry={() => void git.reload()} />}
        {!s && git.loading && (
          <div className="stack" aria-busy="true">
            <Skeleton variant="block" height={220} />
            <Skeleton variant="block" height={120} />
          </div>
        )}
        {s && (
          <>
            <DefaultCard state={s} reload={git.reload} autoRemove={open.current.remove} onBlockedRemove={showUrlRules} />
            <UrlRulesCard state={s} reload={git.reload} autoOpen={open.current.urlRule} />
            <SavedCard supported={s.savedCredentialsSupported} />
            <details className="card" style={{ padding: 'var(--sp-4) var(--sp-5)' }}>
              <summary style={{ cursor: 'default', fontWeight: 500 }}>{t('git.other.title')}</summary>
              <div className="stack" style={{ marginTop: 'var(--sp-3)', '--gap': 'var(--sp-1)' } as React.CSSProperties}>
                {(s.otherHelpers ?? []).length === 0 ? (
                  <div className="hint">{t('git.other.empty')}</div>
                ) : (
                  (s.otherHelpers ?? []).map((h, i) => (
                    <div key={i} className="mono" style={{ fontSize: 'var(--fs-12)' }}>
                      <span className="muted">
                        [{h.scope}] {h.origin}
                      </span>
                      <br />
                      {h.key} = {h.value}
                    </div>
                  ))
                )}
              </div>
            </details>
          </>
        )}
      </div>
    </div>
  )
}
