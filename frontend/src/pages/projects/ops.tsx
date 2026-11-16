// What the Projects page does to the backend: moving repositories (with the identity rules of design 5.12),
// removing a project (the move-out rule for each member) and hiding. Each action confirms what it writes,
// reports errors as toasts and says what happened through the live region.
import { useCallback, useMemo } from 'react'
import { Identity, Repos } from '../../api'
import { toUiError } from '../../api/errors'
import { ErrorCode } from '../../api/models'
import { RepoAction } from '../../api/models'
import type { Author, ProjectInfo, Repo, RepoChangePlan, ReposState } from '../../api/models'
import { RepoName } from '../../components/RepoName'
import { errorText, useI18n } from '../../i18n'
import { useActions } from '../../shell/AppActions'
import { useReportError } from '../repos/hooks'
import { needsMoveConfirm, splitMove, writesIdentity } from './model'
import type { MovePreviewItem } from './model'

export interface OpsDeps {
  state: ReposState | null
  identities: ReadonlyMap<string, Author>
  reload(): Promise<void>
  announce(message: string): void
}

export interface ProjectOps {
  /** Move repositories into a project ('' = Ungrouped). Resolves true when at least one moved. */
  moveRepos(ids: readonly string[], projectId: string, targetName?: string): Promise<boolean>
  removeProject(p: ProjectInfo): Promise<boolean>
  /** Save a project's author identity after showing which repositories change (design 5.12). Reports its own result. */
  applyProjectIdentity(projectId: string, name: string, identity: Author): Promise<void>
  hideRepo(r: Repo): Promise<boolean>
  unhideRepo(r: Repo): Promise<void>
}

/** The identity service does not know this repository (the plain move is used instead). */
export function isUnknownRepo(e: unknown): boolean {
  const ue = toUiError(e)
  return ue.code === ErrorCode.ErrInvalidInput && ue.params.field === 'repoId'
}

function who(a: Author | null | undefined): string {
  return a ? `${a.name} <${a.email}>` : ''
}

function RepoList({ items, tone }: { items: MovePreviewItem[]; tone: 'change' | 'keep' }) {
  const { t } = useI18n()
  return (
    <ul className="proj-confirm__list">
      {items.map((it) => (
        <li key={it.repo.id}>
          <RepoName localName={it.repo.localName} remoteName={it.repo.remoteName} className="proj-confirm__name" />
          {tone === 'change' && (
            <span className="proj-confirm__diff">
              {it.preview?.plan.before && <s>{who(it.preview.plan.before)}</s>}
              <span>{it.preview?.plan.after ? who(it.preview.plan.after) : t('projects.identity.global')}</span>
            </span>
          )}
        </li>
      ))}
    </ul>
  )
}

function PlanList({ plans, tone }: { plans: RepoChangePlan[]; tone: 'change' | 'keep' }) {
  const { t } = useI18n()
  return (
    <ul className="proj-confirm__list">
      {plans.map((pl) => (
        <li key={pl.repoId}>
          <RepoName localName={pl.name} className="proj-confirm__name" />
          {tone === 'change' && (
            <span className="proj-confirm__diff">
              {pl.before && <s>{who(pl.before)}</s>}
              <span>{pl.after ? who(pl.after) : t('projects.identity.global')}</span>
            </span>
          )}
        </li>
      ))}
    </ul>
  )
}

export function useProjectOps({ state, identities, reload, announce }: OpsDeps): ProjectOps {
  const { t } = useI18n()
  const { toast, confirm } = useActions()
  const report = useReportError()

  const repoById = useMemo(() => {
    const m = new Map<string, Repo>()
    for (const r of [...(state?.repos ?? []), ...(state?.hiddenRepos ?? [])]) m.set(r.id, r)
    return m
  }, [state])
  const projectName = useCallback(
    (id: string) => (id ? (state?.projects ?? []).find((p) => p.id === id)?.name ?? '' : t('projects.ungrouped')),
    [state, t],
  )

  const preview = useCallback(async (repo: Repo, to: string): Promise<MovePreviewItem> => {
    try {
      return { repo, preview: await Identity.PreviewMove(repo.id, to) }
    } catch (e) {
      if (isUnknownRepo(e)) return { repo }
      throw e
    }
  }, [])

  const moveRepos = useCallback(
    async (idsIn: readonly string[], to: string, targetName?: string): Promise<boolean> => {
      const repos = idsIn.map((id) => repoById.get(id)).filter((r): r is Repo => !!r && (r.projectId ?? '') !== to)
      if (repos.length === 0) return false
      const target = targetName ?? projectName(to)
      let items: MovePreviewItem[]
      try {
        items = []
        for (const r of repos) items.push(await preview(r, to))
      } catch (e) {
        report(e)
        return false
      }
      const split = splitMove(items)
      if (needsMoveConfirm(split, identities.has(to))) {
        const ok = await confirm({
          title: t('projects.move.confirm.title', { project: target, n: items.length }),
          confirmLabel: t('projects.move.confirm.action'),
          body: (
            <div className="proj-confirm">
              {split.change.length > 0 && (
                <>
                  <p>{t('projects.move.confirm.change', { n: split.change.length })}</p>
                  <RepoList items={split.change} tone="change" />
                </>
              )}
              {split.keep.length > 0 && (
                <>
                  <p>{t('projects.move.confirm.keep', { n: split.keep.length })}</p>
                  <RepoList items={split.keep} tone="keep" />
                </>
              )}
              <p className="muted">{t('projects.move.confirm.backup')}</p>
            </div>
          ),
        })
        if (!ok) {
          announce(t('projects.live.cancelled'))
          return false
        }
      }
      const undoIds: string[] = []
      const plain: Array<{ id: string; from: string }> = []
      const moved: Repo[] = []
      for (const it of items) {
        try {
          if (it.preview) {
            const res = await Identity.ApplyMove(it.repo.id, to, it.preview.planHash)
            const bad = (res.outcomes ?? []).find((o) => !o.ok)
            if (bad?.error) {
              const ue = toUiError(bad.error)
              toast({ kind: 'danger', title: t('projects.move.failed', { repo: it.repo.localName }), body: ue.message })
              continue
            }
            if (res.undoId) undoIds.push(res.undoId)
          } else {
            await Repos.MoveToProject([it.repo.id], to)
            plain.push({ id: it.repo.id, from: it.repo.projectId ?? '' })
          }
          moved.push(it.repo)
        } catch (e) {
          report(e)
        }
      }
      await reload()
      if (moved.length === 0) return false
      const msg = moved.length === 1 ? t('projects.move.done.one', { repo: moved[0].localName, project: target }) : t('projects.move.done.many', { n: moved.length, project: target })
      announce(t('projects.live.moved', { project: target, n: moved.length }))
      toast({
        kind: 'ok',
        title: msg,
        action: {
          label: t('common.undo'),
          onClick: () =>
            void (async () => {
              try {
                for (const id of [...undoIds].reverse()) await Identity.Undo(id)
                for (const p of plain) await Repos.MoveToProject([p.id], p.from)
              } catch (e) {
                report(e)
              }
              await reload()
            })(),
        },
      })
      return true
    },
    [repoById, projectName, preview, identities, confirm, t, announce, toast, reload, report],
  )

  const removeProject = useCallback(
    async (p: ProjectInfo): Promise<boolean> => {
      const members = [...(state?.repos ?? []), ...(state?.hiddenRepos ?? [])].filter((r) => r.projectId === p.id)
      let items: MovePreviewItem[]
      try {
        items = []
        for (const r of members) items.push(await preview(r, ''))
      } catch (e) {
        report(e)
        return false
      }
      const falling = items.filter(writesIdentity)
      const ok = await confirm({
        title: t('projects.remove.confirm.title', { name: p.name }),
        confirmLabel: t('projects.remove.action'),
        body: (
          <div className="proj-confirm">
            <p>{t('projects.remove.confirm.body', { name: p.name, n: members.length })}</p>
            {falling.length > 0 && (
              <>
                <p>{t('projects.remove.confirm.identity', { n: falling.length })}</p>
                <RepoList items={falling} tone="change" />
                <p className="muted">{t('projects.move.confirm.backup')}</p>
              </>
            )}
          </div>
        ),
      })
      if (!ok) return false
      try {
        for (const it of falling) {
          if (!it.preview) continue
          const res = await Identity.ApplyMove(it.repo.id, '', it.preview.planHash)
          const bad = (res.outcomes ?? []).find((o) => !o.ok)
          if (bad?.error) toast({ kind: 'danger', title: t('projects.move.failed', { repo: it.repo.localName }), body: toUiError(bad.error).message })
        }
        await Repos.RemoveProject(p.id)
      } catch (e) {
        report(e)
        await reload()
        return false
      }
      await reload()
      announce(t('projects.live.removed', { name: p.name }))
      toast({ kind: 'ok', title: t('projects.remove.done', { name: p.name, n: members.length }) })
      return true
    },
    [state, preview, confirm, identities, t, toast, report, reload, announce],
  )

  const unhideRepo = useCallback(
    async (r: Repo) => {
      try {
        await Repos.SetHidden(r.id, false)
        announce(t('projects.live.unhidden', { name: r.localName }))
      } catch (e) {
        report(e)
      }
      await reload()
    },
    [t, announce, report, reload],
  )

  const hideRepo = useCallback(
    async (r: Repo) => {
      try {
        await Repos.SetHidden(r.id, true)
      } catch (e) {
        report(e)
        return false
      }
      await reload()
      announce(t('projects.live.hidden', { name: r.localName }))
      toast({ kind: 'ok', title: t('projects.hide.done', { name: r.localName }), action: { label: t('common.undo'), onClick: () => void unhideRepo(r) } })
      return true
    },
    [t, announce, report, reload, toast, unhideRepo],
  )

  const applyProjectIdentity = useCallback(
    async (projectId: string, name: string, identity: Author) => {
      const failedToast = (body?: string) => toast({ kind: 'warn', title: t('projects.new.identityFailed', { name }), body })
      try {
        const req = { projectId, identity }
        const pv = await Identity.PreviewProject(req)
        const plans = pv.repos ?? []
        const change = plans.filter((pl) => pl.action === RepoAction.RepoActionSet || pl.action === RepoAction.RepoActionRemove)
        const keep = plans.filter((pl) => pl.action === RepoAction.RepoActionKeepOwn || pl.action === RepoAction.RepoActionKeepExternal)
        if (change.length > 0 || keep.length > 0) {
          const ok = await confirm({
            title: t('projects.identity.confirm.title', { name }),
            confirmLabel: t('projects.identity.confirm.action'),
            body: (
              <div className="proj-confirm">
                <p>{who(identity)}</p>
                {change.length > 0 && (
                  <>
                    <p>{t('projects.move.confirm.change', { n: change.length })}</p>
                    <PlanList plans={change} tone="change" />
                  </>
                )}
                {keep.length > 0 && (
                  <>
                    <p>{t('projects.move.confirm.keep', { n: keep.length })}</p>
                    <PlanList plans={keep} tone="keep" />
                  </>
                )}
                <p className="muted">{t('projects.move.confirm.backup')}</p>
              </div>
            ),
          })
          if (!ok) {
            announce(t('projects.live.cancelled'))
            return
          }
        }
        const res = await Identity.ApplyProject(req, pv.planHash)
        const bad = (res.outcomes ?? []).filter((o) => !o.ok)
        if (bad.length > 0 || !res.recordUpdated) failedToast(bad.map((o) => o.name).join(', ') || undefined)
      } catch (e) {
        failedToast(errorText(toUiError(e), t).title)
      }
    },
    [confirm, toast, t, announce],
  )

  return useMemo(() => ({ moveRepos, removeProject, applyProjectIdentity, hideRepo, unhideRepo }), [moveRepos, removeProject, applyProjectIdentity, hideRepo, unhideRepo])
}
