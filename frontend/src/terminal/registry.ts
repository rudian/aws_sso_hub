import { TerminalInstance, type InstanceOptions } from './instance'
import type { TerminalClient } from './types'

/** Values every new instance gets unless the caller passes its own. */
export interface RegistryDefaults {
  /** Windows build number from the OS (RtlGetVersion); unknown: omit */
  windowsBuild?: number
}

/**
 * Keeps one TerminalInstance per session alive across page, tab and folder switches: a view mounts the instance and
 * unmounts it, it never destroys it. Only `release` (the session is gone) destroys one.
 * One "terminal:ping" subscription and one visibilitychange listener serve every instance.
 */
export class TerminalRegistry {
  private readonly items = new Map<string, TerminalInstance>()
  private unPing: (() => void) | null = null
  private readonly onVis = (): void => {
    for (const i of this.items.values()) i.pageVisibilityChanged()
  }

  constructor(
    private readonly client: TerminalClient,
    private readonly make: (o: InstanceOptions) => TerminalInstance = (o) => new TerminalInstance(o),
    private readonly defaults: RegistryDefaults = {},
  ) {}

  get size(): number {
    return this.items.size
  }

  has(session: string): boolean {
    return this.items.has(session)
  }

  /** The instance for a session; created (not yet pulling) the first time. */
  acquire(session: string, opts: Partial<InstanceOptions> = {}): TerminalInstance {
    let inst = this.items.get(session)
    if (!inst) {
      inst = this.make({ windowsBuild: this.defaults.windowsBuild, ...opts, session, client: this.client })
      this.items.set(session, inst)
      this.ensureListeners()
    }
    return inst
  }

  /** The session ended for good (its tab was closed): destroy the terminal. */
  release(session: string): void {
    const inst = this.items.get(session)
    if (!inst) return
    this.items.delete(session)
    inst.dispose()
    if (this.items.size === 0) this.dropListeners()
  }

  disposeAll(): void {
    for (const s of [...this.items.keys()]) this.release(s)
  }

  setScreenReader(on: boolean): void {
    for (const i of this.items.values()) i.setScreenReader(on)
  }

  private ensureListeners(): void {
    if (this.unPing) return
    this.unPing = this.client.onPing((p) => this.items.get(p.session)?.controller.sync.ping(p))
    document.addEventListener('visibilitychange', this.onVis)
  }

  private dropListeners(): void {
    this.unPing?.()
    this.unPing = null
    document.removeEventListener('visibilitychange', this.onVis)
  }
}

let shared: TerminalRegistry | null = null

/** The app's registry. A2-W calls setTerminalClient once with the real client. */
export function setTerminalClient(client: TerminalClient, defaults: RegistryDefaults = {}): TerminalRegistry {
  shared?.disposeAll()
  shared = new TerminalRegistry(client, undefined, defaults)
  return shared
}

export function terminalRegistry(): TerminalRegistry {
  if (!shared) throw new Error('terminal client is not set (call setTerminalClient)')
  return shared
}
