import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { onEvent } from '../events'
import { AIToolID } from '../models'
import type { AIInstallState } from '../models'
import { getWorld } from './world'
import { fallbackNaming, installCommand, mockAIToolService, resetMockAITools } from './aitools'

async function call<T>(p: Promise<T>, ms = 1000): Promise<T> {
  const settled = p.then((v) => ({ ok: true as const, v }), (e: unknown) => ({ ok: false as const, e }))
  await vi.advanceTimersByTimeAsync(ms)
  const r = await settled
  if (!r.ok) throw r.e
  return r.v
}

describe('mock AIToolService', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-11-17T04:00:00Z'))
    resetMockAITools('')
  })
  afterEach(() => vi.useRealTimers())

  it('default: Claude Code and OpenCode are there, Codex shows its exact install command', async () => {
    const s = await call(mockAIToolService.Detect())
    expect(s.goos).toBe('darwin')
    expect(s.tools?.map((t) => [t.id, t.installed])).toEqual([['claude', true], ['codex', false], ['opencode', true]])
    expect(s.tools?.[1].installCommand).toBe('brew install --cask codex')
    expect(s.tools?.[0]).toMatchObject({ version: '2.1.325', via: 'file' })
  })

  it('windows commands', async () => {
    resetMockAITools('?aitoolsOS=windows')
    const s = await call(mockAIToolService.Detect())
    expect(s.tools?.map((t) => t.installCommand)).toEqual([
      'winget install --id Anthropic.ClaudeCode -e',
      'winget install --id OpenAI.Codex -e',
      'npm install -g opencode-ai',
    ])
    expect(installCommand('opencode', false)).toBe('brew install opencode')
  })

  it('install streams progress, then the tool is installed', async () => {
    const evs: AIInstallState[] = []
    const stop = (onEvent as unknown as (n: string, cb: (s: AIInstallState) => void) => () => void)('aitools:install', (s) => evs.push(s))
    await call(mockAIToolService.StartInstall(AIToolID.AIToolCodex), 200)
    expect((await call(mockAIToolService.GetInstallState(), 200)).command).toBe('brew install --cask codex')
    await vi.advanceTimersByTimeAsync(3000)
    stop()
    const last = evs[evs.length - 1]
    expect(last).toMatchObject({ finished: true, running: false })
    expect(last.installed).toMatchObject({ id: 'codex', installed: true })
    expect(last.log?.length).toBe(3)
    const s = await call(mockAIToolService.Detect())
    expect(s.tools?.every((t) => t.installed)).toBe(true)
  })

  it('a second install while one runs is refused; cancel stops it', async () => {
    await call(mockAIToolService.StartInstall(AIToolID.AIToolCodex), 200)
    await expect(call(mockAIToolService.StartInstall(AIToolID.AIToolClaude), 200)).rejects.toBeTruthy()
    await call(mockAIToolService.CancelInstall(), 200)
    await vi.advanceTimersByTimeAsync(500)
    const st = await call(mockAIToolService.GetInstallState(), 200)
    expect(st).toMatchObject({ cancelled: true, finished: true, running: false })
    expect((await call(mockAIToolService.Detect())).tools?.[1].installed).toBe(false)
  })

  it('fail: the installer is missing', async () => {
    resetMockAITools('?aitools=fail')
    await call(mockAIToolService.StartInstall(AIToolID.AIToolCodex), 200)
    await vi.advanceTimersByTimeAsync(1000)
    const st = await call(mockAIToolService.GetInstallState(), 200)
    expect(st.failure).toMatchObject({ cause: 'installer-missing' })
  })

  it('naming: fallback rules and the empty description', async () => {
    expect(fallbackNaming('Fix the login bug\nmore', 3)).toMatchObject({ title: 'Fix the login bug', slug: 'fix-the-login-bug-more' })
    expect(fallbackNaming('修复登录页面', 3)).toMatchObject({ title: '修复登录页面', slug: 'task-3' })
    expect(fallbackNaming('x'.repeat(60), 1).title).toHaveLength(40)
    getWorld().appInfo.uiLanguage = 'en'
    expect(await call(mockAIToolService.NameTask(AIToolID.AIToolClaude, '', 4))).toMatchObject({ title: 'New task 4', slug: 'new-task-4', fromAI: false })
    getWorld().appInfo.uiLanguage = 'zh-CN'
    expect(await call(mockAIToolService.NameTask(AIToolID.AIToolClaude, '', 4))).toMatchObject({ title: '新任务 4', slug: 'new-task-4', fromAI: false, reason: 'empty' })
    expect(await call(mockAIToolService.NameTask(AIToolID.AIToolClaude, 'Add search', 4))).toMatchObject({ slug: 'add-search', fromAI: true })
  })

  it('approval vocabulary per tool', async () => {
    expect(await call(mockAIToolService.GetApproval(AIToolID.AIToolClaude, 'bypassPermissions', ''))).toMatchObject({ known: true, mode: 'bypassPermissions', kind: 'permission-mode' })
    expect(await call(mockAIToolService.GetApproval(AIToolID.AIToolClaude, '', ''))).toMatchObject({ known: false, mode: '' })
    expect(await call(mockAIToolService.GetApproval(AIToolID.AIToolClaude, 'default', ''))).toMatchObject({ known: true, mode: 'manual' })
    expect(await call(mockAIToolService.GetApproval(AIToolID.AIToolCodex, '', ''))).toMatchObject({ approval: 'on-request', sandbox: 'workspace-write' })
    expect(await call(mockAIToolService.GetApproval(AIToolID.AIToolOpenCode, '', ''))).toMatchObject({ mode: 'ask' })
  })
})
