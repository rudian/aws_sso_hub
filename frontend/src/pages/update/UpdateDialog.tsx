// The update dialog (modal, 560 px; design 5.13). It opens only when the person asks (banner, page, Settings card),
// never at launch. Closing it keeps a download going; the banner shows "Downloading 64%".
import { useEffect, useMemo, useRef } from 'react'
import type { RefObject } from 'react'
import { UpdatePhase } from '../../api/models'
import type { UpdateState } from '../../api/models'
import { Button } from '../../components/Button'
import { Modal } from '../../components/Modal'
import { useI18n } from '../../i18n'
import { announceKey, UpdatePanel } from './UpdatePanel'
import { updateActions, closeUpdateDialog, useUpdateDialogOpen, useUpdateState } from './useUpdate'
import { useReopenWatch } from './reopenWatch'
import { useRunningTasks } from './runningTasks'

let hosts = 0 // mounted <UpdateDialog>s

/** Retry after a failure: forget a refusal, find the release again and download it again. */
export async function retryUpdate(s: UpdateState): Promise<void> {
  await updateActions.retry()
  // Only a release that is on offer again is downloaded; a check that failed shows its own message.
  if (s.version) await updateActions.download()
}

/**
 * The footer of the dialog and of the page. `onClose` is the dialog's "Later"/"Close"; on the page there is nothing
 * to close, so the button is left out. `focusRef` receives the safe action for the opening focus.
 */
export function UpdateFooter({ s, onClose, focusRef }: { s: UpdateState; onClose?: () => void; focusRef?: RefObject<HTMLButtonElement | null> }) {
  const { t } = useI18n()
  const tasks = useRunningTasks()
  const watch = useReopenWatch()
  const own = useRef<HTMLButtonElement>(null)
  const primary = focusRef ?? own
  const phase = s.phase

  const firstPhase = useRef(true)
  useEffect(() => {
    // After the flow changes, focus goes to the action that makes sense next. Not on the first render: a dialog that
    // opens in one of these phases is focused by the Modal after it has noted the opener (Esc returns focus there).
    if (firstPhase.current) {
      firstPhase.current = false
      return
    }
    if (phase === UpdatePhase.UpdateReady || phase === UpdatePhase.UpdateError || phase === UpdatePhase.UpdateRefused) primary.current?.focus()
  }, [phase])

  const browser = (
    <button type="button" className="link" onClick={() => void updateActions.openBrowser()}>
      {t('update.action.browser')}
    </button>
  )
  const browserShort = (
    <button type="button" className="link" onClick={() => void updateActions.openBrowser()}>
      {t('update.action.browserShort')}
    </button>
  )
  const waiting = watch.mode !== 'off'

  switch (phase) {
    case UpdatePhase.UpdateAvailable:
      return (
        <div className="update-foot">
          <span className="update-foot__btns">
            {onClose && (
              <Button key="later" onClick={onClose}>
                {t('update.action.later')}
              </Button>
            )}
            <Button key="update" ref={primary} variant="primary" onClick={() => void updateActions.download()}>
              {t('update.action.update')}
            </Button>
          </span>
          {browser}
        </div>
      )
    case UpdatePhase.UpdateDownloading:
    case UpdatePhase.UpdateVerifying:
    case UpdatePhase.UpdateReady:
    case UpdatePhase.UpdateRestarting: {
      const ready = phase === UpdatePhase.UpdateReady
      const flowing = ready || phase === UpdatePhase.UpdateDownloading || phase === UpdatePhase.UpdateVerifying
      return (
        <div className="update-foot">
          <span className="update-foot__btns">
            {tasks > 0 ? (
              <>
                <Button key="now" disabled={!ready} onClick={() => void updateActions.restart()}>
                  {t('update.action.reopenNow')}
                </Button>
                <Button key="wait" ref={primary} variant="primary" disabled={!flowing} aria-disabled={waiting || undefined} onClick={waiting ? undefined : updateActions.reopenWhenIdle}>
                  {t('update.action.reopenWhenIdle')}
                </Button>
              </>
            ) : (
              <Button key="reopen" ref={ready ? primary : undefined} variant="primary" disabled={!ready} onClick={() => void updateActions.restart()}>
                {t('update.action.reopen')}
              </Button>
            )}
          </span>
          {browser}
        </div>
      )
    }
    case UpdatePhase.UpdateError:
    case UpdatePhase.UpdateRefused:
      return (
        <div className="update-foot">
          <span className="update-foot__btns">
            {onClose && (
              <Button key="close" onClick={onClose}>
                {t('common.close')}
              </Button>
            )}
            <Button key="retry" ref={primary} variant="primary" onClick={() => void retryUpdate(s)}>
              {t('common.retry')}
            </Button>
          </span>
          {phase === UpdatePhase.UpdateRefused ? browserShort : browser}
        </div>
      )
    default:
      return (
        <div className="update-foot">
          <span className="update-foot__btns">{onClose && <Button onClick={onClose}>{t('common.close')}</Button>}</span>
          {phase === UpdatePhase.UpdateBlocked && browser}
        </div>
      )
  }
}

export function UpdateDialog() {
  const { t } = useI18n()
  const open = useUpdateDialogOpen()
  const s = useUpdateState()
  const key = announceKey(s.phase)
  // The dialog belongs to the page that hosts it: leaving the page closes it, so it never reopens by itself later.
  useEffect(() => {
    hosts++
    return () => {
      hosts--
      queueMicrotask(() => hosts === 0 && closeUpdateDialog()) // a remount in the same tick (StrictMode) keeps it open
    }
  }, [])
  const busy = s.phase === UpdatePhase.UpdateRestarting
  // Opening focus: the safe action of the footer ("Reopen when no task is running" when tasks run), else the panel.
  const safe = useRef<HTMLButtonElement>(null)
  const panel = useRef<HTMLDivElement>(null)
  const initialFocus = useMemo(() => ({ get current() { return safe.current ?? panel.current } }) as RefObject<HTMLElement | null>, [])
  // When the button that was pressed goes away (Update -> downloading), focus must not fall to the page behind.
  useEffect(() => {
    const box = panel.current?.closest('[role=dialog]')
    if (open && box && !box.contains(document.activeElement)) panel.current?.focus()
  }, [open, s.phase])
  return (
    <>
      <div className="sr-only" role="status" aria-live="polite">
        {key ? t(key, { version: s.version ?? '' }) : ''}
      </div>
      <Modal
        open={open}
        onClose={closeUpdateDialog}
        dismissible={!busy}
        className="update-modal"
        title={t('update.dialog.title')}
        initialFocus={initialFocus}
        footer={<UpdateFooter s={s} onClose={closeUpdateDialog} focusRef={safe} />}
      >
        <div ref={panel} tabIndex={-1} className="update-focus">
          <UpdatePanel s={s} />
        </div>
      </Modal>
    </>
  )
}
