import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { buildScenario } from '../../api/mock/scenarios'
import { helperEntries } from '../../api/mock/world'
import type { GitGlobalState } from '../../api/models'
import { DefaultAction } from '../../api/models'
import { codeCommitLines, effectiveDefaultAction, formatEntry, initialDefaultAction } from './model'

describe('git global model', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-11-04T04:00:00Z'))
  })
  afterEach(() => vi.useRealTimers())

  it('keeps only the CodeCommit lines', () => {
    const w = buildScenario('fresh')
    const { other, lines } = helperEntries(w)
    const lines2 = codeCommitLines({ otherHelpers: other, credentialLines: lines } as unknown as GitGlobalState)
    expect(lines2.map(formatEntry)).toEqual([
      'credential.helper = !aws codecommit credential-helper $@ --profile acme',
      'credential.usehttppath = true',
      'credential.https://git-codecommit.us-west-2.amazonaws.com/v1/repos/initech_infra.helper = !aws codecommit credential-helper $@ --profile initech-admin',
    ])
  })
})

describe('default card action guard', () => {
  it('never starts on a disabled option', () => {
    expect(initialDefaultAction({ wantRemove: true, hasDefault: true, urlRulesBlock: true })).toBe(DefaultAction.DefaultKeep)
    expect(initialDefaultAction({ wantRemove: true, hasDefault: false, urlRulesBlock: false })).toBe(DefaultAction.DefaultKeep)
    expect(initialDefaultAction({ wantRemove: true, hasDefault: true, urlRulesBlock: false })).toBe(DefaultAction.DefaultRemove)
    expect(initialDefaultAction({ wantRemove: false, hasDefault: true, urlRulesBlock: false })).toBe(DefaultAction.DefaultKeep)
  })
  it('falls back to keep when the selected removal becomes blocked', () => {
    expect(effectiveDefaultAction(DefaultAction.DefaultRemove, { hasDefault: true, urlRulesBlock: true })).toBe(DefaultAction.DefaultKeep)
    expect(effectiveDefaultAction(DefaultAction.DefaultRemove, { hasDefault: true, urlRulesBlock: false })).toBe(DefaultAction.DefaultRemove)
    expect(effectiveDefaultAction(DefaultAction.DefaultReplace, { hasDefault: true, urlRulesBlock: true })).toBe(DefaultAction.DefaultReplace)
  })
})
