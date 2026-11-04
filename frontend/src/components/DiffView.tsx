import clsx from 'clsx'
import type { ReactNode } from 'react'
import { DiffKind } from '../api/models'
import type { FilePreview } from '../api/models'
import { useI18n } from '../i18n'

export interface DiffViewProps {
  preview: FilePreview
  title?: ReactNode
  /** Default: t('common.diff.fold'). */
  foldLabel?: (n: number) => string
  className?: string
}

const CLASS: Record<string, string> = {
  [DiffKind.DiffContext]: 'diff-ctx',
  [DiffKind.DiffAdded]: 'diff-add',
  [DiffKind.DiffRemoved]: 'diff-del',
  [DiffKind.DiffFolded]: 'diff-com',
}

/** `.code--diff`: header shows the file (or `title`) and "N added, M removed"; folded runs become one comment line. */
export function DiffView({ preview, title, foldLabel, className }: DiffViewProps) {
  const { t } = useI18n()
  const fold = foldLabel ?? ((n: number) => t('common.diff.fold', { n }))
  return (
    <div className={clsx('code', 'code--diff', className)}>
      <div className="code__head">
        <span className="code__lang mono">{title ?? preview.file}</span>
        <span style={{ marginLeft: 'auto' }}>{t('common.diff.summary', { added: preview.added, removed: preview.removed })}</span>
      </div>
      <pre className="code__body">
        {(preview.lines ?? []).map((l, i) => (
          <span key={i} className={CLASS[l.kind] ?? 'diff-ctx'}>
            {l.kind === DiffKind.DiffFolded ? fold(l.count ?? 0) : l.text || ' '}
          </span>
        ))}
      </pre>
    </div>
  )
}
