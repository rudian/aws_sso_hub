import { describe, expect, it } from 'vitest'
import { UpdatePhase } from '../../api/models'
import type { UpdateState } from '../../api/models'
import { bannerKind, codeKey, compareVersions, etaSeconds, noteLines, percent, showsDot, stepsFor } from './model'

const st = (p: Partial<UpdateState>): UpdateState => ({ phase: UpdatePhase.UpdateIdle, current: '1.0.0', ...p })
const flat = (s: UpdateState) => stepsFor(s).map((x) => x.status).join(',')

describe('state mapping', () => {
  it('maps each phase to the four steps', () => {
    expect(flat(st({ phase: UpdatePhase.UpdateAvailable }))).toBe('todo,todo,todo,todo')
    expect(flat(st({ phase: UpdatePhase.UpdateDownloading }))).toBe('now,todo,todo,todo')
    expect(flat(st({ phase: UpdatePhase.UpdateVerifying }))).toBe('done,now,todo,todo')
    expect(flat(st({ phase: UpdatePhase.UpdateReady }))).toBe('done,done,todo,todo')
    expect(flat(st({ phase: UpdatePhase.UpdateRestarting }))).toBe('done,done,now,todo')
  })

  it('marks the failed step', () => {
    expect(flat(st({ phase: UpdatePhase.UpdateError, step: 'download' }))).toBe('error,todo,todo,todo')
    expect(flat(st({ phase: UpdatePhase.UpdateRefused, step: 'verify' }))).toBe('done,error,todo,todo')
    expect(flat(st({ phase: UpdatePhase.UpdateRefused, code: 'digest' }))).toBe('done,error,todo,todo')
    expect(flat(st({ phase: UpdatePhase.UpdateError, step: 'restart' }))).toBe('done,done,error,todo')
  })

  it('shows the dot only while a release is on offer, downloading or ready', () => {
    expect(showsDot(st({ phase: UpdatePhase.UpdateAvailable }))).toBe(true)
    expect(showsDot(st({ phase: UpdatePhase.UpdateReady }))).toBe(true)
    for (const p of [UpdatePhase.UpdateIdle, UpdatePhase.UpdateUpToDate, UpdatePhase.UpdateError, UpdatePhase.UpdateRefused, UpdatePhase.UpdateBlocked]) {
      expect(showsDot(st({ phase: p }))).toBe(false)
    }
  })

  it('picks the banner: nothing for a failed check, a danger line for a failed download', () => {
    expect(bannerKind(st({ phase: UpdatePhase.UpdateAvailable, version: '1.1.0' }))).toBe('available')
    expect(bannerKind(st({ phase: UpdatePhase.UpdateVerifying, version: '1.1.0' }))).toBe('downloading')
    expect(bannerKind(st({ phase: UpdatePhase.UpdateError, step: 'check' }))).toBeNull()
    expect(bannerKind(st({ phase: UpdatePhase.UpdateError, step: 'download', version: '1.1.0' }))).toBe('failed')
    expect(bannerKind(st({ phase: UpdatePhase.UpdateBlocked, block: 'translocated' }))).toBeNull()
  })

  it('computes progress and time left', () => {
    expect(percent(st({ written: 31_200_000, total: 48_600_000 }))).toBe(64)
    expect(percent(st({ written: 5 }))).toBe(0)
    expect(etaSeconds(st({ written: 31_200_000, total: 48_600_000 }), 870_000)).toBe(20)
    expect(etaSeconds(st({ written: 1, total: 100 }), 0)).toBeNull()
  })

  it('knows its failure codes and falls back for others', () => {
    expect(codeKey('digest')).toBe('update.code.digest')
    expect(codeKey('something-new')).toBe('update.code.other')
    expect(codeKey(undefined)).toBe('update.code.other')
  })

  it('compares versions numerically', () => {
    expect(compareVersions('1.10.0', '1.9.3')).toBe(1)
    expect(compareVersions('1.0.0', '1.0.0')).toBe(0)
    expect(compareVersions('v1.0.0-rc1', '1.0.1')).toBe(-1)
  })
})

describe('release notes', () => {
  it('splits lines, drops list markers and blank lines, and keeps markup as text', () => {
    expect(noteLines('- one\r\n\n* <b>two</b>\n  3) three\nplain')).toEqual(['one', '<b>two</b>', 'three', 'plain'])
    expect(noteLines(undefined)).toEqual([])
  })
})

describe('compareVersions with pre-releases', () => {
  it('orders rc numbers numerically and puts a release above its pre-releases', () => {
    expect(compareVersions('1.0.0-rc.2', '1.0.0-rc.1')).toBe(1)
    expect(compareVersions('1.0.0-rc.10', '1.0.0-rc.2')).toBe(1)
    expect(compareVersions('1.0.0', '1.0.0-rc.3')).toBe(1)
    expect(compareVersions('1.0.0-rc.3', '1.0.0')).toBe(-1)
    expect(compareVersions('1.0.0-rc.1', '1.0.0-rc.1')).toBe(0)
    expect(compareVersions('1.1.0', '1.0.9')).toBe(1)
    expect(compareVersions('v1.0.0+build5', '1.0.0')).toBe(0)
  })
})
