// The setup rows on the Health page (design 5.1 "Later in Health"): the same three rows as the setup screen, with an
// "Update" action where the tool is older than recommended (or, if it ever happens, below the minimum).
import { useState } from 'react'
import { toUiError } from '../../api/errors'
import { SetupFix, SetupToolID, ToolStatus } from '../../api/models'
import type { SetupTool } from '../../api/models'
import { Tools } from '../../api/services'
import { Button } from '../../components/Button'
import { Skeleton } from '../../components/Skeleton'
import { errorText, useI18n } from '../../i18n'
import type { T } from '../../i18n'
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
 * The manager (Homebrew or winget), Git and the AWS CLI. ToolService has no update call yet (it only installs what
 * blocks the gate), so the row names the command the user can run and the button opens the tool's official page.
 * `reloadKey` changes when the page's "Check again" ran.
 */
export function ToolRows({ reloadKey = 0, compact }: { reloadKey?: number; compact?: boolean }) {
  const { t } = useI18n()
  const { toast } = useActions()
  const state = useAsync(() => Tools.GetSetupState(), [reloadKey])
  const [busyId, setBusyId] = useState<string | null>(null)
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
  const update = async (tool: SetupTool) => {
    setBusyId(tool.id)
    try {
      await Tools.OpenInstallPage(tool.id)
    } catch (e) {
      const text = errorText(toUiError(e), t)
      toast({ kind: 'danger', title: text.title, body: text.body })
    } finally {
      setBusyId(null)
    }
  }
  return (
    <div className="list">
      {(state.data.tools ?? []).map((tool) => {
        const d = describeToolRow(tool, t, platform)
        const name = toolLabel(tool.id, platform)
        return (
          <StatusRow
            key={tool.id}
            compact={compact}
            status={d.status}
            title={name}
            text={d.text}
            meta={
              d.meta || d.command ? (
                <>
                  {d.meta && <span className="mono">{d.meta}</span>}
                  {d.command && (
                    <div>
                      {t('setup.health.run')} <code className="mono">{d.command}</code>
                    </div>
                  )}
                </>
              ) : undefined
            }
            action={
              d.update ? (
                <Button size="sm" loading={busyId === tool.id} aria-label={t(d.action === 'install' ? 'setup.health.installFor' : 'setup.health.howtoFor', { tool: name })} onClick={() => void update(tool)}>
                  {t(d.action === 'install' ? 'setup.health.install' : 'setup.health.howto')}
                </Button>
              ) : undefined
            }
          />
        )
      })}
    </div>
  )
}
