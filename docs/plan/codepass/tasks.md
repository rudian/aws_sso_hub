# Codepass v1.0.0 - task deployment plan

Date: 2026-11-16 (revision 3, after the milestone-0 gate). Author: planner (Fable). Reads with `product-plan.md` (what and why; "plan N.N" below) and `ui-design.md` (screens and copy; "design N.N"), the mockup in `docs/design/codepass-mockup/` (shots in its `shots/` folder), the spike notes and the two reviews in `docs/research/`, and `manual-checks.md` (what the owner still has to try by hand; "check A3", "check B5").

Revision 3 applies the seven spike notes as corrected by `spike-review-a.md` and `spike-review-b.md`, and turns the owner's requirements of 2026-11-16 into tasks: F1-18, F1-19, F1-20, A2-13, A2-14, A2-15, C3-10, C3-11, C3-12, C3-13.

Revision 4 (2026-11-16, planner: Opus; research and design: Fable) turns the owner's second batch into tasks: F1-21 (merge `main`, AWS SSO Hub 0.0.2), F1-22 (system notifications; sign-in expiry), E5-11 (PHP code intelligence), A2-16 (open a repository from any screen; with F1-CL's Projects search by folder name) and **milestone 8** (ticket queue, AI accounts and usage for Claude Code and Codex: T8-01 to T8-09 and T8-W). Milestone 8 runs after milestone 3, so the run order is 0, 1, 2, 3, 8, 4, 5, 6, 7. A2-04 and A2-05 gained what milestone 8 needs from them. Research notes: `docs/research/notifications.md`, `docs/research/claude-usage-accounts.md`.

Revision 5 (2026-11-17, planner: Opus; design: Fable) adds the milestone 1 gate fixes (F1-GF, group I) and turns the owner's third batch (git) into tasks: a new branch from any branch the owner chooses (as on the Bitbucket website), the counts of commits not pushed and not pulled with how fresh they are, and "拉取全部 / Pull all" (every local branch of every repository brought up to date where that needs no decision). New tasks G4-11, G4-12 and G4-13; G4-01, G4-02, G4-06 and G4-07 grow. Plan 4.6a; design 4.2, 5.2, 5.2b and 5.8; the git facts behind it are in `docs/research/git-pull-all.md`. The owner's fourth batch (2026-11-17) adds Pull all after an AWS sign-in: new task G4-14 in group D of milestone 4; plan 4.6a part 4, design 5.2b "Automatic run" and 5.14.

## How the work runs

**Models.** Sonnet implements every task. Opus reviews every task before it merges, using the task's "Review" line plus the standing checklist below. Fable is called only where a task says "Fable first" (a screen the mockup does not draw) or when a reviewer finds a design gap.

**Branch.** Everything lands on `codepass`, pushed after every merged task. Nothing merges into `main` and nothing is published before milestone 7. Each task runs in its own git worktree branched from `codepass`; after the Opus review passes, the orchestrator merges it into `codepass` and pushes. There is no GitHub pull request per task.

**Parallel groups.** Inside a milestone, groups run in the order given. Tasks inside one group run at the same time and never own the same file. Shared files (`main.go`, `internal/app/container.go`, `internal/app/events.go`, `frontend/src/api/services.ts`, `frontend/src/api/mock/index.ts`, `frontend/src/App.tsx`, `frontend/src/shell/Sidebar.tsx`, `frontend/src/i18n/index.ts`, `frontend/src/styles/*.css`, `go.mod`, `frontend/package.json`) are owned by exactly one task per group, usually the **wiring task** (id ends in `W`). A backend task delivers its package, its service file in `internal/app/`, its API types in `internal/api/`, its mock in `frontend/src/api/mock/` and its locale file in `frontend/src/i18n/locales/`; the wiring task registers them. A task that needs a new dependency names it in its pull request, and the wiring task adds it.

**Dependencies.** A task depends on every task in the earlier groups of its milestone and on the previous milestone's gate. A "Deps" line names anything beyond that, such as a spike. Milestones run in the order 0, 1, 2, 3, 8, 4, 5, 6, 7 (milestone 8 was added on 2026-11-16 and runs after 3); "previous milestone" follows that order.

**Task fields.** Build (what to make), Owns (files and packages), Deps, Accept (acceptance criteria), Tests (tests to add), Review (focus for the Opus reviewer), and the size in brackets after the title. Short tasks carry the same fields on one line.

**Sizes.** S = up to 2 days, M = about 1 week, L = about 2 weeks, including review rounds. They are estimates.

**Standing review checklist (Opus, every task).**

1. No real names: only acme / globex / initech / umbrella, `111122223333`-style IDs, `@example.com`. The scrub check passes.
2. Commands are argument lists. No value is spliced into shell text.
3. Automatic git reads use `--no-optional-locks -c core.fsmonitor=false` and `--no-ext-diff --no-textconv`.
4. Text from other people is never markup. `dangerouslySetInnerHTML` count stays zero.
5. Nothing secret or noisy reaches the log: tokens, keys, terminal output, language-server traffic, hook payloads.
6. Config writes are backup-first; backups, exports and logs never hold private keys or tokens.
7. A typed mock exists for every new service; both languages have every new string; reduced motion is honoured.
8. The task builds for Windows (`GOOS=windows GOARCH=amd64 go build ./...`).
9. A child process that runs elevated or privileged gets an explicit, minimal environment and absolute tool paths. Nothing is inherited (S0-04 review, issue 1).
10. On macOS no developer-tool stub (`/usr/bin/git`, `make`, `clang`, `python3`, `xcrun`) runs before `xcode-select -p` prints an existing path other than `/`. Otherwise macOS opens its "install developer tools" dialog.
11. Anything deleted from disk is listed first with its exact path, belongs to one known record (a task ID or a session ID), and is never reached through a pattern that could match another session's files.

**Gate at the end of every milestone** (run by the orchestrator; all must pass before the next milestone starts):

```
go vet ./... && go test ./...
GOOS=windows GOARCH=amd64 go build ./...
cd frontend && npm run typecheck && npm test && npm run build && npm run check:dist
scripts/scrub-check.sh                      (from F1-07)
```

Plus: the screenshots listed for the milestone, taken from the mock scenarios at 1440x900 (light zh-CN and dark en) and compared side by side with `docs/design/codepass-mockup/shots/<name>.png`; a signed macOS build installed on the owner's Mac through the test channel; a Windows build smoke-tested on a Windows 11 VM (launch, the milestone's main flow once). A Windows 11 on Arm VM on the owner's Mac runs the amd64 build under emulation and is enough for this; Windows 10 1809 x64 needs an Intel PC or a cloud Windows Server 2019 instance (same build, 17763).

---

## Milestone 0 - Spikes (2 weeks) - FINISHED 2026-11-16

**Goal.** Prove or disprove the assumptions the plan rests on, with throwaway code.
**User-visible result.** None. Each spike ended in a findings note (`docs/research/spike-<name>.md`) and its code stays on a local `spike/*` branch that is never merged and never pushed. Two Opus reviews checked the notes against the raw output: `spike-review-a.md` (S0-01 to S0-03) and `spike-review-b.md` (S0-04 to S0-07). Where a review disagrees with a note, this plan follows the review.

| Spike | Result | Note | What is still open (all in `manual-checks.md`) |
|---|---|---|---|
| S0-01 Terminal | **Partly verified** | `spike-terminal.md` | Chinese input, Shift+Enter, Codex, a long real session after trimming (A1-A4); everything on Windows (B8) |
| S0-02 Update | **Done for the macOS trust logic** | `spike-update.md` | Notarized rc1 to rc2 on a clean Mac, the refusals there, the crash window (B5-B7); private test channel and slow download (A16, A17); Windows (B9) |
| S0-03 Signals | **Partly verified** | `spike-signals.md` | Interactive Claude Code, `--session-id`, OpenCode with a working sign-in, Codex (A5-A8); Windows (B10) |
| S0-04 Installers | **Partly verified; the acceptance run has not happened** | `spike-installers.md` | Clean macOS 15 / 26 / 27 VMs without Command Line Tools, a standard account, Gatekeeper off, managed Mac, disk full, proxy (B1-B3, B13, B14); what root's environment inherits (A15); winget (B11) |
| S0-05 Signing | **Done** | `spike-signing.md` | Launch of the hardened build (A9), clean-Mac double-click (B4), `.p12` backup (A10) |
| S0-06 Language servers | **Done on macOS** | `spike-lsp.md` | Windows install path and orphan check (B12); memory on the owner's largest repositories (A13) |
| S0-07 Provider tokens | **Partly verified (GitHub reads only)** | `spike-providers.md` | GitHub write actions and organisation policy (A11, A12), every Bitbucket fact (A14) |

**Group A (all parallel).**

**S0-01 · Terminal in the webview** (M) - partly verified
- Build: PTY (creack/pty) running `claude`, `opencode` and `vim` in xterm.js inside WKWebView, with the pull-by-offset data path (plan 5). Codex was not installed and WebView2 was not run.
- Owns: `spike/terminal`.
- Proven: the reader never waits on the page (50 MB with no reader at all: child wall time 0.2 s; the rejected design stalls at 512 KB); resize reflows; a reload replays a full log to the identical screen for vim, opencode and the claude first-run screen, provided the page drops xterm's own replies while replaying.
- Not proven: a long hide (the in-app flood ran mostly while the window was still visible; App Nap starts after minutes); replay after trimming for anything but vim (opencode showed garbage once, because the log was cut inside an escape sequence); Chinese input; Shift+Enter in a signed-in `claude`; Codex; Windows.
- Decision: Windows ConPTY is the app's own thin wrapper over `golang.org/x/sys/windows`, derived from go-pty's MIT code, with `PROC_THREAD_ATTRIBUTE_JOB_LIST` and `KILL_ON_JOB_CLOSE`. go-pty is not imported. The wrapper (`spike/terminal/winpty/winpty_windows.go`) compiles and has never run.
- Carried into: A2-01, A2-02, A2-07, F1-03 (the app quit when its window was hidden until `ApplicationShouldTerminateAfterLastWindowClosed` was false).

**S0-02 · Signed-manifest update** (M) - done for macOS
- Build: the manifest provider and checks of plan 4.14 against a fake GitHub releases API and a test key; 17 scenarios, 118 assertions, all pass.
- Owns: `spike/update`.
- Proven: a re-published old archive, an ad-hoc bundle, a tampered manifest, a wrong bundle ID, a version mismatch and a changed resource are each refused with the installed app byte-identical; a good update swaps and relaunches; `HandleHelperMode` is the first statement of `main` (AST test). Wails' own signature check lets an ad-hoc bundle replace a Developer ID one, and it runs after the swap, so every trust decision is the app's own and runs on the staged copy.
- Not proven: a notarized build, a clean VM, the real GitHub API, a private test repository, a slow download, the swap's crash window, anything on Windows (the Windows gate does not exist yet: F1-20).
- Carried into: F1-09, F1-10, F1-20, F1-W.

**S0-03 · AI tool signals** (M) - partly verified
- Build: Claude Code hooks, an OpenCode plugin through `OPENCODE_CONFIG_CONTENT`, the process-tree probe. Codex was not installed (documentation only).
- Owns: `spike/signals`.
- Proven: six Claude Code events arrive with `session_id`; two `--settings` flags do not merge (the last one wins); hooks from a session-only plugin (`--plugin-dir`, needs `.claude-plugin/plugin.json` and `hooks/hooks.json`) add to whichever settings win; `--resume` keeps the session ID; OpenCode loads the plugin under `--standalone` and `--session ses_...` creates a session with exactly that ID.
- Not proven: the plugin's `Stop` hook, `Notification` and `PermissionRequest`, `--session-id`, the interactive screens, OpenCode success and permission events, everything about Codex, Windows.
- Found by the review: without `--standalone` OpenCode starts a shared background service that outlives the task and runs outside its process tree, so `--standalone` is needed for process control as well. `opencode auth list` starts that service; `opencode --version` does not. With the plan's launch form the shell exits together with the tool, so without hooks a task can only be "running (no signal)" until it exits.
- Carried into: A2-01 (kill by terminal session, not by process group), A2-04, A2-05, A2-08.

**S0-04 · Tool installers on clean machines** (M) - partly verified, acceptance run open
- Build: Homebrew.pkg through one privileged script (copy to a root-only folder, verify there, install); brew behind a proxy; winget manifests read. "Homebrew into a folder the user owns" was tried and is **disproven**: a prefix longer than 13 bytes is Tier 3 and compiles git, openssl and python from source, which needs Command Line Tools and so an administrator.
- Owns: `spike/installers`.
- Proven: package facts (7.0.13, Team ID `927JGANW46`, notarized, not stapled); damaged, re-wrapped, wrong-team, truncated and HTML downloads all stop before `installer`; the `osascript` transport (no streaming, stdout lost on failure, exit code in the trailing parentheses); all 27 formulae behind git + awscli have bottles for macOS 15, 26 and 27; the brew names and winget IDs.
- Not proven: the script has never run as root; **no clean VM run happened**, so "git and awscli install with no developer-tools dialog" is not met; standard account; Gatekeeper off; MDM; disk full; Windows.
- Found by the review, to fix in F1-12: the environment is a second way to change what root runs (`BASH_ENV`, exported functions, `SHELLOPTS` / `PS4`); installer text is lost on failure; the copy is unbounded and accepts any file type; `spctl` answers differently with Gatekeeper off; the digest is not checked. `open -W` is not built.
- Carried into: F1-12, F1-15. F1-12 cannot be accepted before checks B1 to B3 pass.

**S0-05 · Signing and notarization** (S) - done
- Build: `spike/signing/sign-notarize.sh`: build arm64, re-sign with hardened runtime and a secure timestamp (executable, then bundle), notarize a zip, staple, verify. About 69 s end to end; accepted with no entitlements.
- Owns: `spike/signing`.
- Recorded: Team ID `XC34P8TNZ6`; the designated requirement; the updater uses the full designated requirement, because the short string is also met by the team's Apple Development and Apple Distribution certificates (both are in the keychain).
- Open: a launch of the hardened build (the single-instance guard handed off to the installed app; "Open in terminal" probably needs the Apple Events entitlement); the clean-Mac double-click; the `.p12` backup by the owner.
- Carried into: F1-01, F1-09, F1-10.

**S0-06 · Language-server transport** (S) - done on macOS
- Build: gopls and two TypeScript servers as child processes, `@codemirror/lsp-client` 6.3.0 in a page, messages pulled by offset.
- Owns: `spike/lsp`.
- Proven: hover, completion and definition on a 2,003-file Go workspace and on this repository's `frontend/`; a hidden or frozen page does not stall the server, and the naive event path stalls gopls for 37 s. typescript-language-server 6.1.0 does not start in a TypeScript 7 workspace; the TypeScript 7 native server (`tsc --lsp -stdio`) works at about 290 MB instead of 930 MB but reports diagnostics by pull only. lsp-client renders server Markdown through `innerHTML`.
- Open: Windows install path and orphan check; the real Wails transport; a 2,000-file TypeScript repository.
- Carried into: E5-03, E5-09.

**S0-07 · Provider tokens** (S) - partly verified
- Build: read-only probes against GitHub with the owner's signed-in `gh`; Bitbucket from documentation and anonymous calls.
- Owns: `spike/providers`.
- Proven: one classic token with `repo` + `read:org` reaches every GitHub endpoint the app reads; a 304 on the pull request list and detail costs no quota; a missing classic scope is a 404 whose only clue is the two scope headers.
- Not proven: every write action; every Bitbucket scope and all authenticated Bitbucket behaviour.
- Corrected by the review: Bitbucket still has `GET /user/workspaces` and `GET /workspaces/{workspace}/pullrequests/{selected_user}`, so workspaces come from the token and "my pull requests" is one call per workspace. A GitHub organisation can block classic tokens.
- Carried into: P6-03, P6-04, P6-07, P6-08. Checks A11, A12 and A14 come before P6-03 and P6-04 are signed off.

**Gate (run 2026-11-16).** Seven findings notes and two reviews exist. `product-plan.md`, `ui-design.md` and this file were updated where a finding contradicted them. The checks nobody could run are in `manual-checks.md`, each with the task it blocks. No code gate.

---

## Milestone 1 - Foundation (6 weeks)

**Goal.** The app is Codepass, updates itself safely, opens only when its tools are present, lists any git repository in projects, and sets the author identity at three levels.
**User-visible result.** A notarized Codepass that replaces AWS SSO Hub on the owner's Mac: Projects page, Git settings with identity, the setup screen on a fresh machine, and one-click updates from the test channel.

**Group A (serial, alone: it touches every file).**

**F1-01 · Rename to Codepass** (M)
- Build: Go module path `github.com/rudian/codepass`; display name, window title, app and exe file names; arm64-only macOS build with minimum 15.0 (remove the universal tasks); new icon from `docs/design/codepass-mockup/appicon.svg`. BundleID (`com.rudian.awsssohub`), data folder name and managed git-config markers stay. `build/darwin/Info.plist` gains `NSAppleEventsUsageDescription` in both languages ("Open in terminal" drives Terminal and iTerm through Apple Events); the entitlement `com.apple.security.automation.apple-events` is added only if check A9 shows it is needed. The ad-hoc signing step stays for development builds; release builds are re-signed by F1-10.
- Owns: all import paths, `build/`, `internal/version`, `Taskfile.yml`, `frontend/package.json` name.
- Deps: none.
- Accept: all existing tests pass unchanged in behaviour; the built app is `Codepass.app` (arm64) and `codepass.exe`; existing bindings in a test repository are still recognised.
- Tests: a test that pins BundleID, `paths.AppDirName` and the marker strings.
- Review: nothing user-visible still says "AWS SSO Hub" except migration text; nothing that must not change changed (the BundleID is part of the updater's code requirement and of the single-instance ID).

**Group B (parallel).**

**F1-02 · Versioned data files** (M)
- Build: `internal/store`: atomic JSON files with `schemaVersion`, migration hooks, backup before first write of a new version, refusal to load a newer version. `internal/projects`: `projects.json` (plan 3.1, 3.2) with projects, repository records, hidden flag, integrated commits. A repository record has a random `id` that is never derived from the path, the current `path`, and `legacyId` (the old hash of the path, so 0.0.1 data can be matched). F1-18 builds the registry on top.
- Owns: `internal/store/`, `internal/projects/`.
- Deps: F1-01.
- Accept: `settings.json` is untouched by the new stores; running the 0.0.1 binary against the folder leaves `projects.json` intact; a file with a newer version is reported, not overwritten.
- Tests: round trip, crash between temp write and rename, newer-version refusal, concurrent writers.
- Review: atomicity on Windows (rename over an open file); no new data in the settings struct.

**F1-03 · App menu, window behaviour, old-app handoff** (M)
- Build: custom menu (design 4.4): Edit items kept, Cmd+W and Cmd+R routed to the frontend, no Help web item; close hides the window while tasks run (a `tasksRunning()` hook that returns false for now); `ApplicationShouldTerminateAfterLastWindowClosed` is **false** whenever closing or "keep running in the background" hides the window (today `main.go` sets it to true, and S0-01 saw the app quit at `Hide()`); the quit flow asks a `quitDecision()` hook that answers quit / stay in the background / cancel (it answers "quit" until A2-14 supplies the dialog); logout, restart and shutdown stop everything and quit without a dialog (Wails 3.1.0 has no `NSTerminateLater`; decided at the group-B merge); `updater.HandleHelperMode()` as the first statement of `main` (the one-line change and its AST test exist on `spike/update`); detection of a running AWS SSO Hub before the single-instance handoff, with "quit it" and "move it to the Trash" (Windows: the portable 0.0.1 exe goes to the Recycle Bin).
- Owns: `main.go`, `internal/app/menu.go`, `internal/app/window.go`, `internal/appguard/`, `frontend/src/shell/AppCommands.tsx`, locale `menu.ts`.
- Deps: F1-01.
- Accept: Cmd+R does not reload; Cmd+W never closes the window while an editor tab hook reports a tab; copy, paste and undo work in inputs; with the old app running, Codepass shows the quit prompt instead of exiting silently.
- Tests: menu structure snapshot; old-app detection with a fake process lister; quit flow with a fake clock for each of the three answers; hiding the last window does not end the app; the AST test that `HandleHelperMode` is the first statement.
- Review: nothing runs before the helper-mode call; logout is never blocked.

**F1-04 · Read-only git layer** (M)
- Build: `internal/gitx/read.go`: one runner for automatic reads that always adds the read-only flags and refuses mutating subcommands; status (porcelain v2), branch, ahead / behind, change counts, `config --show-origin --show-scope`, `git var`.
- Owns: `internal/gitx/read.go`, `internal/gitx/read_test.go`, `internal/gitx/status.go`.
- Deps: F1-01.
- Accept: `git status` through the runner takes no `index.lock` (asserted while another process holds the index); a repository with `core.fsmonitor` and a diff driver set to a marker script never runs them.
- Tests: the two above as integration tests with real git; porcelain parser table tests.
- Review: is there any path for an automatic read that bypasses the runner?

**F1-05 · File watcher** (M)
- Build: `internal/watch`: recursive watching with FSEvents (macOS, cgo) and `ReadDirectoryChangesW` (Windows); ignored folders skipped; 150 ms debounce; a subscribe API per folder; `Pause(folder)` / `Resume(folder)` that close and reopen every handle on that folder (a folder cannot be renamed on Windows while a handle is open: C3-10).
- Owns: `internal/watch/`.
- Deps: F1-01.
- Accept: 10,000 file writes in a second produce at most a handful of events; `node_modules` writes produce none; watching 30 repositories uses a bounded number of file handles.
- Tests: burst, ignore, rename, folder removal, on both platforms (Windows in the VM gate).
- Review: handle leaks on unsubscribe and on pause; behaviour when the folder disappears.

**F1-06 · Network and proxy** (S)
- Build: `internal/netx`: system proxy (`scutil --proxy`, WinHTTP), one HTTP client with timeouts and an ETag cache, and `ChildEnv()` that returns the proxy variables for child processes.
- Owns: `internal/netx/`.
- Deps: F1-01.
- Accept: with a proxy set in system settings, requests go through it; `NO_PROXY` is honoured.
- Tests: parser tests for both platforms' outputs; client against a local proxy.
- Review: proxy credentials never logged.

**F1-07 · Scrub check and generic fixtures** (S)
- Build: `scripts/scrub-check.sh` (block list of real names and ID shapes; allows the product strings Claude Code, `claude`, Codex, OpenCode, the two public Team IDs the code must pin (`XC34P8TNZ6` for Codepass, `927JGANW46` for Homebrew) and the BundleID; still blocks AI attribution lines and any home-folder path); rename every mock scenario, fixture and test name to the generic set in design 7.
- Owns: `scripts/scrub-check.sh`, `frontend/src/api/mock/scenarios.ts`, `frontend/src/api/mock/world.ts`, existing fixtures.
- Deps: F1-01.
- Accept: the check fails on a planted real name and passes on the tree; the block list itself is kept outside the repository (path from an environment variable) so the list does not leak the names.
- Tests: the script's own cases.
- Review: the list's location; screenshots in `docs/` re-taken or removed.

**F1-08 · Motion and style foundation** (M)
- Build: the tokens and class families of design 2 and 3 that are not screen-specific: motion durations and easings, list insert / remove / FLIP helpers, sheet, bottom panel, `.suggest`, `.splitbtn`, `.src-tag`, `.prov`, `.tool`; Settings > Appearance > Motion (System / Full / Reduced) replacing the old switch.
- Owns: `frontend/src/styles/tokens.css`, `frontend/src/styles/components.css`, `frontend/src/styles/motion.css`, `frontend/src/lib/motion.ts`, `frontend/src/components/Sheet.tsx`, `frontend/src/components/ProviderMark.tsx`, `frontend/src/components/ToolMark.tsx`.
- Deps: F1-01.
- Accept: with Reduced, every transition is about 1 ms; lists over 200 rows skip motion; the comment colour is `#687180` / `#838D9C`.
- Tests: component tests for Sheet and motion helpers; a contrast test over the syntax tokens.
- Review: only `opacity`, `transform` and grid rows animate.

**F1-09 · Update verification** (L)
- Build: port `internal/update` from `spike/update`. The provider wraps the Wails GitHub provider for discovery and streaming only; version, file name, size, sha256 and notes come only from the verified manifest. Two pinned keys live in the app's provider; `updater.Config.PublicKey` stays empty and the manifest's sha256 is handed to Wails as the digest. Version rules with the stored highest version (an unreadable or invalid file fails closed with a visible message). The bundle gate runs **on the staged copy before `Restart` and again inside `Restart`**, never after the swap: bundle ID, bundle version equals manifest version, `/usr/bin/codesign --verify --deep --strict -R=<the full designated requirement of plan 4.14>`, then `/usr/sbin/spctl -a -t exec` as a second check that is never the only one. Tools are called by absolute path. A fresh `Updater` per check with the app's own timer (Wails' loop skips a refused update for ever); a production HTTP client with dial, TLS and header timeouts but no overall timeout (the Wails default cuts a download off after 30 s); translocation and write-permission detection before the download; old Wails helper logs in `$TMPDIR` removed at start; states for the UI. The test channel is a private repository: its read-only fine-grained token for that one repository is entered once and kept in the Keychain, never compiled in; the production channel needs none. The configured repository name is the final one (`rudian/codepass`), because asset URLs are accepted only under that name.
- Owns: `internal/update/` (not `gate_windows.go`: F1-20).
- Deps: F1-01, S0-02.
- Accept: the refusal cases of S0-02 (R1 to R9, G3) as automated tests; a zip with AppleDouble `._*` entries is refused; release notes come only from the manifest; every refusal removes the staging folder and leaves the installed app byte-identical.
- Tests: tampered manifest, wrong key, second key accepted, old version under a new tag, version below the stored highest, ad-hoc over Developer ID, an Apple Development certificate of the same team, wrong bundle ID, bundle version below the manifest's, changed sealed resource, install folder not writable, API 500, an unsigned release body is never shown, `rc.10` sorts above `rc.2`. `spike/update/run.sh` becomes the macOS integration test (it needs a Developer ID identity, so it runs on the owner's Mac, not in CI); its hostile bundle uses the neutral ID `com.example.other`.
- Review: fail-closed on every error; nothing fetched outside the manifest is trusted; the gate runs before `Restart`, never after; `spctl` is never the sole gate; the second key is really usable for rotation.

**F1-10 · Release pipeline and test channel** (M)
- Build: `build/release/`, starting from `spike/signing/sign-notarize.sh`: build; re-sign with `--force --options runtime --timestamp`, the executable first and then the bundle (`--deep` only to verify); notarize a `ditto` zip with a `--wait` timeout; fail on any status other than `Accepted` and keep the `notarytool log`; staple; build the release archive **from the stapled app** with `ditto -c -k --keepParent --norsrc --noextattr --noqtn --noacl` (a plain `ditto` zip carries `._*` entries that break the signature after extraction); extract the archive and run the same requirement check before upload; write and sign the manifest; upload with `gh` to a given repository. The signing identity, the notary profile name and tool paths come from the environment or a git-ignored local file, never from the repository. A documented key ceremony (generate the two ed25519 keys, where the private key lives); the test channel repository; `docs/release.md` also documents the manual recovery after a crash in the middle of a swap (`Codepass.app.bak` back to `Codepass.app`).
- Owns: `build/release/`, `docs/release.md`.
- Deps: F1-01, S0-05.
- Accept: one command on the owner's Mac produces a release that F1-09 accepts; no secret is read from the repository or from CI; release-candidate tags are `v1.0.0-rc.N` with a dot before the number; a launch smoke test of the signed build passes **with every installed copy quit** (single-instance handoff hides a launch failure): window, browser sign-in, keychain read, "Open in terminal".
- Tests: a dry-run mode with a test key that CI can run.
- Review: the scripts fail loudly if a step is skipped; nothing uploads on a dirty tree; no home-folder path or identity name in the scripts.

Group A merged 2026-11-16 (`f868498`). Group B merged 2026-11-16 (`9302a4c`..`64b1b64`, gate passed); follow-ups FU-02a to FU-10a are listed after "Owner decisions". Group B2 merged 2026-11-16 (`f4176d7`, gate passed); follow-ups FU-18a to FU-18h. Group C merged 2026-11-16 (`82ea29e`..`35d16a0`, gate passed); follow-ups FU-11a to FU-20d. Group D merged 2026-11-16 (`7ede90e`, `e4a4b93`, gate passed); follow-ups FU-Wa to FU-We; wiring notes for group E in `reviews/groupD-wiring.md`. Before group E the orchestrator added `fd36f8b` (test setup: fixes an intermittent unhandled error) and `821cf55` (the `RepoName` stub and the Projects clone-modal entry point). Group E merged 2026-11-16 (`1447a36`..`dc4a213`, gate passed: 56 test files, 749 tests); follow-ups FU-14a to FU-19b; notes in `reviews/groupE-wiring.md`. **Work paused here at the owner's request (2026-11-16).** The owner's second batch of requirements (same day) added F1-21 (carry over AWS SSO Hub 0.0.2 from `main`), F1-22 (system notifications; sign-in expiry notices) and milestone 8 (ticket queue, several Claude accounts, usage meter; it runs after milestone 3). Group F merged 2026-11-16 (`2c24815`: F1-21, a `--no-ff` merge of `main`, so `git log codepass..main` is empty; gate passed: 56 test files, 755 tests); follow-ups FU-21a to FU-21c; notes in `reviews/groupF-wiring.md`. Group G merged 2026-11-17 (`f3fc3e2`: F1-22, gate passed: 60 test files, 782 tests); follow-ups FU-22a to FU-22g; notes in `reviews/groupG-wiring.md`. Group H merged 2026-11-17 (F1-CL in two parallel parts: `0ab5df7` F1-CLa, `86e82d4` F1-CLb; gate passed: 63 test files, 841 tests); follow-ups FU-CLa1 to FU-CLa4 and FU-CLb1 to FU-CLb7 (all A2-CL); notes in `reviews/groupH-wiring.md`. **Every milestone 1 task is merged.** The milestone 1 gate review ran 2026-11-17 (37 screenshots; owner decisions in `reviews/m1-gate-review.md`). Group I merged 2026-11-17 (`4a7e6b5` F1-GF, the 19 gate fixes; `90f1772` A2-01, PTY and process control; gate passed: 64 test files, 863 tests); follow-ups FU-GFa to FU-GFd and FU-A2-01a to FU-A2-01c; notes in `reviews/groupI-wiring.md`. Owner, 2026-11-17: milestone 2 goes on; the gate's test channel and manual checks follow when the owner has time. Milestone 2 group B merged 2026-11-17 (`1d3153b` A2-02, `ebb8783` A2-03, `a157d4c` A2-04, `cfbad11` A2-05, `4be855f` A2-13, `9048069` F1-GF2, `eb361cf` A2-06, `7f53899` A2-07; gate passed: 80 test files, 996 tests; bindings not regenerated until A2-W); 37 follow-ups FU-A2-02a to FU-A2-07f and FU-GF2a, FU-GF2b; notes in `reviews/m2B-wiring.md`. **Next: milestone 2 group C: A2-08 beside F1-SG, then A2-W alone. Handoff: `docs/HANDOFF.md` in the repository.**

**Group B2 (serial, after B).**

**F1-18 · Stable repository IDs** (M)
- Why: today a repository's ID is a hash of its path (`internal/repos` `repoID(path)`), so moving or renaming the folder turns it into a different repository and drops its project, binding record, test result and, later, its tasks. The owner wants to rename the local folder inside the app (C3-10), so the ID must stop being the path before anything else stores it.
- Build: a registry in `internal/projects`: `IDForPath(path)` (creates the record with a random ID the first time a path is seen), `PathOf(id)`, `Relocate(id, newPath)` (atomic, backup-first), and `ByLegacyID(hash)`. `internal/repos` asks the registry instead of calling `repoID(path)` in the engine, add and clone paths; `repoID` stays only to compute `legacyId`. One-time import: the connection test results that `settings.json` keeps under the old path hash are copied into the repository records, and `settings.json` is left as it is for the old app. A repository that disappears from disk keeps its record for 30 days, so a folder moved outside the app can be re-attached: when a scan finds a repository whose `.git` identity (first commit and origin URL) matches exactly one missing record, the row offers "这是移动过来的 acme_api 吗？ / Is this acme_api, moved here?".
- Owns: `internal/projects/registry.go`, `internal/projects/registry_test.go`, the `repoID` call sites in `internal/repos/` (engine, add, clone); also `clone()` in `internal/projects/projects.go` for follow-up FU-02a (deep-copy `MissingSince` before filling it). F1-02 already added `MissingSince`, `FirstCommit` and `OriginURL` to `RepoRecord`.
- Deps: F1-02.
- Accept: the existing repository tests pass unchanged in behaviour; renaming a repository's folder by hand and re-attaching it keeps its project, hidden flag and identity override; two folders with the same name in different parents are two repositories; the 0.0.1 binary run against the data folder still starts.
- Tests: ID stays the same across `Relocate`; legacy lookup; import of old test results; concurrent `IDForPath` on the same path returns one ID; a path that differs only in case on macOS and Windows resolves to one record.
- Review: is any ID still computed from a path outside `legacyId`? Nothing new is written into `settings.json`.

**Group C (parallel, after B).**

**F1-11 · Repository engine for any git repository** (L)
- Build: provider strategies behind one interface in `internal/repos` (CodeCommit strategy = today's behaviour); list any git repository; provider from the remote; status through F1-04 and refresh through F1-05; hidden repositories; "newly found" set after upgrade and after scans; project suggestions (prefix, parent folder); move between projects. Every repository view carries two names: `localName` (the folder's name) and `remoteName` (the last path segment of the origin URL without `.git`; empty when there is no remote). Clone takes an optional folder name per repository, also in multi-clone: validated (no path separator, not `.` or `..`, no name that already exists in the destination, no reserved Windows name), and passed to `git clone` as the target directory argument. Expose F1-18's moved-folder question to the frontend: a field on `api.Repo` (for example `movedFrom {oldId, name}`) filled from `Engine.MovedOffers()`, and service methods for `ReattachMoved(newId)` and `DeclineMoved(newId)` (F1-14 draws the question); a `warnings` field on `api.ReposState` (FU-18c).
- Owns: `internal/repos/`, `internal/api/repos.go`, `internal/app/reposervice.go`, mock `repos.ts`; also `internal/gitx/status.go` for follow-up FU-04b and `movedFrom` in `internal/projects/registry.go` for FU-18a.
- Deps: F1-02, F1-04, F1-05, F1-18.
- Accept: the existing repository tests pass unchanged; a GitHub and a Bitbucket repository are listed as "other binding: machine default"; after upgrade the 13 known repositories stay and new ones wait in the review set; cloning remote `acme_api` as `api-main` gives a repository whose `localName` is `api-main` and `remoteName` is `acme_api`.
- Tests: strategy table tests; upgrade scenario; hidden repositories excluded from background work; folder-name validation table; two clones of one remote under different names are two repositories.
- Review: CodeCommit behaviour byte-for-byte; background fetch only for bound or opened repositories.

**F1-12 · Tools gate and installers** (L)
- Build: `internal/envcheck`: local-only detection with timeouts and the hard minimums. Tools are found by file path (`/opt/homebrew/bin`, `/usr/local/bin`, the Windows install paths); checklist item 10 applies, and the existing rule in `internal/execx.FindGit` is tightened to "`xcode-select -p` prints an existing path other than `/`" and reused for every stub. Pre-flight facts for the setup screen: administrator or not (`id -Gn`), device management (`profiles status -type enrollment`), free space (about 1.2 GB), the system proxy. `internal/toolinstall`, starting from `spike/installers/homebrew-pkg-install.sh` and `spike/installers/classify`:
  - Download `Homebrew.pkg`; size and sha256 come from the GitHub API (fallback when the API is rate-limited: the `releases/latest/download` URL plus the signature check). **Unprivileged pre-check** of size, digest, signature and Team ID: this is the visible "verify signature" step and fails before any password is asked.
  - **One privileged step.** The script text is a constant in the binary and is passed to `/usr/bin/osascript` as an argument, never read from a file; every argument goes through `quoted form of`. `osascript` is started with a fixed environment (`PATH=/usr/bin:/bin:/usr/sbin:/sbin`, nothing inherited) and the script runs under `/usr/bin/env -i /bin/bash --noprofile --norc -c`, with every tool called by absolute path. As root: refuse if `/opt/homebrew` exists; `mktemp -d` under `/var/root`; open the download once, refuse anything that is not a regular file, copy at most the expected size plus one byte from that descriptor; verify the copy (`pkgutil --check-signature`: signed for distribution, leaf Team ID `927JGANW46`, notarized; then `spctl -a -t install` as a second opinion, with its own defined answer when Gatekeeper is turned off); `installer` on the copy; every reason, including the installer's last lines, goes to stderr because stdout is lost on failure. Exit codes 0 / 10 / 11 / 12 / 13 / 14 / 15 / 20 / 21 / 30 / 31 as in the spike note.
  - The step cannot stream, so the UI shows one indeterminate state and then the final error line. `open -W` is not built; the failure text says "open Homebrew.pkg yourself" instead.
  - The pinned Team ID is a constant changed by a normal release. A package signed by another team gets its own code and message.
  - brew and winget runners with `HOMEBREW_NO_ANALYTICS=1`, `HOMEBREW_NO_AUTO_UPDATE=1` for the setup run, and the proxy variables in upper and lower case; step events; failure classification (design 5.1 table); cancel. The AWS CLI on Windows stays "machine-wide, needs an administrator" until check B11 shows `--scope user` works without UAC.
- Owns: `internal/envcheck/`, `internal/toolinstall/`, `internal/execx/` (the stub rule), `internal/api/system.go`, `internal/app/toolservice.go`, mock `system.ts`.
- Also (follow-up FU-04a): the git minimum becomes **2.44** (the first git that honours `GIT_NO_LAZY_FETCH`, which F1-04's reader relies on so a partial clone never fetches during a background read). Homebrew and winget install current git, so only a machine whose git is found elsewhere is affected; the setup screen names the version found and the fix.
- Deps: F1-06, S0-04.
- Accept: no network call during detection (asserted with a failing transport); an offline machine with tools passes; each failure cause maps to its own code; **checks B1 to B3 pass on clean macOS 15, 26 and 27 VMs** (git and awscli install with only "Pouring" lines and no developer-tools dialog; a standard account works with an administrator's name and password).
- Tests: detection matrix with fake binaries, including "no stub is executed while `xcode-select -p` fails"; classifier tests from recorded outputs; the cases of `spike/installers/test-verify.sh` without root (good, wrong size, wrong team, flipped byte, unsigned re-wrap, truncated, HTML page); a symlink to `/dev/zero` and a FIFO as the download are refused; `BASH_ENV`, an exported `grep` function and `SHELLOPTS` + `PS4` planted in the app's own environment have no effect on the script.
- Review: the swap window in the pkg flow; `osascript` gets a fixed environment; the script text is never read from disk; every argument through `quoted form of`; an existing `/opt/homebrew` stops the script; no password ever passes through the app; analytics off.

**F1-13 · Author identity backend** (M)
- Build: `internal/identity`: effective identity from git itself (plan 3.4), sources including `includeIf` and environment variables; global write; project apply to members with a preview; repository override; move-in and move-out rules.
- Owns: `internal/identity/`, `internal/api/identity.go`, `internal/app/identityservice.go`, mock `identity.ts`.
- Deps: F1-02, F1-04.
- Accept: a repository with an `includeIf` rule shows that rule as the source; a shell profile exporting `GIT_AUTHOR_EMAIL` is reported; every write is backed up and undoable.
- Tests: precedence table; preview equals what apply then writes; undo.
- Review: the terminal environment used for `git var` matches what a session will get.

**F1-20 · Windows update gate** (S)
- Build: `internal/update/gate_windows.go`: the unpacked exe's file version must equal the manifest version; trust on Windows rests on the ed25519 manifest and its sha256 (platform-independent code that S0-02 already exercised). No Authenticode check while the Windows build ships unsigned (owner decision 3); the file has one clearly marked place where it would go.
- Owns: `internal/update/gate_windows.go`, a platform-independent helper `internal/update/winver.go` (reading and comparing the exe's file version, so the table tests run on the Mac), and their tests.
- Deps: F1-09.
- Accept: on a Windows 11 VM, rc.1 updates to rc.2 through the test channel (swap, relaunch, version shown); an exe whose file version differs from the manifest is refused and the installed exe is untouched; a locked exe is handled (check B9).
- Tests: version comparison table; a refusal leaves no staging folder.
- Review: the non-Windows, non-macOS gate still refuses everything.

**Group D (serial).**

**F1-W · Wiring** (S)
- Build: register the new services, mocks, locales and routes (Projects, Git settings, Setup, Update) with placeholder pages; sidebar per design 4.1 (Projects replaces Repositories; Terminal page stays until milestone 2); dependencies added (`golang.org/x/mod` becomes a direct dependency for F1-09). Startup handling of `projects.json` (FU-18d): keep F1-18's change in `internal/app/container.go` (the projects store is passed as `repos.Deps.Registry`); `store.ErrNewerVersion` shows the plan's "written by a newer Codepass" message (a native dialog is enough) and quits without writing; `store.ErrCorrupt` runs `Quarantine` and opens a fresh store before the window opens. Everything in the wiring notes `reviews/groupB-wiring.md`, `reviews/groupB2-wiring.md` and `reviews/groupC-wiring.md`, including: register ToolService and IdentityService (IdentityService gets the same `projects.Store` as `repos.Deps.Registry`, exposed as `Container.Projects`), regenerate bindings and swap the hand-written mock types (`mock/toolTypes.ts`, the marked block in `mock/identity.ts`) for the generated ones (FU-13c), the events `tools:install-progress` and `identity:changed`, the repository watch port, a background-fetch loop for bound or opened repositories (every 15 minutes, skipped while offline), `update.ProductionGate()` on every OS (FU-20a: ignore F1-20's note about a placeholder Requirement), and FU-08b.
- Owns: the shared files, `internal/app/loops.go`, the mock files the notes name.
- Deps: group C.
- Accept: typecheck passes with every new service mocked; the app starts and each placeholder renders.
- Review: registration and the wiring notes only, plus the FU-18d startup handling; no feature logic.

**Group E (parallel, after D).**

**F1-14 · Projects page** (L)
- Build: design 5.2 complete: sections, rows, filters, search, suggestions, create / rename / remove, drag and drop with the keyboard path, bulk bar, hide, the "newly found" banner and chooser, all states.
- Owns: `frontend/src/pages/projects/` (replaces `pages/repos/`) except `Clone*.tsx` (F1-19), `frontend/src/state/projects.tsx`, locale `projects.ts`; mock `repos.ts` for FU-11c; the mock `world.ts` (the only group E task that may edit it). New files go under `pages/projects/`; `pages/repos/` stays untouched (`pages/git/` and the clone modal import from it; F1-CL deletes what ends up unused). Every repository name uses `components/RepoName` (a stub until F1-19 lands, same props); the clone modal is imported from `./CloneModal`. Opening a repository calls `RepoService.MarkOpened`. F1-11 added the service methods (on RepoService: projects, found set, hide, moved question, `warnings`, error codes `E_MOVED_HAS_DATA` / `E_MOVED_GONE` / `E_PROJECT_NOT_FOUND`); see `reviews/groupC-wiring.md`. Repository names are drawn with F1-19's `RepoName` component; until it lands the row shows `localName` as plain text.
- Deps: F1-W, F1-08.
- Accept: matches `projects-light-zh`, `projects-dark-en`, `projects-drag-light-zh`, `projects-found-light-zh`; the bind popover still works from the new page; a repository that was moved by hand shows the re-attach question of F1-18.
- Tests: model tests (grouping, filter, suggestion); drag and keyboard move; live-region announcements.
- Review: roving grid accessibility; motion skipped over 200 rows.

**F1-15 · Setup screen and Health rows** (M)
- Build: design 5.1: the gate screen with every state and failure, macOS and Windows variants; the same rows on Health with Update actions. There is no "Install into my own folder" button (disproven by S0-04). The Homebrew row's steps are download, verify signature (before the password prompt), install (one indeterminate state that ends in a single result line). New failure rows: "Homebrew is partly installed", "Download damaged", "Homebrew's signer is not recognised".
- Owns: `frontend/src/pages/setup/`, `frontend/src/pages/health/ToolRows.tsx`, locale `setup.ts`; mock `system.ts` for FU-12a. The copy never promises a backup of a cleared tool path (FU-12b), and handles fix `use-detected` and the `override` flag (F1-12 notes in `reviews/groupC-wiring.md`).
- Deps: F1-W, F1-08.
- Accept: matches the six setup shots plus `setup-noadmin-light-zh` (re-taken without the second button); the gate cannot be bypassed by a route.
- Tests: state machine tests; announcements; focus after failure.
- Review: copy against the design table; the log never shows a secret.

**F1-16 · Git settings and identity UI** (M)
- Build: design 5.12: global card, ladder, effective identity table with source tags, project sheet with the affected list, repository popover.
- Owns: `frontend/src/pages/git/` (identity parts), locale `identity.ts`; mock `identity.ts`; `repoInfo` in `internal/identity/write.go` for FU-13b. F1-13's notes (undo ID, new enum values, events) are in `reviews/groupC-wiring.md`.
- Deps: F1-W, F1-08.
- Accept: matches `identity-light-zh`, `identity-page-light-zh`, `identity-dark-zh`.
- Tests: sheet preview list; source tag mapping.
- Review: the existing Git global guard cards are unchanged.

**F1-17 · Update UI** (M)
- Build: design 5.13: banner, Settings dot, dialog with steps and progress, every state; the Updates card in Settings; daily check with a setting.
- Owns: `frontend/src/pages/update/`, `frontend/src/pages/settings/UpdatesCard.tsx`, `internal/app/updateservice.go`, mock `update.ts`, locale `update.ts`; and, as the one exception to the shared-file rule in group E, the registration lines for UpdateService and the `update:state` event in `main.go`, `internal/app/events.go`, `frontend/src/api/services.ts`, `frontend/src/api/mock/index.ts`, `frontend/src/api/events.ts`, plus the Settings update dot in `frontend/src/shell/Sidebar.tsx`. The controller already runs (`Container.Update`, F1-W notes).
- Deps: F1-W, F1-09.
- Accept: matches `update-light-zh`, `update-banner-light-zh`; notes render as plain text.
- Tests: state mapping; a note containing HTML is shown as text.
- Review: never a modal at launch; "Reopen when no task is running" wording and logic hook.

**F1-19 · Clone with a custom folder name; two names everywhere** (M)
- Build: design 5.2 "Clone" and "Two names". The clone modal moves to the Projects page and gains a "本地文件夹名 / Local folder name" field per repository row (pre-filled with the remote name, editable inline, also in multi-clone), with the live target path and inline errors (name taken, invalid character). `RepoName`: one component that draws the local folder name first and, only when the two differ, the remote name as secondary text ("远程 acme_api / remote acme_api"); used by every list and header that names a repository.
- Owns: `frontend/src/pages/projects/Clone*.tsx`, `frontend/src/components/RepoName.tsx`, locale `clone.ts`. Both files exist as stubs: move the component from `pages/repos/CloneModal.tsx` (and its test) into `pages/projects/CloneModal.tsx`, turn the old file into a one-line re-export, and replace the `RepoName` stub keeping its props.
- Deps: F1-W, F1-08, F1-11.
- Accept: cloning three repositories with one of them renamed creates the three folders and lists the renamed one with both names; the existing AWS clone flow is otherwise unchanged; a name collision is caught before anything is cloned.
- Tests: validation states; `RepoName` renders one name when they are equal and two when they differ, with the secondary text exposed to screen readers as "remote name".
- Review: the folder name never reaches shell text; the remote name is shown as text only.

**Group F (alone, after E).** Added 2026-11-16 (owner, second batch): the fixes the owner made on `main` (AWS SSO Hub 0.0.2) come into Codepass before the gate, so the build that replaces AWS SSO Hub on the owner's Mac does not lose them. F1-CL moved behind F1-21 and F1-22 so that it also clears their follow-ups.

**F1-21 · Carry over AWS SSO Hub 0.0.2 (merge `main`)** (M)
- Why (owner, 2026-11-16): on `main` (`40f556f`..`3674f3f`, 13 commits, version 0.0.2) the owner fixed four things Codepass must keep: (1) the Console button with several Identity Center users on one portal (AWS showed "No access" because the portal cookie belonged to the other user): the console now opens with the app's own Identity Center session (role credentials, then the federation `getSigninToken` / `login` URL; the link never reaches the frontend, the log or an error); (2) two Identity Center users signed in to one portal one after the other ended up as the same person: each sign-in now opens in its own fresh private window of a separate browser instance on a throw-away profile (Chrome, Edge, Brave, Chromium; other browsers keep a private or normal window), closed and deleted when the sign-in ends, when the page is opened again and when the app quits, crash leftovers removed at the next start; (3) private-window ("incognito") sign-in for each account: the existing setting "sign in in a private window" (on by default, already in Codepass since `3717a6f`) now gives every sign-in that isolated window; (4) the session length is set by the AWS administrator in IAM Identity Center, not in the app: the session-length dialog, the "Adjust length" banner and the "Change" button are gone, and the app works the end out from AWS (exact once a renewal is capped or refused; until then an estimate that never claims more than the access token's hour right after a first sign-in). README, guide and tutorial were updated to match.
- Build: in the task worktree, `git merge main`: a real merge, never a squash or a cherry-pick, so `main` becomes an ancestor of `codepass` and the release merge into `main` (R7-08) stays clean. A trial merge on 2026-11-16 conflicted in eight files: the two bindings files `frontend/bindings/github.com/rudian/codepass/internal/api/index.ts` and `.../internal/app/accountservice.ts` (regenerate them after the merge); `frontend/package-lock.json`, `internal/version/version.go` and `internal/app/app_test.go` (Codepass keeps its own version line: 0.0.2's version bump is not taken, and every version string in `build/`, `frontend/package.json` and `version.go` must agree after the merge, because `build/config.yml` and the `Info*.plist` files auto-merge to 0.0.2); `frontend/src/pages/tutorial/content/en.ts`, `zh.ts` and `internal/app/loops.go` (keep what both sides meant). New files from `main` import `github.com/rudian/aws_sso_hub/...` (`internal/awsapi/console.go`, `internal/browser/signal_*.go`, the console tests): change them to `github.com/rudian/codepass/...`. Then fit the merged code into what Codepass changed since `3717a6f`: (a) the isolated sign-in browsers are closed on every Codepass quit path (Cmd+Q, menu, Dock, tray, F1-03's logout path, and later A2-14's "stop all") and leftovers are removed at start; (b) every `internal/browser` caller Codepass added (update, setup, projects, settings) still builds and opens a normal window; (c) the mock (`frontend/src/api/mock/accounts.ts`, `world.ts`) and the Accounts page have no session-length control, and the Console button's busy state shows in the mock; (d) the versioned settings store (F1-02) keeps `sessions.*.sessionEndsAt` and `sessions.*.newSession` and drops nothing a 0.0.2 data folder holds (plan 4.2); (e) README, guide and tutorial say what 0.0.2 says about the administrator's session length and the isolated window, in Codepass's wording; (f) `AWSSSOHUB_SIGNIN_ENDPOINT` works in sandbox mode like the other endpoint overrides.
- Owns: every file the merge touches, plus the adaptations above (the task runs alone, so it may edit shared files). No other feature work.
- Deps: group E.
- Accept: after the merge into `codepass`, `git log codepass..main` prints nothing; no Go file imports `rudian/aws_sso_hub`; the 0.0.2 tests pass inside Codepass; a 0.0.2 data-folder fixture upgrades and keeps both new session fields; no session-length control remains in the UI, the mock or the copy; the gate passes; checks C19 to C21 (`manual-checks.md`) are ready for the owner.
- Tests: the tests that come with the merge; a 0.0.2 data-folder upgrade fixture; quitting through each Codepass quit path closes a fake isolated browser; Windows vet of `internal/browser`.
- Review: each of the eight conflict resolutions; every place where Codepass code meets the merged code (quit paths, browser callers, settings versioning, mock, copy); nothing of 0.0.2 lost: compare `git diff 3717a6f main` with the merged result file by file.
- Merge into `codepass`: **keep history**. Run the group with `keepHistory: ["F1-21"]` in the workflow args; the merge step then uses `git merge --no-ff` instead of `--squash`, and every commit on `task/F1-21` carries the session's two trailer lines.

**Group G (alone, after F).**

**F1-22 · System notifications; sign-in expiry notices** (M)
- Why (owner, 2026-11-16): when an AWS sign-in expires, a macOS notification names which sign-in and its accounts, with a button that signs in again from the notification. The session length is the administrator's, so the button starts a new sign-in; nothing can extend the old session.
- Build: plan 4.15; design 6 ("System notifications" table) and 5.14 (the Sign-in reminders card, the System notifications row); `docs/research/notifications.md` sections 5 (ten rules) and 6 (the fake).
  - `internal/notify`, the app's only notification code: wraps Wails 3.1.0's `notifications` service, starts it itself and survives a start failure (a binary without a bundle ID then uses in-app toasts only; the Wails service is never registered bare, because its start failure would stop the app); owns the single response callback and routes a click by kind to the feature that registered the kind; registers the kinds' button texts at start and again on a language change; one button at most, title and body only, identifiers only in the data map; permission requested in the background the first time a portal is signed in with the window in front, "asked" kept in `notifications.json`, state reported as allowed / not asked / off; opens the system's notification settings; send, replace (same ID) and remove a delivered notice. A fake with a `respond` call is always used in sandbox mode and in tests (and shows each notice as an in-app toast in sandbox mode); real notifications in a development build only with one opt-in environment variable, because the development build shares the installed app's bundle ID.
  - The sign-in kinds: the existing reminder (`maybeRemind`, `session:reminder`) becomes the heads-up; a new "ended" notice is sent once when a sign-in's state becomes expired, never for a network failure, a sign-out or a session that was already expired when the app started (those are marked as told), and once after waking for a session that ended during sleep; one key per portal and sign-in time, stored in `notifications.json`; the heads-up is skipped when the sign-in itself started inside the lead time; a successful sign-in removes the portal's delivered notice; "ended" replaces the heads-up on macOS.
  - The button starts that portal's sign-in in the isolated window (F1-21), or brings a sign-in already running forward; a stale button opens Accounts at the portal with "已经登录了 / Already signed in". A click on the body shows the window and opens Accounts at that portal with the one-time highlight. A click that launched the app is handled after the accounts load and after the startup gates.
  - Settings: `reminders.endedNoticeOff` (zero value = notify) and its row; the help lines; the macOS permission row and the Windows help line; the in-app toast when notifications are off and the window is visible. Locale `notify.ts` and `settings.notify.*`; mock `notify.ts`.
- Owns: `internal/notify/`, `internal/app/notifyservice.go`, `internal/api/notify.go`, `internal/accounts/session.go` and its tests (the "ended" notice, keys), `internal/settings/` (the new field), `frontend/src/pages/settings/` (the rows), `frontend/src/pages/accounts/` (open at a portal), locale `notify.ts`, mock `notify.ts`. It runs alone, so it also registers itself in the shared files (`main.go`, `internal/app/container.go`, `internal/app/events.go`, `frontend/src/api/services.ts`, `frontend/src/api/mock/index.ts`, `frontend/src/App.tsx`).
- Deps: F1-21 (the session end learned from AWS; the isolated sign-in window).
- Accept: with the fake, one heads-up and one "ended" per sign-in, worded as design 6 in both languages; the button starts that portal's sign-in; a stale button only shows "Already signed in"; nothing is sent at start for sessions that ended while the app was closed, for a network failure or for a sign-out; the app starts and works when the Wails service cannot start; the Settings rows match design 5.14; checks C22 to C24 and B19 are ready for the owner (they carry the research note's M1 to M10 and W1 to W4).
- Tests: routing by kind through the fake's `respond`; keys across a restart inside the lead time; sleep and wake; expired at start; network error; sign-out; a language change re-registers the button texts; start failure of the real service; the body for one, two and three or more accounts and for an unknown user; a token, a URL, an account ID and a role name fed in never appear in a title, a body or the data map.
- Review: plan 4.15 "When not to notify", item by item; nothing secret in any text or data; the permission request never blocks the UI thread; a stale or replayed click can never start another portal's sign-in; sandbox mode never reaches the real service.

**Group H (serial, after G).**

**F1-CL · Milestone 1 clean-up** (S)
- Build: fix every follow-up marked "F1-CL" in the follow-up list ("Owner decisions" section), including the ones F1-21 and F1-22 leave behind, each with a test where the follow-up asks for one. Delete the files in `pages/repos/` that nothing imports any more (after F1-14 and F1-19: `ReposPage.tsx`, `RepoRow.tsx`, the one-line `CloneModal.tsx` re-export, page-only tests, the duplicated `.clone-*` rules in `repos.css`). Every list and header that names a repository uses `RepoName` (design 5.2). Mount what group E built but could not place: `<UpdatesCard />` in `SettingsPage.tsx`, `<UpdateBanner />` once under the Projects header, `<ToolRows />` in Health's runtime card (replacing EnvChecks' AWS CLI and Git rows). Delete unused locale keys (`placeholder.setup.*`, `placeholder.update.*` if unused, `git.title` / `git.subtitle`, `clone.folderName`). Optional, from the notes: an icon prop on `components/Banner`. **Owner request (2026-11-16, second batch): find a repository fast on the Projects page** (plan 4.4, design 5.2 "Find a repository by typing"): the ranking (folder name starts with the text, then a word inside it, then anywhere, then the other names and the path), the matched letters in bold, Down / Up / Enter / Cmd+Enter / Esc on the search field (Enter opens the selected repository: until A2-06 that reveals the folder, like the row), the "N repositories match" line and the no-match line with the hidden count; this goes to F1-CLb (frontend) and comes with tests for the ranking and the keys. With about 60 items it can run as two parallel tasks: F1-CLa (Go) and F1-CLb (frontend), split by file.
- Owns: the files those follow-ups name.
- Deps: group G (F1-22).
- Accept: every F1-CL follow-up is fixed or declined with evidence; the gate passes.
- Review: the same scoped verification as any fix round.
- Split (orchestrator, 2026-11-17): group H runs F1-CL as two parallel tasks, split by file. Only the mock `world.ts` and locale `health.ts` may be edited by both, each task on its own lines (the merge joins them). Each follow-up's full text is in the follow-up list; the full review text is in `reviews/<group>-final.json` (local files in the main checkout). Fix each item with the test it asks for, or decline it with evidence.
  - **F1-CLa · Go, scripts and docs.** FU-02b, FU-02c, FU-02d, FU-03b, FU-03c, FU-03d, FU-04c, FU-05a, FU-05b, FU-06a, FU-06b, FU-07a, FU-07b, FU-07c, FU-09a, FU-09b, FU-10a, FU-11a, FU-11b, FU-11d, FU-11e, FU-11f, FU-11g, FU-12b, FU-12c, FU-12d, FU-13a, FU-13d, FU-14h, FU-15e, FU-16a, FU-16c, FU-16d, FU-17c, FU-17d, FU-17f, FU-18e, FU-18f, FU-18g, FU-18h, FU-19c, FU-20a (the comment), FU-20b, FU-20d, FU-21a, FU-21b, FU-21c, FU-22a, FU-22c, FU-22d, FU-22e, FU-22f, FU-22g, FU-Wb, FU-We. Owns: every Go file and test, `scripts/`, `build/`, `README.md`, `README.zh-CN.md`, `docs/release.md`, the generated `frontend/bindings/`, the shared files `main.go`, `internal/app/container.go`, `internal/app/events.go`, `go.mod`, `go.sum`, `.gitignore`, and the frontend files that follow its Go API changes: `frontend/src/pages/health/ToolRows.tsx` and its test (FU-15e), `frontend/src/api/mock/system.ts`, `frontend/src/api/mock/update.ts` and their tests, `frontend/src/api/errors.ts`, locale `errors.ts` (the text of a new error code, such as FU-16a's), the new words FU-15e needs in locale `health.ts`, and the lines a new Go field forces in the mock `world.ts` (FU-17d).
  - **F1-CLb · Frontend.** The Build items above (delete the unused `pages/repos/` files; `RepoName` in every list and header; mount `UpdatesCard`, `UpdateBanner` and `ToolRows` (FU-15d, FU-17g); delete the unused locale keys, where `settings.reminders` is already gone; the optional Banner icon), the owner's Projects search (check C25; one exported ranking function that A2-16 reuses), and FU-08a, FU-14a, FU-14b, FU-14c, FU-14d, FU-14e, FU-14f, FU-14g, FU-14i, FU-15a, FU-15b, FU-15c, FU-16b, FU-16e, FU-16f, FU-17a, FU-17b, FU-17h, FU-19a, FU-19b, FU-22b, FU-Wa, FU-Wc, FU-Wd. Owns: every file under `frontend/src/` that F1-CLa does not own (in `world.ts` and `health.ts`, every line but F1-CLa's), including the shared files `frontend/src/api/services.ts`, `frontend/src/api/mock/index.ts`, `frontend/src/App.tsx`, `frontend/src/shell/Sidebar.tsx`, `frontend/src/i18n/index.ts`, `frontend/src/styles/*.css`, `frontend/package.json` and `frontend/package-lock.json`. It changes no Go API; a need for one goes into notesForWiring.
  - Not in F1-CL: FU-17e (milestone 1 gate), FU-16g (milestone 6).

**Gate.** The standard gate. Screenshots: projects (4), setup (7), identity (3), update (2), settings (2: compare only the cards milestone 1 built); several were not re-taken after group E's fix rounds, so take them all fresh. Also take three screens that have no mockup shot: the clone modal (judge against design 5.2); the Projects search while searching and with no match (`?search=`; design 5.2 "Find a repository by typing"); and Health with ToolRows mounted (group H). Approve with the owner (or Fable) the design deviations listed in `reviews/groupE-wiring.md` (Projects: the extra "Add a folder" button, select-all per section, "Bind all"; Setup: the running footer text and the copy written for causes design 5.1 does not word; Update: "Reopen now" disabled during the download (FU-17e), no release date; Identity: the 200px Source column) and the startup dialog wording in `internal/app/startup.go`. Also from the second batch: the Settings rows of F1-22 (Sign-in reminders card, the notification permission row; mock scenarios for allowed, not asked and off), judged against design 5.14 (no mockup shot), and the notification texts of design 6 read once by the owner in both languages. Manual: upgrade a copy of the owner's 0.0.1 data folder and a copy of a 0.0.2 one; the C checks C1 to C28, where C19 to C28 cover the second batch, group H and the gate review (0.0.2 behaviour inside Codepass; sign-in notifications; finding a repository by typing; quitting closes the sign-in windows; updating a tool from Health; one portal with several accounts and roles); B19 (Windows notifications); install on clean macOS 15, 26 and 27 VMs (gate, then app: checks B1 to B3); the launch smoke test of the signed build with the old app quit (A9); update test channel build N to N+1 on the Mac and once on the Windows 11 VM (B5, B9).

**Gate review (2026-11-17).** The orchestrator re-took 37 screenshots in the Vite mock and Fable compared them with the mockup shots and the design text, together with the startup dialog texts and the notification texts. The owner decided on 2026-11-17: everything as recommended, and D41 as the owner worded it. 31 items are accepted as built (among them D27: Settings stays one column in milestone 1 and A2-CL builds the two-column grid; and FU-17e: "Reopen now" stays disabled during the download). 19 items are fixed by F1-GF. Each item with its decision is in `reviews/m1-gate-review.md`. Owner notes: (1) Codepass cannot extend a sign-in; when it expires, a notification with a button signs in again (that is F1-22, merged); (2) one portal holds several cards, the same account with different roles or different accounts (built; new manual check C28). Still open for the gate, with the owner: the test channel (then a signed, notarized rc) and the manual checks C1 to C28, A9, A16, A17, B1 to B3, B5, B9, B15 to B19. Owner, 2026-11-17: milestone 2 starts now; problems the open checks find go into A2-CL.

**Group I (after the gate review).** Added 2026-11-17 by the orchestrator. F1-GF runs beside milestone 2's A2-01: they share no file (A2-01 owns `internal/pty/`, `go.mod` and `go.sum`).

**F1-GF · Milestone 1 gate fixes** (S)
- Why (owner, 2026-11-17): the 19 items of the gate review marked "to fix". `reviews/m1-gate-review.md` has each item's text and fix hint.
- Build:
  - Projects: D7 (rows that need an action, such as the expired sign-in row with its Sign in action, count in "待处理 / To fix" and in the sidebar count, as the mockup's "To fix 2" and "2 to fix"); D8 (middle truncation without a gap after the ellipsis, and only when the path does not fit); D10 (a status line stays on one line, also in English).
  - Setup: D13 (zh 展开日志 / 收起日志, one word for the log); D14 (a step without a percent shows an indeterminate bar; with reduced motion it stands still); D16 (the padding above the log's first line stays when the log is scrolled to its end); D17 (one footer text for every failure cause).
  - Update: D25 (the footer link at 13px). FU-17e is accepted: change the mockup so "现在就重新打开 / Reopen now" is disabled while downloading, and re-take `docs/design/codepass-mockup/shots/update-light-zh.png` from it.
  - Settings: D28 (the subtitle "Codepass <version> · <platform>" as in the mockup: the version the app reports; macOS "macOS 15 或更高，Apple 芯片 / macOS 15 or later, Apple silicon", Windows "Windows 10 或 11，x64 / Windows 10 or 11, x64"); D29 (`settings.folders.hint`: 在这些文件夹里查找 Git 仓库 / Where to look for Git repositories); D41 (`settings.reminders.autoRenew.hint`: 在管理员允许的时间内自动保持登录，到期后仍要重新登录。 / Keeps you signed in for as long as your administrator allows. After that, you sign in again.).
  - Clone modal: D32 (the home folder shown as `~` in "会克隆到 / Clones into"); D34 (the space between the title and the first field matches the other modals; the modal scrolls at 980x660).
  - Projects search: D36 (the remote-name tag is never cut: it moves to its own line, or the local name shrinks first); D37 (matched letters are marked by more than weight: an underline or a light tint).
  - Texts: T1 (the damaged-data dialog, `internal/app/startup.go` `damagedMessage`: 项目分组的数据坏了，已另存为 <路径>。仓库本身没有动，分组要重新建。 / Your project list was damaged and was kept as <path>. Your repositories are untouched; the projects start empty.); T2 (the newer-version dialog: 「这个版本不会打开它」, and the releases address in both languages from `ReleasesURL` in `internal/app/updateservice.go`, shown without `https://` and `/latest`); T3 (the notification body 「等 N 个账号」, not 账户); T4 (the English body "Sign in again to continue."). T3 and T4 change in `internal/notify/signin.go`, in locale `notify.ts` where it holds the same words, and in design 6's table (`ui-design.md`).
- Owns: the files those items name: `frontend/src/pages/projects/`, `frontend/src/pages/repos/model.ts` and `repos.css`, `frontend/src/components/RepoName*`, `frontend/src/pages/setup/`, `frontend/src/pages/update/`, `frontend/src/pages/settings/SettingsPage.tsx` and its CSS, `frontend/src/shell/Sidebar.tsx` (D7's count), locales `projects.ts`, `setup.ts`, `update.ts`, `settings.ts`, `notify.ts`, `internal/app/startup.go`, `internal/notify/signin.go`, their tests, `docs/plan/codepass/ui-design.md` (design 6's table), `docs/design/codepass-mockup/` (FU-17e).
- Deps: F1-CL.
- Accept: each item visible in a fresh screenshot (projects-light-zh, projects-dark-en, setup, setup-failed, setup-noadmin, update-light-zh, settings-light-zh, the clone modal, the search) and in the re-taken mockup shot; the gate passes.
- Tests: D7's count (the model and the sidebar); D8's split; D14's indeterminate state; D17's footer for each cause; D28's subtitle on both platforms; D32's `~` path; D36 and D37 in RepoName; T1 to T4 in Go, in both languages.
- Review: each item against `reviews/m1-gate-review.md`; nothing else changes.

---

## Milestone 2 - AI terminal and keep-awake (5 weeks)

**Goal.** AI coding inside the app, in the repository's main folder, for three tools, with the computer kept awake while it runs.
**User-visible result.** Open a repository, press Cmd+T, pick Claude Code, Codex or OpenCode, and work in an in-app terminal. Several sessions at once, in several repositories; Cmd+J jumps to any of them from any screen. A cup shows while the Mac is held awake. Programs a closed session left behind are listed and can be force-stopped. Quitting always asks: stop everything, or keep running in the background.

**Group A (serial).**

**A2-01 · PTY and process control** (M)
- Build: `internal/pty`: one interface; spawn, resize, exit code, and `KillSession`.
  - macOS: creack/pty; the child is the leader of a new terminal session. **Stopping kills every process in that terminal session, not one process group**: with the plan's launch form a tool started through a shell function (the owner's `claude` is one) runs as a job in its own process group, so a group kill misses it and everything it starts (review A, X1). `KillSession` lists processes, compares `getsid()` with the leader's, and signals every group they lead: SIGTERM, about 5 s, then SIGKILL. A process that calls `setsid` itself leaves the session; A2-13 keeps track of those.
  - Windows: the app's own ConPTY wrapper over `golang.org/x/sys/windows`, started from `spike/terminal/winpty/winpty_windows.go` (derived from go-pty's MIT code, credited in THIRD_PARTY_NOTICES; go-pty is not imported). The child is created with `PROC_THREAD_ATTRIBUTE_JOB_LIST` in the same attribute list as the pseudo console, inside a Job Object with `KILL_ON_JOB_CLOSE`, so no grandchild can escape and an app crash kills every descendant. `ClosePseudoConsole` runs off the caller's goroutine (it blocks on old Windows 10 builds until the output pipe is drained).
- Owns: `internal/pty/`.
- Deps: S0-01.
- Accept: stopping a session kills a child dev server on both platforms; resize reaches the child; the wrapper passes check B8 on the Windows VM (it has never run).
- Tests: spawn, resize, exit codes; kill-tree with the **real launch form and a shell function**; a grandchild started under `set -m`; a child that ignores SIGTERM is gone after the SIGKILL step; on the Windows VM: kill-tree, and the app killed from Task Manager leaves no child.
- Review: no orphan after an app crash; nothing is killed that is not in the session or the job.

**Group B (parallel, after A).** Orchestrator, 2026-11-17: the group also runs F1-GF2 (at its end; it touches only the Projects table's CSS). Exceptions for this group: A2-06 adds the workspace route to `App.tsx` and changes `openRepo` in `ProjectsPage.tsx`; A2-07 adds the xterm packages (`frontend/package.json`, `package-lock.json`); A2-13 may edit `internal/pty/pty_unix.go` for FU-A2-01a. The terminal contract between A2-02 and A2-07 is in `reviews/m2B-args.md`.

**A2-02 · Terminal sessions and the data path** (L)
- Build: `internal/terminal` rewritten: sessions with a byte log and offsets kept as a ring of fixed chunks (memory stays near the cap; default cap 16 MB per session), a "gap" marker, the small ping event sent from its own goroutine, bound methods `Read(session, offset)` (returns a copy; the log lock is never held across a Wails call; `max` is capped), `Write`, `Resize`; explicit environment (plan 4.7); the login-shell launch form. From S0-01 and its review: the reader tracks DEC private modes (alt screen 1049 / 47 / 1047, mouse, bracketed paste, focus, cursor keys, cursor visibility, keypad and keyboard-protocol modes); the log is trimmed only where the escape parser is in ground state and on a UTF-8 boundary, preferring the latest full clear or alt-screen entry before the cut; a read whose offset was trimmed returns `gap` plus a `prologue` that is the mode state **as of the new base**, with both set and reset sequences. When a session ends, its last scrollback (capped) is written to the task's data folder (`<data>/sessions/<task-id>/scrollback`, mode 0600), so an ended task can still be read after a restart; a setting turns this off; the file never enters a backup, an export or the app log.
- Owns: `internal/terminal/`, `internal/api/terminal.go`, `internal/app/terminalservice.go`.
- Deps: A2-01.
- Accept: the reader goroutine never blocks on the frontend (test with a consumer that never reads); output never reaches the app log; a full-log replay gives the same screen and cursor for vim; a replay after a gap restores the alt screen; an in-app flood **whose child starts after the window is hidden** does not stall; a hide of at least 10 minutes with a slow steady producer does not stall (App Nap). Bytes per hour are measured from a working Claude Code and OpenCode session and the cap is sized so a normal day never trims; if it cannot be, mounted-but-hidden terminals keep pulling at a low rate (the spike showed that pulling while hidden works).
- Tests: offset reads, gap re-sync, concurrent readers, environment contents (`TERM`, `LANG`, no `AWSSSOHUB_*` / `WAILS_*`); mode tracker with sequences split across chunks; trim never cuts inside an escape sequence or a UTF-8 character; escape-dense benchmark.
- Review: argument-list launch; the 8 KB and 64-event limits cannot be hit by design; replay after trimming is the recovery path, not the normal one.

**A2-03 · Keep-awake manager** (M)
- Build: `internal/keepawake`: counter, caffeinate with restart-if-killed, `NSProcessInfo` activity, `SetThreadExecutionState` on a locked thread, grace time, hard ceiling, "screen too" option. Processes a closed session left running (A2-13) count as activity; the hold is released when the last one is gone, and the hard ceiling still applies to them.
- Owns: `internal/keepawake/`, `internal/app/keepawakeservice.go`, mock `keepawake.ts`.
- Deps: none.
- Accept: the test matrix of plan 4.10.
- Tests: fake clock and fake process runner for every row of the matrix.
- Review: released on every exit path; never held by a plain shell.

**A2-04 · AI tools: detect, install, launch** (M)
- Build: `internal/aitools`: detection through the login shell with a timeout, using only `--version` (`opencode auth list` starts OpenCode's shared background service; `opencode --version` does not); install commands per platform shown before they run (macOS: `brew install --cask claude-code`, `brew install --cask codex`, `brew install opencode`; Windows: `winget install --id Anthropic.ClaudeCode -e`, `winget install --id OpenAI.Codex -e`, OpenCode through npm because winget has no package: check B10 confirms the package name); launch arguments per tool; reading each tool's approval or sandbox mode (vocabulary in plan 4.8); never adding a permission-skipping flag. Argument builders cover Claude Code `--session-id`, `--resume`, `--plugin-dir`; OpenCode `--standalone --session ses_<id>` (**always** `--standalone`: without it stopping a task cannot kill what OpenCode runs, and the task's environment does not reach it); Codex `-c` and `resume`. Also the naming call and the first message (plan 4.7 "Naming", owner request 2026-11-16): one headless call per tool with the fixed naming prompt, no tools, the smallest model, an empty working folder and a 5-second budget, its answer parsed and validated (title, slug); and the first-message argument per tool (after `--` for Claude Code and Codex, `--prompt=<text>` for OpenCode). For milestone 8 (second batch): the launch builder takes an optional account environment (`CLAUDE_CONFIG_DIR` / `CODEX_HOME`, filled by T8-01), and the environment the app builds for its own print-mode helper calls (the naming call) leaves out `ANTHROPIC_API_KEY` and `OPENAI_API_KEY`, because in print mode an API key in the environment always wins and bills the API instead of the plan (`docs/research/claude-usage-accounts.md` R2).
- Owns: `internal/aitools/`, `internal/app/aitoolservice.go`, mock `aitools.ts`.
- Deps: F1-12, S0-03.
- Accept: a missing tool yields the exact install command; a user-defined `claude` shell function still applies (plan 4.7 stands: the app launches through the login shell, and its own flags come after the function's, which is why hooks travel as a plugin and not as `--settings`); after an OpenCode task stops, no `opencode` process is left and `~/.config/opencode/service.json` is unchanged. Check A18 (naming on the owner's Mac, per tool) passes.
- Tests: argument builders; a test asserts none of `--dangerously-skip-permissions`, `--allow-dangerously-skip-permissions`, `--permission-mode bypassPermissions`, `--auto`, `--yolo`, `--dangerously-bypass-approvals-and-sandbox`, `--approve-for-me`, nor a Codex sandbox override such as `--sandbox danger-full-access` / `-s danger-full-access` (second batch: the sandbox follows the owner's own settings) can be produced; detection with fake shells; tests that need Codex skip when it is absent. Naming: parser and fallback table (Chinese, English, mixed, emoji, very long, empty, an answer that is not JSON or breaks the branch rules); a description starting with `--dangerously-skip-permissions` reaches the tool only as message text.
- Review: nothing from a task title or path reaches shell text; every injection adds to the user's own setup and is tested per tool (two `--plugin-dir` flags; an OpenCode user `plugins` list; Codex `-c notify=[...]` replaces a user's own `notify`, so it is used only when the user has none).

**A2-05 · Task state signals** (L)
- Build: `internal/taskstate`: the binary's `report` mode and a local socket (user-only permissions). The launcher sets `CODEPASS_TASK` in the child environment and report mode reads it; it forwards **only** task ID, event name, session ID and permission mode, plus two enums added for milestone 8 (second batch, `docs/research/claude-usage-accounts.md` R4): the `error` kind of `StopFailure` (`rate_limit`, `authentication_failed`, ...) and the `notification_type` of `Notification` (T8-04 later adds a third: the queue's done / question flag); never any text; exits at once and never waits for the app. Per-launch injection, never a write to the user's own settings: For a task started with an empty description, the first prompt a hook reports is handed once, in memory, to the naming call (plan 4.7); it is never logged or stored.
  - Claude Code: a session-only plugin folder (`.claude-plugin/plugin.json` and `hooks/hooks.json`, written under the task's data folder) passed with `--plugin-dir`; a `--settings` file is the fallback. Subscribe only to `SessionStart`, `UserPromptSubmit`, `Notification`, `PermissionRequest`, `Stop`, `StopFailure` (a turn ended by an API error or a usage limit: the state "limit reached" with its kind), `SessionEnd`, and one `PreToolUse` hook whose matcher is exactly `AskUserQuestion` (the AI asks the person a question; it fires only for that tool, so it adds no process start to other tool calls; command hooks run synchronously, which is why no other `PreToolUse` / `PostToolUse` is used); a short hook `timeout`.
  - OpenCode: a plugin given through `OPENCODE_CONFIG_CONTENT`, with `--standalone`. Running = `session.execution.started`; waiting = `session.execution.succeeded | failed | interrupted` and `permission.asked` (`session.idle` is unconfirmed: check A7).
  - Codex: hooks (`UserPromptSubmit`, `PermissionRequest`, `Stop`) if they can be supplied per launch without a trust prompt, otherwise `-c notify=[...]` (turn-complete only), otherwise the fallback (check A8 decides).
  - Fallback: `TIOCGPGRP` on the PTY plus one `ps` call on macOS, a process-tree walk on Windows. The foreground process is the tool itself, and with the app's launch form the shell exits together with the tool, so the fallback can only say "alive" or "exited". Without hooks a task shows "running (no signal)" until it exits, and keep-awake relies on the hard ceiling.
- Owns: `internal/taskstate/`, `cmd` entry for report mode in `internal/app/reportmode.go`.
- Deps: S0-03.
- Accept: state changes arrive for all three tools on the owner's Mac with the real tools and in the VM test; `~/.claude/settings.json`, `~/.codex/config.toml`, `~/.config/opencode/opencode.json` and `~/.config/opencode/service.json` are byte-identical before and after (`~/.claude.json` and the tools' data folders are excluded: the tools rewrite those themselves).
- Tests: socket protocol, event-to-state mapping per tool, fallback with a fake process table; a payload carrying a prompt is reduced to the four forwarded fields.
- Review: the socket accepts only the same user; payloads are never logged (they hold the prompt, tool input and the last assistant message).

**A2-13 · Process tracker and force stop** (M)
- Why (owner, 2026-11-16): an AI tool starts background helpers, dev servers and shells. They must be known even after the session's terminal is closed, shown as "closed but still running", and stoppable.
- Build: `internal/proctrack`: per session, a set of every descendant ever seen. A snapshot every 2 s while the session is live and every 10 s afterwards (macOS: one `ps -axo pid=,ppid=,pgid=,sess=,lstart=,pcpu=,rss=,comm=` call, never one call per process; Windows: the Job Object's process list). A process is remembered by PID **and start time**, so a reused PID is never mistaken for it; a process that left the terminal session by `setsid`, or was re-parented to launchd when its parent exited, stays in the set. API: `Leftovers(session)` (count, CPU, memory, command names), `ForceStop(session)`: SIGTERM to every tracked process and group, about 5 s, then SIGKILL (Windows: `TerminateJobObject`), then one more snapshot to confirm. The set survives the session's close and is dropped when its last process is gone. It does not survive an app quit: quitting with "stop all" runs `ForceStop` for every session first (A2-08). **After a crash** (FU-A2-01b, planner 2026-11-17; plan 4.8b): on macOS a `nohup` child or a `set -m` background group survives when the app dies, so the set (PID, start time and command name of every tracked process; never a command line) is saved to `<data>/sessions/leftovers.json` (mode 0600) whenever it changes, and at the next start the survivors (same PID and start time) are listed as "closed, still running" with Force stop. **FU-A2-01a**: `Members()` must not wait for a whole `KillSession` (in group B this task may edit `internal/pty/pty_unix.go` for it: a separate kill mutex, `k.mu` only around each listing).
- Owns: `internal/proctrack/`, `internal/app/proctrackservice.go`, mock `proctrack.ts`.
- Deps: A2-01.
- Accept: a session whose tool started `sh -c "sleep 600 &"` and a server that calls `setsid`, then exited, reports 2 leftover processes; `ForceStop` leaves none; a process started by another session or by the user's own Terminal is never in the set, even when it is a child of the same shell binary.
- Tests: fake process tables for re-parenting, `setsid`, PID reuse (same PID, later start time), a process that ignores SIGTERM; the snapshot uses one process listing per tick; Windows job enumeration on the VM.
- Review: can `ForceStop` ever signal a process the session did not start? Command lines are shown in the UI but never written to the log.

**A2-06 · Workspace frame** (L)
- Build: design 4.2: layout switch, header, rail, six tabs (placeholders where a later milestone fills them), status bar, bottom panel shell, working-tree switcher with only the main workspace for now, repository settings sheet (binding, identity, AI defaults). Opening a repository from the Projects page navigates to the workspace (`openRepo` in `pages/projects/ProjectsPage.tsx` reveals the folder until then). The rail's update pip uses `useUpdateIndicator()` from `pages/update/useUpdate.ts`.
- Owns: `frontend/src/workspace/`, `frontend/src/state/workspace.tsx`, locale `ws.ts`.
- Deps: F1-08.
- Accept: enter and leave motion per design 3; minimum-size behaviour; the Windows frame variant.
- Tests: route and state tests; keyboard map (design 4.3).
- Review: per-folder state model is in place for milestone 3.

**A2-07 · Terminal component** (M)
- Build: `frontend/src/terminal/`: xterm.js wrapper, pull-by-offset client with re-sync, a registry that keeps instances mounted across page, tab and folder switches, Unicode 11 width, `windowsPty`, Shift+Enter mapping (sends ESC CR), OSC 52 off, link rules, paste confirm, search, screen-reader mode, theme from `--term-*`. From S0-01: while replaying history (attach, reload, after a gap) the page drops everything xterm emits through `onData` **and** `onBinary` (replayed queries would be answered into the live child as typed input); on a gap it resets xterm, writes the prologue, then the data, then nudges the child with a size change (cols-1, 120 ms, back) at most once every 2 s; the pull gate is per terminal (active tab or IntersectionObserver), not `document.visibilityState` alone; the loop restarts on `visibilitychange`, never on a timer (timers run about once a second while hidden).
- Owns: `frontend/src/terminal/`, locale `term.ts`.
- Deps: S0-01.
- Accept: the S0-01 acceptance list, in the real app, including checks A1 (Chinese input) and A2 (Shift+Enter); only visible terminals pull.
- Tests: client re-sync logic; replies dropped during replay on both channels; nudge rate limit; link classification; key mapping (Shift+Enter sends ESC CR).
- Review: no terminal text is ever turned into HTML.

**F1-GF2 · Projects rows fit the window** (S). Added 2026-11-17 by the orchestrator (follow-up FU-GFa, found by F1-GF's verifier; it was on codepass before F1-GF): the Projects row's actions column is cut off at window widths from 980 to about 1101 and from 1181 to about 1281. That includes the default window (1200x780) and the 980x660 minimum, so the More button and its row menu cannot be reached with the mouse.
- Build: the Projects table fits every width from 980 up, in both languages: the narrow template's breakpoint (`@media (max-width: 1180px)` in `frontend/src/pages/projects/projects.css`, about line 119) moves to where the wide template stops fitting (about 1290px), and the narrow template fits at 980 (smaller minimums, the branch column hidden below about 1100px, or a sideways scroll inside `.proj__body > div` that keeps its vertical clip for the collapse).
- Owns: `frontend/src/pages/projects/projects.css` and a new test file.
- Accept: at 980x660, 1100x780, 1200x780, 1281x800 and 1440x900, light zh-CN and dark en, every row's More button is fully visible and opens its menu; nothing else on the page changes.
- Tests: a check that the narrow template's column minimums fit its width (or screenshots at each width when jsdom cannot measure, named in the result).
- Review: the collapse animation still clips vertically; no row content overlaps.

**Group C (serial).** Orchestrator, 2026-11-17: A2-08 runs first, beside F1-SG (they share no file); A2-W runs alone after them. Both A2-08 and A2-W read `reviews/m2B-wiring.md` (what group B built and what it left to wire).

**A2-08 · Task lifecycle** (L)
- Build: `internal/tasks`: `tasks.json` (records hold the repository's stable ID from F1-18), start / stop / resume by session ID (**the app creates the ID when the task is created**: Claude Code `--session-id <uuid>`, OpenCode `--session ses_<id>`; Codex's is captured from its hook or `notify`; hook capture is the cross-check), states from A2-05, soft cap, main-workspace isolation only, wiring to keep-awake and to the window-close hook of F1-03, notifications when a task waits, and the first-time "Codepass 还在运行 / Codepass is still running" notification when closing the window hides it (design 4.4; assigned at the group-B merge). Calls F1-03's `SetTasksRunning` from cached state only (the hook runs on the UI thread). One data folder per task (`<data>/sessions/<task-id>/`: the hook plugin, the saved scrollback; later the handoff notes of milestone 8). `<data>/sessions/` is excluded from Time Machine like the worktrees folder (plan 3.1, second batch), because what it holds is conversation and terminal text. A session whose tool has exited or whose terminal was closed while A2-13 still reports processes is "closed, still running". `LiveSummary()` (sessions, state, programs started) feeds the quit dialog; "stop all" stops every task and runs `ForceStop` for every session before the app exits; "keep running" hides the window. Names a task at Start (plan 4.7 "Naming"): the record keeps the title, the slug and whether the name came from the AI or the fallback; Rename changes the label only. Calls `setRunningTasks(n)` (`frontend/src/pages/update/runningTasks.ts`) whenever the number of running tasks changes; it drives the update dialog's "Reopen when no task is running".
- Owns: `internal/tasks/`, `internal/api/tasks.go`, `internal/app/taskservice.go`, mock `tasks.ts`.
- Deps: A2-02 to A2-05, A2-13.
- Accept: quitting with "stop all" leaves no process of any task; tasks reappear as stopped after restart and resume into their own session; keep-awake is released only when the last task and the last leftover process are gone.
- Tests: lifecycle state machine; restart recovery; cap.
- Review: races between exit, stop and state events.
- Follow-ups from group B (full text in `reviews/m2B-wiring.md`; in group C, A2-08 may edit `internal/terminal/` and `internal/taskstate/` for them): FU-A2-02b (the scrollback archive cache keeps serving the previous run after a resume), FU-A2-02d (a reused session ID's new log starts at the previous tail, so a page that followed the old run continues with no gap and no prologue), FU-A2-05a (helpers that run while the login shell starts make a task look ended, then running with no signal), FU-A2-05b (a hook report after a fallback end revives the task only for three events), FU-A2-05c (`OwnerInput` only for keystrokes that can answer, never for focus reports or terminal replies).

**F1-SG · Tests never run the owner's shell** (S). Added 2026-11-17 by the orchestrator (follow-ups FU-A2-04a and FU-A2-04b, found by A2-04's reviewer).
- Why: `go test ./...` starts the owner's login zsh. `execx.Command` builds the child environment through `EnvPATH`, which on macOS runs `$SHELL -l -c` with the real HOME unless the test called `Configure(Options{DisableLoginShell: true})`, and only a few test packages do. So tests read the owner's `~/.zshenv`, `~/.zprofile` and `~/.zlogin`, against the rule that no test runs the owner's shell config.
- Build: one guard, like `internal/awsguard`: in a test binary (`testing.Testing()`), (1) execx's login-shell lookup (`loginShellPATH`, `retryInteractive`) never runs unless the test opted in through a helper that gives it a temp HOME and ZDOTDIR with their own shell files; (2) every place that starts a login or interactive shell (`execx.Command`, `internal/pty`'s `Start`, `internal/aitools` detection, `internal/terminal`'s launch, and any other a search for `-l`, `-i`, `-lic` and `SHELL` finds) refuses, in a test binary, a shell started with `-l` or `-i` whose HOME (and ZDOTDIR for zsh) is not inside a temp folder, records the refusal and returns an error. Production behaviour does not change. Fix the tests that relied on the owner's shell. FU-A2-04b: `TestWindowsWithoutLookupKeepsBareNameOffWindows` must fail when the code goes back to `execx.LookPath` on a host without the tool.
- Owns: `internal/execx/`, a new `internal/shellguard/` if the guard needs its own package, the guard call in `internal/pty/` and in the other places it names, and the tests that need the opt-in.
- Accept: the whole `go test ./...` run with HOME and ZDOTDIR pointing at a trap folder (a new temp folder whose `.zshenv`, `.zprofile`, `.zshrc`, `.zlogin`, `.bash_profile` and `.bashrc` each write a marker file; GOCACHE and GOPATH set explicitly so the build cache is reused) leaves no marker, and the gate passes. The owner's real home is never used for this.
- Tests: the guard refuses a login shell with a non-temp HOME in a test binary and records it; the opt-in helper works; the guard is inert when `testing.Testing()` is false.
- Review: no test can start a shell that reads the owner's files; the guard cannot change production behaviour.

**A2-W · Wiring** (S): registers the services, mocks, locales, the workspace route (A2-06 adds the route in group B; A2-W keeps it), the rail and sidebar changes (AI tasks item, keep-awake chip); the "Switch session" command (Cmd+J; Ctrl+J on Windows, Ctrl+Shift+J while a terminal has focus) in the menu and the command table, and the quit dialog's mount point, both with placeholders that A2-14 and A2-15 fill; checks the xterm packages A2-07 added in group B; removes the Terminal helper page and `internal/terminal`'s old API users. Owns the shared files. Deps: A2-06 to A2-08, A2-13. Group B (2026-11-17; `reviews/m2B-wiring.md` lists every item): register the services and events of A2-02, A2-03, A2-04, A2-05 and A2-13 (`terminal:ping` among them); the report-mode dispatch in `main.go`; one terminal manager in the Container with the proxy environment for its children; `QuietBindingLogger` for the Wails binding log in `main.go`; the mock terminal service; the Settings fields (keep-awake's stored shape lives in `internal/api`, because `keepawake.Stored` inside `api.Settings` is an import cycle: FU-A2-03d); the keep-awake to tracker connection (never call back into the keep-awake Manager from `OnChange`: FU-A2-03f) and the wake hook. Then regenerate the bindings (not done after group B on purpose) and check that `aitoolservice.ts` and the AI model types are still generated. `i18n/index.ts` already registers the `ws` locale (A2-06): keep it (FU-A2-06f).

**Group D (parallel, after C).**

**A2-09 · AI tasks tab and new-task sheet** (L)
- Build: design 5.6: task list, states, task header, session tabs, the session bar, install card, ended bar, "what a task can do" sheet, the fifth-task confirm; the "已关闭，仍在运行 / Closed, still running" group with process count, CPU and "强制停止 / Force stop" (design 5.6a); the state "运行中（没有信号）/ Running (no signal)" for a tool without hooks; the new-task sheet with "main workspace" only enabled (own workspace arrives in milestone 3): a description box instead of a name and the "正在起名… / Naming the task..." state (design 5.6 as revised 2026-11-16). The quit dialog is A2-14.
- Owns: `frontend/src/workspace/ai/`, locale `tasks.ts`.
- Accept: matches `workspace-ai-light-zh`, `workspace-ai-dark-en`, `new-task-light-zh`, `new-task-light-en`, `new-task-naming-light-zh` (except the disabled option).
- Tests: ordering, state rendering, sheet validation; a leftover row appears and disappears with the tracker's count; Force stop asks once and names the programs.
- Review: nothing is drawn inside the terminal; no AWS_PROFILE badge.

**A2-10 · Terminal panel** (M)
- Build: the bottom panel's Terminal tab: shells per repository and folder, tabs, resize, remembered height; "Open in system terminal" as a repository action.
- Owns: `frontend/src/workspace/panel/Terminal*.tsx`.
- Accept: a shell never raises keep-awake; Ctrl+` toggles from anywhere in the workspace.
- Tests: panel state.
- Review: focus rules (design 4.3, last paragraph).

**A2-11 · Keep-awake UI and notifications** (M)
- Build: the chip (sidebar, rail, status bar), the Settings card (design 5.14), the AI tasks banner text, system notifications for waiting tasks.
- Owns: `frontend/src/shell/AwakeChip.tsx`, `frontend/src/pages/settings/KeepAwakeCard.tsx`, `frontend/src/pages/settings/AiToolsCard.tsx`, locale `awake.ts`.
- Accept: the chip is visible exactly while the assertion is held; limits text per platform.
- Tests: mapping from service state to UI.
- Review: the UI never claims more than the OS does.

**A2-12 · Scripted terminal mock** (S)
- Build: a mock terminal that replays a recorded, generic Claude Code session by offsets, and mock tasks matching design 7.
- Owns: `frontend/src/api/mock/terminal.ts`, `frontend/src/api/mock/recordings/`.
- Accept: the mock exercises the same client path as the real service.
- Review: the recording has no real names.

**A2-14 · Quit dialog and background mode** (S)
- Why (owner, 2026-11-16): quitting with live AI sessions always asks, every time, with two choices and no "don't ask again".
- Build: design 5.6b. The dialog F1-03's `quitDecision()` hook opens for Cmd+Q, the menu, the Dock and the tray whenever `LiveSummary()` is not empty (a live session, or a closed one with leftover processes): the list of sessions, "全部停止并退出 / Stop all and quit", "留在后台继续运行 / Keep running in the background" (default focus), Cancel on Esc. Background mode hides the window; the app stays in the Dock (macOS) or the tray (Windows); clicking the icon brings the window back. During logout, restart or shutdown no dialog is shown: "stop all" is taken at once through F1-03's `SetLoggingOut` path. The update dialog's "Reopen now" does not ask a second time. No setting, no remembered answer.
- Owns: `frontend/src/shell/QuitDialog.tsx`, locale `quit.ts`.
- Deps: A2-W, A2-08.
- Accept: matches `quit-dialog-light-zh`; with one live session the dialog appears on every one of five quits in a row; with none the app quits at once; after "Keep running" the sessions still receive output and keep-awake stays held.
- Tests: the three outcomes; the logout path takes "stop all" without a dialog; a cancelled logout does not leave Cmd+Q in logout mode (follow-up FU-03a); no code path stores a choice.
- Review: nothing in settings or storage can suppress the dialog; logout is never blocked.

**A2-15 · Session switcher (Cmd+J)** (M)
- Why (owner, 2026-11-16): several sessions run in several repositories at once (repository A: session 1; repository B: sessions 2 and 3). Every live session must be reachable from one list, from any screen.
- Build: design 5.7a. An overlay on Cmd+J (Ctrl+J on Windows; Ctrl+Shift+J while a terminal has focus there, because Ctrl+J is a line feed that terminal programs use), available in both layouts and over any page. Rows: tool mark, task title, repository (`RepoName`) and branch or "main workspace", state, elapsed time, a digit. Groups in this order: waiting for you, running, closed but still running, ended in the last 10 minutes. Enter opens that repository's workspace on the AI tasks tab with that session's terminal focused; Cmd/Ctrl+Enter opens its changes; typing filters by title, repository or branch; pressing Cmd+J again moves to the next row; 1 to 9 jump directly. The first selected row is the first waiting session that is not already on screen.
- Owns: `frontend/src/shell/SessionSwitcher.tsx`, `frontend/src/state/sessions.ts` (the one sorted cross-repository list; C3-08 reuses it), locale `sessions.ts`.
- Deps: A2-W, A2-08.
- Accept: matches `session-switcher-light-zh` and `session-switcher-dark-en`; with sessions in three repositories, Cmd+J then Enter lands in the right terminal with the cursor in it, from the Projects page, the Accounts page and another repository's Code tab; focus returns to where it was on Esc.
- Tests: sort order; filter; keyboard map on both platforms; a session that ends while the list is open updates in place without moving the selection.
- Review: `role="dialog"` with a `listbox`; the shortcut never swallows Ctrl+J inside a terminal on Windows.

**Group E (alone, after D).** Added 2026-11-16 (owner, second batch).

**A2-16 · Open repository from any screen (Cmd/Ctrl+O)** (S)
- Why (owner, 2026-11-16): find the repository to work in by typing its folder name, and start work at once.
- Build: design 5.7c and plan 4.4. **Fable first**: draw the panel in the mockup from the session switcher's drawing and take `repo-switcher-light-zh` and `repo-switcher-dark-en`. Then the overlay, reusing A2-15's panel, list and keyboard code: recent repositories first (the last 5 opened; `RepoService.MarkOpened` already records opening), then A to Z; typing filters with the ranking F1-CL built for the Projects search (one shared function); Enter opens the workspace on its last tab, Cmd/Ctrl+Enter opens it with the new-task sheet, 1 to 9 jump, Esc returns focus; hidden repositories left out, a gone folder dimmed and sent to its Projects row. Cmd/Ctrl+O in the command table and the app menu "打开仓库… / Open Repository…"; on macOS also while a terminal has focus, on Windows not inside a terminal.
- Owns: `frontend/src/shell/RepoSwitcher.tsx`, locale `reposwitch.ts`, the menu item in `internal/app/menu*.go` and the command-table entry (it runs alone in its group, so it may touch these shared files).
- Deps: A2-06 (workspace route), A2-09 (new-task sheet), A2-15 (panel), F1-CL (ranking).
- Accept: matches the two new shots; from the Projects page, the Accounts page and another repository's workspace: Cmd+O, three letters of a folder name, Enter lands in that repository's workspace; Cmd+Enter opens the new-task sheet there; on Windows, Ctrl+O typed in a terminal reaches the terminal program.
- Tests: ranking shared with the Projects search; recent order; keyboard map on both platforms; gone and hidden repositories.
- Review: `role="dialog"` with a `listbox`; the shortcut never swallows Ctrl+O inside a terminal on Windows.

**Group F (alone, after E).** Added 2026-11-17 by the orchestrator: milestone 1's clean-up (F1-CL) left follow-ups of its own, and milestone 2's groups will leave more.

**A2-CL · Milestone 2 clean-up** (S)
- Build: fix every follow-up marked "A2-CL" in the follow-up list: FU-CLa1 to FU-CLa4 and FU-CLb1 to FU-CLb7 from group H, FU-GFb, FU-GFc and FU-A2-01c from group I, the group B items marked A2-CL in `reviews/m2B-wiring.md` (about 25: terminal modes, keep-awake tests, Windows parent IDs, the Projects table's fix buttons, workspace details and six terminal component items, the paste one first), plus the ones milestone 2's later groups add. Add a test wherever the follow-up asks for one. Also the milestone 1 gate's D27 (`reviews/m1-gate-review.md`): Settings becomes the two-column card grid of design 5.14 (`.set-grid`), with every card milestone 1 and milestone 2 built mounted in it; and the problems the owner's open milestone 1 checks find.
- Owns: the files those follow-ups name. It runs alone in its group, so it may touch the shared files. If the list grows past about 40 items, split it by file like F1-CL (Go and frontend).
- Accept: every A2-CL follow-up is fixed, or declined with evidence; the gate passes.
- Tests: each follow-up's own test fails without its fix.
- Review: each item against its follow-up text; no new behaviour beyond the list.

**Gate.** Standard. Screenshots: workspace-ai (2), new-task (2), settings (2), session-switcher (2), repo-switcher (2), quit-dialog (1). Manual: three tools, two sessions each in two repositories, window hidden for 30 minutes with output flowing; a session closed while its dev server runs shows as "closed, still running" and Force stop ends it; the keep-awake matrix on the owner's Mac (`pmset -g assertions`); Windows VM: one session per tool (B8, B10).

---

## Milestone 3 - Worktree tasks, Changes and commit (5 weeks)

**Goal.** Parallel AI tasks that cannot disturb each other, and the review-and-commit loop.
**User-visible result.** Each new task gets its own folder and branch. The owner reviews what a task changed, stages by file, hunk or line, commits, and merges, pushes or discards the task. Ended sessions show how much disk they hold and can be cleaned up after a preview of every path. A repository's local folder can be renamed or moved inside the app.

**Group A (parallel).**

**C3-01 · Task workspaces** (M)
- Build: `internal/worktree`: create under `~/.codepass/worktrees/<local folder name>/<slug>` (the slug comes from the naming, plan 4.7) (a second repository with the same folder name gets `<name>-2`), `.worktreeinclude` copy, submodules, setup command, remove, orphan detection and prune at startup, Time Machine exclusion; size split into tracked files and everything else (build output, `node_modules`); `Repair(repo)`: `git worktree repair` for every app-managed workspace of a repository after its main folder moved (C3-10).
- Owns: `internal/worktree/`.
- Accept: a taken branch name gets `-2`; removal never touches a folder outside the root; after the main folder is moved and `Repair` runs, `git status` works in every task workspace.
- Tests: create / remove, include patterns, orphan cases, path-escape attempts, repair after a move, same-named repositories.
- Review: path handling; the setup command runs as an argument list in the task's shell.

**C3-02 · Diff and staging** (L)
- Build: `internal/gitdiff` (parse `git diff` into files, hunks, lines; renames; binary), `internal/gitstage` (stage / unstage file, hunk and line through generated patches and `git apply --cached`).
- Owns: `internal/gitdiff/`, `internal/gitstage/`, `internal/api/changes.go`.
- Accept: staging any subset of lines round-trips to the expected index content, including files without a trailing newline and CRLF files.
- Tests: golden diffs; property test (random line subsets vs a reference).
- Review: patch generation edge cases; reads use the read-only runner.

**C3-03 · Commit and discard** (M)
- Build: `internal/gitcommit` (commit, amend with the pushed warning, hook failure output, signing-failure classification) and `internal/gitdiscard` (discard file / hunk / line / task with recovery refs under `refs/codepass/discarded/*`, list, restore, 14-day prune; untracked files to the system trash).
- Owns: `internal/gitcommit/`, `internal/gitdiscard/`.
- Accept: every discard can be restored; nothing is copied outside the repository.
- Tests: discard and restore for each kind; prune; hook failure; signing failure.
- Review: hooks run only here (a user action); the recovery refs are not pushed.

**C3-04 · Diff view component** (M)
- Build: CodeMirror base setup and theme mapping to `--syn-*`; the unified and side-by-side diff with hunk headers, line selection and the floating line bar; collapsed unchanged runs.
- Owns: `frontend/src/editor/base/`, `frontend/src/diff/`.
- Accept: a 20,000-line diff scrolls smoothly; colours pass the contrast test.
- Tests: selection-to-line-range mapping; rendering snapshots.
- Review: no HTML from file content.

**Group B (parallel; then C3-W alone).**

**C3-05 · Task isolation and finish actions** (L)
- Build: `internal/tasks`: own-workspace isolation; "changes against base"; merge flow of plan 4.9 (base into the task, then fast-forward by the three cases); push branch; discard with recovery; keep; clean up finished tasks.
- Owns: `internal/tasks/` (new files `isolation.go`, `finish.go`), `internal/app/changesservice.go`, mock `changes.ts`.
- Deps: C3-01 to C3-03.
- Accept: merging works when the main workspace is on another branch and when the base is not checked out anywhere; the owner's uncommitted work is never stashed without a click.
- Tests: the three fast-forward cases; conflict in step 1; task still running.
- Review: the main workspace is untouched during step 1.

**C3-10 · Rename or move a repository's local folder** (L)
- Why (owner, 2026-11-16): the local folder may be named differently from the remote repository (remote `AAA`, local `BBB`), and that is managed inside the app.
- Build: `internal/repomove`: one transaction, `Plan(repoID, newName, newParent?)` then `Run(plan)`.
  - `Plan` reports what will be stopped (AI sessions and their state, terminals, language servers, watchers) and what will be updated, and refuses early: target exists; invalid name; an operation in progress (merge, rebase); a task in "starting"; the target is on another volume (a cross-volume move is a copy plus a delete and cannot be undone atomically: not offered in 1.0, the message says to move it in Finder or Explorer and re-attach it); the target is inside the repository or inside `~/.codepass`.
  - `Run`: (1) quiesce through a registry of stoppers that each subsystem registers (`Stop(repoID)` / `Resume(repoID)`): tasks stop the AI sessions and run A2-13's `ForceStop`, terminals close, watchers pause (F1-05), language servers shut down (registered by E5-03 in milestone 5); (2) a journal entry with the old and new path is written; (3) `os.Rename` of the folder (one system call on one volume); (4) `Relocate(id, newPath)` in the registry (F1-18), `Repair` for the task workspaces (C3-01), task records and scan folders updated (a folder moved outside every scan folder becomes an extra repository); (5) resume watchers; sessions are **not** restarted, they show as stopped with "Resume". If any step after (3) fails, the folder is renamed back and every record is restored from the journal; a journal found at startup is finished or rolled back before the repository list loads.
  - Not touched: the remote URL, branches, bindings, identity (none holds the path); task workspaces stay where they are. `Plan` warns when a conditional rule in the user's git config points at the old path (from F1-13's source data) and when a Claude Code session ran in the main workspace (that tool files conversations by folder path, so Resume may start a new conversation: check A6).
- Owns: `internal/repomove/`, `internal/tasks/relocate.go`, `internal/projects/relocate.go`, `internal/app/repomoveservice.go`, `internal/api/repomove.go`, mock `repomove.ts`.
- Deps: F1-18, F1-05, A2-08, A2-13, C3-01.
- Accept: remote `acme_api` renamed to local `api-main` with two tasks in their own workspaces and one terminal open: afterwards the project, binding, identity override, hidden flag, task records and both workspaces work, and nothing else on disk changed; a failure injected at each step leaves the folder under its old name and the records byte-identical; on Windows the rename succeeds with no "folder in use" error (R7-01).
- Tests: the transaction with a failing step at every position; journal recovery after a simulated crash between (3) and (4); refusal table; a stopper that fails to stop aborts before (3).
- Review: nothing is lost on any path; no step runs while a tracked process of that repository is alive; the path never reaches shell text.

**C3-12 · Session disk use and clean-up** (L)
- Why (owner, 2026-11-16): free the disk space ended sessions hold, without ever touching a running session or a file that is not that session's.
- Build: `internal/sessionclean`. Per ended task, three measured parts: (a) its workspace (tracked files, and everything else such as build output and `node_modules`: sizes from C3-01); (b) the app's own data for it (`<data>/sessions/<task-id>/`: saved scrollback, hook plugin); (c) the AI tool's own records **for that exact session ID**, through one adapter per tool in `internal/aitools/transcripts.go`:
  - Claude Code: `~/.claude/projects/<folder slug>/<session-id>.jsonl`, the folder `<session-id>/` beside it (sub-agent records) and `~/.claude/file-history/<session-id>/`. The paths are built from the stored session ID and the task's folder; a path is listed only if it exists and its name contains the full ID. The exact layout is confirmed by check A6 before this task starts.
  - Codex: files under `~/.codex/sessions/` whose name ends in `-<session-id>.jsonl` (layout confirmed by check A8).
  - OpenCode: no files are deleted. Its data is a database, so the adapter runs `opencode session delete <id>` (S0-03 used it) and the size is shown as "kept by OpenCode".
  `Preview(taskIDs)` returns every path with its size and kind; `Delete(preview)` deletes exactly the previewed paths, re-checking each one first (still inside its expected root, not a symlink leading out, task still ended). A task that is running, waiting, starting or has leftover processes is never eligible. A workspace with commits that are in no other branch and no remote needs its own confirmation and goes through C3-03's discard (commits recoverable for 14 days under `refs/codepass/discarded/*`; uncommitted files are not recoverable, and the preview says so). Deleting removes the task record last.
- Owns: `internal/sessionclean/`, `internal/aitools/transcripts.go`, `internal/app/sessioncleanservice.go`, `internal/api/sessionclean.go`, mock `sessionclean.ts`.
- Deps: A2-08, A2-13, C3-01, C3-03.
- Accept: with two Claude Code sessions in one folder, cleaning one leaves the other's transcript byte-identical; nothing outside the previewed list changes on disk (asserted by a before / after listing of `~/.claude`, `~/.codex` and the worktree root in a temp home); a running session cannot be selected through the API; an unmerged workspace is refused without the extra confirmation.
- Tests: adapters against fixture trees (a session ID that is a prefix of another; a symlink inside the transcript folder; a missing folder); preview equals what delete removes; the recovery ref exists after an unmerged delete; sizes of sparse and hard-linked files.
- Review: checklist item 11; no glob or prefix match on a session ID; no deletion by a path taken from the frontend.

**C3-W · Wiring** (S): services, mocks, locales, CodeMirror packages; the "Rename local folder" and "Free up space" commands. Deps: C3-04, C3-05, C3-10, C3-12.

**Group C (parallel).**

**C3-06 · Changes tab and commit composer** (L)
- Build: design 5.4: groups, rows, diff with hunk and line staging, composer with identity line and amend, discard confirms with recovery wording, "Recently discarded" group, all states.
- Owns: `frontend/src/workspace/changes/`, locale `changes.ts`.
- Accept: matches `repo-commit-light-zh`, `repo-commit-light-en`, `repo-commit-dark-zh`.
- Tests: stage / unstage flows against the mock; composer validation.
- Review: FLIP motion on stage moves; keyboard path.

**C3-07 · Working-tree switcher and task review** (M)
- Build: the switcher menu, the task stripe, per-folder state (tabs, selection, panel), the sync button following the folder ("Publish branch"), the task bar and the review layout of design 5.5 with the merge, push and discard dialogs.
- Owns: `frontend/src/workspace/TreeSwitch.tsx`, `frontend/src/workspace/TaskBar.tsx`, `frontend/src/workspace/review/`.
- Accept: matches `workspace-changes-light-zh`, `workspace-changes-dark-en`; no branch switching inside a task folder.
- Tests: per-folder state restore; dialog copy for the three fast-forward cases.
- Review: it is always clear which folder is on screen.
- Also FU-A2-06e (group B; `reviews/m2B-wiring.md`): `WorkspacePage.tsx` about line 79 snaps a task folder back to main when it is selected before the repository state lists its worktree.

**C3-08 · AI tasks page** (M)
- Build: design 5.7: cross-repository tables on the shared session list of A2-15 (`state/sessions.ts`), the keep-awake banner, the "closed but still running" section with "Force stop" and "Force stop all" (design 5.6a), the sidebar count and repository badges, the header buttons "切换会话 / Switch session" and "释放空间 / Free up space" (opens C3-13's dialog; it replaces the old "Clean up finished tasks").
- Owns: `frontend/src/pages/aitasks/`.
- Accept: matches `ai-tasks-light-zh`, `ai-tasks-light-en`, `ai-leftover-light-zh`; a repository whose folder name differs from its remote name shows both.
- Tests: grouping and counts; the leftover section appears only while the tracker reports processes.
- Review: Force stop names the programs it will end; the page never offers to delete anything itself (that is the Free up space dialog).
- Owner requirement of 2026-11-16 (all live sessions in one list, plus a global switcher): the switcher is A2-15 (milestone 2); this page is the full list.

**C3-11 · Rename local folder dialog** (M)
- Build: design 5.2a: from the repository row menu and the repository settings sheet. Remote name (fixed) beside the local folder name (editable), the old and new path with "Choose another parent folder...", the two lists from `Plan` ("stopped first", "updated to match"), the warnings, the promise that a failure puts everything back; progress by step; the result toast with "Undo" (renames back through the same transaction while nothing new has started).
- Owns: `frontend/src/pages/projects/RenameFolder*.tsx`, locale `rename.ts`.
- Deps: C3-W, C3-10, F1-19.
- Accept: matches `rename-folder-light-zh`; the primary button reads "停止并重命名 / Stop and rename" when anything will be stopped and "重命名 / Rename" otherwise; every refusal of `Plan` has its own sentence; after success the row shows both names.
- Tests: validation; the lists follow the plan data; refusal copy table; focus starts in the name field with the name selected.
- Review: nothing is stopped before the click; the dialog says exactly what stops.

**C3-13 · Free up space dialog** (M)
- Build: design 5.7b: the dialog over the AI tasks page and from a finished task's row menu ("删除这个会话的数据 / Delete this session's data" opens it with that one row ticked). The table of ended sessions with the three sizes and the total, the extra confirmation for a workspace with unmerged commits, the full list of paths with sizes (never truncated; long paths wrap), the footer total and the danger primary button. Running sessions are not listed. Sizes load progressively ("正在计算… / Measuring…").
- Owns: `frontend/src/aisessions/cleanup/`, locale `cleanup.ts`.
- Deps: C3-W, C3-12.
- Accept: matches `free-space-light-zh`; the number of listed paths equals the number deleted; the button is disabled until sizes are known and at least one row is ticked.
- Tests: selection and totals; the unmerged row cannot be ticked without its confirmation; an error on one path leaves the rest of the report readable.
- Review: the list shown is the list sent; copy says "does not go through the Trash".

**C3-09 · New-task sheet: own workspace** (S)
- Build: enable the recommended option with the line saying the branch and folder are named from the description, the warn line for a second task in the main workspace, the submodule and setup note, the "preparing" state.
- Owns: `frontend/src/workspace/ai/NewTask*.tsx` (handed over from A2-09).
- Accept: matches the `new-task` shots fully.
- Review: defaults remembered per repository.

**Gate.** Standard. Screenshots: repo-commit (3), workspace-changes (2), ai-tasks (2), ai-leftover (1), free-space (1), rename-folder (1), new-task (2). Manual: three tasks at once in one repository, each merged by a different fast-forward case; an AI `git commit` in a task folder while the app refreshes (no `index.lock` failure in 200 commits); rename a repository's folder with a task in its own workspace, then resume the task; free up space for two ended sessions and compare the disk before and after against the preview.

---

## Milestone 8 - Ticket queue, AI accounts and usage (8 weeks) - runs after milestone 3, before milestone 4

Added 2026-11-16 (owner, second batch). It has the last number but runs right after milestone 3, because it needs AI tasks (milestone 2) and task workspaces (milestone 3), and the owner runs into the weekly usage limit today. Plan 4.15 (notifications), 4.16 (AI accounts and usage: "Choosing the account", "Switching keeps the conversation", "Several accounts within the terms"), 4.17 (tickets and the queue); design 4.2 (AI account rows in the Repository settings sheet), 5.2 (the project's "AI 账号 / AI accounts"), 5.6 (the "换账号 / Switch account" menu, the limit banner, the New task account row), 5.7, 5.7a, 5.15, 5.16, the queue notices in 6; research `docs/research/claude-usage-accounts.md` (Claude Code and Codex; R8 carrying a conversation, R9 terms and ban risk).

**Goal.** Ideas are written down in Codepass instead of a separate editor and run by themselves, one at a time, inside the usage the owner has. Several accounts per AI tool: any account for any task, a default per project and per repository, several tasks on different accounts at the same time, and a session moved to another account and back at any time. **A switch never loses the conversation** (owner, 2026-11-16: "context 不能丢失，必须能够继续做任务"). Everything stays inside the companies' terms: the owner's own accounts for the owner's own work, no sharing, no reselling, no ban routed around (owner, 2026-11-16: "需要符合官方的使用条规"). The usage Claude Desktop shows today is visible in Codepass. Claude Code and Codex alike; OpenCode tickets run without a usage check and OpenCode has no accounts.
**User-visible result.** A Tickets page: write a ticket in seconds from any screen, keep it as "编辑中 / Editing" or "暂不执行 / On hold", or drag it into "待执行 / Queued". The queue checks the usage, starts the first ticket as an AI task in its own workspace, waits until it is finished, then starts the next; at a limit it waits for the reset, or continues on another account when the owner says so (automatic switching is off by default and can be turned on per tool); when the AI needs the owner it waits and sends a notification. An AI accounts section on the Accounts page; a default account per project and per repository; an account picker in New task; a "Switch account" menu on every session, both ways, any time, that carries the whole conversation over. A usage meter in the sidebar with every account's session and weekly bars and reset times.

**Before this milestone starts** (owner):
1. Read the terms: plan 4.16 "Several accounts within the terms" and 4.17 "Terms", with the quotes in the research note (R3, R5d, R9), check them on the live pages (D23, D24, D25, D27; D26 is optional), and confirm that the queue may start tickets by itself and how several subscriptions are used (open questions 13 to 17). Until then the plan assumes yes, with automatic switching off by default; "Ask me before each ticket" (default off) is the person-in-the-loop setting either way.
2. Run checks D1 to D4 (`manual-checks.md`, about 30 minutes, needs two Claude accounts), the Codex checks D9 to D12 (Codex installed into a private folder, two ChatGPT accounts) and the carry-over checks D13 to D22 (research R8). D1 and D9 decide T8-02's sources and polling; D3, D11 and the R8 checks decide which layer of plan 4.16 "Switching keeps the conversation" carries a conversation for each tool (the same session, a repaired copy, or a handoff note), and therefore how much of T8-11 is the main path rather than the fallback.

**Group A (parallel, backend).**

**T8-01 · AI accounts** (L)
- Why (owner, 2026-11-16): several Claude and Codex accounts; when one is used up, work continues on another.
- Build: `internal/aiaccounts` (plan 4.16). The list per tool in `ai-accounts.json` (id, tool, label, colour, folder, order, last read sign-in state when the tool reports it; plan 3.1 and 3.2 for the exact fields; no email address and no plan name stored), plus `autoSwitch` per tool, **off by default** (plan 4.16 "Several accounts within the terms"). The tool's own account of each tool is its default folder with the variable unset; the app never moves it and writes into it only what plan 4.16 allows (new conversation copies, add-only, with the owner's consent).
  - **Account defaults** (plan 3.2, 4.16 "Choosing the account", owner 2026-11-16: "账号A在repoA使用，repoB的任务可以自由选择账号A或账号B"): `aiAccounts?: { claude?, codex? }` (account IDs) on `Project` and `RepoRecord` in `projects.json`, added as a schema step through F1-02's store. `ResolveDefault(repoID, tool)` returns the account and its level: repository, then project, then app (the first account of that tool in the order); an ID that names no account counts as not set; nothing is applied while the shell decides (decision 20). `MakeAppDefault(id)` moves an account to the front. `DefaultsFor(id)` lists the app level, projects, repositories (hidden ones counted apart) and live sessions for the "默认用于 / Default for" line. A new account goes last in the order, so no default changes. Remove: the preview adds the projects and repositories that fall back, the tickets fixed to the account (emptied through T8-03's store, wired in T8-W) and refuses while a live session runs under it. Task records: `aiAccountId` is empty for OpenCode and while the shell decides, a record from before milestone 8 means the tool's own account; plus `autoSwitchedFrom` and the list of switches with the layer that carried each conversation (plan 3.2). The export (`internal/transfer/`) leaves defaults and accounts out.
  - **Within the terms** (plan 4.16 decisions 29 to 36, owner 2026-11-16: "需要符合官方的使用条规，这个项目是拿来自己使用而不是转售"). The account-problem mark of decision 34: `attention { kind, at }` on the account (the tool's own message is kept in memory only, never in the file, a log or a notification). It is set only by the closed list of signals: `StopFailure` with `authentication_failed` or `billing_error`; `other` with the conversation field `none` at a turn end; a non-zero `claude auth status` or a failed `codex login status`; an authentication error from the usage call. It is **never** set by a conversation error (decision 22: invalid thinking signature, invalid redacted thinking, missing tool result, "Prompt is too long", Codex's encrypted-content or account-check refusal), by a usage call that fails for another reason, or by a network or proxy error. While the mark is set the account cannot be started on or chosen as a switch target, and its sessions are not moved, neither automatically nor from the menu; nothing retries it. "检查一下 / Check again": for the sign-in kind the status call decides; for billing, refused and unknown it shows what the tool says now, and the mark is cleared only when the owner confirms "我已处理好 / I have dealt with it". Also `modelsReplaced { model, usedInstead }` (what the tool reported under this account), `autoSwitchConfirmedAt` per tool (the one-time confirm), and Claude Code's always-linked parts `settings.json`, `plans/` and `tasks/` besides the conversation folders. Nothing in this package automates sign-up or sign-in, reads a token, changes network or device settings per account, or lets anything outside this computer use an account. Add: create `~/.codepass/ai-accounts/<tool>/<slug>/`; after the owner's consent in the sheet, link the shared parts to the default folder (plan 4.16 lists them per tool; an existing file in the way is never overwritten); then open an app terminal that runs the tool's own login (`claude auth login`, `codex login`) with that folder in the environment. Codepass never sees, reads or stores a token. Status through `claude auth status` / `codex login status` in that folder (logged in or not, method, plan or account label when given; never a token). Remove: a preview, then delete exactly that app-made folder (checklist item 11); never the default folder, never a link's target. The account environment for A2-04's launch builder. The login-shell probe of plan 4.16 (which variables the profile sets, never values) and its warnings. A2-08's task record gains the account and the list of switches. C3-12's transcript adapter learns account folders: it resolves the links, so a shared transcript is listed once and deleted once.
- Owns: `internal/aiaccounts/`, `internal/app/aiaccountservice.go`, `internal/api/aiaccounts.go`, mock `aiaccounts.ts`, the account fields in `internal/tasks/` records, the `aiAccounts` fields and their migration step in `internal/projects/`, `internal/aitools/transcripts.go` (account folders).
- Deps: A2-04, A2-08, C3-12.
- Accept: in a temp home with fake `claude` and `codex` binaries: two accounts per tool, each task starts with the right variable; the tool's own folders are byte-identical before and after adding and removing accounts; removing deletes exactly the previewed folder; a profile that sets `CLAUDE_CONFIG_DIR` or `ANTHROPIC_API_KEY` raises the warning; the example table of plan 4.16 "Choosing the account" resolves as written; adding an account changes no resolved default and no task; an export holds no account ID; an account error of each kind above leaves the account in "Needs your attention" and no session moves off it by itself.
- Tests: folder layout and links (no link points outside the tool's own folder; nothing in the way is overwritten); remove preview equals what is deleted; status parsing per tool (fixtures, unknown keys ignored); probe parsing, including the Windows probe of the app's own environment (plan 4.16); launch environment per tool; default resolution per level; a stale account ID; migration of a milestone-3 `projects.json` and `tasks.json`; remove refused while a session runs; `autoSwitch` false and `autoSwitchConfirmedAt` empty in a fresh file and after migration; the mark is set by each listed signal, never by a conversation error or a network error, and cleared only by the owner; `plans/` and `tasks/` are linked after consent.
- Review: nothing reads a credential file or the Keychain; links only after consent; the tool's own folders are never changed except by the add-only rule of plan 4.16; checklist item 11; nothing can route work around an account the tool refused.

**T8-02 · Usage reader** (L)
- Build: `internal/aiusage` (plan 4.16): per account, the windows (5-hour session, weekly for all models, weekly per model) with percent and reset time, and the usage-credits state. Claude Code: `claude -p "/usage" --output-format stream-json` in the account's folder, reading `usage_report` with a tolerant parser (the type is experimental: unknown kinds are kept as "other", a missing window is "unknown"); "rate limited until" is respected. Codex: a short-lived `codex app-server` in the account's `CODEX_HOME`, asked `account/rateLimits/read` (primary and secondary windows with percent used, window length and reset time; plan type; credits), with the field names taken from the schema the installed version generates (`codex app-server generate-json-schema`), never from memory; whether it costs a request is check D9. The schedule follows check D1: at most every 5 minutes per account while a meter is on screen, once before each queued ticket, again after a limit signal; if D1 shows that `/usage` costs a model request, only before tickets and on demand. The helper's environment leaves out `ANTHROPIC_API_KEY` and `OPENAI_API_KEY`. Event `usage:changed`. Never reads a credential, never calls an undocumented endpoint, never logs the helper's output. The API takes a tool and a folder, not the account list, so it does not depend on T8-01.
- Owns: `internal/aiusage/`, `internal/app/aiusageservice.go`, `internal/api/aiusage.go`, mock `aiusage.ts`.
- Deps: A2-04.
- Accept: fixtures of `usage_report` (session, weekly all, weekly per model, credits on and off, a missing window, a "rate limited until" answer) and of the Codex answer map to the model; a fake clock drives the schedule; with D1's result written into plan 4.16.
- Tests: parsers; schedule; environment without API keys; nothing from the helper reaches the log.
- Review: no credential, no undocumented endpoint, no request the plan did not allow.

**T8-03 · Tickets store** (S)
- Build: `internal/tickets` (plan 4.17): `tickets.json`, versioned and backup-first like F1-02. A ticket: id, text (Markdown), repository ID (F1-18) and tool (both may be empty while editing, both required to queue), account (empty = "跟随仓库 / Follow the repository", the default, or one account ID of the ticket's tool; changing the tool empties it; an ID that names no account counts as empty; a call empties every ticket fixed to one account, for T8-01's remove; an export leaves it out), where it runs (own workspace or main workspace), base branch, state, queue position, created and updated, and the run record (task ID, session ID, accounts used, start, end, outcome) with the state history. Debounced autosave; the state rules of plan 4.17; reorder; delete with Undo; whether tickets go into an export (plan 4.17).
- Owns: `internal/tickets/`, `internal/app/ticketservice.go`, `internal/api/tickets.go`, mock `tickets.ts`.
- Deps: F1-02, F1-18.
- Accept: a crash in the middle of a write leaves the old file readable; a ticket without a repository or tool cannot be queued; the order survives a restart.
- Tests: state rules; migration; autosave bursts; a newer `schemaVersion` is refused, not overwritten.
- Review: no ticket text in the log.

**T8-11 · Carry a conversation to another account: repaired copy and handoff note** (L)
- Why (owner, 2026-11-16): "账号A的weekly usage到了上限，我就切换成账号B继续做任务，但是context当中不能够丢失必须能够继续做任务". A switch must never start the work over without its conversation, even when the other account refuses the same session.
- Build: `internal/aicarry` (plan 4.16 "Switching keeps the conversation"; research R8). One reader per tool for the local conversation record (Claude Code's session transcript and its sub-agent files; Codex's rollout file), tolerant of unknown fields and versions. Layer 2, the **repaired copy**: a new file under a new session ID where the tool looks for sessions, with only the parts R8 names as tied to the old account removed and every visible message, tool call and result kept; the original is never changed; the new ID is returned for T8-05 to resume and to record. Layer 3, the **handoff note**, built locally with no AI call: every message of the owner in full, the AI's visible replies, each tool call with its target and outcome, files changed, the last plan or to-do list, the cut-off point, plus the branch's `git status`, diff summary and recent commits (read-only git layer, F1-04) and the ticket or task text; when it is longer than the target window allows, older tool outputs are shortened first, never the owner's messages; written to `<data>/sessions/<task-id>/handoff-<n>.md`, mode 0600. Both kinds of file are new files only, listed on the task, removed by Free up space for that task by exact path (C3-12; checklist item 11), and never enter Codepass's backups, an export or the log.
  - Rules from plan 4.16 decisions 22, 27 and 28 and the review: a **version check** (a conversation file whose shape the reader does not fully understand is never rewritten; layer 3 is used instead); a last tool call without its result is removed from the copy and reported; Claude Code copies drop every `thinking` and `redacted_thinking` block, never only some, and re-link what pointed to an emptied line (research R8-M4); Codex copies drop `reasoning`, `compaction` and `context_compaction` items and strip `encrypted_content`, keep the items a removed compaction stood for, and keep `creator_user_id`, `creator_account_id` and `account_scoped` **byte-identical** (only `id` and `session_id` change); the copy is written with mode 0600 and its folders 0700, whatever the original's mode; every path is recorded on the task **before** it is written (the switch journal of T8-05).
  - The note follows decision 22's content order; when it is shortened, old tool outcomes go first, then older AI replies, never the owner's messages; the full note stays on disk and the shortened one names its path. The new session gets the note **by path** in a short fixed first message, never as a command-line argument holding the note (Windows limits a command line to 32,767 characters; macOS limits arguments to 1 MiB). For a queued ticket the note ends with the queue note word for word (plan 4.17).
  - **Free up space** (extends C3-12's backend; edits its files): the preview covers every ID in `AITask.sessionIds` and every file recorded by a switch, by recorded path only, never by a pattern.
- Owns: `internal/aicarry/`, `internal/api/aicarry.go`, the session-list part of `internal/sessionclean/` (C3-12's package, edited here).
- Deps: A2-08, C3-12 (transcript paths), F1-04.
- Accept: on fixture conversations of both tools (with thinking or reasoning records, tool calls, a turn cut off by a limit, a sub-agent): the repaired copy keeps every visible message and tool call and drops exactly the parts R8 lists; the original file is byte-identical afterwards; the handoff note holds every message of the owner word for word and names the cut-off point; a note over the size limit keeps all of the owner's messages; a fixture from an unknown tool version gives no copy and a note instead; nothing is written outside the allowed places.
- Tests: readers against fixtures from several tool versions (unknown fields kept or skipped, never a crash); the size rule; modes 0600 / 0700; a second session whose ID shares a prefix is never read or copied; a copy is listed by Free up space only from its recorded path; a note is deleted with `sessions/<task-id>/`; an export (`internal/transfer/`) holds no AI account folder, handoff note or conversation copy.
- Review: no conversation text in the log, the socket or an error message; no AI call anywhere in this package; the copy's new ID cannot collide with an existing session.

**Group B (serial: T8-04, then T8-05, then T8-W).**

**T8-04 · Queue runner** (L)
- Build: `internal/ticketqueue` (plan 4.17). One queue for all repositories, strict order, one ticket at a time, never the same ticket on two accounts. Before each ticket: resolve its account when it starts, never when it is queued (its fixed account, else T8-01's `ResolveDefault` for its repository and tool); the usage gate (T8-02: session, weekly, per-model weekly when known, below 100% minus the reserve; never on usage credits unless allowed); "Ask me before each ticket"; another account with room only when auto-switch is on for that tool (off by default); with it off the queue pauses until that account's reset and sends the "paused at a limit" notice, whose one button is "在「个人」上继续 / Continue on Personal" when another account has room. With automatic switching off and another account with room, the paused notice and the queue bar carry "在「个人」上继续 / Continue on Personal": it starts the unstarted ticket there once, or opens T8-09's switch confirm for a running one; nothing is remembered for the next ticket. An account in "Needs your attention" (T8-01) pauses the queue, with automatic switching on or off, and never shows that button; the queue never moves a ticket off it. The ticket and its task record which account was used and why. Start: an AI task through A2-08 and C3-01, with the ticket text plus the visible queue note as its first message, under that account (T8-01). Report mode (A2-05) gains the third enum: for queue sessions it reads only the last line of `last_assistant_message` locally and forwards `done`, `question` or `none`. Finished = `done`. Needs you = `question`, `none`, an `AskUserQuestion`, a permission prompt or an `idle_prompt`: the queue waits, sends the notice through F1-22's service, and the ticket offers "算完成 / Mark done" and "跳过 / Skip". Limits: `StopFailure` `rate_limit` pauses the ticket. One owner of "continue": the effective `autoContinueAtUsageLimit` is read read-only; when it is on, the queue never sends its own continue for a session Claude Code is waiting on (`quota_auto_resume_fired` ends the wait); when it is off, or after `quota_auto_resume_disabled` / `_stale`, the queue continues at the reset with `--resume <id> -- "continue"`. Codex has no limit hook and no automatic wait: the queue asks T8-02 before and after each Codex ticket and at every turn end, treats a usage-limit stop as "paused at a limit", and always owns "continue after the reset"; a Codex session without hooks (check A8) reports its turn end through `notify`, whose payload carries the last assistant message for the marker check, and one with neither signal is "running (no signal)", judged by the usage poll and the process exit. Auto-switch hands the paused session to T8-05. A failed turn stops the queue with the reason. After an app restart a running ticket is shown as interrupted and resumes by session ID when the queue is resumed. Keep-awake (A2-03) holds while a ticket runs; while waiting for a reset the Mac may sleep, and on wake the queue checks again. Pause, resume and stop the queue. Notice kinds registered with F1-22: needs you, paused at a limit (with the optional "Continue on Personal" button), continued on another account, queue finished, and "AI account needs attention" (no button; never the tool's message).
- Owns: `internal/ticketqueue/`, `internal/app/queueservice.go`, `internal/api/queue.go`, mock `queue.ts`, `internal/taskstate/marker.go`.
- Deps: T8-01, T8-02, T8-03, A2-05, A2-08, C3-01, F1-22; the owner's answer to "Before this milestone starts" item 1.
- Accept: with fake tools and a fake clock: three tickets run in order and never two at once; a limit pauses and continues at the reset, with Claude Code's automatic continue on and off; with auto-switch off (the default) the queue pauses and its notice offers the other account, and with it on the ticket continues on account B with its conversation (T8-05); a default changed while a ticket is queued applies when it starts; a ticket fixed to an account moves at a limit only with auto-switch on; an account error stops the queue and nothing moves; "needs you" waits and notifies; a restart in the middle resumes; no ticket starts over the limit or on credits when credits are not allowed; check D5 (the marker on five real tickets) is done before sign-off.
- Tests: the state machine table, one row per signal; `Stop` racing `StopFailure`; clock jumps (sleep); a marker line inside a code block or with extra spaces; OpenCode tickets without a usage gate.
- Review: the queue can never start a second ticket, and never sends "continue" while Claude Code owns it; no ticket or assistant text in logs or in the socket.

**T8-05 · Move a session to another account, keeping the conversation** (L)
- Why (owner, 2026-11-16): switch between accounts back and forth at any time; above all, when account A reaches its weekly limit, continue the task on account B with nothing of the context lost.
- Build: per tool (plan 4.16 "Switch and resume" and "Switching keeps the conversation", decisions 22 to 28). A switch needs no limit and goes either way, as often as wanted: A to B, B to A, A to B again.
  - **Before anything stops**: check the target (`claude auth status` / `codex login status` in its folder, and no account-problem mark); if the check fails, nothing is stopped and T8-09's bar says why. Then the switch is **journalled** on the task (`pendingSwitch`: target account, planned new session ID, every path it will create), and only then are files written; `aiAccountId`, `toolSessionId` and the layer change only once the session runs under the target. At start-up an in-progress switch is rolled back: the task stays on its old account, stopped, with Resume there and Switch account to retry, and its recorded files stay listed for Free up space.
  - **Stopping**: only between turns; a working session stops its turn inside the switch confirm, and the tool is stopped the way Stop stops it (A2-08: the programs it started end too). Nothing starts under the target until A2-13's tracker confirms the tool's process under the old account is gone: a session never runs under two accounts at once.
  - **The layers, in order, automatically**: **(1)** the same session resumed in the same task folder and terminal with the target's environment (`claude --resume <id>` / `codex resume <id>`), with `--model` set to the session's model or to the replacement `modelsReplaced` knows for the target, and `--permission-mode plan` when the last reported mode was plan; when the folders are not linked, that session's files are first copied **add-only**: if the destination already holds an older file of the same session, T8-11's writer writes the newer conversation as a new file under a new session ID and that one is resumed (the older file stays byte-identical; paths built from the stored session ID and listed before copying; checklist item 11); then, only if checks R8-M3 / R8-M9 showed that it helps, the tool's own copy (`--fork-session` / `codex fork <id>`); **(2)** if refused, T8-11's repaired copy under its new session ID; **(3)** if refused too, a new session (with an app-made `--session-id` recorded before it starts) that starts from T8-11's handoff note, passed by path in a short fixed first message, never as an argument holding the note. "Refused" is the closed list of plan 4.16 decision 22: report mode's conversation field (T8-05 adds this fourth enum in `internal/taskstate`, computed locally from fixed strings, text never sent) in the first turn after the switch, or an exit within a few seconds; for Codex the exit plus the locally matched refusal line. A refusal is never an account problem. "Prompt is too long" after a switch goes to layer 3.
  - **No fresh start**: there is no path that starts over without the conversation. If no session can start on the target at all, the switch does not happen: the task stays on the old account, stopped, with its conversation untouched; when that happens to an automatic switch at a limit, the app owns "continue at the reset" for that session (`--resume <id>` with decision 23's message once a fresh reading shows room). The app never answers the tool's own questions after a resume ("Resume this conversation?", a usage-credits question, a replaced model): the session waits for the owner.
  - **Messages** (decision 23): after a switch at a limit, the fixed text (the previous turn stopped at a usage limit; check the workspace with `git status`; continue exactly where it stopped); after a cut-off by the owner ("stop and switch"), the cut-off text that ends with "wait for the owner's next message"; after a switch between turns with nothing cut off, none. "Cut off at a limit" is persisted on the task (`lastTurnCutOff`), so it survives a restart. A refusal in the first turn carries the owner's unanswered last message into the next layer.
  - **Result**: the layer-1 line is reported only once the tool's `SessionStart` reports the expected session ID with source `resume`; a different ID means "Claude Code started a new conversation" (T8-09's line), not success. Each switch is recorded on the task (plan 3.2: from, to, when, by the owner or automatic, `atLimit`, `layer`, `sessionId`, what was removed, `files`) and sets or clears `autoSwitchedFrom`; every new session ID joins `AITask.sessionIds`. A switch never writes a default.
  - **Resume of a stopped task** looks for the conversation under every ID in `sessionIds`, newest first, in the task's account folder and then in every other account's folder; found elsewhere, it is a switch (confirm and layers); found nowhere, no session starts by itself (T8-09's "could not be found" line, with "Start a new session" as the owner's explicit choice). Automatic switching runs only when it is on for that tool (off by default), never because of a default, never while the shell decides, and never away from an account the tool refused (T8-01's "Needs your attention"). Used by the queue (T8-04) and by T8-09's switch menu (session bar and AI tasks rows), the limit banner, the switch-back hint and "Resume on <account>" for a stopped task.
- Owns: `internal/aiaccounts/switch.go`, `internal/tasks/switch.go`, `internal/taskstate/conversation.go` (report mode's fourth enum).
- Deps: T8-01, T8-11, T8-04 (same group, runs after it), A2-04, A2-08.
- Accept: with fake tools: the session keeps its ID on the other account (layer 1); a refused resume goes to layer 2, a refused layer 2 to layer 3, and each says which layer carried the conversation; A to B to A by hand: back on A the AI still has what was said on B, with linked folders and with the add-only copy (whose older file in the destination stays byte-identical); a limit in the middle of a turn and a stopped task resumed on another account end with the conversation carried; an app restart in the middle of a switch rolls back to the old account with nothing lost; a target that is not signed in means the switch does not happen and the original stays byte-identical; a conversation error in the first turn after a switch goes to the next layer and never marks the account; a 200 KB handoff note starts on Windows (passed by path); an account error never triggers an automatic switch; check D3, D11 and the carry-over checks (D13 to D22) are done before sign-off.
- Tests: per tool and per layer; the stop-between-turns guard; the tracker wait before the target starts; copy exactness against a fixture tree with a second session whose ID shares a prefix; the journal and its rollback at start-up; the first-message rule for each case; the refusal list (each fixed string; a network error is not a refusal); `--permission-mode plan` is passed when the last mode was plan; the switch record and `sessionIds`.
- Review: never mid-turn; never another session's files; never a new session without the conversation; the cost sentence is shown before a manual switch; nothing automatic routes around an account the tool refused.

**T8-W · Wiring** (S): registers the services, mocks and locales; the Tickets route and sidebar item; the AI accounts section on the Accounts page; the usage chip; the quick-capture shortcut (design 4.3); the queue's notice kinds with F1-22's service, including "AI account needs attention" (no button, never the tool's message; a click opens Accounts at that pass) and the optional button of "paused for the limit"; T8-01's remove calling T8-03's "empty the tickets fixed to this account". Owns the shared files. Deps: T8-01 to T8-05, T8-11.

**Group C (parallel, UI; each is "Fable first": Fable draws the mockup screens and shots from the design section before the Sonnet task starts).**

**T8-06 · Tickets page** (L): design 5.15. The three owner columns, the queue with its order and gate state, the finished and needs-you areas, the editor with autosave and the visible queue note, quick capture from any screen, drag between columns and inside the queue (with a keyboard path), the queue bar (start, pause, stop; why it waits) and its short form for the AI tasks page header and the sidebar tooltip; the "工单队列 / Ticket queue" card in Settings (design 5.14: reserve, credits, ask before each ticket, the queue note; automatic switching is set per tool on the Accounts page, T8-07); the editor's account field ("跟随仓库 / Follow the repository" or one account; design 5.15), the account named on queued cards and in the queue bar, and the mark for a fixed account that differs from the repository's default; quick capture has no account field; the two queue-bar rows of design 5.15 for "paused, with Continue on Personal" and "stopped because an account needs the owner", the new needs-you reason and the reworded account help line. Owns `frontend/src/pages/tickets/`, `frontend/src/pages/settings/QueueCard.tsx`, locale `tickets.ts`. Accept: matches the new shots (`tickets-light-zh`, `tickets-dark-en`, `ticket-editor-light-zh`, `tickets-needs-you-light-zh`). Tests: state rules in the UI; drag and keyboard reorder; autosave. Review: ticket text is plain text or Markdown with raw HTML off; nothing is queued without a repository and a tool.

**T8-07 · AI accounts section and add-account sheet** (L): design 5.16 as revised. Accounts per tool with status and usage bars; the order line, the "应用默认 / App default" badge, "工具自带 / Tool's own" on line 2, "Make it the app default"; the "默认用于 / Default for" line and the account details sheet; add (the consent step for the shared folders, including the add-only conversation copies of plan 4.16, the Team or Enterprise line, the one plain sentence of what Codepass cannot promise; then the login terminal; a new account goes last, with the one-time suggestion row); remove with the preview (folder, the projects and repositories that fall back, hidden ones counted, the fixed tickets; refused while a session runs); **automatic switching per tool, off by default, with its one-time confirm per tool showing the full terms sentences as published and the live links of design 5.16's link table (never an archive copy), and a "这些条款怎么说 / What the terms say" link beside the switch** (plan 4.16 "Several accounts within the terms"); the "需要你处理 / Needs your attention" pass with "检查一下 / Check again" (per-kind outcome, plan 4.16 decision 34), "重新登录 / Sign in again", "我已处理好 / I have dealt with it" and the "为什么 / Why" popover (the tool's message, in memory only; the sentence per company); the shared-parts label "计划和待办 / Plans and to-do lists"; the shell-decides and profile-override warnings; the line "Codepass 只用于你自己的账号和你自己的工作。 / Codepass is for your own accounts and your own work." with links to both companies' terms where design 5.14 / 5.16 place it. Owns `frontend/src/pages/accounts/ai/`, locale `aiaccounts.ts`. Accept: matches `ai-accounts-light-zh`, `ai-account-add-light-zh`, `ai-account-details-light-zh`, `ai-account-remove-light-zh`, `ai-account-attention-light-zh`, `ai-autoswitch-confirm-light-zh`. Tests: sheet steps; remove confirm lists the folder and what falls back; Remove is refused while a session runs; "Default for" is per tool; automatic switching starts off and asks once before it turns on. Review: no token field anywhere; the consent text names every link and the copy rule; no fear language; nothing implies a guarantee about an account.

**T8-08 · Usage meter** (S): design 4.1 and 5.16. The sidebar chip (per tool, the account in use; with live sessions on several accounts of one tool, the one closest to its limit with "+1" and the others named in the tooltip) and its popover (every account of both tools, "正在使用 / in use" on every account with a live session and those first, reset times, "unknown", credits and "needs your attention" states; an account that needs attention shows that word on the chip itself in place of its numbers). Owns `frontend/src/shell/UsageChip.tsx`, `frontend/src/shell/UsagePopover.tsx`, locale `usage.ts`. Accept: matches `usage-meter-light-zh`, `usage-meter-dark-en`. Tests: mapping from the usage model; reduced motion. Review: the UI never claims more precision than the source gives.

**T8-09 · Session bar: account and switch menu, limit banner; the new-task account row** (L): design 5.6 (as revised for milestone 8), 5.7, 5.7a and 5.16.
- The account chip is the "换账号 / Switch account" menu: accounts in the owner's order with their readings; used-up, not-signed-in and "needs your attention" accounts disabled with the reason; the confirm (with "stop and switch" while the AI works, the cost sentence, the model and window lines, and "this also ends 2 programs it started" when the tracker reports programs); the switching and result lines, one per layer of plan 4.16 ("对话完整带过去了 / The whole conversation came along", the repaired-copy line, "对话以交接笔记的形式带过去了 / The conversation came along as a handoff note" with "查看笔记 / View note"); the shell-decides chip; `menuitemradio` rows. The same menu and "switched automatically" on AI tasks rows (edits C3-08's `frontend/src/pages/aitasks/`) and the account on the repository's task list rows.
- The limit banner for any session that hits a limit (which limit, reset time, "在 B 上继续 / Continue on B" when another account has room, "等待 / Wait", "Codepass 会在重置后让它继续 / Codepass continues it after the reset" when an automatic switch could not start), the switch-back hint, "Resume on <account>" for a stopped task (same confirm), the "工单 #12 / Ticket #12" label.
- From plan 4.16 decisions 22 to 28 (design 5.6): the confirm's conditional lines (cut off at a limit; stop and switch; another model; a conversation larger than the window) and the "有一样带不过去 / One thing cannot" popover (shown only for a Codex session or a Claude model on research R8's list); the progress text while the next layer is tried; the result lines per layer, "仍在计划模式 / Still in plan mode", "Claude Code 开了一个新对话，原来的对话没有接上。 / Claude Code started a new conversation; the earlier one was not continued.", and the fourth result "没有换成 / The switch did not happen"; the "查看笔记 / View note" sheet and the "交接笔记 / Handoff notes" row-menu item; the account-problem line on the session bar; the rolled-back-switch line and the "找不到这个任务的对话记录，没有开新的会话。 / This task's conversation record could not be found, so no new session was started." line with "开一个新会话 / Start a new session" after a restart; the New task rule for a default that needs attention (nothing pre-selected, Start disabled until the owner picks another).
- Free up space labels and counts for "修好的对话副本 / Repaired conversation copy" and "交接笔记 / Handoff note" (edits C3-13's `frontend/src/aisessions/cleanup/`).
- The account on session switcher rows and in its filter (design 5.7a; edits A2-15's `frontend/src/shell/SessionSwitcher.tsx`).
- The New task account row (taken over from C3-09): pre-selected from T8-01's `ResolveDefault` with where the default comes from; the one-line fallback when the default is used up, not signed in, or every account is used up; a used-up account can still be picked; the checkbox "以后这个仓库都用「个人」 / Use Personal for this repository from now on" (off; the only control here that writes a default); the revised fifth-task wording.
- Owns `frontend/src/workspace/ai/LimitBanner.tsx`, `frontend/src/workspace/ai/AccountChip.tsx`, `frontend/src/workspace/ai/SwitchAccount*.tsx`, the account row in `frontend/src/workspace/ai/NewTask*.tsx`, the account parts of the two edited files above, locale `limits.ts`. Accept: matches `limit-banner-light-zh`, `switch-account-light-zh`, `new-task-account-light-zh`, `switch-result-note-light-zh`, `handoff-note-light-zh`, `session-account-problem-light-zh`. Tests: the pre-selection table; the menu never enables an account without room, one that is not signed in or one that needs attention; only the checkbox writes a default; the result line names the layer. Review: the banner and the menu never offer an account without room; no text promises more than the layer delivered.

**T8-10 · Account defaults in Projects and the repository settings sheet** (M): design 4.2 ("AI accounts in the Repository settings sheet") and 5.2 (the project menu's "AI 账号 / AI accounts"). **Fable first.** One row per tool that has two or more accounts: "跟随项目 acme（现在是「工作」） / Follow project acme (now Work)" or "跟随应用默认 / Follow the app default", then every account; saved at once with a toast and Undo; the one-account line; the shell-decides warn line. The project sheet with its lists of repositories that follow it and that keep their own, "Make it follow the project", the section-header chip, the move-confirm line and the remove-project line. Owns `frontend/src/pages/projects/ProjectAiAccounts*.tsx` and the AI-account rows of the repository settings sheet in `frontend/src/workspace/` (A2-06's file, edited here). Deps: T8-01, T8-W. Accept: shots `repo-settings-ai-light-zh` and `project-ai-accounts-light-zh`. Tests: each option names the level above with its current value; no row for OpenCode or for a tool with one account; saving changes no live session; hidden repositories are left out of the lists. Review: nothing is written into a repository, a tool folder or the shell.

**Gate.** Standard. Screenshots: tickets (2), ticket-editor (1), tickets-needs-you (1), ai-accounts (1), ai-account-add (1), ai-account-details (1), ai-account-remove (1), ai-account-attention (1), ai-autoswitch-confirm (1), usage-meter (2), limit-banner (1), switch-account (1), switch-result-note (1), handoff-note (1), session-account-problem (1), new-task-account (1), repo-settings-ai (1), project-ai-accounts (1). Manual (owner, real accounts): checks D1 to D30 (D28 to D30 with the built app); one evening with three small tickets on a practice repository, one of them crossing a limit, once with automatic switching off (the queue waits and its notice offers the other account) and once on; the meter compared with Claude Desktop and with ChatGPT's usage page; a task in repository A on account A and one in repository B on account B at the same time; a session at its weekly limit moved from A to B that still knows the start of its conversation and carries on, then moved B to A and back to B by hand; the owner re-reads both companies' terms (plan 4.16) and records the date.

---

## Milestone 4 - Git client (7 weeks)

**Goal.** Everything the owner does in Sourcetree and on the Bitbucket website for branches and history.
**User-visible result.** Sync, branches, compare, merge with a conflict screen, rebase, cherry-pick, revert, reset, stash, history with a graph, tags, remotes, file history. Owner, third batch (2026-11-17): a new branch from any branch; on every repository and branch the number of commits not pushed and not pulled, with when the remote was last checked; and "拉取全部 / Pull all", one click that brings every local branch of every repository up to date where that needs no decision. Owner, fourth batch (2026-11-17): after an AWS sign-in, the repositories bound to that portal are pulled by themselves (a Settings switch, on by default).

**Group A (parallel, backend).**

**G4-01 · Sync** (M)
- Build: `internal/gitsync`: fetch, pull (merge or rebase), push, publish; the integrated commit per branch; force push with `--force-with-lease=<ref>:<sha>` and `--force-if-includes`, plus the list of commits that will be replaced; auth-failure classification (AWS session ended, SSH key refused). Third batch (plan 4.6a): one shared network-action runner for fetch, pull, push, publish and Pull all (no prompt of any kind, the auth classification, a per-repository lock so one repository never runs two network actions at once, a no-progress timeout); `Publish(branch, remote)`, which the New branch dialog reuses; a method that sets a branch's "last integrated remote commit" (the lease value), which Pull all calls for the branches it moved. No Pull all logic here.
- Owns: `internal/gitsync/`, `internal/app/syncservice.go`, mock `sync.ts`.
- Accept: a background fetch between "pull" and "force push" does not weaken the lease (test with a third clone pushing in between).
- Tests: the lease scenario; rejected push; auth classification from recorded errors.
- Review: no bare `--force` or bare `--force-with-lease` anywhere.

**G4-02 · Refs** (M)
- Build: `internal/gitrefs`: local and remote branches, ahead / behind, create, switch (bring along or stash), rename, delete (unmerged warning, remote too), upstream; tags; remotes (backup-first); compare; conflict prediction with `merge-tree --write-tree`. Third batch (plan 4.6a): the branch list carries, per local branch, its remote branch, ahead, behind, "gone" and the folder it is checked out in (one `git for-each-ref` through the read-only runner); remote branches carry the age of their last commit; `CreateBranch(name, base, switch, publishTo)` where the base is a local branch, a remote branch, a tag or a commit, always with `--no-track` except when the new name is the remote branch's own name; the name checked with git's own rule (`git check-ref-format --branch`); publishing through G4-01; "how far is local X behind its remote" for the dialog's warn line.
- Owns: `internal/gitrefs/`, `internal/app/refsservice.go`, mock `refs.ts`.
- Accept: prediction matches a real merge on the fixture set.
- Tests: table tests on a fixture repository; a branch created from a remote branch does not follow it, and the same-name exception does; creating inside a task folder never switches.
- Review: switching is refused inside a task folder.

**G4-03 · Operations** (L)
- Build: `internal/gitops`: one state machine for merge, rebase, cherry-pick, revert, reset and stash apply: clean-folder rule with auto-stash and restore, conflict list, take mine / theirs / both per block, mark resolved, continue, skip, abort; rebase of a branch that is not checked out in a temporary worktree; reset modes.
- Owns: `internal/gitops/`, `internal/app/opsservice.go`, mock `ops.ts`.
- Accept: after Abort the tree and index equal the pre-operation state in every case; an in-progress operation found at startup is recognised.
- Tests: each operation with and without conflicts; abort equality; binary and delete / modify conflicts.
- Review: "mine / theirs" mapping for rebase; hard reset wording data (what is lost).

**G4-04 · History** (L)
- Build: `internal/githistory`: paged log, lane assignment for the graph, ref decoration, search and filters, commit details, compare two commits, file history, blame.
- Owns: `internal/githistory/`, `internal/app/historyservice.go`, mock `history.ts`.
- Accept: 100,000 commits page in under 200 ms per page.
- Tests: lane golden tests; filters; blame parser.
- Review: read-only runner everywhere.

**G4-05 · Stash** (S)
- Build: `internal/gitstash`: save (message, untracked, staged only), list, diff, apply, pop, branch, drop.
- Owns: `internal/gitstash/`.
- Accept / Tests: each action on a fixture; apply with conflict hands over to G4-03.
- Review: drop is confirmed upstream.

**G4-13 · Counts and freshness** (M). Owner, third batch (2026-11-17); plan 4.6a; design 5.2 "Counts on the row" and 4.2.
- Build: backend in `internal/repos/live.go` and `internal/app/loops.go`: the last successful fetch time and the last failure reason per repository in the status the page reads; the extra fetch rounds of plan 4.6a (window to the front, a workspace opened, the network back, a sign-in succeeded) with the 5-minute floor and 4 repositories at once; the list of other local branches that have commits to pull. The rule stays: by itself the app fetches only bound or opened repositories. Frontend: the shared `SyncCount` component (`frontend/src/components/SyncCount.tsx`, class `.sync-n`) with its tooltip and the stale style, used by `ProjectRow.tsx` (the order becomes `↓` then `↑`), the project header's meta line and the Open repository rows; exported for G4-06 and G4-07. **Fable first** for `projects-counts-light-zh` and `projects-counts-dark-en`. Trigger (d) sits behind one check, "is the automatic pull on for this portal?"; G4-14 flips it.
- Owns: `internal/repos/live.go` and its tests, `internal/app/loops.go`, `frontend/src/components/SyncCount*`, `frontend/src/pages/projects/ProjectRow.tsx` and the project header file, locale `sync.ts`.
- Accept: with the window in the back for an hour and then brought to the front, one round runs and the pills change without a click; a repository that is neither bound nor opened is never fetched by itself (a test remote records fetches); the tooltip is right in its four forms; matches the two new shots.
- Tests: trigger debounce and the floor; the pill matrix (0/0, n/0, 0/n, n/m, none, gone, detached, 99+, stale).
- Review: the background fetch still runs with the empty hooks folder and the no-prompt settings.

**Group A2 (after A).**

**G4-11 · Pull all: backend** (L). Owner, third batch (2026-11-17); plan 4.6a; the git facts in `docs/research/git-pull-all.md`.
- Build: `internal/gitpullall`: the plan step (which repositories and branches, what is left out, which repositories have a `post-merge` script); the run (4 repositories at once; per repository: look, `git fetch --prune` once per remote that a local branch follows, look again, then per branch the decision table of plan 4.6a: `git fetch . <remote-tracking ref>:refs/heads/<branch>` for a branch that is not checked out, `git merge --ff-only` for the checked-out branch of a clean main folder with no live AI session, nothing in every other case); waiting groups per sign-in or key that resume after the sign-in; cancel; per-repository retry; progress events; the result model (one record per repository and per branch, a case from a fixed list, the hash before and after, counts); the lease value set through G4-01 for every branch it moved. The run takes a "started by" value (owner / sign-in) and a "scripts: never" mode; G4-14 uses both.
- Owns: `internal/gitpullall/`, `internal/app/pullallservice.go`, mock `pullall.ts` (scripted to reproduce the `pull-all-*` shots).
- Deps: G4-01 (runner, classification, lock), G4-02 (branch list).
- Accept: on a fixture set that reproduces every row of the case table, each branch ends in the expected case and no other branch changed; a branch checked out in a task's workspace is untouched byte for byte; nothing is ever pushed (the fixture server's hooks record any push); the git runs of the research note pass on git 2.44, the app's minimum.
- Tests: the research note's rows as table tests; the lease value after a run; cancel during a fetch (no lock file left, `git fsck` clean); resume after a simulated sign-in; hooks: a `reference-transaction` log stays empty for the fetch and for moves of branches that are not checked out, and `post-merge` runs once for the checked-out branch and not at all with the switch off.
- Review: no `update-ref`, no `--force`, no `+` refspec, no `--update-head-ok`, no stash, no `merge` without `--ff-only`, no `rebase` and no `push` anywhere in the package (a grep test enforces it).

**Group B (serial).** **G4-W · Wiring** (S): also registers G4-11's service and events and the mock.

**Group C (parallel, UI).**

**G4-06 · Sync button** (M): design 4.2 and 6: labels per state, caret menu, progress, inline sign-in or "Verify key" on auth failure, the force-push confirm (design 5.8). Owns `frontend/src/workspace/Sync*.tsx`, and the two branch items at the foot of the working-tree switcher's menu (design 4.2 "Branch menu"; the switcher's file is named in A2-06's notes). Third batch: the count pills and their tooltip come from G4-13's `SyncCount`; the caret menu gains "拉取全部分支 / Pull all branches" (G4-11 for one repository, its result as a toast with "查看结果 / See results"); the waiting text while Pull all holds the repository. Accept: the label always describes the selected folder; a push after an expired AWS session offers sign-in in place; the tooltip shows the freshness line in each of its four forms. Tests: label matrix; confirm list. Review: the confirm names other people's commits in warn and needs a second click.

**G4-07 · Branches tab** (L): design 5.8 Branches: lists, detail, compare cards, merge / rebase / create PR entry points, tags, remotes, new-branch and delete dialogs, the clean-folder prompt. Third batch (design 5.8 as revised 2026-11-17): the count pills per local branch with "未发布 / Not published" and "远程分支已删除 / Remote branch deleted"; "从这个分支新建 / New branch from here" on every local and remote row (button, context menu, key N); the New branch dialog with the "基于 / Based on" picker (`frontend/src/workspace/branches/NewBranch*.tsx`, exported so History's "Branch from here" and the header's branch menu open the same dialog); the freshness text and Fetch on the Remote heading; "更新到远程最新 / Bring up to date" on a branch that is behind. **Fable first** for the `new-branch-*` shots. Owns `frontend/src/workspace/branches/`, locale `branches.ts`. Accept: matches `repo-branches-light-zh`, `-en` (re-taken with the pills) and `new-branch-light-zh`, `new-branch-pick-light-zh`, `new-branch-remote-light-zh`, `new-branch-behind-light-zh`. Tests: dialogs and states; the picker's ranking; the name rule. Review: no raw git terms in copy; a new branch from a remote base never follows the base.

**G4-08 · History tab** (L): design 5.8 History: virtualised list with the lane graph, ref chips, toolbar, details, compare, reset / cherry-pick / revert / tag dialogs, file history view. Owns `frontend/src/workspace/history/`, locale `history.ts`. Accept: matches `repo-history-light-zh`, `repo-history-dark-en`. Tests: graph rendering snapshots; reset dialog copy. Review: the hard-reset confirm states exactly what is lost; focus on Cancel.

**G4-09 · Conflict screen** (L): design 5.9: operation bar, three groups, the mine / theirs / result editor with block bars, choice cards, finish into the composer. Owns `frontend/src/workspace/conflict/`, locale `conflict.ts`. Deps: C3-04. Accept: matches `repo-merge-conflict-light-zh`, `-dark-en`. Tests: block resolution state; labels for rebase. Review: the abort sentence is only shown when true.

**G4-12 · Pull all: UI** (L). Owner, third batch (2026-11-17); design 5.2b.
- Build: the "拉取全部 / Pull all" header button, the start popover, the bar with the row states, the sign-in lines, the summary, the results sheet, the project menu, bulk bar and row menu entry points, the toast and the system notification kind "Pull all finished" (design 6; through `internal/notify`). **Fable first**: draw the `pull-all-*` screens in the mockup and take the shots, then build. The bar, button, sheet title and summary read their text from the run's "started by" value (G4-14).
- Owns: `frontend/src/pages/projects/pullall/`, locale `pullall.ts`, and for this task `ProjectsPage.tsx` and the project section and row menus in `frontend/src/pages/projects/`; the new notification kind in `internal/notify` and `internal/app/notifyservice.go`.
- Deps: G4-11, G4-13, G4-W.
- Accept: matches `pull-all-start-light-zh`, `pull-all-light-zh`, `pull-all-dark-en`, `pull-all-signin-light-zh`, `pull-all-done-light-zh`, `pull-all-done-light-en`, `pull-all-clean-light-zh`; the whole flow works with the keyboard only; a screen reader hears the start, each sign-in line and the end once.
- Tests: every line of the case table in both languages; the summary sentence matrix; focus return; one run at a time.
- Review: no raw git term in any string ("fast-forward", "upstream", "HEAD", "worktree", "hook", "prune"); branch names and git messages render as plain text.

**G4-10 · Stash UI** (S): design 5.8 Stash in the Changes left pane, with the stash popover and the apply / keep / branch / delete actions. Owns `frontend/src/workspace/changes/stash/`. Accept: matches `repo-stash-light-zh`. Tests: popover options; apply-with-conflict hands over to the conflict screen. Review: delete confirms; empty state.

**Group D (alone, after C).**

**G4-14 · Pull all after an AWS sign-in** (M). Owner, fourth batch (2026-11-17); plan 4.6a part 4, 4.15; design 5.2b "Automatic run", 5.14, 6.
- Build: (1) the trigger: on every successful portal sign-in, list the repositories bound to that portal's accounts and roles and ask the Pull all engine for an automatic run; no run for the silent token renewal, at app start, with the switch off, or with zero repositories. (2) The automatic mode of the engine: hooks off in every step; the checked-out branch is skipped in a repository with a `post-merge` script or a filter in its own config; no waiting for a sign-in; one run at a time with a waiting request that merges; repositories fetched by a run less than 5 minutes ago are left out. (3) Trigger (d) of G4-13 is skipped for a portal while the switch is on. (4) The setting (stored as "off", absent = on) and the "explained once" flag. (5) UI: the Code sync card in Settings, the automatic bar and button text, the first-run line, the quiet line in the Projects header, the sheet's title, subtitle, footer link and the two new lines, the toast, the notification kind "Automatic pull finished". **Fable first**: draw `pull-all-auto-light-zh`, `pull-all-auto-done-dark-en`, `pull-all-auto-first-light-zh` and the Settings card, take the shots, then build.
- Owns: the trigger and its tests; for this task also the Pull all engine and screens (from G4-11, G4-12), the fetch-trigger code (from G4-13), the Settings screen's new card, the new notification kind, the locale keys `pullall.auto.*`, `settings.sync.*`, `notify.pullallAuto.*`.
- Deps: G4-11, G4-12, G4-13, G4-W.
- Accept:
  1. Sign in to portal `acme` (fake) with 3 repositories bound to it, 1 bound to portal `globex`, 1 on GitHub (`alex-dev`), 1 unbound: exactly the 3 are fetched once and their branches moved; the other 3 record no fetch.
  2. No click is needed, and no popover or question appears, with the window in front, hidden, or closed to the tray.
  3. Switch off: no run; the fetch after sign-in of G4-13 runs as before. Switch on by default on a fresh data folder and on a data folder from the old app.
  4. A repository with a `post-merge` script: its checked-out branch has the same commit and the same files as before, its other branches moved, the result has the "own script" line, and "Pull now" updates it and runs the script once.
  5. No hook of any kind runs in an automatic run (hook logs stay empty), also when a fetched commit adds a new hook file under an in-repository hooks folder.
  6. A clicked Pull all is running when the sign-in succeeds: the automatic run starts after it and skips what was fetched in the last 5 minutes; two sign-ins while waiting give one run.
  7. The sign-in ends during the run: no sign-in window opens, no waiting line; the rest end with "sign-in expired".
  8. The first-run line shows once ever; the quiet line opens the results; nothing is shown when no repository is related; no bar, toast or notification when everything was up to date.
  9. The notification has counts only; a test fails if a portal, repository or branch name from the fixture appears in it.
  10. Keyboard only works for the whole flow; a screen reader hears the start, the first-run line and the end once each, and the switch with its state and help.
  11. Matches the new shots in both languages.
- Tests (fake data only: portals `acme`, `globex`, `initech`, `umbrella`; account `111122223333`; `alex-dev`; `alex@acme.example.com`; throwaway repositories with a local bare "server"):
  - Trigger table: sign-in by pass button, by notification button, by a Pull all line, token renewal, app start with a valid sign-in, switch off, zero repositories, hidden repository, folder gone.
  - Selection: bound to this portal / another portal / GitHub / Bitbucket / unbound / two roles of one portal (one run).
  - Automatic mode on the fixture set of G4-11: every row of the case table, plus the two new lines.
  - Hooks: empty logs for `post-merge`, `reference-transaction`, `post-checkout`; the in-repository hooks folder case of Accept 5.
  - Scheduling: wait behind a running run; merge of waiting requests; the 5-minute skip; Cancel; quit during a run.
  - Trigger (d): not run when the switch is on, run when off (a test remote records fetches).
  - Settings: default on; the stored "off" value survives a rewrite of the settings file by the old app's shape.
  - UI: text of the bar, button, summary matrix, quiet line and sheet in both languages; the first-run line only once.
  - Every test uses the fake sign-in engine and the fake notification service. No test contacts AWS, opens a browser, reads the real keychain or starts the owner's shell.
- Review: no code path lets an automatic run execute a hook or a repository-defined filter; the "never" list of G4-11's grep test still holds; no raw git term in any string ("fast-forward", "upstream", "hook", "prune"); the notification and the log lines carry no names; the switch default is on without writing anything to the settings file.

**Gate.** Standard. Screenshots: repo-branches (2), repo-history (2), repo-merge-conflict (2), repo-stash (1), projects-counts (2), new-branch (4), pull-all (7), workspace-sync (1), pull-all-auto (3), settings-sync (1). Manual: a scripted "day of git" on a fixture repository: branch, commit, rebase with a conflict, force push, cherry-pick, revert, reset, tag; a new branch from a remote branch, published; Pull all over the owner's repositories, once with an expired AWS sign-in; sign in to a real portal in the morning with the window hidden and check that the notification arrives, that the code is current, and that a repository with its own script waited.

---

## Milestone 5 - Editor and code intelligence (5 weeks)

**Goal.** A full code browser and editor for the whole repository.
**User-visible result.** The Code tab: file tree, tabs, split view, quick open, find in files, outline, and for Go and TypeScript / JavaScript: go to definition, references, hover, autocomplete and diagnostics.

**Group A (parallel).**

**E5-01 · File service** (M)
- Build: `internal/editorfs`: tree listing with git status and ignore info, read with the size rules (plan 4.12), binary detection, capped image `data:` URLs, write (3 MB limit), create, rename, delete to the system trash, change notifications from the watcher, the recovery mirror (entries are keyed by the repository's stable ID plus the path inside the folder, so a renamed folder keeps its unsaved edits).
- Owns: `internal/editorfs/`, `internal/app/fileservice.go`, mock `files.ts`.
- Accept: every path is confined to the selected working folder (symlink escapes refused); a 3.5 MB save is refused with its own error.
- Tests: confinement, limits, recovery after a simulated crash, encodings (UTF-8, BOM, CRLF kept).
- Review: path traversal; no file-serving route added.

**E5-02 · Search** (M)
- Build: `internal/search`: file list for quick open (fuzzy ranking in Go), find in files with `git grep` (case, word, regex, globs), replace all with a preview count.
- Owns: `internal/search/`, `internal/app/searchservice.go`, mock `search.ts`.
- Accept: 50,000 files: quick open answers in under 50 ms; find streams results.
- Tests: ranking; glob filters; replace round trip.
- Review: regex passed as an argument, never interpolated.

**E5-03 · Language servers** (L)
- Build: `internal/lsp`: per language and folder lifecycle; toolchain detection and install with consent (sizes shown); stdio transport over a **message log with offsets** (not a byte log), pulled by the page. From S0-06 and its review:
  - Server choice for TypeScript / JavaScript: workspace `node_modules/typescript` at 7 or above, or none (a fresh task workspace) -> the TypeScript 7 native server, `tsc --lsp -stdio` (macOS: `brew install typescript`, no Node needed at run time; Windows: Node only to run `npm install -g typescript`, then the app starts the native `tsc.exe` directly, not the `.cmd` shim). TypeScript 5.x or 6.x in the workspace -> typescript-language-server, which uses the workspace's own `tsserver.js`; it and Node are installed only when such a project is opened. Go: `brew install go` (asked, with size) before `gopls`; Windows: `winget install GoLang.Go`, then `go install`, resolving `go env GOPATH` itself.
  - The log never drops a response or a request. Above a per-session cap (16 MB) it compacts `$/progress`, `window/logMessage` and superseded `publishDiagnostics`; the page sees the hole as `gap`. A **hard limit** above the cap restarts the server and re-opens the documents. The reply carries `exited` so the page fails fast.
  - The ping is its own goroutine, coalesced, with at most one pending; it never blocks the reader. The send path never drops: a full queue blocks the bound method or fails the session (a lost `didChange` desynchronises the server silently).
  - Go answers the server requests a hidden page cannot: `client/registerCapability`, `client/unregisterCapability`, `window/workDoneProgress/create`, `workspace/configuration`, `workspace/diagnostic/refresh`, and `window/showMessageRequest` (answered with no choice; gopls uses it for its telemetry question and the app has no telemetry).
  - Idle shutdown after 10 minutes and after 10 minutes without a pull; at most 2 live servers by default, 4 on machines with 32 GB or more. Request timeout 15 to 30 s (the library's 3 s is shorter than gopls's first hover on a large workspace). Paths are compared case-insensitively on macOS and Windows (the native TypeScript server lower-cases some URIs). Child servers die with the app: process group on macOS, Job Object on Windows.
  - Registers a stopper with C3-10, so a folder rename shuts its servers down first.
- Owns: `internal/lsp/`, `internal/app/lspservice.go`.
- Deps: S0-06.
- Accept: a hidden page does not stall a fake server that floods output; traffic never reaches the log (the native TypeScript server logs one message per request); servers die with the app; the selection rule picks the right server for a TypeScript 7 workspace, a TypeScript 5 workspace and a workspace with no `node_modules`.
- Tests: lifecycle with a fake server; cap, hard limit and idle; a full send queue never loses a message; the Go answer table; install flow with fake runners.
- Review: nothing installs without the click; memory is described as ranges (gopls 0.3 to 1.3 GB, typescript-language-server about 0.9 GB, native TypeScript about 0.3 GB).

**E5-04 · Editor core** (L)
- Build: `frontend/src/editor/`: CodeMirror setup on the base of C3-04, lazy language packs for the ten languages, change-bar gutter with inline hunk and revert, large-file modes, find and replace in file, go to line, multiple cursors, an extension registry for later tasks.
- Owns: `frontend/src/editor/` (except `base/` and `lsp/`), locale `code.ts`.
- Accept: a 1 MB file opens in under 300 ms; bundle growth is reported in the pull request.
- Tests: language loading; gutter mapping from diff data.
- Review: no workers; CSP unchanged.

**E5-05 · Fake language server and file mocks** (S): a scripted mock that answers hover, completion, definition and diagnostics for the mock repository. Owns `frontend/src/api/mock/lsp.ts`, `frontend/src/api/mock/repoFiles/`. Accept: the mock speaks the same protocol messages as a real server, so E5-09 needs no mock-only code path. Tests: one request / response test per feature. Review: generic content only.

**Group B (serial).** **E5-W · Wiring** (S): adds `@codemirror/lsp-client@6.3.0`, `@codemirror/autocomplete`, `@codemirror/lang-go`, `@codemirror/lang-javascript`, `@codemirror/lang-php` (owner, 2026-11-16) and the sanitizer.

**Group C (parallel).**

**E5-06 · Code tab** (L): design 5.3: files pane, editor groups with tabs and split, breadcrumbs, all states (no file, unsaved, changed on disk, large, binary, save failed, restored), per-folder tabs, Cmd+W. Owns `frontend/src/workspace/code/`. Accept: matches `workspace-code-light-zh`, `-dark-en`, `-dark-zh` (without the code-intelligence overlays). Tests: tab model; preview tabs; changed-on-disk decisions. Review: tree accessibility; unsaved edits mirrored within a second.

**E5-07 · Quick open, palette, outline** (M): Cmd/Ctrl+P with `:`, `@`, `#`; Cmd/Ctrl+K palette; the Outline pane. **Fable first** for the palette (not drawn). Owns `frontend/src/workspace/palette/`, `frontend/src/workspace/code/Outline.tsx`. Accept: a file opens in two keystrokes plus its name; the outline follows the cursor. Tests: mode switching; ranking display. Review: keyboard-only use.

**E5-08 · Find in files and Problems** (M): the bottom panel's Find results and Problems tabs. Owns `frontend/src/workspace/panel/Find*.tsx`, `Problems*.tsx`. Accept: matches `workspace-code-find-light-zh`. Tests: result grouping; replace-all confirm. Review: results are virtualised; ignored folders are not searched.

**E5-09 · Code intelligence in the editor** (L): `@codemirror/lsp-client` 6.3.0 wired to E5-03: hover card, completion with the side pane, go to definition, peek, references into Find results, diagnostics, status bar items, the install bar, held diagnostics in fresh workspaces. From S0-06: the app's own `Workspace` class (the library's default is not exported; one `EditorView` per tab); **pull diagnostics** (`textDocument/diagnostic`) for the native TypeScript server, about 30 lines the library lacks; a serial, batched send queue; the library renders server Markdown through `innerHTML`, so `sanitizeHTML` is a real sanitizer or raw HTML is off. Owns `frontend/src/editor/lsp/`. Accept: the overlays in `workspace-code-*` shots; hover Markdown is rendered with raw HTML off. Tests: against the fake server; an `<img onerror>` in a hover, a signature help and a `window/showMessage` stays inert. Review: hover content is untrusted; no server text in logs.

**E5-10 · Blame and editor links** (S): the blame margin in the Code tab, "File history" and "Open in editor" links from Changes, History and the conflict screen. **Fable first** for the blame margin. Owns `frontend/src/editor/blame/`. Accept: clicking a blame block selects that commit in History. Tests: block grouping from blame data. Review: author names shown as text only.

**Group D (alone, after C).** Added 2026-11-16 (owner): PHP gets code intelligence too.

**E5-11 · PHP code intelligence** (M)
- Why (owner, 2026-11-16): the owner also works in PhpStorm; Codepass should replace it like GoLand and WebStorm.
- Research first (Fable, before any code; a short note `docs/research/php-lsp.md`): pick the PHP language server. Candidates: Intelephense (runs on Node; free tier for hover, completion, definition, references and diagnostics, rename and some navigation need a paid licence key; not open source) and Phpactor (open source; needs a PHP 8.1+ runtime). Compare licence, what the free part covers, the runtime it needs and how it is installed on macOS (Homebrew) and Windows (winget / npm), memory on a large framework project, start time, how it finds the project's PHP version and Composer autoload, and how it behaves in a fresh task workspace without `vendor/`. The choice and its reasons go into plan 4.12; a licence key, if one is ever used, is the owner's and is entered by the owner (secret store, never in a file).
- Build: the chosen server in `internal/lsp` (selection rule, install with consent and sizes, Windows path), the editor side for PHP (language, hover, completion, diagnostics through E5-09's code; pull or push diagnostics as the server needs), the Settings row in "Editor and code intelligence" (design 5.14), a PHP case in the fake language server (E5-05), held diagnostics in a fresh workspace without `vendor/` (like `node_modules` for TypeScript).
- Owns: `internal/lsp/php.go` (and its tests), `frontend/src/editor/lsp/php*`, the PHP files of the fake server, the Settings row.
- Deps: E5-03, E5-05, E5-09, E5-W.
- Accept: on the owner's largest PHP repository: hover, completion, go to definition and references work; memory measured and written into plan 4.12; nothing installs without the click; the server dies with the app (Windows: no leftover process, check B12).
- Tests: selection and install flow with fake runners; the fake server's PHP answers; held diagnostics without `vendor/`.
- Review: like E5-03 and E5-09 (hover text is untrusted; no server traffic in the log); no licence key or project path in a log.

**Gate.** Standard. Screenshots: workspace-code (4). Manual: open the owner's largest Go, TS and PHP repositories; a day of reading and small edits without GoLand or PhpStorm; memory with four task folders open (check A13); Windows: no `gopls.exe`, `tsc.exe`, `node.exe` or PHP server process left after quit (B12).

---

## Milestone 6 - GitHub and Bitbucket (4 weeks)

**Goal.** Several GitHub and Bitbucket accounts, each bound per repository, and pull requests in the app.
**User-visible result.** Add accounts with an SSH key and a token; repositories push as the right account; list, create, review status of, merge and close pull requests without the browser.

**Group A (parallel).**

**P6-01 · Token store** (M): `internal/secretstore`: Security.framework through cgo (items trusted for Codepass's signature only), Windows Credential Manager, file fake in sandbox mode. Owns `internal/secretstore/`. Accept: `security find-generic-password` from a terminal cannot read the item without a prompt. Tests: store / read / delete on each backend; the fake in sandbox. Review: no secret in argv, logs or errors.

**P6-02 · SSH keys and host keys** (M): `internal/sshx`: generate ed25519, import with passphrase detection and duplicate-use refusal, Windows ACLs, the app `known_hosts` with pinned provider keys, `ssh -T` verify parsing, the quoted `core.sshCommand` builder with `-F none`. Owns `internal/sshx/`. Tests: builder quoting with spaces and quotes in paths; verify output parsing for both providers; changed-host-key refusal. Review: never auto-accept a host key; key files never copied.

**P6-03 · GitHub client** (L): `internal/provider/github`: one classic token per account with `repo` + `read:org` (decision 2; fine-grained tokens are not supported in 1.0; `workflow` is not needed), repository listing, pull requests (list, get, create, merge, close, review approve / request changes, reviewers), checks from check runs **and** the combined status (or GraphQL `statusCheckRollup`; `null` means "no checks"), `mergeable: null` re-polled after a few seconds, ETag cache (a 304 costs no quota) and backoff. Error mapping: 401 = token expired; 404 with a token = compare `X-OAuth-Scopes` with `X-Accepted-OAuth-Scopes` and say "the token lacks scope X", otherwise "not found or no access"; 403 with `X-GitHub-SSO` = authorise the token for that organisation; 403 for an organisation that blocks classic tokens = that organisation opens in the browser only; 403 / 429 limits honour `retry-after`. Owns `internal/provider/github/`. Accept: checks A11 and A12 are done before sign-off. Tests: against recorded responses: 401, 404 with scope headers, 403 SSO, 403 classic-blocked, secondary-limit 403 / 429, 304. Review: minimum scopes; errors name what is missing.

**P6-04 · Bitbucket client** (L): `internal/provider/bitbucket`: Bearer API token with the five scopes `read:user`, `read:workspace`, `read:repository`, `read:pullrequest`, `write:pullrequest` (names to be confirmed on the token screen: check A14); workspaces from `GET /user/workspaces` (remote pre-fill is the fallback); "mine" from `GET /workspaces/{workspace}/pullrequests/{me}`, one call per workspace; "to review" per repository; pull requests (list, get, create, merge, decline, approve, reviewers), build statuses; merge handles the async answer (202 and the task status) and 400 / 409 / 555; `close_source_branch` on create and merge; the client counts its own calls and stays under 1,000 per hour per token (there is no remaining-count header); ETag and backoff; "token expired, paste a new one" is a routine state (Atlassian tokens are created with an expiry date). Owns `internal/provider/bitbucket/`. Accept: check A14 is done before sign-off. Tests: recorded 401, 404, async merge, 400 / 409 / 555, the call counter. Review as P6-03.

**P6-05 · Git accounts store** (S): `accounts.json` for GitHub and Bitbucket accounts and key metadata; export / import without secrets or key paths. Owns `internal/gitaccounts/`, `internal/transfer/` (extension). Accept: importing on another machine recreates accounts as "key not verified, no token". Review: identity colours stay stable across export and import. Tests: an export never contains a token, a private key or a key path.

**Group B (parallel, after A).**

**P6-06 · SSH binding in the repository engine** (M; accept: two clones of one repository push as two different accounts): GitHub and Bitbucket strategies: bind (writes `core.sshCommand` and `awsssohub.account`, backup-first), probe-based suggestion, "switch remote to SSH", re-verify on auth errors, the "machine default key" state. Owns `internal/repos/provider_github.go`, `provider_bitbucket.go`, `internal/gitx/sshbind.go`. Tests: bind / unbind / undo; two clones with two accounts. Review: probes run outside the repository with `-c` overrides.

**P6-07 · Pull request service** (M): `internal/prs`: one model over both providers, polling rules (GitHub: per-repository lists and details with ETag; "waiting for my review" through search or GraphQL, which have no ETag, at most every 60 s while visible; Bitbucket: "mine" per workspace, "to review" per repository, calls counted against 1,000 per hour), "from an AI task" linking, create from a branch (push first if needed). Owns `internal/prs/`, `internal/app/prservice.go`, mock `prs.ts`. Accept: a repository not on screen is polled every 15 minutes at most; "to review" works without ETag; an expired token, a blocked classic token and a missing SSO authorisation each degrade to "open in browser" with their own sentence. Review: provider text is passed through as plain strings. Tests: mapping tables; polling schedule with a fake clock.

**Group C (serial).** **P6-W · Wiring** (S).

**Group D (parallel).**

**P6-08 · Accounts page and add-account** (L): design 5.11: provider groups, passes with key lines, the four-step modal, SSH detail states. The token step names the exact scopes per provider and only the classic GitHub token; "Check" calls `GET /user` and compares granted scopes with needed ones; Bitbucket workspaces come from the token after step 4 (chips to confirm), not typed in step 1. Owns `frontend/src/pages/accounts/git/`, locale `accountsGit.ts`. Accept: matches `accounts-*`, `add-account-*`. Tests: stepper flow; verify results; wrong-login warning. Review: the token field never echoes; platform-specific keychain wording.

**P6-09 · Pull request list and create** (M): design 5.10 list and the create sheet (**Fable first**: the sheet is not drawn). Owns `frontend/src/workspace/prs/List*.tsx`, `Create*.tsx`, locale `pr.ts`. Accept: matches `repo-prs-*`; the CodeCommit and no-token states. Tests: filters; create validation; offline and rate-limited states. Review: titles and author names render as text.

**P6-10 · Pull request detail** (L): design 5.10 detail: tabs, timeline, merge box, reviewers, checks; safe Markdown renderer. Owns `frontend/src/workspace/prs/Detail*.tsx`, `frontend/src/components/SafeMarkdown.tsx`. Accept: matches `pr-detail-*`. Tests: a description with HTML, a remote image and a `javascript:` link renders as inert text. Review: the renderer.

**P6-11 · Pull requests page and sign-in rows** (M): the cross-repository page, sidebar count, keyring rows for GitHub and Bitbucket. Owns `frontend/src/pages/prs/`, `frontend/src/shell/Keyring.tsx`. Accept: the count shows pull requests waiting for the owner's review only. Tests: grouping; keyring states (verified, unverified, token expired). Review: same row component as the repository tab.

**P6-12 · Clone and bind for the new providers** (M): clone modal source switch and Project field (F1-19's local folder name field works for every source); bind popover for SSH accounts; "bind all suggestions" across providers. Owns `frontend/src/pages/projects/Clone*.tsx`, `BindPopover.tsx`. Accept: a clone from a GitHub account is bound to that account when it finishes. Tests: source switch; the popover's one-line preview. Review: the existing AWS clone flow is unchanged.

**Gate.** Standard. Screenshots: accounts (2), add-account (2), repo-prs (2), pr-detail (2). Manual: checks A11, A12 and A14; two GitHub accounts and one Bitbucket account on the owner's Mac; one pull request created, approved and merged per provider; a revoked key shows the right error.

---

## Milestone 7 - Windows parity, polish, release (3 weeks)

**Goal.** A release worth calling 1.0.0.
**User-visible result.** Codepass v1.0.0 for macOS (notarized) and Windows, published from `rudian/codepass`.

**Group A (parallel).**

- **R7-01 · Windows pass** (L): every screen and flow on Windows 10 1809 and 11: title bar, shortcuts (Ctrl+Shift+J for the session switcher inside a terminal), PowerShell default shell, paths with spaces, winget failures, the three AI tools on Windows (winget for Claude Code and Codex, npm for OpenCode; how `codepass.exe report` is quoted in a hook), rename of a repository folder with nothing left holding a handle, Force stop through the Job Object, no orphan `gopls.exe`, `tsc.exe` or `node.exe` after quit. Owns fixes filed as small tasks. Review: parity list signed off screen by screen.
- **R7-02 · Accessibility pass** (M): keyboard-only run of every screen, screen-reader labels, contrast audit in both themes, reduced motion. Accept: every flow in design 5 can be finished without a pointer. Tests: automated contrast and role checks added to the frontend suite. Review: live-region announcements are not noisy.
- **R7-03 · Performance pass** (M): 50,000-file repository, 100,000 commits, 30 repositories, 6 tasks: startup, status refresh, memory; fixes. Accept: the window is usable within 2 seconds of launch; idle CPU under 2% with 30 repositories. Tests: benchmark scripts kept under `scripts/bench/`. Review: no fix adds polling.
- **R7-04 · Tutorial and guide** (M): rewritten for Codepass in both languages with generic data and new screenshots. Owns `frontend/src/pages/tutorial/`, `docs/guide*.md`, `docs/screenshots/`. Accept: a new user reaches a first AI task by following it. Review: the scrub check on every image caption and alt text.
- **R7-05 · Documents** (S): README (both languages), SECURITY.md, THIRD_PARTY_NOTICES (adds go-pty's licence for the ConPTY wrapper, `marked`, `vscode-languageserver-protocol`, `vscode-jsonrpc` and the sanitizer), `docs/design/ui-spec.md` (plan 10), and the README paragraph on AI accounts (plan 10, second batch: the owner's own accounts only, sign-in through each tool, no token stored, automatic switching off by default, the links to both companies' terms). Review: every claim in the README is true of the build.
- **R7-06 · Security review** (M, Opus leads, Sonnet fixes): updater, installer script, token store, SSH binding, terminal data path, untrusted text, report socket, file confinement. Output: a findings list, all "must fix" closed.

**Group B (serial).**

- **R7-07 · Release candidates** (M): rc.1 and rc.2 through the test channel on clean macOS 15, macOS 26, macOS 27, Windows 10 and Windows 11 VMs: fresh install, upgrade from 0.0.1, rc1 to rc2 update, uninstall. Accept: every row of the test sheet passes on all five machines. Review: the sheet covers each refusal case of the updater.
- **R7-08 · Release** (S, owner present): rename the GitHub repository to `rudian/codepass` (then switch `REPO_URL` in `frontend/src/pages/settings/util.ts`, `GuideURL`, `GuideURLZh` and `ReportURL` in `internal/app/menu.go`, and the `origin` remote to the new name), merge `codepass` into `main`, tag v1.0.0, run the release pipeline on the owner's Mac, publish, verify the update check from an rc build. Before tagging (second batch): re-read the six terms pages of research R9-M1 (check D23), note each page's date, stop the release if a quoted sentence changed until plan 4.16 and the app's quotes are updated, and write the date into the release notes.

**Gate.** Standard, plus all 55 screenshots re-taken from the real app, and the owner's sign-off after one week of daily use of rc2.

---

## Owner decisions (FINAL, 2026-11-16) — these override the options below

1. Required tools cannot be installed: **the gate stays closed** (no limited mode). The own-folder Homebrew path is **not offered**: S0-04 disproved it (a prefix longer than 13 bytes compiles from source and needs Command Line Tools). A standard account installs Homebrew.pkg with an administrator's name and password. On Windows a per-user AWS CLI install may exist (the winget manifest lists a user scope); it is used only if check B11 shows it works without an administrator.
2. GitHub API sign-in: **one classic personal access token per account** (scopes `repo`, `read:org`).
3. Windows signing: **ship the Windows build unsigned in 1.0** and say so in the README (no certificate purchase).
4. Schedule: **everything ships in 1.0.0, however long it takes.** No feature moves to 1.1.
5. The three defaults below are accepted as written (no skip-permissions option; the Mac may sleep after a task has waited 15 minutes; 0.0.1 users on Intel or macOS 13-14 stay on 0.0.1).

Owner requirements of 2026-11-16, now tasks (the long descriptions moved into the tasks and into design 5.2a, 5.6a, 5.6b, 5.7a, 5.7b):
- Local folder name different from the remote name: F1-18 (stable repository IDs), F1-11 and F1-19 (clone under a custom name; both names shown), C3-10 and C3-11 (rename or move an existing folder).
- Free disk space from ended AI sessions: C3-12 (backend), C3-13 (dialog).
- Force-stop programs a closed session left running: A2-13 (tracker), A2-09 and C3-08 (the rows).
- The quit dialog that always asks: F1-03 (hook), A2-14 (dialog).
- All live sessions in one list, and the Cmd+J switcher: A2-15 (switcher), C3-08 (page).
- Task names written by the AI from the task description, no name field (2026-11-16): A2-04 (naming call, first message), A2-05 (first prompt of an empty description), A2-08 (naming at Start; Rename changes the label only), A2-09 and C3-09 (sheet), C3-01 (slug); plan 4.7, design 5.6, check A18.

Owner requirements of 2026-11-16, second batch (asked after the group-E merge, planned the same day). The owner's why: one app instead of a JetBrains IDE (WebStorm, PhpStorm, GoLand), Warp, the Bitbucket website, Claude Desktop (opened only to read the weekly usage) and Sublime Text (where ideas wait until there is usage left to build them); fewer windows and less memory.
- Carry over the fixes made on `main` (0.0.2): the Console with several Identity Center users on one portal; two users signing in to one portal; a private sign-in window for each account; the session length is the administrator's and cannot be changed in the app: **F1-21**, checks C19 to C21.
- A system notification when an AWS sign-in's session ends, naming which portal and accounts, with a button that starts the sign-in again: **F1-22**; plan 4.15; design 6 "System notifications".
- Tickets with the owner's three states (editing, on hold, queued); queued tickets run one at a time, each to the end, and the queue checks Claude's usage before each one and pauses when the limit is reached: **milestone 8**; plan 4.17.
- Several Claude accounts; a session that used up account A continues on account B with its conversation: **milestone 8**; plan 4.16.
- The usage Claude Desktop shows, inside Codepass: **milestone 8**; plan 4.16.
- The same three features for Codex (a person may use GPT instead of Claude): usage, several accounts with a switch that keeps the conversation, and the ticket queue: **milestone 8**, built for both tools from the start; plan 4.16 and 4.17.
- PHP gets code intelligence (hover, completion, definition, references, diagnostics) like Go and TypeScript / JavaScript: **E5-11**; plan 4.12.
- Find a repository by typing its folder name and start work at once: the Projects search with ranking and Enter-to-open (**F1-CL**, milestone 1) and "Open repository" from any screen with Cmd/Ctrl+O (**A2-16**, milestone 2); plan 4.4; design 5.2, 5.7c, 4.3.
- Several Claude or GPT (Codex) accounts, switched back and forth at any time; account A for repository A, and for a task in repository B a free choice of A or B: any account for any task, defaults per repository and per project, a "Switch account" menu on every session (**T8-01, T8-05, T8-09, T8-10**); plan 4.16 "Choosing the account"; design 4.2, 5.2, 5.6, 5.16.
- When account A reaches its weekly limit, switch to B and carry on **with nothing of the context lost**: three layers, the same session, a repaired copy, a handoff note; never a fresh start without the conversation (**T8-05, T8-11**); plan 4.16 "Switching keeps the conversation"; research R8.
- Several accounts within the companies' terms, for the owner's own use and not for resale; worry about bans: the owner's own accounts only, unmodified official tools, each account inside its own plan, automatic switching **off by default**, nothing routed around a refused or suspended account, a terms check before the milestone (**T8-01, T8-04, T8-05, T8-07**); plan 4.16 "Several accounts within the terms"; research R3, R5d, R9.
- TASK A on account A and TASK B on account B at the same time: supported. For Claude Code it is the use Anthropic documents as being signed in to several accounts at once, "such as work and personal accounts"; for Codex it is not forbidden and not described (research R2, R9 case b).
- Hidden reasoning (research R8): for the newest Claude models the AI's hidden thinking records work only in the account that produced them, and the service drops them in another account without an error. Everything visible (the owner's messages, the AI's replies, tool calls and results) carries over. The switch confirm says so behind "有一样带不过去 / One thing cannot". The owner then asked whether the task can still continue: yes. The hidden reasoning is the draft before each reply, its conclusions are already in the replies and the code, and on the other account the AI thinks again from the whole visible conversation and the workspace; Anthropic documents switching accounts as "keep sending the full history", and Claude Code already strips stale thinking signatures after a login switch (plan 4.16 decision 22, "Why the task still continues"). Check D13 proves it on a scratch conversation; T8-05 is not signed off without it.
- Schedule (planner's call under decision 4): all of it ships in 1.0.0. F1-21 and F1-22 run before the milestone-1 gate (they touch the accounts code the gate checks, and the build that replaces AWS SSO Hub must not lose 0.0.2). Milestone 8 runs **right after milestone 3**: it needs AI tasks (milestone 2) and task workspaces (milestone 3), and the owner runs into the weekly limit today.

Owner requirements of 2026-11-17, third batch (git; asked during the milestone 1 gate review and planned the same day; plan 4.6a; design 4.2, 5.2, 5.2b and 5.8; the git facts in `docs/research/git-pull-all.md`):
- As on the Bitbucket website, a new branch from any branch the owner chooses: the New branch dialog's "基于 / Based on" picker (local branches, remote branches, tags; the branch on screen by default) and "从这个分支新建 / New branch from here" on every branch row (**G4-02, G4-07**).
- The number of commits committed but not pushed (`↑n`) and on the remote but not pulled (`↓n`) as count pills on the Projects row, the Sync button and the Branches list, with when the remote was last checked; extra fetch rounds when the window comes to the front, a workspace opens, the network comes back or a sign-in succeeds (**G4-13, G4-06, G4-07**). The Projects row already shows both numbers (milestone 1).
- "拉取全部 / Pull all": one click fetches every repository once and moves every local branch forward to its remote branch where that needs no decision; it never pushes, merges, rebases or stashes; a branch that needs a decision is listed with the reason (**G4-11, G4-12**).
- Planner decisions the owner may change (design 8, open points 21 to 27): publishing a new branch at once is off by default; a new branch never follows its base, except when it takes the remote branch's own name; Pull all covers every listed repository with a remote, also unbound ones, and leaves hidden ones out; any uncommitted change, a live AI session in the main folder or an unfinished merge leaves the checked-out branch alone; the repositories' own `post-merge` scripts run for the checked-out branch, with a switch per run; 4 repositories at once; no undo and no shortcut.
- Schedule: milestone 4, with the rest of the git client (about 1.5 weeks more); nothing ships before 1.0.0 (decision 4).

Owner requirements of 2026-11-17, fourth batch (asked during milestone 2 and planned the same day; plan 4.6a part 4, design 5.2b "Automatic run", 5.14 and 6):
- After an AWS sign-in succeeds, the app runs Pull all by itself for the repositories bound to that portal, one repository after another, so nobody has to click "Pull all". A Settings switch, "登录 AWS 后自动拉取 / Pull automatically after AWS sign-in", turns it off; it is on by default.
- Planner decisions the owner may change (design 8, open points 28 to 31; told to the owner on 2026-11-17): an automatic run never runs a repository's own script, so a repository that has one keeps its checked-out branch for one click; it runs only after a sign-in, not when the app starts.
- Schedule: milestone 4, task G4-14 (about 0.5 week more).

**Open questions for the owner after the milestone-0 gate** (the plan assumes the first answer of each until told otherwise):
1. Decision 3 was relayed as "3 a", which in the lettering below means "buy a certificate", while this block records "ship unsigned". Both reviews ask for a confirmation. Assumed: unsigned. If a certificate is bought, F1-10 gains a Windows signing step no spike has tested and F1-20 gains an Authenticode check.
2. Should app-started `claude` tasks get the flags of your own `claude` shell function (model, effort, channels)? Assumed: yes, the app keeps launching through your login shell (plan 4.7); the two spikes disagreed and review A kept the plan.
3. TypeScript code intelligence now uses the TypeScript 7 native server first, with typescript-language-server only for TypeScript 5 / 6 projects. Assumed: agreed.
4. On Windows, Ctrl+J inside a terminal is a line feed that tools use for a new line. Assumed: the switcher is Ctrl+J everywhere except inside a terminal on Windows, where it is Ctrl+Shift+J. Cmd+J on macOS is unaffected.
5. "Rename local folder" can move the folder to another parent on the same disk. A move to another disk is not offered in 1.0 (it cannot be undone as one step). Assumed: acceptable.
6. An ended session's last terminal screen is saved to disk so it can be read after a restart (it may contain whatever was on screen; it is deleted by Free up space). Assumed: on, with a setting to turn it off.
7. (Second batch; **answered by the owner on 2026-11-16: yes.**) The owner also works in PhpStorm, so PHP gets code intelligence in 1.0 like Go and TypeScript / JavaScript: hover, completion, go to definition, references, diagnostics. New task E5-11 (milestone 5, group D); E5-W adds `@codemirror/lang-php`; plan 4.12 and 6, design 5.14.
8. (Milestone 8, AI accounts.) The first account of a tool on the AI accounts page is both the app default and the first choice for automatic switching. Assumed: one order for both.
9. After an automatic switch, the app only offers "切回「工作」 / Switch back to Work" when Work has usage again. Assumed: an offer, never an automatic return (each return re-sends the whole conversation and spends usage).
10. A ticket fixed to one account whose account is used up. Assumed: it moves to another account only when automatic switching is on for that tool (off by default); otherwise the queue waits for that account's reset and its notice offers the other account.
11. The repository's default account is used up when the owner starts a task. Assumed: the New task sheet pre-selects the first account with room for that one task and says so; the default itself does not change.
12. Automatic switching at a limit is **off by default** (the planner changed it on 2026-11-16 after the owner asked to stay within the terms): an automatic hop at the moment of a limit is the pattern closest to "circumventing rate limits or restrictions" (research R9). Turning it on is per tool and shows the quotes once. Assumed: off.
13. Hidden reasoning cannot move to another account for the newest Claude models (research R8: the service drops it, no tool can change that); everything the owner can see carries over. Assumed: acceptable as "nothing lost", said behind "有一样带不过去 / One thing cannot" in the switch confirm rather than in its body.
14. The terms checkpoint (plan 4.16 decision 35): after reading research R3, R5d and R9, does the owner confirm using several of the owner's own subscriptions this way, with switching by hand at a limit? Assumed: yes until answered; the plan records "not confirmed yet" and the answer must come before milestone 8.
15. An account the tool refused (sign-in, billing, suspension): assumed that its sessions cannot be moved to another account even by hand until the owner clears the mark ("我已处理好 / I have dealt with it").
16. Team or Enterprise seats: assumed that Codepass stores no "team seat" flag; the add-account sheet has one line about the organisation's rules (check D24 tells the owner whether any account is a seat).
17. Asking each company the one written question of research R9-M4 (check D26) is the owner's choice; Codepass never does it. Only a written answer turns the grey cases into a yes or a no.

Orchestrator process change (replaces "one pull request per task" in "How the work runs"): each task runs in its own git worktree branched from `codepass`; after the Opus review passes, the orchestrator merges it into `codepass` and pushes. No GitHub pull request per task.

Review shape (from group B2 on, after group B ran three full reviews per task and none converged): **one full Opus review**, then up to three rounds of Sonnet fix + Opus **verification**. A verification checks only that each finding is fixed (or validly declined), reads the fix diff for regressions and runs the gate. A new blocker or major still blocks; a new minor or nit goes to the follow-up list below and is scheduled into a later task. Review files are kept in `docs/plan/codepass/reviews/` (not in git).

Worktree rules learned in group B: never run `npm ci` / `npm install` / `rm -rf` through a worktree's `frontend/node_modules` link (it emptied the main checkout once); stage files by name; a frontend build must leave `frontend/dist/.gitkeep` tracked (a Vite plugin now rewrites it after every build). The scrub block list lives at `~/.codepass-dev/scrub-list.txt` (mode 600, outside the repository); the gate runs `CODEPASS_SCRUB_LIST=~/.codepass-dev/scrub-list.txt scripts/scrub-check.sh --tracked`. Tree mode only: the merge commits carry the required session trailers, which the commit-range mode would flag.

**Follow-ups** (minor and nit findings that did not block a merge; each goes into a named later task or the milestone's clean-up before its gate). Full text in `reviews/<group>-final.json`.

Group B (merged 2026-11-16, `9302a4c`..`64b1b64`; wiring notes for F1-W in `reviews/groupB-wiring.md`):
- FU-02a (minor, **F1-18**) `internal/projects/projects.go` `clone()`: deep-copy `MissingSince` so a Snapshot never shares it with the store.
- FU-02b (nit, F1-CL) `internal/store/store.go` Quarantine suffix loop: stop on any Lstat error other than ErrNotExist; cap the suffix count.
- FU-02c (nit, F1-CL) Quarantine: use `renameReplace` (the Windows sharing-violation retry) instead of plain `os.Rename`.
- FU-02d (nit, F1-CL) `Load` merges into a reused value: zero it first, or document "fresh value only".
- FU-03a (minor, **A2-14**) `internal/app/session_darwin.go`: the powering-off flag is never cleared, so after a cancelled logout every Cmd+Q counts as a logout. Honour it for about 30 s only, or only during the quit event. Manual check C6.
- FU-03b (minor, F1-CL) the NSWorkspace observer registers in `init()`, before `HandleHelperMode`: register it from `NewLifecycle` (once).
- FU-03c (minor, F1-CL) Windows recycle: add `FOF_WANTNUKEWARNING`, so a drive without a Recycle Bin asks before a permanent delete.
- FU-03d (nits, F1-CL) Windows lister opens every process with a 64 KB buffer; `DE_*` codes shown as Win32 errors; dead `parseTasklist`; stale comments; one misleading test name.
- FU-04a (minor, **F1-12**) git minimum 2.44 (`GIT_NO_LAZY_FETCH`); written into F1-12.
- FU-04b (minor, **F1-11**) Status in a blob:none clone fails when rename detection needs a missing blob: retry without rename detection and mark the row.
- FU-04c (nit, F1-CL) `TestReadsNeverRunAlternateRefsCommand` checks nothing: read with `--alternate-refs`.
- FU-05a (minor, F1-CL) `internal/watch/watch.go` `resolveDir` opens the folder before checking it; on Windows that short handle can make a concurrent rename fail.
- FU-05b (nits, F1-CL) stale `ErrNotWatched` doc; the quiet-window tests fail on a legitimate Overflow under heavy load (gate flake risk).
- FU-06a (nit, F1-CL) `internal/netx/config.go`: a bad proxy URL also fails requests that NO_PROXY or loopback would bypass.
- FU-06b (nit, F1-CL) Windows: a SOCKS-only proxy goes direct silently; report it as unsupported, as on macOS.
- FU-07a (minor, F1-CL) scrub check: `c:/users/<name>` with forward slashes passes.
- FU-07b (minor, F1-CL) scrub check `--commits`: a `-diff` or binary attribute hides added lines.
- FU-07c (nits, F1-CL) PGP private key blocks and camelCase `secretAccessKey`; header comment, `--help` range, a dead test line.
- FU-08a (nit, F1-CL) `--pop-origin` is never set: menus aligned to the end grow from the wrong corner.
- FU-08b (**F1-W**) `Segmented.tsx`: capture `e.currentTarget` before `requestAnimationFrame`; `Keyring.tsx:62` and `pages/accounts/util.ts:17`: smooth scroll must honour Reduced (`motionReduced()`).
- FU-09a (nit, F1-CL) `internal/update/doc.go`: `StagedRecord` is required too.
- FU-09b (nit, F1-CL) unit test for the failed-Restart path.
- FU-10a (nits, F1-CL) `release.sh`: the keygen guard fails late when `--out` is inside another work tree; check the local tag against HEAD before the build, not at upload; two `test.sh` cases test less than they claim; `smoke.sh` joins two pgrep outputs.
- Scope notes (accepted): F1-08 also edited `pages/settings/SettingsPage.tsx`, `i18n/locales/settings.ts`, `lib/theme.ts` and `privateSignIn.test.tsx` (the Motion setting: System / Full / Reduced). F1-07 made comment-only edits in `internal/api/repos.go` and the generated models (F1-11 keeps or reverts both together). The test channel repository gets a throwaway name such as `rudian/codepass-testchannel` (`docs/release.md`), never `rudian/codepass`, which R7-08 needs for the rename.

Group B2 (merged 2026-11-16, `f4176d7`; wiring notes for F1-W in `reviews/groupB2-wiring.md`):
- FU-18a (minor, **F1-11**) `internal/projects/registry.go` `movedFrom`: skip a found record that already has its own data (project, hidden flag, identity override), so the moved question goes away instead of failing on "yes"; map `projects.ErrHasData` and `projects.ErrNotFound` to their own api error codes (today both reach the person as `E_INTERNAL`) that F1-14 explains.
- FU-18b (minor, **F1-11**) `internal/repos/add.go`: "Add existing" while `projects.json` cannot be written returns `E_REPO_NOT_FOUND`; return `E_FILE_IO` with `file=projects.json`.
- FU-18c (minor, **F1-11**) `internal/repos/registry.go`: a folder left out of a scan because `projects.json` could not be written gives no warning; add `warnings` to `api.ReposState` (bindings, mock) and set it from the fallback path.
- FU-18d (minor, **F1-W**) `internal/app/container.go`: a newer-version or corrupt `projects.json` makes `NewContainer` fail and `main` exits through `log.Fatal` before any window; written into F1-W.
- FU-18e (nit, F1-CL) the legacy import is not one-time: after the record that shared a 0.0.1 hash is purged, another repository at the old path gets the old test result. Record imported hashes (or one "import done" flag) in `projects.json`.
- FU-18f (nit, F1-CL) respell flip-flop: one folder reaching `IDsForPaths` under two case spellings in one call rewrites `projects.json` on every scan. Respell at most once per call, or dedupe scanned folders with `os.SameFile`.
- FU-18g (nit, F1-CL) case-sensitive volume, case-only rename: the stored spelling is gone, the record is marked missing and purged after 30 days with its data. Respell when the stored path is gone and the new spelling exists.
- FU-18h (nit, F1-CL) `TestAddExistingAfterManualMoveIsOffered` asserts only the hidden flag; also assert project and identity override.
- Scope notes (accepted): F1-18 edited `internal/app/container.go` (opens `projects.json`, passes it as the registry; forced by the "no silent fallback" fix because the app tests scan through the container), added `internal/repos/registry.go`, removed `SetRepoTest` from `repos.Store` (test results now live in `projects.json`; `settings.json` stays byte-identical for 0.0.1), and added `RepoRecord.FirstSeen` and `DeclinedMoves`.

Group C (merged 2026-11-16, `82ea29e`..`35d16a0`; wiring notes in `reviews/groupC-wiring.md`):
- FU-11a (minor, F1-CL) `internal/repos/other.go` `AddFolder`: the CodeCommit branch always writes `SetHidden(id,false)`, so with `projects.json` not writable it fails with `E_FILE_IO` after `AddExisting` succeeded. Write only when the flag changes.
- FU-11b (minor, F1-CL) `internal/projects/registry.go` `Reattach` keeps the old record's `Pending` flag: show a moved copy, answer yes, and the row jumps back into the review set. `old.Pending = old.Pending && cur.Pending`, with a test.
- FU-11c (minor, **F1-14**) mock `repos.ts` `ReattachMoved` only deletes `movedFrom`; the app moves the row to the old ID with the old project, hidden flag, identity and test result. Make the mock do the same, with a test.
- FU-11d (nit, F1-CL) `other.go`: dead `urlHasUserinfo`.
- FU-11e (nit, F1-CL) `urlHasCredential` counts any bare user name of 20+ characters as a token: a long Bitbucket account name gets a Health warning. For bitbucket.org count only known token prefixes.
- FU-11f (nit, F1-CL) `internal/repos/live.go`: re-subscribing after `WatchStopped` has no back-off; a watch that keeps dying costs a status read every few seconds.
- FU-11g (nit, F1-CL) `f111c_test.go` `TestBackgroundFetchDoesNotPromptOrRunHooks`: the hook assertion cannot fail; drop or rename it.
- FU-12a (nit, **F1-15**) mock `system.ts` lags the Go fixes: failure lines without `execution error:` and the code tail, the forced repair-prefix failure (`-1`, `/opt/homebrew already exists`), and `StartSetupInstall` clears every blocking override row.
- FU-12b (nit, F1-CL; copy **F1-15**) comments in `internal/app/toolservice.go` and `internal/api/system.go` claim the cleared tool path is backed up; the settings write keeps no backup. Fix the comments; the setup copy never promises a backup.
- FU-12c (nit, F1-CL) `internal/toolinstall/installer.go` `runTool`: a child that handles SIGINT exits 130 and is classified `installer_failed` instead of `cancelled`; check `ctx.Err()` first.
- FU-12d (nit, F1-CL) `README.md` and `README.zh-CN.md` still say Git 2.39.1; the minimum is 2.44.0. Add a test that `envcheck.GitMin` equals `gitx.MinVersion`.
- FU-13a (minor, F1-CL) `internal/identity/write.go` `ApplyProject`: a member deduped onto a shared config (a linked worktree) reports OK even when the owner's write of that file failed. Copy the owner's outcome to every deduped member, with a test holding `config.lock`.
- FU-13b (minor, **F1-16**) `write.go` `repoInfo` maps every config-path failure to `E_REPO_NOT_FOUND`; use `E_REPO_NOT_FOUND` only for "not a repository", otherwise `E_GIT_FAILED` or `E_FILE_IO` with the config's display path, so the popover shows design 5.12's "Cannot write this repository's config".
- FU-13c (nit, **F1-W**) the identity mock swap leaves `identity.test.ts` importing enums from `./identity`: re-export them from `../models` or change the test imports.
- FU-13d (nit, F1-CL) undo of a created global config records `sha256('')` as the after-write hash, so `RestoreBackup` of that safety backup refuses with `E_CONFIG_CHANGED`; record `''` for a removed file in `internal/app/watcher.go`.
- FU-20a (nit, **F1-W**; comment F1-CL) use `update.ProductionGate()` on Windows too: the Windows gate ignores BundleID and Requirement, and `validate()` passes because Requirement is set. F1-20's wiring note about a placeholder Requirement is wrong. Put the sentence into the comment in `gate_windows.go`.
- FU-20b (nit, F1-CL) `TestRefusalLeavesNoStagingFolder` never goes through the controller; add `controller_windows_test.go` (runs on the Windows VM, check B16).
- FU-20c (done by the orchestrator) manual check B9 gained the steps to publish a mismatched rc.3.
- FU-20d (nit, F1-CL) `internal/update/winver.go`: the `dir` closure's comment describes the wrong thing.
- Scope notes (accepted): F1-11 added `RepoRecord.Pending`, `Data.DismissedSuggestions` and `internal/projects/review.go`, and edited the mock `world.ts`, `world.test.ts`, `services.test.ts`, `pages/repos/model.test.ts`, `api/errors.ts` and `locales/errors.ts` (three new error codes). Hidden repositories now leave `ReposState.Repos` (plan 4.4); newly found CodeCommit repositories are listed at once as in 0.0.1, only other providers wait in the review set; `AddExisting` still refuses non-CodeCommit, the new `AddFolder` takes any repository. F1-12 edited `internal/gitx/git.go` (`MinVersion` 2.44.0, developer-tool stub guard) and `frontend/src/lib/toolVersions.ts`, added `mock/toolTypes.ts`, and the privileged script also checks the sha256 (fifth argument). A broken but present tool does not block the gate (only missing or below minimum does). F1-13 put the author-identity engine into `internal/identity` next to the existing account colour code (a later clean-up may split the package); the undo ID lives in memory only (32 most recent). F1-20 changed `gate_other.go` only in its build tag and comment.

Group D (merged 2026-11-16, `7ede90e`, `e4a4b93`; notes for group E in `reviews/groupD-wiring.md`):
- FU-Wa (minor, F1-CL) `frontend/src/App.tsx`: while the tools gate is blocked, the login modal still opens over the setup screen (Cmd+L, `?open=login:`, `app:navigate` login). Ignore these while the gate is closed and honour `?open=login` after setup; test the blocked shell.
- FU-Wb (nit, F1-CL) `internal/execx/execx.go` `Command`: no `WaitDelay`, so a grandchild holding stderr (git-remote-https, ssh) keeps `Run` waiting after the context ends. Set `cmd.WaitDelay` (about 5 s); test with a child that keeps the pipe open.
- FU-Wc (nit, F1-CL) `App.tsx`: while the gate is "checking" the window is an empty grid; show the sidebar-less frame with the titlebar drag strip and the brand.
- FU-Wd (nit, F1-CL) `components/Segmented.tsx`: take the next index from the focused radio, not the value prop (two quick arrow presses land on the same item).
- FU-We (nit, F1-CL) `internal/app/loops.go` `online()`: the comment is out of date (it also needs a routable address and a default route).
- Scope notes (accepted): the Git settings route stays on `GitGlobalPage` (F1-16 extends it); the newer-version and damaged-file dialogs before the window are bilingual because settings cannot be read yet (`internal/app/startup.go`; wording to check with the owner at the gate); offline = no routable interface or no default route; the update controller is built in `internal/app/updatewiring.go`, emits `update:state` (registered by F1-17) and checks 30 s after start, then every 6 h, outside sandbox mode; Cmd+1..4 follow design 4.3; the menu's "check-updates" opens the Update page; one shared netx client is created in `main.go`.

Group E (merged 2026-11-16, `1447a36`..`dc4a213`; notes in `reviews/groupE-wiring.md`). All F1-CL unless marked:
- FU-14a (minor) `pages/projects/ops.tsx`: the identity confirms (set identity, remove project) promise Undo, but the toasts have no Undo action. Add Undo with the collected undo IDs (reuse `useIdentityToast` from `pages/git/identityHooks.ts`) or reword to backup-only.
- FU-14b (minor) `ProjectsPage.tsx` `openBind` on the menu paths anchors to `.row__actions`, which cannot take focus: Esc drops focus to `<body>`. Anchor to the menu trigger or the row.
- FU-14c (minor, pre-existing) `pages/repos/BindPopover.tsx` never takes focus; focus the first control on open and return focus on Cancel and Bind.
- FU-14d (nit) the `?open=drag:` screenshot path passes no provider to `DragGhost`.
- FU-14e (nit) en copy "Sign-in ends: 8 min left" should be "Sign-in ends in 48 min" (design 5.2).
- FU-14f (nit) mock `repos.ts`: a failed reattach keeps `movedFrom`; the Go engine forgets the offer.
- FU-14g (nit) `createProject` moves before applying the identity, so two confirms; apply the identity to the empty project first.
- FU-14h (minor, Go) `Identity.ApplyMove` writes `projects.json` directly, so the repos engine's `forgetOffers` never runs and a moved row keeps a stale "moved here?" offer. Make ApplyMove drop the offer.
- FU-14i (nit) the identity mock uses its own repository IDs (`r_acme_api` ...), not the repos mock's; take them from the world so moves show their identity confirms in the mock.
- FU-15a (minor) `pages/setup/SetupPage.tsx`: after "I'll install them myself", when only use-detected rows still block, the primary stays "Check again"; switch to "Continue".
- FU-15b (minor) the winget "ask IT" alert appears on the first window-focus re-check after "Open Microsoft Store"; show it only after an explicit Check again (as a warn hint).
- FU-15c (minor) `pages/setup/model.ts`: right after a successful run the footer briefly says a tool is still missing; keep a verifying state until the setup state is re-read.
- FU-15d (minor) `HealthPage.tsx` does not mount `ToolRows` (written into F1-CL's Build).
- FU-15e (minor, Go + UI) Health "Update" only opens the tool's page; add a ToolService update call (brew upgrade, winget upgrade) and use it.
- FU-16a (minor, Go) `internal/identity/write.go` `planRepo`: a per-repository mode can write a config shared with another member that has its own identity (linked worktree); refuse with a coded error the UI explains.
- FU-16b (minor) `pages/git/RepoPopover.tsx`: focus drops to `<body>` after a failed Save or Retry; an `E_FILE_IO` for `projects.json` is shown as "cannot write this repository's config".
- FU-16c (nit, Go) `gitFileInMessage` keeps git's trailing reason in the file path (`.git/config: invalid unit`).
- FU-16d (nit, Go) `errorItem` ignores the file of an `E_GIT_FAILED` cause (a corrupt `~/.gitconfig` is reported as each repository's config).
- FU-16e (nit) mock `identity.ts`: the `readerr` scenario is missing from the project and move plans.
- FU-16f (nit) `IdentitySection.test.tsx`: wrong comment; add the follow-global marker component step.
- FU-16g (nit, **milestone 6**) the `.suggest` row (offer an account's noreply address) needs a data source; build it with the GitHub and Bitbucket accounts.
- FU-17a (minor) `pages/update/UpdateDialog.tsx`: "Reopen when no task is running" disables the focused button and focus drops to `<body>`.
- FU-17b (minor) opening the dialog in ready, error or refused and pressing Esc loses the opener focus (the footer's focus effect runs first).
- FU-17c (minor, Go) a damaged `update-prefs.json` makes every `SetAutoCheck` fail; Quarantine and save again (or fold into FU-17d).
- FU-17d (minor, Go + mock) plan 3.1: the update-check preference belongs in `settings.json` (`api.Settings.UpdateCheckOff`, zero = on); drop `update-prefs.json`.
- FU-17e (nit, **M1 gate**) "Reopen now" stays disabled during the download while `update-light-zh` shows it enabled: approve the disabled variant and re-take the shot.
- FU-17f (nit) `internal/api/update.go`: the phase-mapping comment claims more than the test checks.
- FU-17g (minor) `UpdatesCard` and `UpdateBanner` are mounted nowhere (written into F1-CL's Build).
- FU-17h (nit) `pages/update/useUpdate.ts`: a stale `State()` answer can overwrite a newer `update:state` event; drop older answers with a sequence counter.
- FU-19a (nit) `pages/projects/CloneModal.tsx`: counts and `start()` read previews without the freshness check (stale "Clone 1 repository" for about 350 ms).
- FU-19b (nit) the "Clones into" fallback joins with `/` on Windows; use the separator `parentDir` uses.
- FU-19c (nit, Go) `internal/repos/clone.go` `ClonePreview.Command` lacks the destination folder, so "Command to run" differs from "Clones into" with a custom name.
- Scope notes (accepted): F1-14 imports `../repos/repos.css` and the bind and HTTPS modals from `pages/repos/`, falls back to a plain move when the identity service does not know a repository (mock only), and adds mock-only `?open=` values; F1-15 writes copy for causes design 5.1 does not word (review at the gate); F1-16 uses new `identity.title` / `identity.subtitle` keys and a review modal before a global save; F1-17 keeps the preference in `<data>/update/prefs.json` for now, made a small additive edit to `internal/update/controller.go` (`ControllerConfig.AutoCheck`) and `internal/app/updatewiring.go`, and checks every 24 h (design 5.13) instead of F1-W's 6 h; F1-19's `RepoName` markup is `.repo-nm` / `.repo-nm__local` / `.remote-name`.

Group F (merged 2026-11-16, `2c24815`, a `--no-ff` merge that keeps `main`'s history; notes in `reviews/groupF-wiring.md`). All F1-CL; FU-21a and FU-21b were already in 0.0.2 (not regressions):
- FU-21a (minor) `internal/app/container.go` (about line 500): a macOS isolated launch already under way when Shutdown cancels `c.ctx` can start the browser while `OpenIsolated` returns an error; the profile folder is deleted and nothing ever closes that window. After the entry check, run the launch on `context.WithTimeout(context.WithoutCancel(c.ctx), 15*time.Second)` so a launch under way returns its handle and the quit closes it; test with a fake `Output` whose open succeeds only after Shutdown began (the fake Signal must see the pid before Shutdown returns).
- FU-21b (minor) `internal/app/container.go` (about line 615): windows handed to `closeSignIn`'s plain goroutine just before the quit (the old window after "Open again", a sign-in that ended a moment earlier) are not awaited by Shutdown, and on Windows `RemoveStale` keeps a profile still in use. Track that goroutine (a `sync.WaitGroup` or `c.spawn`) and let `closeSignInWindows` wait for closes in flight within its budget; test "Open again" with a slow fake Close, then Shutdown.
- FU-21c (nit) `internal/app/quit_signin_test.go` (about line 158): no test fails when the check after the isolated attempt (`container.go:505`) is removed, or the entry check (`container.go:496`) with the private sign-in setting off. Require `E_CANCELLED` and no opens in `TestSignInWindowOpenedDuringShutdownIsClosed`; add the setting-off case to `TestSignInPageAfterShutdownOpensNothing`.
- Scope notes (accepted): Codepass keeps version 0.0.1 in every version file; `settings.json` is still read by `internal/settings` (not the versioned store), so item (d) is a 0.0.2 fixture test; README and guide needed no rewording (main's new lines name no product); four links keep `rudian/aws_sso_hub` until the repository rename at release (R7-08): `REPO_URL` in `frontend/src/pages/settings/util.ts` and `GuideURL`, `GuideURLZh`, `ReportURL` in `internal/app/menu.go`. A2-14's "stop all" must quit through `app.Quit` or `Container.Shutdown`, which closes the isolated sign-in browsers; a new sign-in page opens through `c.openSignIn`. Manual check C26 (quit paths, leftovers after a kill, links open a normal window) was added for the owner.

Group G (merged 2026-11-17, `f3fc3e2`; notes in `reviews/groupG-wiring.md`). All F1-CL:
- FU-22a (minor) `internal/app/notifyservice.go` (about line 166): `authURL[portal]` is never cleared. A "Sign in again" press between a new sign-in's start and its `waiting` progress (launching the isolated browser takes seconds) reopens the previous, finished sign-in's authorize URL through `c.openSignIn`. Clear `authURL[portal]` on the portal's `LoginPhaseStarting` progress and when its sign-in ends, so `focusSignIn` does nothing while no URL is known; test: finish one sign-in, start a second whose page open blocks, press the button, expect no page opened and `RegCount` unchanged.
- FU-22b (minor) `frontend/src/App.tsx` (about line 141): the button goes through `openLogin`, which returns `null` while any login modal is open (`shell/AppActions.tsx:72`). Two portals that end together: pressing "Sign in again" on the second while the first's sign-in runs (often in the hidden window) does nothing and gives no feedback, and the system removes the pressed notice. Keep a login request for another portal and open it when the current modal closes, or show the window with a toast that a sign-in is already running; test with two login navigations (manual check C23 step 3).
- FU-22c (nit) `internal/app/notifyservice_test.go` (about line 140): no app-level test that a successful sign-in removes the portal's delivered notice and starts its record over. Add one: mark `ended` for initech, `Accounts.Login`, expect `Removed == ["signin:initech"]` and `Last("initech", "ended") == ""`; then `Logout` and expect a second removal.
- FU-22d (nit) `internal/notify/signin.go` (about line 63): the heads-up always says "about <lead>" ("about 1 hour"), even when the end is learned inside the lead window (a 90-minute session with lead 60 gets it with 42 minutes left). Send the minutes actually left (`SessionEndsAt` minus now, rounded to 5, at most the lead) with the event and word the text from that, in both languages.
- FU-22e (minor) `internal/browser/isolated.go`: `browser.Isolated` has no `Focus`, so "Sign in again" during a running isolated sign-in opens its page again instead of bringing the window forward. Add `Focus(ctx)` for macOS and Windows behind the existing seam (`focusSignIn` already uses it when present), tested with fakes; no real browser in tests.
- FU-22f (minor, safety) tests must never reach AWS: a first draft of an F1-22 test built a container without the fake endpoints and one sign-in request reached the real AWS OIDC address (fake data; AWS rejected it). Fail closed: in a test binary (`testing.Testing()`), every AWS client refuses an empty or non-loopback endpoint before dialing: `internal/sso/manager.go` (OIDC and portal), `internal/awsapi/awsapi.go` (STS, IAM) and the sign-in federation call in `internal/awsapi/console.go`, `internal/codecommit/codecommit.go`. Test: a `Container` built without `Endpoints` returns an error from `Accounts.Login` and opens no connection.
- FU-22g (nit, docs) `docs/release.md` "System notifications in a development build": say that a development build without `AWSSSOHUB_SANDBOX` uses the real data folder and the real `~/.aws`, so the owner's checks C22 to C24 and B19 use a test-channel build; the variable is for a deliberate check on the owner's own data only.
- Scope notes (accepted): the status row "System notifications" is a card at the top of the single-column Settings page; the card title and first help line use `settings.notify.title` and `settings.notify.expiry.hint` (locale `notify.ts`), and the unused `settings.reminders` key is already deleted (the unused-key sweep skips it); notice bodies never name the Identity Center user (the app does not know it); legacy `sso_start_url` profiles get no notices; `KindSpec.OnClick` runs inside the system's callback and must return quickly (milestone 8 may move it to `Options.Go`); one new indirect requirement `git.sr.ht/~jackmordaunt/go-toast/v2` for the Windows build. Manual checks C22 to C24 and B19 were reworded after the merge.

Group H (merged 2026-11-17: `0ab5df7` F1-CLa, `86e82d4` F1-CLb; notes in `reviews/groupH-wiring.md`). All **A2-CL**:
- FU-CLa1 (nit) the FU-22f tests send a real request to check the guard.
  - In `internal/codecommit/awsguard_internal_test.go` (about line 15) and `internal/awsapi/awsguard_internal_test.go` (about lines 17 to 28), the checks for the CodeCommit factory and `signInClient` send a real request.
  - If either client lost its guard, the test would send a request signed with a fake key to `codecommit.us-east-1.amazonaws.com`, or fetch `example.com`.
  - Fix: check the client before any request, as the sso test does. Assert that the client's `HTTPClient` is the guarded `*http.Client` and that `signInClient.Transport` is not nil. Then send the probe to a reserved name, such as `https://guard-probe.invalid/`.
- FU-CLa2 (nit) `internal/app/container.go` (about line 685), `closeSignInWindows`: one 8 s deadline now covers the wait for opens, the close context and the closes in flight.
  - A browser that hangs while starting uses up the whole budget. Every Close then gets an expired context, and `endProfile`'s process lookup (`ps`, `taskkill`) fails at once.
  - Windows that are already open keep running after the quit, and their profile folders are removed while the browser still uses them.
  - Fix: give the close step a short floor (the later of the deadline and now plus 3 s). State the worst-case quit time in the comment.
- FU-CLa3 (nit) `frontend/src/pages/health/ToolRows.tsx` (about line 90), `updateCauseKey`: two causes an update can report show "the installer reported an error":
  - disk full: brew "no space left on device", winget `0x8A150105`;
  - restart needed: winget `0x8A150109` and `0x8A15010A`.
  - Add `health.tool.cause.diskFull` and `health.tool.cause.reboot` in both languages.
  - The English `health.tool.cause.policy` (locale `health.ts`, about line 133) uses a straight apostrophe: write `computer’s`.
- FU-CLa4 (nit) ToolRows focus has two problems:
  - The Update button (about line 220) sets `loading` without `keepEnabled`, so it is disabled while `StartToolUpdate` starts, and focus can drop to `<body>`. `PassCard.tsx:60` avoids this with `keepEnabled`.
  - The focus effect (about line 148) also runs on mount: after a finished update, a later visit to Health moves focus into the list.
  - Fix: pass `keepEnabled` and ignore clicks while busy. Move focus only when the update finished during this mount; keep the previous value in a ref.
- FU-CLb1 (minor) `frontend/src/pages/projects/ProjectsPage.test.tsx` (about line 577): no test covers `targetHasIdentity` in `createProject`.
  - Removing `!!identity` at `ProjectsPage.tsx:287` brings back the missing move confirm, and the suite still passes.
  - Add the test with Go-shaped spies: `PreviewProject` returns no repositories, `ApplyProject` returns an undo ID, and `PreviewMove` returns KeepOwn for two selected rows.
  - Create the project with an identity. Expect one "Move to idp?" confirm listing both repositories; the toasts "Moved 2 repositories to idp" and "Created project idp"; and no "Author identity set for idp".
- FU-CLb2 (nit) `applyProjectIdentity` (`pages/projects/ops.tsx`, about line 277) returns void.
  - So when the identity save fails, `createProject` (`ProjectsPage.tsx`, about line 287) still passes `targetHasIdentity = true`, and the move confirm says the repositories keep their own identity in a project that has none.
  - Fix: resolve to a boolean (identity saved) and pass it to `moveRepos`.
- FU-CLb3 (nit) the per-row `ProfileSelect` `aria-label` is the CodeCommit name while the row shows the folder name. This is in `pages/repos/BulkBindModal.tsx` (about line 121) and `pages/git/UrlRuleModal.tsx` (about line 89). Use the folder name, or "Account for {repo}" built from it.
- FU-CLb4 (nit) `frontend/src/api/mock/identity.test.ts` (about line 141): the test name still says a move gets an error plan, but a move now rejects. Also, `.rejects.toBeTruthy()` accepts any rejection.
  - Rename the test.
  - Assert `.rejects.toMatchObject({ code: 'E_GIT_FAILED' })`, with the config's display path in `params.file`.
- FU-CLb5 (nit) about 24 `repos.*` keys in locale `repos.ts` were used only by the deleted `ReposPage.tsx` and `RepoRow.tsx`. The `.repo-table` rules in `pages/repos/repos.css` (lines 3 to 18) and `styles/components.css` (about line 90) are unused too.
  - Search for each key, including keys built at run time.
  - Delete the unused keys in both languages, and the rules.
- FU-CLb6 (nit, mock) `frontend/src/api/mock/repos.ts` (about line 532): `ClonePreview.command` is `git clone <url>`.
  - Go (`internal/repos/clone.go:122`, FU-19c) now appends the destination: in single quotes when it has a space, in double quotes on Windows.
  - Match Go, so the mock's "Command to run" shows the folder.
- FU-CLb7 (minor) `frontend/src/pages/git/RepoPopover.tsx` (about line 78): `PreviewRepo` returns F1-CLa's `E_CONFIG_SHARED` (FU-16a) as a plan error, and the popover shows every plan error as the danger state "cannot write this config" with Retry.
  - So the explanation (the shared file, and what to do) never appears, and Retry cannot help.
  - Show the `E_CONFIG_SHARED` message from `errors.ts` without Retry.
  - Test it with a `PreviewRepo` spy that returns the `E_CONFIG_SHARED` plan error.
- Scope notes (accepted):
  - F1-CLa added `internal/awsguard`, the Windows `browser.Isolated.Focus`, `Settings.UpdateCheckOff` with a one-time move of the old update preference file, `ToolService.StartToolUpdate`, `E_CONFIG_SHARED`, `projects.json` `importedLegacy`, `repos.Deps.WatchRetryBase`, and the Git minimum in `docs/guide*.md`.
  - F1-CLb added `lib/repoSearch.ts`, `useActions().requestLogin`, `pages/repos/RepoSentence.tsx`, the mock parameter `?search=`, and `hideTools` on EnvChecks. It reworded the remove-project confirm to backup-only.
  - Deferred to the gate: the Health screenshots with ToolRows.

Group I (merged 2026-11-17: `4a7e6b5` F1-GF, `90f1772` A2-01; notes in `reviews/groupI-wiring.md`):
- FU-GFa (major; on codepass before F1-GF) `frontend/src/pages/projects/projects.css` about line 119: the Projects row's actions column is clipped at window widths from 980 to about 1101 and from 1181 to about 1281, including the default 1200x780 window, so the row menu cannot be reached with the mouse. **F1-GF2** (milestone 2 group B).
- FU-GFb (nit, **A2-CL**) `frontend/src/styles/tokens.css`: under reduced motion the indeterminate progress bar keeps turning slowly everywhere except Setup (F1-GF made Setup's still). One rule for the app, with its styles test.
- FU-GFc (nit, **A2-CL**) `frontend/src/components/RepoName.test.tsx`: the dynamic `import('node:' + 'fs')`. Add `@types/node` to the frontend's dev dependencies and use a plain import, or test the class names instead of the CSS text.
- FU-GFd (nit) plan 4.15's "3 个账户" example: done by the orchestrator (`d8e97cb`).
- FU-A2-01a (minor, **A2-13**) `internal/pty/pty_unix.go` about line 306: `Members()` waits for a whole `KillSession` (`kill()` holds `k.mu` through the grace time and the SIGKILL step). Serialize `kill()` with its own mutex; take `k.mu` only around each listing and the ended check.
- FU-A2-01b (minor, **A2-13**; planner decision 2026-11-17) after an app crash on macOS a `nohup` child or a `set -m` background group survives. No guard process: the tracker saves its set to `<data>/sessions/leftovers.json` and lists the survivors at the next start with Force stop (plan 4.8b "After a crash").
- FU-A2-01c (nit, **A2-CL**) `internal/pty/pty.go` about line 75: the `Close` doc should say that with a `Read` pending the master stays open until that `Read` returns, so `Close` alone hangs nothing up.

Milestone 2 group B (merged 2026-11-17: `1d3153b`..`7f53899`; full text of every item in `reviews/m2B-wiring.md`):
- FU-A2-02a (minor, **A2-CL**) `internal/terminal/modes.go` about line 49: Some streams still give a wrong prologue in xterm.js.
- FU-A2-02b (minor, **A2-08**) `internal/terminal/scrollback.go` about line 175: After a task is resumed, the archive cache can keep serving the previous run.
- FU-A2-02c (nit, **A2-CL**) `internal/terminal/modes.go` about line 367: The run counter is not reset when a string ends with ESC followed by something other than a backslash.
- FU-A2-02d (nit, **A2-08**) `internal/terminal/manager.go` about line 174: A reused ID's new log starts exactly at the previous tail.
- FU-A2-02e (nit, **A2-CL**) `internal/terminal/review1_test.go` about line 47: The doc comment above TestNeverGroundStreamsAreTrimmed describes the scrollback-cap test ('The scrollback file stays within the cap plus the look-back for a clear...').
- FU-A2-02f (nit, **done by the orchestrator (manual checks use <sandbox>/appdata/logs/app.log)**) `internal/paths/paths.go` about line 72: The manual check in A2-02's notesForWiring greps `<sandbox>/logs/app.log`.
- FU-A2-03a (nit, **A2-CL**) `internal/keepawake/manager_test.go` about line 543: TestOnChangeDeliversTheLatestStateInOrder does not reliably catch the regression it guards.
- FU-A2-03b (nit, **A2-CL**) `internal/keepawake/manager_test.go` about line 565: TestTouchOnACountedRunningSessionDoesNoWork also passes when Touch always re-evaluates (mutant `cheap := false`).
- FU-A2-03c (nit, **A2-CL**) `internal/keepawake/manager_test.go` about line 671: TestLostCaffeinateIsSeenAndRetried has a latent timing race.
- FU-A2-03d (nit, **A2-W**) `internal/keepawake/config.go` about line 68: The Settings wiring note says to add `KeepAwake keepawake.Stored` to api.Settings (internal/api/system.go).
- FU-A2-03e (nit, **done by the orchestrator (manual check wording); the code comment goes to A2-CL**) `internal/keepawake/activity_darwin.m` about line 18: The 'setting off mid-task' manual check expects the NSProcessInfo activity to remain in `pmset -g assertions`.
- FU-A2-03f (nit, **A2-CL (and A2-W never calls back into the Manager from OnChange)**) `internal/keepawake/manager.go` about line 39: Options.OnChange (line 39) and SetOnChange (line 165) say the callback is 'never under a lock', but deliver() now calls it while holding emitMu.
- FU-A2-04a (minor, **F1-SG**) `internal/execx/waitdelay_unix_test.go` about line 12: Pre-existing and outside A2-04, which does not change internal/execx.
- FU-A2-04b (nit, **F1-SG**) `internal/aitools/winshim_test.go` about line 125: TestWindowsWithoutLookupKeepsBareNameOffWindows only checks argv[0].
- FU-A2-04c (nit, **A2-CL**) `internal/aitools/winshim.go` about line 52: Two comments no longer match the code.
- FU-A2-05a (minor, **A2-08**) `internal/taskstate/tracker.go` about line 394: Leftover of the fallback fix.
- FU-A2-05b (minor, **A2-08**) `internal/taskstate/tracker.go` about line 263: After a fallback (non-final) end, transition() still revives the task only for SessionStart, UserPromptSubmit and session.execution.started.
- FU-A2-05c (minor, **A2-08 (and A2-W)**) `internal/taskstate/tracker.go` about line 345: The OwnerInput wiring note says to call it from TerminalService.Write.
- FU-A2-05d (nit, **check A7, then A2-CL**) `internal/taskstate/inject_opencode.go` about line 31: plugin.loaded now counts as a signal, which assumes events also flow in the OpenCode TUI under --standalone.
- FU-A2-05e (nit, **A2-CL (same test as FU-GF2b)**) `frontend/src/pages/projects/ProjectsPage.test.tsx` about line 620: Outside A2-05 and already failing before this branch, whose frontend matches codepass aeae5c4 exactly.
- FU-A2-13a (nit, **A2-CL**) `internal/proctrack/source_windows.go` about line 13: On Windows, PPID is always 0.
- FU-A2-13b (nit, **A2-CL**) `internal/app/proctrackservice_test.go` about line 20: The only service test predates round 1.
- FU-A2-13c (nit, **A2-CL**) `internal/proctrack/tracker.go` about line 160: Since round 1, MarkClosed and Forget take tickMu, so they wait for any tick in progress: one ps call (10 s timeout) plus Members() of every live session.
- FU-GF2a (minor, **A2-CL**) `frontend/src/pages/projects/projects.css` about line 121: The narrow template's actions column is 128px from 980 up to 1320, so a fix button gets 94px (128 minus 26 for More minus the 8px gap).
- FU-GF2b (nit, **A2-CL**) `frontend/src/pages/projects/ProjectsPage.test.tsx` about line 620: 'skips the motion above 200 rows' calls waitFor(..., 8000) but runs under vitest's default 5000ms test timeout, so it fails whenever the machine is busy.
- FU-A2-06a (nit, **A2-CL**) `frontend/src/workspace/RepoSettingsSheet.tsx` about line 54: In the real app, Go SetHidden emits repos:changed before the call returns (internal/repos/review.go returns e.changed(ctx)).
- FU-A2-06b (nit, **A2-CL**) `frontend/src/workspace/BottomPanel.tsx` about line 27: The CSS max-height bounds only the drawn height.
- FU-A2-06c (nit, **A2-CL**) `frontend/src/workspace/workspace.css` about line 4: `.ws.ws--still` stops only the grid narrowing.
- FU-A2-06d (nit, **A2-CL**) `frontend/src/workspace/WorkspaceHeader.tsx` about line 38: For a CodeCommit repository where binding does not apply, the header draws a plain chip labelled 'Not bound' (ws.acct.unbound), and the sheet says 'Not bound to an account'.
- FU-A2-06e (nit, **C3-07**) `frontend/src/workspace/WorkspacePage.tsx` about line 79: This is a milestone 3 note.
- FU-A2-06f (nit, **A2-W (keep the registration)**) `frontend/src/i18n/index.ts` about line 25: This is a merge note, not part of the fix diff.
- FU-A2-07a (minor, **A2-CL**) `frontend/src/terminal/instance.ts` about line 299: applyLineHeight ignores devicePixelRatio.
- FU-A2-07b (minor, **A2-CL (first: control bytes reach the terminal)**) `frontend/src/terminal/instance.ts` about line 366: onPaste returns before preventDefault when sanitizing leaves nothing, so xterm's own textarea paste handler then pastes the raw clipboard text.
- FU-A2-07c (minor, **A2-CL**) `frontend/src/terminal/instance.ts` about line 128: Write and resize errors (controller.ts:52 and 90) reach the same onError as read errors.
- FU-A2-07d (nit, **A2-CL**) `frontend/src/terminal/TerminalView.tsx` about line 46: The danger toast body is e.message.
- FU-A2-07e (nit, **A2-CL**) `frontend/src/terminal/terminal.css` about line 20: .term-host--fresh repeats F1-08's .term-enter (motion.css:59): same animation, same purpose ('fades in after the first byte').
- FU-A2-07f (nit, **A2-CL**) `frontend/src/terminal/TerminalView.tsx` about line 56: The removed custom dialog wrapped long URLs with overflow-wrap: anywhere.

## Decisions that were asked (kept for the record)

The plan also assumes three defaults without asking (plan 9): no option to skip an AI tool's permission prompts; the Mac may sleep after a task has waited 15 minutes; 0.0.1 users on Intel or macOS 13-14 stay on 0.0.1. Say so if any of these is wrong.

### 1. When the required tools cannot be installed, what then?

- **A (recommended): the gate stays closed.** The screen names the cause and the next step, and "Check again" works at any time.
- **B: allow a limited mode** (view accounts, sign in to AWS; no repositories, git or AI) after a failed install.
- Facts: you asked for the tools to be mandatory. The cases are a Mac account without admin rights, a company-managed machine that blocks installers, and no network on first launch. Almost everything Codepass does needs git, and CodeCommit needs the AWS CLI, so a limited mode can do very little. Homebrew documents an install into the user's own folder that needs no admin password; the spike tests it, and if it works it covers the first case without opening the gate. Windows has no per-user AWS CLI installer.
- Recommendation: A, plus the own-folder Homebrew path if the spike proves it.
- Result of S0-04 (2026-11-16): the own-folder path does not work (see FINAL decision 1). The sentence "Windows has no per-user AWS CLI installer" is not certain either way: the winget manifest lists a user scope that points at the same MSI as the machine scope.

### 2. How does a GitHub account sign in for pull requests?

- **A (recommended): one classic personal access token per account** with the scopes `repo` and `read:org`.
- **B: fine-grained tokens**, one for personal repositories and one per organization.
- Facts: a fine-grained token covers only one user or one organization; organizations can require approval for them or block them, and SAML organizations need each token authorised. Merging needs Contents: write, and whether check results can be read with a fine-grained token is not confirmed. A classic token works across all your organizations with one paste, but it grants more than the app needs. "Sign in with GitHub" in a browser is a third way; it needs an OAuth app registered under your account and is planned for 1.1 either way.
- Recommendation: A for 1.0, stored so that only Codepass can read it on macOS.

### 3. Sign the Windows build?

- **A: buy a code-signing certificate** (or a signing service, if you are eligible for one) before release.
- **B (recommended): ship the Windows build unsigned in 1.0** and say so in the README.
- Facts: an unsigned exe shows a SmartScreen warning on first run and can be blocked outright on PCs with Smart App Control. Updates are still protected by Codepass's own signed update file on both platforms. A certificate costs a yearly fee and needs an identity check; the macOS build is unaffected (it is signed and notarized with the <owner company> team, as you confirmed).
- Recommendation: B, because your daily machine is the Mac. Revisit when someone else uses the Windows build.

### 4. If v1.0.0 runs late, may features move to 1.1?

- **A (recommended): yes, in this order:** tags and remotes screens; the cross-repository Pull requests page; line-level staging (hunk staging stays); split editor; blame and file history; OpenCode state detection beyond the fallback; the TypeScript language server (Go stays).
- **B: no. Everything ships in 1.0.0, however long it takes.**
- Facts: the estimate is about 32 weeks of the implement-and-review loop, and it is an estimate. The seven items above are the ones nothing else depends on; together they are roughly 5 weeks. You would still get every milestone build through the test channel along the way.
- Recommendation: A. The planner asks you before each item moves.
