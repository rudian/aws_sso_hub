import clsx from 'clsx'
import { CircleCheck, CircleDashed, CircleSlash, CircleX, FolderOpen, TriangleAlert } from 'lucide-react'
import { useEffect, useMemo, useRef, useState } from 'react'
import type { CSSProperties } from 'react'
import { ErrorCode, Repos, System, onEvent, toUiError } from '../../api'
import type { ClonePreview, Profile, RemoteRepo, Repo } from '../../api/models'
import { AcctChip, Badge, Button, CodeBlock, EmptyState, ErrorBanner, Field, IconButton, Input, Modal, Progress, RelativeTime, Select, Skeleton, Spinner } from '../../components'
import { errorText, useI18n } from '../../i18n'
import { RepoName } from '../../components/RepoName'
import type { UiError } from '../../api/errors'
import { AWS_REGIONS } from '../../lib/regions'
import { useAsync } from '../../lib/useAsync'
import { useAccounts } from '../../state/accounts'
import { useSettings } from '../../state/settings'
import { useActions } from '../../shell/AppActions'
import { useReportError } from '../repos/hooks'
import { bindableProfiles, profileLabel, profileUsable } from '../repos/model'
import './CloneModal.css'

export interface CloneModalProps {
  /** Profile to preselect when it is usable. */
  initialProfile?: string
  /** First scan folder (absolute): the default destination. */
  defaultParent: string
  cliReady: boolean
  onClose: () => void
}

const DEFAULT_REGION = 'us-east-1'

/** Mirrors the server's folder-name rules (F1-11) so a slip shows before anything is cloned; the server still decides. */
export function folderNameReason(name: string): string {
  if (name.trim() === '') return 'empty'
  if (name === '.' || name === '..') return 'dots'
  if (/[\\/]/.test(name)) return 'separator'
  // eslint-disable-next-line no-control-regex
  if (/[\u0000-\u001f\u007f<>:"|?*]/.test(name)) return 'badChar'
  if (/[. ]$/.test(name)) return 'trailing'
  const stem = name.split('.')[0].replace(/ +$/, '').toUpperCase()
  if (/^(CON|PRN|AUX|NUL|CONIN\$|CONOUT\$|COM[1-9\u00b9\u00b2\u00b3]|LPT[1-9\u00b9\u00b2\u00b3])$/.test(stem)) return 'reserved'
  if (new TextEncoder().encode(name).length > 255) return 'tooLong'
  return ''
}
const SERVER_REASONS = new Set(['empty', 'dots', 'separator', 'badChar', 'trailing', 'reserved', 'tooLong'])
/** Sign-in errors: every later clone would fail the same way. */
const AUTH = new Set<ErrorCode>([ErrorCode.ErrSessionExpired, ErrorCode.ErrNotSignedIn])

type ItemStatus = 'queued' | 'cloning' | 'done' | 'failed' | 'skipped'
interface QueueItem {
  name: string
  folder: string
  dest: string
  status: ItemStatus
  skip?: 'exists' | 'stopped'
  error?: UiError
  repo?: Repo
}

const stackGap = (g: string) => ({ '--gap': g }) as CSSProperties
const itemKey = (q: QueueItem) => (q.status === 'skipped' ? (q.skip ?? 'exists') : q.status)

function QueueList({ items, className, onOpen }: { items: QueueItem[]; className?: string; onOpen?: (repo: Repo) => void }) {
  const { t } = useI18n()
  return (
    <div className={clsx('card clone-q-list', className)} role="list">
      {items.map((q) => {
        const key = itemKey(q)
        const icon =
          key === 'queued' ? <CircleDashed /> : key === 'cloning' ? <Spinner /> : key === 'done' ? <CircleCheck /> : key === 'failed' ? <CircleX /> : <CircleSlash />
        const err = q.error ? errorText(q.error, t) : null
        return (
          <div key={q.name} role="listitem" className={`clone-q clone-q--${key}`}>
            <span className="clone-q__icon">{icon}</span>
            <span className="clone-q__name truncate" title={q.folder}>
              <RepoName localName={q.folder} remoteName={q.name} />
            </span>
            <span className="clone-q__state">
              {t(`clone.status.${key}` as 'clone.status.done')}
              {onOpen && q.repo && <IconButton size="sm" icon={FolderOpen} label={t('clone.openFolderOf', { name: q.folder })} onClick={() => onOpen(q.repo as Repo)} />}
            </span>
            {err && (
              <div className="clone-q__err">
                {err.title}
                {err.body && err.body !== err.title && <span> {err.body}</span>}
                {q.error?.detail && (
                  <details>
                    <summary>{t('common.details')}</summary>
                    <pre className="mono" style={{ margin: 0, whiteSpace: 'pre-wrap' }}>
                      {q.error.detail}
                    </pre>
                  </details>
                )}
              </div>
            )}
          </div>
        )
      })}
    </div>
  )
}

/** Clone from CodeCommit: account, repositories, destination in one view, then git's progress inside the modal. */
export function CloneModal({ initialProfile, defaultParent, cliReady, onClose }: CloneModalProps) {
  const { t, lang } = useI18n()
  const { state: accounts } = useAccounts()
  const { openLogin } = useActions()
  const { settings, update } = useSettings()
  const report = useReportError()

  const profiles = useMemo(() => bindableProfiles(accounts?.profiles ?? []), [accounts])
  const portals = accounts?.portals ?? []
  const usableNames = useMemo(() => new Set(profiles.filter((p) => profileUsable(p, portals)).map((p) => p.name)), [profiles, portals])

  const last = settings?.lastCloneProfile ?? ''
  const lastRegion = settings?.lastCloneRegion ?? ''
  const preferred = [initialProfile, last].find((n): n is string => !!n && usableNames.has(n))
  const firstUsable = profiles.find((p) => usableNames.has(p.name))?.name ?? ''
  const wanted = preferred ?? firstUsable
  const regionFor = (name: string, fallback: string) => (name === last && lastRegion ? lastRegion : (profiles.find((p) => p.name === name)?.region ?? '') || fallback)

  const [profile, setProfile] = useState(wanted)
  const current: Profile | undefined = profiles.find((p) => p.name === profile)
  const [region, setRegion] = useState(() => regionFor(wanted, DEFAULT_REGION))
  const [query, setQuery] = useState('')
  const [sel, setSel] = useState<Set<string>>(() => new Set())
  const [previews, setPreviews] = useState<Record<string, ClonePreview>>({})
  const [previewing, setPreviewing] = useState(false)
  const [previewNonce, setPreviewNonce] = useState(0)
  const [parentDir, setParentDir] = useState(defaultParent)
  /** Folder names the user typed, by remote name; a repository without an entry uses its remote name. */
  const [names, setNames] = useState<Record<string, string>>({})
  /** Names the server refused at preview time, by remote name: the folder name tried and the server's reason. */
  const [serverErrs, setServerErrs] = useState<Record<string, { folder: string; reason: string }>>({})
  const [phase, setPhase] = useState<'form' | 'cloning' | 'done'>('form')
  const [queue, setQueue] = useState<QueueItem[]>([])
  const [stopping, setStopping] = useState(false)
  const stopRef = useRef(false)
  const touched = useRef(false)
  const [lines, setLines] = useState<string[]>([])
  const [error, setError] = useState<UiError | null>(null)
  const logRef = useRef<HTMLPreElement>(null)
  const doneBtnRef = useRef<HTMLButtonElement>(null)
  const stopBtnRef = useRef<HTMLButtonElement>(null)

  const usable = !!current && usableNames.has(current.name)
  const usableRef = useRef(usable)
  usableRef.current = usable
  useEffect(
    () => () => {
      stopRef.current = true
    },
    [],
  )
  const remote = useAsync<RemoteRepo[]>(() => (usable ? Repos.ListRemoteRepos(profile, region).then((l) => l ?? []) : Promise.resolve([])), [profile, region, usable])
  const all = remote.data ?? []
  const q = query.trim().toLowerCase()
  const list = all.filter((r) => r.name.toLowerCase().includes(q) || r.description.toLowerCase().includes(q))

  const picked = all.filter((r) => sel.has(r.name))
  const multi = picked.length >= 2
  const single = picked.length === 1 ? picked[0] : undefined
  const hidden = picked.filter((r) => !list.includes(r)).length
  const folderOf = (n: string) => names[n] ?? n
  // A preview counts only for the folder name it was made for: while the debounced preview of a typed name is on its
  // way, the earlier answer says nothing about it (FU-19a).
  const fresh = (n: string) => previews[n]?.folderName === folderOf(n)
  const isExisting = (n: string) => fresh(n) && previews[n]?.exists === true
  const existing = picked.filter((r) => isExisting(r.name))
  const toClone = picked.filter((r) => !isExisting(r.name))
  const allFresh = picked.every((r) => fresh(r.name))
  // A preview of an earlier name is stale while the debounced preview of the typed one is on its way.
  const rawPreview = single ? (previews[single.name] ?? null) : null
  const preview = rawPreview && single && rawPreview.folderName === folderOf(single.name) ? rawPreview : null
  const exists = !multi && preview?.exists === true
  // In several-at-once, an unchanged name that exists is skipped; a name the user typed that exists is an error.
  const renamed = (n: string) => folderOf(n) !== n
  const takenAfterRename = (n: string) => multi && renamed(n) && fresh(n) && previews[n]?.exists === true
  const anyTaken = picked.some((r) => takenAfterRename(r.name))
  const existingSkipped = existing.filter((r) => !renamed(r.name) && fresh(r.name))
  const showExists = (n: string) => (multi ? takenAfterRename(n) : previews[n]?.exists === true && previews[n]?.folderName === folderOf(n))
  // Inline errors per ticked repository: the server's rules, then two repositories sharing one name.
  const nameErrors = useMemo(() => {
    const out: Record<string, string> = {}
    // Folded: APFS and NTFS are case-insensitive by default, so Shared and shared collide.
    const key = (s: string) => s.normalize('NFC').toLowerCase()
    const count = new Map<string, number>()
    for (const r of picked) count.set(key(folderOf(r.name)), (count.get(key(folderOf(r.name))) ?? 0) + 1)
    for (const r of picked) {
      const f = folderOf(r.name)
      const why = folderNameReason(f)
      const se = serverErrs[r.name]
      if (why) out[r.name] = `clone.nameErr.${why}`
      else if ((count.get(key(f)) ?? 0) > 1) out[r.name] = 'clone.nameErr.dup'
      else if (se && se.folder === f) out[r.name] = `clone.nameErr.${se.reason}`
    }
    return out
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [picked.map((r) => r.name).join('\n'), names, serverErrs])
  const nameBad = Object.keys(nameErrors).length > 0
  const ready = profile !== '' && parentDir !== '' && picked.length > 0 && !nameBad
  const canClone = ready && cliReady && !previewing && allFresh && toClone.length > 0
  const allShown = list.length > 0 && list.every((r) => sel.has(r.name))
  const someShown = list.some((r) => sel.has(r.name))

  // Late apply: settings or a sign-in arrived after mount and the user has not touched anything yet.
  useEffect(() => {
    const target = preferred ?? (profile === '' ? firstUsable : '')
    if (phase !== 'form' || touched.current || !target || target === profile) return
    setProfile(target)
    setRegion((r) => regionFor(target, r))
    setSel(new Set())
    setPreviews({})
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [settings, usableNames])

  // Unusable SSO portals, listed with an inline sign-in; the one behind the last clone's account comes first.
  const lastProfile = last && last !== profile && !usableNames.has(last) ? profiles.find((p) => p.name === last) : undefined
  const lastPortal = lastProfile?.portal || ''
  const signedOutPortals = useMemo(() => {
    const names = new Set<string>()
    for (const p of profiles) if (p.portal && !usableNames.has(p.name)) names.add(p.portal)
    const l = [...names]
    return lastPortal && names.has(lastPortal) ? [lastPortal, ...l.filter((x) => x !== lastPortal)] : l
  }, [profiles, usableNames, lastPortal])

  const selKey = picked.map((r) => r.name).join('\n')
  useEffect(() => {
    if (!ready) {
      setPreviews({})
      setPreviewing(false)
      return
    }
    let alive = true
    setPreviewing(true)
    const timer = window.setTimeout(async () => {
      const next: Record<string, ClonePreview> = {}
      const names = selKey.split('\n')
      for (let i = 0; i < names.length && alive; i += 6) {
        await Promise.all(
          names.slice(i, i + 6).map((n) =>
            Repos.PreviewClone({ profile, region, repoName: n, parentDir, folderName: folderOf(n) }).then(
              (p) => {
                next[n] = p
              },
              (e) => {
                // Trust the server: a folder name its rules refuse shows inline, before anything is cloned.
                const err = toUiError(e)
                const reason = ({ 'bad-char': 'badChar', 'too-long': 'tooLong' } as Record<string, string>)[err.params.reason] ?? err.params.reason
                if (err.code === ErrorCode.ErrInvalidInput && err.params.field === 'folderName' && SERVER_REASONS.has(reason)) {
                  const folder = folderOf(n)
                  setServerErrs((m) => ({ ...m, [n]: { folder, reason } }))
                }
              },
            ),
          ),
        )
      }
      if (alive) {
        setPreviews(next)
        setPreviewing(false)
      }
    }, 200)
    return () => {
      alive = false
      window.clearTimeout(timer)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [profile, region, parentDir, selKey, selKey.split('\n').map(folderOf).join('\n'), ready, previewNonce])

  useEffect(() => {
    if (phase !== 'cloning') return
    return onEvent('clone:progress', (p) => setLines((l) => [...l.slice(-300), p.line]))
  }, [phase])
  const stopShown = queue.some((x) => x.status === 'queued')
  useEffect(() => {
    if (phase === 'done') doneBtnRef.current?.focus()
    else if (phase === 'cloning') (stopShown && !stopping ? stopBtnRef.current : logRef.current?.closest<HTMLElement>('.modal'))?.focus()
  }, [phase, stopShown, stopping])
  useEffect(() => {
    logRef.current?.scrollTo?.({ top: logRef.current.scrollHeight })
  }, [lines])
  useEffect(() => {
    document.querySelector('.clone-q--cloning')?.scrollIntoView?.({ block: 'nearest' })
  }, [queue])

  const clearAll = () => {
    setSel(new Set())
    setNames({})
    setServerErrs({})
    setPreviews({})
  }
  const pickProfile = (name: string) => {
    touched.current = true
    setProfile(name)
    setRegion((r) => regionFor(name, r))
    clearAll()
  }
  const pickRegion = (r: string) => {
    touched.current = true
    setRegion(r)
    clearAll()
  }
  const toggle = (name: string) => {
    touched.current = true
    setSel((s) => {
      const n = new Set(s)
      if (n.has(name)) n.delete(name)
      else n.add(name)
      return n
    })
  }
  const clickRow = (r: RemoteRepo) => {
    if (picked.length <= 1) {
      touched.current = true
      setSel(new Set([r.name]))
    } else toggle(r.name)
  }
  const toggleShown = () => {
    touched.current = true
    setSel((s) => {
      const n = new Set(s)
      for (const r of list) {
        if (allShown) n.delete(r.name)
        else n.add(r.name)
      }
      return n
    })
  }
  const clearSel = () => {
    touched.current = true
    setSel(new Set())
  }
  const chooseParent = async () => {
    try {
      const dir = await System.ChooseFolder(t('clone.dest'))
      if (dir) setParentDir(dir)
    } catch (e) {
      report(e)
    }
  }
  const signIn = async (portal: string) => {
    await openLogin(portal)
  }

  const remember = () => {
    if (profile === last && region === lastRegion) return
    void update({ lastCloneProfile: profile, lastCloneRegion: region }).catch(() => undefined)
  }

  async function run(items: QueueItem[]) {
    const work = items.map((x) => ({ ...x }))
    const publish = () => setQueue(work.map((x) => ({ ...x })))
    const patch = (i: number, p: Partial<QueueItem>) => {
      Object.assign(work[i], p)
      publish()
    }
    stopRef.current = false
    setStopping(false)
    setError(null)
    setLines([])
    publish()
    setPhase('cloning')
    let ok = 0
    for (let i = 0; i < work.length; i++) {
      if (work[i].status !== 'queued') continue
      if (!usableRef.current) stopRef.current = true
      if (stopRef.current) {
        patch(i, { status: 'skipped', skip: 'stopped' })
        continue
      }
      setLines([])
      patch(i, { status: 'cloning' })
      try {
        const out = await Repos.Clone({ profile, region, repoName: work[i].name, parentDir, folderName: work[i].folder === work[i].name ? '' : work[i].folder })
        if (!out.ok) throw out.error ?? new Error('clone failed')
        patch(i, { status: 'done', repo: out.repo ?? undefined })
        ok++
      } catch (e) {
        const err = toUiError(e)
        if (err.code === ErrorCode.ErrDestExists) patch(i, { status: 'skipped', skip: 'exists' })
        else {
          patch(i, { status: 'failed', error: err })
          if (AUTH.has(err.code) || !usableRef.current) stopRef.current = true
        }
      }
    }
    if (ok > 0) remember()
    if (work.length === 1 && work[0].status !== 'done') {
      if (work[0].error) setError(work[0].error)
      setPreviewNonce((n) => n + 1)
      setPhase('form')
    } else setPhase('done')
  }
  const start = () =>
    void run(
      picked.map((r) => ({
        name: r.name,
        folder: folderOf(r.name),
        dest: (fresh(r.name) ? previews[r.name]?.dest : '') ?? '',
        ...(isExisting(r.name) ? { status: 'skipped' as const, skip: 'exists' as const } : { status: 'queued' as const }),
      })),
    )
  const retry = () =>
    void run(queue.map((x) => (x.status === 'failed' || (x.status === 'skipped' && x.skip === 'stopped') ? { ...x, status: 'queued' as const, error: undefined, skip: undefined } : x)))
  const stop = () => {
    stopRef.current = true
    setStopping(true)
  }

  const regions = AWS_REGIONS.includes(region) ? AWS_REGIONS : [region, ...AWS_REGIONS]
  const skipNames = existingSkipped
    .slice(0, 3)
    .map((r) => folderOf(r.name))
    .join(', ') + (existingSkipped.length > 3 ? ` +${existingSkipped.length - 3}` : '')

  const cmdText = toClone
    .filter((r) => fresh(r.name))
    .map((r) => previews[r.name]?.command)
    .filter(Boolean)
    .join('\n')

  const nDone = queue.filter((x) => x.status === 'done').length
  const nFailed = queue.filter((x) => x.status === 'failed').length
  const nExists = queue.filter((x) => x.status === 'skipped' && x.skip === 'exists').length
  const nStopped = queue.filter((x) => x.status === 'skipped' && x.skip === 'stopped').length
  const nRetry = nFailed + nStopped
  const runTotal = queue.length - nExists
  const runIndex = nDone + nFailed
  const onlyRepo = queue.find((x) => x.status === 'done')?.repo

  const signInRows = () =>
    signedOutPortals.map((portal) => (
      <div key={portal} className="cluster hint" style={{ gap: 'var(--sp-2)' }}>
        <span>{portal === lastPortal ? t('clone.signInHint.last', { profile: last }) : t('clone.signInHint', { portal })}</span>
        <Button size="sm" variant="secondary" onClick={() => void signIn(portal)}>
          {t('clone.signIn')}
        </Button>
      </div>
    ))

  const doneTitle =
    nDone === 0
      ? t('clone.doneNone')
      : nRetry > 0
        ? t('clone.donePartial', { done: nDone, total: nDone + nRetry })
        : nDone === 1
          ? t('clone.done', { name: profile })
          : t('clone.doneMany', { n: nDone, name: profile })

  let footer
  if (phase === 'done') {
    footer = (
      <>
        {nRetry > 0 && (
          <Button variant="secondary" disabled={!usable} onClick={retry}>
            {t('clone.retry', { n: nRetry })}
          </Button>
        )}
        {nDone === 1 && nRetry === 0 && onlyRepo && (
          <Button variant="secondary" onClick={() => void Repos.OpenFolder(onlyRepo.id).catch(report)}>
            {t('clone.openFolder')}
          </Button>
        )}
        <Button ref={doneBtnRef} variant="primary" onClick={onClose}>
          {t('common.done')}
        </Button>
      </>
    )
  } else if (phase === 'cloning') {
    footer = (
      <>
        {stopShown && (
          <Button ref={stopBtnRef} variant="secondary" disabled={stopping} onClick={stop}>
            {t(stopping ? 'clone.stopping' : 'clone.stop')}
          </Button>
        )}
        <Button variant="primary" loading>
          {t('clone.action')}
        </Button>
      </>
    )
  } else {
    footer = (
      <>
        <Button variant="secondary" onClick={onClose}>
          {t('common.cancel')}
        </Button>
        <Button variant="primary" disabled={!canClone || exists || anyTaken} onClick={start}>
          {multi && toClone.length > 0 ? t('clone.actionMany', { n: toClone.length }) : t('clone.action')}
        </Button>
      </>
    )
  }

  let body
  if (phase === 'cloning') {
    const cur = queue.find((x) => x.status === 'cloning')
    const one = queue.length === 1
    body = (
      <div className="stack" style={stackGap('var(--sp-4)')}>
        <div className="stack" style={stackGap('var(--sp-2)')}>
          <div className="clone-run__title">
            {one ? t('clone.progress') : t('clone.progressMany', { i: Math.min(runIndex + 1, runTotal), total: runTotal })}
          </div>
          {one ? <Progress indeterminate label={t('clone.progress')} /> : <Progress pct={(runIndex / Math.max(runTotal, 1)) * 100} label={t('clone.progress')} />}
        </div>
        {one ? <div className="mono muted clone-run__dest">{queue[0].dest}</div> : <QueueList items={queue} className="clone-queue" />}
        <div className="code">
          {!one && (
            <div className="code__head">
              <span className="code__lang truncate">{cur && <RepoName localName={cur.folder} remoteName={cur.name} />}</span>
            </div>
          )}
          <pre ref={logRef} className="code__body mono clone-log" lang={lang}>
            {lines.join('\n')}
          </pre>
        </div>
      </div>
    )
  } else if (phase === 'done' && queue.length === 1) {
    body = (
      <div className="stack" style={{ alignItems: 'center', textAlign: 'center', padding: 'var(--sp-6) 0' }}>
        <CircleCheck size={40} style={{ color: 'var(--ok)' }} aria-hidden="true" />
        <div style={{ fontSize: 'var(--fs-16)', fontWeight: 600 }}>{t('clone.done', { name: profile })}</div>
        <div className="mono muted">{queue[0].dest}</div>
      </div>
    )
  } else if (phase === 'done') {
    const tone = nRetry === 0 ? (nDone === 0 ? 'neutral' : 'ok') : nDone > 0 ? 'warn' : 'danger'
    const Icon = tone === 'ok' ? CircleCheck : tone === 'warn' ? TriangleAlert : tone === 'neutral' ? CircleSlash : CircleX
    const title = doneTitle
    const rank = (x: QueueItem) => (x.status === 'failed' ? 0 : x.status === 'skipped' && x.skip === 'stopped' ? 1 : x.status === 'skipped' ? 2 : 3)
    const sorted = queue.map((x, i) => ({ x, i })).sort((a, b) => rank(a.x) - rank(b.x) || a.i - b.i).map((o) => o.x)
    body = (
      <div className="stack" style={stackGap('var(--sp-4)')}>
        <div className="clone-sum">
          <Icon size={24} className={`clone-sum__icon clone-sum__icon--${tone}`} aria-hidden="true" />
          <div style={{ minWidth: 0 }}>
            <div className="clone-sum__title">{title}</div>
            <div className="mono muted clone-run__dest">{parentDir}</div>
          </div>
        </div>
        <div className="cluster">
          {nDone > 0 && <Badge kind="ok">{t('clone.sum.done', { n: nDone })}</Badge>}
          {nFailed > 0 && <Badge kind="danger">{t('clone.sum.failed', { n: nFailed })}</Badge>}
          {nStopped > 0 && <Badge kind="neutral">{t('clone.sum.stopped', { n: nStopped })}</Badge>}
          {nExists > 0 && <Badge kind="neutral">{t('clone.sum.exists', { n: nExists })}</Badge>}
        </div>
        {nRetry > 0 && !usable && signInRows()}
        <QueueList className="clone-result" items={sorted} onOpen={(repo) => void Repos.OpenFolder(repo.id).catch(report)} />
      </div>
    )
  } else {
    body = (
      <div className="stack" style={stackGap('var(--sp-5)')}>
        {error && <ErrorBanner error={error} />}

        <div className="stack" style={stackGap('var(--sp-3)')}>
          <div className="cluster" style={{ alignItems: 'flex-start', gap: 'var(--sp-4)', flexWrap: 'nowrap' }}>
            <div style={{ flex: 1, minWidth: 0 }}>
              <Field label={t('clone.account')}>
                <Select value={profile} onChange={(e) => pickProfile(e.target.value)}>
                  <option value="" disabled>
                    …
                  </option>
                  {profiles.map((p) => (
                    <option key={p.name} value={p.name} disabled={!usableNames.has(p.name)}>
                      {profileLabel(p, t('repos.profile.notIdentified'))}
                      {usableNames.has(p.name) ? '' : ` — ${t('clone.notSignedIn')}`}
                    </option>
                  ))}
                </Select>
              </Field>
            </div>
            <Field label={t('clone.region')}>
              <Select value={region} onChange={(e) => pickRegion(e.target.value)}>
                {regions.map((r) => (
                  <option key={r} value={r}>
                    {r}
                  </option>
                ))}
              </Select>
            </Field>
          </div>
          {current && (
            <div>
              <AcctChip slot={current.colorSlot} monogram={current.monogram} name={current.name} accountId={current.accountId} />
            </div>
          )}
          {signInRows()}
        </div>

        <Field label={t('clone.repo')} hint={usable && all.length > 1 ? t('clone.repo.hint') : undefined}>
          <div className="stack" style={stackGap('var(--sp-2)')}>
            <Input placeholder={t('clone.repo.search')} aria-label={t('clone.repo.search')} value={query} disabled={!usable} onChange={(e) => setQuery(e.target.value)} />
            <div className="card clone-pick" role="group" aria-label={t('clone.repo')}>
              {!usable ? (
                <div className="hint" style={{ padding: 'var(--sp-4)' }}>
                  {t('clone.repo.pick')}
                </div>
              ) : remote.loading && !remote.data ? (
                <div style={{ padding: 'var(--sp-3)' }} className="stack">
                  <Skeleton variant="text" width="60%" />
                  <Skeleton variant="text" width="45%" />
                  <Skeleton variant="text" width="52%" />
                </div>
              ) : remote.error ? (
                <div style={{ padding: 'var(--sp-3)' }}>
                  <ErrorBanner error={remote.error} onRetry={() => void remote.reload()} />
                </div>
              ) : all.length === 0 ? (
                <EmptyState title={t('clone.repo.empty', { region })} />
              ) : (
                <>
                  <div className="clone-pick__bar">
                    <label className="clone-pick__all">
                      <input
                        type="checkbox"
                        className="checkbox"
                        checked={allShown}
                        disabled={list.length === 0}
                        ref={(el) => {
                          if (el) el.indeterminate = someShown && !allShown
                        }}
                        onChange={toggleShown}
                      />
                      <span>{t(q ? 'clone.select.allShown' : 'clone.select.all', { n: list.length })}</span>
                    </label>
                    <span className="clone-pick__count num" aria-live="polite">
                      {picked.length > 0 && t('clone.select.count', { n: picked.length })}
                      {hidden > 0 && t('clone.select.hidden', { n: hidden })}
                    </span>
                    {picked.length > 0 && (
                      <button type="button" className="link" onClick={clearSel}>
                        {t('clone.select.clear')}
                      </button>
                    )}
                  </div>
                  {list.length === 0 ? (
                    <div className="hint" style={{ padding: 'var(--sp-4)' }}>
                      {t('clone.repo.noMatch')}
                    </div>
                  ) : (
                    list.map((r) => {
                      const on = sel.has(r.name)
                      return (
                        <div key={r.name} className={clsx('clone-row', on && 'is-selected')}>
                          <label className="clone-row__check">
                            <input type="checkbox" className="checkbox" checked={on} aria-label={t('clone.select.row', { name: r.name })} onChange={() => toggle(r.name)} />
                          </label>
                          <button type="button" className="clone-row__pick" tabIndex={-1} onClick={() => clickRow(r)}>
                            <span className="clone-row__text">
                              <span className="row__primary truncate" style={{ display: 'block' }} title={r.name}>
                                {r.name}
                              </span>
                              {r.description && (
                                <span className="row__secondary truncate" style={{ display: 'block' }}>
                                  {r.description}
                                </span>
                              )}
                            </span>
                            {on && multi && !renamed(r.name) && fresh(r.name) && previews[r.name]?.exists && <Badge kind="warn">{t('clone.folderExists')}</Badge>}
                            {r.lastModified && (
                              <span className="muted" style={{ fontSize: 'var(--fs-12)', whiteSpace: 'nowrap' }}>
                                <RelativeTime iso={r.lastModified} />
                              </span>
                            )}
                          </button>
                          {on && (
                            <div className="clone-row__name">
                              <div className="clone-row__field">
                                <label className="clone-row__label" htmlFor={`clone-name-${r.name}`}>
                                  {t('clone.localName')}
                                </label>
                                <Input
                                  id={`clone-name-${r.name}`}
                                  mono
                                  value={folderOf(r.name)}
                                  aria-label={t('clone.localName.for', { name: r.name })}
                                  aria-invalid={!!nameErrors[r.name] || showExists(r.name)}
                                  aria-describedby={`clone-name-err-${r.name}`}
                                  onChange={(e) => {
                                    const v = e.target.value
                                    setNames((m) => ({ ...m, [r.name]: v }))
                                  }}
                                />
                                {!nameErrors[r.name] && folderOf(r.name) !== r.name && <Badge kind="outline">{t('clone.differs')}</Badge>}
                              </div>
                              <div id={`clone-name-err-${r.name}`} className="clone-row__err" aria-live="assertive">
                                {nameErrors[r.name] ? t(nameErrors[r.name] as 'clone.nameErr.dup') : showExists(r.name) ? t('clone.folderExists') : ''}
                              </div>
                            </div>
                          )}
                        </div>
                      )
                    })
                  )}
                </>
              )}
            </div>
          </div>
        </Field>

        <div className="stack" style={stackGap('var(--sp-3)')}>
          <div className="cluster" style={{ alignItems: 'flex-start', flexWrap: 'nowrap', gap: 'var(--sp-4)' }}>
            <div style={{ flex: 1, minWidth: 0 }}>
              <Field label={t('clone.dest')} error={multi && !previewing && toClone.length === 0 ? t('clone.multi.allExist') : undefined}>
                <div className="cluster" style={{ flexWrap: 'nowrap' }}>
                  <Input mono readOnly value={parentDir} style={{ flex: 1 }} />
                  <Button variant="secondary" onClick={() => void chooseParent()}>
                    {t('clone.dest.choose')}
                  </Button>
                </div>
              </Field>
            </div>
          </div>
          {single && !nameBad && (
            <div className="clone-into" aria-live="polite">
              {(() => {
                const [before, after = ''] = t('clone.intoPath', { path: '\u0001' }).split('\u0001')
                // the separator the chosen folder itself uses (a Windows path keeps its backslashes)
                const sep = parentDir.includes('\\') && !parentDir.includes('/') ? '\\' : '/'
                const path = preview?.dest ?? `${parentDir.replace(/[\\/]+$/, '')}${sep}${folderOf(single.name)}`
                return (
                  <>
                    {before}
                    <span className="mono">{path}</span>
                    {after}
                  </>
                )
              })()}
            </div>
          )}
          {multi && toClone.length > 0 && <div className="hint">{t('clone.multi.dest', { n: toClone.length })}</div>}
          {multi && existingSkipped.length > 0 && toClone.length > 0 && (
            <div className="clone-skip">
              <TriangleAlert size={13} aria-hidden="true" />
              <span>{t('clone.multi.skip', { n: existingSkipped.length, names: skipNames })}</span>
            </div>
          )}
          {!multi && rawPreview && <CodeBlock label={t('clone.command')} code={rawPreview.command} copyable={false} />}
          {multi && cmdText !== '' && (
            <CodeBlock
              className="clone-cmds"
              label={t('clone.command')}
              copyable={false}
              code={cmdText}
            />
          )}
        </div>
      </div>
    )
  }

  const announce =
    phase === 'cloning'
      ? queue.length === 1
        ? t('clone.progress')
        : t('clone.progressMany', { i: Math.min(runIndex + 1, runTotal), total: runTotal })
      : phase === 'done'
        ? queue.length === 1
          ? t('clone.done', { name: profile })
          : doneTitle
        : ''

  return (
    <Modal open size="lg" title={t('clone.title')} dismissible={phase !== 'cloning'} onClose={onClose} footer={footer}>
      <div className="sr-only" role="status">
        {announce}
      </div>
      {body}
    </Modal>
  )
}
