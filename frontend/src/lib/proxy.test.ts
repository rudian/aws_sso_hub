import { describe, expect, it } from 'vitest'
import { PROXY_EXAMPLE, cleanProxy, proxyProblem } from './proxy'

describe('console proxy (same rules as browser.CleanProxy)', () => {
  it('accepts an IPv4 address or a DNS name with a port, and gives the canonical form', () => {
    const good: Record<string, string> = {
      '192.0.2.10:3128': '192.0.2.10:3128',
      ' 198.51.100.7:8080 ': '198.51.100.7:8080',
      '203.0.113.5:03128': '203.0.113.5:3128',
      '255.255.255.255:65535': '255.255.255.255:65535',
      'proxy.example.com:3128': 'proxy.example.com:3128',
      'Proxy.Example.COM:3128': 'proxy.example.com:3128',
      'localhost:1': 'localhost:1',
      'egress-1.corp.example.net:80': 'egress-1.corp.example.net:80',
      [PROXY_EXAMPLE]: PROXY_EXAMPLE,
    }
    for (const [input, want] of Object.entries(good)) {
      expect(proxyProblem(input), input).toBeNull()
      expect(cleanProxy(input), input).toBe(want)
    }
  })

  it('empty means no proxy', () => {
    expect(proxyProblem('')).toBeNull()
    expect(proxyProblem('   ')).toBeNull()
    expect(cleanProxy(' ')).toBeNull()
  })

  it('names what is wrong', () => {
    const bad: Record<string, string> = {
      'http://192.0.2.10:3128': 'format',
      'socks5://192.0.2.10:1080': 'format',
      '192.0.2.10:3128/': 'format',
      'proxy.example.com:3128/path': 'format',
      'user@proxy.example.com:3128': 'format',
      'proxy example.com:3128': 'format',
      '192.0.2.10 :3128': 'format',
      '192.0.2.10': 'port',
      '192.0.2.10:': 'port',
      'proxy.example.com': 'port',
      '192.0.2.10:0': 'portRange',
      '192.0.2.10:65536': 'portRange',
      '192.0.2.10:123456': 'portRange',
      '192.0.2.10:+3128': 'portRange',
      '192.0.2.10:-1': 'portRange',
      '192.0.2.10:http': 'portRange',
      ':3128': 'host',
      '[2001:db8::1]:3128': 'host',
      '2001:db8::1:3128': 'host',
      '256.0.2.1:3128': 'host',
      '192.0.2:3128': 'host',
      '192.0.02.1:3128': 'host',
      '-proxy.example.com:3128': 'host',
      'proxy-.example.com:3128': 'host',
      'proxy..example.com:3128': 'host',
      'proxy.example.com.:3128': 'host',
      'prøxy.example:3128': 'host',
      'proxy_1.example:3128': 'host',
      'proxy.example.com:3128;DIRECT': 'portRange',
      [`${'a'.repeat(64)}.example:3128`]: 'host',
    }
    for (const [input, want] of Object.entries(bad)) {
      expect(proxyProblem(input), input).toBe(want)
      expect(cleanProxy(input), input).toBeNull()
    }
  })
})
