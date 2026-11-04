import { describe, expect, it } from 'vitest'
import { compareVersions, versionLt } from './version'

describe('compareVersions', () => {
  it('orders dotted versions numerically', () => {
    expect(versionLt('2.9.0', '2.22.0')).toBe(true)
    expect(versionLt('2.38.2', '2.40.0')).toBe(true)
    expect(versionLt('2.40.0', '2.38.2')).toBe(false)
    expect(compareVersions('2.22', '2.22.0')).toBe(0)
  })
})
