import { useMemo, useState } from 'react'
import type { Repo } from '../../api/models'
import { Button, Modal } from '../../components'
import { useI18n } from '../../i18n'
import { MovedAsk } from './MovedAsk'
import { defaultFoundSelection } from './model'
import { RepoChecklist } from './RepoChecklist'

export interface FoundChooserProps {
  found: Repo[]
  now: number
  onClose: () => void
  /** Show the ticked repositories and hide the rest. Resolves true when it worked. */
  onResolve: (show: string[], hide: string[]) => Promise<boolean>
  onReattach: (r: Repo) => Promise<void>
  onDecline: (r: Repo) => Promise<void>
}

/** "Choose which to show": every newly found repository, those with a commit in the last 90 days pre-ticked (design 5.2). */
export function FoundChooser({ found, now, onClose, onResolve, onReattach, onDecline }: FoundChooserProps) {
  const { t, lang } = useI18n()
  const [checked, setChecked] = useState<Set<string>>(() => defaultFoundSelection(found, now))
  const [busy, setBusy] = useState(false)
  const [asking, setAsking] = useState<string | null>(null)
  const fmt = useMemo(() => new Intl.DateTimeFormat(lang, { year: 'numeric', month: 'short', day: 'numeric' }), [lang])

  const toggle = (id: string, on: boolean) =>
    setChecked((cur) => {
      const n = new Set(cur)
      if (on) n.add(id)
      else n.delete(id)
      return n
    })

  const submit = async () => {
    setBusy(true)
    const show = found.filter((r) => checked.has(r.id)).map((r) => r.id)
    const hide = found.filter((r) => !checked.has(r.id)).map((r) => r.id)
    const ok = await onResolve(show, hide)
    setBusy(false)
    if (ok) onClose()
  }

  const answer = async (r: Repo, fn: (r: Repo) => Promise<void>) => {
    setAsking(r.id)
    await fn(r)
    setAsking(null)
  }

  const n = found.filter((r) => checked.has(r.id)).length
  return (
    <Modal
      open
      title={t('projects.found.title')}
      subtitle={t('projects.found.subtitle', { n: found.length })}
      onClose={onClose}
      dismissible={!busy}
      footer={
        <>
          <Button variant="ghost" onClick={onClose} disabled={busy}>
            {t('common.cancel')}
          </Button>
          <Button variant="primary" loading={busy} onClick={() => void submit()}>
            {t('projects.found.show', { n })}
          </Button>
        </>
      }
    >
      <p className="proj-found__note">{t('projects.found.note', { days: 90 })}</p>
      <RepoChecklist
        repos={found}
        checked={checked}
        onToggle={toggle}
        onToggleAll={(on) => setChecked(on ? new Set(found.map((r) => r.id)) : new Set())}
        ariaLabel={t('projects.found.title')}
        meta={(r) => (r.lastCommitAt ? t('projects.found.lastCommit', { date: fmt.format(new Date(r.lastCommitAt)) }) : t('projects.found.noCommit'))}
        extra={(r) =>
          r.movedFrom ? <MovedAsk name={r.movedFrom.name} busy={asking === r.id} onYes={() => void answer(r, onReattach)} onNo={() => void answer(r, onDecline)} /> : null
        }
      />
    </Modal>
  )
}
