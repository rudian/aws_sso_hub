// Hand-written mirror of the Go types in internal/api/system.go (the tools gate, F1-12) and of the bound methods of
// internal/app/toolservice.go. `wails3 generate bindings` makes no module for ToolService until the wiring task
// registers it in main.go; until then the mock is typed against these. After the wiring task regenerates the
// bindings, delete this file and import the generated `toolservice` module and the models instead: the names,
// fields and JSON shapes below are the generated ones (a Go slice is `T[] | null`, an omitempty field is optional).
import type { CancellablePromise } from '@wailsio/runtime'

export type SetupToolID = 'homebrew' | 'winget' | 'git' | 'awscli'
export type ToolStatus = 'ok' | 'outdated' | 'too-old' | 'v1' | 'missing' | 'error'
export type SetupFix = '' | 'install' | 'upgrade' | 'repair-prefix' | 'open-store' | 'use-detected'
export type Tri = 'yes' | 'no' | 'unknown'

export interface SetupTool {
  id: SetupToolID
  status: ToolStatus
  version?: string
  min?: string
  path?: string
  blocking: boolean
  fix: SetupFix
  elsewhere?: boolean
  override?: boolean
}

export interface ProxyInfo {
  hostPort?: string
  source?: string
  pacOnly?: boolean
  unsupported?: string
}

export interface Preflight {
  admin: Tri
  managed: Tri
  freeBytes: number
  needBytes: number
  enoughSpace: boolean
  prefixExists: boolean
  proxy: ProxyInfo
}

export interface SetupState {
  platform: string
  tools: SetupTool[] | null
  ready: number
  total: number
  blocked: boolean
  preflight: Preflight
  checkedAt: string
}

export type InstallStepKind = 'download' | 'verify' | 'install'
export type InstallStepState = 'queued' | 'running' | 'waiting-os' | 'done' | 'failed' | 'skipped'

export interface InstallStep {
  id: string
  tool: SetupToolID
  kind: InstallStepKind
  state: InstallStepState
  command?: string
  indeterminate?: boolean
  done?: number
  total?: number
}

export type FailureCause =
  | 'not_admin'
  | 'cancelled'
  | 'auth_denied'
  | 'no_gui_session'
  | 'network'
  | 'proxy_auth'
  | 'policy'
  | 'disk_full'
  | 'damaged_download'
  | 'bad_signature'
  | 'unknown_signer'
  | 'not_notarized'
  | 'prefix_exists'
  | 'winget_missing'
  | 'needs_admin'
  | 'reboot_required'
  | 'installer_failed'
  | 'unknown'

export interface InstallFailure {
  tool: SetupToolID
  stepId: string
  cause: FailureCause
  command?: string
  exitCode: number
  proxy?: string
  line?: string
}

export interface InstallState {
  running: boolean
  cancelling: boolean
  cancelled: boolean
  steps: InstallStep[] | null
  log: string[] | null
  failure?: InstallFailure | null
  finished: boolean
}

/** The bound methods of ToolService (Go: GetSetupState, StartSetupInstall, CancelSetupInstall, GetInstallState, OpenInstallPage). */
export interface ToolServiceApi {
  GetSetupState(): CancellablePromise<SetupState>
  StartSetupInstall(): CancellablePromise<void>
  CancelSetupInstall(): CancellablePromise<void>
  GetInstallState(): CancellablePromise<InstallState>
  OpenInstallPage(tool: SetupToolID): CancellablePromise<void>
}

// The event payload, as registered by RegisterEvent[api.InstallState](api.EventToolInstall).
declare module '../events' {
  interface EventPayloads {
    'tools:install-progress': InstallState
  }
}
