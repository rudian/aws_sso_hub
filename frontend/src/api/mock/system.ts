import * as SystemReal from '../../../bindings/github.com/rudian/aws_sso_hub/internal/app/systemservice'
import { initialUI } from '../../lib/initialUI'
import { BackupKind, BackupReason, ErrorCode, ProfileKind } from '../models'
import type { BackupEntry, DiffLine, Settings } from '../models'
import { mockEmit } from '../events'
import { deepCopy, iso, now } from './clock'
import { HOME, SCAN_ROOT_DISPLAY, add, addBackup, ctx, del, fold, getWorld, mkPreview } from './world'
import type { World } from './world'
import { READ, WRITE, fail, run } from './util'

function needBackup(w: World, id: string): BackupEntry {
  const e = w.backups.find((b) => b.id === id)
  if (!e) throw fail(ErrorCode.ErrBackupNotFound, `backup ${id} not found`)
  return e
}

export const mockSystemService: typeof SystemReal = {
  GetAppInfo: () => run(READ, () => getWorld().appInfo),

  SetUILanguage: (lang) =>
    run(READ, () => {
      if (lang !== 'zh-CN' && lang !== 'en') throw fail(ErrorCode.ErrInvalidInput, 'unsupported language', { field: 'lang' })
      getWorld().appInfo.uiLanguage = lang
    }),

  GetSettings: () => run(READ, () => getWorld().settings),

  UpdateSettings: (p) =>
    run(WRITE, () => {
      const w = getWorld()
      const s: Settings = w.settings
      if (p.scanRoots !== undefined && p.scanRoots !== null && p.scanRoots.length === 0) throw fail(ErrorCode.ErrInvalidInput, 'at least one scan folder', { field: 'scanRoots' })
      if (p.language != null) s.language = p.language
      if (p.theme != null) s.theme = p.theme
      if (p.motion != null) s.motion = p.motion
      if (p.scanRoots) s.scanRoots = [...p.scanRoots]
      if (p.reminders) {
        if (![15, 30, 60].includes(p.reminders.leadMinutes)) throw fail(ErrorCode.ErrInvalidInput, 'lead minutes must be 15, 30 or 60', { field: 'reminders.leadMinutes' })
        s.reminders = { ...p.reminders }
      }
      if (p.tray) s.tray = { ...p.tray }
      if (p.terminalApp != null) s.terminalApp = p.terminalApp
      if (p.backupRetention != null) {
        if (![0, 20, 50].includes(p.backupRetention)) throw fail(ErrorCode.ErrInvalidInput, 'retention must be 20, 50 or 0', { field: 'backupRetention' })
        s.backupRetention = p.backupRetention
      }
      if (p.tutorialDone) s.tutorialDone = [...p.tutorialDone]
      if (p.tutorialLang != null) s.tutorialLang = p.tutorialLang
      if (p.awsCliPath != null) s.awsCliPath = p.awsCliPath
      if (p.gitPath != null) s.gitPath = p.gitPath
      if (p.autoBindOnAdd != null) s.autoBindOnAdd = p.autoBindOnAdd
      if (p.privateSignIn != null) s.privateSignIn = p.privateSignIn
      if (p.collapsedPortals) s.collapsedPortals = [...p.collapsedPortals]
      if (p.lastCloneProfile != null) s.lastCloneProfile = p.lastCloneProfile
      if (p.lastCloneRegion != null) s.lastCloneRegion = p.lastCloneRegion
      mockEmit('settings:changed', deepCopy(s))
      return s
    }),

  CompleteFirstRun: () =>
    run(WRITE, () => {
      getWorld().appInfo.firstRun = false
    }),

  FirstRunSummary: () =>
    run(READ, () => {
      const w = getWorld()
      const profiles = w.accounts.profiles ?? []
      return {
        profiles: profiles.length,
        portals: (w.accounts.portals ?? []).filter((p) => !p.draft).length,
        static: profiles.filter((p) => p.kind === ProfileKind.ProfileKindStatic).length,
        repos: w.repos.length,
        scanRoots: [SCAN_ROOT_DISPLAY],
      }
    }),

  CheckEnvironment: () =>
    run(700, () => {
      const w = getWorld()
      w.env.checkedAt = iso(now())
      return w.env
    }),

  CheckHealth: () =>
    run(700, () => {
      const w = getWorld()
      w.health.checkedAt = iso(now())
      return w.health
    }),

  ListBackups: () => run(READ, () => getWorld().backups),

  PreviewRestore: (id) =>
    run(READ, () => {
      const w = getWorld()
      const entry = needBackup(w, id)
      const lines: DiffLine[] = [fold(6), del('# current'), add('# from backup'), ctx('...'), fold(8)]
      const lost = entry.redacted && entry.kind === BackupKind.BackupAWSCredentials && entry.reasonParams?.profile ? [entry.reasonParams.profile] : []
      return { entry, preview: mkPreview(w, entry.file, entry.path, lines), lostProfiles: lost }
    }),

  RestoreBackup: (id, confirmLoss) =>
    run(WRITE, () => {
      const w = getWorld()
      const entry = needBackup(w, id)
      const lost = entry.redacted && entry.kind === BackupKind.BackupAWSCredentials && entry.reasonParams?.profile ? [entry.reasonParams.profile] : []
      if (lost.length > 0 && !confirmLoss) throw fail(ErrorCode.ErrRestoreNeedsConfirm, 'restoring loses key values', { profiles: lost.join(', ') })
      const safety = addBackup(w, entry.kind, BackupReason.ReasonRestore, { file: entry.file }, { path: entry.path, file: entry.file })
      mockEmit('accounts:changed', deepCopy(w.accounts))
      return safety
    }),

  RevealBackup: (id) =>
    run(READ, () => {
      console.info('[mock] reveal backup', needBackup(getWorld(), id).id)
    }),
  OpenBackupFolder: () =>
    run(READ, () => {
      console.info('[mock] open backup folder')
    }),

  OpenURL: (url) =>
    run(READ, () => {
      if (!/^https:\/\//i.test(url)) throw fail(ErrorCode.ErrInvalidInput, 'only https:// links can be opened', { field: 'url' })
      window.open(url, '_blank', 'noopener,noreferrer')
    }),
  RevealPath: (path) =>
    run(READ, () => {
      // Like the backend: the display form "~/..." (used by Health findings) is expanded first.
      const abs = path === '~' ? HOME : path.startsWith('~/') ? `${HOME}/${path.slice(2)}` : path
      const inside = abs === HOME || abs.startsWith(`${HOME}/`)
      if (!inside || abs.split('/').includes('..')) throw fail(ErrorCode.ErrInvalidInput, 'path must be under the home folder', { field: 'path' })
      console.info('[mock] reveal path', abs)
    }),
  OpenAWSConfig: () =>
    run(READ, () => {
      console.info('[mock] open ~/.aws/config')
    }),
  ChooseFolder: (_title) => run(READ, () => initialUI.addPath ?? '/Users/alex/work/legacy-api'),
  OpenTerminal: (profile, dir) =>
    run(READ, () => {
      console.info('[mock] open terminal', profile, dir)
    }),
}

export type { World }
