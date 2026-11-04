import { ProfileKind } from '../../api/models'
import type { Profile } from '../../api/models'

export type ShellId = 'posix' | 'powershell' | 'cmd'

/**
 * A null command means it cannot be shown safely (an unsafe name) or does not apply (static keys have no sign-in).
 * Use {@link hasUnsafeName} to tell the two apart.
 */
export interface TerminalCommands {
  useHere: string | null
  signIn: string | null
  whoami: string | null
  clone: string
}

// Same patterns as the Go side. Names come from ~/.aws/config, which anyone or any tool can edit, so a listed
// name is not trusted: it is checked here before it is put into a command the user may paste into a shell.
export const PROFILE_NAME_RE = /^[A-Za-z0-9_.@+=,-]{1,64}$/
export const SESSION_NAME_RE = /^[A-Za-z0-9_.-]{1,64}$/
export const REGION_RE = /^[a-z]{2}(-gov|-iso[a-z]*)?-[a-z]+-[0-9]{1,2}$/

const DEFAULT_REGION = 'us-east-1'

export function isSafeProfileName(name: string): boolean {
  return PROFILE_NAME_RE.test(name)
}

export function isSafeSessionName(name: string): boolean {
  return SESSION_NAME_RE.test(name)
}

/** CodeCommit endpoint host for a region; China regions use the .com.cn suffix. */
export function codecommitHost(region: string): string {
  return region.startsWith('cn-') ? `git-codecommit.${region}.amazonaws.com.cn` : `git-codecommit.${region}.amazonaws.com`
}

const CONTROL_RE = /[\x00-\x1f\x7f-\x9f]/

/**
 * POSIX shells: bare when every character is harmless, otherwise single quotes with the '\'' idiom. A value
 * that starts with "=" (or has "=" right after a ":") is quoted: zsh, the macOS default shell, replaces a word
 * like =ls by the path of that command, both as an argument and after the "=" of `export NAME=...`.
 */
export function posixQuote(value: string): string {
  if (/^[A-Za-z0-9_.@+,/:-][A-Za-z0-9_.@+=,/:-]*$/.test(value) && !value.includes(':=')) return value
  return `'${value.replace(/'/g, `'\\''`)}'`
}

/** PowerShell single-quoted string: the quote and its typographic variants (U+2018 to U+201B) are doubled. */
export function powershellQuote(value: string): string {
  return `'${value.replace(/['‘-‛]/g, (c) => c + c)}'`
}

/** PowerShell command-line argument: bare only for plain words (a comma or a leading @ would change its meaning). */
function powershellArg(value: string): string {
  return /^[A-Za-z0-9_][A-Za-z0-9_.@+=-]*$/.test(value) ? value : powershellQuote(value)
}

/** The value of `set "NAME=<value>"` in cmd, or null when no quoting can make it safe. */
export function cmdValue(value: string): string | null {
  if (CONTROL_RE.test(value) || /[%^&|<>"!]/.test(value)) return null
  return value
}

function cmdSet(name: string, value: string): string | null {
  const v = cmdValue(value)
  return v === null ? null : `set "${name}=${v}"`
}

/** Whether the profile (or the SSO session it signs in with) has a name that cannot be placed in a command. */
export function hasUnsafeName(profile: Pick<Profile, 'name' | 'kind' | 'portal'>): boolean {
  if (!isSafeProfileName(profile.name)) return true
  return profile.kind === ProfileKind.ProfileKindSSO && !!profile.portal && !isSafeSessionName(profile.portal)
}

/**
 * The command texts of the terminal helper. A command that would contain a name failing its pattern is null; the
 * remaining names are quoted for the chosen shell.
 */
export function terminalCommands(profile: Pick<Profile, 'name' | 'kind' | 'portal' | 'region'>, shell: ShellId): TerminalCommands {
  const profileOk = isSafeProfileName(profile.name)
  const arg = (v: string): string | null => (shell === 'posix' ? posixQuote(v) : shell === 'powershell' ? powershellArg(v) : cmdValue(v))

  let useHere: string | null = null
  let whoami: string | null = null
  let signIn: string | null = null
  if (profileOk) {
    const name = profile.name
    useHere = shell === 'posix' ? `export AWS_PROFILE=${posixQuote(name)}` : shell === 'powershell' ? `$env:AWS_PROFILE = ${powershellQuote(name)}` : cmdSet('AWS_PROFILE', name)
    const a = arg(name)
    whoami = a === null ? null : `aws sts get-caller-identity --profile ${a}`
    if (profile.kind === ProfileKind.ProfileKindLegacySSO) signIn = a === null ? null : `aws sso login --profile ${a}`
  }
  if (profile.kind === ProfileKind.ProfileKindSSO && profile.portal && isSafeSessionName(profile.portal)) {
    const s = arg(profile.portal)
    signIn = s === null ? null : `aws sso login --sso-session ${s}`
  }

  const region = profile.region && REGION_RE.test(profile.region) ? profile.region : DEFAULT_REGION
  return {
    useHere,
    signIn,
    whoami,
    clone: `git clone https://${codecommitHost(region)}/v1/repos/<repo-name>`,
  }
}

export function defaultShell(platform: string): ShellId {
  return platform === 'windows' ? 'powershell' : 'posix'
}
