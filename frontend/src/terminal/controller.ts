import type { Platform } from '../lib/platform'
import { binaryStringToB64, textToB64 } from './b64'
import { mapKey, type KeyAction, type KeyLike } from './keys'
import { TerminalSync } from './sync'
import type { SyncSink, TerminalClient } from './types'

export interface ControllerOptions {
  session: string
  client: TerminalClient
  sink: SyncSink
  platform: Platform
  /** Selection state of the screen, for the copy key. */
  hasSelection: () => boolean
  /** Shown (mounted and on screen) and the page is visible. */
  gate: () => boolean
  now?: () => number
  sleep?: (ms: number) => Promise<void>
  onDone?: () => void
  onError?: (err: unknown) => void
  onRecover?: () => void
  /** The session was started by this page just now: do not treat its first output as history. */
  fresh?: boolean
}

/**
 * Everything one terminal does except draw: pulls output (TerminalSync), sends typed input in order, drops what xterm says
 * while history replays, sends size changes, maps keys. xterm is behind SyncSink, so this is testable without a DOM.
 */
export class TerminalController {
  readonly sync: TerminalSync
  private queue: Promise<void> = Promise.resolve()
  private lastSize = ''
  /** Input and key bytes dropped during replay (for tests and the hud). */
  dropped = 0

  constructor(private readonly o: ControllerOptions) {
    this.sync = new TerminalSync({
      session: o.session,
      client: o.client,
      sink: o.sink,
      gate: o.gate,
      now: o.now,
      sleep: o.sleep,
      onDone: o.onDone,
      onError: o.onError,
      onRecover: o.onRecover,
    })
  }

  /** Input is written in order; a failed write does not block the next one. */
  private send(b64: string): void {
    this.queue = this.queue.then(() => this.o.client.write(this.o.session, b64)).catch((e) => this.o.onError?.(e))
  }

  /**
   * True when input must be dropped (history is replaying, or the session ended). When a replay is stuck because a read
   * failed (no pull is running), the input is a reason to try again.
   */
  private dropInput(): boolean {
    if (!this.sync.replaying && !this.sync.done) return false
    this.dropped++
    if (this.sync.replaying && !this.sync.done && !this.sync.pumping) this.sync.kick()
    return true
  }

  /** xterm onData: typed text and the answers to queries. */
  handleData(s: string): void {
    if (this.dropInput()) return
    this.send(textToB64(s))
  }

  /** xterm onBinary: raw bytes as a latin1 string (mouse reports). Dropped during replay like onData. */
  handleBinary(s: string): void {
    if (this.dropInput()) return
    this.send(binaryStringToB64(s))
  }

  /** Decide a key event. A `send` action has already been written when this returns. */
  handleKey(e: KeyLike): KeyAction {
    const a = mapKey(e, { platform: this.o.platform, hasSelection: this.o.hasSelection() })
    if (a.kind === 'send' && !this.sync.done) this.send(textToB64(a.data))
    return a
  }

  /** The screen was resized (fit). Sent once per distinct size. */
  handleResize(cols: number, rows: number): void {
    const key = `${cols}x${rows}`
    if (key === this.lastSize || cols < 1 || rows < 1) return
    this.lastSize = key
    this.queue = this.queue.then(() => this.o.client.resize(this.o.session, cols, rows)).catch((e) => this.o.onError?.(e))
  }

  attach(): Promise<void> {
    return this.sync.start(0, !this.o.fresh)
  }

  dispose(): void {
    this.sync.stop()
  }
}
