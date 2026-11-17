import { useCallback, useEffect, useRef, useState } from 'react'
import { copyText, openExternal } from '../api/platform'
import { useI18n } from '../i18n'
import { useActions } from '../shell/AppActions'
import type { InstanceHandlers } from './instance'
import { terminalRegistry } from './registry'
import '@xterm/xterm/css/xterm.css'
import './terminal.css'

export interface TerminalViewProps {
  session: string
  /** Pull and take keys only while true (the page or tab is the one shown). The terminal stays alive either way. */
  active?: boolean
  screenReader?: boolean
  /** Take keyboard focus when shown (default false: a list browsed with the arrow keys must keep its keys). */
  autoFocus?: boolean
  /** The session was started by this page just now: startup output is live (not replayed) and fades in once. Read when the terminal is first created. */
  fresh?: boolean
  /** A path with a line number was clicked; open it in the Code tab. */
  onOpenPath?: (path: string, line: number, column?: number) => void
  onEnded?: () => void
  className?: string
}

/**
 * Shows one session's terminal. Mounting moves the registry's instance into this element; unmounting only takes it out,
 * so the screen and scrollback are intact when the user comes back. Terminal text is never turned into HTML: xterm draws
 * it, and every string this component renders comes from the locale files.
 */
export function TerminalView({ session, active = true, screenReader, autoFocus = false, fresh, onOpenPath, onEnded, className }: TerminalViewProps) {
  const { t } = useI18n()
  const actions = useActions()
  const boxRef = useRef<HTMLDivElement>(null)
  const [failed, setFailed] = useState(false)
  const [searching, setSearching] = useState(false)
  const [query, setQuery] = useState('')
  const [miss, setMiss] = useState(false)
  const searchRef = useRef<HTMLInputElement>(null)
  const cbs = useRef({ onOpenPath, onEnded, actions, t })
  cbs.current = { onOpenPath, onEnded, actions, t }

  useEffect(() => {
    const box = boxRef.current
    if (!box || !active) return
    const inst = terminalRegistry().acquire(session, { screenReader, fresh })
    const fail = (e: unknown) =>
      cbs.current.actions.toast({ kind: 'danger', title: cbs.current.t('term.action.failed'), body: e instanceof Error ? e.message : String(e) })
    const copy = (s: string) => copyText(s).catch(fail)
    const ask = (title: string, body: string, confirmLabel: string, cancelLabel: string) =>
      cbs.current.actions.confirm({ title, body, confirmLabel, cancelLabel }).finally(() => inst.focus())
    const handlers: InstanceHandlers = {
      openUrl: (u) => void openExternal(u).catch(fail),
      // No bound method opens http://localhost yet (the https opener refuses it): copy the address and say so.
      openLocalUrl: (u) => {
        void copy(u)
        cbs.current.actions.toast({ kind: 'info', title: cbs.current.t('term.local.copied'), body: cbs.current.t('term.local.copied.body', { url: u }) })
      },
      copyText: (s) => void copy(s),
      confirmLocal: (_port, url) => {
        const { t } = cbs.current
        return ask(t('term.local.title'), t('term.local.body', { url }), t('term.local.confirm'), t('term.local.cancel'))
      },
      confirmPaste: (lines) => {
        const { t } = cbs.current
        return ask(t('term.paste.title', { lines }), t('term.paste.body'), t('term.paste.confirm'), t('term.paste.cancel'))
      },
      openPath: (p, l, c) => cbs.current.onOpenPath?.(p, l, c),
      search: () => setSearching(true),
      ended: () => cbs.current.onEnded?.(),
      error: () => setFailed(true),
      recovered: () => setFailed(false),
    }
    inst.handlers = handlers
    setFailed(inst.failed)
    inst.mount(box)
    if (autoFocus) inst.focus()
    const mo = new MutationObserver(() => inst.refreshTheme())
    mo.observe(document.documentElement, { attributes: true, attributeFilter: ['data-theme'] })
    // Theme = System has no data-theme attribute: follow the OS appearance too.
    const mq = typeof window.matchMedia === 'function' ? window.matchMedia('(prefers-color-scheme: dark)') : null
    const onScheme = () => inst.refreshTheme()
    mq?.addEventListener?.('change', onScheme)
    return () => {
      mo.disconnect()
      mq?.removeEventListener?.('change', onScheme)
      inst.handlers = {}
      inst.unmount()
    }
  }, [session, active]) // screenReader is applied below, without remounting; fresh and autoFocus matter when it is first shown

  useEffect(() => {
    if (screenReader !== undefined && terminalRegistry().has(session)) terminalRegistry().acquire(session).setScreenReader(screenReader)
  }, [session, screenReader])

  useEffect(() => {
    if (searching) searchRef.current?.focus()
  }, [searching])

  const find = useCallback(
    (dir: 'next' | 'prev', q = query) => {
      if (!terminalRegistry().has(session)) return
      const inst = terminalRegistry().acquire(session)
      const ok = dir === 'next' ? inst.findNext(q) : inst.findPrevious(q)
      setMiss(!!q && !ok)
    },
    [session, query],
  )

  const closeSearch = () => {
    setSearching(false)
    setQuery('')
    setMiss(false)
    const inst = terminalRegistry().acquire(session)
    inst.clearSearch()
    inst.focus()
  }

  return (
    <div className={`term ${className ?? ''}`} role="region" aria-label={t('term.view.aria')}>
      <div className="term__box" ref={boxRef} />
      {searching && (
        <div className="term__search" role="search">
          <input
            ref={searchRef}
            className="input term__search-input"
            value={query}
            placeholder={t('term.search.placeholder')}
            aria-label={t('term.search.aria')}
            onChange={(e) => {
              setQuery(e.target.value)
              find('next', e.target.value)
            }}
            onKeyDown={(e) => {
              if (e.key === 'Escape') closeSearch()
              else if (e.key === 'Enter') find(e.shiftKey ? 'prev' : 'next')
            }}
          />
          {miss && <span className="term__miss">{t('term.search.none')}</span>}
          <button type="button" className="btn btn--ghost" onClick={() => find('prev')}>{t('term.search.prev')}</button>
          <button type="button" className="btn btn--ghost" onClick={() => find('next')}>{t('term.search.next')}</button>
          <button type="button" className="btn btn--ghost" onClick={closeSearch}>{t('term.search.close')}</button>
        </div>
      )}
      {failed && (
        <div className="term__error" role="alert">
          <span>{t('term.error.read')}</span>
          <button type="button" className="btn btn--secondary btn--sm" onClick={() => terminalRegistry().acquire(session).retry()}>{t('term.retry')}</button>
        </div>
      )}
    </div>
  )
}
