import { useState } from 'react'
import { GitGlobal } from '../../api'
import { SavedCredentialStore } from '../../api/models'
import type { SavedCredential } from '../../api/models'
import { CircleCheck } from 'lucide-react'
import { Banner, Button, ErrorBanner, Skeleton } from '../../components'
import { useI18n } from '../../i18n'
import type { MessageKey } from '../../i18n'
import { useAsync } from '../../lib/useAsync'
import { useActions } from '../../shell/AppActions'
import { useReportError } from '../repos/hooks'

const STORE_KEY: Record<string, MessageKey> = {
  [SavedCredentialStore.StoreKeychain]: 'git.saved.explain.mac',
  [SavedCredentialStore.StoreCredentialManager]: 'git.saved.explain.win',
}

/** Card 3: old CodeCommit passwords in the keychain / Credential Manager (host only, never the password). */
export function SavedCard({ supported }: { supported: boolean }) {
  const { t } = useI18n()
  const { confirm, toast } = useActions()
  const report = useReportError()
  const list = useAsync<SavedCredential[]>(() => (supported ? GitGlobal.ListSavedCredentials().then((l) => l ?? []) : Promise.resolve([])), [supported])
  const [busy, setBusy] = useState<string | null>(null)
  const items = list.data ?? []

  const clear = async (ids: string[], title: string) => {
    const ok = await confirm({ title, body: t('git.saved.explain'), confirmLabel: ids.length === items.length && ids.length > 1 ? t('git.saved.clearAll') : t('git.saved.clear'), danger: true })
    if (!ok) return
    setBusy(ids.join(','))
    try {
      const n = await GitGlobal.ClearSavedCredentials(ids)
      toast({ kind: 'ok', title: t('git.saved.cleared', { n }) })
      await list.reload()
    } catch (e) {
      report(e)
    } finally {
      setBusy(null)
    }
  }

  return (
    <section className="card" aria-labelledby="git-saved-title">
      <div className="card__header">
        <h2 className="card__title" id="git-saved-title">
          {t('git.saved.title')}
        </h2>
        {supported && items.length > 1 && (
          <Button size="sm" variant="danger-ghost" style={{ marginLeft: 'auto' }} loading={busy !== null} onClick={() => void clear(items.map((i) => i.id), t('git.saved.clearAll.confirm', { n: items.length }))}>
            {t('git.saved.clearAll')}
          </Button>
        )}
      </div>
      <div className="card__body stack" style={{ '--gap': 'var(--sp-4)' } as React.CSSProperties}>
        <p style={{ margin: 0 }} className="muted">
          {t('git.saved.explain')}
        </p>
        {!supported ? (
          <Banner kind="info">{t('git.saved.unsupported')}</Banner>
        ) : list.error ? (
          <ErrorBanner error={list.error} onRetry={() => void list.reload()} />
        ) : list.loading && !list.data ? (
          <Skeleton variant="block" height={44} />
        ) : items.length === 0 ? (
          <p className="inline-ok" role="status">
            <CircleCheck aria-hidden="true" />
            {t('git.saved.empty')}
          </p>
        ) : (
          <div className="card list" role="list">
            {items.map((c) => (
              <div key={c.id} className="list__item" role="listitem">
                <div style={{ minWidth: 0, flex: 1 }}>
                  <div className="mono truncate" style={{ fontSize: 'var(--fs-13)' }}>
                    {c.host}
                    {c.path ?? ''}
                  </div>
                  <div className="hint">
                    {t('git.saved.store')}: {t(STORE_KEY[c.store] ?? 'git.saved.explain.mac')}
                  </div>
                </div>
                <Button size="sm" variant="secondary" loading={busy === c.id} onClick={() => void clear([c.id], t('git.saved.clear.confirm', { host: c.host }))}>
                  {t('git.saved.clear')}
                </Button>
              </div>
            ))}
          </div>
        )}
      </div>
    </section>
  )
}
