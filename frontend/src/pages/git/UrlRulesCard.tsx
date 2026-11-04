import { useEffect, useRef, useState } from 'react'
import { GitGlobal } from '../../api'
import type { GitGlobalState, URLRule, URLRulePlan } from '../../api/models'
import { AcctChip, Button } from '../../components'
import { useI18n } from '../../i18n'
import { useRepos } from '../../state/repos'
import { useReportError } from '../repos/hooks'
import { UrlRuleModal } from './UrlRuleModal'

export interface UrlRulesCardProps {
  state: GitGlobalState
  reload: () => Promise<void>
  /** `open=urlRule`: open the migration of the first rule. */
  autoOpen: boolean
}

/** Card 2: global URL rules and "Bind locally". */
export function UrlRulesCard({ state, reload, autoOpen }: UrlRulesCardProps) {
  const { t } = useI18n()
  const { state: repos } = useRepos()
  const report = useReportError()
  const [plan, setPlan] = useState<URLRulePlan | null>(null)
  const [loading, setLoading] = useState<string | null>(null)
  const rules = state.urlRules ?? []

  const open = async (rule: URLRule) => {
    setLoading(rule.pattern)
    try {
      setPlan(await GitGlobal.PreviewURLRuleMigration(rule.pattern))
    } catch (e) {
      report(e)
    } finally {
      setLoading(null)
    }
  }

  const done = useRef(false)
  useEffect(() => {
    if (!autoOpen || done.current || rules.length === 0) return
    done.current = true
    void open(rules[0])
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [autoOpen, rules.length])

  return (
    <section className="card" id="git-url-rules" aria-labelledby="git-url-title">
      <div className="card__header">
        <h2 className="card__title" id="git-url-title">
          {t('git.url.title')}
        </h2>
      </div>
      <div className="card__body stack" style={{ '--gap': 'var(--sp-4)' } as React.CSSProperties}>
        <p style={{ margin: 0 }} className="muted">
          {t('git.url.explain')}
        </p>
        {rules.length === 0 ? (
          <div className="hint">{t('git.url.empty')}</div>
        ) : (
          <div className="card list" role="list">
            {rules.map((rule) => {
              const matched = (repos?.repos ?? []).filter((r) => (rule.matchedRepoIds ?? []).includes(r.id))
              return (
                <div key={rule.pattern} className="list__item" role="listitem" style={{ alignItems: 'flex-start', flexWrap: 'wrap' }}>
                  <div style={{ minWidth: 0, flex: 1 }}>
                    <div className="mono truncate" style={{ fontSize: 'var(--fs-12)' }} title={rule.pattern}>
                      {rule.pattern}
                    </div>
                    <details style={{ marginTop: 4 }}>
                      <summary className="hint" style={{ cursor: 'default' }}>
                        {t('git.url.matches', { n: (rule.matchedRepoIds ?? []).length })}
                      </summary>
                      <ul className="hint" style={{ margin: '4px 0 0', paddingLeft: 18 }}>
                        {matched.map((r) => (
                          <li key={r.id} className="mono">
                            {r.displayPath}
                          </li>
                        ))}
                      </ul>
                    </details>
                  </div>
                  <AcctChip slot={rule.colorSlot} monogram={rule.monogram} name={rule.profile} none={!rule.profileKnown} />
                  <Button size="sm" variant="secondary" loading={loading === rule.pattern} onClick={() => void open(rule)}>
                    {t('git.url.toLocal')}
                  </Button>
                </div>
              )
            })}
          </div>
        )}
      </div>
      {plan && (
        <UrlRuleModal
          plan={plan}
          onClose={() => setPlan(null)}
          onDone={() => {
            setPlan(null)
            void reload()
          }}
        />
      )}
    </section>
  )
}
