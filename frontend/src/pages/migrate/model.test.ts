import { describe, expect, it } from 'vitest'
import { DeactivationMethod, DeactivationStatus, MigrationStep, ProfileKind } from '../../api/models'
import type { MigrationState, Portal, Profile } from '../../api/models'
import { makeT } from '../../i18n'
import { NEW_PORTAL, currentRailStep, deactivationNote, iamConsoleUrl, nextStatic, previousStep, railSteps, stepIndex, suggestPortal } from './model'

const en = makeT('en')
const portal = (name: string, over: Partial<Portal> = {}): Portal => ({
  name,
  startUrl: `https://${name}.awsapps.com/start`,
  startUrlHost: `${name}.awsapps.com/start`,
  region: 'us-east-1',
  legacyFormat: false,
  draft: false,
  profiles: [],
  session: { portal: name, state: 'active' as never, sessionKnown: false, sessionEndExact: false, sessionMinutes: 480, sessionOverrun: false, progressPct: 0, canRefresh: false, refreshing: false, signInWaiting: false },
  ...over,
})
const profile = (name: string, kind: ProfileKind, over: Partial<Profile> = {}): Profile => ({
  name,
  kind,
  colorSlot: 1,
  monogram: 'XX',
  isRootKey: false,
  hasStaticKeys: kind === ProfileKind.ProfileKindStatic,
  boundRepos: 0,
  isGlobalDefault: false,
  globalDefaultRepos: 0,
  migrationInProgress: false,
  ...over,
})
const state = (over: Partial<MigrationState> = {}): MigrationState => ({
  profile: 'acme',
  step: MigrationStep.MigrationIdentify,
  identity: { profile: 'acme', accountId: '111122223333', isRoot: false },
  keyInMemory: true,
  portalIsDraft: false,
  confirmOther: false,
  keyRemoved: false,
  ...over,
})

describe('migration model', () => {
  it('the rail marks earlier steps done, the shown one current, later ones todo', () => {
    const rail = railSteps(MigrationStep.MigrationReview, state({ portal: 'acme', role: 'AdministratorAccess' }), en)
    expect(rail.map((s) => s.state)).toEqual(['done', 'done', 'done', 'current', 'todo', 'todo', 'todo'])
    expect(rail[0].hint).toBe('Account 111122223333')
    expect(rail[1].hint).toBe('acme')
    expect(rail[2].hint).toBe('AdministratorAccess')
    expect(rail[6].hint).toBe('Optional')
    expect(rail[6].label).toBe('Deactivate the old key in IAM')
  })

  it('a failed verification shows the verify step as error; done completes everything', () => {
    const failed = railSteps(MigrationStep.MigrationVerify, state({ verify: { ok: false } }), en)
    expect(failed[4].state).toBe('error')
    const done = railSteps(MigrationStep.MigrationDone, state(), en)
    expect(done.every((s) => s.state === 'done')).toBe(true)
  })

  it('back is only possible before the config is written', () => {
    expect(previousStep(MigrationStep.MigrationIdentify)).toBeNull()
    expect(previousStep(MigrationStep.MigrationPortal)).toBe(MigrationStep.MigrationIdentify)
    expect(previousStep(MigrationStep.MigrationAccount)).toBe(MigrationStep.MigrationPortal)
    expect(previousStep(MigrationStep.MigrationReview)).toBe(MigrationStep.MigrationAccount)
    expect(previousStep(MigrationStep.MigrationVerify)).toBeNull()
    expect(previousStep(MigrationStep.MigrationRemoveKey)).toBeNull()
    expect(stepIndex(MigrationStep.MigrationDone)).toBe(7)
  })

  it('suggestPortal: a portal that lists the account, else one with a profile for it, else a new portal', () => {
    const portals = [portal('initech'), portal('acme')]
    const profiles = [profile('initech-admin', ProfileKind.ProfileKindSSO, { portal: 'initech', accountId: '777788889999' })]
    const none = new Set<string>()
    expect(suggestPortal({ accountId: '111122223333', portals, profiles, matches: none })).toBe(NEW_PORTAL)
    expect(suggestPortal({ accountId: '777788889999', portals, profiles, matches: none })).toBe('initech')
    expect(suggestPortal({ accountId: '111122223333', portals, profiles, matches: new Set(['acme']) })).toBe('acme')
    expect(suggestPortal({ accountId: '111122223333', portals: [portal('draft', { draft: true })], profiles, matches: none })).toBe(NEW_PORTAL)
    expect(suggestPortal({ portals, profiles, matches: none })).toBe('initech')
    expect(suggestPortal({ portals: [], profiles: [], matches: none })).toBe(NEW_PORTAL)
  })

  it('deactivationNote covers every status', () => {
    const base = { consoleUrl: 'https://x' }
    expect(deactivationNote({ ...base, status: DeactivationStatus.DeactivationDone, method: DeactivationMethod.DeactivateViaSSORole }, en)).toMatchObject({ tone: 'ok', detail: 'Done with your new SSO role' })
    expect(deactivationNote({ ...base, status: DeactivationStatus.DeactivationDone, method: DeactivationMethod.DeactivateViaOldKey }, en).detail).toBe('Done with the old key itself')
    expect(deactivationNote({ ...base, status: DeactivationStatus.DeactivationAlready }, en).text).toBe('This key is already inactive.')
    expect(deactivationNote({ ...base, status: DeactivationStatus.DeactivationDenied }, en).tone).toBe('warn')
    expect(deactivationNote({ ...base, status: DeactivationStatus.DeactivationRootKey }, en).text).toContain('root user key')
    expect(deactivationNote({ ...base, status: DeactivationStatus.DeactivationKeyNotFound }, en).tone).toBe('info')
    expect(deactivationNote({ ...base, status: DeactivationStatus.DeactivationUnavailable }, en).tone).toBe('info')
  })

  it('nextStatic and iamConsoleUrl', () => {
    const list = [profile('acme', ProfileKind.ProfileKindStatic), profile('globex', ProfileKind.ProfileKindStatic), profile('initech-admin', ProfileKind.ProfileKindSSO)]
    expect(nextStatic(list, 'acme')?.name).toBe('globex')
    expect(nextStatic(list, 'globex')?.name).toBe('acme')
    expect(nextStatic([list[0], list[2]], 'acme')).toBeUndefined()
    expect(iamConsoleUrl('acme-dev')).toBe('https://console.aws.amazon.com/iam/home#/users/details/acme-dev?section=security_credentials')
  })
})

describe('currentRailStep', () => {
  it('names the step on screen as n of total', () => {
    const rail = railSteps(MigrationStep.MigrationReview, null, en)
    expect(currentRailStep(rail)).toEqual({ n: 4, total: 7, label: 'Review changes' })
  })
  it('is null once every step is done', () => {
    expect(currentRailStep(railSteps(MigrationStep.MigrationDone, null, en))).toBeNull()
  })
})
