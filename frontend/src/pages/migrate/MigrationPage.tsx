import { ChevronLeft } from 'lucide-react'
import { useCallback, useEffect, useState } from 'react'
import { isCancel, toUiError } from '../../api/errors'
import type { UiError } from '../../api/errors'
import { onEvent } from '../../api/events'
import { Migration } from '../../api/services'
import { ErrorCode, MigrationStep } from '../../api/models'
import type { MigrationState } from '../../api/models'
import { AcctTile } from '../../components/AcctTile'
import { Button } from '../../components/Button'
import { EmptyState } from '../../components/EmptyState'
import { ErrorBanner } from '../../components/ErrorBanner'
import { Skeleton } from '../../components/Skeleton'
import { Stepper } from '../../components/Stepper'
import { useI18n } from '../../i18n'
import { useNavigate, useRoute } from '../../lib/route'
import { GlobalBanner } from '../../shell/GlobalBanner'
import { useActions } from '../../shell/AppActions'
import { useAccounts } from '../../state/accounts'
import { StepAccount } from './StepAccount'
import { StepIdentify } from './StepIdentify'
import { StepDeactivate, StepRemoveKey } from './StepKeys'
import { StepPortal } from './StepPortal'
import { StepReview } from './StepReview'
import { StepVerify } from './StepVerify'
import { currentRailStep, railSteps } from './model'
import type { ItemProgress, WizardCtx } from './wizard'
import './migrate.css'
import { CodeText, codeMark } from '../../components/CodeText'

/** Migration wizard: the whole flow for one static-key profile, driven by the backend state machine. */
export function MigrationPage() {
  const route = useRoute()
  if (!route.profile) return <NoProfile />
  return <Wizard key={route.profile} name={route.profile} />
}

function NoProfile() {
  const { t } = useI18n()
  const navigate = useNavigate()
  return (
    <div className="page">
      <header className="page__header">
        <div>
          <h1 className="page__title">{t('nav.accounts')}</h1>
        </div>
      </header>
      <GlobalBanner />
      <EmptyState
        title={t('migrate.notFound')}
        actions={
          <Button variant="primary" onClick={() => navigate({ page: 'accounts' })}>
            {t('nav.accounts')}
          </Button>
        }
      />
    </div>
  )
}

function Wizard({ name }: { name: string }) {
  const { t } = useI18n()
  const navigate = useNavigate()
  const { toast } = useActions()
  const { state: accounts } = useAccounts()
  const [st, setSt] = useState<MigrationState | null>(null)
  const [view, setView] = useState<MigrationStep>(MigrationStep.MigrationIdentify)
  const [loadError, setLoadError] = useState<UiError | null>(null)
  const [error, setError] = useState<UiError | null>(null)
  const [busy, setBusy] = useState(false)
  const [applying, setApplying] = useState(false)
  const [progress, setProgress] = useState<WizardCtx['progress']>({})
  const [attempt, setAttempt] = useState(0)

  useEffect(() => {
    let alive = true
    setLoadError(null)
    Migration.Start(name).then(
      (s) => {
        if (!alive) return
        if (s.step === MigrationStep.MigrationDone) {
          navigate({ page: 'accounts' })
          return
        }
        setSt(s)
        setView(s.step)
      },
      (e) => alive && !isCancel(e) && setLoadError(toUiError(e)),
    )
    return () => {
      alive = false
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [name, attempt])

  useEffect(
    () =>
      onEvent('migration:progress', (p) => {
        if (p.profile !== name) return
        const entry: ItemProgress = { status: p.status, detail: p.detail }
        setProgress((cur) => ({ ...cur, [p.item]: entry }))
      }),
    [name],
  )

  const run = useCallback<WizardCtx['run']>(async (fn, opts) => {
    setBusy(true)
    if (!opts?.keepError) setError(null)
    try {
      const s = await fn()
      setSt(s)
      setView(s.step)
      return s
    } catch (e) {
      const ue = toUiError(e)
      if (!isCancel(e)) {
        setError(ue)
        opts?.onError?.(ue)
      }
      return null
    } finally {
      setBusy(false)
    }
  }, [])

  const startApply = async () => {
    if (!st?.configPreview) return
    setApplying(true)
    setProgress({})
    setError(null)
    setView(MigrationStep.MigrationVerify)
    try {
      const s = await Migration.Apply(name, st.configPreview.baseHash)
      setSt(s)
      setView(s.step)
    } catch (e) {
      const ue = toUiError(e)
      setError(ue)
      setView(MigrationStep.MigrationReview)
      if (ue.code === ErrorCode.ErrConfigChanged) {
        // the file moved on: show the new diff next to the error
        await Migration.Preview(name).then(setSt, () => undefined)
      }
    } finally {
      setApplying(false)
    }
  }

  const finish = async (next?: string) => {
    setBusy(true)
    try {
      await Migration.Finish(name)
      toast({ kind: 'ok', title: t('toast.migrated', { name }) })
      navigate(next ? { page: 'migrate', profile: next } : { page: 'accounts' })
    } catch (e) {
      setError(toUiError(e))
    } finally {
      setBusy(false)
    }
  }

  const profile = accounts?.profiles?.find((p) => p.name === name)
  const rail = railSteps(view, st, t)
  const currentStep = currentRailStep(rail)

  const w: WizardCtx | null = st && { name, st, profile, view, setView, run, busy, error, setError, progress, applying, startApply, finish }

  return (
    <div className="page">
      <button type="button" className="back-link" onClick={() => navigate({ page: 'accounts' })}>
        <ChevronLeft aria-hidden="true" />
        {t('nav.accounts')}
      </button>
      <header className="page__header" style={{ marginBottom: 'var(--sp-5)' }}>
        <div className="wizard-head" data-acct={profile?.colorSlot || undefined}>
          <AcctTile slot={profile?.colorSlot} monogram={profile?.monogram ?? name.slice(0, 2).toUpperCase()} size="lg" />
          <div>
            <h1 className="page__title"><CodeText text={t('migrate.title', { name: codeMark(name) })} /></h1>
            <p className="page__subtitle"><CodeText text={t('migrate.subtitle', { name: codeMark(name) })} /></p>
          </div>
        </div>
      </header>
      <GlobalBanner />
      <div className="wizard">
        <div className="wizard__rail wizard__rail--v" aria-label={t('migrate.stepsAria')}>
          <Stepper steps={rail} />
        </div>
        <div className="wizard__rail wizard__rail--h" aria-label={t('migrate.stepsAria')}>
          <Stepper horizontal steps={rail} />
          {currentStep && <p className="wizard__stepline">{t('migrate.stepOf', { n: currentStep.n, total: currentStep.total, label: String(currentStep.label) })}</p>}
        </div>
        <div className="card wizard__card">
          {loadError && <ErrorBanner error={loadError} onRetry={() => setAttempt((a) => a + 1)} />}
          {!w && !loadError && (
            <div className="wizard__body" aria-busy="true">
              <Skeleton variant="text" width={240} />
              <Skeleton variant="block" height={120} />
            </div>
          )}
          {w && (
            <>
              {error && (
                <div style={{ marginBottom: 'var(--sp-4)' }}>
                  <ErrorBanner error={error} />
                </div>
              )}
              {view === MigrationStep.MigrationIdentify && <StepIdentify w={w} />}
              {view === MigrationStep.MigrationPortal && <StepPortal w={w} />}
              {view === MigrationStep.MigrationAccount && <StepAccount w={w} />}
              {view === MigrationStep.MigrationReview && <StepReview w={w} />}
              {view === MigrationStep.MigrationVerify && <StepVerify w={w} />}
              {view === MigrationStep.MigrationRemoveKey && <StepRemoveKey w={w} />}
              {view === MigrationStep.MigrationDeactivate && <StepDeactivate w={w} />}
            </>
          )}
        </div>
      </div>
    </div>
  )
}
