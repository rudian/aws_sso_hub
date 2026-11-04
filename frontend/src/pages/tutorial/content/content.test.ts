import { describe, expect, it } from 'vitest'
import { parseInline } from '../Inline'
import { SECTION_ORDER, tutorialFor } from './index'
import type { TutorialSection } from './types'

const zh = tutorialFor('zh-CN')
const en = tutorialFor('en')

// The generic example data set only: nothing from a real setup may appear in the tutorial.
const ALLOWED_ACCOUNTS = new Set(['111122223333', '444455556666', '777788889999'])
const ALLOWED_PORTALS = new Set(['d-1234567890', 'd-1111111111', 'd-2222222222'])
const FORBIDDEN = [/AKIA[0-9A-Z]{16}/, /\/Users\/(?!alex\b)[A-Za-z0-9._-]+/]

function strings(s: TutorialSection): string[] {
  const out = [s.title, s.lead]
  for (const st of s.steps) out.push(st.title, st.text, ...(st.crumbs ?? []), ...(st.code ? [st.code.label, st.code.text] : []))
  for (const c of s.callouts ?? []) out.push(c.title, c.body)
  for (const f of s.faq ?? []) out.push(f.q, f.a)
  return out
}

describe('tutorial content', () => {
  it('has the eight sections in order in both languages', () => {
    expect(zh.map((s) => s.id)).toEqual(SECTION_ORDER)
    expect(en.map((s) => s.id)).toEqual(SECTION_ORDER)
  })

  it('zh and en are equivalent', () => {
    for (let i = 0; i < SECTION_ORDER.length; i++) {
      const a = zh[i]
      const b = en[i]
      expect(a.group, a.id).toBe(b.group)
      expect(a.minutes, a.id).toBe(b.minutes)
      expect(a.steps.length, `${a.id} steps`).toBe(b.steps.length)
      expect((a.callouts ?? []).length, `${a.id} callouts`).toBe((b.callouts ?? []).length)
      expect((a.faq ?? []).length, `${a.id} faq`).toBe((b.faq ?? []).length)
      a.steps.forEach((st, j) => {
        expect((st.crumbs ?? []).length, `${a.id}[${j}] crumbs`).toBe((b.steps[j].crumbs ?? []).length)
        expect(!!st.code, `${a.id}[${j}] code`).toBe(!!b.steps[j].code)
      })
      ;(a.callouts ?? []).forEach((c, j) => expect(c.kind).toBe(b.callouts![j].kind))
    }
  })

  it('has no empty strings', () => {
    for (const s of [...zh, ...en]) for (const str of strings(s)) expect(str.trim(), s.id).not.toBe('')
  })

  it('puts why/enable/assign in the admin group and the rest in the developer group', () => {
    for (const s of zh) expect(s.group).toBe(['why', 'enable', 'assign'].includes(s.id) ? 'admin' : 'dev')
  })

  it('uses only the generic example data and the product name', () => {
    for (const s of [...zh, ...en]) {
      for (const str of strings(s)) {
        for (const re of FORBIDDEN) expect(str, `${s.id}: ${re}`).not.toMatch(re)
        for (const id of str.match(/\b\d{12}\b/g) ?? []) expect(ALLOWED_ACCOUNTS.has(id), `${s.id}: account ${id}`).toBe(true)
        for (const id of str.match(/\bd-[0-9a-f]{10}\b/g) ?? []) expect(ALLOWED_PORTALS.has(id), `${s.id}: portal ${id}`).toBe(true)
      }
    }
  })

  it('only uses the **bold** and `code` markup', () => {
    for (const s of [...zh, ...en]) {
      for (const str of strings(s)) {
        const rest = parseInline(str)
          .filter((p) => p.kind === 'text')
          .map((p) => p.text)
          .join('')
        expect(rest, `${s.id}: stray markup in "${str.slice(0, 40)}"`).not.toMatch(/\*\*|`/)
      }
    }
  })

  it('has the FAQ entry about switching repositories from a key to SSO (both paths and Add existing repo)', () => {
    const faq = en.find((s) => s.id === 'faq')!
    const item = faq.faq!.find((f) => /Switch repos/.test(f.q))!
    expect(item.a).toMatch(/Path 1/)
    expect(item.a).toMatch(/Path 2/)
    expect(item.a).toMatch(/Add existing repo/)
  })
})

describe('parseInline', () => {
  it('splits bold and code', () => {
    expect(parseInline('a **b** `c` d')).toEqual([
      { kind: 'text', text: 'a ' },
      { kind: 'bold', text: 'b' },
      { kind: 'text', text: ' ' },
      { kind: 'code', text: 'c' },
      { kind: 'text', text: ' d' },
    ])
  })
  it('keeps angle brackets as text', () => {
    expect(parseInline('<img src=x onerror=alert(1)>')).toEqual([{ kind: 'text', text: '<img src=x onerror=alert(1)>' }])
  })
})
