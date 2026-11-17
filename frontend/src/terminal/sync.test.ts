import { describe, expect, it } from 'vitest'
import { b64ToBytes, bytesToB64 } from './b64'
import { FakeTerminalClient } from './fakeClient'
import { TerminalSync, NUDGE_DELAY_MS, NUDGE_MIN_INTERVAL_MS } from './sync'
import type { SyncSink } from './types'

const dec = (u: Uint8Array) => new TextDecoder().decode(u)

function setup(opts: { gate?: () => boolean; maxRead?: number } = {}) {
  const client = new FakeTerminalClient()
  const screen: string[] = []
  const events: string[] = []
  let clock = 10_000
  const sink: SyncSink = {
    write: async (b) => void (screen.push(dec(b)), events.push('w:' + dec(b))),
    reset: () => void (screen.length = 0, events.push('reset')),
    size: () => ({ cols: 80, rows: 24 }),
  }
  const sleeps: number[] = []
  const sync = new TerminalSync({
    session: 's1',
    client,
    sink,
    gate: opts.gate ?? (() => true),
    maxRead: opts.maxRead,
    now: () => clock,
    sleep: async (ms) => void (sleeps.push(ms), (clock += ms)),
  })
  return { client, sync, screen, events, sleeps, advance: (ms: number) => (clock += ms) }
}

describe('base64 helpers', () => {
  it('round-trips bytes including high values and large arrays', () => {
    const u = new Uint8Array(100_000).map((_, i) => i % 256)
    expect(b64ToBytes(bytesToB64(u))).toEqual(u)
  })
})

describe('TerminalSync', () => {
  it('replays history from offset 0, then follows; replaying clears once the first tail is reached', async () => {
    const { client, sync, screen } = setup()
    client.emit('s1', 'hello ', false)
    client.emit('s1', 'world', false)
    await sync.start()
    expect(screen.join('')).toBe('hello world')
    expect(sync.offset).toBe(11)
    expect(sync.replaying).toBe(false)
    client.emit('s1', '!', false)
    sync.ping({ session: 's1', tail: 12, done: false })
    await sync.pump()
    expect(screen.join('')).toBe('hello world!')
  })

  it('reads in chunks of at most maxRead and stays in replay until it has read up to the first tail', async () => {
    const { client, sync } = setup({ maxRead: 4 })
    client.emit('s1', '0123456789', false)
    const seen: boolean[] = []
    const p = sync.start()
    seen.push(sync.replaying)
    await p
    expect(client.reads.map((r) => r.offset)).toEqual([0, 4, 8])
    expect(sync.replaying).toBe(false)
    expect(seen[0]).toBe(true)
  })

  it('ignores pings for other sessions and pings that report nothing new', async () => {
    const { client, sync } = setup()
    client.emit('s1', 'abc', false)
    await sync.start()
    const n = client.reads.length
    sync.ping({ session: 'other', tail: 99, done: false })
    sync.ping({ session: 's1', tail: 3, done: false })
    await new Promise((r) => setTimeout(r, 0))
    expect(client.reads.length).toBe(n)
  })

  it('does not read while the gate is closed, and resumes on kick (no timer)', async () => {
    let open = false
    const { client, sync, screen } = setup({ gate: () => open })
    client.emit('s1', 'abc', false)
    await sync.start()
    expect(client.reads.length).toBe(0)
    client.emit('s1', 'def', false)
    sync.ping({ session: 's1', tail: 6, done: false })
    await sync.pump()
    expect(client.reads.length).toBe(0)
    open = true
    sync.kick()
    await sync.pump()
    await new Promise((r) => setTimeout(r, 0))
    expect(screen.join('')).toBe('abcdef')
  })

  it('on a gap: reset, prologue, data, then one nudge (cols-1, 120 ms, cols)', async () => {
    const { client, sync, events, sleeps } = setup()
    client.emit('s1', 'AAAAAAAAAA', false)
    client.emit('s1', 'BBBB', false)
    client.trim('s1', 10, '\x1b[?1049h')
    await sync.start(0)
    expect(events).toEqual(['reset', 'w:\x1b[?1049h', 'w:BBBB'])
    expect(sync.gaps).toBe(1)
    expect(sync.skipped).toBe(10)
    expect(client.resizes).toEqual([
      { session: 's1', cols: 79, rows: 24 },
      { session: 's1', cols: 80, rows: 24 },
    ])
    expect(sleeps).toEqual([NUDGE_DELAY_MS])
  })

  it('a gap puts the sync back into replay until the tail it saw is read', async () => {
    const { client, sync } = setup({ maxRead: 3 })
    client.emit('s1', 'abc', false)
    await sync.start()
    expect(sync.replaying).toBe(false)
    client.emit('s1', 'defghijkl', false)
    client.trim('s1', 6)
    let during: boolean | undefined
    const orig = client.read.bind(client)
    client.read = async (s, o, m) => {
      const r = await orig(s, o, m)
      during = sync.replaying
      return r
    }
    await sync.pump()
    expect(during).toBe(true)
    expect(sync.replaying).toBe(false)
    expect(sync.gaps).toBe(1)
  })

  it('nudges at most once every 2 s', async () => {
    const { client, sync, advance } = setup()
    client.emit('s1', 'x', false)
    await sync.start()
    // first gap: nudge
    client.emit('s1', 'yyyy', false)
    client.trim('s1', 3)
    await sync.pump()
    expect(sync.nudges).toBe(1)
    // second gap 120 ms later (the nudge slept 120): rate limited
    client.emit('s1', 'zzzz', false)
    client.trim('s1', 4)
    await sync.pump()
    expect(sync.gaps).toBe(2)
    expect(sync.nudges).toBe(1)
    // after 2 s the next gap nudges again
    advance(NUDGE_MIN_INTERVAL_MS)
    client.emit('s1', 'wwww', false)
    client.trim('s1', 4)
    await sync.pump()
    expect(sync.gaps).toBe(3)
    expect(sync.nudges).toBe(2)
  })

  it('stops pulling once the session is done and every byte was read', async () => {
    let done = 0
    const client = new FakeTerminalClient()
    const sync = new TerminalSync({ session: 's1', client, sink: { write: async () => {}, reset() {}, size: () => ({ cols: 80, rows: 24 }) }, gate: () => true, onDone: () => done++ })
    client.emit('s1', 'bye', false)
    client.end('s1')
    await sync.start()
    expect(sync.done).toBe(true)
    expect(done).toBe(1)
    const n = client.reads.length
    sync.kick()
    await sync.pump()
    expect(client.reads.length).toBe(n)
  })

  it('a failed read is reported and the next ping retries', async () => {
    const errors: unknown[] = []
    const client = new FakeTerminalClient()
    const screen: string[] = []
    const sync = new TerminalSync({ session: 's1', client, sink: { write: async (b) => void screen.push(dec(b)), reset() {}, size: () => ({ cols: 80, rows: 24 }) }, gate: () => true, onError: (e) => errors.push(e) })
    client.emit('s1', 'abc', false)
    client.failNextRead = new Error('boom')
    await sync.start()
    expect(errors).toHaveLength(1)
    sync.ping({ session: 's1', tail: 3, done: false })
    await sync.pump()
    expect(screen.join('')).toBe('abc')
  })
})
