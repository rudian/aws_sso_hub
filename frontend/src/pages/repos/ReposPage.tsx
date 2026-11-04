import { ChevronDown, Download, FolderPlus, ListChecks, RefreshCw, Search, X } from 'lucide-react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Repos, System, copyText } from '../../api'
import { ErrorCode, OwnerStatus, RemoteKind, TestStatus } from '../../api/models'
import type { BindOutcome, Repo, TestResult } from '../../api/models'
import { toUiError } from '../../api/errors'
import { Button, Checkbox, CodeText, codeMark, EmptyState, ErrorBanner, IconButton, Menu, Progress, Segmented, Skeleton, Tooltip } from '../../components'
import { errorText, useI18n } from '../../i18n'
import { takeOpen } from '../../lib/initialUI'
import { useRefreshHandler, useSearchShortcut } from '../../lib/keyboard'
import { useRoute } from '../../lib/route'
import type { RepoFilter } from '../../lib/route'
import { formatRelative } from '../../lib/time'
import { useNow } from '../../lib/useNow'
import { GlobalBanner } from '../../shell/GlobalBanner'
import { useActions } from '../../shell/AppActions'
import { useAccounts } from '../../state/accounts'
import { useRepos } from '../../state/repos'
import { BindPopover } from './BindPopover'
import { BulkBindModal } from './BulkBindModal'
import { CloneModal } from './CloneModal'
import { HttpsModal } from './HttpsModal'
import { RepoRow } from './RepoRow'
import type { RowCallbacks } from './RepoRow'
import { useReportError, useUndoBackup } from './hooks'
import './repos.css'
import { bindAllPlan, bindableProfiles, canSelect, defaultFilter, filterRepos, needsBinding, resolvedCandidate } from './model'

interface BindTarget {
  repoId: string
  initial: string
  options: string[]
}

type BulkState = { mode: 'review'; ids: string[] } | { mode: 'suggestions' } | null

const FLASH_MS = 2400

/** Which row to scroll to once it is rendered. */
function rowEl(id: string): HTMLElement | null {
  return document.querySelector<HTMLElement>(`[data-repo-id="${CSS.escape(id)}"]`)
}

export function ReposPage() {
  const { t } = useI18n()
  const route = useRoute()
  const { state, loading, error, reload, scan, scanning } = useRepos()
  const { state: accounts } = useAccounts()
  const { toast, confirm, openLogin } = useActions()
  const report = useReportError()
  const now = useNow(30_000)

  const [filter, setFilter] = useState<RepoFilter | null>(route.filter ?? null)
  const [profileFilter, setProfileFilter] = useState<string | undefined>(route.profile)
  const [query, setQuery] = useState('')
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [testing, setTesting] = useState<Set<string>>(new Set())
  const [comparing, setComparing] = useState<Set<string>>(new Set())
  const [bindTarget, setBindTarget] = useState<BindTarget | null>(null)
  const [httpsId, setHttpsId] = useState<string | null>(null)
  const [bulk, setBulk] = useState<BulkState>(null)
  const [cloneOpen, setCloneOpen] = useState(false)
  const [adding, setAdding] = useState(false)
  const [bulkTesting, setBulkTesting] = useState(false)
  const [flashId, setFlashId] = useState<string | null>(null)
  const [scrollTo, setScrollTo] = useState<string | null>(null)

  const searchRef = useRef<HTMLInputElement>(null)
  const anchorRef = useRef<HTMLElement | null>(null)
  useSearchShortcut(searchRef)
  useRefreshHandler(() => void scan())

  const allRepos = useMemo(() => state?.repos ?? [], [state])
  const profiles = accounts?.profiles ?? []
  const cliReady = state?.awsCliReady ?? true
  const counts = state?.counts ?? { all: 0, toFix: 0, bound: 0 }
  const isScanning = !!scanning || !!state?.scanning

  // Pin the filter once: "To fix" while anything is left, then it stays until the user changes it.
  useEffect(() => {
    if (state && filter === null) setFilter(defaultFilter(state.counts))
  }, [state, filter])
  const activeFilter: RepoFilter = filter ?? 'all'

  const rows = useMemo(() => filterRepos(allRepos, activeFilter, profileFilter, query), [allRepos, activeFilter, profileFilter, query])
  const plan = useMemo(() => bindAllPlan(allRepos, profiles), [allRepos, profiles])

  // Drop selections of rows that disappeared.
  useEffect(() => {
    setSelected((cur) => {
      const ids = new Set(allRepos.map((r) => r.id))
      const next = new Set([...cur].filter((id) => ids.has(id)))
      return next.size === cur.size ? cur : next
    })
  }, [allRepos])

  // Look for owners automatically after the first load.
  const detected = useRef(false)
  const [detectSettled, setDetectSettled] = useState(false)
  useEffect(() => {
    if (!state || detected.current) return
    detected.current = true
    const todo = allRepos.some((r) => r.toFix && r.owner.status === OwnerStatus.OwnerUnknown)
    if (todo) {
      void Repos.DetectOwners([])
        .catch(() => undefined)
        .finally(() => setDetectSettled(true))
    } else {
      setDetectSettled(true)
    }
  }, [state, allRepos])

  // Scroll to a row and flash it (after "Add existing repo").
  useEffect(() => {
    if (!scrollTo) return
    const el = rowEl(scrollTo)
    if (!el) return
    el.scrollIntoView({ block: 'center' })
    setFlashId(scrollTo)
    setScrollTo(null)
    const timer = window.setTimeout(() => setFlashId(null), FLASH_MS)
    return () => window.clearTimeout(timer)
  }, [scrollTo, rows])

  const find = useCallback((id: string): Repo | undefined => allRepos.find((r) => r.id === id), [allRepos])
  const showRow = useCallback((id: string) => {
    setFilter('all')
    setProfileFilter(undefined)
    setQuery('')
    setScrollTo(id)
  }, [])

  const undoBackup = useUndoBackup(reload)

  // ---------------------------------------------------------------- actions
  const openBind = useCallback((id: string, initial: string, options: string[]) => {
    anchorRef.current = document.querySelector<HTMLElement>(`[data-repo-actions="${CSS.escape(id)}"]`)
    setBindTarget({ repoId: id, initial, options })
  }, [])

  const onBound = useCallback(
    (repo: Repo, out: BindOutcome, profile: string) => {
      setBindTarget(null)
      if (!out.ok) {
        const text = errorText(toUiError(out.error), t)
        toast({ kind: 'danger', title: text.title, body: text.body })
        return
      }
      toast({
        kind: 'ok',
        title: t('repos.bind.done', { repo: repo.name, name: profile }),
        action: out.backupId ? { label: t('common.undo'), onClick: () => void undoBackup(out.backupId as string) } : undefined,
      })
    },
    [t, toast, undoBackup],
  )

  const test = useCallback(
    async (repo: Repo) => {
      setTesting((s) => new Set(s).add(repo.id))
      try {
        await Repos.TestConnection(repo.id)
        await reload()
      } catch (e) {
        report(e)
      } finally {
        setTesting((s) => {
          const n = new Set(s)
          n.delete(repo.id)
          return n
        })
      }
    },
    [reload, report],
  )

  const unbind = useCallback(
    async (repo: Repo) => {
      const def = state?.globalDefault
      const ok = await confirm({
        title: t('repos.unbind.confirm.title', { repo: repo.name }),
        body: def ? t('repos.unbind.confirm.toDefault', { name: def.profile }) : t('repos.unbind.confirm.fail'),
        confirmLabel: t('repos.unbind.action'),
        danger: true,
      })
      if (!ok) return
      try {
        const out = await Repos.Unbind(repo.id)
        toast({
          kind: 'ok',
          title: t('repos.unbind.done', { repo: repo.name }),
          action: out.backupId ? { label: t('common.undo'), onClick: () => void undoBackup(out.backupId as string) } : undefined,
        })
      } catch (e) {
        report(e)
      }
    },
    [state?.globalDefault, confirm, t, toast, undoBackup, report],
  )

  const removeExtra = useCallback(
    async (repo: Repo) => {
      const ok = await confirm({ title: t('repos.removeExtra'), body: t('repos.removeExtra.note'), confirmLabel: t('repos.removeExtra') })
      if (!ok) return
      try {
        await Repos.RemoveExtra(repo.id)
        await reload()
      } catch (e) {
        report(e)
      }
    },
    [confirm, t, reload, report],
  )

  const compare = useCallback(
    async (repo: Repo) => {
      setComparing((s) => new Set(s).add(repo.id))
      try {
        await Repos.CompareCommits(repo.id)
        await reload()
      } catch (e) {
        report(e)
      } finally {
        setComparing((s) => {
          const n = new Set(s)
          n.delete(repo.id)
          return n
        })
      }
    },
    [reload, report],
  )

  const signInAndRecheck = useCallback(
    async (repo: Repo) => {
      let signedIn = false
      for (const portal of repo.owner.skippedPortals ?? []) {
        if (await openLogin(portal)) signedIn = true
      }
      if (!signedIn) return
      const ids = allRepos.filter((r) => r.owner.status === OwnerStatus.OwnerPartial).map((r) => r.id)
      try {
        await Repos.DetectOwners(ids.length > 0 ? ids : [repo.id])
      } catch (e) {
        report(e)
      }
    },
    [allRepos, openLogin, report],
  )

  const cb: RowCallbacks = useMemo(
    () => ({
      select: (id, checked) =>
        setSelected((cur) => {
          const n = new Set(cur)
          if (checked) n.add(id)
          else n.delete(id)
          return n
        }),
      bind: (repo, profile, options) => openBind(repo.id, profile, options),
      choose: (repo, initial) => openBind(repo.id, initial ?? '', bindableProfiles(profiles).map((p) => p.name)),
      test: (repo) => void test(repo),
      openFolder: (repo) => void Repos.OpenFolder(repo.id).catch(report),
      unbind: (repo) => void unbind(repo),
      copyRemote: (repo) =>
        void copyText(repo.remoteUrl).then(
          () => toast({ kind: 'ok', title: t('common.copied') }),
          (e) => report(e),
        ),
      openTerminal: (repo) => void System.OpenTerminal(repo.binding.profile ?? '', repo.path).catch(report),
      removeExtra: (repo) => void removeExtra(repo),
      toHttps: (repo) => setHttpsId(repo.id),
      compare: (repo) => void compare(repo),
      signIn: (portal) => void openLogin(portal),
      recheck: (repo) => void signInAndRecheck(repo),
    }),
    [openBind, profiles, test, unbind, report, toast, t, removeExtra, compare, openLogin, signInAndRecheck],
  )

  const addExisting = async () => {
    let path = ''
    try {
      path = await System.ChooseFolder(t('repos.add.choose'))
    } catch (e) {
      report(e)
      return
    }
    if (!path) return
    setAdding(true)
    try {
      const res = await Repos.AddExisting(path)
      const repo = res.state.repos?.find((r) => r.id === res.repoId)
      const name = repo?.name ?? path.split(/[\\/]/).filter(Boolean).pop() ?? path
      void reload()
      if (res.autoBound) {
        toast({
          kind: 'ok',
          title: t('repos.add.autoBound', { repo: name, profile: repo?.binding.profile ?? res.outcome?.repo?.binding.profile ?? '' }),
          action: {
            label: t('repos.add.undo'),
            onClick: () => void Repos.Unbind(res.repoId).then(() => reload(), report),
          },
        })
      } else if (!res.added) {
        toast({ kind: 'info', title: t('repos.add.already', { repo: name }) })
      } else {
        const ambiguous = repo?.owner.status === OwnerStatus.OwnerAmbiguous
        toast({ kind: 'ok', title: t('repos.add.added', { repo: name }), body: ambiguous ? t('repos.add.needsChoice') : undefined })
      }
      showRow(res.repoId)
    } catch (e) {
      const ue = toUiError(e)
      if (ue.code === ErrorCode.ErrRepoNotFound) toast({ kind: 'danger', title: t('repos.add.notRepo') })
      else if (ue.code === ErrorCode.ErrNotCodeCommit) toast({ kind: 'danger', title: t('repos.add.notCodeCommit') })
      else report(e)
    } finally {
      setAdding(false)
    }
  }

  const testSelected = async () => {
    const ids = [...selected].filter((id) => find(id)?.remoteKind === RemoteKind.RemoteHTTPS)
    if (ids.length === 0) return
    setBulkTesting(true)
    try {
      const results: TestResult[] = (await Repos.TestMany(ids)) ?? []
      await reload()
      toast({ kind: 'info', title: t('repos.bulk.testDone', { ok: results.filter((r) => r.status === TestStatus.TestOK).length, total: results.length }) })
    } catch (e) {
      report(e)
    } finally {
      setBulkTesting(false)
    }
  }

  // ---------------------------------------------------------------- ?open= (mock screenshots)
  const opened = useRef(false)
  useEffect(() => {
    // Wait until owner detection has finished, so the dialogs open with the suggestions filled in.
    if (!state || !detectSettled || opened.current) return
    opened.current = true
    const first = (name: string, pred?: (r: Repo) => boolean) => allRepos.find((r) => r.name === name && (!pred || pred(r)))
    const bindName = takeOpen('bind:')
    if (bindName !== null) {
      const r = first(bindName)
      if (r) {
        setFilter('all')
        showRow(r.id)
        window.setTimeout(() => openBind(r.id, resolvedCandidate(r)?.profile ?? r.binding.profile ?? '', resolvedCandidate(r)?.profiles ?? bindableProfiles(profiles).map((p) => p.name)), 150)
      }
      return
    }
    const ownerName = takeOpen('owner:')
    if (ownerName !== null) {
      // The menu belongs to an ambiguous row, which may only exist once detection has finished.
      setFilter('all')
      let tries = 0
      const timer = window.setInterval(() => {
        const el = document.querySelector<HTMLElement>(`[data-repo-id] [data-owner-trigger]`)
        const row = Array.from(document.querySelectorAll<HTMLElement>('[data-repo-id]')).find((e) => e.textContent?.includes(ownerName) && e.querySelector('[data-owner-trigger]'))
        const trigger = row?.querySelector<HTMLElement>('[data-owner-trigger]') ?? null
        if (trigger) {
          window.clearInterval(timer)
          trigger.scrollIntoView({ block: 'center' })
          trigger.click()
        } else if (++tries > 40 || !el) {
          if (tries > 40) window.clearInterval(timer)
        }
      }, 150)
      return
    }
    if (takeOpen('bulkBind') !== null) {
      const ids = allRepos.filter(needsBinding).map((r) => r.id)
      setSelected(new Set(ids))
      setFilter('todo')
      setBulk({ mode: 'review', ids })
      return
    }
    const unbindName = takeOpen('unbind:')
    if (unbindName !== null) {
      const r = first(unbindName)
      if (r) {
        setFilter('all')
        showRow(r.id)
        void unbind(r)
      }
      return
    }
    const httpsName = takeOpen('https:')
    if (httpsName !== null) {
      const r = first(httpsName)
      if (r) setHttpsId(r.id)
      return
    }
    if (takeOpen('clone') !== null) setCloneOpen(true)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [state, detectSettled])

  // ---------------------------------------------------------------- render
  const selectableRows = rows.filter((r) => canSelect(r))
  const allSelected = selectableRows.length > 0 && selectableRows.every((r) => selected.has(r.id))
  const someSelected = selectableRows.some((r) => selected.has(r.id))
  const selectedRepos = allRepos.filter((r) => selected.has(r.id))
  const bindableSelected = selectedRepos.filter(needsBinding)
  const root = state?.rootsDisplay?.[0] ?? ''
  const cliTip = cliReady ? undefined : t('banner.cliMissing')
  const bindTargetRepo = bindTarget ? find(bindTarget.repoId) : undefined
  const httpsRepo = httpsId ? find(httpsId) : undefined

  const header = (
    <header className="page__header">
      <div>
        <h1 className="page__title">{t('repos.title')}</h1>
        {state?.scannedAt && (
          <p className="page__subtitle">
            <Tooltip content={(state.rootsDisplay ?? []).length > 1 ? (state.rootsDisplay ?? []).join('\n') : undefined}>
              <span>
                <CodeText text={t('repos.subtitle', { folder: codeMark(root), n: counts.all, when: formatRelative(state.scannedAt, now, t) })} />
              </span>
            </Tooltip>
          </p>
        )}
      </div>
      <div className="page__actions">
        <Button variant="secondary" icon={RefreshCw} loading={isScanning} onClick={() => void scan()}>
          {t('repos.rescan')}
        </Button>
        <Button variant="secondary" icon={FolderPlus} loading={adding} onClick={() => void addExisting()}>
          {t('repos.add.button')}
        </Button>
        <Tooltip content={cliTip}>
          <Button variant="primary" icon={Download} disabled={!cliReady} onClick={() => setCloneOpen(true)}>
            {t('repos.clone')}
          </Button>
        </Tooltip>
      </div>
    </header>
  )

  if (loading && !state) {
    return (
      <div className="page">
        {header}
        <GlobalBanner scope="repos" />
        <div className="stack" aria-busy="true">
          {[0, 1, 2, 3].map((i) => (
            <Skeleton key={i} variant="block" height={56} />
          ))}
        </div>
      </div>
    )
  }

  const accountMenuItems = [
    { id: 'all', label: t('repos.filter.account.all'), onSelect: () => setProfileFilter(undefined) },
    ...profiles.map((p) => ({ id: p.name, label: p.name, onSelect: () => setProfileFilter(p.name) })),
  ]

  const emptyState = (() => {
    if (allRepos.length === 0) {
      return (
        <EmptyState
          title={t('repos.empty.title')}
          text={t('repos.empty.body')}
          actions={
            <>
              <Button variant="secondary" icon={FolderPlus} onClick={() => void addExisting()}>
                {t('repos.add.button')}
              </Button>
              <Button variant="primary" icon={Download} disabled={!cliReady} onClick={() => setCloneOpen(true)}>
                {t('repos.clone')}
              </Button>
            </>
          }
        />
      )
    }
    if (activeFilter === 'todo' && counts.toFix === 0 && !profileFilter && !query.trim()) {
      return <EmptyState title={t('repos.empty.allDone')} text={t('repos.empty.allDone.body')} />
    }
    return <EmptyState title={t('repos.empty.filtered')} />
  })()

  return (
    <div className="page">
      {header}
      <GlobalBanner scope="repos" />
      <div className="page__body" style={{ gap: 'var(--sp-4)' }}>
        {error && <ErrorBanner error={error} onRetry={() => void reload()} />}
        <div className="toolbar">
          <Segmented<RepoFilter>
            ariaLabel={t('repos.filter.aria')}
            value={activeFilter}
            onChange={setFilter}
            items={[
              { value: 'all', label: t('repos.filter.all'), count: counts.all },
              { value: 'todo', label: t('repos.filter.todo'), count: counts.toFix, warn: counts.toFix > 0 },
              { value: 'bound', label: t('repos.filter.bound'), count: counts.bound },
            ]}
          />
          <Menu
            trigger={
              <Button variant="secondary" size="sm">
                {profileFilter ? `${t('repos.filter.account')}: ${profileFilter}` : t('repos.filter.account.all')}
                <ChevronDown className="btn__icon" aria-hidden="true" />
              </Button>
            }
            items={accountMenuItems}
          />
          {profileFilter && <IconButton size="sm" label={t('repos.filter.account.clear')} icon={X} onClick={() => setProfileFilter(undefined)} />}
          {plan.items.length > 0 && (
            <Tooltip content={cliTip}>
              <Button size="sm" variant="secondary" icon={ListChecks} disabled={!cliReady} onClick={() => setBulk({ mode: 'suggestions' })}>
                {t('repos.bindAll', { n: plan.items.length })}
              </Button>
            </Tooltip>
          )}
          <label className="search">
            <Search aria-hidden="true" />
            <input ref={searchRef} className="input" placeholder={t('repos.search')} aria-label={t('repos.search.aria')} value={query} onChange={(e) => setQuery(e.target.value)} />
          </label>
        </div>

        {isScanning && (
          <div className="stack" style={{ '--gap': 'var(--sp-1)' } as React.CSSProperties}>
            <Progress indeterminate label={t('repos.scan.progress', { folder: scanning?.root || root, n: scanning?.found ?? 0 })} />
            <div className="hint">{t('repos.scan.progress', { folder: scanning?.root || root, n: scanning?.found ?? 0 })}</div>
          </div>
        )}

        {rows.length === 0 ? (
          emptyState
        ) : (
          <div className="table repo-table" role="table" aria-label={t('repos.table.aria')}>
            <div className="table__head" role="row">
              <Checkbox
                aria-label={t('repos.selectAll')}
                checked={allSelected}
                ref={(el) => {
                  if (el) el.indeterminate = !allSelected && someSelected
                }}
                onChange={(e) => setSelected(e.target.checked ? new Set(selectableRows.map((r) => r.id)) : new Set())}
              />
              <span>{t('repos.col.repo')}</span>
              <span>{t('repos.col.account')}</span>
              <span>{t('repos.col.status')}</span>
              <span style={{ textAlign: 'right' }}>{t('repos.col.actions')}</span>
            </div>
            {rows.map((r) => (
              <RepoRow
                key={r.id}
                repo={r}
                selected={selected.has(r.id)}
                profiles={profiles}
                portals={accounts?.portals ?? []}
                now={now}
                cliReady={cliReady}
                testing={testing.has(r.id)}
                comparing={comparing.has(r.id)}
                flash={flashId === r.id}
                cb={cb}
              />
            ))}
          </div>
        )}

        {selected.size > 0 && (
          <div className="bulkbar" role="region" aria-label={t('repos.bulk.selected', { n: selected.size })}>
            <span>{t('repos.bulk.selected', { n: selected.size })}</span>
            <Button size="sm" loading={bulkTesting} disabled={!cliReady} onClick={() => void testSelected()}>
              {t('repos.action.test')}
            </Button>
            <Button size="sm" variant="primary" disabled={!cliReady || bindableSelected.length === 0} onClick={() => setBulk({ mode: 'review', ids: bindableSelected.map((r) => r.id) })}>
              {t('repos.bulk.bind')}
            </Button>
            <IconButton size="sm" label={t('repos.bulk.clear')} icon={X} style={{ color: 'var(--on-ink)' }} onClick={() => setSelected(new Set())} />
          </div>
        )}
      </div>

      {bindTarget && bindTargetRepo && (
        <BindPopover
          key={bindTarget.repoId}
          repo={bindTargetRepo}
          anchorRef={anchorRef}
          options={bindTarget.options}
          initial={bindTarget.initial}
          onClose={() => setBindTarget(null)}
          onBound={(out, profile) => onBound(bindTargetRepo, out, profile)}
        />
      )}
      {httpsRepo && (
        <HttpsModal
          repo={httpsRepo}
          onClose={() => setHttpsId(null)}
          onDone={(out, profile) => {
            setHttpsId(null)
            onBound(httpsRepo, out, profile)
          }}
        />
      )}
      {bulk?.mode === 'review' && (
        <BulkBindModal
          mode="review"
          repos={bulk.ids.map(find).filter((r): r is Repo => !!r)}
          onClose={() => {
            setBulk(null)
            setSelected(new Set())
          }}
        />
      )}
      {bulk?.mode === 'suggestions' && <BulkBindModal mode="suggestions" repos={plan.items.map((i) => find(i.repoId)).filter((r): r is Repo => !!r)} items={plan.items} skipped={plan.ambiguous} onClose={() => setBulk(null)} />}
      {cloneOpen && <CloneModal initialProfile={route.profile} defaultParent={state?.roots?.[0] ?? ''} cliReady={cliReady} onClose={() => setCloneOpen(false)} />}
    </div>
  )
}
