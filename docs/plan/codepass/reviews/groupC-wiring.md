# Group C wiring notes (F1-11, F1-12, F1-13, F1-20)
For F1-W and the group E tasks. Follow-ups are in tasks.md (FU-11x, FU-12x, FU-13x, FU-20x).

## F1-11

### notesForWiring
- container.go: pass `Watch: app.NewWatchPort(<*watch.Watcher>)` in `repos.Deps` (the adapter is in `internal/app/reposervice.go`). Without a watcher, status refreshes only through `RefreshStatus`. `RepoService.ServiceShutdown` calls `Engine.Close()`, which stops the subscriptions.
- Schedule `Engine.BackgroundFetch(ctx)` yourself, for example from loops.go, never for hidden or unreviewed repositories (the engine already enforces that). Call `RepoService.MarkOpened(repoID)` when the person opens a repository.
- The repos.Registry interface grew by nine methods, all implemented by `*projects.Store`: `IDsForFound`, `ResolvePending`, `SetHidden`, `SetProject`, `AddProject`, `RenameProject`, `RemoveProject`, `SetProjectCollapsed` and `DismissSuggestion`. Any other registry implementation must add them.
- No separate projects service exists: project create, rename, remove, collapse, move and suggestions are methods on RepoService (`CreateProject`, `CreateProjectWith`, `RenameProject`, `RemoveProject`, `SetProjectCollapsed`, `MoveToProject`, `DismissSuggestion`). `ResolveFound`, `SetHidden`, `AddFolder`, `ReattachMoved`, `DeclineMoved`, `RefreshStatus`, `MarkOpened` and `FetchNow` are also on RepoService. Every state-returning method emits `repos:changed`.
- Bindings are regenerated and committed. The mock in `frontend/src/api/mock/repos.ts` implements all new methods against the generated types. A mock `Scan` finds 4 new repositories once per world; the other mock scenarios are unchanged.
- F1-14 should explain `E_MOVED_HAS_DATA` and `E_MOVED_GONE` and show `repo.movedFrom` (`oldId`, `name`). It should show `ReposState.warnings` (`projects-not-writable`, `count`) and `counts.found` / `counts.hidden`. The Found chooser pre-ticks rows whose `lastCommitAt` is within 90 days.
- FU-18d (a newer or corrupt projects.json makes NewContainer fail) is still F1-W's.
- internal/api/errors.go was not touched; the generated TS `ErrorCode` enum picks up the new codes because they are typed constants in the same package.
- repos.WatchPort changed: Subscribe(dir string, fn func(repos.WatchEvent)). app.NewWatchPort(w) already returns the adapter, so wiring is unchanged; any other WatchPort implementation must pass WatchRemoved or WatchStopped when the watcher reports Removed or Stopped.
- Background fetch now creates an empty folder <AppDataDir>/no-hooks (0700) on first use and points core.hooksPath at it; the app must not put files there.
- No api types or service methods changed this round, so frontend/bindings did not need regenerating.
- Edits outside the Owns list, kept minimal: frontend/src/api/mock/world.ts (repoToFix for non-CodeCommit, done-scenario seed) and world.test.ts (done counts); F1-14 should use the done scenario for projects, a hidden row and the moved question.

### deviations
- Hidden repositories now leave `ReposState.Repos`, as plan 4.4 requires. Two F1-18 tests used `SetHidden` only as a marker for 'record has data of its own' and relied on the row staying listed. I changed those marker calls (and the matching assertions) in `registry_test.go` and `registry_fix1_test.go` to set an identity override instead, through a helper in the new `ownerdata_test.go`. Every other existing test file in internal/repos is untouched and passes.
- FU-18a still leaves the hasOwnData check in `Reattach`, so a direct answer on a found record with data returns `ErrHasData` as the existing projects test expects. Only the offer (`MovedFrom`) disappears.
- Two edits outside the Owns list in internal/projects, needed for the review set, dismissed suggestions and project edits. In `types.go` I added `RepoRecord.Pending` and `Data.DismissedSuggestions`, both zero-value safe so old projects.json files load unchanged. `clone()` in projects.go deep-copies the new slice. The new store methods are in a new file `internal/projects/review.go`: `IDsForFound`, `ResolvePending`, project rename, remove and collapse, and `DismissSuggestion`.
- Edits to shared frontend files, forced by the generated types: `frontend/src/api/mock/world.ts` (new Repo fields, projects, found and hidden lists, suggestions, warnings), `world.test.ts`, `services.test.ts` and `pages/repos/model.test.ts` (counts gained `found` and `hidden`), `frontend/src/api/errors.ts` (three new error codes, because its Record is exhaustive) and `frontend/src/i18n/locales/errors.ts` (zh and en title strings for them, which F1-14 may refine).
- New API error codes live in `internal/api/repos.go`, not `errors.go`, so the Owns list holds: `E_MOVED_HAS_DATA`, `E_MOVED_GONE` and `E_PROJECT_NOT_FOUND`. Hidden CodeCommit repositories stay in the CodeCommit set and are only routed to `hiddenRepos`.
- Newly found CodeCommit repositories are never pending: they are listed at once, as today. Only non-CodeCommit repositories wait for the review. Reason: existing tests, and 0.0.1 behaviour, list a new CodeCommit clone at the next scan. A hand-added or cloned repository is shown at once.
- `AddExisting` is unchanged and still refuses a non-CodeCommit repository with `E_NOT_CODECOMMIT`, because existing tests assert it. The new `AddFolder` service method accepts any git repository and delegates CodeCommit ones to `AddExisting`.
- Folder-name validation also rejects `< > : " | ? *`, control characters and names over 255 bytes. These go beyond the Build list; they hold on every platform because a Windows copy would break otherwise. A name like `aux.` reports reason `trailing` (the trailing check runs before the reserved-name check).
- `Reader.Status` retries without rename detection by building its own command line in `status.go`, with `-c status.renames=false -c diff.renames=false`. The read-form allow-list in `read.go` (not mine) is untouched. The retry happens on any first failure, not only in partial clones. The result is marked `RenamesSkipped`, surfaced as `RepoStatus.renamesSkipped`.
- Background fetch is a policy plus a callable engine, not a scheduler. It exposes `FetchTargets()`, `Fetch(id)`, `BackgroundFetch(ctx)` and `MarkOpened`, but it does not decide when to run. The caller schedules it.
- A tooling slip at the start: my first `git worktree add` used a relative path and created the worktree inside the main checkout. I removed that worktree and its branch at once and recreated both at the specified location. The main checkout is clean.

### deferred
- Nit: routing rec()/classification/detection through StrategyFor(url).AWSAccounts(). The CodeCommit/other split comes from gitx.Inspect, and rec() refuses by set membership; rerouting would touch every CodeCommit path (byte-for-byte rule) for no behaviour gain. strategy.go now says the M6 binding seam uses it.
- Optional: stop reading lastCommitAt for hidden rows. The existing test and the Hidden list rely on LastCommitAt for hidden rows (the chooser pre-ticks recent ones); left unchanged.

### declined

### manualChecks
- Run the new build against a COPY of the owner's real data folder (never the live one). Confirm the 13 known repositories stay in the list with their saved connection results, and that repositories found next to them (forks, GitHub or Bitbucket clones, plain repositories) show in the review set and not in the list. Confirm settings.json stays byte-identical to the copy.
- On the real scan folders, check that 'Hide them all' and 'Choose which to show' behave as in the design and survive an app restart (the pending state is in projects.json).
- Clone a CodeCommit repository under a different folder name (for example remote acme_api as api-main) on a real Mac. Confirm both names show, and that CON, nul.txt and a trailing-dot name are refused before git runs.
- Rename a folder in Finder inside a scan folder, rescan, and answer yes and no to the moved question through the F1-14 UI once it exists. A found folder that already has a project or is hidden should lose the question, not fail on yes.
- A partial clone (`--filter=blob:none`) of a real repository with a staged rename: status should show the row with 'renames not detected' and no error. Needs git 2.44 or newer (FU-04a, F1-12).
- A Windows 11 VM: reserved-name and trailing-dot folder names are refused; a hidden repository leaves the list and its folder is no longer watched (the watcher adapter has not been run on Windows).

## F1-12

### notesForWiring
- Register the service in main.go: application.NewService(app.NewToolService(c, nil)). The second argument is the app's netx client; nil makes the service build its own from netx.Detect. Pass the shared client once the wiring task creates one (it only needs Do, Download and Proxy; SetProxy is used when present).
- internal/app/events.go: add application.RegisterEvent[api.InstallState](api.EventToolInstall). The event name is 'tools:install-progress', declared in internal/api/system.go.
- Regenerate bindings after registration (.tools/bin/wails3 generate bindings -clean=true -ts -i). Then delete frontend/src/api/mock/toolTypes.ts, type mockToolService as `typeof ToolReal` from the generated toolservice module, import the types from '../models', and add 'tools:install-progress': InstallState to EventPayloads in frontend/src/api/events.ts. The hand-written types mirror the Go types exactly (slices as T[] | null, omitempty fields optional). The enums become TS enums in the generated models.
- frontend/src/api/services.ts: export Tools = useMocks ? mock.tools : ToolReal. frontend/src/api/mock/index.ts: add tools: mockToolService (exported from './system'; resetToolMock is exported for tests and screenshots).
- Startup gate: call ToolService.GetSetupState on every launch; blocked=true means the setup screen replaces the window. The existing Container.CheckEnvironment still uses envcheck.CheckGit, so GitMin 2.44 already applies to the Health rows; it may want to share the new gate rows.
- go.mod: golang.org/x/sys is already required; internal/envcheck/freespace_windows.go imports it directly, so move it out of the indirect block if go mod tidy asks.
- Mock screenshots for F1-15: ?setup=missing|old-git|noadmin|managed|lowdisk|partial|proxy|nowinget, plus &setupFail=<cause> and optionally &setupFailAt=<step id>; platform follows initialUI.platform.
- The Go service keeps Homebrew.pkg in <AppDataDir>/downloads (0700). It deletes only that one file by exact name, and only after install or after a failed signature or damaged-size check.
- Edits outside F1-12's Owns, for the scope note: internal/gitx/git.go (MinVersion 2.44.0, dev-tool stub guard in New), internal/gitx/neutral_test.go, frontend/src/lib/toolVersions.ts. All are one-line or test-only changes.
- NewToolService(c, nil) now also sets ToolDeps.ClearPath from the Container (it builds a SystemService{c: c} and calls UpdateSettings with an empty GitPath or AWSCLIPath). Nothing extra to register.
- api.SetupTool has a new field `override` (bool, omitempty) and api.SetupFix a new value 'use-detected'. Frontend/src/api/mock/toolTypes.ts mirrors both by hand; after the wiring task regenerates bindings, delete toolTypes.ts as before. F1-15 needs a row for fix 'use-detected' (text like 'The git you chose in Settings is too old (2.43.0); the one found automatically will be used', no install step) and should treat override rows accordingly. The 'chosen-git' mock scenario (?setup=chosen-git) shows it.
- Setup run behaviour change for F1-15 copy: a Settings path that blocks the gate is cleared (backed up like any settings write) when 'Install' is pressed, even when the run then installs a new copy.
- The Homebrew row now only recognises /opt/homebrew/bin/brew. A Mac with only /usr/local Homebrew goes through the pkg flow and ends up with /opt/homebrew next to it; the screen text should not call that row 'installed'.

### deviations
- Task text names only mock system.ts, but the hand-written TS types needed a home, so I added frontend/src/api/mock/toolTypes.ts and tools.test.ts (new files in the mock folder). toolTypes.ts also augments EventPayloads with 'tools:install-progress' through module augmentation, so events.ts is not edited. Delete it once the bindings are generated.
- The pre-flight and installer API types and the event name constant EventToolInstall live in internal/api/system.go (owned) instead of events.go (shared).
- No locale strings were added: the setup copy is F1-15's. The only Go-side strings are the two sentences in the macOS password window (zh-CN and en), chosen by the UI language.
- The privileged script takes a fifth argument (expected sha256) and verifies it, which closes the review's 'digest is not checked' finding. It uses /usr/bin/openssl dgst, not shasum, so it does not depend on perl.
- Unknown signature verdicts use the script's own texts (pkgutil's 'Status:' and 'Notarization:' lines) and a leaf line of the form '1. Developer ID Installer: ... (TEAMID)'. A leaf that is not a Developer ID Installer is cause bad_signature, not unknown_signer.
- Copy failure (script exit 10, including 'not a regular file') maps to cause installer_failed with the script's text in the log, not unknown. Exit 21 (not root) stays unknown.
- A wrong password from an account whose pre-flight says 'not an administrator' is reported as cause not_admin; for an administrator it stays auth_denied.
- Cancel stops a download at once but lets a running brew, winget or privileged step finish, then skips the rest and sets cancelled=true. This is the reading of 'stops after the current step'.
- While the OS password window is up and while the installer runs, the step is one state, waiting-os (indeterminate). The app cannot tell the two apart, so the screen shows one combined message.
- Windows exit codes (winget classification) and per-user AWS CLI scope remain untested on Windows; the AWS CLI stays machine-wide per the task. On Windows too-old git or AWS CLI uses 'winget upgrade', missing uses 'winget install'.
- A broken or unreadable tool (status error, for example brew --version failing) does not block the gate, per the 'only missing or below minimum blocks' rule, so a Mac whose brew is broken but present passes. If the owner wants such a Homebrew to block, it is a one-line change in gate.go.

### deferred
- B1 to B3 stay manual on clean macOS VMs (real Homebrew.pkg install, real 'brew --version' output without git for the '>=' case, the real password window). Winget codes and the --scope machine behaviour are still unverified on Windows (B11).

### declined

### manualChecks
- B1 to B3 (blocking acceptance): clean macOS 15, 26 and 27 VMs without Command Line Tools. Run the real setup once: osascript password window, Homebrew.pkg install, then brew install git awscli shows only 'Pouring' lines and no 'install developer tools' dialog; /usr/bin/git --version does raise the dialog (the reason the stub rule exists). Confirm the real osascript exit-code transport and that stdout is lost on failure.
- Standard (non-admin) macOS account: the window asks for an administrator's name and password, /opt/homebrew ends up owned by the standard user, and brew install needs no sudo. Cancel once and give a wrong password once to see the real texts (expected -128 and -60005, mapped to cancelled and not_admin or auth_denied).
- Run the real privileged script as root once on a VM (or sudo it by hand with CODEPASS-style test arguments in verify mode): check bash 3.2 compatibility, `[[ -f /dev/fd/3 ]]` as root, and `head -c` on a 154 MB file under /var/root.
- pkgutil --check-signature and spctl with the network cut after the download (Homebrew.pkg is not stapled): see whether notarization verification fails offline or behind a proxy that blocks Apple, and decide whether the not_notarized result is acceptable there.
- Gatekeeper turned off on a VM: confirm `spctl --status` prints 'assessments disabled' and the script reports gatekeeper=off and still installs.
- Managed Mac (MDM profile restricting installers) for the real wording behind the policy classification; disk-full VM for the real installer text; behind a real authenticating proxy for proxy_auth.
- Windows 11 and Windows 10 1809: winget presence and the Store fallback, winget install of Git.Git and Amazon.AWSCLI with the agreement flags, real exit codes against the classification table, and check B11 (does `--scope user` work for the AWS CLI without UAC).

## F1-13

### notesForWiring
- Register `app.NewIdentityService(c, reg)` in main.go. It takes the same *projects.Store that the container passes as repos.Deps.Registry, so expose that store as `Container.Projects` in internal/app/container.go, where it is currently a local variable. Do not open a second store: its snapshots would go stale. The service builds its own identity.Engine from c.Paths, c.git, c.Backup, c.emit and c.files.note.
- Until the service is registered, wails3 generates no bindings for it. frontend/src/api/mock/identity.ts is typed against hand-written types (Author, IdentityOverview, RepoChangePlan, UndoToken and the rest) that mirror internal/api/identity.go exactly, and exports `IdentityServiceShape` (method names and argument order of internal/app/identityservice.go) and `mockIdentityService`. After registration and `wails3 generate bindings -clean=true -ts -i`, replace the type block with imports from ../models and type the mock as `typeof IdentityReal`. In services.ts add `export const Identity = useMocks ? mock.identity : IdentityReal`, and in mock/index.ts add `identity: mockIdentityService`. Scenarios: `resetIdentityMock('default' | 'unset' | 'env' | 'empty')`.
- Events: add `'identity:changed': Record<string, never>` to EventPayloads in frontend/src/api/events.ts. The service emits it after every write and undo (payload is an empty struct), and the Git settings page should refetch GetOverview on it. The mock does not emit it.
- Locale file `frontend/src/i18n/locales/identity.ts` belongs to F1-16, which owns the identity UI strings. The backend returns kinds and params only: source kinds none/global/system/project/from-project/repo/conditional/include/worktree/command/env, and actions set/remove/unchanged/keep-own/keep-external/skip-missing. `IdentitySource.rule`, `file`, `path` and `var` carry what the tags need ('From a conditional rule in ~/.gitconfig', 'From a terminal environment variable', 'Open that file').
- Terminal environment contract for milestone 2 (the code is in internal/identity/shellenv.go). The AI session terminal must run the person's own login shell as `<shell> -l -i` (identity.ShellArgs) with base environment identity.TerminalBaseEnv(Paths.Home), meaning execx.ChildEnv() plus HOME and USERPROFILE pinned to Paths.Home. The shell then re-sources the profiles, so GIT_AUTHOR_* exported there reach git in the session. `identity.ProbeShellEnv` starts that same shell the same way and reads back only identity.IdentityEnvNames. If milestone 2 changes the shell, its flags or its base environment, change those two functions (ShellArgs and TerminalBaseEnv) too, or the identity shown in the app stops matching what commits get. `Engine.TerminalEnv(ctx, refresh)` is reusable for the session bar and the commit composer. The probe result is cached for 15 s.
- Move-in and move-out: F1-14's project service should call `Engine.PreviewMove` / `ApplyMove` (service `PreviewMove` / `ApplyMove`) instead of `projects.Store.SetProject` when a repository changes project. ApplyMove writes the identity change and calls SetProject, and puts the config back if SetProject fails. A project-delete flow should run the move-out rule (`PreviewMove(repo, "")`) for each member. F1-18's `Plan` can warn about an includeIf rule pointing at an old path using the `conditional` source (`rule`, `ruleFile`, `path`) from `Overview`.
- The UI keeps the `UndoToken` returned by every apply and passes it to `Undo` for the toast. Undo refuses with E_CONFIG_CHANGED when a file changed since the app wrote it, and it takes a safety backup of each file first. The generic SystemService.RestoreBackup also works on these backups: kinds git-global and git-local, reason identity.
- `Overview` runs two git reads per visible repository (4 at a time) plus one shell probe (cached 15 s). Hidden repositories are left out of the table but still count as project members for apply.
- API change to bind: IdentityService.Undo now takes undoId string (was UndoToken) and IdentityChangeResult has undoId (string, '' = nothing to undo) instead of undo. The undo ID lives only in memory (32 most recent applies) and is gone after a restart: F1-16's toast must treat E_INVALID_INPUT(field undoId) as 'cannot undo any more'.
- Hand-written TS types in frontend/src/api/mock/identity.ts sit between the '(start)' and '(end)' markers and mirror internal/api/identity.go, with enums named as wails3 will (IdentitySrcNone, RepoActionSet, RepoModeOwn, ProbeNoShell ...) and slices as T[] | null. After F1-W registers the service and wails3 generates bindings, replace that block with imports from ../models and type the mock as typeof IdentityReal; the mock body and test already use enum members and import FilePreview/DiffLine/DiffKind/AppError from ../models.
- New enum values F1-16 must handle: RepoAction 'error' (RepoChangePlan.error = AppError E_GIT_FAILED or E_FILE_IO with params.file; the 'cannot write this repository's config' state with Retry), RepoIdentity.followsGlobal (follow-global marker; override is nil then), TerminalEnvOverride.committerVars / fallbackEmail (shown apart from active), probeError kind + probeExitCode (translate), RepoIdentity.readError as AppError.
- Terminal environment contract for milestone 2: use identity.TerminalEnv(home, proxyEnv) (exported, internal/identity/shellenv.go) as the AI terminal's base environment, start the shell with identity.ShellArgs (-l -i) and pass the same shell as Deps.Shell. Add task-specific variables on top, do not replace it. The probe runs in the home folder; a session runs in the task folder (only a folder-sensitive profile such as direnv could differ). Pass the netx proxy variables (netx.Config.ChildEnv()) as proxyEnv; the identity service currently passes none (Deps.Proxy is nil) because Container exposes no netx config accessor, so wiring may set Deps.Proxy in NewIdentityService if wanted.
- IdentityService now defines a Container method noteIdentityWrite (internal/app/identityservice.go) that wraps files.note for the global git config; F1-W needs nothing from it, but main.go still has to register NewIdentityService(c, reg) with the same projects.Store as repos.Deps.Registry.
- Overview runs three git reads per repository now through the read-only Reader: rev-parse --git-path config, config --list, var GIT_AUTHOR_IDENT (previous note said two).
- Swap step now: replace the block between the two markers with imports from ../models, and delete the CancellablePromise and AppError imports just under the start marker. Set IdentityServiceShape = typeof IdentityReal.

### deviations
- The existing internal/identity package was the AWS colour/monogram code (used by accounts and transfer). I added the author-identity engine as new files in the same package (engine.go, resolve.go, write.go, undo.go, shellenv.go) with non-colliding names and left identity.go untouched, because the entry names internal/identity/ as the owned path. A later cleanup could move the colour code to its own package.
- Worktree location: my first `git worktree add` used a relative path and created the worktree under aws_sso_hub/.codepass-wt/F1-13. I removed it with `git worktree remove --force` and `git branch -D task/F1-13` (nothing was committed on it), then recreated it at ~/Documents/code/.codepass-wt/F1-13 from codepass. The main checkout was not edited. The F1-11 and F1-20 worktrees show up in `git worktree list` under aws_sso_hub/.codepass-wt/, so the same mistake may have been made there. They also leave an untracked .codepass-wt/ folder in the main checkout.
- No new API error codes. internal/api/errors.go is not mine, so the engine reuses E_INVALID_INPUT (field name, email, mode, identity, projectId or repoId), E_CONFIG_CHANGED, E_FILE_IO (file param; this is the 'cannot write this repository's config' case), E_REPO_NOT_FOUND, E_BACKUP_NOT_FOUND and E_GIT_FAILED.
- Event: api.EventIdentityChanged = "identity:changed" is defined in internal/api/identity.go. The mock does not emit it, because mockEmit is typed by the shared EventPayloads map in frontend/src/api/events.ts (wiring file).
- Repository mode 'follow global' is refused with E_INVALID_INPUT (field mode) while the repository's project has its own identity. Git config cannot make a repository skip its project without freezing the global value into the repository's local config, and the UI should not offer it there.
- Project apply updates the project's own record in projects.json only when every repository write succeeded. After a partial failure the plan for a retry starts from the same record, so a retry plans only the remaining repositories. Per-repository errors come back in `outcomes`, and `Undo` still works for the files already written.
- The hint for 'open that file' is a text scan of the shell's startup files (zsh via ZDOTDIR, bash, fish). Whether a variable really reaches git is decided by running the shell; a plain assignment that is never exported is not reported as active. An `export` made in a file that a profile `source`s is detected by the shell run but may show no file.
- On Windows no profile is probed (PowerShell's $PROFILE is not run). The process environment is reported instead.

### deferred
- Windows has no shell probe, so ProbeShellEnv's Dir/sentinel path is exercised on macOS only (shell probe tests skip on Windows; the Windows code path is compile-checked with GOOS=windows build and vet).

### declined

### manualChecks
- On the Mac, with Codepass using the real home: add `export GIT_AUTHOR_EMAIL=...` to your own ~/.zshrc (or ~/.zprofile) and open the Git settings page. The terminal-variable warning and the 'open that file' hint should name that file, and the effective email in every repository row should be the exported one. Remove the line afterwards.
- With your real ~/.gitconfig containing an includeIf "gitdir:..." rule that sets user.email, check that a repository under that folder shows the conditional rule and the included file as its source, and one outside it shows global.
- Project apply on real repositories: save an identity for a project, check the preview list (repositories with their own or outside-the-app values listed last as kept), then check the affected repositories' .git/config and that Undo restores them byte for byte.
- Zsh frameworks (oh-my-zsh, powerlevel10k instant prompt) and slow profiles under `zsh -l -i -c`: confirm the probe still returns within its 5 s limit and that the page shows 'not probed' rather than hanging when a profile blocks.
- A repository set up as a worktree: confirm the identity is written to the shared repository config and the AI task worktree commits with it.
- Windows 11: global save writes the file git reports (`git config --global --list --show-origin`); the shell probe is skipped by design and only the process environment is reported.

## F1-20

### notesForWiring
- Windows blocker in internal/update/controller.go validate(): it returns a config refusal when Gate.Requirement == "". The Windows gate ignores Gate.BundleID and Gate.Requirement. F1-W must either pass a non-empty placeholder Requirement on Windows or make validate() require it on darwin only; otherwise every Windows update is refused with CodeConfig. I did not change controller.go because I do not own it.
- Staged payload on Windows: VerifyBundle expects a regular file ending in .exe (the codepass.exe from the release zip) at the path Wails hands over as DownloadedPath. If the VM check shows Wails stages a folder, only verifyExe in gate_windows.go needs to change.
- No new service, no go.mod change and no frontend change.

### deviations
- The worktree was first created inside the main checkout (aws_sso_hub/.codepass-wt/F1-20) because the relative path I gave git -C resolved there. I fixed it with git worktree move to ~/Documents/code/.codepass-wt/F1-20. No file in the main checkout was edited. Other tasks' worktrees (F1-11, F1-13) are also listed under aws_sso_hub/.codepass-wt/ and leave an untracked .codepass-wt/ folder in the main checkout; the orchestrator may want to move them.
- gate_other.go (which tasks.md says F1-20 changes) is edited only for its build tag and comment. Extra files beyond Owns: gate_other_test.go (runs on linux only) and gate_windows_test.go (compile-checked only on the Mac).

### deferred

### declined

### manualChecks
- B9 on the Windows 11 VM: rc.1 updates to rc.2 through the test channel (swap, relaunch, rc.2 shown). The release exe's file version must be 1.0.0.2 for 1.0.0-rc.2, which depends on build/windows/info.json.
- B9: an exe whose file version differs from the manifest is refused, the installed exe is unchanged and no wails-update-* folder is left.
- B9: a locked exe (the app still running during the swap) is handled. This is the helper's behaviour, not the gate's.
- Confirm that the wails3 Windows build really embeds the VS_VERSION_INFO from info.json (fixed file version fields) and that the gate reads the same value there. Run the gate on the real built exe once.
