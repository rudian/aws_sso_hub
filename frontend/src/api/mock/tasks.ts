import type { CancellablePromise } from '@wailsio/runtime'
import { setRunningTasks } from '../../pages/update/runningTasks'
import { mockEmit } from '../events'
import { ErrorCode } from '../models'
import { deepCopy, MIN, ago, iso, now } from './clock'
import { fail, READ, run, SLOW, WRITE } from './util'
import { getWorld, repoByDefN } from './world'

// The demo backend of the AI task lifecycle (A2-08; plan 4.7, 4.8, 4.8b; design 5.6, 5.6a, 5.6b, 5.7). Nothing runs: the
// demo acts out the same answers as internal/tasks, so A2-09's AI tasks page, the sidebar count, the quit dialog and
// the update dialog can be built and shot without a tool.
//
// The Go service is not registered yet (A2-W does that), so wails3 has not generated its module or its types. Until
// then the shapes below mirror internal/api/tasks.go field for field. A2-W replaces them with
//   import * as TaskReal from '../../../bindings/github.com/rudian/codepass/internal/app/taskservice'
//   import type { Task, TasksState, CreateTaskRequest, LiveSummary, StopAllResult } from '../models'
// and types the export as `typeof TaskReal`. It also adds 'tasks:changed' and 'tasks:open' to EventPayloads (events.ts).
//
// URL switch (mock mode only):
//   ?aitasks=none      no tasks (the default, except with ?scenario=mid)
//   ?aitasks=mid       the five tasks of the mockup: one waiting, two running, one done, one failed (the default
//                      with ?scenario=mid)
//   ?aitasks=running   two running tasks (one with hooks, one without a signal) and a starting one
//   ?aitasks=waiting   one task waiting for a permission and one for its next instruction
//   ?aitasks=ended     a done, a failed and a stopped task (resumable), none live
//   ?aitasks=closed    two closed sessions that still run programs (design 5.6a), plus one running task
//   ?aitasks=cap       four tasks running with the soft cap at 4: starting a fifth asks first (design 5.6 "fifth
//                      concurrent task"); the real cap is 6 and lives in Settings
// Whatever the scenario, the number of running tasks is passed to setRunningTasks (the update dialog's "Reopen when no
// task is running") whenever it changes.

export type TaskState = 'starting' | 'running' | 'waiting' | 'running-no-signal' | 'done' | 'failed' | 'stopped' | 'closed-running'
export type TaskWait = '' | 'permission' | 'question' | 'idle' | 'turn_end' | 'limit' | 'error'
export interface TaskLeftovers {
  count: number
  others: number
  cpu: number
  memory: number
  closedAt?: string
}
export interface Task {
  id: string
  repo: string
  tool: 'claude' | 'codex' | 'opencode'
  title: string
  slug: string
  namedBy: 'ai' | 'fallback' | 'pending' | 'manual'
  number: number
  where: 'main' | 'own'
  dir: string
  state: TaskState
  reason: TaskWait
  createdAt: string
  startedAt: string
  endedAt?: string
  exitCode?: number
  live: boolean
  resumable: boolean
  resumed: boolean
  permissionMode?: string
  leftovers: TaskLeftovers
}
export interface TasksState {
  tasks: Task[]
  running: number
  waiting: number
  live: number
  closed: number
  programs: number
  cap: number
}
export interface CreateTaskRequest {
  repo: string
  tool: 'claude' | 'codex' | 'opencode'
  where: 'main' | 'own'
  description: string
  confirmed: boolean
}
export interface LiveSession {
  task: string
  repo: string
  repoName: string
  tool: 'claude' | 'codex' | 'opencode'
  title: string
  state: TaskState
  programs: number
}
export interface LiveSummary {
  sessions: LiveSession[]
  live: number
  closed: number
  programs: number
}
export interface StopAllResult {
  stopped: number
  survivors: number
  error?: string
}
export interface TaskOpen {
  task: string
  repo: string
}

export interface TaskServiceShape {
  List(): CancellablePromise<TasksState>
  Create(req: CreateTaskRequest): CancellablePromise<Task>
  Stop(id: string): CancellablePromise<Task>
  Resume(id: string, confirmed: boolean): CancellablePromise<Task>
  Rename(id: string, title: string): CancellablePromise<Task>
  LiveSummary(): CancellablePromise<LiveSummary>
  StopAll(): CancellablePromise<StopAllResult>
}

export type TasksScenario = 'none' | 'mid' | 'running' | 'waiting' | 'ended' | 'closed' | 'cap'
const SCENARIOS: TasksScenario[] = ['none', 'mid', 'running', 'waiting', 'ended', 'closed', 'cap']

// 'tasks:changed' and 'tasks:open' join EventPayloads in src/api/events.ts when A2-W registers the service.
const emitUntyped = mockEmit as unknown as (name: string, data: unknown) => void

const TOOL_NAMES = { claude: 'Claude Code', codex: 'Codex', opencode: 'OpenCode' } as const

interface Model {
  tasks: Task[]
  cap: number
  seq: number
  timers: ReturnType<typeof setTimeout>[]
}

let model: Model | null = null
let search = typeof window !== 'undefined' ? window.location.search : ''
let lastRunning = -1

function mk(p: Partial<Task> & Pick<Task, 'id' | 'repo' | 'tool' | 'title' | 'slug' | 'state'>): Task {
  const live = p.state === 'starting' || p.state === 'running' || p.state === 'waiting' || p.state === 'running-no-signal'
  return {
    namedBy: 'ai',
    number: 1,
    where: 'main',
    dir: '',
    reason: '',
    createdAt: ago(40 * MIN),
    startedAt: ago(30 * MIN),
    live,
    resumable: !live,
    resumed: false,
    leftovers: { count: 0, others: 0, cpu: 0, memory: 0 },
    ...p,
  }
}

function repos() {
  const w = getWorld()
  const pick = (n: number) => {
    const r = repoByDefN(w, n)
    return { id: r.id, dir: r.path }
  }
  return { api: pick(3), portal: pick(1), globex: pick(9) }
}

function build(s: TasksScenario): Model {
  const r = repos()
  const base = (x: { id: string; dir: string }) => ({ repo: x.id, dir: x.dir })
  const MB = 1024 * 1024
  const left = (count: number, others: number, cpu: number, mb: number, closedMin: number) => ({
    count,
    others,
    cpu,
    memory: mb * MB,
    closedAt: ago(closedMin * MIN),
  })
  // the titles follow the interface language, as aitools NameTask does: the shots of the zh pages draw these tasks
  const zh = getWorld().appInfo.uiLanguage === 'zh-CN'
  const L = (cn: string, en: string) => (zh ? cn : en)
  const t: Task[] = []
  const add = (p: Parameters<typeof mk>[0]) => t.push(mk(p))
  switch (s) {
    case 'none':
      break
    case 'mid':
      add({ id: 't-0000000001', ...base(r.api), tool: 'claude', title: L('修复结算对账的四舍五入误差', 'Fix rounding error in settlement reconcile'), slug: 'fix-settlement-rounding', state: 'waiting', reason: 'permission', startedAt: ago(14 * MIN), permissionMode: 'manual', number: 1 })
      add({ id: 't-0000000002', ...base(r.api), tool: 'codex', title: L('给 payout 批处理补单元测试', 'Add unit tests for payout batching'), slug: 'payout-batch-tests', state: 'running-no-signal', startedAt: ago(7 * MIN), number: 2, leftovers: { count: 4, others: 2, cpu: 12, memory: 180 * MB } })
      add({ id: 't-0000000003', ...base(r.portal), tool: 'opencode', title: L('把商户列表改成虚拟滚动', 'Virtualise the merchant list'), slug: 'merchant-list-virtual', state: 'running', startedAt: ago(21 * MIN), number: 1, leftovers: { count: 3, others: 1, cpu: 3, memory: 64 * MB } })
      add({ id: 't-0000000004', ...base(r.api), tool: 'claude', title: L('解释 webhook 重试为什么会重复入账', 'Explain why webhook retries double-post'), slug: 'webhook-retries', state: 'done', startedAt: ago(3 * 60 * MIN), endedAt: ago(170 * MIN), exitCode: 0, number: 3 })
      add({ id: 't-0000000005', ...base(r.globex), tool: 'claude', title: L('升级 Go 到 1.25 并修复告警', 'Upgrade Go to 1.25 and fix warnings'), slug: 'go-1-25', state: 'failed', startedAt: ago(5 * 60 * MIN), endedAt: ago(299 * MIN), exitCode: 1, number: 1 })
      break
    case 'running':
      add({ id: 't-0000000001', ...base(r.api), tool: 'claude', title: L('给上传器加重试', 'Add retries to the uploader'), slug: 'uploader-retries', state: 'running', permissionMode: 'acceptEdits', number: 1 })
      add({ id: 't-0000000002', ...base(r.api), tool: 'codex', title: L('给 payout 批处理补单元测试', 'Add unit tests for payout batching'), slug: 'payout-batch-tests', state: 'running-no-signal', number: 2 })
      add({ id: 't-0000000003', ...base(r.portal), tool: 'opencode', title: L('新任务 1', 'New task 1'), slug: 'new-task-1', namedBy: 'pending', state: 'starting', startedAt: ago(5_000) })
      break
    case 'waiting':
      add({ id: 't-0000000001', ...base(r.api), tool: 'claude', title: L('修复结算对账的四舍五入误差', 'Fix rounding error in settlement reconcile'), slug: 'fix-settlement-rounding', state: 'waiting', reason: 'permission', permissionMode: 'manual' })
      add({ id: 't-0000000002', ...base(r.portal), tool: 'claude', title: L('把商户列表改成虚拟滚动', 'Virtualise the merchant list'), slug: 'merchant-list-virtual', state: 'waiting', reason: 'turn_end' })
      break
    case 'ended':
      add({ id: 't-0000000001', ...base(r.api), tool: 'claude', title: L('解释 webhook 重试为什么会重复入账', 'Explain why webhook retries double-post'), slug: 'webhook-retries', state: 'done', endedAt: ago(20 * MIN), exitCode: 0 })
      add({ id: 't-0000000002', ...base(r.globex), tool: 'claude', title: L('升级 Go 到 1.25 并修复告警', 'Upgrade Go to 1.25 and fix warnings'), slug: 'go-1-25', state: 'failed', endedAt: ago(50 * MIN), exitCode: 1 })
      add({ id: 't-0000000003', ...base(r.portal), tool: 'opencode', title: L('把商户列表改成虚拟滚动', 'Virtualise the merchant list'), slug: 'merchant-list-virtual', state: 'stopped', endedAt: ago(90 * MIN) })
      break
    case 'closed':
      add({
        id: 't-0000000001', ...base(r.api), tool: 'claude', title: L('解释 webhook 重试为什么会重复入账', 'Explain why webhook retries double-post'), slug: 'webhook-retries', state: 'closed-running', endedAt: ago(12 * MIN), exitCode: 0,
        leftovers: left(3, 2, 41, 612, 12),
      })
      add({
        id: 't-0000000002', ...base(r.portal), tool: 'opencode', title: L('给导出接口加分页', 'Add paging to the export endpoint'), slug: 'export-paging', state: 'closed-running', endedAt: ago(120 * MIN),
        leftovers: left(1, 1, 0, 48, 120),
      })
      add({ id: 't-0000000003', ...base(r.api), tool: 'codex', title: L('给 payout 批处理补单元测试', 'Add unit tests for payout batching'), slug: 'payout-batch-tests', state: 'running-no-signal' })
      break
    case 'cap':
      for (let i = 1; i <= 4; i++) {
        add({ id: `t-000000000${i}`, ...base(i === 3 ? r.portal : r.api), tool: i === 2 ? 'codex' : 'claude', title: L(`第 ${i} 个任务`, `Task number ${i}`), slug: `task-${i}`, state: 'running', number: i })
      }
      break
  }
  return { tasks: t, cap: s === 'cap' ? 4 : 6, seq: t.length, timers: [] }
}

/** Back to a fresh page load, reading the URL switch (tests pass the query string). */
export function resetMockTasks(q = typeof window !== 'undefined' ? window.location.search : ''): void {
  if (model) model.timers.forEach(clearTimeout)
  search = q
  model = null
  lastRunning = -1
}

function m(): Model {
  if (!model) {
    const params = new URLSearchParams(search)
    const v = params.get('aitasks') as TasksScenario | null
    const fallback: TasksScenario = params.get('scenario') === 'mid' ? 'mid' : 'none'
    model = build(v && SCENARIOS.includes(v) ? v : fallback)
  }
  return model
}

function snapshot(): TasksState {
  const x = m()
  const tasks = deepCopy(x.tasks)
  const st: TasksState = { tasks, running: 0, waiting: 0, live: 0, closed: 0, programs: 0, cap: x.cap }
  for (const t of tasks) {
    if (t.live) st.live++
    if (t.state === 'starting' || t.state === 'running' || t.state === 'running-no-signal') st.running++
    else if (t.state === 'waiting') st.waiting++
    else if (t.state === 'closed-running') {
      st.closed++
      st.programs += t.leftovers.count
    }
  }
  return st
}

/** Tell the window, and the update dialog's count, that the list changed. */
function changed(): void {
  const st = snapshot()
  emitUntyped('tasks:changed', st)
  if (st.running !== lastRunning) {
    lastRunning = st.running
    setRunningTasks(st.running)
  }
}

function find(id: string): Task {
  const t = m().tasks.find((x) => x.id === id)
  if (!t) throw fail(ErrorCode.ErrInvalidInput, 'invalid value for task', { field: 'task' })
  return t
}

function repoName(id: string): string {
  const w = getWorld()
  const r = w.repos.find((x) => x.id === id)
  return r ? r.localName : id
}

function fallbackTitle(text: string): string {
  const line = text.split('\n').map((l) => l.trim()).find((l) => l !== '') ?? ''
  return [...line].slice(0, 40).join('').trim()
}

function ended(t: Task, stopped: boolean): void {
  t.live = false
  t.state = stopped ? 'stopped' : 'done'
  t.reason = ''
  t.endedAt = iso(now())
  t.resumable = true
}

export const mockTaskService: TaskServiceShape = {
  List: () => run(READ, snapshot),

  Create: (req) =>
    run(SLOW, () => {
      const x = m()
      if (!['claude', 'codex', 'opencode'].includes(req.tool)) throw fail(ErrorCode.ErrInvalidInput, 'invalid value for tool', { field: 'tool' })
      if (req.where === 'own') throw fail(ErrorCode.ErrInvalidInput, 'invalid value for where', { field: 'where' })
      const repo = getWorld().repos.find((r) => r.id === req.repo)
      if (!repo) throw fail(ErrorCode.ErrRepoNotFound, 'unknown repository', { path: req.repo })
      const live = x.tasks.filter((t) => t.live).length
      if (!req.confirmed && live >= x.cap) {
        throw fail(ErrorCode.ErrInvalidInput, `${live} tasks are already running`, { field: 'cap', running: String(live), cap: String(x.cap) })
      }
      x.seq++
      const n = x.tasks.filter((t) => t.repo === req.repo).length + 1
      const desc = req.description.trim()
      const zh = getWorld().appInfo.uiLanguage === 'zh-CN'
      const t = mk({
        id: `t-${String(x.seq).padStart(10, '0')}`, repo: req.repo, dir: repo.path, tool: req.tool, number: n,
        title: desc ? fallbackTitle(desc) : zh ? `新任务 ${n}` : `New task ${n}`,
        slug: desc ? `task-${n}` : `new-task-${n}`,
        namedBy: desc ? 'fallback' : 'pending', state: 'starting', startedAt: iso(now()), createdAt: iso(now()),
      })
      x.tasks.push(t)
      // a started tool says so a moment later (hooks), and a task with a description is working at once
      x.timers.push(
        setTimeout(() => {
          if (t.state === 'starting' && t.live) {
            t.state = desc ? 'running' : 'waiting'
            t.reason = desc ? '' : 'idle'
            changed()
          }
        }, 1500),
      )
      queueMicrotask(changed)
      return t
    }),

  Stop: (id) =>
    run(WRITE, () => {
      const t = find(id)
      if (t.live) {
        ended(t, true)
        delete t.exitCode
        changed()
      }
      return t
    }),

  Resume: (id, confirmed) =>
    run(WRITE, () => {
      const x = m()
      const t = find(id)
      if (t.live) throw fail(ErrorCode.ErrInvalidInput, 'invalid value for run', { field: 'run' })
      const live = x.tasks.filter((y) => y.live).length
      if (!confirmed && live >= x.cap) {
        throw fail(ErrorCode.ErrInvalidInput, `${live} tasks are already running`, { field: 'cap', running: String(live), cap: String(x.cap) })
      }
      t.live = true
      t.state = 'starting'
      t.reason = ''
      t.resumable = false
      t.startedAt = iso(now())
      delete t.endedAt
      delete t.exitCode
      x.timers.push(
        setTimeout(() => {
          if (t.state === 'starting' && t.live) {
            t.state = 'running-no-signal'
            t.resumed = true
            changed()
          }
        }, 1500),
      )
      queueMicrotask(changed)
      return t
    }),

  Rename: (id, title) =>
    run(WRITE, () => {
      const t = find(id)
      const v = title.trim()
      if (v === '' || [...v].length > 80 || /[\u0000-\u001f]/.test(v)) throw fail(ErrorCode.ErrInvalidInput, 'invalid value for title', { field: 'title' })
      t.title = v
      t.namedBy = 'manual'
      changed()
      return t
    }),

  LiveSummary: () =>
    run(READ, (): LiveSummary => {
      const out: LiveSummary = { sessions: [], live: 0, closed: 0, programs: 0 }
      for (const t of m().tasks) {
        if (!t.live && t.state !== 'closed-running') continue
        out.sessions.push({ task: t.id, repo: t.repo, repoName: repoName(t.repo), tool: t.tool, title: t.title, state: t.state, programs: t.leftovers.others })
        out.programs += t.leftovers.others
        if (t.live) out.live++
        else out.closed++
      }
      return out
    }),

  StopAll: () =>
    run(SLOW, (): StopAllResult => {
      let stopped = 0
      for (const t of m().tasks) {
        if (t.live) {
          ended(t, true)
          delete t.exitCode
          stopped++
        }
        if (t.state === 'closed-running') {
          t.state = 'stopped'
          t.leftovers = { count: 0, others: 0, cpu: 0, memory: 0 }
        }
      }
      changed()
      return { stopped, survivors: 0 }
    }),
}

/** The tool's display name, as the notification and the page word it. */
export function toolName(tool: Task['tool']): string {
  return TOOL_NAMES[tool]
}
