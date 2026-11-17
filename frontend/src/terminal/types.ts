// The terminal contract shared with the Go side (reviews/m2B-args.md; A2-02 builds it).
// Field names match api.TerminalRead exactly. Offsets are byte offsets into the session's log.

export interface TerminalRead {
  /** base64 bytes from `from` up to `next` */
  data: string
  from: number
  next: number
  tail: number
  /** bytes skipped because the requested offset had been trimmed; 0 = none */
  gap: number
  /** the session ended and every byte has been read */
  done: boolean
  /** base64 DEC private mode sequences to restore; set only when gap > 0 */
  prologue?: string
}

/** Payload of the "terminal:ping" event (coalesced by Go to at most one pending per session). */
export interface TerminalPing {
  session: string
  tail: number
  done: boolean
}

/** The small interface the page codes against. The real one is a thin adapter over the bound TerminalService. */
export interface TerminalClient {
  read(session: string, offset: number, max: number): Promise<TerminalRead>
  /** `data` is base64 */
  write(session: string, data: string): Promise<void>
  resize(session: string, cols: number, rows: number): Promise<void>
  /** Subscribe to "terminal:ping". Returns the unsubscribe function. */
  onPing(cb: (ping: TerminalPing) => void): () => void
}

/** What the sync loop needs from the screen. */
export interface SyncSink {
  write(bytes: Uint8Array): Promise<void>
  reset(): void
  size(): { cols: number; rows: number }
}
