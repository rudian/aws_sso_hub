# m2B: notes passed to the workflow

Milestone 2, group B runs A2-02, A2-03, A2-04, A2-05, A2-13, A2-06, A2-07 and F1-GF2 in parallel, `portBase: 9940`. Examples for the next group's `groupNote` and `notes` args.

## groupNote

Milestone 2, group B: eight tasks in parallel, each in its own worktree: A2-02 (terminal sessions and the data path), A2-03 (keep-awake manager), A2-04 (AI tools: detect, install, launch), A2-05 (task state signals), A2-13 (process tracker and force stop), A2-06 (workspace frame), A2-07 (terminal component) and F1-GF2 (Projects rows fit the window; it owns only frontend/src/pages/projects/projects.css and a new test). Your entry in tasks.md gives Build, Owns and Accept. The wiring task A2-W comes later (group C, after A2-08) and owns the shared files: service registration in internal/app/container.go and main.go, mock registration in frontend/src/api/mock/index.ts, services.ts, locale registration in i18n/index.ts, sidebar and similar go into notesForWiring with the exact lines. Exceptions for this group (orchestrator, 2026-11-17): A2-06 may add the workspace route to frontend/src/App.tsx and change openRepo in frontend/src/pages/projects/ProjectsPage.tsx (A2-W then skips the route); A2-07 owns frontend/package.json and frontend/package-lock.json to add the xterm packages; a Go task that needs a new module may add it to go.mod and go.sum (name it in your summary; the merge resolves both files). A2-13 may edit internal/pty/pty_unix.go for its follow-up FU-01a; no other task edits internal/pty. Group I merged A2-01 (internal/pty: spawn, resize, exit code, KillSession, the Windows ConPTY wrapper) and F1-GF (the milestone 1 gate fixes) into codepass: build on internal/pty and do not change it (a needed change goes into notesForWiring). Terminal contract shared by A2-02 (Go) and A2-07 (page), taken from the S0-01 spike (spike/terminal/main.go): a bound TerminalService with Read(session string, offset int64, max int) returning api.TerminalRead {data: base64, from, next, tail, gap (bytes skipped because the offset was trimmed; 0 = none), done (the session ended and every byte was read), prologue: base64, set only when gap > 0}, Write(session string, data string /* base64 */) and Resize(session string, cols, rows int); and the event "terminal:ping" with {session, tail, done}, coalesced to at most one pending per session. Keep these names and fields exactly, so the two meet without changes; a field you must change goes into notesForWiring. Many Accept items need the wired app, the real AI tools, a hidden window or the Windows VM: never claim them as done; list them under manualChecks with exact steps. Safety: the owner's installed AWS SSO Hub app may be running on this Mac: never run, quit, kill, move or trash it; never point any binary or test at the owner's real data folder or real home config (~/.gitconfig, ~/.aws, shell config such as ~/.zshrc, ~/.zprofile, ~/.bashrc, real repositories). If you start the real app at all, only as a development build with AWSSSOHUB_SANDBOX=<a new temp dir>, and quit it when done. A test or probe that starts a login or interactive shell sets HOME and ZDOTDIR to a new temp dir holding its own shell files, unsets BASH_ENV and ENV, and names the shell explicitly, so the owner's own shell config never runs. A test that stops processes stops only processes it started (through the session it created), never by name, never with pkill, killall or kill -1. Never start a real AI tool (claude, codex, opencode), not even with --version: tests use fake executables in a temp dir on PATH. Never read or write ~/.claude, ~/.claude.json, ~/.codex, ~/.config/opencode or the keychain; point any tool config location a test needs at a temp dir. No real package manager run (brew, winget, npm install -g), no real release, upload, tag push or key generation. Keep-awake tests use fakes: no test holds a real sleep assertion. Never contact AWS: tests use internal/awsapi/awsapitest, and a test that builds a Container must pass the fake Endpoints (internal/awsguard refuses non-loopback hosts in test binaries). Notifications: a development build shares the installed app's bundle ID, so no test, probe or run may send a real system notification, ask for notification permission, register a real category, remove a real delivered notification or open the real System Settings: use the fake. No real browser. No administrator-privileged command, no global config change, no GitHub repository, release, tag or token, no push. The repository is public: only fake data (acme, globex, initech, umbrella, alex-dev, @example.com addresses, 111122223333-style account IDs, d-1234567890 directory IDs, ~ for home paths); never write a real company name, person's name, account ID or repository folder name. Disk space: about 45 GB free; delete large temporary files you create. Part of the gate: CODEPASS_SCRUB_LIST=~/.codepass-dev/scrub-list.txt bash scripts/scrub-check.sh --tracked must exit 0 (never open, print or quote that list).

## notes["A2-02"]

You are A2-02. Read first: docs/research/spike-terminal.md and docs/research/spike-review-a.md (mode tracking, trimming, the gap prologue), plan 4.7 (the launch form and the explicit environment) and section 5's "Terminal data path", and the spike code on branch spike/terminal (spike/terminal/term/session.go, modes.go, modes_test.go, flood_test.go, bench_test.go, and main.go's TermService: git -C ~/Documents/code/aws_sso_hub show spike/terminal:<path>).

Facts checked by the orchestrator:
1. internal/terminal today holds 0.0.2's code that opens the system terminal app in a folder with AWS_PROFILE set (Build, Request, Launch), called by internal/app/systemservice.go OpenTerminal (the Accounts card's terminal button and the Terminal helper page). Keep that API and its tests working unchanged (move it into its own file if that helps). The session code is new, beside it. A2-W decides later what of the old code goes.
2. Build sessions on internal/pty (A2-01, merged). What A2-01 left for you: start with pty.Start(pty.Options{Argv: pty.LaunchArgv(shell, tool, args...), Dir, Env, Cols, Rows}); Env is the complete explicit environment (nil falls back to os.Environ(), so never pass nil). Keep draining Read until it returns io.EOF. Done() means the root process ended, not that the terminal finished (on Windows EOF comes only when the job is empty). To stop: KillSession() (it can block for about the grace time plus 5 s, so never on a UI thread), then Close(); on macOS Close does not end a pending Read, the Read returns EOF after KillSession. Resize after the console closed returns ErrClosed: swallow it. The terminal contract in the group note is yours to implement in internal/api/terminal.go and internal/app/terminalservice.go; the service's registration is A2-W's.
3. The child environment is built explicitly (plan 4.7): TERM, LANG and what the plan lists; never AWSSSOHUB_* or WAILS_*. Test it.
4. The scrollback file of an ended session goes to <data>/sessions/<task-id>/scrollback with mode 0600; tests use a temp data folder; make sure the backup, export and log code never picks it up (say how you checked).
5. Accept items that need the real app, a hidden window, App Nap or real AI tools (the flood whose child starts after the window is hidden, the 10-minute hide, bytes per hour from real Claude Code and OpenCode sessions) go to manualChecks with exact steps. A Go test with a consumer that never reads covers "the reader never blocks".

## notes["A2-03"]

You are A2-03. Plan 4.10 is the test matrix; design 5.14 has the Settings card A2-11 builds later.

Facts checked by the orchestrator:
1. Tests use a fake clock and a fake process runner for caffeinate, and fakes for NSProcessInfo and SetThreadExecutionState; no test holds a real assertion.
2. The macOS NSProcessInfo activity needs cgo (Objective-C): keep it in _darwin.go files, and make sure GOOS=windows GOARCH=amd64 builds and vets. SetThreadExecutionState runs on one locked OS thread on Windows (build and vet only here).
3. Processes a closed session left behind count as activity: A2-13 builds internal/proctrack at the same time. Define the small interface you need in your own package (for example a leftover count per session) and let A2-W connect the two (notesForWiring).
4. The mock keepawake.ts is yours; its registration in mock/index.ts is A2-W's.
5. The owner checks the matrix on the Mac with pmset -g assertions at the milestone 2 gate: give the exact steps under manualChecks.

## notes["A2-04"]

You are A2-04. Read first: docs/research/spike-signals.md (S0-03), plan 4.7 (launch, "Naming", the first message) and 4.8 (approval modes), docs/research/claude-usage-accounts.md R2, and F1-12's internal/toolinstall (reuse its runner, progress and cancel patterns).

Facts checked by the orchestrator:
1. Never run a real claude, codex or opencode (not even --version), brew, winget or npm. Detection and naming tests use fake executables in a temp dir on PATH, run through a login shell whose HOME and ZDOTDIR are a temp dir (the group note).
2. Never read ~/.claude, ~/.claude.json, ~/.codex or ~/.config/opencode. The Accept items about the real OpenCode service.json and check A18 (naming per tool on the owner's Mac) are the owner's: manualChecks.
3. The forbidden-flag test covers every flag the entry lists, and a description starting with -- reaches the tool only as message text (after -- for Claude Code and Codex, --prompt=<text> for OpenCode).
4. The naming call's environment leaves out ANTHROPIC_API_KEY and OPENAI_API_KEY; test it.
5. The service registration and the mock registration are A2-W's.

## notes["A2-05"]

You are A2-05. Read first: docs/research/spike-signals.md (S0-03), plan 4.8 (signals per tool, the fallback) and docs/research/claude-usage-accounts.md R4.

Facts checked by the orchestrator:
1. Report mode is a function in internal/app/reportmode.go; the dispatch in main.go that calls it before the app starts is A2-W's (put the exact lines into notesForWiring). Report mode exits at once and never waits for the app.
2. The socket lives under a temp dir in tests and accepts only the same user (peer credentials on macOS; on Windows a named pipe with a user-only ACL: build and vet only here).
3. The Claude Code plugin folder is written under the task's data folder (a temp dir in tests). Never write ~/.claude, ~/.codex or ~/.config/opencode, and never start the real tools: the Accept item with the real tools and the byte-identical check are the owner's (checks A7 and A8 decide the open questions); list them under manualChecks.
4. For the fallback, internal/pty's Session has Foreground() (TIOCGPGRP on macOS) and Members() (the job's processes on Windows); do not change internal/pty.
5. Payloads are never logged: test that a payload carrying a prompt is reduced to the forwarded fields before anything else sees it.

## notes["A2-13"]

You are A2-13. Read first: plan 4.8b (programs a session leaves behind) and design 5.6a.

Facts checked by the orchestrator:
1. Build on internal/pty (A2-01, merged) for the session leader, its session ID and Members() (on Windows, the job's processes). A process that calls setsid leaves the macOS session and is not killed by KillSession: that is why this tracker exists. Members() and KillSession can take a while; never call them on a UI thread.
1a. Follow-up FU-01a (yours; you may edit internal/pty/pty_unix.go for it, no other pty file): Members() waits for a whole KillSession today, because kill() holds k.mu through the grace period and the SIGKILL step (a probe measured 1.49 s with a 1.5 s grace). Serialize kill() with its own mutex and take k.mu only around each listing and the ended check, so Members() waits for one listing at most. Add a test that fails on the old code.
1b. Follow-up FU-01b (yours; plan 4.8b "After a crash" and your entry): on macOS a nohup child or a set -m background group survives when the app dies (internal/pty's TestCrashLeavesNohupAndJobControlGroups pins it). Save the tracked set (PID, start time, command name; never a command line) to <data>/sessions/leftovers.json with mode 0600 whenever it changes, and at start list the survivors (same PID and start time) as leftovers that Force stop can end. Tests with a temp data folder and fake process tables; a real-process test may start its own nohup child under its own session and stop only that.
2. One ps call per tick on macOS (ps -axo pid=,ppid=,pgid=,sess=,lstart=,pcpu=,rss=,comm=), parsed by a pure function tested on fake tables: re-parenting to launchd, setsid, PID reuse with a later start time, a process that ignores SIGTERM.
3. Real-process tests start their own session through internal/pty and stop only their own processes (the group note); never signal a PID outside the test's own set.
4. Windows: the Job Object's process list; build and vet only here; the VM test is the owner's (manualChecks).
5. Command lines may be shown in the UI but never written to the log.
6. The service registration and the keep-awake connection (A2-03 counts leftovers as activity) are A2-W's.

## notes["A2-06"]

You are A2-06 (frontend). Read first: design 4.2 and 4.3 (the workspace and its keyboard map), design 3 (motion), and the mockup shots workspace-ai-light-zh, workspace-ai-dark-en, workspace-code-light-zh and workspace-changes-light-zh in docs/design/codepass-mockup/shots/ (the frame: header, rail, tabs, status bar, bottom panel).

Facts checked by the orchestrator:
1. In this group you may add the workspace route to frontend/src/App.tsx and change openRepo in frontend/src/pages/projects/ProjectsPage.tsx, so opening a repository (the row, and the Projects search's Enter and Cmd/Ctrl+Enter) goes to the workspace. Keep F1-22's handlers in App.tsx as they are.
2. The six tabs show placeholders where later tasks fill them: the AI tasks tab is A2-09's, the bottom panel's Terminal tab A2-10's, and the terminal component A2-07's (built at the same time; leave a slot, do not import it).
3. The rail's update pip uses useUpdateIndicator() from pages/update/useUpdate.ts.
4. The repository settings sheet reuses what exists: the bind popover (pages/repos), the identity UI (F1-16); AI defaults are a placeholder until milestone 8.
5. UI checks at 1440x900 and at the minimum window size (980x660), light zh-CN and dark en, against the frame parts of the shots above; the Windows frame variant through the mock's platform switch (see the comments at the top of the mock files).

## notes["A2-07"]

You are A2-07 (frontend). Read first: docs/research/spike-terminal.md and the spike page code (git -C ~/Documents/code/aws_sso_hub show spike/terminal:spike/terminal/frontend/src/main.js): the pull-by-offset loop, the replay rules and the nudge.

Facts checked by the orchestrator:
1. You own frontend/package.json and frontend/package-lock.json in this group: add @xterm/xterm and the addons you need (Unicode 11, search, web links, fit) at released versions that are at least two weeks old. Follow the node_modules rule in your prompt: remove only the link, then npm ci in your worktree.
2. The backend is built at the same time (A2-02): code against the terminal contract in the group note, behind a small client interface, and test with a fake. The real connection is A2-W's.
3. Accept items that need the wired app (the S0-01 list in the real app, check A1 Chinese input, check A2 Shift+Enter) go to manualChecks. Vitest covers the re-sync logic, dropping replies on both onData and onBinary during replay, the nudge rate limit, link classification and the key map (Shift+Enter sends ESC CR).
4. No terminal text is ever turned into HTML. Theme colours come from the --term-* tokens (F1-08).

## notes["F1-GF2"]

You are F1-GF2: make the Projects table fit every window width from 980 up (follow-up FU-GFa, measured by F1-GF's verifier; it was on codepass before F1-GF).

Facts measured by the verifier in the mock (?page=projects&scenario=done, en and zh-CN):
1. The row's actions column (the More button that opens the row menu) is clipped at window widths from 980 to about 1101 and from 1181 to about 1281. At 1200x780 (the default window, main.go Width 1200) every row's More button sits at x=1225-1251 while the collapse wrapper (.proj__body > div, overflow hidden) clips at 1170. At 980x660 the buttons sit at 1045-1071 against a clip edge at 950, so the whole 140px actions column is hidden.
2. The wide template needs a window of at least about 1282px; the narrow one needs at least about 1102px. The narrow template starts at `@media (max-width: 1180px)` in frontend/src/pages/projects/projects.css (about line 119).
3. Fix: move the breakpoint to where the wide template stops fitting (about 1290px), then make the narrow template fit at 980 (it needs about 140px less): smaller minimums, hiding the branch column below about 1100px, or a sideways scroll inside .proj__body > div that keeps overflow-y hidden for the collapse. Your choice; say why.
4. You own only frontend/src/pages/projects/projects.css and a new test file. F1-GF's changes are merged (the status column minimum became 230px with nowrap lines; D36's remote-name tag can drop to its own line): keep them.
5. UI checks with your own Vite server: ?page=projects&scenario=done&theme=light&lang=zh-CN and ...&theme=dark&lang=en at 980x660, 1100x780, 1200x780, 1281x800 and 1440x900 (zsh docs/plan/codepass/shoturl.sh <out.png> <w> <h> "<url>"). In each, every row's More button is fully visible; open one row menu. Also check the search state (&search=da) and a collapsed project.
