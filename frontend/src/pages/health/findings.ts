import { HealthKind, Severity } from '../../api/models'
import type { ConfigIssue, HealthFinding } from '../../api/models'
import type { T } from '../../i18n'
import type { RowStatus } from './StatusRow'

export type FindingAction =
  | { kind: 'reveal'; file: string }
  | { kind: 'remove-key'; profile: string }
  | { kind: 'tutorial'; section: 'faq' | 'migrate' }
  | { kind: 'git' }
  | { kind: 'repos' }
  | { kind: 'open-config' }

export interface DescribedFinding {
  status: RowStatus
  title: string
  text?: string
  actionLabel?: string
  action?: FindingAction
}

export function severityStatus(s: Severity): RowStatus {
  switch (s) {
    case Severity.SeverityDanger:
      return 'danger'
    case Severity.SeverityWarn:
      return 'warn'
    case Severity.SeverityInfo:
      return 'info'
    default:
      return 'ok'
  }
}

/** Text and action for one finding of CheckHealth. Unknown kinds fall back to a neutral row with the kind as title. */
export function describeFinding(f: HealthFinding, t: T): DescribedFinding {
  const p = f.params ?? {}
  const status = severityStatus(f.severity)
  switch (f.kind) {
    case HealthKind.HealthShellExport:
      return {
        status,
        title: t('health.shell.found', { file: f.file ?? '', line: f.line ?? 0 }),
        text: t('health.shell.why'),
        actionLabel: t('health.action.reveal'),
        action: f.file ? { kind: 'reveal', file: f.file } : undefined,
      }
    case HealthKind.HealthEnvVar:
      return { status, title: t('health.envVar', { name: p.name ?? '' }), text: t('health.envVar.why') }
    case HealthKind.HealthLeftoverKeys:
      return {
        status,
        title: t('health.leftover', { name: f.profile ?? '' }),
        actionLabel: t('health.action.removeKey'),
        action: f.profile ? { kind: 'remove-key', profile: f.profile } : undefined,
      }
    case HealthKind.HealthDefaultKeys:
      return { status, title: t('health.defaultKeys'), text: t('health.defaultKeys.why'), actionLabel: t('health.action.help'), action: { kind: 'tutorial', section: 'faq' } }
    case HealthKind.HealthOldKey:
      return { status, title: t('health.oldKey', { name: f.profile ?? '', days: p.days ?? '' }) }
    case HealthKind.HealthGlobalStatic:
      return { status, title: t('health.globalStatic', { name: f.profile ?? '' }), actionLabel: t('health.action.gitGlobal'), action: { kind: 'git' } }
    case HealthKind.HealthURLUserinfo:
      return { status, title: t('health.urlUserinfo', { repo: p.repo ?? '' }), text: t('health.urlUserinfo.why'), actionLabel: t('health.action.repos'), action: { kind: 'repos' } }
    case HealthKind.HealthPendingKey:
      return {
        status,
        title: t('health.pending', { name: f.profile ?? '' }),
        text: p.maskedKeyId ? t('health.pending.detail', { maskedKeyId: p.maskedKeyId }) : undefined,
        actionLabel: t('health.action.pending'),
        action: { kind: 'tutorial', section: 'migrate' },
      }
    default:
      return { status, title: String(f.kind) }
  }
}

/**
 * The row for an AWS config file that cannot be parsed. The backend's health check does not report it (the
 * accounts state does), so the page adds it; it is what Accounts shows as its blocking banner.
 */
export function describeConfigIssue(issues: ConfigIssue[], t: T): DescribedFinding {
  const first = issues[0]
  return {
    status: 'danger',
    title: t('accounts.readOnly.title'),
    text: first ? t('error.configParse', { file: first.file, line: first.line }) : undefined,
    actionLabel: t('common.openFile'),
    action: { kind: 'open-config' },
  }
}
