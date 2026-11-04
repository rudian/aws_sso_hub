import { useMocks } from '../api/env'
import { initialUI } from './initialUI'

// Wall clock. In mock mode `?now=<ISO>` makes time start at that instant and advance with the real clock.
const base = useMocks && initialUI.now ? Date.parse(initialUI.now) : null
const t0 = Date.now()

export function now(): number {
  return base === null ? Date.now() : base + (Date.now() - t0)
}

export const MIN = 60_000
export const HOUR = 60 * MIN

/** RFC 3339 UTC without milliseconds: 2026-11-04T02:18:00Z */
export function iso(ms: number): string {
  return new Date(ms).toISOString().replace(/\.\d{3}Z$/, 'Z')
}

export function nowISO(): string {
  return iso(now())
}

export function ago(ms: number): string {
  return iso(now() - ms)
}

export function fromNow(ms: number): string {
  return iso(now() + ms)
}

export function deepCopy<T>(v: T): T {
  return v === undefined ? v : (JSON.parse(JSON.stringify(v)) as T)
}
