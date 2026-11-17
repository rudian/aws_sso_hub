import { b64ToBytes } from './b64'
import type { SyncSink, TerminalClient, TerminalPing } from './types'

export const READ_MAX = 256 * 1024
/** Resize-nudge after a gap: cols-1, this long, then back (S0-01 rule B). */
export const NUDGE_DELAY_MS = 120
/** At most one nudge per this long; a faster one makes the child repaint into a log that trims again (a resync loop). */
export const NUDGE_MIN_INTERVAL_MS = 2000

export interface SyncOptions {
  session: string
  client: TerminalClient
  sink: SyncSink
  /** True while this terminal may pull: it is shown, and the page is visible. Checked before every read. */
  gate: () => boolean
  now?: () => number
  sleep?: (ms: number) => Promise<void>
  maxRead?: number
  onDone?: () => void
  onError?: (err: unknown) => void
  /** A read worked again after onError */
  onRecover?: () => void
}

/**
 * Pull-by-offset loop for one terminal (plan 5, S0-01).
 *
 * - Reads only while `gate()` is true. There is no timer: it restarts on a ping, on `kick()` (visibilitychange, shown again)
 *   and on `start()`. Timers run about once a second in a hidden webview.
 * - While replaying history (attach, reload, after a gap) `replaying` is true and the owner must drop everything xterm emits
 *   through onData and onBinary: replayed queries would be answered into the live child as typed input.
 * - On a gap: reset the screen, write the prologue, write the data, then nudge the child to repaint (rate limited).
 */
export class TerminalSync {
  private off = 0
  private pumping_ = false
  private again = false
  private _replaying = false
  private replayTarget = Infinity
  private pendingNudge = false
  private lastNudge = Number.NEGATIVE_INFINITY
  private _done = false
  private stopped = false
  private failed = false
  private readonly now: () => number
  private readonly sleep: (ms: number) => Promise<void>
  private readonly maxRead: number
  gaps = 0
  skipped = 0
  nudges = 0

  constructor(private readonly o: SyncOptions) {
    this.now = o.now ?? (() => performance.now())
    this.sleep = o.sleep ?? ((ms) => new Promise((r) => setTimeout(r, ms)))
    this.maxRead = o.maxRead ?? READ_MAX
  }

  get offset(): number {
    return this.off
  }
  get replaying(): boolean {
    return this._replaying
  }
  get pumping(): boolean {
    return this.pumping_
  }
  get done(): boolean {
    return this._done
  }

  /**
   * Attach to a session from the start of its log (a new page, or a reload): replay history, then follow.
   * `replay: false` is for a session this page has just started: its first bytes are live output, so the answers to
   * its startup queries must reach the child.
   */
  start(fromOffset = 0, replay = true): Promise<void> {
    this.off = fromOffset
    this.stopped = false
    this._done = false
    this._replaying = replay
    this.replayTarget = Infinity
    return this.pump()
  }

  stop(): void {
    this.stopped = true
  }

  /** A "terminal:ping" arrived. */
  ping(p: TerminalPing): void {
    if (p.session !== this.o.session || this.stopped) return
    if (p.tail <= this.off && !(p.done && !this._done)) return
    void this.pump()
  }

  /** The gate may have opened (visibilitychange, the terminal was shown). */
  kick(): void {
    if (this.stopped || !this.o.gate()) return
    void this.pump()
  }

  pump(): Promise<void> {
    if (this.pumping_) {
      this.again = true
      return Promise.resolve()
    }
    return this.run()
  }

  private async run(): Promise<void> {
    this.pumping_ = true
    try {
      do {
        this.again = false
        await this.drain()
        if (this.pendingNudge) {
          this.pendingNudge = false
          if (this.now() - this.lastNudge >= NUDGE_MIN_INTERVAL_MS) {
            this.lastNudge = this.now()
            await this.nudge()
          }
        }
      } while (this.again && !this.stopped)
    } catch (err) {
      this.failed = true
      this.o.onError?.(err)
    } finally {
      this.pumping_ = false
    }
  }

  private async drain(): Promise<void> {
    while (!this.stopped && !this._done && this.o.gate()) {
      const r = await this.o.client.read(this.o.session, this.off, this.maxRead)
      if (this.failed) {
        this.failed = false
        this.o.onRecover?.()
      }
      if (this._replaying && this.replayTarget === Infinity) this.replayTarget = r.tail
      if (r.gap > 0) {
        this.gaps++
        this.skipped += r.gap
        this.o.sink.reset()
        if (r.prologue) await this.o.sink.write(b64ToBytes(r.prologue))
        this.pendingNudge = true
        this._replaying = true
        this.replayTarget = r.tail
      }
      if (r.data) await this.o.sink.write(b64ToBytes(r.data))
      this.off = r.next
      if (this._replaying && this.off >= this.replayTarget) this._replaying = false
      if (r.done && r.next >= r.tail) {
        this._done = true
        this._replaying = false
        this.o.onDone?.()
        return
      }
      if (r.next >= r.tail) return
    }
  }

  private async nudge(): Promise<void> {
    const { cols, rows } = this.o.sink.size()
    if (cols < 2) return
    this.nudges++
    await this.o.client.resize(this.o.session, cols - 1, rows)
    await this.sleep(NUDGE_DELAY_MS)
    const now = this.o.sink.size()
    await this.o.client.resize(this.o.session, now.cols, now.rows)
  }
}
