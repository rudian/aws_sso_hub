# Group F wiring notes (F1-21)
For F1-22, F1-CL and the milestone 1 gate. Follow-ups FU-21a to FU-21c are in tasks.md.

Merged 2026-11-16 as `2c24815` with `git merge --no-ff` (history kept): the merge of `main` is `5f5a9fe` (parents `5d7e903` and `3674f3f`), followed by four adaptation commits on `task/F1-21`. `git log codepass..main` prints nothing. One full review (four findings: one minor, three nits), one fix round, verification approved. Gate after the merge: 56 test files, 755 tests.

## F1-21

### What the merge resolved
- Version: Codepass stays at 0.0.1 in every version file (`internal/version/version.go`, `build/config.yml`, both `Info*.plist`, `build/windows/*`, `frontend/package.json`, `frontend/package-lock.json`). The lock file and `version.go` were taken whole from codepass, so no `npm ci` was needed.
- Bindings: the two conflicting files were regenerated; the merge left no `rudian/aws_sso_hub` bindings folder.
- `internal/app/app_test.go`: main's assertions kept (`rep.Version == version.Version`, `AWSSSOHUB_SIGNIN_ENDPOINT` in sandbox mode only).
- `internal/app/loops.go`: codepass's loops plus main's `c.spawn(func() { c.removeStaleSignIn(ctx) })`, now covered by `TestStartLoopsRemovesStaleSignInProfiles`.
- Tutorial `en.ts` / `zh.ts`: main's meaning (the app cannot read the administrator's session length and works the end out from AWS) with the Codepass name.

### notesForWiring
- Every quit path ends in Wails' cleanup, then `OnShutdown`, then `Container.Shutdown`, which runs `closeSignInWindows` (the first registered hook). This was checked in Wails v3.1.0's macOS `applicationShouldTerminate` and Windows destroy code. The logout path (`ShouldQuit` returns true) and the update restart (`QuitApproved`) use the same route, so `session_darwin.go` and `window.go` needed nothing. **A2-14's "stop all" must also quit through `app.Quit` or `Container.Shutdown`** to keep this.
- During a quit, `openSignIn` returns `E_CANCELLED` ("the app is quitting") at entry and again after a failed isolated attempt; it never falls back to a private or normal window then. `openIsolated` counts opens in progress (`signInOpening`, a `sync.Cond` on `signInMu`) and `closeSignInWindows` waits for them within its budget.
- Only `AccountService.OpenSignInPage` and the SSO sign-in (`c.openSignIn`) use the isolated window. The update page, install pages, `SystemService.OpenURL`, `OpenPortal`, `OpenConsole` and the menu links use `RT.OpenURL` (a normal window). **A new feature that opens a sign-in page calls `c.openSignIn`** (F1-22's "Sign in again" button does).
- Test helpers in `internal/app`: `waitCond` (main's `signin_test.go`) and `waitFor` (`window_test.go`) both exist; `terminate(c, life)`, `openFakeSignIn` and `liveLifecycle` live in the quit tests (`quit_signin_test.go`).

### deviations (accepted)
- Item (d): `settings.json` is still read by `internal/settings`, not by the versioned `internal/store` (that package never touches `settings.json`), so there was nothing to change. main's normalisation of `sessionEndsAt` auto-merged; a 0.0.2 fixture test (fake data) proves `sessionEndsAt` and `newSession` survive.
- Item (e): README, `docs/guide.md` and `docs/guide.zh-CN.md` already say what 0.0.2 says; main's new lines say "the app" and name no product, so nothing was reworded. The guide's step about the session duration in the administrator's AWS console is AWS advice and stays.
- Item (f): `EndpointsFromEnv` already reads `AWSSSOHUB_SIGNIN_ENDPOINT` in sandbox mode only; main's test covers it.
- Four strings keep `rudian/aws_sso_hub` on purpose: `REPO_URL` in `frontend/src/pages/settings/util.ts` and `GuideURL`, `GuideURLZh`, `ReportURL` in `internal/app/menu.go`. They are links to the GitHub repository, which is renamed only at release (R7-08). No import path uses the old name.

### deferred
- Nit (manual checks): the implementer's manual checks reused the IDs C19 to C21 for other content. The orchestrator kept C19 to C21 as written and added C26 (quit paths, leftovers after a kill, Windows shutdown, normal windows for Settings / Update / Health links).

### followUps (from the verification; in tasks.md)
- FU-21a (minor, F1-CL) `internal/app/container.go` near line 500: a macOS isolated launch that is already under way when Shutdown cancels `c.ctx` can start the browser while `OpenIsolated` returns an error; the profile folder is deleted and nothing ever closes that window. Pre-existing in 0.0.2. Fix: run the launch, after the entry check, on `context.WithTimeout(context.WithoutCancel(c.ctx), 15*time.Second)`, so a launch under way returns its handle and the quit branch closes it; test with a fake `Output` whose open succeeds only after Shutdown began.
- FU-21b (minor, F1-CL) `internal/app/container.go` near line 615: windows handed to `closeSignIn`'s plain goroutine before the quit (the old window after "Open again", the windows of a sign-in that ended just before) are not awaited by Shutdown. Pre-existing in 0.0.2. Fix: track that goroutine (a `sync.WaitGroup` or `c.spawn`) and let `closeSignInWindows` wait for closes in flight; test "Open again" with a slow fake Close, then Shutdown.
- FU-21c (nit, F1-CL) `internal/app/quit_signin_test.go` near line 158: removing the check after the isolated attempt (`container.go:505`), or the entry check with the private sign-in setting off (`container.go:496`), fails no test. Require `E_CANCELLED` with no opens in `TestSignInWindowOpenedDuringShutdownIsClosed`, and add the setting-off case to `TestSignInPageAfterShutdownOpensNothing`.

### manualChecks (for the owner)
- C19 to C21 as written in `manual-checks.md`, plus C26.
- On a real Codepass build: the private sign-in setting still opens the isolated window, and links from Settings, Update and Health open a normal window (in C26).
