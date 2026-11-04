import type { mock as MockLayer } from './mock'

// Production builds resolve './mock' (from services.ts) to this module, so the mock implementation and its
// fixtures never reach the bundle. `useMocks` is false there and these values are never read.
export const mock: typeof MockLayer = undefined as never
