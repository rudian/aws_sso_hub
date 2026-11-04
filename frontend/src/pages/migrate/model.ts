// Pure helpers for the migration wizard. No React, no API calls.
import { DeactivationMethod, DeactivationStatus, MigrationStep, ProfileKind } from '../../api/models'
import type { DeactivationResult, MigrationState, Portal, Profile } from '../../api/models'
import type { MessageKey, T } from '../../i18n'
import type { StepperStep } from '../../components/Stepper'

/** The seven steps the rail shows, in order. `done` is not a step of its own. */
export const STEPS: MigrationStep[] = [
  MigrationStep.MigrationIdentify,
  MigrationStep.MigrationPortal,
  MigrationStep.MigrationAccount,
  MigrationStep.MigrationReview,
  MigrationStep.MigrationVerify,
  MigrationStep.MigrationRemoveKey,
  MigrationStep.MigrationDeactivate,
]

const STEP_LABEL: Record<string, MessageKey> = {
  [MigrationStep.MigrationIdentify]: 'migrate.step1',
  [MigrationStep.MigrationPortal]: 'migrate.step2',
  [MigrationStep.MigrationAccount]: 'migrate.step3',
  [MigrationStep.MigrationReview]: 'migrate.step4',
  [MigrationStep.MigrationVerify]: 'migrate.step5',
  [MigrationStep.MigrationRemoveKey]: 'migrate.step6',
  [MigrationStep.MigrationDeactivate]: 'migrate.step7',
}

export function stepIndex(step: MigrationStep): number {
  return step === MigrationStep.MigrationDone ? STEPS.length : STEPS.indexOf(step)
}

/** The step before `step` that the UI may go back to (nothing after the config was written). */
export function previousStep(step: MigrationStep): MigrationStep | null {
  const i = stepIndex(step)
  return i >= 1 && i <= 3 ? STEPS[i - 1] : null
}

/** Rail entries for the step shown (`view`). Hints summarise what was chosen. */
export function railSteps(view: MigrationStep, st: MigrationState | null, t: T): StepperStep[] {
  const cur = stepIndex(view)
  return STEPS.map((id, i) => {
    let state: StepperStep['state'] = i < cur ? 'done' : i === cur ? 'current' : 'todo'
    if (id === MigrationStep.MigrationVerify && i === cur && st?.verify && !st.verify.ok) state = 'error'
    let hint: string | undefined
    if (id === MigrationStep.MigrationIdentify && st?.identity.accountId) hint = t('migrate.hint.account', { id: st.identity.accountId })
    else if (id === MigrationStep.MigrationPortal && st?.portal && i < cur) hint = st.portal
    else if (id === MigrationStep.MigrationAccount && st?.role && i < cur) hint = st.role
    else if (id === MigrationStep.MigrationDeactivate) hint = t('migrate.optional')
    return { id, label: t(STEP_LABEL[id]), hint, state }
  })
}

/** The step on screen as "n of total" plus its label (for the compact rail). Null when no step is current. */
export function currentRailStep(rail: StepperStep[]): { n: number; total: number; label: StepperStep['label'] } | null {
  const i = rail.findIndex((s) => s.state === 'current' || s.state === 'error')
  return i < 0 ? null : { n: i + 1, total: rail.length, label: rail[i].label }
}

export const NEW_PORTAL = '__new__'

/**
 * Which portal the wizard pre-selects. A portal that is known to list the account (checked after sign-in) wins, then a
 * portal that already has a profile for that account, otherwise a new portal for the account's own organisation.
 */
export function suggestPortal(opts: { accountId?: string; portals: Portal[]; profiles: Profile[]; matches: ReadonlySet<string> }): string {
  const real = opts.portals.filter((p) => !p.draft && !p.legacyFormat)
  const byMatch = real.find((p) => opts.matches.has(p.name))
  if (byMatch) return byMatch.name
  if (opts.accountId) {
    const byProfile = real.find((p) => opts.profiles.some((pr) => pr.kind === ProfileKind.ProfileKindSSO && pr.portal === p.name && pr.accountId === opts.accountId))
    return byProfile ? byProfile.name : NEW_PORTAL
  }
  return real[0]?.name ?? NEW_PORTAL
}

export type Tone = 'ok' | 'warn' | 'danger' | 'info'

/** What to tell the user about the result of the IAM deactivation step. */
export function deactivationNote(d: DeactivationResult, t: T): { tone: Tone; text: string; detail?: string } {
  switch (d.status) {
    case DeactivationStatus.DeactivationDone:
      return { tone: 'ok', text: t('migrate.s7.now.ok'), detail: d.method === DeactivationMethod.DeactivateViaOldKey ? t('migrate.s7.via.key') : t('migrate.s7.via.sso') }
    case DeactivationStatus.DeactivationAlready:
      return { tone: 'ok', text: t('migrate.s7.already') }
    case DeactivationStatus.DeactivationDenied:
      return { tone: 'warn', text: t('migrate.s7.now.denied') }
    case DeactivationStatus.DeactivationRootKey:
      return { tone: 'warn', text: t('migrate.s7.rootKey') }
    case DeactivationStatus.DeactivationKeyNotFound:
      return { tone: 'info', text: t('migrate.s7.notFound') }
    default:
      return { tone: 'info', text: t('migrate.s7.invalidKey') }
  }
}

/** Static-key profiles other than `current`, in config order (offered as "Migrate X next"). */
export function nextStatic(profiles: Profile[], current: string): Profile | undefined {
  return profiles.find((p) => p.kind === ProfileKind.ProfileKindStatic && p.name !== current)
}

/** Deep link to the IAM user's security credentials page (used when the portal shortcut link is not available). */
export function iamConsoleUrl(user: string): string {
  return `https://console.aws.amazon.com/iam/home#/users/details/${encodeURIComponent(user)}?section=security_credentials`
}
