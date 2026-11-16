import { ToolStatus } from '../../api/models'
import type { EnvReport, FileCheck, ToolInfo } from '../../api/models'
import { Button } from '../../components/Button'
import { Skeleton } from '../../components/Skeleton'
import { useI18n } from '../../i18n'
import type { T } from '../../i18n'
import { useNavigate } from '../../lib/route'
import { GIT_MIN_VERSION } from '../../lib/toolVersions'
import { useEnv } from '../../state/env'
import { StatusRow } from './StatusRow'
import type { RowStatus } from './StatusRow'
import { versionLt } from './version'

interface Described {
  status: RowStatus
  text: string
  meta?: string
  install?: boolean
}

export function describeCli(c: ToolInfo, t: T): Described {
  const min = c.min ?? '2.22.0'
  const version = c.version ?? ''
  switch (c.status) {
    case ToolStatus.ToolOK: {
      const recommend = c.recommended && version && versionLt(version, c.recommended) ? t('health.cli.recommend', { version: c.recommended }) : undefined
      return { status: recommend ? 'info' : 'ok', text: t('health.cli.ok', { version }), meta: [c.path, recommend].filter(Boolean).join(' · ') || undefined }
    }
    case ToolStatus.ToolOutdated:
      return { status: 'warn', text: t('health.cli.outdated', { version, min }), meta: c.path, install: true }
    case ToolStatus.ToolTooOld:
      return { status: 'danger', text: t('health.cli.old', { version, min }), meta: c.path, install: true }
    case ToolStatus.ToolV1:
      return { status: 'danger', text: t('health.cli.v1'), meta: c.path, install: true }
    case ToolStatus.ToolMissing:
      return { status: 'danger', text: t('health.cli.missing'), install: true }
    default:
      return { status: 'warn', text: t('health.cli.error'), meta: c.path, install: true }
  }
}

export function describeGit(g: ToolInfo, t: T): Described {
  const version = g.version ?? ''
  switch (g.status) {
    case ToolStatus.ToolOK:
    case ToolStatus.ToolOutdated:
      return { status: 'ok', text: t('health.git.ok', { version }), meta: g.path }
    case ToolStatus.ToolTooOld:
      return { status: 'danger', text: t('health.git.old', { version, min: g.min ?? GIT_MIN_VERSION }), meta: g.path }
    case ToolStatus.ToolMissing:
      return { status: 'danger', text: t('health.git.missing') }
    default:
      return { status: 'warn', text: t('health.git.error'), meta: g.path }
  }
}

export function describeFile(f: FileCheck, t: T): Described {
  if (f.issue) return { status: 'danger', text: t('error.configParse', { file: f.issue.file || f.file, line: f.issue.line }), meta: f.issue.message }
  if (!f.exists) return { status: 'info', text: t('health.config.missing') }
  if (!f.readable) return { status: 'danger', text: t('health.config.unreadable') }
  if (!f.writable) return { status: 'warn', text: t('health.config.readOnly') }
  return { status: 'ok', text: t('health.config.ok') }
}

function InstallButton() {
  const { t } = useI18n()
  const navigate = useNavigate()
  return (
    <Button size="sm" onClick={() => navigate({ page: 'tutorial', section: 'faq' })}>
      {t('health.cli.install')}
    </Button>
  )
}

/** The environment rows shared by the Health page and the first-run screen. */
export function EnvChecks({ report, compact, hideTools }: { report?: EnvReport | null; compact?: boolean; /** Health draws the AWS CLI and Git rows with ToolRows. */ hideTools?: boolean }) {
  const { t } = useI18n()
  const env = useEnv()
  const r = report === undefined ? env.report : report
  if (!r) {
    return (
      <div className="list" aria-busy="true">
        {[0, 1, 2].map((i) => (
          <div key={i} className="list__item">
            <Skeleton variant="text" width="60%" />
          </div>
        ))}
      </div>
    )
  }
  const cli = describeCli(r.awsCli, t)
  const git = describeGit(r.git, t)
  const grcOk = r.grc.status === ToolStatus.ToolOK
  return (
    <div className="list">
      {!hideTools && (
        <>
          <StatusRow compact={compact} status={cli.status} title={t('health.cli.title')} text={cli.text} meta={cli.meta && <span className="mono">{cli.meta}</span>} action={cli.install ? <InstallButton /> : undefined} />
          <StatusRow compact={compact} status={git.status} title={t('health.git.title')} text={git.text} meta={git.meta && <span className="mono">{git.meta}</span>} />
        </>
      )}
      <StatusRow
        compact={compact}
        status={grcOk ? 'ok' : 'info'}
        title={t('health.grc.title')}
        text={grcOk ? t('health.grc') : t('health.grc.missing')}
        meta={grcOk && r.grc.path ? <span className="mono">{r.grc.path}</span> : undefined}
      />
      {(r.files ?? []).map((f) => {
        const d = describeFile(f, t)
        return <StatusRow key={f.kind} compact={compact} status={d.status} title={<span className="mono">{f.file}</span>} text={d.text} meta={d.meta} />
      })}
    </div>
  )
}
