// The tool rows and the log of the setup card (design 5.1).
import clsx from 'clsx'
import { Cloud, CircleCheck, CircleX, GitBranch, Package } from 'lucide-react'
import { useEffect, useRef } from 'react'
import type { ReactNode } from 'react'
import { InstallStepKind, InstallStepState, SetupFix, SetupToolID, ToolStatus } from '../../api/models'
import type { InstallFailure, InstallStep } from '../../api/models'
import { Progress } from '../../components/Progress'
import { Spinner } from '../../components/Spinner'
import { useI18n } from '../../i18n'
import type { T } from '../../i18n'
import { logTone, redactLine, stepLabelKey, stepPercent, toolLabel, whyKey } from './model'
import type { Platform, RowView } from './model'

const ICONS = { homebrew: Package, winget: Package, git: GitBranch, awscli: Cloud } as const

/** The text that says what the running step is doing (the row's second line while it installs). */
export function stepText(step: InstallStep, platform: Platform, t: T): string {
  if (step.state === InstallStepState.StepWaitingOS) return platform === 'windows' ? t('setup.step.waitingWin') : t('setup.step.waitingMac')
  if (step.kind === InstallStepKind.StepDownload) {
    const pct = stepPercent(step)
    return pct === undefined ? t('setup.step.downloadingNoPct') : t('setup.step.downloading', { pct })
  }
  if (step.kind === InstallStepKind.StepVerify) return t('setup.step.verifying')
  return platform === 'windows' ? t('setup.step.runningWin') : t('setup.step.installing')
}

function stateLabel(row: RowView, t: T): ReactNode {
  const v = row.tool.version
  switch (row.kind) {
    case 'ok':
      return (
        <span className="ok">
          <CircleCheck aria-hidden="true" />
          {t('setup.state.installed')}
          {v && <span className="mono muted">{v}</span>}
        </span>
      )
    case 'missing':
      return <span className="muted">{t('setup.state.missing')}</span>
    case 'too-old':
      return (
        <span className="warn">
          {t('setup.state.tooOld')}
          {v && <span className="mono">{v}</span>}
        </span>
      )
    case 'detected':
      return <span className="warn">{t('setup.state.detected')}</span>
    case 'v1':
      return <span className="warn">{t('setup.state.v1')}</span>
    case 'error':
      return <span className="warn">{t('setup.state.unreadable')}</span>
    case 'queued':
      return <span className="muted">{t('setup.state.queued')}</span>
    case 'installing':
      return (
        <span className="info">
          <Spinner />
          {t('setup.state.installing')}
        </span>
      )
    case 'waiting':
      return (
        <span className="info">
          <Spinner />
          {t('setup.state.waiting')}
        </span>
      )
    default:
      return (
        <span className="bad">
          <CircleX aria-hidden="true" />
          {t('setup.state.failed')}
        </span>
      )
  }
}

/** One sentence under the row for the cases the right-hand label cannot explain. */
function rowNote(row: RowView, platform: Platform, t: T): string | null {
  const tool = row.tool
  if (!tool.blocking || row.kind === 'installing' || row.kind === 'waiting' || row.kind === 'failed' || row.kind === 'queued') return null
  const label = toolLabel(tool.id, platform)
  if (tool.fix === SetupFix.FixUseDetected) {
    return tool.status === ToolStatus.ToolTooOld ? t('setup.row.detected', { tool: label, version: tool.version ?? '' }) : t('setup.row.detectedMissing', { tool: label })
  }
  if (tool.override) return tool.path ? t('setup.row.override', { path: tool.path }) : t('setup.row.overrideNoPath')
  if (row.kind === 'too-old') return t(tool.fix === SetupFix.FixUpgrade && !tool.elsewhere ? 'setup.row.tooOldUpgrade' : 'setup.row.tooOld', { min: tool.min ?? '' })
  if (row.kind === 'v1') return t('setup.row.v1')
  if (row.kind === 'error') return t('setup.row.error')
  return null
}

export interface ToolRowProps {
  row: RowView
  platform: Platform
  /** True when the current step has been quiet for over 10 s. */
  quiet?: boolean
}

export function ToolRow({ row, platform, quiet }: ToolRowProps) {
  const { t } = useI18n()
  const Icon = ICONS[row.tool.id as keyof typeof ICONS] ?? Package
  const name = toolLabel(row.tool.id, platform)
  const note = rowNote(row, platform, t)
  const cur = row.current
  const live = (row.kind === 'installing' || row.kind === 'waiting') && cur
  const pct = cur ? stepPercent(cur) : undefined
  const showSteps = live && row.steps.length > 1
  return (
    <li className={clsx('tool-row', live && 'is-current', row.kind === 'failed' && 'is-failed')} data-tool={row.tool.id} data-state={row.kind}>
      <span className="tool-row__icon" aria-hidden="true">
        <Icon />
      </span>
      <div>
        <div className="tool-row__name">{name}</div>
        <div className="tool-row__why">{t(whyKey(row.tool.id as SetupToolID, platform))}</div>
      </div>
      <div className="tool-row__state">{stateLabel(row, t)}</div>
      {note && <div className="tool-row__note">{note}</div>}
      {live && cur && (
        <div className="tool-row__progress">
          <Progress pct={pct} indeterminate={pct === undefined} label={t('setup.progress.label', { tool: name, step: stepText(cur, platform, t) })} />
          <span>
            {cur.command && <span className="mono">{cur.command} · </span>}
            {stepText(cur, platform, t)}
          </span>
          {showSteps && (
            <ol className="tool-steps" aria-label={t('setup.step.list', { tool: name })}>
              {row.steps.map((s) => (
                <li key={s.id} className={clsx('tool-steps__item', `is-${s.state}`)} aria-current={s.id === cur.id ? 'step' : undefined}>
                  <span className="tool-steps__mark" aria-hidden="true" />
                  {t(stepLabelKey(s.kind))}
                  <span className="sr-only">
                    {': '}
                    {s.state === InstallStepState.StepDone ? t('setup.step.done') : s.id === cur.id ? stepText(s, platform, t) : t('setup.step.pending')}
                  </span>
                </li>
              ))}
            </ol>
          )}
          {quiet && cur.state === InstallStepState.StepRunning && <span className="tool-row__slow">{t('setup.slow')}</span>}
        </div>
      )}
    </li>
  )
}

export interface SetupLogProps {
  lines: string[]
  failure?: InstallFailure | null
  expanded: boolean
  onToggle: () => void
}

/** The live log: the last 5 lines, expandable to the full height. role="log" that is not read out line by line. */
export function SetupLog({ lines, failure, expanded, onToggle }: SetupLogProps) {
  const { t } = useI18n()
  const ref = useRef<HTMLPreElement>(null)
  const shown = expanded ? lines : lines.slice(-5)
  useEffect(() => {
    const el = ref.current
    if (el) el.scrollTop = el.scrollHeight
  }, [shown.length, expanded, lines.length, lines[lines.length - 1]])
  if (lines.length === 0) return null
  return (
    <div className="setup__logwrap">
      <pre ref={ref} className={clsx('setup__log', expanded && 'is-expanded')} role="log" aria-live="off" aria-label={t('setup.log.label')} tabIndex={0}>
        {shown.map((l, i) => {
          const line = redactLine(l)
          const tone = logTone(line, failure)
          return (
            <span key={`${lines.length - shown.length + i}`} className={clsx('setup__line', tone !== 'plain' && `setup__line--${tone}`)}>
              {tone === 'ok' ? <span className="g">✓</span> : null}
              {tone === 'ok' ? line.replace(/^✓\s*/, ' ') : line}
              {'\n'}
            </span>
          )
        })}
      </pre>
      {lines.length > 5 && (
        <button type="button" className="setup__logtoggle" aria-expanded={expanded} onClick={onToggle}>
          {expanded ? t('setup.log.collapse') : t('setup.log.expand')}
        </button>
      )}
    </div>
  )
}
