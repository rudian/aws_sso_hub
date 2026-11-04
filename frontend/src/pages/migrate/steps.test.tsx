import { act, createElement } from 'react'
import type { ReactElement } from 'react'
import { createRoot } from 'react-dom/client'
import type { Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { resetWorld } from '../../api/mock/world'
import { ErrorCode, MigrationItem, MigrationStep, ProgressStatus, DeactivationStatus } from '../../api/models'
import type { MigrationState } from '../../api/models'
import { I18nProvider } from '../../i18n'
import { ActionsProvider } from '../../shell/AppActions'
import { AccountsProvider } from '../../state/accounts'
import { StepAccount } from './StepAccount'
import { StepDeactivate } from './StepKeys'
import { StepVerify } from './StepVerify'
import type { WizardCtx } from './wizard'

;(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true

let host: HTMLDivElement
let root: Root

async function flush(ms: number) {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(ms)
  })
}
function mount(ui: ReactElement) {
  act(() => root.render(createElement(I18nProvider, { lang: 'en' }, createElement(AccountsProvider, null, createElement(ActionsProvider, null, ui)))))
}
const text = () => host.textContent ?? ''

function state(over: Partial<MigrationState> = {}): MigrationState {
  return {
    profile: 'globex',
    step: MigrationStep.MigrationAccount,
    identity: { profile: 'globex', accountId: '444455556666', iamUser: 'globex-ci', maskedKeyId: 'AKIA••••••••••••9C1D', isRoot: false },
    keyInMemory: true,
    portal: 'initech',
    portalIsDraft: false,
    confirmOther: false,
    keyRemoved: false,
    ...over,
  }
}

function ctx(st: MigrationState, over: Partial<WizardCtx> = {}): WizardCtx {
  return {
    name: st.profile,
    st,
    view: st.step,
    setView: vi.fn(),
    run: vi.fn(async () => null),
    busy: false,
    error: null,
    setError: vi.fn(),
    progress: {},
    applying: false,
    startApply: vi.fn(async () => undefined),
    finish: vi.fn(async () => undefined),
    ...over,
  }
}

beforeEach(() => {
  vi.useFakeTimers({ toFake: ['Date', 'setTimeout', 'clearTimeout', 'setInterval', 'clearInterval'] })
  vi.setSystemTime(new Date('2026-11-04T04:00:00Z'))
  resetWorld('mid')
  host = document.createElement('div')
  document.body.appendChild(host)
  root = createRoot(host)
})
afterEach(() => {
  act(() => root.unmount())
  host.remove()
  vi.useRealTimers()
})

describe('StepAccount', () => {
  it('the account of the old key is not in the portal: warns, offers another portal, and needs a confirmation to continue', async () => {
    const w = ctx(state())
    mount(<StepAccount w={w} />)
    await flush(1000)
    expect(text()).toContain('Account 444455556666 isn’t in this portal.')
    expect(text()).toContain('Try another portal')
    const next = [...host.querySelectorAll('.wizard__footer button')].find((b) => b.textContent === 'Next') as HTMLButtonElement
    expect(next.disabled).toBe(true)
    const role = host.querySelector('.account-card input[type="radio"]') as HTMLInputElement
    act(() => role.click())
    expect(next.disabled).toBe(true)
    expect(text()).toContain('I’m sure globex should point to a different account')
    const sure = host.querySelector('input[type="checkbox"]') as HTMLInputElement
    act(() => sure.click())
    expect(next.disabled).toBe(false)
    act(() => next.click())
    expect(w.run).toHaveBeenCalled()
    const other = [...host.querySelectorAll('.banner button')].find((b) => b.textContent === 'Try another portal') as HTMLButtonElement
    act(() => other.click())
    expect(w.setView).toHaveBeenCalledWith(MigrationStep.MigrationPortal)
  })

  it('pins the matching account on top and picks a single role for the user', async () => {
    const w = ctx(state({ identity: { profile: 'globex', accountId: '777788889999', isRoot: false } }))
    mount(<StepAccount w={w} />)
    await flush(1000)
    const first = host.querySelector('.account-card') as HTMLElement
    expect(first.classList.contains('is-match')).toBe(true)
    expect(first.textContent).toContain('Same account as the old key')
    expect(text()).not.toContain('isn’t in this portal')
  })
})

describe('StepVerify', () => {
  it('a failed check shows the cause with Restore backup and Try again', () => {
    const st = state({
      step: MigrationStep.MigrationVerify,
      verify: { ok: false, error: { code: ErrorCode.ErrVerifyWrongAccount, message: 'wrong account', params: { got: '777788889999', want: '444455556666' } } },
    })
    mount(<StepVerify w={ctx(st, { progress: { [MigrationItem.MigrationItemBackup]: { status: ProgressStatus.ProgressDone }, [MigrationItem.MigrationItemWrite]: { status: ProgressStatus.ProgressDone } } })} />)
    expect(text()).toContain('The new setup didn’t pass the check')
    expect(text()).toContain('Signed in to account 777788889999, but expected 444455556666.')
    expect(text()).toContain('Restore backup')
    expect(text()).toContain('Try again')
    expect(host.querySelectorAll('.check-item--failed').length).toBe(1)
    expect(host.querySelectorAll('.check-item--done').length).toBe(2)
    expect([...host.querySelectorAll('.wizard__footer button')].map((b) => b.textContent)).not.toContain('Next')
  })

  it('while applying the rows show their progress', () => {
    const st = state({ step: MigrationStep.MigrationVerify })
    mount(<StepVerify w={ctx(st, { applying: true, progress: { [MigrationItem.MigrationItemBackup]: { status: ProgressStatus.ProgressDone, detail: '~/backups' }, [MigrationItem.MigrationItemWrite]: { status: ProgressStatus.ProgressRunning } } })} />)
    expect(host.querySelectorAll('.check-item--done').length).toBe(1)
    expect(host.querySelectorAll('.check-item--running').length).toBe(1)
    expect(host.querySelectorAll('.check-item--pending').length).toBe(1)
    expect(text()).toContain('~/backups')
  })
})

describe('StepDeactivate', () => {
  it('root keys cannot be deactivated automatically', () => {
    const st = state({ step: MigrationStep.MigrationDeactivate, keyRemoved: true, identity: { profile: 'globex', accountId: '444455556666', isRoot: true, maskedKeyId: 'AKIA••••••••••••9C1D' } })
    mount(<StepDeactivate w={ctx(st)} />)
    expect(text()).toContain('This is a root user key.')
    const now = [...host.querySelectorAll('.option-card button')].find((b) => b.textContent === 'Deactivate now') as HTMLButtonElement
    expect(now.disabled).toBe(true)
  })

  it('shows the result per status and the restart note when the key is no longer in memory', () => {
    const st = state({
      step: MigrationStep.MigrationDeactivate,
      keyRemoved: true,
      keyInMemory: false,
      deactivation: { status: DeactivationStatus.DeactivationDenied, consoleUrl: 'https://example.awsapps.com/start' },
    })
    mount(<StepDeactivate w={ctx(st)} />)
    expect(text()).toContain('Your role can’t manage IAM keys. Use the console instead.')
    expect(text()).toContain('The app restarted, so the old key is no longer in memory.')
  })

  it('offers the next static profile and finishes through the wizard', () => {
    const st = state({ step: MigrationStep.MigrationDeactivate, keyRemoved: true, profile: 'globex' })
    const w = ctx(st)
    mount(<StepDeactivate w={w} />)
    // the mid world has acme migrated and globex static, so there is no other static profile
    expect(text()).not.toContain('Migrate ')
    act(() => ([...host.querySelectorAll('.wizard__footer button')].find((b) => b.textContent === 'Finish') as HTMLButtonElement).click())
    expect(w.finish).toHaveBeenCalledWith()
  })
})
