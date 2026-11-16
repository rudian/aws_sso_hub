// The Projects page (design 5.2): repositories grouped into projects, with filters, search, suggestions, create /
// rename / remove, drag and drop with a keyboard path, a bulk bar, hide, and the "newly found" review.
import { ChevronDown, Download, Filter, EyeOff, FolderPlus, Info, ListChecks, Plus, RefreshCw, Search, Sparkles, X } from 'lucide-react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { KeyboardEvent, PointerEvent } from 'react'
import { Repos, System, copyText } from '../../api'
import { ErrorCode, OwnerStatus, RemoteKind, RepoProvider, TestStatus, WarningCode } from '../../api/models'
import type { BindOutcome, Author, ProjectInfo, Repo, TestResult } from '../../api/models'
import { toUiError } from '../../api/errors'
import { Banner, Button, CodeText, codeMark, EmptyState, ErrorBanner, IconButton, Menu, Progress, Segmented, Skeleton, Tooltip } from '../../components'
import { errorText, useI18n } from '../../i18n'
import { takeOpen } from '../../lib/initialUI'
import { useRefreshHandler, useSearchShortcut } from '../../lib/keyboard'
import { useFlip } from '../../lib/motion'
import { setRoute, useRoute } from '../../lib/route'
import { formatRelative } from '../../lib/time'
import { useNow } from '../../lib/useNow'
import { GlobalBanner } from '../../shell/GlobalBanner'
import { useActions } from '../../shell/AppActions'
import { useAccounts } from '../../state/accounts'
import { useProjectIdentities } from '../../state/projects'
import { useRepos } from '../../state/repos'
import { BindPopover } from '../repos/BindPopover'
import { BulkBindModal } from '../repos/BulkBindModal'
import { HttpsModal } from '../repos/HttpsModal'
import { useReportError, useUndoBackup } from '../repos/hooks'
import { bindAllPlan, bindableProfiles, needsBinding, resolvedCandidate } from '../repos/model'
import { CloneModal } from './CloneModal'
import { DragGhost, useRowDrag } from './drag'
import { Emph } from './Emph'
import { FoundChooser } from './FoundChooser'
import { MoveMenu } from './MoveMenu'
import { ProjectModal } from './NewProjectModal'
import { ProjectRow } from './ProjectRow'
import type { ProjectRowCallbacks } from './ProjectRow'
import { ProjectSection } from './ProjectSection'
import { HIDDEN_SECTION, UNGROUPED, activeSuggestion, changesCount, displayOrder, groupRepos, isCodeCommit, moveTargets, movable, providerKind } from './model'
import type { ProjectFilter, SourceFilter } from './model'
import { useProjectOps } from './ops'
import '../repos/repos.css'
import './projects.css'

interface BindTarget {
  repoId: string
  initial: string
  options: string[]
}
type BulkState = { mode: 'review'; ids: string[] } | { mode: 'suggestions' } | null
interface ModalState {
  mode: 'create' | 'add'
  project?: ProjectInfo
  initialName?: string
  initialIds?: string[]
  suggestionKey?: string
}
interface MoveFor {
  ids: string[]
  name: string
}

const FLASH_MS = 2400
const SOURCES: Array<{ value: SourceFilter; key: 'projects.source.all' | 'projects.source.aws' | 'projects.source.github' | 'projects.source.bitbucket' | 'projects.source.other' }> = [
  { value: 'all', key: 'projects.source.all' },
  { value: RepoProvider.ProviderCodeCommit, key: 'projects.source.aws' },
  { value: RepoProvider.ProviderGitHub, key: 'projects.source.github' },
  { value: RepoProvider.ProviderBitbucket, key: 'projects.source.bitbucket' },
  { value: RepoProvider.ProviderOther, key: 'projects.source.other' },
]

const rowEl = (id: string): HTMLElement | null => document.querySelector<HTMLElement>(`[data-repo-id="${CSS.escape(id)}"]`)

function enabledControls(row: HTMLElement): HTMLElement[] {
  return Array.from(row.querySelectorAll<HTMLElement>('[data-grid-control]')).filter((el) => !(el as HTMLButtonElement).disabled && !el.closest('[inert]'))
}

export function ProjectsPage() {
  const { t } = useI18n()
  const route = useRoute()
  const { state, loading, error, reload, scan, scanning } = useRepos()
  const { state: accounts } = useAccounts()
  const { toast, confirm, openLogin } = useActions()
  const report = useReportError()
  const now = useNow(30_000)
  const identities = useProjectIdentities()

  const [filter, setFilter] = useState<ProjectFilter>(route.filter === 'todo' ? 'todo' : 'all')
  const [source, setSource] = useState<SourceFilter>('all')
  const [showHidden, setShowHidden] = useState(false)
  const [query, setQuery] = useState('')
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [testing, setTesting] = useState<Set<string>>(new Set())
  const [comparing, setComparing] = useState<Set<string>>(new Set())
  const [asking, setAsking] = useState<string | null>(null)
  const [bindTarget, setBindTarget] = useState<BindTarget | null>(null)
  const [httpsId, setHttpsId] = useState<string | null>(null)
  const [bulk, setBulk] = useState<BulkState>(null)
  const [cloneOpen, setCloneOpen] = useState(false)
  const [adding, setAdding] = useState(false)
  const [bulkTesting, setBulkTesting] = useState(false)
  const [flashId, setFlashId] = useState<string | null>(null)
  const [scrollTo, setScrollTo] = useState<string | null>(null)
  const [modal, setModal] = useState<ModalState | null>(null)
  const [foundOpen, setFoundOpen] = useState(false)
  const [moveFor, setMoveFor] = useState<MoveFor | null>(null)
  const [renaming, setRenaming] = useState<string | null>(null)
  const [overSection, setOverSection] = useState<string | null>(null)
  const [dragIds, setDragIds] = useState<readonly string[]>([])
  const [localCollapsed, setLocalCollapsed] = useState<Map<string, boolean>>(new Map())
  const [active, setActive] = useState<Record<string, string>>({})
  const [live, setLive] = useState({ n: 0, text: '' })
  const [mockDrag, setMockDrag] = useState<{ label: string; x: number; y: number } | null>(null)

  const searchRef = useRef<HTMLInputElement>(null)
  const anchorRef = useRef<HTMLElement | null>(null)
  const moveAnchor = useRef<HTMLElement | null>(null)
  const listRef = useRef<HTMLDivElement>(null)
  const pendingFocus = useRef<{ repo?: string; section?: string } | null>(null)
  useSearchShortcut(searchRef)
  useRefreshHandler(() => void scan())

  const announce = useCallback((text: string) => setLive((l) => ({ n: l.n + 1, text })), [])

  const allRepos = useMemo(() => state?.repos ?? [], [state])
  const profiles = accounts?.profiles ?? []
  const cliReady = state?.awsCliReady ?? true
  const counts = state?.counts ?? { all: 0, toFix: 0, bound: 0, found: 0, hidden: 0 }
  const isScanning = !!scanning || !!state?.scanning
  const projects = useMemo(() => state?.projects ?? [], [state])
  const ops = useProjectOps({ state, identities: identities.byProject, reload, announce })

  // The hidden list only exists while something is hidden.
  useEffect(() => {
    if (showHidden && counts.hidden === 0) setShowHidden(false)
  }, [showHidden, counts.hidden])

  const sections = useMemo(() => groupRepos(state, { filter, source, query, showHidden }, t('projects.ungrouped')), [state, filter, source, query, showHidden, t])
  const constrained = filter !== 'all' || source !== 'all' || query.trim() !== ''
  const isCollapsed = useCallback(
    (id: string) => {
      if (id === HIDDEN_SECTION) return false
      const local = localCollapsed.get(id)
      if (local !== undefined) return local
      return projects.find((p) => p.id === id)?.collapsed ?? false
    },
    [localCollapsed, projects],
  )
  const collapsedIds = useMemo(() => new Set(sections.filter((s) => isCollapsed(s.id)).map((s) => s.id)), [sections, isCollapsed])
  const order = useMemo(() => displayOrder(sections, collapsedIds), [sections, collapsedIds])
  const orderRef = useRef(order)
  orderRef.current = order
  useFlip(listRef, order)
  const plan = useMemo(() => bindAllPlan(allRepos, profiles), [allRepos, profiles])
  const suggestion = useMemo(() => activeSuggestion(state), [state])
  const find = useCallback((id: string): Repo | undefined => allRepos.find((r) => r.id === id) ?? state?.hiddenRepos?.find((r) => r.id === id), [allRepos, state])

  // Drop selections of rows that disappeared.
  useEffect(() => {
    setSelected((cur) => {
      const ids = new Set(allRepos.map((r) => r.id))
      const next = new Set([...cur].filter((id) => ids.has(id)))
      return next.size === cur.size ? cur : next
    })
  }, [allRepos])

  // Read the git status of the rows once the list has drawn (the file watcher keeps it fresh after that).
  const statusAsked = useRef(false)
  useEffect(() => {
    if (!state || statusAsked.current || allRepos.length === 0) return
    statusAsked.current = true
    if (allRepos.some((r) => !r.status)) void Repos.RefreshStatus(null).catch(() => undefined)
  }, [state, allRepos])

  // Look for owners automatically after the first load.
  const detected = useRef(false)
  const [detectSettled, setDetectSettled] = useState(false)
  useEffect(() => {
    if (!state || detected.current) return
    detected.current = true
    const todo = allRepos.some((r) => isCodeCommit(r) && r.toFix && r.owner.status === OwnerStatus.OwnerUnknown)
    if (todo) {
      void Repos.DetectOwners([])
        .catch(() => undefined)
        .finally(() => setDetectSettled(true))
    } else setDetectSettled(true)
  }, [state, allRepos])

  // Scroll to a row and flash it (after "Add folder").
  useEffect(() => {
    if (!scrollTo) return
    const el = rowEl(scrollTo)
    if (!el) return
    el.scrollIntoView({ block: 'center' })
    setFlashId(scrollTo)
    setScrollTo(null)
    const timer = window.setTimeout(() => setFlashId(null), FLASH_MS)
    return () => window.clearTimeout(timer)
  }, [scrollTo, sections])

  // Focus after an action: the row or the section header the focus should land on once the list has changed.
  useEffect(() => {
    const p = pendingFocus.current
    if (!p) return
    const el = p.repo ? rowEl(p.repo) : null
    const target = el ?? (p.section !== undefined ? document.querySelector<HTMLElement>(`[data-section="${CSS.escape(p.section)}"] .portal__toggle`) : null)
    if (target) {
      pendingFocus.current = null
      target.focus()
    }
  }, [sections, order, live])

  // One tab stop per table (the active row); the controls inside a row are reachable from it with the arrow keys.
  useEffect(() => {
    const root = listRef.current
    if (!root) return
    root.querySelectorAll<HTMLElement>('[role="row"][data-repo-id]').forEach((row) => {
      const on = row.tabIndex === 0
      row.querySelectorAll<HTMLElement>('[data-grid-control]').forEach((c) => {
        c.tabIndex = on ? 0 : -1
      })
    })
  })

  const showRow = useCallback((id: string) => {
    setFilter('all')
    setSource('all')
    setShowHidden(false)
    setQuery('')
    setScrollTo(id)
  }, [])
  const undoBackup = useUndoBackup(reload)

  // ---------------------------------------------------------------- moving
  const openMove = useCallback(
    (ids: string[], anchor: HTMLElement | null) => {
      if (ids.length === 0) return
      moveAnchor.current = anchor
      const first = find(ids[0])
      setMoveFor({ ids, name: first?.localName ?? '' })
      announce(ids.length === 1 ? t('projects.live.picked', { name: first?.localName ?? '' }) : t('projects.live.pickedMany', { n: ids.length }))
    },
    [find, announce, t],
  )
  const doMove = useCallback(
    async (ids: readonly string[], to: string, name?: string, keyboard = false) => {
      if (keyboard && ids[0]) pendingFocus.current = { repo: ids[0] }
      const ok = await ops.moveRepos(ids, to, name)
      if (!ok) pendingFocus.current = null
      else if (keyboard) setSelected(new Set())
      return ok
    },
    [ops],
  )

  const createProject = useCallback(
    async (name: string, ids: string[], identity: Author | null): Promise<boolean> => {
      const before = new Set(projects.map((p) => p.id))
      let id: string | undefined
      try {
        const st = await Repos.CreateProject(name)
        id = st.projects?.find((p) => !before.has(p.id))?.id
      } catch (e) {
        const ue = toUiError(e)
        if (ue.code === ErrorCode.ErrInvalidInput) toast({ kind: 'danger', title: t('projects.new.invalid') })
        else report(e)
        return false
      }
      if (!id) {
        await reload()
        return true
      }
      if (ids.length > 0) await ops.moveRepos(ids, id, name)
      if (identity) {
        await ops.applyProjectIdentity(id, name, identity)
      }
      await reload()
      announce(t('projects.live.created', { name }))
      toast({ kind: 'ok', title: t('projects.new.done', { name, n: ids.length }) })
      return true
    },
    [projects, ops, toast, t, report, reload, announce],
  )

  const addToProject = useCallback((p: ProjectInfo, ids: string[]) => ops.moveRepos(ids, p.id, p.name), [ops])

  // ---------------------------------------------------------------- drag
  const { drag, start: startDrag } = useRowDrag({
    onStart: (ids) => {
      setDragIds(ids)
      const first = find(ids[0])
      announce(ids.length === 1 ? t('projects.live.picked', { name: first?.localName ?? '' }) : t('projects.live.pickedMany', { n: ids.length }))
    },
    onOver: (target) => {
      setOverSection(target)
      if (target !== null) announce(t('projects.live.over', { project: target ? (projects.find((p) => p.id === target)?.name ?? '') : t('projects.ungrouped') }))
    },
    onDrop: (ids, target) => {
      setOverSection(null)
      setDragIds([])
      void doMove(ids, target)
    },
    onCancel: () => {
      setOverSection(null)
      setDragIds([])
      announce(t('projects.live.cancelled'))
    },
    valid: (ids, target) => target !== HIDDEN_SECTION && ids.some((id) => (find(id)?.projectId ?? '') !== target),
  })
  const sectionName = (id: string | null) => (id === null ? null : id === UNGROUPED ? t('projects.ungrouped') : (projects.find((p) => p.id === id)?.name ?? null))

  // ---------------------------------------------------------------- repository actions
  const openBind = useCallback((id: string, initial: string, options: string[], anchor?: HTMLElement) => {
    anchorRef.current = anchor ?? rowEl(id)?.querySelector<HTMLElement>('.row__actions') ?? rowEl(id)
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
        title: t('repos.bind.done', { repo: repo.localName, name: profile }),
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
        title: t('repos.unbind.confirm.title', { repo: repo.localName }),
        body: def ? t('repos.unbind.confirm.toDefault', { name: def.profile }) : t('repos.unbind.confirm.fail'),
        confirmLabel: t('repos.unbind.action'),
        danger: true,
      })
      if (!ok) return
      try {
        const out = await Repos.Unbind(repo.id)
        toast({
          kind: 'ok',
          title: t('repos.unbind.done', { repo: repo.localName }),
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
      for (const portal of repo.owner.skippedPortals ?? []) if (await openLogin(portal)) signedIn = true
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

  /** Opening a repository starts its background fetch. The workspace itself arrives with milestone 2; until then the folder opens. */
  const openRepo = useCallback(
    (repo: Repo) => {
      void Repos.MarkOpened(repo.id).catch(report)
      void Repos.OpenFolder(repo.id).catch(report)
    },
    [report],
  )

  const hideMany = useCallback(
    async (ids: string[]) => {
      const repos = ids.map(find).filter((r): r is Repo => !!r)
      if (repos.length === 1) {
        const i = order.indexOf(repos[0].id)
        const next = order[i + 1] ?? order[i - 1]
        pendingFocus.current = next ? { repo: next } : null
        const ok = await ops.hideRepo(repos[0])
        if (!ok) pendingFocus.current = null
        return
      }
      let done = 0
      for (const r of repos) {
        try {
          await Repos.SetHidden(r.id, true)
          done++
        } catch (e) {
          report(e)
        }
      }
      await reload()
      setSelected(new Set())
      if (done > 0) {
        announce(t('projects.live.hiddenMany', { n: done }))
        toast({
          kind: 'ok',
          title: t('projects.hide.doneMany', { n: done }),
          action: {
            label: t('common.undo'),
            onClick: () =>
              void (async () => {
                for (const r of repos) await Repos.SetHidden(r.id, false).catch(() => undefined)
                await reload()
              })(),
          },
        })
      }
    },
    [find, order, ops, report, reload, announce, t, toast],
  )

  const answerMoved = useCallback(
    async (repo: Repo, yes: boolean) => {
      setAsking(repo.id)
      try {
        if (yes) await Repos.ReattachMoved(repo.id)
        else await Repos.DeclineMoved(repo.id)
        await reload()
        toast({ kind: 'ok', title: yes ? t('projects.moved.reattached', { name: repo.movedFrom?.name ?? repo.localName }) : t('projects.moved.declined', { name: repo.localName }) })
      } catch (e) {
        const ue = toUiError(e)
        if (ue.code === ErrorCode.ErrMovedHasData || ue.code === ErrorCode.ErrMovedGone) {
          const text = errorText(ue, t)
          toast({ kind: 'warn', title: text.title, body: ue.code === ErrorCode.ErrMovedHasData ? t('projects.moved.hasData') : t('projects.moved.gone') })
          await reload()
        } else report(e)
      } finally {
        setAsking(null)
      }
    },
    [reload, toast, t, report],
  )

  const addFolder = useCallback(async () => {
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
      const res = await Repos.AddFolder(path)
      const repo = res.state.repos?.find((r) => r.id === res.repoId)
      const name = repo?.localName ?? path.split(/[\\/]/).filter(Boolean).pop() ?? path
      void reload()
      if (res.autoBound) {
        toast({
          kind: 'ok',
          title: t('repos.add.autoBound', { repo: name, profile: repo?.binding.profile ?? res.outcome?.repo?.binding.profile ?? '' }),
          action: { label: t('repos.add.undo'), onClick: () => void Repos.Unbind(res.repoId).then(() => reload(), report) },
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
      else report(e)
    } finally {
      setAdding(false)
    }
  }, [t, report, reload, toast, showRow])

  const testSelected = async () => {
    const ids = [...selected].filter((id) => {
      const r = find(id)
      return r && isCodeCommit(r) && r.remoteKind === RemoteKind.RemoteHTTPS
    })
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

  // ---------------------------------------------------------------- projects
  const toggleCollapsed = useCallback(
    (id: string) => {
      const next = !isCollapsed(id)
      setLocalCollapsed((m) => new Map(m).set(id, next))
      if (id !== UNGROUPED) void Repos.SetProjectCollapsed(id, next).catch(report)
    },
    [isCollapsed, report],
  )

  const renameProject = useCallback(
    async (p: ProjectInfo, name: string) => {
      setRenaming(null)
      try {
        await Repos.RenameProject(p.id, name)
        await reload()
        announce(t('projects.live.renamed', { name }))
      } catch (e) {
        const ue = toUiError(e)
        if (ue.code === ErrorCode.ErrInvalidInput) toast({ kind: 'danger', title: t('projects.new.invalid') })
        else report(e)
      }
      pendingFocus.current = { section: p.id }
      setLive((l) => ({ ...l }))
    },
    [reload, announce, t, toast, report],
  )

  const createFromSuggestion = useCallback(async () => {
    if (!suggestion) return
    const ok = await createProject(suggestion.label, suggestion.repoIds ?? [], null)
    if (!ok) return
  }, [suggestion, createProject])

  const dismissSuggestion = useCallback(async () => {
    if (!suggestion) return
    try {
      await Repos.DismissSuggestion(suggestion.key)
    } catch (e) {
      report(e)
    }
  }, [suggestion, report])

  const chooseFound = useCallback(
    async (show: string[], hide: string[]): Promise<boolean> => {
      try {
        await Repos.ResolveFound(show, hide)
        await reload()
        announce(t('projects.found.resolved', { shown: show.length, hidden: hide.length }))
        toast({ kind: 'ok', title: t('projects.found.resolved', { shown: show.length, hidden: hide.length }) })
        return true
      } catch (e) {
        report(e)
        return false
      }
    },
    [reload, announce, t, toast, report],
  )

  const hideAllFound = useCallback(async () => {
    const ids = (state?.found ?? []).map((r) => r.id)
    if (ids.length === 0) return
    await chooseFound([], ids)
  }, [state, chooseFound])

  // ---------------------------------------------------------------- the grid
  const rowKey = useCallback(
    (repo: Repo, e: KeyboardEvent<HTMLElement>) => {
      if (e.defaultPrevented) return
      const row = e.currentTarget
      const target = e.target as HTMLElement
      if (!row.contains(target)) return // a menu or popover rendered into the body
      if (e.metaKey || e.ctrlKey || e.altKey) return
      const onRow = target === row
      const ord = orderRef.current
      const i = ord.indexOf(repo.id)
      const controls = enabledControls(row)
      const ci = controls.indexOf(target)
      const tableRows = Array.from((row.closest('[role="grid"]') ?? row.parentElement ?? document).querySelectorAll<HTMLElement>('[role="row"][data-repo-id]'))
      switch (e.key) {
        case 'ArrowDown':
        case 'ArrowUp': {
          if (!onRow && ci < 0) return
          const nextId = ord[i + (e.key === 'ArrowDown' ? 1 : -1)]
          if (!nextId) return
          e.preventDefault()
          const nextRow = rowEl(nextId)
          if (!nextRow) return
          if (onRow || ci < 0) nextRow.focus()
          else (enabledControls(nextRow)[ci] ?? enabledControls(nextRow).at(-1) ?? nextRow).focus()
          return
        }
        case 'Home':
        case 'End':
          if (!onRow) return
          e.preventDefault()
          ;(e.key === 'Home' ? tableRows[0] : tableRows[tableRows.length - 1])?.focus()
          return
        case 'ArrowRight':
          if (onRow) controls[0]?.focus()
          else if (ci >= 0) controls[ci + 1]?.focus()
          else return
          e.preventDefault()
          return
        case 'ArrowLeft':
          if (onRow) return
          if (ci < 0) return
          e.preventDefault()
          if (ci === 0) row.focus()
          else controls[ci - 1]?.focus()
          return
        case 'Escape':
          if (onRow || ci < 0) return
          e.preventDefault()
          row.focus()
          return
        case 'Enter':
          if (!onRow || repo.hidden) return
          e.preventDefault()
          openRepo(repo)
          return
        case ' ':
          if (!onRow || !movable(repo)) return
          e.preventDefault()
          setSelected((cur) => {
            const n = new Set(cur)
            if (n.has(repo.id)) n.delete(repo.id)
            else n.add(repo.id)
            return n
          })
          return
        case 'm':
        case 'M':
          if (!onRow || !movable(repo)) return
          e.preventDefault()
          openMove(moveTargets(repo.id, selected, ord), row)
          return
      }
    },
    [openRepo, openMove, selected],
  )

  const cb: ProjectRowCallbacks = useMemo(
    () => ({
      select: (id, checked) =>
        setSelected((cur) => {
          const n = new Set(cur)
          if (checked) n.add(id)
          else n.delete(id)
          return n
        }),
      open: openRepo,
      bind: (repo, profile, options, anchor) => openBind(repo.id, profile, options, anchor),
      choose: (repo, initial, anchor) => openBind(repo.id, initial ?? '', bindableProfiles(profiles).map((p) => p.name), anchor),
      test: (repo) => void test(repo),
      revealFolder: (repo) => void Repos.OpenFolder(repo.id).catch(report),
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
      move: (repo) => openMove(moveTargets(repo.id, selected, orderRef.current), rowEl(repo.id)),
      hide: (repo) => void hideMany([repo.id]),
      unhide: (repo) => void ops.unhideRepo(repo),
      reattach: (repo) => void answerMoved(repo, true),
      decline: (repo) => void answerMoved(repo, false),
      dragStart: (repo: Repo, e: PointerEvent<HTMLElement>) => {
        const ids = moveTargets(repo.id, selected, orderRef.current)
        startDrag(ids, repo.localName, e, providerKind(repo))
      },
      rowKey,
      focusRow: (repo, sectionId) => setActive((a) => (a[sectionId] === repo.id ? a : { ...a, [sectionId]: repo.id })),
    }),
    [openRepo, openBind, profiles, test, report, unbind, toast, t, removeExtra, compare, openLogin, signInAndRecheck, openMove, selected, hideMany, ops, answerMoved, startDrag, rowKey],
  )

  // ---------------------------------------------------------------- ?open= (mock screenshots)
  const opened = useRef(false)
  useEffect(() => {
    if (!state || !detectSettled || opened.current) return
    opened.current = true
    const first = (name: string) => allRepos.find((r) => r.localName === name || r.name === name)
    const bindName = takeOpen('bind:')
    if (bindName !== null) {
      const r = first(bindName)
      if (r) {
        showRow(r.id)
        window.setTimeout(() => openBind(r.id, resolvedCandidate(r)?.profile ?? r.binding.profile ?? '', resolvedCandidate(r)?.profiles ?? bindableProfiles(profiles).map((p) => p.name)), 150)
      }
      return
    }
    if (takeOpen('bulkBind') !== null) {
      const ids = allRepos.filter(needsBinding).map((r) => r.id)
      setSelected(new Set(ids))
      setBulk({ mode: 'review', ids })
      return
    }
    const unbindName = takeOpen('unbind:')
    if (unbindName !== null) {
      const r = first(unbindName)
      if (r) {
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
    if (takeOpen('clone') !== null) {
      setCloneOpen(true)
      return
    }
    if (takeOpen('new') !== null) {
      setModal({ mode: 'create' })
      return
    }
    if (takeOpen('found') !== null) {
      void scan().then(() => setFoundOpen(true))
      return
    }
    if (takeOpen('scan') !== null) {
      void scan()
      return
    }
    // ?open=drag:<repository>:<project>: the lifted row over a project, as in the drag screenshot.
    const dragSpec = takeOpen('drag:')
    if (dragSpec !== null) {
      const [repoName, projectName] = dragSpec.split(':')
      const r = first(repoName)
      const p = projects.find((x) => x.name === projectName)
      if (r && p) {
        setDragIds([r.id])
        setOverSection(p.id)
        window.setTimeout(() => {
          const head = document.querySelector<HTMLElement>(`[data-drop="${CSS.escape(p.id)}"] .portal__title`)?.getBoundingClientRect()
          const at = head ?? new DOMRect(300, 500, 0, 0)
          setMockDrag({ label: r.localName, x: at.left - 20, y: at.bottom + 50 })
        }, 300)
      }
      return
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [state, detectSettled])

  // ---------------------------------------------------------------- render
  const root = state?.rootsDisplay?.[0] ?? ''
  const cliTip = cliReady ? undefined : t('banner.cliMissing')
  const bindTargetRepo = bindTarget ? find(bindTarget.repoId) : undefined
  const httpsRepo = httpsId ? find(httpsId) : undefined
  const selectedRepos = allRepos.filter((r) => selected.has(r.id))
  const bindableSelected = selectedRepos.filter(needsBinding)
  const dragExtra = Math.max(0, dragIds.length - 1)
  const writable = state?.warnings?.find((w) => w.code === WarningCode.WarnProjectsNotWritable)
  const hiddenCount = counts.hidden
  // The project every moved repository is in already (marked in the menu); none when they come from several places.
  const currentProjects = (() => {
    const from = new Set((moveFor?.ids ?? []).map((id) => find(id)?.projectId ?? ''))
    return from.size === 1 ? from : new Set<string>()
  })()

  const header = (
    <header className="page__header">
      <div>
        <h1 className="page__title">{t('projects.title')}</h1>
        {state?.scannedAt && (
          <p className="page__subtitle">
            <Tooltip content={(state.rootsDisplay ?? []).length > 1 ? (state.rootsDisplay ?? []).join('\n') : undefined}>
              <span>
                <CodeText text={t('projects.subtitle', { p: projects.length, n: counts.all, folder: codeMark(root), when: formatRelative(state.scannedAt, now, t) })} />
              </span>
            </Tooltip>
          </p>
        )}
      </div>
      <div className="page__actions">
        <IconButton bordered label={t('projects.addFolder')} icon={FolderPlus} disabled={adding} onClick={() => void addFolder()} />
        <IconButton bordered label={t('projects.rescan')} icon={RefreshCw} className={isScanning ? 'is-spinning' : undefined} onClick={() => void scan()} />
        <Tooltip content={cliTip}>
          <Button variant="secondary" icon={Download} onClick={() => setCloneOpen(true)}>
            {t('projects.clone')}
          </Button>
        </Tooltip>
        <Button variant="primary" icon={Plus} onClick={() => setModal({ mode: 'create' })}>
          {t('projects.new')}
        </Button>
      </div>
    </header>
  )

  if (loading && !state) {
    return (
      <div className="page">
        {header}
        <GlobalBanner scope="repos" />
        <div className="stack" aria-busy="true">
          {[0, 1, 2].map((i) => (
            <div key={i} className="stack" style={{ '--gap': 'var(--sp-2)' } as React.CSSProperties}>
              <Skeleton variant="block" height={36} />
              <Skeleton variant="block" height={52} />
              <Skeleton variant="block" height={52} />
            </div>
          ))}
        </div>
      </div>
    )
  }

  const sourceItems = [
    ...SOURCES.map((s) => ({ id: String(s.value), label: t(s.key), onSelect: () => (setSource(s.value), setShowHidden(false)) })),
    ...(hiddenCount > 0
      ? [{ id: 'hidden', label: t('projects.source.hidden', { n: hiddenCount }), icon: EyeOff, separatorBefore: true, onSelect: () => setShowHidden((v) => !v) }]
      : []),
  ]
  const sourceLabel = showHidden ? t('projects.source.hidden', { n: hiddenCount }) : t(SOURCES.find((s) => s.value === source)?.key ?? 'projects.source.all')

  const emptyAll = allRepos.length === 0 && !showHidden && counts.found === 0
  const nothingShown = !emptyAll && sections.every((s) => s.repos.length === 0) && constrained
  const ungroupedIdx = sections.findIndex((s) => s.id === UNGROUPED)

  const renderRow = (sectionId: string) => (r: Repo, tabStop: boolean) => (
    <ProjectRow
      key={r.id}
      repo={r}
      selected={selected.has(r.id)}
      tabStop={tabStop}
      hiddenList={sectionId === HIDDEN_SECTION}
      sectionId={sectionId}
      profiles={profiles}
      portals={accounts?.portals ?? []}
      now={now}
      cliReady={cliReady}
      testing={testing.has(r.id)}
      comparing={comparing.has(r.id)}
      flash={flashId === r.id}
      dragging={dragIds.includes(r.id)}
      asking={asking === r.id}
      cb={cb}
    />
  )

  // The backend splits names at '_', '-' or '.': show the separator the first repository really has.
  const suggestionSep = (() => {
    const first = suggestion?.repoIds?.[0] ? find(suggestion.repoIds[0]) : undefined
    const c = first && suggestion ? first.localName.charAt(suggestion.label.length) : ''
    return c === '_' || c === '-' || c === '.' ? c : '_'
  })()

  const suggestionStrip = suggestion && !showHidden && (
    <div className="suggest" role="group" aria-label={t('projects.suggest.aria')}>
      <Sparkles aria-hidden="true" />
      <span>
        <Emph text={t(suggestion.kind === 'prefix' ? 'projects.suggest.prefix' : 'projects.suggest.folder', { n: (suggestion.repoIds ?? []).length, label: codeMark(suggestion.label), sep: suggestionSep })} />
      </span>
      <span className="spacer" />
      <Button size="sm" onClick={() => void createFromSuggestion()}>
        {t('projects.suggest.create', { name: suggestion.label })}
      </Button>
      <Button size="sm" variant="ghost" onClick={() => void dismissSuggestion()}>
        {t('projects.suggest.dismiss')}
      </Button>
    </div>
  )

  const addCandidates = (mode: ModalState | null): Repo[] => {
    if (!mode) return []
    if (mode.mode === 'create') return allRepos.filter((r) => movable(r) && (!r.projectId || mode.initialIds?.includes(r.id)))
    return allRepos.filter((r) => r.projectId !== mode.project?.id && movable(r))
  }

  return (
    <div className="page">
      {header}
      <GlobalBanner scope="repos" />
      <div className="page__body proj-page" style={{ gap: 'var(--sp-5)' }}>
        {error && <ErrorBanner error={error} onRetry={() => void reload()} />}
        {writable && (
          <Banner kind="warn" title={t('projects.warn.notWritable', { n: writable.count })}>
            {t('projects.warn.notWritable.body')}
          </Banner>
        )}
        {counts.found > 0 && !showHidden && (
          <div className="suggest suggest--solid" role="group" aria-label={t('projects.found.aria')}>
            <Info aria-hidden="true" />
            <span>
              <Emph text={t('projects.found.banner', { n: counts.found, count: codeMark(t('projects.found.count', { n: counts.found })) })} />
            </span>
            <span className="spacer" />
            <Button size="sm" variant="primary" onClick={() => setFoundOpen(true)}>
              {t('projects.found.choose')}
            </Button>
            <Button size="sm" variant="ghost" onClick={() => void hideAllFound()}>
              {t('projects.found.hideAll')}
            </Button>
          </div>
        )}
        <div className="toolbar">
          <Segmented<ProjectFilter>
            ariaLabel={t('repos.filter.aria')}
            value={filter}
            onChange={setFilter}
            items={[
              { value: 'all', label: t('repos.filter.all'), count: counts.all },
              { value: 'todo', label: t('repos.filter.todo'), count: counts.toFix, warn: counts.toFix > 0 },
              { value: 'changes', label: t('projects.filter.changes'), count: changesCount(allRepos) },
            ]}
          />
          <Menu
            trigger={
              <Button variant="ghost" size="sm">
                <Filter className="btn__icon" aria-hidden="true" />
                {sourceLabel}
              </Button>
            }
            items={sourceItems}
          />
          {plan.items.length > 0 && (
            <Tooltip content={cliTip}>
              <Button size="sm" variant="secondary" icon={ListChecks} disabled={!cliReady} onClick={() => setBulk({ mode: 'suggestions' })}>
                {t('repos.bindAll', { n: plan.items.length })}
              </Button>
            </Tooltip>
          )}
          <label className="search">
            <Search aria-hidden="true" />
            <input ref={searchRef} className="input" placeholder={t('projects.search')} aria-label={t('projects.search.aria')} value={query} onChange={(e) => setQuery(e.target.value)} />
          </label>
        </div>

        {isScanning && (
          <div className="stack" style={{ '--gap': 'var(--sp-1)' } as React.CSSProperties}>
            <Progress indeterminate label={t('repos.scan.progress', { folder: scanning?.root || root, n: scanning?.found ?? 0 })} />
            <div className="hint">{t('repos.scan.progress', { folder: scanning?.root || root, n: scanning?.found ?? 0 })}</div>
          </div>
        )}

        {emptyAll ? (
          <EmptyState
            title={t('projects.empty.title')}
            text={t('projects.empty.body')}
            actions={
              <>
                <Button variant="secondary" icon={Download} onClick={() => setCloneOpen(true)}>
                  {t('projects.clone')}
                </Button>
                <Button variant="primary" icon={FolderPlus} loading={adding} onClick={() => void addFolder()}>
                  {t('projects.addFolder.long')}
                </Button>
              </>
            }
          />
        ) : nothingShown ? (
          <EmptyState
            title={t('projects.empty.match.title')}
            actions={
              <Button
                variant="secondary"
                onClick={() => {
                  setQuery('')
                  setFilter('all')
                  setSource('all')
                }}
              >
                {t('projects.empty.clear')}
              </Button>
            }
          />
        ) : (
          <div ref={listRef} className="proj-list" style={{ display: 'flex', flexDirection: 'column', gap: 'var(--sp-5)' }}>
            {sections.map((s, i) => (
              <div key={s.id || '__ungrouped'} className="proj-wrap">
                {i === ungroupedIdx && suggestionStrip}
                <ProjectSection
                  section={s}
                  collapsed={isCollapsed(s.id)}
                  onToggle={() => toggleCollapsed(s.id)}
                  identity={s.project ? identities.byProject.get(s.project.id) : undefined}
                  renaming={!!s.project && renaming === s.project.id}
                  onRenameStart={() => setRenaming(s.project?.id ?? null)}
                  onRename={(name) => s.project && void renameProject(s.project, name)}
                  onRenameCancel={() => {
                    setRenaming(null)
                    pendingFocus.current = { section: s.id }
                    setLive((l) => ({ ...l }))
                  }}
                  onAddRepos={() => s.project && setModal({ mode: 'add', project: s.project })}
                  onIdentity={() => setRoute({ page: 'git' })}
                  onRemove={() => s.project && void ops.removeProject(s.project)}
                  dropOver={overSection === s.id}
                  dragging={dragIds.length > 0}
                  selected={selected}
                  onSelectAll={(ids, on) =>
                    setSelected((cur) => {
                      const n = new Set(cur)
                      for (const id of ids) {
                        if (on) n.add(id)
                        else n.delete(id)
                      }
                      return n
                    })
                  }
                  constrained={constrained}
                  tabStopId={active[s.id] && s.repos.some((r) => r.id === active[s.id]) ? active[s.id] : s.repos[0]?.id}
                  renderRow={renderRow(s.id)}
                />
              </div>
            ))}
            {ungroupedIdx < 0 && suggestionStrip}
          </div>
        )}

        {selected.size > 0 && !showHidden && (
          <div className="bulkbar" role="region" aria-label={t('repos.bulk.selected', { n: selected.size })}>
            <span>{t('repos.bulk.selected', { n: selected.size })}</span>
            <Button
              size="sm"
              onClick={(e) => openMove([...selected].filter((id) => order.includes(id)), e.currentTarget)}
            >
              {t('projects.bulk.move')}
              <ChevronDown className="btn__icon" aria-hidden="true" />
            </Button>
            <Button size="sm" loading={bulkTesting} disabled={!cliReady} onClick={() => void testSelected()}>
              {t('repos.action.test')}
            </Button>
            {bindableSelected.length > 0 && (
              <Button size="sm" variant="primary" disabled={!cliReady} onClick={() => setBulk({ mode: 'review', ids: bindableSelected.map((r) => r.id) })}>
                {t('repos.bulk.bind')}
              </Button>
            )}
            <Button size="sm" onClick={() => void hideMany([...selected])}>
              {t('projects.action.hide')}
            </Button>
            <IconButton size="sm" label={t('repos.bulk.clear')} icon={X} style={{ color: 'var(--on-ink)' }} onClick={() => setSelected(new Set())} />
          </div>
        )}
      </div>

      <div className="sr-only" role="status" aria-live="polite" aria-atomic="true" data-live={live.n}>
        {live.text}
      </div>

      <MoveMenu
        open={!!moveFor}
        anchorRef={moveAnchor}
        count={moveFor?.ids.length ?? 0}
        projects={projects}
        currentIds={currentProjects}
        onClose={() => {
          setMoveFor(null)
          announce(t('projects.live.cancelled'))
        }}
        onPick={(to) => {
          const ids = moveFor?.ids ?? []
          setMoveFor(null)
          void doMove(ids, to, undefined, true)
        }}
        onNew={() => {
          const ids = moveFor?.ids ?? []
          setMoveFor(null)
          setModal({ mode: 'create', initialIds: ids })
        }}
      />
      <DragGhost drag={drag ?? (mockDrag ? { ids: dragIds, label: mockDrag.label, x: mockDrag.x, y: mockDrag.y, over: overSection } : null)} extra={dragExtra} moveLabel={sectionName(drag ? drag.over : overSection) ? t('projects.drag.moveTo', { name: sectionName(drag ? drag.over : overSection) ?? '' }) : null} />

      {modal && (
        <ProjectModal
          mode={modal.mode}
          project={modal.project}
          initialName={modal.initialName}
          initialIds={modal.initialIds}
          candidates={addCandidates(modal)}
          onClose={() => setModal(null)}
          onCreate={createProject}
          onAdd={(ids) => (modal.project ? addToProject(modal.project, ids) : Promise.resolve(false))}
        />
      )}
      {foundOpen && (
        <FoundChooser
          found={state?.found ?? []}
          now={now}
          onClose={() => setFoundOpen(false)}
          onResolve={chooseFound}
          onReattach={(r) => answerMoved(r, true)}
          onDecline={(r) => answerMoved(r, false)}
        />
      )}
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
