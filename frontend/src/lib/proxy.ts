// HTTP proxy of a portal's console, written host:port. The rules match browser.CleanProxy in the backend.

/** Placeholder and example of a console proxy. */
export const PROXY_EXAMPLE = '10.0.0.1:3128'

export type ProxyProblem = 'format' | 'port' | 'portRange' | 'host'

function validHost(h: string): boolean {
  if (h === '' || h.length > 253) return false
  // digits and dots only: an IPv4 address, never a name
  if (/^[0-9.]+$/.test(h)) {
    const parts = h.split('.')
    return parts.length === 4 && parts.every((p) => /^(0|[1-9][0-9]{0,2})$/.test(p) && Number(p) <= 255)
  }
  return h.split('.').every((label) => /^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$/.test(label))
}

/** Why a console proxy cannot be used, or null. Empty (no proxy) is fine. Host: IPv4 address or DNS name; port: 1 to 65535. */
export function proxyProblem(value: string): ProxyProblem | null {
  const v = value.trim()
  if (v === '') return null
  if (/[\s/@]/.test(v)) return 'format' // a scheme, a path, user info or spaces
  const i = v.lastIndexOf(':')
  if (i < 0 || i === v.length - 1) return 'port'
  const port = v.slice(i + 1)
  if (!/^[0-9]{1,5}$/.test(port) || Number(port) < 1 || Number(port) > 65535) return 'portRange'
  return validHost(v.slice(0, i).toLowerCase()) ? null : 'host'
}

/** The canonical form the backend stores (host in lower case, port without leading zeros), or null when invalid or empty. */
export function cleanProxy(value: string): string | null {
  const v = value.trim()
  if (v === '' || proxyProblem(v)) return null
  const i = v.lastIndexOf(':')
  return `${v.slice(0, i).toLowerCase()}:${Number(v.slice(i + 1))}`
}
