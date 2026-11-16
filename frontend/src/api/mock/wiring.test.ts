import { describe, expect, it } from 'vitest'
import { mock } from './index'

describe('mock registry', () => {
  it('has a mock for every bound service, tools and identity included', () => {
    expect(Object.keys(mock).sort()).toEqual(['accounts', 'gitGlobal', 'identity', 'migration', 'repos', 'system', 'tools'])
    expect(typeof mock.tools.GetSetupState).toBe('function')
    expect(typeof mock.identity.GetOverview).toBe('function')
  })
})
