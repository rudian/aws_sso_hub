import { afterEach, describe, expect, it } from 'vitest'
import {
  MOTION_ROW_LIMIT,
  animateInsert,
  animateRemove,
  applyMotionMode,
  applyMotionPref,
  currentMotionMode,
  listMotionAllowed,
  measureFlip,
  playFlip,
  resolveReduced,
} from './motion'

afterEach(() => {
  document.documentElement.removeAttribute('data-motion')
  document.body.innerHTML = ''
})

describe('motion mode', () => {
  it('resolves System, Full and Reduced', () => {
    expect(resolveReduced('system', true)).toBe(true)
    expect(resolveReduced('system', false)).toBe(false)
    expect(resolveReduced('full', true)).toBe(false)
    expect(resolveReduced('reduce', false)).toBe(true)
  })

  it('writes and clears data-motion', () => {
    applyMotionMode('reduce')
    expect(document.documentElement.getAttribute('data-motion')).toBe('reduce')
    expect(currentMotionMode()).toBe('reduce')
    applyMotionMode('full')
    expect(document.documentElement.getAttribute('data-motion')).toBe('full')
    applyMotionMode('system')
    expect(document.documentElement.hasAttribute('data-motion')).toBe(false)
    expect(currentMotionMode()).toBe('system')
  })

  it('accepts the API enum values and treats anything else as System', () => {
    applyMotionPref('reduce')
    expect(currentMotionMode()).toBe('reduce')
    applyMotionPref('full')
    expect(currentMotionMode()).toBe('full')
    applyMotionPref('')
    expect(currentMotionMode()).toBe('system')
    applyMotionPref(undefined)
    expect(currentMotionMode()).toBe('system')
  })
})

describe('list motion', () => {
  it('is skipped above 200 rows and under Reduced', () => {
    applyMotionMode('full')
    expect(MOTION_ROW_LIMIT).toBe(200)
    expect(listMotionAllowed(200)).toBe(true)
    expect(listMotionAllowed(201)).toBe(false)
    applyMotionMode('reduce')
    expect(listMotionAllowed(3)).toBe(false)
  })

  it('animateInsert starts one animation when allowed and none when skipped', () => {
    const calls: Keyframe[][] = []
    const el = document.createElement('div')
    ;(el as unknown as { animate: unknown }).animate = (kf: Keyframe[]) => {
      calls.push(kf)
      return {} as Animation
    }
    applyMotionMode('full')
    expect(animateInsert(el, 10)).not.toBeNull()
    expect(animateInsert(el, 500)).toBeNull()
    applyMotionMode('reduce')
    expect(animateInsert(el, 10)).toBeNull()
    expect(calls).toHaveLength(1)
    expect(Object.keys(calls[0][0]).sort()).toEqual(['opacity', 'transform'])
  })

  it('animateRemove resolves at once when skipped and marks the row leaving otherwise', async () => {
    const el = document.createElement('div')
    applyMotionMode('reduce')
    await animateRemove(el, 5)
    expect(el.classList.contains('is-leaving')).toBe(false)
    applyMotionMode('full')
    const p = animateRemove(el, 5)
    expect(el.classList.contains('is-leaving')).toBe(true)
    await p
    const big = document.createElement('div')
    await animateRemove(big, 1000)
    expect(big.classList.contains('is-leaving')).toBe(false)
  })

  function rect(left: number, top: number): DOMRect {
    return { left, top, right: left, bottom: top, width: 0, height: 0, x: left, y: top, toJSON: () => ({}) } as DOMRect
  }
  function list(positions: Record<string, [number, number]>) {
    const host = document.createElement('div')
    const animated: Array<{ key: string; frames: Keyframe[] }> = []
    for (const [key, [l, t]] of Object.entries(positions)) {
      const el = document.createElement('div')
      el.dataset.flipKey = key
      el.getBoundingClientRect = () => rect(l, t)
      ;(el as unknown as { animate: unknown }).animate = (frames: Keyframe[]) => {
        animated.push({ key, frames })
        return {} as Animation
      }
      host.appendChild(el)
    }
    document.body.appendChild(host)
    return { host, animated }
  }

  it('FLIP moves only rows that changed place, with transform only', () => {
    applyMotionMode('full')
    const before = measureFlip(list({ a: [0, 0], b: [0, 40], c: [0, 80] }).host)
    expect([...before.keys()]).toEqual(['a', 'b', 'c'])
    const after = list({ b: [0, 0], a: [0, 40], c: [0, 80], d: [0, 120] })
    const started = playFlip(after.host, before)
    expect(after.animated.map((x) => x.key).sort()).toEqual(['a', 'b'])
    expect(started).toHaveLength(2)
    const a = after.animated.find((x) => x.key === 'a')!
    expect(a.frames[0]).toEqual({ transform: 'translate(0px, -40px)' })
    expect(Object.keys(a.frames[0])).toEqual(['transform'])
  })

  it('FLIP does nothing above 200 rows or under Reduced', () => {
    const many: Record<string, [number, number]> = {}
    for (let i = 0; i < 201; i++) many[`k${i}`] = [0, i * 10]
    applyMotionMode('full')
    const big = list(many)
    expect(playFlip(big.host, new Map([['k0', rect(0, 999)]]))).toEqual([])
    applyMotionMode('reduce')
    const small = list({ a: [0, 0] })
    expect(playFlip(small.host, new Map([['a', rect(0, 50)]]))).toEqual([])
  })
})
