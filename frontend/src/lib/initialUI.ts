import { useMocks } from '../api/env'
import type { PageId } from './route'

export type ScenarioId = 'fresh' | 'mid' | 'done' | 'empty' | 'edge'
export type MockLoginMode = 'success' | 'waiting' | 'device' | 'denied' | 'timeout' | 'port' | 'network'
export type SessionOverride = 'unknown' | 'overrun' | 'ending'

export interface InitialUI {
  scenario: ScenarioId
  page?: PageId
  profile?: string
  step?: string
  section?: string
  filter?: string
  open?: string
  addPortalStep?: 1 | 2 | 3
  onboardingStep?: 1 | 2 | 3
  mockLogin: MockLoginMode
  theme?: 'light' | 'dark'
  lang?: 'zh-CN' | 'en'
  now?: string
  platform: 'darwin' | 'windows'
  /** ?session=unknown|overrun|ending overrides the initech session in any scenario. */
  session?: SessionOverride
  /** Value returned by the mocked System.ChooseFolder. */
  addPath?: string
}

const SCENARIOS: ScenarioId[] = ['fresh', 'mid', 'done', 'empty', 'edge']
const PAGES: PageId[] = ['accounts', 'migrate', 'repos', 'git', 'terminal', 'health', 'tutorial', 'settings', 'onboarding']
const LOGIN_MODES: MockLoginMode[] = ['success', 'waiting', 'device', 'denied', 'timeout', 'port', 'network']
const SESSIONS: SessionOverride[] = ['unknown', 'overrun', 'ending']

function pick<T extends string>(v: string | null, allowed: readonly T[]): T | undefined {
  return v !== null && (allowed as readonly string[]).includes(v) ? (v as T) : undefined
}
function step123(v: string | null): 1 | 2 | 3 | undefined {
  return v === '1' ? 1 : v === '2' ? 2 : v === '3' ? 3 : undefined
}

function defaults(): InitialUI {
  return { scenario: 'fresh', mockLogin: 'success', platform: 'darwin' }
}

/** Parse the mock-mode URL parameters. Outside mock mode every parameter is ignored. */
export function parseInitialUI(search: string, enabled: boolean): InitialUI {
  if (!enabled) return defaults()
  const q = new URLSearchParams(search)
  const now = q.get('now')
  return {
    scenario: pick(q.get('scenario'), SCENARIOS) ?? 'fresh',
    page: pick(q.get('page'), PAGES),
    profile: q.get('profile') ?? undefined,
    step: q.get('step') ?? undefined,
    section: q.get('section') ?? undefined,
    filter: q.get('filter') ?? undefined,
    open: q.get('open') ?? undefined,
    addPortalStep: step123(q.get('addPortalStep')),
    onboardingStep: step123(q.get('onboardingStep')),
    mockLogin: pick(q.get('mockLogin'), LOGIN_MODES) ?? 'success',
    theme: pick(q.get('theme'), ['light', 'dark'] as const),
    lang: pick(q.get('lang'), ['zh-CN', 'en'] as const),
    now: now && !Number.isNaN(Date.parse(now)) ? now : undefined,
    platform: pick(q.get('platform'), ['darwin', 'windows'] as const) ?? 'darwin',
    session: pick(q.get('session'), SESSIONS),
    addPath: q.get('addPath') ?? undefined,
  }
}

// useMocks is a build-time constant, so a production bundle never reads the URL parameters.
export const initialUI: InitialUI = useMocks ? parseInitialUI(typeof window !== 'undefined' ? window.location.search : '', true) : defaults()

let openValue: string | undefined = initialUI.open

/** If `open` starts with prefix, return the rest once ('' when nothing follows), then clear it. Otherwise null. */
export function takeOpen(prefix: string): string | null {
  if (openValue === undefined || !openValue.startsWith(prefix)) return null
  const rest = openValue.slice(prefix.length)
  openValue = undefined
  return rest
}

/** Hand an `open` request to the page that is about to mount (for example Settings opening the export dialog on Accounts). */
export function setOpen(value: string): void {
  openValue = value
}

/** Peek without consuming (used by tests and by pages that need to know whether an `open=` is pending). */
export function peekOpen(): string | undefined {
  return openValue
}
