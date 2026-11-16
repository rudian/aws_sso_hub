// Namespace "notify": system notifications and the sign-in expiry notices (plan 4.15; design 6 "System notifications"
// and 5.14 "Settings additions"). The notification texts themselves are built in Go (internal/notify); the
// notify.signin.* keys are the same words for the demo backend, which draws them as toasts. Keep both in step.
export const zh = {
  'notify.signin.button': '重新登录',
  'notify.signin.headsup.title': '登录快过期了：{portal}',
  'notify.signin.ended.title': '登录已过期：{portal}',
  'notify.signin.names.two': '{a}、{b}',
  'notify.signin.names.many': '{first} 等 {n} 个账户',
  'notify.signin.headsup.when': '大约 {n} 分钟后过期。',
  'notify.signin.headsup.when.hour': '大约 1 小时后过期。',
  'notify.signin.ended.rest': '重新登录后才能继续用。',
  'notify.alreadySignedIn': '已经登录了',

  'settings.notify.title': '登录提醒',
  'settings.notify.expiry.hint': '用系统通知提醒，通知上可以直接重新登录。',
  'settings.notify.ended': '登录过期时通知我',
  'settings.notify.ended.hint': '过期的那一刻通知一次，写明是哪个登录。',
  'settings.notify.admin': '登录能保持多久由管理员决定，Codepass 不能延长，只能重新登录。',
  'settings.notify.row.title': '系统通知',
  'settings.notify.off': 'macOS 没有允许 Codepass 发通知',
  'settings.notify.off.hint': '登录过期和 AI 任务等待的提醒都不会出现。',
  'settings.notify.off.button': '打开系统设置',
  'settings.notify.notAsked': 'Codepass 还没有申请通知权限',
  'settings.notify.notAsked.button': '允许通知',
  'settings.notify.win.help': '没看到通知？请检查 Windows 设置 → 系统 → 通知 里的 Codepass。',
  'settings.notify.win.link': '打开 Windows 设置',
} as const

export const en = {
  'notify.signin.button': 'Sign in again',
  'notify.signin.headsup.title': 'Sign-in expires soon: {portal}',
  'notify.signin.ended.title': 'Sign-in expired: {portal}',
  'notify.signin.names.two': '{a} and {b}',
  'notify.signin.names.many': '{first} and {more} more',
  'notify.signin.headsup.when': 'Expires in about {n} minutes.',
  'notify.signin.headsup.when.hour': 'Expires in about 1 hour.',
  'notify.signin.ended.rest': 'Sign in again to keep using them.',
  'notify.alreadySignedIn': 'Already signed in',

  'settings.notify.title': 'Sign-in reminders',
  'settings.notify.expiry.hint': 'Sends a system notification. You can sign in again from it.',
  'settings.notify.ended': 'Notify me when a sign-in expires',
  'settings.notify.ended.hint': 'One notification at the moment it expires, naming the sign-in.',
  'settings.notify.admin': 'Your administrator decides how long a sign-in lasts. Codepass cannot extend it; it can only sign in again.',
  'settings.notify.row.title': 'System notifications',
  'settings.notify.off': 'macOS does not allow notifications from Codepass',
  'settings.notify.off.hint': 'Reminders about expired sign-ins and waiting AI tasks will not appear.',
  'settings.notify.off.button': 'Open System Settings',
  'settings.notify.notAsked': 'Codepass has not asked for permission to notify you yet',
  'settings.notify.notAsked.button': 'Allow notifications',
  'settings.notify.win.help': 'No notifications? Check Codepass under Windows Settings → System → Notifications.',
  'settings.notify.win.link': 'Open Windows Settings',
} as const
