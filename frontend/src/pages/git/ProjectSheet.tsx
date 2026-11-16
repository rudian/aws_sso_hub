import { Layers } from 'lucide-react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Identity } from '../../api'
import { ErrorCode, RepoIdentityMode } from '../../api/models'
import type { Author, GlobalIdentity, ProjectIdentity, ProjectIdentityPreview, RepoChangePlan, RepoIdentity } from '../../api/models'
import { toUiError } from '../../api/errors'
import { Button, Field, Input, Sheet, Spinner } from '../../components'
import { RepoName } from '../../components/RepoName'
import { errorText, useI18n } from '../../i18n'
import { failedNames, fieldError, planKind, sameAuthor, sortPlans, validateAuthor, wasText } from './identityModel'
import type { RepoMeta } from './IdentitySection'
import { useIdentityToast } from './identityHooks'
import { RepoGlyph } from './RepoGlyph'

export interface ProjectSheetProps {
  /** false while the sheet plays its exit animation. */
  open: boolean
  project: ProjectIdentity
  global: GlobalIdentity
  /** Mock screenshots only (?open=identity:project:<id>:<email>): start in "own" with this email typed. */
  draftEmail?: string
  metaOf: (repoId: string, name: string) => RepoMeta
  onClose: () => void
}

const who = (a?: { name: string; email: string } | null) => (a && a.email ? `${a.name} <${a.email}>` : '')

/**
 * The project side sheet: follow global or the project's own identity, and the list of repositories that
 * change (design 5.12). The list comes from PreviewProject and is recomputed as the fields change.
 */
export function ProjectSheet({ open, project, global, draftEmail, metaOf, onClose }: ProjectSheetProps) {
  const { t } = useI18n()
  const { done, report } = useIdentityToast()
  const saved = project.identity ?? null
  const [mode, setMode] = useState<'follow' | 'own'>(saved || draftEmail ? 'own' : 'follow')
  const [own, setOwn] = useState<Author>({ name: saved?.name ?? global.name, email: draftEmail || (saved?.email ?? global.email) })
  const [pv, setPv] = useState<ProjectIdentityPreview | null>(null)
  // The draft (mode and fields) the shown preview was computed for; Save waits until it matches the draft.
  const [pvKey, setPvKey] = useState('')
  const [touched, setTouched] = useState({ name: false, email: false })
  const listRef = useRef<HTMLUListElement>(null)
  const [pvError, setPvError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const seq = useRef(0)
  const nameRef = useRef<HTMLInputElement>(null)
  const checkedRef = useRef<HTMLInputElement>(null)

  const valid = validateAuthor(own)
  const wanted: Author | null = mode === 'own' ? { name: own.name.trim(), email: own.email.trim() } : null
  const ready = mode === 'follow' || (valid.name && valid.email)
  const dirty = !sameAuthor(wanted, saved)
  const key = `${mode}\n${own.name.trim()}\n${own.email.trim()}`
  const rowsById = useMemo(() => new Map<string, RepoIdentity>((project.repos ?? []).map((r) => [r.repoId, r])), [project.repos])

  const preview = useCallback(async () => {
    if (!ready) {
      ++seq.current
      setPv(null)
      setPvError(null)
      setLoading(false)
      return
    }
    const my = ++seq.current
    setLoading(true)
    try {
      const out = await Identity.PreviewProject({ projectId: project.id, identity: wanted })
      if (my !== seq.current) return
      setPv(out)
      setPvKey(key)
      setPvError(null)
    } catch (e) {
      if (my !== seq.current) return
      const ue = toUiError(e)
      setPv(null)
      setPvKey(key)
      setPvError(errorText(ue, t).title)
    } finally {
      if (my === seq.current) setLoading(false)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [project.id, key, ready, t])

  // Recompute while typing (debounced); the first one runs at once.
  const first = useRef(true)
  useEffect(() => {
    const id = window.setTimeout(() => void preview(), first.current ? 0 : 250)
    first.current = false
    return () => window.clearTimeout(id)
  }, [preview])

  const plans: RepoChangePlan[] = sortPlans(pv?.repos ?? [])
  const writes = pv?.writes ?? 0
  // The shown preview was made for another draft: wait for the matching one before Save.
  const stale = ready && pvKey !== key
  const canSave = dirty && ready && !!pv && !loading && !stale && !busy

  const makeFollow = async (p: RepoChangePlan) => {
    try {
      const req = { repoId: p.repoId, mode: RepoIdentityMode.RepoModeProject }
      const prv = await Identity.PreviewRepo(req)
      const res = await Identity.ApplyRepo(req, prv.planHash)
      done(t('identity.project.makeFollow.done', { name: p.name }), res)
      await preview()
      // the link that was focused is gone: keep the person's place on that repository's row
      window.setTimeout(() => listRef.current?.querySelector<HTMLElement>(`[data-repo="${p.repoId}"]`)?.focus(), 30)
    } catch (e) {
      report(e)
    }
  }

  const save = async () => {
    if (!pv) return
    setBusy(true)
    try {
      const res = await Identity.ApplyProject({ projectId: project.id, identity: wanted }, pv.planHash)
      const failed = failedNames(res.outcomes)
      const plural = (base: string, n: number) => t((n === 1 ? `${base}.one` : base) as 'identity.project.doneRepos', { name: project.name, n, names: failed.join(', ') })
      if (failed.length > 0 && !res.recordUpdated) {
        // a repository could not be written: nothing was recorded for the project, and the sheet stays open
        done(plural('identity.project.failedSave', failed.length), res, 'warn')
        await preview()
        return
      }
      if (failed.length > 0) done(plural('identity.project.partial', failed.length), res, 'warn')
      else done(writes > 0 ? plural('identity.project.doneRepos', writes) : t('identity.project.done', { name: project.name }), res)
      onClose()
    } catch (e) {
      const ue = toUiError(e)
      report(e)
      if (ue.code === ErrorCode.ErrConfigChanged) await preview()
    } finally {
      setBusy(false)
    }
  }

  const saveLabel = writes > 0 ? t(writes === 1 ? 'identity.project.save.one' : 'identity.project.save', { n: writes }) : t('identity.project.saveOnly')

  return (
    <Sheet
      open={open}
      onClose={onClose}
      dismissible={!busy}
      initialFocus={checkedRef}
      icon={
        <span className="portal__icon" aria-hidden="true">
          <Layers />
        </span>
      }
      title={t('identity.project.title', { name: project.name })}
      subtitle={t('identity.project.subtitle')}
      footer={
        <>
          <Button variant="secondary" disabled={busy} onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button variant="primary" loading={busy} disabled={!canSave} onClick={() => void save()}>
            {saveLabel}
          </Button>
        </>
      }
    >
      <div className="stack" role="radiogroup" aria-label={t('identity.project.title', { name: project.name })} style={{ '--gap': '8px' } as React.CSSProperties}>
        <label className={`opt${mode === 'follow' ? ' is-on' : ''}`}>
          <input ref={mode === 'follow' ? checkedRef : undefined} type="radio" className="radio" name="ident-project" checked={mode === 'follow'} onChange={() => setMode('follow')} />
          <span className="opt__title">{t('identity.project.follow')}</span>
          <span className="opt__text">{global.set ? who(global) : t('identity.project.followUnset')}</span>
        </label>
        <label className={`opt${mode === 'own' ? ' is-on' : ''}`}>
          <input
            ref={mode === 'own' ? checkedRef : undefined}
            type="radio"
            className="radio"
            name="ident-project"
            checked={mode === 'own'}
            onChange={() => {
              setMode('own')
              window.setTimeout(() => nameRef.current?.focus(), 0)
            }}
          />
          <span className="opt__title">{t('identity.project.own')}</span>
          <span className="opt__text">{t('identity.project.own.hint')}</span>
        </label>
      </div>

      {mode === 'own' && (
        <div className="ident-sheet-fields">
          <Field label={t('identity.field.name')} error={fieldError(own.name, valid.name, touched.name) ? t('identity.field.name.invalid') : undefined}>
            <Input ref={nameRef} value={own.name} autoComplete="off" spellCheck={false} onBlur={() => setTouched((x) => ({ ...x, name: true }))} onChange={(e) => setOwn({ ...own, name: e.target.value })} />
          </Field>
          <Field label={t('identity.field.email')} error={fieldError(own.email, valid.email, touched.email) ? t(own.email.trim() === '' ? 'identity.field.email.empty' : 'identity.field.email.invalid') : undefined}>
            <Input value={own.email} type="email" autoComplete="off" spellCheck={false} onBlur={() => setTouched((x) => ({ ...x, email: true }))} onChange={(e) => setOwn({ ...own, email: e.target.value })} />
          </Field>
        </div>
      )}

      <div className="stack" style={{ '--gap': '8px' } as React.CSSProperties}>
        <div className="field__label" role="status" aria-live="polite">
          {loading || stale ? (
            <span className="cluster" style={{ gap: 6 }}>
              <Spinner /> {t('identity.project.affect.loading')}
            </span>
          ) : pvError ? (
            <span style={{ color: 'var(--danger)' }}>{t('identity.project.previewFailed')}</span>
          ) : !pv ? null : writes > 0 ? (
            t(writes === 1 ? 'identity.project.affect.one' : 'identity.project.affect', { n: writes })
          ) : (
            t('identity.project.affect.none')
          )}
        </div>
        {pvError && <div className="field__error">{pvError}</div>}
        {plans.length > 0 && (
          <ul ref={listRef} className="affect" aria-busy={loading || stale || undefined} style={loading || stale ? { opacity: 0.5 } : undefined}>
            {plans.map((p) => {
              const kind = planKind(p)
              const sunken = kind === 'keeps-own' || kind === 'keeps-external'
              return (
                <li key={p.repoId} data-repo={p.repoId} tabIndex={-1} className={kind === 'error' ? 'is-error' : sunken ? 'is-sunken' : undefined}>
                  <RepoGlyph provider={metaOf(p.repoId, p.name).provider} />
                  <RepoName localName={p.name} remoteName={metaOf(p.repoId, p.name).remoteName} className={sunken || kind === 'missing' ? 'muted' : undefined} />
                  <span className="spacer" />
                  {kind === 'change' && (wasText(p, rowsById) ? <span className="was">{wasText(p, rowsById)}</span> : <span className="hint">{t('identity.table.notSet')}</span>)}
                  {kind === 'change' && !p.after && <span className="hint">{t('identity.project.row.removed')}</span>}
                  {kind === 'unchanged' && <span className="hint">{t('identity.project.row.unchanged')}</span>}
                  {kind === 'missing' && <span className="hint">{t('identity.project.row.missing')}</span>}
                  {kind === 'keeps-own' && (
                    <>
                      <button type="button" className="link" aria-label={t('identity.project.makeFollow.named', { name: p.name })} onClick={() => void makeFollow(p)}>
                        {t('identity.project.makeFollow')}
                      </button>
                      <span className="src-tag src-tag--repo">{t('identity.project.keepsOwn')}</span>
                    </>
                  )}
                  {kind === 'keeps-external' && (
                    <>
                      <button type="button" className="link" aria-label={t('identity.project.makeFollow.named', { name: p.name })} onClick={() => void makeFollow(p)}>
                        {t('identity.project.makeFollow')}
                      </button>
                      <span className="src-tag src-tag--repo">{t('identity.project.keepsExternal')}</span>
                    </>
                  )}
                  {kind === 'error' && (
                    <>
                      <b>{t('identity.error.writeTitle')}</b>
                      <span className="affect__err mono">{p.error?.params?.file ?? p.path}</span>
                      <Button size="sm" variant="secondary" onClick={() => void preview()}>
                        {t('common.retry')}
                      </Button>
                    </>
                  )}
                </li>
              )
            })}
          </ul>
        )}
        <span className="field__hint">{t('identity.project.backup')}</span>
      </div>
    </Sheet>
  )
}
