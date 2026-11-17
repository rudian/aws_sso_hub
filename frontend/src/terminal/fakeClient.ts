import { bytesToB64 } from './b64'
import type { TerminalClient, TerminalPing, TerminalRead } from './types'

/** In-memory terminal backend for tests and the mock layer: a byte log per session, echo of what is written. */
export class FakeTerminalClient implements TerminalClient {
  logs = new Map<string, { base: number; bytes: number[]; done: boolean; prologue: string }>()
  reads: Array<{ session: string; offset: number; max: number }> = []
  writes: Array<{ session: string; bytes: Uint8Array }> = []
  resizes: Array<{ session: string; cols: number; rows: number }> = []
  echo = false
  failNextRead: Error | null = null
  private listeners = new Set<(p: TerminalPing) => void>()

  private log(s: string) {
    let l = this.logs.get(s)
    if (!l) this.logs.set(s, (l = { base: 0, bytes: [], done: false, prologue: '' }))
    return l
  }

  /** Append output as the child would print it, and ping. */
  emit(session: string, text: string | Uint8Array, ping = true): void {
    const b = typeof text === 'string' ? new TextEncoder().encode(text) : text
    const l = this.log(session)
    for (const x of b) l.bytes.push(x)
    if (ping) this.ping(session)
  }

  /** Drop the first `n` bytes of the log, as the Go ring does when it passes its cap. */
  trim(session: string, n: number, prologue = ''): void {
    const l = this.log(session)
    l.bytes.splice(0, n)
    l.base += n
    l.prologue = prologue
  }

  end(session: string): void {
    this.log(session).done = true
    this.ping(session)
  }

  tail(session: string): number {
    const l = this.log(session)
    return l.base + l.bytes.length
  }

  ping(session: string): void {
    const p: TerminalPing = { session, tail: this.tail(session), done: this.log(session).done }
    for (const cb of [...this.listeners]) cb(p)
  }

  async read(session: string, offset: number, max: number): Promise<TerminalRead> {
    this.reads.push({ session, offset, max })
    if (this.failNextRead) {
      const e = this.failNextRead
      this.failNextRead = null
      throw e
    }
    const l = this.log(session)
    const tail = l.base + l.bytes.length
    let from = offset
    let gap = 0
    if (from < l.base) {
      gap = l.base - from
      from = l.base
    }
    const end = Math.min(tail, from + max)
    const data = Uint8Array.from(l.bytes.slice(from - l.base, end - l.base))
    return {
      data: data.length ? bytesToB64(data) : '',
      from,
      next: end,
      tail,
      gap,
      done: l.done && end >= tail,
      prologue: gap > 0 && l.prologue ? bytesToB64(new TextEncoder().encode(l.prologue)) : undefined,
    }
  }

  async write(session: string, data: string): Promise<void> {
    const bin = atob(data)
    const u = new Uint8Array(bin.length)
    for (let i = 0; i < bin.length; i++) u[i] = bin.charCodeAt(i)
    this.writes.push({ session, bytes: u })
    if (this.echo) this.emit(session, u)
  }

  async resize(session: string, cols: number, rows: number): Promise<void> {
    this.resizes.push({ session, cols, rows })
  }

  onPing(cb: (p: TerminalPing) => void): () => void {
    this.listeners.add(cb)
    return () => this.listeners.delete(cb)
  }
}
