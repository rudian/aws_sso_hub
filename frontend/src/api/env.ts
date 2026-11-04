// Detection: inside Wails or not.
// Do NOT use window._wails.environment: on macOS it is injected after the page has loaded.
interface WailsWindow {
  chrome?: { webview?: { postMessage?: unknown } }
  webkit?: { messageHandlers?: { external?: { postMessage?: unknown } } }
}

const w = (typeof window !== 'undefined' ? window : undefined) as (Window & WailsWindow) | undefined

export const isWails: boolean =
  !!w &&
  (typeof w.chrome?.webview?.postMessage === 'function' || // WebView2 (Windows)
    typeof w.webkit?.messageHandlers?.external?.postMessage === 'function') // WKWebView (macOS)

/**
 * Whether the typed in-page mock API replaces the Go bindings.
 *
 * This is a build-time decision: import.meta.env.DEV is the literal `false` in a production build, so the whole
 * expression folds to `false` and the mock layer (and its URL switches) is dropped from the bundle. In development
 * and under vitest it is on outside Wails, and `?mock` forces it on inside Wails (demo screenshots).
 */
export const useMocks: boolean = import.meta.env.DEV && (!isWails || (!!w && new URLSearchParams(w.location.search).has('mock')))
