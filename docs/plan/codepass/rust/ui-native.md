# Codepass UI on gpui / GPUI Kit: how the existing design is built natively

Author: designer (Fable), 2026-11-17. Read with `design.md` in this folder ("UI map" there means this file).

Proposal only (2026-11-17). Nothing the owner sees or can do changes; this says how each part of `docs/plan/codepass/ui-design.md` is built in Rust. Capabilities were read from source, not run, unless the spike report says so.

Source shorthand (crate sources as cargo unpacks them, `<cargo home>/registry/src/index.crates.io-*/`; the spike used these exact versions):
`gc/` = `gpui-component-0.10.0/src/`, `gb/` = `gpui-base-0.10.0/src/`, `gp/` = `gpui-pre-0.3.12/src/`, `gp-mac/` = `gpui-pre-macos-0.3.12/src/`, `gp-win/` = `gpui-pre-windows-0.3.12/src/`. Effort: S = under a day, M = days, L = a week or more.

## 1. Tokens to theme

**Rule.** `docs/design/tokens.css` stays the source of the values. A Rust module `ui::tokens` holds one struct `Tokens` (a gpui global, one value for light and one for dark) with a field per CSS token, same names with `_` for `-`. A unit test parses `tokens.css` and `mockup.css` and fails when a value differs, so the two cannot drift. `Tokens::apply(cx)` then fills GPUI Kit's `Theme` so Kit components come out in our colours; custom components read `Tokens` directly.

| Token group | GPUI Kit target | Custom code |
|---|---|---|
| Neutrals `--bg --surface* --border* --text*` | `ThemeColor` (`gc/theme/theme_color.rs`): `background`=`--bg`, `sidebar`=`--bg-sidebar`, `sidebar_border`/`border`=`--border`, `popover`/`table`/`list`=`--surface`, `table_head`/`muted`/`secondary`=`--surface-2`, `list_hover`/`table_hover`/`secondary_hover`=`--surface-3`, `foreground`=`--text`, `muted_foreground`=`--text-3`, `input`=`--border-strong`, `overlay`=`--backdrop`, `skeleton`=`--skeleton-a`, `scrollbar_thumb`=`--scrollbar` | `--text-2`, `--text-disabled`, `--surface-sunken`, `--border-subtle` have no Kit field: read from `Tokens` |
| Ink (primary action) | `primary`/`primary_hover`/`primary_active`/`primary_foreground` and the `button_primary*` set = `--ink`, `--ink-hover`, `--ink-press`, `--on-ink` | none |
| Status `--ok --warn --danger --info` + `-soft` + `-line` | `success*`, `warning*`, `danger*`, `info*` take the solid values | Kit has no soft / line pair per status: `Tokens` only; badges, banners, toasts tint from it |
| Focus, link, selection | `ring`=`--focus`, `link*`=`--link`, `selection`=`--focus` at 28% alpha, `caret`=`--text` | double ring, see below |
| Identity palette `--acct-1..8` (+soft, ink, on, line) | none | `Tokens::acct(n) -> AcctColors`; `--acct-line` = solid at 38% alpha (replaces `color-mix`). `[data-acct]` becomes a parameter `acct: Option<u8>` on pass, chip, tile, bar |
| Syntax `--syn-*`, editor `--editor-*` | `Theme.highlight_theme` (`gc/theme/mod.rs:170`, `gc/highlighter/`) built from the six `--syn-*` values; editor background / gutter / current line set on the editor's style | a small builder `syntax_theme(&Tokens) -> HighlightTheme` (S) |
| Git `--git-*`, diff `--diff-*` | diff tints passed to `Diff` (`change_background`, `gc/diff/mod.rs`) | `Tokens` |
| Terminal `--term-*` | none (terminal is ours) | 16-colour table + bg/fg/dim/line from `Tokens`; same in both themes unless the setting says follow the app |
| Type scale 11/12/13/14/16/20/24/32, weights, line heights | `Theme.font_size` = 14 (Kit default is 16, `gc/theme/mod.rs:183`); `mono_font_size` = 12.5 | named text styles in `ui::text` (`page_title()`, `caption()` ...), each sets size, weight, line height; zh line height 1.6 picked by the app language. Tabular numbers: `font_features` `tnum` on the base style (`gp/style.rs:446`). Display tracking -0.01em: gpui has no letter spacing in `Style`; dropped (it is 0 in Chinese already; in English the 24px title is about 0.2px per letter wider) |
| Spacing `--sp-*`, control heights, `--row-h`, `--sidebar-w`, `--rail-w`, workspace frame | none needed | `const` pixels in `ui::tokens::size`. All sizes are written in **rems with rem = 14px base** so View > Zoom works through `Window::set_rem_size` (`gp/window.rs:3349`) |
| Radii 4/6/8/12/16/full | `Theme.radius` = 8 (buttons, inputs), `Theme.radius_lg` = 16 (dialog; `gc/theme/mod.rs:200-202`) | cards 12, chips 6, kbd 4 set at the call site from `Tokens` |
| Shadows `--shadow-1/2/3` | `Theme.shadow = true`; | exact values as `Vec<BoxShadow>` with offset, blur, spread, inset (`gp/style.rs:292,349-360`), so both the two-layer light shadows and the dark 1px ring (`0 0 0 1px`) are exact. Helpers `shadow_1/2/3(cx)` |
| Motion durations and easings | `Theme.motion: MotionTokens` (`gc/theme/motion.rs`): `duration_fast` 120, `duration_normal` 160, `duration_slow` 240, `easing_enter` (.2,.8,.2,1), `easing_exit` (.4,0,1,1), `easing_move` (.3,.7,.2,1) | the other durations (90, 180, 200, 220, 420) and `--ease-spring` live in `Tokens`; verify `Easing::cubic_bezier` accepts the overshoot value 1.4, else use `Spring` |
| Focus ring (`--focus-ring`: 2px gap in surface colour, then 2px `--focus`) | Kit draws a ring outside the border when `Theme.focus_ring` is on (`gc/theme/mod.rs:206-214`) | to match the gap exactly: `.focus_visible(|s| s.box_shadow([spread 2px surface, spread 4px focus]))` (`gp/elements/div.rs`, `focus_visible`); one helper `focus_ring()` used by every custom control. Inputs keep the softer 3px halo the same way |
| z-index scale | not needed: overlays are drawn by `Root` layers (`gc/root.rs`) and `deferred` / `anchored` (`gp/elements/`) in a fixed order: popover < sheet/dialog < toast < tooltip | none |
| Theme light / dark / system | `Theme::change(mode)`; follow the OS with `Window::observe_window_appearance` (`gp/window.rs:2537`) | Settings value `light / dark / system` |

## 2. Component inventory

"Kit" = used as is with our tokens; "Kit+" = Kit component inside our wrapper that fixes look; "Custom" = built on gpui primitives (`div`, `uniform_list`, `canvas`, `anchored`).

| Design component | Built with | Effort | Gap and fix (owner-visible result kept) |
|---|---|---|---|
| Button primary / secondary / ghost / danger, sm / lg, loading, icon button, link | Kit+ `gc/button/button.rs` (`ButtonVariant` incl. `Custom`, `.loading()`, Role::Button) | S | heights 26/32/40 and hover = stronger border come from `ButtonCustomVariant`; loading keeps the width (spinner centred): wrapper measures once |
| Split button (`.splitbtn`, Sync) | Kit+ `gc/button/dropdown_button.rs` (button + caret menu) | S | the two count pills sit inside the main part as children |
| Count pills `.sync-n`, badges, dots, chips | Custom (a `div` with text), `gc/tag.rs` not close enough | S | dashed "Not published" border: `BorderStyle::Dashed` exists (`gp/scene.rs:597-603`) |
| Identity pass, stripe, monogram tile, account chip, perforated action row | Custom | M | hatched stripe of the static pass and the perforation are painted in a `canvas` (no CSS gradients or masks) |
| Sidebar nav, keyring, usage chip, keep-awake chip | Custom on `v_flex`; Kit `gc/sidebar/` not used (its look and collapse differ) | M | role Navigation, current item marked selected |
| Icon rail | Custom | S | tooltips from Kit |
| Tabs (`.tabs`, workspace tabs, editor tabs) | Kit+ `gc/tab/` (`TabVariant::Underline`; Role::TabList / Tab) | S | counts as children; editor tabs add close and the unsaved dot |
| Segmented control | Kit `TabVariant::Segmented` or `gb/toggle_group.rs` | S | `aria-pressed` look kept through selected state |
| Tables with the roving grid (Projects, Accounts) | Custom rows + one shared `RovingGrid` helper; `gc/table/` `DataTable` only for flat tables | M | `DataTable` already has Role::Grid / GridCell and Up, Down, Left, Right, Home, End, Page keys (`gc/table/data_table.rs:20-28,227`, `state.rs:1605`), but our rows are 52px cards per project with buttons in cells; the helper copies the same key set and roles |
| Long lists (commits, files, find results, tasks, switcher) | Kit `gc/list/` (`ListDelegate`, sections, search) on `gc/virtual_list.rs` / `gp/elements/uniform_list.rs` | M | spike: 10,000 rows at about 1.5ms CPU per frame |
| Tree (files, outline) | Kit+ `gc/tree.rs` + `gb/tree.rs` (Role::Tree / TreeItem, context menu, drag) | M | git letter and colour as trailing children |
| Dialog / confirm (`.modal`) | Kit `gc/dialog/` (`Dialog`, `AlertDialog`: overlay, Esc, `ok_text`, `cancel_text`, Role::Dialog / AlertDialog) + `gb/focus_trap.rs` | S | focus starts on Cancel: set initial focus by handle; primary repeats the verb through `ok_text` |
| Side sheet (`.sheet`) | Kit `gc/sheet.rs` (right side, overlay, resizable) | S | 24px slide-in set through its transition |
| Popover, hover card | Kit `gc/popover.rs`, `gc/hover_card.rs` | S | hover card waits 350ms (delay option) |
| Menu, context menu, submenus | Kit `gc/menu/popup_menu.rs`, `context_menu.rs`, `dropdown_menu.rs` (Role::Menu / MenuItem, arrows) | S | **type-ahead is missing** in `popup_menu.rs`: add in our wrapper (S). Menus are clipped to the window; for menus near the edge of a small window use `gc/native_menu/` (OS-drawn) |
| Tooltip | Kit `gc/tooltip.rs` (`show_delay`, Role::Tooltip) | S | none |
| Toasts (`.toast`), stacking, Undo action | Kit+ `gc/notification.rs` (placement, `max_items`, width; `gb/toast.rs` sets Role::Alert) | S | bottom-right placement, one action button, danger ones stay until closed; announcement see section 3 |
| Banners, bars (`.banner`, `.opbar`, `.pullall` bar, queue bar, update banner) | Custom (`gc/alert.rs` as reference) | S each | role Status + announcer |
| Progress bar, indeterminate, spinner, skeleton, success check draw | Kit `gc/progress/`, `gc/spinner.rs`, `gc/skeleton.rs`; check draw Custom | S | check is a path whose length animates (`gp/path_builder.rs`) |
| Switch, checkbox, radio | Kit `gc/switch.rs`, `gc/checkbox.rs`, `gc/radio.rs` (Role::Switch / CheckBox / RadioButton, `aria_toggled`) | S | none |
| Text input, textarea, select, combobox (`.base-pick`), search field, number | Kit `gc/input/` (`input.rs`, `textarea.rs`, `search.rs`), `gc/select.rs`, `gc/combobox.rs` | S | IME marked text present (`gb/input/base/state.rs:4648-5126`); needs the owner's real input methods once |
| Stepper (setup, wizard) | Kit+ `gc/stepper/` | S | horizontal variant at narrow width |
| Empty state, key-value list, kbd | Kit `gc/empty.rs`, `gc/description_list.rs`, `gc/kbd.rs` | S | none |
| Drag and drop rows (repo to project, reorder accounts, tickets) | Custom on gpui `on_drag` / `drag_over` / `on_drop` / `can_drop` (`gp/elements/div.rs`) | M | ghost has shadow-3 and the target ring; **no tilt or 96% scale** (section 7, item 1). Keyboard path and spoken steps through the announcer |
| Expand / collapse groups | Kit `gc/collapsible.rs` + height transition (`gb/animation.rs` `EffectTransition::height`) | S | chevron turn: the icon is an SVG, which can rotate (`gp/elements/svg.rs:66`) |
| Resizable panes, bottom panel | Kit `gc/resizable.rs` (`gb/resizable/`) | S | sizes remembered per repository by us |
| Workspace header, status bar | Custom; `gc/status_bar.rs` as base | S | drag region: `window_control_area(Drag)` (`gp/elements/div.rs`) |
| **Terminal** | Custom: `alacritty_terminal` 0.26 + our painter (spike `app/src/term.rs`, 150 lines, no input) | **L** | still to build: keyboard and mouse reporting, selection and copy, resize, IME at the cursor (implement `EntityInputHandler` with `bounds_for_range`), links, scrollback search, paste guard, screen-reader mode (a read-only text view of the grid), `FairMutex` around the grid (spike finding) |
| **Code editor** | Kit `gc/input/editor.rs` + `gb/input/editor/` (display map, search, diagnostics, `lsp/`; providers: Completion, Hover, Definition, CodeAction, `gc/input/popovers/`); tree-sitter grammars by feature flag | **L** | to add around it: tabs and split, change-bar gutter, peek view, blame margin, references list, go to line / symbol, recovery store. Spike: 50k lines open in 335-431ms (target 300) and cost about 95MB; open big files read-only-first or lazily |
| Diff view (`.diff`, `.hunk`, `.dl`) | Kit `gc/diff/` (parses a patch, `DiffMode::Split` / `Unified`, virtualised, line tints, gutter markers, annotations, syntax) | M | read-only, which is what Changes, History, task review and PR files need. Stage / discard per hunk: buttons through `render_header` / gutter markers |
| Merge conflict view (`.cf*`) | Kit+ `gc/diff/conflict.rs` (`DiffConflict`, `DiffConflictResolution`, current / base / incoming ranges) + the editor for the result | L | Kit gives parsing and block actions; the three-part layout, "block resolved" bar and F7 stepping are ours |
| Markdown view (PR text, release notes, hover docs, tutorial) | Kit `gb/text/` `TextView::markdown` | S | **must set** `.markdown_raw_html(false)` (`gb/text/markdown_ext.rs:300`), an `image_source` that returns a placeholder for every URL (`gb/text/text_view.rs:375-384`; the default would load images), and `on_link_click` to our https-only opener. One wrapper `safe_markdown()`; a test forbids `TextView::markdown` / `html` anywhere else |
| Command palette, quick open, session switcher, open repository | Kit+ `gc/command/` (search field over grouped list, key hints) | M | active-descendant model: `aria_active_descendant` exists (`gp/elements/div.rs`) |
| Settings rows and cards | Kit+ `gc/setting/` or plain `v_flex` | S | none |
| Timeline, checks, merge box, commit graph refs | Custom | M | graph lines painted in `canvas` |
| Select-and-copy text (IDs, paths, keys, log) | Kit `gb/selectable_text.rs`, `gc/text/window_selection.rs` | S | section 3 |
| Scrollbars | Kit `gc/scroll/`, `gb/scrollbar.rs` | S | section 3 |

## 3. Web-only notions translated

**CSS class names.** The stable class list (`.sync-n`, `.pullall*`, `.base-pick`, `.ws*`, `.pass` ...) stops being code. Each family becomes one Rust component in `ui/components/` whose file and type carry the name (`sync_n.rs` `SyncN`, `pullall/` `PullAllBar` / `PullAllSheet`, `base_pick.rs`). The design keeps using the class names as the names of the parts; a table `docs/design/components.md` (class -> Rust type) is kept by the task that builds each one. Modifiers (`.is-active`, `--warn`) become enum values or builder methods.

**ARIA to AccessKit.** gpui exposes on any element (`gp/elements/div.rs`): `.role(accesskit::Role)`, `.aria_label`, `.aria_description`, `.aria_expanded`, `.aria_selected`, `.aria_toggled`, `.aria_value`, `.aria_numeric_value` (+ min, max, step), `.aria_level`, `.aria_position_in_set`, `.aria_size_of_set`, `.aria_row_count` / `_index`, `.aria_column_count` / `_index`, `.aria_orientation`, `.aria_placeholder`, `.aria_keyshortcuts`, `.aria_active_descendant`.

| Web | Native |
|---|---|
| `role="dialog"` / `alertdialog` | Role::Dialog / AlertDialog (Kit dialogs set it) |
| `<nav aria-label>`, `aria-current="page"` | Role::Navigation + label; current item `aria_selected(true)` |
| roving grid | Role::Grid, Row, GridCell + row / column index |
| listbox + `aria-activedescendant` | Role::ListBox / ListBoxOption + `aria_active_descendant` |
| `button[role=switch][aria-checked]` | Role::Switch + `aria_toggled` |
| `aria-expanded` + `aria-controls` | `aria_expanded`; "controls" is not exposed by gpui and is dropped (the content follows the button in order, which is what readers use) |
| `aria-valuenow` on progress | Role::ProgressIndicator + `aria_numeric_value` |
| `role="status"`, `role="alert"`, `role="log"`, `aria-live` | roles exist, **live announcements do not**: see next paragraph |
| `aria-hidden` on dots, `.sr-only` text | decorative elements get no role and no label; "screen-reader-only text" becomes the `aria_label` / `aria_description` of the visible element |
| `aria-label` on icon buttons | `.aria_label` (Kit warns when an icon-only button has none, `gc/input/clear_button.rs:13`) |

**Live regions: not supported by gpui-pre 0.3.12.** AccessKit has the node property (`accesskit-0.24.1/src/lib.rs:584`, `enum Live`), and `accesskit_macos` handles it, but gpui-pre never sets it: no `Live::` or `set_live` anywhere in `gp/`, `gb/` or `gc/`. Kit's toast and alert only set Role::Alert. Proposal: one service `ui::a11y::announce(text, Politeness::{Polite, Assertive})`, called from every place the design names a live region (setup progress, drag steps, queue bar, Pull all `.sr.*` strings, verify result, countdown state changes, toasts):

- macOS: `NSAccessibilityPostNotificationWithUserInfo(mainWindow, NSAccessibilityAnnouncementRequestedNotification, {NSAccessibilityAnnouncementKey: text, NSAccessibilityPriorityKey: medium | high})`.
- Windows: `UiaRaiseNotificationEvent(provider, NotificationKind_Other, NotificationProcessing_All | ImportantAll, text, activityId)` on the window's UIA provider (Windows 10 1709 and later; Narrator, NVDA and JAWS read it).
- Later, if the vendored gpui-pre gains a `.aria_live()` (a few lines that call AccessKit's `set_live`), the service can switch to it without touching callers.

The rules stay: polite by default, assertive for "failed" and "needs you", the countdown speaks only at state changes, the setup log is never read line by line.

**Focus and roving tab index.** gpui has `FocusHandle`, `.track_focus()`, `tab_index` / `tab_stop` (`gp/window.rs:582-593`), `Window::focus_next` / `focus_prev` (`gp/window.rs:2655-2666`) and key contexts. A composite (grid, tree, menu, tabs, segmented) is **one tab stop**; arrows move an inner "active" index that the composite owns, which is the roving pattern. Focus trap and return-to-trigger for dialogs and sheets: `gb/focus_trap.rs`. Shortcuts are gpui actions bound per key context: `Workspace`, `Editor`, `Terminal`, `Dialog`. The `Terminal` context binds only the keys section 4.3 lists (Ctrl+`, Cmd/Ctrl+1..5, quick capture, the switcher, Cmd+O on macOS); every other key is sent to the process.

**Hover, pressed, focus-visible.** `.hover()`, `.active()`, `.focus()`, `.focus_visible()` style refinements exist on `div`. Focus rings show only for keyboard focus, as today. Cursor shapes are set per element (`cursor_pointer()`).

**Motion and reduced motion.** Building blocks: gpui `with_animation` (`gp/elements/animation.rs:84`), Kit `EffectTransition` with fade, slide x / y, width, height (`gb/animation.rs:160-221`), `Presence` for exit animations (`gb/motion/presence.rs`), `Spring`, stagger and keyframes (`gb/motion/`). Reduced motion: `gb/reduce_motion.rs` reads the OS (`accessibilityDisplayShouldReduceMotion` on macOS, `SPI_GETCLIENTAREAANIMATION` on Windows) and has `set_reduce_motion_preference`; Settings > Motion = System / Reduced / Full maps onto it. Spinners keep the slow turn and the waiting dot goes steady by an explicit check in those two components. What cannot be done as drawn: section 7, items 1 and 2.

**Text selection.** The web default "nothing selectable except data" is already how gpui behaves: text is not selectable unless made so. Data values (`.mono`, `.kv__val`, code blocks, the setup log, public keys) use `SelectableText` (`gb/selectable_text.rs`) with Cmd/Ctrl+C and the Edit menu; selection colour = `--focus` at 28%.

**Scrollbars.** `Theme.scrollbar_mode` (`gb/scrollbar.rs:47-55`: Scrolling, Hover, Always). To match the mockup's always-visible 10px thumb with 3px inset, use `Always` in panes and lists, thumb `--scrollbar`, transparent track.

**Window widths and layout.** No media queries: views read the window bounds and choose. Rules kept: minimum 980x660 (`WindowOptions.window_min_size`, `gp/platform.rs:2673`); workspace needs 1100x700, below that left panes fold to 44px strips and the second editor group closes; at 980 the pass grid goes from 3 to 2 columns, paths truncate from the middle (our helper), the wizard rail becomes the horizontal stepper. Page max width 1120 centred. Flexbox and grid come from taffy (`gp/taffy.rs`), so `.stack`, `.cluster`, `.grid-cards` become `v_flex`, `h_flex`, `grid`.

**Zoom.** View > Actual size / Zoom in / Zoom out = `Window::set_rem_size`; this is why every size is in rems. Windows 125% and 150% scaling is handled by gpui's scale factor.

**High contrast (Windows).** gpui has no forced-colours signal (no match in any crate). Read `SPI_GETHIGHCONTRAST` ourselves and switch to a third `Tokens` value where borders and text use the system colours and identity stripes keep theirs (M, with the Windows milestone).

## 4. Windows and OS surfaces

| Surface | Native plan |
|---|---|
| Main window | one gpui window, 1200x780 default, min 980x660, bounds remembered by us (`WindowOptions.window_bounds`). Root view is Kit `Root` (`gc/root.rs`), which hosts dialogs, sheets and toasts |
| macOS title bar | `TitlebarOptions { appears_transparent: true, traffic_light_position }` (`gp/platform.rs:2827-2834`); the sidebar's top 40px and the 46px workspace header are drag regions; first 84px of the header reserved |
| Windows title bar | system frame and title bar, as the design says (`titlebar: Some(TitlebarOptions{ title, appears_transparent: false })`); Kit's custom `TitleBar` (`gc/title_bar.rs`) is not used |
| Dialogs, confirms, sheets, palette, switcher | **in-window layers** (Kit `Dialog` / `Sheet` on `Root`), never OS windows; that is what the mockup draws (backdrop, 16px radius, rise). File and folder pickers are the OS panels (`cx.prompt_for_paths`) |
| The AWS sign-in window | not ours: the browser's private window, unchanged |
| macOS app menu | `cx.set_menus` (`gp/app.rs:2902`) with exactly the table in 4.4; every item is a gpui action, so the palette, the shortcut and the menu run the same code. No framework default menu exists, so the Cmd+R / Cmd+W / Help problems of 4.4 cannot occur |
| Windows: no menu bar | as designed; commands in the palette. (`gc/menu/app_menu_bar.rs` exists and stays unused) |
| Dock (macOS) | app stays in the Dock in background mode; click reopens the window: `cx.on_reopen` (`gp/app.rs:369`). Closing the window while tasks run hides it: `on_window_should_close` (`gp/window.rs:7162`) returns false and hides. Quit dialog: `on_quit` hook (`gp/platform.rs:463`) returns false and opens 5.6b |
| Tray (Windows), required by 4.4 | **gpui has no tray / status item** (no match in any gpui-pre crate). Use the `tray-icon` crate (MIT / Apache; same event loop on Windows) or about 150 lines over `Shell_NotifyIconW`. Menu: Open Codepass, Quit; left click shows the window |
| macOS menu-bar item (ui-spec 7.12, "P1, if supported") | not in ui-design.md's rules (macOS uses the Dock). Same crate would give it; see question 2 |
| Dock / taskbar badges | the design asks for none; none built |
| System notifications | gpui has them: `cx.show_system_notification(SystemNotification{tag, title, body, actions})`, `dismiss_system_notification(tag)`, `on_system_notification_response` giving tag and button id (`gp/app.rs:1975-1996`, `gp/platform.rs:676-711`). The tag gives "replaces the heads-up" and "once per sign-in"; one button fits the rule. Gaps: (a) **no sound control**: the Windows toast is forced silent (`gp-win/system_notifications.rs:184-186`) and the macOS content sets no sound, but the table asks for the default sound on six notices: patch the vendored crate (a `sound: bool` field, about 10 lines per platform); (b) macOS posts only from a signed app bundle (`gp-mac/system_notifications.rs:120-125`), so dev runs fall back to the in-app toast, which is the design's "notifications unavailable" rule; (c) Windows needs the AppUserModelID registered (done by the crate, line 193) |
| Logout / shutdown | no dialog, stop everything and quit: `on_app_quit` (`gp/app.rs:2842`) |

## 5. Fonts

No font is bundled; system fonts only, as today.

| Use | macOS | Windows | How |
|---|---|---|---|
| UI Latin and digits | SF Pro (`.SystemUIFont`) | Segoe UI (gpui maps the system font to it, `gp-win/direct_write.rs:1744`); Segoe UI Variable on Windows 11 if we name it first | `Theme.font_family` |
| UI Chinese | PingFang SC, then Hiragino Sans GB | Microsoft YaHei UI, then Microsoft YaHei | set `font_fallbacks` explicitly (`gp/style.rs:449`, `gp/text_system/font_fallbacks.rs`) in this order. Without it the OS picks by system language, and a non-Chinese Windows or macOS can show Japanese glyph shapes for some Han characters |
| Monospace (IDs, paths, editor, terminal) | SF Mono if it resolves (try the system monospaced face name), else Menlo | Cascadia Mono, else Consolas | `Theme.mono_font_family`; Kit already falls back Menlo -> Monaco and Consolas -> Cascadia Mono when one is missing (`gc/theme/mono_font.rs:20-25`). CJK inside mono: same fallback list; the terminal paints wide characters at exact columns (spike finding g) |
| Emoji and symbols | Apple Color Emoji | Segoe UI Emoji / Symbol (`gp/svg_renderer.rs:17`) | system fallback |
| Linux (not a target now) | | | Noto Sans CJK SC, DejaVu Sans Mono, if ever built |

Sizes: UI 14 base; editor 13; terminal 12.5 with 19px lines (design 5.6). Icons are our own SVG set from the mockup, embedded with `rust-embed` and drawn by gpui's SVG renderer in `currentColor` style (tinted by text colour).

## 6. Design review in Rust

**Mock mode.** The binary takes `--mock <scenario>`: every service trait gets its fake (the Rust twin of `frontend/src/api/mock`), nothing touches the disk, network, keychain or the owner's tools. Scenarios keep their names: `fresh`, `mid`, `done`, `empty`, `edge` (`frontend/src/api/mock/scenarios.ts`), with the same generic data (acme, globex, initech, umbrella, alex-dev, `111122223333`, example.com). The two scripted fakes stay: a terminal that replays a recorded session and a language server that answers hover, completion and definition.

**Shots.** Cargo feature `shots` (it turns on `test-support` in `gpui-pre`, `gpui-pre-apple`, `gpui-pre-macos`; without all three `render_to_image` says "not implemented", spike finding a). Command:

```
codepass --mock mid --shots <out-dir> [--only <name>] [--size 1440x900]
```

A table in `ui/shots.rs` lists every shot: `name`, `screen`, `state`, `theme`, `lang`, and a setup closure (open the dialog, start the drag, type "find"). For each row the app sets theme and language, navigates, waits for two idle frames with animations forced to their end state, calls `Window::render_to_image()` (`gp/window.rs:3095`) and writes `<out-dir>/<name>.png`. No Screen Recording permission is needed; a window still opens briefly on macOS. `VisualTestContext::open_offscreen_window` (`gp/app/visual_test_context.rs:97`) may remove even that; untried.

**Names.** Exactly the 56 names in `docs/design/codepass-mockup/shots/` (`projects-light-zh`, `workspace-code-dark-en`, `quit-dialog-light-zh`, `setup-win-light-en` ...): `<screen>[-<state>]-<theme>-<lang>.png`, 1440x900 logical. Native shots go to `docs/design/native-shots/` so the HTML ones stay as the reference. Screens drawn later ("Fable first") are still drawn in the HTML mockup first; nothing changes there.

**Comparing.** `cargo run -p shotdiff -- <mockup-dir> <native-dir> <report-dir>` makes, per name, one sheet: mockup, native, and a difference map side by side, plus a number (share of pixels that differ by more than a threshold after a 2px blur, so glyph edges do not count). The gate is a person or a reviewing agent reading the sheet against a fixed checklist: layout and sizes (boxes within 2px), colours (sampled token swatches exact), every string present and unclipped in zh and en, icons, states, focus ring, dark theme. Text is never expected to match pixel for pixel (different rasteriser). Once a native shot is approved it becomes the baseline, and later runs compare native against native with a strict threshold (regression check in CI on macOS).

## 7. What will look or behave differently unless extra work is done

| # | Difference | Recommendation |
|---|---|---|
| 1 | **No scale or rotation of ordinary elements.** gpui transforms only SVGs and sprites (`gp/elements/svg.rs:66`; no transform on `div`). Affected: modal scale .985, popover scale .98, drag ghost 1.2 degree tilt and 96% scale, "ghost shrinks into the target". | Keep fade + rise / drop + shadow (the larger part of each effect); drop the scale and tilt. The drop ends with a 200ms fade of the ghost toward the target header. Question 1 |
| 2 | **FLIP reorder, siblings sliding, cross-fades** are not automatic. Each needs explicit position / height animation. | Build one `AnimatedList` helper (M) for insert, remove, move under 200 rows; number and dot cross-fades are two stacked elements with opposite opacity. Result as designed |
| 3 | **Text looks slightly different**: native rasteriser (CoreText / DirectWrite) instead of Chromium; no letter spacing; line boxes can differ by 1px. Mono may be Menlo where the mockup shows SF Mono. | Accept; review by checklist, not pixels (section 6). Try the system monospaced face first |
| 4 | **Screen-reader announcements** do not exist in gpui-pre. | Build the `announce` service (section 3), S per platform. Needs a VoiceOver and a Narrator pass by a person; the spike did not test either |
| 5 | **Chinese input method** is implemented but unproven with real input methods; the terminal needs its own IME code. | First manual test of the first milestone; release blocker if the candidate window is misplaced |
| 6 | **Notification sound** cannot be chosen. | Patch the vendored gpui-pre (section 4) |
| 7 | **Menus and popovers are clipped to the window** (drawn by gpui). In Chromium they were too, so no change; only context menus near the edge of a minimum-size window may flip sides. | Accept; `NativeMenu` where it matters |
| 8 | **Menu type-ahead** is missing in Kit. | Add in our wrapper |
| 9 | **Hatched stripe, perforation, gradients, masks** need painting code. | `canvas` painters, S each; exact look kept |
| 10 | **Editor differences from CodeMirror**: no word-level diff strike-through (the design did not want it), selection and scrolling feel native; 50k-line files open slower than the 300ms target and cost memory. | Lazy load for large files; measure again on an idle machine |
| 11 | **Terminal is hand-built** (xterm.js gave selection, links, search, IME, screen-reader mode for free). | Budget L; it is the biggest UI risk. All behaviours of 5.6 stay required |
| 12 | **Windows High Contrast** is not detected. | Own detection + third token set (section 3) |
| 13 | **Scrollbars** default to fade-out. | Use `Always` to match the mockup |
| 14 | **Windows tray** is not in gpui. | `tray-icon` crate or own 150 lines |
| 15 | **"Reload replays the terminal log"**, CSP, `dangerouslySetInnerHTML`: web-only; they disappear. | The safety rule stays as "other people's text is never markup": the `safe_markdown()` wrapper and its test |

**Questions for the owner (2).**

1. *Small scale and tilt effects (item 1).* gpui cannot scale or tilt a panel. **A (recommended):** keep fade, rise and shadow; drop the 1.5% scale on dialogs, the 2% on popovers and the tilt on the dragged row. Hard to notice, no cost. **B:** keep them exactly; this needs a patch to the vendored gpui to draw a subtree into a texture, about 1 to 2 weeks, and it must be redone at each gpui update.
2. *macOS menu-bar item.* The old spec (ui-spec 7.12) listed a menu-bar menu "if the framework supports it"; ui-design.md 4.4 uses only the Dock on macOS. **A (recommended):** Dock on macOS, tray on Windows, exactly as ui-design.md says. **B:** also add the macOS menu-bar item with the sign-in list; about 3 days, one more thing to test.

**Settled 2026-11-17** (`design.md` section 13, item 5): question 1 is option A. Question 2 is moot: the menu-bar and tray item exists today (`internal/app/tray.go`, setting `tray.enabled`, off by default), so it stays on both platforms.

## 8. Proposed text for ui-design.md

All requirements stay. These are additions and wording swaps only.

**8.1 Header line.** Insertion point: directly under the title block at the top of the file, before `## 0.` Add:

> **Implementation (owner, 2026-11-17).** The app is built in Rust with gpui through GPUI Kit. This document's requirements do not change. Where it uses web words (a CSS class, an ARIA role, CodeMirror, xterm.js), read them as the name of the part or the behaviour; section 7a says what each becomes.

**8.2 New section.** Insertion point: after the last bullet of section 7 (the line that starts ``- `dangerouslySetInnerHTML` stays at zero uses``) and before the `---` that precedes `## 8. Not drawn, and open points`. Add:

> ## 7a. Native implementation (Rust, gpui, GPUI Kit)
>
> Section 7 described the web frontend. From 2026-11-17 the UI is native; what the owner sees and can do is unchanged.
>
> - **Tokens.** `docs/design/tokens.css` and `mockup.css` stay the source of every value. The Rust module `ui::tokens` mirrors them (one field per token, light and dark), and a test fails when they differ. GPUI Kit's theme is filled from it.
> - **Components.** Every class family named in section 2 is one Rust component with the same name (`.sync-n` is `SyncN`, `.pullall*` is `pullall::*`). `docs/design/components.md` lists class, Rust type and whether it is a GPUI Kit component or our own.
> - **Editor.** GPUI Kit's code editor with tree-sitter highlighting from `--syn-*`; diff and conflict views are GPUI Kit's diff component with the `.dl` look. The language server talks to the editor through its completion, hover and definition providers.
> - **Terminal.** `alacritty_terminal` for emulation and our own painter and input. Every behaviour in 5.6 is required as written.
> - **Markdown.** One wrapper, `safe_markdown()`: raw HTML off, no image is ever loaded, links go through the https-only opener. A test forbids any other use of the Markdown or HTML view. This replaces the `dangerouslySetInnerHTML` rule.
> - **Accessibility.** ARIA roles and properties map to AccessKit roles and properties of the same name. A "live region" is a call to `announce(text, polite | assertive)`, which uses the platform's announcement (macOS announcement notification, Windows UI Automation notification event). The rules about what is announced and when are unchanged.
> - **Keyboard.** A grid, tree, menu or tab row is one Tab stop; arrow keys move inside it. Shortcuts are actions bound per context; the terminal context takes only the keys listed in 4.3.
> - **Motion.** Section 3 holds, with one limit of the toolkit: ordinary elements cannot be scaled or rotated. Dialogs and popovers keep their fade and movement without the scale; the dragged row keeps its shadow and ring without the tilt.
> - **Strings.** The i18n keys of section 7 are kept as they are, in the Rust catalogue.
> - **Mock mode and shots.** `codepass --mock <scenario>` runs on fake services with the generic data of section 7 (scenarios `fresh`, `mid`, `done`, `empty`, `edge`); the scripted terminal and language server stay. `codepass --mock <scenario> --shots <dir>` writes the shots of section 9 under the same names. New screens are still drawn in the HTML mockup first.

**8.3 Per-section notes.**

| Where (exact insertion point) | Text to add |
|---|---|
| Section 2, after the paragraph that starts "Reused unchanged: `.app`, `.nav` ..." | "Native build: class names are component names (7a). Values are unchanged." |
| Section 3, Rules, after the bullet "Pull all adds no ambient loop ..." | "- Native build: 'only `opacity`, `transform` and grid-row size are animated' reads 'only opacity, position and height are animated'. Scale and rotation are not available for panels (7a)." |
| Section 4.2, end of the **Header** bullet (after "... on Windows the system title bar sits above and the reserve is dropped.") | "The traffic lights are placed by the window's title-bar option; the header is the drag region." |
| Section 4.4, replace the first paragraph's second sentence ("The default menu of the framework is not used, because ...") | "There is no framework default menu; every item below is an app action, so the menu, the shortcut and the palette run the same command." (The rules for Cmd+W and Cmd+R below it stay.) |
| Section 4.4, bullet "Closing the window while AI tasks run", after "or the tray (Windows)" | "(the tray icon is the app's own; its menu has Open Codepass and Quit Codepass)" |
| Section 4.4, bullet "Unsaved edits survive a crash or reload" | change "crash or reload" to "crash"; the rest stays |
| Section 5.6, the **Terminal** paragraph: replace the words "xterm.js on `--term-bg`" | "The terminal view on `--term-bg`" (everything after it stays) |
| Section 6, **System notifications**, after "The banner is drawn by the operating system, so only the words are ours." | "On macOS a notification needs the installed, signed app; a development run shows the in-app toast instead (the 'notifications unavailable' rule below)." |
| Section 6, bullet **Minimum window** | no change in values; add "The layout rules are applied from the window size by the views." |
| Section 7, title | rename to "7. Implementation notes for the web frontend (replaced by 7a)" and keep the i18n and mock-data bullets, which 7a refers to |
| Section 9, after the "To re-take one:" line | "Native shots: `codepass --mock mid --shots docs/design/native-shots` (same names, 1440x900). Compare with `shotdiff docs/design/codepass-mockup/shots docs/design/native-shots <report-dir>`." |
| `docs/design/ui-spec.md` section 10, bullet "Windows High Contrast (`forced-colors`)" | add "Native build: the app reads the system high-contrast setting and switches to a high-contrast token set; identity colours are kept." |
| `docs/plan/codepass/product-plan.md` section 5 table, rows "Terminal UI", "Code editor", "Diff data", "CSP" | belongs to the architect's proposal; this file only notes that the UI column of those rows becomes: own terminal view on `alacritty_terminal`; GPUI Kit editor; GPUI Kit diff; CSP row removed |
