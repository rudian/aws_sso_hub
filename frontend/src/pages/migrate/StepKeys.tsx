import { ExternalLink } from 'lucide-react'
import { useEffect, useState } from 'react'
import { openExternal } from '../../api/platform'
import { Migration } from '../../api/services'
import { ErrorCode } from '../../api/models'
import { Banner } from '../../components/Banner'
import { Button } from '../../components/Button'
import { DiffView } from '../../components/DiffView'
import { Skeleton } from '../../components/Skeleton'
import { useI18n } from '../../i18n'
import { useActions } from '../../shell/AppActions'
import { deactivationNote, iamConsoleUrl, nextStatic } from './model'
import { useAccounts } from '../../state/accounts'
import { StepHead, WizardFooter } from './wizard'
import type { WizardCtx } from './wizard'
import { CodeText, codeMark } from '../../components/CodeText'

/** Step 6: remove the old key from ~/.aws/credentials (masked diff, redacted backup, danger confirm). */
export function StepRemoveKey({ w }: { w: WizardCtx }) {
  const { t } = useI18n()
  const { confirm, toast } = useActions()
  const preview = w.st.credentialsPreview
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    if (!preview) void w.run(() => Migration.PreviewKeyRemoval(w.name))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [!!preview])

  const remove = async () => {
    if (!preview) return
    const ok = await confirm({ title: t('migrate.s6.title'), body: t('migrate.s6.backupNote'), confirmLabel: t('migrate.s6.action'), danger: true })
    if (!ok) return
    setBusy(true)
    const s = await w.run(() => Migration.RemoveKey(w.name, preview.baseHash), {
      // the file moved on: show the new diff next to the error
      onError: (e) => {
        if (e.code === ErrorCode.ErrConfigChanged) void w.run(() => Migration.PreviewKeyRemoval(w.name), { keepError: true })
      },
    })
    setBusy(false)
    if (s?.keyRemoved) toast({ kind: 'ok', title: t('migrate.s6.done', { name: w.name }) })
  }

  return (
    <>
      <StepHead title={t('migrate.s6.title')} lead={t('migrate.s6.body')} />
      <div className="wizard__body">
        {preview ? <DiffView preview={preview} /> : <Skeleton variant="block" height={110} />}
        <p className="muted" style={{ margin: 0, fontSize: 'var(--fs-12)' }}>
          {t('migrate.s6.masked')} · {t('migrate.s6.backupNote')}
        </p>
      </div>
      <WizardFooter
        w={w}
        backTo={null}
        extra={
          <Button variant="secondary" disabled={busy || w.busy} onClick={() => void w.run(() => Migration.KeepKey(w.name))}>
            {t('migrate.s6.keep')}
          </Button>
        }
        primary={
          <Button variant="danger" loading={busy} disabled={!preview} onClick={() => void remove()}>
            {t('migrate.s6.action')}
          </Button>
        }
      />
    </>
  )
}

/** Step 7: deactivate the old key in IAM (optional), then finish. */
export function StepDeactivate({ w }: { w: WizardCtx }) {
  const { t } = useI18n()
  const { confirm } = useActions()
  const { state: accounts } = useAccounts()
  const { st } = w
  const id = st.identity
  const [busy, setBusy] = useState(false)
  const d = st.deactivation
  const note = d ? deactivationNote(d, t) : null
  const canAuto = !!id.iamUser && !id.isRoot && !id.errorCode
  const consoleUrl = d?.consoleUrl || (id.iamUser ? iamConsoleUrl(id.iamUser) : '')
  const next = nextStatic(accounts?.profiles ?? [], w.name)
  const maskedKeyId = id.maskedKeyId ?? ''

  const deactivate = async () => {
    const ok = await confirm({
      title: t('migrate.s7.now'),
      body: t('migrate.s7.now.body', { user: id.iamUser ?? '', maskedKeyId }),
      confirmLabel: t('migrate.s7.now.confirm'),
      danger: true,
    })
    if (!ok) return
    setBusy(true)
    await w.run(() => Migration.Deactivate(w.name))
    setBusy(false)
  }

  return (
    <>
      <StepHead title={t('migrate.s7.title')} lead={t('migrate.s7.body')} />
      <div className="wizard__body">
        {!st.keyRemoved && <Banner kind="info">{t('migrate.s6.kept')}</Banner>}
        {!st.keyInMemory && <Banner kind="info">{t('migrate.s7.unavailable')}</Banner>}
        {id.isRoot && <Banner kind="warn">{t('migrate.s7.rootKey')}</Banner>}
        <div className="option-cards">
          <section className="option-card">
            <h3>{t('migrate.s7.now')}</h3>
            <p>
              <CodeText text={t('migrate.s7.now.body', { user: codeMark(id.iamUser ?? '—'), maskedKeyId: codeMark(maskedKeyId || '—') })} />
            </p>
            <Button variant="secondary" loading={busy} disabled={!canAuto || d?.status === 'deactivated'} onClick={() => void deactivate()}>
              {t('migrate.s7.now')}
            </Button>
          </section>
          <section className="option-card">
            <h3>{t('migrate.s7.console')}</h3>
            <p>{t('migrate.s7.console.body', { maskedKeyId: maskedKeyId || '—' })}</p>
            <Button variant="secondary" icon={ExternalLink} disabled={!consoleUrl} onClick={() => void openExternal(consoleUrl)}>
              {t('migrate.s7.console.open')}
            </Button>
          </section>
        </div>
        {note && (
          <Banner kind={note.tone}>
            <div>{note.text}</div>
            {note.detail && <div className="muted">{note.detail}</div>}
          </Banner>
        )}
        {!d && <p className="muted" style={{ margin: 0, fontSize: 'var(--fs-12)' }}>{t('migrate.s7.skipHint')}</p>}
      </div>
      <WizardFooter
        w={w}
        backTo={null}
        extra={
          next ? (
            <Button variant="secondary" loading={w.busy} onClick={() => void w.finish(next.name)}>
              <CodeText text={t('migrate.finish.next', { name: codeMark(next.name) })} />
            </Button>
          ) : undefined
        }
        primary={
          <Button variant="primary" loading={w.busy} onClick={() => void w.finish()}>
            {t('migrate.finish')}
          </Button>
        }
      />
    </>
  )
}
