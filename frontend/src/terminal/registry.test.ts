import { describe, expect, it, vi } from 'vitest'
import { FakeTerminalClient } from './fakeClient'
import type { TerminalInstance } from './instance'
import { TerminalRegistry } from './registry'

function fakeInstance(session: string) {
  const sync = { ping: vi.fn(), kick: vi.fn() }
  return { session, controller: { sync }, pageVisibilityChanged: vi.fn(() => sync.kick()), dispose: vi.fn(), setScreenReader: vi.fn() } as unknown as TerminalInstance & {
    controller: { sync: typeof sync }
    dispose: ReturnType<typeof vi.fn>
  }
}

describe('TerminalRegistry', () => {
  it('returns the same instance for a session and routes pings to it', () => {
    const client = new FakeTerminalClient()
    const made: ReturnType<typeof fakeInstance>[] = []
    const reg = new TerminalRegistry(client, (o) => {
      const i = fakeInstance(o.session)
      made.push(i)
      return i
    })
    const a = reg.acquire('a')
    expect(reg.acquire('a')).toBe(a)
    reg.acquire('b')
    expect(made).toHaveLength(2)
    client.emit('a', 'x')
    expect(made[0].controller.sync.ping).toHaveBeenCalledTimes(1)
    expect(made[1].controller.sync.ping).not.toHaveBeenCalled()
  })

  it('visibilitychange kicks every instance; release disposes and drops the listeners with the last one', () => {
    const client = new FakeTerminalClient()
    const made: ReturnType<typeof fakeInstance>[] = []
    const reg = new TerminalRegistry(client, (o) => (made.push(fakeInstance(o.session)), made[made.length - 1]))
    reg.acquire('a')
    reg.acquire('b')
    document.dispatchEvent(new Event('visibilitychange'))
    expect(made.map((m) => m.controller.sync.kick.mock.calls.length)).toEqual([1, 1])
    reg.release('a')
    expect(made[0].dispose).toHaveBeenCalled()
    expect(reg.size).toBe(1)
    reg.release('b')
    document.dispatchEvent(new Event('visibilitychange'))
    expect(made[1].controller.sync.kick).toHaveBeenCalledTimes(1)
    client.emit('b', 'x')
    expect(made[1].controller.sync.ping).not.toHaveBeenCalled()
  })
})

describe('no terminal text becomes HTML', () => {
  it('the terminal folder never uses innerHTML, outerHTML, insertAdjacentHTML, document.write or dangerouslySetInnerHTML', () => {
    const files = import.meta.glob(['./*.ts', './*.tsx', '!./*.test.ts'], { query: '?raw', import: 'default', eager: true }) as Record<string, string>
    expect(Object.keys(files).length).toBeGreaterThan(8)
    for (const [f, src] of Object.entries(files)) expect(src, f).not.toMatch(/innerHTML|outerHTML|insertAdjacentHTML|document\.write|dangerouslySetInnerHTML/)
  })
})

describe('TerminalRegistry defaults', () => {
  it('gives every new instance the Windows build, unless the caller passes its own', () => {
    const seen: Array<number | undefined> = []
    const reg = new TerminalRegistry(new FakeTerminalClient(), (o) => {
      seen.push(o.windowsBuild)
      return fakeInstance(o.session)
    }, { windowsBuild: 19045 })
    reg.acquire('a')
    reg.acquire('b', { windowsBuild: 22631 })
    expect(seen).toEqual([19045, 22631])
  })
})
