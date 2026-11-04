import { Call, CancellablePromise } from '@wailsio/runtime'
import type { ErrorCode } from '../models'
import type { AppError } from '../models'
import { deepCopy } from './clock'

// Latency: reads 120 ms, writes 300 ms, discovery/detection/test/list-remote 700 ms, scan 1200 ms.
export const READ = 120
export const WRITE = 300
export const SLOW = 700
export const SCAN = 1200

/** Thrown inside mock handlers; turned into a rejected promise carrying a Call.RuntimeError whose cause is the AppError. */
export class MockError extends Error {
  app: AppError
  constructor(app: AppError) {
    super(app.message)
    this.app = app
  }
}

export function fail(code: ErrorCode, message: string, params?: Record<string, string>, detail?: string): MockError {
  const app: AppError = { code, message }
  if (params) app.params = params
  if (detail) app.detail = detail
  return new MockError(app)
}

export function toRuntimeError(e: unknown): unknown {
  if (e instanceof MockError) return new Call.RuntimeError(e.app.message, { cause: e.app })
  return e
}

/** Resolve (with a deep copy) after `ms` milliseconds. `fn` runs when the timer fires so it sees the then-current world. */
export function run<T>(ms: number, fn: () => T): CancellablePromise<T> {
  let id: ReturnType<typeof setTimeout> | undefined
  return new CancellablePromise<T>(
    (resolve, reject) => {
      id = setTimeout(() => {
        try {
          resolve(deepCopy(fn()))
        } catch (e) {
          reject(toRuntimeError(e))
        }
      }, ms)
    },
    () => {
      if (id !== undefined) clearTimeout(id)
    },
  )
}

/** A promise that rejects after `ms` with the given AppError (used by the login mock). */
export function rejectLater(ms: number, e: MockError): CancellablePromise<never> {
  return run<never>(ms, () => {
    throw e
  })
}
