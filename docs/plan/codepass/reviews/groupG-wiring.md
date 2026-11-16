# Group G wiring notes (F1-22)
For F1-CL, the milestone 1 gate and milestone 8 (its notice kinds). Follow-ups FU-22a to FU-22g are in tasks.md.

Merged 2026-11-17 as `f3fc3e2` (squash, one commit on top of `c90f81d`). One full review (three majors, four minors, two nits), one fix round, verification approved with five follow-ups (two minor, three nits). Gate after the merge: 60 test files, 782 tests.

## F1-22

### What it built
- `internal/notify`: the notice service (kinds, one button at most, the data map, the permission state, the store `notifications.json`), the fake, the real backend around Wails 3.1.0's `pkg/services/notifications`, and the sign-in texts (`signin.go`, design 6 in both languages).
- `internal/app/notifyservice.go`: the bound `NotifyService` (the Settings permission row, toasts, `SetReady`, `RespondToast`) and the container side (`noticeLog`, `sendSignIn`, the click handler).
- `internal/accounts`: the heads-up (`maybeRemind`) and `ended` (`watchEnd`) are sent once per sign-in and survive a restart; `NoticeLog` records them.
- Frontend: the Settings rows (`pages/settings/NotifyRows.tsx`, design 5.14), the in-app toast, "show this portal once" on Accounts (`pages/accounts/focus.ts`), the mock (`api/mock/notify.ts`) and locale `notify.ts`.
- Settings: `reminders.endedNoticeOff` (zero value = notify; a 0.0.1 or 0.0.2 file reads as notify).

### Real or fake
- A release build (`-tags production`) uses the system's service. Sandbox mode, every test binary (`testing.Testing()`) and a development build use the fake, which also shows each notice as a toast. A development build outside sandbox mode uses the real service only with `AWSSSOHUB_REAL_NOTIFICATIONS=1` (paragraph in `docs/release.md`).
- The owner's checks C22 to C24 use a test-channel build. A development build without `AWSSSOHUB_SANDBOX` uses the real data folder and the real `~/.aws`, so it is not the way to test real notifications (FU-22g fixes the release.md wording).
- The Wails service is never put bare in `application.Options.Services`: `internal/notify` wraps and starts it, and a start failure or panic leaves toasts only.

### notesForWiring
- `main.go`: `application.NewService(app.NewNotifyService(c))` is FIRST in the Services list on purpose: its `ServiceStartup` starts `internal/notify`, which sets the macOS delegate and the Windows COM class before the accounts ticker or a click can arrive. It never returns an error.
- `internal/app/container.go`: field `Notify *notify.Service`; `initNotify(o)` right after settings open, before `accounts.New` (which gets `Notices: noticeLog{c}`); `wireNotify()` right after. `ContainerOptions.NotifyBackend` and `NotifyAlsoToast` are for tests. `internal/app/events.go` registers `session:ended`, `notify:toast` and `notify:state`. Bindings are regenerated and committed.
- `accounts.NoticeLog` has `SignInStarting(portal)`, called at the top of `Engine.Login`: the window-in-front check for the permission question is made when the sign-in starts, not when it ends (the browser is in front then). Implementers of `NoticeLog`: `internal/accounts` `memLog` and `internal/app` `noticeLog` (the test `recLog` embeds `memLog`).
- "Sign in again" emits `app:navigate {page: "accounts", action: "login", arg: portal}` without `ShowWindow`; `App.tsx` opens the login modal, which calls `AccountService.Login` like the pass (isolated window through `c.openSignIn`, Cancel, Open again, device fallback, toasts). The stale and signing-in checks stay in Go. A click is handled only after the window reported `Notify.SetReady(true)` (accounts loaded, gates open); a blocked gate reports `false` and drops it.
- While that portal's sign-in already runs, the button opens its page again through `c.openSignIn` (the real `browser.Isolated` has no `Focus` yet: FU-22e).
- Legacy pseudo-portals (`sso_start_url` profiles without `sso-session`) get no notices: `Tick` skips them and `NoticeInfo` returns `ok=false`.
- `KindSpec.OnClick` runs inside the system's callback (macOS delegate, Windows COM activation) and must return quickly; the one slow path (`focusSignIn`) is spawned. **Milestone 8 (T8-W)**: a new notice kind registers with `Notify.RegisterKind`, has one button at most, puts identifiers only in the data map and sends through `Container.Notify.Send`; move `OnClick` to `Options.Go` with a panic guard if its handlers need to block.
- `go.mod`: one new indirect requirement, `git.sr.ht/~jackmordaunt/go-toast/v2 v2.0.3`, which the Windows build of the Wails notifications package needs.
- Milestone gate shots: `?mock&scenario=mid&page=settings&open=reminders` with `&notify=notasked|off|allowed`, `&platform=windows` for the Windows help line, plus `&theme` and `&lang`; the toast: `?mock&page=accounts&toast=ended|headsup` (`&toastPortal=<name>`). The implementer's shots were deleted with its worktree; the gate takes them again.

### deviations (accepted)
- Shared files outside the orchestrator's list were edited because the task needs them: `frontend/src/i18n/index.ts` (registers locale `notify.ts`), `frontend/src/api/mock/world.ts` (`endedNoticeOff: false`), `internal/accounts/engine.go` and `portals.go` (`NoticeLog`, rename and remove), `internal/api/events.go` (`EventSessionEnded`), `internal/app/runtime.go` (`WindowVisible`, `WindowFocused`), `go.mod` / `go.sum`, and small edits to existing tests.
- The Settings status row "System notifications" is a card at the top of the single-column page (the page has no right column yet).
- The card title and first help line use new keys `settings.notify.title` and `settings.notify.expiry.hint` in locale `notify.ts`. The unused `settings.reminders` key is already deleted, so F1-CL's unused-key cleanup skips it.
- Notice bodies never carry the Identity Center user ("alex · "): the app does not know it. The builder supports it and is tested; the mock leaves it out too.
- Safety: a first draft of `TestDamagedNotificationsFileDoesNotStopTheApp` built a container without the fake endpoints, so one sign-in request may have reached the real AWS OIDC address with fake data. AWS rejected it (`E_AWS_API`); no credentials or real account data were involved, and the final test uses the fakes. FU-22f makes tests fail closed.

### deferred
- `OnClick` through `Options.Go` (milestone 8, if its kinds need it).
- A real `Focus` on `browser.Isolated` (FU-22e, F1-CL).

### followUps (in tasks.md)
- FU-22a (minor) stale authorize URL reopened by the button between two sign-ins.
- FU-22b (minor) the button for a second portal does nothing while a login modal is open.
- FU-22c (nit) no app-level test that sign-in and sign-out remove the delivered notice.
- FU-22d (nit) the heads-up says "about 1 hour" when less is left.
- FU-22e (minor) `browser.Isolated` has no `Focus`.
- FU-22f (minor, safety) tests must fail closed on real AWS endpoints.
- FU-22g (nit, docs) `docs/release.md` points the owner's checks at a development build.

### manualChecks (for the owner)
- C22 to C24 and B19 in `manual-checks.md` were reworded after this merge: the permission question comes after the first sign-in that started with the Codepass window in front (once); "Sign in again" opens the login in the window (the browser comes up, the window stays where it is); a successful sign-in removes the notice; the window hidden in the tray; two portals ending together; legacy profiles get no notices.
- The owner reads the notice texts once in both languages (`internal/notify/signin.go`; the demo copy is in `frontend/src/i18n/locales/notify.ts`) and judges the Settings rows against design 5.14 in the milestone gate shots.
