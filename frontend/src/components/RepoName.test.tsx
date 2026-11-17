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

  it('keeps the local name and the remote tag as separate elements (D36)', () => {
    const cell = document.createElement('div')
    cell.className = 'repo-cell__name'
    document.body.appendChild(cell)
    act(() => createRoot(cell).render(createElement(I18nProvider, { lang: 'en' }, createElement(RepoName, { localName: 'dashboard-main', remoteName: 'initech_dash' }))))
    expect(cell.querySelector('.repo-nm > .repo-nm__local')?.textContent).toBe('dashboard-main')
    expect(cell.querySelector('.repo-nm > .remote-name')?.textContent).toBe('remote initech_dash')
  })
  it('pins the D36 and D37 style contract in the source', async () => {
    // vitest serves css as an empty module and the frontend has no node types, so the sheets are read from disk (tests run from frontend/).
    const { readFileSync } = (await import(/* @vite-ignore */ 'node:' + 'fs')) as { readFileSync(path: string, enc: string): string }
    const repoNameCss = readFileSync('src/components/RepoName.css', 'utf8')
    const projectsCss = readFileSync('src/pages/projects/projects.css', 'utf8')
    const repoName = repoNameCss
    const projects = projectsCss
    const rule = (css: string, sel: string) => css.split('\n').find((l) => l.startsWith(sel)) ?? ''
    // D37: a matched letter is marked by more than weight (underline and tint).
    const hit = rule(repoName, '.repo-nm__hit')
    expect(hit).toMatch(/text-decoration:\s*underline/)
    expect(hit).toMatch(/background:/)
    // D36: the name cell wraps its pieces; the tag is never ellipsized, it wraps its own text at worst.
    expect(rule(projects, '.repo-cell__name .repo-nm {')).toMatch(/flex-wrap:\s*wrap/)
    const tag = rule(projects, '.repo-cell__name .remote-name')
    expect(tag).not.toMatch(/text-overflow:\s*ellipsis/)
    expect(tag).toMatch(/overflow-wrap:\s*anywhere/)
    expect(repoName).not.toMatch(/\.remote-name[^}]*text-overflow:\s*ellipsis/)
  })
})
