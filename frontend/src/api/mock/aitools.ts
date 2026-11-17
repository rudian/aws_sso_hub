import * as AIToolReal from '../../../bindings/github.com/rudian/codepass/internal/app/aitoolservice'
import { mockEmit } from '../events'
import { AIApprovalKind, AIInstallCause, AIToolID, AIToolVia, ErrorCode } from '../models'
import type { AIApproval, AIInstallState, AINaming, AIToolInfo, AIToolsState } from '../models'
import { READ, fail, run } from './util'
import { getWorld as getMockWorld } from './world'

// The demo backend of the AI tools (A2-04): which of Claude Code, Codex and OpenCode are installed, the install
// run, the task's name and the approval mode of a task. Nothing runs; the demo only acts out the same answers.
//
// URL switches (mock mode only):
//   ?aitools=default|all|none|fail   default: Claude Code and OpenCode installed, Codex missing; all / none; fail:
//                                    the install of the missing tools fails (the installer is missing)
//   ?aitoolsOS=windows               show the winget and npm commands (default: macOS, brew)

type Scenario = 'default' | 'all' | 'none' | 'fail'

interface World {
  scenario: Scenario
  windows: boolean
  installed: Record<string, string> // id -> version
  install: AIInstallState
  timers: ReturnType<typeof setTimeout>[]
}

const VERSIONS: Record<string, string> = { claude: '2.1.325', codex: '0.31.2', opencode: '2.3.0' }
const NAMES: Record<string, string> = { claude: 'Claude Code', codex: 'Codex', opencode: 'OpenCode' }
const ORDER = ['claude', 'codex', 'opencode']

/** The same table as aitools.InstallFor in Go. */
export function installCommand(id: string, windows: boolean): string {
  if (windows) {
    if (id === 'claude') return 'winget install --id Anthropic.ClaudeCode -e'
    if (id === 'codex') return 'winget install --id OpenAI.Codex -e'
    return 'npm install -g opencode-ai'
  }
  if (id === 'claude') return 'brew install --cask claude-code'
  if (id === 'codex') return 'brew install --cask codex'
  return 'brew install opencode'
}

function emptyInstall(): AIInstallState {
  return { tool: '' as AIToolID, command: '', running: false, cancelling: false, cancelled: false, finished: false, log: [] }
}

let world: World | undefined

function query(): URLSearchParams {
  return new URLSearchParams(typeof window !== 'undefined' ? window.location.search : '')
}

/** Back to the state a fresh page load has (tests). */
export function resetMockAITools(search?: string): void {
  if (world) world.timers.forEach(clearTimeout)
  const q = search === undefined ? query() : new URLSearchParams(search)
  const s = q.get('aitools')
  const scenario: Scenario = s === 'all' || s === 'none' || s === 'fail' ? s : 'default'
  const installed: Record<string, string> = {}
  for (const id of ORDER) {
    if (scenario === 'all' || (scenario === 'default' && id !== 'codex')) installed[id] = VERSIONS[id]
  }
  world = { scenario, windows: q.get('aitoolsOS') === 'windows', installed, install: emptyInstall(), timers: [] }
}

function w(): World {
  if (!world) resetMockAITools()
  return world as World
}

const emit = mockEmit as unknown as (name: string, data: unknown) => void

function push(x: World): void {
  emit('aitools:install', JSON.parse(JSON.stringify(x.install)))
}

function row(x: World, id: string): AIToolInfo {
  const version = x.installed[id]
  const info: AIToolInfo = {
    id: id as AIToolID,
    name: NAMES[id],
    installed: version !== undefined,
    via: AIToolVia.AIViaNone,
    installCommand: installCommand(id, x.windows),
  }
  if (version !== undefined) {
    info.version = version
    info.via = AIToolVia.AIViaFile
    info.path = x.windows ? `C:\\Tools\\${id}\\${id}.exe` : `/opt/homebrew/bin/${id}`
  }
  return info
}

function finish(x: World, patch: Partial<AIInstallState>): void {
  x.install = { ...x.install, running: false, cancelling: false, finished: true, ...patch }
  push(x)
}

function words(text: string): string {
  const ws = text.toLowerCase().match(/[a-z0-9]+/g) ?? []
  let slug = ''
  for (const wd of ws) {
    const cand = slug ? `${slug}-${wd}` : wd
    if (cand.length > 40) {
      if (!slug) slug = wd.slice(0, 40)
      break
    }
    slug = cand
  }
  return slug
}

/** The quiet fallback of plan 4.7 "Naming": the first line cut to 40 characters, the slug from its ASCII words, else task-<n>. */
export function fallbackNaming(description: string, n: number): AINaming {
  const first = description.split('\n').find((l) => l.trim() !== '') ?? ''
  const title = [...first.trim().replace(/\s+/g, ' ')].slice(0, 40).join('').trim()
  const slug = words(description) || `task-${n}`
  return { title: title || slug, slug, fromAI: false, reason: '' }
}

export const mockAIToolService: typeof AIToolReal = {
  Detect: () =>
    run(READ, (): AIToolsState => {
      const x = w()
      return { goos: x.windows ? 'windows' : 'darwin', tools: ORDER.map((id) => row(x, id)) }
    }),

  StartInstall: (tool) =>
    run(READ, () => {
      const x = w()
      const id = tool as string
      if (!ORDER.includes(id)) throw fail(ErrorCode.ErrInvalidInput, 'invalid value for tool', { field: 'tool' })
      if (x.install.running) throw fail(ErrorCode.ErrInvalidInput, 'an install is already running', { field: 'run' })
      x.install = { ...emptyInstall(), tool: tool, command: installCommand(id, x.windows), running: true }
      push(x)
      const step = (ms: number, f: () => void) => x.timers.push(setTimeout(() => x.install.running && !x.install.cancelling && f(), ms))
      if (x.scenario === 'fail') {
        step(400, () => finish(x, { failure: { cause: AIInstallCause.AICauseInstallerMissing, detail: `${x.install.command.split(' ')[0]} is not installed`, exit: -1 } }))
        return
      }
      const lines = x.windows ? ['Found ' + NAMES[id], 'Downloading', 'Successfully installed'] : ['==> Downloading', '==> Installing Cask ' + id, `🍺  ${id} was successfully installed!`]
      lines.forEach((l, i) =>
        step(300 + i * 500, () => {
          x.install.log = [...(x.install.log ?? []), l]
          push(x)
        }),
      )
      step(300 + lines.length * 500, () => {
        x.installed[id] = VERSIONS[id]
        finish(x, { installed: row(x, id) })
      })
    }),

  CancelInstall: () =>
    run(READ, () => {
      const x = w()
      if (!x.install.running) return
      x.timers.forEach(clearTimeout)
      x.install.cancelling = true
      push(x)
      x.timers.push(setTimeout(() => finish(x, { cancelled: true }), 200))
    }),

  GetInstallState: () => run(READ, () => w().install),

  NameTask: (_tool, description, n) =>
    run(700, (): AINaming => {
      if (description.trim() === '') {
        // the same words as aitools.EmptyNaming in Go: the title follows the interface language
        const zh = getMockWorld().appInfo.uiLanguage === 'zh-CN'
        return { title: zh ? `新任务 ${n}` : `New task ${n}`, slug: `new-task-${n}`, fromAI: false, reason: 'empty' }
      }
      const f = fallbackNaming(description, n)
      // the demo's "AI" answer: the same words, so the sheet shows the naming step end to end
      return { ...f, fromAI: true }
    }),

  GetApproval: (tool, permissionMode) =>
    run(READ, (): AIApproval => {
      if (tool === AIToolID.AIToolClaude) {
        // "default" is Claude Code's own name for manual (aitools.ClaudeApproval in Go)
        const mode = permissionMode === 'default' ? 'manual' : permissionMode
        const known = ['manual', 'acceptEdits', 'auto', 'plan', 'dontAsk', 'bypassPermissions'].includes(mode)
        return { tool, kind: AIApprovalKind.AIApprovalPermissionMode, mode: known ? mode : '', approval: '', sandbox: '', known, source: known ? 'hook' : '' }
      }
      if (tool === AIToolID.AIToolCodex) {
        return { tool, kind: AIApprovalKind.AIApprovalPolicySandbox, mode: '', approval: 'on-request', sandbox: 'workspace-write', known: true, source: 'config' }
      }
      if (tool === AIToolID.AIToolOpenCode) {
        return { tool, kind: AIApprovalKind.AIApprovalRules, mode: 'ask', approval: '', sandbox: '', known: true, source: 'config' }
      }
      throw fail(ErrorCode.ErrInvalidInput, 'invalid value for tool', { field: 'tool' })
    }),
}
