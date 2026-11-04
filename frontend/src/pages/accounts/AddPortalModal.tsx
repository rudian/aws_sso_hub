import { ChevronLeft } from 'lucide-react'
import { useEffect, useMemo, useRef, useState } from 'react'
import { toUiError } from '../../api/errors'
import type { UiError } from '../../api/errors'
import { Accounts } from '../../api/services'
import { ErrorCode } from '../../api/models'
import type { FilePreview } from '../../api/models'
import { Banner } from '../../components/Banner'
import { Button } from '../../components/Button'
import { DiffView } from '../../components/DiffView'
import { ErrorBanner } from '../../components/ErrorBanner'
import { Modal } from '../../components/Modal'
import { Skeleton } from '../../components/Skeleton'
import { Stepper } from '../../components/Stepper'
import { useI18n } from '../../i18n'
import { initialUI } from '../../lib/initialUI'
import { useActions } from '../../shell/AppActions'
import { useAccounts } from '../../state/accounts'
import { PortalFields, usePortalForm } from './PortalForm'
import { RolePicker, nameProblems, toSpecs } from './RolePicker'
import type { PickedRole } from './RolePicker'
import './accounts.css'

export interface AddPortalModalProps {
  /** `new`: details, roles, preview. `addRole`: roles and preview for an existing portal. */
  mode: 'new' | 'addRole'
  portal?: string
  onClose: () => void
}

type Step = 1 | 2 | 3

/** The preset used by screenshots (mock mode only): `?open=addPortal&addPortalStep=2|3` skips the typing. */
const SCREENSHOT_PRESET = { startUrl: 'https://d-1111111111.awsapps.com/start', region: 'us-east-1', name: 'acme' }

export function AddPortalModal({ mode, portal: existingPortal, onClose }: AddPortalModalProps) {
  const { t } = useI18n()
  const { openLogin, toast } = useActions()
  const { state } = useAccounts()
  const auto = mode === 'new' && (initialUI.addPortalStep ?? 1) >= 2
  const form = usePortalForm(auto ? SCREENSHOT_PRESET : undefined)

  const [step, setStep] = useState<Step>(mode === 'addRole' ? 2 : 1)
  const [hidden, setHidden] = useState(false)
  const [draft, setDraft] = useState<{ name: string; startUrl: string; region: string } | null>(null)
  const [written, setWritten] = useState(false)
  const [picked, setPicked] = useState<Record<string, PickedRole>>({})
  const [pickerKey, setPickerKey] = useState(0)
  const [preview, setPreview] = useState<FilePreview | null>(null)
  const [error, setError] = useState<UiError | null>(null)
  const [busy, setBusy] = useState(false)
  const closing = useRef(false)

  const portalName = mode === 'addRole' ? (existingPortal as string) : (draft?.name ?? '')
  const portalInfo = state?.portals?.find((p) => p.name === portalName)
  const portalRegion = (mode === 'addRole' ? portalInfo?.region : draft?.region) ?? form.values.region
  const existingNames = useMemo(() => new Set((state?.profiles ?? []).map((p) => p.name)), [state])
  const problems = useMemo(() => nameProblems(picked, existingNames), [picked, existingNames])
  const pickedCount = Object.keys(picked).length

  const close = () => {
    if (closing.current) return
    closing.current = true
    if (draft && !written) void Accounts.DiscardPortalDraft(draft.name).catch(() => undefined)
    onClose()
  }

  const signIn = async (name: string): Promise<boolean> => {
    setHidden(true)
    try {
      const res = await openLogin(name)
      return !!res
    } finally {
      setHidden(false)
    }
  }

  const continueFromDetails = async () => {
    setError(null)
    const { ok, values } = await form.validate()
    if (!ok) return
    setBusy(true)
    try {
      let current = draft
      if (!current || current.name !== values.name || current.startUrl !== values.startUrl || current.region !== values.region) {
        if (current) await Accounts.DiscardPortalDraft(current.name)
        setDraft(null)
        const p = await Accounts.AddPortalDraft({ name: values.name, startUrl: values.startUrl, region: values.region })
        current = { name: p.name, startUrl: p.startUrl, region: p.region }
        setDraft(current)
      }
      if (await signIn(current.name)) {
        setPickerKey((k) => k + 1)
        setStep(2)
      }
    } catch (e) {
      setError(toUiError(e))
    } finally {
      setBusy(false)
    }
  }

  // screenshots: start the flow without typing
  const started = useRef(false)
  useEffect(() => {
    if (!auto || started.current || !form.valid) return
    started.current = true
    void continueFromDetails()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [auto, form.valid])
  useEffect(() => {
    if (!auto || initialUI.addPortalStep !== 3 || step !== 2 || !draft) return
    let alive = true
    void Accounts.DiscoverAccounts(draft.name).then((accounts) => {
      const a = accounts?.[0]
      const r = a?.roles?.[0]
      if (!alive || !a || !r) return
      setPicked({ [`${a.accountId}|${r.name}`]: { name: r.suggestedProfile, accountId: a.accountId, accountName: a.accountName, role: r.name, region: draft.region, colorSlot: a.colorSlot } })
      setStep(3)
    })
    return () => {
      alive = false
    }
  }, [auto, step, draft])

  const request = useMemo(() => ({ portal: portalName, profiles: toSpecs(picked) }), [portalName, picked])

  const loadPreview = async () => {
    setPreview(null)
    setError(null)
    try {
      setPreview(await Accounts.PreviewCreateProfiles(request))
    } catch (e) {
      setError(toUiError(e))
    }
  }
  useEffect(() => {
    if (step === 3) void loadPreview()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [step])

  const write = async () => {
    if (!preview) return
    setBusy(true)
    setError(null)
    try {
      const res = await Accounts.CreateProfiles(request, preview.baseHash)
      setWritten(true)
      toast({ kind: 'ok', title: t('addPortal.done', { n: res.profiles?.length ?? pickedCount }) })
      closing.current = true
      onClose()
    } catch (e) {
      const ue = toUiError(e)
      setError(ue)
      setBusy(false)
      if (ue.code === ErrorCode.ErrConfigChanged || ue.code === ErrorCode.ErrProfileExists) void loadPreview()
    }
  }

  const steps = [
    { id: 'details', label: t('addPortal.step.details'), state: step === 1 ? ('current' as const) : ('done' as const) },
    { id: 'roles', label: t('addPortal.step.roles'), state: step === 2 ? ('current' as const) : step > 2 ? ('done' as const) : ('todo' as const) },
    { id: 'preview', label: t('addPortal.step.preview'), state: step === 3 ? ('current' as const) : ('todo' as const) },
  ]

  const title = mode === 'addRole' ? t('card.add.title') : t('addPortal.title')
  const canNext = pickedCount > 0 && Object.keys(problems).length === 0

  return (
    <Modal
      open={!hidden}
      size="lg"
      title={title}
      subtitle={mode === 'addRole' ? <span className="mono">{existingPortal}</span> : undefined}
      onClose={close}
      className="add-portal"
      footer={
        <>
          {step === 3 && (
            <Button variant="ghost" icon={ChevronLeft} className="add-portal__back" onClick={() => setStep(2)}>
              {t('addPortal.back')}
            </Button>
          )}
          {step === 2 && mode === 'new' && (
            <Button variant="ghost" icon={ChevronLeft} className="add-portal__back" onClick={() => setStep(1)}>
              {t('addPortal.back')}
            </Button>
          )}
          <Button variant="secondary" onClick={close}>
            {t('common.cancel')}
          </Button>
          {step === 1 && (
            <Button variant="primary" loading={busy} onClick={() => void continueFromDetails()}>
              {t('addPortal.signInContinue')}
            </Button>
          )}
          {step === 2 && (
            <Button variant="primary" disabled={!canNext} onClick={() => setStep(3)}>
              {t('common.next')}
            </Button>
          )}
          {step === 3 && (
            <Button variant="primary" loading={busy} disabled={!preview} onClick={() => void write()}>
              {t('addPortal.write')}
            </Button>
          )}
        </>
      }
    >
      <div className="add-portal__body">
        <Stepper horizontal steps={steps} />
        {step === 1 && (
          <div className="add-portal__step">
            <PortalFields form={form} autoFocus />
            {error && <ErrorBanner error={error} />}
          </div>
        )}
        {step === 2 && portalName && (
          <div className="add-portal__step">
            <RolePicker
              key={pickerKey}
              portal={portalName}
              portalRegion={portalRegion}
              picked={picked}
              onPick={setPicked}
              onSignIn={() =>
                void signIn(portalName).then((ok) => {
                  if (ok) setPickerKey((k) => k + 1)
                })
              }
            />
          </div>
        )}
        {step === 3 && (
          <div className="add-portal__step">
            <p className="muted" style={{ margin: 0 }}>
              {t('addPortal.preview.lead')} {t('addPortal.preview.profiles', { n: pickedCount })}
            </p>
            {error && <ErrorBanner error={error} onRetry={() => void loadPreview()} />}
            {!preview && !error && <Skeleton variant="block" height={140} />}
            {preview && (
              <>
                <DiffView preview={preview} />
                <Banner kind="info">{t('addPortal.preview.backup', { path: preview.backupDir })}</Banner>
              </>
            )}
          </div>
        )}
      </div>
    </Modal>
  )
}
