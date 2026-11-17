import { describe, expect, it } from 'vitest'
import { TerminalController } from './controller'
import { FakeTerminalClient } from './fakeClient'

function setup(platform: 'darwin' | 'windows' = 'darwin', hasSelection = false) {
  const client = new FakeTerminalClient()
  const c = new TerminalController({
    session: 's1',
    client,
    platform,
    hasSelection: () => hasSelection,
    gate: () => true,
    sink: { write: async () => {}, reset() {}, size: () => ({ cols: 80, rows: 24 }) },
    sleep: async () => {},
  })
  return { client, c }
}
const flush = () => new Promise((r) => setTimeout(r, 0))
const sent = (client: FakeTerminalClient) => client.writes.map((w) => new TextDecoder('latin1').decode(w.bytes))

describe('input during replay', () => {
  it('drops onData and onBinary while replaying, and sends them after', async () => {
    const { client, c } = setup()
    client.emit('s1', 'history', false)
    // hold the first read open so that we are mid-replay
    let release!: () => void
    const orig = client.read.bind(client)
    client.read = async (s, o, m) => {
      await new Promise<void>((r) => (release = r))
      return orig(s, o, m)
    }
    const attach = c.attach()
    await flush()
    expect(c.sync.replaying).toBe(true)
    c.handleData('\x1b[?12;2$y') // a reply to a replayed DECRQM
    c.handleBinary('\x1b[M !!') // a mouse report
    release()
    await attach
    expect(c.sync.replaying).toBe(false)
    expect(c.dropped).toBe(2)
    await flush()
    expect(client.writes).toHaveLength(0)
    c.handleData('ls\r')
    c.handleBinary('\xff\x00A')
    await flush()
    await flush()
    expect(client.writes.map((w) => Array.from(w.bytes))).toEqual([[0x6c, 0x73, 0x0d], [0xff, 0x00, 0x41]])
  })

  it('sends typed text as UTF-8 (Chinese input)', async () => {
    const { client, c } = setup()
    await c.attach()
    c.handleData('你好')
    await flush()
    expect(Array.from(client.writes[0].bytes)).toEqual([0xe4, 0xbd, 0xa0, 0xe5, 0xa5, 0xbd])
  })

  it('keeps input in order', async () => {
    const { client, c } = setup()
    await c.attach()
    c.handleData('a')
    c.handleData('b')
    c.handleData('c')
    await flush()
    await flush()
    expect(sent(client)).toEqual(['a', 'b', 'c'])
  })
})

describe('keys', () => {
  const key = (k: string, extra: Partial<KeyboardEventInit> = {}, type = 'keydown') => ({ type, key: k, shiftKey: false, ctrlKey: false, metaKey: false, altKey: false, ...extra })

  it('Shift+Enter writes ESC CR and xterm skips the key', async () => {
    const { client, c } = setup()
    await c.attach()
    expect(c.handleKey(key('Enter', { shiftKey: true })).kind).toBe('send')
    await flush()
    expect(Array.from(client.writes[0].bytes)).toEqual([0x1b, 0x0d])
    // key press / key up of the same key do not send a second time
    expect(c.handleKey(key('Enter', { shiftKey: true }, 'keypress')).kind).toBe('swallow')
    expect(c.handleKey(key('Enter', { shiftKey: true }, 'keyup')).kind).toBe('swallow')
    await flush()
    expect(client.writes).toHaveLength(1)
  })

  it('plain Enter is left to xterm', () => {
    const { c } = setup()
    expect(c.handleKey(key('Enter')).kind).toBe('native')
  })

  it('copy: Cmd+C on macOS and Ctrl+C on Windows copy only with a selection', () => {
    expect(setup('darwin', true).c.handleKey(key('c', { metaKey: true })).kind).toBe('copy')
    expect(setup('windows', true).c.handleKey(key('c', { ctrlKey: true })).kind).toBe('copy')
    expect(setup('windows', false).c.handleKey(key('c', { ctrlKey: true })).kind).toBe('native') // interrupt
    expect(setup('windows', false).c.handleKey(key('C', { ctrlKey: true, shiftKey: true })).kind).toBe('swallow')
  })

  it('Cmd/Ctrl+F opens search', () => {
    expect(setup('darwin').c.handleKey(key('f', { metaKey: true })).kind).toBe('search')
    expect(setup('windows').c.handleKey(key('f', { ctrlKey: true })).kind).toBe('search')
  })

  it('app keys pass to the page: Ctrl+`, Cmd+1, Cmd+J (mac), Ctrl+Shift+J (win); Ctrl+J on Windows stays in the terminal', () => {
    const mac = setup('darwin').c
    const win = setup('windows').c
    expect(mac.handleKey(key('`', { ctrlKey: true })).kind).toBe('app')
    expect(mac.handleKey(key('1', { metaKey: true })).kind).toBe('app')
    expect(mac.handleKey(key('j', { metaKey: true })).kind).toBe('app')
    expect(mac.handleKey(key('o', { metaKey: true })).kind).toBe('app')
    expect(mac.handleKey(key('N', { metaKey: true, shiftKey: true })).kind).toBe('app')
    expect(win.handleKey(key('J', { ctrlKey: true, shiftKey: true })).kind).toBe('app')
    expect(win.handleKey(key('j', { ctrlKey: true })).kind).toBe('native')
    expect(win.handleKey(key('o', { ctrlKey: true })).kind).toBe('native')
  })
})

describe('resize', () => {
  it('sends each distinct size once', async () => {
    const { client, c } = setup()
    c.handleResize(100, 30)
    c.handleResize(100, 30)
    c.handleResize(90, 30)
    await flush()
    await flush()
    expect(client.resizes.map((r) => `${r.cols}x${r.rows}`)).toEqual(['100x30', '90x30'])
  })
})

describe('after the session ended', () => {
  it('drops typed input', async () => {
    const { client, c } = setup()
    client.emit('s1', 'x', false)
    client.end('s1')
    await c.attach()
    c.handleData('a')
    await flush()
    expect(client.writes).toHaveLength(0)
  })
})

describe('a failed read', () => {
  it('reports it, and the next input kicks the pull so the screen comes up and later input is sent', async () => {
    const written: string[] = []
    const client = new FakeTerminalClient()
    const errors: unknown[] = []
    let recovered = 0
    const c = new TerminalController({
      session: 's1',
      client,
      platform: 'darwin',
      hasSelection: () => false,
      gate: () => true,
      sink: { write: async (b) => void written.push(new TextDecoder().decode(b)), reset() {}, size: () => ({ cols: 80, rows: 24 }) },
      sleep: async () => {},
      onError: (e) => errors.push(e),
      onRecover: () => recovered++,
    })
    client.emit('s1', 'prompt$ ', false)
    client.failNextRead = new Error('boom')
    await c.attach()
    expect(errors).toHaveLength(1)
    expect(c.sync.replaying).toBe(true)
    expect(written).toEqual([])
    c.handleData('ls\r') // dropped, but it restarts the pull
    await flush()
    await flush()
    expect(written).toEqual(['prompt$ '])
    expect(c.sync.replaying).toBe(false)
    expect(recovered).toBe(1)
    c.handleData('ls\r')
    await flush()
    await flush()
    expect(sent(client)).toEqual(['ls\r'])
  })
})

describe('a session this page just started', () => {
  it('starts live: answers to startup queries are not dropped', async () => {
    const client = new FakeTerminalClient()
    const c = new TerminalController({
      session: 's1',
      client,
      platform: 'darwin',
      hasSelection: () => false,
      gate: () => true,
      sink: { write: async () => {}, reset() {}, size: () => ({ cols: 80, rows: 24 }) },
      sleep: async () => {},
      fresh: true,
    })
    client.emit('s1', '\x1b[c', false)
    const p = c.attach()
    expect(c.sync.replaying).toBe(false)
    c.handleData('\x1b[?62c')
    await p
    await flush()
    expect(sent(client)).toEqual(['\x1b[?62c'])
  })
})
