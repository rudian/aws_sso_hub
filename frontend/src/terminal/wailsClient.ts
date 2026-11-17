import { Call, Events } from '@wailsio/runtime'
import type { TerminalClient, TerminalPing, TerminalRead } from './types'

/** Bound name of A2-02's service in package app; change it if the generated bindings say otherwise. */
export const TERMINAL_SERVICE = 'github.com/rudian/codepass/internal/app.TerminalService'

/**
 * The real client: bound TerminalService calls and the "terminal:ping" event. A2-W may replace the calls with the generated
 * binding functions; the method names and fields are the contract either way.
 */
export function createWailsTerminalClient(service: string = TERMINAL_SERVICE): TerminalClient {
  const call = <T>(m: string, ...a: unknown[]): Promise<T> => Call.ByName(`${service}.${m}`, ...a) as Promise<T>
  return {
    read: (session, offset, max) => call<TerminalRead>('Read', session, offset, max),
    write: (session, data) => call<void>('Write', session, data),
    resize: (session, cols, rows) => call<void>('Resize', session, cols, rows),
    onPing: (cb) =>
      Events.On('terminal:ping', (ev: { data: unknown }) => {
        // Wails delivers the payload as the event data (an array of one when emitted with several arguments)
        const d = (Array.isArray(ev.data) ? ev.data[0] : ev.data) as TerminalPing | undefined
        if (d && typeof d.session === 'string') cb(d)
      }),
  }
}
