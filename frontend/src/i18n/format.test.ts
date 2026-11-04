import { describe, expect, it } from 'vitest'
import { formatMessage } from './format'

describe('formatMessage', () => {
  it('replaces placeholders', () => {
    expect(formatMessage('Hello {name}, {n} left', { name: 'Alex', n: 3 }, 'en')).toBe('Hello Alex, 3 left')
  })
  it('keeps an unknown placeholder visible', () => {
    expect(formatMessage('Hi {who}', {}, 'en')).toBe('Hi {who}')
  })
  it('handles plural one/other with #', () => {
    const tpl = '{n, plural, one {# minute ago} other {# minutes ago}}'
    expect(formatMessage(tpl, { n: 1 }, 'en')).toBe('1 minute ago')
    expect(formatMessage(tpl, { n: 5 }, 'en')).toBe('5 minutes ago')
    expect(formatMessage(tpl, { n: 0 }, 'en')).toBe('0 minutes ago')
  })
  it('handles a nested placeholder inside a branch', () => {
    const tpl = 'From ~/.aws/config, {n, plural, one {# profile in {portal}} other {# profiles in {portal}}}'
    expect(formatMessage(tpl, { n: 4, portal: 'initech' }, 'en')).toBe('From ~/.aws/config, 4 profiles in initech')
    expect(formatMessage(tpl, { n: 1, portal: 'initech' }, 'en')).toBe('From ~/.aws/config, 1 profile in initech')
  })
  it('leaves a zh template without plural untouched', () => {
    expect(formatMessage('{n} 分钟前', { n: 5 }, 'zh-CN')).toBe('5 分钟前')
  })
  it('returns plain text unchanged', () => {
    expect(formatMessage('no braces # here', undefined, 'en')).toBe('no braces # here')
  })
})
