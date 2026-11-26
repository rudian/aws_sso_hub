import { CancelError, Call } from '@wailsio/runtime'
import { ErrorCode } from './models'
import type { AppError } from './models'
import type { MessageKey } from '../i18n'

export interface UiError {
  code: ErrorCode
  message: string
  detail?: string
  params: Record<string, string>
}

const knownCodes = new Set<string>(Object.values(ErrorCode).filter((v) => v !== ''))

function asAppError(v: unknown): AppError | null {
  let x = v
  if (typeof x === 'string') {
    try {
      x = JSON.parse(x)
    } catch {
      return null
    }
  }
  if (x && typeof x === 'object' && typeof (x as AppError).code === 'string') return x as AppError
  return null
}

function fromAppError(a: AppError): UiError {
  const code = knownCodes.has(a.code) ? a.code : ErrorCode.ErrInternal
  const params: Record<string, string> = {}
  for (const [k, v] of Object.entries(a.params ?? {})) if (typeof v === 'string') params[k] = v
  return { code, message: a.message ?? '', detail: a.detail || undefined, params }
}

export function isCancel(e: unknown): boolean {
  return e instanceof CancelError || (e instanceof Error && e.name === 'CancelError')
}

/** Call.RuntimeError -> e.cause as AppError; CancelError -> E_CANCELLED; anything else -> E_INTERNAL. */
export function toUiError(e: unknown): UiError {
  if (isCancel(e)) return { code: ErrorCode.ErrCancelled, message: 'cancelled', params: {} }
  if (e instanceof Call.RuntimeError || (e instanceof Error && e.name === 'RuntimeError')) {
    const app = asAppError((e as Error & { cause?: unknown }).cause) ?? asAppError(e.message)
    if (app) return fromAppError(app)
  }
  const app = e instanceof Error ? asAppError((e as Error & { cause?: unknown }).cause) : asAppError(e)
  if (app) return fromAppError(app)
  const message = e instanceof Error ? e.message : typeof e === 'string' ? e : 'unknown error'
  return { code: ErrorCode.ErrInternal, message, detail: message, params: {} }
}

const k = (code: string, body = false): MessageKey => (`errors.${code}${body ? '.body' : ''}`) as MessageKey
const e = (code: ErrorCode, body = false): { title: MessageKey; body?: MessageKey } => {
  const c = code.toString()
  return body ? { title: k(c), body: k(c, true) } : { title: k(c) }
}

/** Error code table. $zero maps to E_INTERNAL. Texts live in i18n/locales/errors.ts (and app.ts for the shared keys). */
export const errorKeys: Record<ErrorCode, { title: MessageKey; body?: MessageKey }> = {
  [ErrorCode.$zero]: { title: 'error.generic.title', body: 'errors.E_INTERNAL' },
  [ErrorCode.ErrInternal]: { title: 'error.generic.title', body: 'errors.E_INTERNAL' },
  [ErrorCode.ErrCancelled]: e(ErrorCode.ErrCancelled),
  [ErrorCode.ErrNotImplemented]: e(ErrorCode.ErrNotImplemented),
  [ErrorCode.ErrInvalidInput]: e(ErrorCode.ErrInvalidInput),
  [ErrorCode.ErrConfigParse]: { title: 'error.configParse' },
  [ErrorCode.ErrConfigChanged]: e(ErrorCode.ErrConfigChanged),
  [ErrorCode.ErrReadOnly]: { title: 'error.readOnly.tooltip' },
  [ErrorCode.ErrFileIO]: e(ErrorCode.ErrFileIO),
  [ErrorCode.ErrProfileNotFound]: e(ErrorCode.ErrProfileNotFound),
  [ErrorCode.ErrProfileExists]: e(ErrorCode.ErrProfileExists),
  [ErrorCode.ErrPortalNotFound]: e(ErrorCode.ErrPortalNotFound),
  [ErrorCode.ErrPortalExists]: e(ErrorCode.ErrPortalExists),
  [ErrorCode.ErrPortalInUse]: e(ErrorCode.ErrPortalInUse),
  [ErrorCode.ErrInvalidName]: e(ErrorCode.ErrInvalidName),
  [ErrorCode.ErrInvalidStartURL]: e(ErrorCode.ErrInvalidStartURL),
  [ErrorCode.ErrInvalidRegion]: e(ErrorCode.ErrInvalidRegion),
  [ErrorCode.ErrNotSignedIn]: e(ErrorCode.ErrNotSignedIn),
  [ErrorCode.ErrSessionExpired]: e(ErrorCode.ErrSessionExpired),
  [ErrorCode.ErrLoginInProgress]: e(ErrorCode.ErrLoginInProgress),
  [ErrorCode.ErrLoginDenied]: e(ErrorCode.ErrLoginDenied, true),
  [ErrorCode.ErrLoginTimeout]: e(ErrorCode.ErrLoginTimeout, true),
  [ErrorCode.ErrLoginPort]: e(ErrorCode.ErrLoginPort, true),
  [ErrorCode.ErrLoginState]: e(ErrorCode.ErrLoginState),
  [ErrorCode.ErrLoginFailed]: e(ErrorCode.ErrLoginFailed),
  [ErrorCode.ErrNetwork]: { title: k('E_NETWORK'), body: 'error.network' },
  [ErrorCode.ErrAWSAccessDenied]: e(ErrorCode.ErrAWSAccessDenied),
  [ErrorCode.ErrAWSInvalidKey]: e(ErrorCode.ErrAWSInvalidKey),
  [ErrorCode.ErrAWSAPI]: e(ErrorCode.ErrAWSAPI),
  [ErrorCode.ErrRoleNotAssigned]: e(ErrorCode.ErrRoleNotAssigned),
  [ErrorCode.ErrVerifyWrongAccount]: e(ErrorCode.ErrVerifyWrongAccount, true),
  [ErrorCode.ErrMigrateConfirmOther]: e(ErrorCode.ErrMigrateConfirmOther),
  [ErrorCode.ErrMigrateState]: e(ErrorCode.ErrMigrateState),
  [ErrorCode.ErrNoStaticKey]: e(ErrorCode.ErrNoStaticKey),
  [ErrorCode.ErrAWSCLIMissing]: { title: k('E_AWS_CLI_MISSING'), body: 'banner.cliMissing' },
  [ErrorCode.ErrAWSCLITooOld]: e(ErrorCode.ErrAWSCLITooOld),
  [ErrorCode.ErrGitMissing]: e(ErrorCode.ErrGitMissing),
  [ErrorCode.ErrGitTooOld]: e(ErrorCode.ErrGitTooOld),
  [ErrorCode.ErrGitFailed]: e(ErrorCode.ErrGitFailed),
  [ErrorCode.ErrRepoNotFound]: e(ErrorCode.ErrRepoNotFound),
  [ErrorCode.ErrNotCodeCommit]: e(ErrorCode.ErrNotCodeCommit),
  [ErrorCode.ErrRemoteSSH]: e(ErrorCode.ErrRemoteSSH, true),
  [ErrorCode.ErrRemoteGRC]: e(ErrorCode.ErrRemoteGRC),
  [ErrorCode.ErrUnsafeValue]: e(ErrorCode.ErrUnsafeValue),
  [ErrorCode.ErrDestExists]: e(ErrorCode.ErrDestExists),
  [ErrorCode.ErrCloneFailed]: e(ErrorCode.ErrCloneFailed),
  [ErrorCode.ErrURLRulesFirst]: e(ErrorCode.ErrURLRulesFirst),
  [ErrorCode.ErrBackupNotFound]: e(ErrorCode.ErrBackupNotFound),
  [ErrorCode.ErrRestoreNeedsConfirm]: e(ErrorCode.ErrRestoreNeedsConfirm),
  [ErrorCode.ErrSandbox]: e(ErrorCode.ErrSandbox),
  [ErrorCode.ErrUnsupportedOS]: e(ErrorCode.ErrUnsupportedOS),
  [ErrorCode.ErrImportFormat]: e(ErrorCode.ErrImportFormat, true),
  [ErrorCode.ErrChromeMissing]: e(ErrorCode.ErrChromeMissing, true),
}
