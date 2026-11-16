import { CircleCheck, RefreshCw } from 'lucide-react'
import { useState } from 'react'
import { MigrationStep } from '../../api/models'
import { toUiError } from '../../api/errors'
import { System } from '../../api/services'
import { Button } from '../../components/Button'
import { ErrorBanner } from '../../components/ErrorBanner'
import { RelativeTime } from '../../components/RelativeTime'
import { Skeleton } from '../../components/Skeleton'
import { errorText, useI18n } from '../../i18n'
import { useRefreshHandler } from '../../lib/keyboard'
import { useNavigate } from '../../lib/route'
import { useAsync } from '../../lib/useAsync'
import { useActions } from '../../shell/AppActions'
import { GlobalBanner } from '../../shell/GlobalBanner'
import { useAccounts } from '../../state/accounts'
import { useEnv } from '../../state/env'
import { EnvChecks } from './EnvChecks'
import { ToolRows } from './ToolRows'
import { describeConfigIssue, describeFinding } from './findings'
import type { FindingAction } from './findings'
import { StatusRow } from './StatusRow'
import './health.css'

export function HealthPage() {
  const { t } = useI18n()
  const navigate = useNavigate()
  const { toast } = useActions()
  const env = useEnv()
  const accounts = useAccounts()
  const health = useAsync(() => System.CheckHealth(), [])
  const [rechecking, setRechecking] = useState(false)
  const [toolsKey, setToolsKey] = useState(0)

  const recheck = async () => {
    setRechecking(true)
    try {
      setToolsKey((k) => k + 1)
      await Promise.all([env.recheck(), health.reload()])
    } finally {
      setRechecking(false)
    }
  }
  useRefreshHandler(() => void recheck())

  const run = async (a: FindingAction) => {
    try {
      switch (a.kind) {
        case 'reveal':
          await System.RevealPath(a.file)
          break
        case 'remove-key':
          navigate({ page: 'migrate', profile: a.profile, step: MigrationStep.MigrationRemoveKey })
          break
        case 'tutorial':
          navigate({ page: 'tutorial', section: a.section })
          break
        case 'git':
          navigate({ page: 'git' })
          break
        case 'repos':
          navigate({ page: 'repos' })
          break
        case 'open-config':
          await System.OpenAWSConfig()
          break
      }
    } catch (e) {
      const text = errorText(toUiError(e), t)
      toast({ kind: 'danger', title: text.title, body: text.body })
    }
  }

  const findings = health.data?.findings ?? []
  const configIssue = accounts.state?.readOnly ? describeConfigIssue(accounts.state.issues ?? [], t) : null

  return (
    <div className="page">
      <header className="page__header">
        <div>
          <h1 className="page__title">{t('health.title')}</h1>
          <p className="page__subtitle">{t('health.subtitle')}</p>
        </div>
        <div className="page__actions">
          <Button icon={RefreshCw} loading={rechecking} onClick={() => void recheck()}>
            {t('health.recheck')}
          </Button>
        </div>
      </header>
      <GlobalBanner />
      <div className="page__body">
        <section className="card" aria-labelledby="health-env">
          <div className="card__header">
            <h2 className="card__title" id="health-env">
              {t('health.group.env')}
            </h2>
          </div>
          <ToolRows reloadKey={toolsKey} />
          <EnvChecks hideTools />
        </section>

        <section className="card" aria-labelledby="health-sec">
          <div className="card__header">
            <h2 className="card__title" id="health-sec">
              {t('health.group.security')}
            </h2>
            {health.data?.checkedAt && (
              <span className="hgroup__caption">
                {t('health.checkedAt')} <RelativeTime iso={health.data.checkedAt} />
              </span>
            )}
          </div>
          {health.error && !health.data ? (
            <div className="card__body">
              <ErrorBanner error={health.error} onRetry={() => void health.reload()} />
            </div>
          ) : !health.data ? (
            <div className="list" aria-busy="true">
              {[0, 1].map((i) => (
                <div key={i} className="list__item">
                  <Skeleton variant="text" width="70%" />
                </div>
              ))}
            </div>
          ) : findings.length === 0 && !configIssue ? (
            <div className="health__good" role="status">
              <span className="hrow__icon hrow__icon--ok">
                <CircleCheck aria-hidden="true" />
              </span>
              <div>
                <div className="hrow__title">{t('health.allGood')}</div>
                <div className="hrow__text">{t('health.allGood.body')}</div>
              </div>
            </div>
          ) : (
            <div className="list">
              {configIssue && (
                <StatusRow
                  status={configIssue.status}
                  title={configIssue.title}
                  text={configIssue.text}
                  action={
                    configIssue.action && configIssue.actionLabel ? (
                      <Button size="sm" onClick={() => void run(configIssue.action!)}>
                        {configIssue.actionLabel}
                      </Button>
                    ) : undefined
                  }
                />
              )}
              {findings.map((f) => {
                const d = describeFinding(f, t)
                const action = d.action
                return (
                  <StatusRow
                    key={f.id}
                    status={d.status}
                    title={d.title}
                    text={d.text}
                    action={
                      action && d.actionLabel ? (
                        <Button size="sm" onClick={() => void run(action)}>
                          {d.actionLabel}
                        </Button>
                      ) : undefined
                    }
                  />
                )
              })}
            </div>
          )}
        </section>
      </div>
    </div>
  )
}
