# Group B wiring notes for F1-W

Collected at the group-B merge (2026-11-16) from the implementer reports and the fix rounds. F1-W implements the registrations; anything that is not registration goes back to the orchestrator.

## F1-02

- No new dependencies. Nothing is registered in container.go yet. F1-18 should call projects.Open(filepath.Join(paths.AppDataDir, "projects.json")). A missing file creates nothing until the first write.
- Open returns store.ErrNewerVersion (a *store.NewerVersionError with the file and app versions) or store.ErrCorrupt. The caller must surface these to the user and not fall back to an empty store, otherwise a later write would be refused anyway. store.File.Quarantine() moves a corrupt file aside.
- accounts.json and tasks.json should use store.Open with their own Spec, whose Migrations map covers every older version.
- paths.Paths has no ProjectsFile field. Add it in the wiring task if wanted.
- Breaking API change: store.File.Quarantine now takes the target pointer, as in f.Quarantine(&Data{}). Any caller added by another task must pass it.
- store.File.Update now zeroes v and requires a non-nil pointer, so callers (accounts.json, tasks.json) can reuse a cached struct safely. Update returns an error for a non-pointer.
- F1-18 can fill RepoRecord.MissingSince (*time.Time), FirstCommit and OriginURL without touching types.go.

## F1-03

- i18n/index.ts: register the new namespace (import * as menu from './locales/menu', then add it to the namespaces object and to the zhAll and enAll spreads). keys.test.ts then covers it.
- frontend/src/api/events.ts: add 'app:command': string to EventPayloads. AppCommands.tsx currently subscribes with Events.On directly. The Go side registers the event itself in menu.go init, so do not register it again in events.go.
- App.tsx: mount <AppCommands /> once inside ActionsProvider. Its Cmd/Ctrl+R preventDefault and App.tsx's Cmd+R refresh handler work together because the listener is in the capture phase and only prevents the reload.
- Later tasks hook in as follows. A2-05 calls life.SetTasksRunning(...) and A2-14 calls life.SetQuitDecision(...). Logout detection calls life.SetLoggingOut(...), and the update dialog's 'Reopen now' calls life.ApproveQuit(). The editor registers registerEditorTabsHook and registerLayoutHook from AppCommands.tsx, and pages listen for the codepass:command window event. main.go already creates the Lifecycle and passes it to NewShell.
- Pages that handle codepass:command ids need to cover: new-task, quick-open, save, close-tab, find, find-in-files, nav-projects, nav-tasks, nav-pulls, switch-session, toggle-terminal. nav-projects, nav-tasks and nav-pulls have no pages yet and are dispatched as events only.
- F1-09 owns internal/update. main.go calls the Wails updater.HandleHelperMode directly. If F1-09 adds its own AST test for the same thing, the two are compatible.
- Orchestrator: amend tasks.md F1-03/A2-14, design 4.4/5.6b and plan 4.8b for the dropped 20-second logout dialog (Wails 3.1.0 has no NSTerminateLater), assign the first-time 'Codepass is still running' notification to A2-08, and record that the Windows trash step now exists (0.0.1 is a portable zip, so only the exe is recycled).
- Manual-check entry to add (not my file): once A2-05 supplies tasksRunning, with a fake live task log out and restart on macOS and expect no 'interrupted log out' message; on a Windows VM, try the old-app handoff with a real 0.0.1 zip exe and cancel a shutdown at the 'apps are preventing shutdown' screen (Codepass must stay open).
- A2-05: the tasksRunning hook is called on the UI thread; it must read cached state and return at once.
- appguard.FileOps gained CanRecycle() and Recycle(path); any other fake of FileOps must add them. main.go is unchanged (HandleHelperMode still first).

## F1-04

- Construct with gitx.NewReader(g) from an existing *gitx.Git. No dependency, go.mod, container or event changes are needed.
- F1-05 (watcher) and the repos status code should call Reader.Status and never run `git status` through Git.Run, so the review question 'any path that bypasses the runner' can be answered by grepping for Git.Run in automatic paths.
- dir "" runs outside any repository (global and system config only), as Git.Run does.
- Callers outside the Reader (for example clone or fetch code in other tasks) that run git against a partial clone are unaffected; only Reader reads set GIT_NO_LAZY_FETCH=1. Reads of missing objects in a blob:none clone now fail with exit 128 and surface as E_GIT_FAILED, so the UI should treat that as 'content not local'.
- Reader now sets log.diffMerges=separate and refuses remerge forms, so any UI code that asked for remerge diffs must use -m or --diff-merges=cc.

## F1-05

- No new dependency or go.mod change. The Windows backend uses `golang.org/x/sys/windows` (already in go.mod as a direct dependency). The macOS backend needs cgo and links CoreServices and CoreFoundation. Without cgo (or off macOS and Windows) `New().Subscribe` returns `ErrUnsupported`.
- Use it as `w := watch.New(watch.Options{})`, then `unsub, err := w.Subscribe(repoPath, func(e watch.Event){...})`. Refresh on `e.GitChanged`, `e.Overflow` or any change. On `e.Removed`, stop or re-attach. `Event.Paths` is a sample capped at 64.
- The rename transaction (C3-10) should call `Pause(old)`, rename, `Relocate(old, new)`, then `Resume(new)`. A watcher stopper registered in the quiesce registry can wrap `Pause` and `Resume`.
- Call `Close()` on app shutdown. Callbacks for one folder never overlap, but they run on a timer goroutine, so subscribers must not block for long.
- F1-11: on Windows renaming or moving the watched folder itself (also Explorer Delete to the Recycle Bin) is noticed only by Check; call Check when the window regains focus or the machine wakes
- Relocate now returns an error unless every folder beneath oldDir is detached and covered by a pause; callers pause oldDir itself (not only a nested folder) before the rename

## F1-06

- Create one shared client at startup with netx.New(netx.Options{Proxy: netx.Detect(ctx)}) and share it. Re-run Detect when the user retries after a network failure.
- Child processes (brew, winget, curl, git): build the environment as append(netx.StripProxyEnv(base), cfg.ChildEnv()...). Never log that slice.
- No go.mod or go.sum changes were needed.
- Large downloads (the 154 MB Homebrew.pkg, the update download) must use netx.Client.Download() with a context and a progress watchdog, not Client.HTTP, which has a 30 s overall timeout. Download() shares the proxy, so SetProxy reaches it.
- F1-12 and the setup screen: when Config.Empty() and Config.Unsupported is "socks" or "autodiscovery" (or PACOnly()), show the "set the proxy by hand" message.
- Config now marshals to JSON in a masked form (credentials removed). It does not round-trip, so never persist a Config as JSON; keep a manual proxy's credentials in the Keychain.
- A configured proxy URL that does not parse now makes requests fail with a fixed error instead of going direct. The UI can show a 'proxy setting is invalid' message when HostPort() is empty but HTTP or HTTPS is set.

## F1-07

- Create the real block list outside the repo and export CODEPASS_SCRUB_LIST=<path> before running `scripts/scrub-check.sh`. Run it before every merge and in the R7-04 publish review.
- Optionally add a Taskfile entry (Taskfile.yml is shared, so not edited here), for example `scrub` running `scripts/scrub-check.sh` and `scrub:test` running `scripts/scrub-check.test.sh`. The test script needs no real list.
- `scripts/scrub-check.sh --commits codepass~3..HEAD` already flags AI attribution lines in the last three commit messages, and a non-example e-mail in each. Some of these commits were likely made before the no-attribution rule. Rewrite the messages before any publish.
- The script needs git and perl, both present on macOS and in Git Bash. It works with bash 3.2.
- docs/screenshots is skipped by design, as the task says. The docs task (R7-04) must re-take or remove those images, since the images cannot be scanned.
- Squash-merge F1-07 as one commit without attribution trailers so a --commits review of the pre-publish range does not inherit planted values from the early commits.
- Edits outside Owns, comment-only and consistent: internal/api/repos.go lines 83 and 91, and the generated frontend/bindings/github.com/rudian/codepass/internal/api/models.ts lines 249 and 1416. F1-11 should keep them or revert both together.
- The check now treats a tab or line break in any tracked path as a setup error (exit 2), and a block list in UTF-16 as a setup error. Mention both in the release checklist.

## F1-08

- frontend/src/lib/theme.ts: `applyMotion(settings.motion)` only writes `data-motion="reduce"`. Make it call `applyMotionPref` from lib/motion.ts (or delete it and call that from App.tsx). That writes `reduce` and `full` and removes the attribute for System, so Full works.
- frontend/src/pages/settings/SettingsPage.tsx and locales/settings.ts: replace the 'Reduce motion' Switch with a Segmented of System / Full / Reduced bound to `settings.motion` (MotionPref values `system`, `full` and `reduce`). Add i18n keys for zh and en: 跟随系统 / 完整 / 减少 and System / Full / Reduced. The hint is 「减少」会去掉滑动和淡入，只保留必要的进度提示。 / 'Reduced removes sliding and fading and keeps only the progress cues that are needed.' The old `prefersReducedMotion` logic in SettingsPage can go; `systemPrefersReducedMotion` and `resolveReduced` are in lib/motion.ts.
- frontend/src/components/index.ts: add `export { Sheet } from './Sheet'`, `export { ProviderMark, providerName } from './ProviderMark'` (plus type ProviderKind) and `export { ToolMark, toolName } from './ToolMark'` (plus type ToolKind).
- Screen tasks should use `.page-enter`, `.row-enter`, `.collapse`/`.expand`, `useFlip` and `animateInsert`/`animateRemove` instead of writing their own keyframes. For the task-state dot, use `.is-breathing` on a 'waiting' dot; Reduced makes it steady.
- No go.mod or package.json change was needed.
- F1-W: put .page-enter on the page content wrapper in App.tsx, keyed by route; Toast exit (160ms) and the slide-up of stacked toasts need Toast.tsx changes (outside F1-08 Owns)
- Owner of components/: Segmented.tsx onKey must capture `const group = e.currentTarget` before requestAnimationFrame
- Keyring.tsx:62 and pages/accounts/util.ts:17: use behavior: motionReduced() ? 'auto' : 'smooth' from lib/motion.ts
- Orchestrator: add SettingsPage.tsx, locales/settings.ts, lib/theme.ts and privateSignIn.test.tsx to F1-08 Owns; optionally change .gitignore `dist/` to `/dist/` so the .gitkeep negation works

## F1-09

- go.mod/go.sum: add golang.org/x/mod v0.41.0 as a direct dependency (go get golang.org/x/mod@v0.41.0), with go.sum lines h1:qJmnOUb4YB+FsEuM3HcWucdZASCPGhsX6uljO6pog0c= and /go.mod h1:Ek9pY8RKWXwsWvd3rQiHYtMqkjSUV+s1Rj7j4H5Ur6o=. Without it internal/update and its importers do not build.
- main.go: updater.HandleHelperMode() (github.com/wailsapp/wails/v3/pkg/updater) must be the first statement of main(). After that, update_test.go's AST test starts checking main.go.
- Startup: call update.Highest{Path: <data dir>/update/highest}.Record(version.Version) on each start and log any error. Call update.CleanHelperLogs() and log the paths it returns.
- Build a Controller with update.NewController(ControllerConfig{Quit: app.Quit, Running: version, Gate: update.ProductionGate(), Highest, Channel, OnState: emit}). Channel() returns update.ProductionChannel() for the public channel. For the test channel it builds update.Channel{Name: "test", Repository: <entered repo>, Prerelease: true, Keys: update.ParseKeys(<test key hex>), Token: <from update.NewTokenStore(runtime.GOOS).Get()>}. Start the check loop with go c.Run(ctx, 30*time.Second, 6*time.Hour).
- Service methods for the frontend: Check(), Download(), Restart() and State(). Emit the update.State struct on every OnState call. Phases: idle, checking, up-to-date, available, downloading, verifying, ready, restarting, blocked, refused, error. For blocked, the fields block (translocated, not-writable, unsupported-os) and folder drive the message, which must name the folder and be shown before any download. Notes are plain text and must be rendered as text only.
- Restart: for 'Reopen when no task is running', the app waits and calls Restart only when no AI task is running. Restart asks the host to quit through ControllerConfig.Quit once the helper is waiting.
- Token entry UI (test channel only): TokenStore.Set(token) accepts only [A-Za-z0-9_-]{20,255}. Never log or display the token.
- F1-20 must change gate_other.go's build tag to !darwin && !windows when it adds gate_windows.go. gate_windows.go must define GateSupported() bool and VerifyBundle(path, wantVersion string, g Gate) error and return *Refusal on failure. Until then Windows reports blocked/unsupported-os and nothing is downloaded.
- F1-10 key ceremony: put the two public keys (hex) into pinnedKeyHex in internal/update/identity.go. The release script should reuse DesignatedRequirement via the integration tool (`tool requirement`), or copy the same string. The archive is built with ditto --norsrc --noextattr --noqtn --noacl, otherwise the app refuses it as an AppleDouble archive.
- F1-W: use internal/update/doc.go as the wiring notes and discard the round-1 notesForWiring (they contradict it on Quit, CleanHelperLogs, test-channel keys, NewTokenStore(sandbox), and omit Retry, Close, HelperLog, StagedRecord).
- F1-W: call Controller.Close() from the shutdown hook; it is now safe inside Quit during Restart (the staged folder survives) and it cancels a running download, waiting up to 5 s.
- F1-W / F1-17: TestChannel(repo, token) now errors on an empty token; surface that as the 'enter the test channel token' state. State.checkedAt is omitted until the first check.
- go.mod/go.sum (golang.org/x/mod direct require, v0.41.0) were edited outside Owns in round 1. Keep both require lines when merging with other group-B branches and drop the 'x/mod becomes direct' step from F1-W.
- ControllerConfig.IdleTimeout and ProviderConfig.IdleTimeout are new optional fields (default 60 s); the wiring should leave them zero.

## F1-10

- F1-09 must pin the manifest format that relkit writes: ed25519 over "codepass-update-manifest-v1\n" plus the exact manifest bytes, base64 in manifest-<os>-<arch>.json.sig. The manifest fields are version, os, arch, file, size, sha256, notes; unknown fields are refused. Public keys are exchanged as hex (relkit keygen and relkit pub print hex), so the pinned keys in the app should be parsed from hex. The archive is named Codepass-<version>-darwin-arm64.zip. The release assets are the archive, the manifest and the signature.
- go.mod and go.sum are untouched: relkit uses only the standard library.
- The updater only accepts asset URLs under the final repository name rudian/codepass, so the private test channel must carry that name (see docs/release.md).
- The release runs `wails3 task package` and `go build` of relkit. WAILS3 defaults to $ROOT/.tools/bin/wails3, then PATH. The release host needs Command Line Tools installed, and the script checks xcode-select -p first.
- To release, set the version in internal/version/version.go, build/darwin/Info.plist and build/config.yml, commit, then run build/release/release.sh. Put the identity, team id and notary profile in the git-ignored build/release/local.env (copy local.env.example).
- F1-09 integration: add a check in release.sh that CODEPASS_TEAM_ID equals internal/update TeamID and that the signing key's public half is in pinnedKeyHex (production) or testKeyHex (test channel), read from the HEAD export; track on the merge checklist
- F1-20: the Windows exe file version gate can rely on win_file_version being one-to-one (1.0.0 -> 1.0.0.0, 1.0.0-rc.N -> 1.0.0.N, N >= 1)
- Owner: before the first real release, set origin so that its URL ends in the target OWNER/NAME if --target HEAD0 is wanted; for a separate test-channel repository the tag is created by GitHub at that repository's default branch (it needs the initial commit)
