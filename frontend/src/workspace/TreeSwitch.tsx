import clsx from 'clsx'
import { Check, ChevronDown, GitBranch } from 'lucide-react'
import { Menu } from '../components'
import type { MenuItem } from '../components/Menu'
import { useI18n } from '../i18n'
import type { T } from '../i18n'
import type { WorkTree } from '../state/workspace'

/** "主工作区 · main": the row's and the pill's text. */
export function treeLabel(tree: WorkTree, t: T): string {
  const name = tree.kind === 'main' ? t('ws.tree.main') : (tree.title ?? tree.id)
  return tree.branch ? `${name} · ${tree.branch}` : name
}

export interface TreeSwitchProps {
  trees: readonly WorkTree[]
  current: string
  onSelect: (id: string) => void
}

/**
 * The working-tree switcher (design 4.2): a menu with the main workspace first, then one row per AI task that has its own
 * folder (milestone 3). Choosing a row re-points the tabs and the terminal at that folder. The branch items of G4-06 go
 * under a separator at the foot of `items`.
 */
export function TreeSwitch({ trees, current, onSelect }: TreeSwitchProps) {
  const { t } = useI18n()
  const cur = trees.find((x) => x.id === current) ?? trees[0]
  const items: MenuItem[] = trees.map((tr) => ({
    id: tr.id,
    icon: tr.id === cur?.id ? Check : GitBranch,
    label: treeLabel(tr, t),
    onSelect: () => onSelect(tr.id),
  }))
  const label = cur ? treeLabel(cur, t) : t('ws.tree.main')
  return (
    <Menu
      items={items}
      trigger={
        <button type="button" className={clsx('tree-switch', cur?.kind === 'task' && 'is-task')} aria-label={t('ws.tree.aria', { name: label })}>
          <GitBranch aria-hidden="true" />
          <span>{cur?.kind === 'main' || !cur ? t('ws.tree.main') : (cur.title ?? cur.id)}</span>
          {cur?.branch && (
            <>
              <span aria-hidden="true">·</span>
              <b>{cur.branch}</b>
            </>
          )}
          <ChevronDown aria-hidden="true" />
        </button>
      }
    />
  )
}
