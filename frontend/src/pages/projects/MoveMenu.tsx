import { Check, FolderPlus } from 'lucide-react'
import { useEffect, useRef } from 'react'
import type { KeyboardEvent, RefObject } from 'react'
import type { ProjectInfo } from '../../api/models'
import { Popover } from '../../components'
import { useI18n } from '../../i18n'
import { UNGROUPED } from './model'

export interface MoveMenuProps {
  open: boolean
  anchorRef: RefObject<HTMLElement | null>
  onClose: () => void
  projects: ProjectInfo[]
  /** Project IDs ('' = Ungrouped) every moved repository is already in: marked and not offered. */
  currentIds: ReadonlySet<string>
  onPick: (projectId: string) => void
  onNew: () => void
  /** How many repositories the menu moves, for its label. */
  count: number
}

/** "Move to project": a menu of every project, Ungrouped and "New project...". Opened by M, the row menu and the bulk bar. */
export function MoveMenu({ open, anchorRef, onClose, projects, currentIds, onPick, onNew, count }: MoveMenuProps) {
  const { t } = useI18n()
  const listRef = useRef<HTMLDivElement>(null)
  const buttons = () => Array.from(listRef.current?.querySelectorAll<HTMLButtonElement>('button[role="menuitem"]:not(:disabled)') ?? [])

  useEffect(() => {
    if (open) window.setTimeout(() => buttons()[0]?.focus(), 0)
  }, [open])

  const typed = useRef({ text: '', at: 0 })
  const onKey = (e: KeyboardEvent<HTMLDivElement>) => {
    const bs = buttons()
    // Type-ahead: letters typed together jump to the next item that starts with them.
    if (e.key.length === 1 && e.key !== ' ' && !e.ctrlKey && !e.metaKey && !e.altKey) {
      const now = Date.now()
      typed.current = { text: (now - typed.current.at > 700 ? '' : typed.current.text) + e.key.toLowerCase(), at: now }
      const cur = bs.indexOf(document.activeElement as HTMLButtonElement)
      const order = [...bs.slice(cur + 1), ...bs.slice(0, cur + 1)]
      const hit = order.find((b) => (b.textContent ?? '').trim().toLowerCase().startsWith(typed.current.text))
      if (hit) {
        e.preventDefault()
        hit.focus()
      }
      return
    }
    const i = bs.indexOf(document.activeElement as HTMLButtonElement)
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      bs[(i + 1) % bs.length]?.focus()
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      bs[(i - 1 + bs.length) % bs.length]?.focus()
    } else if (e.key === 'Home') {
      e.preventDefault()
      bs[0]?.focus()
    } else if (e.key === 'End') {
      e.preventDefault()
      bs[bs.length - 1]?.focus()
    } else if (e.key === 'Tab') {
      onClose()
    }
  }

  const item = (id: string, label: string) => {
    const here = currentIds.has(id)
    return (
      <button key={id || 'none'} type="button" role="menuitem" className="menu__item" disabled={here} aria-current={here ? 'true' : undefined} onClick={() => onPick(id)}>
        <span className="proj-move__label">{label}</span>
        {here && <Check size={14} aria-hidden="true" />}
      </button>
    )
  }

  return (
    <Popover open={open} anchorRef={anchorRef} onClose={onClose} ariaLabel={t('projects.move.menu', { n: count })} className="proj-move">
      <div ref={listRef} role="menu" aria-label={t('projects.move.menu', { n: count })} onKeyDown={onKey}>
        {projects.map((p) => item(p.id, p.name))}
        {item(UNGROUPED, t('projects.ungrouped'))}
        <div className="menu__sep" role="separator" />
        <button type="button" role="menuitem" className="menu__item" onClick={onNew}>
          <FolderPlus size={14} aria-hidden="true" />
          <span className="proj-move__label">{t('projects.move.new')}</span>
        </button>
      </div>
    </Popover>
  )
}
