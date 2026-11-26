import { useId, useState } from 'react'
import type { FormEvent } from 'react'
import { toUiError } from '../../api/errors'
import type { UiError } from '../../api/errors'
import type { Portal } from '../../api/models'
import { Button } from '../../components/Button'
import { ErrorBanner } from '../../components/ErrorBanner'
import { Field } from '../../components/Field'
import { Input } from '../../components/Input'
import { Modal } from '../../components/Modal'
import { useI18n } from '../../i18n'
import type { MessageKey } from '../../i18n'
import { PROXY_EXAMPLE, cleanProxy, proxyProblem } from '../../lib/proxy'
import type { ProxyProblem } from '../../lib/proxy'
import { useActions } from '../../shell/AppActions'
import { useSettings } from '../../state/settings'
import './accounts.css'

const problemText: Record<ProxyProblem, MessageKey> = {
  format: 'portal.consoleProxy.invalid.format',
  port: 'portal.consoleProxy.invalid.port',
  portRange: 'portal.consoleProxy.invalid.portRange',
  host: 'portal.consoleProxy.invalid.host',
}

/**
 * The HTTP proxy a portal's console opens through (settings.consoleProxies). With one set, the Console button
 * opens a Google Chrome of its own that sends AWS traffic through the proxy (macOS). Empty or "Turn off" = the
 * default browser again.
 */
export function ConsoleProxyDialog({ portal, onClose }: { portal: Portal; onClose: () => void }) {
  const { t } = useI18n()
  const { toast } = useActions()
  const { settings, update } = useSettings()
  const formId = useId()
  const name = portal.legacyFormat ? portal.startUrlHost.split('/')[0] : portal.name
  const current = settings?.consoleProxies?.[portal.name] ?? ''
  const [value, setValue] = useState(current)
  const [checked, setChecked] = useState(false) // the problem shows after leaving the field or trying to save
  const [busy, setBusy] = useState<'save' | 'off' | null>(null)
  const [error, setError] = useState<UiError | null>(null)
  const problem = proxyProblem(value)

  const write = async (proxy: string) => {
    setBusy(proxy ? 'save' : 'off')
    setError(null)
    try {
      await update({ consoleProxies: { [portal.name]: proxy } })
      toast({ kind: 'ok', title: proxy ? t('portal.consoleProxy.saved', { portal: name, proxy }) : t('portal.consoleProxy.turnedOff', { portal: name }) })
      onClose()
    } catch (e) {
      setError(toUiError(e))
      setBusy(null)
    }
  }

  const save = (e?: FormEvent) => {
    e?.preventDefault()
    if (busy) return
    setChecked(true)
    if (problem) return
    const next = cleanProxy(value) ?? ''
    if (next === current) onClose()
    else void write(next)
  }

  return (
    <Modal
      open
      title={t('portal.consoleProxy.title')}
      subtitle={<span className="mono">{name}</span>}
      onClose={onClose}
      footer={
        <>
          {current && (
            <Button variant="ghost" className="console-proxy__off" loading={busy === 'off'} disabled={!!busy} onClick={() => void write('')}>
              {t('portal.consoleProxy.off')}
            </Button>
          )}
          <Button variant="secondary" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button type="submit" form={formId} variant="primary" loading={busy === 'save'} disabled={!!busy}>
            {t('common.save')}
          </Button>
        </>
      }
    >
      <form id={formId} className="console-proxy" onSubmit={save} noValidate>
        <p className="console-proxy__lead">{t('portal.consoleProxy.body')}</p>
        <Field
          label={t('portal.consoleProxy.field')}
          hint={t('portal.consoleProxy.hint')}
          error={checked && problem ? t(problemText[problem], { example: PROXY_EXAMPLE }) : undefined}
        >
          <Input
            value={value}
            mono
            placeholder={PROXY_EXAMPLE}
            spellCheck={false}
            autoComplete="off"
            autoCapitalize="off"
            onChange={(e) => setValue(e.target.value)}
            onBlur={() => setChecked(true)}
          />
        </Field>
        {error && <ErrorBanner error={error} />}
      </form>
    </Modal>
  )
}
