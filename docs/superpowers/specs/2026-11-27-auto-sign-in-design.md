# Open the sign-in page by itself before a session ends

Date: 2026-11-27. Status: draft for approval.

## Why

The user works with several IAM Identity Center portals. Each portal's session ends at a length the
administrator sets (8 hours by default). Inside a session the app renews the access token on its own, but at
the session end only a new interactive sign-in in the browser helps, and today the user finds out when `git`
or `aws` stops working, or by watching the countdown.

The user wants the app, a few minutes (10 by default) before a portal's session ends, to open the sign-in page
by itself, without a notification first. The user signs in, and a new session of full length starts, so the
sign-in lasts longer.

Because there are several portals, a browser window that opens by itself must say which portal it is for, and
which profiles that sign-in unlocks. AWS's own sign-in page does not say it, and the app cannot change AWS's
pages, so the app shows a page of its own first, in the same browser window.

## Goals

- N minutes before a portal's session ends (N from Settings, default 10), the app opens that portal's sign-in
  in the browser, with no notification first.
- The browser first shows an app page that names the portal and its profiles, with a button that continues to
  the AWS sign-in.
- Signing in there starts a new session, recorded exactly like a sign-in started with the **SSO sign-in**
  button.

## Non-goals

- Opening the AWS console. The console uses the current session; it cannot extend it.
- A reminder or notification of any kind.
- Changing AWS's pages, or a browser extension.
- Signing in without the user: a new session always needs the user in the browser.
- The device-code flow for the automatic sign-in.

## Anti-goals (failures even if it works)

- Windows opening again and again for the same session (a loop).
- A page that does not make clear which portal it is for.
- Any URL check loosened beyond the one loopback page described below.
- The **SSO sign-in** button behaving worse than today (for example an "already in progress" error).
- Tokens, codes or other secrets on the app's page or in logs.

## What exists today

- `internal/accounts/session.go`: `Engine.Tick` runs every 30 s. `sessionStatusFor` works out the session end
  (`SessionEndsAt`, exact once AWS cut a token short). `maybeRemind` emits `session:reminder` when the end is
  within `ReminderLead`, in two cases: a known session end, or a token that cannot be renewed (its expiry).
  **Nothing listens to `session:reminder`** (not the frontend, not `internal/app/shell.go`), so the Settings
  switch "Remind me before sign-in expires" (`reminders.expiryNotice`, default on; `reminders.leadMinutes`
  15 | 30 | 60, default 15) has no visible effect.
- `internal/sso/login.go`: `Manager.Login` → `loginPKCE` listens on `127.0.0.1:<random port>`, serves
  `/oauth/callback` (state check, exact `Host` check), and opens the authorize URL through
  `Config.OpenSignIn` = `Container.openSignIn`. The callback waits `CallbackTimeout` (10 min). Logins are
  per sso-session key, so several portals can sign in at once.
- `internal/app/container.go`: `openSignIn` tries an isolated browser (a new Chromium-family instance on a
  fresh profile), then a private window, then the normal open. The window is tracked under the opened URL and
  closed by `signInEnded` (3 s after success, so the done page shows briefly).
- Only a sign-in in a fresh isolated browser surely starts a new session: a browser still signed in to the
  portal hands back its running session (comment in `Engine.Login`). The "private sign-in" setting is on by
  default.
- URL rules: `checkOpenable` / `checkHTTPS` (`internal/app/runtime.go`) and `browser.CheckURL` accept `https`
  only; sandbox mode also accepts loopback `http` in `checkOpenable`.
- `sso.CallbackPage(lang, ok, errValue)` does not name the portal.
- `LoginModal.tsx` starts its own `Accounts.Login`; on `E_LOGIN_IN_PROGRESS` it retries 4 times, 350 ms apart,
  then shows the error.
- Profiles carry an account colour slot (1..8) and a monogram; the palette is in
  `frontend/src/styles/tokens.css` (`--acct-1` … `--acct-8`, light and dark).

## Approaches

1. **Backend-driven (picked).** The 30 s ticker in the Go accounts engine starts the sign-in when the session
   end is near. The sign-in's own loopback listener also serves the intro page; the browser opens that page
   first. It works while the window is hidden in the tray, and reuses the isolated window, its closing and the
   session recording.
2. **Frontend-driven.** The React app listens to `session:reminder` and does what the button does. Rejected: a
   hidden window's web view can be throttled or paused by the system, so the sign-in might not start on time;
   the intro page needs backend work anyway.
3. **Intro page as a local file.** Rejected: one more temporary file to manage, and it needs the same URL
   exception; the loopback listener already exists with its own checks.

Also considered: Chrome's `--window-name`. Chromium-only, and macOS shows no window title bar, so it would not
be seen. Not pursued.

The user did not choose how it is built; approach 1 is the builder's (Claude's) call. The user chose option A
for the intro page: a button, no automatic forward.

## Behaviour

### Which portals

- Portals with an `sso-session` only. Legacy-format portals (`legacy:<host>`) and drafts are left out, as the
  tray leaves them out: the app never renews a legacy portal's token, so its end is about an hour after each
  sign-in and the window would open about every hour.

### When it opens

- Only for an **exact** session end (`SessionEndExact`: AWS has shown it). An estimated end (the learned length,
  or 8 h) moves to each new token's expiry when the real session is longer, so it cannot be trusted for this.
- With automatic renewal on, AWS shows the exact end inside the session's last hour, at the latest about 15
  minutes before the end (a token that expires inside the last hour is renewed with 15 minutes left and comes
  back cut short). So with N = 10 the end is always exact in time. With N = 30 or 60 it opens N minutes before
  the end, or as soon as AWS shows the end if that comes later.
- It needs **Renew sign-in automatically** (on by default). With that off the app does not learn the exact end
  and the window does not open; Settings says so next to the switch.
- With the setting on, the sign-in opens when `0 < time left ≤ N`. The ticker checks every 30 s, so it opens
  within about 30 s of the moment. If the app starts, or the computer wakes, inside that window, it opens right
  away (once the exact end is known). After the end, it does not open.
- Never for a session no longer than N in total (it would open right after every sign-in).
- It does not open for a portal that is signed out, expired, in error, or already signing in (by the button
  or automatically). The app must be running (window or tray).
- Browser (PKCE) flow only.

### At most once

- At most one automatic opening per sign-in (`LoginAt`), and never a second one for the same session end. So:
  ignored, cancelled or timed out means no second window for that sign-in; a sign-in that hands back the same
  session (a browser still signed in) has the same end, so it does not open another window either.
- A sign-in that starts a real new session gets its own exact end later, and can open once near that end.
- Remembering this in memory is enough; after an app restart inside the window it may open once more. The
  builder may persist it if that is cheap.

### The automatic sign-in

- Same as the **SSO sign-in** button (same window rules: isolated, then private, then normal) except that the
  browser opens the intro page first.
- It waits until the old session ends, but at least the usual 10 minutes, so a longer lead (30, 60) does not
  close the window long before the end.
- It runs in the background; the app window is not brought to the front.
- While it waits, the portal stays signed in everywhere: the old token still works, so the countdown,
  **Console**, **Terminal**, repository listing and cloning, and the tray behave as before. The portal only
  gets a small marker, such as "Sign-in page open" / "登录页已打开". (Today any running sign-in makes
  `sso.Manager.Status` report `signing-in`, which hides the countdown, Console and Terminal, makes the tray say
  "Not signed in" and marks the profiles unusable. The automatic sign-in must not do that.)
- **SSO sign-in** stays available on that portal; pressing it stops the automatic one (its window closes) and
  starts the normal sign-in, with no "already in progress" error. **Sign out** stays available and stops it
  too.
- There is no "Not now" button. Closing the browser window is how the user says no: the app does not notice,
  and the wait ends by itself when the old session ends. That is harmless, because the portal stays signed in
  meanwhile.
- On success the app records and shows the new session like after a manual sign-in.
- On failure or timeout nothing else happens: no retry, no device code. The old session ends as it would have.

### Intro page (only for the automatic sign-in)

```
┌───────────────────────────────┐
│ AWS SSO Hub                   │
│ Sign in again: acme           │
│ ■ AD acme-dev  ■ AP acme-prod │
│ This session ends at 17:00.   │
│                               │
│   [ Continue to AWS sign-in → ]│
└───────────────────────────────┘
```

- Served by the sign-in's loopback listener, on an unguessable path (a fresh random value, not the OAuth
  state), `GET` only, exact `Host` check like the callback, `Cache-Control: no-store`, a strict
  `Content-Security-Policy` (no script needed), `frame-ancestors 'none'`, `Referrer-Policy: no-referrer`.
- Content, in the app's UI language, light and dark:
  - the app name;
  - "Sign in again: {portal}" / zh "重新登录：{portal}" (copy follows `docs/design/copy.md`: zh uses 登录);
  - the portal's profiles: account colour, monogram, profile name (colours as in the app);
  - the current session's end as a local time (with the date when it is not today);
  - one line saying the app opened it because the session ends soon;
  - one button, "Continue to AWS sign-in" / "继续到 AWS 登录", linking to the authorize URL in the same tab.
- All names HTML-escaped (they come from `~/.aws/config`). No tokens, codes or account IDs on the page.
- The page can be reloaded and the button pressed again while the sign-in waits.
- A sign-in started with the button goes straight to AWS, as today.

### The one URL exception

- Opening `http://127.0.0.1:<port>/<path>` is allowed only for the intro page of a sign-in that is running:
  IPv4 loopback literal, no user info, the live listener's port. Everything else stays `https` only, on every
  path that opens it (isolated, private, normal).
- The intro URL is tracked like the authorize URL, so its window closes when the sign-in ends.

### Done page

- The callback page names the portal: "Signed in to {portal}" / "已登录 {portal}" (for every sign-in, manual
  too), escaped. It does not show the new session end: at that moment AWS has not given it yet. The app shows
  it.

### Setting

- The switch that does nothing today becomes "Open the sign-in page before the session ends" (zh e.g.
  "登录快到期时自动打开登录页"), with "How early" 10 / 15 / 30 / 60 minutes, default 10. On by default.
- Settings saved by an older version: the switch keeps its value; the lead becomes 10, because the old value
  was for a reminder that never appeared.
- `session:reminder` is removed (nothing listens). Mock backend, generated bindings and i18n (en, zh-CN) are
  updated.

### Docs

- `README.md`, `README.zh-CN.md` (features), `docs/guide.md`, `docs/guide.zh-CN.md` (sections 5 and 13), and
  the built-in tutorial if it covers this.
- Say that it needs **Renew sign-in automatically** on, that it leaves legacy portals out, and that it works
  best with private sign-in on: in a browser still signed in to the portal, AWS hands back the same session
  and nothing is extended.

## Constraints

- Go + Wails v3 + React, existing patterns, no new dependencies.
- Safety rules of `docs/plan/implementation-plan.md`: tests use temporary homes and fake servers, never a real
  `aws sso login`, never the real `~/.aws`; manual runs in sandbox mode.
- macOS and Windows.
- Work and commit on `main` (the user's instruction).

## Left to the builder

- The hook between the engine and the container (`Tick` must not block: a sign-in can wait many minutes).
- How `sso.Manager.Login` receives the intro page data.
- The palette in Go (mirror `tokens.css`, light and dark).
- How the automatic sign-in is stopped by the button or by sign-out.
- How the waiting automatic sign-in is told apart from `signing-in` (a flag, or a separate field), and where
  the marker goes on the card, the portal header and the sidebar.
- Whether to persist the once-per-sign-in memory.
- Exact copy, within the rules above.
- Tests: opens at N and not before; only for an exact end; never for a session no longer than N; once per
  sign-in and never twice for the same end; not for legacy portals or drafts; not while signing in; the portal
  stays usable while it waits; stops on button and sign-out; intro page escaping and headers; the URL
  exception stays narrow; the window closes when the sign-in ends; settings migration.

## Calls made without asking the user

The user decided: open the sign-in page by itself, no reminder, about 10 minutes before the end; the browser
names the portal and its profiles; option A (a button on the app's page, no automatic forward); work on
`main`. Everything below is Claude's call, for the user to check:

1. The sign-in page, not the console (only a new sign-in extends the session).
2. The page names the portal and lists its profiles (one sign-in covers every profile of the portal).
3. Backend-driven (approach 1).
4. Only an exact end triggers it; it needs automatic renewal on.
5. At most once per sign-in, never twice for the same end, never for a session no longer than N.
6. Legacy portals and drafts are left out.
7. Several portals: each opens its own window.
8. It runs in the background; the app window does not come to the front.
9. While it waits the portal stays signed in, with a small marker; **SSO sign-in** takes over, **Sign out**
   stops it; no "Not now" button.
10. It waits until the old session ends (at least 10 minutes).
11. The app's page only for the automatic sign-in; the button goes straight to AWS as today.
12. The done page names the portal (every sign-in), without the new end time.
13. The unused reminder switch becomes this feature: on by default, 10 / 15 / 30 / 60 minutes, default 10;
    older settings keep the switch and get a lead of 10; `session:reminder` is removed.
