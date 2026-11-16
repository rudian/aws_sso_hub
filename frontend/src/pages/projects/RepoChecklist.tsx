import { FolderGit2 } from 'lucide-react'
import type { ReactNode } from 'react'
import type { Repo } from '../../api/models'
import { Checkbox, ProviderMark } from '../../components'
import { RepoName } from '../../components/RepoName'
import { useI18n } from '../../i18n'
import { providerKind } from './model'

export interface RepoChecklistProps {
  repos: Repo[]
  checked: ReadonlySet<string>
  onToggle: (id: string, on: boolean) => void
  onToggleAll: (on: boolean) => void
  ariaLabel: string
  /** Text next to a row (the last commit date in the found chooser). */
  meta?: (r: Repo) => ReactNode
  /** Something under the row (the moved-folder question). */
  extra?: (r: Repo) => ReactNode
  /** Select all is not drawn for an empty or one-row list. */
  empty?: ReactNode
}

/** A scrolling checklist of repositories with a "Select all" bar: the New project dialog and the found chooser. */
export function RepoChecklist({ repos, checked, onToggle, onToggleAll, ariaLabel, meta, extra, empty }: RepoChecklistProps) {
  const { t } = useI18n()
  if (repos.length === 0) return <div className="proj-check__empty">{empty}</div>
  const all = repos.every((r) => checked.has(r.id))
  const some = repos.some((r) => checked.has(r.id))
  const n = repos.filter((r) => checked.has(r.id)).length
  return (
    <div className="proj-check" role="group" aria-label={ariaLabel}>
      <div className="proj-check__bar">
        <label className="proj-check__all">
          <input
            type="checkbox"
            className="checkbox"
            checked={all}
            ref={(el) => {
              if (el) el.indeterminate = some && !all
            }}
            onChange={(e) => onToggleAll(e.target.checked)}
          />
          <span>{t('projects.check.all')}</span>
        </label>
        <span className="proj-check__count" aria-live="polite">
          {t('projects.check.count', { n })}
        </span>
      </div>
      <div className="proj-check__list">
        {repos.map((r) => {
          const kind = providerKind(r)
          return (
            <div key={r.id} className="proj-check__row">
              <Checkbox checked={checked.has(r.id)} aria-label={t('repos.select', { name: r.localName })} onChange={(e) => onToggle(r.id, e.target.checked)} />
              {kind ? <ProviderMark provider={kind} /> : <span className="prov" aria-hidden="true"><FolderGit2 size={15} /></span>}
              <span className="proj-check__text">
                <RepoName localName={r.localName} remoteName={r.remoteName} className="row__primary truncate" />
                <span className="row__secondary mono truncate" title={r.displayPath}>
                  {r.displayPath}
                </span>
                {extra?.(r)}
              </span>
              {meta && <span className="proj-check__meta">{meta(r)}</span>}
            </div>
          )
        })}
      </div>
    </div>
  )
}
