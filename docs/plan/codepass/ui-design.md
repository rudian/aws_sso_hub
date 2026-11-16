# Codepass v1.0 - UI/UX design

Date: 2026-11-16 (revision 3, after the milestone-0 gate; revision 2 followed the principal engineer's review). Author: UI designer and planner (Fable). Audience: implementers (Sonnet), reviewers (Opus), the owner.
Extends `docs/design/ui-spec.md` (the "Keycard" language). Everything in that spec still holds unless this document changes it.

Companion files:

| File | What it is |
|---|---|
| `docs/design/codepass-mockup/index.html` | Clickable static mockup. `index.html?screen=<id>&theme=light\|dark&lang=zh-CN\|en`. Extra switches: `&os=win`, `&state=...` (setup: `idle`, `failed`, `noadmin`, `done`; projects: `drag`, `found`; identity: `page`; update: `banner`; ai-tasks: `leftover`), `&panel=find`, `&bar=0` (hide the mockup switcher). `[` and `]` step through screens. Screens are functions in `core.js`, `s-pages.js`, `s-ws.js`, `s-git.js` and `s-m0.js` (the five screens added at the milestone-0 gate). |
| `docs/design/codepass-mockup/tokens.css`, `components.css` | Unchanged copies of `frontend/src/styles/`. |
| `docs/design/codepass-mockup/mockup.css` | Only what is new. Treat it as the draft of the new CSS; class names in this document are from it. |
| `docs/design/codepass-mockup/shots/*.png` | 1440x900 renders (list at the end). |

Screen ids: `setup`, `projects`, `workspace-code`, `workspace-changes`, `repo-commit`, `workspace-ai`, `new-task`, `ai-tasks`, `ai-leftover`, `free-space`, `session-switcher`, `quit-dialog`, `rename-folder`, `repo-history`, `repo-branches`, `repo-stash`, `repo-merge-conflict`, `repo-prs`, `pr-detail`, `accounts`, `add-account`, `identity`, `update`, `settings`.

---

## 0. Decisions and review changes applied in this revision

`product-plan.md` (revision 2) and this document now agree. The owner's decisions: the name **Codepass** in both languages; macOS Apple silicon with macOS 15+, and Windows 10 1809+ / 11 amd64; Homebrew (macOS) and winget (Windows) are required; Claude Code, Codex and OpenCode, each installed on first use; six workspace tabs with **Code first, as a full editor for the whole repository** with code intelligence for Go and TS/JS; a comprehensive git client; pull requests in the app; a motion system; Developer ID signing with notarization; one release, v1.0.0.

What the review changed in the design (details in the sections named):

| Change | Where |
|---|---|
| Mock data, tests, tutorial and screenshots use generic names only (acme, globex, initech, `111122223333`, `@example.com`). Real names never reach the public branch. | all screens, 7 |
| The setup gate blocks only when a tool is missing or below the hard minimum. It never needs the network to pass. New failure states. | 5.1 |
| The app has its own menu. Cmd+W closes an editor tab, Cmd+R never reloads the page, closing the window while tasks run hides it. | 4.4 |
| The sync button follows the selected folder ("Publish branch" for a task that was never pushed). Editor tabs are kept per folder. | 4.2 |
| An AI task runs with the user's rights, and the UI says so. The "AWS_PROFILE not passed" badge is gone. The app never draws inside the tool's own screen: its line sits in a bar above the terminal. | 5.6 |
| "Merge into main" updates the task first, then moves the base forward. | 5.5 |
| Force push names the exact commits it replaces, counted from the last commit the user integrated. | 5.8, 6 |
| Operations that can conflict start from a clean folder, so "Abort" is always safe to promise. | 5.9 |
| Discarded changes can be brought back for 14 days. | 5.4 |
| Text from other people (pull requests, release notes, hover docs, commit messages) is shown as text or safe Markdown, without remote images. | 6 |
| Update dialog: "Reopen when no task is running"; release notes come from the signed file. | 5.13 |
| "Hide repository" and a one-time review of newly found repositories. | 5.2 |
| Comment colour raised to pass 4.5:1; line numbers declared decorative. | 2 |
| Large files are read-only above a fixed limit. | 5.3 |

What the milestone-0 gate changed (revision 3):

| Change | Where |
|---|---|
| A repository has two names, the local folder name and the remote name. Both show wherever they differ. Clone under a custom name; "Rename local folder". | 5.2, 5.2a |
| Session switcher on Cmd+J from any screen. | 4.3, 5.7a |
| "Closed, still running" rows with Force stop. | 5.6a, 5.7 |
| The quit dialog asks every time: stop all, or keep running in the background. | 4.4, 5.6b |
| "Free up space" replaces "Clean up finished tasks": sizes per part, every path listed before anything is deleted. | 5.7b |
| Setup: no "install into my own folder" button; "verify signature" comes before the password prompt; three new failure rows. | 5.1 |
| A tool that sends no signals shows "Running (no signal)". | 5.6 |
| TypeScript uses the TypeScript 7 native server first; at most 2 language servers by default. | 5.3 |
| Add account: only the classic GitHub token with exact scopes; Bitbucket workspaces come from the token. | 5.11 |

Carried over from AWS SSO Hub 0.0.2, and the notification addition (2026-11-16):

| Change | Where |
|---|---|
| No session-length control. An IAM Identity Center administrator sets the length; the app only shows the end it learns from AWS. The old setting is gone and is not redrawn. | 5.11 (AWS passes, as in 0.0.2) |
| The Console button shows a busy state while the console opens. | 5.11 (AWS passes, as in 0.0.2) |
| Each sign-in opens in its own fresh private window, in a separate browser instance. The texts that say so are taken from 0.0.2 unchanged. | 5.11 (AWS passes, as in 0.0.2) |
| New: system notifications. A heads-up before a sign-in expires and a notice when it has expired, each with "重新登录 / Sign in again". One table lists every notification the app sends. | 6, 5.14 |

Milestone 8 (2026-11-16; plan 4.16 and 4.17). Specified in words and text sketches; not drawn in the mockup yet (section 8):

| Change | Where |
|---|---|
| New page **Tickets**: the owner's three columns (编辑中 / 暂不执行 / 待执行), the queue, the finished list, the editor, quick capture from any screen. | 5.15, 4.1, 4.3, 4.4 |
| The **queue bar** always says what the queue is doing and why it is not doing more: checking usage, running, waiting for you, waiting for a reset. | 5.15 |
| **AI accounts**: several accounts per tool (Claude Code, Codex) on the Accounts page, added through the tool's own sign-in. Codepass stores no AI sign-in. | 5.16, 5.11 |
| **Usage meter**: a chip in the sidebar and a popover with every account's windows, reset times and the usage-credits note. | 5.16, 4.1 |
| **Limit banner** on a session's bar: which limit, when it resets, "在 B 上继续 / Continue on B", "等待 / Wait". The session bar also names the account, and the account chip is the "换账号 / Switch account" menu (next table). | 5.6 |
| Four queue notifications replace the placeholder row (a fifth, for an account that needs attention, came with the third batch). | 6 |
| Queue settings card. | 5.14 |

Choosing and switching the AI account (2026-11-16, the owner's addition to milestone 8; plan 4.16 decisions 13 to 21). Words and text sketches only:

| Change | Where |
|---|---|
| **Any account for any task.** The New task sheet's account row is pre-selected from a default and can be set to any account of the tool. The old rule "the first account in the owner's order that has room" is now only the fallback when the default is used up, and the sheet says so in one line. | 5.6 |
| **Defaults in three levels per tool**, like the author identity: repository › project › app (the first account in the order). Set in the Repository settings sheet, in the project section's menu, and by the order on the AI accounts page. | 4.2, 5.2, 5.16 |
| **Switch any time, both ways.** The account chip on the session bar is a menu, "换账号 / Switch account"; the AI tasks page rows have the same menu. After an automatic switch (only when the owner turned automatic switching on) a quiet hint offers the way back; nothing switches back by itself. | 5.6, 5.7 |
| The account shows in session switcher rows and AI tasks rows. A ticket's account is "跟随仓库 / Follow the repository" or a fixed one. An account's pass says where it is the default, and the remove preview lists those places. The "默认 / Default" badge on the tool's own folder is gone; line 2 of that pass says "工具自带 / Tool's own", so "默认 / default" has one meaning. | 5.7, 5.7a, 5.15, 5.16 |

Keeping the conversation, and staying inside the terms (2026-11-16, the owner's third batch; plan 4.16 decisions 22 to 36). Words and text sketches only:

| Change | Where |
|---|---|
| **A switch never starts over without the conversation.** The confirm says what comes along, the cost, and, when it applies, another model, a conversation longer than the other account's window, and a turn cut off at the limit. One result line per layer: the whole conversation, a repaired copy (naming what was left out), or a handoff note with "查看笔记 / View note". The old "could not be continued, start a new session" text and the "ask Work for a summary" checkbox are gone. | 5.6 |
| **Automatic switching is off by default**, per tool, and asks once before it turns on, with the sentences of the terms that bear on it. With it off, the limit banner, the switch menu, the queue bar and the queue's notice still switch with one click. | 5.16, 5.6, 5.15, 6 |
| **"需要你处理 / Needs your attention"**: a new account state for an authentication, billing, suspension or policy error. The app stops using the account and never offers another account in its place. | 5.16, 5.6, 5.15, 4.1, 6 |
| **Add account**: the consent now covers conversation copies; one line for a Team or Enterprise seat; one plain sentence about what Codepass cannot promise. | 5.16 |
| **Settings**: "Codepass 只用于你自己的账号和你自己的工作。 / Codepass is for your own accounts and your own work." with links to both companies' terms. | 5.14 |
| **Free up space** lists repaired conversation copies and handoff notes, per task, by exact path. | 5.7b |
| Several tasks on different accounts at the same time were already possible; the design now says so where the owner looks for it. | 5.6, 5.16 |

Finding a repository fast (2026-11-16, second batch; plan 4.4):

| Change | Where |
|---|---|
| The Projects search ranks by the local folder name, bolds the matched letters, and is driven by the keyboard: Cmd/Ctrl+F, Down / Up, Enter opens the repository, Cmd/Ctrl+Enter opens it with a new task. | 5.2, 4.3 |
| New: **Open repository** (Cmd/Ctrl+O) from any screen, a panel like the session switcher: recent first, type a folder name, Enter. Not drawn yet (A2-16 is "Fable first"). | 5.7c, 4.3 |

---

## 1. What stays, what changes

**Stays.** Neutral ink chrome on porcelain / slate. Saturated colour only for account identity (8 hues) and status. Identity stripe, monogram tile, perforated action row. Radius hierarchy 16 / 12 / 8 / 6 / 4. Voice: calm, specific, short; no exclamation marks; say what happened and what to do next. Full-width punctuation in Chinese with a half-width space between CJK and Latin or digits. Sentence case in English.

**Changes.**

1. **Two window layouts.** Sidebar layout for the management pages; **workspace layout** (icon rail + full-width header + status bar) once a repository is open. One click goes back.
2. **Three new surfaces need colour, so three rules are added.**
   - *Syntax colours* (`--syn-*`) are low-chroma. An identity chip or a status dot must stay louder than any token in the editor.
   - *Git status in trees and lists* reuses status hues (`--git-mod` = info, `--git-add` = ok, `--git-del` = danger) and is always paired with a letter (M, A, D, U, C).
   - *The terminal* is one dark surface in both themes (`--term-*`). The tools inside draw their own colours; a light terminal would fight them. A setting can switch it to follow the app theme.
3. **Provider marks.** A small monochrome glyph (cloud for CodeCommit, the GitHub and Bitbucket marks) tells providers apart. Colour still means "this account", never "this provider".
4. **Tool marks.** Claude Code, Codex and OpenCode each get a neutral glyph in a 22px ink tile (`.tool`). No vendor colours or logos.
5. **Words.** New nouns in primary UI: 项目 / project, 工作区 / workspace (a folder a task or you work in), 任务 / task, 贮藏 / stash, 拉取请求 / pull request, 密钥 / key, 令牌 / token (allowed now, because the user creates one by hand). Still kept out of primary UI: worktree, PTY, LSP, HEAD, index, caffeinate. "Worktree" is always "独立工作区 / own workspace".

---

## 2. New tokens and classes

Defined in `mockup.css`. Existing tokens are untouched.

| Token | Light | Dark | Use |
|---|---|---|---|
| `--rail-w` | 52px | | icon rail |
| `--ws-head-h` / `--ws-tabs-h` / `--status-h` | 46 / 40 / 26px | | workspace frame |
| `--syn-kw` `--syn-str` `--syn-num` `--syn-com` `--syn-type` `--syn-fn` | #7440B6 #10775B #9B541F #687180 #0B6C86 #2A4BB8 | #C3ABFF #7FDDBF #F0BC92 #838D9C #85DAEC #A9BCFF | editor, diff, hover, peek |
| `--editor-bg` `--editor-gutter` `--editor-line` | #FFFFFF #9AA3AF #F4F6F9 | #181C22 #5B6470 #1F242C | editor |
| `--git-mod` `--git-add` `--git-del` | info / ok / danger | same roles | trees, gutters, counts |
| `--term-bg` `--term-fg` `--term-dim` `--term-line` + ok / warn / info / danger / accent | #151A21 ... | #0E1115 ... | terminal, setup log |
| `--dur-page` `--dur-panel` `--dur-list` | 200 / 220 / 180ms | | section 3 |
| `--ease-enter` `--ease-exit` `--ease-move` | (.2,.8,.2,1) (.4,0,1,1) (.3,.7,.2,1) | | section 3 |

All syntax and git colours are at least 4.5:1 on `--editor-bg` in both themes (same method as the identity palette). Comments are `#687180` in light (4.9:1 on white, 4.55:1 on the current-line tint) and `#838D9C` in dark (5.1:1); the first draft's values failed the rule and were replaced. **Line numbers (`--editor-gutter`) are decorative**: 2.55:1 in light and 2.85:1 in dark. They repeat information the status bar gives as text ("行 27，列 32"), so they are exempt from the text-contrast rule; the current line's number uses `--text-2` and does pass.

New class families (stable names): `.ws*` (frame), `.rail__btn`, `.tree-switch`, `.splitbtn`, `.pane*`, `.tree`, `.fl` (file list), `.ed*` (editor), `.ln` (code line), `.hover-card`, `.peek`, `.ac` (completion), `.outline`, `.bpanel` (bottom panel), `.found`, `.diff`, `.hunk`, `.dl` (diff line), `.composer`, `.src-tag`, `.taskbar`, `.task`, `.state`, `.task-head`, `.term*`, `.sheet*`, `.opt`, `.tool-pick`, `.commit`, `.ref`, `.who`, `.refs`, `.cmp`, `.cf*` (conflict), `.opbar`, `.pr-*`, `.checks`, `.tl` (timeline), `.mergebox`, `.prov`, `.tool`, `.proj*`, `.suggest`, `.ai-badge`, `.awake`, `.setup*`, `.tool-row`, `.keyline`, `.pubkey`, `.howto`, `.verify`, `.ident*`, `.ladder`, `.affect`, `.update-banner`, `.ver`, `.usteps`, `.set-row`.

Reused unchanged: `.app`, `.nav`, `.keyring`, `.btn*`, `.icon-btn`, `.card`, `.pass`, `.portal*`, `.acct-*`, `.badge`, `.dot`, `.banner`, `.field`, `.input`, `.select`, `.checkbox`, `.radio`, `.switch`, `.modal*`, `.menu`, `.toast`, `.progress`, `.spinner`, `.empty`, `.table`, `.row`, `.tabs`, `.segmented`, `.stepper`, `.kbd`.

---

## 3. Motion

The owner asked for tasteful animation. `ui-spec.md` section 9 said "no entrance animations"; that line is replaced by this section. The rule that survives: **motion explains a change of place or state; nothing moves to decorate.**

| Change | Motion | Duration / easing |
|---|---|---|
| Page switch (sidebar), workspace tab switch | new content fades in and rises 6px; old content is removed at once | `--dur-page` 200ms, `--ease-enter` |
| Enter / leave the workspace | sidebar narrows 232 -> 52px while labels fade; header slides down 8px | 220ms, `--ease-move` |
| Modal | existing: fade + 8px rise + scale .985 | 240ms |
| Side sheet | slides in 24px from the right with a fade; backdrop fades | `--dur-panel` 220ms; out 160ms `--ease-exit` |
| Bottom panel (terminal, problems, find) | height grows from the status bar; content fades after 60ms | 220ms |
| Popover, menu, hover card, completion list | fade + 4px drop + scale .98 from the anchor edge | 160ms; hover card waits 350ms before it shows |
| Project / portal / file-list group expand and collapse | height (grid rows 0fr -> 1fr) + fade; chevron turns 90 degrees | 220ms / 160ms |
| List insert (new task, new commit, staged file) | row fades in and drops 4px; siblings slide to make room | `--dur-list` 180ms |
| List remove (discard, unstage, dismiss suggestion) | row fades and collapses its height | 160ms, `--ease-exit` |
| Reorder / stage-unstage move | FLIP: the row travels to its new place | 200ms, `--ease-move` |
| Drag a repo to a project | the row lifts into a ghost (shadow-3, 1.2 degree tilt, 96% scale); valid targets get a 2px focus ring within 90ms; on drop the ghost shrinks into the target header and the count ticks | lift 120ms, drop 200ms; cancel returns in 200ms |
| Toast | existing 240ms in; out 160ms; stacked toasts slide up | |
| Progress bars | width eases; never jumps backwards | 240ms |
| Task state change | dot colour cross-fades; "waiting for you" breathes at 1.6s; "running" uses the existing pulse | 160ms |
| Terminal session start | the terminal fades in after the first byte, never an empty flash | 120ms |
| Success (verify key, install done, merge done) | existing check draw | 420ms |
| Conflict block resolved | block bar turns from warn to ok; chosen lines flash the add colour once | 240ms |

Rules:

- Durations stay within 120-240ms except the check draw. Nothing blocks input: every animation can be interrupted by the next action.
- Only `opacity`, `transform` and grid-row size are animated. No layout thrash in lists of hundreds of rows: above 200 rows, insert / remove / reorder motion is skipped.
- Ambient loops are limited to three: the running pulse, the waiting breath, the login handoff dot. Each exists only while its state lasts.
- `prefers-reduced-motion: reduce` or Settings > Appearance > Motion = Reduced: every transition and animation drops to about 1ms (the existing global rule), spinners keep a slow turn, the waiting breath becomes a steady warn dot, drag keeps the ring but no lift. Motion = Full overrides the OS; Motion = System follows it.
- The terminal and the editor never animate their text.

---

## 4. Information architecture

### 4.1 Sidebar (management layout)

```
[icon] Codepass
       账号 · 仓库 · AI 编码          Accounts · repos · AI

  项目          2 待处理             Projects        2 to fix
  AI 任务       1 等待你             AI tasks        1 waiting
  工单          3 待执行             Tickets         3 queued
  拉取请求      2 待评审             Pull requests   2 review
  账号                               Accounts

  AI 用量                            AI usage
   [cc] 本次 62% · 本周 81%          session 62% · week 81%        (5.16; one line per tool in use)
   [cx] 5 小时 18% · 本周 40%        5 h 18% · week 40%

  登录状态                           Sign-in status
   * [aws] acme-prod      6:12
   * [aws] initech       48m
   * [gh]  alex-dev       密钥 / Key
   * [bb]  alex_acme      密钥 / Key

  Git 设置                           Git settings
  安全与环境                         Health
  教程                               Tutorial
  ------------------------------
  [cup] 保持唤醒中     2 个任务      Keeping awake   2 tasks     (only while held)
  设置  (dot when an update is ready) Settings
```

- Order is by daily use. Counts show work to do, never totals, and hide at zero. "AI tasks" turns warn when a task waits for the user and shows a pulsing info dot when tasks only run.
- **Pull requests** (sidebar) is every GitHub / Bitbucket repository's list in one page, grouped by repository, with the same row component as the repository tab. It is not drawn separately in the mockup; the nav item opens the per-repository screen.
- **Tickets** (`工单 / Tickets`, milestone 8) sits right after AI tasks. Its count is the number of queued tickets; it turns warn with "1 等待你 / 1 needs you" when a ticket waits for the owner, and shows "等待重置 / waiting for reset" in muted text while the queue is paused for a limit.
- **AI usage** (`.usage-chip`, milestone 8): one line per tool that has run at least once, for the account in use (the one the last session or the queue used). When several accounts of a tool have live sessions, the line shows the one closest to its limit, with "+1" for the others; the tooltip names them. Then: tool mark, then the two windows as words and numbers. 80% and above is warn, 100% is danger; colour is never the only sign, the number is always there. Unknown: "用量未知 / usage unknown" in muted text. An account that needs the owner's attention (5.16): the line reads "工作 · 需要你处理 / Work · needs your attention" in danger, with the alert icon, in place of the numbers. Click opens the usage popover (5.16). The lines hide when no AI tool is installed.
- Keyring rows gain a provider mark. A GitHub / Bitbucket row shows `密钥 / Key` when the key is verified, `未验证 / Unverified` in warn when not, `令牌过期 / Token expired` in warn when the API token fails.
- The Terminal helper page is gone. Its job moved into the workspace's terminal panel.
- The keep-awake chip (`.awake`) sits above Settings: cup icon, "保持唤醒中", task count. Click opens AI tasks. Tooltip: "有 AI 任务在运行，这台 Mac 不会自动睡眠 / AI tasks are running, so this Mac will not sleep on its own".

### 4.2 Workspace layout

```
+--------------------------------------------------------------------------------------+
| o o o [<] acme > [aws] acme_api  [主工作区 · main v]   [↓0 ↑2 推送|v] [AP acme-prod] [gear] |
+----+---------------------------------------------------------------------------------+
|rail| 代码  改动 5  历史  分支  拉取请求  AI 任务 1            [搜索文件、符号、命令  ⌘K] |
|    |---------------------------------------------------------------------------------|
|    |  tab content                                                                    |
|    |  (optional bottom panel: 终端 / 问题 / 查找结果)                                |
+----+---------------------------------------------------------------------------------+
| 终端 ⌃`   ⊗1 ⚠0   在文件中查找 ⇧⌘F                 gopls ✓  tsserver ✓  行 27，列 32  UTF-8  Go  保持唤醒 · 2 个任务 |
+--------------------------------------------------------------------------------------+
```

- **Header** (`.ws__head`, 46px, full width, drag region). On macOS the first 84px are reserved for the traffic lights; on Windows the system title bar sits above and the reserve is dropped. Contents: back button (to Projects), project > provider mark + repository name, the **working-tree switcher**, spacer, the **sync button**, the account chip (opens the bind popover, as today), the gear (repository settings sheet).
- **Working-tree switcher** (`.tree-switch`). A menu with "主工作区 · `<branch>`" first, then one row per AI task that has its own workspace (tool mark, title, branch, state). Choosing a row re-points Code, Changes, History and the terminal at that folder. While a task workspace is selected the pill turns info-tinted and the header gets a 2px info line along its bottom edge, so the folder on screen is never ambiguous. Shortcut: Ctrl+Tab cycles. Open editor tabs, unsaved edits, the selected file and the bottom panel are kept **per folder**: switching away and back restores them. Inside a task's folder the branch cannot be switched (the branch control is replaced by the task's branch chip with the tooltip "任务的工作区固定在它自己的分支上 / A task's workspace stays on its own branch").
- **Sync button** (`.splitbtn`): `↓n ↑m` and the obvious verb. `↑>0` "推送 / Push"; `↓>0` "拉取 / Pull"; both "同步 / Sync"; no upstream "发布分支 / Publish branch"; nothing to do "获取 / Fetch". The caret menu: Fetch, Pull (merge), Pull (rebase), Push, Push with lease... (confirm), Manage remotes. While running, the icon spins and the label becomes the step ("正在推送... / Pushing..."). The button always describes the **selected folder**. With a task's workspace selected and its branch never pushed, it reads "发布分支 / Publish branch" with no counts (shot `workspace-changes-light-zh`). "Push with lease" is described in 5.8.
- **Rail** (`.ws__rail`): the sidebar's destinations as 36px icon buttons with tooltips, pips for counts, the cup when keep-awake is held. The Projects button is active while a repository is open.
- **Tabs** (`.ws__tabs`): six, with counts where there is work (changed files, open PRs, tasks waiting). The quick field on the right opens the palette (Cmd/Ctrl+K).
- **Status bar** (`.ws__status`, 26px): terminal toggle; problems; context (branch, or the task folder); on the right the language servers, cursor position, encoding, language, keep-awake. Every item is a button with a tooltip.
- **Bottom panel** (`.bpanel`): Terminal / Problems / Find results. It opens under any tab, 196px by default, resizable, remembered per repository. A plain shell here is not an AI task and never holds the computer awake.
- **Repository settings sheet** (gear; `.sheet`, not drawn): account binding, remotes (add, rename, remove, set URL), author identity (follow project / follow global / own), AI defaults for this repository (tool, where tasks run, "启动 AI 任务时预先选好这个仓库的 AWS 账号 / Start AI tasks with this repository's AWS account selected", off by default. Its help line is honest: "这只是省一步操作。不论开关，任务都以你的身份运行，能使用你已登录的任何 AWS 账号。 / This only saves a step. Either way a task runs as you and can use any AWS account you are signed in to."), setup command for new workspaces. Also: "隐藏这个仓库 / Hide this repository" (5.2).
- **AI accounts in the Repository settings sheet** (milestone 8; plan 4.16 decision 14). Inside "AI defaults for this repository", under the tool, one row per tool that has more than one AI account (Claude Code, Codex; never OpenCode). It is the repository level of the three levels repository › project › app, and it reads like the author identity rows above it.

  ```
  AI 账号 / AI accounts          仓库 › 项目 › 应用，越具体的越优先 / Repository › project › app. The most specific one wins.
    [cc] Claude Code   这个仓库默认用 / This repository uses   [ 跟随项目 acme（现在是「工作」）  v ]
    [cx] Codex         这个仓库默认用 / This repository uses   [ ● 个人                           v ]
    新任务会先选好这个账号，开始前还可以换。 / A new task starts with this account selected. You can still change it before it starts.
  ```

  - Options of each select: first "跟随项目 acme（现在是「工作」）/ Follow project acme (now Work)", then every account of that tool with its colour dot and label. A repository in no project shows "跟随应用默认（现在是「工作」）/ Follow the app default (now Work)" instead. The value in brackets is what the level above gives today, so the owner sees the result before choosing.
  - Saving is immediate, with the toast "acme_agent_portal 的 Claude Code 任务以后默认用「个人」 / Claude Code tasks in acme_agent_portal now start on Personal" and Undo. Nothing on disk in the repository changes, and running sessions keep their account.
  - A tool with one account shows no row. With no tool that has two accounts, one muted line stands in: "每个工具只有一个账号。在「账号」页可以再添加。 / Each tool has one account. Add another on the Accounts page." with the link "添加 AI 账号 / Add an AI account".
  - When the shell decides the account for a tool (5.16), its row stays editable and a warn line sits under it: "你的终端环境设置了 CLAUDE_CONFIG_DIR，Claude Code 用哪个账号由它决定，这里的选择暂时不起作用。 / Your shell sets CLAUDE_CONFIG_DIR, so it decides which account Claude Code uses. This choice has no effect for now."
  - An account that is used up or not signed in can still be the default; the option shows "用完了，周一 00:00 重置 / used up, resets Mon 00:00" or "未登录 / not signed in" after the label.

### 4.3 Keyboard

| Shortcut (mac / win) | Action |
|---|---|
| Cmd/Ctrl + 1..4 | Projects, AI tasks, Pull requests, Accounts |
| Cmd/Ctrl + 5 | Tickets (milestone 8). It is the fifth number although the item sits second in the sidebar, so the four shortcuts people already know do not move. |
| Cmd/Ctrl + Shift + N | Quick capture: a new ticket from any screen (5.15). Works while a terminal has focus. |
| Cmd/Ctrl + , | Settings |
| Cmd/Ctrl + K | Palette: project, repository, task, file, command |
| Cmd/Ctrl + O | Open repository: type part of the folder name, Enter (5.7c; milestone 2). On macOS also while a terminal has focus; on Windows not inside a terminal (Ctrl+O belongs to the terminal program there). |
| Cmd/Ctrl + F on the Projects page | Focus the search: type a folder name, Down / Up to choose, Enter to open (5.2) |
| Cmd + J (mac) / Ctrl + J (win) | Session switcher: every live AI session in every repository (5.7a). On Windows, while a terminal has focus, it is Ctrl + Shift + J, because Ctrl+J is a line feed that terminal programs use for a new line. |
| In a workspace | |
| Ctrl + 1..6 (mac) / Alt + 1..6 (win) | workspace tabs |
| Cmd/Ctrl + P | Quick open file |
| Cmd/Ctrl + Shift + O | Go to symbol in file |
| Cmd/Ctrl + T | New AI task |
| Cmd/Ctrl + Shift + F | Find in files |
| Cmd/Ctrl + F / G | Find in file / next |
| Ctrl + G | Go to line |
| F12 or Cmd/Ctrl + click | Go to definition; Alt + F12 peek; Shift + F12 references |
| Ctrl + Space | Completion |
| Cmd/Ctrl + S | Save; Cmd/Ctrl + W close the editor tab (never the window while a tab is open); Cmd/Ctrl + \ split |
| Ctrl + ` | Toggle terminal panel |
| Cmd/Ctrl + Enter | Commit (in the message box) |
| F7 / Shift + F7 | Next / previous change or conflict |
| Ctrl + Tab | Cycle working trees |
| Esc | Close peek, popover, sheet, modal; in the terminal it goes to the terminal |

When the terminal has focus, only Ctrl+`, Cmd/Ctrl+1..5, quick capture (Cmd/Ctrl+Shift+N), the session switcher (Cmd+J on macOS, Ctrl+Shift+J on Windows) and, on macOS only, Open repository (Cmd+O) are taken by the app. Everything else, including Esc and Ctrl+C, goes to the process.

### 4.4 App menu, closing the window, quitting

The app installs its own menu. The default menu of the framework is not used, because it reloads the page on Cmd+R, closes the window on Cmd+W and has a Help item that loads a website into the app window.

| Menu | Items |
|---|---|
| Codepass | About Codepass, Check for updates..., Settings... (Cmd+,), Hide, Hide others, Quit Codepass (Cmd+Q) |
| File | New AI task (Cmd+T), New ticket (Shift+Cmd+N), Quick open (Cmd+P), Save (Cmd+S), Close tab (Cmd+W), Close window (Shift+Cmd+W) |
| Edit | Undo, Redo, Cut, Copy, Paste, Select all (the system items; text fields, the editor and the terminal need them), Find (Cmd+F), Find in files (Shift+Cmd+F) |
| View | Projects, AI tasks, Pull requests, Accounts (Cmd+1..4), Tickets (Cmd+5), Switch session (Cmd+J), Toggle terminal (Ctrl+`), Actual size / Zoom in / Zoom out, Enter full screen |
| Window | Minimize, Zoom, Bring all to front |
| Help | Tutorial, Illustrated guide, Report a problem (opens the repository's issues page in the browser) |

- **Cmd+W** closes the active editor tab. With no tab open it does nothing in the workspace layout and closes the window in the management layout.
- **Cmd+R** rescans repositories on the Projects page and does nothing elsewhere. The page is never reloaded by a shortcut.
- **Closing the window while AI tasks run** hides the window; the app stays in the Dock (macOS) or the tray (Windows) and the tasks keep running. The first time, a toast-style system notification (built by A2-08) says so: "Codepass 还在运行，2 个 AI 任务没有中断 / Codepass is still running. 2 AI tasks were not interrupted". With no task running, closing the window quits, as today.
- **Quit** (Cmd+Q, the menu, the Dock, the tray) with live AI sessions, or with programs a closed session left running, **always** opens the quit dialog (5.6b): "全部停止并退出 / Stop all and quit" or "留在后台继续运行 / Keep running in the background". It asks every time; there is no "don't ask again" and no setting. Unsaved files are listed in the same dialog (5.3). When the computer is logging out, restarting or shutting down, no dialog is shown (the framework cannot hold a logout open): everything is stopped and the app quits at once; unsaved edits are kept in the recovery store, and nothing blocks the shutdown.
- **Background mode.** After "Keep running in the background" the window is closed and the app stays in the Dock (macOS) or the tray (Windows) with its sessions; clicking the icon brings the window back. The app does not quit when its last window is hidden.
- **Unsaved edits survive a crash or reload.** Every unsaved change is mirrored to the app's recovery store within a second. After a crash, the file reopens with a bar: "恢复了上次没保存的修改 / Restored your unsaved changes from last time" - 保留 / 丢弃 (Keep / Discard).
- Windows has no menu bar: the same commands live in the palette (Ctrl+K) and the shortcuts above; Alt+F4 and the close button follow the rules for closing the window.

---

## 5. Screens

Each screen lists layout, components, states, interactions, accessibility and copy. Light and dark use the same structure; notes appear only where a theme needs care.

### 5.1 First-run setup (`setup`)

Shots: `setup-light-zh`, `setup-idle-light-zh`, `setup-failed-light-zh`, `setup-done-light-zh`, `setup-win-light-en`, `setup-dark-zh`.

**When.** Every launch runs the environment check. It uses only local checks (does the file exist, what does `--version` print, with a 5 second limit) and **never the network**, so a Mac without a connection whose tools are installed goes straight into the app. This screen replaces the whole window only when something is **missing or below the hard minimum**: Homebrew (macOS) or winget (Windows) not installed, git older than 2.39.1, AWS CLI older than 2.9.0. No sidebar, no way around it except quitting. A tool that is merely older than recommended (for example AWS CLI below 2.22) never blocks: it shows as an "更新 / Update" action on the Health page.

**Layout.** Two columns, centred: introduction on the left (app icon, title, lead, three notes), one card on the right (`.setup__card`): header with "n of 3 ready", one `.tool-row` per tool, the live log, the footer with the single primary button.

**Tool rows.** Icon tile, name, one line saying what it is for, state on the right.

| State | Right side |
|---|---|
| Found | ok check + "已安装 / Installed" + version (mono) |
| Missing | "未安装 / Not installed" (muted) |
| Too old | warn "版本太旧 / Too old" + found version; it will be upgraded |
| Queued | "排队中 / Queued" |
| Installing | spinner + "正在安装 / Installing"; the row tints info and shows a progress bar and the exact command with the current step |
| Failed | danger "没有装上 / Not installed" |

**Flow, macOS.** (1) Homebrew: download `Homebrew.pkg`; its size, checksum and signature are checked **before** any password is asked ("验证签名 / Verify signature"); then macOS shows its own password prompt once; under that single approval the package is copied to a protected place, checked again there against Homebrew's developer identity, and only then installed. The row shows three short steps (下载 / 验证签名 / 安装). The install step cannot report progress while it runs, so it shows one indeterminate state ("正在安装，大约一分钟 / Installing, about a minute") and then a single result line, not a live log. (2) `brew install git`. (3) `brew install awscli`. One click runs all three in order. The app turns off Homebrew's analytics for every brew command it runs, and the second note says so. **Flow, Windows.** winget must exist (if not: one button "打开 Microsoft Store 安装「应用安装程序」/ Open Microsoft Store to get App Installer", then re-check on focus). Then `winget install --id Git.Git -e` and `winget install --id Amazon.AWSCLI -e`; each raises a User Account Control prompt. The AWS CLI installer is treated as machine-wide and needing an administrator (a per-user install is offered only if the Windows test shows it works).

**States.**

- *Idle*: primary "一键安装 / Install everything"; ghost "我自己安装 / I'll install them myself" (opens the official pages and switches the footer to "重新检查 / Check again").
- *Running*: primary shows the spinner; ghost becomes "取消 / Cancel" (stops after the current step; what is installed stays). The log (`.setup__log`) shows the real output, last 5 lines, expandable to full height. Over 10 s without output: "还在下载，网络较慢时会久一些 / Still downloading. This takes longer on a slow connection".
- *Waiting for the OS prompt*: the current row says "等待你在 macOS 的窗口里确认 / Waiting for you to confirm in the macOS window" (Windows: "等待你在 Windows 的窗口里确认"). The app window dims nothing; the prompt belongs to the OS.
- *Failed*: a danger banner inside the card names the tool and the cause in plain words, the log shows the failing lines in danger colour, buttons: "重试 / Try again" (continues from the failed step), "复制日志 / Copy log", "我自己安装". Known causes get their own sentence: timeout or proxy, password prompt cancelled ("你取消了 macOS 的密码窗口 / You cancelled the macOS password window"), disk full, package signature invalid ("安装包的签名不对，已停止 / The installer's signature is wrong, so nothing was run"), company policy blocks installs.
- *Failure states with their own sentence and next step* (each keeps "复制日志 / Copy log" and "我自己安装 / I'll install them myself"):

  | Cause | Title (zh / en) | Next step |
  |---|---|---|
  | Not an administrator (shot `setup-noadmin-light-zh`) | "Homebrew 没有装上：这个账号不是管理员 / Homebrew was not installed: this account is not an administrator" | "请管理员在 macOS 的窗口里输入他的用户名和密码，然后重试 / Ask an administrator to enter their name and password in the macOS window, then try again". Shown before the prompt when the account is not an administrator, so the user knows whom to fetch. There is no "install into my own folder" button: milestone 0 showed that such an install compiles from source and needs an administrator anyway. |
  | Network or proxy | "下载超时 / The download timed out" | "检查网络后重试 / Check the network and try again". When the system has a proxy configured the line adds "已使用系统代理 proxy.example.com:8080 / Using the system proxy proxy.example.com:8080". |
  | Blocked by company device management | "这台电脑的管理策略不允许安装 / This computer's management policy does not allow the install" | "请联系你的 IT 管理员 / Contact your IT administrator", with the exact package name to ask for. |
  | Disk full | "磁盘空间不够，还需要大约 1.2 GB / Not enough disk space. About 1.2 GB more is needed" | "腾出空间后重试 / Free some space and try again" |
  | Download damaged (size or checksum differs, or a proxy returned a web page) | "下载的安装包不完整 / The download is incomplete" | "重新下载 / Download again". No password was asked. |
  | Package signature wrong | "安装包的签名不对，已停止 / The installer's signature is wrong, so nothing was run" | "重试 / Try again" downloads it again. |
  | Homebrew's signer not recognised | "这个版本的 Codepass 不认识 Homebrew 安装包的签名者 / This version of Codepass does not know who signed Homebrew's installer" | "更新 Codepass 后重试 / Update Codepass, then try again"; "我自己安装" stays available. |
  | Homebrew partly installed (`/opt/homebrew` exists without a working `brew`) | "/opt/homebrew 里已经有一份没装完的 Homebrew / There is an unfinished Homebrew in /opt/homebrew" | "请先自己修好或删除它，然后重新检查 / Repair or remove it yourself, then check again". Codepass never deletes it. |
  | winget unavailable (Windows, Store blocked) | "这台电脑没有 winget，也打不开 Microsoft Store / This PC has no winget and cannot open Microsoft Store" | "请 IT 管理员安装「应用安装程序」/ Ask IT to install App Installer". |

- The gate stays closed after a failed install (owner decision 1): there is no limited mode.
- *Done*: all rows green, footer "都准备好了 / Everything is ready", primary "进入 Codepass / Open Codepass". The card's check draws once.
- *Later in Health*: the same rows appear on the Health page with "更新 / Update" actions, plus the AI tools and language servers.

**Copy.**

| Key | zh-CN | en |
|---|---|---|
| title (mac / win) | 先把这台 Mac 准备好 / 先把这台电脑准备好 | Let's get this Mac ready / Let's get this PC ready |
| lead (mac) | Codepass 用 Homebrew 安装和更新它需要的工具。点一次，其余的自动完成。 | Codepass uses Homebrew to install and update the tools it needs. One click, and the rest happens by itself. |
| note 1 (mac) | 安装 Homebrew 时 macOS 会要你的密码。密码直接交给系统，Codepass 看不到。 | macOS asks for your password to install Homebrew. It goes straight to macOS; Codepass never sees it. |
| note 2 (mac) | 只用官方安装包：Homebrew.pkg（先验证签名），再用 brew 安装 git 和 awscli。Codepass 运行 brew 时会关闭它的匿名统计。 | Only official installers: Homebrew.pkg (signature checked first), then git and awscli through brew. Codepass turns off Homebrew’s analytics for the commands it runs. |
| note 3 | Claude Code、Codex 和 OpenCode 不在这里安装，第一次用到时再装。 | Claude Code, Codex and OpenCode are not installed here. Each one is installed the first time you use it. |
| card title | 需要的工具 | Required tools |
| why: Homebrew | 安装和更新下面两项，以及之后用到的工具 | Installs and updates the two tools below, and anything needed later |
| why: Git / AWS CLI | 克隆、提交、推送 / 登录 AWS，访问 CodeCommit | Clone, commit, push / Sign in to AWS and reach CodeCommit |
| fail title | Git 没有装上：下载超时 | Git was not installed: the download timed out |
| fail text | 已经装好的不受影响。检查网络或代理后再试一次，会从这一步继续。 | What is already installed is untouched. Check the network or proxy and try again; it continues from this step. |

**Accessibility.** The tool list is a `<ul>`; each state change is announced through one `aria-live="polite"` region ("Git 已安装，2 / 3"). The log is `role="log"` and is not announced line by line. Focus starts on the primary button; after a failure it moves to "Try again". Progress bars carry `aria-valuenow`.

### 5.2 Projects (`projects`)

Shots: `projects-light-zh`, `projects-dark-en`, `projects-drag-light-zh`.

**Header.** Title "项目 / Projects"; subtitle "3 个项目 · 12 个仓库 · 刚刚扫描 ~/Documents/code". Actions: Rescan (icon), "克隆仓库 / Clone repository", primary "新建项目 / New project".

**Toolbar.** Segmented filter 全部 / 待处理 / 有改动 (All / To fix / With changes), a provider filter menu, search (repository or project name; matching repositories stay under their project header, other projects hide).

**Find a repository by typing** (owner, 2026-11-16, second batch; plan 4.4). The search is how the owner gets to work: type a few letters of the local folder name, press Enter.
- Cmd/Ctrl+F focuses the field from anywhere on the page; its placeholder becomes "输入文件夹名找仓库 / Type a folder name to find a repository".
- Ranking, best first: the local folder name starts with the text; a word inside the folder name starts with it (after `_`, `-`, `.` or a case change, so `agent` finds `acme_agent_portal`); the text anywhere in the folder name; then the remote name, the CodeCommit name and the path. Rows keep their project grouping; inside a project, better matches come first.
- The matched letters are bold in the name (the `.remote-name` tag too when it matched).
- Keys while the field has focus: Down / Up move a visible selection (the ink bar of the session switcher) through the matching rows; **Enter opens the selected repository**, which is the best match until another is selected; Cmd/Ctrl+Enter opens it with the new-task sheet (milestone 2 on); Esc clears the text, then a second Esc leaves the field.
- One line under the toolbar while searching: "3 个仓库匹配 · ⏎ 打开 acme_agent_portal / 3 repositories match · ⏎ opens acme_agent_portal". No match: "没有文件夹名或远程名包含「xyz」的仓库 / No repository's folder or remote name contains "xyz"" with "清除搜索 / Clear search"; hidden repositories that match are counted: "另有 1 个已隐藏 / 1 more is hidden" with "显示 / Show".

**Project section** (`.proj`). The portal-section pattern from Accounts, reused: chevron + layers tile, name, meta ("4 个仓库 · 2 个 AI 任务运行中"), the project's author identity on the right if it has one, a menu (rename, author identity, AI 账号 / AI accounts, add repositories, new AI task in..., remove project). Collapse state is remembered. Collapsed sections show the account chips of their repositories as a peek, as portals do.

**Project menu: "AI 账号 / AI accounts"** (milestone 8; plan 4.16 decision 14; shown only when a tool has more than one AI account). It opens a small sheet (`.modal--sm`), the project level of repository › project › app:

```
项目 acme 的 AI 账号 / AI accounts for project acme
  [cc] Claude Code   这个项目默认用 / This project uses   [ 跟随应用默认（现在是「工作」）  v ]
  [cx] Codex         这个项目默认用 / This project uses   [ 跟随应用默认（现在是「工作」）  v ]

  会跟着变的仓库 3 个 / 3 repositories follow this
    acme_api
    acme_cs_portal
    acme_merchant_portal_v4
  保留自己的 1 个 / 1 keeps its own
    acme_agent_portal · Claude Code 用「个人」 / Claude Code uses Personal
                                                        改为跟随项目 / Make it follow the project
                                                        [取消 / Cancel]  [保存 / Save]
```

- Options of each select: "跟随应用默认（现在是「工作」）/ Follow the app default (now Work)", then every account of that tool. The app default is the first account in the order on the AI accounts page (5.16); the sheet links there: "调整顺序 / Change the order".
- The two lists use the `.affect` pattern of the identity sheet (5.12): repositories that follow the project, then repositories with their own account on a sunken row, each with "改为跟随项目 / Make it follow the project". Hidden repositories are left out of both lists and of the move confirm; they keep their setting (5.16).
- Saving changes only which account is pre-selected for new tasks and tickets. One muted line says so: "只影响以后新开的任务，正在运行的会话不变。 / Only tasks started later are affected. Running sessions keep their account."
- A project with a fixed account shows it in the section header beside the author identity, as a small chip with the tool mark: "[cc] 个人 / Personal". Ungrouped has no menu, so its repositories follow the app default unless they set their own.
- Moving a repository into a project whose account default differs adds one line to the move confirm: "Claude Code 的默认账号会从「工作」变成「个人」。 / The default Claude Code account changes from Work to Personal." Repositories with their own account are not named, because nothing changes for them.

**Repository row** (`.proj-table`, 6 columns): select; provider mark + name (two names when they differ, see below) + path (middle-truncated); account chip (or dashed "未绑定 / Not bound"); branch and sync (`↑2 · 5 个改动`, `↓4`, "干净 / Clean"); status (dot + two lines, same vocabulary as today); actions (AI badge, a primary fix button when the row needs one, else the More menu). **Clicking a row opens the workspace.** The chip still opens the bind popover; the checkbox and buttons do not open the workspace.

- AI badge (`.ai-badge`): "1 运行中 / 1 running" in info with a pulsing dot; "1 等待你 / 1 waiting" in warn when any task of that repository waits. Click goes to the repository's AI tasks tab.
- Status additions: "有 4 个新提交可以拉取 / 4 new commits to pull", "1 个拉取请求等你评审 / 1 pull request to review", "SSH 密钥还没验证 / SSH key not verified yet" (warn, action "验证 / Verify"), "登录还剩 48 分钟 / Sign-in ends in 48 min".

**Two names** (`.remote-name`). A repository has a local folder name and a remote name (the last part of the origin URL). When they are the same, one name is shown, as before. When they differ, the **local folder name comes first** and the remote name follows as a small outlined tag: `api-main` `远程 acme_api` / `api-main` `remote acme_api`. This one component (`RepoName`) is used in project rows, the workspace header crumb, the AI tasks page, the session switcher, the quit dialog, the Free up space dialog, pull request groups and search results. Screen readers hear "api-main，远程仓库名 acme_api / api-main, remote name acme_api". Search matches either name. A repository with no remote shows only the folder name.

**Row menu** (More): Open, Bind account..., Move to project, **重命名本地文件夹… / Rename local folder...** (5.2a), Reveal in Finder, Open in system terminal, Hide.

**Moved by hand.** When a folder was renamed or moved outside the app, its row turns warn: "找不到这个文件夹 / This folder is gone" with "重新定位… / Locate..." and "从列表移除 / Remove from list". When a scan finds a repository that matches exactly one such row (same first commit and same remote), the found row asks instead: "这是移动过来的 acme_api 吗？ / Is this acme_api, moved here?" - 是，接上原来的记录 / 不是，当作新仓库 (Yes, keep its records / No, treat it as new).

**Ungrouped.** The last section, "未分组 / Ungrouped", with a folder tile and no menu.

**Suggestion** (`.suggest`, dashed, calm): "发现 3 个以 initech_ 开头的仓库，要放进同一个项目吗？ / Found 3 repositories that start with initech_. Put them in one project?" with "创建项目 initech / Create project initech" and "不用了 / No thanks". One suggestion at a time, placed directly above Ungrouped. Dismissed suggestions do not return for the same prefix.

**Newly found repositories** (shot `projects-found-light-zh`). Codepass lists every git repository under the scan folders, not only CodeCommit ones. After the upgrade from AWS SSO Hub, and whenever a scan finds repositories it has not shown before, they are **not** added to the list. One solid `.suggest` row appears under the header: "又发现了 23 个仓库（Codepass 现在会列出所有 Git 仓库，不只是 CodeCommit）。它们还没有显示在列表里。 / Found 23 more repositories (Codepass now lists every Git repository, not only CodeCommit). They are not in the list yet." with primary "挑选要显示的 / Choose which to show" (a checklist modal: name, path, provider mark, last commit date; "全选 / Select all"; pre-ticked when the repository has a commit from the last 90 days) and "全部隐藏 / Hide them all". Repositories that were in the list before the upgrade stay.

**Hide repository.** Row menu and bulk bar: "隐藏 / Hide". A hidden repository leaves the list, the counts, background checks and suggestions; nothing on disk changes. The toolbar's filter menu gains "已隐藏 12 / Hidden 12", which shows them dimmed with "取消隐藏 / Unhide". Toast: "已隐藏 old_fork / Hid old_fork" with Undo.

**Create / rename / remove.**

- New project: `.modal--sm`, one field "项目名 / Project name", then a checklist of ungrouped repositories (pre-ticked by prefix match), optional "作者身份 / Author identity" disclosure. Primary "创建项目 / Create project".
- Rename: inline edit of the title (Enter saves, Esc cancels).
- Remove: confirm "移除项目 acme？4 个仓库会回到「未分组」，磁盘上的文件不会动。 / Remove project acme? Its 4 repositories go back to Ungrouped. Nothing on disk is touched." Primary "移除项目 / Remove project". If the project has an identity: one more line listing the repositories that fall back to the global identity. If the project has an AI account default: one more line, "这 3 个仓库的 Claude Code 默认账号会从「个人」变成「工作」。 / The default Claude Code account of these 3 repositories changes from Personal to Work."

**Move.** Drag a row by its grip (appears on hover and on keyboard focus) onto a project header or onto Ungrouped; or select rows and use the bulk bar: "已选 2 个仓库 · 移到项目 ▾ · 检查连接". Keyboard: focus a row, press M, pick a project in the menu. If the destination has an identity, a confirm lists which repositories' author identity will change and which keep their own. Toast: "已把 initech_gateway 移到 globex / Moved initech_gateway to globex" with Undo.

**States.** Loading: skeleton rows under real section headers. Scanning: the indeterminate bar under the toolbar with "正在扫描 ~/Documents/code... 已找到 9 个". Empty (no repositories at all): `.empty` "还没有仓库 / No repositories yet" with "克隆仓库" and "添加文件夹 / Add a folder". Empty project: one dashed row "把仓库拖到这里，或者从菜单添加 / Drag repositories here, or add them from the menu". Search with no result: "没有匹配的仓库 / No matching repositories" + "清除搜索 / Clear search". Git status of a row failing to load shows "读取不了这个文件夹 / Cannot read this folder" in danger with "重试".

**Clone** (the existing multi-select modal, now on this page). Each ticked repository row gains an inline field "本地文件夹名 / Local folder name", pre-filled with the remote name. Below the list, live: "会克隆到 ~/Documents/code/api-main / Clones into ~/Documents/code/api-main". Errors are inline and block the primary button: "这个文件夹已经存在 / This folder already exists", "名字里不能有 / 或 \\ / The name cannot contain / or \\", "两个仓库不能用同一个文件夹名 / Two repositories cannot use the same folder name". A changed name shows a small "和远程不同 / differs from remote" tag so a slip is visible. Nothing else in the AWS clone flow changes; the source switch and the Project field arrive with the new providers.

**Accessibility.** Each section is a `<section>` with the toggle as a button (`aria-expanded`, `aria-controls`). The table keeps the roving grid from today. Drag and drop has the keyboard path above and announces "已拿起 initech_gateway / Picked up initech_gateway", "在 globex 上方 / Over globex", "已移到 globex / Moved to globex" through a live region.

### 5.2a Rename local folder (`rename-folder`)

Shot: `rename-folder-light-zh`.

From the repository row menu and the repository settings sheet. Modal, 640px. It renames the folder on this computer and can move it under another parent folder on the same disk. The remote repository is not touched.

**Layout, top to bottom.**

1. Header: "重命名本地文件夹 / Rename local folder"; sub "只改这台电脑上的文件夹。远程仓库不会变。 / Only the folder on this computer changes. The remote repository stays as it is."
2. **Two name boxes** (`.two-name`) with an arrow between them. Left, sunken and fixed: provider mark, "远程仓库名 / Remote name", the name in mono, "保持不变 / Stays the same". Right, ink-outlined (the only editable thing): folder icon, "本地文件夹名 / Local folder name", a mono input, hint "现在是 acme_api / Now acme_api". Focus starts here with the name selected.
3. **放在哪里 / Location** (`.path-line`): the old path struck through, arrow, the new path in full; button "换一个上级文件夹… / Choose another parent folder...". The line updates as the user types.
4. Two lists side by side (`.rf-cols`, each an `.affect` list with an icon, a title and one line):
   - **开始前会停止 / Stopped first**: AI sessions (count, with the live state on the right; "对话会保留，改名后可以继续 / Conversations are kept and can be resumed"), terminals (count and shell), code intelligence ("之后自动重启 / restart by themselves"), file watching ("之后自动恢复 / Comes back by itself"). Rows that do not apply are left out. When nothing would be stopped, the list is replaced by "没有需要停止的程序 / Nothing needs to stop".
   - **会跟着更新 / Updated to match**: project and repository records ("绑定的账号、作者身份不变 / Account binding and author identity are unchanged"), task workspaces ("重新连到新位置，文件夹不动 / Re-linked to the new location; their folders do not move"), task records and connection checks.
5. Warnings, only when true (warn banner, one sentence each):
   - "在主工作区运行过的 1 个 Claude Code 会话可能接不上原来的对话：这个工具按文件夹路径保存对话。 / 1 Claude Code session that ran in the main workspace may not find its earlier conversation: that tool files conversations by folder path."
   - "~/.gitconfig 里有一条按旧路径写的规则，Codepass 不会改它。 / A rule in ~/.gitconfig names the old path. Codepass does not change it." with "打开那个文件 / Open that file".
6. Promise line (`.note-line`, shield): "任何一步失败，文件夹都会改回原来的名字，记录也恢复原样。 / If any step fails, the folder gets its old name back and every record is restored."
7. Footer: 取消 / Cancel; primary "停止并重命名 / Stop and rename" (or "重命名 / Rename" when nothing has to stop).

**States.**

| State | What shows |
|---|---|
| Name unchanged or empty | primary disabled |
| Name taken | inline danger under the field: "~/Documents/code 里已经有 api-main / ~/Documents/code already has api-main" |
| Invalid name | "名字里不能有 / 或 \\，也不能是 . 或 .. / The name cannot contain / or \\, or be . or .." |
| Another disk chosen | warn under the location: "换到另一个磁盘需要整份复制，Codepass 1.0 不做这件事。请在访达里移动，再回来重新定位。 / Moving to another disk means copying everything, which Codepass 1.0 does not do. Move it in Finder, then locate it again here." Primary disabled. |
| Operation in progress (merge, rebase) | banner "先完成或放弃正在进行的合并 / Finish or abort the merge in progress first"; primary disabled |
| Running | the two lists become a step list with checks: 停止 2 个会话 / 重命名文件夹 / 更新记录 / 重新连接 2 个工作区 / 恢复监视 (Stop 2 sessions / Rename the folder / Update records / Re-link 2 workspaces / Resume watching). Cancel is disabled once the folder rename starts. |
| Failed | danger banner naming the step: "没有重命名：有程序还在占用这个文件夹 / Not renamed: a program is still using this folder", then "已经改回原样 / Everything was put back", with "重试 / Try again". On Windows the banner names the program when it is known. |
| Done | the modal closes; toast "已把 acme_api 重命名为 api-main / Renamed acme_api to api-main" with "撤销 / Undo" (while no session has started since); the row shows both names; stopped sessions show "继续 / Resume". |

**Accessibility.** `role="dialog"`; the two boxes are a group labelled "名字 / Names"; the path line is `aria-live="polite"`; the lists are real lists; the failure banner is `role="alert"`.

### 5.3 Code (`workspace-code`)

Shots: `workspace-code-light-zh`, `workspace-code-dark-en`, `workspace-code-dark-zh`, `workspace-code-find-light-zh`.

The Code tab is a full editor for the whole repository. It is the first tab and the default when a repository opens with no pending changes.

**Layout.** Left column 248px (resizable 200-420): **Files** on top, **Outline** below (collapsible, 30% by default). Centre: one or two editor groups side by side (split is cheap with CodeMirror: two views on shared documents). Optional bottom panel.

**Files** (`.tree`).

- Header: "文件 / Files", New file, "只看有改动的文件 / Show changed files only" toggle. Filter field filters by path as you type (matches are bold; folders with matches stay open).
- Rows 24px: chevron, folder / file icon, name, git letter on the right. Name colour follows git status: modified info, added ok, deleted danger with strike, conflicted danger, ignored disabled. A folder is tinted when something inside changed. Single-child folders are compacted (`web/admin/src/api`).
- Interactions: click opens in a preview tab (italic title); double-click or editing pins it. Enter opens, arrows move, Right / Left expand / collapse, type-ahead. Context menu: New file, New folder, Rename, Delete (confirm, moves to the system trash), Copy path, Copy relative path, Reveal in Finder / Explorer, Open in terminal, File history, Discard changes.
- Dotfiles are shown; `.git` is hidden; ignored folders (`node_modules`) appear dimmed and are not expanded by filter or search.

**Outline** (`.outline`). Symbols of the active file from the language server (Go, TS/JS) or from the editor's parser for other languages. Kind tile (S struct, I interface, f function, m method, c const, v var), name, signature in muted text. The symbol holding the cursor is highlighted. Click jumps; the filter field appears on typing.

**Editor group** (`.ed`).

- **Tabs** (`.ed__tab`): file icon, name (git-coloured), close button or the dirty dot. Middle-click closes. Drag to reorder or to the other group. Overflow scrolls; a chevron menu lists all open files. Two files with the same name show their folder in muted text.
- **Breadcrumbs** (`.ed__crumbs`): path segments then symbols. Each segment opens a menu of its siblings.
- **Code** (`.ed__code`): 12.5px mono, 20px lines. Gutter: a 3px change bar (info modified, ok added, a danger wedge where lines were removed), then line numbers. Clicking the bar opens an inline diff of that hunk with "还原这一块 / Revert this hunk". Current line highlighted; matching brackets; indent guides; multiple cursors; soft wrap optional.
- **Search in file**: a compact bar at the top right of the group (find, replace, case, word, regex, n of m).

**Code intelligence (Go via gopls; TS/JS via the TypeScript 7 native server, or typescript-language-server for a project on TypeScript 5 or 6).** The status bar item reads "TypeScript ✓" either way; its tooltip names the server in use.

| Affordance | Design |
|---|---|
| Hover (`.hover-card`) | After 350ms on a symbol: signature block (mono, highlighted), one paragraph of documentation, source file in muted text, footer with "⌘ 点击 跳到定义 / ⌘ click to go to definition" and "⇧F12 12 处引用 / 12 references". Stays while the pointer is inside it. Max 460x280, scrolls. |
| Completion (`.ac`) | List of up to 8 rows: kind tile, name with the typed part in focus colour, type on the right. A side pane shows the selected item's signature and documentation. Enter inserts, Tab replaces the word, Esc closes. Appears after `.` and on Ctrl+Space. |
| Go to definition | F12 or Cmd/Ctrl+click jumps (new preview tab if another file). The symbol under the pointer underlines while Cmd/Ctrl is held. |
| Peek definition (`.peek`) | Alt+F12: an inline frame under the line, focus-coloured top and bottom borders, header with file, folder, "CalcFee 的定义 / definition of CalcFee", "打开文件 ⏎", "关闭 Esc". 6-10 lines, the definition line tinted. Editable. |
| Find references | Shift+F12: results in the bottom panel's Find results tab, grouped by file. |
| Diagnostics | Wavy underline (danger error, warn warning), a mark in the scrollbar, the count in the status bar, the list in Problems. Hovering shows the message first, then the hover content. |
| Rename symbol, format | F2 and Shift+Alt+F when the server supports them. Not required for 1.0. |
| Server status | Status bar: "gopls ✓". States: "正在启动 gopls... / Starting gopls...", "正在索引... / Indexing...", "gopls 没有运行 / gopls is not running" (danger, click for the log and "重新启动 / Restart"). First use: a quiet bar above the editor "打开 Go 文件需要 gopls，正在安装... / Go files need gopls. Installing..." with progress; failure offers "重试" and "先不用代码智能 / Continue without code intelligence". |

Other languages (Python, Java, SQL, YAML, JSON, Markdown, Dockerfile, shell): highlighting, outline from the parser, bracket matching, find. No server.

**Quick open** (Cmd/Ctrl+P; not drawn). A 560px palette at top centre: fuzzy match on path, recent files first, git letter on the right, `:` jumps to a line, `@` switches to symbols of the current file, `#` to workspace symbols (Go, TS). Cmd/Ctrl+K opens the same palette in "everything" mode with commands.

**Find in files** (`panel=find`). Bottom panel tab "查找结果 / Find results": query field with case / word / regex toggles and include / exclude globs, "3 个文件中有 7 处 / 7 results in 3 files", results grouped by file with line numbers and the match marked. Enter or click opens the file at the match; F4 / Shift+F4 step. Replace all asks for confirmation with the count. Searches respect `.gitignore`.

**States.**

- *No file open*: centred, quiet: "打开一个文件开始 / Open a file to start" with the four shortcuts (Quick open, Find in files, New AI task, Terminal) as `kbd` rows.
- *Loading a file*: skeleton lines for over 150ms.
- *Unsaved*: dirty dot on the tab; closing asks "保存对 settlement.ts 的修改吗？ / Save changes to settlement.ts?" 保存 / 不保存 / 取消. Quitting lists all unsaved files.
- *Changed on disk* (an AI task or git rewrote it): no unsaved edits, reload silently and flash the changed lines once; with unsaved edits, a bar: "这个文件在磁盘上被改了 / This file changed on disk" - 比较 / 用磁盘上的 / 保留我的 (Compare / Use the disk version / Keep mine).
- *Large file*: over 1 MB or a line over 20,000 characters opens with highlighting and code intelligence off: "文件较大，已关闭高亮和代码智能 / Large file: highlighting and code intelligence are off". It can still be edited and saved up to **3 MB**. Above 3 MB the file is **read-only**: "这个文件超过 3 MB，只能查看。请用其他编辑器修改。 / This file is over 3 MB and can only be viewed here. Use another editor to change it." with "在 Finder 中显示 / Reveal in Finder". Above 50 MB only the first 5 MB are shown, with a line saying so. (The limit comes from how the app sends a save; see the plan, section 5.)
- *Binary*: no editor. Images up to 5 MB preview with size and dimensions; larger images and other binaries show type and size with "在 Finder 中显示 / Reveal in Finder".
- *Save failed*: danger bar with the OS reason and "重试".
- *Restored after a crash*: see 4.4.
- *Fresh task workspace without dependencies* (TypeScript, no `node_modules`): diagnostics are held back and one quiet bar says "这个工作区还没有安装依赖，先不显示类型错误 / Dependencies are not installed in this workspace yet, so type errors are hidden for now" with "运行安装命令 / Run the setup command" (the repository's setup command, in the terminal panel) and "仍然显示 / Show them anyway".
- *Language tools missing*: gopls needs Go. On macOS the TypeScript 7 server needs nothing else; a project on TypeScript 5 or 6 needs typescript-language-server, which brings Node. On Windows Node is needed once, to install the server. The first-use bar names what will be installed and how big it is, and asks: "要用 Go 的代码智能，需要安装 Go（约 250 MB）和 gopls。 / Code intelligence for Go needs Go (about 250 MB) and gopls." - 安装 / 先不用 (Install / Not now). For an older TypeScript project: "这个项目用的是 TypeScript 5，需要另一个语言服务和 Node（约 120 MB）。 / This project uses TypeScript 5, which needs a different language server and Node (about 120 MB)." Nothing is installed without that click.
- *Many folders open*: at most 2 language servers run at once (4 on a Mac with 32 GB or more), and servers for folders that have not been looked at for 10 minutes are shut down; the status bar item shows "gopls 已休眠 / gopls is asleep" and wakes on the next edit or hover. When the limit forces a server to stop, the item says which: "为了省内存，已让 acme_api 的 gopls 休眠 / To save memory, gopls for acme_api was put to sleep".
- *Read-only workspace* (a finished task being inspected after its folder was removed) does not exist: removed workspaces leave the switcher.

**Accessibility.** The tree is `role="tree"`; tabs are a `tablist`; the editor exposes CodeMirror's accessible text area. Hover content is reachable with Cmd/Ctrl+K Cmd/Ctrl+I at the cursor. Completion is a `listbox` tied with `aria-activedescendant`. Git status is announced as text ("已修改 / modified"), not only by colour.

### 5.4 Changes and the commit composer (`repo-commit`)

Shots: `repo-commit-light-zh`, `repo-commit-light-en`, `repo-commit-dark-zh`.

**Layout.** Left pane 340px: file groups, then the composer pinned at the bottom. Right: the diff of the selected file.

**File groups** (`.fl__group` + `.fl`): **已暂存 / Staged**, **未暂存 / Not staged** (tracked changes and untracked files together; untracked show U), **贮藏 / Stashes** (collapsed; section 5.8). Each group header has its count and one action ("全部取消 / Unstage all", "全部暂存 / Stage all", "贮藏当前改动 / Stash changes"). A row: checkbox (ticking stages, unticking unstages; a partly staged file shows the indeterminate mark), name, folder, letter. Space toggles, Enter focuses the diff. Context menu: Stage, Discard changes..., Open in editor, File history, Copy path, Add to .gitignore.

**Diff** (`.diff`).

- File header: name, folder, +/- counts, Unified / Side by side, "暂存文件 / Stage file" (or "取消暂存 / Unstage file"), "丢弃改动 / Discard changes", file history.
- Hunk header (`.hunk`): the `@@` line with "暂存这一块 / Stage hunk" and "丢弃这一块 / Discard hunk". For a staged file the buttons read "取消暂存这一块 / Unstage hunk".
- Lines (`.dl`): two line-number columns, sign, code with syntax colours. Drag across lines (or Shift+arrows) to select; a small floating bar appears: "暂存 3 行 / Stage 3 lines", "丢弃 3 行 / Discard 3 lines".
- Whitespace-only changes can be hidden (view menu). Long unchanged runs collapse to "展开 24 行 / Expand 24 lines".
- New files show all lines added; deleted files all removed; renames show "retry.go → retrier.go"; binary files show "二进制文件，大小从 12 KB 变为 14 KB".

**Composer** (`.composer`). Message box (first line is the summary; a soft mark at 72 characters, never a block), the identity line, the amend checkbox, the commit split button.

- Identity line: user icon, `Alex <alex@acme.example.com>`, and a source tag (`.src-tag`): "来自项目 acme / From project acme" (info tint), "来自全局 / From global" (neutral), "仓库自己的 / Repository's own" (warn tint). Clicking opens the identity editor. No identity at all: the line turns warn, "还没有设置作者身份 / No author identity yet" + "去设置 / Set it", and Commit is disabled.
- Button: "提交 2 个文件 / Commit 2 files" with Cmd/Ctrl+Enter. Caret: "提交并推送 / Commit and push". Disabled with a tooltip when nothing is staged ("先暂存要提交的文件 / Stage the files you want to commit") or the message is empty.
- Amend: ticking loads the last message and adds a note "会改写上一次提交 a41c9e2 / This rewrites the last commit a41c9e2"; if that commit is already pushed: warn "上一次提交已经推送，改写后需要强制推送 / The last commit is already pushed. Rewriting it needs a force push".

**Discard** always confirms and says what happens: "丢弃 batch.go 里的改动？这 8 行改动会从文件里移除。14 天内可以在「最近丢弃」里找回。 / Discard the changes in batch.go? These 8 changed lines are removed from the file. You can bring them back from Recently discarded for 14 days." For untracked files: "文件会移到废纸篓 / The file moves to the Trash". Focus starts on Cancel. Primary repeats the verb. The toast offers Undo. **最近丢弃 / Recently discarded** is a collapsed group under Stashes in the left pane: one row per discard (file or hunk, time), with "恢复 / Restore" and "彻底删除 / Delete for good". Discarded changes stay inside the repository; they are never copied anywhere else.

**States.** Clean: `.empty` "没有改动 / No changes" with the last commit line and "在这个仓库新建 AI 任务". Loading: skeleton list. Committing: button `.is-loading`; then the staged group empties with the remove motion and a toast "已提交 a41c9e2" with "推送 / Push". Hook failure: danger banner above the composer with the hook's output in a details disclosure. Operation in progress (merge, rebase, cherry-pick, revert): the `.opbar` replaces nothing and sits above both panes (section 5.9). Signing failure (the repository signs commits and the key needs a PIN or passphrase the app cannot ask for): danger banner "提交需要签名，但签名程序要你输入密码，Codepass 弹不出那个窗口 / This commit must be signed, and the signing program wants a passphrase that Codepass cannot show a window for", with "在终端里提交 / Commit in the terminal" (opens the terminal panel with the command typed, not run).

### 5.5 Reviewing an AI task (`workspace-changes`)

Shots: `workspace-changes-light-zh`, `workspace-changes-dark-en`.

The same Changes tab, pointed at a task's workspace through the switcher. It is not a second diff UI.

- Header: the switcher pill is info-tinted with the task's branch; the 2px info line runs under the header.
- **Task bar** (`.taskbar`) across the top: tool mark, "正在查看任务 修复结算对账的四舍五入误差 / Reviewing task ...", `ai/fix-settlement-rounding → main`, the state, then: "回到任务终端 / Back to its terminal", "丢弃任务 / Discard task" (danger ghost), "推送分支 / Push branch", primary "合并到 main / Merge into main".
- Left pane: **相对 main 的改动 / Changes against main** (everything the task did since it branched: committed and not), then **任务里的提交 / Commits in this task**, then **还没提交 / Not committed yet**. A footer line states which identity will be used if merging has to commit first.
- Right: the combined diff, file after file, each with "在编辑器中打开 / Open in editor" (opens the Code tab in the task's folder; edits there are saved into the task's workspace).

**Finish actions.**

| Action | Confirm copy (zh / en) |
|---|---|
| Merge into `<base>` | "把 ai/fix-settlement-rounding 合并到 main？包含 2 个提交和 1 个还没提交的文件（会先提交）。任务的工作区随后会删除。" / "Merge ai/fix-settlement-rounding into main? It has 2 commits and 1 file not committed yet (it will be committed first). The task's workspace is removed afterwards." Fields: commit message for the uncommitted part; checkbox "合并后删除分支 / Delete the branch after merging" (on). The dialog lists the steps it will take (below) in one muted line. |
| Push branch | No confirm. Toast "已推送 ai/fix-settlement-rounding" with "创建拉取请求 / Create pull request" (GitHub, Bitbucket) or "在 AWS 控制台创建拉取请求 / Create it in the AWS console" (CodeCommit). The task stays. |
| Discard task | "丢弃这个任务？2 个提交和 4 个改动的文件会被删除，分支 ai/fix-settlement-rounding 也会删除。14 天内可以在「最近丢弃」里找回。" / "Discard this task? 2 commits and 4 changed files are deleted, and so is the branch ai/fix-settlement-rounding. You can bring it back from Recently discarded for 14 days." Primary "丢弃任务". |
| Keep | The default: do nothing. |

**How merging works, in the order the user sees it.**

1. *Bring the task up to date.* If `main` moved since the task started, main is first merged **into the task's workspace**. Conflicts are resolved there, in the conflict screen, with the task stripe on. The main workspace is not touched while this happens.
2. *Move `main` forward.* Once the task contains everything in main, main only needs to move forward to the task's last commit. Three cases:
   - `main` is checked out in the main workspace and has no changes in the way: it moves forward, and the files in the main workspace update.
   - `main` is not checked out anywhere (the main workspace is on another branch): it moves forward without touching any folder. The toast says "已合并到 main。主工作区还在 feat/x 分支，没有变化。 / Merged into main. The main workspace is still on feat/x and did not change."
   - `main` is checked out and has uncommitted changes in the same files: the app stops and asks: "主工作区有 3 个还没提交的改动会受影响 / The main workspace has 3 uncommitted changes in the way" - 先贮藏再合并 / 取消 (Stash them, then merge / Cancel). Nothing is stashed without that click.
3. *Clean up.* The task's workspace is removed and, if ticked, its branch is deleted.

While the task is still running, Merge and Discard first ask to stop it. "Rebase onto main" on a branch that is not checked out follows the same honesty: the confirm says "会在一个临时文件夹里完成，你的工作区不会变 / This runs in a temporary folder. Your workspace does not change."

### 5.6 AI tasks in a repository (`workspace-ai`, `new-task`)

Shots: `workspace-ai-light-zh`, `workspace-ai-dark-en`, `new-task-light-zh`, `new-task-light-en`, `new-task-naming-light-zh`.

**Layout.** Left pane 330px: the repository's tasks. Right: the selected task's header, session tabs, terminal.

**Task list.** Header "这个仓库的任务 / Tasks in this repository" and primary "新任务 / New task" (Cmd/Ctrl+T). Rows (`.task`): tool mark, title, elapsed time; second line: state, branch (or "主工作区 / main workspace"), changed-file count. When the tool has more than one AI account, the row's second line ends with the account's colour dot and label. Active row: ink bar on the left. Order: waiting first, then running by start time; "已结束 / Finished" is a collapsible group (done, failed, stopped). Context menu: Rename, Review changes, Stop, Discard, Reveal folder. Rename edits the title in place and changes only the label. One muted line under the edit box says so: "只改任务名。分支名不变，AI 可能已经在用它。 / Only the task's name changes. The branch keeps its name, because the AI may already use it."

**States** (`.state`, always dot + word).

| State | zh / en | Dot |
|---|---|---|
| starting | 启动中 / Starting | busy pulse |
| running | 运行中 / Running | busy pulse (info) |
| running, no signal | 运行中（没有信号）/ Running (no signal) | steady info dot, no pulse. For a tool that cannot tell the app what it is doing. Tooltip: "这个工具不会告诉 Codepass 它是在干活还是在等你，请看终端。 / This tool does not tell Codepass whether it is working or waiting. Look at its terminal." |
| waiting | 等待你 / Waiting for you | warn, breathing |
| done | 已完成 / Done | ok |
| failed | 失败 / Failed | danger |
| stopped | 已停止 / Stopped | hollow |
| closed, still running | 已关闭，仍在运行 / Closed, still running | warn, steady; always with the process count (5.6a) |

"Waiting" raises: the tab count, the rail pip, the sidebar count, the repository row's badge, and a system notification when the window is not focused ("acme_api：Claude Code 在等你回答 / Claude Code is waiting for your answer").

**Task header** (`.task-head`). Large tool mark, title (double-click to rename), branch chip (`.ref--ai`), "从 main 创建 · 独立工作区 / from main · own workspace", elapsed time, state. Actions: "查看改动 4 / Review changes" (switches the working tree and opens Changes), "合并到 main" with a caret (Push branch, Discard task, Keep), Stop.

**Session tabs** (`.term-tabs`). One tab for the AI tool, then any shells opened in the same folder ("zsh"), then `+`. On the right, in dim mono: the terminal size.

**Session bar** (`.term-note`, 30px, between the tabs and the terminal). The app cannot and must not draw inside the tool's own screen, so everything the app wants to say about a session sits here: "这个任务以你的身份运行，你能读写的它都能读写。 / This task runs as you. It can read and change whatever you can."; the tool's approval mode ("批准方式 每次询问 / Approvals: ask every time", read from the tool's own settings; Codex shows its sandbox mode); the author that commits will use ("提交作者 Alex <alex@acme.example.com>"); and the link "了解任务能做什么 / What a task can do". That link opens a short sheet in plain words: a task can read and change any file you can, use any AWS account you are signed in to, use the SSH keys on this computer, and run any command; Codepass never turns off the tool's own permission questions and has no switch to do so; choose "own workspace" so tasks cannot disturb each other, and read the changes before you merge. There is no "AWS_PROFILE" badge: it suggested a protection that does not exist.

**Account on the session bar** (milestone 8, only when the tool has more than one AI account; Claude Code and Codex alike): a chip with the account's colour dot, the label and a caret, "账号 工作 ▾ / Account: Work ▾". Its tooltip gives the latest usage reading in one line. A task that came from a ticket also shows "工单 #12 / Ticket #12", a link to the ticket.

**The chip is a menu: "换账号 / Switch account"** (plan 4.16 decision 16). The owner can move a session to another account at any time, in either direction, as often as wanted. No limit has to be reached first.

```
 [● 账号 工作 ▾]
 +--------------------------------------------------------------+
 | 换账号 / Switch account                                       |
 | ✓ ● 工作 / Work        本次 62% · 本周 81%     这个仓库的默认  |
 |   ● 个人 / Personal    本次 0% · 本周 12%                     |
 |--------------------------------------------------------------|
 |   管理 AI 账号 / Manage AI accounts                           |
 +--------------------------------------------------------------+
```

- Rows: every account of this session's tool, in the owner's order. The current one has the check mark and cannot be chosen. Each row shows the latest reading; a reading older than 15 minutes shows its age. The repository's default carries a muted tag: "这个仓库的默认 / This repository's default".
- An account without room is listed but disabled: "用完了，周一 00:00 重置 / Used up, resets Mon 00:00". "Room" follows the one rule in plan 4.16 (after decision 15): for a session the owner started, every window below 100% with no reserve, and an unknown reading counts as room; for a queued ticket's session, below 100% minus the reserve. One that is not signed in is disabled with "未登录 / Not signed in" and the link "登录 / Sign in". One that needs the owner's attention (5.16) is disabled with "需要你处理 / Needs your attention" and the link "查看 / Open". When the session's **own** account needs attention, every row is disabled and the foot says why: "先处理「工作」的问题，才能换账号。 / Deal with the problem on Work first. Then the account can be changed." (plan 4.16 decision 34).
- Choosing an account opens **the switch confirm** (focus on Cancel). The limit banner, the AI tasks rows, the queue bar and the queue's notice open the same one.

  ```
  在「个人」上继续这个会话？
  对话会带过去：你说的每句话原样带过去，AI 的回复、工具调用和结果也带过去
  （完整的，或者在交接笔记里，笔记会指向完整记录）。                      带不过去的只有一样
  「个人」上的第一轮会把整段对话重新发送一遍，会用掉它的一部分用量。
  ! 上一轮在用量上限处被打断了。换过去后，AI 会先看一眼工作区，再从停下的地方接着做。
  ! 「个人」的套餐没有 Opus，会改用它有的最接近的模型：Sonnet。
                                                              [取消]  [继续]

  Continue this session on Personal?
  The conversation comes along: everything you said, word for word, and the AI's replies, tool calls
  and results (in full, or in a handoff note that points to the full record).                One thing cannot
  The first turn on Personal sends the whole conversation again and uses part of its allowance.
  ! The last turn was cut off at the usage limit. After the switch the AI checks the workspace first, then continues where it stopped.
  ! Personal's plan does not have Opus. The closest model it has is used: Sonnet.
                                                              [Cancel]  [Continue]
  ```

  - Lines 1 and 2 are always there. The "!" lines (`.hint`, info, not warn) appear only when they apply (plan 4.16 decision 24):
    - **Cut off at a limit:** the sentence in the sketch. Not shown for a switch between turns. For "停下并换账号 / Stop and switch" the line reads instead: "换过去后，AI 会先看一眼工作区，然后等你的下一句话。 / After the switch the AI checks the workspace first, then waits for your next message." (plan 4.16 decision 23).
    - **Another model:** the sentence in the sketch, naming both models, only when Codepass has seen the tool replace this model under the other account before; the session is then resumed with that replacement (plan 4.16 decision 24). When it does not know, no line; the result line names the model if it changed.
    - **A conversation longer than the other account can hold:** "这段对话比「个人」能装下的长。Claude Code 会先把较早的部分压缩成摘要；压缩不了时，改用交接笔记。 / This conversation is longer than Personal can hold. Claude Code first compacts the earlier part into a summary. If it cannot, a handoff note is used." (the tool's name follows the session's tool).
  - **"带不过去的只有一样 / One thing cannot"** is a link that opens a small popover, plain text: "对最新的几种 Claude 模型，AI 没显示出来的思考记录只属于产生它的账号，服务方不会交给另一个账号；Codex 是否也这样，还不确定。Codepass 和工具都改变不了。你看得到的内容一样不少。任务照样能接着做：那些思考是 AI 每次回答前打的草稿，结论已经写在它的回答和代码里；换过去后，AI 会读完整段对话、重新思考，然后接着做。第一轮会慢一点，也会多用一点新账号的用量。 / For the newest Claude models, the AI's hidden reasoning belongs to the account that produced it, and the service does not hand it to another account. Whether Codex does the same is not known yet. Neither Codepass nor the tool can change that. Nothing you can see is missing. The task carries on anyway: that reasoning is the draft the AI writes before each reply, and its conclusions are already in its replies and in the code. After the switch the AI reads the whole conversation, thinks again and continues. The first turn is slower and uses a little more of the new account's allowance." (Plan 4.16 decision 22, "Why the task still continues".) The link is shown only for a Codex session or a Claude model on the list in research R8 (plan 4.16 decision 22); for any other model line 1 ends without it. Line 1 is written before the layer is known, so it promises no more than the handoff note delivers.
  - While the AI is working, one more line and a different primary: "AI 正在干活，换账号前要先让它停下这一轮。 / The AI is working. It has to stop this turn before the account can change." · "停下并换账号 / Stop and switch". This one confirm is the only one; no second Stop confirm follows. A switch stops the tool the way "Stop task" does, so programs the session started end too (plan 4.16 step 3). When the tracker reports programs, working or not, the confirm adds the Stop line: "还会结束它启动的 2 个程序 / This also ends 2 programs it started".
  - The confirm has no option about how the conversation is carried. The three layers are tried in order by themselves (plan 4.16 decision 22); the owner reads the outcome in the result line.
- **While the switch runs**, the chip shows a small spinner and "正在换到「个人」… / Switching to Personal...". If the session does not resume as it is, the text changes once, without alarm: "原样接不上，正在换一种方式带过去… / It did not resume as it is. Trying another way...". The terminal area keeps the last screen, dimmed, until the tool is back.
- **The result line** (one info line on the bar, `role="status"`, dismissible; plan 4.16 decision 26). It names the layer that carried the conversation and never says more than that layer delivered:

  | What carried the conversation | Line (zh / en) | Extra |
  |---|---|---|
  | The same session | 已换到「个人」。对话完整带过去了。 / Now on Personal. The whole conversation came along. | none |
  | A repaired copy | 已换到「个人」。对话带过去了，只去掉了只属于「工作」的部分：AI 的思考记录。 / Now on Personal. The conversation came along, without the parts that belong only to Work: the AI's reasoning records. | Codex names its parts: "AI 的推理记录和加密的摘要 / the AI's reasoning records and encrypted summaries". When the last tool call never finished and was left out: "还有最后一次没做完的工具调用 / and the last tool call, which never finished". |
  | A handoff note | 已换到「个人」。对话以交接笔记的形式带过去了。 / Now on Personal. The conversation came along as a handoff note. | Button "查看笔记 / View note". Help under the line, muted: "原来的对话记录还在，新会话需要时会去读。 / The earlier conversation record is still there, and the new session reads it when it needs a detail." |
  | No session could start on the other account | 没有换成：「个人」上启动不了会话。会话还在「工作」上，对话没有动。 / The switch did not happen: no session could start on Personal. The session is still on Work, and its conversation is untouched. | Danger. The tool's own message under it, as text. Buttons "继续 / Resume" (on Work) and "查看账号 / Open the account". |

  | The tool started a new conversation itself | Claude Code 开了一个新对话，原来的对话没有接上。 / Claude Code started a new conversation; the earlier one was not continued. | Danger. Shown when the tool reports a session ID other than the one resumed (the owner chose "Start a new conversation" in the tool's own screen). Button "换账号 / Switch account" to retry; the earlier conversation is untouched. |

  The first line ("The whole conversation came along") is shown only once the tool reports that it resumed the expected session; until then the chip keeps its spinner. If the tool refuses only in the first turn, after the owner has typed, the line changes to the next layer's, and that layer carries the owner's last message: the owner does not type it again (plan 4.16 decision 26).

  After any of the first three, three short additions when they are true: "模型换成了 Sonnet。 / The model is now Sonnet.", "Claude Code 把较早的对话压缩成了摘要。 / Claude Code compacted the earlier conversation into a summary." and "仍在计划模式。 / Still in plan mode." (a session that was in plan mode stays in it after a switch; plan 4.16 decision 22). **There is no line that offers a new session without the conversation**; that path does not exist (plan 4.16 step 5).
- **"查看笔记 / View note"** opens a read-only sheet (640px, from the right): header "交接笔记 · 工作 → 个人 · 今天 14:02 / Handoff note · Work → Personal · today 14:02"; the note as plain text in the mono font (never rendered as markup); one muted line above it: "这份笔记由 Codepass 在这台电脑上根据对话记录和 git 整理，没有调用 AI。你说过的每句话都原样在里面。 / Codepass built this note on this computer from the conversation record and git. No AI was called. Everything you said is in it, word for word."; footer "复制路径 / Copy path" and "在访达中显示 / Reveal in Finder". The same sheet opens from the task's row menu on the AI tasks page ("交接笔记 / Handoff notes", listed by date when there are several).
- **What the tool may ask in its own screen after a switch** (plan 4.16 decision 22): "Resume this conversation?", a usage-credits question, or a notice that the model was replaced. Codepass never answers these and draws nothing over them. The task reads "等待你 / Waiting for you" and the bar adds one muted line: "Claude Code 在问你一个问题，请在终端里回答。 / Claude Code is asking you something. Answer it in the terminal." Right after a switch the line adds: "选「Resume」才能接着原来的对话。 / Choose \"Resume\" to keep the conversation." ("Resume" is the tool's own word and is not translated.)
- Going back is the same menu: on Personal, the menu lists Work, and the same confirm and result lines apply in that direction. Nothing about a switch changes the repository's default, and the menu says so in its foot when the session is not on the default: "这个仓库的默认还是「工作」。 / This repository's default is still Work."
- A stopped task that can be resumed keeps the chip. Choosing another account there changes the "继续 / Resume" button to "在「个人」上继续 / Resume on Personal". "在「个人」上继续 / Resume on Personal" opens the same confirm as the menu, with the cost sentence. A stopped task whose account was removed shows "已移除的账号 / Removed account" on the chip; "继续 / Resume" then opens the menu first, to pick an account, and its confirm.
- **The shell decides** (plan 4.16 decision 20): the chip reads "账号 由终端环境决定 / Account: set by your shell" with no caret. Its popover holds the sentence of 5.16: "你的终端环境设置了 CLAUDE_CONFIG_DIR，Claude Code 用哪个账号由它决定，Codepass 选不了。 / Your shell sets CLAUDE_CONFIG_DIR, so it decides which account Claude Code uses. Codepass cannot choose." On Windows the sentence names the Windows environment variable instead (5.16). A task started in this state records no account, so its rows elsewhere read "由终端环境决定 / set by your shell" (5.7, 5.7a).
- The menu changes the account only. The approval mode, the sandbox mode and the author on the same bar stay as they are: the settings file that holds them is always linked between a tool's accounts (plan 4.16 decision 21). MCP servers the owner added, and folder trust, are kept per account, so after a switch the tool may ask to trust the folder again; the confirm does not repeat this, the add-account sheet said it (5.16 step 2).
- Keyboard: the chip is a button (`aria-haspopup="menu"`); the rows are `menuitemradio`; a disabled row's reason is part of its name.

**Limit banner** (`.term-note--limit`, milestone 8). When the session reaches a usage limit, the session bar grows a second line, warn-tinted, `role="status"`. It replaces nothing the tool shows in its own screen; it adds what the app knows.

```
! 「工作」账号的每周用量用完了，周一 00:00 重置。                    [在「个人」上继续]  [等待]
  Claude Code 会在重置后自己继续。
! The weekly usage of "Work" is used up. It resets Mon 00:00.        [Continue on Personal]  [Wait]
  Claude Code continues by itself after the reset.
```

- Line 1 names the account and the window: "本次用量 / session usage" (Claude Code), "5 小时用量 / 5-hour usage" (Codex), "每周用量 / weekly usage", "Opus 的每周用量 / weekly usage for Opus". Then the reset time in the person's local time, weekday included when it is not today. Unknown reset: "重置时间未知 / reset time unknown".
- Line 2 says who continues. Claude Code with its own automatic continue on: "Claude Code 会在重置后自己继续。 / Claude Code continues by itself after the reset." With it off: "重置后不会自己继续。 / It will not continue by itself after the reset." Codex: a checkbox "到时自动继续 / Continue automatically then" (off; when ticked the app resumes the session at the reset).
- **"在「个人」上继续 / Continue on Personal"** is a shortcut into the same switch as the account chip's menu. It shows only when another account of the same tool has room (the same rule as the menu's disabled rows); with several, it is a split button listing them with their percentages. It opens the switch confirm above, with the cut-off line, because the limit ended the turn. This is the owner's S1 case: one click on the banner, one on the confirm, and the task carries on with its conversation. Nothing switches without that click unless the owner turned automatic switching on (5.16).
- **The other account is checked before anything stops** (plan 4.16 step 3). When it is not signed in or needs attention, nothing is stopped and the bar says why: "「个人」没有登录，没有换。 / Personal is not signed in, so nothing was switched." or "「个人」需要你处理，没有换。 / Personal needs your attention, so nothing was switched." When an automatic switch still could not start a session on the other account, line 2 of the banner reads "Codepass 会在重置后让它继续 / Codepass continues it after the reset": the tool was stopped for the switch, so Codepass continues the session on its own account once a fresh reading shows room.
- **"等待 / Wait"** collapses the banner to one muted line: "等待重置 · 周一 00:00 / Waiting for reset · Mon 00:00". The task's state reads "等待重置 / Waiting for reset" (warn, steady dot) in every list.
- After an automatic switch (only when the owner turned automatic switching on for this tool, 5.16; plan 4.16 decision 33): info-tinted, one line, dismissible. It is the result line of the switch with the reason in front: "「工作」的用量用完了，已在「个人」上继续。对话完整带过去了。 / Work ran out of usage, so this session continued on Personal. The whole conversation came along." The second sentence follows the layer exactly as in the result-line table above, including "查看笔记 / View note". When programs the session started were ended by the switch, the line adds: "它启动的 2 个程序已结束。 / The 2 programs it started were ended."
- **Switch-back hint** (plan 4.16 decision 17). The app never switches back by itself. After an automatic switch, when a later reading shows that the account the session left has room again, the bar shows one quiet line, muted, not warn, with a link and a close button:

  ```
  「工作」又有用量了 · 切回「工作」                                                        [×]
  Work has usage again · Switch back to Work                                               [×]
  ```

  The link opens the same confirm as the menu, with "切回 / Switch back" as the primary. Nothing happens without that click. The hint shows once per reset, only between turns, and only for a session that was switched automatically; closing it keeps it away until the next reset. A switch the owner made by hand gets no hint: the menu is always there.
- **When the session does not resume as it is on the other account**, nothing is asked and nothing is offered: the next layer is tried by itself, and the result line says which one carried the conversation (the table above). The earlier danger line "在「个人」上接不上原来的对话 / The conversation could not be continued on Personal", its button "Start a new session on Personal" and the checkbox "Ask Work for a summary first" are removed.
- **An account problem is not a limit** (plan 4.16 decision 34). When the tool reports an authentication, billing, suspension, disabled-account or policy error, the bar shows a different second line, danger-tinted, `role="alert"`, with no other account on it:

  ```
  ! 「工作」需要你处理：Claude Code 报告这个账号的登录或账号本身有问题。              [查看账号]
    Codepass 不会自动换账号，也不会重试。
    > (the tool's own message, as plain text)
  ! Work needs your attention: Claude Code reports a problem with this account's sign-in or the account itself.   [Open the account]
    Codepass will not switch accounts or retry by itself.
  ```

  "查看账号 / Open the account" opens the Accounts page at that pass (5.16), where "重新检查 / Check again" and "重新登录 / Sign in again" are. The line stays until the account's state is cleared there.
- Usage credits on for that account: no limit banner appears, because the tool does not stop. Instead a muted line on the bar while the account is past 100%: "正在使用用量额度，按量计费 / Running on usage credits, billed by use."

**Account in the New task sheet** (milestone 8; plan 4.16 decisions 13 to 15): when the chosen tool has more than one AI account, a row under the tool cards, "用哪个账号 / Account", with a select. Any account of the tool can be chosen for any repository.

```
  用哪个账号 / Account   [ ● 工作 · 本周 81%                         v ]   这个仓库的默认 / This repository's default
                          ● 工作 / Work        本次 62% · 本周 81%      这个仓库的默认
                          ● 个人 / Personal    本次 0% · 本周 12%
```

- **What is pre-selected:** the default for this repository and this tool, from the three levels repository › project › app (4.2, 5.2, 5.16). A muted tag after the select names the source, in the vocabulary of the identity source tags (5.12): "这个仓库的默认 / This repository's default", "来自项目 acme / From project acme", "应用默认 / App default". Changing the tool card re-selects for that tool.
- **Options:** every account of the tool in the owner's order, each with its colour dot, label and latest reading ("工作 · 本周 81% / Work · week 81%"). An account with no room reads "用完了，周一 00:00 重置 / Used up, resets Mon 00:00" and can still be chosen. One that is not signed in reads "未登录 / Not signed in" and can be chosen too; the tool's own sign-in then appears in the terminal. One that needs the owner's attention reads "需要你处理 / Needs your attention" and cannot be chosen. When it is the default, nothing is pre-selected in its place: the select shows it, "开始任务 / Start task" is disabled, and the line under the row reads "「工作」需要你处理。先去处理，或者自己选另一个账号。 / Work needs your attention. Deal with it first, or choose another account yourself." with the link "查看账号 / Open the account" (plan 4.16 decision 34: the app never puts another account in its place by itself).
- **The default has no room** (decision 15): the sheet pre-selects the first account in the owner's order that has room and says so in one line under the row (`.hint`, info):

  ```
  用哪个账号 / Account   [ ● 个人 · 本周 12%                         v ]
  「工作」用完了，周一 00:00 重置，这次先用「个人」。
  Work is used up until Mon 00:00, so this task uses Personal.
  ```

  The default not signed in: "「工作」没有登录，这次先用「个人」。 / Work is not signed in, so this task uses Personal." No account has room: the default stays selected, with "所有账号都用完了。「工作」周一 00:00 重置；最早重置的是「个人」，15:45。 / Every account is used up. Work resets Mon 00:00; the first to reset is Personal, at 15:45." The line gives both times because nothing moves the task at the earlier one. For a Codex task it adds: "Codex 不会自己继续，到时要你点继续。 / Codex does not continue by itself; continue it then." A reading that is unknown counts as room. Here "room" is below 100% on every window; the queue's reserve is not counted for a task the owner starts by hand (the one rule in plan 4.16, after decision 15).
- **Picking the used-up account anyway** is allowed. The line becomes "「工作」用完了。任务会等到周一 00:00，或者自动换到有用量的账号。 / Work is used up. The task waits until Mon 00:00, or switches to an account with usage." (the second half only when automatic switching is on for that tool, 5.16). For a Codex task the line adds: "Codex 不会自己继续，到时要你点继续。 / Codex does not continue by itself; continue it then."
- **One task only.** The choice does not change any default. When the owner picked a different account by hand, a checkbox appears under the row, off: "以后这个仓库都用「个人」 / Use Personal for this repository from now on"; ticked, Start also saves it as the repository's default (4.2).
- **Several accounts at once** (plan 4.16 decision 29). Nothing limits which accounts run together: a task on Work in acme_api and a task on Personal in globex_web run side by side, each with its own chip. The sheet needs no extra control for it.
- **The shell decides** (decision 20): the select is disabled and shows what the default would be; the line under it is the sentence of 5.16: "你的终端环境设置了 CLAUDE_CONFIG_DIR，Claude Code 用哪个账号由它决定，Codepass 选不了。 / Your shell sets CLAUDE_CONFIG_DIR, so it decides which account Claude Code uses. Codepass cannot choose."
- OpenCode chosen, or a tool with one account: no row.
- The sheet opened from Projects (Cmd/Ctrl+Enter) or from the AI tasks page behaves the same once the repository is known.

**Terminal** (`.term`). xterm.js on `--term-bg`, 12.5px mono, 19px lines, 16px padding. The tool draws its own interface; the app adds nothing inside it. Copy on select is off; Cmd/Ctrl+C copies when there is a selection, otherwise it is sent to the process. Paste of more than 5 lines asks once. Shift+Enter inserts a new line in tools that support multi-line input. Chinese and other wide characters keep their columns; typing with a Chinese input method shows the candidate window at the cursor. `https://` links are Cmd/Ctrl+clickable and open in the browser; `http://localhost` and `http://127.0.0.1` links (dev servers) also open, after a one-time confirm per port; other `http://` links are copied, not opened. File paths with a line number open in the Code tab. A program inside the terminal cannot write to the clipboard. Scrollback 10,000 lines, kept while the task exists; a terminal stays alive when you switch page, tab or folder, so coming back shows the exact screen you left. Search in scrollback: Cmd/Ctrl+F inside the terminal. Settings has "屏幕阅读器模式 / Screen reader mode" for the terminal (off by default; it exposes the output as text at some cost in speed).

**Keep-awake line.** At the bottom of the task list while held: cup + "有任务在运行，这台 Mac 保持唤醒。合上盖子仍会睡眠。 / Tasks are running, so this Mac stays awake. Closing the lid still sleeps it." The same state shows in the status bar, the rail and the sidebar.

**Other states.**

- *No tasks*: `.empty` with the three tool marks: "让 AI 在这个仓库里干活 / Put an AI to work in this repository", text "每个任务有自己的文件夹和分支，可以同时开几个，互不影响。 / Each task gets its own folder and branch. Run several at once; they cannot disturb each other.", primary "新任务".
- *Tool not installed*: the terminal area shows a card instead: tool mark, "还没有安装 OpenCode / OpenCode is not installed yet", the exact command that will run, primary "安装并开始 / Install and start", then the install log in the same terminal surface.
- *Tool needs sign-in*: nothing special. The tool's own sign-in appears in the terminal; a one-line hint above it the first time: "登录由 Claude Code 自己处理，Codepass 不保存任何 AI 账号信息。 / Sign-in is handled by Claude Code itself. Codepass stores no AI account details."
- *Process ended*: a bar under the terminal: done "任务结束了 / The task ended" + "查看改动", "再开一个会话 / Start another session"; failed shows the exit code and "重新开始 / Start again".
- *App restarted*: tasks are listed as stopped with "继续 / Resume". Resume reopens **that task's own conversation** in its folder (the app remembers each session's identifier), so two tasks in one folder never resume into each other. Resume looks for the conversation under every session ID the task has had, newest first, in the task's account folder and then in the tool's other account folders (plan 4.16, "Switch and resume"). Found in another account's folder, it is a switch, with the confirm and the result line above. Found nowhere, **no session starts**: "找不到这个任务的对话记录，没有开新的会话。 / This task's conversation record could not be found, so no new session was started." with the button "开一个新会话 / Start a new session", the owner's explicit choice. A switch that the restart interrupted is undone: the task is stopped on the account it was on, with "上次换账号没有完成，会话还在「工作」上。 / The last switch did not finish. The session is still on Work.", "继续 / Resume" and the account menu to retry.
- *Fifth concurrent task*: a confirm, once: "已经有 4 个任务在运行。同一个账号下的任务共用它的用量，开得越多用得越快。 / 4 tasks are already running. Tasks under the same account share its usage allowance; more tasks spend it faster." 继续 / 取消. The cap (default 6) is in Settings.
- *Quit with running tasks*: the quit dialog (5.6b), every time. Closing the window does not ask: it hides the window and the tasks keep running (4.4).
- *Stop task*: ends the tool and every program it started (a dev server, a test watcher). The confirm says so when such programs exist: "还会结束它启动的 2 个程序 / This also ends 2 programs it started".

#### 5.6a Closed, but still running (`ai-leftover`)

Shot: `ai-leftover-light-zh` (drawn on the AI tasks page; the same rows appear in the repository's task list).

An AI tool starts other programs: dev servers, test watchers, helper agents, shells. When the session's tab is closed or the tool exits, some keep running. Codepass keeps track of everything a session started and shows what is left.

- **In the repository's task list** (5.6): a group "已关闭，仍在运行 / Closed, still running" between the live tasks and "已结束 / Finished". Row: tool mark, title; second line: warn dot + "3 个进程 / 3 processes", "CPU 41%", and a small danger-ghost button "强制停止 / Force stop". Selecting the row shows, in place of the terminal, the process list and the session's last screen (read-only).
- **On the AI tasks page** (5.7): a warn-outlined section (`.leftover`) above everything else, shown only while such sessions exist. Head: alert icon, "2 个会话已经关闭，但它们启动的程序还在运行 / 2 sessions are closed, but programs they started are still running", sub "这些程序还在占用 CPU 和内存，电脑也因此保持唤醒。 / They still use CPU and memory, and they keep this Mac awake.", button "全部强制停止 / Force stop all". Table columns: 会话 / Session (title; repository with both names; where it ran), 还在运行 / Still running (count), 占用 / Using (CPU and memory), 关闭时间 / Closed ("会话 12 分钟前关闭 / session closed 12 min ago"), actions: "查看进程 / Show processes" (toggles an inline list `.procs`: PID, command line, CPU, memory) and "强制停止 / Force stop".
- **Force stop** confirms once, naming what ends: "强制停止这 3 个程序？vite、esbuild、sh。没保存的东西会丢失。 / Force stop these 3 programs? vite, esbuild, sh. Anything they have not saved is lost." Primary "强制停止 / Force stop", focus on Cancel. Then the row shows a spinner and "正在停止… 5 秒后强制结束 / Stopping... forced after 5 seconds". When nothing is left the row leaves with the remove motion and a toast says "已停止 3 个程序 / Stopped 3 programs". If something survives (it belongs to another user, or it was restarted by the system): danger "还有 1 个程序停不掉 / 1 program could not be stopped" with its PID and "复制 PID / Copy PID".
- The count, CPU and memory refresh every few seconds; numbers do not animate.
- Command lines are shown as plain text and never written to the log. A process the session did not start is never listed.
- Keep-awake: these programs count. The sidebar chip reads "保持唤醒中 · 2 个后台程序 / Keeping awake · 2 background programs" when they are the only reason.
- Accessibility: the section is a `region` with its heading; Force stop buttons carry the session's title in their label; the countdown is announced once.

#### 5.6b Quit with live sessions (`quit-dialog`)

Shot: `quit-dialog-light-zh`.

Shown **every time** the app is asked to quit while any AI session is live or any closed session still has programs running. It has no "don't ask again" box and no setting; the footer says so. `role="alertdialog"`, 600px.

- Header: "退出 Codepass？ / Quit Codepass?"; sub "还有 3 个 AI 会话在运行。 / 3 AI sessions are still running." (with leftovers: "还有 2 个 AI 会话在运行，另有 1 个已关闭的会话留下了程序。 / 2 AI sessions are running, and 1 closed session left programs behind.")
- The list (`.affect`): tool mark, title, repository (both names when they differ) and "还启动了 2 个程序 / started 2 programs", state on the right. More than 5 rows scroll.
- Unsaved files, when any, as one more row: "3 个文件还没保存，会留在恢复区 / 3 unsaved files are kept for recovery".
- **Two choices** (`.choice`, two cards side by side; each is one button):
  - "全部停止并退出 / Stop all and quit" (danger title, stop icon): "结束这 3 个会话和它们启动的 3 个程序。分支、改动和对话都保留，下次可以继续。 / Ends these 3 sessions and the 3 programs they started. Branches, changes and conversations are kept, so you can resume later."
  - "留在后台继续运行 / Keep running in the background" (cup icon; **default focus**, ink outline): "只关闭窗口。Codepass 留在程序坞里，会话不中断，电脑继续保持唤醒。 / Only the window closes. Codepass stays in the Dock, the sessions carry on and this Mac stays awake." Windows: "留在系统托盘里 / stays in the system tray".
- Footer: left, muted: "每次退出都会问，不会替你记住选择。 / Codepass asks every time and never remembers a choice for you."; right: "取消 / Cancel" (Esc), which closes the dialog and changes nothing.
- Keys: Tab moves between the two cards and Cancel; Enter takes the focused card; Esc cancels. There is no single-letter shortcut for "Stop all".
- After "Stop all": the dialog shows "正在停止 3 个会话… / Stopping 3 sessions..." with a small progress line, then the app exits. If a program survives the forced stop, the app still exits and the next launch says so once.
- Logout, restart or shutdown of the computer: no dialog. Everything stops and the app quits at once; unsaved edits go to the recovery store. On Windows, cancelling at the system's "apps are preventing shutdown" screen keeps Codepass open with its sessions. (The earlier 20-second countdown was dropped at the group-B merge: Wails 3.1.0 cannot delay a logout.)
- The update dialog's "Reopen now" (5.13) does not show this dialog a second time; its own banner already says what happens.

**New task** (`new-task`, modal 580px, one screen, no wizard).

1. **用哪个工具 / Tool** (`.tool-pick`): three cards with mark, name and status line ("已安装 2.4.11", "未安装，开始时安装 / Not installed. Installs on start"). Remembers the last choice per repository.
2. **在哪里运行 / Where it runs** (`.opt`): "独立工作区 / Own workspace" (badge "推荐 / Recommended") or "主工作区 / Main workspace". If another task already runs in the main workspace, the second option shows a warn line: "已有一个任务在主工作区运行，两个任务会同时改同一份文件。 / Another task is already running in the main workspace. Both would edit the same files."
3. **要 AI 做什么 / What should the AI do?** (`.textarea.task-desc`, 3 rows, grows to 8, then scrolls): the task description. It is sent to the chosen tool as the task's first message, and to nothing else. There is no task name field: the AI names the task at Start. On the right of the label, muted: `⌘ ⏎` "开始 / to start" (Ctrl on Windows).
   - Hint, line 1 (the tool name follows the chosen card): "会作为第一条消息发给 Claude Code，任务名也由它根据这段话来起。 / Sent to Claude Code as its first message. Claude Code also names the task from it."
   - Hint, line 2, own workspace only: "分支 `ai/<英文短名>` 和文件夹也在开始时命名。 / Branch `ai/<short-name>` and its folder are named when the task starts." Main workspace has no line 2.
   - Empty is allowed and Start stays enabled. Placeholder: "写下要做的事，比如“修复结算金额的四舍五入”。也可以留空，进了终端再说。 / Describe the job, for example “fix the rounding in settlement totals”. Or leave it empty and say it in the terminal." While the box is empty the hint reads: "留空也可以：任务先叫“新任务 3”，你发出第一条消息后再改成合适的名字。 / Empty is fine: the task is called “New task 3” until your first message gives it a name." and, for an own workspace, "分支 `ai/new-task-3`，文件夹同名，之后不变。 / Branch `ai/new-task-3` and a folder of the same name. They keep that name." The tool then starts with no first message. The first message renames the label only.
   - Naming at Start: a short title in the description's language becomes the list label; an English kebab-case slug becomes the branch `ai/<slug>` and the folder `~/.codepass/worktrees/<repo>/<slug>`. A taken branch name gets `-2`. If naming fails or takes more than 5 seconds, a name made from the description's first line is used. No error is shown.
4. A muted note about clean copies and `.worktreeinclude`. If the repository has submodules or a setup command, a second line: "开始前会准备子模块并运行 npm ci / Submodules are prepared and npm ci runs before the task starts".
5. Footer: "这个仓库已有 2 个任务在运行" on the left, 取消, primary "开始任务 / Start task".

Focus starts in the description. Cmd/Ctrl+Enter starts the task. Start shows the button with a spinner and "正在起名… / Naming the task..." (about 1-3 seconds; skipped when the description is empty), then "正在准备工作区… / Preparing the workspace...". While either runs, the fields are dimmed and locked (state `naming`); 取消 / Cancel stays active. Failure to create the workspace stays in the modal with the git message, and the description is kept. Mockup states: default (filled), `empty`, `naming`.

### 5.7 AI tasks across repositories (`ai-tasks`)

Shots: `ai-tasks-light-zh`, `ai-tasks-light-en`, `ai-leftover-light-zh`.

Header "AI 任务" with "5 个任务 · 2 个运行中 · 1 个等待你" (plus "· 2 个已关闭仍在运行" when true). Actions: "切换会话 ⌘J / Switch session" (ghost; opens 5.7a), "释放空间 6.4 GB / Free up space" (trash icon and the total that can be freed; opens 5.7b; hidden when nothing can be freed) and primary "新任务" (asks for the repository first). Then, in this order: the "closed, still running" section when it applies (5.6a); an ok banner while keep-awake is held (the reason, when it ends, the lid limit, a link to Settings); one table per repository (`.tasks-table`): tool, task (title + branch), where it runs, state, elapsed, one action ("去回答 / Answer" in primary for waiting tasks, "查看 n 个改动 / Review n files", or "打开 / Open"). The group label shows both names when they differ. Clicking a row opens that repository's AI tasks tab on that task. A finished task's row menu has "删除这个会话的数据… / Delete this session's data..." (opens 5.7b with that one row ticked). Empty: the same `.empty` as 5.6 with "选择仓库 / Choose a repository".

**Account on a row** (milestone 8; when the task's tool has more than one AI account): the account chip of 5.6 (colour dot + label) sits after the tool mark, for Claude Code and Codex tasks alike. It is the same "换账号 / Switch account" menu as on the session bar, with the same confirm, so a session can be moved without opening its terminal; the row shows "正在换到「个人」… / Switching to Personal..." in place of the state while it runs. A row that was switched automatically (only with automatic switching on, 5.16) shows a small muted "自动换的 / switched automatically" after the chip until the owner opens the task. A row whose last switch was carried by a handoff note shows a small muted "交接笔记 / handoff note" there instead, a link to the note (5.6). A row whose account needs the owner's attention shows the chip in danger with "需要你处理 / Needs your attention" and no menu. The switch-back hint is only on the session bar. A task that recorded no account because the shell decided shows "由终端环境决定 / set by your shell" in place of the chip, with no menu. A stopped or ended task whose account was removed shows "已移除的账号 / Removed account"; its Resume asks for an account first (5.6).

The page and the switcher read the same list, so their order and counts always agree.

#### 5.7a Session switcher (`session-switcher`)

Shots: `session-switcher-light-zh`, `session-switcher-dark-en`.

The owner runs several sessions in several repositories at once. Cmd+J (Ctrl+J on Windows; Ctrl+Shift+J there while a terminal has focus) opens one list of all of them, over whatever is on screen, in both layouts. It is a palette, not a page: it opens in under 100 ms from data the app already holds, and it closes on Esc or on a click outside, giving focus back to where it was.

**Layout** (`.switcher`, 720px, top of the window, 92px from the top edge, backdrop as a modal).

- **Head**: search icon, a field with the placeholder "搜索会话、仓库或分支 / Search sessions, repositories or branches" (focused on open), then the count "4 个在运行 · 1 个等待你 / 4 live · 1 waiting for you" and the shortcut keys.
- **List** (`role="listbox"`), in groups with small labels and counts, in this fixed order:
  1. **等待你 / Waiting for you**: needs an answer or a permission. One short reason in warn after the branch when the app knows it: "要你批准一条命令 / needs you to approve a command", "这一轮做完了 / finished its turn".
  2. **运行中 / Running** (including "running, no signal"), newest activity first.
  3. **已关闭，仍在运行 / Closed, still running**: instead of a state and a time, "3 个进程 / 3 processes" and "CPU 41%".
  4. **刚结束 / Just ended**: done, failed or stopped in the last 10 minutes, so a session that finished while the owner looked elsewhere is still one keystroke away.
- **Row** (`.sess`, 52px): tool mark; title; second line: provider mark, repository name (both names when they differ), branch in mono or "主工作区 / main workspace"; on the right the state (dot + word), elapsed time in mono, and a digit key (1 to 9). When the session's tool has more than one AI account (milestone 8), the second line ends with the account: its colour dot and label, "● 工作 / ● Work". It is display only here; switching is done on the session bar or the AI tasks page. A session that recorded no account because the shell decided reads "由终端环境决定 / set by your shell" there; one whose account was removed reads "已移除的账号 / Removed account".
- The selected row has the ink bar on its left and a raised surface. On open, the selection is the first waiting session that is not the one already on screen; with none waiting, the most recently used other session. So Cmd+J, Enter flips between the two sessions in use.
- **Foot**: key hints: "↑↓ 选择 / Select", "⏎ 跳到这个会话的终端 / Go to its terminal", "⌘J 下一个 / Next", "⌘⏎ 查看改动 / Review changes"; on the right a link "打开 AI 任务页 / Open AI tasks".

**Interactions.**

| Key or action | Result |
|---|---|
| Enter, or click a row | Opens that repository's workspace on the AI tasks tab with that session selected and **the cursor in its terminal**. For a closed session: its row in the task list with the process list. |
| Cmd/Ctrl + Enter | Opens that session's changes (the working tree switched to its workspace). Disabled for a main-workspace session with no changes. |
| Cmd + J again (Ctrl + J) | Moves the selection down one row, wrapping. Holding Cmd and tapping J walks the list. |
| 1 to 9 | Jumps to that row at once (only while the search field is empty). |
| Typing | Filters by title, repository (either name), branch or account label; groups stay; matches are bold. No match: "没有匹配的会话 / No matching sessions". |
| Esc, click outside | Closes; focus returns. |

**States.** No live session at all: the list is replaced by a quiet line "现在没有在运行的 AI 会话 / No AI session is running right now" and one button "新任务 / New task". A session that ends while the list is open changes its state in place; the selection does not jump. More than about 9 rows: the list scrolls inside the panel, groups stay in order.

**Motion.** The panel fades in and drops 4px (160ms), like other popovers; rows do not animate when the list is filtered.

**Accessibility.** `role="dialog"` labelled "切换会话 / Switch session"; the field controls the listbox with `aria-activedescendant`; each option's name is "title, repository, state, elapsed", with "账号 工作 / account Work" added when the account is shown; group labels are presentation. The shortcut is listed in the View menu and in the palette.

#### 5.7c Open repository (`repo-switcher`)

Owner request (2026-11-16, second batch; plan 4.4): find the repository to work in by typing its folder name, from any screen. Not drawn yet: A2-16 is "Fable first" (shots `repo-switcher-light-zh`, `repo-switcher-dark-en`).

The same panel as the session switcher (5.7a: `.switcher`, position, backdrop, motion, focus return), opened with Cmd/Ctrl+O or from the app menu "打开仓库… / Open Repository…".

```
  🔍 输入文件夹名…                                    14 个仓库   ⌘O
  ─────────────────────────────────────────────────────────────────
  最近打开 / Recent
  ▌ [aws] acme_agent_portal          acme · main · 2 个 AI 任务运行中
    [aws] api-main  acme_api         acme · ai/fix-rounding
    [gh]  globex_web                 globex · main
  全部 / All (A–Z)
    [bb]  initech_tools              umbrella · develop
  ─────────────────────────────────────────────────────────────────
  ↑↓ 选择   ⏎ 打开   ⌘⏎ 打开并新建任务   esc 关闭
```

- **Rows**: provider mark; the local folder name (the remote name as the small tag when it differs, as in 5.2); second part: project, current branch, and the live AI task count when there is one. Recent first (the last 5 opened), then all others A to Z; with text typed, one list ranked as in 5.2's search, matched letters bold.
- **Keys**: Enter opens the repository's workspace (the last tab used there); Cmd/Ctrl+Enter opens it with the new-task sheet; 1 to 9 jump while the field is empty; Esc or a click outside closes and returns focus.
- **Placeholder** "输入文件夹名… / Type a folder name..."; no match: "没有匹配的仓库 / No matching repository" with "去项目页 / Go to Projects".
- **Not listed**: hidden repositories. A repository whose folder is gone is listed dimmed with "找不到这个文件夹 / This folder is gone"; Enter on it opens its row on the Projects page.
- **Windows**: Ctrl+O inside a terminal goes to the terminal program; there the menu item opens the panel.
- **Accessibility**: `role="dialog"` labelled "打开仓库 / Open repository", the field drives the listbox with `aria-activedescendant`; option name "folder name, remote name if different, project, branch".

#### 5.7b Free up space (`free-space`)

Shot: `free-space-light-zh`.

Ended sessions keep things on disk: a workspace (often with gigabytes of build output), Codepass's own terminal record, and the AI tool's own record of the conversation. This dialog shows how much, and deletes only what the owner ticks, only after showing every path. Modal, 880px, over the AI tasks page.

**Layout, top to bottom.**

1. Header: "释放空间 / Free up space"; sub "4 个已结束的会话占用 6.4 GB。正在运行的 3 个会话不在这里，也不会被动到。 / 4 ended sessions use 6.4 GB. The 3 sessions that are running are not listed and are never touched."
2. **Table** (`.fs-table`), one row per ended session: checkbox; tool mark + title + "repository · how it ended · when"; then three sizes and the total:
   - **工作区 / Workspace**: the size, and under it the part that is build output ("其中 node_modules 2.1 GB / 2.1 GB of it is node_modules"). "没有（主工作区）/ None (main workspace)" for a session that ran in the repository folder: that folder is never offered.
   - **终端记录 / Terminal log**: Codepass's own saved screen and hook files for that session, and its handoff notes ("含 2 份交接笔记 / includes 2 handoff notes").
   - **工具的对话记录 / Tool transcript**: the files Claude Code or Codex keep for exactly that session, under every session ID the task has had, and the repaired conversation copies Codepass made at account switches (plan 4.8a). A small muted count under the size when there are any: "含 1 份修好的副本 / includes 1 repaired copy". For OpenCode: "由 OpenCode 保管 / kept by OpenCode" (it stores sessions in its own database; Codepass asks OpenCode to delete the session and cannot show a size).
   - **合计 / Total**, bold.
   Sessions that merged or were discarded are ticked by default; a session whose workspace holds commits that exist nowhere else is **not** ticked, its row is warn-tinted and its workspace cell says "有 2 个提交没有合并 / 2 commits not merged anywhere".
3. **Extra confirmation** (`.fs-extra`, warn box) for each such session: a checkbox "也删除「重写对账报表的导出」（2 个提交还没合并）/ Also delete "Rewrite the reconcile report export" (2 commits not merged)" with "这 2 个提交 14 天内可以在「最近丢弃」里找回；没提交的文件不能找回。 / The 2 commits can be brought back from Recently discarded for 14 days. Files that were never committed cannot." Ticking it ticks the row.
4. **The exact list** (`.paths`): label "将要删除的 9 项，一项不多 / Exactly these 9 items will be deleted". One line per path: the full path in mono (long paths wrap; nothing is shortened with an ellipsis), what it is, its size. A repaired copy reads "修好的对话副本（换到「个人」时建的）/ Repaired conversation copy (made for the switch to Personal)"; a handoff note reads "交接笔记 / Handoff note". They are listed like every other path: full path, kind, size. An OpenCode session appears as the command that will run (`opencode session delete ses_...`), in info colour, with "由 OpenCode 自己删除这个会话 / OpenCode removes this session itself". The list follows the ticks at once and scrolls inside its box.
5. Footer: left "已选 3 个会话 · 将释放 4.27 GB · 不经过废纸篓 / 3 sessions selected · frees 4.27 GB · does not go through the Trash"; 取消 / Cancel; danger primary "删除并释放 4.27 GB / Delete and free 4.27 GB". Focus starts on Cancel.

**Rules the screen makes visible.**

- A session that is starting, running, waiting, or closed with programs still running is not in the table. The owner who wants its space stops it first.
- Only paths that carry that session's full ID are listed. Two sessions in one folder never share a line. A repaired copy has its own session ID; it is listed because its exact path was recorded on the task when it was made, never by a pattern.
- Copies and notes are deleted for that task only. The conversation records and copies of other tasks are not touched, and nothing here reaches a backup or an export. This dialog is the only place where Codepass deletes a file of a tool (plan 4.16 decision 2).
- What is listed is exactly what is deleted. If a listed path changed in a way that makes it unsafe (it is now a link pointing elsewhere), that line is skipped and reported.
- Nothing is deleted from a repository's main folder.

**States.**

| State | What shows |
|---|---|
| Measuring | sizes show a skeleton and "正在计算… / Measuring..."; rows can already be ticked; the primary button is disabled until every ticked row has its sizes |
| Nothing to free | `.empty`: "没有可以释放的空间 / Nothing to free up" with "已结束的会话没有留下文件 / Ended sessions left no files behind" |
| Deleting | the list becomes a progress list, one check per path; Cancel reads "停止 / Stop" and stops before the next path (what is already deleted stays deleted, and the report says which) |
| Done | the dialog closes; toast "已释放 4.27 GB / Freed 4.27 GB"; the sessions leave the lists with the remove motion |
| Partly failed | the dialog stays: "已释放 4.1 GB，有 1 项没删掉 / Freed 4.1 GB. 1 item was not deleted", the failed path with the system's reason, "重试 / Try again" and "复制路径 / Copy path" |
| A session started running meanwhile | its row turns disabled with "已经在运行，不会删除 / Running again, so it will not be deleted" |

**Accessibility.** The table is a grid with a header row; each checkbox is labelled with the session title; the path list is a real list whose items read "path, kind, size"; the footer total is `aria-live="polite"`; the primary button's name includes the amount.

### 5.8 History, Branches, Stash

**History** (`repo-history`; shots `repo-history-light-zh`, `repo-history-dark-en`).

- Left: toolbar (search by message, author or hash; 全部分支 / current branch; filter by author, path, date), then the commit list (`.commit`, 44px rows): lane graph (two-colour limit: neutral for the current branch line, info for others; the checked-out commit is a ring), ref chips (`.ref--head` current branch, `.ref--remote`, `.ref--tag`, `.ref--ai` for task branches), message, author initial + name + relative time, short hash. Unpushed commits show `↑`. Virtualised; loads 200 at a time.
- Right: commit details (`.cdetail`): title, body, author with email, date, hash with copy; actions: "从这里建分支 / Branch from here", "拣选到 main / Cherry-pick onto main", "还原这次提交 / Revert this commit", "打标签 / Tag", "重置 main 到这里 / Reset main to here". Then the changed files and the diff of the selected one. Selecting two commits (Cmd/Ctrl+click) shows the comparison between them.
- **Reset** opens a confirm with three options (`.opt`): "保留改动，取消提交（soft）", "保留文件，取消暂存（mixed）", "全部丢弃（hard）". Hard is danger-styled and states "会丢掉 2 个提交之后的全部改动，以及现在还没提交的 3 个文件 / Discards everything after these 2 commits and the 3 files you have not committed". Focus on Cancel.
- **File history and blame**: from any file's history button. History filters the list to that file with the file's diff per commit. Blame opens the file in the Code tab with a left margin of author initial + date + short hash per block; clicking a block selects that commit here.
- States: empty repository "还没有提交 / No commits yet"; search with no result; loading skeleton rows; detached checkout shown as a ring with the chip "游离 / Detached".

**Branches** (`repo-branches`; shots `repo-branches-light-zh`, `repo-branches-light-en`).

- Left (`.refs`): filter; **本地 / Local** (current branch has a check and the "当前 / Current" badge; ahead / behind per branch; task branches use the bot icon), **远程 / Remote** per remote, **标签 / Tags**, **远程仓库 / Remotes**. "新建分支 / New branch" (name, from which branch or commit, "创建后切换 / Switch to it").
- Right: the selected branch: name (mono, 18px), what it tracks, last commit; primary "切换到这个分支 / Switch to this branch"; More: Rename, Push, Set upstream, Delete. **Compare** (`.cmp`): two cards "这里有、main 还没有的提交 3" and "main 有、这里还没有的提交 1", "换一个比较对象 / Compare with another". Actions: "合并到 main / Merge into main", "变基到 main 之上 / Rebase onto main", "把 main 合并进来 / Merge main into this", and for GitHub / Bitbucket "创建拉取请求 / Create pull request". Then a card with Commits / Files changed. A warn banner predicts conflicts when both sides touched the same file ("合并时 1 个文件可能冲突 / 1 file may conflict when merging").
- Switching with a dirty tree asks: "main 上有 3 个还没提交的改动 / You have 3 uncommitted changes on main" - 带过去 / 先贮藏 / 取消 (Bring them along / Stash them first / Cancel).
- Delete warns about unmerged commits with the count; a pushed branch offers "同时删除远程分支 / Also delete the remote branch".
- Rebase is plain (no interactive list in 1.0). It runs with the `.opbar`: "正在把 feat/payout-batch 变基到 main · 第 2 / 3 个提交" with 中止 / 跳过 / 继续. A rebased branch that was pushed offers "推送（覆盖远程，带保护）/ Push (replace remote, with lease)". The confirm lists the remote commits that will be replaced, counted from **the last remote commit you pulled or pushed**, not from whatever the app fetched in the background: "远程 origin/feat/payout-batch 上的这 2 个提交会被替换 / These 2 commits on origin/feat/payout-batch will be replaced" with author and subject per row. If someone pushed after that point, the list shows their commits in warn with "有别人的新提交，建议先拉取 / Someone else pushed. Pull first", and the primary button is "先拉取 / Pull first"; replacing other people's commits needs a second, explicit click. If git refuses at push time: "远程在你不知道的时候变了，没有推送 / The remote changed without you seeing it. Nothing was pushed".
- Tags: create (name, message, at which commit), push, delete. Remotes: add, rename, set URL, remove, each with the backup-first rule.
- **Clean folder first.** Merge, rebase, cherry-pick, revert and pull need a folder with no uncommitted changes, so that Abort can always put everything back. With changes present the action asks: "先把 3 个改动贮藏起来，结束后再取回 / Stash your 3 changes first and bring them back afterwards" - 贮藏并继续 / 取消. The stash is restored automatically after the operation finishes or is aborted; if restoring conflicts, the stash stays in the list and a toast says so.

**Stash** (`repo-stash`; shot `repo-stash-light-zh`). Lives in the Changes tab: the Stashes group in the left pane expands into rows (archive icon, message, `stash@{0}`, branch, time, file count). Right: stash header with "取出并删除 / Apply and remove" (primary), "取出，保留贮藏 / Apply and keep", "建成分支 / Make a branch", "删除 / Delete" (confirm), then files and diff. "贮藏当前改动 / Stash changes" opens a small popover: message, "包含未跟踪的文件 / Include untracked files" (on), "只贮藏已暂存的 / Only what is staged". Applying with conflicts goes to the conflict screen. Empty: "没有贮藏 / No stashes".

### 5.9 Merge conflicts (`repo-merge-conflict`)

Shots: `repo-merge-conflict-light-zh`, `repo-merge-conflict-dark-en`.

Used by merge, pull, rebase, cherry-pick, revert and stash apply. It takes over the Changes tab until the operation finishes or is aborted.

- **Operation bar** (`.opbar`, warn tint, full width): icon, "正在把 feat/payout-batch 合并到 main / Merging feat/payout-batch into main", "3 个文件有冲突，已解决 1 个", a small progress bar, "放弃这次合并 / Abort merge" (danger ghost), primary "完成合并 / Finish merge" (disabled until every file is resolved; tooltip "先解决全部冲突"). The verb follows the operation ("继续变基 / Continue rebase").
- **Left pane**: 还有冲突 / Still in conflict (count of conflicts per file in danger), 已解决 / Resolved (ok check), 自动合并，没有冲突 / Merged automatically (collapsed). A footer line: "合并开始前这里没有未提交的改动，所以放弃后 main 会回到合并前的样子。 / There were no uncommitted changes when this merge started, so aborting puts main back exactly as it was." This sentence is true because of the clean-folder rule in 5.8; if changes were stashed first, the line adds "你贮藏的 3 个改动会自动取回 / Your 3 stashed changes come back automatically".
- **Conflict editor** (`.cf`): file header with "2 处冲突，已解决 1 处", 上一处 / 下一处 (F7), "在编辑器中打开". Top half: two read-only panes, **我的 / Mine** (current branch, info dot and tint) and **对方的 / Theirs** (incoming branch, iris dot and tint), each with "整个文件用我的 / Use mine for the whole file". Bottom half: **合并结果 / Result**, editable. Each conflict is a block bar (`.cf__block`): "冲突 2 · 两边都改了这 2 行" with **用我的 / Take mine**, **用对方的 / Take theirs**, **两边都要 / Take both**. A resolved block turns ok: "冲突 1 · 已采用对方的" with 撤销 / Undo. Editing the result by hand also resolves a block. When no block is open, "标记为已解决 / Mark as resolved" enables and the file moves to Resolved.
- Mine and theirs are told apart by label, dot and position, not only by tint. For rebase the labels are "基底上的 / On the base" and "这次提交的 / This commit" because "mine / theirs" are reversed there; the UI never shows the raw git terms.
- Binary or deleted-on-one-side conflicts get a choice card instead of the editor: "main 删除了这个文件，feat/payout-batch 修改了它 / main deleted this file, feat/payout-batch changed it" - 保留文件 / 删除文件.
- Finishing a merge opens the composer with the default message filled in.
- Abort confirms only when blocks were already resolved by hand ("已经解决的 1 处会丢失").

### 5.10 Pull requests (`repo-prs`, `pr-detail`)

Shots: `repo-prs-light-zh`, `repo-prs-light-en`, `pr-detail-light-zh`, `pr-detail-dark-en`.

Available for GitHub and Bitbucket repositories whose account has an API token. For CodeCommit the tab shows a quiet card: "这个仓库在 CodeCommit，拉取请求请在 AWS 控制台里管理 / This repository is on CodeCommit. Manage pull requests in the AWS console" with "打开控制台 / Open the console".

**List.** Toolbar: 我创建的 / 等我评审 / 全部 (Mine / To review / All); 打开和最近的 / 已合并 / 已关闭; search; primary "新建拉取请求 / New pull request". Rows (`.pr-row`): state icon (open ok, draft neutral, merged iris, closed danger; always with a text badge for draft, merged, closed), title + number, `from → base` chips, author ("我创建的 / by me"), "来自 AI 任务 / from an AI task" chip, "等你评审 / Needs your review" in warn; checks (`.checks`: 6 项通过 / 3 / 6 运行中 / 1 项失败); reviews (avatar stack + "1 / 2 批准"); updated; comments. A footer line says which account and token read the list and when. Refresh on focus, and every 2 minutes for the list on screen only. Repositories not on screen refresh every 15 minutes at most, and unchanged answers cost nothing against the provider's limit. The cross-repository page shows "5 分钟前更新 / updated 5 min ago" per repository.

**Create** (sheet from the list, from Branches, or from a pushed AI task; not drawn). Fields: 从 / From (current branch, pushes it first if needed), 合并到 / Into (default branch), 标题 / Title (pre-filled from the branch's first commit), 说明 / Description (Markdown, pre-filled with the commit list), 评审人 / Reviewers (people picker from the repository's collaborators), "创建为草稿 / Create as draft" (GitHub). Below: commits and files that will be included. Primary "创建拉取请求".

**Detail.** Back link; title + number; meta (state badge, branches, author, age, commit count); "在 GitHub 上打开 / Open on GitHub". Tabs: 概览 / 提交 / 改动的文件 (Overview / Commits / Files changed; Files uses the app's diff, read-only). Overview left: description card (Markdown shown safely: no raw HTML, no remote images, links open in the browser after the https check), then a timeline (`.tl`): pushes, reviews, comment summaries. **Line comments are not shown inline in 1.0**: the timeline row "aina-r 留下了 3 条逐行评论" links to the provider ("在 GitHub 上查看和回复 / View and reply on GitHub"). Avatars are always monogram tiles; no picture is loaded from the provider. Right column:

- **Merge box** (`.mergebox`): "可以合并 / Ready to merge" with the facts ("检查全部通过 · 1 个批准 · 和 main 没有冲突"), merge method select (创建合并提交 / 压缩成一个提交再合并 / 变基后合并, limited to what the repository allows), "合并后删除 feat/phone-verify", primary "合并拉取请求 / Merge pull request", danger-ghost "关闭，不合并 / Close without merging" (Bitbucket: "拒绝 / Decline"). Not mergeable: the head line turns warn and lists what is missing ("还差 1 个批准", "2 项检查没通过", "和 main 有冲突" with "在本地解决 / Resolve locally" which checks out the branch and merges base into it). For a PR that waits for the user's review: "批准 / Approve" and "要求修改 / Request changes" replace the merge button when the user is not the author.
- **Reviewers**: avatar, name, state (已批准 / 要求修改 / 还没评审), "添加 / Add".
- **Checks**: one row per check with state and duration; a failed check links to its log on the provider.

**States.** No token: `.empty` "要在这里管理拉取请求，需要给 alex-dev 添加 API 令牌 / To manage pull requests here, add an API token for alex-dev" + "添加令牌 / Add token". Token expired: warn banner "alex-dev 的令牌过期了 / The token for alex-dev has expired" with "更新令牌 / Update token". Token lacks a scope: the banner names it ("令牌缺少 read:org / The token is missing read:org"). The organisation needs single sign-on authorised: "acme 组织要求先为这个令牌授权单点登录 / The acme organisation needs single sign-on authorised for this token" + "去授权 / Authorise" (opens GitHub). The organisation blocks classic tokens: a quiet card "acme 组织不允许经典令牌，这里的拉取请求请在浏览器里处理 / The acme organisation does not allow classic tokens. Handle its pull requests in the browser" + "在 GitHub 上打开 / Open on GitHub". Loading: skeleton rows. Offline: the last list with "离线，显示的是 10 分钟前的内容 / Offline. Showing what was loaded 10 minutes ago". Rate limited: "GitHub 暂时限制了请求，3 分钟后自动重试". Merge failed: the provider's message in a danger banner inside the merge box.

### 5.11 Accounts (`accounts`, `add-account`)

Shots: `accounts-light-zh`, `accounts-dark-en`, `add-account-light-zh`, `add-account-light-en`.

**Page.** Three provider groups, each introduced by a `.section-label` with the provider mark: **AWS · IAM Identity Center** (today's portal sections and passes, unchanged, including the legacy panel when static keys exist), **GitHub**, **Bitbucket**. From milestone 8 a fourth group follows them, **AI 账号 / AI accounts** (Claude Code, Codex), specified in 5.16; the chooser of "Add account" then also lists "Claude Code 账号 / Claude Code account" and "Codex 账号 / Codex account". "添加账号 / Add account" is a split button: the main part opens a chooser (AWS SSO portal, GitHub account, Bitbucket account); the caret lists the three directly.

**GitHub / Bitbucket pass** (the same `.pass`): stripe and tile in the account's identity colour, provider glyph on the tile's corner, login, host or workspace; fact "2 个仓库" (links to Projects filtered by this account); two key lines (`.keyline`):

- key: "SSH 密钥已验证 / SSH key verified" in ok + fingerprint; or warn "SSH 密钥还没验证 / SSH key not verified yet"; or danger "GitHub 不认识这把密钥了 / GitHub no longer recognises this key".
- token: "API 令牌有效，可以管理拉取请求 / API token valid, pull requests enabled"; or warn "还没有 API 令牌，拉取请求暂不可用 / No API token yet, pull requests are off" + "添加 / Add"; or warn "API 令牌已过期 / API token expired" + "更新 / Update".

Actions row: "验证 / Verify", "复制公钥 / Copy public key", More (Update token, Change colour, Suggested author identity, Show key file in Finder, Remove account). Remove confirms with the repositories that will lose their binding and offers "同时删除这台电脑上的密钥文件 / Also delete the key file on this computer" (off).

**Add account** (modal 720px, horizontal stepper, 4 steps; the mockup shows step 3 after a successful verify).

1. **账号 / Account**: provider (already chosen), "用户名 / Username" (GitHub login; Bitbucket username). A colour swatch row with the next free identity colour selected. (Bitbucket workspaces are confirmed in step 4, from the token.)
2. **SSH 密钥 / SSH key** (`.opt` x2): "新建一把密钥（推荐）/ Create a new key (recommended)" - "每个账号一把，互不混用 / One key per account, never shared"; or "用我已有的密钥 / Use a key I already have" (file picker; a key already used by another account in the app is refused with the reason: one key cannot serve two accounts on the same host).
3. **交给 GitHub / Give it to GitHub**: lead, the public key (`.pubkey`) with Copy, the private key's path, three numbered lines (`.howto`), the footer button "打开 GitHub 的 SSH 设置 / Open GitHub SSH settings", and the verify box (`.verify`): idle "添加好之后点验证 / Choose Verify when you have added it" + primary "验证 / Verify"; running spinner + "正在询问 GitHub... / Asking GitHub..."; ok "GitHub 认出了这把密钥 · 账号是 alex-dev"; failed in danger with the server's sentence and one hint ("公钥可能还没保存，或者加到了另一个账号 / The key may not be saved yet, or it was added to a different account"). If the server reports a different login than the one typed: warn "这把密钥属于 alex-work，不是 alex-dev / This key belongs to alex-work, not alex-dev" + "改用 alex-work / Use alex-work instead". Next is enabled only after a successful verify; "稍后验证 / Verify later" is a link that saves the account as unverified.
4. **API 令牌 / API token**: why ("用来在 Codepass 里查看、创建和合并拉取请求，以及从列表里克隆仓库 / Used to view, create and merge pull requests in Codepass, and to clone from a list"), the kind of token and exactly what to tick, as a short list.
   - **GitHub**: a **classic** personal access token ("Tokens (classic)") with two boxes ticked: `repo` and `read:org`. Nothing else is needed. One line under it: "细粒度令牌（fine-grained）在 1.0 里不支持 / Fine-grained tokens are not supported in 1.0." And a quiet note: "有的组织不允许经典令牌，或者要求先为它授权单点登录。那样的组织，拉取请求会在浏览器里打开。 / Some organisations do not allow classic tokens, or require single sign-on to be authorised for them first. For those organisations, pull requests open in the browser."
   - **Bitbucket**: an Atlassian API token with five boxes ticked: read:user, read:workspace, read:repository, read:pullrequest, write:pullrequest. A note: "Atlassian 的令牌都有到期日。到期后在这里换一个新的。 / Atlassian tokens always have an expiry date. Paste a new one here when it expires."
   - "打开创建令牌的页面 / Open the token page", a password-type field with paste, "检查 / Check". Check shows the login and compares what the token grants with what is needed: ok "令牌可用：alex-dev，权限齐全 / Token works: alex-dev, all permissions present"; warn "令牌缺少 read:org / The token is missing read:org" (one line per missing item); danger "令牌无效或已过期 / The token is invalid or has expired".
   - **Bitbucket workspaces** are read from the token after a successful check and shown as chips to confirm ("这个令牌能访问 2 个工作区 / This token can reach 2 workspaces"); they are not typed in step 1 any more. Without a token, the workspaces found in existing remotes are used.
   - macOS: "令牌保存在系统的钥匙串里，只有 Codepass 能读取，不会写进设置、备份或导出文件。 / The token is stored in the system keychain, readable only by Codepass. It is never written to settings, backups or exports." Windows: "令牌保存在 Windows 凭据管理器里。用你的账号运行的其他程序也能读到它。 / The token is stored in Windows Credential Manager. Other programs running under your account can read it too." Primary "完成 / Finish"; "先跳过 / Skip for now" keeps PR features off for this account.

After finishing: the pass appears with the insert motion; a toast offers "为 3 个仓库绑定 alex-dev / Bind alex-dev to 3 repositories" when unbound repositories on that host can be read with the new key.

**SSH details the UI must show.**

- *A key with a passphrase* (import): "这把密钥有密码。Codepass 不保存密码，需要你先把它加进系统的密钥代理。 / This key has a passphrase. Codepass does not store it; add the key to the system's key agent first." with the exact command and "重新检查 / Check again".
- *Host check*: the app ships the providers' published host keys and uses its own known-hosts file, so git started from the app never stops at a question nobody can answer. If a host presents a different key: danger "bitbucket.org 的身份和 Codepass 记录的不一样，已停止连接 / bitbucket.org identified itself differently from what Codepass has on record. The connection was stopped", with "了解原因 / Learn why".
- *Key no longer accepted* (revoked or removed at the provider): the bound repository row turns danger "alex-dev 的密钥被拒绝 / The key for alex-dev was refused" with "验证 / Verify". The app never falls back to another key on the machine for a bound repository.
- *Unbound GitHub / Bitbucket repository*: the dashed chip reads "未绑定 · 用这台电脑的默认密钥 / Not bound · uses this computer's default key" so it is clear which identity a push would use.
- Backups keep configuration only. **Private keys and tokens are never copied into a backup, an export or a log.** Removing an account offers to delete its key file; nothing else ever deletes or copies one.

**Accessibility.** The stepper is an ordered list with `aria-current="step"`. The public key is a read-only text field so it can be selected by keyboard. Verify results use `role="status"` (ok) or `role="alert"` (failed). The token field never echoes into logs or toasts.

### 5.12 Author identity (`identity`)

Shots: `identity-light-zh` (with the project sheet), `identity-page-light-zh`, `identity-dark-zh`.

**Git settings page.** Title "Git 设置", subtitle "提交时用谁的名字和邮箱，以及 Git 全局保护". Top card "全局作者身份 / Global author identity": name, email, Save (enabled on change), the line "没有自己设置的项目和仓库都用它。现在有 6 个仓库在用。" and on the right the **ladder** (`.ladder`): 仓库 › 项目 › 全局 with "越具体的越优先 / The most specific one wins". Below, the existing Git global guard cards, unchanged.

**Effective identity table** (`.ident-table`): project rows (layers icon, name, repository count) and their repositories indented, then Ungrouped. Columns: name; **生效的作者身份 / Effective author identity** (`Name <email>`, or "跟随全局 / Follows global" in muted text on a project without one); **来源 / Source** tag; action. Source tags: "项目设置 / Project setting", "来自项目 / From project", "来自全局 / From global", "仓库自己的 / Repository's own", and "仓库自己的 · 在应用外设置 / set outside the app" when the app did not write it.

**Editors.**

- *Project* (side sheet, drawn): two options, "跟随全局" (shows the global value) and "这个项目用自己的" (with the note that it is written into each repository's own config and works with Codepass closed), name and email fields, then **保存后会改动 3 个仓库 / Saving changes 3 repositories**: a list (`.affect`) with each repository and its old value struck through; repositories with their own setting are listed last on a sunken row with "保留自己的 / Keeps its own" and a link "改为跟随项目 / Make it follow the project". Footnote about the backup. Primary "保存并应用到 3 个仓库 / Save and apply to 3 repositories".
- *Repository* (popover from the pencil, or in the repository settings sheet): three options, "跟随项目 acme" (value shown), "跟随全局" (value shown), "这个仓库用自己的" with two fields. Saving writes or removes the repository-local values and updates the tag.
- *Suggestions*: when an account is added, a `.suggest` row appears here once: "要给绑定 alex-dev 的 2 个仓库使用 alex-dev 的 GitHub noreply 邮箱吗？ / Use alex-dev's GitHub noreply address for the 2 repositories bound to alex-dev?" with 应用 / 不用了.

**Where else it shows.** The commit composer (5.4), the project header on Projects, the task merge footer (5.5), and the session bar above an AI terminal (5.6).

**The effective identity is what git itself reports**, not what the app thinks it wrote. Extra source tags cover what the owner's machine may already do: "来自 ~/.gitconfig 的条件设置 / From a conditional rule in ~/.gitconfig" (an `includeIf` file) and "来自终端环境变量 / From a terminal environment variable" (warn; `GIT_AUTHOR_NAME` or `GIT_AUTHOR_EMAIL` exported in a shell profile). The second one wins inside terminals and AI tasks whatever the app sets, so the row says: "终端和 AI 任务里的提交会用这个，而不是上面的设置 / Commits made in terminals and AI tasks use this instead of the setting above", with "打开那个文件 / Open that file".

**States.** No global identity: the card is warn-bordered with "还没有设置。没有它就不能提交。 / Not set yet. You cannot commit without it." Invalid email: inline field error. A repository whose config cannot be written shows danger "写不进这个仓库的设置 / Cannot write this repository's config" with the path and "重试".

### 5.13 Updates (`update`)

Shots: `update-light-zh` (dialog), `update-banner-light-zh` (banner only).

- **Check**: at launch and once a day, against the latest release of `github.com/rudian/codepass`. Never a modal on launch. Behind a company proxy the system's proxy settings are used.
- **Banner** (`.update-banner`, quiet, on the Projects page under the header; dismissible for this version): ink tile with the up arrow, "Codepass 1.1.0 可以更新 · 你现在用的是 1.0.0", "更新内容 / What's new" (opens the dialog on the notes), primary small "更新 / Update". The sidebar's Settings item shows a dot (`.nav__dot`); in the workspace layout the rail's Settings button gets a pip.
- **Dialog** (modal 560px): versions side by side (`.ver`), release notes (plain text lines from the signed update file, shown in `.notes`, max 8 lines then scroll; never fetched separately and never rendered as HTML), then the progress block: label + bytes + time left, the bar, and four steps (`.usteps`): 下载 / 校验签名和版本 / 替换应用 / 重新打开 (Download / Verify signature and version / Replace app / Reopen).
- **AI tasks running**: a warn banner inside the dialog, "2 个 AI 任务还在运行。重新打开应用会结束它们。分支和改动都会保留，之后可以从原来的会话继续。" The footer then offers "现在就重新打开 / Reopen now" and primary "等没有任务在运行时再重新打开 / Reopen when no task is running". An interactive AI session never exits by itself, so the condition is "no task in the running state" (every task is waiting for you, done or stopped) for 60 seconds; the banner then counts down 10 seconds with "取消 / Cancel" before reopening. With no tasks the single primary is "重新打开 / Reopen".
- The app stays usable while downloading; closing the dialog keeps the download going and the banner shows "正在下载 64%".
- **States.** Up to date (Settings): "已是最新版本 1.0.0". Checking: spinner in the button. Check failed: "检查不了更新：连不上 GitHub / Cannot check for updates: GitHub is unreachable" + "重试". Download failed: the bar stops, danger text, "重试" and the link "改为在浏览器中下载 / Download in the browser instead". Verification failed (checksum, the update file's signature, a version that is not newer than the one installed, or the macOS code signature): a danger banner "下载的更新没有通过校验，已经丢弃。应用没有任何改动。 / The download did not pass verification and was discarded. The app was not changed." with the failed step named, "重试" and "在浏览器中下载". Cannot replace (the app runs from a read-only place or a standard user cannot write to Applications): "先把 Codepass 移到「应用程序」文件夹，再更新 / Move Codepass to the Applications folder, then update". After reopening: a toast "已更新到 1.1.0" with "更新内容". Old app still running (first launch after replacing AWS SSO Hub): a small window before anything else, "旧版 AWS SSO Hub 还在运行。请先退出它，再打开 Codepass。 / The old AWS SSO Hub is still running. Quit it first, then open Codepass." with "退出旧版并继续 / Quit the old app and continue"; afterwards one prompt offers "把旧版移到废纸篓 / Move the old app to the Trash".

### 5.14 Settings additions (`settings`)

Shots: `settings-light-zh`, `settings-dark-en`.

Two columns of cards (`.set-grid`); each row is `.set-row`: title, one line of help, the control on the right. Existing settings (scan folders, backups, sign-in in a private window, export / import) stay above these and are not redrawn.

| Card | Rows |
|---|---|
| **AI 编码工具 / AI coding tools** | One row per tool: mark, name, command and how it is kept current, version badge, "更新 / Update" or "安装 / Install". Then "新任务在哪里运行 / Where a new task runs" (独立工作区 / 主工作区), "同时运行的任务上限 / Tasks running at once" (select, default 6), and under a disclosure: custom command and extra arguments per tool, "任务等你时发系统通知 / Notify me when a task waits" (on), "会话结束后保留最后的终端内容 / Keep the last terminal screen after a session ends" (on; help: "重启后还能回看。它可能包含屏幕上出现过的任何内容，「释放空间」会删掉它。 / Lets you read it after a restart. It may hold anything that was on screen; Free up space deletes it."). Each tool row also shows its approval mode as read from the tool ("每次询问 / Ask every time"), read-only, with "在工具里修改 / Change it in the tool". There is no Codepass switch that skips a tool's permission questions. Last line of the card, muted, always shown (milestone 8; plan 4.16 decision 35): "Codepass 只用于你自己的账号和你自己的工作。 / Codepass is for your own accounts and your own work." followed by two links that open in the browser: "Anthropic 的条款 / Anthropic's terms" (`https://www.anthropic.com/legal/consumer-terms`) and "OpenAI 的条款 / OpenAI's terms" (`https://openai.com/policies/terms-of-use/`). Every terms link in the app opens the company's live page, never an archive copy; the full list of targets is in 5.16. Its tooltip: "不共享账号，不转售，不替别人跑任务。登录始终由工具自己完成。 / No shared accounts, no reselling, no work run for other people. Sign-in is always done by the tool itself." |
| **编辑器和代码智能 / Editor and code intelligence** | Go · gopls, TypeScript / JavaScript and PHP rows (PHP added 2026-11-16; its server is named after E5-11's research) with what they provide, version badge, Update / Reinstall in a menu; "同时运行的语言服务 / Language servers running at once" (2 / 4; help: "每个要用几百 MB 到 1 GB 多的内存 / Each one uses from a few hundred MB to over 1 GB of memory"); 字号 / Font size; Tab width; 自动换行 / Wrap long lines; "保存时格式化 / Format on save" (off); terminal theme (深色 / 跟随应用). |
| **登录提醒 / Sign-in reminders** (the existing card; rows added, the two existing rows unchanged) | Existing: "登录快过期时提醒 / Remind me before sign-in expires" (on) and "提前多久提醒 / How early" (15 分钟 / 30 分钟 / 1 小时). The first row's help now says what it does: "用系统通知提醒，通知上可以直接重新登录。 / Sends a system notification. You can sign in again from it." New row under them: "登录过期时通知我 / Notify me when a sign-in expires" (on), help "过期的那一刻通知一次，写明是哪个登录。 / One notification at the moment it expires, naming the sign-in." It has its own switch because the two are different wishes: some people want no early reminder but must know when work stops. Last line of the card, muted: "登录能保持多久由管理员决定，Codepass 不能延长，只能重新登录。 / Your administrator decides how long a sign-in lasts. Codepass cannot extend it; it can only sign in again." |
| **系统通知 / System notifications** (one status row, shown at the top of the right column only when there is something to say) | *macOS, turned off for Codepass:* warn dot + "macOS 没有允许 Codepass 发通知 / macOS does not allow notifications from Codepass", help "登录过期和 AI 任务等待的提醒都不会出现。 / Reminders about expired sign-ins and waiting AI tasks will not appear.", button "打开系统设置 / Open System Settings" (opens the Notifications pane; the row re-checks when the window gets focus again and disappears once allowed). *macOS, not asked yet:* neutral dot + "Codepass 还没有申请通知权限 / Codepass has not asked for permission to notify you yet", button "允许通知 / Allow notifications" (macOS then shows its own question). *Allowed:* no row. *Windows:* no state can be read, so one muted help line under the Sign-in reminders card instead: "没看到通知？请检查 Windows 设置 → 系统 → 通知 里的 Codepass。 / No notifications? Check Codepass under Windows Settings → System → Notifications." with the link "打开 Windows 设置 / Open Windows Settings". The switches above stay usable in every state; they are not greyed out. |
| **工单队列 / Ticket queue** (milestone 8; plan 4.17) | "给自己留多少用量 / Keep this much usage for myself" (0% / 10% / 20% / 30%, default 0%; help: "队列用到只剩这么多时就停下，留给你自己用。 / The queue stops when this much is left, so you can still work yourself."). One muted line with a link to the Accounts page, in place of a switch: "自动换账号默认关闭，在「账号」页按工具设置 / Automatic switching is off by default. It is set per tool on the Accounts page" (the setting applies to the owner's own sessions too, so it lives in 5.16). "允许队列使用用量额度 / Let the queue use usage credits" (off; help: "关着时，用量到头队列就停，不会按量计费。一轮很长的回答可能在停下前用掉一点额度。 / When off, the queue stops at the limit and nothing is billed by use. One very long turn may spend a little before it stops."). "每张票开始前先问我 / Ask me before each ticket" (off). "队列附言 / Queue note": a 3-row text field with the two marker lines and "恢复默认 / Restore default"; help: "加在每张票的第一条消息后面，让 AI 说明它是做完了还是在等你。 / Added after each ticket's first message so the AI says whether it is finished or waiting for you." The queue notifications (section 6) follow the existing switch "任务等你时发系统通知 / Notify me when a task waits" for "needs you", and one new switch "队列暂停、换账号或跑完时通知我 / Notify me when the queue pauses, switches account or finishes" (on) for the other three; the notice for an account that needs attention follows this switch too. |
| **保持唤醒 / Keep awake** | Header badge "现在生效 · 2 个任务 / Active now · 2 tasks" while held. "AI 任务运行时不让电脑睡眠" (on). "屏幕也保持常亮" (on): "关掉后屏幕可以熄灭并锁定，AI 照常工作。在办公室更安全。" "任务等你回答时，继续保持" 5 分钟 / 15 分钟 / 1 小时 / 一直. "最长保持 / Longest hold" 4 小时 / 8 小时 / 12 小时 (default 8): after that long without any answer from you or sign of progress, the hold is released even if a task still looks busy. Then the limits, always visible: closing a MacBook's lid still sleeps it unless it is on power with an external display; while the screen stays on, the computer does not lock by itself. On Windows the first limit reads "按下睡眠键或合上盖子仍然会睡眠 / Pressing sleep or closing the lid still sleeps it". |
| **更新 / Updates** | Current version, when it was last checked and from where, "现在检查 / Check now"; "每天自动检查一次" (on) with "只读取版本号，不上传任何信息。下载的更新会先校验签名。" |
| **外观 / Appearance** | Theme (跟随系统 / 浅色 / 深色), Language (中文 / English), **Motion** (跟随系统 / 完整 / 减少) with "「减少」会去掉滑动和淡入，只保留必要的进度提示。" This replaces the old "Reduce motion" switch. |

**What "smart" keep-awake means in the UI.** The indicator is on exactly while the assertion is held: at least one task is running, or has been waiting for less than the grace time. It turns off the moment the last such task ends. A plain shell never turns it on. The indicator never claims more than the OS does: it says "保持唤醒", not "不会睡眠".

### 5.15 Tickets (`tickets`)

Milestone 8; plan 4.17. Not drawn yet (section 8): the text sketches below are the specification until the mockup screens exist. Data is generic.

**Purpose in one line.** Write down work for later; let the queue run it one ticket at a time when there is usage for it.

**Page.**

```
工单 / Tickets                    3 张待执行 · 1 张在运行                 [新工单 ⇧⌘N]
+--------------------------------------------------------------------------------------------+
| 队列  ● 正在运行 #12「修复结算金额的四舍五入」 · acme_api · Claude Code · 工作      [暂停队列] |
|       下一张开始前会检查用量。本周 81%，给自己留 10%。                                        |
+--------------------------------------------------------------------------------------------+
  编辑中 2                      暂不执行 1                    待执行 3  (按顺序执行，可拖动)
 +--------------------------+  +--------------------------+  +------------------------------+
 | #15 导出对账单时带上税号  |  | #9 把日志换成结构化日志   |  | 1 #13 给商户列表加分页        |
 | acme_api · 未选工具       |  | globex_core · Codex       |  |   acme_merchant_portal_v4     |
 | 2 分钟前编辑              |  | 上周                      |  |   Claude Code · 独立工作区     |
 +--------------------------+  +--------------------------+  | 2 #14 升级 PHP 依赖            |
 | #16 （没有标题）          |                                |   acme_cs_portal · Codex       |
 | 未选仓库                  |                                | 3 #11 README 补上本地运行步骤   |
 +--------------------------+                                 +------------------------------+
  已结束 4  v   #10 已完成 · 查看改动 3    #8 失败 · 重试    #7 已跳过    #6 已完成 · 已合并
```

English labels: "Tickets", "3 queued · 1 running", "New ticket", columns "Editing", "On hold", "Queued (runs in this order; drag to reorder)", "Finished".

- **Three columns are the owner's three states**, in the owner's words: 编辑中 / Editing, 暂不执行 / On hold, 待执行 / Queued. A ticket moves between them by drag, by the row menu, or by the buttons in the editor. Keyboard: the row menu has "移到 编辑中 / 暂不执行 / 待执行 · Move to Editing / On hold / Queued" and "上移 / 下移 · Move up / Move down" inside Queued.
- **Card** (`.ticket`): number, first line of the text as the title (or "（没有标题）/ (no title)"), repository with its provider mark (both names when they differ) or warn "未选仓库 / No repository", tool mark and name or warn "未选工具 / No tool", last edited. Queued cards show their place in line.
- **Queueing needs a repository and a tool.** Dropping a ticket without them into Queued opens the editor with the two fields marked and one line: "选好仓库和工具才能排队 / Choose a repository and a tool to queue it."
- **The running ticket** is not in a column. It is the first line of the queue bar, and its card sits pinned at the top of Queued with a pulsing info dot and "运行中 / Running"; click opens the task's terminal.
- **Finished** (collapsed group under the columns): done, failed, skipped, newest first. Done rows carry the task's review action ("查看改动 3 / Review 3 files", or "已合并 / Merged"); failed rows carry "重试 / Retry" (puts a copy of the ticket at the front of the queue, on the same task workspace); a row menu has "再排一次 / Queue again" and "删除 / Delete". Deleting a ticket never deletes its task, branch or workspace; the confirm says so.
- **Empty page:** `.empty` with "把以后要做的事先记下来 / Write down work for later", text "排进队列的工单会一张一张地交给 AI 去做，每张开始前先看用量够不够。 / Queued tickets are handed to an AI one at a time. Before each one, Codepass checks that there is usage left.", primary "新工单 / New ticket".

**Queue bar** (`.queue-bar`, always on top of the page while anything is queued or running; `role="status"`). Line 1 is the state, line 2 is the reason or what happens next. One state at a time:

| State | Line 1 (zh / en) | Line 2 | Actions |
|---|---|---|---|
| Idle, nothing queued | 队列是空的 / The queue is empty | 把工单拖到「待执行」就会开始。 / Drag a ticket to Queued to start. | none |
| Checking | 正在检查用量… / Checking usage... | 「工作」· Claude Code | 暂停队列 / Pause queue |
| Running | 正在运行 #12「…」 / Running #12 "..." | 下一张开始前会检查用量。本周 81%，给自己留 10%。 / Usage is checked before the next one. Week 81%, keeping 10% for you. | 打开终端 / Open terminal · 暂停队列 / Pause queue |
| Needs you | #12 在等你 / #12 needs you | one of the reasons below | 去回答 / Answer · 算完成 / Mark done · 跳过 / Skip |
| Waiting for reset, the tool continues | #12 等待重置，周一 00:00 / #12 is waiting for the reset, Mon 00:00 | Claude Code 会在重置后自己继续。 / Claude Code continues by itself after the reset. | 在「个人」上继续 / Continue on Personal (when it has room: for a ticket's session that is below 100% minus the reserve, plan 4.16). It opens the switch confirm of 5.6 for that session. |
| Waiting for reset, the queue continues | the same line 1 | 到时 Codepass 会让它继续。 / Codepass will continue it then. | the same |
| Paused for the limit, nothing running | 队列因用量暂停，周一 00:00 再试 / Paused for usage. Trying again Mon 00:00 | 所有账号都没有余量了（给自己留 10%）。 / No account has room (keeping 10% for you). | 现在再查一次 / Check again now |
| Paused for this ticket's account, another account has room (automatic switching off, the default) | 队列因用量暂停，周一 00:00 再试 / Paused for usage. Trying again Mon 00:00 | 「工作」没有余量了。「个人」还有，要用它继续吗？ / Work has no room. Personal has. Continue on it? | 在「个人」上继续 / Continue on Personal · 现在再查一次 / Check again now |
| Paused because an account needs the owner | 队列停下了：「工作」需要你处理 / The queue stopped: Work needs your attention | Claude Code 报告这个账号有问题。队列不会换到别的账号。 / Claude Code reports a problem with this account. The queue will not move to another account. (the tool's name follows the account's tool: "Codex 报告… / Codex reports..." for a Codex account) | 查看账号 / Open the account |
| Usage unknown | 正在运行 #13「…」 | 没法读取用量，这张票直接开始了。 / Usage could not be read, so this ticket started without a check. | as Running |
| Paused by the owner | 队列已暂停 / The queue is paused | 当前这张做完后不会开始下一张。 / After the current ticket, no new one starts. | 继续队列 / Resume queue |
| Asking before each ticket | 下一张：#13「…」 / Next: #13 "..." | 你打开了「每张票开始前先问我」。 / "Ask me before each ticket" is on. | 开始 / Start · 跳过 / Skip |
| Usage credits would be used | 队列停下了：再跑就要按量计费 / The queue stopped: going on would be billed by use | 「工作」开着用量额度，而队列不允许使用它。 / Usage credits are on for Work, and the queue is not allowed to use them. | 设置 / Settings |

"在「个人」上继续 / Continue on Personal" is the owner's click, so it works with automatic switching off (plan 4.17). For a ticket that has not started, it starts that ticket on Personal, this once; for a running ticket at a limit, it opens the switch confirm (5.6) and the conversation comes along. Nothing is remembered for the next ticket. The button is never shown when the reason is an account that needs the owner.

**"Needs you" reasons** (line 2; the same words are the ticket's sub-line and the notification body):

| Reason | zh / en |
|---|---|
| The AI ended with the question marker | AI 有问题要问你 / The AI has a question for you |
| The AskUserQuestion tool | AI 让你选一个答案 / The AI asks you to choose an answer |
| A permission prompt | AI 在等你批准一个操作 / The AI is waiting for you to approve an action |
| Turn ended with no marker | AI 停下了，但没说做完没有 / The AI stopped without saying whether it is finished |
| A turn ended by an error | AI 那边出错了，这一轮没有做完 / The AI service returned an error and the turn did not finish |
| The tool exited | 会话结束了（退出码 1） / The session ended (exit code 1) |
| The tool reports a problem with the account (plan 4.16 decision 34) | 「工作」需要你处理 / Work needs your attention |
| OpenCode | OpenCode 不会告诉 Codepass 做完没有，请看一眼 / OpenCode does not tell Codepass whether it is finished. Take a look |

- "算完成 / Mark done" and "跳过 / Skip" confirm nothing: both only change the ticket and let the queue go on. They are undoable from a toast for 10 seconds ("已标记为完成 / Marked as done" + "撤销 / Undo"), because the next ticket starts after that delay.
- "去回答 / Answer" opens the task's terminal with focus in it.
- The queue bar's short form appears on the AI tasks page header and as the tooltip of the sidebar's Tickets count, so the reason is one hover away from anywhere.

**Editor** (`ticket-editor`, side sheet 640px from the right; the page stays visible behind it).

```
工单 #15                                                   编辑中 · 已保存
+--------------------------------------------------------------+
| 导出对账单时带上税号                                          |
|                                                              |
| - 导出的 CSV 增加一列 tax_id                                  |
| - 旧数据没有税号的留空                                        |
+--------------------------------------------------------------+
  预览 / Preview
  仓库 / Repository   [aws] acme_api                    v
  工具 / Tool         ( Claude Code ) ( Codex ) ( OpenCode )
  账号 / Account      跟随仓库（现在是「工作」）/ Follow the repository (now Work)   v
  在哪里运行 / Where   (•) 独立工作区 / Own workspace   ( ) 主工作区 / Main workspace
  v 队列附言 / Queue note
    When everything in this ticket is finished, end your last message with the single line CODEPASS-DONE. ...
                          [删除 / Delete]   [暂不执行 / Put on hold]   [排入队列 / Queue it]
```

- The text is Markdown, edited as plain text in the mono font; "预览 / Preview" shows it with the app's safe renderer (raw HTML off, no remote images).
- **Autosave** about a second after the last keystroke; the header shows "正在保存… / Saving..." then "已保存 / Saved". There is no Save button. Closing the sheet (Esc, or a click outside) never loses text.
- **Account** (plan 4.16 decision 18): shown only when the tool has more than one AI account. Options: "跟随仓库（现在是「工作」）/ Follow the repository (now Work)", the default, then each account of the tool with its latest reading. "Follow the repository" uses the repository's default for that tool (repository › project › app; 4.2) as it is when the ticket starts, so a default changed later still applies to a ticket already in the queue. With no repository chosen yet it reads "跟随仓库 / Follow the repository" with no value. Changing the tool sets the account back to "Follow the repository". An account that no longer exists, or a ticket imported on another computer, also reads "Follow the repository".
- Help line under the row: "这个账号用完时，队列会停下来等；另一个账号有用量时，它会问你要不要换。 / When this account is used up, the queue stops and waits. If another account has usage, it asks whether to continue there." With "一个账号用完后自动换下一个 / Switch to the next account when one runs out" turned on for that tool (5.16; off by default), the line reads instead: "这个账号用完时，队列会自动换到有用量的账号。 / When this account is used up, the queue switches to an account with usage." The queue never treats a fixed account as "never switch", and never switches by itself while that setting is off.
- Queued cards and the queue bar name the account the ticket will start on ("Claude Code · 工作"); a fixed account that differs from the repository's default has a small pin mark with the tooltip "这张票固定用「个人」 / This ticket always starts on Personal". When the shell decides the account, the row is disabled with the sentence of 5.16.
- "队列附言 / Queue note" is collapsed; opened, it shows the text that will be added and the link "在设置里修改 / Change it in Settings".
- OpenCode chosen: a muted line "OpenCode 的票不检查用量，每一轮结束都会等你看一眼。 / OpenCode tickets run without a usage check, and wait for you after every turn."
- A ticket that is running or finished opens read-only with its state, the reason, and "打开任务 / Open task".
- Footer buttons follow the state: Editing shows all three; On hold shows "继续编辑 / Edit" and "排入队列 / Queue it"; Queued shows "移出队列 / Take out of the queue".

**Quick capture** (`ticket-capture`, Cmd/Ctrl+Shift+N from any screen and both layouts).

```
+----------------------------- 新工单 / New ticket -----------------------------+
| 记下要做的事…  / Write what should be done...                                 |
|                                                                               |
+-------------------------------------------------------------------------------+
  [aws] acme_api v     Claude Code v                 存为编辑中 ⏎      排入队列 ⌘⏎
```

- A small centred sheet (560px), focus in the text. It opens in under 100 ms and gives focus back to where it was, also to a terminal.
- The repository and tool are pre-filled from the screen it was opened on (the open workspace's repository; its default tool); they can be cleared. Quick capture has no account field: the ticket follows the repository until the editor says otherwise.
- Enter adds a new line. The two actions: "存为编辑中 / Save as editing" (Esc does the same when there is text) and "排入队列 / Queue it" (Cmd/Ctrl+Enter; disabled with a tooltip until a repository and a tool are set).
- It saves while typing, so a crash loses nothing. Toast after closing: "已存为工单 #17 / Saved as ticket #17" + "打开 / Open".

**Where tickets show elsewhere.** A task that came from a ticket has "工单 #12 / Ticket #12" on its session bar (5.6) and in its row on the AI tasks page. The AI tasks page header carries the queue bar's short form. Session switcher rows (5.7a) show a small "队列 / queue" tag for the queue's session.

**Motion.** Cards use the existing insert, remove and reorder motions (section 3). The queue bar's text changes without motion. Reduced motion: no drag ghost animation; the card moves at drop.

**Accessibility.** Columns are lists with headings and counts. Drag has full keyboard and menu equivalents. The queue bar is a polite live region; "needs you" is announced once. State is always a word, never colour alone. Every action button names the ticket in its label.

### 5.16 AI accounts and usage (`ai-accounts`)

Milestone 8; plan 4.16. Not drawn yet (section 8). Claude Code and Codex; OpenCode has no accounts here in 1.0.

**AI accounts group on the Accounts page** (after Bitbucket; section label with the two tool marks: "AI 账号 / AI accounts"). One sub-group per tool; rows are the same `.pass` component, in the owner's order (drag to reorder). The order means two things, and one muted line under the tool's name says both: "排第一的是应用默认账号；一个账号用完时，按这个顺序找下一个有用量的。拖动可以调整。 / The first account is the app default. When one runs out, the next one with usage is looked for in this order. Drag to reorder."

Under that line, one switch per tool, shown once the tool has two accounts and **off by default** (plan 4.16 decision 33): "一个账号用完后自动换下一个 / Switch to the next account when one runs out", with the help line "关着时，用完会停下来问你，点一下就能在另一个账号上继续，对话会带过去。 / When off, a session stops at the limit and asks you. One click continues it on another account, with its conversation." Turned on, the help line reads "对你自己开的会话和队列都有效。换账号后的第一轮会把整段对话重新发送一遍。 / Applies to your own sessions and to the queue. The first turn after a switch sends the whole conversation again." It is set per tool and stored with the accounts (plan 3.1, 4.16), not in the queue's settings. Adding a second account never turns it on.

**Turning it on asks once** (per tool; `alertdialog`, 560px, focus on Cancel). The quoted sentences are shown as published, in English, in both languages, each one whole (a cut would be marked with "…"); the lines around them are ours. For Claude Code:

```
打开「用完后自动换账号」？                                Turn on automatic switching?

打开后，Claude Code 的一个账号用完时，Codepass 不等你点，    When one Claude Code account runs out, Codepass continues the
直接在下一个有用量的账号上继续这个会话。                    session on the next account with usage, without waiting for you.

服务条款没有写明这样做可不可以。和它有关的原话：            The terms do not say whether this is allowed. The sentences
                                                          that bear on it:
  “Advertised usage limits for Pro, Max, and Ultra plans assume ordinary, individual usage”
  “Except when you are accessing our Services via an Anthropic API Key or where we otherwise explicitly permit it,
   to access the Services through automated or non-human means, whether through a bot, script, or otherwise.”
                                                          (列在不允许的行为里 / listed among what is not allowed)
  阅读全文 / Read them in full:  Anthropic 消费者条款 / Consumer Terms · 使用政策 / Usage Policy · Claude Code 法律说明 / legal page

Codepass 不能保证账号不会被限制或停用。几个订阅怎么用，     Codepass cannot guarantee that an account will not be limited
由你决定。                                                 or suspended. How you use several subscriptions is your decision.

                                   [取消 / Cancel]   [打开自动换账号 / Turn it on]
```

- For Codex the two quoted lines are replaced by one: “Interfere with or disrupt our Services, including circumvent any rate limits or restrictions or bypass any protective measures or safety mitigations” with "(列在不允许的行为里 / listed among what is not allowed)", and the links are "OpenAI 使用条款 / Terms of Use · 使用政策 / Usage Policies".
- No warn or danger colour, no icon, no checkbox to tick: two paragraphs, the quotes in a quiet block, one sentence, two buttons. The dialog is shown once per tool; turning the switch off and on again later does not show it again, and the same text stays reachable from a small "条款怎么说 / What the terms say" link beside the switch.
- The second Anthropic quote is shown with its opening clause. That exception is part of the sentence, and it is what allows the unattended features Anthropic ships itself (research R9).
- The quotes and the link targets are kept in one place in the code with the date they were read, and the release checklist re-reads them (plan section 10). **Link targets**, always the live pages, never `web.archive.org`:

  | Link | Target |
  |---|---|
  | Anthropic 消费者条款 / Consumer Terms; "Anthropic 的条款 / Anthropic's terms" | `https://www.anthropic.com/legal/consumer-terms` |
  | Anthropic 使用政策 / Usage Policy | `https://www.anthropic.com/legal/aup` |
  | Claude Code 法律说明 / legal page | `https://code.claude.com/docs/en/legal-and-compliance` |
  | OpenAI 使用条款 / Terms of Use; "OpenAI 的条款 / OpenAI's terms" | `https://openai.com/policies/terms-of-use/` |
  | OpenAI 使用政策 / Usage Policies | `https://openai.com/policies/usage-policies/` |

The sketch shows Claude Code with two accounts and Codex in its one-account state (before "个人 / Personal" is added for Codex).

```
AI 账号 / AI accounts                                          Codepass 不保存任何 AI 登录信息
 Claude Code
 +----------------------------------------------+  +----------------------------------------------+
 | [cc] 工作 / Work     应用默认 / App default   |  | [cc] 个人 / Personal                          |
 | 已登录 / Signed in · 工具自带 / Tool's own     |  | 已登录 / Signed in · ~/.codepass/ai-accounts/… |
 | 本次 62%  ▓▓▓▓▓▓░░░░  15:45 重置              |  | 本次 0%   ░░░░░░░░░░                          |
 | 本周 81%  ▓▓▓▓▓▓▓▓░░  周一 00:00 重置         |  | 本周 12%  ▓░░░░░░░░░  周四 09:00 重置         |
 | Opus 本周 40%                                 |  | 3 分钟前读取                                   |
 | 默认用于：应用默认                             |  | 默认用于：项目 globex · acme_agent_portal      |
 | [刷新] [登录状态] [···]                        |  | [刷新] [登录状态] [···]                        |
 +----------------------------------------------+  +----------------------------------------------+
 Codex
 +----------------------------------------------+
 | [cx] 工作 / Work                              |     [ + 添加 Codex 账号 / Add a Codex account ]
 | 5 小时 18% · 16:10 重置   本周 40% · 周三重置  |
 +----------------------------------------------+
```

- **Pass content.** Tool mark, the owner's label, and the badge "应用默认 / App default" on the first account in the order (it moves when the order changes). Line 2: sign-in state ("已登录 / Signed in", warn "未登录 / Not signed in" + "登录 / Sign in", danger "需要你处理 / Needs your attention" (below), muted "正在检查… / Checking...") and where it lives ("工具自带 / Tool's own" for the account in the tool's own folder, with the tooltip "用工具自己的文件夹。Codepass 不改动里面的文件，只在「释放空间」里删除你选中的已结束会话的记录。 / It uses the tool's own folder. Codepass never changes a file in it, and deletes only the records of ended sessions you choose in Free up space."; or the extra folder's path, which can be copied). The earlier "默认 / Default" badge on the tool's own folder is gone: in the UI, "默认 / default" now only means the account a new task starts with. Then one bar per usage window with the percentage as a number, the window's name and the reset time. Then the reading's age.
- **"默认用于 / Default for"** (plan 4.16 decision 14). One line on the pass after the reading's age, listing where this account is the default: "应用默认 / App default", then projects ("项目 acme / Project acme"), then repositories by local folder name. Two fit on the line; more are counted, "默认用于：项目 globex · acme_agent_portal · 另外 3 个 / Default for: project globex · acme_agent_portal · 3 more". The line names only the places where this account is set: a project or repository that follows the level above is not listed, so Work's line in the sketch reads "默认用于：应用默认 / Default for: App default". Hidden repositories are left out of the line. Not the default anywhere: "没有项目或仓库把它设为默认 / No project or repository uses it by default". The line is a button; it opens the account's details:

  ```
  「个人」默认用于 / Personal is the default for
    项目 / Projects
      globex                       3 个仓库跟随 / 3 repositories follow          改 / Change
    仓库 / Repositories
      acme_agent_portal · acme     仓库自己的设置 / The repository's own setting   改 / Change
    正在使用它的会话 / Sessions using it now
      修复结算金额的四舍五入 · acme_agent_portal · 运行中                        打开 / Open
  ```

  "改 / Change" opens the project's AI accounts sheet (5.2) or the Repository settings sheet (4.2). The list is per tool: this is a Claude Code account, so only Claude Code defaults appear.
- **"需要你处理 / Needs your attention"** (plan 4.16 decision 34). The state of an account for which the tool reported an authentication, billing, suspension, disabled-account or policy error. It is not the state of a used-up account, and not of one the owner has simply not signed in to.

  ```
  +----------------------------------------------+
  | [cc] 工作 / Work     应用默认 / App default   |
  | ! 需要你处理 / Needs your attention · 14:02   |
  | Claude Code 报告这个账号的登录或账号本身有问题。|
  | > (the tool's own message, as plain text)     |
  | 在你处理之前，Codepass 不用这个账号，也不会把   |
  | 它的会话换到别的账号。                         |
  | [重新检查] [重新登录] [为什么]                  |
  +----------------------------------------------+
  ```

  - Text: "Claude Code 报告这个账号的登录或账号本身有问题。 / Claude Code reports a problem with this account's sign-in or the account itself." (the tool's name follows the account's tool) Then the tool's own message, unchanged, as plain text (shown from memory; after a restart the line reads "重新检查可以看到工具现在怎么说 / Check again to see what the tool says now"). Then: "在你处理之前，Codepass 不用这个账号，也不会把它的会话换到别的账号。 / Until you deal with it, Codepass does not use this account and does not move its sessions to another account."
  - The usage bars are hidden while the state is set.
  - "重新检查 / Check again" asks the tool for its status, which says only signed in or not (plan 4.16 decision 34). For the sign-in kind that decides: signed in, the state goes; not signed in, the pass becomes a normal "未登录 / Not signed in" pass. For the kinds billing, refused and unknown the status cannot tell, so Check again shows what the tool says now, and the state goes only when the owner presses "我已处理好 / I have dealt with it" (a quiet button that appears after the check, for these kinds only). Either way the toast is "「工作」的标记已清除 / The mark on Work is cleared": Codepass does not claim the account works. The next account error marks it again.
  - "重新登录 / Sign in again" opens the tool's own login in a terminal sheet, as for any account.
  - "为什么 / Why" opens a small popover, plain and without alarm: "如果是登录过期或付款的问题，处理好再点「重新检查」。如果是服务方停用了这个账号，请按它自己的流程申诉。它的条款不允许换一个账号绕过停用，所以 Codepass 不会替你换。 / If the sign-in expired or a payment failed, fix it and press Check again. If the service suspended this account, appeal through its own process. Its terms do not allow going around a suspension with another account, so Codepass does not switch for you." with the link to Anthropic's Usage Policy. That last sentence is for a Claude Code account. For a Codex account it reads: "OpenAI 的条款没有专门写这一条，但写了绕过规则和保护措施可能失去使用权，所以 Codepass 同样不会替你换。 / OpenAI's terms have no sentence on this, but say that circumventing their rules and safeguards may cost access, so Codepass does not switch for you either." with the link to OpenAI's Usage Policies.
  - Everywhere else the account is disabled with the same two words: the switch menu and the New task row (5.6), the ticket editor's account field, the queue bar (5.15), the usage chip and popover (4.1). A screen reader gets "工作，需要你处理 / Work, needs your attention".
- **Window names.** Claude Code: "本次 / Session", "本周 / Week", "Opus 本周 / Week, Opus" (the model's name as the tool gives it). Codex: "5 小时 / 5 hours", "本周 / Week" (named from the window's length; an unexpected length is shown as "N 小时 / N hours").
- **Reset times** are local, "15:45 重置 / resets 15:45" today, "周一 00:00 重置 / resets Mon 00:00" otherwise.
- **Usage credits**: a warn chip on the pass, "已开启用量额度 / Usage credits are on", tooltip "这个账号用量到头后不会停，会按量计费。队列默认不使用它。 / This account does not stop at its limit; it is billed by use. The queue does not use credits unless you allow it."
- **Unknown usage**: the bars are replaced by one muted line. Claude Code: "用量未知，运行过一次会话后才知道 / Usage unknown until a session has run". Codex: "用量未知 / Usage unknown". When the reason is known, it follows: "终端环境里设置了 ANTHROPIC_API_KEY，工具会改用它 / ANTHROPIC_API_KEY is set in your shell, so the tool uses it instead".
- **Actions.** "刷新 / Refresh" (disabled with a countdown tooltip inside the 5-minute spacing or the tool's own "rate limited until"); "登录状态 / Sign-in status" (asks the tool and shows its answer as text in a small popover); More: Rename, Change colour, 设为应用默认 / Make it the app default (moves it to the front of the order; hidden on the first one), Sign in again (opens the tool's login in a terminal sheet), Shared parts..., Show folder, Remove account. The tool's own account has no Remove and no Shared parts.
- **Remove** confirms with exact paths: "移除「个人」？会先让 Claude Code 在这个文件夹里退出登录，然后删除下面的文件夹。共享的对话记录不受影响。 / Remove Personal? Claude Code signs out in this folder first, then the folder below is deleted. Shared conversations are not affected." + the folder path + the list of links that will be removed. The sentence "共享的对话记录不受影响 / Shared conversations are not affected" is shown only when the conversation folders are linked. When they are not (plan 4.16 decisions 2 and 19), the preview lists instead the tasks whose latest conversation is only in this folder, under "这些任务最新的对话只在这个文件夹里。会先把对话复制到「工作」。 / The latest conversation of these tasks is only in this folder. Their conversations are copied to Work first.", and the folder is deleted only after every copy succeeded. When the account is a default somewhere (plan 4.16 decision 19), the preview adds what falls back, before the paths:

  ```
  这些地方现在默认用「个人」，移除后会改用上一级的默认：
  These use Personal by default. After removal they use the default of the level above:
    项目 globex                    →  应用默认「工作」 / app default, Work
    仓库 acme_agent_portal · acme  →  项目 acme「工作」 / project acme, Work
  2 张工单固定用「个人」，会改为跟随仓库。 / 2 tickets are fixed to Personal. They will follow the repository.
  另有 1 个已隐藏的仓库也默认用「个人」 / 1 hidden repository also uses Personal by default
  ```

  A hidden repository keeps its setting. The project sheet's lists (5.2), the move confirm and the "默认用于 / Default for" line leave hidden repositories out; the remove preview counts them in the one line above, shown only when there is at least one. A stopped or ended task that ran under the removed account shows "已移除的账号 / Removed account" afterwards (5.6, 5.7).

  Removing the first account in the order makes the next one the app default; the preview says "应用默认会变成「工作」 / The app default becomes Work". While a session runs under the account, Remove is blocked: "还有 1 个会话在用「个人」。先停掉它，或者把它换到别的账号。 / 1 session is still using Personal. Stop it, or switch it to another account first." with "打开 / Open" per session.
- **The shell decides** (plan 4.16 probe): a warn row above a tool's accounts, "你的终端环境设置了 CLAUDE_CONFIG_DIR，Claude Code 用哪个账号由它决定，Codepass 选不了。 / Your shell sets CLAUDE_CONFIG_DIR, so it decides which account Claude Code uses. Codepass cannot choose." The passes stay visible and read-only. The defaults of that tool stay visible everywhere ("默认用于 / Default for", 4.2, 5.2) but are not applied; the account row of the New task sheet, the "换账号 / Switch account" menu and the ticket editor's account field are disabled with this same sentence (5.6, 5.15). **Windows** has no login shell, so the probe reads the app's own environment (user and system variables), and the row reads: "你的 Windows 环境变量设置了 CLAUDE_CONFIG_DIR，Claude Code 用哪个账号由它决定，Codepass 选不了。 / A Windows environment variable sets CLAUDE_CONFIG_DIR, so it decides which account Claude Code uses. Codepass cannot choose."
- **One account only** (the usual start): the group shows the tool's own pass, with no "应用默认 / App default" badge and no "默认用于 / Default for" line, and a quiet button "添加账号 / Add account". No empty state.

**Add an AI account** (`add-ai-account`, modal 640px, three steps; the same stepper as 5.11).

1. **名字 / Name.** Tool (already chosen), "给它起个名字 / Give it a name" (for example "个人 / Personal"), a colour. Under it, muted, the folder that will be created: `~/.codepass/ai-accounts/claude/personal/`. Then two lines in body text, no box and no warning colour (plan 4.16 decisions 29, 30 and 36):
   - "只添加你自己的账号：你自己付费、自己登录的。如果它是团队或企业席位，哪些工作可以用它，由你的团队决定。 / Add only an account of your own: one you pay for and sign in to yourself. If it is a Team or Enterprise seat, your team decides which work may run on it."
   - "Codepass 不能保证账号不会被限制或停用。几个订阅怎么用，由你决定。 / Codepass cannot guarantee that an account will not be limited or suspended. How you use several subscriptions is your decision." with the links "Anthropic 的条款 / Anthropic's terms" or "OpenAI 的条款 / OpenAI's terms" for the chosen tool. This sentence appears here and in the automatic-switching confirm, nowhere else.
2. **共享 / Share.** The consent step. Text: "新账号有自己的登录。下面这些可以和工具自带的账号共用，这样换账号后还能接着原来的对话，工具的行为也一样。Codepass 会在新文件夹里建立指向它们的链接。 / The new account has its own sign-in. The items below can be shared with the tool's own account, so a conversation can continue after a switch and the tool behaves the same. Codepass creates links to them inside the new folder." Under the list, the consent sentence for conversation copies (plan 4.16 decision 27), as its own short paragraph: "换账号时如果原样接不上，Codepass 会在共享的对话文件夹里新建一份这段对话的副本，用新的会话编号。它只新建文件，从不修改工具已有的文件；对话记录只有你在「释放空间」里选中时才会删除。这些副本也在那里看到和删除。 / If a session cannot continue as it is after a switch, Codepass adds a copy of that conversation, under a new session ID, to the shared conversations folder. It only adds files. It never changes a file the tool already has; conversation records are deleted only when you choose them in Free up space. You can see and delete these copies there too." When the conversation folders are not linked (the fallback of plan 4.16 decision 2), one sentence is added after the first: "对话文件夹没有共享时，换账号会把这段对话复制一份到另一个账号的文件夹里。 / When the conversation folders are not shared, a switch copies that conversation into the other account's folder." "下一步 / Next" is the consent to the links and to both kinds of copy.
   - Ticked and locked, each with its reason: "对话记录、文件历史、计划和待办（换账号后接着对话需要它们）/ Conversations, file history, plans and to-do lists (needed to continue a conversation after a switch)" (Codex: "对话记录 / Conversations") and "设置（换账号后，AI 能做什么保持不变）/ Settings (so a switch never changes what the AI may do)". This holds for both tools: Claude Code's `settings.json` and Codex's `config.toml` are always linked (plan 4.16).
   - Ticked, can be unticked: "说明文件 / Instructions file", "代理、技能、命令 / Agents, skills, commands", "插件 / Plugins". For Codex: the instructions file and the other shared parts the tool has.
   - Always separate, stated: "不共享：登录信息和账号文件。所以你自己加的 MCP 服务和「信任这个文件夹」要在新账号里再设一次。 / Not shared: the sign-in and the account file. So MCP servers you added yourself, and \"trust this folder\", are set up once more for the new account."
   - One line: "Codepass 不会读取、保存或发送任何 AI 登录信息。 / Codepass never reads, stores or sends any AI sign-in."
3. **登录 / Sign in.** A terminal inside the modal runs the tool's own login for the new folder. Lead: "下面是 Claude Code 自己的登录。按它的提示在浏览器里完成。 / This is Claude Code's own sign-in. Follow it in your browser." Under the terminal the result: ok "已登录 / Signed in" (the app asked the tool), or "还没有登录 / Not signed in yet" + "重新检查 / Check again". Primary "完成 / Finish" is enabled after a successful check; "稍后登录 / Sign in later" saves the account as not signed in.

After finishing: the pass appears with the insert motion, last in the order, so no default changes; a first usage reading starts. Automatic switching stays off. When it is the tool's second account, one `.suggest` row appears once under the group: "现在可以给项目或仓库各选一个默认账号了，不同的任务也可以同时用不同的账号。一个账号用完时，你点一下就能在另一个上继续。 / You can now choose a default account per project or repository, and different tasks can run on different accounts at the same time. When one runs out, one click continues the session on another." with "去设置 / Set them" (opens Projects) and "不用了 / No thanks".

**Several accounts at the same time** (plan 4.16 decision 29). A task on Work and a task on Personal running side by side is a normal state and needs no control: both passes show "正在使用 / in use" in the popover below, and the account details list "正在使用它的会话 / Sessions using it now" per account.

**Usage popover** (`usage-popover`, from the sidebar chip or the rail's usage button; 380px).

```
AI 用量 / AI usage                                    3 分钟前 · [刷新 / Refresh]
 Claude Code
  ● 工作 / Work   (正在使用 / in use)
     本次 / Session      62%  ▓▓▓▓▓▓░░░░   15:45 重置
     本周 / Week         81%  ▓▓▓▓▓▓▓▓░░   周一 00:00 重置
     Opus 本周           40%  ▓▓▓▓░░░░░░   周一 00:00 重置
  ○ 个人 / Personal
     本次 0% · 本周 12%                     周四 09:00 重置
 Codex
  ● 工作 / Work   (正在使用 / in use)
     5 小时 / 5 hours    18%  ▓▓░░░░░░░░   16:10 重置
     本周 / Week         40%  ▓▓▓▓░░░░░░   周三 08:00 重置
  ○ 个人 / Personal
     5 小时 0% · 本周 6%                    周五 10:00 重置
 ---------------------------------------------------------------------
 队列给自己留 10% · 管理账号 / The queue keeps 10% for you · Manage accounts
```

- Every account of both tools. "正在使用 / in use" on every account with a live session; those come first and are expanded. The others are one line each, expandable. With no live session, the account the last session or the queue used is the one expanded, without the mark.
- "这些数字来自工具自己的报告，也包含你在它其他应用里的用量。 / These numbers come from the tool's own report and include your use in its other apps." as a muted footnote on first open.
- A reading older than 15 minutes shows its age in warn. Refresh follows the same spacing as on the pass.
- With the limit reached on the account in use, the top of the popover repeats the limit banner's line 1 and its actions (5.6).
- An account that needs the owner's attention is listed first, expanded, with "需要你处理 / Needs your attention" in danger in place of its bars and the link "查看账号 / Open the account".
- Workspace layout: the rail gets a usage button (a small gauge) with a pip at 80% and above; the status bar shows the short form "CC 81%" as a button that opens the same popover.

**States.** No AI tool installed: the group and the chip are hidden. A tool installed but never signed in: the tool's own pass with "未登录 / Not signed in" and "登录 / Sign in" (opens the tool's login in a terminal sheet). Reading in progress: the bars keep their last values, dimmed, with a small spinner by the age.

**Accessibility.** Each bar is a `progressbar` with a text value ("本周用量 81%，周一 00:00 重置 / Weekly usage 81%, resets Monday 00:00"). The popover is a dialog with a heading; accounts are a list. Warn and danger are also words ("快用完了 / almost used up", "已用完 / used up", "需要你处理 / needs your attention") for a screen reader. The automatic-switching confirm is an `alertdialog` whose description includes the quoted sentences; the quotes carry `lang="en"` in the Chinese interface.

---

## 6. Global patterns

- **Toasts**: unchanged rules. New ones follow the same shape: what happened + one action. Examples: "已把 3 个文件暂存 / Staged 3 files"; "已合并 ai/fix-settlement-rounding 到 main" + "撤销合并 / Undo merge" (until pushed); "推送被拒绝：远程有新的提交 / Push rejected: the remote has new commits" + "先拉取 / Pull first" (danger, stays).
- **Auth failures inline**: a push that fails because the AWS session ended shows the existing sign-in button in the toast and on the sync button; an SSH failure names the account and offers "验证密钥 / Verify key".
- **Confirms** keep the rule: primary repeats the verb, focus on Cancel, the sentence says exactly what is lost. Force push is only ever "带保护的强制推送 / force push with lease" and its confirm names the branch, the remote and the commits that will be replaced.
- **Backups**: every write to a config file (repository config for bindings, identity, remotes; global git config; the app's known-hosts file) keeps the existing backup-first behaviour, and toasts offer Undo where one exists. Backups never contain private keys or tokens; creating a key is not a backed-up action.
- **Text written by other people** (pull request titles, descriptions and timelines, release notes, hover documentation, commit messages, branch and file names) is shown as plain text, or as Markdown with raw HTML turned off. No remote images load anywhere in the app; avatars are monogram tiles. Links open in the browser only after the https check. This matters more than before: the window can now type into a terminal, so nothing from outside may ever become markup.
- **What the app runs by itself.** Background refreshes only read; they never run a repository's own hooks or helper programs. Those run only for an action the user started (commit, push, pull, fetch by button).
- **Proxy.** Network actions use the system proxy. Errors behind a proxy name it.
- **Empty, loading, error**: `.empty` for empty, skeletons for loads over 150ms, inline danger text + "重试" for errors; a full-pane error only when the pane has nothing else to show.
- **Windows**: system title bar; the workspace header drops the 84px reserve; shortcuts use Ctrl; the status bar and rail are identical; terminal default shell is PowerShell; "Reveal in Finder" reads "在资源管理器中显示 / Show in Explorer"; the keep-awake limits text changes as noted.
- **Minimum window**: 980x660 still holds for the management layout. The workspace needs 1100x700; below that the left panes collapse to 44px icon strips and the second editor group closes.
- **Dark theme checks**: diff tints use the existing `--diff-*` tokens; conflict tints are 11-13% mixes and keep code at 4.5:1; the terminal is darker than the dark surface, with a hairline `--term-line` so it never merges with the page.
- **System notifications**: every notification the app sends is in the table below (plan 4.15). The banner is drawn by the operating system, so only the words are ours.

  | Notification | When | Title | Body | Button | Click on the notification |
  |---|---|---|---|---|---|
  | Sign-in heads-up | The time left drops below "How early". Once per sign-in. No sound. | 登录快过期了：acme / Sign-in expires soon: acme | alex · acme-prod 等 3 个账户。大约 15 分钟后过期。 / alex · acme-prod and 2 more. Expires in about 15 minutes. | 重新登录 / Sign in again | Shows the window, opens Accounts at that portal. |
  | Sign-in ended | The sign-in has expired. Once per sign-in. On macOS it replaces the heads-up. Default sound. | 登录已过期：acme / Sign-in expired: acme | alex · acme-prod 等 3 个账户。重新登录后才能继续用。 / alex · acme-prod and 2 more. Sign in again to keep using them. | 重新登录 / Sign in again | Shows the window, opens Accounts at that portal. |
  | AI task waiting | A task changes to "waiting" while the window is not focused (5.6). Default sound. | acme_api | Claude Code 在等你回答 / Claude Code is waiting for your answer | none | Shows the window, opens that task's terminal. |
  | Still running | The first time the window is closed while tasks run (4.4). Once ever. No sound. | Codepass 还在运行 / Codepass is still running | 2 个 AI 任务没有中断 / 2 AI tasks were not interrupted | none | Shows the window. |
  | Queue: needs you | A queued ticket's session needs the owner (5.15), window focused or not, once per ticket and reason. Default sound. | 工单 #12 在等你 / Ticket #12 needs you | acme_api · AI 有问题要问你 / acme_api · The AI has a question for you (the reason line from 5.15) | 去回答 / Answer | Shows the window, opens that task's terminal. The button does the same. |
  | Queue: paused for the limit | No account has room, or the running ticket waits for a reset, or this ticket's account is used up while automatic switching is off. Once per pause. No sound. | 队列因用量暂停 / Queue paused for usage | Claude Code · 周一 00:00 重置后继续 / Claude Code · continues after the reset, Mon 00:00. When another account has room and automatic switching is off: 「工作」用完了，「个人」还有用量 / Work is used up. Personal has usage. | none; **"在「个人」上继续 / Continue on Personal"** when another account has room and automatic switching is off | Shows the window, opens Tickets. The button shows the window and opens the switch confirm for that ticket's session (5.6); for a ticket that has not started, it starts it on Personal. |
  | Queue: continued on another account | An automatic switch happened for a queued ticket (only with automatic switching on for that tool). No sound. | 已换到「个人」继续 / Continued on Personal | 工单 #12 · 「工作」的用量用完了，对话带过去了 / Ticket #12 · Work ran out of usage. The conversation came along (with a handoff note: 对话以交接笔记的形式带过去了 / The conversation came along as a handoff note) | none | Shows the window, opens that task's terminal. |
  | AI account needs attention | A tool reported an authentication, billing, suspension or policy error for an account (5.16). Once per account until the owner cleared it; window focused or not when the queue stopped because of it, otherwise only when the window is not focused. Default sound. | 「工作」需要你处理 / Work needs your attention | Claude Code 报告这个账号有问题。队列已停下，不会换到别的账号。 / Claude Code reports a problem with this account. The queue stopped and will not move to another account. (without a queue, the second sentence is: 有一个会话因此停下了。Codepass 不会把它换到别的账号。 / A session stopped because of it. Codepass will not move it to another account.) The tool's name follows the account's tool: "Codex 报告… / Codex reports..." for a Codex account. Other sessions running under the account are left alone (plan 4.16 decision 34). | none | Shows the window, opens Accounts at that pass. |
  | Queue: finished | The last queued ticket ended and nothing waits for the owner. Once. Default sound. | 队列跑完了 / The queue is finished | 完成 3 张，失败 1 张，跳过 0 张 / 3 done, 1 failed, 0 skipped | 查看 / Review | Shows the window, opens Tickets at Finished. |

  - **Names in the body.** First the Identity Center user (left out when unknown), then " · ", then the accounts: one account "acme-prod"; two "acme-prod、globex / acme-prod and globex"; three or more "acme-prod 等 3 个账户 / acme-prod and 2 more". A portal with no account yet: only the second sentence.
  - **How early, in words:** "大约 15 分钟后 / in about 15 minutes", "大约 30 分钟后 / in about 30 minutes", "大约 1 小时后 / in about 1 hour". Always "about".
  - **"Sign in again"** starts that portal's sign-in in its own private window, the same as the button on the pass. The Codepass window stays where it is. Already signed in again: the click opens Accounts at that portal with the toast "已经登录了 / Already signed in".
  - **Opening Accounts at a portal:** the page scrolls to the portal's section and its pass gets the one-time highlight (the existing insert motion; none under reduced motion).
  - **Never in a notification:** tokens, sign-in codes, URLs, account IDs, role names, file paths. A long portal name is cut by the system, not by us.
  - **When notifications are off or unavailable** and the window is visible: the same title, body and button as an in-app toast (danger for "ended", warn for the heads-up; "ended" stays until dismissed). When the window is hidden, nothing extra: the pass, the sidebar count and the repository rows already show the expired state.
  - **Queue notices** (milestone 8) name the ticket by number, the repository by its local folder name and the account by the owner's label. They never contain ticket text, the AI's words, a file path or an email address. The account notice never contains the tool's own message (it may name an email address); that message is shown only in the app. "Needs you" for a queued ticket replaces the plain "AI task waiting" notice for that task; the two are never both sent.
  - **"Continue on Personal" from a notice** is the owner's click, so it is offered while automatic switching is off (plan 4.17). It never switches without the confirm when the ticket is already running, and it is never on the account notice.
  - **One button at most**, because macOS shows a single button by name and folds two or more into a menu. The button text follows the app language, also for notifications sent after a language change.
  - **Windows:** the same words. The title is one line. Old notifications cannot be withdrawn there, so a heads-up may still be listed after the sign-in was renewed; pressing its button then only shows "已经登录了 / Already signed in".

---

## 7. Implementation notes for the frontend

- Editor: CodeMirror 6. Map `--syn-*` to a `HighlightStyle`; the change bar is a gutter; hover, completion and peek are CodeMirror tooltips / widgets styled with `.hover-card`, `.ac`, `.peek`. Diff and conflict views use `@codemirror/merge` with the `.dl` look (line tints, no word-level strike-through).
- Terminal: xterm.js with a theme object built from `--term-*`; the DOM renderer first.
- Lists that can be long (commits, files, find results, tasks) are virtualised; motion is skipped above 200 rows (section 3).
- All new strings go through i18n with the keys grouped by screen (`setup.*`, `projects.*`, `ws.*`, `code.*`, `changes.*`, `tasks.*`, `history.*`, `branches.*`, `stash.*`, `conflict.*`, `pr.*`, `accounts.git.*`, `identity.*`, `update.*`, `settings.ai.*`, `settings.awake.*`, `settings.notify.*`, `notify.*` for the notification texts in section 6; from milestone 8 also `tickets.*`, `queue.*`, `aiaccounts.*`, `usage.*`, `settings.queue.*`). The copy in this document is the source for both languages.
- Mock scenarios for the demo backend match the mockup data, which is **generic by rule**: project `acme` (repositories `acme_api`, `acme_agent_portal`, `acme_cs_portal`, `acme_merchant_portal_v4`), projects `globex` and `umbrella`, AWS accounts `acme-prod`, `globex`, `initech` with account IDs `111122223333`, `444455556666`, `777788889999`, GitHub account `alex-dev`, Bitbucket account `alex_acme`, the person `Alex <alex@acme.example.com>`, and the five tasks. The same names are used in tests, fixtures, the tutorial, the guide and every screenshot. No real company, repository, account or person name may appear in the repository; the publish check blocks them.
- Every new Go service gets a typed mock in `frontend/src/api/mock` (the mocks are typed against the generated bindings, so a missing one fails the type check). Two mocks are scripted: a terminal that replays a recorded Claude Code session, and a language server that answers hover, completion and definition for the mock files.
- `dangerouslySetInnerHTML` stays at zero uses; a test enforces it.

---

## 8. Not drawn, and open points

Not drawn in the mockup; specified above in words: quick open / palette, create pull request sheet, repository settings sheet, working-tree switcher menu (open), reset / discard confirms, blame margin, the cross-repository Pull requests page, the Windows window frame around the workspace, the updated clone modal (the local folder name field; later the source switch: AWS profile / GitHub account / Bitbucket account / paste a URL, and a Project field), the Force stop confirm, the "moved by hand" row, the running and failed states of Rename local folder and Free up space, the system notification banner (drawn by macOS or Windows; title, body and button are given in the table in section 6), the Settings rows for sign-in reminders and for notifications turned off (5.14).

**Open repository (5.7c) is drawn later**, by A2-16 ("Fable first"), from the session switcher's drawing.

**Milestone 8 screens are drawn later.** Tickets (5.15: the page, the queue bar in each state, the editor, quick capture), AI accounts and usage (5.16: the accounts group, the add-account sheet, the usage popover, the sidebar chip), the limit banner and account chip on the session bar (5.6), the "换账号 / Switch account" menu, the switch-back hint and the New task sheet's account row with its one-line reason (5.6), the two account rows in the Repository settings sheet (4.2), the project's AI accounts sheet (5.2), the account on AI tasks rows and session switcher rows (5.7, 5.7a), the ticket editor's account field (5.15), the "默认用于 / Default for" line, the account details and the remove preview (5.16), the queue settings card (5.14) and the Tickets item in the sidebar (4.1) exist only as words and text sketches. So do the third batch's parts: the switch confirm with its extra lines, the four result lines and the "查看笔记 / View note" sheet, the account-problem line on the session bar (5.6); the automatic-switching switch with its one-time confirm, the "需要你处理 / Needs your attention" pass and its "为什么 / Why" popover, the two new lines in step 1 and the consent paragraph in step 2 of the add-account sheet (5.16); the personal-use line in Settings (5.14); copies and notes in Free up space (5.7b); the two new queue bar rows (5.15). The milestone's tasks draw them in the mockup first ("Fable first"), take the shots, and only then build. Mock data for them: AI accounts "工作 / Work" (the tool's own folder, first in the order, so the app default) and "个人 / Personal" for Claude Code, and the same two labels for Codex ("工作 / Work" is the tool's own, first; "个人 / Personal" is the extra one); project globex uses Personal, repository acme_agent_portal in project acme uses Personal, acme_api and globex_web follow their projects; tickets #6 to #17 in the repositories of section 7.

Drawn at the milestone-0 gate (2026-11-16): the session switcher, Rename local folder, Free up space, the "closed, still running" rows, the quit dialog.

Open points for the planner or the owner:

1. **Tab order.** Code is first because the owner called the editor a first-class part. If daily use shows Changes is opened more, the default tab can follow "last used per repository" without moving the tabs.
2. **Pull requests in the sidebar** adds a fourth primary item. It can fold into Projects (as a filter) if four feels heavy.
3. **Line comments on pull requests** open in the browser in 1.0 (owner's allowance). The Files changed tab is read-only.
4. **Interactive rebase** is left out of 1.0 (owner marked it optional).
5. **Session switcher on Windows.** Ctrl+J inside a terminal is a line feed that tools use for a new line, so there the switcher is Ctrl+Shift+J. The owner asked for Ctrl+J; this is the one place the design departs from it.
6. **Rename to another disk** is not offered in 1.0.
7. **Keep-awake vs screen lock.** The owner asked for `caffeinate -d`, which also stops the screen from locking. The setting "屏幕也保持常亮" defaults to on as asked; the limits text says so plainly.
8. Mock data in the mockup (versions, sizes, fingerprints, account IDs, email addresses, paths, session IDs) is illustrative, not checked against real releases.
9. **The word for a ticket.** The design uses "工单 / Ticket". The owner said "ticket" in English; "工单" is the planner's choice for Chinese and is the owner's to change (for example "待办" or "任务单").
10. **Tickets as a board.** Three columns side by side follow the owner's three states. If the owner mostly keeps a long Editing list, one list with a state filter would use the width better.
11. **Where the usage chip sits.** In the sidebar above sign-in status, always visible. It could instead be a single line by the keep-awake chip.
12. **An account's identity.** A pass shows the owner's label only, never an email address or plan name, because the app stores nothing from a tool's account file. If the owner wants the email shown, it can be read from the tool on demand and shown without being stored.
13. **One order, two meanings.** The order on the AI accounts page gives both the app default (the first account) and the order in which the next account with usage is looked for (the New task fallback, the "Continue on" button, and automatic switching when it is on). If the owner wants them apart, the app default becomes its own setting.
14. **"默认" has one meaning.** The "默认 / Default" badge on the tool's own folder is gone; line 2 of that pass says "工具自带 / Tool's own", because "默认 / default" now means the account a new task starts with.
15. **Switching back is offered, never done.** The hint after an automatic switch is one quiet line. If the owner would rather have sessions return by themselves at the reset, it becomes a setting; each return sends the whole conversation again.
16. **The word for the hidden reasoning.** The confirm's popover says "AI 没有显示出来的思考记录 / the AI's hidden reasoning notes". It is the one thing a switch cannot carry, for some models, and the design says so once, behind a link. If the owner wants it in the confirm's body instead of behind the link, it becomes line 3.
17. **Automatic switching: how much to say.** The confirm quotes two sentences for Claude Code and one for Codex, in English as published. A Chinese gloss under each quote would help reading but would be our wording of a legal text; the design leaves the quotes alone and links to the pages.
18. **A Team or Enterprise seat is not marked.** The add-account sheet has one line about it. Codepass does not store what kind of account it is, so it cannot add a line to the switch confirm when a conversation moves between a seat and a personal account. If the owner wants that line, the sheet gets a checkbox "这是团队或企业席位 / This is a Team or Enterprise seat", stored as one flag.
19. **An account that needs attention blocks its sessions from moving.** The design is strict: until the owner cleared the state, the app does not move that account's sessions, even by hand. The other choice is to allow the owner's own click from the switch menu, with the "为什么 / Why" text shown first.
20. **"修好的副本 / repaired copy".** The word in Free up space and in the result line's help. "去掉账号专属内容的副本 / a copy without the account-only parts" is more exact and longer.

---

## 9. Screenshots

All 1440x900, in `docs/design/codepass-mockup/shots/` (55 files with generic data: 49 from revision 2, of which `ai-tasks-light-zh`, `ai-tasks-light-en` and `setup-noadmin-light-zh` were re-taken, plus 6 new). Light zh-CN for every screen; dark and English for a selection; extra states: `setup-idle`, `setup-failed`, `setup-noadmin`, `setup-done`, `setup-win`, `projects-drag`, `projects-found`, `workspace-code-find`, `identity-page`, `update-banner`.

New in revision 3: `session-switcher-light-zh`, `session-switcher-dark-en`, `rename-folder-light-zh`, `free-space-light-zh`, `ai-leftover-light-zh`, `quit-dialog-light-zh`. `setup-noadmin-light-zh` was re-taken with the new sentence.

To re-take one: `docs/plan/codepass/shoturl.sh docs/design/codepass-mockup/shots/<name>.png 1440 900 "file://<repo>/docs/design/codepass-mockup/index.html?screen=<id>&theme=light&lang=zh-CN&bar=0"` (add `&state=<state>` where needed).
