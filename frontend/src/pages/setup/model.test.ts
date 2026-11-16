import { describe, expect, it } from 'vitest'
import { FailureCause, InstallStepKind, InstallStepState, SetupFix, SetupToolID, ToolStatus, Tri } from '../../api/models'
import type { InstallFailure, InstallState, InstallStep, SetupState, SetupTool } from '../../api/models'
import { en, zh } from '../../i18n/locales/setup'
import { failureView, logLines, idleNotice, installPageTargets, logText, logTone, phaseOf, primaryKind, readyCount, redactLine, rowView, rowViews, stepPercent } from './model'

const ok = (id: SetupToolID, version = '1.0.0'): SetupTool => ({ id, status: ToolStatus.ToolOK, version, blocking: false, fix: SetupFix.FixNone })
const missing = (id: SetupToolID): SetupTool => ({ id, status: ToolStatus.ToolMissing, blocking: true, fix: SetupFix.FixInstall })
const pre = { admin: Tri.TriYes, managed: Tri.TriNo, freeBytes: 184e9, needBytes: 1.2e9, enoughSpace: true, prefixExists: false, proxy: { source: 'none' } }
const state = (tools: SetupTool[], over: Partial<SetupState> = {}): SetupState => ({
  platform: 'darwin', tools, ready: tools.filter((t) => !t.blocking).length, total: tools.length, blocked: tools.some((t) => t.blocking), preflight: pre, checkedAt: '', ...over,
})
const step = (id: string, tool: SetupToolID, kind: InstallStepKind, s: InstallStepState, over: Partial<InstallStep> = {}): InstallStep => ({ id, tool, kind, state: s, ...over })
const run = (over: Partial<InstallState> = {}): InstallState => ({ running: false, cancelling: false, cancelled: false, steps: [], log: [], finished: false, ...over })
const fail = (cause: FailureCause, over: Partial<InstallFailure> = {}): InstallFailure => ({ tool: SetupToolID.SetupGit, stepId: 'git:install', cause, exitCode: 1, ...over })

const { SetupHomebrew: BREW, SetupGit: GIT, SetupAWSCLI: AWS, SetupWinget: WINGET } = SetupToolID
const S = InstallStepState
const K = InstallStepKind

describe('phaseOf', () => {
  const blocked = state([ok(BREW), missing(GIT), ok(AWS)])
  it('idle until a run starts, running while it goes, failed after a failure, done when the gate is open', () => {
    expect(phaseOf(blocked, null)).toBe('idle')
    expect(phaseOf(blocked, run({ running: true }))).toBe('running')
    expect(phaseOf(blocked, run({ finished: true, failure: fail(FailureCause.CauseNetwork) }))).toBe('failed')
    expect(phaseOf(state([ok(BREW), ok(GIT), ok(AWS)]), run({ finished: true }))).toBe('done')
  })
  it('a stale failure does not hide an open gate (the user installed it by hand and checked again)', () => {
    expect(phaseOf(state([ok(BREW), ok(GIT), ok(AWS)]), run({ finished: true, failure: fail(FailureCause.CauseNetwork) }))).toBe('done')
  })
  it('a cancelled run is idle, not failed', () => {
    expect(phaseOf(blocked, run({ finished: true, cancelled: true }))).toBe('idle')
  })
  it('nothing is known before the first answer: idle', () => {
    expect(phaseOf(null, null)).toBe('idle')
  })
})

describe('rowView', () => {
  const git = missing(GIT)
  it('without a run the row follows the gate status', () => {
    expect(rowView(git, null).kind).toBe('missing')
    expect(rowView({ ...git, status: ToolStatus.ToolTooOld, fix: SetupFix.FixUpgrade }, null).kind).toBe('too-old')
    expect(rowView({ ...git, status: ToolStatus.ToolV1 }, null).kind).toBe('v1')
    expect(rowView({ ...git, status: ToolStatus.ToolError }, null).kind).toBe('error')
    expect(rowView(ok(GIT, '2.56.1'), null)).toMatchObject({ kind: 'ok', version: '2.56.1' })
    expect(rowView({ ...ok(AWS), status: ToolStatus.ToolOutdated }, null).kind).toBe('ok')
  })
  it('use-detected is its own row, with no step', () => {
    const t: SetupTool = { ...git, status: ToolStatus.ToolTooOld, fix: SetupFix.FixUseDetected, override: true }
    expect(rowView(t, run({ running: true }))).toMatchObject({ kind: 'detected', steps: [] })
  })
  it('queued, installing, waiting-os, done and failed follow the steps of that tool', () => {
    const steps = (s: InstallStepState[]) => run({ running: true, steps: s.map((x, i) => step(`homebrew:${i}`, BREW, [K.StepDownload, K.StepVerify, K.StepInstall][i], x)) })
    const brew = missing(BREW)
    expect(rowView(brew, steps([S.StepQueued, S.StepQueued, S.StepQueued])).kind).toBe('queued')
    expect(rowView(brew, steps([S.StepRunning, S.StepQueued, S.StepQueued]))).toMatchObject({ kind: 'installing', current: { id: 'homebrew:0' } })
    expect(rowView(brew, steps([S.StepDone, S.StepDone, S.StepWaitingOS]))).toMatchObject({ kind: 'waiting', current: { id: 'homebrew:2' } })
    expect(rowView(brew, steps([S.StepDone, S.StepDone, S.StepDone])).kind).toBe('ok')
    expect(rowView(brew, steps([S.StepDone, S.StepFailed, S.StepSkipped])).kind).toBe('failed')
  })
  it('after a failure the skipped rows wait for the retry; after Cancel they are just not installed', () => {
    const skipped = [step('awscli:install', AWS, K.StepInstall, S.StepSkipped)]
    expect(rowView(missing(AWS), run({ finished: true, failure: fail(FailureCause.CauseNetwork), steps: skipped })).kind).toBe('queued')
    expect(rowView(missing(AWS), run({ finished: true, cancelled: true, steps: skipped })).kind).toBe('missing')
  })
  it('a row with no step of its own is untouched by a run', () => {
    expect(rowView(ok(BREW, '5.1.2'), run({ running: true, steps: [step('git:install', GIT, K.StepInstall, S.StepRunning)] })).kind).toBe('ok')
  })
  it('readyCount counts what the rows show, so a tool the run just installed counts before the state is re-read', () => {
    const s = state([ok(BREW), missing(GIT), missing(AWS)])
    const st = run({ running: true, steps: [step('git:install', GIT, K.StepInstall, S.StepDone), step('awscli:install', AWS, K.StepInstall, S.StepRunning)] })
    expect(readyCount(rowViews(s, st))).toBe(2)
  })
})

describe('stepPercent', () => {
  it('only a download with a known size has a percentage', () => {
    expect(stepPercent(step('d', BREW, K.StepDownload, S.StepRunning, { done: 50, total: 200 }))).toBe(25)
    expect(stepPercent(step('d', BREW, K.StepDownload, S.StepRunning, { done: 0, total: 0 }))).toBeUndefined()
    expect(stepPercent(step('i', GIT, K.StepInstall, S.StepRunning, { done: 5, total: 10 }))).toBeUndefined()
    expect(stepPercent(step('d', BREW, K.StepDownload, S.StepRunning, { done: 999, total: 10 }))).toBe(100)
  })
})

describe('primaryKind', () => {
  const g = (tools: SetupTool[]) => state(tools)
  it('one-click install by default, retry after a failure, enter when done, busy while running', () => {
    const s = g([ok(BREW), missing(GIT), ok(AWS)])
    expect(primaryKind('idle', s, null, false)).toBe('install')
    expect(primaryKind('failed', s, fail(FailureCause.CauseNetwork), false)).toBe('retry')
    expect(primaryKind('failed', s, fail(FailureCause.CauseCancelled), false)).toBe('retry')
    expect(primaryKind('failed', s, fail(FailureCause.CauseBadSignature), false)).toBe('retry')
    expect(primaryKind('failed', s, fail(FailureCause.CauseDamagedDownload), false)).toBe('redownload')
    expect(primaryKind('running', s, null, false)).toBe('busy')
    expect(primaryKind('done', s, null, false)).toBe('enter')
  })
  it('after "I will install them myself" only use-detected rows still blocking wait for Continue, not Check again', () => {
    const detected = { ...missing(GIT), fix: SetupFix.FixUseDetected } as SetupTool
    expect(primaryKind('idle', g([ok(BREW), detected, ok(AWS)]), null, true)).toBe('continue')
    expect(primaryKind('idle', g([ok(BREW), detected, missing(AWS)]), null, true)).toBe('recheck')
  })
  it('causes the app cannot fix ask for a re-check, not a retry', () => {
    const s = g([missing(BREW), missing(GIT), missing(AWS)])
    for (const c of [FailureCause.CausePrefixExists, FailureCause.CauseWingetMissing, FailureCause.CauseReboot]) expect(primaryKind('failed', s, fail(c), false)).toBe('recheck')
  })
  it('"I will install them myself" turns the footer into Check again', () => {
    expect(primaryKind('idle', g([missing(GIT)]), null, true)).toBe('recheck')
  })
  it('a repair-prefix row asks for a re-check before any run; a missing winget opens the Store; only use-detected rows just continue', () => {
    expect(primaryKind('idle', g([{ ...missing(BREW), fix: SetupFix.FixRepairPrefix }, missing(GIT)]), null, false)).toBe('recheck')
    expect(primaryKind('idle', g([{ ...missing(WINGET), fix: SetupFix.FixOpenStore }, missing(GIT)]), null, false)).toBe('open-store')
    const det: SetupTool = { ...missing(GIT), status: ToolStatus.ToolTooOld, fix: SetupFix.FixUseDetected, override: true }
    expect(primaryKind('idle', g([ok(BREW), det, ok(AWS)]), null, false)).toBe('continue')
    expect(primaryKind('idle', g([ok(BREW), det, missing(AWS)]), null, false)).toBe('install')
  })
})

describe('failureView', () => {
  const causes = Object.values(FailureCause).filter((c): c is Exclude<FailureCause, FailureCause.$zero> => c !== FailureCause.$zero)
  it('every cause has its own sentence, in both languages, except the unknown ones that share the generic text', () => {
    for (const c of causes) {
      const v = failureView(fail(c), 'darwin', pre)
      expect(v.titleKey in zh, `${c} title zh`).toBe(true)
      expect(v.titleKey in en, `${c} title en`).toBe(true)
      expect(v.bodyKey in zh && v.bodyKey in en, `${c} body`).toBe(true)
      if (c !== FailureCause.CauseUnknown && c !== FailureCause.CauseInstallerFailed) expect(v.titleKey, c).not.toBe('setup.fail.generic.title')
    }
  })
  it('the sentences of design 5.1 are the ones used', () => {
    expect(zh['setup.fail.signature.title']).toBe('安装包的签名不对，已停止')
    expect(en['setup.fail.signature.title']).toBe('The installer’s signature is wrong, so nothing was run')
    expect(zh['setup.fail.damaged.title']).toBe('下载的安装包不完整')
    expect(en['setup.fail.signer.title']).toBe('This version of Codepass does not know who signed Homebrew’s installer')
    expect(zh['setup.fail.prefix.title']).toContain('没装完的 Homebrew')
    expect(en['setup.fail.winget.title']).toBe('This PC has no winget and cannot open Microsoft Store')
    expect(zh['setup.fail.cancelled.title.mac']).toContain('你取消了 macOS 的密码窗口')
    expect(en['setup.fail.network.title']).toBe('{tool} was not installed: the download timed out')
  })
  it('the network case adds the proxy line only when a proxy was used', () => {
    expect(failureView(fail(FailureCause.CauseNetwork, { proxy: 'proxy.example.com:8080' }), 'darwin').proxyKey).toBe('setup.fail.proxyUsed')
    expect(failureView(fail(FailureCause.CauseNetwork), 'darwin').proxyKey).toBeUndefined()
  })
  it('fills the names: the tool, the package to ask IT for, the space', () => {
    const v = failureView(fail(FailureCause.CausePolicy, { tool: BREW }), 'darwin', pre)
    expect(v.params).toMatchObject({ tool: 'Homebrew', pkg: 'Homebrew.pkg' })
    expect(failureView(fail(FailureCause.CausePolicy, { tool: AWS }), 'windows').params.pkg).toBe('Amazon.AWSCLI')
    expect(failureView(fail(FailureCause.CauseDiskFull), 'darwin', { ...pre, freeBytes: 0.6e9 }).params).toMatchObject({ need: '0.6', free: '0.6' })
    expect(failureView(fail(FailureCause.CauseCancelled), 'windows').titleKey).toBe('setup.fail.cancelled.title.win')
  })
  it('no sentence promises a backup of a cleared tool path (FU-12b)', () => {
    for (const text of [...Object.values(zh), ...Object.values(en)]) expect(text).not.toMatch(/备份|backup|backed up/i)
  })
})

describe('installPageTargets and idleNotice', () => {
  it('with Homebrew missing only its page is opened; otherwise one page per blocking row', () => {
    expect(installPageTargets(state([missing(BREW), missing(GIT), missing(AWS)]))).toEqual([BREW])
    expect(installPageTargets(state([ok(BREW), missing(GIT), missing(AWS)]))).toEqual([GIT, AWS])
    expect(installPageTargets(state([ok(BREW), { ...missing(GIT), fix: SetupFix.FixUseDetected }, ok(AWS)]))).toEqual([])
    expect(installPageTargets(state([{ ...missing(WINGET), fix: SetupFix.FixOpenStore }, missing(GIT)], { platform: 'windows' }))).toEqual([WINGET, GIT])
  })
  it('notices before the password window', () => {
    const all = [missing(BREW), missing(GIT), missing(AWS)]
    expect(idleNotice(state(all))).toBeNull()
    expect(idleNotice(state(all, { preflight: { ...pre, admin: Tri.TriNo } }))).toBe('not-admin')
    expect(idleNotice(state(all, { preflight: { ...pre, managed: Tri.TriYes } }))).toBe('managed')
    expect(idleNotice(state(all, { preflight: { ...pre, enoughSpace: false } }))).toBe('low-disk')
    expect(idleNotice(state(all, { preflight: { ...pre, prefixExists: true } }))).toBe('prefix')
    expect(idleNotice(state(all, { preflight: { ...pre, proxy: { source: 'system', pacOnly: true } } }))).toBe('proxy-pac')
    // an account that is not an administrator matters only when Homebrew itself has to be installed
    expect(idleNotice(state([ok(BREW), missing(GIT), ok(AWS)], { preflight: { ...pre, admin: Tri.TriNo } }))).toBeNull()
  })
})

describe('the log', () => {
  it('never shows a secret', () => {
    expect(redactLine('curl https://user:hunter2@example.com/x')).toBe('curl https://example.com/x')
    expect(redactLine('token=abc123 and password: hunter2')).toBe('token=*** and password: ***')
    expect(redactLine('Authorization: Bearer abc.def')).toBe('Authorization: ***')
    expect(redactLine('ghp_1234567890abcdefghij')).toBe('***')
    expect(redactLine('The administrator user name or password was incorrect.')).toBe('The administrator user name or password was incorrect.')
    expect(logText(run({ log: ['token=abc', 'ok'], failure: fail(FailureCause.CauseNetwork, { exitCode: 28 }) }))).toBe('token=***\nok\n# git: network at git:install, exit code 28')
  })
  it('colours: failing lines are danger only once the run failed; $ and ==> are dim', () => {
    const f = fail(FailureCause.CauseNetwork, { line: 'Error: boom' })
    expect(logTone('Error: boom', f)).toBe('bad')
    expect(logTone('curl: (28) timed out', f)).toBe('bad')
    expect(logTone('Error: boom', null)).toBe('plain')
    // Go logs a verify failure as "verify: <detail>" while failure.line is the detail
    expect(logTone('verify: signature: no signature', { ...f, line: 'signature: no signature' })).toBe('bad')
    expect(logTone('$ brew install git', null)).toBe('dim')
    expect(logTone('==> Pouring git', null)).toBe('dim')
    expect(logTone('✓ signature valid', null)).toBe('ok')
  })
})

describe('logLines', () => {
  it('adds the failure line when no log line carries it, in the log text too', () => {
    const f = fail(FailureCause.CauseInstallerFailed, { line: 'git is still not usable after the install (2.39.5, needs 2.44.0)' })
    const r = run({ log: ['==> Pouring git'], failure: f })
    expect(logLines(r)).toEqual(['==> Pouring git', f.line])
    expect(logText(r).split('\n').slice(0, 2)).toEqual(['==> Pouring git', f.line])
  })
  it('keeps the log as is when a line already ends with it ("verify: <detail>")', () => {
    const r = run({ log: ['verify: no signature'], failure: fail(FailureCause.CauseBadSignature, { line: 'no signature' }) })
    expect(logLines(r)).toEqual(['verify: no signature'])
  })
})

describe('locale', () => {
  it('both languages have every key with the same placeholders', () => {
    const ph = (s: string) => [...s.matchAll(/\{(\w+)\}/g)].map((m) => m[1]).sort().join(',')
    for (const k of Object.keys(zh) as (keyof typeof zh)[]) expect(ph(en[k]), k).toBe(ph(zh[k]))
    expect(Object.keys(en).sort()).toEqual(Object.keys(zh).sort())
  })
})
