// The body of the update dialog (design 5.13): versions, release notes, progress and steps, warnings. The page shows
// the same panel inline, so the two never drift apart.
import { ArrowRight, Check, X } from 'lucide-react'
import { useRef } from 'react'
import { UpdatePhase } from '../../api/models'
import type { UpdateState } from '../../api/models'
import { Banner } from '../../components/Banner'
import { Button } from '../../components/Button'
import { Progress } from '../../components/Progress'
import { Spinner } from '../../components/Spinner'
import { useI18n } from '../../i18n'
import type { MessageKey, T } from '../../i18n'
import { guessPlatform } from '../../lib/platform'
import { useAppInfo } from '../../state/appInfo'
import { codeKey, etaSeconds, mb, noteLines, percent, stepKey, stepsFor } from './model'
import type { StepId, StepStatus } from './model'
import { updateActions } from './useUpdate'
import { useReopenWatch } from './reopenWatch'
import { useRunningTasks } from './runningTasks'

/** Plain-text release notes. Every line is a React text child: a note that contains HTML is shown as the text it is. */
export function Notes({ notes }: { notes?: string }) {
  const { t } = useI18n()
  const lines = noteLines(notes)
  if (lines.length === 0) {
    return (
      <ul className="notes notes--empty" aria-label={t('update.notes.label')}>
        <li>{t('update.notes.empty')}</li>
      </ul>
    )
  }
  return (
    <ul className="notes" aria-label={t('update.notes.label')} tabIndex={lines.length > 8 ? 0 : undefined}>
      {lines.map((l, i) => (
        <li key={i}>{l}</li>
      ))}
    </ul>
  )
}

function Versions({ s }: { s: UpdateState }) {
  const { t } = useI18n()
  return (
    <div className="ver">
      <div className="ver__box ver__box--cur">
        <div className="ver__n">{s.current}</div>
        <div className="ver__k">{t('update.ver.current')}</div>
      </div>
      <ArrowRight aria-hidden="true" />
      <div className="ver__box">
        <div className="ver__n">{s.version}</div>
        <div className="ver__k">{t('update.ver.next')}</div>
      </div>
    </div>
  )
}

/** Bytes per second, measured between the last two progress events of one download. */
function useSpeed(s: UpdateState): number {
  const last = useRef<{ at: number; written: number; speed: number } | null>(null)
  if (s.phase !== UpdatePhase.UpdateDownloading) {
    last.current = null
    return 0
  }
  const now = Date.now()
  const w = s.written ?? 0
  const prev = last.current
  if (!prev || w < prev.written) last.current = { at: now, written: w, speed: 0 }
  else if (w > prev.written && now > prev.at) {
    const inst = ((w - prev.written) * 1000) / (now - prev.at)
    last.current = { at: now, written: w, speed: prev.speed ? prev.speed * 0.6 + inst * 0.4 : inst }
  }
  return last.current?.speed ?? 0
}

export function etaText(sec: number | null, t: T): string {
  if (sec === null) return ''
  return sec >= 90 ? t('update.progress.etaMin', { n: Math.ceil(sec / 60) }) : t('update.progress.etaSec', { n: sec })
}

function StepItem({ id, status }: { id: StepId; status: StepStatus }) {
  const { t } = useI18n()
  return (
    <li className={status === 'todo' ? undefined : `is-${status}`} aria-current={status === 'now' ? 'step' : undefined}>
      {status === 'now' && <Spinner />}
      {status === 'done' && <Check aria-hidden="true" />}
      {status === 'error' && <X aria-hidden="true" />}
      <span>{t(stepKey(id))}</span>
      {status === 'error' && <span className="sr-only">{t('update.step.failed')}</span>}
    </li>
  )
}

/** Why it failed, in one or two sentences, in the user's language. */
export function failureText(s: UpdateState, t: T): { title: string; body?: string } {
  const reason = t(codeKey(s.code))
  if (s.step === 'check') return s.code === 'network' ? { title: t('update.fail.check') } : { title: t('update.fail.checkRefused', { reason }) }
  const failedId = stepsFor(s).find((x) => x.status === 'error')?.id
  if (s.phase === UpdatePhase.UpdateRefused) {
    return { title: t('update.fail.refused'), body: t('update.fail.refusedStep', { step: failedId ? t(stepKey(failedId)) : '', reason }) }
  }
  if (s.step === 'restart') return { title: t('update.fail.restart', { reason }) }
  return { title: t('update.fail.download', { reason }) }
}

export function blockedText(s: UpdateState, t: T, platform: string): string {
  switch (s.block) {
    case 'translocated':
      return platform === 'windows' ? t('update.blocked.moveWin') : t('update.blocked.move')
    case 'not-writable':
      return t('update.blocked.notWritable', { folder: s.folder ?? '' })
    default:
      return t('update.blocked.unsupported')
  }
}

export function UpdatePanel({ s, showNotes = true, idleLine = true }: { s: UpdateState; showNotes?: boolean; idleLine?: boolean }) {
  const { t } = useI18n()
  const info = useAppInfo()
  const tasks = useRunningTasks()
  const watch = useReopenWatch()
  const speed = useSpeed(s)
  const platform = info?.platform ?? guessPlatform()
  const steps = stepsFor(s)
  const busy = s.phase === UpdatePhase.UpdateDownloading || s.phase === UpdatePhase.UpdateVerifying || s.phase === UpdatePhase.UpdateReady || s.phase === UpdatePhase.UpdateRestarting
  const isFail = s.phase === UpdatePhase.UpdateError || s.phase === UpdatePhase.UpdateRefused

  let label: string | null = null
  let meta = ''
  if (s.phase === UpdatePhase.UpdateDownloading) {
    label = t('update.progress.downloading')
    const eta = etaText(etaSeconds(s, speed), t)
    const bytes = s.total ? t('update.progress.bytes', { written: mb(s.written), total: mb(s.total) }) : t('update.progress.bytesOnly', { written: mb(s.written) })
    meta = eta ? `${bytes} · ${eta}` : bytes
  } else if (s.phase === UpdatePhase.UpdateVerifying) label = t('update.progress.verifying')
  else if (s.phase === UpdatePhase.UpdateReady) label = t('update.progress.ready')
  else if (s.phase === UpdatePhase.UpdateRestarting) label = t('update.progress.restarting')
  else if (s.phase === UpdatePhase.UpdateAvailable && s.total) meta = t('update.size', { size: `${mb(s.total)} MB` })

  const fail = isFail ? failureText(s, t) : null
  const pct = s.phase === UpdatePhase.UpdateReady || s.phase === UpdatePhase.UpdateRestarting ? 100 : percent(s)

  return (
    <>
      {s.version && s.phase !== UpdatePhase.UpdateBlocked && <Versions s={s} />}
      {showNotes && s.version && s.phase !== UpdatePhase.UpdateBlocked && <Notes notes={s.notes} />}

      {(busy || (isFail && !!s.version) || s.phase === UpdatePhase.UpdateAvailable) && (
        <div className="uprog">
          {(label || meta) && (
            <div className="uprog__head">
              <span className="uprog__label">{label}</span>
              <span className="uprog__meta">{meta}</span>
            </div>
          )}
          {(busy || isFail) && <Progress pct={pct} indeterminate={s.phase === UpdatePhase.UpdateVerifying || s.phase === UpdatePhase.UpdateRestarting || (s.phase === UpdatePhase.UpdateDownloading && !s.total)} label={label ?? t('update.dialog.title')} />}
          <ol className="usteps" aria-label={t('update.steps.aria')}>
            {steps.map((x) => (
              <StepItem key={x.id} id={x.id} status={x.status} />
            ))}
          </ol>
        </div>
      )}

      {s.phase === UpdatePhase.UpdateUpToDate && <p className="uprog__meta">{t('update.status.upToDate', { current: s.current })}</p>}
      {s.phase === UpdatePhase.UpdateChecking && (
        <p className="uprog__meta" role="status">
          <Spinner /> {t('update.status.checking')}
        </p>
      )}
      {idleLine && s.phase === UpdatePhase.UpdateIdle && <p className="uprog__meta">{t('update.status.idle')}</p>}

      {fail && (
        <Banner kind="danger" title={fail.title}>
          {fail.body}
        </Banner>
      )}

      {s.phase === UpdatePhase.UpdateBlocked && (
        <Banner kind="warn" title={blockedText(s, t, platform)}>
          {s.block === 'translocated' && s.folder ? t('update.blocked.folder', { folder: s.folder }) : undefined}
        </Banner>
      )}

      {busy && s.phase !== UpdatePhase.UpdateRestarting && tasks > 0 && (
        <Banner kind="warn" title={t('update.tasks.title', { n: tasks })}>
          {t('update.tasks.body')}
        </Banner>
      )}

      {watch.mode !== 'off' && (
        <Banner
          kind="info"
          title={watch.mode === 'countdown' ? t('update.wait.countdown', { s: watch.secondsLeft }) : t('update.wait.title')}
          actions={
            <Button size="sm" onClick={updateActions.cancelWait}>
              {t('update.action.cancelWait')}
            </Button>
          }
        >
          {watch.mode === 'countdown' ? t('update.wait.countdownBody') : t('update.wait.body')}
        </Banner>
      )}
    </>
  )
}

/** The words a screen reader hears when the phase changes (not on every progress tick). */
export function announceKey(phase: UpdatePhase): MessageKey | null {
  switch (phase) {
    case UpdatePhase.UpdateAvailable:
      return 'update.announce.available'
    case UpdatePhase.UpdateReady:
      return 'update.announce.ready'
    case UpdatePhase.UpdateError:
    case UpdatePhase.UpdateRefused:
      return 'update.announce.failed'
    case UpdatePhase.UpdateRestarting:
      return 'update.announce.restarting'
    default:
      return null
  }
}

