import type { Plugin } from 'vite'

/** The policy written into production builds. The dev server runs without it (React refresh and HMR need inline script and websockets). */
export const CSP_POLICY = [
  "default-src 'self'",
  "script-src 'self'",
  "style-src 'self' 'unsafe-inline'",
  "img-src 'self' data:",
  "font-src 'self' data:",
  "connect-src 'self'",
  "object-src 'none'",
  "base-uri 'none'",
  "form-action 'none'",
  "frame-src 'none'",
  "worker-src 'none'",
].join('; ')

/** Insert the Content-Security-Policy meta tag as the first child of <head>. */
export function injectCsp(html: string): string {
  const meta = `<meta http-equiv="Content-Security-Policy" content="${CSP_POLICY}" />`
  const head = /<head(\s[^>]*)?>/i
  if (!head.test(html)) throw new Error('index.html has no <head> element')
  return html.replace(head, (open) => `${open}\n    ${meta}`)
}

/** Build-only plugin: adds the Content-Security-Policy meta tag to index.html. */
export function cspPlugin(): Plugin {
  return {
    name: 'aws-sso-hub:csp',
    apply: 'build',
    transformIndexHtml: { order: 'post', handler: (html) => injectCsp(html) },
  }
}

/**
 * Production builds resolve the mock layer imported by src/api/services.ts to a stub, so the mock implementation and
 * its fixtures are not part of the bundle even if a bundler cannot prove the branch that uses them is dead.
 */
export function mockStubPlugin(stub: string): Plugin {
  return {
    name: 'aws-sso-hub:mock-stub',
    apply: 'build',
    enforce: 'pre',
    resolveId(source, importer) {
      if (source === './mock' && importer && /[\\/]src[\\/]api[\\/]services\.ts$/.test(importer)) return stub
      return null
    },
  }
}
