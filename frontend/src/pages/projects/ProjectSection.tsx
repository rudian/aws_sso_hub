import clsx from 'clsx'
import { ChevronRight, Ellipsis, FolderClosed, IdCard, Layers, Pencil, Plus, Trash2, User } from 'lucide-react'
import { useEffect, useId, useRef, useState } from 'react'
import type { KeyboardEvent, ReactNode } from 'react'
import type { Author, Repo } from '../../api/models'
import { AcctChip, Checkbox, IconButton, Menu } from '../../components'
import type { MenuItem } from '../../components/Menu'
import { useI18n } from '../../i18n'
import { HIDDEN_SECTION, UNGROUPED, peekProfiles } from './model'
import type { Section } from './model'

export interface ProjectSectionProps {
  section: Section
  collapsed: boolean
  onToggle: () => void
  identity?: Author
  /** The title is an input (Enter saves, Esc cancels). */
  renaming: boolean
  onRenameStart: () => void
  onRename: (name: string) => void
  onRenameCancel: () => void
  onAddRepos: () => void
  onIdentity: () => void
  onRemove: () => void
  /** A drag is over this section. */
  dropOver: boolean
  /** A drag is running: every section is a possible target. */
  dragging: boolean
  selected: ReadonlySet<string>
  onSelectAll: (ids: string[], on: boolean) => void
  /** A filter or search hides some rows, so an empty body means "no match" rather than "empty project". */
  constrained: boolean
  /** The one repository that is this table's tab stop. */
  renderRow: (r: Repo, tabStop: boolean) => ReactNode
  tabStopId: string | undefined
  children?: ReactNode
}

function RenameInput({ value, onSave, onCancel, label }: { value: string; onSave: (v: string) => void; onCancel: () => void; label: string }) {
  const [text, setText] = useState(value)
  const done = useRef(false)
  const ref = useRef<HTMLInputElement>(null)
  useEffect(() => {
    ref.current?.focus()
    ref.current?.select()
  }, [])
  const finish = (save: boolean) => {
    if (done.current) return
    done.current = true
    if (save && text.trim() !== '' && text.trim() !== value) onSave(text.trim())
    else onCancel()
  }
  const onKey = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter') {
      e.preventDefault()
      finish(true)
    } else if (e.key === 'Escape') {
      e.preventDefault()
      e.stopPropagation()
      finish(false)
    }
  }
  return <input ref={ref} className="input proj__rename" aria-label={label} value={text} maxLength={200} onChange={(e) => setText(e.target.value)} onKeyDown={onKey} onBlur={() => finish(true)} />
}

export function ProjectSection(p: ProjectSectionProps) {
  const { t } = useI18n()
  const { section, collapsed } = p
  const bodyId = useId()
  const isHidden = section.id === HIDDEN_SECTION
  const isUngrouped = section.id === UNGROUPED
  const name = isHidden ? t('projects.hidden.title') : isUngrouped ? t('projects.ungrouped') : (section.project?.name ?? '')
  const ids = section.repos.map((r) => r.id)
  const allSel = ids.length > 0 && ids.every((id) => p.selected.has(id))
  const someSel = ids.some((id) => p.selected.has(id))
  const peek = collapsed ? peekProfiles(section.repos) : []

  const menu: MenuItem[] = section.project
    ? [
        { id: 'rename', label: t('projects.menu.rename'), icon: Pencil, onSelect: p.onRenameStart },
        { id: 'identity', label: t('projects.menu.identity'), icon: IdCard, onSelect: p.onIdentity },
        { id: 'add', label: t('projects.menu.add'), icon: Plus, onSelect: p.onAddRepos },
        { id: 'remove', label: t('projects.menu.remove'), icon: Trash2, danger: true, separatorBefore: true, onSelect: p.onRemove },
      ]
    : []

  const Icon = isUngrouped || isHidden ? FolderClosed : Layers
  return (
    <section className={clsx('proj', collapsed && 'is-collapsed', p.dropOver && 'is-drop', p.dragging && 'is-target')} data-drop={section.id} data-section={section.id} aria-labelledby={`${bodyId}-t`}>
      <div className="proj__head">
        <button type="button" className="portal__toggle" aria-expanded={!collapsed} aria-controls={bodyId} aria-label={t(collapsed ? 'projects.expand' : 'projects.collapse', { name })} onClick={p.onToggle}>
          <ChevronRight className="portal__chevron" aria-hidden="true" />
          <span className="portal__icon">
            <Icon aria-hidden="true" />
          </span>
        </button>
        <div className="proj__id">
          {p.renaming ? (
            <RenameInput value={name} label={t('projects.rename.label')} onSave={p.onRename} onCancel={p.onRenameCancel} />
          ) : (
            <h2 className="portal__title" id={`${bodyId}-t`}>
              {name}
            </h2>
          )}
          <div className="portal__meta">
            <span>{t('projects.meta.repos', { n: section.total })}</span>
            {peek.length > 0 && (
              <span className="portal__peek">
                {peek.map((r) => (
                  <AcctChip key={r.binding.profile} slot={r.binding.colorSlot} monogram={r.binding.monogram} name={r.binding.profile ?? ''} accountId={r.binding.accountId} />
                ))}
              </span>
            )}
          </div>
        </div>
        {p.identity && (
          <span className="proj__ident" title={t('projects.identity.title')}>
            <User aria-hidden="true" />
            <b>{p.identity.name}</b> &lt;{p.identity.email}&gt;
          </span>
        )}
        {menu.length > 0 && <Menu align="end" trigger={<IconButton size="sm" label={t('projects.menu.aria', { name })} icon={Ellipsis} />} items={menu} />}
      </div>
      <div className="proj__body" id={bodyId} inert={collapsed}>
        <div>
          {section.repos.length === 0 ? (
            <div className="proj__empty">{p.constrained ? t('projects.empty.match') : isHidden ? t('projects.hidden.empty') : t('projects.empty.project')}</div>
          ) : (
            <div className="table proj-table" role="grid" aria-label={name} aria-rowcount={section.repos.length + 1}>
              <div className="table__head" role="row">
                <span role="columnheader">
                  {!isHidden && (
                    <Checkbox
                      checked={allSel}
                      aria-label={t('projects.selectAll', { name })}
                      ref={(el) => {
                        if (el) el.indeterminate = !allSel && someSel
                      }}
                      onChange={(e) => p.onSelectAll(ids, e.target.checked)}
                    />
                  )}
                </span>
                <span role="columnheader">{t('repos.col.repo')}</span>
                <span role="columnheader">{t('repos.col.account')}</span>
                <span role="columnheader">{t('projects.col.branch')}</span>
                <span role="columnheader">{t('repos.col.status')}</span>
                <span role="columnheader">
                  <span className="sr-only">{t('repos.col.actions')}</span>
                </span>
              </div>
              {section.repos.map((r) => p.renderRow(r, r.id === p.tabStopId))}
            </div>
          )}
        </div>
      </div>
      {p.children}
    </section>
  )
}
