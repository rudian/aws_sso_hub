import { describe, expect, it } from 'vitest'
import { makeT } from '../i18n'
import { formatClock, formatDateTime, formatKeyringTime, formatRelative, formatRemaining, formatSessionLength, formatUntil, formatUntilDay, remainingMs } from './time'

const zh = makeT('zh-CN')
const en = makeT('en')
const now = Date.parse('2026-11-04T00:00:00Z')
const at = (min: number) => new Date(now + min * 60_000).toISOString()

describe('formatRemaining', () => {
  it('formats hours and minutes', () => {
    expect(formatRemaining(at(6 * 60 + 12), now, zh)).toBe('还剩 6 小时 12 分')
    expect(formatRemaining(at(6 * 60 + 12), now, en)).toBe('6 h 12 min left')
  })
  it('formats minutes under an hour', () => {
    expect(formatRemaining(at(48), now, zh)).toBe('还剩 48 分钟')
    expect(formatRemaining(at(48), now, en)).toBe('48 min left')
  })
  it('rounds partial minutes up so the label does not flicker', () => {
    expect(formatRemaining(new Date(now + (6 * 60 + 12) * 60_000 - 1500).toISOString(), now, en)).toBe('6 h 12 min left')
  })
  it('never goes negative', () => {
    expect(remainingMs(at(-5), now)).toBeLessThan(0)
    expect(formatRemaining(at(-5), now, en)).toBe('0 min left')
  })
})

describe('formatKeyringTime', () => {
  it('uses h:mm from one hour and Nm below', () => {
    expect(formatKeyringTime(at(6 * 60 + 12), now)).toBe('6:12')
    expect(formatKeyringTime(at(60), now)).toBe('1:00')
    expect(formatKeyringTime(at(48), now)).toBe('48m')
    expect(formatKeyringTime(at(65), now)).toBe('1:05')
  })
})

describe('date formatting', () => {
  it('formats a local date time in a given time zone', () => {
    expect(formatDateTime('2026-11-03T18:20:00Z', 'zh-CN', 'Asia/Shanghai')).toBe('11月4日 02:20')
    expect(formatDateTime('2026-11-03T18:20:00Z', 'en', 'Asia/Shanghai')).toBe('Nov 4, 02:20')
    expect(formatDateTime('2026-11-03T18:20:00Z', 'en', 'Europe/Berlin')).toBe('Nov 3, 19:20')
    expect(formatDateTime('2026-11-03T18:20:00Z', 'en', 'UTC')).toBe('Nov 3, 18:20')
    expect(formatClock('2026-11-04T10:55:00Z', 'en', 'Asia/Shanghai')).toBe('18:55')
  })
  it('midnight is 00:xx, not 24:xx', () => {
    expect(formatClock('2026-11-03T16:05:00Z', 'zh-CN', 'Asia/Shanghai')).toBe('00:05')
  })
  it('formatUntil shows only the clock for today', () => {
    const n = Date.parse('2026-11-04T01:00:00Z')
    expect(formatUntil('2026-11-04T10:55:00Z', n, 'en', 'Asia/Shanghai')).toBe('18:55')
    expect(formatUntil('2026-11-06T10:55:00Z', n, 'en', 'Asia/Shanghai')).toBe('Nov 6, 18:55')
  })
  it('formatUntilDay names today and tomorrow', () => {
    const n = Date.parse('2026-11-04T01:00:00Z')
    expect(formatUntilDay('2026-11-04T10:55:00Z', n, 'en', 'Asia/Shanghai')).toBe('Today 18:55')
    expect(formatUntilDay('2026-11-04T10:55:00Z', n, 'zh-CN', 'Asia/Shanghai')).toBe('今天 18:55')
    expect(formatUntilDay('2026-11-05T10:55:00Z', n, 'en', 'Asia/Shanghai')).toBe('Tomorrow 18:55')
    expect(formatUntilDay('2026-11-05T10:55:00Z', n, 'zh-CN', 'Asia/Shanghai')).toBe('明天 18:55')
    expect(formatUntilDay('2026-11-07T10:55:00Z', n, 'en', 'Asia/Shanghai')).toBe('Nov 7, 18:55')
    expect(formatUntilDay('nope', n, 'en')).toBe('—')
  })
  it('returns a dash for garbage', () => {
    expect(formatDateTime('nope', 'en')).toBe('—')
  })
})

describe('formatRelative', () => {
  it('picks the unit', () => {
    const base = Date.parse('2026-11-04T12:00:00Z')
    expect(formatRelative('2026-11-04T11:59:40Z', base, en)).toBe('just now')
    expect(formatRelative('2026-11-04T11:58:00Z', base, en)).toBe('2 minutes ago')
    expect(formatRelative('2026-11-04T11:59:00Z', base, en)).toBe('1 minute ago')
    expect(formatRelative('2026-11-04T09:00:00Z', base, en)).toBe('3 hours ago')
    expect(formatRelative('2026-11-04T09:00:00Z', base, zh)).toBe('3 小时前')
    expect(formatRelative('2026-11-02T12:00:00Z', base, en)).toBe('2 days ago')
  })
})

describe('formatSessionLength', () => {
  it('formats presets', () => {
    expect(formatSessionLength(480, 'en')).toBe('8 hours')
    expect(formatSessionLength(480, 'zh-CN')).toBe('8小时')
    expect(formatSessionLength(15, 'en')).toBe('15 minutes')
    expect(formatSessionLength(4320, 'en')).toBe('3 days')
  })
})
