import { Banner } from '../../components/Banner'
import { AcctTile } from '../../components/AcctTile'
import { Button } from '../../components/Button'
import { useI18n } from '../../i18n'
import { MigrationStep, StepHead, WizardFooter } from './wizard'
import type { WizardCtx } from './wizard'

/** Step 1: who owns the key. Read-only; the app asked AWS "who am I" and changed nothing. */
export function StepIdentify({ w }: { w: WizardCtx }) {
  const { t } = useI18n()
  const id = w.st.identity
  const slot = w.profile?.colorSlot
  return (
    <>
      <StepHead title={t('migrate.s1.title')} lead={t('migrate.s1.body')} />
      <div className="wizard__body">
        {id.errorCode && (
          <Banner kind="warn">
            <div>{t('migrate.s1.invalid')}</div>
            <div className="muted mono">{t('migrate.s1.errorCode', { code: id.errorCode })}</div>
          </Banner>
        )}
        {id.isRoot && <Banner kind="danger">{t('migrate.s7.rootKey')}</Banner>}
        <div className="facts">
          <div className="fact">
            <div className="fact__k">{t('migrate.s1.account')}</div>
            <div className="fact__v mono" data-acct={slot || undefined}>
              {id.accountId ? (
                <>
                  <AcctTile slot={slot} monogram={w.profile?.monogram ?? ''} size="sm" />
                  {id.accountId}
                </>
              ) : (
                <span className="muted" style={{ fontFamily: 'var(--font-sans)' }}>
                  {t('migrate.s1.noAccount')}
                </span>
              )}
            </div>
          </div>
          <div className="fact">
            <div className="fact__k">{t('migrate.s1.user')}</div>
            <div className="fact__v mono">{id.iamUser ?? '—'}</div>
          </div>
          <div className="fact">
            <div className="fact__k">{t('migrate.s1.keyId')}</div>
            <div className="fact__v mono">{id.maskedKeyId ?? '—'}</div>
          </div>
        </div>
      </div>
      <WizardFooter
        w={w}
        primary={
          <Button variant="primary" onClick={() => w.setView(MigrationStep.MigrationPortal)}>
            {t('common.next')}
          </Button>
        }
      />
    </>
  )
}
