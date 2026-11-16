import * as NotifyReal from '../../../bindings/github.com/rudian/codepass/internal/app/notifyservice'
import { systemLang, translate } from '../../i18n'
import type { Lang } from '../../i18n'
import { initialUI } from '../../lib/initialUI'
import { mockEmit } from '../events'
import { NotifyAction, NotifyPermission, NotifyTone } from '../models'
import type { NotifyState, NotifyToast } from '../models'
import { getWorld } from './world'
import { READ, run } from './util'

// The demo backend of the system notifications (F1-22). The real service sends notifications through the operating
// system; the demo draws the same title, body and button as a toast, like sandbox mode does.
//
// URL switches (mock mode only), for the screenshots of the milestone gate:
//   ?notify=allowed|notasked|off   what the system says about permission (default allowed)
//   ?toast=ended|headsup           show that sign-in notice as a toast right after the page loads
//                                  (portal: ?toastPortal=<name>, default acme)

let permission: NotifyPermission = NotifyPermission.NotifyAllowed
let pending = new Map<string, { portal: string; kind: string }>()
let seq = 0

function readQuery(): URLSearchParams {
  return new URLSearchParams(typeof window !== 'undefined' ? window.location.search : '')
}

/** Back to the state a fresh page load has (tests). */
export function resetMockNotify(search = ''): void {
  const q = new URLSearchParams(search)
  const v = q.get('notify')
  permission = v === 'off' ? NotifyPermission.NotifyOff : v === 'notasked' ? NotifyPermission.NotifyNotAsked : NotifyPermission.NotifyAllowed
  pending = new Map()
  seq = 0
}

function lang(): Lang {
  return initialUI.lang ?? systemLang()
}

/** The words of design 6 for a sign-in notice, built from the locale like internal/notify builds them in Go. */
export function signInNoticeText(kind: 'ended' | 'headsup', portal: string, names: string[], leadMinutes = 15, l: Lang = lang()): { title: string; body: string } {
  const t = (k: Parameters<typeof translate>[1], p?: Record<string, string | number>) => translate(l, k, p)
  const title = t(kind === 'ended' ? 'notify.signin.ended.title' : 'notify.signin.headsup.title', { portal })
  let who = ''
  if (names.length === 1) who = names[0]
  else if (names.length === 2) who = t('notify.signin.names.two', { a: names[0], b: names[1] })
  else if (names.length > 2) who = t('notify.signin.names.many', { first: names[0], n: names.length, more: names.length - 1 })
  who = who ? (l === 'zh-CN' ? `${who}。` : `${who}. `) : ''
  const rest = kind === 'ended' ? t('notify.signin.ended.rest') : leadMinutes >= 60 ? t('notify.signin.headsup.when.hour') : t('notify.signin.headsup.when', { n: leadMinutes })
  return { title, body: who + rest }
}

function accountNames(portal: string): string[] {
  const w = getWorld()
  const seen: string[] = []
  for (const p of w.accounts.profiles ?? []) {
    if (p.portal === portal && p.accountName && !seen.includes(p.accountName)) seen.push(p.accountName)
  }
  return seen
}

/** Draw a sign-in notice as a toast, as sandbox mode does. */
export function emitMockSignInNotice(kind: 'ended' | 'headsup', portal: string): NotifyToast {
  const { title, body } = signInNoticeText(kind, portal, accountNames(portal), getWorld().settings.reminders.leadMinutes)
  const toast: NotifyToast = {
    id: `signin:${portal}#${++seq}`,
    kind: kind === 'ended' ? 'signin.ended' : 'signin.headsup',
    title,
    body,
    button: translate(lang(), 'notify.signin.button'),
    tone: kind === 'ended' ? NotifyTone.NotifyToneDanger : NotifyTone.NotifyToneWarn,
    sticky: kind === 'ended',
  }
  pending.set(toast.id, { portal, kind })
  mockEmit('notify:toast', toast)
  return toast
}

const q0 = readQuery()
resetMockNotify(typeof window !== 'undefined' ? window.location.search : '')
if (typeof window !== 'undefined' && q0.has('toast')) {
  const kind = q0.get('toast') === 'headsup' ? 'headsup' : 'ended'
  window.setTimeout(() => emitMockSignInNotice(kind, q0.get('toastPortal') ?? 'acme'), 600)
}

function state(): NotifyState {
  return { permission, available: true }
}

export const mockNotifyService: typeof NotifyReal = {
  GetState: () => run(READ, state),

  // macOS asks its own question; the demo answers "allow" after a moment, like the first sign-in does
  RequestPermission: () =>
    run(READ, () => {
      if (permission === NotifyPermission.NotifyNotAsked) {
        window.setTimeout(() => {
          permission = NotifyPermission.NotifyAllowed
          mockEmit('notify:state', state())
        }, 800)
      }
      return state()
    }),

  OpenSystemSettings: () => run(READ, () => undefined),

  RespondToast: (id, action) =>
    run(READ, () => {
      const p = pending.get(id)
      if (!p) return
      pending.delete(id)
      if (action === NotifyAction.NotifyActionButton) mockEmit('app:navigate', { page: 'accounts', action: 'login', arg: p.portal }) // the window starts the sign-in
      else mockEmit('app:navigate', { page: 'accounts', action: 'show', arg: p.portal })
    }),

  SetReady: () => run(READ, () => undefined),
}
