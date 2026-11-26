import { ArrowLeftRight, ChevronsDownUp, ChevronsUpDown, Download, Plus, RefreshCw, Upload } from 'lucide-react'
import { useEffect, useMemo, useRef, useState } from 'react'
import { Accounts } from '../../api/services'
import { ProfileKind, StaticState } from '../../api/models'
import { Button } from '../../components/Button'
import { CodeText, codeMark } from '../../components/CodeText'
import { EmptyState } from '../../components/EmptyState'
import { ErrorBanner } from '../../components/ErrorBanner'
import { IconButton } from '../../components/IconButton'
import { Menu } from '../../components/Menu'
import { Skeleton } from '../../components/Skeleton'
import { useI18n } from '../../i18n'
import { takeOpen } from '../../lib/initialUI'
import { useRefreshHandler } from '../../lib/keyboard'
import { useNavigate } from '../../lib/route'
import { GlobalBanner } from '../../shell/GlobalBanner'
import { useAccounts } from '../../state/accounts'
import { AddPortalModal } from './AddPortalModal'
import { ColorDialog } from './ColorDialog'
import { ConsoleProxyDialog } from './ConsoleProxyDialog'
import { ExportDialog } from './ExportDialog'
import { Gated, useCollapsedPortals, useSignOut } from './hooks'
import type { DialogState } from './hooks'
import { ImportDialog } from './ImportDialog'
import { LegacyPanel } from './LegacyPanel'
import { MoveReposDialog } from './MoveReposDialog'
import { PortalSection } from './PortalSection'
import { RemovePortalDialog, RenamePortalDialog } from './PortalDialogs'
import { RemoveProfileDialog } from './RemoveProfileDialog'
import { staticProfiles } from './model'
import './accounts.css'

/** Accounts: static-key profiles, one section per portal, dialogs for everything that writes. */
export function AccountsPage() {
  const { t } = useI18n()
  const navigate = useNavigate()
  const { state, loading, error, reload } = useAccounts()
  const signOut = useSignOut()
  const [dialog, setDialog] = useState<DialogState | null>(null)
  const [refreshing, setRefreshing] = useState(false)

  const readOnly = !!state?.readOnly
  const profiles = state?.profiles ?? []
  const portals = useMemo(() => (state?.portals ?? []).filter((p) => !p.draft), [state])
  const portalNames = useMemo(() => portals.map((p) => p.name), [portals])
  const fold = useCollapsedPortals(portalNames)
  const statics = useMemo(() => staticProfiles(profiles), [profiles])

  const doRefresh = async () => {
    setRefreshing(true)
    try {
      await reload()
    } finally {
      setRefreshing(false)
    }
  }
  useRefreshHandler(() => void doRefresh())

  // Identify static keys once each (STS GetCallerIdentity); the result arrives through accounts:changed.
  const identified = useRef(new Set<string>())
  useEffect(() => {
    const todo = statics.filter((p) => (p.staticState === StaticState.StaticUnverified || !p.staticState || (p.staticState === StaticState.StaticOK && !p.accountId)) && !identified.current.has(p.name))
    if (todo.length === 0) return
    todo.forEach((p) => identified.current.add(p.name))
    void (async () => {
      for (const p of todo) await Accounts.IdentifyStatic(p.name).catch(() => undefined)
    })()
  }, [statics])

  // ?open=… (screenshots and deep links)
  const opened = useRef(false)
  useEffect(() => {
    if (!state || opened.current) return
    opened.current = true
    let v: string | null
    if (takeOpen('addPortal') !== null) setDialog({ kind: 'addPortal' })
    else if ((v = takeOpen('addRole:')) !== null) setDialog({ kind: 'addRole', portal: v })
    else if ((v = takeOpen('color:')) !== null) setDialog({ kind: 'color', profile: v })
    else if ((v = takeOpen('removeProfile:')) !== null) setDialog({ kind: 'remove', profile: v })
    else if ((v = takeOpen('move:')) !== null) setDialog({ kind: 'move', profile: v })
    else if ((v = takeOpen('consoleProxy:')) !== null) setDialog({ kind: 'consoleProxy', portal: v })
    else if ((v = takeOpen('signOut:')) !== null) {
      const p = (state.portals ?? []).find((x) => x.name === v)
      if (p) void signOut(p)
    } else if (takeOpen('export') !== null) setDialog({ kind: 'export' })
    else if ((v = takeOpen('import')) !== null) setDialog({ kind: 'import', sample: v === ':sample' })
  }, [state, signOut])

  const close = () => setDialog(null)
  const portalOf = (name: string) => portals.find((p) => p.name === name)
  const profileOf = (name: string) => profiles.find((p) => p.name === name)

  const nothing = !!state && portals.length === 0 && profiles.length === 0

  return (
    <div className="page">
      <header className="page__header">
        <div>
          <h1 className="page__title">{t('accounts.title')}</h1>
          <p className="page__subtitle">{state ? <CodeText text={t('accounts.subtitle', { n: profiles.length, file: codeMark('~/.aws/config') })} /> : t('common.loading')}</p>
        </div>
        <div className="page__actions">
          {portals.length > 1 && (
            <IconButton
              label={t(fold.allCollapsed ? 'portal.expandAll' : 'portal.collapseAll')}
              icon={fold.allCollapsed ? ChevronsUpDown : ChevronsDownUp}
              bordered
              onClick={() => fold.setAll(!fold.allCollapsed)}
            />
          )}
          <IconButton label={t('tip.refresh')} icon={RefreshCw} bordered className={refreshing ? 'is-spinning' : undefined} disabled={refreshing} onClick={() => void doRefresh()} />
          <Menu
            align="end"
            trigger={<IconButton label={t('transfer.menu')} icon={ArrowLeftRight} bordered />}
            items={[
              { id: 'export', label: t('transfer.export'), icon: Upload, onSelect: () => setDialog({ kind: 'export' }) },
              { id: 'import', label: t('transfer.import'), icon: Download, disabled: readOnly, onSelect: () => setDialog({ kind: 'import' }) },
            ]}
          />
          <Gated blocked={readOnly}>
            <Button variant="primary" icon={Plus} onClick={() => setDialog({ kind: 'addPortal' })}>
              {t('accounts.addPortal')}
            </Button>
          </Gated>
        </div>
      </header>
      <GlobalBanner scope="accounts" />

      <div className="page__body">
        {error && !state && <ErrorBanner error={error} onRetry={() => void reload()} />}

        {loading && !state && (
          <div className="portal-panel" aria-busy="true" aria-label={t('card.spinnerAria')}>
            <Skeleton variant="text" width={180} />
            <div className="portal__grid">
              {[0, 1, 2].map((i) => (
                <Skeleton key={i} variant="block" height={160} />
              ))}
            </div>
          </div>
        )}

        {nothing && (
          <EmptyState
            title={t('accounts.empty.title')}
            text={t('accounts.empty.body')}
            actions={
              <>
                <Gated blocked={readOnly}>
                  <Button variant="primary" icon={Plus} onClick={() => setDialog({ kind: 'addPortal' })}>
                    {t('accounts.addPortal')}
                  </Button>
                </Gated>
                <Button variant="secondary" onClick={() => navigate({ page: 'tutorial' })}>
                  {t('accounts.empty.tutorial')}
                </Button>
              </>
            }
          />
        )}

        <LegacyPanel profiles={statics} readOnly={readOnly} />

        {portals.map((p) => (
          <PortalSection key={p.name} portal={p} profiles={profiles.filter((x) => x.kind !== ProfileKind.ProfileKindStatic)} readOnly={readOnly} onDialog={setDialog} collapsed={fold.isCollapsed(p.name)} onToggle={() => fold.toggle(p.name)} />
        ))}
      </div>

      {dialog?.kind === 'addPortal' && <AddPortalModal mode="new" onClose={close} />}
      {dialog?.kind === 'addRole' && portalOf(dialog.portal) && <AddPortalModal mode="addRole" portal={dialog.portal} onClose={close} />}
      {dialog?.kind === 'color' && profileOf(dialog.profile) && <ColorDialog profile={profileOf(dialog.profile)!} onClose={close} />}
      {dialog?.kind === 'remove' && profileOf(dialog.profile) && (
        <RemoveProfileDialog profile={profileOf(dialog.profile)!} onMove={() => setDialog({ kind: 'move', profile: dialog.profile })} onClose={close} />
      )}
      {dialog?.kind === 'move' && profileOf(dialog.profile) && <MoveReposDialog from={profileOf(dialog.profile)!} onClose={close} />}
      {dialog?.kind === 'rename' && portalOf(dialog.portal) && <RenamePortalDialog portal={portalOf(dialog.portal)!} existing={portals.map((p) => p.name)} onClose={close} />}
      {dialog?.kind === 'removePortal' && portalOf(dialog.portal) && <RemovePortalDialog portal={portalOf(dialog.portal)!} onClose={close} />}
      {dialog?.kind === 'consoleProxy' && portalOf(dialog.portal) && <ConsoleProxyDialog portal={portalOf(dialog.portal)!} onClose={close} />}
      {dialog?.kind === 'export' && <ExportDialog onClose={close} />}
      {dialog?.kind === 'import' && <ImportDialog autoSample={dialog.sample} onClose={close} />}
    </div>
  )
}
