import clsx from 'clsx'
import { CircleX, GitBranch, SquareTerminal, TriangleAlert } from 'lucide-react'
import { Kbd, Tooltip } from '../components'
import { useI18n } from '../i18n'
import { isMac } from '../lib/platform'
import type { Platform } from '../lib/platform'
import type { PanelState, PanelTab } from '../state/workspace'

export interface StatusBarProps {
  panel: PanelState
  /** The branch, or the task folder's name, of the folder on screen. */
  context: string
  platform: Platform
  onTogglePanel: () => void
  onOpenPanel: (tab: PanelTab) => void
  /** The folder's branch button: shows the Branches tab. */
  onOpenBranches?: () => void
  /** Errors and warnings of the language servers (milestone 3); 0 until then. */
  problems?: { errors: number; warnings: number }
}

/** The status bar (`.ws__status`, 26px). Every item is a button with a tooltip; the right side fills with the editor's items in milestone 3. */
export function StatusBar({ panel, context, platform, onTogglePanel, onOpenPanel, onOpenBranches, problems = { errors: 0, warnings: 0 } }: StatusBarProps) {
  const { t } = useI18n()
  const mac = isMac(platform)
  const toggleKeys = mac ? '⌃`' : 'Ctrl+`'
  const findKeys = mac ? '⇧⌘F' : 'Ctrl+Shift+F'
  const termOpen = panel.open && panel.tab === 'terminal'
  return (
    <footer className="ws__status" aria-label={t('ws.status.aria')}>
      <Tooltip content={t('ws.status.terminal.tip')}>
        <button type="button" className={clsx('st st--btn', termOpen && 'is-on')} aria-pressed={termOpen} onClick={onTogglePanel}>
          <SquareTerminal aria-hidden="true" />
          {t('ws.status.terminal')}
          <Kbd>{toggleKeys}</Kbd>
        </button>
      </Tooltip>
      <Tooltip content={t('ws.status.problems.tip')}>
        <button
          type="button"
          className={clsx('st st--btn', problems.errors > 0 && 'st--danger')}
          aria-label={`${t('ws.status.problems')}: ${t('ws.status.problems.aria', { errors: problems.errors, warnings: problems.warnings })}`}
          onClick={() => onOpenPanel('problems')}
        >
          <CircleX aria-hidden="true" />
          {problems.errors}
          <TriangleAlert aria-hidden="true" />
          {problems.warnings}
        </button>
      </Tooltip>
      <Tooltip content={t('ws.status.find.tip')}>
        <button type="button" className="st st--btn" onClick={() => onOpenPanel('find')}>
          {t('ws.status.find')}
          <Kbd>{findKeys}</Kbd>
        </button>
      </Tooltip>
      {context && (
        <Tooltip content={t('ws.status.context.tip')}>
          <button type="button" className="st st--btn" aria-label={`${t('ws.status.context.aria')}: ${context}`} onClick={onOpenBranches}>
            <GitBranch aria-hidden="true" />
            {context}
          </button>
        </Tooltip>
      )}
      <span className="spacer" />
    </footer>
  )
}
