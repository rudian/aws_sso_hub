import { useEffect, useState } from 'react'
import { Migration } from '../../api/services'
import { Banner } from '../../components/Banner'
import { AcctTile } from '../../components/AcctTile'
import { Button } from '../../components/Button'
import { CodeText, codeMark } from '../../components/CodeText'
import { DiffView } from '../../components/DiffView'
import { Skeleton } from '../../components/Skeleton'
import { Tabs } from '../../components/Tabs'
import { useI18n } from '../../i18n'
import { StepHead, WizardFooter } from './wizard'
import type { WizardCtx } from './wizard'

/** Step 4: the config diff. The credentials file is only listed (step 6 handles it). */
export function StepReview({ w }: { w: WizardCtx }) {
  const { t } = useI18n()
  const { st } = w
  const [tab, setTab] = useState('config')
  const preview = st.configPreview

  useEffect(() => {
    if (!preview) void w.run(() => Migration.Preview(w.name))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [!!preview])

  return (
    <>
      <StepHead title={t('migrate.s4.title')} lead={<CodeText text={t('migrate.s4.body', { profileSec: codeMark(`[profile ${w.name}]`), ssoSec: codeMark(`[sso-session ${st.portal ?? ''}]`) })} />} />
      <div className="wizard__body">
        <div className="facts">
          <div className="fact">
            <div className="fact__k">{t('migrate.s1.account')}</div>
            <div className="fact__v mono" data-acct={w.profile?.colorSlot || undefined}>
              <AcctTile slot={w.profile?.colorSlot} monogram={w.profile?.monogram ?? ''} size="sm" />
              {st.accountId}
            </div>
          </div>
          <div className="fact">
            <div className="fact__k">{t('migrate.fact.role')}</div>
            <div className="fact__v">{st.role}</div>
          </div>
          <div className="fact">
            <div className="fact__k">{t('migrate.fact.method')}</div>
            <div className="fact__v">{t('migrate.fact.method.value')}</div>
          </div>
        </div>
        <Tabs
          ariaLabel={t('migrate.s4.title')}
          value={tab}
          onChange={setTab}
          tabs={[
            { id: 'config', label: '~/.aws/config' },
            { id: 'credentials', label: '~/.aws/credentials', badge: t('migrate.s4.stepBadge'), disabled: true, tooltip: t('migrate.s4.credentialsTip') },
          ]}
        />
        {preview ? <DiffView preview={preview} /> : <Skeleton variant="block" height={180} />}
        <Banner kind="info">{t('migrate.s4.note')}</Banner>
      </div>
      <WizardFooter
        w={w}
        primary={
          <Button variant="primary" loading={w.busy} disabled={!preview} onClick={() => void w.startApply()}>
            {t('migrate.s4.action')}
          </Button>
        }
      />
    </>
  )
}
