// Vitest setup (vite.config.ts test.setupFiles).
//
// @wailsio/runtime's drag module starts a 50 ms interval when it is imported and clears it on the first tick.
// A test file that finishes in under 50 ms lets jsdom remove `window` first; the tick then throws
// "ReferenceError: window is not defined", which Vitest reports as an unhandled error and fails the run.
// Import the runtime here and let that first tick pass while `window` still exists.
import '@wailsio/runtime'

await new Promise((resolve) => setTimeout(resolve, 60))
