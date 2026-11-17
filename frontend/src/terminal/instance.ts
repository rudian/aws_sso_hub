import { Terminal, type IDisposable, type ILink, type ILinkProvider, type ITerminalOptions } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { SearchAddon } from '@xterm/addon-search'
import { Unicode11Addon } from '@xterm/addon-unicode11'
import { WebLinksAddon } from '@xterm/addon-web-links'
import { guessPlatform, type Platform } from '../lib/platform'
import { TerminalController } from './controller'
import { classifyLink, findPathLinks, lineText, type LinkDecision } from './links'
import { needsPasteConfirm, sanitizePaste } from './paste'
import { readTermTheme } from './theme'
import type { TerminalClient } from './types'

/** What the surrounding UI does for the things a terminal cannot decide alone. Set by the view; every one is optional. */
export interface InstanceHandlers {
  /** https:// link, after Cmd/Ctrl+click (the caller uses the https-only opener) */
  openUrl?: (url: string) => void
  /** http://localhost or http://127.0.0.1 link the user confirmed. Not the https opener: it refuses http. */
  openLocalUrl?: (url: string) => void
  copyText?: (text: string) => void
  /** http://localhost link: resolve true to open. Asked once per port; the instance remembers a yes. */
  confirmLocal?: (port: string, url: string) => Promise<boolean>
  /** A path with a line number: open it in the Code tab */
  openPath?: (path: string, line: number, column?: number) => void
  /** Paste of more than 5 lines: resolve true to paste */
  confirmPaste?: (lines: number) => Promise<boolean>
  /** Cmd/Ctrl+F inside the terminal */
  search?: () => void
  /** The child ended and every byte was shown */
  ended?: () => void
  /** A read or write failed: show it with a Retry (which calls `retry()` on the instance) */
  error?: (err: unknown) => void
  /** A read worked again after an error */
  recovered?: () => void
}

export interface InstanceOptions {
  session: string
  client: TerminalClient
  platform?: Platform
  screenReader?: boolean
  fontFamily?: string
  /** Windows build number (from the OS), so xterm knows whether ConPTY reflows (21376 and later). Unknown: omit. */
  windowsBuild?: number
  /** The session was started by this page just now: its startup output is live (not replayed) and fades in once. */
  fresh?: boolean
}

const FONT_SIZE = 12.5
/** Row height in px (ui-design 5.6). xterm multiplies its measured character height, so the option is computed. */
const ROW_PX = 19
const RESIZE_DEBOUNCE_MS = 80
const FRESH_MS = 300

/** The --font-mono token as one line (the token has line breaks), or a plain fallback. */
function monoFont(): string {
  const v = getComputedStyle(document.documentElement).getPropertyValue('--font-mono').replace(/\s+/g, ' ').trim()
  return v || 'ui-monospace, Menlo, "Cascadia Mono", Consolas, monospace'
}

/** One xterm.js terminal plus its pull loop. It lives in a detached host element and is moved into a view when shown. */
export class TerminalInstance {
  readonly session: string
  readonly host: HTMLDivElement
  readonly term: Terminal
  readonly controller: TerminalController
  handlers: InstanceHandlers = {}
  private readonly fit = new FitAddon()
  private readonly searchAddon = new SearchAddon()
  private readonly platform: Platform
  private opened = false
  private mounted = false
  private intersecting = true
  private io: IntersectionObserver | null = null
  private ro: ResizeObserver | null = null
  private subs: IDisposable[] = []
  private allowedPorts = new Set<string>()
  private disposed = false
  private failedFlag = false
  private fitQueued = false
  private resizeTimer: ReturnType<typeof setTimeout> | null = null
  private sizeSent = false
  private freshPending: boolean

  constructor(o: InstanceOptions) {
    this.session = o.session
    this.freshPending = o.fresh ?? false
    this.platform = o.platform ?? guessPlatform()
    this.host = document.createElement('div')
    this.host.className = 'term-host'

    const opts: ITerminalOptions = {
      allowProposedApi: true, // the Unicode 11 addon and search decorations need it
      scrollback: 10000,
      fontSize: FONT_SIZE,
      lineHeight: ROW_PX / (FONT_SIZE * 1.2), // corrected after open, from the measured character height
      fontFamily: o.fontFamily ?? monoFont(),
      cursorBlink: true,
      screenReaderMode: o.screenReader ?? false,
      theme: readTermTheme(),
      // OSC 8 links get the same rules as other links (modifier, https check); never xterm's confirm + window.open.
      linkHandler: { activate: (e, uri) => this.activateUrl(e, uri), allowNonHttpProtocols: false },
    }
    if (this.platform === 'windows') opts.windowsPty = { backend: 'conpty', ...(o.windowsBuild ? { buildNumber: o.windowsBuild } : {}) }
    this.term = new Terminal(opts)
    this.term.loadAddon(this.fit)
    this.term.loadAddon(this.searchAddon)
    this.term.loadAddon(new Unicode11Addon())
    this.term.unicode.activeVersion = '11'
    // A program inside the terminal cannot write to the clipboard: OSC 52 is accepted and ignored.
    this.subs.push(this.term.parser.registerOscHandler(52, () => true))

    this.controller = new TerminalController({
      session: o.session,
      client: o.client,
      platform: this.platform,
      hasSelection: () => this.term.hasSelection(),
      gate: () => this.canPull(),
      sink: {
        write: (b) => {
          this.showFresh(b.length)
          return new Promise<void>((res) => this.term.write(b, res))
        },
        reset: () => this.term.reset(),
        size: () => ({ cols: this.term.cols, rows: this.term.rows }),
      },
      onDone: () => this.handlers.ended?.(),
      fresh: o.fresh,
      onError: (err) => {
        this.failedFlag = true
        this.handlers.error?.(err)
      },
      onRecover: () => {
        this.failedFlag = false
        this.handlers.recovered?.()
      },
    })

    this.subs.push(this.term.onData((s) => this.controller.handleData(s)))
    this.subs.push(this.term.onBinary((s) => this.controller.handleBinary(s)))
    this.subs.push(this.term.onResize(({ cols, rows }) => this.queueResize(cols, rows)))
    this.term.attachCustomKeyEventHandler((e) => this.onKey(e))

    this.term.loadAddon(new WebLinksAddon((e, uri) => this.activateUrl(e, uri)))
    this.subs.push(this.term.registerLinkProvider(this.pathProvider()))
  }

  private canPull(): boolean {
    return !this.disposed && this.mounted && this.intersecting && document.visibilityState === 'visible'
  }

  private onKey(e: KeyboardEvent): boolean {
    const a = this.controller.handleKey(e)
    // Only the app's own keys (and keys xterm or the browser still has to handle) reach the page.
    if (a.kind !== 'app' && a.kind !== 'native' && a.kind !== 'paste') {
      e.preventDefault()
      e.stopPropagation()
    }
    switch (a.kind) {
      case 'native':
        return true
      case 'copy':
        this.handlers.copyText?.(this.term.getSelection())
        return false
      case 'search':
        this.handlers.search?.()
        return false
      default:
        return false // send, paste, app, swallow: xterm skips it (paste: the browser's paste event follows; app: the page sees it)
    }
  }

  private async act(d: LinkDecision): Promise<void> {
    switch (d.kind) {
      case 'open':
        this.handlers.openUrl?.(d.url)
        break
      case 'copy':
        this.handlers.copyText?.(d.url)
        break
      case 'confirm-local': {
        if (!this.allowedPorts.has(d.port)) {
          const ok = (await this.handlers.confirmLocal?.(d.port, d.url)) ?? false
          if (!ok) return
          this.allowedPorts.add(d.port)
        }
        // Its own opener: the https one refuses http. Without one the address is copied.
        if (this.handlers.openLocalUrl) this.handlers.openLocalUrl(d.url)
        else this.handlers.copyText?.(d.url)
        break
      }
    }
  }

  private activateUrl(e: MouseEvent, uri: string): void {
    const mod = this.platform === 'darwin' ? e.metaKey : e.ctrlKey
    if (!mod) return
    void this.act(classifyLink(uri))
  }

  private pathProvider(): ILinkProvider {
    return {
      provideLinks: (y, cb) => {
        const line = this.term.buffer.active.getLine(y - 1)
        if (!line) return cb(undefined)
        // Columns are cells, not string indices: a wide (CJK) character is one character and two cells.
        const { text, cols } = lineText(line)
        const links: ILink[] = findPathLinks(text).map((m) => ({
          text: text.slice(m.index, m.index + m.length),
          range: { start: { x: cols[m.index] + 1, y }, end: { x: cols[m.index + m.length - 1] + 1, y } },
          activate: (e) => {
            const mod = this.platform === 'darwin' ? e.metaKey : e.ctrlKey
            if (mod) this.handlers.openPath?.(m.path, m.line, m.column)
          },
        }))
        cb(links.length ? links : undefined)
      },
    }
  }

  /** Put the terminal into a container (it moves; the screen and scrollback are kept) and start pulling. */
  mount(container: HTMLElement): void {
    if (this.disposed) return
    container.appendChild(this.host)
    if (!this.opened) {
      this.opened = true
      this.term.open(this.host)
      this.applyLineHeight()
      this.term.element?.addEventListener('paste', this.onPaste, true)
      void this.controller.attach()
    }
    this.mounted = true
    this.intersecting = true
    this.io?.disconnect()
    this.ro?.disconnect()
    if (typeof IntersectionObserver !== 'undefined') {
      this.io = new IntersectionObserver((es) => {
        const was = this.canPull()
        this.intersecting = es[es.length - 1]?.isIntersecting ?? true
        if (!was && this.canPull()) this.controller.sync.kick()
      })
      this.io.observe(this.host)
    }
    if (typeof ResizeObserver !== 'undefined') {
      this.ro = new ResizeObserver(() => this.scheduleFit())
      this.ro.observe(container)
    }
    this.refit()
    this.controller.sync.kick()
  }

  /** Take it out of the page but keep it alive; it stops pulling until it is mounted again. */
  unmount(): void {
    this.mounted = false
    this.io?.disconnect()
    this.ro?.disconnect()
    this.io = this.ro = null
    this.host.remove()
  }

  refit(): void {
    if (!this.mounted || this.host.clientWidth === 0 || this.host.clientHeight === 0) return
    this.applyLineHeight()
    try {
      this.fit.fit()
    } catch {
      /* the renderer is not ready yet */
    }
    this.queueResize(this.term.cols, this.term.rows)
  }

  /** Fit at most once per animation frame while a window or panel is dragged. */
  private scheduleFit(): void {
    if (this.fitQueued) return
    this.fitQueued = true
    const run = () => {
      this.fitQueued = false
      if (!this.disposed) this.refit()
    }
    if (typeof requestAnimationFrame === 'function') requestAnimationFrame(run)
    else run()
  }

  /** The PTY hears the first size at once and later sizes after the drag settles (each one is a SIGWINCH and a redraw). */
  private queueResize(cols: number, rows: number): void {
    if (this.resizeTimer) clearTimeout(this.resizeTimer)
    this.resizeTimer = null
    if (!this.sizeSent) {
      this.sizeSent = true
      this.controller.handleResize(cols, rows)
      return
    }
    this.resizeTimer = setTimeout(() => {
      this.resizeTimer = null
      if (!this.disposed) this.controller.handleResize(this.term.cols, this.term.rows)
    }, RESIZE_DEBOUNCE_MS)
  }

  /** Make a row ROW_PX tall: the cell height is the measured character height times lineHeight (floored). */
  private applyLineHeight(): void {
    const el = this.term.element?.querySelector<HTMLElement>('.xterm-char-measure-element')
    const h = el ? Math.ceil(el.getBoundingClientRect().height) : 0
    if (!h) return
    const lh = (ROW_PX + 0.01) / h
    if (Math.abs((this.term.options.lineHeight ?? 0) - lh) > 0.0005) this.term.options.lineHeight = lh
  }

  /** The first bytes of a session this page started fade in once; a re-shown terminal never fades. */
  private showFresh(n: number): void {
    if (!this.freshPending || n === 0) return
    this.freshPending = false
    this.host.classList.add('term-host--fresh')
    setTimeout(() => this.host.classList.remove('term-host--fresh'), FRESH_MS)
  }

  get failed(): boolean {
    return this.failedFlag
  }

  /** Try again after an error: the pull restarts from where it stopped. */
  retry(): void {
    this.failedFlag = false
    this.handlers.recovered?.()
    this.controller.sync.kick()
  }

  /** The page became visible again (visibilitychange). */
  pageVisibilityChanged(): void {
    this.controller.sync.kick()
  }

  focus(): void {
    this.term.focus()
  }

  refreshTheme(): void {
    this.term.options.theme = readTermTheme()
    const f = monoFont()
    if (this.term.options.fontFamily !== f) {
      this.term.options.fontFamily = f
      this.applyLineHeight()
    }
  }

  setScreenReader(on: boolean): void {
    this.term.options.screenReaderMode = on
  }

  findNext(q: string, caseSensitive = false): boolean {
    return q ? this.searchAddon.findNext(q, { caseSensitive }) : false
  }
  findPrevious(q: string, caseSensitive = false): boolean {
    return q ? this.searchAddon.findPrevious(q, { caseSensitive }) : false
  }
  clearSearch(): void {
    this.searchAddon.clearDecorations()
  }

  /** Paste text as the user would (bracketed paste when the program asked for it). */
  paste(text: string): void {
    const clean = sanitizePaste(text)
    if (clean) this.term.paste(clean)
  }

  private onPaste = (e: ClipboardEvent): void => {
    const text = sanitizePaste(e.clipboardData?.getData('text/plain') ?? '')
    if (!text) return
    e.preventDefault()
    e.stopPropagation()
    if (!needsPasteConfirm(text) || !this.handlers.confirmPaste) {
      this.paste(text)
      return
    }
    const lines = text.replace(/\r\n?/g, '\n').replace(/\n$/, '').split('\n').length
    void this.handlers.confirmPaste(lines).then((ok) => {
      if (ok) this.paste(text)
    })
  }

  dispose(): void {
    if (this.disposed) return
    this.disposed = true
    if (this.resizeTimer) clearTimeout(this.resizeTimer)
    this.controller.dispose()
    this.unmount()
    for (const s of this.subs) s.dispose()
    this.term.dispose()
  }
}
