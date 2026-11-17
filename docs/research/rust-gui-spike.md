# Rust + gpui spike (2026-11-17)

Throwaway spike for the owner's move to Rust (program and UI). The spike's code and binaries stayed in the session scratchpad and are not in the repository; this is its report as written by the spike agent (Sonnet), kept as research. Paths below are relative to the spike folder or to crates in the cargo registry.

Date 2026-11-17. Machine: Apple M4 Mac, macOS 27 (Darwin 27.2.0), 10 cores, display reported as 1920x1080 @ 75 Hz (Retina 2560x1664 panel). All numbers: release build, median of 5 runs (`results/run*.jsonl`, `results/summary.txt`).

**Important caveat about noise.** Other agents were running on this Mac the whole time (load average 12 to 32 on 10 cores). In about half of the runs, the compositor delivered frames every 3rd vsync (40 ms, i.e. 25 Hz) for whole scenarios; in quiet runs frames came every 13.3 ms (75 Hz) and the app never missed one. So frame-interval numbers below show both the median of 5 runs and the quiet-run value. The app's own CPU cost per frame (`render_to_paint`) is a cleaner signal and is always 1-3 ms. Re-run `bin/codepass-spike --bench` on an idle machine to confirm.

## Stack actually used

- `gpui-kit 0.10.0` (umbrella crate by the gpui-component authors) -> `gpui-component 0.10.0` + `gpui-base 0.10.0` + `gpui-pre 0.3.12` (exact pin `=0.3.12`) + `gpui-pre-macos/apple 0.3.12`. All from crates.io, **no git dependency, nothing cloned**. `alacritty_terminal 0.26.0` for emulation. `gpui-terminal`/`gpui_xterm` not tried (they target the old official `gpui 0.2.x`, which would add a second gpui copy); wrote a ~150-line terminal painter in `app/src/term.rs`.
- Note: the version of gpui-component the owner may have in mind (0.5.x, `longbridge/gpui-component`) is gone: the project was renamed to **GPUI Kit** (repo `longbridge/gpui-kit`), and the crate is now 0.10.0 on a fork-like snapshot named `gpui-pre`.
- **Rust 1.100.0 needed.** 1.94 fails (`std::hint::cold_path`), 1.95/1.96/1.97 fail (`result_option_map_or_default` in gpui-pre-macos `window.rs:896,957,1068`). 1.100.0 works (pinned in `rust-toolchain.toml`). The crates declare no `rust-version`.
- Runtime shaders: `gpui-pre-apple` feature `runtime_shaders` (enabled by a direct dependency in `app/Cargo.toml`). Works without the Metal Toolchain; the build never calls `xcrun metal`. Cost: first frame includes compiling the Metal shader source at start-up. I could not compare against precompiled shaders (no Metal toolchain), so the exact cost is unknown; it is inside the first-frame number below.

## Results

| Metric | Target | Measured (median of 5) | Result |
|---|---|---|---|
| Process start to first frame (warm) | <= 300 ms | 324 ms from `main()` (quiet runs 284-307); whole process wall time incl. start+exit 413 ms | **marginal fail** (borderline; includes runtime shader compile) |
| RSS idle, all 4 panes built | <= 150 MB | 110 MB with empty editor; **204 MB** after 50k-line file loaded; **243 MB** after terminal scrollback is full | pass only if editor unloaded; **fail** with real content |
| CPU idle 5 s | ~0% | 0.01% (0 frames drawn) | pass |
| List scroll, 10,000 rows, p95 frame | <= 16.7 ms | quiet runs 13.6-13.8 ms (= vsync); median of 5 runs 39.8 ms (load-throttled); app CPU per frame p95 1.5 ms | pass when machine quiet; see caveat |
| Display refresh | record | 75 Hz (13.3 ms); 120 Hz p95 <= 8.3 ms not testable here (app CPU p95 1.5 ms would fit) | n/a |
| Editor: 50k lines to first paint | <= 300 ms | 431 ms (quiet runs 335-363) | **fail** |
| Editor: 200 keystrokes p95 key-to-frame | <= 16 ms | 14.5 ms (includes waiting for the next 75 Hz vsync) | pass |
| Terminal flood: throughput | >= 50 MB/s | 141 MB/s with window drawing (99-199); 131 MB/s not drawn | pass |
| Terminal flood: max frame gap | <= 100 ms | 40 ms (quiet runs 14 ms); 112 ms seen once in a realistic run under load | pass (with fair lock, see below) |
| Terminal realistic ~2 MB/s: p95 frame | <= 16.7 ms | quiet runs 13.5 ms; median of 5 runs 40 ms (load-throttled); CPU per frame p95 1.0 ms | pass when machine quiet |

Other numbers:
- Clean release build (`rm -rf target`): **3 min 47 s** wall (16 min CPU, machine loaded), thin LTO, codegen-units 16. Release re-link after touching one file: 63 s. Debug clean build 2 min 8 s.
- **Incremental debug rebuild after touching one UI file: 2-3 s** (the app is small; a real product will be slower).
- Release binary, stripped: **20.2 MB** (`bin/codepass-spike`). No bundled fonts; system fonts only.
- RSS after each scenario: initial 110 MB, after editor 204, after list 205, after flood 243, after realistic 243, idle at end 243 MB.
- Sizes: `cargo-home` 1.5 GB, `target` 4.9 GB with debug+release (1.6 GB release only).
- Frame measurement method: a `canvas` element at the end of the root view records `Instant::now()` in its paint callback (before GPU present); keystroke latency = time from `insert()` call to the next painted frame, so it contains the wait for vsync.
- The flood finishes in 0.3 s (50 MB at ~140 MB/s), so only 10-30 frames are drawn during it. Flood data is a repeated 1 MB pre-generated block (SGR colours, 256/true colour, bold, `\r` progress bars, cursor moves, CJK/wide chars), grid 120x36, scrollback 10,000.

## Findings a to g

**a. Screenshots without Screen Recording permission: YES, works.** `gpui::Window::render_to_image() -> RgbaImage` (`gpui-pre-0.3.12/src/window.rs:3095`) renders the current frame's scene into an offscreen Metal texture (`gpui-pre-apple-0.3.12/src/metal_renderer.rs:710-790`, `render_offscreen`), reads pixels back, no window server capture involved. It sits behind the `test-support` feature of `gpui-pre`, `gpui-pre-apple` and `gpui-pre-macos` (all three needed, else it returns "not implemented for this platform"). I built a `shots` cargo feature in `app/Cargo.toml` and a `--shots <dir>` mode: it opens the normal window, switches each tab (light/dark), calls `render_to_image`, saves PNG. 7 PNGs were produced in `shots/` and reviewed; text, CJK, colours, dark theme all correct (1100x761, window not covered). Also exist: `VisualTestContext` / `open_offscreen_window` (`app/visual_test_context.rs:97`) and `HeadlessAppContext::capture_screenshot` (`app/headless_app_context.rs:168`) for windowless runs; not tried. Binary with the feature: `bin/codepass-spike-shots` (usage: `bin/codepass-spike-shots --shots /some/dir`). It still opens a window briefly (a real Metal layer is required).

**b. Accessibility: AccessKit is present.** Cargo.lock has `accesskit 0.24.1`, `accesskit_macos 0.26.3`, `accesskit_windows 0.34.0`, `accesskit_unix 0.22.1`. gpui builds a tree: `gpui-pre-0.3.12/src/window.rs:1744-1806` (TreeUpdate), docs `gpui-pre-0.3.12/src/_accessibility.rs`, element role API `elements/div.rs`, macOS bridge `gpui-pre-macos-0.3.12/src/window.rs:2425-2447` (`a11y_tree_update`, `request_initial_tree`). gpui-component sets roles/labels: 219 `Role::` uses in gpui-component + gpui-base, e.g. `button/button.rs:337,823-827` (`.role()`, Button/Link), inputs `.aria_label()`, `.accessibility_id()` (`input/input.rs:288-293`); README claims "AccessKit roles, names, states, relationships, and actions". **Not verified with a real screen reader** (VoiceOver step is in MANUAL-TEST.md).

**c. Windows build (x86_64-pc-windows-msvc), not installed or tried.** Needs: MSVC build tools + Windows SDK. `gpui-pre-windows-0.3.12/build.rs` compiles `src/shaders.hlsl` with **`fxc.exe`** (from the Windows SDK; env `GPUI_FXC_PATH` overrides, else `where.exe` / SDK search, `panic!("Failed to find fxc.exe")`). That code is guarded by `#[cfg(target_os = "windows")]` and release only (`build.rs:4-14`), which in a build script means the *host* OS, so cross-building from macOS (cargo-xwin) would probably skip shader compilation and fail or need a patch. Practical route: build natively on a Windows machine/CI runner (Visual Studio Build Tools + SDK, which require accepting Microsoft licences), `windows-manifest` feature embeds a resource via `embed_resource` (needs `rc.exe`). Windows uses DirectX 11, so no Metal issue. Also `gpui-fast` optional feature (GPUI Fast fork) exists, not tried. Linux/Windows are not exercised by the owner's tests here.

**d. Input methods (IME) on macOS: implemented.** `gpui-base-0.10.0/src/input/base/state.rs:4648` `impl EntityInputHandler for InputBaseState<M>`: `marked_text_range` (4706), `unmark_text` (4715), `replace_and_mark_text_in_range` (4988), `bounds_for_range` (5126, used to place the candidate window at the caret). The same state type backs Input, Textarea and the code Editor (`InputMode`, `TextareaMode`, `EditorMode`), so all three get it. macOS side: `gpui-pre-macos-0.3.12/src/window.rs:214-239,2740,3464-3468` registers the `NSTextInputClient` methods (`setMarkedText:selectedRange:replacementRange:`, `hasMarkedText`, `firstRectForCharacterRange`). There are unit tests that simulate composition ("n" -> "ni" -> "你", `state.rs:8815-8817`, `6573`, `9559-9585`). **Real typing with a system IME was not tested by me** (needs a human): that is the first item of MANUAL-TEST.md. Observed in stderr: `error messaging the mach port for IMKCFRunLoopWakeUpReliable` printed on each start (harmless, comes from InputMethodKit, but note it).

**e. Licences (467 crates in macOS dependency tree, normal + build deps; full list `results/licenses.tsv`).** All MIT / Apache-2.0 / BSD / Zlib / Unicode-3.0 / 0BSD / CC0 / Unlicense / BSL-1.0 variants. **No GPL, AGPL or LGPL, no unknown.** Only flag: **MPL-2.0: `option-ext 0.2.0`** (small file-level copyleft, linked, unmodified, fine for distribution) and **`cbindgen 0.28.0`** (build time only, not shipped). `alacritty_terminal` is Apache-2.0. gpui-pre*, gpui-base, gpui-component, gpui-kit: Apache-2.0. The Windows/Linux target trees were not enumerated.

**f. Versions and churn.**
- gpui-component / gpui-kit / gpui-base 0.10.0 (2026-11-16); before: 0.9.1 (11-09), 0.9.0 (11-02), 0.8.1 (10-26), 0.8.0 (10-19), 0.7.3 and 0.7.2 (10-12). **A new 0.x release every week, with breaking minors.**
- gpui-pre 0.3.12 (11-09; 0.3.13 out on 11-16): a *republished snapshot of Zed's gpui* (`zed@d2a4fb6`, see `[package.metadata.gpui-pre]` in its Cargo.toml), weekly as well. Single crates.io owner `huacnlee` for gpui-pre, gpui-base, gpui-kit and gpui-component. The official `gpui` crate is 0.2.2 from 2025-10-22 (a year old, Zed team owned) and is NOT what gpui-component uses any more.
- gpui-component pins `gpui-pre` with `=0.3.12`, so apps must follow its pin exactly.
- Minimum Rust: not declared; in practice somewhere between 1.98 and 1.100 (1.97 fails, 1.100 works, not bisected; edition 2024).

**g. Chinese text: yes in every pane**, with system font fallback (no font bundled). Verified in PNGs: list ("feature/登录优化"), single and multi-line inputs (Chinese, Japanese, Korean mixed), editor (Chinese comments, syntax highlight), terminal (Chinese in ANSI output; wide characters painted one by one at exact column so alignment holds). Terminal uses Menlo; fallback picked CJK glyphs fine.

## Problems and workarounds
1. Rust version: needs 1.100 (above). `rust-toolchain.toml` pins it; rustup installed it beside the default, nothing global changed.
2. Metal toolchain missing: used `runtime_shaders`.
3. Terminal output starved the UI at first: with a plain `std::sync::Mutex` the emulator thread re-took the lock immediately, UI waited up to 97 ms to paint during flood. Switched to `alacritty_terminal::sync::FairMutex`: frame gap went to ~14-40 ms. A real terminal view must be designed around this lock.
4. `render_to_image` is hidden behind `test-support`; need three feature flags.
5. Machine load made frame timing noisy (see caveat); used `caffeinate -d -i` during runs to keep the display awake (otherwise macOS also throttles an unattended window).
6. Not tested: real IME, VoiceOver, Windows/Linux, 120 Hz display, mouse selection in the terminal (the spike terminal has no selection, input, resize or links), LSP hooks.

## Risks for a full product on this stack
1. **Dependency churn and supply chain.** Weekly breaking 0.x releases pinned to an unofficial snapshot of Zed's gpui (`gpui-pre`) kept alive by one person; the official gpui crate is stale. Needs vendoring/pinning and a plan if the maintainer stops. Rust must be very new (1.100 now).
2. **IME and accessibility are unproven in practice.** The code paths exist, but only a human typing with the owner's real input methods (Sogou/pinyin/wubi/Japanese) can confirm candidate-window position and edge cases; screen reader support is untested. Chinese UI makes this a release blocker risk.
3. **Memory and editor start.** 50k-line file costs about 95 MB and 430 ms to first paint (misses 300 ms); terminal scrollback adds about 40 MB. 150 MB idle is not reachable with real content loaded. Also terminal is a hand-built component (no selection, resize, IME, hyperlinks, mouse reporting): sizeable work. Windows build needs a Windows CI (HLSL `fxc.exe`).

## Files
- App source: `app/` (`main.rs` UI + bench, `term.rs` terminal, `data.rs` fake data). Binaries: `bin/codepass-spike`, `bin/codepass-spike-shots`. Raw results: `results/`. Screenshots: `shots/`. Manual checklist: `MANUAL-TEST.md`.
