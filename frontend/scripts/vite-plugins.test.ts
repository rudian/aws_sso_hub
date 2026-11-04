import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'
import { CSP_POLICY, cspPlugin, injectCsp, mockStubPlugin } from './vite-plugins'

const indexHtml = readFileSync(resolve(process.cwd(), 'index.html'), 'utf8')

const EXPECTED =
  "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self' data:; " +
  "connect-src 'self'; object-src 'none'; base-uri 'none'; form-action 'none'; frame-src 'none'; worker-src 'none'"

describe('Content-Security-Policy plugin', () => {
  it('uses the exact policy', () => {
    expect(CSP_POLICY).toBe(EXPECTED)
  })

  it('inserts the meta tag as the first child of <head>', () => {
    const plugin = cspPlugin()
    expect(plugin.apply).toBe('build')
    const hook = plugin.transformIndexHtml as { handler: (html: string) => string }
    const out = hook.handler(indexHtml)
    expect(out).toContain(`<meta http-equiv="Content-Security-Policy" content="${EXPECTED}" />`)
    const doc = new DOMParser().parseFromString(out, 'text/html')
    const first = doc.head.firstElementChild
    expect(first?.getAttribute('http-equiv')).toBe('Content-Security-Policy')
    expect(first?.getAttribute('content')).toBe(EXPECTED)
  })

  it('does not allow remote or inline script, eval, or framing', () => {
    expect(CSP_POLICY).not.toMatch(/script-src[^;]*('unsafe-inline'|'unsafe-eval'|https?:|\*)/)
    expect(CSP_POLICY).toContain("object-src 'none'")
    expect(CSP_POLICY).toContain("frame-src 'none'")
  })

  it('index.html has no inline script or event-handler attribute', () => {
    const doc = new DOMParser().parseFromString(injectCsp(indexHtml), 'text/html')
    for (const s of Array.from(doc.querySelectorAll('script'))) expect(s.getAttribute('src'), 'inline <script>').toBeTruthy()
    for (const el of Array.from(doc.querySelectorAll('*'))) {
      for (const a of Array.from(el.attributes)) expect(a.name.startsWith('on'), `${el.tagName}[${a.name}]`).toBe(false)
    }
  })

  it('fails loudly when there is no <head>', () => {
    expect(() => injectCsp('<html></html>')).toThrow()
  })
})

describe('mock stub plugin', () => {
  it('redirects only the mock import of services.ts', () => {
    const plugin = mockStubPlugin('/stub.ts')
    expect(plugin.apply).toBe('build')
    const resolveId = plugin.resolveId as (source: string, importer?: string) => string | null
    expect(resolveId('./mock', '/app/frontend/src/api/services.ts')).toBe('/stub.ts')
    expect(resolveId('./mock', '/app/frontend/src/api/other.ts')).toBeNull()
    expect(resolveId('./mock/world', '/app/frontend/src/api/services.ts')).toBeNull()
  })
})
