import { useEffect, useMemo, useRef, useState } from 'react'
import { toUiError } from '../../api/errors'
import { Accounts, Migration } from '../../api/services'
import { ProfileKind } from '../../api/models'
import type { Portal } from '../../api/models'
import { Badge } from '../../components/Badge'
import { Button } from '../../components/Button'
import { Dot } from '../../components/Dot'
import { Radio } from '../../components/Radio'
import { useI18n } from '../../i18n'
import { useNow } from '../../lib/useNow'
import { useActions } from '../../shell/AppActions'
import { useAccounts } from '../../state/accounts'
import { PortalFields, usePortalForm } from '../accounts/PortalForm'
import { isSignedIn, pillState, pillText } from '../accounts/model'
import { NEW_PORTAL, suggestPortal } from './model'
import { StepHead, WizardFooter } from './wizard'
import type { WizardCtx } from './wizard'
import { CodeText, codeMark } from '../../components/CodeText'

/** Step 2: pick the portal (or add a new one) and sign in to it. */
export function StepPortal({ w }: { w: WizardCtx }) {
  const { t } = useI18n()
  const { openLogin } = useActions()
  const { state: accounts } = useAccounts()
  const now = useNow(30_000)
  const { st } = w
  const form = usePortalForm({ name: w.name })

  const portals = useMemo(() => (accounts?.portals ?? []).filter((p) => !p.legacyFormat && (!p.draft || p.name === st.portal)), [accounts, st.portal])
  const [matches, setMatches] = useState<ReadonlySet<string>>(new Set())
  const [choice, setChoice] = useState<string | null>(st.portal ?? null)
  const [draft, setDraft] = useState<string | null>(st.portalIsDraft ? (st.portal ?? null) : null)
  const [working, setWorking] = useState(false)

  // Ask every signed-in portal whether it lists the key's account (read-only).
  const checked = useRef(new Set<string>())
  useEffect(() => {
    const id = st.identity.accountId
    if (!id) return
    for (const p of portals) {
      if (p.draft || !isSignedIn(p.session) || checked.current.has(p.name)) continue
      checked.current.add(p.name)
      Accounts.DiscoverAccounts(p.name).then(
        (list) => {
          if ((list ?? []).some((a) => a.accountId === id)) setMatches((cur) => new Set([...cur, p.name]))
        },
        () => undefined,
      )
    }
  }, [portals, st.identity.accountId])

  const suggestion = useMemo(
    () => suggestPortal({ accountId: st.identity.accountId, portals, profiles: accounts?.profiles ?? [], matches }),
    [st.identity.accountId, portals, accounts, matches],
  )
  const selected = choice ?? suggestion
  const portal: Portal | undefined = portals.find((p) => p.name === selected)
  const isNew = selected === NEW_PORTAL || !portal
  const ready = !isNew && !!portal && isSignedIn(portal.session)

  const choose = (name: string) => setChoice(name)

  const chooseInBackend = async (name: string) => {
    // a draft the user created earlier and then abandoned goes away
    if (draft && draft !== name) await Accounts.DiscardPortalDraft(draft).catch(() => undefined)
    if (draft && draft !== name) setDraft(null)
    await w.run(() => Migration.ChoosePortal(w.name, name))
  }

  const next = async () => {
    w.setError(null)
    setWorking(true)
    try {
      if (isNew) {
        const { ok, values } = await form.validate()
        if (!ok) return
        let name = draft
        if (!name || name !== values.name) {
          if (draft) await Accounts.DiscardPortalDraft(draft).catch(() => undefined)
          const p = await Accounts.AddPortalDraft({ name: values.name, startUrl: values.startUrl, region: values.region })
          name = p.name
          setDraft(name)
        }
        const res = await openLogin(name)
        if (res) await chooseInBackend(name)
      } else if (portal) {
        if (!ready) {
          const res = await openLogin(portal.name)
          if (!res) return
        }
        await chooseInBackend(portal.name)
      }
    } catch (e) {
      w.setError(toUiError(e))
    } finally {
      setWorking(false)
    }
  }

  const primaryLabel = isNew ? t('addPortal.signInContinue') : ready ? t('common.next') : t('migrate.s2.signIn')

  return (
    <>
      <StepHead title={<CodeText text={t('migrate.s2.title', { name: codeMark(w.name) })} />} lead={<CodeText text={t('migrate.s2.help', { name: codeMark(w.name) })} />} />
      <div className="wizard__body">
        <div className="portal-choices" role="radiogroup" aria-label={t('migrate.s2.pick')}>
          {portals.map((p) => {
            const state = pillState(p.session)
            const accountsOfPortal = (accounts?.profiles ?? []).filter((x) => x.kind === ProfileKind.ProfileKindSSO && x.portal === p.name).length
            return (
              <div key={p.name} className="portal-choice">
                <label className="portal-choice__head">
                  <Radio name="portal" checked={selected === p.name} onChange={() => choose(p.name)} />
                  <strong>{p.name}</strong>
                  {matches.has(p.name) && <Badge kind="ok">{t('migrate.s3.match')}</Badge>}
                  <span className="spacer" />
                  <span className="muted cluster" style={{ gap: 6 }}>
                    <Dot kind={state === 'active' ? 'ok' : state === 'signed-out' ? 'off' : state === 'signing-in' ? 'busy' : 'warn'} />
                    {pillText(p.session, now, t)}
                  </span>
                </label>
                <div className="portal-choice__meta">
                  <span className="mono">{p.startUrlHost}</span>
                  <span>{p.region}</span>
                  {accountsOfPortal > 0 && <span>{(accounts?.profiles ?? []).filter((x) => x.portal === p.name).map((x) => x.name).join(', ')}</span>}
                </div>
              </div>
            )
          })}
          <div className="portal-choice">
            <label className="portal-choice__head">
              <Radio name="portal" checked={isNew} onChange={() => choose(NEW_PORTAL)} />
              <strong>{t('migrate.s2.newPortal')}</strong>
            </label>
            {isNew && (
              <div className="portal-choice__form">
                <PortalFields form={form} />
              </div>
            )}
          </div>
        </div>
      </div>
      <WizardFooter
        w={w}
        primary={
          <Button variant="primary" loading={working || w.busy} onClick={() => void next()}>
            {primaryLabel}
          </Button>
        }
      />
    </>
  )
}
