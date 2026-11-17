import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { onEvent } from '../events'
import { NotifyAction, NotifyPermission, NotifyTone } from '../models'
import type { NotifyToast } from '../models'
import { mock } from './index'
import { emitMockSignInNotice, resetMockNotify, signInNoticeText } from './notify'
import { resetWorld } from './world'

beforeEach(() => {
  vi.useFakeTimers()
  resetWorld('mid')
  resetMockNotify('')
})
afterEach(() => vi.useRealTimers())

async function call<T>(p: Promise<T>): Promise<T> {
  const done = p.then((v) => ({ v }))
  await vi.advanceTimersByTimeAsync(2000)
  return (await done).v
}

describe('the notification texts of design 6', () => {
  it('heads-up, three accounts', () => {
    expect(signInNoticeText('headsup', 'acme', ['acme-prod', 'globex', 'initech'], 15, 'zh-CN')).toEqual({ title: '登录快过期了：acme', body: 'acme-prod 等 3 个账号。大约 15 分钟后过期。' })
    expect(signInNoticeText('headsup', 'acme', ['acme-prod', 'globex', 'initech'], 15, 'en')).toEqual({ title: 'Sign-in expires soon: acme', body: 'acme-prod and 2 more. Expires in about 15 minutes.' })
  })
  it('ended, one and two accounts and none', () => {
    expect(signInNoticeText('ended', 'acme', ['acme-prod'], 15, 'zh-CN')).toEqual({ title: '登录已过期：acme', body: 'acme-prod。重新登录后才能继续用。' })
    expect(signInNoticeText('ended', 'acme', ['acme-prod', 'globex'], 15, 'zh-CN').body).toBe('acme-prod、globex。重新登录后才能继续用。')
    expect(signInNoticeText('ended', 'acme', ['acme-prod', 'globex'], 15, 'en').body).toBe('acme-prod and globex. Sign in again to continue.')
    expect(signInNoticeText('ended', 'acme', [], 15, 'en').body).toBe('Sign in again to continue.')
  })
  it('the lead time in words, always "about"', () => {
    expect(signInNoticeText('headsup', 'acme', [], 30, 'en').body).toBe('Expires in about 30 minutes.')
    expect(signInNoticeText('headsup', 'acme', [], 60, 'en').body).toBe('Expires in about 1 hour.')
    expect(signInNoticeText('headsup', 'acme', [], 60, 'zh-CN').body).toBe('大约 1 小时后过期。')
  })
})

describe('mock notify service', () => {
  it('permission follows ?notify=', async () => {
    expect((await call(mock.notify.GetState())).permission).toBe(NotifyPermission.NotifyAllowed)
    resetMockNotify('?notify=off')
    expect((await call(mock.notify.GetState())).permission).toBe(NotifyPermission.NotifyOff)
    resetMockNotify('?notify=notasked')
    expect((await call(mock.notify.GetState())).permission).toBe(NotifyPermission.NotifyNotAsked)
  })

  it('asking from "not asked" ends in "allowed" and says so with notify:state', async () => {
    resetMockNotify('?notify=notasked')
    const seen: NotifyPermission[] = []
    const off = onEvent('notify:state', (s) => seen.push(s.permission))
    await call(mock.notify.RequestPermission())
    expect(seen).toEqual([NotifyPermission.NotifyAllowed])
    off()
  })

  it('a toast carries the title, body and button; its button signs in, its body shows the portal', async () => {
    const toasts: NotifyToast[] = []
    const nav: unknown[] = []
    const offT = onEvent('notify:toast', (t) => toasts.push(t))
    const offN = onEvent('app:navigate', (r) => nav.push(r))
    const ended = emitMockSignInNotice('ended', 'initech')
    expect(ended.tone).toBe(NotifyTone.NotifyToneDanger)
    expect(ended.sticky).toBe(true)
    expect(ended.button).toBe(navigator.language.toLowerCase().startsWith('zh') ? '重新登录' : 'Sign in again')
    const heads = emitMockSignInNotice('headsup', 'initech')
    expect(heads.tone).toBe(NotifyTone.NotifyToneWarn)
    expect(toasts.map((t) => t.id)).toEqual([ended.id, heads.id])
    await call(mock.notify.RespondToast(ended.id, NotifyAction.NotifyActionBody))
    expect(nav).toEqual([{ page: 'accounts', action: 'show', arg: 'initech' }])
    await call(mock.notify.RespondToast(ended.id, NotifyAction.NotifyActionBody)) // answered once
    expect(nav).toHaveLength(1)
    offT()
    offN()
  })
})
