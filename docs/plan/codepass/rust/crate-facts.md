# Rust crate facts for the Go → Rust + gpui port

**Checked: 2026-11-17.** I read the facts from the crates.io JSON API (`/api/v1/crates/<name>`, `/owners`, `/dependencies`): latest stable version, licence of that version, its release date, MSRV (`rust_version`), downloads and repository. I also read crate sources by streaming the published `.crate` tarballs to stdout (nothing was installed or built), and I used the GitHub REST API (last push, releases), docs.rs, official docs, and web search. Nothing was installed. No cargo command was run.

Each row gives: crate | latest version | licence | latest-release date | maintained? | Windows + macOS | note.
"Maintained" is my judgement, based on recent releases and pushes. Text marked *(inference)* is my reading of the code or docs, not a statement by the project.

---

## 0. Surprises (summary)

1. **The official `gpui` crate is stale.** Its latest version is 0.2.2 from 2025-10-22, about 13 months ago. Current GPUI is available only as **`gpui-pre`**, a weekly snapshot of Zed `main`. One person, crates.io user `huacnlee` (the GPUI Kit maintainer at Longbridge), publishes it, not Zed Industries. **GPUI Kit 0.10.0 pins `gpui-pre =0.3.12`, not 0.3.13.**
2. **Upstream gpui (gpui-pre 0.3.13) now has AccessKit accessibility and a built-in `SystemNotification` API with action buttons.** On macOS it uses UNUserNotificationCenter. On Windows it uses WinRT toasts. Neither feature is in the official crate 0.2.2.
3. **A Windows release build of gpui needs `fxc.exe` on a Windows build host.** The shader-compile step in `build.rs` is gated on the *host* OS, so a release cross-build from macOS fails: `include!(.../shaders_bytes.rs)` finds no file. I found no documented way to cross-compile. Build Windows on a Windows CI runner.
4. **Licence flags:**
   - `user-notify` is **LGPL-3.0-or-later**.
   - `ini-merge` is **LGPL-3.0-only**.
   - `configparser` and `ini` are MIT **or** LGPL-3.0; choose MIT.
   - `apple-codesign` is **MPL-2.0** and has had no release since 2024-11.
   - `notify` is **CC0-1.0**.
   - The AWS default TLS stack pulls in `aws-lc-sys`, whose licence combines ISC, Apache-2.0, MIT, BSD-3-Clause and MIT-0.
   - **WiX v6+ requires the Open Source Maintenance Fee** from organisations with more than $10k annual revenue, and **v7 enforces EULA acceptance**.
   - **Inno Setup asks commercial users to buy a licence** (since 6.5.0, 2025-08).
   - **cargo-xwin makes you accept Microsoft's CRT/SDK licence.**
   - **I found no GPL or AGPL crate in this list.** All `gpui-pre-*` crates are Apache-2.0 and match Zed's per-crate `Cargo.toml`.
5. **Abandoned or superseded crates:**
   - `single-instance`: no release since 2021.
   - `tower-lsp`: no release since 2023. Use the fork `tower-lsp-server`.
   - `lsp-types`: last release 2024-06, repo idle. GPUI Kit still uses it.
   - `winrt-notification`: no release since 2022. Use `tauri-winrt-notification`.
   - `command-group`: superseded by `process-wrap`.
   - `conpty`: no release since 2024.
   - `cargo-msix` and `msix`: stale.
   - **`portable-pty` has had no crates.io release since 2025-02.** It is still worked on inside the wezterm repo, but those fixes are unreleased.
   - `ropey` 2.0 has been in beta since 2025-08. GPUI Kit pins `=2.0.0-beta.1`.
6. **The Microsoft `windows` and `windows-sys` crates are frozen at 0.62.2 / 0.61.2 (2025-10).** windows-rs moved its other crates to 0.100.0 (MSRV 1.95) on 2026-09-03. It deliberately held back the two umbrella crates.
7. **AWS SDK MSRV is 1.94.1.** A plain `aws-sdk-ssooidc = "1"` compiles **two HTTP stacks**: legacy hyper 0.14 with rustls 0.21 (ring), and hyper 1 with rustls 0.23 (aws-lc). The cause is the default `rustls` feature. Use `default-features = false, features = ["default-https-client", "rt-tokio"]`.
8. **No Windows support:**
   - `pty-process`: Unix only.
   - `nix`: Unix only.
   - `libproc`: macOS and Linux only.
   - `rustix`: on Windows, only its `net` module works.
   - `security-framework`, `objc2*`, `mac-notification-sys`: Apple only.
9. **Rust 1.100.0 is out** (2026-11-12). The highest MSRV in this list is 1.95 (`windows-core` 0.100, `sysinfo` 0.40, `gpui-ce`).
10. **No mature comment-preserving INI editor exists for `~/.aws/config`.**
    - For `~/.gitconfig`, use `gix-config`. It does lossless round-trips.
    - For `~/.aws/config`, the only options are tiny new crates (`ini-preserve`, `ini-edit`), the parser-only `ini-roundtrip`, or an in-house editor. `rust-ini` discards comments.

---

## 1. UI (gpui ecosystem)

| Crate | Latest | Licence | Released | Maintained? | Win + macOS | Note |
|---|---|---|---|---|---|---|
| gpui (official, Zed) | 0.2.2 | Apache-2.0 | 2025-10-22 | **Stale on crates.io.** 7 versions, none since 2025-10. Zed `main` is very active. Owners are 3 Zed staff and the team `github:zed-industries:crates-io`. The crate was set to trusted-publishing-only (metadata updated 2026-10-30), but no new version followed. | yes / yes (Metal; Windows DirectX 11) | Predates Zed's split into `gpui` + `gpui_platform` + per-OS crates. Has no AccessKit and no SystemNotification. Third-party libraries built on it (e.g. gpui-terminal) do not mix with gpui-pre. |
| gpui-pre | **0.3.13** | Apache-2.0 | **2026-11-16** (00:31 UTC) | Yes. A weekly snapshot of Zed `main`, published Mondays around 00:30 UTC. 0.3.0 came out 2026-09-03; 0.3.13 is `zed@125ca86`. **Owner: crates.io user `huacnlee`** (GPUI Kit maintainer, Longbridge), not Zed. The repository field points to zed-industries/zed. | yes / yes, through gpui-pre-platform with gpui-pre-macos / -windows / -linux (+ -web, -wgpu, also 0.3.13) | The library name is `gpui`, so depend on it as `gpui = { package = "gpui-pre", version = "=0.3.x" }`. The docs warn of frequent breaking changes. It includes AccessKit 0.24 and `SystemNotification`. If the cadence holds, 0.3.14 should arrive around 2026-11-23. |
| gpui-pre-windows | 0.3.13 | Apache-2.0 | 2026-11-16 | weekly | Windows only | This is Zed's `gpui_windows` crate. It uses `windows` 0.62, accesskit_windows 0.34 and WinRT toasts. Its `build.rs` needs `fxc.exe` for release builds (see §17). |
| gpui-kit | 0.10.0 | Apache-2.0 (docs prose also CC BY 4.0) | 2026-11-16 | Very active: 16 versions since 2026-09-03, repo pushed 2026-11-17, about 20.9k stars. | yes / yes (+ Linux, wasm32) | Umbrella crate that re-exports GPUI, gpui-base, gpui-component and assets. It **pins `gpui-pre =0.3.12`**. The optional `gpui-fast` feature switches to Longbridge's fork (`gpui-fast` 0.1.17, Apache-2.0). |
| gpui-component | 0.10.0 | Apache-2.0 | 2026-11-16 | Active. 39 versions since 2025-02. | yes / yes | The styled layer. Depends on `lsp-types ^0.97`, `ropey =2.0.0-beta.1` (exact pin), `windows ^0.58` (older than gpui's 0.62, so two `windows` versions get built), `rust-i18n 4`, `notify 8`, `similar ^2.7`. Optional: `tree-sitter 0.26` with about 40 grammars, or `syntect`. |
| gpui-base | 0.10.0 | Apache-2.0 | 2026-11-16 | Active | yes / yes | Unstyled behaviour layer: input and editor core, IME, LSP provider traits, AccessKit wiring. |
| gpui-ce | 0.3.0 | Apache-2.0 | 2026-10-20 | Community fork (owner `philocalyst`). Repo pushed 2026-11-14, about 1.6k stars. | yes / yes | Alternative published fork. MSRV 1.95, includes accesskit 0.24. Not what GPUI Kit uses. |
| gpui-terminal | 0.1.0 | MIT OR Apache-2.0 (repo LICENSE: Apache-2.0) | 2025-12-24 | One release only. Repo pushed 2026-07-30. | via portable-pty 0.9 + alacritty_terminal 0.25.1 | **Targets the official `gpui ^0.2.2`**, so it is incompatible with gpui-pre / GPUI Kit. |
| gpui_xterm | 0.3.0 | MIT | 2026-11-10 | Active but tiny: 5 stars, about 190 downloads. | uses alacritty_terminal 0.25.1 (its own tty/ConPTY; no portable-pty) | **Targets `gpui-kit ^0.9.0`** (gpui-pre 0.3.12). It will not build against gpui-kit 0.10 until updated. |

### GPUI Kit (github.com/longbridge/gpui-kit; formerly gpui-component)

**README claims (main branch, read 2026-11-17):**
- 75+ components and primitives.
- WebAssembly support.
- **AccessKit accessibility:** "roles, names, states, relationships, and actions are built into the interaction layer and covered by tests".
- Headless UI integration tests.
- Data tables with virtual scrolling across hundreds of thousands of rows.
- **Virtual lists**, including items of different sizes.
- **Code editor:** "Stable performance at 200K lines with Tree-sitter highlighting and LSP diagnostics, completion, and hover".
- Dock layout.
- Markdown, HTML and charts.
- Optional JavaScript extension runtime (`gpui-shell`).
- Production use in Longbridge Pro.

**IME:** `docs/ARCHITECTURE.md` lists "cursor, selection, IME, clipboard, and focus" as part of the base input module. GPUI supplies platform IME through its input handler.

**LSP:** the editor exposes *provider traits*: `CompletionProvider`, `HoverProvider`, `DefinitionProvider`, `CodeActionProvider`, plus diagnostics, semantic tokens and document colours, in `gpui-base/src/input/editor/lsp/`. They use `lsp-types` 0.97. **No LSP client or transport is included**; the application supplies it.

**Highlighting:** tree-sitter grammars are opt-in features (`tree-sitter-<lang>`). Since 0.8.0 a lighter `syntect` backend is also available.

**Requirements** (from the gpui-kit.com installation page):
- Windows: Windows 10+, VS 2022 Build Tools with MSVC and a Windows SDK, the MSVC toolchain (not GNU), and **CMake on PATH**.
- macOS: the page lists **"macOS 15 or later"** plus the Xcode CLT. It is not clear whether this is a runtime or a development minimum.
- Rust: **1.92 or later**.
- The page says nothing about fxc or cross-compiling.

**Licence:** Apache-2.0 for code. Docs prose is also CC BY 4.0. The README states the `gpui-pre-*` crates are Zed snapshots "published with Zed's license and notices intact."

**Breaking-release cadence:** a new 0.x minor (breaking under Cargo semver) every 2–3 weeks:
- 0.6.0 on 09-03
- 0.7.0 on 09-28
- 0.8.0 on 10-19
- 0.9.0 on 11-02
- 0.10.0 on 11-16

Patch releases come roughly weekly and follow the gpui-pre snapshot (0.7.3 moved to gpui-pre 0.3.9, 0.9.0 to 0.3.11). 0.6.5 and 0.7.2 were yanked. Release notes for 0.8.0, 0.9.0 and 0.10.0 each mention breaking changes.

**Roadmap and future:**
- The README mentions sibling project Native Shell, whose GPUI desktop backend "is in development and will be built on GPUI Kit".
- The getting-started page tells you to keep the gpui-pre versions aligned, and says the default backend is "the pinned upstream GPUI snapshot".
- **I found no statement from Zed about publishing the split gpui crates.** gpui.rs only says "For the near future, GPUI is tied to Zed".
- A third-party blog (2026-10-01) notes that crates.io gpui is about 11 months behind `main`, and that the split crates (`gpui_platform`, `gpui_web`, …) are not on crates.io.

---

## 2. Terminal

| Crate | Latest | Licence | Released | Maintained? | Win + macOS | Note |
|---|---|---|---|---|---|---|
| alacritty_terminal | 0.26.0 | Apache-2.0 | 2026-04-06 | Yes (Alacritty project) | yes (ConPTY through windows-sys/miow) / yes (rustix-openpty) | **Yes, it exposes `pub mod sync` with `FairMutex<T>`** (`new`, `lease`, `lock`, `lock_unfair`, `try_lock_unfair`). Has its own `tty` module. MSRV 1.85. Both gpui terminal crates still use 0.25.1. |
| portable-pty (wezterm) | 0.9.0 | MIT | 2025-02-11 | **Code is maintained but unreleased.** 2026 commits in wezterm `pty/`: winapi→windows-sys (2026-08-25), Windows `kill()` fix (2026-06-07), typo fix (2026-09-12). None of these is on crates.io; 0.9.0 still uses `winapi`. | yes / yes | **ConPTY: yes.** Prefers a side-loaded `conpty.dll` + OpenConsole.exe if one is present, otherwise kernel32 `CreatePseudoConsole`. **Job Objects: no. `PROC_THREAD_ATTRIBUTE_JOB_LIST`: no.** It sets only `PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE` (constant 0x20016). To kill a process tree you must add job handling yourself (fork, or assign after spawn). |
| pty-process | 0.5.4 | MIT | 2026-10-19 | Yes | **Windows: no** (Unix only, rustix) / macOS: yes | Wraps std or tokio `Command` with a pty. |
| conpty | 0.7.0 | MIT | 2024-09-23 | Stale (repo idle since 2024-09) | Windows only | Thin ConPTY wrapper. |
| winpty-rs (alternative) | 1.0.8 | MIT OR Apache-2.0 | 2026-10-28 | Yes (pywinpty) | Windows only | ConPTY plus legacy WinPTY backends. |

---

## 3. AWS SDK

| Crate | Latest | Licence | Released | Maintained? | Win + macOS | Note |
|---|---|---|---|---|---|---|
| aws-config | 1.13.2 | Apache-2.0 | 2026-11-10 | Yes (frequent releases) | yes / yes | **MSRV 1.94.1.** Default features: `default-https-client`, `rt-tokio`, `credentials-process`, `sso`. It uses the SDK crates internally with `default-features=false`. The `sso` feature's `SsoTokenProvider` *loads* `~/.aws/sso/cache/<hash>.json` and refreshes it near expiry; **it does not perform the interactive login**. New optional feature `credentials-login` (uses `aws-sdk-signin`, created 2025-11-19) supports `aws login` console credentials. |
| aws-sdk-sso | 1.121.0 | Apache-2.0 | 2026-11-10 | Yes | yes / yes | MSRV 1.94.1. Operations: GetRoleCredentials, ListAccounts, ListAccountRoles, Logout. Crate size about 97 KiB. |
| aws-sdk-ssooidc | 1.123.0 | Apache-2.0 | 2026-11-10 | Yes | yes / yes | Operations: `create_token`, `create_token_with_iam`, `register_client`, `start_device_authorization`. **CreateToken supports `authorization_code` (with `code`, `redirect_uri`, `code_verifier`, i.e. PKCE), `device_code` and `refresh_token` grants.** RegisterClient takes `grant_types`, `redirect_uris`, `issuer_url` and `scopes`. Both the device flow and auth-code + PKCE are possible. |
| aws-sdk-sts | 1.126.0 | Apache-2.0 | 2026-11-10 | Yes | yes / yes | MSRV 1.94.1. Default features also include `sigv4a`. Crate size about 212 KiB. |
| aws-smithy-runtime | 1.16.4 | Apache-2.0 | 2026-11-16 | Yes | yes / yes | `default-https-client` uses aws-smithy-http-client 1.5.3: hyper 1.x, rustls 0.23 with aws-lc-rs, rustls-native-certs roots, and hyper-util `client-proxy`. `tls-rustls` is the legacy stack: hyper 0.14, rustls 0.21, ring. |

**Default TLS and HTTP:** the AWS docs say "By default, the SDK for Rust uses an HTTPS client based on `hyper`, `rustls`, and `aws-lc-rs`". That means hyper 1.x. Alternatives through `aws-smithy-http-client`: `s2n-tls`, `rustls-aws-lc-fips`, `rustls-ring`. aws-lc-rs needs a C compiler, and on some platforms CMake.

**Gotcha:** each service crate's `default` feature set is `["rustls", "default-https-client", "rt-tokio"]`. Its `rustls` feature enables the legacy hyper 0.14 / rustls 0.21 stack, so default features compile both stacks.

**Proxy** *(inference)*: the default client enables hyper-util `client-proxy` but not `client-proxy-system`, so OS proxy settings are probably not read.

**Weight:**
- Direct dependencies: aws-config 29 (18 non-optional), aws-sdk-ssooidc 15, aws-smithy-runtime 18.
- `aws-lc-sys` is a 9.3 MiB crate of C code. It dominates the cold build.
- I found no measured compile time; lib.rs blocked automated access. Users generally call the SDK dependency tree heavy (r/rust thread on S3 crates, 2025-03).
- sso, ssooidc and sts are small services, so the cost is mostly the shared runtime and TLS.

---

## 4. HTTP and async

| Crate | Latest | Licence | Released | Maintained? | Win + macOS | Note |
|---|---|---|---|---|---|---|
| tokio | 1.54.1 | MIT | 2026-10-21 | Yes | yes / yes | MSRV 1.85. LTS lines: **1.51.x until Mar 2027** and **1.53.x until Sep 2027** (MSRV 1.71). |
| reqwest | 0.13.5 | MIT OR Apache-2.0 | 2026-09-08 | Yes | yes / yes | Details below. MSRV 1.85. |
| ureq | 3.5.2 | MIT OR Apache-2.0 | 2026-11-12 | Yes | yes / yes | Blocking client. Default `rustls` uses ring and webpki-roots (CDLA-Permissive-2.0); `platform-verifier` is optional. `win-system-proxy` reads the Windows registry. **No macOS system proxy.** |
| hyper | 1.12.0 | MIT | 2026-10-06 | Yes | yes / yes | MSRV 1.63. hyper-util 0.1.21 (MIT) contains the proxy matcher. |
| rustls | 0.23.46 | Apache-2.0 OR ISC OR MIT | 2026-10-14 | Yes. **0.24.0-dev.3 pre-releases appeared 2026-11-12, so a breaking 0.24 is coming.** | yes / yes | Default provider is aws-lc-rs; ring is optional. |
| rustls-platform-verifier | 0.7.1 | MIT OR Apache-2.0 | 2026-09-24 | Yes | yes / yes | Uses the OS verifier (Security.framework, CryptoAPI). Trusts enterprise or MITM-proxy root CAs installed in the OS. reqwest 0.13 uses it by default. The AWS SDK uses `rustls-native-certs` instead. |

**reqwest 0.13 TLS:** 0.13 made rustls the default TLS, with aws-lc-rs as provider and rustls-platform-verifier for certificate checks.
- Features: `rustls` (default through `default-tls`), `rustls-no-provider`, `native-tls`, `native-tls-vendored`, `native-tls-no-alpn`.
- **`rustls-tls*` was renamed or removed in 0.13.**

**reqwest system proxy: yes.** The default feature `system-proxy` maps to hyper-util `client-proxy-system`. It reads env vars first, then OS settings.
- **macOS proxy auto-detection since v0.11.21.** The `macos-system-configuration` feature arrived in v0.12.0.
- **Windows registry proxy since v0.10.** `ProxyOverride` fixes landed in 0.12.13 and 0.12.15.
- No PAC/WPAD is documented.

---

## 5. Keychain, secrets and platform bindings

| Crate | Latest | Licence | Released | Maintained? | Win + macOS | Note |
|---|---|---|---|---|---|---|
| keyring | **4.2.0** (v4) | MIT OR Apache-2.0 | 2026-08-29 | Yes. 4.0.0 shipped 2026-04-26; v3 ended at 3.6.3 (2025-07-27). | yes / yes | Details below. MSRV 1.88. |
| keyring-core | 1.0.1 | MIT OR Apache-2.0 | 2026-10-13 | Yes | API only | The API crate for v4. |
| apple-native-keyring-store | 1.0.2 | MIT OR Apache-2.0 | 2026-08-06 | Yes | macOS/iOS | Features: `keychain` (legacy file keychain) and `protected` (Data Protection keychain, macOS 10.15+; some features need provisioning or entitlements). **Calls Security.framework through `security-framework`. It never shells out to `/usr/bin/security`.** |
| windows-native-keyring-store | 1.1.0 | MIT OR Apache-2.0 | 2026-05-24 | Yes | Windows | Credential Manager through windows-sys 0.61. MSRV 1.88. |
| security-framework (kornelski) | 3.7.0 | MIT OR Apache-2.0 | 2026-02-20 | Yes (repo pushed 2026-08) | macOS/iOS only | MSRV 1.85. |
| windows (microsoft/windows-rs) | 0.62.2 | MIT OR Apache-2.0 | 2025-10-06 | windows-rs is active, but **the umbrella crate is frozen** (details below). | Windows | gpui-pre-windows uses 0.62; gpui-component uses 0.58. |
| windows-sys | 0.61.2 | MIT OR Apache-2.0 | 2025-10-06 | Same freeze | Windows | MSRV 1.71. |
| windows-core / windows-registry | 0.100.0 | MIT OR Apache-2.0 | 2026-09-03 | Yes | Windows | MSRV 1.95. gpui still uses 0.62 / 0.6. |
| objc2 | 0.6.5 | MIT | 2026-10-05 | Yes | Apple only | MSRV 1.71. |
| objc2-foundation | 0.3.2 | MIT | 2025-10-04 | Yes (the generated framework crates release less often) | Apple only | Has `NSProcessInfo::beginActivityWithOptions_reason`. |
| objc2-app-kit | 0.3.2 | Zlib OR Apache-2.0 OR MIT | 2025-10-04 | Yes | macOS | |
| objc2-user-notifications | 0.3.2 | Zlib OR Apache-2.0 OR MIT | 2025-10-04 | Yes | Apple | Raw UNUserNotificationCenter bindings. gpui-pre-macos and user-notify use them. |
| block2 | 0.6.2 | MIT | 2025-10-04 | Yes | Apple | Objective-C blocks, needed for completion handlers. |

**keyring v4:** the crate is now a thin wrapper plus CLI sample code. Its default feature `v1` turns on the apple keychain, windows and zbus secret-service stores. **The v3 features `apple-native` and `windows-native` no longer exist.** The maintainers recommend depending on `keyring-core` plus specific store crates.

**windows-rs freeze:** Release 74 (2026-09-03) moved the supported crates to **0.100.0** (MSRV 1.95, edition 2024). It **deliberately held back `windows` and `windows-sys`** while the metadata quality is checked (August 2026 newsletter, issue #4867). Microsoft advises libraries to use the small crates plus `windows-bindgen` instead.

---

## 6. Notifications with action buttons and click callbacks

| Crate | Latest | Licence | Released | Maintained? | Win + macOS | Note |
|---|---|---|---|---|---|---|
| **gpui built-in `SystemNotification`** (gpui-pre ≥ 0.3.13) | 0.3.13 | Apache-2.0 | 2026-11-16 | Weekly | yes / yes | Details below. **Not in official gpui 0.2.2.** |
| mac-notification-sys | 0.6.15 | MIT/Apache-2.0 | 2026-06-16 | Yes | macOS only | Uses the **deprecated NSUserNotificationCenter** with a fake bundle-id trick, so it **works without an app bundle**. Supports a main button, dropdown actions, close button, reply and `wait_for_click`. Responses: `ActionButton` / `CloseButton` / `Click` / `Reply`, delivered in blocking style. Objective-C compiled in `build.rs`. |
| notify-rust | 4.19.0 | MIT OR Apache-2.0 | 2026-11-08 | Yes | yes / yes (Linux/BSD is its primary target) | Details below. MSRV 1.89. |
| user-notify | 0.4.2 | **LGPL-3.0-or-later** | 2026-01-23 | Moderate (repo pushed 2026-05, 14 stars) | yes / yes (+ Linux) | Details below. **The licence is a problem for a statically linked Rust binary.** |
| tauri-winrt-notification | 0.8.1 | MIT OR Apache-2.0 | 2026-07-17 | Yes (Tauri) | Windows only | `add_button(content, action)`, `on_activated(\|action\| …)`, `on_dismissed`, progress. Events are **in-process only**: there is no COM activator, so a click after the app has exited is not delivered. Needs an AUMID; uninstalled apps fall back to `POWERSHELL_APP_ID`. MSRV 1.82. |
| winrt-notification | 0.5.1 | MIT | 2022-01-11 | **Abandoned** | Windows | Use tauri-winrt-notification. |
| windows crate `UI::Notifications` (do it yourself) | 0.62.2 | MIT OR Apache-2.0 | 2025-10-06 | (see §5) | Windows | Details below. |

**gpui built-in `SystemNotification`:**
- **macOS:** UNUserNotificationCenter with UNNotificationCategory/UNNotificationAction buttons, `on_response` callbacks, and a dedicated worker thread. It **disables itself when the app is not running from a bundle**.
- **Windows:** WinRT `ToastNotification` with `<actions>` and an `Activated` handler, in-process. It registers the AUMID under `HKCU\Software\Classes\AppUserModelId` for unpackaged apps.
- See `examples/system_notifications.rs`.

**notify-rust backends:**
- **macOS:** NSUserNotificationCenter by default through mac-notification-sys. A **UNUserNotificationCenter** backend is behind the `preview-macos-un` feature (`mac-usernotifications` 0.3.1, MIT OR Apache-2.0; needs an app bundle). The README says both macOS backends support "actions, close callbacks, `wait_for_response()` and inline replies".
- **Windows:** through tauri-winrt-notification, with actions mapped to `add_button` and the `on_activated` response passed to `wait_for_action`. If no `app_id` is set, it defaults to `POWERSHELL_APP_ID`.

**user-notify:**
- Started as part of Delta Chat's Electron→Tauri port (NGI0-funded).
- macOS uses UNUserNotificationCenter and needs a signed bundle (macOS 10.14+). Action buttons and inline reply work there.
- **Windows: buttons and inline reply are still TODO (issue #2).**

**windows crate `UI::Notifications`:**
- Gives full control: `ToastNotificationManager::CreateToastNotifierWithId(AUMID)` and the `Activated` / `Dismissed` / `Failed` events.
- For **activation when the app is not running**, you need a COM activator (`INotificationActivationCallback`) and a ToastActivatorCLSID on a Start-menu shortcut or in an MSIX manifest. A stub-CLSID shortcut works if you do not need in-process activation (Microsoft Learn).

**Capability summary:**
- **Buttons + click callback on macOS (UNUserNotificationCenter):** gpui built-in, `objc2-user-notifications` (do it yourself), notify-rust with `preview-macos-un`, user-notify (LGPL). All of them need a signed `.app`.
- **Buttons + callback on macOS without a bundle:** mac-notification-sys (deprecated API).
- **Buttons + click callback on Windows:** gpui built-in, tauri-winrt-notification, notify-rust, or the `windows` crate directly. All are in-process only, unless you implement a COM activator with the `windows` crate.

---

## 7. Keep awake and power

| Crate / API | Latest | Licence | Released | Maintained? | Win + macOS | Note |
|---|---|---|---|---|---|---|
| keepawake | 0.6.1 | MIT | 2026-08-20 | Yes (small: 49 stars, repo pushed 2026-09) | yes / yes (+ Linux) | Windows: `SetThreadExecutionState`. macOS: `IOPMAssertionCreateWithName` (PreventUserIdleSystemSleep / DisplaySleep / PreventSystemSleep). Linux: org.freedesktop.ScreenSaver and systemd inhibitor. CLI dependencies sit behind the optional `bin` feature. *(Inference)* The Windows mechanism is thread-scoped, so create and drop the guard on one long-lived thread, not a migrating tokio task. MSRV 1.69. |
| objc2-foundation `NSProcessInfo` | 0.3.2 | MIT | 2025-10-04 | Yes | macOS | Apple's high-level API. Needs features `NSProcessInfo` + `NSString`. Signatures below. |
| nosleep | 0.2.1 | MIT | 2022-11-20 | Stale | yes / yes | Do not use. |

The `NSProcessInfo` signatures are:
- `beginActivityWithOptions_reason(NSActivityOptions, &NSString) -> Retained<ProtocolObject<dyn NSObjectProtocol>>`
- `unsafe endActivity(&ProtocolObject<…>)`

---

## 8. Processes

| Crate | Latest | Licence | Released | Maintained? | Win + macOS | Note |
|---|---|---|---|---|---|---|
| sysinfo | 0.40.4 | MIT | 2026-11-16 | Yes | yes / yes (+ Linux, FreeBSD, Android, iOS) | **MSRV 1.95.** Frequent breaking 0.x releases. |
| libproc | 0.15.0 | MIT | 2026-11-04 | Yes | **Windows: no** / macOS: yes (+ Linux) | `pidpath`, `listpids`, `pidinfo`. |
| nix | 0.32.0 (0.31.4 backport 2026-11-17) | MIT | 2026-11-14 | Yes | **Windows: no** / macOS: yes | Unix only. MSRV 1.69. |
| rustix | 1.1.6 | Apache-2.0 WITH LLVM-exception OR Apache-2.0 OR MIT | 2026-10-27 | Yes | Windows: `net` (Winsock) only / macOS: yes | MSRV 1.65. |
| windows-sys | 0.61.2 | MIT OR Apache-2.0 | 2025-10-06 | Frozen (see §5) | Windows | |
| process-wrap | 10.0.1 (8.2.2 backport the same day) | Apache-2.0 OR MIT | 2026-09-23 | Yes (watchexec) | yes / yes | Wrappers: `JobObject`, `CreationFlags` (Windows); `ProcessGroup`, `ProcessSession`, `ResetSigmask` (Unix); `KillOnDrop`. Frontends: `std` or `tokio1`. JobObject spawns with **CREATE_SUSPENDED → AssignProcessToJobObject → ResumeThread**, not `PROC_THREAD_ATTRIBUTE_JOB_LIST`, and sets KILL_ON_JOB_CLOSE. No PTY support. MSRV 1.87. |
| command-group | 5.0.1 | Apache-2.0 OR MIT | 2023-11-18 | **Superseded.** README: "The successor of command-group is process-wrap. No further work will be done". | yes / yes | Use process-wrap. |

---

## 9. File watching

| Crate | Latest | Licence | Released | Maintained? | Win + macOS | Note |
|---|---|---|---|---|---|---|
| notify | 8.2.2 stable; **9.0.0-rc.9** (2026-11-15) | **CC0-1.0** | 2026-11-08 | Yes | yes (**ReadDirectoryChangesW**) / yes (**FSEvents by default**, `macos_fsevent` through objc2-core-services; kqueue optional) | MSRV 1.88; 9.0-rc needs 1.91. 9.0 adds `tokio` and `futures` features. Plan for an 8→9 migration soon. |
| notify-debouncer-full | 0.7.0; 0.8.0-rc.4 (2026-11-15) | MIT OR Apache-2.0 | 2026-01-23 | Yes | yes / yes | Pairs with notify 8 (0.7) or 9 (0.8). |
| notify-debouncer-mini | 0.7.0; 0.8.0-rc.2 | MIT OR Apache-2.0 | 2025-08-03 | Yes | yes / yes | Simpler debouncer. |

---

## 10. Single instance

| Crate | Latest | Licence | Released | Maintained? | Win + macOS | Note |
|---|---|---|---|---|---|---|
| single-instance | 0.3.3 | MIT | 2021-12-16 | **Abandoned** (repo idle since 2024-07; uses winapi and old nix) | yes (named mutex) / yes (`flock` file) | Detects a second instance only; no argument forwarding. **Do not use.** |
| interprocess | 2.4.8 | 0BSD OR Apache-2.0 | 2026-11-08 | Yes | yes (named pipes) / yes (Unix domain sockets) | Cross-platform "local sockets", optional tokio. **Recommended building block:** the first instance listens; a second instance connects, forwards its args or URL, then exits. MSRV 1.75. |
| tauri-plugin-single-instance (reference only) | 2.5.5 | Apache-2.0 OR MIT | 2026-10-28 | Yes | yes / yes | Tied to Tauri. Windows: `CreateMutexW` plus a hidden window receiving `WM_COPYDATA`. macOS: a Unix socket. Linux: D-Bus. |

---

## 11. Updater and signatures

| Crate | Latest | Licence | Released | Maintained? | Win + macOS | Note |
|---|---|---|---|---|---|---|
| ed25519-dalek | **3.0.0** | BSD-3-Clause | 2026-07-06 | Yes | yes / yes | New major version. MSRV 1.85. Pure Rust. |
| self_update | 1.3.3 | MIT | 2026-10-27 | Yes | yes / yes | Backends: GitHub, GitLab, Gitea, Gitee, S3. Feature `signatures` adds zipsign-api (ed25519-signed zip/tar); `checksums` adds sha2. Uses self-replace, and reqwest 0.13 or ureq 3. MSRV 1.88. |
| self-replace | 1.5.1 | Apache-2.0 | 2026-10-26 | Yes (mitsuhiko) | yes / yes | Replaces or deletes the running executable, including on Windows. |
| velopack | 1.2.161 stable; newest 1.3.39-g8d2844f (2026-10-17) | **MIT** | 2026-09-29 | Yes (repo pushed 2026-11-16, 2.4k stars) | yes / yes (+ Linux) | A full installer and delta-update framework. The Rust crate is the in-app updater. The packaging CLI `vpk` is a **.NET global tool**, so the build machine needs .NET. vpk can sign and notarize on macOS (codesign, notarytool). MSRV 1.75. |
| zip | 9.6.1 | MIT | 2026-11-16 | Yes (zip-rs/zip2) | yes / yes | MSRV 1.88. Major versions come often. |
| sha2 | 0.11.1 | MIT OR Apache-2.0 | 2026-10-12 | Yes | yes / yes | 0.11 belongs to the new digest-0.11 generation; expect 0.10 and 0.11 side by side for a while. MSRV 1.88. |
| minisign-verify (alternative) | 0.3.0 | MIT | 2026-09-25 | Yes | yes / yes | Zero-dependency minisign verification; Tauri's updater uses it. |

---

## 12. Serialization and config (comment-preserving INI)

| Crate | Latest | Licence | Released | Maintained? | Win + macOS | Note |
|---|---|---|---|---|---|---|
| serde | 1.0.229 | MIT OR Apache-2.0 | 2026-07-18 | Yes | yes / yes | |
| serde_json | 1.0.154 | MIT OR Apache-2.0 | 2026-10-11 | Yes | yes / yes | |
| toml | 1.2.1+spec-1.1.0 | MIT OR Apache-2.0 | 2026-11-06 | Yes | yes / yes | Now at 1.x. For comment-preserving TOML edits use **toml_edit 0.25.19**. |
| rust-ini | 0.21.3 | MIT | 2025-08-30 | Slow | yes / yes | **Discards comments.** `parse_comment()` just skips to the end of the line. Not suitable for editing. |
| configparser | 3.3.0 | MIT OR LGPL-3.0-or-later | 2026-09-06 | Yes | yes / yes | Comments are ignored on load and never written back. Choose MIT. |
| ini | 2.0.0 | MIT OR LGPL-3.0-or-later | 2026-09-06 | Yes | yes / yes | A macro over configparser; same limitation. |
| **gix-config** (gitoxide) | 0.62.2 | MIT OR Apache-2.0 | 2026-10-24 | Yes (64M downloads) | yes / yes | **Best choice for `~/.gitconfig`.** Gitoxide's crate-status says it can "write files back for lossless round-trips … keep comments and whitespace, and only change lines that are affected by actual changes". Supports includes and `includeIf`. **Not for `~/.aws/config`**, because `[profile x]` is not git syntax. |
| ini-preserve | 0.1.4 | MIT | 2026-10-07 | New (created 2026-03; one maintainer; about 2.2k downloads) | yes / yes | Format-preserving `load`/`get`/`set`/`remove`/`add_section`/`remove_section`/`save` with atomic save; no dependencies. **The closest fit for `~/.aws/config`, but immature.** Test AWS cases first: nested `s3 =` sub-properties, `[sso-session x]`, `[services x]`. |
| ini-roundtrip | 0.2.1 | MIT | 2025-12-22 | Low activity | yes / yes | Format-preserving *parser* only (raw-line events). You write the editor yourself. Its user `ini-merge` is **LGPL-3.0-only**. |
| ini-edit / edikt-ini | 0.5.0 / 0.4.1 | MIT / MIT OR Apache-2.0 | 2026-10-24 / 2026-09-26 | Very new, about 300 downloads each | yes / yes | Lossless rowan-based CSTs. Too young to rely on. |

**Verdict on INI editing:**
- For `.gitconfig`, use **gix-config**.
- For `~/.aws/config`, **no mature crate exists**. Either vendor and pin `ini-preserve` after testing it, or keep a small in-house line-preserving editor (port of the Go logic).
- aws-config's own profile parser is read-only.

---

## 13. i18n

| Crate | Latest | Licence | Released | Maintained? | Win + macOS | Note |
|---|---|---|---|---|---|---|
| rust-i18n | 4.2.4 | MIT | 2026-09-29 | Yes (Longbridge; gpui-component depends on it) | yes / yes | Details below. **Best fit for zh-CN + en with JSON/YAML keys.** |
| fluent / fluent-bundle | 0.17.0 / 0.16.0 | Apache-2.0 OR MIT | 2025-05-23 / 2025-05-22 | Slow (repo pushed 2026-09) | yes / yes | Uses `.ftl` files only, not JSON or YAML. |
| i18n-embed | 0.16.0 | MIT | 2025-07-09 | Yes (repo pushed 2026-11) | yes / yes | Fluent or gettext backends plus rust-embed. No JSON or YAML. |
| fluent-templates | 0.15.1 | MIT/Apache-2.0 | 2026-08-05 | Yes | yes / yes | Fluent with static loading. |

**rust-i18n:**
- Reads **YAML (default), JSON or TOML** locale files and compiles them in through `t!`.
- Supports fallback chains, and **`zh-CN` falls back to `zh`** (since v2.4).
- Ships a `cargo i18n` extractor.
- The `load-path` feature enables runtime loading.

---

## 14. Logging

| Crate | Latest | Licence | Released | Maintained? | Win + macOS | Note |
|---|---|---|---|---|---|---|
| tracing | 0.1.44 | MIT | 2025-12-18 | Yes (repo pushed 2026-11-12) | yes / yes | |
| tracing-subscriber | 0.3.23 | MIT | 2026-03-13 | Yes | yes / yes | |
| tracing-appender | 0.2.5 | MIT | 2026-04-17 | Yes | yes / yes | Rolling files and a non-blocking writer. |

---

## 15. Text, diff and editor support

| Crate | Latest | Licence | Released | Maintained? | Win + macOS | Note |
|---|---|---|---|---|---|---|
| similar | 3.2.1 | Apache-2.0 | 2026-10-24 | Yes (mitsuhiko) | yes / yes | Major version 3. gpui-component still uses `^2.7`, so two versions get built. |
| imara-diff | 0.2.0 | Apache-2.0 | 2025-06-14 | Moderate (repo pushed 2026-06) | yes / yes | Fast Myers and histogram diffs; gitoxide and Helix use it. |
| ropey | 1.6.1 stable (2023-10-18); 2.0.0-beta.1 (2025-08-02) | MIT | 2023-10-18 | Slow (repo pushed 2026-09; 2.0 has been in beta for 15 months) | yes / yes | **gpui-component pins `=2.0.0-beta.1`.** |
| tree-sitter | 0.27.2 | MIT | 2026-11-07 | Yes | yes / yes | MSRV 1.90. gpui-component's optional dependency is still 0.26. |
| tree-sitter-go | 0.25.0 | MIT | 2025-08-29 | Yes | yes / yes | Grammars depend on `tree-sitter-language ^0.1`, so they work with 0.27. They build C code with `cc`. |
| tree-sitter-typescript | 0.23.2 | MIT | 2024-11-11 | Grammar is stable; releases are rare | yes / yes | |
| tree-sitter-php | 0.25.1 | MIT | 2026-10-06 | Yes | yes / yes | |
| lsp-types | 0.97.0 | MIT | 2024-06-04 | **Stale** (repo idle since 2024-07) | yes / yes | GPUI Kit's editor providers use 0.97 types, so you are tied to it. |
| async-lsp | 0.2.4 | MIT OR Apache-2.0 | 2026-04-24 | Yes | yes / yes | **Works as an LSP client as well as a server** (tower-based; "logically symmetric"). Depends on `lsp-types ^0.95`, which differs from GPUI Kit's 0.97, so you need conversion or a fork. |
| tower-lsp | 0.20.0 | MIT OR Apache-2.0 | 2023-08-11 | **Abandoned** | yes / yes | Server only. Fork: **tower-lsp-server** 0.23.0 (2025-12-07; 0.24.0-rc.1 on 2026-09-11; MIT OR Apache-2.0), which is also server-only and uses the `ls-types` 0.0.6 fork. |
| lsp-server | 0.10.0 | MIT OR Apache-2.0 | 2026-07-16 | Yes (rust-analyzer) | yes / yes | Synchronous, crossbeam-based **server** scaffold. Does not depend on lsp-types. Not a client. |

---

## 16. Errors

| Crate | Latest | Licence | Released | Maintained? | Win + macOS | Note |
|---|---|---|---|---|---|---|
| thiserror | 2.0.21 | MIT OR Apache-2.0 | 2026-09-23 | Yes | yes / yes | MSRV 1.77. |
| anyhow | 1.0.104 | MIT OR Apache-2.0 | 2026-07-18 | Yes | yes / yes | MSRV 1.68. |

---

## 17. Tooling and cross-compiling gpui for Windows

| Crate | Latest | Licence | Released | Maintained? | Host | Note |
|---|---|---|---|---|---|---|
| cargo-deny | 0.21.0 | MIT OR Apache-2.0 | 2026-11-16 | Yes | all | MSRV 1.89. Needs explicit allows for CC0-1.0, BSD-3-Clause, ISC, 0BSD, Zlib, MPL-2.0, CDLA-Permissive-2.0 and MIT-0 (the last two only if those crates are used). |
| cargo-about | 0.9.3 | MIT OR Apache-2.0 | 2026-10-26 | Yes | all | Generates third-party licence notices. |
| cargo-nextest | 0.9.152 | Apache-2.0 OR MIT | 2026-11-13 | Yes | all | MSRV 1.91. |
| cargo-xwin | 0.23.1 | MIT | 2026-08-13 | Yes | macOS/Linux | **Yes, it builds `x86_64-pc-windows-msvc` from macOS** (prerequisite: `brew install llvm`). It downloads the MSVC CRT and Windows SDK through xwin, and **you must accept Microsoft's licence**. It does **not** provide `fxc.exe` or run Windows binaries; the Docker image includes wine. |
| cargo-zigbuild | 0.24.1 | MIT | 2026-11-04 | Yes | all | Linux and macOS targets, plus `*-windows-gnu` only; **no MSVC**. Not useful for gpui on Windows, because GPUI Kit requires the MSVC toolchain. |

**Does gpui-pre-windows need `fxc.exe`? Yes, for release builds.** Source: `gpui-pre-windows-0.3.13/build.rs`.

`build.rs` runs `compile_shaders()` only under `#[cfg(target_os = "windows")]` and `#[cfg(not(debug_assertions))]`. It looks for fxc.exe in three places, in order:
1. the `GPUI_FXC_PATH` env var;
2. `where.exe fxc.exe` on PATH;
3. the newest Windows SDK found through the registry key `HKLM\SOFTWARE\WOW6432Node\Microsoft\Microsoft SDKs\Windows\v10.0`.

If none is found it panics. It compiles 8 shader modules to `vs_4_1`/`ps_4_1`, and the renderer embeds them with `include!(concat!(env!("OUT_DIR"), "/shaders_bytes.rs"))` in release builds. Debug builds compile `src/shaders.hlsl` at **runtime** through `D3DCompileFromFile`, using a path taken from `CARGO_MANIFEST_DIR`. That only works on the build machine. The official `gpui 0.2.2` `build.rs` follows the same pattern.

**Cross-compiling from macOS** *(inference from that code)*:
- Build scripts are compiled for the *host*, so on a macOS host the `cfg(target_os = "windows")` block is skipped. A release `cargo xwin build --target x86_64-pc-windows-msvc` should then **fail**, because `shaders_bytes.rs` is missing.
- A debug cross-build compiles, but the binary looks for the `.hlsl` file at a macOS path at runtime.
- An older Zed commit (#13382, 2024) once switched `build.rs` to `CARGO_CFG_TARGET_OS` "to make cross-compilation to Windows possible". The current Windows shader step does not use that.
- **I found no documented or working recipe.**

Options:
- **(a)** Build Windows on a Windows CI runner, which has the SDK and fxc. This is what Zed does.
- **(b)** Vendor or patch the crate to embed pre-generated shader bytes produced once on Windows.
- **(c)** Patch `build.rs` to use `CARGO_CFG_TARGET_OS` and run fxc.exe under wine through `GPUI_FXC_PATH`. Untested, and check the SDK licence terms.

---

## 18. Rust toolchain

**Latest stable: Rust 1.100.0, released 2026-11-12.** The stable channel manifest shows `date = "2026-11-12"` and `version = "1.100.0 (0ef1bc386 2026-11-09)"`.

**Headlines** (secondary coverage):
- The never type `!` is stabilised. Never-type fallback changes in older editions, which may break some code.
- Core Allocator API parts are stable.
- `DropGuard` added.
- Cargo can report unused dependencies and has a minimum-publish-age setting.

**Upcoming releases** (6-week cadence): 1.101 around 2026-12-24, 1.102 around 2027-02-04.

**MSRV floor from this list:**
- 1.95: `windows-core` 0.100, `sysinfo` 0.40, `gpui-ce`
- 1.94.1: AWS SDK
- 1.92 or later: GPUI Kit
- "latest stable": gpui README

Pinning 1.100 in `rust-toolchain.toml` is reasonable.

---

## 19. macOS packaging

| Crate | Latest | Licence | Released | Maintained? | Host | Note |
|---|---|---|---|---|---|---|
| cargo-bundle | 0.12.0 | MIT OR Apache-2.0 | 2026-09-20 | Yes (repo pushed 2026-11-14) | macOS | Builds a `.app` from `[package.metadata.bundle]`. Does not sign or notarize. |
| cargo-packager (CrabNebula) | 0.11.8 | Apache-2.0 OR MIT | 2025-11-27 | Repo active (pushed 2026-11-16), but **no crates.io release for about 12 months** | all | Outputs `.app`, `.dmg`, NSIS `.exe`, WiX `.msi`, deb, AppImage. Has a companion `cargo-packager-updater`. |
| apple-codesign (rcodesign) | 0.29.0 | **MPL-2.0** | 2024-11-29 | **Low.** No release in about 2 years; the repo gets only dependency bumps in 2026. | macOS/Linux/Windows | Signs and notarizes without Xcode, which is useful on Linux CI. On macOS runners, prefer Apple's `codesign`/`notarytool`. |

---

## 20. Windows installer

| Tool | Latest | Licence | Released | Maintained? | Note |
|---|---|---|---|---|---|
| cargo-wix | 0.3.9 | MIT/Apache-2.0 | 2025-03-13 | Slow (repo pushed 2026-07) | Supports WiX v3.14.1 (`candle`/`light`) and v4+ (`wix.exe`). Runs on Windows only. |
| WiX Toolset v6 / v7 | v6.0.0 (2025-04); v7.0.0 (2026-04) | Source: **MS-RL** (reciprocal). FireGiant says it does not cover the MSIs you build. | — | Yes | Details below. |
| Inno Setup | 7.x (download page) | "Inno Setup License" (SPDX `InnoSetup`, permissive) | — | Yes | **Since 6.5.0 (2025-08-12), commercial users are asked to buy a commercial licence.** It is not technically enforced and the FAQ wording is ambiguous. Licences are perpetual with 2 years of updates. |
| MSIX | — | — | — | No maintained Rust tool: `msix` 0.4.0 (2022), `cargo-msix` 0.5.1 (2024-01), `msixbundle-cli` (tiny) | Use Windows SDK `makeappx` + `signtool`. Needs a trusted code-signing certificate. MSIX also gives a package identity, which makes toast COM activation easy. |

**WiX licensing:**
- **Open Source Maintenance Fee since v6:** required from organisations with **more than $10k annual revenue**, paid through GitHub Sponsors.
- **v7 enforces EULA acceptance:** `-acceptEula wix7`, `wix eula accept wix7`, or the `AcceptEula` MSBuild property.

---

## Sources

**Crate metadata and source code**
- crates.io API, per crate: `https://crates.io/api/v1/crates/<name>`, `/owners`, `/<name>/<version>/dependencies` (e.g. https://crates.io/api/v1/crates/gpui-pre, https://crates.io/api/v1/crates/gpui-pre/owners, https://crates.io/api/v1/crates/gpui-kit/0.10.0/dependencies)
- crates.io search API (INI crates): https://crates.io/api/v1/crates?q=ini%20preserve&sort=downloads
- Crate source code read from tarballs: https://static.crates.io/crates/gpui-pre-windows/gpui-pre-windows-0.3.13.crate (`build.rs`, `src/directx_renderer.rs`, `src/system_notifications.rs`, `Cargo.toml`), https://static.crates.io/crates/gpui-pre-macos/gpui-pre-macos-0.3.13.crate, https://static.crates.io/crates/gpui-pre/gpui-pre-0.3.13.crate, https://static.crates.io/crates/gpui/gpui-0.2.2.crate, https://static.crates.io/crates/alacritty_terminal/alacritty_terminal-0.26.0.crate, https://static.crates.io/crates/portable-pty/portable-pty-0.9.0.crate, https://static.crates.io/crates/process-wrap/process-wrap-10.0.1.crate, https://static.crates.io/crates/apple-native-keyring-store/apple-native-keyring-store-1.0.2.crate, https://static.crates.io/crates/tauri-winrt-notification/tauri-winrt-notification-0.8.1.crate, https://static.crates.io/crates/mac-notification-sys/mac-notification-sys-0.6.15.crate, https://static.crates.io/crates/notify-rust/notify-rust-4.19.0.crate, https://static.crates.io/crates/keepawake/keepawake-0.6.1.crate, https://static.crates.io/crates/single-instance/single-instance-0.3.3.crate, https://static.crates.io/crates/tauri-plugin-single-instance/tauri-plugin-single-instance-2.5.5.crate, https://static.crates.io/crates/rust-ini/rust-ini-0.21.3.crate, https://static.crates.io/crates/gix-config/gix-config-0.62.2.crate, https://static.crates.io/crates/aws-config/aws-config-1.13.2.crate, https://static.crates.io/crates/gpui-base/gpui-base-0.10.0.crate, https://static.crates.io/crates/gpui-component/gpui-component-0.10.0.crate
- GitHub REST API (pushed_at, releases, commits): https://api.github.com/repos/longbridge/gpui-kit, https://api.github.com/repos/longbridge/gpui-kit/releases, https://api.github.com/repos/wezterm/wezterm/commits?path=pty, https://api.github.com/repos/velopack/velopack, https://api.github.com/repos/indygreg/apple-platform-rs/releases, https://api.github.com/repos/microsoft/windows-rs/releases

**GPUI and GPUI Kit**
- https://raw.githubusercontent.com/longbridge/gpui-kit/main/README.md ; https://github.com/longbridge/gpui-kit/blob/main/docs/ARCHITECTURE.md ; https://gpui-kit.com/docs/getting-started ; https://gpui-kit.com/docs/installation ; https://github.com/longbridge/gpui-kit/releases
- https://raw.githubusercontent.com/zed-industries/zed/main/crates/gpui/README.md ; https://www.gpui.rs/ ; https://docs.rs/crate/gpui-pre/0.3.3 ; https://baeseokjae.github.io/posts/native-app-for-coding-agents-rust-gpui/ (third party; crates.io lag) ; https://github.com/BumpyClock/gpui-component/pull/17
- Zed build.rs history: https://git.secluded.site/zed/commit/7be1ffb9ecdd299ddbdad1d1301cf4fb9ea701cc (#13382) ; https://git.secluded.site/zed/commit/769a8a650e18b93df93feb036c9a7f95ec1f701f (#38711) ; https://docs.rs/crate/rgpui-windows/0.2.2/source/build.rs

**Cross-compiling**
- https://github.com/rust-cross/cargo-xwin (README) ; https://github.com/rust-cross/cargo-zigbuild (README)

**AWS**
- https://docs.rs/aws-sdk-ssooidc/latest/aws_sdk_ssooidc/operation/create_token/builders/struct.CreateTokenFluentBuilder.html ; https://docs.rs/aws-sdk-ssooidc/latest/aws_sdk_ssooidc/client/struct.Client.html ; https://docs.aws.amazon.com/sdk-for-rust/latest/dg/http.html ; https://redlib.hbubli.cc/r/rust/comments/1j4cd0d/are_there_any_minimal_dependencies_s3_crates_out (r/rust mirror)

**HTTP and async**
- https://raw.githubusercontent.com/seanmonstar/reqwest/master/CHANGELOG.md ; https://docs.rs/hyper-util/latest/hyper_util/client/proxy/matcher/struct.Matcher.html ; https://raw.githubusercontent.com/tokio-rs/tokio/master/README.md

**Keychain and windows-rs**
- https://raw.githubusercontent.com/open-source-cooperative/keyring-rs/main/README.md ; https://github.com/microsoft/windows-rs/issues/4867 ; https://github.com/microsoft/windows-rs/releases

**Notifications and power**
- https://docs.rs/objc2-foundation/latest/objc2_foundation/struct.NSProcessInfo.html
- https://raw.githubusercontent.com/hoodie/notify-rust/main/README.md ; https://docs.rs/user-notify/latest/user_notify/ ; https://docs.rs/crate/user-notify/0.4.2/source/Readme.md
- https://learn.microsoft.com/en-us/windows/apps/design/shell/tiles-and-notifications/send-local-toast-other-apps

**Processes, watching, single instance**
- https://raw.githubusercontent.com/watchexec/command-group/main/README.md ; https://raw.githubusercontent.com/watchexec/process-wrap/main/README.md
- https://raw.githubusercontent.com/notify-rs/notify/main/README.md
- https://raw.githubusercontent.com/kotauskas/interprocess/main/README.md ; https://raw.githubusercontent.com/bytecodealliance/rustix/main/README.md ; https://raw.githubusercontent.com/GuillaumeGomez/sysinfo/main/README.md ; https://raw.githubusercontent.com/andrewdavidmackenzie/libproc-rs/master/README.md

**Config, i18n, LSP**
- https://github.com/GitoxideLabs/gitoxide/blob/main/crate-status.md (gix-config lossless) ; https://github.com/Le-Syl21/ini-preserve ; https://raw.githubusercontent.com/QEDK/configparser-rs/master/README.md
- https://raw.githubusercontent.com/longbridge/rust-i18n/main/README.md
- https://raw.githubusercontent.com/oxalica/async-lsp/main/README.md ; https://raw.githubusercontent.com/tower-lsp-community/tower-lsp-server/main/README.md

**Updater**
- https://github.com/velopack/velopack ; https://docs.velopack.io/packaging/signing ; https://www.nuget.org/packages/vpk

**Rust toolchain**
- https://static.rust-lang.org/dist/channel-rust-stable.toml ; https://phoronix.com/news/Rust-1.100-Released ; https://linuxiac.com/?p=223025

**Installers and packaging**
- https://docs.firegiant.com/wix/osmf/ ; https://www.firegiant.com/blog/2025/4/7/wix-v600-available/ ; https://github.com/wixtoolset/wix/releases
- https://jrsoftware.org/isorder.php ; https://jrsoftware.org/files/is6.5-whatsnew.htm ; https://spdx.org/licenses/InnoSetup.html
- https://raw.githubusercontent.com/volks73/cargo-wix/main/README.md ; https://raw.githubusercontent.com/crabnebula-dev/cargo-packager/main/README.md
