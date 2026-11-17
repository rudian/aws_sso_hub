import { useCallback, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import type { BindOutcome, Repo } from '../api/models'
import { toUiError } from '../api/errors'
import { errorText, useI18n } from '../i18n'
import { useActions } from '../shell/AppActions'
import { useAccounts } from '../state/accounts'
import { useRepos } from '../state/repos'
import { BindPopover } from '../pages/repos/BindPopover'
import { useUndoBackup } from '../pages/repos/hooks'
import { bindableProfiles, suggestedProfile } from '../pages/repos/model'

/** The account chip and the settings sheet share the Projects page's bind popover (pages/repos). */
export function useBindPopover(repo: Repo): { open: (anchor: HTMLElement | null) => void; element: ReactNode } {
  const { t } = useI18n()
  const { toast } = useActions()
  const { reload } = useRepos()
  const { state: accounts } = useAccounts()
  const undoBackup = useUndoBackup(reload)
  const anchorRef = useRef<HTMLElement | null>(null)
  const [shown, setShown] = useState(false)

  const open = useCallback((anchor: HTMLElement | null) => {
    anchorRef.current = anchor
    setShown(true)
  }, [])

  const onBound = (out: BindOutcome, profile: string) => {
    setShown(false)
    if (!out.ok) {
      const text = errorText(toUiError(out.error), t)
      toast({ kind: 'danger', title: text.title, body: text.body })
      return
    }
    toast({
      kind: 'ok',
      title: t('repos.bind.done', { repo: repo.localName, name: profile }),
      action: out.backupId ? { label: t('common.undo'), onClick: () => void undoBackup(out.backupId as string) } : undefined,
    })
  }

  const options = bindableProfiles(accounts?.profiles ?? []).map((p) => p.name)
  const element = shown ? (
    <BindPopover key={repo.id} repo={repo} anchorRef={anchorRef} options={options} initial={repo.binding.profile ?? suggestedProfile(repo) ?? ''} onClose={() => setShown(false)} onBound={onBound} />
  ) : null
  return { open, element }
}
