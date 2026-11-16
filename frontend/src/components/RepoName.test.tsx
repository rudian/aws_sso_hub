import { act, createElement } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, describe, expect, it } from 'vitest'
import { I18nProvider } from '../i18n'
import { RepoName } from './RepoName'

;(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true

function render(lang: 'en' | 'zh-CN', props: Parameters<typeof RepoName>[0]) {
  const host = document.createElement('div')
  document.body.appendChild(host)
  act(() => createRoot(host).render(createElement(I18nProvider, { lang }, createElement(RepoName, props))))
  return host
}
afterEach(() => {
  document.body.innerHTML = ''
})

describe('RepoName', () => {
  it('shows one name when the names are equal or there is no remote', () => {
    expect(render('en', { localName: 'acme_api', remoteName: 'acme_api' }).querySelector('.remote-name')).toBeNull()
    expect(render('en', { localName: 'acme_api' }).textContent).toBe('acme_api')
  })
  it('shows the remote name as secondary text and for screen readers when they differ', () => {
    const h = render('en', { localName: 'api-main', remoteName: 'acme_api' })
    expect(h.querySelector('.remote-name')?.textContent).toBe('remote acme_api')
    expect(h.querySelector('.sr-only')?.textContent).toContain('remote name acme_api')
    expect(h.querySelector('.repo-nm__local')?.textContent).toBe('api-main')
  })
  it('uses its own inline class, not the Repos table cell rule', () => {
    const root = render('en', { localName: 'api-main', remoteName: 'acme_api', className: 'x' }).firstElementChild as HTMLElement
    expect(root.classList.contains('repo-nm')).toBe(true)
    expect(root.classList.contains('x')).toBe(true)
    expect(root.classList.contains('repo-name')).toBe(false)
  })
  it('speaks zh-CN', () => {
    expect(render('zh-CN', { localName: 'api-main', remoteName: 'acme_api' }).querySelector('.remote-name')?.textContent).toBe('远程 acme_api')
  })
  it('draws the matched letters bold, in the name and in the remote tag', () => {
    const h = render('en', { localName: 'acme_agent_portal', remoteName: 'agent-portal', query: 'agent' })
    expect([...h.querySelectorAll('.repo-nm__hit')].map((e) => e.textContent)).toEqual(['agent', 'agent'])
    expect(h.querySelector('.repo-nm__local')?.textContent).toBe('acme_agent_portal')
    expect(h.querySelector('.remote-name')?.textContent).toBe('remote agent-portal')
  })
  it('bolds nothing without a query or a match', () => {
    expect(render('en', { localName: 'acme_api', query: '' }).querySelector('.repo-nm__hit')).toBeNull()
    expect(render('en', { localName: 'acme_api', query: 'zzz' }).querySelector('.repo-nm__hit')).toBeNull()
  })
})
