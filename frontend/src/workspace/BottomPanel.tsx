import clsx from 'clsx'
import { X } from 'lucide-react'
import { useRef } from 'react'
import type { KeyboardEvent as ReactKeyboardEvent, PointerEvent as ReactPointerEvent } from 'react'
import type { Repo } from '../api/models'
import { IconButton } from '../components'
import { useI18n } from '../i18n'
import type { MessageKey } from '../i18n'
import { PANEL_STEP, PANEL_TABS, clampPanelHeight } from '../state/workspace'
import type { PanelState, PanelTab } from '../state/workspace'
import { TerminalSlot } from './slots'

const LABELS: Record<PanelTab, MessageKey> = { terminal: 'ws.panel.terminal', problems: 'ws.panel.problems', find: 'ws.panel.find' }

export interface BottomPanelProps {
  repo: Repo
  folderId: string
  panel: PanelState
  onChange: (patch: Partial<PanelState>) => void
}

/** The bottom panel (`.bpanel`): Terminal / Problems / Find results under any tab, 196px by default, resizable. */
export function BottomPanel({ repo, folderId, panel, onChange }: BottomPanelProps) {
  const { t } = useI18n()
  const drag = useRef<{ y: number; h: number } | null>(null)

  const onDown = (e: ReactPointerEvent<HTMLDivElement>) => {
    drag.current = { y: e.clientY, h: panel.height }
    e.currentTarget.setPointerCapture?.(e.pointerId)
  }
  const onMove = (e: ReactPointerEvent<HTMLDivElement>) => {
    if (drag.current) onChange({ height: drag.current.h + (drag.current.y - e.clientY) })
  }
  const onUp = () => {
    drag.current = null
  }
  const onKey = (e: ReactKeyboardEvent<HTMLDivElement>) => {
    if (e.key === 'ArrowUp') onChange({ height: panel.height + PANEL_STEP })
    else if (e.key === 'ArrowDown') onChange({ height: panel.height - PANEL_STEP })
    else return
    e.preventDefault()
  }
  const onTabKey = (e: ReactKeyboardEvent<HTMLDivElement>) => {
    const i = PANEL_TABS.indexOf(panel.tab)
    const next = e.key === 'ArrowRight' ? (i + 1) % PANEL_TABS.length : e.key === 'ArrowLeft' ? (i - 1 + PANEL_TABS.length) % PANEL_TABS.length : -1
    if (next < 0) return
    e.preventDefault()
    onChange({ tab: PANEL_TABS[next] })
    requestAnimationFrame(() => document.getElementById(`ws-ptab-${PANEL_TABS[next]}`)?.focus())
  }

  const height = clampPanelHeight(panel.height)
  return (
    <section className="bpanel" style={{ height }} aria-label={t('ws.panel.aria')}>
      <div
        className="bpanel__grip"
        role="separator"
        aria-orientation="horizontal"
        aria-label={t('ws.panel.resize')}
        aria-valuenow={height}
        tabIndex={0}
        onPointerDown={onDown}
        onPointerMove={onMove}
        onPointerUp={onUp}
        onPointerCancel={onUp}
        onKeyDown={onKey}
      />
      <div className="bpanel__tabs">
        <div className="bpanel__tablist" role="tablist" aria-label={t('ws.panel.tabs')} onKeyDown={onTabKey}>
          {PANEL_TABS.map((tab) => (
            <button
              key={tab}
              id={`ws-ptab-${tab}`}
              type="button"
              role="tab"
              aria-selected={panel.tab === tab}
              tabIndex={panel.tab === tab ? 0 : -1}
              className={clsx('bpanel__tab', panel.tab === tab && 'is-active')}
              onClick={() => onChange({ tab })}
            >
              {t(LABELS[tab])}
            </button>
          ))}
        </div>
        <span className="spacer" />
        <IconButton label={t('ws.panel.close')} icon={X} size="sm" onClick={() => onChange({ open: false })} />
      </div>
      <div className="bpanel__body" role="tabpanel" aria-labelledby={`ws-ptab-${panel.tab}`}>
        {panel.tab === 'terminal' && <TerminalSlot repo={repo} folderId={folderId} />}
        {panel.tab === 'problems' && <div className="ws-empty">{t('ws.panel.problems.none')}</div>}
        {panel.tab === 'find' && <div className="ws-empty">{t('ws.panel.find.none')}</div>}
      </div>
    </section>
  )
}
