import { describe, expect, it } from 'vitest'
import { pickBanner } from './GlobalBanner'

describe('pickBanner', () => {
  it('shows nothing when nothing is wrong', () => {
    expect(pickBanner({})).toBeNull()
  })
  it('puts the config parse error before everything else', () => {
    expect(pickBanner({ configParse: true, cliBlocking: true, globalDefault: true, external: true, repair: true })).toBe('configParse')
  })
  it('puts a missing CLI before the global default', () => {
    expect(pickBanner({ cliBlocking: true, globalDefault: true })).toBe('cli')
  })
  it('then the global default, before the softer notices', () => {
    expect(pickBanner({ globalDefault: true, cliOutdated: true, external: true, repair: true })).toBe('globalDefault')
  })
  it('keeps the old order for the rest', () => {
    expect(pickBanner({ cliOutdated: true, external: true, repair: true })).toBe('cliOutdated')
    expect(pickBanner({ external: true, repair: true })).toBe('external')
    expect(pickBanner({ repair: true })).toBe('repair')
  })
})
