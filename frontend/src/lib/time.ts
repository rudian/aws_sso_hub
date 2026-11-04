import type { Lang, T } from '../i18n'

const MIN = 60_000
const HOUR = 60 * MIN

/** Milliseconds left until `iso` (negative when past). NaN-safe: an unparsable value counts as already past. */
export function remainingMs(iso: string, now: number): number {
  const t = Date.parse(iso)
  return Number.isNaN(t) ? -1 : t - now
}

function totalMinutesCeil(ms: number): number {
  return Math.max(0, Math.ceil(ms / MIN))
}

/** zh "还剩 6 小时 12 分" / en "6 h 12 min left"; under an hour "还剩 48 分钟". Rounds up to whole minutes. */
export function formatRemaining(iso: string, now: number, t: T): string {
  const total = totalMinutesCeil(remainingMs(iso, now))
  if (total >= 60) return t('common.remaining.hm', { h: Math.floor(total / 60), m: total % 60 })
  return t('common.remaining.m', { m: total })
}

/** "6:12" (h:mm) from one hour up, "48m" below. */
export function formatKeyringTime(iso: string, now: number): string {
  const total = totalMinutesCeil(remainingMs(iso, now))
  if (total >= 60) return `${Math.floor(total / 60)}:${String(total % 60).padStart(2, '0')}`
  return `${total}m`
}

function parts(iso: string, lang: Lang, timeZone: string | undefined, opts: Intl.DateTimeFormatOptions) {
  const f = new Intl.DateTimeFormat(lang === 'zh-CN' ? 'zh-CN' : 'en-US', { hourCycle: 'h23', timeZone, ...opts })
  const out: Record<string, string> = {}
  for (const p of f.formatToParts(new Date(iso))) out[p.type] = p.value
  return out
}

/** zh "11月4日 01:46", en "Nov 4, 01:46" (local time unless timeZone is given). */
export function formatDateTime(iso: string, lang: Lang, timeZone?: string): string {
  if (Number.isNaN(Date.parse(iso))) return '—'
  if (lang === 'zh-CN') {
    const p = parts(iso, lang, timeZone, { month: 'numeric', day: 'numeric', hour: '2-digit', minute: '2-digit' })
    return `${p.month}月${p.day}日 ${p.hour}:${p.minute}`
  }
  const p = parts(iso, lang, timeZone, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })
  return `${p.month} ${p.day}, ${p.hour}:${p.minute}`
}

/** "18:55" */
export function formatClock(iso: string, lang: Lang, timeZone?: string): string {
  if (Number.isNaN(Date.parse(iso))) return '—'
  const p = parts(iso, lang, timeZone, { hour: '2-digit', minute: '2-digit' })
  return `${p.hour}:${p.minute}`
}

/** "valid until {time}": the clock only when it is today, otherwise date and clock. */
export function formatUntil(iso: string, now: number, lang: Lang, timeZone?: string): string {
  const sameDay = (a: number, b: number) => {
    const f = (x: number) => new Intl.DateTimeFormat('en-CA', { timeZone, year: 'numeric', month: '2-digit', day: '2-digit' }).format(x)
    return f(a) === f(b)
  }
  const ts = Date.parse(iso)
  return Number.isNaN(ts) ? '—' : sameDay(ts, now) ? formatClock(iso, lang, timeZone) : formatDateTime(iso, lang, timeZone)
}

/** Like formatUntil, but names the day: "Today 18:55", "Tomorrow 03:20", otherwise date and clock. */
export function formatUntilDay(iso: string, now: number, lang: Lang, timeZone?: string): string {
  const ts = Date.parse(iso)
  if (Number.isNaN(ts)) return '—'
  const key = (x: number) => new Intl.DateTimeFormat('en-CA', { timeZone, year: 'numeric', month: '2-digit', day: '2-digit' }).format(x)
  const clock = formatClock(iso, lang, timeZone)
  if (key(ts) === key(now)) return `${lang === 'zh-CN' ? '今天' : 'Today'} ${clock}`
  if (key(ts) === key(now + 24 * HOUR)) return `${lang === 'zh-CN' ? '明天' : 'Tomorrow'} ${clock}`
  return formatDateTime(iso, lang, timeZone)
}

/** justNow / minutesAgo / hoursAgo / daysAgo */
export function formatRelative(iso: string, now: number, t: T): string {
  const ago = now - Date.parse(iso)
  if (Number.isNaN(ago) || ago < MIN) return t('common.relative.justNow')
  if (ago < HOUR) return t('common.relative.minutesAgo', { n: Math.floor(ago / MIN) })
  if (ago < 24 * HOUR) return t('common.relative.hoursAgo', { n: Math.floor(ago / HOUR) })
  return t('common.relative.daysAgo', { n: Math.floor(ago / (24 * HOUR)) })
}

/** 480 -> "8 hours" / "8 小时"; 15 -> "15 minutes"; 2880 -> "2 days". Uses Intl unit formatting, so no extra i18n keys. */
export function formatSessionLength(minutes: number, lang: Lang): string {
  const nf = (unit: string, v: number) =>
    new Intl.NumberFormat(lang === 'zh-CN' ? 'zh-CN' : 'en-US', { style: 'unit', unit, unitDisplay: 'long', maximumFractionDigits: 2 }).format(v)
  if (minutes < 60) return nf('minute', minutes)
  if (minutes % 1440 === 0 && minutes >= 2880) return nf('day', minutes / 1440)
  return nf('hour', minutes / 60)
}
