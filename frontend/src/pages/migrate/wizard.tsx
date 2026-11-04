import { ChevronLeft } from 'lucide-react'
import type { ReactNode } from 'react'
import type { UiError } from '../../api/errors'
import { MigrationStep } from '../../api/models'
import type { MigrationItem, MigrationState, Profile, ProgressStatus } from '../../api/models'
import { Button } from '../../components/Button'
import { useI18n } from '../../i18n'
import { useNavigate } from '../../lib/route'
import { previousStep } from './model'
import './migrate.css'

export interface ItemProgress {
  status: ProgressStatus
  detail?: string
}

/** What every step component gets from the page. */
export interface WizardCtx {
  name: string
  st: MigrationState
  profile?: Profile
  /** The step on screen. It trails `st.step` only while the user goes back before the config is written. */
  view: MigrationStep
  setView: (s: MigrationStep) => void
  /** Run a call that returns the new wizard state; the page adopts it. Resolves null when the call failed (the error is shown). */
  run: (fn: () => Promise<MigrationState>, opts?: { onError?: (e: UiError) => void; keepError?: boolean }) => Promise<MigrationState | null>
  busy: boolean
  error: UiError | null
  setError: (e: UiError | null) => void
  progress: Partial<Record<MigrationItem, ItemProgress>>
  applying: boolean
  startApply: () => Promise<void>
  /** Leave the wizard: back to the accounts page with the state kept. */
  finish: (next?: string) => Promise<void>
}

export function StepHead({ title, lead }: { title: ReactNode; lead?: ReactNode }) {
  return (
    <>
      <h2 className="wizard__title">{title}</h2>
      {lead && <p className="wizard__lead">{lead}</p>}
    </>
  )
}

/** Back / Not now / extra actions / the step's primary action. */
export function WizardFooter({ w, primary, extra, backTo }: { w: WizardCtx; primary?: ReactNode; extra?: ReactNode; backTo?: MigrationStep | null }) {
  const { t } = useI18n()
  const navigate = useNavigate()
  const back = backTo === undefined ? previousStep(w.view) : backTo
  return (
    <div className="wizard__footer">
      <Button variant="ghost" icon={ChevronLeft} disabled={!back || w.busy} onClick={() => back && w.setView(back)}>
        {t('common.back')}
      </Button>
      <span className="spacer" />
      <Button variant="secondary" onClick={() => navigate({ page: 'accounts' })}>
        {t('common.later')}
      </Button>
      {extra}
      {primary}
    </div>
  )
}

export const isStep = (s: MigrationStep, ...of: MigrationStep[]) => of.includes(s)
export { MigrationStep }
