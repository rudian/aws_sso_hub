// The setup screen (design 5.1): the gate. It replaces the whole window while a required tool is missing or too old.
import { Bot, Copy, Lock, ShieldCheck, TriangleAlert } from 'lucide-react'
import { useCallback, useEffect, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { toUiError } from '../../api/errors'
import type { UiError } from '../../api/errors'
import { copyText } from '../../api/platform'
import { onEvent } from '../../api/events'
import { FailureCause, SetupFix, SetupToolID } from '../../api/models'
import type { InstallFailure, InstallState, SetupState } from '../../api/models'
import { Tools } from '../../api/services'
import appIcon from '../../assets/appicon.svg'
import { Banner } from '../../components/Banner'
import { Button } from '../../components/Button'
import { ErrorBanner } from '../../components/ErrorBanner'
import { useI18n } from '../../i18n'
import { requestGateRecheck } from '../../lib/gate'
import { guessPlatform } from '../../lib/platform'
import { failureView, idleNotice, installPageTargets, logLines, logText, missingBytes, phaseOf, primaryKind, readyCount, rowViews, toolLabel } from './model'
import type { IdleNotice, Phase, PrimaryKind, RowKind } from './model'
import { SetupLog, ToolRow, stepText } from './SetupRows'
import './setup.css'

const QUIET_MS = 10_000

/** A change that counts as output: a new log line or any step moving or gaining bytes. The log is capped (200 lines), so its newest line counts too. */
function activitySignature(s: InstallState | null): string {
  if (!s) return ''
  const log = s.log ?? []
  return `${log.length}|${log[log.length - 1] ?? ''}|${(s.steps ?? []).map((x) => `${x.state}:${x.done ?? 0}`).join(',')}`
}

export function SetupPage() {
  const { t, lang } = useI18n()
  const [setup, setSetup] = useState<SetupState | null>(null)
  const [install, setInstall] = useState<InstallState | null>(null)
  const [loadError, setLoadError] = useState<UiError | null>(null)
  const [actionError, setActionError] = useState<UiError | null>(null)
  const [starting, setStarting] = useState(false)
  const [checking, setChecking] = useState(false)
  const [selfInstall, setSelfInstall] = useState(false)
  const [logOpen, setLogOpen] = useState(false)
  const [copied, setCopied] = useState(false)
  const [cancelAsked, setCancelAsked] = useState(false)
  const [stillBlocked, setStillBlocked] = useState<string | null>(null)
  const [wingetStuck, setWingetStuck] = useState(false)
  // A run just ended without a failure: the gate is read again before the footer says anything about what is missing (FU-15c)
  const [verifying, setVerifying] = useState(false)
  const [announce, setAnnounce] = useState('')
  const [now, setNow] = useState(() => Date.now())
  const primaryRef = useRef<HTMLButtonElement>(null)
  const cancelRef = useRef<HTMLButtonElement>(null)
  const focusPending = useRef(false)
  const storeOpened = useRef(false)
  const lastActivity = useRef(Date.now())
  const sig = useRef('')
  const prevKinds = useRef<Record<string, RowKind> | null>(null)
  const prevPhase = useRef<Phase | null>(null)
  const copyTimer = useRef<number | undefined>(undefined)

  const platform = setup?.platform === 'windows' ? 'windows' : setup?.platform === 'darwin' ? 'darwin' : guessPlatform()

  // ---- data ------------------------------------------------------------------------------------------------
  const refreshSetup = useCallback(async (): Promise<SetupState | null> => {
    try {
      const s = await Tools.GetSetupState()
      setSetup(s)
      setLoadError(null)
      return s
    } catch (e) {
      setLoadError(toUiError(e))
      return null
    }
  }, [])

  useEffect(() => {
    let alive = true
    void refreshSetup()
    // a reloaded window picks a run up again
    Tools.GetInstallState().then(
      (s) => alive && setInstall((cur) => cur ?? s),
      () => undefined,
    )
    const off = onEvent('tools:install-progress', (s) => setInstall(s))
    return () => {
      alive = false
      off()
    }
  }, [refreshSetup])

  // A step that finished or a run that ended changes what the gate says: read it again (local checks, no network).
  const doneSteps = (install?.steps ?? []).filter((s) => s.state === 'done').length
  const finished = !!install?.finished
  useEffect(() => {
    if (!install) return
    const clean = finished && !install.failure && !install.cancelled
    if (clean) setVerifying(true)
    void refreshSetup().finally(() => setVerifying(false))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [doneSteps, finished])

  // "Still downloading" after 10 s without output.
  const running = !!install?.running
  useEffect(() => {
    if (!running) return
    const id = window.setInterval(() => setNow(Date.now()), 1000)
    return () => window.clearInterval(id)
  }, [running])
  useEffect(() => {
    const next = activitySignature(install)
    if (next !== sig.current) {
      sig.current = next
      lastActivity.current = Date.now()
      setNow(Date.now())
    }
  }, [install])

  // ---- derived ---------------------------------------------------------------------------------------------
  const phase = phaseOf(setup, install)
  const failure = phase === 'failed' ? install?.failure : null
  const rows = setup ? rowViews(setup, install) : []
  const ready = readyCount(rows)
  const kind: PrimaryKind = primaryKind(phase, setup, failure, selfInstall)
  const quiet = phase === 'running' && now - lastActivity.current > QUIET_MS
  const cancelling = phase === 'running' && (!!install?.cancelling || cancelAsked)
  const cancelledRun = phase === 'idle' && !!install?.finished && !!install.cancelled
  const unverified = phase === 'idle' && !!install?.finished && !install.failure && !install.cancelled && !!setup?.blocked
  const notice = setup && phase === 'idle' && !selfInstall && !cancelledRun ? idleNotice(setup) : null
  const sep = lang === 'zh-CN' ? '、' : ', '
  const blockedNames = (setup?.tools ?? []).filter((t2) => t2.blocking).map((t2) => toolLabel(t2.id, platform)).join(sep)

  // ---- announcements and focus -------------------------------------------------------------------------------
  useEffect(() => {
    if (!setup) return
    const kinds: Record<string, RowKind> = Object.fromEntries(rows.map((r) => [r.tool.id, r.kind]))
    const prev = prevKinds.current
    prevKinds.current = kinds
    if (!prev) return
    // Two rows can change in one render: the last one carries the count, which is what a listener wants to hear.
    let message = ''
    for (const r of rows) {
      if (r.kind === 'ok' && prev[r.tool.id] !== 'ok') message = t('setup.live.installed', { tool: toolLabel(r.tool.id, platform), n: ready, total: setup.total })
      else if (r.kind === 'waiting' && prev[r.tool.id] !== 'waiting' && r.current) message = t('setup.live.waiting', { tool: toolLabel(r.tool.id, platform), text: stepText(r.current, platform, t) })
    }
    if (message) setAnnounce(message)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [setup, install])

  const focusedFirst = useRef(false)
  useEffect(() => {
    if (setup && !focusedFirst.current) {
      focusedFirst.current = true
      primaryRef.current?.focus()
    }
  }, [setup])

  useEffect(() => {
    const prev = prevPhase.current
    prevPhase.current = phase
    if (prev === phase) return
    // Focus starts on the primary button; after a failure it is on "Try again"; after the run it comes back to it.
    // The primary is disabled while a request is in flight, so the focus is placed by the effect below once it can take it.
    if (prev !== null || phase !== 'running') focusPending.current = true
    if (phase === 'done' && prev !== null) setAnnounce(t('setup.live.done'))
    if (phase === 'running' && prev !== null) setAnnounce(t('setup.live.started'))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [phase])

  // Places the focus a phase change or a finished check asked for, as soon as the primary is enabled again.
  const busyNow = phase === 'running' || starting || checking
  useEffect(() => {
    if (!focusPending.current) return
    if (phase === 'running') {
      // the primary is a disabled "Installing" while a run goes: Cancel is where the keyboard stays
      if (cancelRef.current && !cancelRef.current.disabled) {
        focusPending.current = false
        cancelRef.current.focus()
      }
      return
    }
    if (busyNow || !primaryRef.current || primaryRef.current.disabled) return
    focusPending.current = false
    primaryRef.current.focus()
  }, [phase, busyNow])

  // Windows: the Store page is opened outside the app; check again when the window is back (also after "I'll install them myself").
  const watchFocus = kind === 'open-store' || selfInstall
  useEffect(() => {
    if (!watchFocus) return
    // Only an explicit "Check again" may say "ask IT": coming back from the Store page proves nothing yet
    const onFocus = () => void refreshSetup()
    window.addEventListener('focus', onFocus)
    return () => window.removeEventListener('focus', onFocus)
  }, [watchFocus, refreshSetup])

  useEffect(() => () => window.clearTimeout(copyTimer.current), [])

  // ---- actions ---------------------------------------------------------------------------------------------
  /** After the Store page was opened, a check that still finds no winget is the "ask IT" case of design 5.1. */
  function noteStore(s: SetupState | null) {
    if (!s) return
    if (!s.blocked) setWingetStuck(false)
    else if (storeOpened.current && (s.tools ?? []).some((x) => x.id === SetupToolID.SetupWinget && x.blocking)) setWingetStuck(true)
  }

  const start = async () => {
    setActionError(null)
    setSelfInstall(false)
    setStillBlocked(null)
    setWingetStuck(false)
    setCancelAsked(false)
    focusPending.current = true
    setStarting(true)
    try {
      await Tools.StartSetupInstall()
      setInstall(await Tools.GetInstallState())
    } catch (e) {
      setActionError(toUiError(e))
    } finally {
      setStarting(false)
    }
  }

  const recheck = async () => {
    setActionError(null)
    setChecking(true)
    setAnnounce(t('setup.live.checking'))
    try {
      const s = await refreshSetup()
      noteStore(s)
      if (s?.blocked) {
        // a row that only waits for "Continue" (use-detected) is not something the user still has to install
        const names = (s.tools ?? []).filter((x) => x.blocking && x.fix !== SetupFix.FixUseDetected).map((x) => toolLabel(x.id, s.platform === 'windows' ? 'windows' : 'darwin')).join(sep)
        setStillBlocked(names || null)
        if (names) setAnnounce(t('setup.live.stillBlocked', { names }))
      } else if (s) {
        setStillBlocked(null)
        setSelfInstall(false)
      }
    } finally {
      focusPending.current = true
      setChecking(false)
    }
  }

  const openPage = async (id: SetupToolID) => {
    try {
      await Tools.OpenInstallPage(id)
      if (id === SetupToolID.SetupWinget) storeOpened.current = true
      return true
    } catch (e) {
      setActionError(toUiError(e))
      return false
    }
  }

  const chooseSelf = async () => {
    if (!setup) return
    setActionError(null)
    let opened = false
    for (const id of installPageTargets(setup)) opened = (await openPage(id)) || opened
    setSelfInstall(true)
    setStillBlocked(null)
    if (opened) setAnnounce(t('setup.live.opened'))
  }

  const cancel = async () => {
    setCancelAsked(true)
    try {
      await Tools.CancelSetupInstall()
      setAnnounce(t('setup.live.cancelled'))
    } catch (e) {
      setActionError(toUiError(e))
    }
  }

  const copyLog = async () => {
    try {
      await copyText(logText(install))
      setCopied(true)
      setAnnounce(t('setup.live.copied'))
      window.clearTimeout(copyTimer.current)
      copyTimer.current = window.setTimeout(() => setCopied(false), 1500)
    } catch {
      /* no clipboard: nothing to confirm */
    }
  }

  const enter = () => {
    void recheck()
    requestGateRecheck()
  }

  const onPrimary = () => {
    switch (kind) {
      case 'install':
      case 'continue':
      case 'retry':
      case 'redownload':
        return void start()
      case 'recheck':
        return void recheck()
      case 'open-store':
        return void openPage('winget' as SetupToolID)
      case 'enter':
        return enter()
      default:
    }
  }

  // ---- copy ------------------------------------------------------------------------------------------------
  const mac = platform === 'darwin'
  const primaryLabel: Record<PrimaryKind, string> = {
    install: t('setup.btn.install'),
    continue: t('setup.btn.continue'),
    retry: t('setup.btn.retry'),
    redownload: t('setup.btn.redownload'),
    recheck: t('setup.btn.recheck'),
    'open-store': t('setup.btn.openStore'),
    enter: t('setup.btn.enter'),
    busy: t('setup.btn.installing'),
  }
  const busy = kind === 'busy' || starting || checking || verifying
  let footText: string
  if (phase === 'done') footText = t('setup.foot.done')
  else if (phase === 'running') footText = cancelling ? t('setup.foot.cancelling') : t('setup.foot.running')
  else if (verifying) footText = t('setup.foot.verifying')
  else if (stillBlocked) footText = t('setup.foot.stillBlocked', { names: stillBlocked })
  else if (selfInstall && kind !== 'continue') footText = t('setup.foot.self')
  else if (phase === 'failed' && ready > 0) footText = t('setup.foot.failed')
  else if (cancelledRun) footText = t('setup.foot.cancelled')
  else if (unverified) footText = t('setup.foot.unverified', { names: blockedNames })
  else if (kind === 'continue') footText = t('setup.foot.detected')
  else if (kind === 'open-store') footText = t('setup.foot.store')
  else footText = t('setup.foot.idle')

  // Windows without winget, after the Store page was opened and a check still found nothing: the "ask IT" sentence.
  const wingetHint: InstallFailure | null = wingetStuck && kind === 'open-store' && phase === 'idle' ? { tool: SetupToolID.SetupWinget, cause: FailureCause.CauseWingetMissing, stepId: 'winget:install', exitCode: -1 } : null
  const failView = failure ? failureView(failure, platform, setup?.preflight) : wingetHint ? failureView(wingetHint, platform, setup?.preflight) : null
  const noticeView = notice && setup ? noticeCopy(notice, setup, t) : null

  const ghost = (): ReactNode => {
    if (phase === 'done') return null
    if (phase === 'running') {
      return (
        <Button ref={cancelRef} variant="ghost" onClick={() => void cancel()} disabled={cancelling}>
          {cancelling ? t('setup.btn.cancelling') : t('setup.btn.cancel')}
        </Button>
      )
    }
    if (selfInstall && kind !== 'continue') {
      return (
        <Button variant="ghost" onClick={() => setSelfInstall(false)} disabled={busy}>
          {t('setup.btn.useOneClick')}
        </Button>
      )
    }
    if (kind === 'open-store') {
      return (
        <Button variant="ghost" onClick={() => void recheck()} disabled={busy}>
          {t('setup.btn.recheck')}
        </Button>
      )
    }
    // use-detected rows have no page to open: "install them myself" would lead nowhere
    if (kind === 'continue' || !setup || installPageTargets(setup).length === 0) return null
    return (
      <Button variant="ghost" onClick={() => void chooseSelf()} disabled={busy}>
        {t('setup.btn.self')}
      </Button>
    )
  }

  return (
    <div className={`setup${phase === 'done' ? ' setup--done' : ''}`}>
      <div className="setup__bar" aria-hidden="true" />
      <div className="setup__wrap">
        <div className="setup__intro">
          <img className="setup__mark" src={appIcon} alt="" width={64} height={64} />
          <h1 className="setup__title">{t(mac ? 'setup.title.mac' : 'setup.title.win')}</h1>
          <p className="setup__lead">{t(mac ? 'setup.lead.mac' : 'setup.lead.win')}</p>
          <ul className="setup__notes">
            <li>
              <Lock aria-hidden="true" />
              <span>{t(mac ? 'setup.note.mac.1' : 'setup.note.win.1')}</span>
            </li>
            <li>
              <ShieldCheck aria-hidden="true" />
              <span>{t(mac ? 'setup.note.mac.2' : 'setup.note.win.2')}</span>
            </li>
            <li>
              <Bot aria-hidden="true" />
              <span>{t('setup.note.3')}</span>
            </li>
          </ul>
        </div>

        <section className="card setup__card" aria-labelledby="setup-card-title" aria-busy={phase === 'running' || undefined}>
          <div className="card__header">
            <h2 className="card__title" id="setup-card-title">
              {t('setup.card.title')}
            </h2>
            <span className="spacer" />
            {setup && <span className="setup__count">{t('setup.card.countOf', { n: ready, total: setup.total })}</span>}
          </div>

          {loadError && !setup ? (
            <div className="setup__fail">
              <ErrorBanner error={loadError} onRetry={() => void refreshSetup()} />
            </div>
          ) : (
            <ul className="setup__tools" aria-label={t('setup.tools.label')}>
              {rows.map((r) => (
                <ToolRow key={r.tool.id} row={r} platform={platform} quiet={quiet && r.current?.state === 'running' && !(platform === 'windows' && r.current.kind === 'install')} />
              ))}
            </ul>
          )}

          {failView && (
            <div className={failure ? 'banner banner--danger setup__fail' : 'banner banner--warn setup__fail'} role={failure ? 'alert' : 'status'}>
              {failView.icon === 'lock' ? <Lock className="banner__icon" aria-hidden="true" /> : <TriangleAlert className="banner__icon" aria-hidden="true" />}
              <div className="banner__body">
                <div className="banner__title">{t(failView.titleKey, failView.params)}</div>
                <div>{t(failView.bodyKey, failView.params)}</div>
                {failView.proxyKey && <div className="setup__proxy">{t(failView.proxyKey, failView.params)}</div>}
              </div>
            </div>
          )}
          {noticeView && (
            <Banner kind={noticeView.kind} className="setup__fail" title={noticeView.title}>
              {noticeView.body}
            </Banner>
          )}
          {actionError && (
            <div className="setup__fail">
              <ErrorBanner error={actionError} />
            </div>
          )}

          <SetupLog lines={phase === 'done' ? [] : phase === 'failed' ? logLines(install) : (install?.log ?? [])} failure={failure} expanded={logOpen} onToggle={() => setLogOpen((v) => !v)} />

          <div className="setup__foot">
            <span className="setup__foottext">{footText}</span>
            <span className="spacer" />
            {ghost()}
            {phase === 'failed' && (
              <Button icon={Copy} onClick={() => void copyLog()}>
                {copied ? t('setup.btn.logCopied') : t('setup.btn.copyLog')}
              </Button>
            )}
            <Button ref={primaryRef} variant="primary" size="lg" className="setup__primary" loading={busy} disabled={!setup && !loadError} onClick={onPrimary}>
              {primaryLabel[kind]}
            </Button>
          </div>
        </section>
      </div>
      <div className="sr-only" role="status" aria-live="polite">
        {announce}
      </div>
    </div>
  )
}

function noticeCopy(n: Exclude<IdleNotice, null>, setup: SetupState, t: ReturnType<typeof useI18n>['t']): { kind: 'warn' | 'info'; title: string; body: ReactNode } {
  const p = setup.preflight
  const gb = (b: number) => (Math.max(0, b) / 1e9).toFixed(1)
  const proxyLine = p.proxy.hostPort ? <div className="setup__proxy">{t('setup.notice.proxy', { proxy: p.proxy.hostPort })}</div> : null
  switch (n) {
    case 'not-admin':
      return { kind: 'warn', title: t('setup.notice.notAdmin.title'), body: t('setup.notice.notAdmin.body') }
    case 'managed':
      return { kind: 'warn', title: t('setup.notice.managed.title'), body: t('setup.notice.managed.body') }
    case 'prefix':
      return { kind: 'warn', title: t('setup.notice.prefix.title', { prefix: '/opt/homebrew' }), body: t('setup.notice.prefix.body') }
    case 'low-disk':
      return { kind: 'warn', title: t('setup.notice.disk.title', { need: gb(missingBytes(p)) }), body: t('setup.notice.disk.body', { free: p.freeBytes >= 0 ? gb(p.freeBytes) : '?' }) }
    case 'proxy-pac':
      return { kind: 'info', title: t('setup.notice.pac.title'), body: t('setup.notice.pac.body') }
    default:
      return { kind: 'info', title: t('setup.notice.socks.title'), body: <>{t('setup.notice.socks.body')}{proxyLine}</> }
  }
}
