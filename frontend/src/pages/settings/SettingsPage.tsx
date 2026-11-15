import { ExternalLink, FolderOpen, FolderPlus, ScanSearch, X } from 'lucide-react'
import { useEffect, useMemo, useRef, useState } from 'react'
import type { FormEvent, ReactNode } from 'react'
import { Language, MotionPref, Theme } from '../../api/models'
import type { BackupEntry, Settings, SettingsPatch } from '../../api/models'
import { toUiError } from '../../api/errors'
import { onEvent } from '../../api/events'
import { openExternal } from '../../api/platform'
import { System } from '../../api/services'
import { Badge } from '../../components/Badge'
import { Button } from '../../components/Button'
import { ErrorBanner } from '../../components/ErrorBanner'
import { IconButton } from '../../components/IconButton'
import { Input } from '../../components/Input'
import { Segmented } from '../../components/Segmented'
import { Select } from '../../components/Select'
import { Skeleton } from '../../components/Skeleton'
import { Switch } from '../../components/Switch'
import { errorText, useI18n } from '../../i18n'
import { setOpen, takeOpen } from '../../lib/initialUI'
import { useRefreshHandler } from '../../lib/keyboard'
import { guessPlatform } from '../../lib/platform'
import { useNavigate } from '../../lib/route'
import { formatDateTime } from '../../lib/time'
import { useAsync } from '../../lib/useAsync'
import { useActions } from '../../shell/AppActions'
import { GlobalBanner } from '../../shell/GlobalBanner'
import { useAppInfo } from '../../state/appInfo'
import { useEnv } from '../../state/env'
import { useRepos } from '../../state/repos'
import { useSettings } from '../../state/settings'
import { TerminalAppSelect } from '../terminal/TerminalAppSelect'
import { RestoreModal } from './RestoreModal'
import { REPO_URL, backupReasonText, formatBytes, newestBackup, platformName } from './util'
import './settings.css'

function Row({ label, desc, children, stack, id }: { label: ReactNode; desc?: ReactNode; children?: ReactNode; stack?: boolean; id?: string }) {
  return (
    <div className={stack ? 'srow srow--stack' : 'srow'}>
      <div>
        <div className="srow__label" id={id}>
          {label}
        </div>
        {desc && <div className="srow__desc">{desc}</div>}
      </div>
      {children !== undefined && <div className="srow__control">{children}</div>}
    </div>
  )
}

function Card({ title, id, children }: { title: ReactNode; id: string; children: ReactNode }) {
  return (
    <section className="card" aria-labelledby={id}>
      <div className="card__header">
        <h2 className="card__title" id={id}>
          {title}
        </h2>
      </div>
      {children}
    </section>
  )
}

/** One tool-path override: edit, save, or go back to automatic detection. */
function ToolPath({ label, value, detected, onSave }: { label: string; value: string; detected?: string; onSave: (v: string) => Promise<void> }) {
  const { t } = useI18n()
  const [draft, setDraft] = useState(value)
  const [saving, setSaving] = useState(false)
  useEffect(() => setDraft(value), [value])
  const dirty = draft.trim() !== value
  const submit = async (e?: FormEvent, next = draft.trim()) => {
    e?.preventDefault()
    setSaving(true)
    try {
      await onSave(next)
    } finally {
      setSaving(false)
    }
  }
  return (
    <form className="paths__row" onSubmit={(e) => void submit(e)}>
      <label className="paths__label" htmlFor={`path-${label}`}>
        {label}
      </label>
      <Input id={`path-${label}`} mono value={draft} placeholder={detected ?? ''} spellCheck={false} onChange={(e) => setDraft(e.target.value)} />
      <Button type="submit" size="sm" disabled={!dirty} loading={saving}>
        {t('settings.paths.save')}
      </Button>
      <Button size="sm" variant="ghost" disabled={value === '' && draft === ''} onClick={() => void submit(undefined, '')}>
        {t('settings.paths.auto')}
      </Button>
      <div className="paths__detected">{detected ? t('settings.paths.detected', { path: detected }) : t('settings.paths.notFound')}</div>
    </form>
  )
}

export function SettingsPage() {
  const { t, lang } = useI18n()
  const info = useAppInfo()
  const { settings, update } = useSettings()
  const repos = useRepos()
  const env = useEnv()
  const navigate = useNavigate()
  const { toast } = useActions()
  const platform = info?.platform ?? guessPlatform()
  const backups = useAsync(() => System.ListBackups(), [])
  const [restoreId, setRestoreId] = useState<string | null>(null)

  const fail = (e: unknown, title?: string) => {
    const text = errorText(toUiError(e), t)
    toast({ kind: 'danger', title: title ?? text.title, body: title ? text.title : text.body })
  }
  const save = async (p: SettingsPatch) => {
    try {
      await update(p)
    } catch (e) {
      fail(e, t('settings.saveFailed'))
    }
  }

  useRefreshHandler(() => void backups.reload())
  useEffect(() => {
    const offs = [onEvent('accounts:changed', () => void backups.reload()), onEvent('repos:changed', () => void backups.reload())]
    return () => offs.forEach((off) => off())
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // ?open=restore: the restore dialog for the newest backup, once the list is there.
  const openedRestore = useRef(false)
  useEffect(() => {
    if (openedRestore.current || !backups.data) return
    openedRestore.current = true
    if (takeOpen('restore') !== null) {
      const newest = newestBackup(backups.data)
      if (newest) setRestoreId(newest.id)
    }
  }, [backups.data])

  const display = useMemo(() => {
    const m = new Map<string, string>()
    const roots = repos.state?.roots ?? []
    const shown = repos.state?.rootsDisplay ?? []
    roots.forEach((r, i) => m.set(r, shown[i] ?? r))
    return (root: string) => m.get(root) ?? root
  }, [repos.state?.roots, repos.state?.rootsDisplay])

  if (!settings) {
    return (
      <div className="page settings">
        <header className="page__header">
          <h1 className="page__title">{t('settings.title')}</h1>
        </header>
        <GlobalBanner />
        <div className="card" aria-busy="true">
          <div className="card__body">
            <Skeleton variant="text" width="60%" />
          </div>
        </div>
      </div>
    )
  }

  const s: Settings = settings
  const roots = s.scanRoots ?? []

  const addFolder = async () => {
    try {
      const dir = await System.ChooseFolder(t('settings.folders.choose'))
      if (!dir || roots.includes(dir)) return
      await save({ scanRoots: [...roots, dir] })
    } catch (e) {
      fail(e)
    }
  }

  const goTransfer = (which: 'export' | 'import') => {
    setOpen(which)
    navigate({ page: 'accounts' })
  }

  const entries: BackupEntry[] = backups.data ?? []
  const retention = String(s.backupRetention)

  return (
    <div className="page settings">
      <header className="page__header">
        <div>
          <h1 className="page__title">{t('settings.title')}</h1>
        </div>
      </header>
      <GlobalBanner />
      <div className="page__body">
        <Card id="set-general" title={t('settings.group.general')}>
          <Row label={t('settings.language')} desc={t('settings.language.hint')}>
            <Segmented<Language>
              role="radiogroup"
              ariaLabel={t('settings.language')}
              value={s.language}
              onChange={(v) => void save({ language: v })}
              items={[
                { value: Language.LanguageZhCN, label: t('settings.language.zh') },
                { value: Language.LanguageEn, label: t('settings.language.en') },
                { value: Language.LanguageSystem, label: t('settings.language.system') },
              ]}
            />
          </Row>
          <Row label={t('settings.theme')}>
            <Segmented<Theme>
              role="radiogroup"
              ariaLabel={t('settings.theme')}
              value={s.theme}
              onChange={(v) => void save({ theme: v })}
              items={[
                { value: Theme.ThemeSystem, label: t('settings.theme.system') },
                { value: Theme.ThemeLight, label: t('settings.theme.light') },
                { value: Theme.ThemeDark, label: t('settings.theme.dark') },
              ]}
            />
          </Row>
          <Row label={t('settings.motion')} desc={t('settings.motion.hint')}>
            <Segmented<MotionPref>
              role="radiogroup"
              ariaLabel={t('settings.motion')}
              value={s.motion}
              onChange={(v) => void save({ motion: v })}
              items={[
                { value: MotionPref.MotionSystem, label: t('settings.motion.system') },
                { value: MotionPref.MotionFull, label: t('settings.motion.full') },
                { value: MotionPref.MotionReduce, label: t('settings.motion.reduce') },
              ]}
            />
          </Row>
        </Card>

        <Card id="set-repos" title={t('settings.group.repos')}>
          <Row stack label={t('settings.folders')} desc={t('settings.folders.hint')}>
            {roots.length === 0 ? (
              <div className="folders__empty" style={{ width: '100%' }}>
                {t('settings.folders.empty')}
              </div>
            ) : (
              <ul className="folders" style={{ width: '100%' }}>
                {roots.map((r) => (
                  <li key={r}>
                    <span className="folders__path" title={r}>
                      {display(r)}
                    </span>
                    <IconButton label={roots.length === 1 ? t('settings.folders.last') : t('settings.folders.remove', { folder: display(r) })} icon={X} size="sm" disabled={roots.length === 1} onClick={() => void save({ scanRoots: roots.filter((x) => x !== r) })} />
                  </li>
                ))}
              </ul>
            )}
            <div className="cluster" style={{ marginTop: 'var(--sp-3)' }}>
              <Button icon={FolderPlus} onClick={() => void addFolder()}>
                {t('settings.folders.add')}
              </Button>
              <Button icon={ScanSearch} loading={!!repos.scanning} onClick={() => void repos.scan().catch((e) => fail(e))}>
                {t('settings.folders.scanNow')}
              </Button>
            </div>
          </Row>
          <Row label={t('settings.repos.autoBindOnAdd')} desc={t('settings.repos.autoBindOnAdd.hint')}>
            <Switch label={t('settings.repos.autoBindOnAdd')} checked={s.autoBindOnAdd} onChange={(v) => void save({ autoBindOnAdd: v })} />
          </Row>
        </Card>

        <Card id="set-transfer" title={t('settings.transfer.title')}>
          <Row label={t('settings.transfer.body')}>
            <Button onClick={() => goTransfer('export')}>{t('settings.transfer.export')}</Button>
            <Button onClick={() => goTransfer('import')}>{t('settings.transfer.import')}</Button>
          </Row>
        </Card>

        <Card id="set-signin" title={t('settings.group.signin')}>
          <Row label={t('settings.signin.private')} desc={t('settings.signin.private.hint')}>
            <Switch label={t('settings.signin.private')} checked={s.privateSignIn} onChange={(v) => void save({ privateSignIn: v })} />
          </Row>
        </Card>

        <Card id="set-reminders" title={t('settings.reminders')}>
          <Row label={t('settings.reminders.expiry')}>
            <Switch label={t('settings.reminders.expiry')} checked={s.reminders.expiryNotice} onChange={(v) => void save({ reminders: { ...s.reminders, expiryNotice: v } })} />
          </Row>
          <Row label={t('settings.reminders.lead')}>
            <Select
              aria-label={t('settings.reminders.lead')}
              value={String(s.reminders.leadMinutes)}
              disabled={!s.reminders.expiryNotice}
              onChange={(e) => void save({ reminders: { ...s.reminders, leadMinutes: Number(e.target.value) } })}
            >
              <option value="15">{t('settings.reminders.lead.option', { n: 15 })}</option>
              <option value="30">{t('settings.reminders.lead.option', { n: 30 })}</option>
              <option value="60">{t('settings.reminders.lead.hour')}</option>
            </Select>
          </Row>
          <Row label={t('settings.reminders.autoRenew')} desc={t('settings.reminders.autoRenew.hint')}>
            <Switch label={t('settings.reminders.autoRenew')} checked={s.reminders.autoRenew} onChange={(v) => void save({ reminders: { ...s.reminders, autoRenew: v } })} />
          </Row>
        </Card>

        <Card id="set-tray" title={t('settings.group.tray')}>
          <Row label={platform === 'windows' ? t('settings.tray.win') : t('settings.tray')}>
            <Switch label={platform === 'windows' ? t('settings.tray.win') : t('settings.tray')} checked={s.tray.enabled} onChange={(v) => void save({ tray: { ...s.tray, enabled: v } })} />
          </Row>
          <Row label={t('settings.tray.background')} desc={s.tray.enabled ? undefined : t('settings.tray.background.hint')}>
            <Switch label={t('settings.tray.background')} checked={s.tray.keepRunning} disabled={!s.tray.enabled} onChange={(v) => void save({ tray: { ...s.tray, keepRunning: v } })} />
          </Row>
          <Row label={t('term.app')}>
            <TerminalAppSelect platform={platform} value={s.terminalApp} onChange={(v) => void save({ terminalApp: v })} />
          </Row>
        </Card>

        <Card id="set-paths" title={t('settings.paths')}>
          <Row stack label={t('settings.paths.hint')}>
            <div className="paths" style={{ width: '100%' }}>
              <ToolPath
                label={t('settings.paths.aws')}
                value={s.awsCliPath}
                detected={env.report?.awsCli.path}
                onSave={async (v) => {
                  await save({ awsCliPath: v })
                  await env.recheck()
                }}
              />
              <ToolPath
                label={t('settings.paths.git')}
                value={s.gitPath}
                detected={env.report?.git.path}
                onSave={async (v) => {
                  await save({ gitPath: v })
                  await env.recheck()
                }}
              />
            </div>
          </Row>
        </Card>

        <Card id="set-backups" title={t('settings.backups')}>
          <div className="srow srow--stack" style={{ paddingBottom: 'var(--sp-3)', borderBottom: 0 }}>
            <div className="srow__desc" style={{ marginTop: 0 }}>
              {t('settings.backups.hint')}
            </div>
          </div>
          {backups.error && !backups.data ? (
            <div className="card__body">
              <ErrorBanner error={backups.error} onRetry={() => void backups.reload()} />
            </div>
          ) : !backups.data ? (
            <div className="card__body" aria-busy="true" aria-label={t('settings.backups.loading')}>
              <Skeleton variant="text" width="70%" />
            </div>
          ) : entries.length === 0 ? (
            <div className="folders__empty" style={{ margin: '0 var(--sp-5) var(--sp-4)' }}>
              {t('settings.backups.empty')}
            </div>
          ) : (
            <div className="table btable" role="table" aria-label={t('settings.backups')}>
              <div className="table__head" role="row">
                <span role="columnheader">{t('settings.backups.time')}</span>
                <span role="columnheader">{t('settings.backups.file')}</span>
                <span role="columnheader" className="btable__size">
                  {t('settings.backups.size')}
                </span>
                <span role="columnheader">{t('settings.backups.reason')}</span>
                <span role="columnheader" aria-label={t('settings.backups.restore')} />
              </div>
              {[...entries]
                .sort((a, b) => Date.parse(b.createdAt) - Date.parse(a.createdAt))
                .map((b) => (
                  <div className="row" role="row" key={b.id}>
                    <span className="btable__time" role="cell">
                      {formatDateTime(b.createdAt, lang)}
                    </span>
                    <span className="btable__file" role="cell" title={b.file}>
                      {b.file}
                    </span>
                    <span role="cell" className="row__secondary btable__size">
                      {formatBytes(b.size)}
                    </span>
                    <span role="cell" className="btable__reason">
                      <span>{backupReasonText(b, t)}</span>
                      {b.redacted && <Badge kind="neutral">{t('settings.backups.redacted')}</Badge>}
                    </span>
                    <span role="cell" className="btable__actions">
                      <Button size="sm" variant="ghost" onClick={() => void System.RevealBackup(b.id).catch((e) => fail(e))}>
                        {t('settings.backups.show')}
                      </Button>
                      <Button size="sm" onClick={() => setRestoreId(b.id)}>
                        {t('settings.backups.restore')}
                      </Button>
                    </span>
                  </div>
                ))}
            </div>
          )}
          <div className="bfoot">
            <span>{t('settings.backups.location')}</span>
            <span className="bfoot__path">{info?.backupDir ?? ''}</span>
            <Button size="sm" icon={FolderOpen} onClick={() => void System.OpenBackupFolder().catch((e) => fail(e))}>
              {t('settings.backups.folder')}
            </Button>
            <label className="bfoot__keep">
              <span>{t('settings.backups.keep')}</span>
              <Select value={retention} onChange={(e) => void save({ backupRetention: Number(e.target.value) })}>
                <option value="20">{t('settings.backups.keep.n', { n: 20 })}</option>
                <option value="50">{t('settings.backups.keep.n', { n: 50 })}</option>
                <option value="0">{t('settings.backups.keep.all')}</option>
              </Select>
            </label>
          </div>
        </Card>

        <Card id="set-about" title={t('settings.about')}>
          <div className="about">
            <div className="about__v about__name" style={{ gridColumn: '1 / -1' }}>
              {t('app.name')}
              <Badge kind="outline">{t('settings.about.version', { version: info?.version ?? '' })}</Badge>
              {info?.sandbox && <Badge kind="warn">{t('app.sandbox')}</Badge>}
            </div>
            <span className="about__k">{t('settings.about.platform')}</span>
            <span className="about__v">{info ? `${platformName(info.platform)} · ${info.arch}` : '—'}</span>
            <span className="about__k">{t('settings.about.dataDir')}</span>
            <span className="about__v mono">{info?.dataDir ?? '—'}</span>
            <span className="about__k">{t('settings.about.awsCli')}</span>
            <span className="about__v">
              {env.report?.awsCli.version ? (
                <>
                  <span>{env.report.awsCli.version}</span>
                  {env.report.awsCli.path && <span className="mono">{env.report.awsCli.path}</span>}
                </>
              ) : (
                t('settings.about.awsCli.missing')
              )}
            </span>
            <span className="about__k">{t('settings.about.license')}</span>
            <span className="about__v">{t('settings.about.license.name')}</span>
            <span className="about__k">{t('settings.about.repo')}</span>
            <span className="about__v">
              <Button size="sm" icon={ExternalLink} onClick={() => void openExternal(REPO_URL).catch((e) => fail(e))}>
                {t('settings.about.repo.open')}
              </Button>
              <span className="mono">{REPO_URL.replace('https://', '')}</span>
            </span>
            <span className="about__k">{t('settings.about.tutorial')}</span>
            <span className="about__v">
              <Button size="sm" onClick={() => navigate({ page: 'tutorial' })}>
                {t('settings.about.tutorial.open')}
              </Button>
            </span>
            <p className="about__disclaimer">{t('settings.about.disclaimer')}</p>
          </div>
        </Card>
      </div>

      {restoreId && (
        <RestoreModal
          backupId={restoreId}
          onClose={(restored) => {
            setRestoreId(null)
            if (restored) void backups.reload()
          }}
        />
      )}
    </div>
  )
}
