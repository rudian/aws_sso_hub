import { CancellablePromise } from '@wailsio/runtime'
import { initialUI } from '../../lib/initialUI'
import * as MigrationReal from '../../../bindings/github.com/rudian/codepass/internal/app/migrationservice'
import { BackupKind, BackupReason, DeactivationMethod, DeactivationStatus, ErrorCode, HealthKind, MigrationItem, MigrationStep, ProgressStatus, ProfileKind, Severity, StaticState } from '../models'
import type { MigrationAccountChoice, MigrationState, Profile } from '../models'
import { mockEmit } from '../events'
import { deepCopy } from './clock'
import { applyLoginSuccess } from './accounts'
import {
  ACCT,
  PORTAL_OF_STATIC,
  HOME,
  PORTAL_INITECH,
  STATIC_TRUTH,
  acctKeyOfId,
  addBackup,
  buildMigrationConfigPreview,
  del,
  findPortal,
  findProfile,
  getWorld,
  identifyStaticProfile,
  mkPortal,
  mkPreview,
  newSessionModel,
  recompute,
  sortPortals,
} from './world'
import type { World } from './world'
import { READ, SLOW, WRITE, fail, run, toRuntimeError } from './util'

const REGION_RE = /^[a-z]{2}(-gov|-iso[a-z]*)?-[a-z]+-[0-9]{1,2}$/
const ORDER: MigrationStep[] = [
  MigrationStep.MigrationIdentify,
  MigrationStep.MigrationPortal,
  MigrationStep.MigrationAccount,
  MigrationStep.MigrationReview,
  MigrationStep.MigrationVerify,
  MigrationStep.MigrationRemoveKey,
  MigrationStep.MigrationDeactivate,
  MigrationStep.MigrationDone,
]

function need(w: World, profile: string): MigrationState {
  const st = w.migrations[profile]
  if (!st) throw fail(ErrorCode.ErrMigrateState, 'no migration in progress for ' + profile)
  return st
}

function allow(st: MigrationState, steps: MigrationStep[]): void {
  if (!steps.includes(st.step)) throw fail(ErrorCode.ErrMigrateState, `step ${st.step} does not allow this call`)
}

function requireWritable(w: World): void {
  if (w.accounts.readOnly) throw fail(ErrorCode.ErrReadOnly, 'config file has a parse issue')
}

function publish(w: World): void {
  recompute(w)
  mockEmit('accounts:changed', deepCopy(w.accounts))
}

// ------------------------------------------------------------------ state machine, synchronous parts

function startState(w: World, profile: string): MigrationState {
  const existing = w.migrations[profile]
  if (existing) return existing
  const p = findProfile(w, profile)
  if (!p) throw fail(ErrorCode.ErrProfileNotFound, `profile ${profile} not found`, { name: profile })
  if (!p.hasStaticKeys) throw fail(ErrorCode.ErrNoStaticKey, `${profile} has no access key`, { name: profile })
  const identity = identifyStaticProfile(w, profile)
  const st: MigrationState = {
    profile,
    step: MigrationStep.MigrationIdentify,
    identity,
    keyInMemory: true,
    portalIsDraft: false,
    confirmOther: false,
    keyRemoved: false,
  }
  if (p.kind === ProfileKind.ProfileKindSSO) {
    // migrated, keys left: resume at "remove key"
    st.portal = p.portal
    st.accountId = p.accountId
    st.accountName = p.accountName
    st.role = p.role
    st.region = p.region
    st.verify = { ok: true, accountId: p.accountId }
    st.step = MigrationStep.MigrationRemoveKey
  }
  w.migrations[profile] = st
  publish(w)
  return st
}

function doChoosePortal(w: World, st: MigrationState, portal: string): void {
  allow(st, [MigrationStep.MigrationIdentify, MigrationStep.MigrationPortal, MigrationStep.MigrationAccount, MigrationStep.MigrationReview])
  const p = findPortal(w, portal)
  if (!p) throw fail(ErrorCode.ErrPortalNotFound, `portal ${portal} not found`, { portal })
  st.portal = portal
  st.portalIsDraft = p.draft
  st.accountId = undefined
  st.accountName = undefined
  st.role = undefined
  st.region = undefined
  st.confirmOther = false
  st.configPreview = undefined
  st.step = MigrationStep.MigrationAccount
}

function doChooseAccount(w: World, st: MigrationState, c: MigrationAccountChoice): void {
  allow(st, [MigrationStep.MigrationAccount, MigrationStep.MigrationReview])
  if (!/^\d{12}$/.test(c.accountId)) throw fail(ErrorCode.ErrInvalidInput, 'account id must be 12 digits', { field: 'accountId' })
  if (!c.role) throw fail(ErrorCode.ErrInvalidInput, 'role required', { field: 'role' })
  if (c.region && !REGION_RE.test(c.region)) throw fail(ErrorCode.ErrInvalidRegion, 'invalid region')
  if (st.identity.accountId && st.identity.accountId !== c.accountId && !c.confirmOther) throw fail(ErrorCode.ErrMigrateConfirmOther, 'confirm pointing to another account')
  st.accountId = c.accountId
  st.accountName = c.accountName
  st.role = c.role
  st.region = c.region
  st.confirmOther = c.confirmOther
  st.configPreview = undefined
  st.verify = undefined
  st.step = MigrationStep.MigrationReview
  void w
}

function doPreview(w: World, st: MigrationState): void {
  allow(st, [MigrationStep.MigrationReview])
  const p = findProfile(w, st.profile) as Profile
  st.configPreview = buildMigrationConfigPreview(w, st.profile, st.portal as string, st.accountId as string, st.role as string, st.region || p.region || 'us-east-1')
}

function doApply(w: World, st: MigrationState): void {
  const p = findProfile(w, st.profile) as Profile
  const portal = findPortal(w, st.portal as string)
  p.kind = ProfileKind.ProfileKindSSO
  p.portal = st.portal
  p.accountId = st.accountId
  p.accountName = st.accountName
  p.role = st.role
  if (st.region) p.region = st.region
  p.staticState = StaticState.StaticKeysLeft
  p.colorSlot = ACCT[acctKeyOfId(st.accountId as string) ?? 'acme'].slot
  if (portal) portal.draft = false
  const b = addBackup(w, BackupKind.BackupAWSConfig, BackupReason.ReasonMigrate, { profile: st.profile })
  st.configBackupId = b.id
  st.step = MigrationStep.MigrationVerify
  st.verify = { ok: true, accountId: st.accountId, arn: `arn:aws:sts::${st.accountId}:assumed-role/AWSReservedSSO_${st.role}_0123456789abcdef/alex` }
}

function doPreviewKeyRemoval(w: World, st: MigrationState): void {
  if (!(st.step === MigrationStep.MigrationVerify && st.verify?.ok) && st.step !== MigrationStep.MigrationRemoveKey) throw fail(ErrorCode.ErrMigrateState, 'verify first')
  const masked = st.identity.maskedKeyId ?? STATIC_TRUTH[st.profile]?.masked ?? 'AKIA••••••••••••XXXX'
  st.credentialsPreview = mkPreview(w, '~/.aws/credentials', `${HOME}/.aws/credentials`, [
    del(`[${st.profile}]`),
    del(`aws_access_key_id     = ${masked}`),
    del('aws_secret_access_key = ••••••••'),
  ])
  st.step = MigrationStep.MigrationRemoveKey
}

function doRemoveKey(w: World, st: MigrationState): void {
  allow(st, [MigrationStep.MigrationRemoveKey])
  const p = findProfile(w, st.profile) as Profile
  const b = addBackup(w, BackupKind.BackupAWSCredentials, BackupReason.ReasonRemoveKey, { profile: st.profile }, { redacted: true })
  st.credentialsBackupId = b.id
  st.keyRemoved = true
  st.step = MigrationStep.MigrationDeactivate
  p.staticState = StaticState.$zero
  p.maskedKeyId = undefined
  if (!st.identity.isRoot && !st.identity.errorCode) {
    w.health.findings = [
      ...(w.health.findings ?? []).filter((f) => !(f.kind === HealthKind.HealthPendingKey && f.profile === st.profile)),
      { id: `pending-deactivation:${st.profile}`, kind: HealthKind.HealthPendingKey, severity: Severity.SeverityInfo, profile: st.profile, params: { maskedKeyId: st.identity.maskedKeyId ?? '', days: '0' } },
    ]
  }
}

function doDeactivate(w: World, st: MigrationState): void {
  allow(st, [MigrationStep.MigrationDeactivate])
  const portal = findPortal(w, st.portal as string)
  const iamUrl = `https://console.aws.amazon.com/iam/home#/users/details/${st.identity.iamUser ?? ''}?section=security_credentials`
  const consoleUrl = `${portal?.startUrl ?? PORTAL_INITECH.startUrl}/#/console?account_id=${st.accountId}&role_name=${st.role}&destination=${encodeURIComponent(iamUrl)}`
  if (st.identity.isRoot) st.deactivation = { status: DeactivationStatus.DeactivationRootKey, consoleUrl }
  else if (st.identity.errorCode) st.deactivation = { status: DeactivationStatus.DeactivationUnavailable, consoleUrl }
  else {
    st.deactivation = {
      status: DeactivationStatus.DeactivationDone,
      method: st.profile === 'globex' && st.keyInMemory ? DeactivationMethod.DeactivateViaOldKey : DeactivationMethod.DeactivateViaSSORole,
      consoleUrl,
    }
    w.health.findings = (w.health.findings ?? []).filter((f) => !(f.kind === HealthKind.HealthPendingKey && f.profile === st.profile))
  }
}

/** A static profile migrates into the portal of its own organisation; it is created as a draft when it does not exist yet. */
function targetPortal(w: World, profile: string): string {
  const own = PORTAL_OF_STATIC[profile]
  if (!own) return PORTAL_INITECH.name
  if (!findPortal(w, own.name)) {
    w.accounts.portals = sortPortals([...(w.accounts.portals ?? []), mkPortal(own.name, own.startUrl, own.region, true)])
    w.portalModels[own.name] = newSessionModel()
  }
  return own.name
}

/** `?page=migrate&profile=<p>&step=<s>`: make Start return a state prefilled up to step `s`. */
function prefill(w: World, st: MigrationState, target: MigrationStep): void {
  const idx = ORDER.indexOf(target)
  if (idx <= 0 || st.step !== MigrationStep.MigrationIdentify) return
  if (idx === 1) {
    st.step = MigrationStep.MigrationPortal
    return
  }
  const portal = targetPortal(w, st.profile)
  const m = w.portalModels[portal] ?? (w.portalModels[portal] = newSessionModel())
  if (m.file !== 'present') applyLoginSuccess(w, portal)
  doChoosePortal(w, st, portal)
  if (idx === 2) return
  const p = findProfile(w, st.profile) as Profile
  const a = STATIC_TRUTH[st.profile]
  doChooseAccount(w, st, {
    accountId: st.identity.accountId ?? ACCT[a?.account ?? 'acme'].id,
    accountName: st.identity.accountId ? ACCT[acctKeyOfId(st.identity.accountId) ?? 'acme'].name : ACCT[a?.account ?? 'acme'].name,
    role: 'AdministratorAccess',
    region: p.region ?? '',
    confirmOther: false,
  })
  doPreview(w, st)
  if (idx === 3) return
  doApply(w, st)
  if (idx === 4) return
  doPreviewKeyRemoval(w, st)
  if (idx === 5) return
  doRemoveKey(w, st)
  if (idx === 6) return
  doDeactivate(w, st)
  st.step = MigrationStep.MigrationDone
}

export const mockMigrationService: typeof MigrationReal = {
  Start: (profile) =>
    run(SLOW, () => {
      const w = getWorld()
      const fresh = !w.migrations[profile]
      const st = startState(w, profile)
      if (fresh && initialUI.profile === profile && initialUI.step) {
        const step = ORDER.find((s) => s === initialUI.step)
        if (step) prefill(w, st, step)
      }
      publish(w)
      return st
    }),

  Get: (profile) => run(READ, () => need(getWorld(), profile)),

  ChoosePortal: (profile, portal) =>
    run(WRITE, () => {
      const w = getWorld()
      const st = need(w, profile)
      doChoosePortal(w, st, portal)
      publish(w)
      return st
    }),

  ChooseAccount: (profile, choice) =>
    run(WRITE, () => {
      const w = getWorld()
      const st = need(w, profile)
      doChooseAccount(w, st, choice)
      publish(w)
      return st
    }),

  Preview: (profile) =>
    run(WRITE, () => {
      const w = getWorld()
      const st = need(w, profile)
      doPreview(w, st)
      return st
    }),

  Apply: (profile, _baseHash) => {
    const w = getWorld()
    const timers: Array<ReturnType<typeof setTimeout>> = []
    return new CancellablePromise<MigrationState>(
      (resolve, reject) => {
        let st: MigrationState
        try {
          st = need(w, profile)
          allow(st, [MigrationStep.MigrationReview])
          if (!st.configPreview) throw fail(ErrorCode.ErrMigrateState, 'preview first')
          requireWritable(w)
        } catch (e) {
          reject(toRuntimeError(e))
          return
        }
        const prog = (item: MigrationItem, status: ProgressStatus, detail?: string) => mockEmit('migration:progress', { profile, item, status, detail })
        const at = (ms: number, fn: () => void) => timers.push(setTimeout(fn, ms))
        prog(MigrationItem.MigrationItemBackup, ProgressStatus.ProgressRunning)
        at(400, () => {
          prog(MigrationItem.MigrationItemBackup, ProgressStatus.ProgressDone, w.appInfo.backupDir)
          prog(MigrationItem.MigrationItemWrite, ProgressStatus.ProgressRunning)
        })
        at(800, () => {
          doApply(w, st)
          publish(w)
          prog(MigrationItem.MigrationItemWrite, ProgressStatus.ProgressDone)
          prog(MigrationItem.MigrationItemVerify, ProgressStatus.ProgressRunning)
        })
        at(1200, () => {
          prog(MigrationItem.MigrationItemVerify, ProgressStatus.ProgressDone, `${st.verify?.accountId} • ${st.verify?.arn}`)
          resolve(deepCopy(st))
        })
      },
      () => timers.forEach(clearTimeout),
    )
  },

  RetryVerify: (profile) =>
    run(SLOW, () => {
      const w = getWorld()
      const st = need(w, profile)
      allow(st, [MigrationStep.MigrationVerify])
      st.verify = { ok: true, accountId: st.accountId, arn: `arn:aws:sts::${st.accountId}:assumed-role/AWSReservedSSO_${st.role}_0123456789abcdef/alex` }
      return st
    }),

  RestoreConfig: (profile) =>
    run(WRITE, () => {
      const w = getWorld()
      const st = need(w, profile)
      allow(st, [MigrationStep.MigrationVerify])
      if (!st.configBackupId) throw fail(ErrorCode.ErrMigrateState, 'no config backup')
      const p = findProfile(w, profile) as Profile
      p.kind = ProfileKind.ProfileKindStatic
      p.staticState = StaticState.StaticOK
      p.portal = undefined
      p.accountId = st.identity.accountId
      p.accountName = undefined
      p.role = undefined
      const portal = findPortal(w, st.portal as string)
      if (portal && st.portalIsDraft) portal.draft = true
      const entry = addBackup(w, BackupKind.BackupAWSConfig, BackupReason.ReasonRestore, { file: '~/.aws/config' })
      void entry
      st.step = MigrationStep.MigrationReview
      st.verify = undefined
      st.configPreview = undefined
      publish(w)
      return st
    }),

  PreviewKeyRemoval: (profile) =>
    run(WRITE, () => {
      const w = getWorld()
      const st = need(w, profile)
      doPreviewKeyRemoval(w, st)
      return st
    }),

  RemoveKey: (profile, _baseHash) =>
    run(WRITE, () => {
      const w = getWorld()
      const st = need(w, profile)
      requireWritable(w)
      doRemoveKey(w, st)
      publish(w)
      return st
    }),

  KeepKey: (profile) =>
    run(READ, () => {
      const w = getWorld()
      const st = need(w, profile)
      allow(st, [MigrationStep.MigrationRemoveKey])
      st.keyRemoved = false
      st.step = MigrationStep.MigrationDeactivate
      return st
    }),

  Deactivate: (profile) =>
    run(SLOW, () => {
      const w = getWorld()
      const st = need(w, profile)
      doDeactivate(w, st)
      return st
    }),

  Finish: (profile) =>
    run(WRITE, () => {
      const w = getWorld()
      const st = need(w, profile)
      allow(st, [MigrationStep.MigrationDeactivate, MigrationStep.MigrationRemoveKey])
      st.step = MigrationStep.MigrationDone
      st.keyInMemory = false
      const final = deepCopy(st)
      delete w.migrations[profile]
      publish(w)
      return final
    }),

  Discard: (profile) =>
    run(READ, () => {
      const w = getWorld()
      delete w.migrations[profile]
      publish(w)
    }),
}
