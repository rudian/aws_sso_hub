import { useEffect, useRef, useState } from 'react'
import { ErrorCode } from '../../api/models'
import { toUiError } from '../../api/errors'
import type { UiError } from '../../api/errors'
import { Accounts } from '../../api/services'
import type { FilePreview, Portal } from '../../api/models'
import { Banner } from '../../components/Banner'
import { Button } from '../../components/Button'
import { DiffView } from '../../components/DiffView'
import { ErrorBanner } from '../../components/ErrorBanner'
import { Field } from '../../components/Field'
import { Input } from '../../components/Input'
import { Modal } from '../../components/Modal'
import { useI18n } from '../../i18n'
import { useActions } from '../../shell/AppActions'
import { SESSION_NAME_RE } from './model'
import './accounts.css'

/** Rename a portal: the new name is checked, the config diff previewed, then written. */
export function RenamePortalDialog({ portal, existing, onClose }: { portal: Portal; existing: string[]; onClose: () => void }) {
  const { t } = useI18n()
  const { toast } = useActions()
  const [name, setName] = useState(portal.name)
  const [preview, setPreview] = useState<FilePreview | null>(null)
  const [error, setError] = useState<UiError | null>(null)
  const [busy, setBusy] = useState(false)

  const trimmed = name.trim()
  const problem = trimmed === portal.name ? 'same' : !SESSION_NAME_RE.test(trimmed) ? t('addPortal.name.invalid') : existing.includes(trimmed) ? t('addPortal.name.taken') : null

  useEffect(() => {
    setPreview(null)
    setError(null)
    if (problem) return
    let alive = true
    const id = window.setTimeout(() => {
      Accounts.PreviewRenamePortal(portal.name, trimmed).then(
        (p) => alive && setPreview(p),
        (e) => alive && setError(toUiError(e)),
      )
    }, 200)
    return () => {
      alive = false
      window.clearTimeout(id)
    }
  }, [portal.name, trimmed, problem])

  const rename = async () => {
    if (!preview) return
    setBusy(true)
    try {
      await Accounts.RenamePortal(portal.name, trimmed, preview.baseHash)
      toast({ kind: 'ok', title: t('portal.rename.done', { name: trimmed }) })
      onClose()
    } catch (e) {
      setError(toUiError(e))
      setBusy(false)
    }
  }

  return (
    <Modal
      open
      size="lg"
      title={t('portal.rename.title', { name: portal.name })}
      onClose={onClose}
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button variant="primary" loading={busy} disabled={!preview || !!problem} onClick={() => void rename()}>
            {t('portal.rename.action')}
          </Button>
        </>
      }
    >
      <div className="stack" style={{ ['--gap' as string]: 'var(--sp-4)' }}>
        <Field label={t('portal.rename.newName')} error={problem && problem !== 'same' ? problem : undefined} hint={t('addPortal.name.hint')}>
          <Input value={name} mono onChange={(e) => setName(e.target.value)} autoFocus />
        </Field>
        {error && <ErrorBanner error={error} />}
        {preview && <DiffView preview={preview} />}
      </div>
    </Modal>
  )
}

/** Remove a portal that no profile uses any more. A portal that is still in use explains what to delete first. */
export function RemovePortalDialog({ portal, onClose }: { portal: Portal; onClose: () => void }) {
  const { t } = useI18n()
  const { toast } = useActions()
  const cancelRef = useRef<HTMLButtonElement>(null)
  const [preview, setPreview] = useState<FilePreview | null>(null)
  const [error, setError] = useState<UiError | null>(null)
  const [busy, setBusy] = useState(false)
  const n = portal.profiles?.length ?? 0

  useEffect(() => {
    let alive = true
    Accounts.PreviewRemovePortal(portal.name).then(
      (p) => alive && setPreview(p),
      (e) => alive && setError(toUiError(e)),
    )
    return () => {
      alive = false
    }
  }, [portal.name])

  const inUse = n > 0 || error?.code === ErrorCode.ErrPortalInUse
  const remove = async () => {
    if (!preview) return
    setBusy(true)
    try {
      await Accounts.RemovePortal(portal.name, preview.baseHash)
      toast({ kind: 'ok', title: t('portal.remove.done', { name: portal.name }) })
      onClose()
    } catch (e) {
      setError(toUiError(e))
      setBusy(false)
    }
  }

  return (
    <Modal
      open
      size="lg"
      title={t('portal.remove.confirm.title', { name: portal.name })}
      onClose={onClose}
      initialFocus={cancelRef}
      footer={
        <>
          <Button ref={cancelRef} variant="secondary" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button variant="danger" loading={busy} disabled={!preview || inUse} onClick={() => void remove()}>
            {t('portal.remove.confirm.action')}
          </Button>
        </>
      }
    >
      <div className="stack" style={{ ['--gap' as string]: 'var(--sp-4)' }}>
        <p style={{ margin: 0 }}>{t('portal.remove.confirm.body', { name: portal.name })}</p>
        {inUse && <Banner kind="warn">{t('portal.remove.inUse', { n: n || Number(error?.params.n ?? 1), name: portal.name })}</Banner>}
        {error && !inUse && <ErrorBanner error={error} />}
        {preview && <DiffView preview={preview} />}
      </div>
    </Modal>
  )
}
