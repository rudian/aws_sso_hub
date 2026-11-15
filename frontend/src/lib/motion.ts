import { useLayoutEffect, useRef } from 'react'
import type { RefObject } from 'react'
import type { MotionPref } from '../api/models'

/** ui-design.md section 3. Durations in ms; they mirror --dur-* and --ease-* in tokens.css. */
export const DUR = { page: 200, panel: 220, list: 180, leave: 160, move: 200, check: 420 } as const
export const EASE = {
  enter: 'cubic-bezier(.2, .8, .2, 1)',
  exit: 'cubic-bezier(.4, 0, 1, 1)',
  move: 'cubic-bezier(.3, .7, .2, 1)',
} as const

/** Above this many rows, insert / remove / reorder motion is skipped (lists of hundreds of rows). */
export const MOTION_ROW_LIMIT = 200

/** What Settings > Appearance > Motion stores: the values of MotionPref in the Go API. */
export type MotionMode = 'system' | 'full' | 'reduce'

export function systemPrefersReducedMotion(): boolean {
  return typeof window !== 'undefined' && typeof window.matchMedia === 'function' && window.matchMedia('(prefers-reduced-motion: reduce)').matches
}

/** True when motion should not run: Reduced, or System while the OS asks for less. Full overrides the OS. */
export function resolveReduced(mode: MotionMode, systemReduced: boolean): boolean {
  if (mode === 'reduce') return true
  if (mode === 'full') return false
  return systemReduced
}

/** The mode currently written on <html data-motion>. No attribute means System. */
export function currentMotionMode(): MotionMode {
  if (typeof document === 'undefined') return 'system'
  const v = document.documentElement.getAttribute('data-motion')
  return v === 'reduce' || v === 'full' ? v : 'system'
}

export function motionReduced(): boolean {
  return resolveReduced(currentMotionMode(), systemPrefersReducedMotion())
}

/**
 * Whether JS-driven list motion may run for a list of `rows` rows.
 * CSS motion is switched off by the rules in tokens.css; this covers the helpers below.
 */
export function listMotionAllowed(rows: number): boolean {
  return rows <= MOTION_ROW_LIMIT && !motionReduced()
}

/** Writes the choice on <html>: reduce and full are explicit, system removes the attribute so the OS decides. */
export function applyMotionMode(mode: MotionMode): void {
  const root = document.documentElement
  if (mode === 'system') root.removeAttribute('data-motion')
  else root.setAttribute('data-motion', mode)
}

/** Same as applyMotionMode, from the generated MotionPref enum value (the string values are the same). */
export function applyMotionPref(pref: MotionPref | string | undefined | null): void {
  applyMotionMode(pref === 'reduce' || pref === 'full' ? pref : 'system')
}

/** Milliseconds to wait for an exit animation before unmounting: 0 when motion is off. */
export function exitDelay(ms: number = DUR.leave): number {
  return motionReduced() ? 0 : ms
}

type Animatable = Pick<Element, 'animate'>
function canAnimate(el: Element): el is Element & Animatable {
  return typeof (el as Partial<Animatable>).animate === 'function'
}

/** Insert: the row fades in and drops 4px. Skipped in long lists and under reduced motion. */
export function animateInsert(el: Element, rows: number): Animation | null {
  if (!listMotionAllowed(rows) || !canAnimate(el)) return null
  return el.animate([{ opacity: 0, transform: 'translateY(-4px)' }, { opacity: 1, transform: 'none' }], { duration: DUR.list, easing: EASE.enter })
}

/**
 * Remove: the row fades and collapses its height (grid rows 1fr -> 0fr through the .collapse classes).
 * Resolves when the row can be taken out of the DOM; resolves at once when motion is skipped.
 */
export function animateRemove(el: HTMLElement, rows: number): Promise<void> {
  if (!listMotionAllowed(rows)) return Promise.resolve()
  el.classList.add('is-leaving')
  return new Promise((resolve) => window.setTimeout(resolve, DUR.leave))
}

/** Positions of the keyed rows inside a container, taken before a change (the F of FLIP). */
export function measureFlip(container: ParentNode): Map<string, DOMRect> {
  const out = new Map<string, DOMRect>()
  container.querySelectorAll<HTMLElement>('[data-flip-key]').forEach((el) => {
    const key = el.dataset.flipKey
    if (key !== undefined) out.set(key, el.getBoundingClientRect())
  })
  return out
}

/**
 * The L, I and P of FLIP: after the DOM changed, each row that kept its key and moved travels from the old place
 * to the new one (transform only, 200ms, --ease-move). New and removed rows are not touched. Returns the animations started.
 */
export function playFlip(container: ParentNode, before: Map<string, DOMRect>, rows?: number): Animation[] {
  const els = Array.from(container.querySelectorAll<HTMLElement>('[data-flip-key]'))
  // `rows` is the full list length; a virtualised list renders far fewer elements than it holds.
  if (!listMotionAllowed(rows ?? els.length)) return []
  const started: Animation[] = []
  for (const el of els) {
    const key = el.dataset.flipKey
    const from = key === undefined ? undefined : before.get(key)
    if (!from || !canAnimate(el)) continue
    const to = el.getBoundingClientRect()
    const dx = from.left - to.left
    const dy = from.top - to.top
    if (Math.abs(dx) < 1 && Math.abs(dy) < 1) continue
    started.push(el.animate([{ transform: `translate(${dx}px, ${dy}px)` }, { transform: 'none' }], { duration: DUR.move, easing: EASE.move }))
  }
  return started
}

/**
 * Hook for reorder and stage / unstage moves: put data-flip-key on each row, pass the keys in display order.
 * When the order changes, positions are measured during that render (the DOM still shows the old layout) and
 * played in the layout effect right after the DOM changed. No snapshot is kept between changes, so a scroll or
 * resize in between never turns into motion.
 */
export function useFlip(containerRef: RefObject<HTMLElement | null>, order: readonly string[]): void {
  const snap = useRef<Map<string, DOMRect> | null>(null)
  const committed = useRef(order.join('\u0000'))
  const joined = order.join('\u0000')
  if (joined !== committed.current) {
    const el = containerRef.current
    if (el) snap.current = measureFlip(el)
  }
  useLayoutEffect(() => {
    const el = containerRef.current
    const before = snap.current
    snap.current = null
    committed.current = joined
    if (el && before) playFlip(el, before, order.length)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [joined])
}
