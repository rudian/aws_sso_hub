import { describe, expect, it } from 'vitest'
import { BackupKind, BackupReason } from '../../api/models'
import type { BackupEntry } from '../../api/models'
import { makeT, zhAll } from '../../i18n'
import { backupReasonText, formatBytes, newestBackup, settingsSubtitle } from './util'

const entry = (reason: BackupReason, params: Record<string, string> = {}, createdAt = '2026-11-04T10:00:00Z'): BackupEntry => ({
  id: createdAt + reason,
  createdAt,
  path: '/Users/alex/.aws/config',
  file: '~/.aws/config',
  kind: BackupKind.BackupAWSConfig,
  reason,
  reasonParams: params,
  size: 1,
  redacted: false,
})

describe('backup reasons', () => {
  it('has a text for every reason the backend can record', () => {
    for (const r of Object.values(BackupReason)) {
      if (r === ('' as BackupReason)) continue
      expect(zhAll[`settings.backups.reason.${r}` as keyof typeof zhAll], r).toBeDefined()
    }
  })
  it('fills in the parameters', () => {
    expect(backupReasonText(entry(BackupReason.ReasonMigrate, { profile: 'acme' }), makeT('en'))).toBe('Migrated acme')
    expect(backupReasonText(entry(BackupReason.ReasonBind, { repo: 'acme_cs_portal' }), makeT('zh-CN'))).toBe('绑定 acme_cs_portal')
    expect(backupReasonText(entry(BackupReason.ReasonAddProfiles, { n: '3' }), makeT('en'))).toBe('Added 3 profiles')
    expect(backupReasonText(entry(BackupReason.ReasonImport, { n: '1' }), makeT('en'))).toBe('Imported 1 account item')
  })
  it('falls back for an unknown reason', () => {
    expect(backupReasonText(entry('from-the-future' as BackupReason), makeT('en'))).toBe('Other')
  })
})

describe('helpers', () => {
  it('formats sizes', () => {
    expect(formatBytes(812)).toBe('812 B')
    expect(formatBytes(2048)).toBe('2.0 KB')
    expect(formatBytes(1572864)).toBe('1.5 MB')
  })
  it('finds the newest backup', () => {
    const a = entry(BackupReason.ReasonBind, {}, '2026-11-04T09:00:00Z')
    const b = entry(BackupReason.ReasonUnbind, {}, '2026-11-04T11:00:00Z')
    expect(newestBackup([a, b])).toBe(b)
    expect(newestBackup(null)).toBeNull()
  })
})

describe('settingsSubtitle (D28)', () => {
  it('names the version and the platform on macOS and Windows, in both languages', () => {
    const zh = makeT('zh-CN')
    const en = makeT('en')
    expect(settingsSubtitle('1.0.0', 'darwin', zh)).toBe('Codepass 1.0.0 · macOS 15 或更高，Apple 芯片')
    expect(settingsSubtitle('1.0.0', 'darwin', en)).toBe('Codepass 1.0.0 · macOS 15 or later, Apple silicon')
    expect(settingsSubtitle('1.0.0', 'windows', zh)).toBe('Codepass 1.0.0 · Windows 10 或 11，x64')
    expect(settingsSubtitle('1.0.0', 'windows', en)).toBe('Codepass 1.0.0 · Windows 10 or 11, x64')
  })
  it('shows only the platform while the version is not known', () => {
    expect(settingsSubtitle(undefined, 'darwin', makeT('en'))).toBe('macOS 15 or later, Apple silicon')
  })
})
