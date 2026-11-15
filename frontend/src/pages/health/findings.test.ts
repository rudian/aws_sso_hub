import { describe, expect, it } from 'vitest'
import { HealthKind, Severity } from '../../api/models'
import type { HealthFinding } from '../../api/models'
import { makeT } from '../../i18n'
import { describeConfigIssue, describeFinding } from './findings'

const t = makeT('en')
const base = { id: 'x', severity: Severity.SeverityWarn }

describe('describeFinding', () => {
  it('shell export: reveals the file', () => {
    const d = describeFinding({ ...base, kind: HealthKind.HealthShellExport, severity: Severity.SeverityDanger, file: '~/.zshrc', line: 42 } as HealthFinding, t)
    expect(d.status).toBe('danger')
    expect(d.title).toBe('~/.zshrc exports an access key on line 42')
    expect(d.action).toEqual({ kind: 'reveal', file: '~/.zshrc' })
  })
  it('leftover keys: jumps to the remove-key step of the profile', () => {
    const d = describeFinding({ ...base, kind: HealthKind.HealthLeftoverKeys, profile: 'initech-admin' } as HealthFinding, t)
    expect(d.action).toEqual({ kind: 'remove-key', profile: 'initech-admin' })
  })
  it('default keys, global static, url userinfo, pending deactivation point at the right place', () => {
    expect(describeFinding({ ...base, kind: HealthKind.HealthDefaultKeys } as HealthFinding, t).action).toEqual({ kind: 'tutorial', section: 'faq' })
    expect(describeFinding({ ...base, kind: HealthKind.HealthGlobalStatic, profile: 'acme' } as HealthFinding, t).action).toEqual({ kind: 'git' })
    expect(describeFinding({ ...base, kind: HealthKind.HealthURLUserinfo, params: { repo: 'acme_design' } } as HealthFinding, t).action).toEqual({ kind: 'repos' })
    const p = describeFinding({ ...base, kind: HealthKind.HealthPendingKey, severity: Severity.SeverityInfo, profile: 'acme', params: { maskedKeyId: 'AKIA••••••••••••3F2A' } } as HealthFinding, t)
    expect(p.status).toBe('info')
    expect(p.action).toEqual({ kind: 'tutorial', section: 'migrate' })
    expect(p.text).toContain('3F2A')
  })
  it('env var has no action', () => {
    const d = describeFinding({ ...base, kind: HealthKind.HealthEnvVar, severity: Severity.SeverityDanger, params: { name: 'AWS_ACCESS_KEY_ID' } } as HealthFinding, t)
    expect(d.title).toBe('AWS_ACCESS_KEY_ID is set in the app’s environment')
    expect(d.action).toBeUndefined()
  })
})

describe('describeConfigIssue', () => {
  it('a config parse error is a danger row that opens the file', () => {
    const d = describeConfigIssue([{ file: '~/.aws/config', line: 12, message: 'bad section' }], t)
    expect(d.status).toBe('danger')
    expect(d.title).toBe('Fix the config file first')
    expect(d.text).toBe('~/.aws/config can’t be read at line 12. Fix it to continue.')
    expect(d.action).toEqual({ kind: 'open-config' })
  })
})
