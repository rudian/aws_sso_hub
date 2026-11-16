// Pointer drag of repository rows onto a project section (design 5.2 and section 3). The keyboard path (M, the row
// menu, the bulk bar) lives on the page; both end in the same move.
import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react'
import type { PointerEvent as ReactPointerEvent } from 'react'
import { createPortal } from 'react-dom'
import { ProviderMark } from '../../components'
import type { ProviderKind } from '../../components/ProviderMark'
import { DUR, EASE, motionReduced } from '../../lib/motion'

export interface DragState {
  ids: readonly string[]
  label: string
  /** The provider mark drawn before the name. */
  provider?: ProviderKind | null
  x: number
  y: number
  /** The section under the pointer (project ID, '' = Ungrouped), or null. */
  over: string | null
}

export interface DragHandlers {
  onStart(ids: readonly string[]): void
  /** The pointer moved onto another section (or off every section). */
  onOver(target: string | null): void
  onDrop(ids: readonly string[], target: string): void
  onCancel(ids: readonly string[]): void
  /** A section the rows can go to: false for the section they are all in already. */
  valid(ids: readonly string[], target: string): boolean
}

const THRESHOLD = 4

/** On drop the ghost shrinks into the target's header (a copy of it, because the real one unmounts at once). 200ms; skipped under Reduced motion. */
function animateGhostDrop(ghost: HTMLElement, to: DOMRect): void {
  if (motionReduced() || typeof ghost.animate !== 'function') return
  const from = ghost.getBoundingClientRect()
  const copy = ghost.cloneNode(true) as HTMLElement
  document.body.appendChild(copy)
  const dx = to.left + 40 - (from.left + from.width / 2)
  const dy = to.top + to.height / 2 - (from.top + from.height / 2)
  const a = copy.animate([{ opacity: 1, transform: 'translate(0, 0) scale(.96)' }, { opacity: 0, transform: `translate(${dx}px, ${dy}px) scale(.2)` }], { duration: DUR.move, easing: EASE.move })
  const remove = () => copy.remove()
  a.onfinish = remove
  a.oncancel = remove
}

/** On cancel the ghost flies back to the row it was lifted from (200ms; skipped under Reduced motion). */
function animateGhostReturn(ghost: HTMLElement | null, id: string | undefined): void {
  if (!ghost || !id || motionReduced() || typeof ghost.animate !== 'function') return
  const row = document.querySelector<HTMLElement>(`[data-repo-id="${CSS.escape(id)}"]`)
  if (!row) return
  const to = row.getBoundingClientRect()
  const from = ghost.getBoundingClientRect()
  const copy = ghost.cloneNode(true) as HTMLElement
  document.body.appendChild(copy)
  const dx = to.left + 20 - from.left
  const dy = to.top + to.height / 2 - (from.top + from.height / 2)
  const a = copy.animate([{ opacity: 1, transform: 'translate(0, 0) rotate(-1.2deg) scale(.96)' }, { opacity: 0, transform: `translate(${dx}px, ${dy}px) rotate(0deg) scale(1)` }], { duration: DUR.move, easing: EASE.move })
  const remove = () => copy.remove()
  a.onfinish = remove
  a.oncancel = remove
}

/** Start a drag from a pointerdown on a grip. A press that never moves 4px stays a click. */
export function useRowDrag(handlers: DragHandlers): { drag: DragState | null; start: (ids: readonly string[], label: string, e: ReactPointerEvent<HTMLElement>, provider?: ProviderKind | null) => void } {
  const [drag, setDrag] = useState<DragState | null>(null)
  const h = useRef(handlers)
  h.current = handlers
  const cleanup = useRef<(() => void) | null>(null)
  useEffect(() => () => cleanup.current?.(), [])

  const start = useCallback((ids: readonly string[], label: string, e: ReactPointerEvent<HTMLElement>, provider?: ProviderKind | null) => {
    if (e.button !== 0 || cleanup.current) return
    e.preventDefault()
    const sx = e.clientX
    const sy = e.clientY
    let active = false
    let over: string | null = null

    const sectionAt = (x: number, y: number): string | null => {
      const el = document.elementFromPoint(x, y)
      const sec = el?.closest<HTMLElement>('[data-drop]')
      const id = sec?.dataset.drop
      return id === undefined || !h.current.valid(ids, id) ? null : id
    }
    const end = () => {
      window.removeEventListener('pointermove', move)
      window.removeEventListener('pointerup', up)
      window.removeEventListener('pointercancel', cancel)
      window.removeEventListener('keydown', key, true)
      document.body.classList.remove('is-dragging-row')
      cleanup.current = null
      setDrag(null)
    }
    function move(ev: PointerEvent) {
      if (!active) {
        if (Math.hypot(ev.clientX - sx, ev.clientY - sy) < THRESHOLD) return
        active = true
        document.body.classList.add('is-dragging-row')
        h.current.onStart(ids)
      }
      const next = sectionAt(ev.clientX, ev.clientY)
      if (next !== over) {
        over = next
        h.current.onOver(next)
      }
      setDrag({ ids, label, provider, x: ev.clientX, y: ev.clientY, over: next })
    }
    function up(ev: PointerEvent) {
      const target = active ? sectionAt(ev.clientX, ev.clientY) : null
      const wasActive = active
      const rect = target === null ? null : document.querySelector<HTMLElement>(`[data-drop="${CSS.escape(target)}"] .proj__head`)?.getBoundingClientRect() ?? null
      const ghost = document.querySelector<HTMLElement>('.drag-ghost')
      end()
      if (!wasActive) return
      if (target !== null) {
        if (ghost && rect) animateGhostDrop(ghost, rect)
        h.current.onDrop(ids, target)
      } else {
        animateGhostReturn(ghost, ids[0])
        h.current.onCancel(ids)
      }
    }
    function cancel() {
      const was = active
      const ghost = document.querySelector<HTMLElement>('.drag-ghost')
      end()
      if (was) {
        animateGhostReturn(ghost, ids[0])
        h.current.onCancel(ids)
      }
    }
    function key(ev: KeyboardEvent) {
      if (ev.key === 'Escape') {
        ev.preventDefault()
        ev.stopPropagation()
        cancel()
      }
    }
    window.addEventListener('pointermove', move)
    window.addEventListener('pointerup', up)
    window.addEventListener('pointercancel', cancel)
    window.addEventListener('keydown', key, true)
    cleanup.current = end
  }, [])

  return { drag, start }
}

/** The ghost under the pointer: lifted (120ms to shadow, 1.2 degree tilt, 96% scale); Reduced motion keeps the ring but no lift. */
export function DragGhost({ drag, extra, moveLabel }: { drag: DragState | null; extra: number; moveLabel: string | null }) {
  const ref = useRef<HTMLDivElement>(null)
  const present = !!drag
  useLayoutEffect(() => {
    const el = ref.current
    if (!present || !el || motionReduced() || typeof el.animate !== 'function') return
    el.animate([{ transform: 'none', boxShadow: 'none' }, { transform: 'rotate(-1.2deg) scale(.96)', boxShadow: 'var(--shadow-3)' }], { duration: 120, easing: EASE.enter })
  }, [present])
  if (!drag) return null
  const reduced = motionReduced()
  return createPortal(
    <div ref={ref} className="drag-ghost" style={{ left: drag.x + 14, top: drag.y + 10, transform: reduced ? 'none' : 'rotate(-1.2deg) scale(.96)', boxShadow: reduced ? 'none' : undefined }} aria-hidden="true">
      {drag.provider && <ProviderMark provider={drag.provider} />}
      <span>{drag.label}</span>
      {extra > 0 && <span className="drag-ghost__n">+{extra}</span>}
      {moveLabel && <span className="badge badge--info">{moveLabel}</span>}
    </div>,
    document.body,
  )
}
