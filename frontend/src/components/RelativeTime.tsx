import { formatDateTime, formatRelative } from '../lib/time'
import { useNow } from '../lib/useNow'
import { useI18n } from '../i18n'

export function RelativeTime({ iso }: { iso: string }) {
  const { lang, t } = useI18n()
  const now = useNow(30_000)
  return (
    <time dateTime={iso} title={formatDateTime(iso, lang)}>
      {formatRelative(iso, now, t)}
    </time>
  )
}
