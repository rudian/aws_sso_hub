import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { getRunningTasks, setRunningTasks } from '../../pages/update/runningTasks'
import { onEvent } from '../events'
import { mockTaskService as svc, resetMockTasks, toolName } from './tasks'
import { getWorld } from './world'

beforeEach(() => {
  vi.useFakeTimers()
  setRunningTasks(0)
})
afterEach(() => vi.useRealTimers())

async function call<T>(p: Promise<T>): Promise<T> {
  const done = p.then(
    (v) => ({ v }),
    (e) => ({ e }),
  )
  await vi.advanceTimersByTimeAsync(3000)
  const r = (await done) as { v?: T; e?: unknown }
  if (r.e) throw r.e
  return r.v as T
}

function causeOf(e: unknown): { code: string; params?: Record<string, string> } {
  return (e as { cause: { code: string; params?: Record<string, string> } }).cause
}

describe('mock task service', () => {
  it('starts empty, and with ?scenario=mid has the five tasks of the mockup', async () => {
    resetMockTasks('')
    expect((await call(svc.List())).tasks).toEqual([])
    resetMockTasks('?scenario=mid')
    const st = await call(svc.List())
    expect(st.tasks.map((t) => t.state)).toEqual(['waiting', 'running-no-signal', 'running', 'done', 'failed'])
    expect(st).toMatchObject({ running: 2, waiting: 1, live: 3, closed: 0, cap: 6 })
  })

  it('titles follow the interface language, and the mid tasks started programs for the quit dialog', async () => {
    getWorld().appInfo.uiLanguage = 'zh-CN'
    resetMockTasks('?scenario=mid')
    const zh = await call(svc.List())
    expect(zh.tasks.map((t) => t.title)).toEqual([
      '修复结算对账的四舍五入误差', '给 payout 批处理补单元测试', '把商户列表改成虚拟滚动', '解释 webhook 重试为什么会重复入账', '升级 Go 到 1.25 并修复告警',
    ])
    expect(await call(svc.LiveSummary())).toMatchObject({ live: 3, programs: 3 })
    expect((await call(svc.LiveSummary())).sessions.map((x) => x.programs)).toEqual([0, 2, 1])
    getWorld().appInfo.uiLanguage = 'en'
    resetMockTasks('?scenario=mid')
    expect((await call(svc.List())).tasks[0].title).toBe('Fix rounding error in settlement reconcile')
  })

  it('has a scenario for each state the page draws', async () => {
    resetMockTasks('?aitasks=waiting')
    expect((await call(svc.List())).tasks.map((t) => t.reason)).toEqual(['permission', 'turn_end'])
    resetMockTasks('?aitasks=ended')
    const ended = await call(svc.List())
    expect(ended.live).toBe(0)
    expect(ended.tasks.every((t) => t.resumable && !t.live)).toBe(true)
    resetMockTasks('?aitasks=closed')
    const closed = await call(svc.List())
    expect(closed).toMatchObject({ closed: 2, programs: 4, running: 1 })
    expect(closed.tasks[0].leftovers).toMatchObject({ count: 3, others: 2 })
    const live = await call(svc.LiveSummary())
    expect(live).toMatchObject({ live: 1, closed: 2, programs: 3 })
    expect(live.sessions[0].repoName).toBe('acme_api')
  })

  it('asks before the fifth task and starts it when confirmed', async () => {
    resetMockTasks('?aitasks=cap')
    const st = await call(svc.List())
    expect(st).toMatchObject({ live: 4, cap: 4 })
    const repo = st.tasks[0].repo
    const req = { repo, tool: 'claude' as const, where: 'main' as const, description: 'one more', confirmed: false }
    const err = await call(svc.Create(req)).catch((e) => e)
    expect(causeOf(err)).toMatchObject({ code: 'E_INVALID_INPUT', params: { field: 'cap', running: '4', cap: '4' } })
    const t = await call(svc.Create({ ...req, confirmed: true }))
    expect(t).toMatchObject({ state: 'starting', live: true, title: 'one more', namedBy: 'fallback' })
    expect((await call(svc.List())).live).toBe(5)
  })

  it('an empty description is "New task n" until named; the tool says it started a moment later', async () => {
    resetMockTasks('?aitasks=running')
    const repo = (await call(svc.List())).tasks[0].repo
    const t = await call(svc.Create({ repo, tool: 'codex', where: 'main', description: '  ', confirmed: false }))
    expect(t).toMatchObject({ namedBy: 'pending', slug: expect.stringMatching(/^new-task-\d+$/) })
    expect(t.title).toMatch(/^(New task|新任务) \d+$/)
    await vi.advanceTimersByTimeAsync(2000)
    expect((await call(svc.List())).tasks.find((x) => x.id === t.id)?.state).toBe('waiting')
  })

  it('stop, resume and rename', async () => {
    resetMockTasks('?aitasks=running')
    const [a] = (await call(svc.List())).tasks
    const stopped = await call(svc.Stop(a.id))
    expect(stopped).toMatchObject({ state: 'stopped', live: false, resumable: true })
    resetMockTasks('?aitasks=ended')
    const [d] = (await call(svc.List())).tasks
    const back = await call(svc.Resume(d.id, false))
    expect(back).toMatchObject({ state: 'starting', live: true })
    expect(causeOf(await call(svc.Resume(d.id, false)).catch((e) => e)).params?.field).toBe('run')
    const renamed = await call(svc.Rename(d.id, '  Mine '))
    expect(renamed).toMatchObject({ title: 'Mine', namedBy: 'manual' })
    expect(causeOf(await call(svc.Rename(d.id, ' ')).catch((e) => e)).params?.field).toBe('title')
  })

  it('stop all ends every task and the programs closed sessions left', async () => {
    resetMockTasks('?aitasks=closed')
    expect(await call(svc.StopAll())).toEqual({ stopped: 1, survivors: 0 })
    const st = await call(svc.List())
    expect(st).toMatchObject({ live: 0, closed: 0, programs: 0 })
    expect(st.tasks.every((t) => t.state === 'stopped')).toBe(true)
  })

  it('sends tasks:changed and tells the update dialog how many tasks run', async () => {
    resetMockTasks('?aitasks=running')
    const seen: number[] = []
    const off = onEvent('tasks:changed' as never, ((s: { running: number }) => seen.push(s.running)) as never)
    const [a] = (await call(svc.List())).tasks
    await call(svc.Stop(a.id))
    expect(seen.at(-1)).toBe(2)
    expect(getRunningTasks()).toBe(2)
    off()
  })

  it('names the tools as the notification does', () => {
    expect(toolName('claude')).toBe('Claude Code')
    expect(toolName('opencode')).toBe('OpenCode')
  })
})
