# Milestone 0 spike review A: S0-01, S0-02, S0-03

Date: 2026-11-16. Reviewer: Opus (review A). Read-only review; this file is the only thing written.
Inputs: `docs/research/spike-terminal.md`, `spike-update.md`, `spike-signals.md`; branches `spike/terminal` (e7e6dc7), `spike/update` (521bd9e), `spike/signals` (9648847); the raw outputs the spikes left in this session's scratchpad (`out/rep-*.jsonl`, `out/run.log`, `s0-02/full.log`, `s0-02/req.sh`, `cc/t1..t4.jsonl`, `oc/e1..e3.jsonl`); Wails v3.1.0 `pkg/updater` source; the OpenCode log.

## What I checked myself

| Check | Result |
|---|---|
| Re-ran `go test ./term` on `spike/terminal` (flood with no reader, 4 concurrent readers, blocking-queue control, kill tree, log offsets, mode tracker) | All pass. Child wall 209 ms, longest write 1 ms; reader lock wait 105 us with 4 readers; the control stalls at 512 KB. Matches the note. |
| `GOOS=windows GOARCH=amd64 go build` and `go vet` of `spike/terminal/winpty` | Builds. Vet reports `possible misuse of unsafe.Pointer` at line 68, as the note says. |
| Re-ran `go test ./internal/update` on `spike/update`; Windows build and vet of `internal/update` | All pass, including the AST test and the logged `rc10 < rc2` hazard. |
| Raw scenario output against the notes | `s0-02/full.log` ends `RESULT: 118 passed, 0 failed`. Terminal and signals logs match the numbers quoted, with the exceptions below. |
| `git ls-remote --heads origin` | Only `codepass` and `main`. Nothing was pushed. |
| `~/.claude/settings.json`, `~/.config/opencode/opencode.json` against the spike's `user-before.sha` | Byte-identical. |
| Process groups under the plan's launch form (`$SHELL -i -c '"$@"' codepass <tool>`), small PTY experiment | See X1: a tool started through a shell function gets its own process group. |
| Leftover processes and files | A detached `opencode serve --service` started by this session is still running; see S0-03. |

## Verdicts

| Spike | Verdict | In one line |
|---|---|---|
| S0-01 Terminal | **Partly verified** | macOS data path proven; reload proven for full logs; reload after trimming proven for vim only; IME, Shift+Enter, Codex, WebView2 and ConPTY not run. |
| S0-02 Update | **Solid** (macOS trust logic) | Every refusal asserted byte-for-byte on a real signed `.app` against a fake API. Windows gate not written, no VM, no notarized build, no real GitHub. |
| S0-03 Signals | **Partly verified** | Claude Code hook mechanics and resume verified in `-p` mode; OpenCode only up to a 401; Codex from docs only; "nothing written" needs three caveats. |

---

## S0-01 Terminal in the webview

### Claims against evidence

Backed:
- **The reader never waits on the page.** By construction (its only blocking points are `pty.Read`, a short append lock and a non-blocking ping send), by the unit tests, and by my re-run. The negative control reproduces the stall the plan predicts.
- **Resize.** `rep-resize.jsonl`: xterm 126, 62, 126, 42 columns; the child prints `SIZE=45 62/126/42`; the 200-character line wraps to 5 rows at 42 columns.
- **Reload with a full log.** vim (alt screen), opencode TUI and the claude first-run screen: 0 rows differ, same cursor. Rule A (drop replies during replay) has a control: in `rep-vim-nosup.jsonl` the cursor moves from (4,29) to (69,30) and vim writes 139 extra bytes in reaction to the stray input.
- Hidden WKWebView still runs fetch and promises (101 bound calls in the pull-while-hidden run); timers are throttled.

Overstated or not run:
1. **"50 MB with the window hidden" is mostly a visible-window run in the app.** The page spawns the flood, pulls for 200 ms, then asks Go to hide. The child ran 285 ms and exited 89 ms after the hide, so roughly two thirds of the bytes went out while the window was visible (`run.log` shows 28 `Read` calls before `GO hide requested`). The conclusion still holds: the unit test with no reader at all is the stronger proof, and the page fell about 30 MB behind even while visible (estimate from the reported totals). But nothing measured a long hide; App Nap engages after minutes, not 89 ms.
2. **Reload after trimming is verified for vim only.** The only opencode run with a trimmed log (`rep-oc-gap.jsonl`, 2 KB cap) shows an escape fragment printed as text on row 0 (`28;128;128m`) and the normal buffer instead of the alt screen, before and after the reload; the "0 rows differ" metric passed because both screens were equally wrong. The follow-up `rep-oc-gap2.jsonl` never reached the TUI (551 bytes: "Starting background server..." and echoed keys), so the 2-second nudge limit was never shown to fix opencode. `rep-cl-gap.jsonl` never trimmed (log base 0). The note's table is accurate, but "a 2 s minimum ... stops it" and the proposed plan text "Reload is therefore supported" go beyond the evidence.
3. **Kill tree.** `TestKillTree` uses `/bin/sh -c`, which has no job control. The plan's launch form behaves differently (X1).
4. Not run, and the note says so: Chinese IME, Shift+Enter in a logged-in claude, Codex, WebView2, the ConPTY wrapper, a long real session.

Code notes for A2-02 and A2-07 (spike code, not blockers):
- `Log.Append` trims at an arbitrary byte, so a replay can start inside an escape sequence or a UTF-8 character. That is the opencode garbage above. Trim only where the reader's escape parser is in ground state and at a UTF-8 boundary; prefer the latest full clear or alt-screen entry before the cut.
- `Prologue()` returns the mode state at the tail, but the page replays from the base. It should be the state at the new base (fold mode events up to the cut). It only emits `h`, so a hidden cursor (`?25l`, on by default) is not restored. Also track 47/1047 and keypad / keyboard-protocol modes.
- The log holds between cap and 2x cap: a 16 MB cap means up to 32 MB per session and a 16 MB memmove per trim. A ring of fixed chunks keeps memory near the cap.
- The page drops replies in `onData` only; `onBinary` is not gated. The note's plan text (both) is right; the spike code is not.
- The pull gate is `document.visibilityState`. With several mounted terminals in a visible window, every terminal pulls. "Only visible terminals pull" needs a per-terminal gate (active tab or IntersectionObserver).

### Decision

Windows ConPTY: **own thin wrapper over `golang.org/x/sys/windows`, derived from go-pty's MIT code, with `PROC_THREAD_ATTRIBUTE_JOB_LIST` and `KILL_ON_JOB_CLOSE`; go-pty is not imported.** I checked the reasons against go-pty v0.2.3: no Job Object, the thread handle is closed at `cmd_windows.go:158` (so no suspended start), and `go.mod` requires u-root, x/crypto and go 1.25. I agree. The wrapper has never run.

### Rules

Nothing pushed. Claude ran with a temp `CLAUDE_CONFIG_DIR`. `reload.sh` hard-codes the scratchpad path, which contains the local user name: fine on a local branch, must not be merged. No `spiketerm` or `floodchild` process is left.

### Plan changes

Endorse:
- product-plan 5, "Terminal data path": mode table; gap reply with a prologue; reset, prologue, data, then a size nudge (cols-1, 120 ms, back) at most every 2 s; drop `onData` and `onBinary` while replaying. Plus the code notes above (safe trim points, prologue as of the base, both set and reset, chunked ring).
- product-plan 5, "PTY": own ConPTY wrapper with JOB_LIST, marked "never run; first Windows VM task".
- tasks: S0-01 macOS part done and manual items moved to the milestone-0 gate list; A2-01 starts from `winpty_windows.go`, runs `ClosePseudoConsole` off the caller's goroutine, and adds Windows VM kill-tree and app-crash tests; A2-02 mode tracker, copy-out reads, reload tests; A2-07 reply suppression, nudge rate limit, restart on `visibilitychange`, Shift+Enter test; A2-04 tests skip Codex when it is absent.
- F1-03 (window behaviour): `ApplicationShouldTerminateAfterLastWindowClosed` must be false whenever closing hides the window. Observed by the spike (the app quit at `Hide()`); current `main.go:70` sets it to true. I did not re-run this.

Do not endorse as written:
- "Terminal instances" row changed to "Reload is therefore supported". Keep "kept mounted" as the rule. Write instead: full-log replay is verified; trimmed replay is verified for vim only and failed once for opencode; it is the recovery path, not the normal one.
- Plan 4.7 changed to "never the owner's shell function". This reverses plan 4.7 and A2-04's accept line ("a user-defined `claude` shell function still applies"), and S0-03 found a way to keep the function. Owner decision (X2).

Add:
- A2-02 Accept: an in-app flood where the child starts after the hide, and a hide of at least 10 minutes with a slow steady producer (App Nap). Measure bytes per hour from a working Claude Code and opencode session and size the cap so a normal day never trims, or let mounted-but-hidden terminals keep pulling at a low rate (the spike showed that pulling while hidden works).

---

## S0-02 Signed-manifest update

### Claims against evidence

All headline claims are backed by `full.log` (17 scenarios, 118 PASS), my re-run of the unit tests and Windows build, and the Wails source:
- `validateInstalled` runs after the swap, and `verifySignature` enforces a verdict only if the original also passes, so an ad-hoc bundle replaces a Developer ID one (controls C1, C2). Confirmed in `pkg/updater/validate.go`.
- `periodicCheckLoop` skips `StateReady` and there is no public discard. Confirmed.
- For a bundle, `replaceTarget` is `RemoveAll` then rename (`helper_unix.go:55-67`). Confirmed.
- The GitHub provider's default client has `Timeout: 30s`. Confirmed.
- "The plan's requirement string accepts an Apple Development certificate of the same team": the test script (`s0-02/req.sh`) is in the scratchpad but its output was not kept. The claim fits how Apple certificates carry the Team ID in the leaf OU, and the tightened string is the standard Developer ID form.

Items read from source are labelled as such in the note. Nothing is claimed for Windows.

### Gaps the spike did not cover

1. **The test channel is a private repository** (plan 4.14 item 8). The Wails GitHub provider needs `Config.Token` for a private repository; the spike used an unauthenticated fake. Decide where that token lives: a fine-grained read-only token for that one repository, entered once and kept in the Keychain, never compiled in. The production channel (public repository) needs none.
2. The gate runs `codesign`, `spctl` and `plutil` through PATH. Use `/usr/bin/codesign`, `/usr/sbin/spctl`, `/usr/bin/plutil`.
3. `Highest.Get` returns "" for an unreadable or invalid file, which silently switches the rule off. Fail closed with a visible message, or document it as a local-trust limit.
4. `inAPI` accepts asset URLs only under `<base>/repos/<configured repo>/`. After a rename the API returns the new name, so a build configured with the old name stops updating (it fails closed). Configure the final name.
5. Wails helper logs in `$TMPDIR` are never deleted. Remove old ones at start.
6. The swap's crash window has no automatic recovery. Document the manual one (`Codepass.app.bak` back to `Codepass.app`).
7. **Windows has no update path yet**: the gate refuses everything. On Windows the trust rests on the ed25519 manifest and its sha256, which is platform-independent code and is what R3 and R4 exercised. Whether Authenticode is also checked depends on decision 3 (X4).

### Decisions

Wrap the Wails GitHub provider for discovery and streaming only; keep two pinned keys in the own provider, leave `Config.PublicKey` empty and pass the manifest sha256 to Wails as the digest; run the bundle gate on the staged copy before `Restart` and again inside `Restart`; a fresh `Updater` per check with an own timer; `HandleHelperMode` as the first statement of `main` (done, AST-tested; the repository's only `init()` is in `internal/app/events.go` and only registers event types). I agree with all five.

### Rules

Nothing pushed. No secret in the branch: the ed25519 keys are throwaway files in the scratchpad, and `DEVID_HASH` comes from the environment. `run.sh` contains the Team ID and the bundle ID; both are public, and the product's requirement string needs the Team ID anyway, so the F1-07 scrub list must allow it. Test bundles were signed locally with the owner's Developer ID identity, without a timestamp. The signed copies, one of them with bundle ID `com.evil.app`, are still in the scratchpad (`s0-02/apps`, `zips`, `work`, `x`) and should be deleted. When `run.sh` becomes the integration test, use a neutral ID such as `com.example.other`.

### Plan changes

Endorse every item of the note's "What the plan must change": 4.14 items 1-4 and 7, the new "Failure behaviour" item, the UI paragraph, the risk row; F1-09, F1-10, F1-W, S0-05; `golang.org/x/mod` as a direct dependency; the Windows gate task.
Add: gaps 1-6 above to F1-09 and F1-10, and make the Windows gate plus one Windows 11 rc1-to-rc2 run part of milestone 1's gate, not milestone 7.

---

## S0-03 AI tool signals

### Claims against evidence

Backed by raw logs and the committed sample:
- Six Claude Code events arrive with `session_id`; `permission_mode` is in UserPromptSubmit, PreToolUse and Stop; `CODEPASS_TASK` reaches the hook process.
- Two `--settings` flags: only the last file's hooks fire (t2: A last, only A fired; t3: B last, only B fired).
- `--resume` keeps the session ID and fires `SessionStart` with `source: resume`.
- The OpenCode plugin given only through `OPENCODE_CONFIG_CONTENT` loads under `--standalone`; `session.execution.started` and `.failed` carry `sessionID`; `--session ses_codepass...` creates a session with exactly that ID.
- The process-tree probe works on a plain zsh.

Narrower than stated:
- The plugin route was observed for `SessionStart` only. t4 ran `-p "/context"`, which makes no model turn, so the plugin's `Stop` hook could not fire. The plugin also needs `.claude-plugin/plugin.json` (t4.sh writes one); the note mentions only `hooks/hooks.json`.
- "`--standalone` is required" is reasoned, not tested with a negative run. The OpenCode log supports it: the TUI runs at 01:09 attached to the existing shared service as clients.
- OpenCode success and permission events, everything about Codex, the interactive TUIs, and `Notification` / `PermissionRequest` were not observed (the note says so).
- The `SessionStart` payload also carries `seconds_since_last_response`, `context_tokens`, `prompt_cache_likely_expired` and `estimated_cache_write_usd` (committed sample); the table leaves them out. Harmless.

### "Nothing was written into the user's own tool settings"

The two settings files are byte-identical (re-checked). The review focus passes, because none of the following is a settings file, but the claim needs three caveats the note does not make:
1. `opencode auth list`, run from this session at 01:04:06, spawned OpenCode's shared service (`opencode serve --service`). It is detached (parent is launchd) and still running (pid 92434 at 01:28; its environment carries this session's ID). `~/.config/opencode/service.json` (the service's local password, mode 0600) was last written at 01:04, the minute the service started. The terminal spike's opencode TUIs later attached to this service.
2. Three transcripts (about 700 KB) were written to `~/.claude/projects/-private-tmp-...-scratchpad-cc-work/`, Claude Code's normal behaviour without a temp `CLAUDE_CONFIG_DIR`. The note says "`~/.claude` never edited".
3. OpenCode's own database and log changed (sessions created, then deleted), as the note discloses.

### What the leftover service means for the product

A non-standalone OpenCode command starts a shared service outside the task's PTY session and process tree. It outlives the task and runs commands with the environment of whoever started it first. Without `--standalone`, therefore: stopping a task cannot kill what OpenCode runs; per-task environment (`CODEPASS_TASK`, proxy variables, the AWS-profile convenience) does not reach the service; and the process-tree fallback sees nothing. `--standalone` is needed for process control, not only for the plugin. Tool detection must use `opencode --version`, which did not start the service (`auth list` did; the service started 42 ms after it in the log).

### Decisions

Claude Code hooks as a session-only plugin (`--plugin-dir`), `--settings` as fallback; session IDs created by the app (Claude `--session-id`, untested; OpenCode `--session ses_...`, verified); OpenCode through `OPENCODE_CONFIG_CONTENT` plus `--standalone`; Codex hooks if per-launch supply works, otherwise `notify`; the fallback can only tell "alive" from "exited". I agree.

### Plan changes

Endorse the note's section 7: hooks through the plugin; forward only task ID, event, session ID and mode; app-generated session IDs; the fallback wording; OpenCode 2.x event names; the [S26] and [S12] updates; the vocabulary table; the byte-identical file list; A2-05, the A2-04 forbidden-flag list, A2-08.

Add:
- Byte-identical exclusions: also `~/.config/opencode/service.json`.
- A2-04 / A2-05: OpenCode always runs with `--standalone`. Accept: after an OpenCode task stops, no `opencode` process is left and `service.json` is unchanged.
- A2-05: subscribe only to the events the state needs (SessionStart, UserPromptSubmit, Notification, PermissionRequest, Stop, SessionEnd), not PreToolUse / PostToolUse. Command hooks run synchronously, so each of those would add a process start to every tool call. Set a short hook `timeout`; the report mode exits at once and never waits for the app.
- Every injection must add to the user's own setup, tested per tool. Codex `-c notify=[...]` replaces a user's own `notify`. The OpenCode log suggests `OPENCODE_CONFIG_CONTENT` merges (the user's AGENTS.md, skills and provider still loaded), but a user `plugins` list was not tested (the owner has none). Also test two `--plugin-dir` flags.
- Fallback: with the plan's `-c` launch the shell exits together with the tool, so "back at the shell" never happens for an AI task. Without hooks the state is "running (no signal)" until exit, and keep-awake relies on the hard ceiling. Say so in plan 4.8 and in the UI.
- The owner's Claude Code default mode is `bypassPermissions` (it is in every payload). Permission prompts, and the "waiting" state they drive, will be rare for the owner; the task header will show that mode, which is correct.

---

## Cross-cutting

**X1. Killing a task (A2-01, plan 4.8).** I ran the plan's launch form in a PTY. With an external tool, zsh execs it and a group kill works. With a shell function (the owner's `claude` is one), the tool runs as a job in its own process group (leader 76486; tool pid and pgid 76487), so `kill(-leader)` misses the tool and everything it starts. `zsh -i` and `bash -i` give every job its own group. Change "setsid plus a group kill" to "kill every process in the PTY's session": enumerate processes and compare `getsid()` (macOS `ps -o sess` prints 0), and kill each group they lead; document that a process that calls `setsid` itself escapes on macOS. A2-01 tests: the real launch form with a shell function, and a grandchild started under `set -m`.

**X2. The two spikes disagree about the owner's `claude` function.** S0-01 says launch the binary directly; S0-03 keeps the plan's login-shell launch and makes it safe with the plugin route. Default: keep plan 4.7. The owner decides whether app-launched tasks should get the function's `--model opus --effort max --chrome --channels ...` flags.

**X3. Nothing ran on Windows in any of the three spikes.** A Windows 11 on Arm VM on this Mac runs amd64 builds under emulation and is enough for WebView2, ConPTY and Job Object basics. Windows 10 1809 x64 needs an Intel PC or a cloud instance (Windows Server 2019 has the same build, 17763). Both are needed before milestone 1's gate (updater) and A2-01.

**X4. Decision 3.** The owner's answer relayed to this workflow, "1 a, 2 a, 3 a, 4 b", matches the FINAL block in `tasks.md` for 1, 2 and 4. In `tasks.md`'s own lettering "3 a" is "buy a code-signing certificate", while the FINAL block records "ship unsigned". Please confirm; it decides whether the Windows update gate checks Authenticode.

**X5. Clean-up for the owner** (I changed nothing): stop the leftover `opencode serve --service` if you did not start one; delete the Developer ID-signed test bundles under the scratchpad's `s0-02/`; optionally delete the three spike transcripts under `~/.claude/projects/...scratchpad-cc-work/`.

## Manual checks for the owner (consolidated)

S0-01, macOS
1. Chinese Pinyin IME in claude, opencode and vim: candidate window at the cursor, Space or a number commits, Backspace during composition does not delete committed text. Highest risk for this owner.
2. Shift+Enter in a logged-in claude gives a new line, not a submit.
3. Codex reload test after `brew install --cask codex`.
4. Reload of a long real claude and opencode session whose log has trimmed; compare by eye.

S0-01, Windows
5. WebView2 with the ConPTY wrapper: flood while minimised, resize, reload; Job Object kill-tree, including the app killed from Task Manager; `ClosePseudoConsole` on build 17763.

S0-02
6. Clean macOS 15 VM, rc1 to rc2 with notarized builds: Safari download (quarantine), `/Applications` owned by an admin, `~/Applications` as a standard user; requirement string and `spctl` on the extracted notarized copy.
7. Same VM: rc1 re-published as rc3, and an ad-hoc build; both refused, rc2 keeps running.
8. `kill -9` the helper during the swap; recover from `.bak`.
9. Private test repository with a token: authentication, the CDN redirect, the header stripped on the redirect.
10. Download throttled to about 1 MB/s with the production HTTP client.
11. Windows 11 rc1 to rc2 once the Windows gate exists.

S0-03
12. Interactive claude through the owner's function plus `--plugin-dir` and `--settings`: no trust prompt; `Notification` and `PermissionRequest` payloads with `--permission-mode manual`; whether `{"ultracode":true}` is dropped.
13. `--session-id <uuid>`, then `--resume` from the same folder and from another one.
14. OpenCode with working sign-in under `--standalone`, TUI included: succeeded, idle and permission events; no `serve --service` started; `service.json` unchanged.
15. Codex in a temp prefix: `-c notify=...`, inline hooks through `-c`, the trust prompt, `codex resume`.
16. All three tools on Windows, including how `codepass.exe report` is quoted in a hook command.
