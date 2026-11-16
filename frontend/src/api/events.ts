import { Events } from '@wailsio/runtime'
import { useMocks } from './env'
import type {
  AccountsState,
  CloneProgress,
  ExternalChange,
  InstallState,
  LoginProgress,
  MigrationProgress,
  NavigateRequest,
  ReposState,
  ScanProgress,
  SessionStatus,
  Settings,
} from './models'

export interface EventPayloads {
  'accounts:changed': AccountsState
  'session:changed': SessionStatus
  'session:renewed': SessionStatus
  'session:reminder': SessionStatus
  'login:progress': LoginProgress
  'migration:progress': MigrationProgress
  'repos:changed': ReposState
  'repos:scan-progress': ScanProgress
  'clone:progress': CloneProgress
  'files:external-change': ExternalChange
  'settings:changed': Settings
  'app:navigate': NavigateRequest
  'app:second-instance': string[]
  'tools:install-progress': InstallState
  'identity:changed': Record<string, never>
  /** Menu and tray commands; the Go side registers it in internal/app/menu.go. */
  'app:command': string
}

type Handler = (data: unknown) => void
const local = new Map<string, Set<Handler>>()

/** Subscribe to a backend event (or to the in-page mock bus when mocks are active). Returns the unsubscribe function. */
export function onEvent<K extends keyof EventPayloads>(name: K, cb: (data: EventPayloads[K]) => void): () => void {
  if (!useMocks) {
    // nil slices arrive as null (app:second-instance is the only event whose payload is a slice)
    return Events.On(name, (ev: { data: unknown }) => cb((ev.data ?? (name === 'app:second-instance' ? [] : ev.data)) as EventPayloads[K]))
  }
  const set = local.get(name) ?? new Set<Handler>()
  const h: Handler = (d) => cb(d as EventPayloads[K])
  set.add(h)
  local.set(name, set)
  return () => {
    set.delete(h)
  }
}

/** Used only by the mock API. Payloads are deep-copied so listeners can never mutate the mock world. */
export function mockEmit<K extends keyof EventPayloads>(name: K, data: EventPayloads[K]): void {
  const set = local.get(name)
  if (!set) return
  const copy = JSON.parse(JSON.stringify(data)) as EventPayloads[K]
  for (const cb of [...set]) cb(copy)
}
