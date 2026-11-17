# groupI: wiring notes

Group I ran F1-GF (the 19 fixes of the milestone 1 gate review) and A2-01 (PTY and process control, milestone 2) in parallel (workflow `wf_167aa421-9b2`, `portBase` 9880). Both were approved after two fix rounds each.

## Merges and gate

- `4a7e6b5` codepass(F1-GF), `90f1772` codepass(A2-01), `a0ed06f` codepass: regenerate bindings (only the order of the `ErrConfigShared` enum in the generated models).
- The first merge attempt stopped at its precondition: the orchestrator had an uncommitted change in `tasks.md`. Nothing was merged then. After the commit `d8e97cb`, the merge step was re-run alone (resume with a changed merge prompt; the other agents replayed from the cache).
- Gate on `a0ed06f`: vet, Go tests, Windows build, typecheck, frontend tests (863), build, check:dist and scrub all pass.

## F1-GF

What it did: every item of `reviews/m1-gate-review.md` "To fix (19)". Facts for later tasks:

- **D7.** One rule decides which rows need an action: `needsAction()` in `frontend/src/pages/projects/model.ts` (a CodeCommit row that needs a binding, or a bound row whose portal is expired, signed out or failed; an expiring sign-in does not count). The "To fix" filter, its number and the sidebar count all use it; `Sidebar.tsx` imports `actionCount` from that file. The backend's `counts.toFix` is no longer used for the number.
- **D8.** `MiddleText` in `ProjectRow.tsx` measures the path with a canvas and a `ResizeObserver`; `fitMiddle()` in `pages/repos/model.ts` builds the shortened text. The `.midtrunc__head` and `.midtrunc__tail` spans are gone. jsdom has no layout, so tests see the full path.
- **D10.** The status column's minimum is 230px (was 170px); status lines do not wrap and end with an ellipsis.
- **D14.** The still bar under reduced motion is only in `pages/setup/setup.css` (for `.tool-row__progress`): it shows full width at 40% opacity. `styles/tokens.css` still keeps every other indeterminate bar turning slowly under reduced motion (a styles test pins it): follow-up FU-GFb.
- **D17.** In the failed state the footer always reads "What is already installed is untouched."
- **D28.** The Settings subtitle shows only the platform text when the app reports no version.
- **D32.** The app has no home folder value; the Projects page derives it from the scan root (`homeFromRoot`) and passes it to `CloneModal` as the optional `home` prop.
- **D34.** The clone modal has the class `clone-modal` with 40px top padding; it already scrolled at 980x660.
- **D36, D37.** The remote-name tag drops to its own line when it does not fit; matched letters are bold with an underline and a light tint.
- **T1 to T4.** `internal/app/startup.go` and `internal/notify/signin.go` carry the new texts; design 6's table in `ui-design.md` too. Plan 4.15's example was changed by the orchestrator (`d8e97cb`).
- **FU-17e.** The mockup shows "Reopen now" disabled while downloading; `shots/update-light-zh.png` was re-taken.
- `RepoName.test.tsx` reads its CSS from disk through a dynamic `import('node:' + 'fs')`, because the frontend has no `@types/node` and vitest serves CSS imports as empty strings: follow-up FU-GFc.

Accepted deviations: the design file path (the brief named `docs/design/ui-design.md`; the file is `docs/plan/codepass/ui-design.md`); D7's rule lives in `pages/projects/model.ts`; D8 measures in the page instead of CSS; D14's still bar is local to Setup.

## A2-01

What it did: `internal/pty`, one `Session` interface with a platform type behind `Start(Options)`. macOS: creack/pty v1.1.24 (`go.mod`), the child leads a new terminal session, `KillSession()` signals every process group of that session (SIGTERM, the grace time, SIGKILL). Windows: the app's own ConPTY wrapper over `golang.org/x/sys/windows` (from the spike's `winpty_windows.go`, credited in `THIRD_PARTY_NOTICES.md`), the child inside a job object with kill-on-close. Facts for later tasks:

- Start with `pty.Start(pty.Options{Argv: pty.LaunchArgv(shell, tool, args...), Dir, Env, Cols, Rows})`. `Env` is the complete explicit environment: nil falls back to `os.Environ()`, so never pass nil (A2-02).
- Keep draining `Read` until `io.EOF`. `Done()` closes when the root process exits; on Windows EOF follows only when the job is empty. Treat `Done` as "root ended", not "terminal finished" (A2-02).
- To stop: `KillSession()` (blocks up to about the grace time plus 5 s on macOS; never on a UI thread), then `Close()`. On macOS `Close` does not end a pending `Read`: the reader ends after `KillSession`. On Windows `Close` ends the tree at once. Late calls on an ended session are harmless. Call `KillSession` for every session at app quit (A2-02, A2-08).
- `ExitCode()` is -1 until `Done`, then the exit code, or 128+n when signal n ended the process (macOS).
- `Resize` after the console closed returns `ErrClosed`; swallow it (A2-02).
- `Foreground()` (TIOCGPGRP) on Unix and `Members()` on Windows serve A2-05's fallback; `Members()` and the Windows job list serve A2-13. A process that calls `setsid` leaves the macOS session and is not killed by `KillSession` (A2-13 tracks it).
- `internal/terminal` (the old "open the system terminal" helper) is untouched.

Accepted deviations: `KillSession()` takes no argument (the grace time is `Options.Grace`, default 5 s); `Close()` on macOS only releases the terminal; the Windows pseudo console is closed off the goroutine as soon as the root exits (whether that loses tail output on old Windows 10 builds is check B8); the spike's `unsafe.Pointer(hpc)` became `*(*unsafe.Pointer)(unsafe.Pointer(&hpc))` for go vet; `procs_other.go` reads `/proc` so the package builds on Linux (not a target). Deferred by the fixer and accepted: `revoke(2)` in `Close` on macOS; a chain test that cannot force a 1 to 2 ms race on every run (two injected-table tests cover it).

## Follow-ups

- **FU-GFa** (major; on codepass before F1-GF) `frontend/src/pages/projects/projects.css` about line 119: the Projects row's actions column is clipped at window widths from 980 to about 1101 and from 1181 to about 1281, including the default 1200x780 window, so the row menu cannot be reached with the mouse. **F1-GF2** (milestone 2 group B).
- **FU-GFb** (nit) `frontend/src/styles/tokens.css`: under reduced motion the indeterminate progress bar keeps turning slowly everywhere except Setup. Decide one rule for the app (a still bar, as Setup now has) and change the styles test with it. **A2-CL**.
- **FU-GFc** (nit) `frontend/src/components/RepoName.test.tsx`: the dynamic `import('node:' + 'fs')`. Add `@types/node` to the frontend's dev dependencies and use a plain import, or test the class names instead of the CSS text. **A2-CL**.
- **FU-GFd** (nit) plan 4.15's "3 个账户" example: done by the orchestrator in `d8e97cb`.
- **FU-A2-01a** (minor) `internal/pty/pty_unix.go` about line 306: `Members()` waits for a whole `KillSession`, because `kill()` holds `k.mu` through the grace time and the SIGKILL step (a probe measured 1.49 s with a 1.5 s grace). Serialize `kill()` with its own mutex and take `k.mu` only around each listing and the ended check. **A2-13** (it may edit that file in group B).
- **FU-A2-01b** (minor; planner decision) after an app crash on macOS, a `nohup` child or a `set -m` background group survives (`TestCrashLeavesNohupAndJobControlGroups` pins it). Decided 2026-11-17: no guard process; **A2-13** saves the tracked set to `<data>/sessions/leftovers.json` and lists the survivors at the next start with Force stop (plan 4.8b "After a crash").
- **FU-A2-01c** (nit) `internal/pty/pty.go` about line 75: the `Close` doc should say that with a `Read` pending the master stays open until that `Read` returns, so `Close` alone hangs nothing up; the hang-up it describes is what an app crash does. **A2-CL**.

## Manual checks (the owner)

- F1-GF: check C29 (new): the path ellipsis follows a window resize; the Settings subtitle; the still Setup bar under reduced motion; the clone modal at 980x660 and the `~` path; the two startup dialogs on a copy of the data folder. T3 and T4 as real notifications are part of C22 to C24.
- A2-01: check B8 (rewritten): first the cross-compiled test binary of `internal/pty` on the Windows 11 and Windows 10 1809 VMs (spawn, resize, kill tree, kill from Task Manager, no hang in `ClosePseudoConsole`), then the in-app terminal once milestone 2's terminal is wired. Optional, on the owner's Mac with the real tools once A2-08 exists: stopping a Claude Code task started through the owner's `claude` shell function leaves no process.
