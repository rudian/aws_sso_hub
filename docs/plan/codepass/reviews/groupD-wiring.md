# Group D wiring notes (F1-W)
For the group E tasks and F1-CL. Follow-ups FU-Wa..e are in tasks.md.

## notesForWiring (addressed to group E)
- Group E tasks: replace the whole page files in place and keep the export names. F1-14 replaces pages/projects/ProjectsPage.tsx (currently re-exports ReposPage as ProjectsPage). F1-15 replaces pages/setup/SetupPage.tsx. F1-17 replaces pages/update/UpdatePage.tsx. App.tsx and the sidebar need no change for them.
- Locale stubs projects.ts, setup.ts, identity.ts and update.ts are registered and empty ({} as const). The Sidebar uses nav.projects. The placeholder texts are in app.ts under 'placeholder.setup.*' and 'placeholder.update.*', and F1-15 and F1-17 can delete them once the pages are replaced.
- F1-17: Container.Update (*update.Controller) is already built and running. Write internal/app/updateservice.go on top of it (Check, Download, Restart, Retry and State). The controller emits update.State on EventUpdateState = 'update:state'. Register and type that event when the service is registered. Call life.QuitApproved around Restart, which main.go already does for the controller's Quit.
- Generated IdentityService and ToolService bindings exist. The 'identity:changed' payload is typed Record<string, never> in EventPayloads, and 'tools:install-progress' is InstallState. 'app:command' is typed as string in EventPayloads.
- Container.Projects is the single projects.Store, shared by repos.Deps.Registry and the IdentityService. Nothing else should open projects.json.
- The repository watcher is created in NewContainer and closed through OnShutdown. Container tests that never call Shutdown leave one watcher open, which is harmless.
- FU-18d is in internal/app/startup.go. main.go calls app.OpenContainer, which returns app.ErrStartupStopped after the dialog and has the caller exit quietly.
- RepoService.MarkOpened(repoID) must still be called by the Projects UI (F1-14) when a repository is opened, or it never joins background fetch.
- F1-15: replace pages/setup/SetupPage.tsx only; the shell already renders it alone (no sidebar) while Tools.GetSetupState().blocked. After a successful install or manual re-check call requestGateRecheck() from lib/gate.ts to reopen the gate. The Setup route is not needed for the gate itself.
- F1-14/F1-16/F1-17: Sidebar.tsx and the nav order are final per design 4.1; add pages by route, not by editing NAV. The 'repos' PageId is now an alias that renders ProjectsPage; pages/repos/ReposPage is no longer imported by App.tsx, so F1-14 can delete the folder once ProjectsPage no longer wraps it.
- OpenContainer shows two native dialogs before the window: newer-version (quit) and damaged projects.json (continue, names the quarantined file). A new store file opened in NewContainer must wrap its Open error in storeFileError{File, Err} to get the same handling.
- The sidebar now matches the mockup's plain 12px gap with no divider line. F1-14, F1-16 and F1-17 can compare screenshots against it.

## deviations
- The Git settings route stays on the existing GitGlobalPage instead of becoming a placeholder. F1-16 extends that same page (its Owns is pages/git/) and a placeholder would remove the working CodeCommit guard cards. Only the sidebar label changed, to 'Git 设置' / 'Git settings' per design 4.1.
- The plan's exact 'written by a newer Codepass' text is not in plan or design, so I wrote the dialog text myself (internal/app/startup.go). It is bilingual because settings, and so the UI language, cannot be read at that point. It is shown through appguard's native Prompter.Tell (osascript on macOS, message box on Windows). Please review the wording.
- Offline detection: the repo had no connectivity signal. Background fetch is skipped when no non-loopback network interface is up and has an address (no network traffic). That is the only offline signal, and ContainerOptions.Online can override it. The first fetch runs 15 minutes after start, not at start.
- Update controller: no UpdateService or API state type exists yet (F1-17 owns them), so StartUpdates (internal/app/updatewiring.go) builds the controller and emits update.State on 'update:state' (constant EventUpdateState in package app). That event is deliberately not registered with RegisterEvent and not in EventPayloads. The controller is kept in Container.Update. Checks run only outside sandbox mode, 30 s after start and then every 6 h. With no pinned release keys the production channel errors before any network call.
- Sidebar order: Projects, Git settings, Terminal, Health, Tutorial. The Accounts item stays first, as it was; design 4.1 puts Projects first but AI tasks and Pull requests (milestone 2) have no pages. The 'repos' route is kept as a legacy id that highlights Projects, and 'projects' is the new route.
- Keyboard: Cmd/Ctrl+1..4 now follow design 4.3 (1 = Projects, 4 = Accounts, 2 and 3 do nothing until AI tasks and Pull requests exist). Cmd+5 and Cmd+6 no longer navigate.
- 'check-updates' menu command now routes to the new Update placeholder page instead of Settings. 'nav-projects' now routes to Projects.
- Moved-folder offer (groupB2 note: MovedFrom field and a service method for ReattachMoved) is not done. The notes for RepoService (ReattachMoved, DeclineMoved, movedFrom) show F1-11 already added them. This was registration-only work for me.
- Mock tool types: system.ts keeps its string-literal logic through a local Lit<> type that maps the generated enums to their string values. The service methods cast to the generated types at the boundary. Converting about 115 literals to enum members would have been a large change to the mock logic.
- The F1-08 notes on theme.ts applyMotion, SettingsPage and the components/index.ts Sheet export were partly done already (applyMotionPref and the Segmented are in). I added only the missing component exports.
- The Setup gate routes only: on launch the app calls Tools.GetSetupState and, when blocked, sets the route to the Setup placeholder. It does not stop navigation elsewhere (F1-15 builds the real gate).
- A shared netx client is created in main.go (netx.New with netx.Detect, 5 s timeout) and passed to NewToolService and the update controller's proxy resolver. Its proxy is read once at start; the update resolver reads nc.Proxy() on every request.

## deferred
- Optional Settings update-dot hook in Sidebar.tsx: not added; F1-17 can add it in Sidebar.tsx, which F1-W owns, so they should ask wiring or edit it with the update controller state.
- No App-level test for the gate ordering: initialUI reads the URL at import time, so it was verified by the Vite mock screenshot instead.
- No screenshot comparison was taken. The change only removes a CSS rule and renames an aria-label.

## manualChecks
- Real app in a sandbox (AWSSSOHUB_SANDBOX=<new temp dir>): the app starts, and the Projects, Git settings and Terminal pages open. Setup and Update open only through ?page=setup / ?page=update in the Vite mock (the menu's 'check-updates' command opens Update). The sidebar shows Projects in place of Repositories.
- With a projects.json copy written by a newer schemaVersion (use a temp data folder, never the real one): the app shows the bilingual native dialog, quits, and the file stays byte-identical. With a damaged projects.json: the app opens with an empty list and a projects.json.corrupt-<unix> file next to it. Check the dialog's look and wording on macOS and on Windows.
- Background fetch: with a bound repository on a real network, a fetch happens about 15 minutes after start. With Wi-Fi off, nothing is attempted and no error appears in the log.
- Update controller: a build with no pinned keys records the running version in <data>/update/highest and shows no crash. The real update path (including the Windows gate on a VM) stays with the F1-09/F1-10/F1-20 manual checks.
- Tools gate on a Mac or VM without git 2.44 or the AWS CLI: the window opens on the Setup placeholder (the real screen is F1-15).
- Reduced motion: with Settings > Motion set to Reduced, clicking a keyring row scrolls to the portal without smooth scrolling. Switching pages fades and rises the page content (page-enter) when motion is on.

## Orchestrator additions (after the merge)
- `fd36f8b`: `frontend/src/test-setup.ts` (Vitest setup) lets the Wails runtime's first drag poll run before a test file ends; it fixed an intermittent 'window is not defined' unhandled error that failed the gate with every test passing.
- `821cf55`: `components/RepoName.tsx` is a stub with the final props (`localName`, `remoteName?`, `className?`); `pages/projects/CloneModal.tsx` re-exports the clone modal from `pages/repos/`. F1-14 uses both; F1-19 replaces both.
