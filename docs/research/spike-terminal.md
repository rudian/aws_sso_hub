# Spike S0-01: terminal in the webview

Date 2026-11-16. Branch `spike/terminal` (commit e7e6dc7, worktree `.claude/worktrees/wf_e2601f60-0f7-1`, code in `spike/terminal/`, own Go module, root `go.mod` untouched). Never pushed.
Machine: macOS 27.2.1 arm64, Go 1.27.4, Wails v3.1.0, `@xterm/xterm` 6.0.0 (+ addon-fit, addon-unicode11), creack/pty 1.1.24. Load average was 10 to 45 during runs (other sessions), so all timings are pessimistic.

## What was built

- `term/session.go`: one PTY per session (`pty.StartWithSize`, child is a session/group leader). **One reader goroutine** does `pty.Read` into a 64 KB buffer, appends to a `Log` (byte slice + absolute `base` offset, trimmed to `cap` when it passes `2*cap`), then a non-blocking send on a capacity-1 `Ping` channel. It touches nothing else.
- `Log.Read(offset, max)` returns a **copy** (so the lock is never held while Wails serialises), the real `from/next/tail`, and `gap` = bytes skipped when the offset was already trimmed.
- `main.go`: Wails service `TermService` with `Spawn / Read / Write / Resize / Kill / Current / Status`, and a **separate pinger goroutine** that turns `Ping` into a small `term:ping {id, tail}` event (15 ms rate limit). If `Emit` blocks, only the pinger waits.
- `frontend/src/main.js`: xterm.js (DOM renderer, Unicode 11 addon, fit addon). Pull loop: on ping, visibilitychange or a 400 ms timer, if the page is visible: `Read(id, off, 256 KB)`, base64 decode, `term.write(bytes, cb)`, await the callback before the next pull (xterm's own backpressure), repeat until `next >= tail`. Shift+Enter sends `ESC CR`.
- `term/modes.go`: a tracker of DEC private modes (alt screen 1049, mouse 1000/1002/1003/1006, bracketed paste 2004, focus 1004, cursor keys ...) updated by the reader; used on a gap (see 4).
- `winpty/winpty_windows.go`: ConPTY + Job Object sketch (140 lines). Compile-checked only.
- Scenario drivers in the page (`?scenario=flood|resize|reload`) report through a bound `Report` method into a JSONL file; the Go side owns window hide/show and the stopwatch.

Tools on this Mac: `claude` (~/.local/bin/claude 2.1.325; in the owner's shell it is a zsh *function* that adds `--model opus --effort max --settings ... --chrome --channels ...`, so the spike spawned the binary directly), `opencode` (/opt/homebrew/bin/opencode), `vim` (/usr/bin/vim). **`codex` is not installed** (`command -v codex` is empty), so Codex was not run.
Spike children ran with a temp `CLAUDE_CONFIG_DIR` and temp `XDG_*`; nothing in `~/.claude` or `~/.config/opencode` was edited. Two early opencode runs, before I added the XDG redirect, may have written opencode's normal data/log files. `spiketerm` has a hard timeout and I checked no process was left behind.

## Results

### 1. 50 MB flood with the window hidden: the child is not stalled (PASS)

Go test (`go test ./term -v`; `cmd/floodchild` writes 50 MB to the tty in 64 KB writes and records its own time inside `write()`); nobody ever calls `Read`:

```
child stats: bytes=52481520 writes=801 wall_ms=218 blocked_sum_ms=218 max_write_ms=1
reader: totalRead=50 MB, longest append+notify=1.73ms, longest lock wait=10us, log base=41947288 tail=53138340 buf=10 MB
```
Under heavy machine load the same test gave wall 0.7 to 3.4 s and max single write 48 to 116 ms: that is PTY throughput on a busy machine, never a hang.

In the real app (WKWebView window `Hide()`den by Go, so `visibilityState` becomes `hidden`), same flood, log cap 16 MB:

```
go-hidden-window  {"childExitedAfterHideMs":89,"childStats":"bytes=52481520 writes=801 wall_ms=285 blocked_sum_ms=284 max_write_ms=7","logBase":33555608,"logTail":53138340,"pingsEmitted":20,"readerMaxAppendUs":3685}
page-after-hidden {"visibilityTransitions":["hidden"],"pageRxWhileHidden":262144,"readCallsWhileHidden":1,"timerTicksWhileHidden":6,"pingsReceived":21}
flood-after-show  {"drainMs":549,"rx":26261040,"gaps":1,"skipped":26877300,"off":53138340,"accounted":53138340,"medianPullMs":3}
```
`accounted == total`: every byte is either rendered or counted in a gap. The hidden page pulled once (the visibility gate works) and caught up 0.55 s after show. A variant where the page keeps pulling while hidden also works (101 reads, 25 MB while hidden), so "only visible terminals pull" is a choice, not a necessity.

**Negative control** (`TestControlBlockingReaderStallsChild`): the design plan 5 rejects, a reader that hands 8 KB chunks to a bounded 64-slot queue nobody drains. After 3 s the child is still blocked in `write()` having moved 512 KB of 50 MB. The predicted stall is reproduced.

### 2. Is the reader ever waiting on the page? No (by construction and measured)

The reader's only blocking points are `pty.Read` (kernel) and `Log.mu.Lock()` for an append. A page-side `Read` takes `RLock` only for a memcpy of at most 256 KB, released before JSON, base64 or any Wails dispatch. With 4 goroutines hammering `Read(.., 256 KB)` during the 50 MB flood the reader's longest lock wait was 79 us (3 to 40 ms on the loaded re-runs). The ping is a non-blocking send; `Emit` runs on its own goroutine.
Keep out of the real code: sending data in events from the reader; calling Wails while holding the log lock; unbounded `max` (the spike caps it at 1 MB).
Cost of the mode tracker on the reader path: 1.1 GB/s on plain text, 172 MB/s on escape-dense TUI output (benchmark on the loaded machine), above PTY throughput; it allocates a small slice per mode event and can be tuned.

### 3. Resize reflows (PASS)

`?scenario=resize`: child is `sh` with `trap 'echo SIZE=$(stty size)' WINCH`, a 200-column line printed first; container width 100% to 50% to 100% to 35%:

```
xterm: 126x45 -> 62x45 -> 126x45 -> 42x45
child: SIZE=45 62 / SIZE=45 126 / SIZE=45 42
the 200-digit line occupies 5 buffer rows at 42 cols (expected ceil(200/42)=5): reflowed
```
ResizeObserver, `fit()`, `term.onResize`, bound `Resize` and `pty.Setsize` all work in WKWebView.

### 4. Webview reload re-syncs (PASS, with two required rules)

Scenario: run the app, settle, send keys, snapshot visible xterm rows and cursor, `win.Reload()` (page destroyed), the new page calls `Current()`, replays the log from offset 0, snapshot again, compare.

| App | log | result |
|---|---|---|
| vim (alt screen, 400-line file with Chinese text, `:set number`, scrolled) | full replay, 11.9 KB | 0 rows differ, cursor identical, buffer `alternate` |
| vim | trimmed (cap 32 KB, 301 KB had flowed): gap, reset, mode prologue, SIGWINCH nudge | 0 rows differ, cursor identical, buffer `alternate` |
| opencode TUI | full, 14.6 KB | 0 rows differ, cursor identical |
| claude 2.1.325 first-run screen (temp config dir) | full, 3.3 KB | 0 rows differ, cursor identical |

**Rule A, found by a control run: drop terminal-generated replies during replay.** Replayed history contains queries (DA `CSI c`, CPR `CSI 6n`, DECRQM `CSI ? Ps $p`, OSC 10/11 colour queries, focus). xterm.js answers them again and `onData` writes the answers into the *live* child as typed input. With suppression off, vim after reload had its cursor at (69,30) instead of (4,29): stray input. With a `replaying` flag from attach until `off >= tail-at-first-read`, replies are dropped and the screen is identical. Dropped replies seen: vim 5 (`ESC[O`, `ESC[2;2R`, `ESC[3;1R`, `ESC[>0;276;0c`, `ESC[?12;2$y`), opencode 18 (OSC 10/11 colours, DECRQM, CPR), claude 2 (`ESC[O`, `ESC[?1;2c`). Real keystrokes typed during the replay window are dropped too (milliseconds, up to 0.5 s).

**Rule B: after a gap the screen state is unknowable, so reset, restore modes, make the child repaint.** Trimming the front loses `CSI ? 1049 h`; without a prologue the page ended in the `normal` buffer (vim output went into scrollback, wrong mouse/cursor-key modes). Tested fix: the Go reader tracks DEC private modes; a gap reply carries a `prologue` (for example `ESC[?1049h`); the page does `term.reset()`, writes the prologue, then the data, then nudges SIGWINCH (resize cols-1, 120 ms, back). The page must rate-limit the nudge: with a cap smaller than one screen repaint (my first try used 2 KB) every repaint trims the log again and the page resyncs in a loop (20 gaps seen). A 2 s minimum between nudges stops it, and a real cap of MBs never gets there.

### 5. Other facts

- A hidden window quits a Wails app that has `ApplicationShouldTerminateAfterLastWindowClosed: true` (the spike died at `Hide()` until set to false). Tray / keep-running mode must keep it false.
- While hidden, WKWebView still runs promises and fetch (101 Wails calls went through) but `setTimeout` is throttled to about 1 per second (6 ticks of 100 ms in about 1.7 s). Do not depend on timers in the hidden case; `visibilitychange` fired (`hidden`) and is the restart signal.
- `Call.ByName("main.TermService.Read", ...)` works in `@wailsio/runtime` 3.1.0, as do page-to-Go and Go-to-page events. Base64 bound calls of 256 KB: median 3 ms; 26 MB rendered in 0.55 s.
- The 8 KB and 64-event limits cannot be hit: events are tiny and rate limited (20 pings for a 50 MB flood), bytes move only through the bound method.
- `TestKillTree`: `kill(-pgid)` takes down a shell's background grandchild on macOS.

## Decision: Windows ConPTY library

**Own thin wrapper behind `internal/pty`, started from go-pty's Windows code (MIT; add attribution to THIRD_PARTY_NOTICES). Do not import `aymanbagabas/go-pty`.**

Reasons (source read: go-pty v0.2.3, `pty_windows.go` 164 lines, `cmd_windows.go` 418 lines):
1. The plan needs "no orphan after an app crash" (A2-01 review line, plan 5 "Job Objects on Windows"). go-pty has no Job Object, and its `Cmd.start` closes the thread handle, so a child cannot be created suspended and assigned to a job without a race: a grandchild started in that gap escapes. The clean fix is `PROC_THREAD_ATTRIBUTE_JOB_LIST` in the same attribute list as the pseudo console, which go-pty does not expose. The sketch `winpty/winpty_windows.go` does it in 140 lines using only `golang.org/x/sys/windows` (`CreatePseudoConsole`, `ResizePseudoConsole`, `NewProcThreadAttributeList`, `CreateJobObject`; only the constant 0x2000D is missing from x/sys). It builds with `GOOS=windows GOARCH=amd64` (`go vet` notes the Handle-to-pointer conversion, which go-pty has too).
2. go-pty is v0.x, one maintainer, needs go 1.25, and pulls `u-root` and `x/crypto/ssh` into the module graph for SSH helpers Codepass never uses. The part we would use is the same size as our wrapper.
3. creack/pty 1.1.24 has no ConPTY, so the `internal/pty` interface has to exist anyway.
4. go-pty's `Close` calls `ClosePseudoConsole` synchronously; on older Windows 10 builds that blocks until the output pipe is drained. Our reader always drains, but the wrapper must still run it off the caller's goroutine (done in the sketch).

Unverified: nothing ran on Windows; the wrapper has never created a console (see below).

## What did not work / limits

- Codex is not installed: not exercised. Its TUI is a full-screen alt-screen app like opencode, so expect the same, unverified.
- Claude Code ran only to its first-run screen (temp config dir, no login), a normal-buffer app. The conversation UI, a logged-in session and Shift+Enter were not exercised.
- No GUI screenshots; UI checks compare xterm.js buffer text and cursor read by the page and reported to a file.
- Windows / WebView2 not tested.
- Timings were taken on a loaded machine.

## Needs manual verification

1. **Chinese IME (candidate window at the cursor).** Build: `cd spike/terminal && ./build.sh <npm-prefix with @xterm/xterm @xterm/addon-fit @xterm/addon-unicode11 @wailsio/runtime esbuild> <outdir>`, then `SPIKE_SCENARIO=interactive SPIKE_ARGV='["/opt/homebrew/bin/opencode"]' <outdir>/spiketerm` (also try `["/Users/<you>/.local/bin/claude"]` and `["/bin/zsh","-l"]`). Switch to Pinyin, type `nihao`: the candidate window must sit at the text cursor, Space or a number must commit `你好`, Backspace during composition must not delete committed text. Repeat in vim insert mode. Open risk: WKWebView with the hidden-inset title bar and page zoom.
2. **Shift+Enter in Claude Code.** In the same window run `claude` with the owner's normal login, type text, press Shift+Enter: expect a newline in the prompt, not a submit. The page sends `ESC CR` (0x1b 0x0d); that was not observed in a logged-in claude.
3. **Codex.** `brew install --cask codex` first (the spike must not install), then `SPIKE_SCENARIO=reload SPIKE_ARGV='["codex"]' SPIKE_SETTLE=5000 ...` (see `spike/terminal/reload.sh`) and compare before/after rows.
4. **Windows 11 VM (WebView2 + ConPTY).** Run the xterm page on the `winpty` wrapper: 50 MB flood with the window minimised, resize, reload; Job Object kill-tree (a `cmd /c start /b ping -t localhost` grandchild dies on Kill and when the app is killed from Task Manager); `ClosePseudoConsole` does not hang on Windows 10 1809; check which builds emit extra sequences (win32-input-mode) and set xterm's `windowsPty` option.
5. **Reload of a long, real Claude Code / opencode conversation** with a log that has actually trimmed (spinners, long history), compare by eye.

## What the plan must change

`product-plan.md`, section 5:
- "Terminal data path" row, add: "The log keeps a small DEC-private-mode table (alt screen, mouse, bracketed paste, focus, cursor keys). A read whose offset was trimmed returns `gap` and a mode `prologue`; the page resets xterm, writes the prologue, then the data, then nudges the child with a size change (cols-1, 120 ms, cols), at most once every 2 s. While replaying history (attach, reload, after a gap) the page drops everything xterm emits through `onData` / `onBinary`, because replayed queries would otherwise be answered into the live child. Log cap at least 8 full-screen repaints; default 16 MB per session."
- "Terminal instances" row: replace "Replaying a trimmed byte buffer garbles full-screen tools when it starts mid-sequence" with: "A full log replays to the identical screen; a trimmed one needs the prologue and nudge above. Reload is therefore supported; terminals stay mounted for speed."
- "PTY" row: replace "(`aymanbagabas/go-pty` [S20] or a thin wrapper ..., chosen in the spike)" with "an own wrapper over `golang.org/x/sys/windows` (CreatePseudoConsole, `PROC_THREAD_ATTRIBUTE_JOB_LIST`, Job Object with KILL_ON_JOB_CLOSE), derived from go-pty's MIT code and not imported".
- Plan 4.7: launch is the tool binary with an explicit argument list, never the owner's shell function (here `claude` is a zsh function that adds model, effort and channel flags). Note `codex` may be absent.

`tasks.md`:
- S0-01: mark macOS done; carry manual items 1 to 5 above to the milestone-0 gate list, Windows items with the VM.
- A2-01: add "Windows implementation starts from `spike/terminal/winpty/winpty_windows.go`; uses JOB_LIST so an app crash kills every descendant; Close runs off the caller's goroutine"; Tests add "kill-tree and app-crash on the Windows VM".
- A2-02: Build add "mode tracker and `prologue` in the Read reply; `Read` returns a copy and never holds the lock across a Wails call"; Accept add "reload test: full-log replay gives the same screen and cursor for vim; replay after a gap restores the alt screen"; Tests add "mode tracker with sequences split across chunks; escape-dense benchmark".
- Frontend terminal task (Deps S0-01, Group B): Build add "drop `onData` / `onBinary` while replaying; nudge rate limit; pull gate on `visibilityState`, restart on `visibilitychange` (timers are throttled to about 1 per second while hidden)"; Tests add "replies dropped during replay; Shift+Enter sends ESC CR".
- Shell/tray task: "`ApplicationShouldTerminateAfterLastWindowClosed` must be false whenever tray / keep-running is on; on macOS `Hide()` otherwise quits the app".
- A2-04 tests that need codex must skip when it is absent.
