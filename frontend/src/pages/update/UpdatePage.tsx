// The Update page (design 5.13): reached from the menu's "Check for updates" and from the Settings card. It shows
// the same panel as the dialog, inline, and never opens a modal by itself.
import { RefreshCw } from 'lucide-react'
import { useEffect, useRef } from 'react'
import { UpdatePhase } from '../../api/models'
import { Banner } from '../../components/Banner'
import { Button } from '../../components/Button'
import { useI18n } from '../../i18n'
import { formatRelative } from '../../lib/time'
import { useNow } from '../../lib/useNow'
import { canCheck, noteLines } from './model'
import { Notes, UpdatePanel } from './UpdatePanel'
import { UpdateFooter } from './UpdateDialog'
import { rememberedNotes, updateActions, useUpdateState } from './useUpdate'

export function UpdatePage() {
  const { t } = useI18n()
  const s = useUpdateState()
  const now = useNow(30_000)
  // "Check for updates..." in the menu opens this page, so the page checks once when it opens, after the first state
  // has arrived (current is empty until then) and only when no download or restart is on its way.
  const asked = useRef(false)
  useEffect(() => {
    if (asked.current || !s.current) return
    asked.current = true
    const phase = s.phase
    if (canCheck(s) && phase !== UpdatePhase.UpdateChecking && phase !== UpdatePhase.UpdateAvailable) void updateActions.check()
  }, [s])
  const checking = s.phase === UpdatePhase.UpdateChecking
  const offered = !!s.version && s.phase !== UpdatePhase.UpdateUpToDate && s.phase !== UpdatePhase.UpdateChecking && s.phase !== UpdatePhase.UpdateBlocked
  const history = s.phase === UpdatePhase.UpdateUpToDate || s.phase === UpdatePhase.UpdateIdle ? rememberedNotes(s.current) : ''

  return (
    <div className="page update-page">
      <header className="page__header">
        <div>
          <h1 className="page__title">{t('update.page.title')}</h1>
          <p className="page__subtitle">{t('update.page.sub')}</p>
        </div>
      </header>
      <div className="page__body">
        <section className="card update-card" aria-label={t('update.page.card', { current: s.current })}>
          <div className="update-card__head">
            <div>
              <h2 className="update-card__title">{t('update.page.card', { current: s.current })}</h2>
              <div className="update-card__sub">
                {s.checkedAt && s.phase !== UpdatePhase.UpdateBlocked ? t('update.status.lastChecked', { when: formatRelative(s.checkedAt, now, t) }) : t('update.status.never')}
              </div>
            </div>
            <div className="update-card__actions">
              <Button icon={RefreshCw} loading={checking} disabled={!canCheck(s)} onClick={() => void updateActions.check()}>
                {t('update.action.check')}
              </Button>
            </div>
          </div>
          {s.phase === UpdatePhase.UpdateUpToDate && (
            <Banner kind="ok" title={t('update.status.upToDate', { current: s.current })} />
          )}
          {s.phase !== UpdatePhase.UpdateUpToDate && <UpdatePanel s={s} showNotes={offered} idleLine={false} />}
          {offered && (
            <div style={{ marginTop: 'var(--sp-4)' }}>
              <UpdateFooter s={s} />
            </div>
          )}
          {history && noteLines(history).length > 0 && (
            <>
              <h3 className="update-card__h">{t('update.page.history')}</h3>
              <Notes notes={history} />
            </>
          )}
        </section>
      </div>
    </div>
  )
}
