import { Button } from '../../components'
import { useI18n } from '../../i18n'

/** "Is this acme_api, moved here?" (design 5.2 "Moved by hand"): shown under a row, or in the found chooser. */
export function MovedAsk({ name, busy, onYes, onNo }: { name: string; busy?: boolean; onYes: () => void; onNo: () => void }) {
  const { t } = useI18n()
  return (
    <div className="moved-ask" role="group" aria-label={t('projects.moved.question', { name })}>
      <span className="moved-ask__q">{t('projects.moved.question', { name })}</span>
      <Button size="sm" variant="secondary" loading={busy} data-grid-control="" onClick={onYes}>
        {t('projects.moved.yes')}
      </Button>
      <Button size="sm" variant="ghost" disabled={busy} data-grid-control="" onClick={onNo}>
        {t('projects.moved.no')}
      </Button>
    </div>
  )
}
