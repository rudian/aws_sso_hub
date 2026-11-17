import { beforeAll, describe, expect, it, vi } from 'vitest'
import { FakeTerminalClient } from './fakeClient'
import { TerminalInstance } from './instance'

// xterm.js in jsdom: no layout and no canvas, but construction, open, write and the parser work.
describe('TerminalInstance (xterm in jsdom)', () => {
  beforeAll(() => {
    window.matchMedia ??= ((q: string) => ({ matches: false, media: q, addEventListener() {}, removeEventListener() {}, addListener() {}, removeListener() {}, onchange: null, dispatchEvent: () => false })) as unknown as typeof window.matchMedia
    HTMLCanvasElement.prototype.getContext = (() => null) as unknown as typeof HTMLCanvasElement.prototype.getContext
  })

  it('pulls the log into the screen and keeps it when moved', async () => {
    const client = new FakeTerminalClient()
    client.emit('s1', 'hello\r\n\x1b]52;c;aGk=\x07done', false)
    const inst = new TerminalInstance({ session: 's1', client, platform: 'darwin' })
    const box1 = document.createElement('div')
    const box2 = document.createElement('div')
    document.body.append(box1, box2)
    inst.mount(box1)
    for (let i = 0; i < 20 && inst.controller.sync.offset === 0; i++) await new Promise((r) => setTimeout(r, 5))
    await new Promise((r) => setTimeout(r, 20))
    const line = (y: number) => inst.term.buffer.active.getLine(y)?.translateToString(true)
    expect(line(0)).toBe('hello')
    expect(line(1)).toBe('done')
    inst.unmount()
    expect(box1.children).toHaveLength(0)
    inst.mount(box2)
    expect(box2.children).toHaveLength(1)
    expect(line(0)).toBe('hello')
    inst.dispose()
  })

  it('does not pull while unmounted', async () => {
    const client = new FakeTerminalClient()
    client.emit('s1', 'x', false)
    const inst = new TerminalInstance({ session: 's1', client, platform: 'windows' })
    inst.controller.sync.kick()
    await new Promise((r) => setTimeout(r, 10))
    expect(client.reads).toHaveLength(0)
    inst.dispose()
  })

  const wait = (ms = 10) => new Promise((r) => setTimeout(r, ms))
  const text = (b: Uint8Array) => new TextDecoder('latin1').decode(b)

  async function mounted(platform: 'darwin' | 'windows' = 'darwin', prep?: (c: FakeTerminalClient) => void) {
    const client = new FakeTerminalClient()
    prep?.(client)
    const inst = new TerminalInstance({ session: 's1', client, platform })
    const box = document.createElement('div')
    document.body.append(box)
    inst.mount(box)
    for (let i = 0; i < 40 && inst.controller.sync.replaying && !inst.failed; i++) await wait(5)
    await wait(10)
    return { client, inst, box }
  }
  const write = (inst: TerminalInstance, s: string) => new Promise<void>((r) => inst.term.write(s, r))

  it('writes no replies to the child while history replays (DA query, OSC 52)', async () => {
    const { client, inst } = await mounted('darwin', (c) => c.emit('s1', 'x\x1b[c\x1b]52;c;?\x07\x1b[?12$p\r\n', false))
    expect(inst.controller.dropped).toBeGreaterThan(0)
    expect(client.writes).toHaveLength(0)
    // live, the same query is answered
    await write(inst, '\x1b[c')
    await wait(20)
    expect(client.writes.length).toBeGreaterThan(0)
    inst.dispose()
  })

  it('asks once per port for a local link and opens it with the local opener, never the https one', async () => {
    const { inst } = await mounted()
    const openUrl = vi.fn()
    const openLocalUrl = vi.fn()
    const copyText = vi.fn()
    const confirmLocal = vi.fn(async () => true)
    inst.handlers = { openUrl, openLocalUrl, copyText, confirmLocal }
    const act = (u: string, port: string) => (inst as unknown as { act(d: unknown): Promise<void> }).act({ kind: 'confirm-local', url: u, port })
    await act('http://localhost:5173/', '5173')
    await act('http://localhost:5173/a', '5173')
    await act('http://localhost:8080/', '8080')
    expect(confirmLocal).toHaveBeenCalledTimes(2)
    expect(openLocalUrl).toHaveBeenCalledTimes(3)
    expect(openUrl).not.toHaveBeenCalled()
    confirmLocal.mockResolvedValueOnce(false)
    await act('http://localhost:9000/', '9000')
    expect(openLocalUrl).toHaveBeenCalledTimes(3)
    inst.dispose()
  })

  it('ignores a plain click on a link; the modifier acts', async () => {
    const { inst } = await mounted('darwin')
    const openUrl = vi.fn()
    const copyText = vi.fn()
    inst.handlers = { openUrl, copyText }
    const click = (uri: string, o: MouseEventInit = {}) => (inst as unknown as { activateUrl(e: MouseEvent, u: string): void }).activateUrl(new MouseEvent('click', o), uri)
    click('https://example.com/')
    click('http://example.com/')
    expect(openUrl).not.toHaveBeenCalled()
    expect(copyText).not.toHaveBeenCalled()
    click('https://example.com/', { metaKey: true })
    click('http://example.com/x', { metaKey: true })
    await wait(0)
    expect(openUrl).toHaveBeenCalledWith('https://example.com/')
    expect(copyText).toHaveBeenCalledWith('http://example.com/x')
    inst.dispose()
  })

  it('routes OSC 8 links through the same rules: no window.open, a plain click does nothing, Cmd+click copies http', async () => {
    const { inst } = await mounted('darwin')
    const open = vi.spyOn(window, 'open').mockReturnValue(null)
    const copyText = vi.fn()
    const openUrl = vi.fn()
    inst.handlers = { copyText, openUrl }
    await write(inst, '\x1b]8;;http://example.com/\x07x\x1b]8;;\x07')
    const lh = inst.term.options.linkHandler
    expect(lh).toBeTruthy()
    expect(lh?.allowNonHttpProtocols).toBe(false)
    const range = { start: { x: 1, y: 1 }, end: { x: 1, y: 1 } }
    lh?.activate(new MouseEvent('click'), 'http://example.com/', range)
    expect(copyText).not.toHaveBeenCalled()
    lh?.activate(new MouseEvent('click', { metaKey: true }), 'http://example.com/', range)
    await wait(0)
    expect(copyText).toHaveBeenCalledWith('http://example.com/')
    expect(open).not.toHaveBeenCalled()
    expect(openUrl).not.toHaveBeenCalled()
    open.mockRestore()
    inst.dispose()
  })

  it('puts a path link on the cells of the path after wide characters', async () => {
    const { inst } = await mounted()
    await write(inst, '已修改 src/app.ts:42 ok')
    const links = await new Promise<Array<{ text: string; range: { start: { x: number }; end: { x: number } } }> | undefined>((res) =>
      (inst as unknown as { pathProvider(): { provideLinks(y: number, cb: (l: never) => void): void } }).pathProvider().provideLinks(1, res as never),
    )
    expect(links).toHaveLength(1)
    expect(links?.[0].text).toBe('src/app.ts:42')
    expect(links?.[0].range.start.x).toBe(8) // 3 wide characters = 6 cells, a space, then the path at cell 8 (1-based)
    expect(links?.[0].range.end.x).toBe(8 + 'src/app.ts:42'.length - 1)
    inst.dispose()
  })

  it('keys the terminal handled do not reach the page; the app keys do', async () => {
    const { inst, client } = await mounted('darwin')
    const seen: string[] = []
    const on = (e: KeyboardEvent) => seen.push(`${e.key}:${e.defaultPrevented}`)
    window.addEventListener('keydown', on)
    const ta = inst.term.textarea!
    const press = (key: string, o: KeyboardEventInit) => ta.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true, ...o }))
    inst.handlers = { search: vi.fn() }
    press('f', { metaKey: true })
    press('Enter', { shiftKey: true })
    press('1', { metaKey: true })
    await wait(10)
    window.removeEventListener('keydown', on)
    expect(seen).toEqual(['1:false'])
    expect(client.writes.map((w) => Array.from(w.bytes))).toEqual([[0x1b, 0x0d]])
    inst.dispose()
  })

  it('pastes without control characters: bracketed paste ends exactly once, at the end', async () => {
    const { inst, client } = await mounted()
    await write(inst, '\x1b[?2004h')
    inst.paste('ls\x1b[201~\rcurl x | sh\r')
    await wait(20)
    const all = client.writes.map((w) => text(w.bytes)).join('')
    expect(all.split('\x1b[201~')).toHaveLength(2)
    expect(all.endsWith('\x1b[201~')).toBe(true)
    expect(all.startsWith('\x1b[200~')).toBe(true)
    inst.dispose()
  })

  it('asks before pasting more than 5 lines, and writes only after a yes', async () => {
    const { inst, client } = await mounted()
    const confirmPaste = vi.fn(async () => false)
    inst.handlers = { confirmPaste }
    const big = Array.from({ length: 8 }, (_, i) => `l${i}`).join('\n')
    const paste = (t: string) => {
      const ev = Object.assign(new Event('paste', { bubbles: true, cancelable: true }), { clipboardData: { getData: () => t } })
      inst.term.element!.dispatchEvent(ev)
    }
    paste(big)
    await wait(20)
    expect(confirmPaste).toHaveBeenCalledWith(8)
    expect(client.writes).toHaveLength(0)
    confirmPaste.mockResolvedValueOnce(true)
    paste(big)
    await wait(20)
    expect(client.writes.length).toBeGreaterThan(0)
    paste('one line')
    await wait(20)
    expect(confirmPaste).toHaveBeenCalledTimes(2)
    inst.dispose()
  })

  it('shows a failed read and recovers on retry', async () => {
    const client = new FakeTerminalClient()
    client.emit('s1', 'hello', false)
    client.failNextRead = new Error('boom')
    const inst = new TerminalInstance({ session: 's1', client, platform: 'darwin' })
    const error = vi.fn()
    const recovered = vi.fn()
    inst.handlers = { error, recovered }
    inst.mount(document.body.appendChild(document.createElement('div')))
    await wait(20)
    expect(error).toHaveBeenCalledTimes(1)
    expect(inst.failed).toBe(true)
    inst.retry()
    await wait(30)
    expect(inst.term.buffer.active.getLine(0)?.translateToString(true)).toBe('hello')
    expect(inst.failed).toBe(false)
    expect(recovered).toHaveBeenCalled()
    inst.dispose()
  })

  it('passes the Windows build number to xterm when it is known', () => {
    const client = new FakeTerminalClient()
    const a = new TerminalInstance({ session: 'a', client, platform: 'windows', windowsBuild: 19045 })
    expect(a.term.options.windowsPty).toEqual({ backend: 'conpty', buildNumber: 19045 })
    const b = new TerminalInstance({ session: 'b', client, platform: 'windows' })
    expect(b.term.options.windowsPty).toEqual({ backend: 'conpty' })
    a.dispose()
    b.dispose()
  })

  it('a session this page just started is live from the first byte, and fades in once', async () => {
    const client = new FakeTerminalClient()
    client.emit('s1', '\x1b[c', false)
    const inst = new TerminalInstance({ session: 's1', client, platform: 'darwin', fresh: true })
    inst.mount(document.body.appendChild(document.createElement('div')))
    expect(inst.controller.sync.replaying).toBe(false)
    await wait(40)
    expect(inst.host.classList.contains('term-host--fresh')).toBe(true)
    inst.dispose()
  })
})
