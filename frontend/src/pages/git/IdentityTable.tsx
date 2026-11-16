import { Layers, Pencil } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { takeOpen } from '../../lib/initialUI'
import type { ReactNode } from 'react'
import type { GlobalIdentity, IdentityOverview, ProjectIdentity, RepoIdentity } from '../../api/models'
import { Button, IconButton } from '../../components'
import { RepoName } from '../../components/RepoName'
import type { RepoMeta } from './IdentitySection'
import { useI18n } from '../../i18n'
import { DUR, exitDelay } from '../../lib/motion'
import { sourceTag } from './identityModel'
import { ProjectSheet } from './ProjectSheet'
import { RepoGlyph } from './RepoGlyph'
import { RepoPopover } from './RepoPopover'

function SourceTag({ source }: { source: Parameters<typeof sourceTag>[0] }) {
  const { t } = useI18n()
  const tag = sourceTag(source)
  const cls = tag.tone === 'proj' ? 'src-tag src-tag--proj' : tag.tone === 'repo' ? 'src-tag src-tag--repo' : tag.tone === 'warn' ? 'src-tag src-tag--warn' : 'src-tag'
  return (
    <span className={cls} title={tag.title ? t(tag.title.key, tag.title.params) : undefined}>
      {t(tag.key, tag.params)}
    </span>
  )
}

function Who({ name, email }: { name: string; email: string }) {
  return (
    <span className="ident" title={`${name} <${email}>`}>
      {name} <span>&lt;{email}&gt;</span>
    </span>
  )
}

interface RepoRowProps {
  row: RepoIdentity
  project?: ProjectIdentity
  global: GlobalIdentity
  meta: RepoMeta
  editing: boolean
  onEdit: (id: string | null) => void
  onRetry: () => void
}

function RepoRow({ row, project, global, meta, editing, onEdit, onRetry }: RepoRowProps) {
  const { t } = useI18n()
  const btn = useRef<HTMLButtonElement>(null)
  const eff = row.effective
  let identity: ReactNode
  let tag: ReactNode = null
  if (row.missing) identity = <span className="ident is-inherit">{t('identity.table.missing')}</span>
  else if (row.readError) {
    const rePath = row.readError.params?.file ?? row.readError.params?.path
    identity = (
      <>
        <span className="ident is-danger" title={rePath ? `${t('identity.table.readError')} ${rePath}` : undefined}>
          {t('identity.table.readError')}
          {rePath && <span className="mono hint"> {rePath}</span>}
        </span>
        <button type="button" className="btn btn--ghost btn--sm ident-retry" onClick={onRetry}>
          {t('common.retry')}
        </button>
      </>
    )
  } else if (!eff.set) {
    identity = <span className="ident is-warn">{t('identity.table.notSet')}</span>
  } else {
    identity = <Who name={eff.name} email={eff.email} />
    tag = <SourceTag source={eff.source} />
  }
  return (
    <div className="row is-child" role="row">
      <span className="row__cell" role="cell">
        <RepoGlyph provider={meta.provider} />
        <RepoName localName={row.name} remoteName={meta.remoteName} className="repo-name" />
      </span>
      <span className="row__cell" role="cell">
        {identity}
      </span>
      <span className="row__cell" role="cell">
        {tag}
      </span>
      <span className="row__cell row__actions" role="cell">
        {!row.missing && (
          <IconButton ref={btn} label={t('identity.table.edit', { name: row.name })} icon={Pencil} size="sm" aria-haspopup="dialog" aria-expanded={editing} onClick={() => onEdit(editing ? null : row.repoId)} />
        )}
        {editing && <RepoPopover anchorRef={btn} row={row} project={project} global={global} onClose={() => onEdit(null)} onRecovered={onRetry} />}
      </span>
    </div>
  )
}

export interface IdentityTableProps {
  overview: IdentityOverview
  metaOf: (repoId: string, name: string) => RepoMeta
  reload: () => void
}

/** The effective identity table (`.ident-table`): projects with their repositories, then the ungrouped ones. */
export function IdentityTable({ overview, metaOf, reload }: IdentityTableProps) {
  const { t } = useI18n()
  // ?open=identity:project:<id>[:<email>] and ?open=identity:repo:<id> (mock screenshots): consumed once.
  const [start] = useState(() => {
    const p = takeOpen('identity:project:')
    if (p !== null) {
      const [id, ...rest] = p.split(':')
      return { sheet: id, email: rest.join(':'), edit: null as string | null }
    }
    const r = takeOpen('identity:repo:')
    return { sheet: null as string | null, email: '', edit: r }
  })
  // The sheet stays mounted while it leaves (Sheet plays its exit), so its project id outlives the open flag.
  const [sheet, setSheet] = useState<string | null>(start.sheet)
  const [sheetOpen, setSheetOpen] = useState(start.sheet !== null)
  const [editing, setEditing] = useState<string | null>(start.edit)
  const projects = overview.projects ?? []
  const ungrouped = overview.ungrouped ?? []
  const open = projects.find((p) => p.id === sheet)
  useEffect(() => {
    if (sheetOpen || sheet === null) return
    const id = window.setTimeout(() => setSheet(null), exitDelay(DUR.leave) + 40)
    return () => window.clearTimeout(id)
  }, [sheetOpen, sheet])

  const head = (
    <div className="table__head" role="row">
      <span role="columnheader">{t('identity.table.name')}</span>
      <span role="columnheader">{t('identity.table.effective')}</span>
      <span role="columnheader">{t('identity.table.source')}</span>
      <span role="columnheader" />
    </div>
  )

  if (projects.length === 0 && ungrouped.length === 0) {
    return (
      <div className="stack" style={{ '--gap': 'var(--sp-3)' } as React.CSSProperties}>
        <h2 className="section-label">{t('identity.table.title')}</h2>
        <div className="hint">{t('identity.table.empty')}</div>
      </div>
    )
  }

  const rows = (list: RepoIdentity[], project?: ProjectIdentity) =>
    list.map((r) => <RepoRow key={r.repoId} row={r} project={project} global={overview.global} meta={metaOf(r.repoId, r.name)} editing={editing === r.repoId} onEdit={setEditing} onRetry={reload} />)

  return (
    <div className="stack" style={{ '--gap': 'var(--sp-3)' } as React.CSSProperties}>
      <h2 className="section-label">{t('identity.table.title')}</h2>
      <div className="table ident-table" role="table" aria-label={t('identity.table.title')}>
        {head}
        {projects.map((p) => (
          <div key={p.id} role="rowgroup">
            <div className="row is-proj" role="row">
              <span className="row__cell" role="cell">
                <span className="row__primary">
                  <Layers aria-hidden="true" />
                  {p.name}
                  <span className="muted" style={{ fontWeight: 400, fontSize: 12 }}>
                    {t((p.repos ?? []).length === 1 ? 'identity.table.repos.one' : 'identity.table.repos', { n: (p.repos ?? []).length })}
                  </span>
                </span>
              </span>
              <span className="row__cell" role="cell">
                {p.identity ? <Who name={p.identity.name} email={p.identity.email} /> : <span className="ident is-inherit">{t('identity.table.followsGlobal')}</span>}
              </span>
              <span className="row__cell" role="cell">
                {p.identity && <span className="src-tag src-tag--proj">{t('identity.src.project')}</span>}
              </span>
              <span className="row__cell row__actions" role="cell">
                <Button size="sm" aria-label={t('identity.table.setFor', { name: p.name })} onClick={() => {
                    setSheet(p.id)
                    setSheetOpen(true)
                  }}>
                  {t('identity.table.set')}
                </Button>
              </span>
            </div>
            {rows(p.repos ?? [], p)}
          </div>
        ))}
        {ungrouped.length > 0 && (
          <div role="rowgroup">
            <div className="row is-proj is-group" role="row">
              <span className="row__cell" role="cell">
                {t('identity.table.ungrouped')}
              </span>
              <span role="cell" />
              <span role="cell" />
              <span role="cell" />
            </div>
            {rows(ungrouped)}
          </div>
        )}
      </div>
      {open && <ProjectSheet key={open.id} open={sheetOpen} project={open} draftEmail={start.email} global={overview.global} metaOf={metaOf} onClose={() => setSheetOpen(false)} />}
    </div>
  )
}
