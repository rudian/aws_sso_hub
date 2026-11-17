# Codepass re-platform to Rust and gpui: design

Date: 2026-11-17. Author: planner (Opus). Status: draft for the owner's approval; the owner's answers of 2026-11-17 are in section 13. Files in this folder: this design, `ui-native.md` (Fable's UI map), `crate-facts.md` (crate versions and licences checked on 2026-11-17).
Reads with: `docs/plan/codepass/product-plan.md` ("plan N.N"), `ui-design.md` ("design N.N"), `tasks.md`, `docs/research/rust-gui-spike.md` ("spike"), `docs/release.md`, `reviews/m2B-wiring.md` and `reviews/m2C-wiring.md` ("m2B", "m2C"), and Fable's UI mapping `ui-native.md` in this folder ("UI map N"). Crate facts were checked on crates.io, docs.rs and GitHub on 2026-11-17; the raw list is `crate-facts.md` in this folder.

A **planner's call** is a choice the owner may change. Everything else follows from the owner's decision of 2026-11-17 or from the plan. Tuning numbers (worker counts, chunk sizes, channel capacities, thresholds, budgets) are planner's calls unless a plan section is cited next to them.

## 0. Summary

- Same product, new engine. One Rust binary; the UI is gpui through GPUI Kit; no webview. Every requirement, screen, file format and safety rule stays.
- About 24 crates in one Cargo workspace at the repository root. Service crates never depend on gpui, so their tests build and run in seconds.
- The UI thread never waits. Services run on one tokio runtime and on dedicated threads and publish state; views read state at most once per frame.
- The Go code (51,577 lines, 1,334 Go test functions), the frontend tests (1,005) and the reviewed edge cases are the executable specification. A spec map ties every Go test to a Rust test or to a written reason for dropping it.
- New milestones: **R0** foundation (18 points), **R1** port of the AWS SSO Hub core and milestone 1 (57), **R2** port of milestone 2's Go work, then the rest of milestone 2 (40), then **3, 8, 4, 5, 6, 7** as planned (162). Total about **277 points, about 48 plan-weeks**. The re-platform itself costs about 101 points (about 14 weeks).
- Owner decisions of 2026-11-17 (section 13): Windows builds and tests run on GitHub Actions; Chinese input and VoiceOver passed the owner's own test on the spike; Apple's Metal toolchain is settled in R0-09.

## 1. Intent

**Why.** The owner, 2026-11-17: performance must be the best in UI and in logic; the UI refined and the experience friendly; features and requirements do not change. The webview build spent its effort on transport workarounds (the pull-by-offset terminal and LSP paths exist because Wails stalls an emitter at 64 queued events and parks payloads over 8 KB, plan 5) and carries a browser engine's memory. The two heaviest parts of the app, the code editor and the terminal, are exactly where gpui is strongest (spike: 141 MB/s terminal, 10,000-row list at 1.5 ms CPU per frame, idle CPU 0.01%).

**Goals.**
1. Every requirement of plan 2 to 4 and design 4 to 6, and every edge case recorded in `tasks.md` follow-ups, m2B and m2C.
2. Best performance: frames at the display's rate (60 to 120 Hz), near-zero idle cost, less memory than the webview build. Budgets in section 9.
3. A native, refined UI that matches the design (UI map 1 to 8).
4. One language for program and UI: no bindings generator, no TypeScript mocks typed against bindings, no webview transport.
5. The same shipping safety: signed manifest updates, notarization, test channel, scrub check, test guards.

**Non-goals.** New features; Linux; a redesign (Fable maps the existing design; UI map 7 lists the few visual deltas, which need the owner's OK); a plugin system; telemetry.

**Anti-goals (must never happen).**
- A requirement changes because "the toolkit cannot". A gap is built, or taken to the owner.
- GPL, LGPL or AGPL code in the binary or the repository. Zed's `gpui` is Apache-2.0, but most other Zed crates (editor, terminal_view, workspace, language, project) are GPL-3.0: never copy them, never use them as a template.
- A webview anywhere: not for Markdown, not for sign-in (sign-in stays in the system browser), not for help pages.
- A data-format break with AWS SSO Hub 0.0.1 and 0.0.2 (plan 3.1, 4.2): `settings.json`, the data folder `AWS SSO Hub`, `~/.aws` files, the SSO token cache, git-config markers, keychain item names.
- A regression against the Go behaviour or a reviewed edge case.
- A test or tool that touches real AWS, the owner's shell, the owner's home config or the installed old app.

## 2. Constraints

- **Platforms.** macOS 15+ on Apple silicon (arm64 only); Windows 10 1809+ and 11, amd64 (plan 1.2).
- **Toolchain.** Rust **1.100.0** (the latest stable, released 2026-11-12; 1.101 is due about 2026-12-24), pinned in `rust-toolchain.toml` (gpui-pre needs it: 1.97 fails, spike; the AWS SDK needs at least 1.94.1); edition 2024; components rustfmt and clippy; targets `aarch64-apple-darwin`, `x86_64-pc-windows-msvc`.
- **Licence.** The repository is public under Apache-2.0. Dependencies: MIT, Apache-2.0, BSD-2/3, ISC, Zlib, 0BSD, CC0-1.0 (`notify`), Unicode-3.0, Unlicense, BSL-1.0, and MIT-0 or CDLA-Permissive-2.0 only for the crypto and root-certificate crates that carry them. MPL-2.0 only unmodified (today: `option-ext`, spike). Never `user-notify` or `ini-merge` (LGPL), never `apple-codesign` (MPL-2.0 and unmaintained; Apple's own tools sign). No GPL, LGPL, AGPL, SSPL or unknown. `cargo deny` enforces it (section 7.3).
- **Identity that never changes.** BundleID `com.rudian.awsssohub`; data folder `AWS SSO Hub` (`~/Library/Application Support/AWS SSO Hub`, `%AppData%\AWS SSO Hub`); git markers `awsssohub.profile`, `awsssohub.account`; helper, fallback and guard strings (`gitx/helper.go`); keychain service "Codepass update test channel"; the update public keys; the manifest signature domain `codepass-update-manifest-v1\n` (plan 1.1, 4.14).
- **Test safety** (HANDOFF 7): no real AWS (fake endpoints on loopback only); no real shell (login or interactive shells only with a temp HOME and ZDOTDIR); no real home config (`~/.aws`, `~/.gitconfig`, `~/.ssh`, AI tool folders); development runs only with `AWSSSOHUB_SANDBOX=<new temp dir>`; fake data only (acme, globex, initech, umbrella, alex-dev, `111122223333`, `example.com`, `d-1234567890`); no real notifications or keychain from development builds; signed test bundles deleted after use.
- **The installed AWS SSO Hub is never run, quit, killed, moved or deleted** by an agent, a test or a script. Only the shipped app asks the owner (plan 4.2).
- **Releases** are built and signed on the owner's Mac; no secret in CI (`docs/release.md`).

## 3. Approaches considered

| Option | Verdict | Why |
|---|---|---|
| **gpui + GPUI Kit** (Rust) | **Chosen by the owner, 2026-11-17** | GPU text and lists; a code editor, virtual lists, diff and conflict views ready-made in GPUI Kit; `alacritty_terminal` for the terminal; AccessKit and IME code present; screenshots without screen-recording permission (spike a). Risks: pre-1.0, weekly breaking releases, one maintainer (section 8). |
| Slint (Rust) | Dropped | Mature and stable, but no code editor and no terminal: both from scratch, the two largest parts. |
| Tauri (Rust + webview) | Dropped | A webview again: the same transport limits and browser memory the owner wants gone. |
| egui | Dropped | Immediate mode redraws; text layout, IME and a refined look for a large app are weak points. |
| iced | Dropped | No editor or terminal at the needed level; smaller ecosystem for this kind of app. |
| Stay on Go + Wails 3 | Dropped by the owner | Webview memory; the 64-event and 8 KB limits shaped the terminal and LSP designs. |

## 4. Architecture

### 4.1 Repository layout

```
Cargo.toml            workspace members, [workspace.dependencies] (exact pins), profiles
Cargo.lock            committed; every build uses --locked
rust-toolchain.toml   channel = "1.100.0"
.cargo/config.toml    alias xtask; test env (guards); per-target linker settings
deny.toml about.toml clippy.toml
crates/<name>/        one folder per crate (4.2)
xtask/                cargo xtask: gate, bundle, relkit, shots, shotdiff, perf, spec-map, notices
assets/               app icon (from docs/design/codepass-mockup/appicon.svg), Info.plist template, lproj strings, Windows resources
third_party/          only upstream crates we had to patch, with PATCHES.md (empty at start)
```

During the port, `main.go`, `internal/`, `frontend/` and the Wails build files stay where they are, frozen and read-only. They are deleted in one commit (11.6).

### 4.2 Crates

Planner's call: about 24 crates at the end of R2, about 28 at 1.0. Why: (1) crates without gpui compile and test in seconds, which keeps the implement-review loop fast; (2) a parallel task owns whole crates, so merges rarely conflict; (3) an incremental rebuild recompiles only the touched crate and its dependents; (4) façade crates (`cp-ui-kit`, `cp-platform`) hold the churn of GPUI Kit and OS APIs in one place.

| Crate | Holds | gpui |
|---|---|---|
| `codepass` (bin) | `main.rs`: the start order (4.8), update helper mode, report mode | links all |
| `cp-core` | paths and sandbox layout, `AppError` and codes, redaction, logging, ids, clock trait, the guard switch (7.4) | no |
| `cp-i18n` | zh-CN and en catalogue, generated key enum, formatter | no |
| `cp-store` | versioned JSON files, `settings.json` model, small app files (`notifications.json`, `update/*`) | no |
| `cp-files` | line-preserving INI editor, backups, atomic writer, text diff for previews | no |
| `cp-exec` | child processes (argv only), environment building, GUI PATH from the login shell, developer-tool stub rule, shell guard, terminal-app launcher | no |
| `cp-net` | system proxy, the HTTP client, ETag cache, proxy variables for children, the AWS guard, the tokio runtime | no |
| `cp-platform` | keychain, process table, keep-awake holders, notification backend, single instance and IPC, https-only opener, reveal, Time Machine exclusion, browser launch, the menu-bar and tray item | no |
| `cp-aws` | `~/.aws` model, SSO token cache, sign-in (PKCE, device code), portal, STS, IAM, console link, CodeCommit | no |
| `cp-git` | git runners and parsers, binding, global guard, cached CodeCommit passwords; later every git operation | no |
| `cp-repos` | repository engine, `projects.json` and registry, author identity, background fetch | no |
| `cp-accounts` | accounts engine, sessions and reminders, migration wizard, export and import, old-app guard | no |
| `cp-tools` | tool detection, Homebrew and winget installers, AI tool detect, install and launch | no |
| `cp-update` | manifest, keys, version rules, gate, download, staging, swap | no |
| `cp-notify` | notification service, kinds, routing, fake | no |
| `cp-pty` | PTY on Unix and ConPTY, kill by terminal session, Job Objects | no |
| `cp-term` | terminal sessions: emulator, reader and writer threads, scrollback file | no |
| `cp-tasks` | task-state signals (hooks, socket, tracker, fallback), process tracker, keep-awake manager, task lifecycle; later the queue | no |
| `cp-app` | service traits, the real `Services` container, loops, event bus | no |
| `cp-mock` | fake services and the generic world for mock mode | no |
| `cp-ui-kit` | tokens, theme, text styles, components over GPUI Kit, `announce`, `safe_markdown`, icons (UI map 1 to 3) | yes |
| `cp-term-view` | terminal element: paint, keys, mouse, IME, selection, links, search, screen-reader text | yes |
| `cp-ui` | window, shell, pages, workspace, dialogs, models, the shots table | yes |
| `cp-testkit` (dev only) | fake OIDC, portal, STS, IAM, CodeCommit and federation servers; git repository builders; temp HOME and trap shells | no |
| later: `cp-lsp`, `cp-editor` (M5), `cp-providers` (M6), `cp-ai` (M8) | language servers; editor glue; GitHub, Bitbucket, SSH keys; AI accounts, usage, tickets, conversation carry | `cp-editor` only |

Planner's call: `cp-ui` starts as one crate and is split (shell, pages, workspace) when an incremental debug build after a one-line UI change takes more than 15 s (`cargo xtask perf --build`).

### 4.3 Build profiles and compile time

- gpui and GPUI Kit are dependencies of `cp-ui-kit`, `cp-term-view`, `cp-ui`, `codepass` (later `cp-editor`) only.
- `dev`: workspace crates at opt-level 0, dependencies at opt-level 1 (a debug UI stays usable), `debug = "line-tables-only"`, `split-debuginfo = "unpacked"` on macOS. Planner's call; R0-01 measures clean and incremental times.
- `release` (local runs, perf): thin LTO, codegen-units 16 (spike: 3 min 47 s clean).
- `dist` (shipping): fat LTO, codegen-units 1, symbols stripped; the dSYM stays in the release output folder (never uploaded); `panic = "unwind"`. Features `mock`, `shots` and `perf` are never in `dist` (the gate checks the feature set).
- **Task worktrees.** Each worktree gets its own `target/` seeded by an APFS clone of the main checkout's `target/` (`cp -c -R`: copy-on-write, near-zero space, dependencies already built). If R0-08 shows cargo rebuilds the registry crates anyway, use `sccache` (Apache-2.0) instead. A group starts only with at least 25 GB free, and at most 3 Rust tasks build at the same time (the Mac has 24 GB of memory); a finished task's `target/` goes with its worktree at cleanup (spike: 4.9 GB per target with debug and release). Planner's call after the builder check: the disk had 28 GB free on 2026-11-17; the owner agreed to clear rebuildable caches (Xcode DerivedData, the Go build cache, the npm and Homebrew caches), which left 54 GB.
- **Lock file.** A merge conflict in `Cargo.lock` is resolved by taking `codepass`'s file and running `cargo check --workspace --offline`, which adds only the new entries.

### 4.4 Threads and async: the UI thread never blocks

| Thread | Runs | Never |
|---|---|---|
| Main (gpui foreground executor) | input, model entities, layout, paint, short foreground tasks | file or network I/O, starting or waiting for a process, sleeping, taking a lock a service holds for long, more than about 2 ms in one handler |
| gpui background executor | CPU work for views: ranking (repository search, quick open), diff layout, highlighting, parsing results | blocking I/O beyond reading one file |
| tokio runtime `cp-io` (2 workers plus the blocking pool; owned by `cp-net`, started in `main`) | network (HTTP client, AWS SDK), the IPC server, git and tool child processes (`tokio::process`, timeouts, kill on drop), timers and loops (15-minute fetch, sign-in ticks, usage reads) | touching gpui |
| Dedicated threads | per terminal: one PTY reader, one writer; Windows `ClosePseudoConsole` (it can block on old Windows 10, A2-01); file watcher backends; Windows `SetThreadExecutionState` holder; Windows notification activation | waiting for the UI |

**Is tokio needed?** Yes, in the service layer only. The AWS SDK and the HTTP client run on tokio. gpui code awaits tokio `JoinHandle`s and `tokio::sync` channels, which work on any executor. Views never create runtimes.

**How data reaches views.**
- *State* goes through `tokio::sync::watch` (latest value only). A model entity keeps one foreground task per subscription: on change it takes the snapshot (an `Arc`) and calls `cx.notify()`. A slow UI skips states; a service never waits for the UI. This is the Go lesson: a sender that waits on a 64-event queue stalled gopls for 37 s and the terminal at 512 KB (plan 5).
- *Events* (one-off facts: a sign-in finished, a notification was clicked, a second instance started) go through `tokio::sync::broadcast` (capacity 256); a receiver that lags reads the snapshot again.
- *High-rate sources* (terminal output, process tracker, file watcher, language servers) never send one message per item. They update shared state and set a dirty flag; at most one wake is pending per source; the view reads at paint, so updates cap at the frame rate.
- *Terminal lock.* The reader holds the emulator's `alacritty_terminal::sync::FairMutex` only per chunk (at most 64 KB of input); the painter takes it to copy the visible cells. With a plain mutex the spike's reader starved painting for 97 ms; the fair lock brought frame gaps to 14 to 40 ms.
- *Platform callbacks* (notification clicks, FSEvents, COM activation) only hand their data to the runtime or to `AsyncApp`; they never do work in the callback.
- *Long stops.* "Stop all" can block about 15 s (m2C); it runs on the runtime while the window shows progress; the app quits after.

**Enforced by:** clippy `disallowed-methods` in the gpui crates (`std::fs::*`, `std::process::Command::{output, status, spawn}`, `std::thread::sleep`, any `block_on`); a development-build watchdog that logs every main-thread task over 50 ms (and fails UI tests); the frame-time checks of section 9.

### 4.5 App state: how services reach views

- **Service traits** in `cp-app`, one per Go service and named the same (`AccountService`, `RepoService`, `GitGlobalService`, `SystemService`, `ToolService`, `IdentityService`, `UpdateService`, `NotifyService`, `TerminalService`, `TaskService`, `KeepAwakeService`, `ProctrackService`, `AIToolService`, `MigrationService`). Methods are async and return `Result<T, AppError>`; each trait has `fn state(&self) -> watch::Receiver<Snapshot>`. The Go API types in `internal/api` become plain Rust structs in `cp-app::api` with the same field names.
- **`Services`** is a struct of `Arc<dyn …>`, built by `cp_app::real(paths, opts)` or `cp_mock::world(scenario)`, and stored as a gpui global. Mock mode swaps the whole struct; no view knows which one runs.
- **Models.** One entity per area, as the React providers in `frontend/src/state/` today: accounts, repositories and projects, environment, settings, app info, update, tasks, sessions, keep-awake, notifications. Created once at start; views observe them.
- **Router.** One entity: the page routes and `Workspace { repo, folder, tab }`; per-folder state (tabs, open files, panel height) keyed by repository and folder id, as `state/workspace.tsx`.
- **Commands** are gpui actions; the menu, the shortcut and the palette run the same action (UI map 4).
- **Rule:** only services touch the disk, the network, processes or the OS. Views never call `cp-platform`, `cp-exec` or `std::fs`.

### 4.6 Errors

`AppError { code, params, detail }`. The codes are exactly the strings of `internal/api/errors.go` (`E_INTERNAL`, `E_INVALID_INPUT`, `E_CONFIG_SHARED`, `E_LOGIN_TIMEOUT`, `E_GIT_TOO_OLD` …) with the same param names (`field`, `running`, `cap` …), so the `errors.*` locale keys and the UI's error mapping carry over. Crates use `thiserror` enums inside and convert at the service boundary; `anyhow` only in `xtask` and tests. `detail` is redacted when the error is built. Release builds unwind: a panic in a service task becomes `E_INTERNAL` plus a redacted log line; a panic on the main thread leaves a crash marker that Health shows at the next start. Nothing is uploaded.

### 4.7 Logging and redaction

`tracing` with a JSON-lines writer to `<LogDir>/app.log`, rotated at 1 MiB keeping 3 files (as `logx`); level info, debug with `AWSSSOHUB_DEBUG`. A redaction layer passes every message and string field through `cp_core::redact` (port of `redact.go`, later with GitHub, Bitbucket, Anthropic and OpenAI token shapes, plan 3.3). Secrets live in `secrecy::SecretString`, so `Debug` never prints them. Never logged: terminal bytes, language-server traffic, hook payloads, tokens and keys, notification text, ticket and conversation text, command lines of tracked programs. The crates that carry such bytes (`cp-pty`, `cp-term`, `cp-tasks::report`, later `cp-lsp`) do not depend on `tracing` at all, and `cargo xtask gate` checks the dependency graph (the Rust form of Go's `TestNoLogging`).

### 4.8 Start order (`main.rs`)

1. Update helper mode, as the first statement (a `syn` test checks it; plan 4.14 item 6).
2. Report mode `codepass report <tool>`: read the hook payload, reduce it to the fixed fields, send, exit within 5 s; no paths, log or services (A2-05).
3. `cp_core::guards::production()`, called only by this binary. Test binaries never call it, so every guard is armed in every test (7.4).
4. Paths (the sandbox only in non-`dist` builds), folders, log, tokio runtime.
5. Stores: a `projects.json` from a newer Codepass shows a native message and quits without writing; a damaged one is quarantined and replaced (FU-18d).
6. Old-app guard (not in sandbox): AWS SSO Hub running → "quit it" or "move it to the Trash" (plan 4.2, `appguard`).
7. Single instance with the Wails-compatible lock (5.2h). A second instance sends its arguments over IPC and exits.
8. IPC server (hook reports and second instance); system proxy detection (at most 5 s, off the main thread); `Services`; the notification backend first, so a click that launched the app is routed; update controller (`Highest.Record`, staged leftovers removed).
9. gpui `Application`: theme, menus, window 1200x780 (minimum 980x660), first frame.

## 5. Every Go package and its Rust home

Versions were checked on 2026-11-17 (`crate-facts.md`). Where GPUI Kit or gpui already uses a crate, its version is taken so nothing is built twice (for example `similar` 2.7, not 3.2; `tree-sitter` 0.26, not 0.27); otherwise the newest. Licences: A = Apache-2.0, M = MIT, "A/M" = either.

### 5.1 The map

| Go package | Rust home | Key crates | Must not be lost (from the code, reviews and wiring notes) |
|---|---|---|---|
| `accounts` | `cp-accounts::engine` | — | Accounts state from the config files and settings; sign-in, refresh, logout with the countdown; the session end learned from AWS (0.0.2: exact once a renewal is capped or refused; before that an estimate never above the token's hour); one heads-up and one "ended" per portal sign-in, keyed by portal and sign-in time; a network failure is never "expired"; profile writes backup-first |
| `aitools` | `cp-tools::ai` | — | Detection only through `--version` in a constant shell text; install commands per OS shown before they run; Claude `--session-id`, `--resume`, `--plugin-dir` (never `--settings`); OpenCode always `--standalone`; Codex `-c notify=[…]` only when the user has none, `-c` before `resume`; the description only after `--` or as `--prompt=`; the test that no permission-skipping or sandbox-override flag can be produced; naming call (fixed prompt, smallest model, empty folder, 5 s, API keys unset); account variable only `CLAUDE_CONFIG_DIR`/`CODEX_HOME` with an absolute path; Windows refuses `.cmd`/`.bat` and starts the npm package's real entry |
| `api` | `cp-app::api` | `serde` 1.0.229 (A/M) | Same field names, error codes, event names, so the locale keys and the mock world carry over |
| `app` | `cp-app` (services, loops), `cp-ui::shell` (menu, window), `codepass` (main) | — | Menu of design 4.4 (Cmd+R never reloads, Cmd+W closes an editor tab); closing hides while tasks run; quit decision; logout stops all without a dialog; startup dialogs (damaged or newer `projects.json`); background fetch only for bound or opened repositories; the existing menu-bar (macOS) and tray (Windows) item with each portal's sign-in state and countdown (`tray.go`, settings `tray.enabled`, `tray.keepRunning`) stays on both platforms, built with `tray-icon` 0.26.1 (A/M) because gpui has none (this also answers UI map question 2: the item exists today, so it stays). Dropped with reason: `runtimeguard` (no webview), `QuietBindingLogger` (no bindings) |
| `appguard` | `cp-accounts::old_app` | — | Detect a running AWS SSO Hub (bundle path, process name) before the single-instance check; "quit it" or "move it to the Trash" (Recycle Bin for the 0.0.1 portable exe); every system touch behind a trait with fakes |
| `awsapi` | `cp-aws::{sts, iam, console}` | `aws-sdk-sts` 1.126.0 (A); IAM by signed request (5.2a) | Console link through the federation `getSigninToken` with `SessionDuration`, per partition; the link is a secret (never logged, never in an error, never to a view); `Endpoints` override for the fakes |
| `awsconfig` | `cp-aws::config` on `cp-files::ini` | — | `~/.aws/config` and `credentials`, `sso-session` and legacy profiles; `AWS_CONFIG_FILE` and `AWS_SHARED_CREDENTIALS_FILE` outside the sandbox |
| `awsguard` | `cp-net::guard` | — | Non-loopback hosts refused while armed (7.4) |
| `backup` | `cp-files::backup` | — | Redacted copy before every write; atomic writer (temp, fsync, rename; Windows sharing-violation retry); retention; restore |
| `browser` | `cp-platform::browser` | — | Private window of the default browser, launched by absolute path with argv only; an isolated instance with an empty profile per sign-in (Chrome, Edge, Brave, Chromium), closed and deleted when the sign-in ends, when the page reopens and at quit; leftovers removed at start; Windows focus rules |
| `codecommit` | `cp-aws::codecommit` | signed request (5.2a) | Owner detection with `BatchGetRepositories` per usable account (100 names a call, metadata in chunks of 25); regions only from the remote URL |
| `credstore` | `cp-git::credstore` | — | Cached CodeCommit passwords listed and cleared through `/usr/bin/security` and the Credential Manager with argv only; output discarded, exit codes only |
| `envcheck` | `cp-tools::env` | — | No network in detection; known paths plus `--version` with a timeout; no developer-tool stub before `xcode-select -p` prints a real path; git ≥ 2.44, AWS CLI ≥ 2.9.0; key scan returns no line content |
| `execx` | `cp-exec` | — | PATH for apps started from Finder through the login shell (guarded, opt-in in tests); sanitised environment; argv only; FU-SGb (keep an earlier lookup error) |
| `gitglobal` | `cp-git::global` | — | Default helper, guard, URL rules; every change simulated on a temp copy (preview), backed up, hash-checked, applied |
| `gitx` | `cp-git::{remote, matching, bind, clone, conntest, scan, read, status}` | — | Credential matching that mirrors git's own; the exact helper, fallback and guard strings (old bindings must be recognised); connection tests outside the repository with `-c`; read-only flags (4.6 of the plan); `GIT_NO_LAZY_FETCH` |
| `identity` | `cp-repos::identity` (author) and `cp-accounts::colors` (colour slot, monogram) | — | Identity computed by git (`config --show-origin --show-scope`, `git var GIT_AUTHOR_IDENT`) in the terminal's environment; `includeIf` and environment sources; writes backup-first with undo; monogram rules (2 Latin letters or digits, or 1 CJK character) |
| `ini` | `cp-files::ini` | — (no mature Rust crate keeps comments: `rust-ini` drops them, `ini-preserve` is months old) | An unmodified document writes back byte-identical; an edit touches only its lines |
| `keepawake` | `cp-tasks::keepawake`, holders in `cp-platform::awake` | `objc2-foundation` 0.3.2 (M), `windows` 0.62.2 (A/M) | One counter; `/usr/bin/caffeinate -d -i -w <pid>` restarted if killed; `NSProcessInfo` activity against App Nap; `SetThreadExecutionState` on one thread; grace and ceiling; leftovers count; never a plain shell; the change callback must not re-enter (FU-A2-03f) |
| `logx` | `cp-core::log` | `tracing` 0.1.44 (M) | 4.7 |
| `migrate` | `cp-accounts::migrate` | — | The old static key only in memory, dropped on finish, discard or exit; pending deactivations in settings |
| `netx` | `cp-net` | `reqwest` 0.13.5 (A/M) on `rustls` 0.23 with `rustls-platform-verifier` 0.7.1 (A/M; the OS trust store, so a company proxy's root works) | System proxy (`scutil --proxy`, WinHTTP) for own requests and for children, upper and lower case, `NO_PROXY`: one source of truth, so reqwest's own `system-proxy` feature is off and `cp-net` hands it the detected proxy; ETag cache; proxy credentials never logged |
| `notify` | `cp-notify`, backend in `cp-platform::notify` | gpui-pre's notification API (5.2f) | One service; one button per kind; the fake in sandbox, tests and development builds; permission asked the first time a portal signs in with the window in front; `notifications.json`; every rule of plan 4.15 "When not to notify"; no secret, URL, ID or path in any text |
| `paths` | `cp-core::paths` | — | Sandbox layout `<root>/home`, `<root>/appdata`; sandbox only in non-`dist` builds; the data folder name |
| `proctrack` | `cp-tasks::proctrack`, table in `cp-platform::proc` | `libc` 0.2.191 (A/M), `windows` 0.62.2 | One process listing per tick (`sysctl` instead of spawning `ps`); identity is PID plus start time; `setsid` and re-parented children kept; Force stop TERM, about 5 s, KILL, or `TerminateJobObject`; `leftovers.json` mode 0600 with names only; survivors listed after a crash; Windows parent IDs (FU-A2-13a) |
| `projects` | `cp-repos::projects` | — | `projects.json` schema; random stable IDs and `legacyId`; registry (`IDForPath`, `Relocate`, `ByLegacyID`); moved-folder offer (first commit and origin, 30 days); never touches `settings.json` |
| `pty` | `cp-pty` | `rustix` 1.1.6 (A/M), `windows` 0.62.2 | 5.2c |
| `redact` | `cp-core::redact` | `regex` 1.13.1 (A/M) | The same masks; new token shapes added with their milestones |
| `repos` | `cp-repos::engine` | — | CodeCommit behaviour byte for byte; provider strategies; the 1,300+ lines of repository tests; hidden repositories leave all background work; found set after scans; folder-name validation |
| `settings` | `cp-store::settings` | `serde_json` 1.0.154 (A/M) | 5.2i |
| `shellguard` | `cp-exec::shellguard` | — | 7.4, with FU-SGa to FU-SGc |
| `sso` | `cp-aws::sso` | `aws-sdk-ssooidc` 1.123.0, `aws-sdk-sso` 1.121.0 (A) | The CLI-compatible cache (file shape, key = sha1 of the session name or start URL, botocore time layout `%Y-%m-%dT%H:%M:%SZ`, 15-minute refresh window); the app's own client registration, never the CLI's file; PKCE with a loopback redirect and device code; per-session logout; a cached token is never printed |
| `store` | `cp-store::versioned` | — | `schemaVersion`; forward migration; backup before the first write of a new version; a newer file is refused, never overwritten; quarantine of a damaged file |
| `tasks` | `cp-tasks::lifecycle` | — | `tasks.json`; task IDs `^[A-Za-z0-9._:-]{1,128}$`, lowercase session IDs; session IDs made by the app; soft cap 6 with its confirm; Resume never starts a new session by itself; "stop all" = tasks, then terminals (10 s), then Force stop of every leftover, off the UI thread; the first "still running" notice once ever; `sessions/` excluded from Time Machine; ended logs released only after their scrollback is written (FU-A2-02b); wiring order (FU-A2-08b) |
| `taskstate` | `cp-tasks::{report, socket, tracker, fallback, inject}` | — | Report mode forwards only fixed fields and values from fixed lists (the first prompt only when armed for naming); same-user socket or pipe; Claude plugin folder, OpenCode `OPENCODE_CONFIG_CONTENT`; fallback says only alive or exited; FU-A2-05a to FU-A2-05c behaviour; the four conversation strings in one place with their tool version |
| `terminal` | `cp-term` (sessions) and `cp-exec::terminal_app` ("Open in terminal") | `alacritty_terminal` 0.26.0 (A), `vte` 0.15.0 (A/M) | 5.2d; "Open in terminal" refuses folders with `\`, `!` or `"` on macOS |
| `textdiff` | `cp-files::diff` | `similar` 2.7.0 (A) | The same line diffs in write previews |
| `toolinstall` | `cp-tools::install` | — | 5.2l |
| `transfer` | `cp-accounts::transfer` | — | Allow-list document, versioned; nothing that looks like a credential or a command is accepted on import |
| `update` | `cp-update` | `ed25519-dalek` 3.0.0 (BSD-3), `sha2` 0.10.9 (A/M), `semver` 1.0.28 (A/M), `zip` 9.6.1 (M), `security-framework` 3.7.0 (A/M) | Section 10 |
| `version` | `cp-core::version` | — | BundleID and product name constants; version from the workspace |
| `watch` | `cp-watch` | `notify` 8.2.2 (CC0-1.0) | Recursive with bounded handles (one FSEvents stream or one directory handle per root); `node_modules`, `.git/objects` and lock files dropped; 150 ms debounce; Pause and Resume close and reopen the handles (Windows renames); a vanished folder reported |
| `main.go` | `codepass::main` | — | 4.8 |
| `frontend/src` | `cp-ui`, `cp-ui-kit`, `cp-term-view`, `cp-i18n` (locales), `cp-mock` (api/mock world) | gpui-kit 0.10.0, gpui-component 0.10.0, gpui-base 0.10.0, gpui-pre 0.3.12 (all A) | The design (UI map); the logic of every `model.ts`; the terminal rules of A2-07 (Shift+Enter sends ESC CR, OSC 52 off, paste confirm over 5 lines, control bytes in a paste never reach the child (FU-A2-07b), replies never counted as typing) |

### 5.2 Notes by topic

**a. AWS: official SDK or plain HTTP.**

| | Official `aws-sdk-*` crates | Plain HTTP + `aws-sigv4` |
|---|---|---|
| Correctness | AWS keeps endpoints (partitions, FIPS), error shapes (`authorization_pending`, `slow_down`, `invalid_grant`, `UnauthorizedException`) and retries | we write and maintain them |
| Cost | tokio plus the smithy runtime (tokio is in gpui's tree already); IAM and CodeCommit crates are large generated code for one or two calls | small, fast to compile |
| Tests | an `HttpClient` adapter routes them through `cp-net` (proxy, TLS roots, AWS guard) | the same adapter |

Planner's call: the SDK for the sign-in path, `aws-sdk-ssooidc` (`RegisterClient`, `StartDeviceAuthorization`, `CreateToken` with device code, authorization code plus PKCE, refresh token) and `aws-sdk-sso` (`ListAccounts`, `ListAccountRoles`, `GetRoleCredentials`, `Logout`), plus `aws-sdk-sts` (`GetCallerIdentity`). IAM `UpdateAccessKey` (one call, migration wizard) and CodeCommit `ListRepositories`/`BatchGetRepositories` are signed requests through `cp-net`. The SDK crates take `default-features = false, features = ["rt-tokio"]` plus the `HttpClient` adapter over `cp-net`'s reqwest, so the whole app has one HTTP and TLS stack (with default features they would compile a second one: hyper 0.14 with rustls 0.21). No `aws-config`: like the Go code, credentials are always explicit (static keys or role credentials from `cp-aws::sso`), and `aws-config` never runs an interactive sign-in anyway. Rule for the builder: if R1-06 measures that the CodeCommit SDK crate adds less than 30 s to a clean `dist` build and less than 2 MB to the binary, use it instead.

**b. Git CLI runner** (`cp-git`). Three constructors: `read_only()` always adds `--no-optional-locks -c core.fsmonitor=false` (and `--no-ext-diff --no-textconv` for diff-like commands), sets `GIT_NO_LAZY_FETCH=1`, refuses mutating subcommands; `background_network()` adds no prompt of any kind, an empty hooks folder, `--no-auto-maintenance`, a 2-minute no-progress timeout; `user_action()` runs hooks and filters because the owner clicked. All are argv lists, run on the runtime with timeouts and kill on drop; `/usr/bin/git` only after the `xcode-select -p` rule. Pull all, force push with lease, discard refs and the rest of plan 4.6 and 4.6a build on these in milestone 4.

**c. PTY** (`cp-pty`, own code; planner's call over `portable-pty`, which has no Job Object list and no kill by session). Unix: `openpty` through `rustix`, the child made a session leader with a controlling terminal (`setsid`, `TIOCSCTTY` in `pre_exec`), resize with `TIOCSWINSZ`, foreground group with `tcgetpgrp`; `kill_session` lists processes whose `getsid()` equals the leader's and signals every group they lead (TERM, about 5 s, KILL); `members()` never waits for a kill (FU-A2-01a). Windows: `CreatePseudoConsole`, one attribute list with `PSEUDOCONSOLE` and `JOB_LIST`, a Job Object with `KILL_ON_JOB_CLOSE`, `ClosePseudoConsole` on its own thread; written over the `windows` crate (credit `portable-pty`, MIT, where its ConPTY code is followed). The shell guard runs before every start.

**d. Terminal emulation** (`cp-term`). One `alacritty_terminal::Term` per session behind its `FairMutex` (4.4). A reader thread feeds the `vte` parser in chunks of at most 64 KB and appends the raw bytes to a ring (4 MB, the scrollback-file cap); a writer thread drains a bounded queue, so a child that stops reading its input makes `write` fail, never the UI wait. Emulator events: `PtyWrite` (terminal replies) go to the writer and never count as the owner's typing (FU-A2-05c: only Enter, a lone Esc or a digit counts as an answer; typing renews keep-awake); OSC 52 store and load are ignored (clipboard writes off); title, bell and child exit become session state. Scrollback 10,000 lines in memory. At session end the ring is written to `<data>/sessions/<task-id>/scrollback` (mode 0600; a setting turns it off; never in a backup, export or log), cut only where the parser is in ground state and on a UTF-8 boundary, preferring the last full clear or alt-screen entry (the Go `modes.go` rules with FU-A2-02a and FU-A2-02c fixed); an ended task's screen is restored by replaying that file into a fresh `Term`. A session ID that is started again resets its view (FU-A2-02d). The pull-by-offset transport, the prologue and the size nudge existed for the webview and are dropped with that reason. The terminal crates on crates.io do not fit (`gpui-terminal` targets the old `gpui` 0.2.2, `gpui_xterm` targets GPUI Kit 0.9), so the view is our own (`cp-term-view`, UI map 2). The environment keeps the deny list and additions of m2B (drop `AWSSSOHUB_*`, `WAILS_*`, `CODEPASS_*`, `TERM`, `COLORTERM`, `PWD`, `OLDPWD`; add `TERM=xterm-256color`, `COLORTERM=truecolor`, a UTF-8 `LANG`, the proxy variables).

**e. Editor and language servers** (M5). The editor is GPUI Kit's code editor (tree-sitter highlighting, its completion, hover, definition and code-action providers; UI map 2), wrapped in `cp-editor`. `cp-lsp` keeps the technology table's message-log design without the webview: one reader thread per server appends whole messages to a per-server log with sequence numbers; responses and requests are never dropped; above 16 MB progress notes, log messages and superseded diagnostics are compacted; a hard limit restarts the server; requests from the server (configuration, work-done progress) are answered by the service even when no view is open; pull diagnostics for the native TypeScript server; idle and no-pull shutdown after 10 minutes; at most 2 servers (4 with 32 GB or more). Hover Markdown only through `safe_markdown()`. Types from `lsp-types` 0.97.0 (M), already in GPUI Kit's tree.

**f. Notifications with a button.** gpui-pre 0.3.12 already has `show_system_notification` (title, body, buttons, a tag that replaces), `dismiss_system_notification` and `on_system_notification_response` (tag and button), and it owns the macOS delegate and the Windows AppUserModelID (UI map 4). Missing for plan 4.15: the permission state (allowed, not asked, off) and asking at the chosen moment (gpui asks at the first show), the default sound for four kinds, and a click on a notice while Codepass is not running (Windows toasts call back only in-process; launching the app needs a COM activator registered with a `ToastActivatorCLSID`, which Wails did, plan research N 3.3 and 3.4). Planner's call: use gpui's backend and add these in a patch in `third_party/gpui-pre-*` (about a few dozen lines per platform, UI map 4); never install a second delegate. If R0-09 finds the gaps larger, `cp-platform::notify` replaces the whole backend (`objc2-user-notifications` 0.3.2 on macOS; toasts and a COM activator over `windows` on Windows, as Wails did) and gpui's is not used.

**g. Keychain.** macOS through `security-framework` 3.7.0 (A/M), calling Security.framework directly, so an item trusts Codepass's signature; never `/usr/bin/security` for the app's own secrets (plan 3.3, R11). Windows: `CredWriteW`/`CredReadW`/`CredDeleteW` over `windows`. Sandbox and tests: a file fake. Planner's call: a 200-line module instead of the `keyring` crate, so item names and attributes stay exactly as specified.

**h. Single instance and IPC** (`cp-platform`). Same lock as Wails, so AWS SSO Hub 0.0.x and Codepass can never run at once: on macOS `flock` on `$TMPDIR/com.rudian.awsssohub.lock`; on Windows the mutex `wails-app-com.rudian.awsssohub-sim`. The first instance also listens for the distributed notification named `com.rudian.awsssohub` (what a Wails app posts when it starts second) and shows its window. Codepass's own second instance sends its arguments over the IPC endpoint: a Unix socket in the data folder (path length checked, fallback under `$TMPDIR`) or a named pipe with a user-only ACL; the peer must be the same user (`getpeereid`; the pipe client's token). The same endpoint carries hook reports (5 s client timeout; the app never makes a hook wait). Crate: `interprocess` 2.4.8 (0BSD or A) or own code over `tokio::net` (builder's call); not the `single-instance` crate (abandoned since 2021, no argument forwarding).

**i. Settings and the data store** (`cp-store`). `settings.json` keeps every field name and meaning of 0.0.1, 0.0.2 and Go Codepass; a new field must have a safe zero value, stored as "off" where needed (`updateCheckOff`, `reminders.endedNoticeOff`, `git.autoPullAfterSignInOff`); `privateSignIn` absent means true. Go writes `null` for empty lists and maps, so every list and map field reads `null` as empty. Times read and write in the RFC 3339 form Go uses (nanoseconds and an offset). Unknown fields are kept on write (`serde_json` with `preserve_order` and a flattened map; planner's call: a newer app's field survives this one). The versioned files (`projects.json`, `tasks.json`, later `accounts.json`, `ai-accounts.json`, `tickets.json`) follow `store`: `schemaVersion`, migration, backup before the first write of a new version, refusal of a newer file, quarantine, atomic write.

**j. Migration from the old app's data.** Nothing is converted in place. Codepass reads the 0.0.x folder as it is (`settings.json`, `backups/`, `registrations/`, `logs/`; `WebView2/` is left alone), adds its own files, and imports the old test results from `settings.json` (keyed by the old path hash) into `projects.json` once (F1-18), leaving `settings.json` untouched for the old app. Shared with the AWS CLI and git, byte-compatible: `~/.aws/config`, `credentials`, `sso/cache`, `cli/cache`, repository-local and global git config with the markers. Accept test: the 0.0.1 binary still starts on a folder Codepass has written, and its data is intact.

**k. Languages** (`cp-i18n`). The 21 TypeScript namespaces become `locales/zh-CN/<ns>.json` and `locales/en/<ns>.json` by a one-time `xtask` conversion; **every key keeps its name**. A build script generates a `Key` enum, so a missing key is a compile error (as the TypeScript `MessageKey` type is today), and checks that both languages have the same keys and placeholders. The formatter ports `format.ts`: `{name}`, `{n, plural, one {…} other {…}}`, `#`. The strings Go held (notification texts, startup and old-app dialogs, menu labels) move into the same catalogue. The tutorial becomes Markdown per language. Language: the setting, else the system language (`zh*` → zh-CN, else en). GPUI Kit translates its own built-in strings with `rust-i18n` 4; `cp-i18n` sets that locale whenever the app's language changes. Planner's call: our own formatter rather than `rust-i18n` for our strings, because it keeps today's message syntax and makes a missing key a compile error.

**l. Homebrew and winget** (`cp-tools::install`). The plan 4.1 flow without change: download with size and sha256 from the GitHub API; the unprivileged pre-check (size, digest, `pkgutil --check-signature`, Team ID `927JGANW46`); one privileged step whose script is a constant passed to `/usr/bin/osascript` as an argument (every value through `quoted form of`), started with a fixed environment and running under `/usr/bin/env -i /bin/bash --noprofile --norc`, tools by absolute path, copy from one descriptor into a root-only folder, verify the copy, install, reasons on stderr, the exit codes of the spike note; brew and winget as the user with analytics off and the proxy variables in both cases; failure classification tested on the recorded outputs. Never a password through the app.

**m. CodeCommit credentials, backup and transfer** are in the table (`gitx`, `gitglobal`, `credstore`, `backup`, `transfer`); their tests and `testdata/` move unchanged (7.2).

## 6. UI architecture

### 6.1 Views per screen

All views live in `cp-ui` on top of `cp-ui-kit` (UI map 2 lists the component behind each design part).

| Area | Views (module) | Models | Design |
|---|---|---|---|
| Shell | `shell::{sidebar, keyring, global_banner, toasts, confirm, quit_dialog, session_switcher, repo_switcher, quick_capture}` | Router, Accounts, Tasks, KeepAwake, Update | 4.1, 4.4, 5.6b, 5.7a, 5.7c |
| Setup (the tools gate) | `pages::setup` | Env, Tools | 5.1 |
| Projects | `pages::projects::{page, section, row, clone, bind, found, moved, rename_folder, pull_all}` | Projects, Repos | 5.2, 5.2a, 5.2b |
| Accounts | `pages::accounts::{page, portal, pass, login, add_portal, color, export, import, remove_profile, move_repos, legacy}`; later AI accounts (M8) and GitHub/Bitbucket (M6) | Accounts | 5.11, 5.16 |
| Git settings and identity | `pages::git::{default, url_rules, saved, identity}` | GitGlobal, Identity | 5.12 |
| Health, Tutorial, Onboarding, Migration wizard | `pages::{health, tutorial, onboarding, migrate}` | Env, Tools, Migration | existing 0.0.x screens |
| Settings, Update | `pages::{settings, update}` | Settings, Update, Notify | 5.13, 5.14 |
| AI tasks (all repositories), Tickets, Pull requests | `pages::{ai_tasks, tickets, prs}` | Tasks, Tickets, PRs | 5.7, 5.15, 5.10 |
| Workspace | `workspace::{frame, header, rail, tabs, status_bar, panel, tree_switch, repo_settings}` and one module per tab: `code`, `changes`, `history`, `branches`, `conflict`, `prs`, `ai` | Router (per folder), Repos, Tasks, Terminals | 4.2, 5.3 to 5.9 |

### 6.2 State flow

Service snapshot → `watch` → model entity → `cx.notify()` → views re-render from the model. A user action calls the service on the runtime (`cx.spawn` awaiting the service future); the result arrives as a new snapshot or as an `AppError` that the view maps to text through the `errors.*` keys. No optimistic UI except where the design says so (the Undo toasts). Lists render only visible rows (`uniform_list`, GPUI Kit `List`/`DataTable`); filtering and ranking over large lists run on the background executor.

### 6.3 Mock mode

`codepass --mock <scenario> [--page …] [--repo …] [--tab …] [--theme light|dark] [--lang zh-CN|en] [--platform windows] [--opt key=value]` replaces the Vite mock and its URL parameters. It exists only in non-`dist` builds (feature `mock`). `cp-mock` implements every service trait over the generic world ported from `frontend/src/api/mock/world.ts` (scenarios `fresh`, `mid`, `done`, `empty`, `edge`; per-service options such as `aitools=fail`, `update=available`, `search=<text>`, `leftover=stuck`). Mock mode never builds a real service, so it touches no disk outside a temp folder, no network, no keychain and no tool. Two scripted fakes stay: a terminal that replays a recorded generic Claude Code session **through the real `cp-term` emulator** (so the mock exercises the real view path), and a language server (M5).

### 6.4 Screenshot pipeline

As UI map 6: feature `shots` turns on `test-support` in `gpui-pre`, `gpui-pre-apple` and `gpui-pre-macos` (all three, spike a). `codepass --mock mid --shots <dir> [--only <name>]` walks the shots table in `cp-ui::shots` (name, scenario, route, theme, language, a setup closure), waits for two idle frames with animations at their end state, calls `Window::render_to_image()` and writes `<name>.png` at 1440x900 logical. Names are exactly the 56 names in `docs/design/codepass-mockup/shots/`; output goes to `docs/design/native-shots/`. `cargo xtask shotdiff` makes one sheet per name (mockup, native, difference map, a changed-pixel share after a 2 px blur). First review is by a person or a reviewing agent with UI map 6's checklist; an approved native shot becomes the baseline, and later gates compare native against native with a strict threshold. No Screen Recording permission is needed; a window opens briefly.

### 6.5 Text, keys and accessibility in views

- Strings come only from `cp-i18n` keys (5.2k); a view never holds a literal user-visible string (a clippy-like `xtask` check scans `cp-ui` for string literals in `label`/`text` calls).
- Every interactive element has an AccessKit role and a label (UI map 3); live regions go through `cp_ui_kit::a11y::announce` (UI map 3: gpui-pre has no live regions).
- Key contexts: `Workspace`, `Editor`, `Terminal`, `Dialog`, `List`. The `Terminal` context binds only the keys design 4.3 lists; on Windows Ctrl+J and Ctrl+O stay in the terminal.

## 7. Testing and the new gate

### 7.1 Test layers

1. **Unit tests** in every service crate (no gpui), run with `cargo nextest`.
2. **Integration tests** with real git in temp repositories, real PTYs with a temp HOME and ZDOTDIR, and the fake servers of `cp-testkit` on loopback (ported from `internal/sso/ssotest`, `internal/awsapi/awsapitest`, `internal/gitx/gitxtest`).
3. **Property tests** (`proptest`) for parsers: INI round trip, terminal mode tracking and cut points, porcelain and for-each-ref parsing, redaction, version rules.
4. **UI tests** with gpui's `TestAppContext` and `VisualTestContext`: keyboard maps, focus, state rendering, routing, dialogs. They assert on models and element state, not pixels.
5. **Screenshots** (6.4) for the look; **perf runs** (section 9) for speed.

### 7.2 Carrying the Go and frontend tests over

The Go code and its tests are the executable specification until they are deleted.

- **Spec map.** `cargo xtask spec-map --init <pkg>` lists every `Test…`/`Fuzz…` function of a Go package (`go test -list`) into `docs/plan/codepass/rust/spec/<pkg>.md`. A Rust test that covers one carries `// spec: <pkg>/<file>_test.go:<TestName>`. Every Go test ends as **ported** (a Rust test names it) or **dropped** with a reason (for example "Wails binding transport", "webview replay"). `cargo xtask spec-map --check` fails the gate while a ported package has an unmapped Go test. The frontend's logic tests (`*model.test.ts`, `repoSearch.test.ts`, `keys.test.ts`, `format.test.ts`, the terminal key and paste tests) get the same treatment; component-rendering tests are replaced by UI tests plus shots.
- **Fixtures move as they are.** Every `testdata/` folder (`awsconfig`, `gitglobal`, `gitx`, `transfer`, `execx`), the recorded installer outputs, the 0.0.1 and 0.0.2 data-folder fixtures and the manifest golden vectors are copied byte for byte into the Rust crate's `tests/fixtures/`. Same input, same expected output.
- **Differential tests** (planner's call) for five pure modules where a subtle difference would be silent: the INI editor, redaction, git credential matching (`gitx/match.go`), SSO cache keys, manifest verification. A tiny Go oracle program (`tools/go-oracle`, built from the frozen Go code) and the Rust code run the same generated inputs and must agree. The oracle is deleted with the Go code.
- **Follow-ups.** Every open follow-up of the Go and React code (FU-A2-02a … FU-A2-08b, FU-SGa to FU-SGd, FU-CLa*, FU-CLb*, FU-GFb, FU-GFc) is listed in the port task of its package and is either fixed there (with the test it asks for) or marked web-only in the spec map.
- **Wiring notes are acceptance.** The facts in m2B and m2C (for example: OwnerInput only for keys that can answer, FU-A2-05c; a reused session ID starts one byte past the old tail, FU-A2-02d; ConnectProctrack only after both services exist, FU-A2-08b) become numbered acceptance items of the R2 tasks.

### 7.3 The gate (replaces `gate.sh`)

`cargo xtask gate` on the Mac, one summary line per step, logs under `$TMPDIR/codepass-gate/`:

```
cargo fmt --all --check
cargo clippy --workspace --all-targets --locked -- -D warnings
cargo xtask test          # nextest, HOME and ZDOTDIR = a trap folder, guards armed, loopback only
cargo deny check          # licences, advisories (RustSec), bans (one gpui-pre), sources (crates.io and third_party/ only)
cargo xtask notices --check   # THIRD_PARTY_NOTICES.md matches cargo-about's output
cargo xtask deps --check  # no gpui outside the UI crates; no tracing in byte-carrying crates; dist has no mock/shots/perf
cargo xtask wincheck      # Windows compile check (7.5)
cargo xtask shots --check # the milestone's shots against approved native baselines
cargo xtask perf --quick  # CPU per frame and terminal throughput (robust under load)
cargo xtask spec-map --check   # port phase only
CODEPASS_SCRUB_LIST="$HOME/.codepass-dev/scrub-list.txt" bash scripts/scrub-check.sh --tracked
git status --short        # must print nothing
```

At a **milestone gate** add: the full perf run on an idle Mac with the thresholds of section 9; a Windows build, the Windows tests and a smoke run on the Windows host; a signed build through the test channel; the owner's manual checks. The workflow script (`codepass-group-v2.js`) gets a Rust variant with this gate string and without the `node_modules` link rules (section 14).

### 7.4 The safety guards in Rust

- **Armed by default.** `cp_core::guards` is armed in every binary; only the app's `main` calls `guards::production()`. A test binary can never forget to arm it (the Go guards used `testing.Testing()`; Rust has no equivalent across crates, so the default is reversed).
- **AWS guard** (Go `awsguard`, FU-22f). Every HTTP client, including the AWS SDK clients, is built by `cp-net`. While armed, its connector refuses any host that is not loopback, before a connection is opened. Tests must pass fake endpoints; a forgotten one fails at once.
- **Shell guard** (Go `shellguard`, F1-SG). While armed, `cp-exec` and `cp-pty` refuse to start a shell with `-l`, `-i` or an rc-reading form (zsh, bash, sh, fish, nu) whose HOME, or ZDOTDIR for zsh, is not inside the temp folder; the refusal is recorded and returned. The flag parsing includes the FU-SGa fixes (zsh `+f`, `-o rcs`; fish value letters). The login-shell PATH lookup does not run at all unless a test opts in with a temp HOME (`cp_exec::allow_login_shell_for_test`).
- **Trap run.** `cargo xtask test` runs the whole suite with HOME and ZDOTDIR set to a new trap folder whose `.zshenv`, `.zprofile`, `.zshrc`, `.zlogin`, `.bash_profile` and `.bashrc` each write a marker; any marker fails the gate (F1-SG accept).
- **Fakes by default.** Notifications use the fake in tests, sandbox and development builds (`AWSSSOHUB_REAL_NOTIFICATIONS=1` only for the owner's manual checks); the keychain is a file fake in sandbox and tests; keep-awake holders, the process table, dialogs, the clock and the browser are traits with fakes; the old-app guard's process lister and Trash mover are fakes in every test.
- **No real home.** Tests build paths from temp roots; the sandbox layout (`AWSSSOHUB_SANDBOX=<root>`: `<root>/home`, `<root>/appdata`) is unchanged; `AWS_CONFIG_FILE` and `AWS_SHARED_CREDENTIALS_FILE` are honoured only outside the sandbox, as today.

### 7.5 Windows builds: the `fxc.exe` problem

`gpui-pre-windows`'s build script compiles its HLSL shaders with `fxc.exe` from the Windows SDK, guarded by the **host** OS, so a cross build from the Mac skips the step and fails or needs a patch (spike c). Options:

| Option | What | Cost |
|---|---|---|
| A. Windows VM on the owner's Mac | Windows 11 on Arm VM with Visual Studio Build Tools and the Windows SDK; builds `x86_64-pc-windows-msvc` with the x64 cross tools; runs the amd64 build under emulation for tests and smoke. `cargo xtask winbuild` drives it over SSH. | Microsoft licence acceptance in the VM; about 40 GB disk; one more machine to keep current. Matches "built on the owner's machines". |
| B. GitHub Actions Windows runner | Builds and tests on `windows-latest` (how Zed builds gpui for Windows); no secret involved; the Mac downloads the zip, checks its sha256 and signs the manifest. | Builds outside the owner's machines (supply-chain trust in the runner); needs the owner's OK for Actions. |
| C. Cross build from the Mac | `cargo-xwin` plus shader blobs precompiled once on Windows and a patched `gpui-pre-windows` (`third_party/`). | A permanent patch to keep; untested. |

**Owner's decision (2026-11-17): B, GitHub Actions.** The Mac has no VM software and too little disk for a Windows VM. R0-08 adds `.github/workflows/windows.yml`: on every push to `codepass` (and by hand), `windows-latest` builds `x86_64-pc-windows-msvc`, runs the Windows tests and keeps the zip as a run artifact; it needs no secret. `cargo xtask wincheck` waits for that run on the pushed commit and reports it; the next group starts only when it is green. A compile check on the Mac (plain `cargo check --target x86_64-pc-windows-msvc`) is added if R0-08 shows it works without accepting any licence; `cargo-xwin` is not used, because its CRT and SDK licence would have to be accepted on the owner's behalf. For a release the Mac downloads the run's zip, checks its sha256 against the run's log and signs the manifest; the exe stays unsigned (owner decision 3). Manual Windows checks at milestone gates still need a Windows machine or VM, which the owner provides at the R1 gate; the Windows 10 1809 check stays with milestone 7 as `tasks.md` has it (an Intel PC or a cloud Windows Server 2019).

## 8. Dependency policy

**GPUI Kit and gpui-pre** (planner's calls throughout).
- **Exact pins** in `[workspace.dependencies]`: `gpui-kit`, `gpui-component`, `gpui-base` at `=0.10.0` (or the version current at R0-01 that builds), `gpui-pre*` at the version GPUI Kit pins (`=0.3.12` in 0.10.0, although 0.3.13 came out on 2026-11-16; GPUI Kit pins it exactly, spike f). Facts of 2026-11-17: gpui-pre is a weekly snapshot of Zed's main branch published by one crates.io user (`huacnlee`, GPUI Kit's maintainer), not by Zed; Zed's own `gpui` crate stopped at 0.2.2 (2025-10-22). `Cargo.lock` committed, `--locked` everywhere, crates.io only (no git dependencies). `cargo deny` bans a second version of any `gpui-pre*` crate.
- **Not vendored by default.** The crates come from crates.io. Only a crate we must change is copied verbatim into `third_party/<crate>-<version>/` and used through `[patch.crates-io]`; `third_party/PATCHES.md` lists each change, why, and the upstream issue. Patches are offered upstream when the owner agrees. Expected early: notification permission and sound (5.2f), live-region announcements if `announce` needs them (UI map 3).
- **Upgrade cadence.** Never inside a milestone. At most one "kit upgrade" task (S, or M when the API broke) at the start of a milestone, and only for a fix or a feature we need; the stream of releases is not followed (gpui-pre weekly; GPUI Kit a breaking 0.x every one to three weeks). The task reads the upstream diff of the platform crates (unsafe code, process, file and network access), runs the full gate, the shots diff and the perf run. A Rust toolchain bump only rides along when the kit needs it.
- **If the maintainer stops.** The sources are Apache-2.0 and the pinned versions stay on crates.io. We also keep an offline copy of every locked crate (`cargo vendor` archive of `Cargo.lock`, refreshed at each upgrade, kept on the owner's Mac and in the owner's backup). Then we vendor the whole set into `third_party/` and maintain it ourselves, or move to Zed's official `gpui` if Zed publishes it again; the façade crates keep either move inside the UI crates. No rewrite in another toolkit is planned.
- All gpui use goes through `cp-ui-kit`, `cp-term-view` and `cp-ui` (later `cp-editor`); service crates never see it.

**Every other crate.** Prefer a crate already in gpui's tree, at the same version (tokio 1.54.1, notify 8.2.2, tree-sitter 0.26.13, lsp-types 0.97.0, objc2 0.6.5, windows 0.62.2 (frozen upstream; the version gpui-pre-windows uses), rustls 0.23.46, serde 1.0.229). A new crate needs: a licence on the allow list (section 2); a release in the last 12 months or a stable 1.x; macOS and Windows support; one line of reason in the task's notes. `cargo deny` checks licences, RustSec advisories, bans and sources at every gate; `cargo about` regenerates `THIRD_PARTY_NOTICES.md`. Credits for derived code (`portable-pty` for ConPTY details, MIT; Wails `pkg/updater` swap logic, MIT) are added by hand.

**Rust toolchain.** `rust-toolchain.toml`: `channel = "1.100.0"`, `components = ["rustfmt", "clippy"]`, `targets = ["aarch64-apple-darwin", "x86_64-pc-windows-msvc"]`, `profile = "minimal"`; `rust-version = "1.100"` and `edition = "2024"` in `[workspace.package]`. A bump only with a kit upgrade that needs it, or for a security fix, with the full gate.

**Tools** (user-level installs, no administrator rights): `cargo-nextest` 0.9.152, `cargo-deny` 0.21.0, `cargo-about` 0.9.3, `cargo-xwin` 0.23.1 (only if `wincheck` works; it needs Homebrew's `llvm` and accepting Microsoft's CRT and SDK licence, and it does not bring `fxc.exe`), `sccache` 0.19.0 (only if needed). `cargo xtask gate` checks their versions before it starts.

## 9. Performance budgets for 1.0

Measured on the owner's M4 Mac with a `dist` build, median of 5 runs, p95 inside a run. The spike numbers are the baseline (release build, machine under load, 75 Hz display).

| Metric | Spike | Budget for 1.0 | How it is measured |
|---|---|---|---|
| Process start to first frame | 324 ms (quiet runs 284 to 307), runtime shaders, empty panes | ≤ 350 ms with the Projects page of the `mid` world (30 repositories) from cached state; ≤ 250 ms with precompiled shaders (question 2) | `perf` feature: process start time (`kinfo_proc`) to the first paint callback |
| Repository status of 30 repositories | n/a | complete ≤ 3 s in the background, no frame over one vsync meanwhile | real git on 30 fixture repositories |
| Idle CPU, window visible, 30 repositories watched, no task | 0.01 % (no frame drawn) | ≤ 0.2 % over 60 s; ≤ 0.5 % with 6 idle AI sessions; hidden window draws no frame | `getrusage` deltas; frame counter |
| App CPU per frame (render to paint), p95 | 1.0 to 1.5 ms | ≤ 4 ms in: 10,000-row list scroll, terminal at 2 MB/s, editor typing, Projects animations | frame hooks; robust under load, so checked at every group gate |
| Frame interval, p95 | = vsync when quiet | ≤ one vsync of the display in use (16.7 ms at 60 Hz, 8.3 ms at 120 Hz) | frame hooks, idle machine only; a 120 Hz run needs a ProMotion display (the spike's display ran at 75 Hz) |
| Key to painted frame, p95 | 14.5 ms at 75 Hz | ≤ one vsync + 4 ms in editor and terminal | synthetic key events through the platform input path |
| Terminal throughput, view visible | 141 MB/s | ≥ 100 MB/s; largest frame gap in a 50 MB flood ≤ 50 ms | scripted child writing an escape-dense 50 MB file |
| Terminal child never waits for the UI | n/a in Rust (Go: 0.2 s for 50 MB) | 50 MB written in ≤ 1 s while a test hook blocks the main thread for 2 s | integration test, every gate |
| Memory, idle, Projects, 30 repositories | 110 MB (empty panes) | ≤ 130 MB | `proc_pidinfo` RSS |
| Memory per terminal, full scrollback (10,000 lines x 120 columns) | about +40 MB | ≤ 40 MB | flood scenario |
| Memory, one 50,000-line file open | about +95 MB | ≤ 80 MB extra | editor scenario |
| Memory, working day (6 AI sessions with full scrollback, 3 files, 30 repositories) | n/a | ≤ 600 MB (language servers are separate processes, limited by plan 4.12) | combined scenario |
| 50,000-line file to first paint | 431 ms (quiet 335 to 363) | ≤ 250 ms: visible lines first, highlighting in the background; editable to 3 MB, read-only above (plan 4.12) | editor scenario |
| Large repository (R7-03: 50,000 files, 100,000 commits) | n/a | quick-open results ≤ 30 ms per keystroke after the background index; first history screen ≤ 200 ms | fixture repository built by `cp-testkit` |
| Binary | 20.2 MB stripped (no services) | ≤ 40 MB stripped; release zip ≤ 25 MB | `dist` output |
| Developer loop (engineering budget) | 2 to 3 s incremental (tiny app) | ≤ 15 s incremental debug after a one-line UI change; ≤ 20 s warm `nextest -p <service crate>` | `cargo xtask perf --build` |

`cargo xtask perf` builds with feature `perf`, drives each scenario on mock services, repeats 5 times and writes `docs/perf/<date>-<commit>.json`. Start time, frame interval and key latency count only when the 1-minute load average is below 2 (the spike's numbers swung threefold under load); CPU per frame and throughput are checked at every group gate. A milestone gate fails on any budget miss; the fix or an owner-approved new number comes before the next milestone.

## 10. Packaging, signing, notarization, installer, updater

The design of `docs/release.md` and plan 4.14 stays. What changes is only what replaced Wails.

**macOS.**
- `cargo xtask bundle` builds `dist` for `aarch64-apple-darwin` and assembles `Codepass.app`: `Contents/MacOS/codepass`; `Info.plist` from `assets/` (BundleID `com.rudian.awsssohub`, both version keys, `LSMinimumSystemVersion` 15.0, `NSAppleEventsUsageDescription` through `en.lproj` and `zh-Hans.lproj`); `icons.icns`.
- `build/release/release.sh` keeps every step: dirty-tree refusal, export of `HEAD`, re-sign with `--force --options runtime --timestamp` (executable, then bundle), the full designated-requirement check, notarize (stop on anything but Accepted, keep the log), staple, `ditto -c -k --keepParent --norsrc --noextattr --noqtn --noacl`, list and verify the extracted copy, sign the manifest, `gh` draft, smoke test, `--publish`. Only two commands change: `wails3 task package` becomes `cargo xtask bundle`, and `go run ./build/release/relkit` becomes `cargo xtask relkit` (same manifest, same domain string; the Go golden manifest and signature are its tests).
- No entitlements by default: compiling Metal shaders at run time needs none. The Apple Events entitlement is added only if check A9 needs it, as today.
- The version is written once in `[workspace.package] version`; the bundle, the Windows resource and `cp-core::version` are generated from it, and `release.sh` checks them (the "four places" rule becomes one place).

**Windows.**
- Built on the GitHub Actions Windows runner (7.5) with `--profile dist --target x86_64-pc-windows-msvc`. The exe carries a version resource (`1.0.0-rc.N` → `1.0.0.N`, the F1-20 contract), the icon and a manifest (per-monitor DPI v2, UTF-8 code page, Common Controls 6). Zipped as `Codepass-<v>-windows-amd64.zip` with only `codepass.exe`. Unsigned (owner decision 3). The manifest is signed on the Mac.
- Toasts need an AppUserModelID. If check W1 shows the portable exe cannot show them, a per-user installer (Inno Setup or NSIS; build tools only, never linked) adds the Start-menu shortcut, and the zip stays the update payload. Planner's call.

**Updater** (`cp-update`; plan 4.14 unchanged).
- Same channel: GitHub releases; `manifest-<os>-<arch>.json` plus `.sig`; ed25519 over `codepass-update-manifest-v1\n` plus the manifest bytes; two pinned public keys compiled in; the test channel is a private repository read with a read-only token kept in the keychain under "Codepass update test channel".
- Same rules: version equals the tag, newer than running, at least the highest ever installed (unreadable store fails closed), os and arch match, plain semver, `rc.N` with a dot; unknown manifest fields refused; release notes only from the manifest, as plain text.
- Same gate, on the staged copy before the swap and again inside the restart: bundle ID; bundle version equals the manifest; `/usr/bin/codesign --verify --deep --strict -R=<the full designated requirement>`; then `/usr/sbin/spctl -a -t exec`, never the only check. Windows: the exe's file version equals the manifest.
- Download: connect, TLS and header timeouts, a no-progress timeout, **no overall timeout**; size and sha256 checked while streaming.
- Extract with the app's own zip reader. Refused: AppleDouble `._*`, `__MACOSX`, absolute paths, `..`, links that leave the bundle.
- **Swap (replaces Wails' `pkg/updater`).** The app copies its executable into the staging folder and starts that copy in helper mode, then quits through the direct path (no quit dialog, as F1-W notes). The helper waits for the app's PID (at most 30 s), copies `Codepass.app` to `Codepass.app.bak`, removes it, renames the staged bundle into place, runs the gate once more, relaunches with `/usr/bin/open -n`, and records the restart in `<data>/update/restarts`. Windows: rename the running exe to `.bak`, move the new one in, start it. The crash window and the manual recovery in `docs/release.md` stay as they are. Every refusal removes the staging folder and leaves the installed app byte-identical; the S0-02 scenarios R1 to R9 and G3 are its tests.
- No Go Codepass was ever shipped and 0.0.x has no updater, so the chain starts with the first Rust release candidate. The key ceremony (owner present) is unchanged and still pending.

**Urgent fixes after the R1 gate** (planner's call after the builder check). From the R1 gate the owner uses the Rust build every day while development goes on. At that gate the branch `release/1.0` is cut from the gate's commit. An urgent fix is made on `release/1.0` first, shipped as the next `rc.N` through the test channel, and merged back into `codepass` the same day; `codepass` itself never ships between gates. The owner keeps AWS SSO Hub 0.0.2 as the fallback until R2-DEL: when the Rust build first starts, the old-app guard's choice is "quit it", not "move it to the Trash".

## 11. Migration plan

### 11.1 Milestones

Run order: **R0, R1, R2, 3, 8, 4, 5, 6, 7**. Sizes: S = 1 point, M = 2, L = 3 (tasks.md). Weeks are plan-weeks of the implement-and-review loop.

| Milestone | Content | Points | Weeks | Gate adds |
|---|---|---|---|---|
| **R0 Foundation** | workspace, toolchain, gate, core crates, UI kit, shell, mock mode, shots, perf harness, packaging, Windows host, risk probes | 18 | 3 | none extra (Chinese input and VoiceOver passed on the spike, section 13) |
| **R1 Port 1** | everything AWS SSO Hub 0.0.2 does, plus milestone 1 | 57 | 7 | milestone 1's gate on the Rust build: shots, test channel rc, checks C1 to C28, A9, A16, A17, B1 to B3, B5, B9, B15 to B19; the Rust build replaces AWS SSO Hub on the owner's Mac |
| **R2 Milestone 2** | port of A2-01 to A2-08, A2-13 and the frame, then A2-W, A2-09 to A2-16, A2-CL; the Go and React code is deleted inside it (11.6) | 40 | 6 | milestone 2's gate (tasks.md), manual group E |
| 3, 8, 4, 5, 6, 7 | as in tasks.md, same tasks, sizes and order | 31, 29, 34, 25, 28, 15 | 5, 8, 7, 5, 4, 3 | as in tasks.md |
| **Total** | | **277** | **48** | |

The Go plan still had 176 points to do (a recount of the sizes in tasks.md; HANDOFF's 254 − 83 gives 171). The re-platform therefore adds about **101 points and about 14 plan-weeks**. Planner's call: milestones 3 to 8 keep their sizes; known deltas cancel out (M5 about −2: GPUI Kit's editor providers and diff view; M7 about +2: Windows high contrast, announcements and a Narrator pass on Windows). Each milestone is re-estimated at its start.

### 11.2 R0 groups

- **A (alone).** R0-01 Workspace, toolchain, gate (S): everything in 4.1 and 4.3; every crate created empty with its owner named; `cargo xtask gate`; the Rust variant of the workflow script. Accept: the gate passes on the empty workspace; build times recorded.
- **B (parallel).** R0-02 `cp-core` (M). R0-03 `cp-store` and the `settings.json` model with the 0.0.1 and 0.0.2 fixtures (M). R0-04 `cp-i18n` with the conversion of the 21 namespaces (M). R0-05 `cp-ui-kit` (L). R0-06 `cp-platform` base: single instance, IPC, opener, reveal, process table, Time Machine exclusion (M).
- **C (alone).** R0-07 App shell and wiring (L): the start order, `cp-app` traits, the window and shell, menus, close and quit hooks, key contexts, mock mode with the ported world, shots and `shotdiff`, perf harness.
- **D (parallel).** R0-08 Build and package (M): `xtask bundle`, `xtask relkit`, `release.sh --dry-run` on Rust, the GitHub Actions Windows workflow, `winbuild`, `wincheck`, worktree target seeding. R0-09 Risk probes (S): (Chinese input and VoiceOver already passed on the spike, 2026-11-17) a 120 Hz run if a ProMotion display is at hand; gpui's notification API (permission state, a click that launches the app); precompiled versus run-time shaders at start.

### 11.3 R1 groups

- **A (parallel).** R1-01 `cp-files`: INI editor, backups, atomic writer, text diff (M). R1-02 `cp-exec`: argv runner, GUI PATH, stub rule, shell guard with FU-SGa to FU-SGc, terminal-app launcher (M). R1-03 `cp-net` (M). R1-04 `cp-watch` (M). R1-05 `cp-git` read side: runners, remote parsing, credential matching, config, status, scan (L).
- **B (parallel).** R1-06 `cp-aws` (L). R1-07 `cp-git` write side: bind, unbind, helpers, global guard and previews, cached passwords, connection tests (L). R1-08 `cp-update` with `cp-platform::keychain` (L). R1-09 `cp-tools`: detection and installers (L). R1-10 `cp-repos::projects`: `projects.json`, registry, the one-time import of old test results (M; as F1-02 and F1-18 came before F1-11 and F1-13).
- **C (parallel).** R1-11 `cp-accounts` engine with the 0.0.2 sessions, isolated browser and console (L). R1-12 `cp-repos` engine and background fetch (L). R1-13 author identity (M). R1-14 `cp-notify` with its backend (M). R1-15 migration wizard, export and import, old-app guard (M). A task that needs a sibling's service in the same group codes against a trait and R1-W connects them (accounts → notify).
- **D (alone).** R1-W Wiring (S).
- **E (parallel, UI).** R1-16 Accounts page (L). R1-17 Sign-in and portal dialogs, and the menu-bar and tray item (M). R1-18 Projects page with clone, bind and search, every width from 980 (L). R1-19 Setup and Health (M). R1-20 Git settings and identity (L). R1-21 Settings, Update, notification rows, restore (L). R1-22 Migration wizard, onboarding, tutorial (M).
- **F (alone).** R1-CL Clean-up and gate fixes (S).

### 11.4 R2 groups

- **A (alone).** R2-01 `cp-pty` (M).
- **B (parallel).** R2-02 `cp-term` sessions (L). R2-03 keep-awake (M). R2-04 AI tools: detect, install, launch, naming (M). R2-05 task-state signals (L). R2-06 process tracker and force stop (M). R2-07 workspace frame (L). R2-08 terminal view I: paint, keys, mouse reporting, resize, IME (L).
- **C (parallel).** R2-09 task lifecycle (L). R2-10 terminal view II: selection and copy, links, search, paste guard, screen-reader text (M).
- **D (alone).** R2-W Wiring (S; the Go A2-W).
- **E (alone).** R2-DEL Delete the Go and React code (S; 11.6).
- **F (parallel).** A2-09 (L), A2-10 (M), A2-11 (M), A2-12 (S), A2-14 (S), A2-15 (M), as in tasks.md.
- **G (alone).** A2-16 (S). **H (alone).** A2-CL (S).

**Rules for parallel Rust tasks.** Tasks in one group own disjoint crates, or disjoint module paths of one crate. The wiring task alone owns the shared files: root `Cargo.toml`, `crates/codepass/src/main.rs`, `crates/cp-app/src/services.rs`, `crates/cp-ui/src/shell/{sidebar,routes,menu,commands}.rs`, `crates/cp-mock/src/lib.rs`, `deny.toml`, `THIRD_PARTY_NOTICES.md`. Locale files are one file per namespace, found by `cp-i18n`'s build script, so no task edits a shared registry. A task that needs a new third-party crate adds it to its own `Cargo.toml` with an exact version and `cargo deny` passing; the wiring task moves it to `[workspace.dependencies]`.

### 11.5 Every finished Go task and the Rust task that redoes it

| Go task (merged) | Redone by |
|---|---|
| S0-01 to S0-07 spikes | not redone; their findings are carried: S0-01 → R2-02, R2-08; S0-02 → R1-08; S0-03 → R2-05; S0-04 → R1-09; S0-05 → R0-08; S0-06 → M5; S0-07 → M6 |
| F1-01 Rename | R0-01 (names), R0-08 (bundle, icon, arm64, minimum 15.0) |
| F1-02 Versioned data files | R0-03 (store); R1-10 (`projects.json`) |
| F1-03 Menu, window, old-app handoff | R0-07 (menu, window, quit hooks, helper mode first); R1-15 (old-app guard) |
| F1-04 Read-only git layer | R1-05 |
| F1-05 File watcher | R1-04 |
| F1-06 Network and proxy | R1-03 |
| F1-07 Scrub and generic fixtures | R0-01 (scrub in the gate); R0-07 (the generic world) |
| F1-08 Motion and style | R0-05 |
| F1-09 Update verification; F1-20 Windows update gate | R1-08 |
| F1-10 Release pipeline | R0-08 (and `release.sh` changes in 10) |
| F1-18 Stable IDs; F1-11 Repository engine | R1-10 (registry); R1-12 (engine); R1-07 (binding) |
| F1-12 Tools gate and installers | R1-09 |
| F1-13 Author identity | R1-13 |
| F1-W Wiring | R1-W |
| F1-14 Projects page; F1-19 Clone with a folder name; F1-GF2 rows fit the window | R1-18 (with R1-12 for the engine side) |
| F1-15 Setup and Health | R1-19 |
| F1-16 Git settings and identity UI | R1-20 |
| F1-17 Update UI | R1-21 |
| F1-21 AWS SSO Hub 0.0.2 carry-over | R1-06 (console link, sign-in end from AWS), R1-11 (isolated browser, sessions), R1-16 (no session-length control), R0-03 (0.0.2 fixture) |
| F1-22 Notifications and sign-in notices | R1-14 (service, kinds, notices); R1-21 (Settings rows) |
| F1-CL, F1-GF clean-up and gate fixes | each follow-up goes to its package's port task; the gate-fixed look is the spec for R1-16 to R1-22 |
| F1-SG Tests never run the owner's shell | R1-02 (guard); R0-01 (trap run in the gate) |
| A2-01 PTY and process control | R2-01 |
| A2-02 Terminal sessions | R2-02 |
| A2-03 Keep-awake | R2-03 |
| A2-04 AI tools | R2-04 |
| A2-05 Task state signals | R2-05 |
| A2-13 Process tracker | R2-06 |
| A2-06 Workspace frame | R2-07 |
| A2-07 Terminal component | R2-08, R2-10 |
| A2-08 Task lifecycle | R2-09 |
| AWS SSO Hub 0.0.x features with no task ID (accounts, SSO sign-in, profiles, CodeCommit binding, git global guard, migration wizard, export and import, backups and restore, menu-bar and tray item, onboarding, tutorial, Health, open in terminal) | R1-02, R1-06, R1-07, R1-11, R1-15 to R1-17, R1-19 to R1-22 |

### 11.6 When and how the Go and React code is deleted

**When:** task R2-DEL, right after R2-W passes the gate, and only when (1) the R1 gate has passed and the owner has used the Rust build daily for at least a week; (2) `cargo xtask spec-map --check` shows every Go test of every package ported or dropped with a reason, and the frontend logic tests likewise; (3) the shots of every screen built so far are approved. Until then the Go code is frozen: no Go task runs, the Go gate is not part of the Rust gate, nobody edits it.

**How:** one commit, "remove the Go and React implementation", that deletes `main.go`, `deps.go`, `servermode_guard.go`, `go.mod`, `go.sum`, `internal/`, `frontend/`, `Taskfile.yml`, `build/config.yml`, `build/*/Taskfile.yml`, the Wails parts of `build/windows/` (`nsis`, `msix`, `wails.exe.manifest`, `info.json`), `build/release/relkit/`, `tools/go-oracle/`, `scripts/shell-trap-test.sh` and `docs/plan/codepass/orchestration/gate.sh`. It keeps `docs/` (the HTML mockup and `shot.sh`/`shoturl.sh` stay as design tools). Before it, `docs/plan/codepass/rust/go-reference.md` records the last commit that has the Go code, so it can be read from git history; HANDOFF, README build lines, SECURITY scope and THIRD_PARTY_NOTICES are updated in the same task.

### 11.7 The later milestones

Milestones 3, 8, 4, 5, 6 and 7 keep their task IDs, sizes, acceptance and order. When `tasks.md` is re-laid (the planner's next step after approval), their Owns lines change from Go and React paths to crates and modules (for example C3-02 owns `crates/cp-git/src/diff/`; E5-03 builds `cp-lsp` with the message-log design of 5.2e; E5-04 builds on GPUI Kit's editor in `cp-editor`; the wiring tasks own the shared files of 11.4). Each milestone still ends with the standard gate, its shots, a test-channel build and a Windows smoke run.

## 12. Risks

| Risk | Mitigation |
|---|---|
| GPUI Kit and gpui-pre: breaking 0.x releases every one to three weeks, both published by one person; Zed's own `gpui` crate is 13 months old | Exact pins; upgrades only in planned tasks (section 8); `cp-ui-kit` and `cp-term-view` hold most Kit calls; full source available under Apache-2.0, so a stop means vendoring into `third_party/`, not a rewrite |
| Screen readers beyond a first VoiceOver run are unproven (spike b); Chinese input and VoiceOver passed the owner's own test on the spike (2026-11-17) | Fallback in section 13, item 3; terminal IME built in R2-08; `announce` service (UI map 3); accessibility pass in M7 |
| The terminal is hand-built (selection, links, search, IME, mouse, screen-reader text) | Two tasks (R2-08, R2-10) and acceptance copied from A2-07 and design 5.6; `alacritty_terminal` for emulation; the fair-lock design of 4.4 |
| The editor: GPUI Kit's editor maturity for split view, multiple cursors, 3 MB files, peek, blame | A probe at the start of M5 before E5-04; the remaining features built in `cp-editor`; never Zed's GPL editor |
| Windows needs a Windows machine to build (`fxc.exe`) | GitHub Actions runner (section 13, item 1); `wincheck` per group; Windows build and smoke per milestone |
| Behaviour regressions against Go and its reviewed edge cases | Spec map, fixtures byte for byte, differential tests, wiring notes as acceptance (7.2) |
| Data-format drift (Go writes `null` for empty lists; Go time format; field names; unknown fields) | Golden fixtures from 0.0.1, 0.0.2 and Go Codepass; the 0.0.1 binary-compatibility check of F1-02 repeated on Rust-written folders |
| Memory above the old 150 MB idle target once content loads (spike 204 to 243 MB) | Section 9 budgets per part; scrollback and editor caps; perf at every milestone gate |
| Compile times and disk with parallel worktrees (4.9 GB per target) | Service crates without gpui; APFS-cloned targets or sccache; 25 GB free before a group, at most 3 builds at once |
| AWS SDK for Rust: heavy, frequent releases | Only three small service crates, default features off (5.2a); behind `cp-aws`; exact pins |
| GPL contamination by copying Zed code | Rule in section 1; `cargo deny`; reviewers check new files that look ported |
| Old app and Codepass running at once on one data folder | Same single-instance lock as Wails plus the old-app guard (5.2h) |
| Supply chain: a weekly upstream from a single crates.io owner | `cargo deny` advisories and sources; upgrade diffs read before merging (section 8) |
| Agents slower in Rust (compile-error loops, longer reviews) | Smaller crates, service tests without gpui, the gate's fast steps first; estimates re-checked after R0 |

## 13. Owner decisions (2026-11-17)

The three questions of the draft and the builder check's questions, as answered or settled:

1. **Windows builds and tests: GitHub Actions** (`windows-latest`), chosen by the owner over a Windows VM on the Mac (7.5).
2. **Apple's Metal toolchain:** settled in R0-09 with the measured start-time gain. The planner's recommendation stands: install it only if the start is at least 50 ms faster. Until then shaders compile at start.
3. **Chinese input:** the owner typed Chinese with the system input method in the spike's single-line field, multi-line field and code editor: it works (2026-11-17). **VoiceOver** read the spike's buttons and fields (owner, 2026-11-17); the full screen-reader pass stays in milestone 7, and Narrator on Windows is tested there. If anything in gpui's input or accessibility fails later: planner's default is a fix in `third_party/` (up to two weeks); the owner is asked only if that is not enough.
4. **Disk:** the owner agreed to clear rebuildable caches; 25 GB freed, 54 GB free (4.3).
5. **UI map question 1 (scale and tilt):** option A, drop them and keep fade, movement and shadow (designer's recommendation; the owner was told and did not object). UI map question 2: the menu-bar and tray item exists today, so it stays on both platforms (5.1, row `app`).
6. **Urgent fixes once the owner uses the Rust build daily:** a `release/1.0` branch from the R1 gate (section 10, last paragraph; planner's call).

## 14. Left to the builder

- Exact crate versions at R0's start (the newest that build with Rust 1.100 and with each other), pinned as in section 8.
- Module layout inside each crate; trait-object form for async service methods (`async-trait` or boxed futures); the IPC message format (one JSON line per message, size-limited).
- The clippy lint set, the dev-profile numbers, the `shotdiff` thresholds, the watchdog limit, tokio worker counts: each tuned by measurement in R0.
- User-level tools on the Mac (no administrator rights, nothing global): `cargo-nextest`, `cargo-deny`, `cargo-about`, `cargo-xwin` if `wincheck` works, `sccache` if needed.
- The Rust variant of `codepass-group-v2.js` (gate string, worktree setup without `node_modules`, `Cargo.lock` merge rule of 4.3).
- Everything UI map 2 to 8 leaves open at the component level. Its questions are settled (section 13, item 5).
- Not left to the builder: anything a requirement, the design, a file format or a safety rule fixes. When code and this document disagree, the plan, the design and the Go tests win, and the builder asks the planner.
