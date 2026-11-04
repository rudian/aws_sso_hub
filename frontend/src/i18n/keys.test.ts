import { describe, expect, it } from 'vitest'
import { ErrorCode } from '../api/models'
import { errorKeys } from '../api/errors'
import { errorText, makeT, namespaces, zhAll } from './index'

describe('locale files', () => {
  for (const [name, ns] of Object.entries(namespaces)) {
    it(`${name}: zh and en have the same keys`, () => {
      expect(Object.keys(ns.en).sort()).toEqual(Object.keys(ns.zh).sort())
    })
    it(`${name}: no empty strings`, () => {
      for (const [k, v] of [...Object.entries(ns.zh), ...Object.entries(ns.en)]) expect(v, k).not.toBe('')
    })
  }

  it('a key defined in two namespaces has identical text in both (session.* keys live in app.ts for the shared Countdown)', () => {
    const seen = new Map<string, { ns: string; zh: string; en: string }>()
    for (const [name, ns] of Object.entries(namespaces)) {
      for (const k of Object.keys(ns.zh)) {
        const cur = { ns: name, zh: (ns.zh as Record<string, string>)[k], en: (ns.en as Record<string, string>)[k] }
        const prev = seen.get(k)
        if (prev) {
          expect(cur.zh, `${k}: zh differs between ${prev.ns} and ${name}`).toBe(prev.zh)
          expect(cur.en, `${k}: en differs between ${prev.ns} and ${name}`).toBe(prev.en)
        }
        seen.set(k, cur)
      }
    }
  })

  it('every errorKeys value exists', () => {
    for (const [code, keys] of Object.entries(errorKeys)) {
      expect(zhAll[keys.title as keyof typeof zhAll], `${code} title`).toBeDefined()
      if (keys.body) expect(zhAll[keys.body as keyof typeof zhAll], `${code} body`).toBeDefined()
    }
  })

  it('formats an error with params', () => {
    const t = makeT('zh-CN')
    const text = errorText({ code: ErrorCode.ErrProfileNotFound, message: 'x', params: { name: 'acme' } }, t)
    expect(text.title).toBe('找不到配置文件 acme')
    const en = errorText({ code: ErrorCode.ErrLoginDenied, message: 'x', params: {}, detail: 'access_denied' }, makeT('en'))
    expect(en.title).toBe('Access was denied')
    expect(en.body).toContain('Deny')
    expect(en.detail).toBe('access_denied')
  })

  it('the banner.cliMissing override replaces the default text', () => {
    expect(makeT('zh-CN')('banner.cliMissing')).toBe('没有找到 AWS CLI v2，仓库绑定和 Git 推送暂时不可用。')
  })
})

describe('E_INVALID_INPUT field labels', () => {
  it('uses the label of a known field and the raw name otherwise', () => {
    const input = (field: string) => ({ code: ErrorCode.ErrInvalidInput, message: 'x', params: { field } })
    expect(errorText(input('dir'), makeT('en')).title).toBe('Check this field: folder name has characters that can’t be passed to a terminal safely')
    expect(errorText(input('gitPath'), makeT('en')).title).toBe('Check this field: must be the full path to the git program')
    expect(errorText(input('awsCliPath'), makeT('zh-CN')).title).toBe('请检查输入：必须是 aws 程序的完整路径')
    expect(errorText(input('dirMissing'), makeT('en')).title).toBe('Check this field: the folder no longer exists; rescan the folders')
    expect(errorText(input('remote'), makeT('zh-CN')).title).toBe('请检查输入：远程名称')
    expect(errorText(input('somethingElse'), makeT('en')).title).toBe('Check this field: somethingElse')
  })

  it('every documented field has a label in both languages', () => {
    for (const f of ['dir', 'dirMissing', 'awsCliPath', 'gitPath', 'role', 'startUrl', 'path', 'remote', 'profile', 'url', 'parentDir', 'folderName', 'repoName']) {
      expect((namespaces.errors.zh as Record<string, string>)[`errors.field.${f}`], f).toBeTruthy()
      expect((namespaces.errors.en as Record<string, string>)[`errors.field.${f}`], f).toBeTruthy()
    }
  })

  it('E_UNSAFE_VALUE no longer mentions Git only', () => {
    const en = errorText({ code: ErrorCode.ErrUnsafeValue, message: 'x', params: { value: 'a"b' } }, makeT('en'))
    expect(en.title).toBe('a"b contains characters that can’t be written safely.')
    expect(en.title).not.toMatch(/git/i)
  })
})
