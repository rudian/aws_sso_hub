import { KeyRound, TriangleAlert } from 'lucide-react'
import { StaticState } from '../../api/models'
import type { Profile } from '../../api/models'
import { AcctTile } from '../../components/AcctTile'
import { Badge } from '../../components/Badge'
import { Button } from '../../components/Button'
import { CodeText, codeMark } from '../../components/CodeText'
import { Skeleton } from '../../components/Skeleton'
import { Tooltip } from '../../components/Tooltip'
import { useI18n } from '../../i18n'
import { useNavigate } from '../../lib/route'
import { Gated } from './hooks'
import { impactLine, primaryMigrateProfile } from './model'
import './accounts.css'

/** Profiles that still use long-term access keys: one compact pass each, one primary migrate button. */
export function LegacyPanel({ profiles, readOnly }: { profiles: Profile[]; readOnly: boolean }) {
  const { t } = useI18n()
  const navigate = useNavigate()
  const primary = primaryMigrateProfile(profiles)
  if (profiles.length === 0 || !primary) return null
  return (
    <section className="legacy" aria-labelledby="legacy-title">
      <div className="legacy__head">
        <TriangleAlert aria-hidden="true" />
        <div style={{ flex: 1 }}>
          <h2 className="legacy__title" id="legacy-title">
            {t('legacy.title', { n: profiles.length })}
          </h2>
          <p className="legacy__text">
            <CodeText text={t('legacy.body', { file: codeMark('~/.aws/credentials'), cmd: codeMark(`--profile ${primary.name}`) })} />
          </p>
        </div>
        <button type="button" className="link" style={{ fontSize: 13, whiteSpace: 'nowrap' }} onClick={() => navigate({ page: 'tutorial', section: 'why' })}>
          {t('legacy.why')}
        </button>
      </div>
      <div className="legacy__grid">
        {profiles.map((p) => {
          const unverified = p.staticState === StaticState.StaticUnverified || !p.staticState
          const invalid = p.staticState === StaticState.StaticInvalid
          return (
            <article key={p.name} className="pass pass--static pass--compact" data-acct={p.colorSlot || undefined} data-profile={p.name}>
              <span className="pass__stripe" />
              <AcctTile slot={p.colorSlot} monogram={p.monogram} />
              <div style={{ flex: 1, minWidth: 0 }}>
                <div className="pass__name">
                  {p.name}
                  <Badge kind="warn" icon={KeyRound}>
                    {t('legacy.badge')}
                  </Badge>
                  {invalid && (
                    <Tooltip content={t('legacy.invalid.tooltip', { code: p.staticError ?? '' })}>
                      <span tabIndex={0}>
                        <Badge kind="danger">{t('legacy.invalid')}</Badge>
                      </span>
                    </Tooltip>
                  )}
                </div>
                <dl className="pass__meta" style={{ margin: '6px 0 0' }}>
                  <dt>{t('card.label.account')}</dt>
                  <dd>
                    {unverified ? (
                      <>
                        <Skeleton variant="text" width={104} />
                        <span className="sr-only">{t('legacy.unverified')}</span>
                      </>
                    ) : invalid || !p.accountId ? (
                      <span className="muted" style={{ fontFamily: 'var(--font-sans)' }}>
                        {t('card.account.unknown')}
                      </span>
                    ) : (
                      p.accountId
                    )}
                  </dd>
                  <dt>{t('card.label.region')}</dt>
                  <dd>{p.region}</dd>
                </dl>
                <div className="pass__role" style={{ marginTop: 6 }}>
                  {impactLine(p, t)}
                </div>
              </div>
              <Gated blocked={readOnly}>
                <Button variant={p.name === primary.name ? 'primary' : 'secondary'} onClick={() => navigate({ page: 'migrate', profile: p.name })}>
                  {t('legacy.migrate')}
                </Button>
              </Gated>
            </article>
          )
        })}
      </div>
    </section>
  )
}
