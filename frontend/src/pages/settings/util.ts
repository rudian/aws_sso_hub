import type { BackupEntry } from '../../api/models'
import type { MessageKey, T } from '../../i18n'

export const REPO_URL = 'https://github.com/rudian/aws_sso_hub'

/** 812 -> "812 B", 2048 -> "2.0 KB", 1572864 -> "1.5 MB" */
export function formatBytes(n: number): string {
  if (!Number.isFinite(n) || n < 0) return '—'
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  return `${(n / (1024 * 1024)).toFixed(1)} MB`
}

/** The text for a backup's reason (`settings.backups.reason.<reason>`), with the recorded parameters. */
export function backupReasonText(entry: Pick<BackupEntry, 'reason' | 'reasonParams'>, t: T): string {
  const key = `settings.backups.reason.${entry.reason}` as MessageKey
  const params: Record<string, string | number> = {}
  for (const [k, v] of Object.entries(entry.reasonParams ?? {})) if (v !== undefined) params[k] = v
  const text = t(key, params)
  // An unknown reason from a newer backend: show the generic text instead of the key.
  return text === key ? t('settings.backups.reason.identity') : text
}

/** The most recently created backup, or null. */
export function newestBackup(entries: BackupEntry[] | null | undefined): BackupEntry | null {
  let best: BackupEntry | null = null
  for (const e of entries ?? []) if (!best || Date.parse(e.createdAt) > Date.parse(best.createdAt)) best = e
  return best
}

export function platformName(platform: string): string {
  return platform === 'darwin' ? 'macOS' : platform === 'windows' ? 'Windows' : platform
}
