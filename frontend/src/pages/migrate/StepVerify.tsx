import { Check, FolderOpen, X } from 'lucide-react'
import { Migration, System } from '../../api/services'
import { ErrorCode, MigrationItem, ProgressStatus } from '../../api/models'
import type { AppError } from '../../api/models'
import { Banner } from '../../components/Banner'
import { Button } from '../../components/Button'
import { CodeText, codeMark } from '../../components/CodeText'
import { Spinner } from '../../components/Spinner'
import { errorText, useI18n } from '../../i18n'
import { useActions } from '../../shell/AppActions'
import { useAppInfo } from '../../state/appInfo'
import { StepHead, WizardFooter } from './wizard'
import type { ItemProgress, WizardCtx } from './wizard'

type Row = 'pending' | 'running' | 'done' | 'failed'

const ITEMS: Array<{ item: MigrationItem; label: 'migrate.s5.backup' | 'migrate.s5.write' | 'migrate.s5.verify' }> = [
  { item: MigrationItem.MigrationItemBackup, label: 'migrate.s5.backup' },
  { item: MigrationItem.MigrationItemWrite, label: 'migrate.s5.write' },
  { item: MigrationItem.MigrationItemVerify, label: 'migrate.s5.verify' },
]

function rowState(p: ItemProgress | undefined, applying: boolean, finished: 'ok' | 'failed' | null): Row {
  if (p) return p.status === ProgressStatus.ProgressDone ? 'done' : p.status === ProgressStatus.ProgressFailed ? 'failed' : 'running'
  if (applying) return 'pending'
  return finished === 'ok' ? 'done' : 'pending'
}

/** Step 5: backup, write, verify with the new SSO setup. A failed check offers "Restore backup" and "Try again". */
export function StepVerify({ w }: { w: WizardCtx }) {
  const { t } = useI18n()
  const { toast } = useActions()
  const appInfo = useAppInfo()
  const { st, progress, applying } = w
  const verify = st.verify
  const finished: 'ok' | 'failed' | null = applying || !verify ? null : verify.ok ? 'ok' : 'failed'

  const failure: AppError | null | undefined = finished === 'failed' ? verify?.error : null
  const failText = failure
    ? failure.code === ErrorCode.ErrVerifyWrongAccount && failure.params
      ? t('migrate.s5.fail.wrongAccount', { got: failure.params.got ?? '', want: failure.params.want ?? '' })
      : errorText({ code: failure.code, message: failure.message, detail: failure.detail, params: (failure.params ?? {}) as Record<string, string> }, t).title
    : ''

  const goNext = () => w.run(() => Migration.PreviewKeyRemoval(w.name))

  return (
    <>
      <StepHead title={t('migrate.step5')} />
      <div className="wizard__body">
        <ul className="check-list" aria-live="polite">
          {ITEMS.map(({ item, label }) => {
            const p = progress[item]
            const state = rowState(p, applying, finished)
            const failed = state === 'failed' || (item === MigrationItem.MigrationItemVerify && finished === 'failed')
            const kind = failed ? 'failed' : state
            const isBackup = item === MigrationItem.MigrationItemBackup
            // The backup location is also known without the progress event (for example after the page was reopened).
            const detail =
              p?.detail ??
              (isBackup && kind === 'done' ? appInfo?.backupDir : undefined) ??
              (item === MigrationItem.MigrationItemVerify && verify?.ok ? [verify.accountId, verify.arn].filter(Boolean).join(' • ') : undefined)
            return (
              <li key={item} className={`check-item check-item--${kind}`}>
                <span className="check-item__mark">
                  {kind === 'running' && <Spinner />}
                  {kind === 'done' && <Check aria-hidden="true" />}
                  {kind === 'failed' && <X aria-hidden="true" />}
                </span>
                <div>
                  <div>
                    <CodeText text={t(label, { file: codeMark('~/.aws/config'), profileSec: codeMark(`[profile ${w.name}]`), ssoSec: codeMark(`[sso-session ${st.portal ?? ''}]`) })} />
                  </div>
                  {detail && <div className="check-item__detail mono">{detail}</div>}
                  {isBackup && kind === 'done' && (
                    <Button size="sm" variant="ghost" icon={FolderOpen} onClick={() => void System.OpenBackupFolder().catch(() => undefined)}>
                      {t('migrate.s5.backupFolder')}
                    </Button>
                  )}
                </div>
              </li>
            )
          })}
        </ul>
        {finished === 'ok' && <Banner kind="ok"><CodeText text={t('migrate.s5.ok', { name: codeMark(w.name) })} /></Banner>}
        {finished === 'failed' && (
          <Banner
            kind="danger"
            title={t('migrate.s5.fail.title')}
            actions={
              <>
                <Button size="sm" loading={w.busy} onClick={() => void w.run(() => Migration.RestoreConfig(w.name)).then((s) => s && toast({ kind: 'ok', title: t('migrate.s5.restored') }))}>
                  {t('migrate.s5.restore')}
                </Button>
                <Button size="sm" variant="primary" loading={w.busy} onClick={() => void w.run(() => Migration.RetryVerify(w.name))}>
                  {t('migrate.s5.tryAgain')}
                </Button>
              </>
            }
          >
            {failText}
          </Banner>
        )}
      </div>
      <WizardFooter
        w={w}
        backTo={null}
        primary={
          finished === 'ok' ? (
            <Button variant="primary" loading={w.busy} onClick={() => void goNext()}>
              {t('common.next')}
            </Button>
          ) : finished === null ? (
            <Button variant="primary" loading={applying} disabled>
              {t('migrate.s5.running')}
            </Button>
          ) : undefined
        }
      />
    </>
  )
}

