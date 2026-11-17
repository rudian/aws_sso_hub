import clsx from 'clsx'
import { Bot, Code2, FileDiff, GitBranch, GitPullRequest, History, Search } from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import type { KeyboardEvent } from 'react'
import { Badge, Kbd, Tooltip } from '../components'
import { useI18n } from '../i18n'
import type { MessageKey } from '../i18n'
import { guessPlatform, isMac } from '../lib/platform'
import { WS_TABS } from '../state/workspace'
import type { WsTab } from '../state/workspace'

const ICONS: Record<WsTab, LucideIcon> = { code: Code2, changes: FileDiff, history: History, branches: GitBranch, prs: GitPullRequest, ai: Bot }
const LABELS: Record<WsTab, MessageKey> = { code: 'ws.tab.code', changes: 'ws.tab.changes', history: 'ws.tab.history', branches: 'ws.tab.branches', prs: 'ws.tab.prs', ai: 'ws.tab.ai' }

export const tabId = (tab: WsTab) => `ws-tab-${tab}`
export const panelId = (tab: WsTab) => `ws-tabpanel-${tab}`

export interface TabBarProps {
  value: WsTab
  onChange: (tab: WsTab) => void
  /** Counts where there is work (changed files, open PRs, tasks waiting). 0 or missing: no badge. */
  counts?: Partial<Record<WsTab, number>>
  platform?: 'darwin' | 'windows'
}

/** The six tabs (`.ws__tabs`) and the quick field that opens the palette (milestone 3 builds the palette). */
export function TabBar({ value, onChange, counts, platform = guessPlatform() }: TabBarProps) {
  const { t } = useI18n()
  const mac = isMac(platform)
  const onKey = (e: KeyboardEvent<HTMLDivElement>) => {
    const i = WS_TABS.indexOf(value)
    let next = -1
    if (e.key === 'ArrowRight') next = (i + 1) % WS_TABS.length
    else if (e.key === 'ArrowLeft') next = (i - 1 + WS_TABS.length) % WS_TABS.length
    else if (e.key === 'Home') next = 0
    else if (e.key === 'End') next = WS_TABS.length - 1
    if (next < 0) return
    e.preventDefault()
    onChange(WS_TABS[next])
    requestAnimationFrame(() => document.getElementById(tabId(WS_TABS[next]))?.focus())
  }
  return (
    <div className="ws__tabs">
      <div className="ws__tablist" role="tablist" aria-label={t('ws.tabs.aria')} onKeyDown={onKey}>
        {WS_TABS.map((tab, i) => {
          const Icon = ICONS[tab]
          const n = counts?.[tab] ?? 0
          const active = tab === value
          return (
            <Tooltip key={tab} content={`${mac ? '⌃' : 'Alt+'}${i + 1}`}>
              <button
                id={tabId(tab)}
                type="button"
                role="tab"
                aria-selected={active}
                aria-controls={panelId(tab)}
                tabIndex={active ? 0 : -1}
                className={clsx('tab', active && 'is-active')}
                onClick={() => onChange(tab)}
              >
                <Icon aria-hidden="true" />
                {t(LABELS[tab])}
                {n > 0 && <Badge>{n}</Badge>}
              </button>
            </Tooltip>
          )
        })}
      </div>
      <Tooltip content={t('ws.quick.soon')}>
        <button type="button" className="ws__quick" aria-disabled="true">
          <Search aria-hidden="true" />
          <span>{t('ws.quick')}</span>
          <span className="keys" aria-hidden="true">
            <Kbd>{mac ? '⌘' : 'Ctrl'}</Kbd>
            <Kbd>K</Kbd>
          </span>
        </button>
      </Tooltip>
    </div>
  )
}
