import { cloneElement, useEffect, useRef, useState } from 'react'
import type { ReactElement } from 'react'
import { toUiError } from '../../api/errors'
import { Accounts } from '../../api/services'
import type { Portal } from '../../api/models'
import { Tooltip } from '../../components/Tooltip'
import { errorText, useI18n } from '../../i18n'
import { useActions } from '../../shell/AppActions'
import { useSettings } from '../../state/settings'

/** Open dialogs of the Accounts page. Entries name their subject; the page looks the object up in the current state. */
export type DialogState =
  | { kind: 'addPortal' }
  | { kind: 'addRole'; portal: string }
  | { kind: 'color'; profile: string }
  | { kind: 'remove'; profile: string }
  | { kind: 'move'; profile: string }
  | { kind: 'length'; portal: string }
  | { kind: 'rename'; portal: string }
  | { kind: 'removePortal'; portal: string }
  | { kind: 'export' }
  | { kind: 'import'; sample?: boolean }

/** Shows a danger toast for a failed call (cancellations are ignored). */
export function useErrorToast(): (e: unknown) => void {
  const { t } = useI18n()
  const { toast } = useActions()
  return (e: unknown) => {
    const ue = toUiError(e)
    if (ue.code === 'E_CANCELLED') return
    const text = errorText(ue, t)
    toast({ kind: 'danger', title: text.title, body: text.body && text.body !== text.title ? text.body : undefined })
  }
}

/** Confirm, then sign out of a portal (clears the CLI role cache; credentials already issued stay valid until they expire). */
export function useSignOut(): (portal: Portal) => Promise<void> {
  const { t, lang } = useI18n()
  const { confirm, toast } = useActions()
  const fail = useErrorToast()
  return async (portal: Portal) => {
    const names = (portal.profiles ?? []).join(lang === 'zh-CN' ? '、' : ', ')
    const ok = await confirm({
      title: t('session.signOut.confirm.title', { portal: portal.name }),
      body: (
        <div className="stack" style={{ ['--gap' as string]: 'var(--sp-3)' }}>
          <p style={{ margin: 0 }}>{t('session.signOut.confirm.body', { profiles: names })}</p>
          <p className="muted" style={{ margin: 0, fontSize: 'var(--fs-12)' }}>
            {t('session.logout.note')}
          </p>
        </div>
      ),
      confirmLabel: t('session.signOut'),
      danger: true,
    })
    if (!ok) return
    try {
      await Accounts.Logout(portal.name)
      toast({ kind: 'ok', title: t('session.signOut.done', { portal: portal.name }) })
    } catch (e) {
      fail(e)
    }
  }
}

/** Wraps a write action: in read-only mode (config file with a parse error) it is disabled and explains why. */
export function Gated({ blocked, fill, children }: { blocked: boolean; fill?: boolean; children: ReactElement<{ disabled?: boolean }> }) {
  const { t } = useI18n()
  if (!blocked) return children
  return (
    <Tooltip content={t('error.readOnly.tooltip')}>
      <span className={fill ? 'gated gated--fill' : 'gated'} tabIndex={0}>
        {cloneElement(children, { disabled: true })}
      </span>
    </Tooltip>
  )
}

/** Collapsed portals: change locally first (instant), then write to settings. While settings is null everything is expanded. */
export function useCollapsedPortals(portalNames: string[]) {
  const { settings, update } = useSettings()
  const failToast = useErrorToast()
  const saved = settings?.collapsedPortals
  const [local, setLocal] = useState<string[] | null>(null)
  const pending = useRef(0)

  // settings changed with no write of ours in flight: drop the local copy and follow settings
  useEffect(() => {
    if (pending.current === 0) setLocal(null)
  }, [saved])

  const list = local ?? saved ?? []

  const write = (next: string[]) => {
    const clean = next.filter((n) => portalNames.includes(n)) // also drops deleted or renamed portals
    setLocal(clean)
    pending.current += 1
    update({ collapsedPortals: clean })
      .catch((e) => {
        setLocal(null) // failed: fall back to the saved value
        failToast(e)
      })
      .finally(() => {
        pending.current -= 1
      })
  }

  const isCollapsed = (name: string) => list.includes(name)
  const toggle = (name: string) => write(isCollapsed(name) ? list.filter((n) => n !== name) : [...list, name])
  const allCollapsed = portalNames.length > 0 && portalNames.every(isCollapsed)
  const setAll = (collapsed: boolean) => write(collapsed ? [...portalNames] : [])
  return { isCollapsed, toggle, allCollapsed, setAll }
}
