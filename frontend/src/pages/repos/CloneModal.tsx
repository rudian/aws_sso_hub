import { CircleCheck } from 'lucide-react'
import { useEffect, useMemo, useRef, useState } from 'react'
import { Repos, System, onEvent, toUiError } from '../../api'
import type { ClonePreview, Profile, RemoteRepo, Repo } from '../../api/models'
import { AcctChip, Button, CodeBlock, EmptyState, ErrorBanner, Field, Input, Modal, Progress, RelativeTime, Select, Skeleton } from '../../components'
import { useI18n } from '../../i18n'
import type { UiError } from '../../api/errors'
import { AWS_REGIONS } from '../../lib/regions'
import { useAsync } from '../../lib/useAsync'
import { useAccounts } from '../../state/accounts'
import { useActions } from '../../shell/AppActions'
import { useReportError } from './hooks'
import { bindableProfiles, profileLabel, profileUsable } from './model'

export interface CloneModalProps {
  /** Profile to preselect when it is usable. */
  initialProfile?: string
  /** First scan folder (absolute): the default destination. */
  defaultParent: string
  cliReady: boolean
  onClose: () => void
}

const DEFAULT_REGION = 'us-east-1'

/** Clone from CodeCommit: account, repository, destination in one view, then git's progress inside the modal. */
export function CloneModal({ initialProfile, defaultParent, cliReady, onClose }: CloneModalProps) {
  const { t, lang } = useI18n()
  const { state: accounts } = useAccounts()
  const { openLogin } = useActions()
  const report = useReportError()

  const profiles = useMemo(() => bindableProfiles(accounts?.profiles ?? []), [accounts])
  const portals = accounts?.portals ?? []
  const usableNames = useMemo(() => new Set(profiles.filter((p) => profileUsable(p, portals)).map((p) => p.name)), [profiles, portals])

  const [profile, setProfile] = useState(() => (initialProfile && usableNames.has(initialProfile) ? initialProfile : (profiles.find((p) => usableNames.has(p.name))?.name ?? '')))
  const current: Profile | undefined = profiles.find((p) => p.name === profile)
  const [region, setRegion] = useState(current?.region || DEFAULT_REGION)
  const [query, setQuery] = useState('')
  const [repoName, setRepoName] = useState('')
  const [parentDir, setParentDir] = useState(defaultParent)
  const [folderName, setFolderName] = useState('')
  const [preview, setPreview] = useState<ClonePreview | null>(null)
  const [phase, setPhase] = useState<'form' | 'cloning' | 'done'>('form')
  const [lines, setLines] = useState<string[]>([])
  const [error, setError] = useState<UiError | null>(null)
  const [cloned, setCloned] = useState<Repo | null>(null)
  const logRef = useRef<HTMLPreElement>(null)

  const usable = !!current && usableNames.has(current.name)
  const remote = useAsync<RemoteRepo[]>(() => (usable ? Repos.ListRemoteRepos(profile, region).then((l) => l ?? []) : Promise.resolve([])), [profile, region, usable])
  const list = (remote.data ?? []).filter((r) => r.name.toLowerCase().includes(query.trim().toLowerCase()) || r.description.toLowerCase().includes(query.trim().toLowerCase()))

  // Unusable SSO portals, listed with an inline sign-in.
  const signedOutPortals = useMemo(() => {
    const names = new Set<string>()
    for (const p of profiles) if (p.portal && !usableNames.has(p.name)) names.add(p.portal)
    return [...names]
  }, [profiles, usableNames])

  const req = { profile, region, repoName, parentDir, folderName }
  const ready = profile !== '' && repoName !== '' && parentDir !== '' && folderName.trim() !== ''

  useEffect(() => {
    if (!ready) {
      setPreview(null)
      return
    }
    let alive = true
    const timer = window.setTimeout(() => {
      Repos.PreviewClone(req).then(
        (p) => alive && setPreview(p),
        () => alive && setPreview(null),
      )
    }, 200)
    return () => {
      alive = false
      window.clearTimeout(timer)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [profile, region, repoName, parentDir, folderName, ready])

  useEffect(() => {
    if (phase !== 'cloning') return
    return onEvent('clone:progress', (p) => setLines((l) => [...l.slice(-300), p.line]))
  }, [phase])
  useEffect(() => {
    logRef.current?.scrollTo({ top: logRef.current.scrollHeight })
  }, [lines])

  const pickProfile = (name: string) => {
    setProfile(name)
    const p = profiles.find((x) => x.name === name)
    if (p?.region) setRegion(p.region)
    setRepoName('')
    setFolderName('')
  }
  const pickRepo = (r: RemoteRepo) => {
    setRepoName(r.name)
    setFolderName(r.name)
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

  const clone = async () => {
    setError(null)
    setLines([])
    setPhase('cloning')
    try {
      const out = await Repos.Clone(req)
      if (!out.ok) throw out.error ?? new Error('clone failed')
      setCloned(out.repo ?? null)
      setPhase('done')
    } catch (e) {
      setError(toUiError(e))
      setPhase('form')
    }
  }

  const regions = AWS_REGIONS.includes(region) ? AWS_REGIONS : [region, ...AWS_REGIONS]
  const exists = preview?.exists === true
  const busy = phase === 'cloning'

  let footer
  if (phase === 'done') {
    footer = (
      <>
        {cloned && (
          <Button variant="secondary" onClick={() => void Repos.OpenFolder(cloned.id).catch(report)}>
            {t('clone.openFolder')}
          </Button>
        )}
        <Button variant="primary" onClick={onClose}>
          {t('common.done')}
        </Button>
      </>
    )
  } else {
    footer = (
      <>
        <Button variant="secondary" onClick={onClose} disabled={busy}>
          {t('common.cancel')}
        </Button>
        <Button variant="primary" loading={busy} disabled={!ready || exists || !cliReady} onClick={() => void clone()}>
          {t('clone.action')}
        </Button>
      </>
    )
  }

  return (
    <Modal open size="lg" title={t('clone.title')} dismissible={!busy} onClose={onClose} footer={footer}>
      {phase === 'done' ? (
        <div className="stack" style={{ alignItems: 'center', textAlign: 'center', padding: 'var(--sp-6) 0' }}>
          <CircleCheck size={40} style={{ color: 'var(--ok)' }} aria-hidden="true" />
          <div style={{ fontSize: 'var(--fs-16)', fontWeight: 600 }}>{t('clone.done', { name: profile })}</div>
          <div className="mono muted">{preview?.dest ?? ''}</div>
        </div>
      ) : (
        <div className="stack" style={{ '--gap': 'var(--sp-5)' } as React.CSSProperties}>
          {error && <ErrorBanner error={error} />}

          <div className="stack" style={{ '--gap': 'var(--sp-3)' } as React.CSSProperties}>
            <div className="cluster" style={{ alignItems: 'flex-start', gap: 'var(--sp-4)', flexWrap: 'nowrap' }}>
              <div style={{ flex: 1, minWidth: 0 }}>
              <Field label={t('clone.account')}>
                <Select value={profile} disabled={busy} onChange={(e) => pickProfile(e.target.value)}>
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
                <Select value={region} disabled={busy} onChange={(e) => setRegion(e.target.value)}>
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
            {signedOutPortals.map((portal) => (
              <div key={portal} className="cluster hint" style={{ gap: 'var(--sp-2)' }}>
                <span>{t('clone.signInHint', { portal })}</span>
                <Button size="sm" variant="secondary" onClick={() => void signIn(portal)}>
                  {t('clone.signIn')}
                </Button>
              </div>
            ))}
          </div>

          <Field label={t('clone.repo')}>
            <div className="stack" style={{ '--gap': 'var(--sp-2)' } as React.CSSProperties}>
              <Input placeholder={t('clone.repo.search')} aria-label={t('clone.repo.search')} value={query} disabled={!usable || busy} onChange={(e) => setQuery(e.target.value)} />
              <div className="card list" role="listbox" aria-label={t('clone.repo')} style={{ maxHeight: 220, overflow: 'auto' }}>
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
                ) : (remote.data ?? []).length === 0 ? (
                  <EmptyState title={t('clone.repo.empty', { region })} />
                ) : list.length === 0 ? (
                  <div className="hint" style={{ padding: 'var(--sp-4)' }}>
                    {t('clone.repo.noMatch')}
                  </div>
                ) : (
                  list.map((r) => (
                    <button
                      key={r.name}
                      type="button"
                      role="option"
                      aria-selected={r.name === repoName}
                      className="list__item"
                      disabled={busy}
                      onClick={() => pickRepo(r)}
                      style={{
                        width: '100%',
                        textAlign: 'left',
                        font: 'inherit',
                        color: 'inherit',
                        border: 0,
                        borderBottom: '1px solid var(--border-subtle)',
                        background: r.name === repoName ? 'color-mix(in srgb, var(--focus) 9%, var(--surface))' : 'transparent',
                      }}
                    >
                      <span style={{ minWidth: 0, flex: 1 }}>
                        <span className="row__primary" style={{ display: 'block' }}>
                          {r.name}
                        </span>
                        {r.description && <span className="row__secondary truncate" style={{ display: 'block' }}>{r.description}</span>}
                      </span>
                      {r.lastModified && (
                        <span className="muted" style={{ fontSize: 'var(--fs-12)', whiteSpace: 'nowrap' }}>
                          <RelativeTime iso={r.lastModified} />
                        </span>
                      )}
                    </button>
                  ))
                )}
              </div>
            </div>
          </Field>

          <div className="stack" style={{ '--gap': 'var(--sp-3)' } as React.CSSProperties}>
            <div className="cluster" style={{ alignItems: 'flex-start', flexWrap: 'nowrap', gap: 'var(--sp-4)' }}>
              <div style={{ flex: 1, minWidth: 0 }}>
              <Field label={t('clone.dest')}>
                <div className="cluster" style={{ flexWrap: 'nowrap' }}>
                  <Input mono readOnly value={parentDir} style={{ flex: 1 }} />
                  <Button variant="secondary" disabled={busy} onClick={() => void chooseParent()}>
                    {t('clone.dest.choose')}
                  </Button>
                </div>
              </Field>
              </div>
              <Field label={t('clone.folderName')} error={exists ? t('clone.folderExists') : undefined}>
                <Input mono value={folderName} disabled={busy} onChange={(e) => setFolderName(e.target.value)} />
              </Field>
            </div>
            {preview && <CodeBlock label={t('clone.command')} code={preview.command} copyable={false} />}
          </div>

          {busy && (
            <div className="stack" style={{ '--gap': 'var(--sp-2)' } as React.CSSProperties}>
              <div className="hint">{t('clone.progress')}</div>
              <Progress indeterminate label={t('clone.progress')} />
              <pre ref={logRef} className="code code__body mono" lang={lang} style={{ maxHeight: 130, overflow: 'auto', margin: 0 }}>
                {lines.join('\n')}
              </pre>
            </div>
          )}
        </div>
      )}
    </Modal>
  )
}
