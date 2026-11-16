// The update screen (design 5.13). F1-17 replaces this file with the real screen (same export name).
import { PlaceholderPage } from '../../components/PlaceholderPage'
import { useI18n } from '../../i18n'

export function UpdatePage() {
  const { t } = useI18n()
  return <PlaceholderPage title={t('placeholder.update.title')} text={t('placeholder.update.text')} />
}
