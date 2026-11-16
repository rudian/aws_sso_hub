# Group H wiring notes (F1-CLa, F1-CLb)
For the milestone 1 gate, A2-CL and the milestone 2 tasks that build on this code (A2-06, A2-14, A2-16). Follow-ups FU-CLa1 to FU-CLa4 and FU-CLb1 to FU-CLb7 are in tasks.md.

Group H was F1-CL (the milestone 1 clean-up) run as two parallel tasks split by file. Merged 2026-11-17 on top of `a95687a`: `0ab5df7` (F1-CLa, squash, 106 files, +2957/−319) and `86e82d4` (F1-CLb, squash, 58 files, +1425/−1283). Each task got one full review (F1-CLa: four minors, eight nits; F1-CLb: six minors, four nits) and one fix round. Verification approved both: F1-CLa with four follow-ups, F1-CLb with five. The merge needed no integration fix. Gate after the merge: 63 test files, 841 tests.

## F1-CLa (Go, scripts, docs)

### What it did
- It fixed all 56 items of its list and declined none. FU-22e is only partly done (see deviations).
- FU-22f: the new package `internal/awsguard`. In a test binary, every AWS client sends its requests through a transport that refuses any host that is not loopback. That covers SSO OIDC and the portal, STS, IAM, CodeCommit and the console sign-in request. The refusal happens before a connection opens, and the refused host is recorded (`awsguard.Refused()`). A production binary is unchanged: `Client()` returns nil and `Transport(nil)` returns nil.
- FU-22e: `browser.Isolated.Focus` on Windows restores the browser's own window and brings it forward, without asking for any permission. On macOS it returns `ErrFocusUnsupported`, and `focusSignIn` opens the page again as before.
- FU-17c and FU-17d: `api.Settings.UpdateCheckOff` replaces `<data>/update/update-prefs.json`. Its zero value means checks are on.
  - `NewContainer` moves an old file once, in `migrateUpdatePrefs`, right after the settings open.
  - A damaged old file is set aside with Quarantine. A file from a newer version is left alone.
  - `UpdateService.SetAutoCheck` and `State` keep their API.
- FU-15e: `ToolService.StartToolUpdate` runs `brew upgrade` or `winget upgrade`. It can be cancelled and sends install's progress events, with step id `<tool>:update`.
  - ToolRows shows Update only for a git or AWS CLI that is outdated or too old and sits in a copy Homebrew or winget owns.
  - Every other row keeps How to update or Install, which open the official page.
- FU-16a: `E_CONFIG_SHARED` refuses a per-repository identity write when that config is shared with another member that has its own identity, for example a linked worktree.
- The rest of its list:
  - FU-22a, FU-22c and FU-22d;
  - FU-21a to FU-21c (the quit races): a launch under way now finishes and is then closed, and Shutdown waits for closes in flight within one 8 s budget;
  - the scrub check (FU-07a to FU-07c) and the release scripts (FU-10a);
  - the Windows items (FU-03c, FU-03d, FU-06b, FU-20b);
  - the store, watch, netx, execx, repos, projects, identity and update items.

### Facts for later tasks
- Nothing new is registered in `main.go`. `ToolService.StartToolUpdate` is on the already registered ToolService.
- Every reader of `tools:install-progress` now also sees one-step runs with step id `<tool>:update`. That includes SetupPage. These runs start only from Health.
- ToolRows calls `Tools.GetInstallState()` on mount and keeps only runs with an `:update` step. Every mock or test that mounts ToolRows needs that method; the `system.ts` mock has it.
  - Its props are unchanged: `reloadKey` and `compact`, both optional.
  - When an update finishes and focus has fallen to `<body>`, ToolRows moves focus to its own list (`tabIndex -1`).
- Mock: `world.ts` `baseSettings.updateCheckOff` is false. The mock `UpdateService.SetAutoCheck` writes it and emits `settings:changed`, like the Go service.
- FU-14h adds `identity.Deps.OnRecordData` and `repos.Engine.ForgetMovedOffers`, wired in `NewIdentityService` (`identityservice.go`, not `container.go`).
- FU-11f adds `repos.Deps.WatchRetryBase` (0 means 5 s). The first stop of a watch is still answered at once. Further quick stops back off, doubling up to 5 min.
- FU-18e adds `importedLegacy` to `projects.json` (omitempty: the imported 0.0.1 path hashes). The schema version is unchanged.
- A new AWS client must take `awsguard.Client()` (the SDK's `HTTPClient` option) or `awsguard.Transport(next)`. Its guard test should inspect the client before it sends any request (FU-CLa1).

### Deviations (accepted)
- FU-22e is done on Windows only. This is reasoning; nobody has tried it on a real Mac:
  - Bringing one browser process forward among several of the same browser needs Accessibility or AppleScript to System Events. Both are ruled out.
  - Since macOS 14, `NSRunningApplication.activate` works only while the caller is the active app, and a notification button press is not.
  - `open -b` would raise whichever instance Launch Services picks, possibly the owner's own browser.
  - The Windows half has not run yet (B19 step 4).
- FU-02d is documented ("`Load` needs a fresh value") instead of zeroing the value, because callers fill in defaults before `Load`.
- FU-17d accepts both `update-prefs.json` and `prefs.json`. **Orchestrator's correction:** both names come from Codepass's own F1-17 (group E). Neither 0.0.1 nor 0.0.2 ever wrote them, so the move matters only for test builds made since group E. A 0.0.1 or 0.0.2 settings file reads as "checks on".
- FU-12d also corrected the Git minimum (2.44.0) in `docs/guide.md` and `docs/guide.zh-CN.md`. A test checks all four documents.
- `scripts/scrub-check.test.sh`: the case "the repository passes its own check" now tests only an invented word, because the real docs name the AI vendor.
- `build/release/test.sh`: `rel()` accepts `RELSCRIPT=path`. The stale-tag case uses a scratch copy of the scripts, so no tag in the real repository is touched.
- `frontend/bindings`: wails3 moves some doc comments between neighbouring enum constants from one run to the next. A regeneration may therefore show comment-only differences in `models.ts`.
- FU-17f: the comment now says the test checks only the pairs it lists.
- One commit in the middle of the series (FU-19c) did not compile its test. The squash merge hides it.

## F1-CLb (frontend)

### What it did
- **The owner's Projects search** (plan 4.4, design 5.2, check C25):
  - The ranking is in `frontend/src/lib/repoSearch.ts` (`rankRepo`, `searchRepos`, `matchRange`). These are pure functions with their own tests. The order is:
    1. the folder name starts with the text;
    2. a word inside the folder name starts with it;
    3. the text appears anywhere in the folder name;
    4. the remote name, the CodeCommit name, then the path.
  - Rows keep their project grouping, with better matches first inside each project.
  - Sections stay open while a search runs. A section folded during a search stays folded only until the search ends.
  - Matched letters are bold in `RepoName` through a new `query` prop, including the `.remote-name` tag.
  - Keys:
    - Cmd/Ctrl+F focuses the field.
    - Down and Up move the selection.
    - Enter opens the selected repository, which is the best match until another is chosen. It opens it the way the row does: until A2-06, that reveals the folder.
    - Cmd/Ctrl+Enter does the same for now.
    - Esc clears the text; a second Esc leaves the field.
    - Keys are ignored while an input method is composing, such as Chinese pinyin.
  - Lines under the field:
    - "N repositories match · ⏎ opens X".
    - With no match: Clear search, plus the count of matching hidden repositories with Show.
    - With a filter on: a separate "No matching repositories" line.
- **Mounted:**
  - UpdatesCard in Settings, below the menu bar and terminal card. F1-22's notifications card stays at the top.
  - UpdateBanner once, under the Projects header.
  - ToolRows in Health's runtime card, in place of EnvChecks' AWS CLI and Git rows. EnvChecks takes `hideTools`, and the Health re-check also reloads ToolRows.
- **Deleted:** `pages/repos/ReposPage.tsx`, `RepoRow.tsx`, the `CloneModal.tsx` re-export, the page-only tests, and the unused locale keys `placeholder.*`, `git.title`, `git.subtitle` and `clone.folderName`.
- **FU-22b and FU-Wa (one design):** `useActions().requestLogin(portal)` returns `'opened' | 'queued' | 'running'`.
  - A login request for another portal while a login modal is open is queued: in order, with no duplicates. It opens when the modal closes, and an in-window toast says so.
  - A request for the same portal returns `'running'`.
  - Login requests are ignored while the tools gate is closed. `?open=login` is honoured after setup.
- The rest of its list: FU-08a, FU-14a to FU-14g, FU-14i, FU-15a to FU-15c, FU-16b, FU-16e, FU-16f, FU-17a, FU-17b, FU-17h, FU-19a, FU-19b, FU-Wc and FU-Wd.

### Facts for later tasks
- A2-16 imports `rankRepo`, `searchRepos` and `matchRange` from `frontend/src/lib/repoSearch.ts`. `RepoName` takes `query` for the bold letters.
- Anything outside the window that asks for a login uses `useActions().requestLogin(portal)`, not `openLogin`.
- A queued sign-in's toast is not seen while the window is hidden in the tray, because the notification button deliberately does not show the window. If C23 shows this matters, the Go click handler would have to show the window before it emits `app:navigate` login.
- Identity mock repository IDs are `repoIdOf(name)`, exported from `api/mock/identity.ts`. The identity mock also moves the repository in the repos mock on ApplyMove and Undo.
- A new mock URL parameter, `?search=<text>` (`lib/initialUI.ts`), shows the search for screenshots.
- `pages/repos/RepoSentence.tsx` names a repository by its folder name in the bind popover and the HTTPS modal.

### Deviations (accepted)
- FU-14a: the remove-project confirm is reworded to backup-only (`projects.remove.confirm.backup`), because undoing only the identity writes would not bring the project back. Setting a project's identity gets a real Undo toast (`projects.identity.done`).
- FU-22b: a request is queued and opens after the modal closes, with a toast. There is no binding that shows the window.
- Cmd/Ctrl+Enter equals Enter until milestone 2's new-task sheet exists.
- The FU-15c regression test passed before the fix too, so it is only a guard.
- The search field is at most 320 px wide (`repos.css`), and `.update-banner` has a bottom margin (`update.css`).

## Leftovers found after the merge
The two tasks ran at the same time, so neither saw the other's change:
- The mock `repos.ts` `ClonePreview.command` still lacks the destination that Go now adds (FU-19c): FU-CLb6.
- `E_CONFIG_SHARED` (F1-CLa) comes from `PreviewRepo` as a plan error. `RepoPopover` (F1-CLb) shows every plan error as "cannot write this config" with Retry, so the sharing explanation never appears and Retry cannot help: FU-CLb7.
- Neither task took a screenshot of Health with ToolRows mounted. The gate takes it.

## Follow-ups (in tasks.md, all A2-CL)
- FU-CLa1 (nit) the FU-22f tests for the CodeCommit factory and `signInClient` send a real request.
- FU-CLa2 (nit) `closeSignInWindows`: one 8 s deadline can leave every Close with an expired context.
- FU-CLa3 (nit) ToolRows: disk full and restart needed show as "the installer reported an error"; a straight apostrophe.
- FU-CLa4 (nit) ToolRows: the Update button drops focus while starting; the focus effect also runs on mount.
- FU-CLb1 (minor) no test for `targetHasIdentity` in `createProject`.
- FU-CLb2 (nit) `applyProjectIdentity` returns void, so a failed identity save still reads as "keep their own identity".
- FU-CLb3 (nit) per-row `aria-label` uses the CodeCommit name (`BulkBindModal`, `UrlRuleModal`).
- FU-CLb4 (nit) `identity.test.ts`: a stale test name and a loose `.rejects` check.
- FU-CLb5 (nit) about 24 unused `repos.*` keys and the `.repo-table` rules.
- FU-CLb6 (nit) the mock clone command lacks the destination.
- FU-CLb7 (minor) `RepoPopover` hides the `E_CONFIG_SHARED` explanation behind "cannot write this config" and Retry.

## Manual checks (for the owner, in `manual-checks.md`)
- C25 is rewritten to match the search as built: ranking, bold letters, Down, Up, Enter and Cmd+Enter, Esc twice, Chinese input, and the two lines.
- C27 (new): Health → Update on an outdated Homebrew git or AWS CLI. This runs a real `brew upgrade`.
- B19 step 4 (new): on Windows, "Sign in again" brings the sign-in browser forward.
- B15: recycling from a drive without a Recycle Bin; the old app is still found.
- B14: a SOCKS-only proxy on Windows.
- B16: the new Windows test packages.
- B11: "ask IT" appears only after Check again.
- C17: Esc returns focus in the bind popover, the repository popover and the update dialog.
- C18: the card in Settings, the banner, and a 0.0.1 or 0.0.2 copy reading as "checks on".
- C23 step 3: the second "Sign in again" is queued.
