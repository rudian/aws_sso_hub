// The setup rows on the Health page (design 5.1 "Later in Health"): the same three rows as the setup screen, with an
// "Update" action where the tool is older than recommended (or, if it ever happens, below the minimum).
import { useEffect, useRef, useState } from 'react'
import { toUiError } from '../../api/errors'
import { onEvent } from '../../api/events'
import { FailureCause, SetupFix, SetupToolID, ToolStatus } from '../../api/models'
import type { InstallFailure, InstallState, SetupTool } from '../../api/models'
import { Tools } from '../../api/services'
import { Button } from '../../components/Button'
import { Skeleton } from '../../components/Skeleton'
import { errorText, useI18n } from '../../i18n'
import type { MessageKey, T } from '../../i18n'
import { useAsync } from '../../lib/useAsync'
import { guessPlatform } from '../../lib/platform'
import { useActions } from '../../shell/AppActions'
import { toolLabel } from '../setup/model'
import type { Platform } from '../setup/model'
import { ErrorBanner } from '../../components/ErrorBanner'
import { StatusRow } from './StatusRow'
import type { RowStatus } from './StatusRow'

export interface DescribedTool {
  status: RowStatus
  text: string
  meta?: string
  /** True when the row offers an action (how to update, or install). */
  update: boolean
  /** The action's wording: "How to update" for a tool that is there, "Install" for one that is missing. */
  action?: 'howto' | 'install'
  /** The exact command the user can run, when this copy of the tool came from Homebrew or winget. */
  command?: string
}

const WINGET_IDS: Partial<Record<SetupToolID, string>> = { [SetupToolID.SetupGit]: 'Git.Git', [SetupToolID.SetupAWSCLI]: 'Amazon.AWSCLI' }
const BREW_FORMULAS: Partial<Record<SetupToolID, string>> = { [SetupToolID.SetupGit]: 'git', [SetupToolID.SetupAWSCLI]: 'awscli' }

/**
 * The command that updates (or installs) the tool, for the copies the app can name: a Homebrew copy under
 * /opt/homebrew (the only prefix the gate prefers) and winget on Windows. Nothing for anything else (an .pkg copy, v1).
 */
export function updateCommand(tool: SetupTool, platform: Platform): string | undefined {
  if (tool.status === ToolStatus.ToolV1 || tool.status === ToolStatus.ToolError) return undefined
  const missing = tool.status === ToolStatus.ToolMissing
  if (platform === 'windows') {
    const id = WINGET_IDS[tool.id as SetupToolID]
    return id ? `winget ${missing ? 'install' : 'upgrade'} --id ${id} -e` : undefined
  }
  if (tool.id === SetupToolID.SetupHomebrew) return missing ? undefined : 'brew update'
  const formula = BREW_FORMULAS[tool.id as SetupToolID]
  if (!formula) return undefined
  if (missing) return `brew install ${formula}`
  return tool.path?.startsWith('/opt/homebrew/') ? `brew upgrade ${formula}` : undefined
}

/** One row of Health from a gate row. Pure, so it is tested without the page. */
export function describeToolRow(tool: SetupTool, t: T, platform: Platform = 'darwin'): DescribedTool {
  const version = tool.version ?? ''
  const installed = version ? `${t('setup.state.installed')} ${version}` : t('setup.state.installed')
  if (tool.fix === SetupFix.FixUseDetected) return { status: 'warn', text: t('setup.health.useDetected'), meta: tool.path, update: false }
  const command = updateCommand(tool, platform)
  const act = (action: 'howto' | 'install'): Pick<DescribedTool, 'update' | 'action' | 'command'> => ({ update: true, action, ...(command ? { command } : {}) })
  switch (tool.status) {
    case ToolStatus.ToolOK:
      return { status: 'ok', text: installed, meta: tool.path, update: false }
    case ToolStatus.ToolOutdated:
      return { status: 'warn', text: `${installed} · ${t('setup.health.outdated')}`, meta: tool.path, ...act('howto') }
    case ToolStatus.ToolTooOld:
      return { status: 'danger', text: `${t('setup.state.tooOld')} ${version}`.trim(), meta: tool.path, ...act('howto') }
    case ToolStatus.ToolV1:
      return { status: 'danger', text: t('setup.health.v1'), meta: tool.path, ...act('howto') }
    case ToolStatus.ToolError:
      return { status: 'warn', text: t('setup.row.error'), meta: tool.path, ...act('howto') }
    default:
      return { status: 'danger', text: t('setup.state.missing'), ...act('install') }
  }
}

/**
 * True when the app can update this row itself: git or the AWS CLI that is there but old, in a copy its installer
 * owns (the same cases `updateCommand` names a `brew upgrade` / `winget upgrade` command for).
 */
export function canUpdateHere(tool: SetupTool, platform: Platform): boolean {
  if (tool.id !== SetupToolID.SetupGit && tool.id !== SetupToolID.SetupAWSCLI) return false
  if (tool.status !== ToolStatus.ToolOutdated && tool.status !== ToolStatus.ToolTooOld) return false
  if (tool.fix === SetupFix.FixUseDetected) return false
  return /^(brew|winget) upgrade /.test(updateCommand(tool, platform) ?? '')
}

/** The cause of a failed update in words that fit "The update did not finish: …" (the setup titles speak of installing). */
function updateCauseKey(f: InstallFailure): MessageKey {
  switch (f.cause) {
    case FailureCause.CauseNetwork:
      return 'health.tool.cause.network'
    case FailureCause.CauseProxyAuth:
      return 'health.tool.cause.proxyAuth'
    case FailureCause.CauseNotAdmin:
    case FailureCause.CauseNeedsAdmin:
      return 'health.tool.cause.admin'
    case FailureCause.CausePolicy:
      return 'health.tool.cause.policy'
    case FailureCause.CauseCancelled:
    case FailureCause.CauseAuthDenied:
      return 'health.tool.cause.cancelled'
    default:
      return 'health.tool.cause.generic'
  }
}

const updateStep = (install: InstallState | null, id: string) => (install?.steps ?? []).find((s) => s.id === `${id}:update`)

/**
 * The manager (Homebrew or winget), Git and the AWS CLI. A git or AWS CLI copy that Homebrew or winget owns has an
 * "Update" button that runs `brew upgrade` / `winget upgrade` through ToolService (progress and Cancel in the row, the
 * failure's cause in words); every other old or missing tool names the command and the button opens the official page.
 * `reloadKey` changes when the page's "Check again" ran.
 */
export function ToolRows({ reloadKey = 0, compact }: { reloadKey?: number; compact?: boolean }) {
  const { t } = useI18n()
  const { toast } = useActions()
  const state = useAsync(() => Tools.GetSetupState(), [reloadKey])
  const [busyId, setBusyId] = useState<string | null>(null)
  const [install, setInstall] = useState<InstallState | null>(null)
  useEffect(() => onEvent('tools:install-progress', setInstall), [])
  // coming back to Health while an update runs (or after one ended): show that run, not a fresh Update button
  useEffect(() => {
    let live = true
    Tools.GetInstallState().then(
      (s) => {
        if (live && (s.steps ?? []).some((x) => x.id.endsWith(':update'))) setInstall((cur) => cur ?? s)
      },
      () => undefined,
    )
    return () => {
      live = false
    }
  }, [])
  // an update that ended well changes what the gate says: read it again (local checks)
  const updateDone = !!install?.finished && !install.failure && !install.cancelled && (install.steps ?? []).some((s) => s.id.endsWith(':update'))
  const reload = state.reload
  useEffect(() => {
    if (updateDone) void reload()
  }, [updateDone, reload])
  // when the focused Cancel / Update button goes away with the finished update, keep the keyboard on this card
  const listRef = useRef<HTMLDivElement>(null)
  const finished = !!install?.finished
  const rowsRead = state.data
  useEffect(() => {
    if (finished && rowsRead && (document.activeElement === document.body || document.activeElement === null)) listRef.current?.focus()
  }, [finished, rowsRead])
  const platform: Platform = state.data?.platform === 'windows' ? 'windows' : state.data?.platform === 'darwin' ? 'darwin' : guessPlatform()

  if (state.error && !state.data) {
    return (
      <div className="card__body">
        <ErrorBanner error={state.error} onRetry={() => void state.reload()} />
      </div>
    )
  }
  if (!state.data) {
    return (
      <div className="list" aria-busy="true" aria-label={t('setup.health.loading')}>
        {[0, 1, 2].map((i) => (
          <div key={i} className="list__item">
            <Skeleton variant="text" width="55%" />
          </div>
        ))}
      </div>
    )
  }
  const update = async (tool: SetupTool, here: boolean) => {
    setBusyId(tool.id)
    try {
      if (here) {
        setInstall(null)
        await Tools.StartToolUpdate(tool.id)
      } else await Tools.OpenInstallPage(tool.id)
    } catch (e) {
      const text = errorText(toUiError(e), t)
      toast({ kind: 'danger', title: text.title, body: text.body })
    } finally {
      setBusyId(null)
    }
  }
  return (
    <div className="list" ref={listRef} tabIndex={-1}>
      {(state.data.tools ?? []).map((tool) => {
        const d = describeToolRow(tool, t, platform)
        const name = toolLabel(tool.id, platform)
        const here = canUpdateHere(tool, platform)
        const step = updateStep(install, tool.id)
        const updating = !!install?.running && !!step
        const failure = install?.finished && step && install.failure?.stepId === step.id ? install.failure : null
        const reason = failure ? t(updateCauseKey(failure)) : ''
        return (
          <StatusRow
            key={tool.id}
            compact={compact}
            status={updating ? 'busy' : d.status}
            title={name}
            text={updating ? t('health.tool.updating') : d.text}
            meta={
              d.meta || d.command || failure ? (
                <>
                  {d.meta && <span className="mono">{d.meta}</span>}
                  {d.command && (
                    <div>
                      {t('setup.health.run')} <code className="mono">{d.command}</code>
                    </div>
                  )}
                  {failure && <div role="status">{t('health.tool.failed', { reason })}</div>}
                </>
              ) : undefined
            }
            action={
              updating ? (
                <Button size="sm" aria-disabled={install?.cancelling || undefined} aria-label={t('health.tool.cancelFor', { tool: name })} onClick={() => !install?.cancelling && void Tools.CancelSetupInstall()}>
                  {t('health.tool.cancel')}
                </Button>
              ) : d.update ? (
                <Button size="sm" loading={busyId === tool.id} aria-label={t(here ? 'health.tool.updateFor' : d.action === 'install' ? 'setup.health.installFor' : 'setup.health.howtoFor', { tool: name })} onClick={() => void update(tool, here)}>
                  {t(here ? 'health.tool.update' : d.action === 'install' ? 'setup.health.install' : 'setup.health.howto')}
                </Button>
              ) : undefined
            }
          />
        )
      })}
    </div>
  )
}
