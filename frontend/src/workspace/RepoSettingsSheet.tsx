import { useEffect, useRef, useState } from 'react'
import { Identity, Repos } from '../api'
import type { Repo, RepoIdentity } from '../api/models'
import { AcctChip, Button, Sheet, Skeleton } from '../components'
import { onEvent } from '../api/events'
import { useI18n } from '../i18n'
import { useAsync } from '../lib/useAsync'
import { setRoute } from '../lib/route'
import { useActions } from '../shell/AppActions'
import { useRepos } from '../state/repos'
import { RepoPopover } from '../pages/git/RepoPopover'
import { useReportError } from '../pages/repos/hooks'
import { chipBindable, isCodeCommit } from '../pages/projects/model'

export interface RepoSettingsSheetProps {
  repo: Repo
  open: boolean
  onClose: () => void
  onBind: (anchor: HTMLElement | null) => void
}

const who = (a?: { name: string; email: string } | null) => (a && a.email ? `${a.name} <${a.email}>` : '')

/**
 * The repository settings sheet (gear, design 4.2): account binding and author identity reuse what the Projects page
 * has (bind popover, identity popover); remotes and AI defaults are placeholders until their tasks; hiding is the
 * Projects page's own action.
 */
export function RepoSettingsSheet({ repo, open, onClose, onBind }: RepoSettingsSheetProps) {
  const { t } = useI18n()
  const { toast } = useActions()
  const { reload } = useRepos()
  const report = useReportError()
  const bindRef = useRef<HTMLButtonElement>(null)
  const identRef = useRef<HTMLButtonElement>(null)
  const [identOpen, setIdentOpen] = useState(false)
  const [hiding, setHiding] = useState(false)

  // The identity overview is read only while the sheet is open
  const ov = useAsync(() => (open ? Identity.GetOverview() : Promise.resolve(null)), [open, repo.id])
  // Every apply and undo of an identity emits identity:changed: while the sheet is open, read the overview again
  useEffect(() => (open ? onEvent('identity:changed', () => void ov.reload()) : undefined), [open, ov.reload])
  const overview = ov.data
  const project = overview?.projects?.find((p) => p.repos?.some((r) => r.repoId === repo.id))
  const row: RepoIdentity | undefined = project?.repos?.find((r) => r.repoId === repo.id) ?? overview?.ungrouped?.find((r) => r.repoId === repo.id)

  const b = repo.binding
  const cc = isCodeCommit(repo)
  const bindable = chipBindable(repo)

  const hide = async () => {
    setHiding(true)
    try {
      await Repos.SetHidden(repo.id, true)
      // Leave first: the repos:changed event removes the repository from the list, and the sheet must not outlive it
      onClose()
      setRoute({ page: 'projects' })
      void reload().catch(() => undefined)
      toast({
        kind: 'ok',
        title: t('projects.hide.done', { name: repo.localName }),
        action: { label: t('common.undo'), onClick: () => void Repos.SetHidden(repo.id, false).then(() => reload()).catch(report) },
      })
    } catch (e) {
      report(e)
    } finally {
      setHiding(false)
    }
  }

  return (
    <Sheet open={open} onClose={onClose} title={t('ws.set.title')} subtitle={repo.displayPath} footer={<Button onClick={onClose}>{t('common.close')}</Button>}>
      <section className="ws-set" aria-labelledby="ws-set-acct">
        <h3 id="ws-set-acct" className="ws-set__h">{t('ws.set.account')}</h3>
        {cc ? (
          <div className="ws-set__row">
            {b.profile && b.profileKnown ? <AcctChip slot={b.colorSlot} monogram={b.monogram} name={b.profile} accountId={b.accountId} /> : b.profile ? <AcctChip none name={b.profile} /> : <span className="ws-set__muted">{t('ws.set.account.unbound')}</span>}
            <span className="spacer" />
            {bindable && (
              <Button ref={bindRef} size="sm" onClick={() => onBind(bindRef.current)}>
                {b.profile ? t('ws.set.account.change') : t('ws.set.account.bind')}
              </Button>
            )}
          </div>
        ) : (
          <p className="ws-set__muted">{t('ws.set.account.none')}</p>
        )}
      </section>

      <section className="ws-set" aria-labelledby="ws-set-ident">
        <h3 id="ws-set-ident" className="ws-set__h">{t('ws.set.identity')}</h3>
        {ov.loading && !overview ? (
          <Skeleton variant="block" height={32} />
        ) : (
          <div className="ws-set__row">
            <span className="ws-set__val">{row ? who(row.effective) || t('ws.set.identity.unset') : t('ws.set.identity.unset')}</span>
            <span className="spacer" />
            {row && overview && (
              <Button ref={identRef} size="sm" onClick={() => setIdentOpen(true)}>
                {t('ws.set.identity.change')}
              </Button>
            )}
          </div>
        )}
        {identOpen && row && overview && (
          <RepoPopover
            anchorRef={identRef}
            row={row}
            project={project}
            global={overview.global}
            onClose={() => setIdentOpen(false)}
            onRecovered={() => void ov.reload()}
          />
        )}
      </section>

      <section className="ws-set" aria-labelledby="ws-set-remotes">
        <h3 id="ws-set-remotes" className="ws-set__h">{t('ws.set.remotes')}</h3>
        {repo.remoteUrl && (
          <div className="ws-set__row">
            <code className="ws-set__code">{repo.remote || 'origin'}</code>
            <span className="ws-set__val mono" title={repo.remoteUrl}>{repo.remoteUrl}</span>
          </div>
        )}
        <p className="ws-set__muted">{t('ws.set.remotes.soon')}</p>
      </section>

      <section className="ws-set" aria-labelledby="ws-set-ai">
        <h3 id="ws-set-ai" className="ws-set__h">{t('ws.set.ai')}</h3>
        <p className="ws-set__muted">{t('ws.set.ai.soon')}</p>
      </section>

      <section className="ws-set" aria-labelledby="ws-set-hide">
        <h3 id="ws-set-hide" className="ws-set__h">{t('ws.set.hide')}</h3>
        <div className="ws-set__row">
          <span className="ws-set__muted">{t('ws.set.hide.hint')}</span>
          <span className="spacer" />
          <Button size="sm" loading={hiding} onClick={() => void hide()}>
            {t('ws.set.hide.button')}
          </Button>
        </div>
      </section>
    </Sheet>
  )
}
