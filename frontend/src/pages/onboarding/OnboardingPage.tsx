import { Check, RefreshCw } from 'lucide-react'
import { useState } from 'react'
import appIcon from '../../assets/appicon.svg'
import { Language, Theme, ToolStatus } from '../../api/models'
import { toUiError } from '../../api/errors'
import { System } from '../../api/services'
import { Button } from '../../components/Button'
import { ErrorBanner } from '../../components/ErrorBanner'
import { Segmented } from '../../components/Segmented'
import { Skeleton } from '../../components/Skeleton'
import { Stepper } from '../../components/Stepper'
import type { StepperStep } from '../../components/Stepper'
import { errorText, useI18n } from '../../i18n'
import type { Lang } from '../../i18n'
import { initialUI } from '../../lib/initialUI'
import { useNavigate } from '../../lib/route'
import type { Route } from '../../lib/route'
import { useAsync } from '../../lib/useAsync'
import { useActions } from '../../shell/AppActions'
import { GlobalBanner } from '../../shell/GlobalBanner'
import { useEnv } from '../../state/env'
import { useSettings } from '../../state/settings'
import { EnvChecks } from '../health/EnvChecks'
import './onboarding.css'

type Step = 1 | 2 | 3

export function OnboardingPage() {
  const { t, lang } = useI18n()
  const { settings, update } = useSettings()
  const navigate = useNavigate()
  const { toast } = useActions()
  const env = useEnv()
  const [step, setStep] = useState<Step>(initialUI.onboardingStep ?? 1)
  const [rechecking, setRechecking] = useState(false)
  const [finishing, setFinishing] = useState<'tutorial' | 'accounts' | null>(null)
  const summary = useAsync(() => System.FirstRunSummary(), [])

  const fail = (e: unknown) => {
    const text = errorText(toUiError(e), t)
    toast({ kind: 'danger', title: text.title, body: text.body })
  }

  const finish = async (to: 'tutorial' | 'accounts') => {
    setFinishing(to)
    try {
      await System.CompleteFirstRun()
      const route: Route = to === 'tutorial' ? { page: 'tutorial', section: 'why' } : { page: 'accounts' }
      navigate(route)
    } catch (e) {
      fail(e)
      setFinishing(null)
    }
  }

  const recheck = async () => {
    setRechecking(true)
    try {
      await env.recheck()
    } finally {
      setRechecking(false)
    }
  }

  const labels = [t('onboarding.step.welcome'), t('onboarding.step.env'), t('onboarding.step.found')]
  const steps: StepperStep[] = labels.map((label, i) => ({
    id: String(i + 1),
    label,
    state: i + 1 < step ? 'done' : i + 1 === step ? 'current' : 'todo',
  }))

  const cliStatus = env.report?.awsCli.status
  const cliUsable = cliStatus === ToolStatus.ToolOK || cliStatus === ToolStatus.ToolOutdated

  return (
    <div className="page onboarding">
      <GlobalBanner />
      <nav aria-label={t('onboarding.aria')} className="onboarding__stepper">
        <Stepper steps={steps} horizontal />
      </nav>

      <div className="card onboarding__card">
        <div className="card__body">
          {step === 1 && (
            <>
              <div className="onboarding__hero">
                <img className="onboarding__icon" src={appIcon} alt="" width={96} height={96} />
                <h1 className="onboarding__title">{t('onboarding.welcome.title')}</h1>
                <p className="onboarding__lead">{t('onboarding.welcome.body')}</p>
              </div>
              <div className="onboarding__prefs">
                <span className="onboarding__prefs-label" id="ob-lang">
                  {t('onboarding.language')}
                </span>
                <Segmented<Lang>
                  role="radiogroup"
                  ariaLabel={t('onboarding.language')}
                  value={lang}
                  onChange={(v) => void update({ language: v === 'zh-CN' ? Language.LanguageZhCN : Language.LanguageEn }).catch(fail)}
                  items={[
                    { value: 'zh-CN', label: t('settings.language.zh') },
                    { value: 'en', label: t('settings.language.en') },
                  ]}
                />
                <span className="onboarding__prefs-label" id="ob-theme">
                  {t('onboarding.theme')}
                </span>
                <Segmented<Theme>
                  role="radiogroup"
                  ariaLabel={t('onboarding.theme')}
                  value={settings?.theme ?? Theme.ThemeSystem}
                  onChange={(v) => void update({ theme: v }).catch(fail)}
                  items={[
                    { value: Theme.ThemeSystem, label: t('settings.theme.system') },
                    { value: Theme.ThemeLight, label: t('settings.theme.light') },
                    { value: Theme.ThemeDark, label: t('settings.theme.dark') },
                  ]}
                />
              </div>
              <div className="onboarding__footer onboarding__footer--center">
                <Button variant="primary" size="lg" onClick={() => setStep(2)}>
                  {t('onboarding.start')}
                </Button>
              </div>
            </>
          )}

          {step === 2 && (
            <>
              <h1 className="onboarding__heading">{t('onboarding.env.title')}</h1>
              <p className="onboarding__text">{t('onboarding.env.body')}</p>
              <div className="onboarding__env">
                <EnvChecks compact />
              </div>
              <div className="onboarding__env-actions">
                <Button icon={RefreshCw} loading={rechecking} onClick={() => void recheck()}>
                  {t('common.checkAgain')}
                </Button>
                {env.report && !cliUsable && <span className="onboarding__env-note">{t('onboarding.env.cliNote')}</span>}
              </div>
              <div className="onboarding__footer">
                <Button variant="ghost" onClick={() => setStep(1)}>
                  {t('common.back')}
                </Button>
                <span className="spacer" />
                <Button variant="primary" onClick={() => setStep(3)}>
                  {t('common.next')}
                </Button>
              </div>
            </>
          )}

          {step === 3 && (
            <>
              <h1 className="onboarding__heading">{t('onboarding.found.title')}</h1>
              {summary.error && !summary.data ? (
                <ErrorBanner error={summary.error} onRetry={() => void summary.reload()} />
              ) : !summary.data ? (
                <div aria-busy="true" aria-label={t('onboarding.found.loading')}>
                  <Skeleton variant="text" width="80%" />
                </div>
              ) : (
                <>
                  <p className="onboarding__text">
                    {t('onboarding.found.body', {
                      profiles: summary.data.profiles,
                      portals: summary.data.portals,
                      static: summary.data.static,
                      repos: summary.data.repos,
                    })}
                  </p>
                  {(summary.data.scanRoots ?? []).length > 0 && (
                    <p className="onboarding__folders">
                      {t('onboarding.found.folders')} <span className="mono">{(summary.data.scanRoots ?? []).join(', ')}</span>
                    </p>
                  )}
                </>
              )}
              <p className="onboarding__note">
                <Check size={16} aria-hidden="true" />
                {t('onboarding.found.note')}
              </p>
              <div className="onboarding__footer">
                <Button variant="ghost" onClick={() => setStep(2)} disabled={finishing !== null}>
                  {t('common.back')}
                </Button>
                <span className="spacer" />
                <Button loading={finishing === 'tutorial'} disabled={finishing !== null} onClick={() => void finish('tutorial')}>
                  {t('onboarding.cta.tutorial')}
                </Button>
                <Button variant="primary" loading={finishing === 'accounts'} disabled={finishing !== null} onClick={() => void finish('accounts')}>
                  {t('onboarding.cta.accounts')}
                </Button>
              </div>
            </>
          )}
        </div>
      </div>
    </div>
  )
}
