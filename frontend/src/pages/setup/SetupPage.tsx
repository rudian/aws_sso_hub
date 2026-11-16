// The setup screen (design 5.1). F1-15 replaces this file with the real screen (same export name).
import { PlaceholderPage } from '../../components/PlaceholderPage'
import { useI18n } from '../../i18n'

export function SetupPage() {
  const { t } = useI18n()
  return <PlaceholderPage title={t('placeholder.setup.title')} text={t('placeholder.setup.text')} />
}
