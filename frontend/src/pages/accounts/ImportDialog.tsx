import { FolderOpen } from 'lucide-react'
import { useEffect, useMemo, useRef, useState } from 'react'
import { isCancel, toUiError } from '../../api/errors'
import type { UiError } from '../../api/errors'
import { Accounts } from '../../api/services'
import { ErrorCode, ImportAction, ImportItemKind } from '../../api/models'
import type { ImportItem, ImportPreview, ImportRequest } from '../../api/models'
import { Badge } from '../../components/Badge'
import { Banner } from '../../components/Banner'
import { Button } from '../../components/Button'
import { Checkbox } from '../../components/Checkbox'
import { DiffView } from '../../components/DiffView'
import { ErrorBanner } from '../../components/ErrorBanner'
import { Input } from '../../components/Input'
import { Modal } from '../../components/Modal'
import { Segmented } from '../../components/Segmented'
import { Switch } from '../../components/Switch'
import { useI18n } from '../../i18n'
import type { MessageKey, T } from '../../i18n'
import { useActions } from '../../shell/AppActions'
import { scrollToPortal } from './util'
import './accounts.css'

type Choice = 'skip' | 'replace' | 'rename'

/** Text for a warning code such as "unknown-key:foo". */
export function warningText(code: string, t: T): string {
  const i = code.indexOf(':')
  const kind = i < 0 ? code : code.slice(0, i)
  const detail = i < 0 ? '' : code.slice(i + 1)
  if (kind === 'unknown-key') return t('import.warn.unknown-key', { key: detail })
  if (kind === 'portal-differs') return t('import.warn.portal-differs', { portal: detail })
  return code
}

function rejectedText(item: ImportItem, t: T): string {
  const key = (item.reasonKey ?? 'import.rejected.invalid') as MessageKey
  return t(key, { detail: item.detail ?? '', portal: item.detail ?? '' })
}

const ACTION_BADGE: Record<string, 'ok' | 'info' | 'warn' | 'danger' | 'neutral'> = {
  [ImportAction.ImportActionNew]: 'ok',
  [ImportAction.ImportActionSame]: 'neutral',
  [ImportAction.ImportActionConflict]: 'warn',
  [ImportAction.ImportActionMapped]: 'info',
  [ImportAction.ImportActionRejected]: 'danger',
}

/** Import accounts from JSON: choose a file or paste, preview with the config diff, decide per conflict, apply. */
export function ImportDialog({ autoSample, onClose }: { autoSample?: boolean; onClose: () => void }) {
  const { t } = useI18n()
  const { toast } = useActions()
  const [text, setText] = useState('')
  const [fileName, setFileName] = useState('')
  const [include, setInclude] = useState<Record<string, boolean>>({})
  const [choices, setChoices] = useState<Record<string, Choice>>({})
  const [renames, setRenames] = useState<Record<string, string>>({})
  const [extras, setExtras] = useState(true)
  const [preview, setPreview] = useState<ImportPreview | null>(null)
  const [error, setError] = useState<UiError | null>(null)
  const [busy, setBusy] = useState<'file' | 'preview' | 'apply' | null>(null)
  const seq = useRef(0)

  const request = (json: string, inc = include, ch = choices, rn = renames, ex = extras): ImportRequest => ({ json, include: inc, choices: ch, renames: rn, importExtras: ex })

  const runPreview = async (json: string, req: ImportRequest, first = false) => {
    const my = ++seq.current
    if (first) setBusy('preview')
    setError(null)
    try {
      const p = await Accounts.PreviewImport(req)
      if (my !== seq.current) return
      setPreview(p)
    } catch (e) {
      if (my !== seq.current) return
      setError(toUiError(e))
      if (first) setPreview(null)
    } finally {
      if (my === seq.current) setBusy(null)
    }
    void json
  }

  const chooseFile = async () => {
    setBusy('file')
    setError(null)
    try {
      const src = await Accounts.OpenImportFile()
      setText(src.json)
      setFileName(src.name)
      await runPreview(src.json, request(src.json), true)
    } catch (e) {
      if (!isCancel(e) && toUiError(e).code !== ErrorCode.ErrCancelled) setError(toUiError(e))
      setBusy(null)
    }
  }

  const started = useRef(false)
  useEffect(() => {
    if (autoSample && !started.current) {
      started.current = true
      void chooseFile()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // Any decision re-plans the import so the item list and the diff stay in step.
  const change = (patch: { include?: Record<string, boolean>; choices?: Record<string, Choice>; renames?: Record<string, string>; extras?: boolean }) => {
    const inc = patch.include ?? include
    const ch = patch.choices ?? choices
    const rn = patch.renames ?? renames
    const ex = patch.extras ?? extras
    setInclude(inc)
    setChoices(ch)
    setRenames(rn)
    setExtras(ex)
    void runPreview(text, request(text, inc, ch, rn, ex))
  }

  const apply = async () => {
    if (!preview) return
    setBusy('apply')
    setError(null)
    try {
      const res = await Accounts.ApplyImport(request(text), preview.baseHash)
      toast({ kind: 'ok', title: t('import.done', { n: res.added + res.replaced + res.renamed }) })
      onClose()
      const first = res.needSignIn?.[0]
      if (first) scrollToPortal(first)
    } catch (e) {
      const ue = toUiError(e)
      setError(ue)
      setBusy(null)
      if (ue.code === ErrorCode.ErrConfigChanged) void runPreview(text, request(text), true)
    }
  }

  const items = preview?.items ?? []
  const toWrite = preview?.toWrite ?? 0
  const grouped = useMemo(() => [...items.filter((i) => i.kind === ImportItemKind.ImportKindPortal), ...items.filter((i) => i.kind === ImportItemKind.ImportKindProfile)], [items])

  return (
    <Modal
      open
      size="lg"
      title={t('import.title')}
      onClose={onClose}
      footer={
        preview ? (
          <>
            <Button variant="ghost" className="transfer__left" onClick={() => setPreview(null)}>
              {t('common.back')}
            </Button>
            <Button variant="secondary" onClick={onClose}>
              {t('common.cancel')}
            </Button>
            <Button variant="primary" loading={busy === 'apply'} disabled={toWrite === 0} onClick={() => void apply()}>
              {toWrite === 0 ? t('import.nothing') : t('import.apply', { n: toWrite })}
            </Button>
          </>
        ) : (
          <>
            <Button variant="secondary" onClick={onClose}>
              {t('common.cancel')}
            </Button>
            <Button variant="primary" loading={busy === 'preview'} disabled={!text.trim()} onClick={() => void runPreview(text, request(text), true)}>
              {t('import.preview')}
            </Button>
          </>
        )
      }
    >
      <div className="stack" style={{ ['--gap' as string]: 'var(--sp-4)' }}>
        {!preview && (
          <>
            <p className="muted" style={{ margin: 0 }}>
              {t('import.lead')}
            </p>
            <div className="cluster">
              <Button variant="secondary" icon={FolderOpen} loading={busy === 'file'} onClick={() => void chooseFile()}>
                {t('import.chooseFile')}
              </Button>
              {fileName && <span className="mono muted">{t('import.file', { name: fileName })}</span>}
            </div>
            <label className="field__label" htmlFor="import-json">
              {t('import.paste')}
            </label>
            <textarea
              id="import-json"
              className="textarea input--mono import-json"
              rows={10}
              spellCheck={false}
              value={text}
              onChange={(e) => {
                setText(e.target.value)
                setFileName('')
              }}
            />
          </>
        )}
        {error && <ErrorBanner error={error} />}
        {preview && (
          <>
            {preview.source && <div className="muted">{t('import.source', { source: preview.source })}</div>}
            <ul className="import-items" aria-label={t('import.items')}>
              {grouped.map((item) => (
                <ImportRow
                  key={item.key}
                  item={item}
                  checked={include[item.key] ?? item.include}
                  choice={(choices[item.key] ?? (item.choice as Choice)) || 'skip'}
                  rename={renames[item.key] ?? item.suggested}
                  onInclude={(on) => change({ include: { ...include, [item.key]: on } })}
                  onChoice={(c) => change({ choices: { ...choices, [item.key]: c } })}
                  onRename={(v) => setRenames({ ...renames, [item.key]: v })}
                  onRenameCommit={() => change({})}
                />
              ))}
            </ul>
            {preview.extrasCount > 0 && (
              <div className="transfer__switch-row">
                <label className="transfer__switch">
                  <Switch checked={extras} onChange={(on) => change({ extras: on })} label={t('import.includeExtras')} />
                  <span>{t('import.includeExtras')}</span>
                </label>
                <span className="muted">{t('import.extrasCount', { n: preview.extrasCount })}</span>
              </div>
            )}
            {toWrite === 0 && <Banner kind="info">{t('import.nothing')}</Banner>}
            {(preview.preview.lines?.length ?? 0) > 0 && <DiffView preview={preview.preview} />}
          </>
        )}
      </div>
    </Modal>
  )
}

function ImportRow(props: {
  item: ImportItem
  checked: boolean
  choice: Choice
  rename: string
  onInclude: (on: boolean) => void
  onChoice: (c: Choice) => void
  onRename: (v: string) => void
  onRenameCommit: () => void
}) {
  const { t } = useI18n()
  const { item } = props
  const rejected = item.action === ImportAction.ImportActionRejected
  const actionText =
    item.action === ImportAction.ImportActionMapped
      ? t('import.action.mapped', { portal: item.target })
      : t(`import.action.${item.action}` as MessageKey)
  const choices: Array<{ value: Choice; label: string }> = [
    { value: 'skip', label: t('import.choice.skip') },
    ...(item.canReplace ? [{ value: 'replace' as Choice, label: t('import.choice.replace') }] : []),
    { value: 'rename', label: t('import.choice.rename') },
  ]
  return (
    <li className={`import-row${rejected ? ' is-rejected' : ''}`}>
      <div className="import-row__main">
        <Checkbox
          checked={props.checked && !rejected}
          disabled={rejected}
          aria-label={item.name}
          onChange={(e) => props.onInclude(e.target.checked)}
          label={
            <span>
              <span className="muted">{item.kind === ImportItemKind.ImportKindPortal ? t('import.kind.portal') : t('import.kind.profile')}</span> <strong className="mono">{item.name}</strong>
            </span>
          }
        />
        <span className="spacer" />
        <Badge kind={ACTION_BADGE[item.action] ?? 'neutral'}>{actionText}</Badge>
      </div>
      {item.kind === ImportItemKind.ImportKindProfile && !rejected && (
        <div className="import-row__meta muted mono">
          {[item.accountId, item.role, item.portal].filter(Boolean).join(' · ')}
        </div>
      )}
      {item.kind === ImportItemKind.ImportKindPortal && !rejected && item.startUrl && <div className="import-row__meta muted mono">{[item.startUrl.replace(/^https:\/\//, ''), item.region].filter(Boolean).join(' · ')}</div>}
      {rejected && (
        <div className="import-row__reason" role="alert">
          {rejectedText(item, t)}
        </div>
      )}
      {item.action === ImportAction.ImportActionConflict && props.checked && (
        <div className="import-row__choice">
          <Segmented items={choices} value={props.choice} ariaLabel={t('import.choice.aria', { name: item.name })} onChange={props.onChoice} />
          {props.choice === 'rename' && (
            <Input
              mono
              value={props.rename}
              placeholder={t('import.rename.placeholder')}
              aria-label={t('import.choice.rename')}
              onChange={(e) => props.onRename(e.target.value)}
              onBlur={props.onRenameCommit}
            />
          )}
        </div>
      )}
      {(item.warnings ?? []).map((w) => (
        <div key={w} className="import-row__warn">
          {warningText(w, t)}
        </div>
      ))}
    </li>
  )
}
