# Spike S0-06 - Language-server transport

Date: 2026-11-16. Branch: `spike/lsp` (commit b2902d0, local only, not pushed), worktree `.claude/worktrees/wf_39cfdb02-5e6-3`, code in `spike/lsp/`.
Machine: macOS arm64 (Darwin 27.2), Go 1.27.4, Node 26.13.1, Google Chrome headless (puppeteer-core 25.17.0) as the page host.

## What was built

- `spike/lsp/main.go` (Go, stdlib only, `//go:build !windows`): spawns a language server as a child process (own process group), parses the `Content-Length` framing, and only ever **appends** each server message to a per-session log with byte offsets. A tiny SSE ping ("new data up to offset N", coalesced to one per 20 ms) tells the page; the page **pulls by offset** (`/pull?from=N&max=262144`) and acknowledges by advancing `from`. `/send` takes batched client messages and writes them to the server through a bounded queue. Extras for the review focus: a retained-bytes cap with message-aware compaction, Go-side auto-answers for server-to-client requests, a `-naive` mode that emulates a stalled event emitter, `/probe` (Go times its own request to the server), `/stress` (didOpen + many didChange rounds to make the server publish diagnostics with no page involved), `/stats` (RSS of the whole process tree via `ps`).
- `spike/lsp/web/main.js`: CodeMirror 6 + `@codemirror/lsp-client` 6.3.0 (latest on npm, modified 2026-09-17, written by the CodeMirror author). `PullTransport` implements the client's `Transport` (`send/subscribe/unsubscribe`). Own `Workspace` subclass (multi-file, `displayFile` for cross-file jumps, because the library's default workspace is not exported).
- `spike/lsp/test/run.mjs`, `hidden.mjs`, `pulldiag.mjs`: headless-Chrome drivers. HTTP on loopback stands in for the Wails event + bound method (see "Not verified").
- Installed only into temp prefixes: gopls v0.24.0 (`GOBIN=<tmp>`), `typescript-language-server@6.1.0` with `typescript@5.9.3` and with `typescript@7.0.2` (`npm --prefix <tmp>`). Nothing global was touched. `frontend/node_modules` was installed in the worktree (git-ignored) with `npm ci --ignore-scripts`.

## Results

### Hover, completion, definition (through the real lsp-client UI path, not only raw requests)

Hover = mouse move over the token and read `.cm-tooltip-hover`; completion = type `api.` / `Math.` / `ast.`, `startCompletion`, read `currentCompletions`; definition = `jumpToDefinition` command, cross-file, then check the editor now shows the other file at the right line.

| Target | Server | Hover | Completion | Definition (UI jump) |
|---|---|---|---|---|
| This repo `internal/` (138 .go files, 33.8k lines, wails deps) | gopls 0.24.0 | pass; first request 678 ms, then 11-15 ms | pass; 322 items, `ReposState` present; 102 ms UI, ~20 ms raw | pass; `reposervice.go` -> `internal/api/repos.go`, 52 ms |
| Go workspace of **2,003 .go files** (x/tools v0.52.0 1,291 + x/net v0.60.0 712 in a `go.work`, 257 packages, copied to a temp dir) | gopls 0.24.0 | pass; first request 2,533 ms, second 437 ms | pass; 111 items, `Inspect` present, 304 ms UI | pass; `inspector.go` -> `ast/edge/edge.go`, 52 ms |
| `frontend/` (184 .ts/.tsx under src, TS 7 + React 19 in node_modules) | typescript-language-server 6.1.0 + TS 5.9.3 beside it | pass; first 569 ms | pass; 44 items (labels carry a leading dot, e.g. `.abs`) | pass; `App.tsx` -> `AccountsPage.tsx`, 51 ms |
| same | **TypeScript 7.0.2 native server `tsc --lsp -stdio`** | pass; first 67 ms | pass; 44 items, `floor` present, 102 ms | pass; 51 ms |

The plan asks for ~2,000 files; the Go case above is 2,003 files. The TS repo is 184 source files (plus `node_modules` types); no 2,000-file TS repo was available offline, so TS at scale is unverified.

### Memory per server (RSS of the whole process tree from `ps`; idle 3-20 s later is flat within 1-3%)

| Case | After `initialize` | After hover+completion+definition | Idle 15-20 s |
|---|---|---|---|
| gopls, this repo (warm Go build cache) | 60 MB | 298-358 MB (3 runs) | 299-360 MB |
| gopls, this repo, **first run on a cold cache** | 250 MB | **1,307 MB** | 1,307 MB |
| gopls, 2,003-file workspace | 62 MB | 1,158 MB | 1,134 MB |
| typescript-language-server + TS 5.9 (4 node processes) | 62 MB | 927-940 MB | 927-941 MB |
| TS 7 native `tsc --lsp` | 18 MB | 290 MB | 290 MB |

gopls also forks a helper (`gopls ** telemetry **`, ~28 MB) and a short-lived `go list`. Four live servers at these sizes is 1.2 to 4.6 GB, so the plan's "cap of 4" needs real idle shutdown.

### Transport numbers

Loopback HTTP pull, warm: raw hover/completion/definition 11-35 ms round trip; the largest single pull (gopls session, 216 KB total) took 131 ms; `maxPullMs` stayed under 140 ms in every run. Ping coalescing: 17-23 pings for 20-80 messages. Ordering from the page needs a serial send queue (done in `PullTransport.flush`); parallel fetches could reorder a `didChange` and the request that follows it.

### Review focus: backpressure when the page is hidden

Setup: gopls, page marked hidden (`visibilitychange`), then 40 files opened and rewritten 21 times by Go with no page involvement (170-315 `publishDiagnostics`), `-cap 32768`. Go's `/probe` sends its own request to the server while hidden.

| Variant | Did Go keep reading? | Server responsive while hidden (probes 2-4) | Memory held in Go | On show |
|---|---|---|---|---|
| Visible (control) | yes | 0 ms | 0 | 180 msgs, 43/43 files show the final errors |
| **Hidden, pull design** (page stops pulling, Go keeps reading) | yes (log grew 32 KB -> 60 KB) | 0-4 ms | peak 32.9 KB (= cap), 78 stale messages dropped | caught up in 192 ms, 85 msgs, 43/43 files with the final errors, a request works (10 ms) |
| Hidden, real page freeze (`Page.setWebLifecycleState frozen`, after visibilitychange) | yes | 0 ms | peak 33 KB, 156 dropped | resumed in 143 ms, 43/43 files correct, request works (3 ms) |
| **Hidden, naive event emitter** (reader blocks once 64 events queue, like the Wails emitter) | **no: log offset stuck at 27,713 for 37 s** | every probe timed out at 8 s | 64 msgs | caught up 195 ms after show, but the server was stalled the whole time |

(The first probe in every run, including the visible control, times out: gopls is busy type-checking the stress. That is not backpressure.)

What this shows:
- Pull-by-offset is safe for LSP: the reader never blocks, so a hidden page cannot stall gopls or tsserver.
- LSP is **not** a pure byte stream: responses and requests must never be dropped, but `$/progress`, `window/logMessage` and superseded `publishDiagnostics` (older than the newest for the same URI) can be. So the log keeps messages with offsets and compacts under a cap; the page sees the hole as `gap: true` and the newest diagnostics win. End state was identical to the control in every run. tsgo sends one `window/logMessage` per handled request (195 dropped for 20 files), so compaction matters, and per plan rule 5 those messages must never reach the app log (the spike writes them only to a temp debug file).
- Server-to-client **requests** need an answer or the server can wait on the page. lsp-client answers every server request with MethodNotFound while visible, but a hidden page answers nothing. The spike's Go side auto-answers `client/registerCapability`, `client/unregisterCapability`, `window/workDoneProgress/create` and `workspace/configuration`; tsgo sent one `client/registerCapability` (auto-answered, count 1). gopls and tsls sent none to this client because lsp-client does not advertise those capabilities. If Codepass advertises more, the Go answer table must grow with it.
- A page that never comes back: the cap bounds memory, but nothing kills the server; a "no pull for N minutes, shut it down" rule is not built.
- A server that exits is invisible to lsp-client until the request timeout (3 s default, 30 s in the spike): the pull response carries `exited` so the page can fail fast.

### What did not work / surprises

1. **typescript-language-server cannot run against TypeScript 7.** `npm i typescript@latest` is 7.0.2 and the Homebrew `typescript` formula is also 7.0.2. TS 7 has no `tsserver.js`. With `typescript-language-server@6.1.0` next to TS 7 and a TS 7 workspace, `initialize` fails at once: "The TypeScript of the workspace (TypeScript 7.0.2 ...) provides no tsserver.js. No other valid TypeScript installation was found. Exiting." It works only when a TS 5.x/6.x sits beside it (here 5.9.3, logged as "bundled"). This repo's `frontend/package.json` pins `typescript ^7.0.2`. `brew info typescript-language-server` shows `Required: node, typescript`, so the plan's `brew install typescript-language-server` pulls TS 7 and would be broken out of the box (I did not install it, per the machine rules; this is from `brew info`, and the brew-installed server on this Mac is 5.2.0 with TS 6.0.3 globally). In 6.x the `--tsserver-path` flag no longer exists (`error: unknown option '--tsserver-path'`); the path moved to `initializationOptions.tsserver.path`.
2. The TypeScript 7 native server (`tsc --lsp -stdio`; binary lives in `@typescript/typescript-<os>-<arch>`) works with the same client, needs no Node at runtime, uses about a third of the memory (290 vs 927 MB) and answered the first hover in 67 ms vs 569 ms. Gaps: it reports diagnostics by **pull** (`textDocument/diagnostic`; tested, returned "Type 'string' is not assignable to type 'number'." in about 2.5 s including sync) and lsp-client 6.3.0 only handles push, so Codepass needs about 30 lines of its own; it logs every request; and it lower-cases some URIs on case-insensitive macOS paths (`file://~/...` for a tsconfig diagnostic), so URI-to-path mapping must compare case-insensitively on macOS and Windows.
3. `@codemirror/lsp-client` renders server Markdown with `innerHTML` (`docToHTML`: hover, signature help, `window/showMessage`). Standing checklist item 4 ("text from other people is never markup") is not met by the library as shipped: it exposes a `sanitizeHTML` config hook; the spike passes an identity function, so a real sanitizer (DOMPurify or a strict allow-list) is mandatory.
4. lsp-client's default workspace class is not exported; multi-tab or split editors need an own `Workspace` (about 60 lines, in the spike). `view.setState` for tab switching worked, but threw `RangeError: Field is not present in this state` (from a library dialog's close microtask) when a message dialog was pending: use one `EditorView` per tab, or catch.
5. The library's default request timeout is 3 s; gopls's first hover on 2,003 files took 2.5 s. Set 15-30 s.
6. gopls memory varies 4x with cache state (298 MB warm vs 1,307 MB cold on the same repo). Quote ranges, not a point value.
7. Bundle cost: lsp-client + CodeMirror view/state/autocomplete = 373 KB minified, 122 KB gzipped (pulls in `marked` and `vscode-languageserver-protocol`/`vscode-jsonrpc`); THIRD_PARTY_NOTICES must list them.
8. Windows: the harness uses `Setpgid` and `kill(-pgid)` to take down tsserver children; the Windows equivalent is a Job Object with `KILL_ON_JOB_CLOSE`, otherwise orphaned `node`/`tsserver` survive a crash. Only verified that the spike is excluded on Windows by build tag (`GOOS=windows go build ./spike/lsp` -> "build constraints exclude all Go files").

## Decisions the spike was asked to make

1. **Transport: pull-by-offset for LSP is a go**, with the changes above (message log with offsets, cap + compaction, `gap`, `exited`, Go-side auto-answers, serial batched send queue, hidden flag from the page). Verified hidden, frozen and visible, and verified that the naive event path stalls the server.
2. **TS/JS server: use the TypeScript 7 native server (`tsc --lsp -stdio`) instead of `typescript-language-server`**, because tsls does not start against current `typescript` and the brew install path is broken by it. Fallback for projects pinned to TS below 7: tsls with a TS 5/6 beside it. The plan names tsls, so the owner should confirm.
3. **Memory**: real (gopls 0.3-1.3 GB, tsls 0.9 GB, tsgo 0.3 GB); idle shutdown must exist; "cap of 4" only with idle shutdown.
4. **lsp-client is usable** for hover, completion, definition and push diagnostics, with three required additions: own Workspace, pull diagnostics for tsgo, a sanitizer. Rename, references, signature help and workspace symbols exist in the library but were not exercised.

## Needs manual verification

Windows (no Windows machine here). Steps on a clean Windows 11 VM with no Go or Node:
1. `winget install --id GoLang.Go -e`, new shell, `go install golang.org/x/tools/gopls@latest`; confirm `%USERPROFILE%\go\bin\gopls.exe` exists and `gopls version` runs. Check `%USERPROFILE%\go\bin` is on PATH for a GUI-launched process (often not until re-login); the app should resolve `go env GOPATH` itself.
2. `winget install --id OpenJS.NodeJS.LTS -e`, new shell, `npm install -g typescript`; confirm `tsc --version` is 7.x and that the native `...\@typescript\typescript-win32-x64\lib\tsc.exe --lsp -stdio` starts and waits for input. For the fallback: `npm install -g typescript-language-server typescript@5` and confirm it starts. Launch the native `.exe` directly rather than the `.cmd` shim (Go's `exec.Command` needs PATHEXT handling for `.cmd`).
3. Run the spike on Windows after replacing `syscall.Kill(-pgid)` with a Job Object; check that quitting leaves no `gopls.exe`, `tsc.exe` or `node.exe` behind, and that pipes are binary (no CRLF rewriting of the `Content-Length` framing).
4. On a proxied or locked-down machine: `go install` needs network or `GOPROXY`; check antivirus does not hold first launch of gopls for many seconds.

Not verified on this Mac either:
- the real Wails event + bound-method path (the spike uses HTTP + SSE on loopback, so latencies are optimistic; the non-blocking reader property does not depend on it; verify by reusing the `Pull(offset)` method from the S0-01 terminal spike);
- a hidden WKWebView and App Nap (headless Chrome visibility override and lifecycle freeze were used instead);
- a 2,000-file TypeScript repository;
- the brew install of `typescript-language-server` itself (not installed, per the rules);
- rename, references, signature help, workspace symbols.

## What the plan must change

`product-plan.md`
- Line ~199 (install list) and ~319: replace `brew install typescript-language-server` and "typescript-language-server" with the TypeScript 7 native server started as `tsc --lsp -stdio` (npm/brew package `typescript`; no Node needed at runtime). Windows: `winget install OpenJS.NodeJS.LTS` then `npm install -g typescript`. Keep typescript-language-server only as a fallback for projects pinned below TS 7, and then it needs `typescript` 5/6 installed beside it. Drop the "typescript-language-server 6.1.0" fact.
- Line ~319 ("Installed automatically hides two toolchains"): for TS the toolchain is Node only to run `npm -g`; once installed the server itself does not need Node (confirm the brew formula).
- Line ~361 ("Language servers" row): keep pull-by-offset but say it is a message log with offsets, not a byte log: requests and responses are never dropped; `$/progress`, `window/logMessage` and superseded diagnostics are compacted above a per-session cap (suggest 16 MB); the page reports hidden/visible; Go answers `client/registerCapability`, `client/unregisterCapability`, `window/workDoneProgress/create` and `workspace/configuration` itself; a session with no pull for 10 minutes is shut down.
- Line ~377 (rule 5): add that language servers log one message per request and these are dropped, never logged.
- New engineering rule: lsp-client renders Markdown through `innerHTML`; `sanitizeHTML` must be a real sanitizer, with a test that an `<img onerror>` in a hover is neutralised (standing checklist item 4).
- Line ~435 (risk "Language servers eat memory"): add the measured numbers and change "a cap of 4" to a default of 2 live servers (4 only on large-RAM machines), with idle shutdown.
- Milestone 5 scope (line ~413): add "pull diagnostics for the TS server (lsp-client has none)" and "own multi-file Workspace".
- Line ~462: THIRD_PARTY_NOTICES add `marked`, `vscode-languageserver-protocol`, `vscode-jsonrpc`.

`tasks.md`
- S0-06: record "done on macOS; tsls does not start against TS 7: use `tsc --lsp -stdio`"; the "2,000-file Go repository" was met by a 2,003-file x/tools + x/net workspace; "Windows install path confirmed" stays **open, needs manual verification** (steps above).
- Milestone 5 language-server tasks: replace tsls with the native server; add Build items (Go-side auto-answer table, message-aware log with cap, idle shutdown, `exited` signal, serial send queue) and Accept ("a hidden page does not stall the server, tested with a fake server that floods output"; hover, completion, definition and diagnostics for Go and TS); Review focus "sanitizeHTML set; no server text in logs".
- Owner decisions: the deferral list in "Decisions that were asked" item 4 names "the TypeScript language server"; decision 4 (everything ships) makes it moot, but the choice of server itself changed and the owner should confirm.
- Windows task: Job Object for child servers (macOS uses a process group); add "no orphan gopls/tsc/node after quit" to the Windows smoke test.
- Wiring task dependency note: `@codemirror/lsp-client@6.3.0`, `@codemirror/autocomplete`, `@codemirror/lang-go`, `@codemirror/lang-javascript`, and a sanitizer.
