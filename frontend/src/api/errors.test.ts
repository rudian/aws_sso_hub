import { describe, expect, it } from 'vitest'
import { Call, CancelError } from '@wailsio/runtime'
import { ErrorCode } from './models'
import { errorKeys, isCancel, toUiError } from './errors'

describe('toUiError', () => {
  it('maps a RuntimeError with an AppError cause', () => {
    const e = new Call.RuntimeError('E_PROFILE_NOT_FOUND: nope', { cause: { code: 'E_PROFILE_NOT_FOUND', message: 'nope', detail: 'd', params: { name: 'acme' } } })
    const u = toUiError(e)
    expect(u.code).toBe(ErrorCode.ErrProfileNotFound)
    expect(u.message).toBe('nope')
    expect(u.detail).toBe('d')
    expect(u.params).toEqual({ name: 'acme' })
  })

  it('accepts a JSON string cause', () => {
    const e = new Call.RuntimeError('x', { cause: JSON.stringify({ code: 'E_NETWORK', message: 'net' }) })
    expect(toUiError(e).code).toBe(ErrorCode.ErrNetwork)
  })

  it('maps CancelError to E_CANCELLED', () => {
    const u = toUiError(new CancelError('cancelled'))
    expect(u.code).toBe(ErrorCode.ErrCancelled)
    expect(isCancel(new CancelError())).toBe(true)
    expect(isCancel(new Error('x'))).toBe(false)
  })

  it('maps anything else to E_INTERNAL', () => {
    expect(toUiError(new Error('boom')).code).toBe(ErrorCode.ErrInternal)
    expect(toUiError('oops').code).toBe(ErrorCode.ErrInternal)
    expect(toUiError(undefined).code).toBe(ErrorCode.ErrInternal)
    expect(toUiError(new Call.RuntimeError('plain message')).code).toBe(ErrorCode.ErrInternal)
  })

  it('maps an unknown code from a newer backend to E_INTERNAL', () => {
    const e = new Call.RuntimeError('x', { cause: { code: 'E_FUTURE', message: 'future' } })
    expect(toUiError(e).code).toBe(ErrorCode.ErrInternal)
  })
})

describe('errorKeys', () => {
  it('covers every ErrorCode member', () => {
    for (const code of Object.values(ErrorCode)) expect(errorKeys[code], String(code)).toBeDefined()
  })
})
