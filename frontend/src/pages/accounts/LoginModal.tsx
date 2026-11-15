import { Check, Copy, ExternalLink, Globe } from 'lucide-react'
import { useEffect, useMemo, useRef, useState } from 'react'
import { copyText, openExternal } from '../../api/platform'
import { isCancel, toUiError } from '../../api/errors'
import type { UiError } from '../../api/errors'
import { onEvent } from '../../api/events'
import { Accounts } from '../../api/services'
import { ErrorCode, LoginMode, LoginPhase } from '../../api/models'
import type { LoginProgress, LoginResult } from '../../api/models'
import appicon from '../../assets/appicon.svg'
import { AcctChip } from '../../components/AcctChip'
import { Banner } from '../../components/Banner'
import { Button } from '../../components/Button'
import { ErrorBanner } from '../../components/ErrorBanner'
import { Input } from '../../components/Input'
import { Modal } from '../../components/Modal'
import { Spinner } from '../../components/Spinner'
import { useI18n } from '../../i18n'
import { now as clockNow } from '../../api/mock/clock'
import { formatUntilDay } from '../../lib/time'
import { useNow } from '../../lib/useNow'
import { useActions } from '../../shell/AppActions'
import { useAccounts } from '../../state/accounts'
import { useSettings } from '../../state/settings'
import { loginErrorText, loginSuccessText, splitFirstSentence } from './model'
import { cancelQuietly, clock } from './util'
import './accounts.css'

export interface LoginModalProps {
  portal: string
  initialMode?: LoginMode
  onClose: (result: LoginResult | null) => void
}

type Phase = 'starting' | 'waiting' | 'device' | 'exchanging' | 'success' | 'error'

const PHASES: Record<string, Phase> = {
  [LoginPhase.LoginPhaseStarting]: 'starting',
  [LoginPhase.LoginPhaseWaiting]: 'waiting',
  [LoginPhase.LoginPhaseDevice]: 'device',
  [LoginPhase.LoginPhaseExchanging]: 'exchanging',
}

/** Sign-in modal: browser (PKCE) first, device code as the fallback, with every state and transition. */
export function LoginModal({ portal, initialMode, onClose }: LoginModalProps) {
  const { t, lang } = useI18n()
  const { toast, confirm } = useActions()
  const { state } = useAccounts()
  const { settings } = useSettings()
  // device flow: whether the last "Open browser" press ended up in a private window (null until pressed)
  const [devicePrivate, setDevicePrivate] = useState<boolean | null>(null)
  // browser flow: whether the last "Open again" press ended up in a private window (null until pressed)
  const [reopenPrivate, setReopenPrivate] = useState<boolean | null>(null)

  const [mode, setMode] = useState<LoginMode>(initialMode ?? LoginMode.LoginModeBrowser)
  const [attempt, setAttempt] = useState(0)
  const [phase, setPhase] = useState<Phase>('starting')
  const [progress, setProgress] = useState<LoginProgress | null>(null)
  const [result, setResult] = useState<LoginResult | null>(null)
  const [error, setError] = useState<UiError | null>(null)
  const confirming = useRef(false)
  const cancelRef = useRef<HTMLButtonElement>(null)
  const doneRef = useRef<HTMLButtonElement>(null)
  const closing = useRef(false)

  const info = state?.portals?.find((p) => p.name === portal)
  const profiles = useMemo(() => (info?.profiles ?? []).map((n) => state?.profiles?.find((p) => p.name === n)).filter((p) => !!p), [info, state])

  // One login flow per (mode, attempt). The flow starts on the next tick so a React dev double-mount never starts two.
  useEffect(() => {
    let alive = true
    let promise: ReturnType<typeof Accounts.Login> | null = null
    let timer: ReturnType<typeof setTimeout> | undefined
    setPhase('starting')
    setProgress(null)
    setDevicePrivate(null)
    setReopenPrivate(null)
    setError(null)
    const off = onEvent('login:progress', (p) => {
      if (!alive || p.portal !== portal) return
      setProgress(p)
      const next = PHASES[p.phase]
      if (next) setPhase((cur) => (cur === 'success' || cur === 'error' ? cur : next))
    })
    const start = (tries: number) => {
      const pr = Accounts.Login(portal, mode)
      promise = pr
      pr.then(
        (res) => {
          if (!alive) return
          setResult(res)
          setPhase('success')
        },
        (e) => {
          if (!alive || isCancel(e)) return
          const ue = toUiError(e)
          if (ue.code === ErrorCode.ErrLoginInProgress && tries < 4) {
            // the previous flow is still shutting down
            timer = setTimeout(() => start(tries + 1), 350)
            return
          }
          setError(ue)
          setPhase('error')
        },
      )
    }
    timer = setTimeout(() => start(0), 0)
    return () => {
      alive = false
      off()
      if (timer) clearTimeout(timer)
      cancelQuietly(promise)
    }
  }, [mode, attempt, portal])

  const running = phase === 'starting' || phase === 'waiting' || phase === 'device' || phase === 'exchanging'
  const sessionEnd = info?.session.sessionEndsAt ?? info?.session.expiresAt ?? result?.expiresAt
  const successNames = ((result?.profiles?.length ? result.profiles : info?.profiles) ?? []) as string[]
  const untilText = sessionEnd ? formatUntilDay(sessionEnd, clockNow(), lang) : ''

  const finish = (r: LoginResult | null) => {
    if (closing.current) return
    closing.current = true
    if (r) {
      const names = (r.profiles?.length ? r.profiles : (info?.profiles ?? [])) as string[]
      toast({
        kind: 'ok',
        title: t('toast.signedIn.title', { portal }),
        // a portal that is still a draft has no profiles yet, so there is nothing to count
        body: names.length > 0 ? t('toast.signedIn.body', { n: names.length, time: sessionEnd ? formatUntilDay(sessionEnd, clockNow(), lang) : '' }) : undefined,
      })
    }
    onClose(r)
  }

  // Cancel / Esc while a flow runs asks first (except right at the start, where nothing is pending yet).
  const requestCancel = async () => {
    if (confirming.current || closing.current) return
    if (phase === 'waiting' || phase === 'device') {
      confirming.current = true
      const yes = await confirm({
        title: t('login.cancel.confirm'),
        body: t('login.cancel.confirm.body'),
        confirmLabel: t('login.cancel.confirm.yes'),
        cancelLabel: t('login.cancel.confirm.no'),
        danger: true,
      })
      confirming.current = false
      if (!yes) return
    }
    if (closing.current) return
    closing.current = true
    toast({ kind: 'info', title: t('login.cancelled') })
    onClose(null)
  }
  const cancelRequest = useRef(requestCancel)
  cancelRequest.current = requestCancel

  useEffect(() => {
    if (!running) return
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && !confirming.current) {
        e.preventDefault()
        void cancelRequest.current()
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [running])

  // success: close by itself after 1.2 s unless the focus moved somewhere else inside the modal
  useEffect(() => {
    if (phase !== 'success' || !result) return
    const id = window.setTimeout(() => {
      const a = document.activeElement
      const root = doneRef.current?.closest('.modal')
      if (root && a && root.contains(a) && a !== doneRef.current && a !== root) return
      finish(result)
    }, 1200)
    return () => window.clearTimeout(id)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [phase, result])

  const openDevicePage = async (url: string) => {
    try {
      setDevicePrivate(await Accounts.OpenSignInPage(url))
    } catch {
      setDevicePrivate(false)
      try {
        await openExternal(url)
      } catch {
        /* the address is on screen to copy */
      }
    }
  }

  // "Open again" goes through the same backend open as the first open, so it is private again when the
  // setting is on.
  const reopenPage = async (url: string) => {
    try {
      setReopenPrivate(await Accounts.OpenSignInPage(url))
    } catch {
      setReopenPrivate(false)
      try {
        await openExternal(url)
      } catch {
        /* the address can be copied */
      }
    }
  }

  const switchMode = (m: LoginMode) => {
    setMode(m)
    setAttempt((a) => a + 1)
  }

  const isPrivate = reopenPrivate ?? !!progress?.privateWindow
  const privateMissed = !!settings?.privateSignIn && !!progress?.browserOpened && !isPrivate

  const subtitle = info ? <span className="mono">{info.startUrlHost}</span> : undefined

  return (
    <Modal
      open
      title={t('login.title', { portal })}
      subtitle={subtitle}
      dismissible={!running}
      onRequestClose={running ? () => void requestCancel() : undefined}
      onClose={() => finish(phase === 'success' ? result : null)}
      initialFocus={phase === 'success' ? doneRef : cancelRef}
      className="login-modal"
      footer={
        <>
          {phase === 'waiting' && (
            <button type="button" className="link-button login-modal__switch" onClick={() => switchMode(LoginMode.LoginModeDevice)}>
              {t('login.useDevice')}
            </button>
          )}
          {phase === 'device' && (
            <button type="button" className="link-button login-modal__switch" onClick={() => switchMode(LoginMode.LoginModeBrowser)}>
              {t('login.useBrowser')}
            </button>
          )}
          {(phase === 'starting' || phase === 'waiting' || phase === 'device' || phase === 'exchanging') && (
            <Button ref={cancelRef} variant="secondary" onClick={() => void requestCancel()}>
              {t('common.cancel')}
            </Button>
          )}
          {phase === 'device' && progress?.verificationUriComplete && (
            <Button variant="primary" icon={ExternalLink} onClick={() => void openDevicePage(progress.verificationUriComplete as string)}>
              {t('login.device.open')}
            </Button>
          )}
          {phase === 'error' && (
            <>
              <Button ref={cancelRef} variant="ghost" className="login-modal__switch" onClick={() => finish(null)}>
                {t('common.close')}
              </Button>
              <Button variant={error?.code === ErrorCode.ErrLoginPort ? 'primary' : 'secondary'} onClick={() => switchMode(LoginMode.LoginModeDevice)}>
                {t('login.error.useDevice')}
              </Button>
              <Button variant={error?.code === ErrorCode.ErrLoginPort ? 'secondary' : 'primary'} onClick={() => setAttempt((a) => a + 1)}>
                {t('common.retry')}
              </Button>
            </>
          )}
          {phase === 'success' && (
            <Button ref={doneRef} variant="primary" onClick={() => finish(result)}>
              {t('common.done')}
            </Button>
          )}
        </>
      }
    >
      {(phase === 'starting' || phase === 'exchanging') && (
        <div className="login-modal__body login-modal__body--tall">
          <Handoff />
          <p className="login-lead">{phase === 'starting' ? t('login.starting') : t('login.exchanging')}</p>
        </div>
      )}

      {phase === 'waiting' && (
        <div className="login-modal__body login-modal__body--tall">
          <Handoff />
          <p className="login-lead">{t('login.waiting.title')}</p>
          {splitFirstSentence(t(isPrivate ? 'login.waiting.body.private' : 'login.waiting.body')).map((line, i) => line && (
            <p key={i} className="login-text">
              {line}
            </p>
          ))}
          {privateMissed && (
            <p className="login-text" role="status">
              {t('login.private.unavailable')}
            </p>
          )}
          {progress && !progress.browserOpened && (
            <p className="login-text" role="status">
              {t('login.browser.fallback')}
            </p>
          )}
          <div className="login-affects">
            <span>{t('login.unlocks')}</span>
            {profiles.length === 0 && <span className="muted">{t('login.profiles.none')}</span>}
            {profiles.map((p) => p && <AcctChip key={p.name} slot={p.colorSlot} monogram={p.monogram} name={p.name} accountId={p.accountId} />)}
          </div>
          <div className="login-fallback" role="status">
            <Spinner />
            <ExpiryText expiresAt={progress?.expiresAt} />
            <span className="spacer" />
            <Button
              variant="ghost"
              size="sm"
              icon={Copy}
              onClick={async () => {
                if (!progress?.authorizeUrl) return
                try {
                  await copyText(progress.authorizeUrl)
                  toast({ kind: 'ok', title: t('login.copyLink.done') })
                } catch {
                  /* clipboard unavailable */
                }
              }}
            >
              {t('login.copyLink')}
            </Button>
            <Button variant="ghost" size="sm" icon={ExternalLink} onClick={() => progress?.authorizeUrl && void reopenPage(progress.authorizeUrl)}>
              {t('login.openAgain')}
            </Button>
          </div>
        </div>
      )}

      {phase === 'device' && (
        <div className="login-modal__body login-modal__body--tall">
          <p className="login-lead login-lead--device">{t('login.device.title')}</p>
          <p className="login-text">{t('login.device.body')}</p>
          {settings?.privateSignIn && devicePrivate !== false && <p className="login-text">{t('login.device.private')}</p>}
          {settings?.privateSignIn && devicePrivate === false && (
            <p className="login-text" role="status">
              {t('login.private.unavailable')}
            </p>
          )}
          <UserCode code={progress?.userCode ?? ''} label={t('login.device.code.aria', { code: progress?.userCode ?? '' })} />
          <div className="input-group" style={{ marginTop: 'var(--sp-4)' }}>
            <Input mono readOnly aria-label={t('login.device.url')} value={progress?.verificationUriComplete ?? progress?.verificationUri ?? ''} onFocus={(e) => e.currentTarget.select()} />
            <Button
              variant="secondary"
              icon={Copy}
              onClick={async () => {
                const url = progress?.verificationUriComplete ?? progress?.verificationUri
                if (!url) return
                try {
                  await copyText(url)
                  toast({ kind: 'ok', title: t('common.copied') })
                } catch {
                  /* clipboard unavailable */
                }
              }}
            >
              {t('common.copy')}
            </Button>
          </div>
          <div className="login-fallback" role="status">
            <Spinner />
            <ExpiryText expiresAt={progress?.expiresAt} />
          </div>
        </div>
      )}

      {phase === 'success' && (
        <div className="login-modal__body" style={{ textAlign: 'center' }}>
          <div className="success-mark" role="img" aria-label={t('login.doneAria')}>
            <Check aria-hidden="true" />
          </div>
          <p className="login-lead">{t('login.success.title')}</p>
          <p className="login-text">
            {loginSuccessText(successNames, untilText, lang, t)}
          </p>
        </div>
      )}

      {phase === 'error' && error && (
        <div className="login-modal__body">
          {error.code === ErrorCode.ErrNetwork ? (
            <Banner kind="danger" title={loginErrorText(error, t).title}>
              {loginErrorText(error, t).body}
            </Banner>
          ) : (
            <ErrorBanner error={error} />
          )}
        </div>
      )}
    </Modal>
  )
}

function Handoff() {
  return (
    <div className="handoff" aria-hidden="true">
      <span className="handoff__node handoff__node--app">
        <img src={appicon} width={56} height={56} alt="" />
      </span>
      <span className="handoff__line" />
      <span className="handoff__node">
        <Globe />
      </span>
    </div>
  )
}

/** "Waiting. Expires in 9:18"; announces once when a minute is left. */
function ExpiryText({ expiresAt }: { expiresAt?: string }) {
  const { t } = useI18n()
  const now = useNow(1000)
  const left = expiresAt ? Date.parse(expiresAt) - now : NaN
  const soon = !Number.isNaN(left) && left > 0 && left <= 60_000
  const [before, after] = t('login.waiting.status', { time: '\u0001' }).split('\u0001')
  return (
    <>
      <span>
        {before}
        <span className="timer" aria-live="off">
          {Number.isNaN(left) ? '' : clock(left)}
        </span>
        {after}
      </span>
      <span className="sr-only" role="status" aria-live="polite">
        {soon ? t('login.expiresSoon') : ''}
      </span>
    </>
  )
}

/** XXXX-XXXX in large boxes; assistive technology reads the aria-label as one string. */
function UserCode({ code, label }: { code: string; label: string }) {
  return (
    <div className="user-code" role="img" aria-label={label}>
      {[...code].map((c, i) => (c === '-' ? <i key={i}>-</i> : <span key={i}>{c}</span>))}
    </div>
  )
}

