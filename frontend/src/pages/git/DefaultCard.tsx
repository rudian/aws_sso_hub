import { useEffect, useMemo, useRef, useState } from 'react'
import { GitGlobal } from '../../api'
import { DefaultAction } from '../../api/models'
import type { DefaultChangePreview, DefaultChangeRequest, GitGlobalState } from '../../api/models'
import { AcctChip, Banner, Button, CodeBlock, DiffView, Modal, Radio } from '../../components'
import { useI18n } from '../../i18n'
import { useAccounts } from '../../state/accounts'
import { useActions } from '../../shell/AppActions'
import { ProfileSelect } from '../repos/ProfileSelect'
import { bindableProfiles } from '../repos/model'
import { useReportError, useUndoBackup } from '../repos/hooks'
import { codeCommitLines, effectiveDefaultAction, formatEntry, initialDefaultAction } from './model'

export interface DefaultCardProps {
  state: GitGlobalState
  reload: () => Promise<void>
  /** `open=gitDefault:remove`: select "remove" and open the preview. */
  autoRemove: boolean
  /** The deep link asked for removal but URL rules block it: point the user at the URL-rule card. */
  onBlockedRemove?: () => void
}

function Choice({ checked, disabled, onSelect, title, hint, children }: { checked: boolean; disabled?: boolean; onSelect: () => void; title: string; hint?: string; children?: React.ReactNode }) {
  return (
    <div
      className="card card--inset"
      style={{
        padding: 'var(--sp-3) var(--sp-4)',
        opacity: disabled ? 0.6 : 1,
        borderColor: checked ? 'var(--ink)' : undefined,
      }}
    >
      <label style={{ display: 'flex', gap: 10, alignItems: 'flex-start' }}>
        <Radio name="git-default" checked={checked} disabled={disabled} onChange={onSelect} style={{ marginTop: 3 }} />
        <span style={{ flex: 1 }}>
          <span style={{ fontWeight: 500, display: 'block' }}>{title}</span>
          {hint && <span className="hint" style={{ display: 'block' }}>{hint}</span>}
        </span>
      </label>
      {checked && children && <div style={{ marginTop: 'var(--sp-3)', marginLeft: 26 }}>{children}</div>}
    </div>
  )
}

/** Card 1: what the global CodeCommit default is, and keep / remove / replace it. */
export function DefaultCard({ state, reload, autoRemove, onBlockedRemove }: DefaultCardProps) {
  const { t } = useI18n()
  const { state: accounts } = useAccounts()
  const { toast } = useActions()
  const report = useReportError()
  const undo = useUndoBackup(reload)
  const def = state.globalDefault
  const urlRulesBlock = (state.urlRules ?? []).length > 0
  const choices = useMemo(() => bindableProfiles(accounts?.profiles ?? []), [accounts])

  const [chosen, setAction] = useState<DefaultAction>(() => initialDefaultAction({ wantRemove: autoRemove, hasDefault: !!def, urlRulesBlock }))
  // A choice that became unavailable (for example a URL rule appeared) never stays selected.
  const action = effectiveDefaultAction(chosen, { hasDefault: !!def, urlRulesBlock })
  const [profile, setProfile] = useState('')
  const [busy, setBusy] = useState(false)
  const [review, setReview] = useState<{ req: DefaultChangeRequest; data: DefaultChangePreview } | null>(null)

  const req: DefaultChangeRequest = action === DefaultAction.DefaultReplace ? { action, profile } : { action }
  const canApply = action !== DefaultAction.DefaultKeep && (action !== DefaultAction.DefaultReplace || profile !== '')

  const preview = async (r: DefaultChangeRequest) => {
    setBusy(true)
    try {
      setReview({ req: r, data: await GitGlobal.PreviewDefaultChange(r) })
    } catch (e) {
      report(e)
    } finally {
      setBusy(false)
    }
  }

  const autoDone = useRef(false)
  useEffect(() => {
    if (!autoRemove || autoDone.current) return
    autoDone.current = true
    if (def && !urlRulesBlock) {
      setAction(DefaultAction.DefaultRemove)
      void preview({ action: DefaultAction.DefaultRemove })
    } else if (def && urlRulesBlock) {
      onBlockedRemove?.()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [autoRemove])

  const apply = async () => {
    if (!review) return
    setBusy(true)
    try {
      const res = await GitGlobal.ApplyDefaultChange(review.req, review.data.preview.baseHash)
      setReview(null)
      setAction(DefaultAction.DefaultKeep)
      toast({
        kind: 'ok',
        title: t('git.default.done'),
        action: res.backupId ? { label: t('common.undo'), onClick: () => void undo(res.backupId) } : undefined,
      })
      await reload()
    } catch (e) {
      report(e)
    } finally {
      setBusy(false)
    }
  }

  const lines = codeCommitLines(state)

  return (
    <section className="card" aria-labelledby="git-default-title">
      <div className="card__header">
        <h2 className="card__title" id="git-default-title">
          {t('git.default.title')}
        </h2>
      </div>
      <div className="card__body stack" style={{ '--gap': 'var(--sp-4)' } as React.CSSProperties}>
        {def ? (
          <div className="cluster" style={{ gap: 'var(--sp-3)' }}>
            <span>{t('git.default.explain', { name: def.profile })}</span>
            <AcctChip slot={def.colorSlot} monogram={def.monogram} name={def.profile} />
          </div>
        ) : (
          <div>{state.guard.installed ? t('git.default.guard') : t('git.default.none')}</div>
        )}

        <CodeBlock label={t('git.default.lines')} copyable={false} code={lines.length > 0 ? lines.map(formatEntry).join('\n') : t('git.default.noLines')} />

        <div className="stack" role="radiogroup" aria-label={t('git.default.title')} style={{ '--gap': 'var(--sp-2)' } as React.CSSProperties}>
          <Choice checked={action === DefaultAction.DefaultKeep} onSelect={() => setAction(DefaultAction.DefaultKeep)} title={t('git.default.keep')} hint={t('git.default.keep.hint')} />
          {def && (
            <Choice
              checked={action === DefaultAction.DefaultRemove}
              disabled={urlRulesBlock}
              onSelect={() => setAction(DefaultAction.DefaultRemove)}
              title={t('git.default.remove')}
              hint={urlRulesBlock ? t('git.default.urlRulesFirst') : t('git.default.remove.hint')}
            />
          )}
          <Choice checked={action === DefaultAction.DefaultReplace} onSelect={() => setAction(DefaultAction.DefaultReplace)} title={t('git.default.change')} hint={t('git.default.change.hint')}>
            <ProfileSelect aria-label={t('git.default.change.profile')} profiles={choices} value={profile} onChange={setProfile} placeholder={t('git.default.change.choose')} />
          </Choice>
        </div>
      </div>
      <div className="card__footer">
        <Button variant="primary" loading={busy && !review} disabled={!canApply} onClick={() => void preview(req)}>
          {t('git.default.apply')}
        </Button>
      </div>

      {review && (
        <Modal
          open
          size="lg"
          title={t('git.default.confirm.title')}
          subtitle={t('git.default.confirm.body', { file: review.data.preview.file })}
          dismissible={!busy}
          onClose={() => setReview(null)}
          footer={
            <>
              <Button variant="secondary" disabled={busy} onClick={() => setReview(null)}>
                {t('common.cancel')}
              </Button>
              <Button variant="primary" loading={busy} onClick={() => void apply()}>
                {t('git.default.confirm.action')}
              </Button>
            </>
          }
        >
          <div className="stack" style={{ '--gap': 'var(--sp-4)' } as React.CSSProperties}>
            <DiffView preview={review.data.preview} />
            {(review.req.action === DefaultAction.DefaultRemove || review.data.unboundBlocked > 0) && (
              <Banner kind="warn">{t('git.default.impact', { bound: review.data.boundUnaffected, unbound: review.data.unboundBlocked })}</Banner>
            )}
          </div>
        </Modal>
      )}
    </section>
  )
}
