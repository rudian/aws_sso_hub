import { act, createElement, useRef } from 'react'
import { createRoot } from 'react-dom/client'
import type { Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { applyMotionMode, playFlip, useFlip } from './motion'

;(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true

const ROW = 40
let host: HTMLDivElement
let root: Root
let scroll = 0
let animated: string[] = []
const protoRect = HTMLElement.prototype.getBoundingClientRect
const protoAnimate = (HTMLElement.prototype as unknown as { animate?: unknown }).animate

function Rows({ order }: { order: string[] }) {
  const ref = useRef<HTMLDivElement>(null)
  useFlip(ref, order)
  return createElement(
    'div',
    { ref },
    order.map((k) => createElement('div', { key: k, 'data-flip-key': k })),
  )
}

const show = (order: string[]) => act(() => root.render(createElement(Rows, { order })))

beforeEach(() => {
  applyMotionMode('full')
  scroll = 0
  animated = []
  // Layout by DOM position: a row's top is its index among its siblings, shifted by the scroll offset.
  HTMLElement.prototype.getBoundingClientRect = function (this: HTMLElement) {
    const i = this.parentElement ? Array.from(this.parentElement.children).indexOf(this) : 0
    const top = i * ROW - scroll
    return { left: 0, top, right: 0, bottom: top + ROW, width: 0, height: ROW, x: 0, y: top, toJSON: () => ({}) } as DOMRect
  }
  ;(HTMLElement.prototype as unknown as { animate: unknown }).animate = function (this: HTMLElement) {
    animated.push(this.dataset.flipKey ?? '?')
    return {} as Animation
  }
  host = document.createElement('div')
  document.body.appendChild(host)
  root = createRoot(host)
})
afterEach(() => {
  act(() => root.unmount())
  host.remove()
  HTMLElement.prototype.getBoundingClientRect = protoRect
  if (protoAnimate) (HTMLElement.prototype as unknown as { animate: unknown }).animate = protoAnimate
  else delete (HTMLElement.prototype as unknown as { animate?: unknown }).animate
  document.documentElement.removeAttribute('data-motion')
})

describe('useFlip', () => {
  it('animates the rows that moved, and again on the next reorder', () => {
    show(['a', 'b', 'c', 'd'])
    expect(animated).toEqual([])
    show(['b', 'a', 'c', 'd'])
    expect(animated.sort()).toEqual(['a', 'b'])
    animated = []
    show(['a', 'b', 'c', 'd'])
    expect(animated.sort()).toEqual(['a', 'b'])
  })

  it('does not turn a scroll between two reorders into motion', () => {
    show(['a', 'b', 'c', 'd', 'e'])
    scroll = 200
    show(['a', 'b', 'd', 'c', 'e'])
    expect(animated.sort()).toEqual(['c', 'd'])
  })

  it('does not animate when the order is unchanged', () => {
    show(['a', 'b'])
    scroll = 100
    show(['a', 'b'])
    expect(animated).toEqual([])
  })

  it('skips motion for a list of more than 200 rows', () => {
    const many = Array.from({ length: 201 }, (_, i) => `k${i}`)
    show(many)
    show(['k1', 'k0', ...many.slice(2)])
    expect(animated).toEqual([])
  })
})

describe('playFlip row count', () => {
  it('skips motion when the list holds more than 200 rows even if few are rendered', () => {
    const box = document.createElement('div')
    const el = document.createElement('div')
    el.dataset.flipKey = 'a'
    box.appendChild(el)
    document.body.appendChild(box)
    let n = 0
    ;(el as unknown as { animate: unknown }).animate = () => {
      n++
      return {} as Animation
    }
    el.getBoundingClientRect = () => ({ left: 0, top: 0 }) as DOMRect
    const before = new Map([['a', { left: 0, top: 50 } as DOMRect]])
    expect(playFlip(box, before, 201)).toEqual([])
    expect(n).toBe(0)
    expect(playFlip(box, before, 40)).toHaveLength(1)
    box.remove()
  })
})
