# Codepass v1.0.0 - task deployment plan

Date: 2026-11-16 (revision 3, after the milestone-0 gate). Author: planner (Fable). Reads with `product-plan.md` (what and why; "plan N.N" below) and `ui-design.md` (screens and copy; "design N.N"), the mockup in `docs/design/codepass-mockup/` (shots in its `shots/` folder), the spike notes and the two reviews in `docs/research/`, and `manual-checks.md` (what the owner still has to try by hand; "check A3", "check B5").

Revision 3 applies the seven spike notes as corrected by `spike-review-a.md` and `spike-review-b.md`, and turns the owner's requirements of 2026-11-16 into tasks: F1-18, F1-19, F1-20, A2-13, A2-14, A2-15, C3-10, C3-11, C3-12, C3-13.

## How the work runs

**Models.** Sonnet implements every task. Opus reviews every task before it merges, using the task's "Review" line plus the standing checklist below. Fable is called only where a task says "Fable first" (a screen the mockup does not draw) or when a reviewer finds a design gap.

**Branch.** Everything lands on `codepass`, pushed after every merged task. Nothing merges into `main` and nothing is published before milestone 7. Each task runs in its own git worktree branched from `codepass`; after the Opus review passes, the orchestrator merges it into `codepass` and pushes. There is no GitHub pull request per task.

**Parallel groups.** Inside a milestone, groups run in the order given. Tasks inside one group run at the same time and never own the same file. Shared files (`main.go`, `internal/app/container.go`, `internal/app/events.go`, `frontend/src/api/services.ts`, `frontend/src/api/mock/index.ts`, `frontend/src/App.tsx`, `frontend/src/shell/Sidebar.tsx`, `frontend/src/i18n/index.ts`, `frontend/src/styles/*.css`, `go.mod`, `frontend/package.json`) are owned by exactly one task per group, usually the **wiring task** (id ends in `W`). A backend task delivers its package, its service file in `internal/app/`, its API types in `internal/api/`, its mock in `frontend/src/api/mock/` and its locale file in `frontend/src/i18n/locales/`; the wiring task registers them. A task that needs a new dependency names it in its pull request, and the wiring task adds it.

**Dependencies.** A task depends on every task in the earlier groups of its milestone and on the previous milestone's gate. A "Deps" line names anything beyond that, such as a spike.

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

Group A merged 2026-11-16 (`f868498`). Group B merged 2026-11-16 (`9302a4c`..`64b1b64`, gate passed); follow-ups FU-02a to FU-10a are listed after "Owner decisions". Group B2 merged 2026-11-16 (`f4176d7`, gate passed); follow-ups FU-18a to FU-18h. Group C merged 2026-11-16 (`82ea29e`..`35d16a0`, gate passed); follow-ups FU-11a to FU-20d. Group D merged 2026-11-16 (`7ede90e`, `e4a4b93`, gate passed); follow-ups FU-Wa to FU-We; wiring notes for group E in `reviews/groupD-wiring.md`. Before group E the orchestrator added `fd36f8b` (test setup: fixes an intermittent unhandled error) and `821cf55` (the `RepoName` stub and the Projects clone-modal entry point). Group E merged 2026-11-16 (`1447a36`..`dc4a213`, gate passed: 56 test files, 749 tests); follow-ups FU-14a to FU-19b; notes in `reviews/groupE-wiring.md`. **Work paused here at the owner's request (2026-11-16); next: F1-CL, then the gate. Handoff: `docs/HANDOFF.md` in the repository.**

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

**Group F (serial, after E).**

**F1-CL · Milestone 1 clean-up** (S)
- Build: fix every follow-up marked "F1-CL" in the follow-up list ("Owner decisions" section), each with a test where the follow-up asks for one. Delete the files in `pages/repos/` that nothing imports any more (after F1-14 and F1-19: `ReposPage.tsx`, `RepoRow.tsx`, the one-line `CloneModal.tsx` re-export, page-only tests, the duplicated `.clone-*` rules in `repos.css`). Every list and header that names a repository uses `RepoName` (design 5.2). Mount what group E built but could not place: `<UpdatesCard />` in `SettingsPage.tsx`, `<UpdateBanner />` once under the Projects header, `<ToolRows />` in Health's runtime card (replacing EnvChecks' AWS CLI and Git rows). Delete unused locale keys (`placeholder.setup.*`, `placeholder.update.*` if unused, `git.title` / `git.subtitle`, `clone.folderName`). Optional, from the notes: an icon prop on `components/Banner`. With about 60 items it can run as two parallel tasks: F1-CLa (Go) and F1-CLb (frontend), split by file.
- Owns: the files those follow-ups name.
- Deps: group E.
- Accept: every F1-CL follow-up is fixed or declined with evidence; the gate passes.
- Review: the same scoped verification as any fix round.

**Gate.** The standard gate. Screenshots: projects (4), setup (7), identity (3), update (2); several were not re-taken after group E's fix rounds, so take them all fresh, plus the clone modal (no mockup shot; judge against design 5.2). Approve with the owner (or Fable) the design deviations listed in `reviews/groupE-wiring.md` (Projects: the extra "Add a folder" button, select-all per section, "Bind all"; Setup: the running footer text and the copy written for causes design 5.1 does not word; Update: "Reopen now" disabled during the download (FU-17e), no release date; Identity: the 200px Source column) and the startup dialog wording in `internal/app/startup.go`. Manual: upgrade a copy of the owner's 0.0.1 data folder; install on clean macOS 15, 26 and 27 VMs (gate, then app: checks B1 to B3); the launch smoke test of the signed build with the old app quit (A9); update test channel build N to N+1 on the Mac and once on the Windows 11 VM (B5, B9).

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

**Group B (parallel, after A).**

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
- Build: `internal/aitools`: detection through the login shell with a timeout, using only `--version` (`opencode auth list` starts OpenCode's shared background service; `opencode --version` does not); install commands per platform shown before they run (macOS: `brew install --cask claude-code`, `brew install --cask codex`, `brew install opencode`; Windows: `winget install --id Anthropic.ClaudeCode -e`, `winget install --id OpenAI.Codex -e`, OpenCode through npm because winget has no package: check B10 confirms the package name); launch arguments per tool; reading each tool's approval or sandbox mode (vocabulary in plan 4.8); never adding a permission-skipping flag. Argument builders cover Claude Code `--session-id`, `--resume`, `--plugin-dir`; OpenCode `--standalone --session ses_<id>` (**always** `--standalone`: without it stopping a task cannot kill what OpenCode runs, and the task's environment does not reach it); Codex `-c` and `resume`. Also the naming call and the first message (plan 4.7 "Naming", owner request 2026-11-16): one headless call per tool with the fixed naming prompt, no tools, the smallest model, an empty working folder and a 5-second budget, its answer parsed and validated (title, slug); and the first-message argument per tool (after `--` for Claude Code and Codex, `--prompt=<text>` for OpenCode).
- Owns: `internal/aitools/`, `internal/app/aitoolservice.go`, mock `aitools.ts`.
- Deps: F1-12, S0-03.
- Accept: a missing tool yields the exact install command; a user-defined `claude` shell function still applies (plan 4.7 stands: the app launches through the login shell, and its own flags come after the function's, which is why hooks travel as a plugin and not as `--settings`); after an OpenCode task stops, no `opencode` process is left and `~/.config/opencode/service.json` is unchanged. Check A18 (naming on the owner's Mac, per tool) passes.
- Tests: argument builders; a test asserts none of `--dangerously-skip-permissions`, `--allow-dangerously-skip-permissions`, `--permission-mode bypassPermissions`, `--auto`, `--yolo`, `--dangerously-bypass-approvals-and-sandbox`, `--approve-for-me` can be produced; detection with fake shells; tests that need Codex skip when it is absent. Naming: parser and fallback table (Chinese, English, mixed, emoji, very long, empty, an answer that is not JSON or breaks the branch rules); a description starting with `--dangerously-skip-permissions` reaches the tool only as message text.
- Review: nothing from a task title or path reaches shell text; every injection adds to the user's own setup and is tested per tool (two `--plugin-dir` flags; an OpenCode user `plugins` list; Codex `-c notify=[...]` replaces a user's own `notify`, so it is used only when the user has none).

**A2-05 · Task state signals** (L)
- Build: `internal/taskstate`: the binary's `report` mode and a local socket (user-only permissions). The launcher sets `CODEPASS_TASK` in the child environment and report mode reads it; it forwards **only** task ID, event name, session ID and permission mode, exits at once and never waits for the app. Per-launch injection, never a write to the user's own settings: For a task started with an empty description, the first prompt a hook reports is handed once, in memory, to the naming call (plan 4.7); it is never logged or stored.
  - Claude Code: a session-only plugin folder (`.claude-plugin/plugin.json` and `hooks/hooks.json`, written under the task's data folder) passed with `--plugin-dir`; a `--settings` file is the fallback. Subscribe only to `SessionStart`, `UserPromptSubmit`, `Notification`, `PermissionRequest`, `Stop`, `SessionEnd` (command hooks run synchronously, so `PreToolUse` / `PostToolUse` would add a process start to every tool call); a short hook `timeout`.
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
- Build: `internal/proctrack`: per session, a set of every descendant ever seen. A snapshot every 2 s while the session is live and every 10 s afterwards (macOS: one `ps -axo pid=,ppid=,pgid=,sess=,lstart=,pcpu=,rss=,comm=` call, never one call per process; Windows: the Job Object's process list). A process is remembered by PID **and start time**, so a reused PID is never mistaken for it; a process that left the terminal session by `setsid`, or was re-parented to launchd when its parent exited, stays in the set. API: `Leftovers(session)` (count, CPU, memory, command names), `ForceStop(session)`: SIGTERM to every tracked process and group, about 5 s, then SIGKILL (Windows: `TerminateJobObject`), then one more snapshot to confirm. The set survives the session's close and is dropped when its last process is gone. It does not survive an app quit: quitting with "stop all" runs `ForceStop` for every session first (A2-08).
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

**Group C (serial).**

**A2-08 · Task lifecycle** (L)
- Build: `internal/tasks`: `tasks.json` (records hold the repository's stable ID from F1-18), start / stop / resume by session ID (**the app creates the ID when the task is created**: Claude Code `--session-id <uuid>`, OpenCode `--session ses_<id>`; Codex's is captured from its hook or `notify`; hook capture is the cross-check), states from A2-05, soft cap, main-workspace isolation only, wiring to keep-awake and to the window-close hook of F1-03, notifications when a task waits, and the first-time "Codepass 还在运行 / Codepass is still running" notification when closing the window hides it (design 4.4; assigned at the group-B merge). Calls F1-03's `SetTasksRunning` from cached state only (the hook runs on the UI thread). One data folder per task (`<data>/sessions/<task-id>/`: the hook plugin, the saved scrollback). A session whose tool has exited or whose terminal was closed while A2-13 still reports processes is "closed, still running". `LiveSummary()` (sessions, state, programs started) feeds the quit dialog; "stop all" stops every task and runs `ForceStop` for every session before the app exits; "keep running" hides the window. Names a task at Start (plan 4.7 "Naming"): the record keeps the title, the slug and whether the name came from the AI or the fallback; Rename changes the label only. Calls `setRunningTasks(n)` (`frontend/src/pages/update/runningTasks.ts`) whenever the number of running tasks changes; it drives the update dialog's "Reopen when no task is running".
- Owns: `internal/tasks/`, `internal/api/tasks.go`, `internal/app/taskservice.go`, mock `tasks.ts`.
- Deps: A2-02 to A2-05, A2-13.
- Accept: quitting with "stop all" leaves no process of any task; tasks reappear as stopped after restart and resume into their own session; keep-awake is released only when the last task and the last leftover process are gone.
- Tests: lifecycle state machine; restart recovery; cap.
- Review: races between exit, stop and state events.

**A2-W · Wiring** (S): registers the services, mocks, locales, the workspace route, the rail and sidebar changes (AI tasks item, keep-awake chip); the "Switch session" command (Cmd+J; Ctrl+J on Windows, Ctrl+Shift+J while a terminal has focus) in the menu and the command table, and the quit dialog's mount point, both with placeholders that A2-14 and A2-15 fill; adds xterm packages; removes the Terminal helper page and `internal/terminal`'s old API users. Owns the shared files. Deps: A2-06 to A2-08, A2-13.

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

**Gate.** Standard. Screenshots: workspace-ai (2), new-task (2), settings (2), session-switcher (2), quit-dialog (1). Manual: three tools, two sessions each in two repositories, window hidden for 30 minutes with output flowing; a session closed while its dev server runs shows as "closed, still running" and Force stop ends it; the keep-awake matrix on the owner's Mac (`pmset -g assertions`); Windows VM: one session per tool (B8, B10).

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

## Milestone 4 - Git client (5 weeks)

**Goal.** Everything the owner does in Sourcetree and on the Bitbucket website for branches and history.
**User-visible result.** Sync, branches, compare, merge with a conflict screen, rebase, cherry-pick, revert, reset, stash, history with a graph, tags, remotes, file history.

**Group A (parallel, backend).**

**G4-01 · Sync** (M)
- Build: `internal/gitsync`: fetch, pull (merge or rebase), push, publish; the integrated commit per branch; force push with `--force-with-lease=<ref>:<sha>` and `--force-if-includes`, plus the list of commits that will be replaced; auth-failure classification (AWS session ended, SSH key refused).
- Owns: `internal/gitsync/`, `internal/app/syncservice.go`, mock `sync.ts`.
- Accept: a background fetch between "pull" and "force push" does not weaken the lease (test with a third clone pushing in between).
- Tests: the lease scenario; rejected push; auth classification from recorded errors.
- Review: no bare `--force` or bare `--force-with-lease` anywhere.

**G4-02 · Refs** (M)
- Build: `internal/gitrefs`: local and remote branches, ahead / behind, create, switch (bring along or stash), rename, delete (unmerged warning, remote too), upstream; tags; remotes (backup-first); compare; conflict prediction with `merge-tree --write-tree`.
- Owns: `internal/gitrefs/`, `internal/app/refsservice.go`, mock `refs.ts`.
- Accept: prediction matches a real merge on the fixture set.
- Tests: table tests on a fixture repository.
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

**Group B (serial).** **G4-W · Wiring** (S).

**Group C (parallel, UI).**

**G4-06 · Sync button** (M): design 4.2 and 6: labels per state, caret menu, progress, inline sign-in or "Verify key" on auth failure, the force-push confirm (design 5.8). Owns `frontend/src/workspace/Sync*.tsx`. Accept: the label always describes the selected folder; a push after an expired AWS session offers sign-in in place. Tests: label matrix; confirm list. Review: the confirm names other people's commits in warn and needs a second click.

**G4-07 · Branches tab** (L): design 5.8 Branches: lists, detail, compare cards, merge / rebase / create PR entry points, tags, remotes, new-branch and delete dialogs, the clean-folder prompt. Owns `frontend/src/workspace/branches/`, locale `branches.ts`. Accept: matches `repo-branches-light-zh`, `-en`. Tests: dialogs and states. Review: no raw git terms in copy.

**G4-08 · History tab** (L): design 5.8 History: virtualised list with the lane graph, ref chips, toolbar, details, compare, reset / cherry-pick / revert / tag dialogs, file history view. Owns `frontend/src/workspace/history/`, locale `history.ts`. Accept: matches `repo-history-light-zh`, `repo-history-dark-en`. Tests: graph rendering snapshots; reset dialog copy. Review: the hard-reset confirm states exactly what is lost; focus on Cancel.

**G4-09 · Conflict screen** (L): design 5.9: operation bar, three groups, the mine / theirs / result editor with block bars, choice cards, finish into the composer. Owns `frontend/src/workspace/conflict/`, locale `conflict.ts`. Deps: C3-04. Accept: matches `repo-merge-conflict-light-zh`, `-dark-en`. Tests: block resolution state; labels for rebase. Review: the abort sentence is only shown when true.

**G4-10 · Stash UI** (S): design 5.8 Stash in the Changes left pane, with the stash popover and the apply / keep / branch / delete actions. Owns `frontend/src/workspace/changes/stash/`. Accept: matches `repo-stash-light-zh`. Tests: popover options; apply-with-conflict hands over to the conflict screen. Review: delete confirms; empty state.

**Gate.** Standard. Screenshots: repo-branches (2), repo-history (2), repo-merge-conflict (2), repo-stash (1). Manual: a scripted "day of git" on a fixture repository: branch, commit, rebase with a conflict, force push, cherry-pick, revert, reset, tag.

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

**Group B (serial).** **E5-W · Wiring** (S): adds `@codemirror/lsp-client@6.3.0`, `@codemirror/autocomplete`, `@codemirror/lang-go`, `@codemirror/lang-javascript` and the sanitizer.

**Group C (parallel).**

**E5-06 · Code tab** (L): design 5.3: files pane, editor groups with tabs and split, breadcrumbs, all states (no file, unsaved, changed on disk, large, binary, save failed, restored), per-folder tabs, Cmd+W. Owns `frontend/src/workspace/code/`. Accept: matches `workspace-code-light-zh`, `-dark-en`, `-dark-zh` (without the code-intelligence overlays). Tests: tab model; preview tabs; changed-on-disk decisions. Review: tree accessibility; unsaved edits mirrored within a second.

**E5-07 · Quick open, palette, outline** (M): Cmd/Ctrl+P with `:`, `@`, `#`; Cmd/Ctrl+K palette; the Outline pane. **Fable first** for the palette (not drawn). Owns `frontend/src/workspace/palette/`, `frontend/src/workspace/code/Outline.tsx`. Accept: a file opens in two keystrokes plus its name; the outline follows the cursor. Tests: mode switching; ranking display. Review: keyboard-only use.

**E5-08 · Find in files and Problems** (M): the bottom panel's Find results and Problems tabs. Owns `frontend/src/workspace/panel/Find*.tsx`, `Problems*.tsx`. Accept: matches `workspace-code-find-light-zh`. Tests: result grouping; replace-all confirm. Review: results are virtualised; ignored folders are not searched.

**E5-09 · Code intelligence in the editor** (L): `@codemirror/lsp-client` 6.3.0 wired to E5-03: hover card, completion with the side pane, go to definition, peek, references into Find results, diagnostics, status bar items, the install bar, held diagnostics in fresh workspaces. From S0-06: the app's own `Workspace` class (the library's default is not exported; one `EditorView` per tab); **pull diagnostics** (`textDocument/diagnostic`) for the native TypeScript server, about 30 lines the library lacks; a serial, batched send queue; the library renders server Markdown through `innerHTML`, so `sanitizeHTML` is a real sanitizer or raw HTML is off. Owns `frontend/src/editor/lsp/`. Accept: the overlays in `workspace-code-*` shots; hover Markdown is rendered with raw HTML off. Tests: against the fake server; an `<img onerror>` in a hover, a signature help and a `window/showMessage` stays inert. Review: hover content is untrusted; no server text in logs.

**E5-10 · Blame and editor links** (S): the blame margin in the Code tab, "File history" and "Open in editor" links from Changes, History and the conflict screen. **Fable first** for the blame margin. Owns `frontend/src/editor/blame/`. Accept: clicking a blame block selects that commit in History. Tests: block grouping from blame data. Review: author names shown as text only.

**Gate.** Standard. Screenshots: workspace-code (4). Manual: open the owner's largest Go and TS repositories; a day of reading and small edits without GoLand; memory with four task folders open (check A13); Windows: no `gopls.exe`, `tsc.exe` or `node.exe` left after quit (B12).

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
- **R7-05 · Documents** (S): README (both languages), SECURITY.md, THIRD_PARTY_NOTICES (adds go-pty's licence for the ConPTY wrapper, `marked`, `vscode-languageserver-protocol`, `vscode-jsonrpc` and the sanitizer), `docs/design/ui-spec.md` (plan 10). Review: every claim in the README is true of the build.
- **R7-06 · Security review** (M, Opus leads, Sonnet fixes): updater, installer script, token store, SSH binding, terminal data path, untrusted text, report socket, file confinement. Output: a findings list, all "must fix" closed.

**Group B (serial).**

- **R7-07 · Release candidates** (M): rc.1 and rc.2 through the test channel on clean macOS 15, macOS 26, macOS 27, Windows 10 and Windows 11 VMs: fresh install, upgrade from 0.0.1, rc1 to rc2 update, uninstall. Accept: every row of the test sheet passes on all five machines. Review: the sheet covers each refusal case of the updater.
- **R7-08 · Release** (S, owner present): rename the GitHub repository to `rudian/codepass` (then switch `REPO_URL` in `frontend/src/pages/settings/util.ts` and the `origin` remote to the new name), merge `codepass` into `main`, tag v1.0.0, run the release pipeline on the owner's Mac, publish, verify the update check from an rc build.

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

**Open questions for the owner after the milestone-0 gate** (the plan assumes the first answer of each until told otherwise):
1. Decision 3 was relayed as "3 a", which in the lettering below means "buy a certificate", while this block records "ship unsigned". Both reviews ask for a confirmation. Assumed: unsigned. If a certificate is bought, F1-10 gains a Windows signing step no spike has tested and F1-20 gains an Authenticode check.
2. Should app-started `claude` tasks get the flags of your own `claude` shell function (model, effort, channels)? Assumed: yes, the app keeps launching through your login shell (plan 4.7); the two spikes disagreed and review A kept the plan.
3. TypeScript code intelligence now uses the TypeScript 7 native server first, with typescript-language-server only for TypeScript 5 / 6 projects. Assumed: agreed.
4. On Windows, Ctrl+J inside a terminal is a line feed that tools use for a new line. Assumed: the switcher is Ctrl+J everywhere except inside a terminal on Windows, where it is Ctrl+Shift+J. Cmd+J on macOS is unaffected.
5. "Rename local folder" can move the folder to another parent on the same disk. A move to another disk is not offered in 1.0 (it cannot be undone as one step). Assumed: acceptable.
6. An ended session's last terminal screen is saved to disk so it can be read after a restart (it may contain whatever was on screen; it is deleted by Free up space). Assumed: on, with a setting to turn it off.

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
