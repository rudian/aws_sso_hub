import { ChevronRight } from 'lucide-react'
import { useMemo, useRef, useState } from 'react'
import type { Author, ProjectInfo, Repo } from '../../api/models'
import { Button, Field, Input, Modal } from '../../components'
import { useI18n } from '../../i18n'
import { prefixMatches } from './model'
import { RepoChecklist } from './RepoChecklist'

export interface ProjectModalProps {
  /** 'create': a new project; 'add': add repositories to an existing one. */
  mode: 'create' | 'add'
  project?: ProjectInfo
  initialName?: string
  /** Pre-ticked repositories (a suggestion, a row that opened the dialog). */
  initialIds?: readonly string[]
  /** Repositories that can be chosen: the ungrouped ones, or (add) every repository outside the project. */
  candidates: Repo[]
  onClose: () => void
  onCreate: (name: string, ids: string[], identity: Author | null) => Promise<boolean>
  onAdd: (ids: string[]) => Promise<boolean>
}

const MAX_NAME = 100
const EMAIL_RE = /^[^\s@]+@[^\s@]+$/

/** New project (design 5.2): the name, a checklist of ungrouped repositories pre-ticked by prefix, an optional identity. */
export function ProjectModal({ mode, project, initialName = '', initialIds = [], candidates, onClose, onCreate, onAdd }: ProjectModalProps) {
  const { t } = useI18n()
  const [name, setName] = useState(initialName)
  const [touched, setTouched] = useState<Set<string> | null>(null)
  const [withIdentity, setWithIdentity] = useState(false)
  const [idName, setIdName] = useState('')
  const [idEmail, setIdEmail] = useState('')
  const [busy, setBusy] = useState(false)
  const [tried, setTried] = useState(false)
  const nameRef = useRef<HTMLInputElement>(null)

  const checked = useMemo(() => {
    if (touched) return touched
    const s = new Set<string>(initialIds)
    if (mode === 'create') for (const id of prefixMatches(name, candidates.filter((r) => !r.projectId))) s.add(id)
    return s
  }, [touched, initialIds, mode, name, candidates])

  const toggle = (id: string, on: boolean) => {
    const next = new Set(checked)
    if (on) next.add(id)
    else next.delete(id)
    setTouched(next)
  }
  const toggleAll = (on: boolean) => setTouched(on ? new Set(candidates.map((r) => r.id)) : new Set())

  const trimmed = name.trim()
  const nameError = mode === 'create' && tried && (trimmed === '' ? t('projects.new.nameEmpty') : [...trimmed].length > MAX_NAME ? t('projects.new.nameLong', { n: MAX_NAME }) : '')
  const identityError = mode === 'create' && withIdentity && tried && (idName.trim() === '' || !EMAIL_RE.test(idEmail.trim())) ? t('projects.new.identityInvalid') : ''
  const ids = candidates.filter((r) => checked.has(r.id)).map((r) => r.id)

  const submit = async () => {
    setTried(true)
    if (mode === 'add') {
      if (ids.length === 0) return
      setBusy(true)
      const ok = await onAdd(ids)
      setBusy(false)
      if (ok) onClose()
      return
    }
    if (trimmed === '' || [...trimmed].length > MAX_NAME) return
    if (withIdentity && (idName.trim() === '' || !EMAIL_RE.test(idEmail.trim()))) return
    setBusy(true)
    const ok = await onCreate(trimmed, ids, withIdentity ? { name: idName.trim(), email: idEmail.trim() } : null)
    setBusy(false)
    if (ok) onClose()
  }

  const title = mode === 'create' ? t('projects.new.title') : t('projects.add.title', { name: project?.name ?? '' })
  return (
    <Modal
      open
      size="sm"
      title={title}
      onClose={onClose}
      dismissible={!busy}
      initialFocus={mode === 'create' ? nameRef : undefined}
      footer={
        <>
          <Button variant="ghost" onClick={onClose} disabled={busy}>
            {t('common.cancel')}
          </Button>
          <Button variant="primary" loading={busy} disabled={mode === 'add' && ids.length === 0} onClick={() => void submit()}>
            {mode === 'create' ? t('projects.new.create') : t('projects.add.action', { n: ids.length })}
          </Button>
        </>
      }
    >
      <form
        className="proj-new"
        onSubmit={(e) => {
          e.preventDefault()
          void submit()
        }}
      >
        {mode === 'create' && (
          <Field label={t('projects.new.name')} error={nameError || undefined}>
            <Input ref={nameRef} value={name} maxLength={200} autoComplete="off" onChange={(e) => setName(e.target.value)} />
          </Field>
        )}
        <div className="field">
          <div className="field__label">{t('projects.new.repos')}</div>
          <RepoChecklist
            repos={candidates}
            checked={checked}
            onToggle={toggle}
            onToggleAll={toggleAll}
            ariaLabel={t('projects.new.repos')}
            empty={<span className="muted">{t('projects.new.noRepos')}</span>}
          />
        </div>
        {mode === 'create' && (
          <div className="proj-new__identity">
            <button type="button" className="proj-new__disclose" aria-expanded={withIdentity} onClick={() => setWithIdentity((v) => !v)}>
              <ChevronRight size={14} aria-hidden="true" className={withIdentity ? 'is-open' : undefined} />
              {t('projects.new.identity')}
            </button>
            {withIdentity && (
              <div className="proj-new__fields">
                <p className="muted">{t('projects.new.identity.hint')}</p>
                <Field label={t('projects.new.identity.name')}>
                  <Input value={idName} autoComplete="off" onChange={(e) => setIdName(e.target.value)} />
                </Field>
                <Field label={t('projects.new.identity.email')} error={identityError || undefined}>
                  <Input value={idEmail} type="email" autoComplete="off" onChange={(e) => setIdEmail(e.target.value)} />
                </Field>
              </div>
            )}
          </div>
        )}
        <button type="submit" hidden />
      </form>
    </Modal>
  )
}
