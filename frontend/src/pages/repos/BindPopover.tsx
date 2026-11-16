import { useEffect, useMemo, useRef, useState } from 'react'
import type { RefObject } from 'react'
import { BindWarning } from '../../api/models'
import type { BindOutcome, BindPreview } from '../../api/models'
import type { Repo } from '../../api/models'
import { Repos } from '../../api'
import { Banner, Button, CodeBlock, ErrorBanner, Popover, Skeleton } from '../../components'
import { useI18n } from '../../i18n'
import { useAsync } from '../../lib/useAsync'
import { useAccounts } from '../../state/accounts'
import { ProfileSelect } from './ProfileSelect'
import { bindableProfiles, portalOf } from './model'
import { useReportError } from './hooks'
import { useRepoSentence } from './RepoSentence'

export interface BindPopoverProps {
  repo: Repo
  anchorRef: RefObject<HTMLElement | null>
  /** Profiles the user may pick from. One entry: no chooser is shown. */
  options: string[]
  /** Preselected profile ('' = the user has to choose). */
  initial: string
  onClose: () => void
  onBound: (outcome: BindOutcome, profile: string) => void
}

/** The confirm popover of a single bind: which profile, what gets written, warnings. Binding happens only on the primary button. */
export function BindPopover({ repo, anchorRef, options, initial, onClose, onBound }: BindPopoverProps) {
  const { t } = useI18n()
  const sentence = useRepoSentence()
  const { state: accounts } = useAccounts()
  const report = useReportError()
  const [profile, setProfile] = useState(initial)
  const [busy, setBusy] = useState(false)
  const bodyRef = useRef<HTMLDivElement>(null)

  // Keyboard users arrive inside the popover and leave back at the control that opened it.
  useEffect(() => {
    bodyRef.current?.querySelector<HTMLElement>('select, button:not(:disabled)')?.focus()
  }, [])
  const giveFocusBack = () => {
    const a = anchorRef.current
    if (a?.isConnected) a.focus()
  }
  const cancel = () => {
    onClose()
    giveFocusBack()
  }

  const profiles = accounts?.profiles ?? []
  const portals = accounts?.portals ?? []
  const choices = useMemo(() => bindableProfiles(profiles).filter((p) => options.includes(p.name)), [profiles, options])
  const selected = profiles.find((p) => p.name === profile)

  const preview = useAsync<BindPreview | null>(() => (profile ? Repos.PreviewBind(repo.id, profile) : Promise.resolve(null)), [repo.id, profile])
  const data = preview.data
  const warnings = profile && data?.profile === profile ? (data.warnings ?? []) : []

  const warnText = (w: BindWarning): string | null => {
    switch (w) {
      case BindWarning.BindWarnReplaces:
        return t('repos.bind.replaceWarn')
      case BindWarning.BindWarnStatic:
        return t('repos.bind.staticWarn', { name: profile })
      case BindWarning.BindWarnSignedOut:
        return t('repos.bind.signedOutWarn', { portal: portalOf(selected, portals)?.name ?? selected?.portal ?? profile })
      case BindWarning.BindWarnOwnerMismatch:
        return t('repos.bind.mismatchWarn')
      default:
        return null
    }
  }

  const submit = async () => {
    if (!profile) return
    setBusy(true)
    try {
      const out = await Repos.Bind(repo.id, profile)
      onBound(out, profile)
      giveFocusBack()
    } catch (e) {
      report(e)
    } finally {
      setBusy(false)
    }
  }

  return (
    <Popover anchorRef={anchorRef} open onClose={onClose} align="end" ariaLabel={sentence.plain('repos.bind.confirm', repo, { name: profile || '…' })} className="bind-popover">
      <div ref={bodyRef} className="stack" style={{ '--gap': 'var(--sp-3)' } as React.CSSProperties}>
        <div style={{ fontWeight: 600 }}>{sentence.node('repos.bind.confirm', repo, { name: profile || '…' })}</div>
        <div className="mono muted" style={{ fontSize: 'var(--fs-12)', wordBreak: 'break-all' }}>
          {repo.displayPath}
        </div>
        {choices.length > 1 && <ProfileSelect aria-label={t('repos.bind.profile')} profiles={choices} value={profile} onChange={setProfile} placeholder={t('repos.bind.choose')} />}
        {profile && preview.loading && !data && <Skeleton variant="text" width="100%" />}
        {preview.error && <ErrorBanner error={preview.error} onRetry={() => void preview.reload()} />}
        {warnings.map((w) => {
          const text = warnText(w)
          return text ? (
            <Banner key={w} kind="warn">
              {text}
            </Banner>
          ) : null
        })}
        {data && data.profile === profile && (data.commands ?? []).length > 0 && (
          <details>
            <summary style={{ cursor: 'default', fontSize: 'var(--fs-12)', color: 'var(--text-2)' }}>{t('repos.bind.showConfig')}</summary>
            <div style={{ marginTop: 6 }}>
              <CodeBlock copyable={false} code={(data.commands ?? []).join('\n')} />
            </div>
          </details>
        )}
        <div className="cluster" style={{ justifyContent: 'flex-end' }}>
          <Button size="sm" variant="secondary" onClick={cancel}>
            {t('common.cancel')}
          </Button>
          <Button size="sm" variant="primary" loading={busy} disabled={!profile || preview.loading} onClick={() => void submit()}>
            {t('repos.bind.action')}
          </Button>
        </div>
      </div>
    </Popover>
  )
}
