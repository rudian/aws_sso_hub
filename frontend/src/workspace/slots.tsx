// Places later tasks fill. Each is replaced by the task named next to it; the frame does not change.
import type { ComponentType } from 'react'
import { Button } from '../components'
import { Tooltip } from '../components'
import { RefreshCw, ChevronDown } from 'lucide-react'
import { useI18n } from '../i18n'
import type { Repo } from '../api/models'
import type { WsTab } from '../state/workspace'

export interface TabViewProps {
  repo: Repo
  folderId: string
}

/** Header: the sync button (G4-06 replaces it with the count pills and the verb of the folder on screen). */
export function SyncSlot() {
  const { t } = useI18n()
  return (
    <Tooltip content={t('ws.sync.soon')}>
      <span className="splitbtn" aria-disabled="true">
        <Button type="button" size="sm" disabled>
          <RefreshCw aria-hidden="true" />
          {t('ws.sync.fetch')}
        </Button>
        <Button type="button" size="sm" disabled aria-label={t('ws.sync.more')}>
          <ChevronDown aria-hidden="true" />
        </Button>
      </span>
    </Tooltip>
  )
}

/** Bottom panel, Terminal tab (A2-10 mounts the terminal component of A2-07 here). `data-terminal` marks it for the keyboard map. */
export function TerminalSlot({ repo, folderId }: TabViewProps) {
  const { t } = useI18n()
  return (
    <div className="ws-empty" data-slot="terminal" data-repo={repo.id} data-folder={folderId}>
      {t('ws.panel.terminal.soon')}
    </div>
  )
}

/** The six tabs' bodies. A2-09 fills 'ai'; the Code, Changes, History, Branches and Pull requests tasks fill the rest. */
export const TAB_VIEWS: Partial<Record<WsTab, ComponentType<TabViewProps>>> = {}
