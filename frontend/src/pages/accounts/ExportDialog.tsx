import { Copy, Download } from 'lucide-react'
import { useMemo, useState } from 'react'
import { copyText } from '../../api/platform'
import { isCancel, toUiError } from '../../api/errors'
import type { UiError } from '../../api/errors'
import { Accounts, System } from '../../api/services'
import { ProfileKind } from '../../api/models'
import type { ExportRequest, Profile } from '../../api/models'
import { AcctTile } from '../../components/AcctTile'
import { Badge } from '../../components/Badge'
import { Button } from '../../components/Button'
import { Checkbox } from '../../components/Checkbox'
import { EmptyState } from '../../components/EmptyState'
import { ErrorBanner } from '../../components/ErrorBanner'
import { Modal } from '../../components/Modal'
import { Switch } from '../../components/Switch'
import { useI18n } from '../../i18n'
import { useActions } from '../../shell/AppActions'
import { useAccounts } from '../../state/accounts'
import './accounts.css'

/** Export portals and their SSO profiles as JSON (never keys or tokens). Static-key and legacy profiles are listed but disabled. */
export function ExportDialog({ onClose }: { onClose: () => void }) {
  const { t } = useI18n()
  const { toast } = useActions()
  const { state } = useAccounts()
  const portals = useMemo(() => (state?.portals ?? []).filter((p) => !p.draft && !p.legacyFormat), [state])
  const profiles = state?.profiles ?? []

  const ssoOf = (portal: string) => profiles.filter((p) => p.kind === ProfileKind.ProfileKindSSO && p.portal === portal)
  const [portalSel, setPortalSel] = useState<Set<string>>(() => new Set(portals.map((p) => p.name)))
  const [profileSel, setProfileSel] = useState<Set<string>>(() => new Set(profiles.filter((p) => p.kind === ProfileKind.ProfileKindSSO).map((p) => p.name)))
  const [extras, setExtras] = useState(true)
  const [busy, setBusy] = useState<'copy' | 'save' | null>(null)
  const [error, setError] = useState<UiError | null>(null)

  const otherProfiles = profiles.filter((p) => p.kind === ProfileKind.ProfileKindStatic || p.kind === ProfileKind.ProfileKindLegacySSO)
  const request: ExportRequest = {
    portals: portals.filter((p) => portalSel.has(p.name)).map((p) => p.name),
    profiles: profiles.filter((p) => p.kind === ProfileKind.ProfileKindSSO && profileSel.has(p.name) && !!p.portal && portalSel.has(p.portal)).map((p) => p.name),
    includeExtras: extras,
  }
  const nothing = portals.length === 0 && ssoOf('').length === 0
  const none = request.portals!.length === 0

  const flip = (set: Set<string>, name: string, on: boolean) => {
    const next = new Set(set)
    if (on) next.add(name)
    else next.delete(name)
    return next
  }

  const copy = async () => {
    setBusy('copy')
    setError(null)
    try {
      const res = await Accounts.BuildExport(request)
      await copyText(res.json)
      toast({ kind: 'ok', title: t('export.copied') })
    } catch (e) {
      setError(toUiError(e))
    } finally {
      setBusy(null)
    }
  }

  const save = async () => {
    setBusy('save')
    setError(null)
    try {
      const res = await Accounts.SaveExport(request)
      const path = res.path ?? ''
      toast({
        kind: 'ok',
        title: t('export.saved', { path }),
        action: path ? { label: t('export.showFolder'), onClick: () => void System.RevealPath(path).catch(() => undefined) } : undefined,
      })
      onClose()
    } catch (e) {
      if (!isCancel(e) && toUiError(e).code !== 'E_CANCELLED') setError(toUiError(e))
      setBusy(null)
    }
  }

  return (
    <Modal
      open
      size="lg"
      title={t('export.title')}
      onClose={onClose}
      footer={
        <>
          <Button variant="ghost" className="transfer__left" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button variant="secondary" icon={Copy} loading={busy === 'copy'} disabled={nothing || none || busy === 'save'} onClick={() => void copy()}>
            {t('export.copy')}
          </Button>
          <Button variant="primary" icon={Download} loading={busy === 'save'} disabled={nothing || none || busy === 'copy'} onClick={() => void save()}>
            {t('export.save')}
          </Button>
        </>
      }
    >
      <div className="stack" style={{ ['--gap' as string]: 'var(--sp-4)' }}>
        <p className="muted" style={{ margin: 0 }}>
          {t('export.body')}
        </p>
        {nothing ? (
          <EmptyState title={t('export.empty')} />
        ) : (
          <>
            <div className="transfer__list" role="group" aria-label={t('export.selection')}>
              {portals.map((p) => {
                const on = portalSel.has(p.name)
                return (
                  <div key={p.name} className="transfer__group">
                    <Checkbox
                      checked={on}
                      label={
                        <span>
                          <strong>{p.name}</strong> <span className="mono muted">{p.startUrlHost}</span>
                        </span>
                      }
                      onChange={(e) => setPortalSel(flip(portalSel, p.name, e.target.checked))}
                    />
                    <div className="transfer__children">
                      {ssoOf(p.name).map((pr) => (
                        <ProfileRow key={pr.name} profile={pr}>
                          <Checkbox
                            checked={on && profileSel.has(pr.name)}
                            disabled={!on}
                            label={<ProfileLabel profile={pr} />}
                            onChange={(e) => setProfileSel(flip(profileSel, pr.name, e.target.checked))}
                          />
                        </ProfileRow>
                      ))}
                    </div>
                  </div>
                )
              })}
              {otherProfiles.length > 0 && (
                <div className="transfer__group">
                  <div className="transfer__children">
                    {otherProfiles.map((pr) => (
                      <ProfileRow key={pr.name} profile={pr}>
                        <Checkbox checked={false} disabled label={<ProfileLabel profile={pr} />} />
                        <Badge kind="neutral">{pr.kind === ProfileKind.ProfileKindStatic ? t('export.static') : t('export.legacy')}</Badge>
                      </ProfileRow>
                    ))}
                  </div>
                </div>
              )}
            </div>
            <label className="transfer__switch">
              <Switch checked={extras} onChange={setExtras} label={t('export.includeExtras')} />
              <span>{t('export.includeExtras')}</span>
            </label>
          </>
        )}
        {error && <ErrorBanner error={error} />}
      </div>
    </Modal>
  )
}

/** Colour tile and name; the checkbox that owns this label comes first in every row. */
function ProfileLabel({ profile }: { profile: Profile }) {
  return (
    <span className="transfer__profile">
      <AcctTile slot={profile.colorSlot} monogram={profile.monogram} size="sm" />
      <span className="mono">{profile.name}</span>
    </span>
  )
}

function ProfileRow({ profile, children }: { profile: Profile; children: React.ReactNode }) {
  return (
    <div className="transfer__row" data-acct={profile.colorSlot || undefined}>
      {children}
    </div>
  )
}
