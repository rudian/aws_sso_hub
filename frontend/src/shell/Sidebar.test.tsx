import { act, createElement } from 'react'
import { createRoot } from 'react-dom/client'
import type { Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { BindingSource, ProfileKind, RepoProvider, SessionState } from '../api/models'
import type { Repo } from '../api/models'
import { I18nProvider } from '../i18n'
import type { Lang } from '../i18n'
import { Sidebar } from './Sidebar'

const world: { repos: Repo[]; session: SessionState } = { repos: [], session: SessionState.SessionExpired }

vi.mock('../state/repos', () => ({ useRepos: () => ({ state: { repos: world.repos } }) }))
vi.mock('../state/accounts', () => ({
  useAccounts: () => ({
    state: {
      staticCount: 0,
      profiles: [{ name: 'acme', kind: ProfileKind.ProfileKindSSO, portal: 'acme-portal' }],
      portals: [{ name: 'acme-portal', session: { state: world.session } }],
    },
  }),
}))
vi.mock('../state/appInfo', () => ({ useAppInfo: () => ({ platform: 'darwin' }) }))
vi.mock('../state/env', () => ({ useEnv: () => ({ report: null }) }))
vi.mock('../pages/update/useUpdate', () => ({ useUpdateIndicator: () => false }))
vi.mock('./Keyring', () => ({ Keyring: () => null }))

;(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true

function repo(name: string, over: Partial<Repo> = {}): Repo {
  return {
    id: `id_${name}`,
    path: `/Users/alex/code/${name}`,
    displayPath: `~/code/${name}`,
    name,
    localName: name,
    remoteName: name,
    remote: 'origin',
    remoteUrl: '',
    remoteKind: 'https',
    hasUserinfo: false,
    binding: { source: '', profileKnown: false, needsRepair: false, legacyPlain: false },
    owner: { status: 'unknown', candidates: [], checkedWith: 0, skipped: 0 },
    toFix: false,
    extra: false,
    provider: RepoProvider.ProviderCodeCommit,
    hidden: false,
    pending: false,
    ...over,
  } as Repo
}

let host: HTMLDivElement
let root: Root
function mount(lang: Lang) {
  act(() => root.render(createElement(I18nProvider, { lang }, createElement(Sidebar))))
}
const count = () => host.querySelector('.nav__count--warn')?.textContent

beforeEach(() => {
  host = document.createElement('div')
  document.body.appendChild(host)
  root = createRoot(host)
  world.repos = [
    repo('bound', { binding: { source: BindingSource.BindingLocal, profile: 'acme', profileKnown: true, needsRepair: false, legacyPlain: false } } as Partial<Repo>),
    repo('unbound', { toFix: true }),
  ]
})
afterEach(() => {
  act(() => root.unmount())
  host.remove()
})

describe('Sidebar "N to fix" (D7)', () => {
  it('counts an unbound row and a bound row whose sign-in expired', () => {
    world.session = SessionState.SessionExpired
    mount('en')
    expect(count()).toBe('2 to fix')
    mount('zh-CN')
    expect(count()).toBe('2 待处理')
  })
  it('drops the expired row once the sign-in is usable', () => {
    for (const st of [SessionState.SessionActive, SessionState.SessionExpiring]) {
      world.session = st
      mount('en')
      expect(count()).toBe('1 to fix')
    }
  })
})
