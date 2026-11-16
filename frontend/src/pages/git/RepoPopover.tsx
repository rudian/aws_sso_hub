import { useEffect, useRef, useState } from 'react'
import type { RefObject } from 'react'
import { Identity } from '../../api'
import { ErrorCode, RepoAction } from '../../api/models'
import type { Author, GlobalIdentity, ProjectIdentity, RepoIdentity } from '../../api/models'
import { toUiError } from '../../api/errors'
import { Button, Field, Input, Popover } from '../../components'
import { useI18n } from '../../i18n'
import { fieldError, repoFollow, repoOptions, repoOwnStart, repoRequest, sameAuthor, validateAuthor } from './identityModel'
import type { RepoFollow } from './identityModel'
import { useIdentityToast } from './identityHooks'

export interface RepoPopoverProps {
  anchorRef: RefObject<HTMLElement | null>
  row: RepoIdentity
  project?: ProjectIdentity
  global: GlobalIdentity
  onClose: () => void
  /** Read the table again (after a Retry that found the config readable). */
  onRecovered?: () => void
}

const who = (a?: { name: string; email: string } | null) => (a && a.email ? `${a.name} <${a.email}>` : '')

/**
 * The repository editor (popover from the pencil), design 5.12: follow the project (project members), follow global,
 * or the repository's own. A config that cannot be read or written shows the danger state with the path and Retry.
 */
export function RepoPopover({ anchorRef, row, project, global, onClose, onRecovered }: RepoPopoverProps) {
  const { t } = useI18n()
  const { done, report } = useIdentityToast()
  const opts = repoOptions(project)
  const current = repoFollow(row, project)
  const [choice, setChoice] = useState<RepoFollow>(current)
  const startOwn = repoOwnStart(row, global)
  const [own, setOwn] = useState<Author>(startOwn)
  const [touched, setTouched] = useState({ name: false, email: false })
  const [busy, setBusy] = useState(false)
  // The config cannot be read or written: the file git or the app failed on ('' = the repository's folder).
  const [fail, setFail] = useState<{ file: string } | null>(() => (row.readError ? { file: row.readError.params?.file ?? row.readError.params?.path ?? row.path } : null))
  const firstRadio = useRef<HTMLInputElement>(null)
  const nameRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    const id = window.setTimeout(() => firstRadio.current?.focus(), 0)
    return () => window.clearTimeout(id)
  }, [])

  const ownValid = validateAuthor(own)
  const dirty = choice !== current || (choice === 'own' && !sameAuthor(own, startOwn))
  const canSave = dirty && (choice !== 'own' || (ownValid.name && ownValid.email))

  const close = () => {
    onClose()
    anchorRef.current?.focus()
  }

  const save = async () => {
    setBusy(true)
    const hadFail = fail
    setFail(null)
    try {
      const req = repoRequest(row, choice, own, project)
      const pv = await Identity.PreviewRepo(req)
      if (pv.plan.action === RepoAction.RepoActionError) {
        setFail({ file: pv.plan.error?.params?.file ?? row.path })
        return
      }
      if (hadFail && !dirty) {
        // Retry of a read failure with nothing changed: the config reads now, so just show the table again.
        onRecovered?.()
        close()
        return
      }
      const res = await Identity.ApplyRepo(req, pv.planHash)
      const bad = (res.outcomes ?? []).find((o) => !o.ok)
      if (bad) {
        setFail({ file: bad.error?.params?.file ?? row.path })
        return
      }
      done(t('identity.repo.done', { name: row.name }), res)
      close()
    } catch (e) {
      const ue = toUiError(e)
      if ((ue.code === ErrorCode.ErrGitFailed || ue.code === ErrorCode.ErrFileIO) && ue.params?.file) setFail({ file: ue.params.file })
      else report(e)
    } finally {
      setBusy(false)
    }
  }

  const globalValue = global.set ? who(global) : t('identity.project.followUnset')
  const projectValue = project?.identity ? who(project.identity) : globalValue
  const radio = (id: RepoFollow, title: string, text: string | null) => (
    <label className={`opt${choice === id ? ' is-on' : ''}`}>
      <input
        ref={choice === id ? firstRadio : undefined}
        type="radio"
        className="radio"
        name={`ident-repo-${row.repoId}`}
        checked={choice === id}
        onChange={() => {
          setChoice(id)
          setFail(null)
          if (id === 'own') window.setTimeout(() => nameRef.current?.focus(), 0)
        }}
      />
      <span className="opt__title">{title}</span>
      {text && <span className="opt__text">{text}</span>}
    </label>
  )


  return (
    <Popover anchorRef={anchorRef} open onClose={close} align="end" ariaLabel={t('identity.repo.title', { name: row.name })} className="ident-pop">
      <div className="ident-pop__title">{t('identity.repo.title', { name: row.name })}</div>
      <div className="stack" role="radiogroup" aria-label={t('identity.repo.title', { name: row.name })} style={{ '--gap': '8px' } as React.CSSProperties}>
        {opts.project && radio('project', t('identity.repo.followProject', { project: project?.name ?? '' }), projectValue)}
        {opts.global && radio('global', t('identity.repo.followGlobal'), globalValue)}
        {radio('own', t('identity.repo.own'), null)}
      </div>
      {choice === 'own' && (
        <div className="stack" style={{ '--gap': 'var(--sp-3)' } as React.CSSProperties}>
          <Field label={t('identity.field.name')} error={fieldError(own.name, ownValid.name, touched.name) ? t('identity.field.name.invalid') : undefined}>
            <Input ref={nameRef} value={own.name} autoComplete="off" spellCheck={false} onBlur={() => setTouched((x) => ({ ...x, name: true }))} onChange={(e) => setOwn({ ...own, name: e.target.value })} />
          </Field>
          <Field label={t('identity.field.email')} error={fieldError(own.email, ownValid.email, touched.email) ? t(own.email.trim() === '' ? 'identity.field.email.empty' : 'identity.field.email.invalid') : undefined}>
            <Input value={own.email} type="email" autoComplete="off" spellCheck={false} onBlur={() => setTouched((x) => ({ ...x, email: true }))} onChange={(e) => setOwn({ ...own, email: e.target.value })} />
          </Field>
        </div>
      )}
      {fail && (
        <div className="ident-pop__err" role="alert">
          <b>{t('identity.error.writeTitle')}</b>
          <span className="mono">{t('identity.error.writeBody', { file: fail.file })}</span>
        </div>
      )}
      <div className="ident-pop__foot">
        <Button variant="secondary" size="sm" disabled={busy} onClick={close}>
          {t('common.cancel')}
        </Button>
        <Button variant="primary" size="sm" loading={busy} disabled={!canSave && !fail} onClick={() => void save()}>
          {fail ? t('common.retry') : t('identity.repo.save')}
        </Button>
      </div>
    </Popover>
  )
}
