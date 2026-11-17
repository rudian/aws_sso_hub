# m2C: wiring notes

Milestone 2, group C ran A2-08 and F1-SG side by side (workflow `wf_57a42d83-d76`, resumed once after a usage-limit stop). Both were approved: F1-SG after 2 fix rounds, A2-08 after 3.

**This is the last Go group.** On 2026-11-17 the owner decided to move the program and the UI to Rust (gpui + gpui-component), with the requirements unchanged. A2-W and group D do not run in Go. These notes and `m2B-wiring.md` stay as the reference for the Rust port: they say how the task lifecycle, the terminal, the process tracker, keep-awake and the task-state signals connect, and which edge cases the reviews found.

## Merges and gate

- `63a150c` A2-08, `70e3c41` F1-SG.
- Integration: 72cd499 codepass: integrate A2-08, F1-SG: added shellguard.RefuseInTest("terminal.Open") in internal/terminal/terminal.go Open (before execx.Start/Command), as F1-SG's wiring notes ask.
- Gate (merge step): go vet clean; go test ./... all 44 packages ok; windows/amd64 go build ok; frontend typecheck ok, vitest 81 files / 1005 tests passed, build ok, check:dist ok; scrub-check --tracked exit 0. Pushed head 72cd499.
- Note: Bindings regeneration was NOT committed: wails3 generate bindings only produced deletions (302 lines, including AIToolService bindings and AI* models that frontend/src/api/mock/aitools.ts imports, so typecheck failed) because AIToolService and TaskService are not yet registered in main.go (wiring is A2-W's job). I reverted the regenerated output, so no 'codepass: regenerate bindings' commit exists. Bindings must be regenerated once A2-W registers the services.
- Note: Plain `go test` prints ld linker warnings about macOS version (harmless, exit 0).

## A2-08 · Task lifecycle

Accepted deviations:

- Added `terminal.Manager.SetOnInput(func(id, data))`, which Write calls for live sessions. This is beyond FU-A2-02b and FU-A2-02d, but TerminalService.Write goes straight to Manager.Write, so this is the only way to send keystrokes to the tracker and keep-awake without editing TerminalService. NewTasksManager connects it.
- FU-A2-05c is built as two helpers in taskstate/input.go. `CanAnswer` (Enter, a lone Esc, or a digit, outside escape sequences) decides OwnerInput. `IsTyping` (text, Enter, Esc, or arrow and editing keys; not focus, CPR, DA, OSC or mouse replies) decides Touch. The tasks.md fix only named the first; the second keeps ordinary typing renewing keep-awake's ceiling.
- FU-A2-02d is fixed in code. A reused ID's log starts one byte past the old tail, so a page at the old tail gets gap > 0 and the default prologue. A2-07 shows no gap notice for this. A2-07 needs no change.
- FU-A2-02b has a third guard beyond the three fixes in the follow-up: an archive generation counter, so a file read before a drop is never cached after it.
- Soft cap default is 6, from plan 4.8 and design 5.6. The design's "fifth concurrent task" example says 4 running, which conflicts. The cap is `Deps.SoftCap`, so the Settings value replaces it. The mock `?aitasks=cap` scenario uses 4.
- Own-workspace is refused with E_INVALID_INPUT, field "where" (milestone 3). Only the main workspace works.
- Resume does not check the tool's own conversation store (never read ~/.claude, ~/.codex or opencode). "Found nowhere, start no session" (design 5.6) is A2-09's or milestone 8's. Resume without any session ID (Codex never reported one) fails with field "session".
- A2-08 does not Release a task's terminal log at exit. Releasing at once races the scrollback write (FU-A2-02b case 2). Instead logs of ended tasks beyond the newest 8 are released, and only once their scrollback file exists.
- Tool-missing is not pre-checked. A missing tool exits 127 in the login shell and the task shows failed. The install card (A2-09) uses AITools.Detect before Create.
- The frontend mock hand-writes the TaskService types, like proctrack.ts, because the service is not registered and the bindings are not regenerated. The mock also calls setRunningTasks whenever its running count changes.
- No `Co-Authored-By` or session lines in the commit message, as this task's instructions say (and scrub-check scans commit messages for attribution).

Notes for wiring (A2-W in Go; read them for the Rust port):

- Container fields (A2-W). `c.Terminal` is built once with NewTerminalManager(c) and the ProxyEnv from the group-B notes. `c.Proc = proctrack.New(proctrack.Options{DataDir: c.Paths.AppDataDir})`. `c.Awake = keepawake.NewDefault(...)` with a leftovers adapter whose Count(session) is `c.Proc.Leftovers(session).Count`. `c.TaskRelay = &tasks.Relay{}` and `c.TaskState = taskstate.NewTracker(taskstate.Options{}, c.TaskRelay.OnChange)`.
- Hook socket in main.go: `srv, err := taskstate.Listen(taskstate.Endpoint(p.AppDataDir))`, then `srv.Serve(c.Tasks.Sink(), func(problem string){ logger.Debug(problem) })` and `defer srv.Close()`. Sink applies the report and also Touches keep-awake. Do not pass the raw tracker.
- Build the service after all of the above, and pass `Endpoint: srv.Endpoint()`: `c.Tasks = app.NewTasksManager(c, app.TasksWiring{Terminal: c.Terminal, Tracker: c.TaskState, Relay: c.TaskRelay, Endpoint: srv.Endpoint(), Proc: c.Proc, Awake: c.Awake, AITools: aiToolSvc})`. It connects the relay and `Terminal.SetOnInput(svc.Input)` itself. `aiToolSvc` is the `*AIToolService` that is also registered. Exe defaults to os.Executable.
- Services list, in this order: NotifyService first, then the terminal service, then `app.NewProctrackService(c.Proc, c.emit)`, then `application.NewService(app.NewTaskService(c.Tasks))`. The task service's ServiceStartup calls ProcChanged after proctrack restored leftovers.json. Its ServiceShutdown only closes the service; the orderly stop is StopAll.
- Events in internal/app/events.go: `application.RegisterEvent[api.TasksState](api.EventTasksChanged)` and `application.RegisterEvent[api.TaskOpen](api.EventTaskOpen)`. Add `'tasks:changed': TasksState` and `'tasks:open': TaskOpen` to EventPayloads in frontend/src/api/events.ts, then regenerate the bindings.
- Hooks, one line each. `c.Lifecycle.SetTasksRunning(c.Tasks.TasksRunning)` (cached flag, UI-thread safe). Where CloseAction returns CloseHide, call `c.Tasks.WindowHidden()` once; it returns at once and sends the first-time notice. `c.Proc.SetOnChange(func(){ c.Tasks.ProcChanged(); <emit the proctrack event> })`. Never call back into the keep-awake Manager from its own OnChange (FU-A2-03f).
- Quit path (A2-14): call `c.Tasks.StopAll()` off the UI thread. It stops the tasks, runs Terminal.Shutdown(10 s) and Proc.ForceStopAll (it also covers panel shells), and it can block about 15 s. Then exit. Its StopAllResult carries the survivors. The dialog's list comes from `c.Tasks.LiveSummary()`.
- Frontend (A2-W): register `mock.tasks = mockTaskService` and `export const Tasks = useMocks ? mock.tasks : TaskReal`. Replace the hand-written types in mock/tasks.ts with the generated ones (`typeof TaskReal`). Subscribe to 'tasks:changed' once in the events layer and call `setRunningTasks(state.running)`; the mock already does this. Update wiring.test.ts's expected mock list.
- Settings: the soft cap (default 6) is not in Settings yet. Pass `Deps.SoftCap` from the Settings store when it exists. NewTasksManager leaves it nil, so the default is used.
- A2-09: on E_INVALID_INPUT with `params.field == "cap"`, show the "N tasks are already running" confirm (running and cap are in params) and send the same request again with Confirmed true (Resume's second argument). After Resume on a task whose terminal was open, the page must reset its terminal view: the log continues, and a page at the old tail gets a gap and prologue (FU-A2-02d).
- Windows: BuildLaunch returns ErrBadMessage for OpenCode when its entry point is missing; the service maps it to E_INVALID_INPUT with field "description". The page should word it. On Windows `Others` (programs the tool started) includes the tool itself (FU-A2-13a).
- F1-SG: no test of mine starts a login shell with the owner's HOME. The real-pty tests set HOME and ZDOTDIR to a temp folder with their own shell files, BASH_ENV and ENV empty, and a minimal base environment. If F1-SG's guard refuses any of them, they are in internal/tasks/real_unix_test.go (internal/tasks is mine; the guard should accept a temp HOME).
- Order in main.go / Container (A2-W): srv, err := taskstate.Listen(taskstate.Endpoint(p.AppDataDir)); c.Tasks = app.NewTasksManager(c, app.TasksWiring{..., Endpoint: srv.Endpoint()}); srv.Serve(c.Tasks.Sink(), ...); defer srv.Close().
- Right after proctrackSvc := app.NewProctrackService(tracker, c.emit), call app.ConnectProctrack(proctrackSvc, c.Tasks). Do not call c.Proc.SetOnChange yourself: it would replace one of the two targets.
- TaskService no longer has Delete (and the mock TaskServiceShape no longer has it): the generated bindings and frontend/src/api/services.ts must not list it. Deletion of task data is C3-12's Preview/Delete.
- tasks.Deps.Procs now needs Tick() error; *proctrack.Tracker already has it, so no wiring change.
- None new. Service gained an unexported test-only field tailHook (nil in production); nothing to wire.

Manual checks (for the Go build; the Rust build needs the same behaviour checked):

- On the Mac, in a development build with AWSSSOHUB_SANDBOX set to a new temp dir: start a task per tool with a real description and with none. Check the naming (title in the description's language, English slug), the 'New task n' label, and that the first message renames only the label. Check A18 and A7 in manual-checks.md cover the flag forms.
- 'Stop all and quit' end to end (needs A2-W and A2-14): run two tasks whose tools start a dev server (one with `nohup`, one with `set -m`), choose Stop all, and check with `ps` that nothing of the tasks is left. Relaunch and check the tasks are listed as stopped and resume into their own conversation (Claude Code `--resume <id>`, OpenCode `--session ses_<id>`).
- Closing the window while a task runs: the first time a 'Codepass 还在运行 / Codepass is still running' notification with no sound appears, and never again, also after a restart (it is remembered in tasks.json). Clicking it shows the window.
- A task that turns to waiting while the window is in the background: a notification titled with the repository's local folder name and 'Claude Code 在等你回答' appears, with sound. Clicking it shows the window and opens that task's terminal (tasks:open). Answering in the terminal takes the delivered notification back. With the window in front, no notification is sent.
- Keep-awake with real tasks (pmset -g assertions): held while a task runs or waits within the grace time. With the last task ended but a dev server still running, it stays held. After Force stop (or the server exiting), it is released.
- Permission prompt with a real Claude Code: focus in and out of the terminal and the first arrow key do not turn the waiting task to running; Enter, Esc or a digit does (FU-A2-05c). Chinese input and Shift+Enter are A2-07's checks.
- Shell startup noise (FU-A2-05a): with a slow login shell (for example brew shellenv, nvm or conda in ~/.zshrc), a Claude Code task must not flash 'ended' or 'running (no signal)' while the shell starts. An empty-description task stays 'starting' until the first prompt. This needs the owner's real shell setup.
- Windows 11 VM: build and vet only here. Start a task, Stop it (no child left in Task Manager), check `<data>\sessions\<id>` holds the hook plugin and `tasks.json` is written, and that OpenCode on npm gets a prompt with cmd.exe characters or a clear E_INVALID_INPUT.
- Time Machine: after the first start on the Mac, `tmutil isexcluded <data>/sessions` says Excluded (the call is `tmutil addexclusion`, run only by the app, never by tests).

## F1-SG · Tests never run the owner's shell

Accepted deviations:

- No test needed AllowLoginShellForTest in the existing suites: with the lookup skipped by default, none of them depends on the owner's login-shell PATH. The helper is covered only by new tests in internal/execx.
- Guarded starts: execx.Command, Start and Run, pty.Start, aitools.RealRun. execx.Command and Start return the refusal through cmd.Err, so Start and Run fail with it. execx.Run checks again with cmd.Env, because identity.ProbeShellEnv replaces cmd.Env after Command. internal/identity and internal/terminal start shells only through execx or pty, so they are covered without edits. Their own raw starts were checked: no other non-test code starts a shell with -l, -i or -lic. The remaining exec.Command calls run ps, osascript, scutil, open and similar tools. toolinstall runs bash --noprofile --norc, which is not a login shell.
- Windows is covered by the same guard call in pty_windows.go, but nothing was run on Windows beyond cross-compile and vet.
- The temp check is os.TempDir() with symlinks resolved on both sides. A HOME under /tmp on macOS is refused unless TMPDIR points there.
- Only HOME and ZDOTDIR are checked, as specified; BASH_ENV and ENV are not checked by the guard. The opt-in helper clears them, and the trap script unsets them.

Notes for wiring (A2-W in Go; read them for the Rust port):

- Nothing to wire. No shared file was changed.
- No test in internal/terminal, internal/taskstate or internal/tasks needs the opt-in. The full trap run passed with those packages as they are, including A2-08's tree at the base commit. A2-08's new tests that start a shell with -l or -i (for example pty.LaunchArgv with a real shell) must set HOME and ZDOTDIR to t.TempDir() or use execx.AllowLoginShellForTest(t), or Start returns shellguard.ErrRefused.
- A test that wants the real login-shell PATH lookup calls execx.AllowLoginShellForTest(t). It returns the temp HOME, which is also ZDOTDIR, so the test can write its own .zprofile there.
- internal/terminal (A2-08): in terminal.Open, before execx.Start / execx.Command (the osascript path on macOS and the powershell.exe / wt.exe path on Windows, which load the owner's shell or profile), add: if err := shellguard.RefuseInTest("terminal.Open"); err != nil { return err }  (import github.com/rudian/codepass/internal/shellguard). It records the refusal in a test binary and returns nil in production. Any A2-08 test that really must reach it has no opt-in; such tests should stub the launcher.
- Behaviour change for other packages' tests: zsh and fish are now refused on every start (even -c) when HOME or ZDOTDIR is outside temp, and pty.Start refuses a bare shell with a non-temp HOME. Trap run found no current test affected; a test in internal/terminal, internal/taskstate or internal/tasks that starts a shell should set HOME and ZDOTDIR to t.TempDir().
- AllowLoginShellForTest only takes effect when Options.DisableLoginShell is off: the test must call execx.Configure(execx.Options{}) first (and restore Configure(Options{DisableLoginShell: true}) in t.Cleanup), and set SHELL with t.Setenv, otherwise the inherited SHELL runs.
- Command's refusal is no longer recorded by Command itself; Run records it. A caller that uses the *exec.Cmd directly (cmd.Output, cmd.Start) still gets ErrRefused from cmd.Err but no Refused() entry.

Manual checks (for the Go build; the Rust build needs the same behaviour checked):

- Run scripts/shell-trap-test.sh in your own terminal once and confirm it prints PASS. It uses a temp trap folder and never your real home.
- Check that the installed app behaves the same on first launch: the login-shell PATH lookup runs as before, because the guard is off in a production binary. The tests prove this only through the testing.Testing() switch.

Deferred by the fixer:

- nit internal/terminal/real_unix_test.go:49 flaky wait (Ended without RootDone): internal/terminal belongs to A2-08; route to A2-08 or A2-CL with the fix: wait for st.Ended && st.RootDone
- FU-A2-04b winshim test: already checks a fake codex on PATH found by execx.LookPath and asserts argv[0] stays bare, so a return to LookPath fails it; no change made
- I did not add a process-level probe for zsh -N through execx.Run or pty.Start. Its scratch home would have to sit outside os.TempDir(), which means a real folder outside temp. The shellguard.Check unit cases cover the same decision, and both guarded call sites call Check. The trap run passed.

## Follow-ups

All are nits. Owner: the Rust port of the same code (the Go code is not changed any more).

- **FU-A2-08a** (nit) `internal/tasks/fu_a208_test.go` about line 602: If the gap fix regresses, Resume #2 (called synchronously here, and in the same way at line 76 in TestTwoConcurrentResumesStartOnlyOneRun) blocks inside gateTerminals.Start. The test then hangs until the go test timeout instead of failing; a scratch mutation run showed this. The fixer saw the same thing (a 120 s hang). Fix: Call the second Resume in a goroutine and fail if it has not returned within about 2 s. Alternatively, have gateTerminals.Start return an error when calls > 1. Either way a regression fails fast with a clear message.
- **FU-A2-08b** (nit) `internal/app/taskservice.go` about line 120: The wiring note says to call ConnectProctrack 'right after NewProctrackService', and the round-1 notesForWiring says the same. If A2-W builds the proctrack service before c.Tasks, ConnectProctrack(ps, nil) installs a callback that panics on the nil *tasks.Service (ProcChanged locks s.mu) at the first proctrack change. That panic happens on the tracker's goroutine and takes down the app. Fix: Change the note to 'after both NewProctrackService and NewTasksManager'. Also make ConnectProctrack panic at once on a nil argument, so a wrong order fails at wiring time and not at the first leftovers change.
- **FU-SGa** (nit) `internal/shellguard/shellguard.go` about line 123: parseFlags still exempts a few contrived argument forms by mistake. (1) zsh: a later +f, -o rcs or +o norcs turns the startup files back on, but noRC stays set. Probe with a scratch HOME/ZDOTDIR outside temp: execx.Run(execx.Command(ctx, "/bin/zsh", "-f", "-o", "rcs", "-c", "echo ran"), nil) was allowed and wrote the .zshenv marker. (2) fish: a value attached to a value-taking letter is scanned as more flags. Check("fish", {"-fN","-c","x"}, bad) and Check("fish", {"-cN"}, bad) are both allowed, although getopt reads N as the value (features 'N', command 'N') and fish then reads its config. No current test uses these forms. Fix: Make noRC follow the last setting in argument order: zsh -f, -o norcs and --no-rcs set it; +f, -o rcs and +o norcs clear it. For fish value-taking letters (c, C, d, D, f, o, p): when the letter is not the last in the flag, the rest of the flag is its value, so stop scanning that flag; otherwise skip the next argument. Add D to the fish letters. Add these cases: zsh -f -o rcs -c, zsh -f +f -c, fish -fN -c x and fish -cN, all refused with a bad HOME.
- **FU-SGb** (nit) `internal/execx/execx.go` about line 301: Command overwrites a LookPath error already in cmd.Err with the shellguard refusal, and Run then clears that refusal. The lookup error is lost, and Run execs the bare relative name. Probe in a test binary with the process HOME outside temp and PATH without zsh: Command(ctx, "zsh", "-c", "echo hi") left cmd.Err = the refusal instead of exec.ErrNotFound. After cmd.Env was set to a temp HOME, Run returned 'fork/exec zsh: no such file or directory', and it would have run ./zsh from the working folder if one existed. This also bypasses exec.ErrDot. It happens only in test binaries; in production Would returns nil. Fix: Keep an earlier error: `if err := shellguard.Would(name, args, cmd.Env); err != nil && cmd.Err == nil { cmd.Err = err }`. Add a test: bare shell name not on PATH, temp HOME in cmd.Env; Run must return exec.ErrNotFound.
- **FU-SGc** (nit) `internal/shellguard/shellguard_test.go` about line 135: In TestShellNamedByTheSHELLVariableIsAShell, the second case's comment says 'SHELL unset in env and process'. In fact Check falls back to the process SHELL, which is /bin/zsh here. The test would fail on a machine whose login shell's base name is nu. Fix: Call t.Setenv("SHELL", "/bin/zsh") (or t.Setenv("SHELL", "")) before the second Check, and correct the comment.
- **FU-SGd** (nit) `internal/terminal/real_unix_test.go` about line 49: a flaky wait (Ended without RootDone); wait for `st.Ended && st.RootDone`. Deferred by F1-SG's fixer because the file belongs to A2-08.
