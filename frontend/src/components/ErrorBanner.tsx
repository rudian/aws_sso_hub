import { Banner } from './Banner'
import { Button } from './Button'
import { errorText, useI18n } from '../i18n'
import type { UiError } from '../api/errors'

/** Danger banner for a UiError: title and body from the error table; `detail` only inside a "Details" disclosure. */
export function ErrorBanner({ error, onRetry }: { error: UiError; onRetry?: () => void }) {
  const { t } = useI18n()
  const text = errorText(error, t)
  return (
    <Banner kind="danger" title={text.title} actions={onRetry ? <Button size="sm" onClick={onRetry}>{t('common.retry')}</Button> : undefined}>
      {text.body && text.body !== text.title && <div>{text.body}</div>}
      {text.detail && (
        <details style={{ marginTop: 6 }}>
          <summary style={{ cursor: 'default' }}>{t('common.details')}</summary>
          <pre className="mono" style={{ margin: '6px 0 0', whiteSpace: 'pre-wrap', userSelect: 'text' }}>{text.detail}</pre>
        </details>
      )}
    </Banner>
  )
}
