# Codepass v1.0.0 - product plan

Date: 2026-11-16 (revision 3, after the milestone-0 gate). Author: planner (Fable). Audience: UI designer (Fable), implementers (Sonnet), reviewers (Opus), the owner.

Revision 2 replaces revision 1 completely. It applies the owner's final decisions, the owner's clarification that the editor covers the whole repository, and the principal engineer's review. Companion files: `ui-design.md` (screens and copy), `mockup/` and `shots/` (the drawn screens), `tasks.md` (who builds what, in which order).

Revision 3 applies the findings of the seven spikes (`docs/research/spike-*.md`) as corrected by the two Opus reviews (`spike-review-a.md`, `spike-review-b.md`), and the owner's requirements of 2026-11-16 (a local folder name that differs from the remote name; disk clean-up for ended AI sessions; force-stopping programs a session left behind; a quit dialog that always asks; a session switcher on Cmd+J). A finding is cited as `[M0 S0-0N]`. What nobody could run yet is listed in `manual-checks.md`.

Added on 2026-11-16 (the owner's third batch, three statements): a switch to another account at a usage limit must lose nothing of the conversation; several accounts must stay inside the companies' terms, for the owner's own use and not for resale; task A on account A and task B on account B at the same time. They are in 4.16 ("Switching keeps the conversation", decisions 22 to 28; "Several accounts within the terms", decisions 29 to 36) and in 4.17, with the research in `docs/research/claude-usage-accounts.md` sections R8 and R9.

Status of facts: an external claim carries a source tag `[S#]` (section 11). `[R#]` is a finding of the principal engineer's review, checked against the repository or the Wails v3.1.0 source in the module cache. "Checked locally" means it was run on the owner's Mac. "Spike" meant nobody had proved it; after milestone 0 such a line says either what was measured or "not verified", with the manual check that settles it.

---

## 0. The plan in one page

- **AWS SSO Hub becomes Codepass**: the one window the owner works in all day. Accounts (AWS SSO, GitHub, Bitbucket), projects that group repositories, a full git client, pull requests, AI coding tasks (Claude Code, Codex, OpenCode; several at once per repository), and a code editor for the whole repository with code intelligence for Go, TypeScript / JavaScript and PHP (PHP added by the owner on 2026-11-16).
- **One release, v1.0.0.** Work happens on the `codepass` branch and merges into `main` at release. There are nine internal milestones (0 to 8; milestone 8, added on 2026-11-16, runs right after milestone 3). Each one ends in a signed build that the owner uses daily and that updates itself through a private test channel. Nothing is published before everything is done.
- **Platforms:** macOS on Apple silicon, macOS 15 or later (arm64 only); Windows 10 1809+ and Windows 11, amd64.
- **Size, honestly:** about 45 weeks of the implement-and-review loop (32 before the owner's first requirements and the spike findings added ten tasks; 35 before the owner's second batch of 2026-11-16 added the 0.0.2 carry-over, system notifications, PHP code intelligence and milestone 8; 43 before the owner's third batch, the three statements S1 to S3 of the same day, grew milestone 8 from 6 to 8 weeks). This is an estimate, not a measurement. The owner decided that nothing moves to 1.1 (decision 4).
- **The owner's second batch (2026-11-16)**, with the why: one app instead of a JetBrains IDE, Warp, the Bitbucket website, Claude Desktop (opened only for the weekly usage) and Sublime Text (where ideas wait for usage). It adds: the fixes of AWS SSO Hub 0.0.2 (4.2, task F1-21); system notifications with a "Sign in again" button when an AWS sign-in expires (4.15); several AI accounts per tool, chosen freely per task with a default per project and per repository, with a switch in both directions that never starts the work over without its conversation, several tasks on different accounts at once, all of it inside the companies' terms for the owner's own use, and the usage meter (4.16); tickets and the queue that runs them within the usage (4.17); PHP code intelligence (4.12). 4.16 and 4.17 cover Claude Code and Codex alike.
- **The four decisions are made** (`tasks.md`, "Owner decisions (FINAL)"): the tools gate stays closed; one classic GitHub token per account; the Windows build ships unsigned; everything ships in 1.0.0. Six smaller questions the spikes raised are listed there under "Open questions for the owner".
- **Where the plan differs from the owner's first wording, with the reason:**
  1. Bitbucket is not SSH-only. API tokens work for git over HTTPS and for the REST API [S1][S2]. The app still uses one SSH key per account for git, and a token for pull requests.
  2. Several AI sessions in one folder damage each other's work. Each new task therefore gets its own git worktree and branch by default.
  3. An AI task runs with the user's own rights. The app organises tasks and keeps them apart from each other; it does not sandbox them, and the UI says so.
  4. "Smart" keep-awake releases after a task has waited for the owner for a while, and has a hard ceiling, so a forgotten session cannot keep the Mac awake for days.

---

## 1. Vision, name, platforms

**One sentence:** every account, every repository and every AI coding task in one calm window, with the right identity applied automatically.

**Who it is for:** a developer who works for several companies at once: several AWS organizations, several GitHub and Bitbucket accounts, a dozen or more repositories, and AI agents doing part of the coding.

**What it replaces:** Sourcetree and the Bitbucket website for daily work, a separate terminal for AI coding, hand-edited `~/.ssh/config` and `.git/config`, and opening GoLand or WebStorm to read and fix code.

**What it does not replace:** pipelines, issues, wikis, permission management, line-by-line pull request discussion (these open in the browser), a debugger, run configurations and refactoring tools.

**The promise that stays:** the right account on every repository, every change shown first and backed up, plain language, nothing hidden.

### 1.1 Name and identifiers

| Thing | Value | Note |
|---|---|---|
| Display name | **Codepass** in zh-CN and en | No separate Chinese name. |
| Subtitle | 账号 · 仓库 · AI 编码 / Accounts, repositories, AI coding | Under the brand in the sidebar. |
| BundleID | `com.rudian.awsssohub` (unchanged) | Keeps the data folder decisions simple. Consequences in 4.2. It is also part of the updater's code requirement (4.14) and of the single-instance ID, so it must never change without a hand-over release. |
| App file | `Codepass.app`, `codepass.exe` | The old `AWS SSO Hub.app` / `aws-sso-hub.exe` is a different file; 4.2 handles it. |
| Data folder | `AWS SSO Hub` (unchanged, `internal/paths.AppDirName`) | Settings and backups stay where they are. |
| GitHub repository | `rudian/aws_sso_hub` renamed to `rudian/codepass` at release | The updater uses the new path. GitHub redirects the old one. |
| Go module path | renamed to `github.com/rudian/codepass` in milestone 1 | One mechanical commit. |
| Managed markers in users' git config | unchanged (`awsssohub.profile`, helper and guard text) | Old bindings must keep being recognised. New marker `awsssohub.account` for SSH bindings. |

### 1.2 Platforms and signing

- macOS: arm64 only, `LSMinimumSystemVersion` 15.0. The universal build tasks in `build/darwin/Taskfile.yml` are removed. Reason: Homebrew is mandatory, and Homebrew's package installer supports Apple silicon and macOS 15+ only [S6][S7].
- Windows: amd64, Windows 10 1809+ (the floor for ConPTY and for Claude Code [S10]) and Windows 11.
- **Signing (macOS):** Developer ID Application certificate of the team "<owner company>" (Team ID `XC34P8TNZ6`), hardened runtime, notarized, stapled. Done once end to end in about 70 seconds, accepted with no entitlements [M0 S0-05]. Checked locally: the identity is present in the owner's login keychain. The owner confirmed on 2026-11-16 that Xcode is signed in with this account and can sign with this team. Only a team's Account Holder can create a Developer ID certificate [S22]; the owner is an Admin, so the existing certificate and its private key are precious: section 8 has the backup task.
- **Signing (Windows):** the build ships unsigned in 1.0 (decision 3; one confirmation is still asked, see `tasks.md`). The exe gets SmartScreen warnings and may be blocked by Smart App Control [R-S16]. Updates are still protected by the signed manifest.
- AWS SSO Hub 0.0.1 keeps working on Intel Macs and macOS 13-14 and gets no further updates (it has no updater). The README says so.

---

## 2. Information architecture

Full detail, copy and states are in `ui-design.md` sections 4 and 5. Summary:

**Sidebar (management layout):** Projects, AI tasks, Tickets (milestone 8; 4.17), Pull requests, Accounts; the AI usage chip (4.16); sign-in status (one row per AWS portal, GitHub account, Bitbucket account); Git settings, Health, Tutorial; the keep-awake chip while it is held; Settings (dot when an update is ready). The Terminal helper page is removed; its job moves into the workspace.

**Projects page:** today's Repositories table, grouped into collapsible project sections (the same component as portal sections on Accounts). Rows show provider mark, the repository's local folder name (and "remote: NAME" as secondary text when the two differ), account chip, branch and sync counts, status, AI badge. Drag or bulk-move between projects. Repositories in no project sit in "Ungrouped". New in revision 2: "Hide repository" and a one-time review of newly found repositories (4.4).

**Repository workspace:** opening a repository switches the window to the workspace layout (icon rail, full-width header, status bar). Six tabs, **Code first**:

| Tab | Holds |
|---|---|
| **代码 Code** | A full editor for the whole repository: file tree with git colours and filter, tabs, split view, breadcrumbs, outline, quick open, go to symbol, find in files, go to line, code intelligence for Go and TS/JS, editing with save. |
| **改动 Changes** | Changed files, diff, stage / unstage / discard by file, hunk and line, commit composer with amend, stashes, recently discarded. Becomes the conflict screen during a merge, rebase, cherry-pick, revert or stash apply. |
| **历史 History** | Commit list with lane graph and ref chips, search and filters, commit details with diff, compare two commits, file history, blame; cherry-pick, revert, reset, tag, branch from here. |
| **分支 Branches** | Local and remote branches, tags, remotes; compare; merge, rebase, create pull request. |
| **拉取请求 Pull requests** | GitHub and Bitbucket: list, create, detail with checks and approvals, merge, close or decline. |
| **AI 任务 AI tasks** | Task list and the selected task's terminal. |

Not tabs: the **working-tree switcher** in the header (main workspace, or any AI task's own workspace; it re-points Code, Changes, History and the terminal at that folder, and each folder keeps its own open tabs); the **sync button** (it always describes the selected folder); the **terminal panel** at the bottom (a plain shell there is not an AI task and never holds the computer awake); the **repository settings sheet** behind the gear.

**App menu and window behaviour** (`ui-design.md` 4.4): the app installs its own menu. Cmd+W closes an editor tab, Cmd+R never reloads the page, closing the window while tasks run hides it. **Quit with live AI sessions always asks, every time:** "Stop all and quit" or "Keep running in the background" (no "don't ask again"); it never blocks a logout.

**Session switcher** (`ui-design.md` 5.7a): Cmd+J (Ctrl+J on Windows) from any screen lists every live AI session in every repository, waiting ones first; Enter goes straight to that session's terminal.

---

## 3. Data model and storage

### 3.1 Files

`settings.json` keeps **preferences only**. Projects, accounts and tasks get their own versioned files in the same data folder. Reason: `settings.Open` reads into a fixed struct and `saveLocked` rewrites the whole struct, so one run of the old 0.0.1 app would delete anything new that lives in `settings.json` [R4].

| File | Holds | Written by |
|---|---|---|
| `settings.json` | Preferences (theme, language, motion, scan folders, editor, AI defaults, keep-awake, update check). New fields only with safe zero values. | `internal/settings` |
| `projects.json` | `schemaVersion`, projects, per-repository records (project membership, hidden flag, identity override, AI defaults, the last integrated remote commit per branch); the AI account defaults per tool, on projects and on repositories (4.16 "Choosing the account"). An AI account default is an account ID from `ai-accounts.json`, never a folder path. | `internal/projects` |
| `accounts.json` | `schemaVersion`, GitHub and Bitbucket account metadata, SSH key metadata (path, public key, fingerprint). **No token, no private key.** | `internal/accounts` (git providers part) |
| `tasks.json` | `schemaVersion`, AI task records | `internal/tasks` |
| `sessions/<task-id>/` | Per task: the session-only hook plugin, the saved scrollback of an ended session (mode 0600; a setting turns saving off), and the **handoff notes** of account switches (`handoff-<n>.md`, mode 0600; 4.16 decision 27). Never in Codepass's backups, an export or the log. The folder is excluded from Time Machine, like the worktrees folder (4.7 point 2). Removed by "Free up space" (4.8a). | `internal/tasks`, `internal/terminal`, `internal/aicarry` |
| `recovery/` | Mirrors of unsaved editor buffers, one file per open buffer, removed on save or discard | `internal/editorfs` |
| `known_hosts` | The providers' pinned SSH host keys, app-managed | `internal/sshx` |
| `notifications.json` | `schemaVersion`, whether the app asked for notification permission, the last notice sent per portal sign-in (4.15). No notification text. | `internal/notify` |
| `ai-accounts.json` | `schemaVersion`, the AI accounts per tool: label, colour, order (the first account in the order is the app-level default for that tool, 4.16), the config folder path (empty for the tool's own account), whether automatic switching is on for that tool (`autoSwitch`, **off unless the owner turned it on**, and when the owner confirmed it; 4.16 decision 33), which shared parts are linked, the last usage reading with its time (4.16), and whether the account needs the owner's attention (the kind and the time only, never the tool's message; 4.16 decision 34). **No token, no email address, nothing read from a tool's account file.** | `internal/aiaccounts` |
| `tickets.json` | `schemaVersion`, tickets (text, repository, tool, the account choice, state, order, the linked task) and the queue's own state (4.17). Ticket text is the owner's own writing: it is in a backup, and in an export only when the owner ticks "tickets". | `internal/tickets` |
| `ai-accounts/<tool>/<slug>/` | The config folder of one **extra** AI account. The tool owns everything inside (its sign-in, its account file). The app creates the folder, the links to the shared parts, and the conversation copies of 4.16 decisions 2 and 27, nothing else. Never in a backup or an export. | the tool; links by `internal/aiaccounts` |
| A **repaired conversation copy** (not in the data folder: it is one new file, plus its sub-agent folder, in the tool's conversation folder, where the tool looks for sessions) | The conversation of one session under a new session ID, without the parts tied to the account it left (4.16 decisions 22 and 27). Always a new file; the original is never changed. The copy is written with mode 0600 and its folders 0700, whatever the original's mode. Its exact paths are recorded on the task. Never in Codepass's backups, an export or the log (Time Machine copies it like the tool's own transcripts). Removed by "Free up space" for that task only (4.8a). | `internal/aicarry` |

Every file: atomic write (temp file + rename), a `schemaVersion`, forward migration with a backup snapshot before the first write of a new version, and a refusal to load a file whose version is newer than the app understands (the app says so instead of overwriting it).

### 3.2 Entities

```
Project        { id, name, collapsed, order, identity?: Identity, aiAccounts?: AIAccountDefaults, createdAt }

RepoRecord     { id (random, stable; never derived from the path), legacyId (the 0.0.1 hash of the path),
                 path, projectId?, hidden, provider, lastTest?,
                 binding?: Binding, identityOverride?: Identity, ai?: RepoAIConfig, aiAccounts?: AIAccountDefaults,
                 integrated: { "<remote>/<branch>": sha } }
   provider    = "codecommit" | "github" | "bitbucket" | "other"   (from the remote URL, cached)

ProviderAccount
   aws         -> not stored. Read live from ~/.aws/config, as today.
   github      { id, login, host:"github.com", sshKeyId, tokens:[{scope:"user"|org, state}], color, monogram }
   bitbucket   { id, username, workspaces:[slug], sshKeyId, tokenState, color, monogram }

SSHKey         { id, accountId, type:"ed25519", privatePath, publicKey, fingerprint,
                 createdAt, verifiedAt?, managed, hasPassphrase }

Binding        aws:  { kind:"aws-profile", profile }        (today's behaviour)
               ssh:  { kind:"ssh-key", accountId }          (GitHub / Bitbucket)
               none: the repository follows the machine's own git setup

Identity       { name, email }

AIToolConfig   { tool:"claude"|"codex"|"opencode", command, extraArgs:[...], enabled }
RepoAIConfig   { defaultTool?, defaultIsolation?: "worktree"|"main", setupCommand?, preselectAwsProfile:false }
AIAccountDefaults { claude?: accountId, codex?: accountId }   (a tool that is missing = follow the level above; 4.16)

AITask         { id, repoId, title, tool, isolation, worktreePath?, branch?, baseBranch, baseCommit,
                 state, createdAt, endedAt?, exitCode?, toolSessionId (created by the app at task creation),
                 aiAccountId? (the account of the current session; changes on a switch, 4.16; empty for OpenCode and
                              while the shell decides; a record without it, made before milestone 8, is the tool's own account),
                 sessionIds: [...] (every session ID this task has had, oldest first; the last one equals toolSessionId.
                              A copy made by the tool or by the app gets a new ID; 4.16 decision 25),
                 switches: [{ from, to, at, by: "owner"|"auto", atLimit: bool,
                              layer: "same-session"|"repaired-copy"|"handoff-note" (what carried the conversation; 4.16 decision 22),
                              sessionId (the ID the session has after this switch), removed?: [...kinds taken out of a repaired copy],
                              files?: [...exact paths the app created for this switch: the copy, or the note] }],
                 autoSwitchedFrom?: accountId (the account the session was on before the first automatic switch in a row;
                              kept through later automatic switches; cleared when the session is back on it or when the owner switches),
                 pendingSwitch?: { state: "in-progress", to, plannedSessionId, files: [...every path the app will create], by, startedAt }
                              (the journal of a switch; written before any file, removed when the session runs under the
                              other account or the switch is rolled back; 4.16 "Switch and resume"),
                 lastTurnCutOff?: "limit"|"owner" (the last turn was cut off at a usage limit, or by "Stop and switch"; persisted,
                              so the first message of 4.16 decision 23 survives a restart; cleared when the next turn starts),
                 lastPermissionMode? (the last permission mode the tool reported, 4.8; persisted for 4.16 decision 22),
                 ticketId? }
   state       = "starting" | "running" | "waiting" | "done" | "failed" | "stopped"
   limit       = none | { window:"session"|"weekly"|"model", resetsAt?, continuesBy:"tool"|"queue"|"nobody" }   (memory only; 4.16. That the last turn was cut off is not memory only: `lastTurnCutOff` above)

AIAccount      { id, tool:"claude"|"codex", label, color, order, configDir? (empty = the tool's own default folder),
                 linked:[...shared parts], toolOwn (= configDir is empty; it lives in the tool's own folder),
                 lastUsage?: UsageReading,
                 modelsReplaced?: [{ model, usedInstead, seenAt }] (models the tool replaced under this account, and by which; only for the switch confirm,
                              4.16 decision 24; emptied when the owner signs in again),
                 attention?: { kind: "sign-in"|"billing"|"refused"|"unknown", at } (set when the tool reports a problem with the
                              account itself; cleared only by the owner; 4.16 decision 34) }
AIToolAccounts { tool:"claude"|"codex", autoSwitch:false (per tool, not a queue setting; off in a new file and after an upgrade;
                 4.16 decision 33), autoSwitchConfirmedAt? (when the owner read the confirm and turned it on) }   (in ai-accounts.json)
UsageReading   { readAt, source:"tool-report"|"unknown", creditsOn?: bool,
                 windows:[{ kind:"session"|"weekly"|"weekly-model", model?, usedPercent, resetsAt? }] }

Ticket         { id, text (Markdown), repoId?, tool?,
                 aiAccountId? (empty = follow the repository, 4.17; an ID that names no account counts as empty; left out of an export),
                 isolation?,
                 order, createdAt, updatedAt,
                 state, taskId?, note?: "limit"|"needs-you"|... , endedAt? }
   state       owner: "editing" | "on-hold" | "queued"
               system: "running" | "waiting-reset" | "needs-you" | "done" | "failed" | "skipped"

Queue          { paused: bool, pausedReason?, currentTicketId?, settings: QueueSettings }   (one for the whole app)
QueueSettings  { reservePercent:0, allowCredits:false, askBeforeEach:false, queueNote: text }

TerminalSession { id, repoId?, taskId?, cwd, kind:"shell"|"ai", pid, cols, rows, startedAt }   (memory only)
```

- A repository belongs to at most one project. Projects are flat.
- **Which AI account a new task uses by default** is resolved per tool in three levels, like the author identity (3.4): repository (`RepoRecord.aiAccounts`) > project (`Project.aiAccounts`) > app (the first account of that tool in the order of `ai-accounts.json`). A level that names no account follows the level above. An ID that no longer names an account counts as not set. The default only pre-selects; the task's own `aiAccountId` is what it runs under (4.16).
- **A repository's ID is not its path.** In 0.0.1 the ID is a hash of the path, so a renamed folder is a different repository. From milestone 1 every record gets a random ID once; the path is a field that can change (4.4a). A repository has two names: the **local folder name** and the **remote name** (the last segment of the origin URL). They may differ, and the UI shows both when they do.
- `AITask` records persist, so finished tasks, their branches and workspaces can still be reviewed, merged or discarded after a restart. Processes do not survive a quit; "Resume" restarts the tool with the stored `toolSessionId`.
- **A switch record says what carried the conversation.** Every entry of `switches` names the layer (4.16 decision 22) and the exact paths of any file the app created for it. Those paths, the paths in `pendingSwitch` (also after a switch that was rolled back: they move to a `switches` entry that names no layer), and every ID in `sessionIds`, are what "Free up space" lists for the task (4.8a). A path is recorded before the file is written, so no copy or note can be on disk unlisted. A task record from before this field counts as `same-session`.
- The repository engine (`internal/repos`) stops being CodeCommit-only. Classification, detection and binding become per-provider strategies behind one interface. The existing repository tests (over 1,300 lines) must pass unchanged before any new provider is added.
- **Export and import** cover projects, account metadata (login, colour, workspaces), identities and preferences. They never contain tokens, private keys or key file paths. A test asserts it. The AI account defaults are not exported: AI accounts belong to one computer. An export never contains an AI account folder, anything signed in, a handoff note or a conversation copy (4.16 decisions 27 and 30).

### 3.3 Where secrets live

| Secret | Where | Honest limit |
|---|---|---|
| AWS SSO tokens | `~/.aws/sso/cache`, as today | Shared with the AWS CLI. Any process of the user can read them. |
| SSH private keys | `~/.ssh/codepass_<provider>_<login>_ed25519`, mode 0600; on Windows an ACL that allows only the user | `ssh` needs a file or an agent. Keys the app creates have no passphrase, and the UI says so. Any process of the user, including an AI task, can read them. |
| GitHub / Bitbucket API tokens, macOS | Keychain, through **Security.framework** called from Go (cgo is already on) | The keychain item then trusts Codepass's code signature only. A library that shells out to `/usr/bin/security` would make the item readable by that tool, and so by any process, without a prompt [R11]. |
| The same, Windows | Windows Credential Manager | Readable by any process running as the user. The UI says so. |
| AI tool sign-ins (Claude Code, Codex, OpenCode) | **Not the app's.** Each tool keeps its own sign-in in its own config folder or in its own keychain entry for that folder. Codepass never reads, copies, stores or sends one (4.16). | Any process of the user, including an AI task, can read what the tool stores in a file. An extra account's folder under `~/.codepass/ai-accounts/` may hold that tool's sign-in as a file (Codex's `auth.json`; Claude Code's fallback file when the Keychain refuses); the folder is mode 0700 and is never in a backup or an export. |
| Update signing public keys (two: current and next) | Compiled into the binary | The trust anchor cannot be replaced by a download. |
| Update signing private key, Developer ID certificate | The owner's Mac only. Never in GitHub Actions or any CI secret. | Release builds are made on the owner's Mac (4.14). |

In sandbox mode (`AWSSSOHUB_SANDBOX`) the token store is a file fake, as `internal/credstore` already refuses to touch the real keychain. Logs never contain tokens, terminal output, language-server traffic or hook payloads; `internal/redact` gains token shapes.

### 3.4 Author identity: three levels, one rule

Precedence: **repository > project > global**. The most specific one that is set wins.

- **Global** = `git config --global user.name / user.email` (backup first, like every other write).
- **Project** = stored in `projects.json` and written into each member repository's local config. Changing it re-applies to the members after showing the list.
- **Repository** = local config. A project change never touches it.
- Worktrees share the repository's config, so AI tasks commit with the same identity.
- **The effective identity is computed by git itself**, not by the app's own bookkeeping: `git config --show-origin --show-scope` and `git var GIT_AUTHOR_IDENT`, run with the same environment a terminal gets. An `includeIf` file or a `GIT_AUTHOR_*` variable exported in a shell profile is then shown as the real source. Those win for commits made in terminals and by AI tools whatever the app writes, and the UI says so [R-S14].

### 3.5 Binding: how a repository uses an account

| Provider | Remote | What binding writes (repository-local, backup first) |
|---|---|---|
| CodeCommit | HTTPS | Today's credential helper pinned to the profile. Unchanged. |
| GitHub, Bitbucket | SSH | `core.sshCommand` = `ssh -i '<key>' -o IdentitiesOnly=yes -o UserKnownHostsFile='<app known_hosts>' -F none`, and the marker `awsssohub.account` [S3][S4] |
| GitHub, Bitbucket | HTTPS | The app offers "switch this remote to SSH". |
| other | any | Not bound. |

Details that the review added [R-S8]:

- `core.sshCommand` is run through `sh`, so the key path is quoted. Windows user names often contain spaces.
- Git started from the app has no terminal to answer a host-key question. The app writes the providers' published host keys into its own `known_hosts` and points the command at it. An unknown or changed host key stops the connection; it is never auto-accepted.
- `-F none` keeps ssh from falling through to other keys in `~/.ssh/config` when the bound key is refused, which would push as a different account. On an authentication error the app re-verifies the account and reports it.
- Imported keys with a passphrase are detected; they need the system key agent, and the UI explains how.
- An unbound GitHub or Bitbucket repository shows "uses this computer's default key". The Git global guard is not extended to SSH hosts in 1.0.

---

## 4. Key user flows

### 4.1 Startup: the tools gate

Verified design after S0-04 and its review. The clean-machine run (macOS 15, 26 and 27 VMs without Command Line Tools) has **not** happened yet; it is acceptance for task F1-12 (`manual-checks.md` B1 to B3).

Rule: Homebrew is required on every Mac, winget on every Windows PC, plus git and AWS CLI v2. The gate **blocks only when something is missing or below the hard minimum** the app already uses: git 2.39.1, AWS CLI 2.9.0 (`internal/envcheck`: `GitMin`, `AWSMinBlocking`). "Older than recommended" is an action on the Health page, never a block [R5].

- **Detection never uses the network.** File checks in known locations (`/opt/homebrew/bin`, `/usr/local/bin`, the Windows install paths) plus `--version` with a timeout. An offline Mac with the tools installed goes straight in. **On macOS the app never runs a developer-tool stub** (`/usr/bin/git`, `make`, `clang`, `python3`, `xcrun`) unless `xcode-select -p` prints an existing path other than `/`: on a Mac without Command Line Tools such a call opens the "install developer tools" dialog. `internal/execx.FindGit` already has this rule for git; it is tightened and applied to every stub [M0 S0-04].
- **macOS install:** one click.
  1. Download `Homebrew.pkg` from the latest `Homebrew/brew` release (154 MB when checked [S8]); size and sha256 come from the GitHub API at download time (fallback when the API is rate-limited: the `releases/latest/download` URL and the signature check).
  2. **Unprivileged pre-check**, shown as "verify signature" before any password is asked: size, digest, `pkgutil --check-signature`, Team ID. A truncated download or a proxy's HTML page is reported as "download damaged, try again", not as a wrong signature.
  3. **One privileged step under a single administrator prompt.** The script text is embedded in the app and passed to `osascript` as an argument, never as a file (a script file beside an app in a user-writable folder would be a way to run code as root). `osascript` is started with a fixed minimal environment and the script runs under `/usr/bin/env -i /bin/bash --noprofile --norc`, with every tool called by absolute path: the review showed that `BASH_ENV`, an exported shell function or `SHELLOPTS` with `PS4` in the app's environment would otherwise change what root runs. As root the script refuses if `/opt/homebrew` exists; copies the package from **one open descriptor** into a `mktemp -d` folder under `/var/root` (root-only), refusing anything that is not a regular file and copying at most the expected size plus one byte; verifies **the copy** (`pkgutil --check-signature` with Team ID `927JGANW46` and notarization; `spctl -a -t install` as a second opinion, with a defined answer when Gatekeeper is turned off); runs `installer` on the copy; writes every reason to stderr, because stdout is lost when the script fails. There is no window in which a process of the user can swap the file [R2] [M0 S0-04].
     - **Pin the Team ID, not the certificate.** Homebrew's package is signed by one maintainer's Developer ID; the certificate changed between 5.1.13 and 7.0.13 and the Team ID did not. The value is a constant changed by a normal Codepass release.
     - The step cannot stream output (`do shell script` returns only at the end), so the UI shows one indeterminate state and then the result.
     - `open -W` (hand the package to Installer.app) is **not built**: its exit status cannot tell cancel from success and a swap window cannot be excluded.
  4. `brew install git awscli` as the user, with `HOMEBREW_NO_ANALYTICS=1`, `HOMEBREW_NO_AUTO_UPDATE=1` for the setup run, and the proxy variables in upper and lower case (below). No sudo after setup [S6]. All 27 formulae behind git and awscli have bottles for macOS 15, 26 and 27 and none has a post-install step, so nothing is compiled.
- **Windows install:** `winget install --id Git.Git -e` and `winget install --id Amazon.AWSCLI -e` with the agreement flags; each may raise a UAC prompt. The AWS CLI installer is treated as machine-wide and needing an administrator [R-S16]; the winget manifest also lists a user scope, but both scopes point at the same MSI, so "per user without an administrator" is used only if the Windows run shows it works (`manual-checks.md` B11). If winget is missing, the app opens the Store page for App Installer.
- **Failure states with a next step** (`ui-design.md` 5.1): not an administrator; network or proxy; blocked by device management; disk full; download damaged; wrong package signature; Homebrew's signer not recognised by this version of Codepass; Homebrew partly installed (`/opt/homebrew` exists); winget unavailable. "Not an administrator" is detected before the prompt (`id -Gn`).
- **Own-folder Homebrew is not offered.** Homebrew's installer refuses a prefix longer than `/opt/homebrew` (13 bytes), and a longer prefix is a Tier 3 setup that compiles git, openssl and python from source, which needs Command Line Tools and so an administrator anyway [M0 S0-04]. A standard account installs the pkg with an administrator's name and password typed once into the macOS prompt; Homebrew's post-install then makes the signed-in user the owner of `/opt/homebrew`. The gate stays closed when the install fails (decision 1).
- **Proxy:** Go's HTTP client, and brew or winget started from a Finder-launched app, do not see the system proxy. The app reads it (`scutil --proxy` on macOS, WinHTTP settings on Windows), uses it for its own requests, and passes `HTTPS_PROXY` / `HTTP_PROXY` / `NO_PROXY` to child processes [R-S11].
- **The app never asks for or handles a password itself.**
- **AI tools and language servers are not installed here.** Each is installed the first time it is used (4.7, 4.12). macOS, with brew: `brew install --cask claude-code` and `brew install --cask codex` (casks that only place a binary; no sudo), `brew install opencode`; for Go, `brew install go` (asked, with its size) and then `brew install gopls` (gopls does not bring Go with it); for TypeScript / JavaScript, `brew install typescript` (the native server, no Node needed) and `brew install typescript-language-server` only for projects on TypeScript 5 or 6 (4.12). Windows: `winget install --id Anthropic.ClaudeCode -e`, `winget install --id OpenAI.Codex -e`; OpenCode has no winget package and comes from npm (not verified: `manual-checks.md` B10); `winget install --id GoLang.Go -e` then `go install`; `winget install --id OpenJS.NodeJS.LTS -e` then `npm install -g typescript`. Names and IDs confirmed 2026-11-16 [M0 S0-04]. No `curl | bash`.
- **Test matrix (milestone 1 and 7):** fresh VMs of macOS 15, macOS 26 and macOS 27 (the owner's Mac runs 27) without Command Line Tools (confirm that no "install developer tools" dialog appears), Windows 10 1809 and Windows 11.

### 4.2 Upgrading from AWS SSO Hub 0.0.1

0.0.1 has no updater, so the first Codepass is a manual download. Three things go wrong unless handled [R4]:

1. **Both apps share the BundleID and the single-instance ID.** If the old app is running, Codepass would hand off to it and quit without a word. Before the single-instance handoff, v1 detects a running old app (by bundle path and process name), asks the user to quit it, and then offers to move `AWS SSO Hub.app` / `aws-sso-hub.exe` to the Trash. On Windows 0.0.1 shipped as a portable zip, so only that exe goes to the Recycle Bin (there is no uninstaller).
2. **The old app rewrites `settings.json` from its own struct.** New data therefore lives in separate files (3.1).
3. **macOS asks again for Documents access.** The old permission was tied to the ad-hoc build's code hash. The release notes say so.

After the first scan, repositories that were already listed stay; newly found ones go through the one-time review (4.4).

A data folder written by AWS SSO Hub 0.0.2 upgrades like a 0.0.1 one: its extra session fields in `settings.json` (`sessions.*.sessionEndsAt`, `sessions.*.newSession`) are read as they are and kept, and nothing else in that folder differs.

### 4.3 Add a GitHub or Bitbucket account

Four steps (`ui-design.md` 5.11): account; SSH key (create one, or use an existing key that no other account in the app uses); give the public key to the provider and verify with `ssh -T` using exactly that key; API token.

- Each account has **an SSH key for git and an API token for pull requests**.
- **GitHub token:** one **classic** personal access token per account with the scopes `repo` and `read:org` only (decision 2; `workflow` is not needed because git goes over SSH). Fine-grained tokens are not supported in 1.0. Read endpoints, including check runs and the combined status, were measured with a classic token; write actions are from documentation until `manual-checks.md` A11 [M0 S0-07]. An organisation can block classic tokens, and a SAML organisation needs the token authorised per organisation [S23]: for such an organisation pull requests open in the browser, and the add-account text says so.
- **Bitbucket token:** an Atlassian API token with the scopes `read:user`, `read:workspace`, `read:repository`, `read:pullrequest`, `write:pullrequest` (the spike's mapping from the API's older scope names; confirm on the token screen: A14). REST calls use `Authorization: Bearer <token>` [S1]. Workspaces come from the token (`GET /user/workspaces`); pre-filling from existing remotes is the fallback. Only `/workspaces` and `/user/permissions/workspaces` were removed on 2026-04-14 [S9]. Closing the source branch is the `close_source_branch` flag on create and merge. Atlassian tokens are created with an expiry date, so "token expired, paste a new one" is a routine state. "App password" never appears in the UI: app passwords stopped working on 2026-06-09 [S2].

### 4.4 Projects

- **Suggestions** from a shared name prefix or a shared parent folder. One dismissible banner per suggestion; never automatic.
- **Create, rename, remove, move** as designed (`ui-design.md` 5.2). Removing a project touches nothing on disk.
- **Newly found repositories.** The engine now lists every git repository under the scan folders, so the owner's 13 rows could become dozens of forks and experiments. New repositories are not added to the list. A one-time banner offers "Choose which to show" or "Hide them all" [R-S13].
- **Hide repository.** A hidden repository leaves the list, the counts and all background work.
- **Clone:** the existing multi-select clone modal gains a "local folder name" field per repository (milestone 1; also in multi-clone), and later a source switch (AWS profile / GitHub account / Bitbucket account / paste a URL) and a Project field (milestone 6).
- **Find a repository fast and start work** (owner, 2026-11-16, second batch: "filter the repository list by name, to find the one I want and start working quickly"). Two ways, both typed, both keyed on the **local folder name** first:
  - **On the Projects page** (the search box F1-14 built, which already matches the local folder name, the remote name, the CodeCommit name and the path): Cmd/Ctrl+F focuses it; results are ranked folder name starting with the text, then a word inside the folder name starting with it (after `_`, `-`, `.` or a case change: `agent` finds `acme_agent_portal`), then the text anywhere in the folder name, then the other names and the path; the matched letters are highlighted; Down and Up move through the matches, **Enter opens the selected repository** (the first match unless another is selected), Cmd/Ctrl+Enter opens it with the new-task sheet (from milestone 2 on), Esc clears the search and returns to the list. Until milestone 2 adds the workspace, "open" reveals the folder, as the row does today.
  - **From any screen: "打开仓库 / Open repository"** (Cmd/Ctrl+O, also in the app menu): an overlay in the style of the session switcher (design 5.7c), listing recently opened repositories first; typing filters with the same ranking; Enter opens the repository's workspace, Cmd/Ctrl+Enter opens it with the new-task sheet. On macOS Cmd+O works even while a terminal has focus (Cmd never reaches the terminal); on Windows Ctrl+O inside a terminal belongs to the terminal program, so there it is reached from the menu or after leaving the terminal. Hidden repositories are not listed; a repository whose folder is gone is listed dimmed with "找不到这个文件夹 / This folder is gone". Milestone 2 (A2-16); the palette of milestone 5 (Cmd/Ctrl+K) keeps repositories too.

### 4.4a Local folder name and remote name

Owner requirement (2026-11-16): the folder on this computer may be named differently from the remote repository (remote `AAA`, local `BBB`), and the app manages that.

- **Two names.** Wherever a repository is named, the local folder name comes first; when the remote name differs it follows as secondary text ("remote: AAA").
- **Clone** under any folder name.
- **Rename local folder** (repository menu; optionally another parent folder on the same disk). One transaction: (1) stop that repository's AI sessions and the programs they started, terminals, language servers and file watchers, after a dialog that lists them (Windows cannot rename a folder while a handle is open); (2) rename on disk with one system call; (3) update the record's path, run `git worktree repair` for the app's task workspaces, update task records and scan folders; (4) resume watching. A journal makes it all-or-nothing: if any step fails, the folder gets its old name back and every record is restored, also after a crash. Sessions are not restarted by themselves; they show as stopped with "Resume".
- **What does not change:** the remote URL, branches, the account binding, the author identity, task workspaces' own folders. Two honest limits are shown in the dialog when they apply: a conditional rule in the user's own git config that names the old path is not rewritten; Claude Code files conversations by folder path, so a session that ran in the main workspace may not be found where the tool looks after the rename (not verified: `manual-checks.md` A6). Resume then follows the rule of 4.16 ("Switch and resume", the paragraph on Resume): it looks for the conversation under every session ID of the task, and when it is found nowhere, no new session starts unless the owner chooses that.
- **Not in 1.0:** a move to another disk (it is a copy plus a delete and cannot be undone as one step).
- A folder renamed outside the app is offered for re-attachment when a scan finds a repository that matches exactly one missing record.

### 4.5 Bind a repository to an account

1. Provider from the remote host.
2. Suggested account: AWS as today; GitHub / Bitbucket: the only account for that provider, or a probe (`git ls-remote` with each account's key, outside the repository, with `-c` overrides as today's connection tests do).
3. The popover shows the one line that will be written. Backup first, undo in the toast.

### 4.6 Git work

Everything the owner listed is in 1.0: commit composer (stage and unstage files, hunks and lines; amend; discard), merge with the conflict screen, plain rebase, cherry-pick, revert, reset (soft / mixed / hard with confirmation), tags, remotes, fetch, pull (merge or rebase), push, force push with lease, compare branches, blame, file history, branches, stash, history. Interactive rebase is not in 1.0 (the owner marked it optional).

Rules that the review added:

- **Automatic reads never interfere and never run a repository's own programs** [R8]. Status for project rows, tree colours and change counts runs continuously on every repository and every task folder. Every automatic read is `git --no-optional-locks -c core.fsmonitor=false ...` with `--no-ext-diff --no-textconv`. Without this, plain `git status` takes `index.lock` and an AI's `git add` in the same folder fails at random, and it runs `core.fsmonitor` and diff drivers from the repository's own config, which would break the README's promise. Hooks, filters and helpers run only for actions the user started. Background fetch runs only for repositories the user bound or opened.
- **Refresh is driven by a file watcher** with debouncing, not by polling [R-S4] (section 5).
- **A clean folder before anything that can conflict** [R-S7]. Merge, rebase, cherry-pick, revert and pull require a clean tree, or stash automatically with the user's click and restore afterwards. Then "Abort puts everything back" is true.
- **Discard is recoverable for 14 days.** Discarded changes are kept as git objects (`git stash create` / `hash-object -w`) under `refs/codepass/discarded/*` and pruned after 14 days. The app's backup store is the wrong tool: it would copy `.env` secrets out of the repository.
- **Force push** is `--force-with-lease=<ref>:<sha>` plus `--force-if-includes`, where `<sha>` is the remote commit the user last pulled or pushed (stored per branch), not the remote-tracking ref. The app's own background fetch keeps moving that ref, which voids a bare `--force-with-lease` [S25]. The confirm lists the commits that will be replaced, counted from that sha.
- **Conflict prediction** uses `git merge-tree --write-tree` (git 2.38+, inside the 2.39.1 minimum).
- **Signed commits:** when `commit.gpgsign` needs a PIN or passphrase prompt the app cannot show, the error is reported plainly with "Commit in the terminal".

### 4.7 Start an AI coding task

1. In the repository: AI tasks > New task (Cmd/Ctrl+T). One sheet: tool (Claude Code / Codex / OpenCode), where it runs (own workspace, recommended; or main workspace), and what the AI should do (the task description, which becomes the tool's first message). There is no name field (owner request, 2026-11-16): the AI names the task.
   **Naming.** On Start the app asks the chosen tool for a name in one headless call: a fixed naming prompt plus the description, no tools allowed, the tool's smallest model where one can be chosen, run in an empty folder under the app's data folder (so it never touches the repository or the repository's session history), with a 5-second budget. The answer is a short title in the description's language and an English kebab-case slug of at most 40 characters; both are validated, and a slug that breaks git's branch-name rules is rebuilt from its words. The workspace is created after the name is known (the sheet shows "正在起名… / Naming the task..."). Fallback, used quietly when no valid answer arrives in time or the tool is missing or not signed in: the title is the description's first line cut to 40 characters and the slug comes from its ASCII words, else `task-<n>`. The description goes only to the AI service that will run the task anyway. An empty description is allowed and skips the naming call: the task is "新任务 <n> / New task <n>" on branch `ai/new-task-<n>` (folder of the same name, kept), and its label (not its branch) is named the same way from the first message the person types, where the tool reports it (Claude Code and Codex hooks). Cancel stays active while naming (nothing is created yet), and the description is kept when creating the workspace fails. Renaming a task later changes its label only; the branch keeps its name, because the AI may already use it.
2. **Own workspace:** `git worktree add -b ai/<slug> ~/.codepass/worktrees/<repo>/<slug> <current branch>` (`<repo>` is the local folder name; a second repository with the same folder name gets `<repo>-2`). The folder is outside the repository and has no space in its path. Then, in order: copy ignored files listed in `.worktreeinclude` (the convention Claude Code uses [S11]); `git submodule update --init` when the repository has submodules; the repository's setup command, if set. `~/.codepass/worktrees` is excluded from Time Machine.
3. **Launch without building shell text:** `$SHELL -l -i -c '"$@"' codepass <tool> <args...>` in a PTY in the task folder. The description travels as the first message in argument form only: after `--` for Claude Code and Codex (their positional prompt), as `--prompt=<text>` for OpenCode, so a description that starts with a dash is never read as a flag (a description of `--dangerously-skip-permissions` is just text). The user's login shell still applies (PATH, version managers, a `claude` shell function), but no task value is spliced into a command string [R11]. Windows: the tool's executable is started directly under ConPTY with an argument list. Two consequences found in milestone 0: a shell function puts its own flags **before** the app's, so anything the app passes must not depend on flag order (4.8); and a tool started through a function runs in its own process group, so stopping a task kills by terminal session, not by group (4.8). OpenCode is always started with `--standalone`: without it OpenCode hands work to a shared background service that outlives the task, ignores the task's environment and cannot be stopped with it [M0 S0-03].
4. **Environment, built explicitly:** `TERM=xterm-256color`, `COLORTERM=truecolor`, a UTF-8 `LANG` (apps launched from Finder get none, which breaks Chinese input in zsh), the proxy variables; `AWSSSOHUB_*` and `WAILS_*` variables removed.
5. **Installed on first use:** if the tool is missing, the terminal area shows the exact brew (or winget / npm) command and "Install and start". Detection uses only `--version` (`opencode auth list` starts OpenCode's background service).
6. Sign-in to each AI tool is the tool's own. The app stores no AI credentials. With several accounts per tool (4.16) this stays true: the launcher only adds the chosen account's folder variable (`CLAUDE_CONFIG_DIR` or `CODEX_HOME`) to the environment in point 4; the tool's own account adds nothing.
7. **Helper calls run in print mode, under the same account.** The naming call (point 1) and the usage reading (4.16) are short headless calls. They run with the same account variable as the task. Claude Code documents that in print mode an `ANTHROPIC_API_KEY` in the environment "is always used when present" [U]: with such a key in the shell profile, these calls would be billed to the API key instead of the plan. The start-up probe (4.16) detects that; the helper calls then do not run, naming uses its quiet fallback, the meter reads "unknown", and the UI says why.

**What a task can do, stated in the UI:** it runs as the user. It can read `~/.aws/config` and use any signed-in AWS profile, read every passphrase-less SSH key, and run any command. The app never passes a flag that skips a tool's permission prompts, and shows each tool's approval or sandbox mode in the task header. "Start tasks with this repository's AWS account selected" is a convenience setting, off by default, and is described as a convenience, not a safeguard.

Why the app creates the worktree instead of `claude --worktree`: that flag places worktrees under `<repo>/.claude/worktrees/`, names the branch itself and cannot take a branch as the base [S11]. Creating the worktree with git works the same for all three tools.

### 4.8 Several tasks at once

| State | Meaning | Signal |
|---|---|---|
| starting | Process spawned, no output yet | spawn |
| running | The AI is working | Claude Code hook `UserPromptSubmit`; OpenCode `session.execution.started`; Codex hook `UserPromptSubmit` if hooks can be supplied per launch; fallback below |
| waiting | Needs an answer or permission, or finished its turn | Claude Code hooks `Notification`, `PermissionRequest` and `Stop` [S13]; OpenCode `session.execution.succeeded / failed / interrupted` and `permission.asked` [S26] (`session.idle` is unconfirmed); Codex hooks `PermissionRequest` and `Stop`, or `notify` (turn complete only) |
| running (no signal) | The tool is alive but sends no events | the fallback below; keep-awake then relies on its hard ceiling |
| closed, still running | The session's terminal is closed or the tool exited, but programs it started are alive | the process tracker (4.8b) |
| done / failed | Process exited 0 / non-zero | exit code |
| stopped | The owner stopped it, or the app quit | app |

- **Hooks are injected per launch, never written into the user's own settings** [M0 S0-03].
  - Claude Code: a **session-only plugin** passed with `--plugin-dir` (a folder with `.claude-plugin/plugin.json` and `hooks/hooks.json`); a `--settings` file is only the fallback. Two `--settings` flags do not merge: the last one wins and the earlier ones are ignored entirely. The owner's `claude` shell function passes `--settings`, so a second one from the app would either lose the app's hooks or drop the owner's settings, depending on order. Plugin hooks add to whichever settings win. Only the events the state needs are subscribed (`SessionStart`, `UserPromptSubmit`, `Notification`, `PermissionRequest`, `Stop`, `SessionEnd`); tool-call hooks would add a process start to every tool call. **Added for the usage limit and the queue (4.16, 4.17):** `StopFailure` (a turn ended by an API error; its error kind `rate_limit` is the "limit reached" signal) and one `PreToolUse` hook whose matcher is the single tool name `AskUserQuestion` (the AI asks the person a question), so it runs for that tool only [U].
  - OpenCode 2.x: a plugin given through `OPENCODE_CONFIG_CONTENT` (no file written), with `--standalone`.
  - Codex: its lifecycle hooks if they can be supplied per launch without a trust prompt; otherwise `-c notify=[...]`, which reports only "turn complete" and replaces a user's own `notify`, so it is used only when the user has none; otherwise the fallback. Not verified (Codex was not installed): `manual-checks.md` A8.
  - The hook command is the app's own binary in a tiny "report" mode. The launcher sets `CODEPASS_TASK` in the task's environment; report mode sends only **task ID, event, session ID and permission mode** to the running app over a local socket and exits at once. Hook payloads carry the prompt, tool input and the last assistant message; none of that is forwarded or logged. **Five more fields, each a value from a fixed list and never free text** (4.16, 4.17): the error kind of `StopFailure` (`rate_limit`, `overloaded`, `authentication_failed`, `billing_error`, `server_error`, `other`); the notification type (`permission_prompt`, `idle_prompt`, `elicitation_dialog`, `quota_auto_resume_fired`, `quota_auto_resume_stale`, `quota_auto_resume_disabled`, `other`); the end marker of a queued ticket (`done`, `question`, `none`), which report mode works out locally by comparing the last line of the last assistant message with two fixed words; the **conversation mark** of `StopFailure` (`conversation`, `none`), worked out locally in the same way: it is `conversation` when `error_details` or `last_assistant_message` contains one of four fixed strings, "Invalid `signature` in `thinking` block", "Invalid `data` in `redacted_thinking` block", "tool_use ids were found without tool_result" or "Prompt is too long", else `none`, and the text itself is never sent (it tells an error about the conversation from an error about the account; 4.16 decisions 22 and 34); and the source of `SessionStart` (`startup`, `resume`, `clear`, `compact`, `other`), which says whether a resume really resumed and when the tool compacted (4.16 decisions 24 and 26). A value outside a list is sent as `other`. The four strings are kept in one place with the tool version they were read from, and R8's checks confirm them.
  - Byte-identical before and after a task, **in the config folder of the account the task ran under** (4.16; the paths below are the tool's own account's): `~/.claude/settings.json`, `~/.codex/config.toml`, `~/.config/opencode/opencode.json`, `~/.config/opencode/service.json`. (`~/.claude.json` and the tools' data folders are rewritten by the tools themselves.)
- **Fallback when no hook exists** [R-S6]: "output is flowing" alone is wrong both ways (a tool that redraws while idle looks busy forever; a 20-minute silent build looks idle). The foreground process of the terminal is the tool itself (`TIOCGPGRP` on the PTY plus one `ps` call on macOS; a process-tree walk on Windows), and with the app's launch form the shell exits together with the tool. So the fallback can only tell **alive** from **exited**, never working from waiting. States are advisory; the terminal is one click away.
- **Resume uses session IDs the app creates** when the task is created: Claude Code `--session-id <uuid>` (from the tool's help text; not run yet), OpenCode `--session ses_<id>` (verified: created if absent); Codex's ID is captured from its hook or `notify`. The ID seen in hook events is the cross-check. Not `--continue`, which is ambiguous with several sessions in one folder. Claude Code keeps conversations per folder, so resume runs in the same folder.
- "Waiting" raises the sidebar count, the repository row badge and a system notification when the window is not focused.
- **Stopping a task or quitting kills every process in the task's terminal session**, not one process group: `zsh -i` gives every job its own group, and a tool started through a shell function is such a job, so a group kill would miss the tool and everything it starts [M0 review A, X1]. macOS: list processes, compare `getsid()` with the session leader's, signal every group they lead (SIGTERM, about 5 seconds, SIGKILL). Windows: a Job Object with `KILL_ON_JOB_CLOSE`, the child created inside it through `PROC_THREAD_ATTRIBUTE_JOB_LIST` so nothing can escape. Otherwise dev servers the AI started keep running [R-S5].

**Approval and sandbox vocabulary** (what the task header shows, read-only):

| Tool | Name | Values | Where the app reads it |
|---|---|---|---|
| Claude Code | permission mode | `manual`, `acceptEdits`, `auto`, `plan`, `dontAsk`, `bypassPermissions` | `permission_mode` in hook payloads (not in `SessionStart`); it can change during a session, so the header shows "as of the last event" |
| Codex | approval policy and sandbox mode | `on-request`, `never`; `read-only`, `workspace-write`, `danger-full-access` | hook payloads (documentation); `~/.codex/config.toml`, read-only |
| OpenCode | permission rules | `allow`, `ask`, `deny` per action | the merged config; no event carries it, so it may read "unknown" |

The owner's own Claude Code default is `bypassPermissions`. The header will show exactly that, and "waiting for permission" will be rare for the owner.
- A soft cap (default 6) asks before starting more: all sessions that run under one account share that account's usage allowance. (With several accounts per tool, 4.16, sessions under different accounts do not share one. The cap still counts all running tasks, because it is also about memory and attention.)
- **A session that hits a usage limit** is `waiting` with a limit note, not `failed`: 4.16 says how it is detected per tool and what the session bar offers.

### 4.8a Disk space of ended sessions

Owner requirement (2026-11-16). Every ended session shows the disk it holds in three parts: its **workspace** (with build output such as `node_modules` counted separately), the **app's own data** for it (`sessions/<task-id>/`: saved scrollback, hook plugin), and the **AI tool's own records for that exact session ID**:

| Tool | What belongs to one session | How it is removed |
|---|---|---|
| Claude Code | `~/.claude/projects/<folder slug>/<session-id>.jsonl`, the folder `<session-id>/` beside it (sub-agent records), `~/.claude/file-history/<session-id>/` | the app deletes exactly these paths. Layout not verified yet: `manual-checks.md` A6 |
| Codex | the file under `~/.codex/sessions/` whose name ends in `-<session-id>.jsonl` | the app deletes that file. Not verified: A8 |
| OpenCode | rows in OpenCode's own database | `opencode session delete <id>`; no file is touched and no size can be shown |

`~/.claude` and `~/.codex` in this table mean **the config folder of the account the session ran under** (4.16). After an account switch a session may have files in the shared, linked folders (one copy, reached from every account) or, without links, one copy per account; the preview lists every copy with its full path.

**What account switches add (4.16 decisions 22, 25 and 27).** A task can have more than one session ID: the tool's own copy and the app's repaired copy each get a new one. All of them are on the task record (`sessionIds`), and the table above applies to each ID. Two more kinds of file belong to the task:

| What | Where | How it is removed |
|---|---|---|
| A repaired conversation copy | the tool's conversation folder, under the copy's own session ID (the same three kinds of path as the table above) | the app deletes exactly the paths recorded on the switch |
| A handoff note | `sessions/<task-id>/handoff-<n>.md` in the app's data folder | with the app's own data for the task |

Both are session data like the rest: listed with their size and full path, deleted only for that task, only by the exact recorded path, and only after the preview. The preview names them in plain words ("修好的对话副本 / Repaired conversation copy", "交接笔记 / Handoff note"; design 5.7b). They are never in a backup, an export or the log. While a task is live they stay, also the older ones: an earlier copy or note is the record of what an earlier switch carried.

Rules: one session's data can be deleted, or several through "Free up space", and **always after a preview that lists every path and its size**. What is deleted is exactly the previewed list. A session that is starting, running, waiting, or has programs still running is never eligible. A path is listed only when its name contains the full stored session ID; no pattern or prefix match. A workspace with commits that exist in no other branch and no remote needs its own confirmation; those commits stay recoverable for 14 days (4.6), uncommitted files do not, and the dialog says so. Deletion does not go through the system trash (the point is to free space).

### 4.8b Programs a session leaves behind

Owner requirement (2026-11-16). AI tools start helpers, sub-agents, dev servers and shells. Some outlive the tool, or the tab that showed them.

- **Tracking.** While a session lives, the app snapshots the process table every 2 seconds (one listing per tick) and remembers every descendant by PID **and start time**. A process that leaves the terminal session (`setsid`) or is re-parented when its parent exits stays in the set.
- **Shown.** A session whose terminal is closed or whose tool exited while tracked processes are alive is listed as "closed, still running" with the process count, CPU and memory, in the repository's task list, on the AI tasks page and in the session switcher. The process names can be expanded.
- **Force stop.** SIGTERM to every tracked process and group, about 5 seconds, then SIGKILL; on Windows the job is terminated. Then one more snapshot confirms nothing is left.
- **Keep-awake** counts these processes as activity and is released when the last one is gone (the hard ceiling of 4.10 still applies).
- **Quit** (owner decision): with any live session or leftover program, quitting **always** shows a dialog with two choices: "Stop all and quit" (every session is stopped and force-stopped, then the app exits; branches, changes and conversations stay) or "Keep running in the background" (only the window closes; the app stays in the Dock or the tray and holds the sessions). There is no "don't ask again" and no setting. "In the background" cannot mean "after the app has exited": the sessions' terminals belong to the app's process, so they end when it ends. At logout, restart or shutdown there is no dialog: Wails 3.1.0 cannot hold a system logout open (no `NSTerminateLater`), so Codepass stops everything at once and quits; unsaved edits are kept in the recovery store (decided at the group-B merge, 2026-11-16).

### 4.9 Review, then merge, push or discard

The working-tree switcher points Changes and Code at the task's folder. Changes shows everything the task did against its base.

**Merge into `<base>`** cannot be a plain merge in the main workspace: git cannot merge into a branch that is not checked out, and the base may be checked out elsewhere [R9]. The flow:

1. Bring the base **into the task's own folder** (merge base into `ai/<slug>`). Conflicts are resolved there.
2. Fast-forward the base:
   - base checked out in the main workspace and clean: `git merge --ff-only ai/<slug>` there;
   - base not checked out anywhere: `git fetch . ai/<slug>:<base>`;
   - base checked out with local edits in the way: ask (stash first, or cancel).
3. Remove the workspace; delete the branch if ticked.

"Rebase onto main" for a branch that is not checked out runs in a temporary worktree and says so.

**Push branch** pushes `ai/<slug>` and offers "Create pull request". **Discard** removes the workspace and the branch; the commits stay recoverable for 14 days under `refs/codepass/discarded/*`. **Keep** does nothing.

### 4.10 Keep-awake

Owner's requirement: while AI coding is in progress, macOS must run `caffeinate -d`.

- **One manager with a counter.** It holds the assertion while at least one AI task anywhere in the app is active, and releases it when the last one stops being active.
- **macOS:** `/usr/bin/caffeinate -d -i -w <app pid>` [S14]. `-w` makes caffeinate exit by itself if the app dies. If caffeinate is killed from outside while tasks are active, the manager restarts it. While tasks run the app also holds an `NSProcessInfo` activity, so App Nap does not throttle a hidden window [R6].
- **Windows:** `SetThreadExecutionState(ES_CONTINUOUS | ES_SYSTEM_REQUIRED | ES_DISPLAY_REQUIRED)` on a dedicated locked OS thread [S15].
- **"Smart" means:** active = `running`, or `waiting` for less than the grace time (default 15 minutes; 5 / 15 / 60 / never release), or a closed session with programs still running (4.8b). A plain shell never counts. **Hard ceiling:** after N hours (default 8) with no hook event and no input from the owner, the hold is released even if a task still looks busy.
- **Visible:** the cup in the sidebar, the rail, the status bar and the AI tasks page. Settings: on / off; "keep the screen on too" (on, as asked; off uses `-i` only, so the screen can lock).
- **Honest limits, in the setting's text:** closing a MacBook's lid still sleeps it unless it is on power with an external display. While the screen is held on, the Mac does not lock by itself.
- **Test matrix:** app crash, force kill, sleep and wake, setting turned off mid-task, last task killed from outside, caffeinate killed from outside.

### 4.11 Author identity

Global on the Git settings page; project from the project menu (shows which repositories change and which keep their own); repository in its settings sheet. Every place that shows an identity shows where it comes from, as git reports it (3.4). The commit composer shows it next to the Commit button.

### 4.12 The editor (Code tab)

A full code browser and editor for the whole repository, with GoLand / WebStorm-like convenience. Not a diff viewer: the Changes view is a separate tab and one of several things the workspace does.

- **Browse:** project file tree with git status colours and letters, filter, "changed files only", compact single-child folders, context menu (new, rename, delete to trash, reveal, file history).
- **Edit:** multiple tabs with preview tabs, dirty markers and save; split view; multiple cursors; find and replace in file; go to line; breadcrumbs; change bars in the gutter with inline hunk diff and revert.
- **Navigate:** quick open (Cmd/Ctrl+P), go to symbol in file and outline, find in files with a results panel and replace all, palette (Cmd/Ctrl+K).
- **Code intelligence through language servers, Go (gopls) and TypeScript / JavaScript (the TypeScript 7 native server, `tsc --lsp -stdio`; typescript-language-server for projects on TypeScript 5 or 6):** go to definition, peek definition, find references, hover, autocomplete, diagnostics, workspace symbols. **PHP** gets the same (owner, 2026-11-16: the owner also works in PhpStorm); the server is chosen by E5-11's research before any code (Intelephense: Node runtime, free tier, not open source; Phpactor: open source, needs PHP 8.1+), installed on consent like the others, and a fresh task workspace without `vendor/` holds diagnostics back the way one without `node_modules` does. Other languages (Python, Java, SQL, YAML, JSON, Markdown, Dockerfile, shell): syntax highlighting, outline from the editor's parser, bracket matching, find.
- **Which TypeScript server** [M0 S0-06]: typescript-language-server 6.1.0 does not start in a TypeScript 7 workspace (TypeScript 7 has no `tsserver.js`), and `brew install typescript-language-server` pulls in TypeScript 7. Rule: the workspace has `node_modules/typescript` 7 or later, or none (a fresh task workspace) -> the native server from the `typescript` package, started as `tsc --lsp -stdio`. The workspace has TypeScript 5.x or 6.x -> typescript-language-server, which then uses the workspace's own `tsserver.js`. The native server reports diagnostics only when asked (pull), which the editor library does not do by itself; the app adds that.
- **"Installed automatically" hides toolchains:** gopls needs Go (`brew install gopls` does not bring it) [R-S10]. On macOS the native TypeScript server needs no Node; typescript-language-server brings Node with it. The app asks before installing a toolchain and shows its size. Windows: winget has Go and Node but not the servers, so the servers come from `go install` and `npm -g`; Node is needed only to run `npm`, and the app starts the native `tsc.exe` directly. The Windows path is not verified (`manual-checks.md` B12).
- **Resources** [R-S5] [M0 S0-06]: one server per language per open working folder. Measured: gopls 0.3 to 1.3 GB depending on cache state (1.1 GB on 2,003 files), typescript-language-server about 0.9 GB, the native TypeScript server about 0.3 GB. Idle servers, and servers whose page has not pulled for 10 minutes, shut down; at most **2** run at once by default (4 on machines with 32 GB or more). A fresh task workspace without `node_modules` holds TypeScript diagnostics back and shows a hint instead of a flood of errors.
- **Files:** highlighting off above 1 MB; editable up to 3 MB; read-only above that. Reason: the app's request guard caps bodies at 4 MB and refuses chunked bodies (`internal/app/runtimeguard.go`) [R-S2]. Image previews are size-capped `data:` URLs from a bound method, never a file-serving route.
- **Changed on disk** (an AI task rewrote the file): reload silently if there are no unsaved edits; otherwise compare / use disk / keep mine.
- **Unsaved edits are mirrored to Go** within a second, so a webview reload or crash restores them.

### 4.13 Pull requests

For GitHub and Bitbucket repositories whose account has a token: lists (mine / to review / all), create from a branch (title, description, reviewers, draft on GitHub), status (checks, approvals, mergeability), merge with the methods the repository allows, close or decline, approve or request changes. Line-by-line comments open in the browser. CodeCommit repositories show a link to the AWS console.

- Polling uses ETags and backs off: the list on screen every 2 minutes; everything else every 15 minutes at most [R-S9]. GitHub: per-repository lists and details use ETag, and an unchanged answer (304) costs no quota (measured); "waiting for my review" across repositories goes through search (30 a minute) or GraphQL, which have no ETag, so at most every 60 seconds while visible. Bitbucket: a personal token gets about 1,000 requests an hour with no "remaining" header, so the app counts its own calls; "mine" is one call per workspace, "to review" one per repository; 15 minutes off screen [M0 S0-07].
- Errors are mapped, never guessed: 401 = the token expired; GitHub 404 with a token = compare the granted and the accepted scope headers and name the missing scope, otherwise "not found or no access"; 403 with the SSO header = authorise the token for that organisation; 403 for an organisation that blocks classic tokens = open in browser; 403 / 429 limits wait for `retry-after`.
- Checks come from check runs **and** the combined commit status (two separate systems on GitHub); neither present means "no checks".
- All text from the provider is untrusted (section 5).

### 4.14 Update the app

The Wails updater's own checks are not enough [R1]: its GitHub provider fills in only a SHA-256 from the checksum file and never a signature, so a pinned public key rejects every update; its signature covers only the archive digest while the version comes from the unsigned tag, so someone who controls the GitHub account could re-publish an old signed archive as a new version; its macOS check lets an unsigned bundle through; release notes come unsigned from the API.

**Design:**

Milestone 0 built this against a fake releases API and a real signed `.app`: 17 scenarios, 118 assertions, every refusal leaving the installed app byte-identical [M0 S0-02]. Two controls confirmed the reason for the design: with Wails alone, an ad-hoc bundle replaced a Developer ID one, and Wails' own check runs **after** the swap.

1. **A signed manifest per platform.** Each release carries `manifest-<os>-<arch>.json` and its `.sig`. The manifest holds version, os, arch, file name, size, sha256 and the release notes. The signature is ed25519 over `"codepass-update-manifest-v1\n"` plus the manifest bytes. Unknown fields and a sha256 that is not 64 hex characters are refused. The app's own provider wraps the Wails GitHub provider **for discovery and streaming only**; version, file name, size, digest and notes come only from the verified manifest.
2. **Two pinned public keys**, the current one and the next one, so the key can be rotated. They live in the app's provider. Wails' `Config.PublicKey` is left empty (it holds one key and switches on Wails' own mode); the manifest's sha256 is handed to Wails as the digest it checks while streaming.
3. **Accept only if** the manifest version equals the tag, is newer than the running version, and is at least the highest version ever installed (stored; an unreadable store fails closed); os and arch match; the tag is plain semver. Release-candidate tags are `v1.0.0-rc.N` with a dot (`rc10` sorts below `rc2`, `rc.10` above `rc.2`).
4. **On the staged copy, before the helper starts, and again inside `Restart`:** the bundle ID and the bundle's version (Windows: the exe's file version) must equal the manifest's. On macOS also `/usr/bin/codesign --verify --deep --strict -R=` with the **full designated requirement**:
   `anchor apple generic and identifier "com.rudian.awsssohub" and certificate 1[field.1.2.840.113635.100.6.2.6] exists and certificate leaf[field.1.2.840.113635.100.6.1.13] exists and certificate leaf[subject.OU] = "XC34P8TNZ6"`
   The shorter string of revision 2 (anchor, identifier, Team ID) is also satisfied by the team's Apple Development and Apple Distribution certificates, and both are in the owner's keychain. `spctl -a -t exec` runs as a second check and is never the only one (it answers "accepted" when Gatekeeper is off). Unsigned, ad-hoc and development-signed bundles fail.
5. **Release notes** are shown only from the signed manifest, as plain text.
6. **`updater.HandleHelperMode()` is the first line of `main()`.** Today `main.go` opens paths, the log, settings and backups before `application.New`, where Wails runs the helper.
7. **Releases are built on the owner's Mac**: build; re-sign with `--force --options runtime --timestamp`, the executable first and then the bundle; notarize a zip and stop on any status other than Accepted (the notary log is kept); staple; build the release archive **from the stapled app** with `ditto -c -k --keepParent --norsrc --noextattr --noqtn --noacl` (a plain `ditto` zip carries `._*` entries that break the signature after extraction); extract it and run the requirement check; sign the manifest; upload with `gh`. The ed25519 private key and the Developer ID certificate never go to GitHub Actions. The Windows exe is cross-built on the Mac (as today) or on a Windows VM, and its manifest is signed on the Mac.
8. **A test channel:** a private throwaway repository and a test key. A private repository needs a token: a fine-grained, read-only token for that one repository, entered once and kept in the Keychain, never compiled in (the public production channel needs none). Every milestone build reaches the owner through it. The whole path (rc.1 to rc.2) is tested on a clean macOS VM and a Windows 11 VM at the end of milestone 1, and again before 1.0.0.
9. **Failure behaviour.** Every refusal removes the staging folder and leaves the installed app untouched. A fresh updater per check with the app's own timer (Wails' loop never retries a refused update). The download client has no overall timeout (Wails' default cuts off after 30 seconds). The swap itself is Wails': copy to `Codepass.app.bak`, remove, rename. If the helper dies in that millisecond window there is no app until `.bak` is renamed back by hand; the release document says how.
10. **Windows** has no Authenticode check while the build is unsigned; trust rests on the manifest signature and digest. The Windows gate (file version equals manifest version) is written and run on a VM in milestone 1.

UI: a quiet banner and a dot on Settings, never a modal on launch; download, verify, replace, reopen with progress; with AI tasks running the choice is "Reopen now" or "Reopen when no task is running" (an interactive session never exits by itself). If the app cannot replace itself (read-only location, no write access to Applications) it says so **before** downloading, names the folder, and says how to fix it.

### 4.15 System notifications

Owner's requirement (2026-11-16): when an AWS sign-in expires, macOS shows a notification that names the account, with a button that starts the sign-in again.

Research: `docs/research/notifications.md` [N]. Short result: the button is possible on macOS and on Windows. Nothing was run yet; the manual checks M1 to M10 and W1 to W4 in that file are acceptance for the task that builds this.

**One service for the whole app.** `internal/notify` is the only code that talks to the operating system about notifications. Every feature sends through it.

| Kind | Sent by | Built |
|---|---|---|
| Sign-in heads-up | accounts engine (the existing reminder) | now |
| Sign-in ended | accounts engine | now |
| "Codepass is still running" | window close with tasks running (design 4.4) | milestone 2 (A2-08) |
| AI task is waiting | task lifecycle (4.7, design 5.6) | milestone 2 (A2-11) |
| Ticket queue: needs you, paused for the limit (with "Continue on Personal" when another account has room and automatic switching is off), continued on another account, queue finished | the queue (4.17) | milestone 8; the same service, texts in design 6 |
| An AI account needs the owner's attention | AI accounts (4.16 decision 34) | milestone 8; text in design 6 |

- **Why one service.** The framework has a single response callback for the whole app, and button texts are registered once per kind [N 1]. One owner avoids two features overwriting each other.
- **What a sender gives:** a kind, a stable ID, the title and body already in the person's language, and a small data map with identifiers only. The service adds the buttons for that kind, checks the setting and the permission, sends, and later routes the click back to the feature.
- **The service wraps the framework's notification service and starts it itself.** A start failure (a bare binary has no bundle identifier) is caught and logged; the app then runs with in-app toasts only. The framework's service is never registered directly, because a service that fails to start stops the app [N 1, limit 6].
- **Started at app start**, so a click on an old notification reaches the app even when that click is what launched it.
- **Button texts follow the language.** They are registered at start and again when the language changes.
- **Format rules for every kind:** title and body only; one button at most; no sound for a heads-up or for "still running", the default sound for "ended" and "task waiting".

**Sign-in: when a notification is sent.** "Sign-in" here means one portal sign-in (one `sso-session`), not one profile. A portal with five accounts gets one notification, not five.

1. **Heads-up, before the end.** The existing reminder: setting `reminders.expiryNotice` (on) and `reminders.leadMinutes` (15, 30 or 60). The engine already emits `session:reminder` once per sign-in when the time left drops below the lead time. The service now turns that event into a system notification. Nothing listens to the event today, so until now this setting did nothing visible.
2. **Ended.** Sent once when the app sees that the sign-in is over: the session state becomes `expired`. That is: AWS showed the end and the end has passed, or AWS refused a renewal and the last token ran out, or the token ran out and the app cannot renew it. New setting, on by default (see "Settings" below).

**What it says.** One line names the portal, one line names who and what.

- Title: the kind and the portal's display name. "登录已过期：acme / Sign-in expired: acme".
- Body, first part: the Identity Center user when the app knows it, then the AWS accounts this sign-in covers. One account: its name. Two: both names. Three or more: the first name and a count, "acme-prod 等 3 个账户 / acme-prod and 2 more". Accounts are the distinct account names of the portal's profiles, in the order of the Accounts page.
- Body, second part: what to do, or how long is left. Full copy for both languages is in the design (ui-design 6).
- The Identity Center user is left out when the app does not know it. It is never guessed from a profile name.
- The time in a heads-up always says "about" ("大约 15 分钟后 / in about 15 minutes"), because the end is an estimate until AWS shows it.

**Text that never appears** in a title, a body or the data map: tokens, the device code, any URL or start-URL host, account IDs, role names, email addresses other than the Identity Center user name, file paths. Reason: a notification is shown on the lock screen, kept by the system after the app quits, and on Windows the whole notification is stored inside the button's argument [N 3.2]. The data map holds the kind, the portal name and the sign-in time it was sent for, nothing else.

**The button "重新登录 / Sign in again".** Both sign-in kinds have it, and it is the only button.

- It starts that portal's sign-in exactly like the button on the pass: the isolated sign-in window (a separate browser instance with a throw-away profile), the same progress on the Accounts page, the same result handling.
- It does not bring the Codepass window forward. The browser window is what the person needs next.
- A sign-in for that portal is already running: nothing new starts; its browser window is brought forward.
- The portal is already signed in again (a stale notification): no new sign-in. The Accounts page opens at that portal with the toast "已经登录了 / Already signed in". "Signed in again" means: the portal's current sign-in is newer than the one the notification was sent for. So the heads-up button, pressed while that same sign-in is still running, does start a sign-in: signing in again before the end is the point (see the last paragraph).
- The portal no longer exists: the Accounts page opens, with nothing selected.
- The app was not running: the click starts Codepass, and the sign-in starts as soon as accounts are loaded. The startup checks (old app running, tools gate) come first; if one of them blocks, the click is dropped and the window shows the gate.
- Sandbox mode never starts a real browser, as today.

**A click on the notification itself** shows the Codepass window (also when it was hidden or the app was closed) and opens the Accounts page scrolled to that portal, with the pass highlighted once.

**When not to notify.**

- **Already signed in again.** A successful sign-in removes that portal's delivered notification (macOS) and resets its record. No "ended" notice is sent for a sign-in that was replaced before it ended.
- **Signed out by the person.** Signing out in the app is not "expired". Nothing is sent, and a delivered heads-up is removed.
- **Duplicates.** One heads-up and one "ended" per sign-in, keyed by the portal and the sign-in time. Both use the same notification ID per portal, so on macOS "ended" replaces the heads-up instead of adding a second entry. The last key sent per portal is stored (see "Storage"), so a restart inside the lead time does not send the heads-up twice.
- **A sign-in that ended while the app was closed.** No notification at start, however long ago it ended. Reason: the person has just opened the app and the passes show the state at once; a banner for something already on screen is noise, and on Monday morning it would be one banner per portal. At start, every portal that is already expired is marked as "told".
- **A sign-in that ended while the computer slept.** The app was running, so one "ended" notice is sent when the app sees it after waking. Reason: the person has not been told, and the window may be hidden.
- **A heads-up right after signing in.** Skipped when the sign-in itself happened inside the lead time (a session shorter than the lead time). The person signed in a moment ago.
- **A renewal that fails for a network reason is not "expired".** Offline, a proxy error or a timeout leaves the state as it is; the app tries again on the next tick and shows the existing inline error. "Ended" is sent only when AWS says so or a known end has passed. An estimated end that passes while the token still works is also not "expired".
- **Several portals at once.** Each gets its own notification. They are different sign-ins and each needs its own button.
- **Window in front.** Sign-in notifications are sent even when Codepass is the front app (the person may be in a workspace, far from the Accounts page). The "AI task is waiting" kind keeps its own rule: only when the window is not focused.

**Automatic renewal is unchanged.** "Renew sign-in automatically" (`reminders.autoRenew`) keeps renewing the one-hour access token inside the session and sends no notification, whether it works or not.

**Permission on macOS.**

- macOS asks the person once, the first time the app requests it [N 2.4]. Codepass requests it **the first time a portal is signed in while the window is in front**, not at first launch. Reason: at that moment the question has an obvious reason, and a person who uses no AWS account is not asked until another kind needs it (the first AI task start; the first "still running" notice).
- The request runs in the background and never blocks the window. The app stores that it asked.
- **Allowed:** nothing is shown.
- **Not asked yet:** Settings shows one hint row with "允许通知 / Allow notifications".
- **Turned off** (asked before, and macOS says no): Settings shows a hint with "打开系统设置 / Open System Settings". The app never asks again by itself and shows no banner about it elsewhere. While the Codepass window is visible, the same text and button appear as an in-app toast instead. While it is hidden, nothing appears; the passes and the sidebar badge still show the state.
- The app cannot see Focus modes or a banner style of "None". So no screen may depend on a notification having been seen.

**Windows.** The same service, kinds, texts and button. Differences, all from the framework [N 3]:

- No permission question, and the app cannot tell whether notifications are off. Settings shows one help line instead of a state.
- A shown notification cannot be replaced or removed. A heads-up may still be in the notification centre after "ended" arrives, and both may remain after signing in. The stale-click rules above cover it.
- Whether the portable, unsigned exe can show notifications without a Start-menu shortcut is not confirmed (manual check W1). If it cannot, notifications on Windows need the installer, and the portable exe falls back to in-app toasts.

**Sandbox mode and tests use a fake.** `AWSSSOHUB_SANDBOX` always selects it; so do unit and end-to-end tests. The fake records each notification, can report any permission state, and has a "respond" call that presses the button or the body, so the routing code is the same as for a real click. In sandbox mode it also shows each notification as an in-app toast. Reason beyond testability: the development build has the same bundle identifier as the installed app, so a real notification from it would use, and could change, the installed app's notification permission [N 2.9]. Real notifications in a development build are opt-in with one environment variable, for the manual checks only.

**Settings.** `settings.json` takes only fields whose zero value is safe (3.1), because one run of the old app drops unknown fields. The new switch is therefore stored as "off": `reminders.endedNoticeOff` (absent or false = notify). The existing `reminders.expiryNotice` and `reminders.leadMinutes` keep their meaning.

**Storage.** `notifications.json` in the data folder (`schemaVersion`; "asked for permission: yes/no"; per portal, the key of the last heads-up and the last "ended" notice sent). It holds no secret and no text from a notification. It is not part of an export.

**The session length is the administrator's.** An IAM Identity Center administrator sets how long a session lasts, and the app cannot change or extend it; that is why the button signs in again instead of "extending", and why signing in again from the heads-up, before the end, is the way to keep working without a gap.

### 4.16 AI accounts and the usage meter

Owner's requirements (2026-11-16): several accounts per AI tool; when one account's usage is used up, the running session continues on another account with its conversation; and the app shows the usage, so Claude Desktop need not be opened for it. The owner also decided that Codex gets the same as Claude Code. Added the same day: the owner switches between the accounts back and forth, and chooses the account freely per task ("account A in repository A; for a task in repository B, account A or account B").

Three more statements of the owner, the same day, in the owner's words:

- **S1.** "随时来回切换账号，还有一种情况会常用到，就是比如账号A的weekly usage到了上限，我就切换成账号B继续做任务，但是context当中不能够丢失必须能够继续做任务". Besides switching back and forth at any time, one case will be common: account A reaches its weekly usage limit, the owner switches to account B and carries on with the task; nothing in the context may be lost, and the task must be able to continue. Answered by "Switching keeps the conversation" (decisions 22 to 28).
- **S2.** "需要考虑到使用多账号担心被claude或gpt封号，需要符合官方的使用条规，但是目前这个项目是拿来自己使用而不是转售". Using several accounts must not get them banned by Claude or GPT; it must follow the official terms of use; the project is for the owner's own use, not for resale. Answered by "Several accounts within the terms" (decisions 29 to 36).
- **S3.** "我同时处理着TASKA和TASKB，TASKA 使用账号A，TASKB 使用账号B，这个应该是被允许的吧". Task A on account A and task B on account B at the same time: that should be allowed. Answered by decision 29: yes for Claude Code (documented); for Codex nothing forbids it and nothing describes it.

Research: `docs/research/claude-usage-accounts.md` [U]. **Nothing in this section has been run.** The checks R1-M1, R2-M1 to M4, R5-M1 to M5, R8-M1 to M10 and R9-M1 to M5 in that file are acceptance for the tasks that build it; Codex was not installed when it was written. The numbered choices below are the planner's, written so the owner can correct them.

**Scope (planner's decision 1).** Claude Code and Codex get accounts, the meter, the queue's usage check and switch-and-resume. OpenCode gets none of these in 1.0: it shows no usage and no limit signal [U R6]. Its tasks and tickets still run.

**An account is a label and a config folder (decision 2).**

- Each tool already has one variable that moves its whole state: `CLAUDE_CONFIG_DIR` for Claude Code (documented by Anthropic for "multiple accounts side by side") and `CODEX_HOME` for Codex (documented as the state folder; its use for accounts follows from the code, not from a docs page) [U R2, R5b].
- **The tool's own account** is the tool's own default folder (`~/.claude`, `~/.codex`) with the variable **unset**. The app never moves it and never sets the variable to that path. **One rule for a tool's files, used everywhere in this plan, for the tool's own folder and for every extra account's folder:** the app never changes a file of a tool. With the owner's consent, given in the add-account sheet, it adds new files in two cases only: the repaired copy (decision 27) and the add-only copy at the end of this decision. It deletes a tool's file only through "Free up space" (4.8a): the exact paths of an ended task's own session IDs and recorded copies, after the preview. (Why the variable stays unset: Claude Code picks a different Keychain entry when it is set, even to the default path [U R2].)
- **An extra account** is a folder `~/.codepass/ai-accounts/<tool>/<slug>/` (mode 0700, no space in the path, so the owner can use it in any terminal with the same variable). `<slug>` comes from the label.
- **Sign-in is the tool's own.** "Add account" creates the folder and opens an app terminal that runs the tool's login command with the variable set (`claude auth login`, `codex login`). Codepass never sees, reads or stores a token, and never touches a tool's Keychain entry. Removing an account runs the tool's logout in that folder first, then deletes the folder after a preview of its paths.
- **Who the account is** comes from the tool, on demand: `claude auth status` (JSON; exit 0 when signed in) and `codex login status`. The app shows "signed in / not signed in" and the sign-in kind. It stores neither an email address nor a plan name; the owner's label is the name.
- **Shared parts, with the owner's consent once, in the add-account sheet.** Each extra folder gets links to the tool's own folder for:
  - Claude Code: `projects/` and `file-history/` (needed so another account can resume a conversation), `plans/` and `tasks/` (a resumed plan-mode session reads its plan and its to-do list from there [U R8]; not verified, R8-M7), and `settings.json`, all always linked: `settings.json` holds the owner's permission mode, and it must be the same under every Claude Code account, for the same reason as Codex's `config.toml` below. By default also `CLAUDE.md`, `agents/`, `skills/`, `commands/`, `plugins/` (so the tool behaves the same under every account); each of these can be unticked.
  - Codex: `sessions/` (needed for resume) and `config.toml`, both always linked: `config.toml` holds the owner's approval policy and sandbox mode, and those must be the same under every Codex account, so a switch never changes what the AI may do. By default also `AGENTS.md`, the prompts folder and the other shared parts found at the first install (hooks file, skills); each of these can be unticked. The exact names are not verified [U R5b].
  - **Never shared:** the account file and the sign-in (`.claude.json`, `.credentials.json`; `auth.json`). So user-scope MCP servers and folder trust are set up once per account; the sheet says so.
  - Links are plain symbolic links (Windows: directory junctions; files copied once instead, with a note). The app creates them, lists them in the account's details, and removes only links it created.
- **If linked resume does not work** (R2-M2 step 6; R5-M4): no links for the session folders; at switch time the app copies that one session's files into the other account's folder (the same paths 4.8a lists; only paths that contain the full session ID, listed before copying). The copy is add-only. When the folder it goes to already holds an older file of the same session (A→B→A), that file is not replaced: the newer conversation is written there as a new file under a new session ID, by the same writer as the repaired copy (decision 27), and that one is resumed. A→B→A therefore resumes with B's turns, and no existing file of a tool is changed. This is the second case of the rule above: without links, a switch from an extra account to the tool's own account adds such a copy inside the tool's own folder, and a switch the other way adds one inside the extra account's folder. Every path of a copy is recorded on the task before it is written ("Switch and resume"), has mode 0600 (folders 0700), and is listed by "Free up space" (4.8a).

**Choosing the account (decisions 13 to 21).** The owner's request (2026-11-16): switch between several Claude or Codex accounts back and forth; use account A in repository A, and for a task in repository B choose account A or account B freely. These nine choices are the planner's, for the owner to correct. "Default" means the account that is pre-selected for a new task. The account that lives in the tool's own folder is called "the tool's own account".

- **13. Free choice per task.** The New task sheet has an account row for the chosen tool. It pre-selects the default of decision 14, and the owner can pick any account of that tool, for any repository. The choice belongs to that one task; it does not change a default. The task's account is shown on the session bar, on the task's row on the AI tasks page and in the session switcher, for both tools.
- **14. Defaults in three levels per tool, like the author identity (3.4).** Precedence: **repository > project > app**. The most specific one that is set wins.
  - **App** = the first account in the owner's order on the AI accounts page. Drag to change it.
  - **Project** = "这个项目默认用 / This project uses": follow the app default, or one fixed account. Set from the project section's menu.
  - **Repository** = "这个仓库默认用 / This repository uses": follow the project (the app default for a repository in no project), or one fixed account. Set in the Repository settings sheet.
  - Each level is set separately for Claude Code and for Codex. Stored in `projects.json` as account IDs (3.1, 3.2); the app level is the order in `ai-accounts.json`.
  - A default writes nothing into a repository, a tool's folder or the shell. It is only what the New task sheet and the queue start from.
  - An account's details on the AI accounts page list where it is the default.
- **15. The default has no room.** The New task sheet still lists that account as used up, with its reset time. It pre-selects the first account in the owner's order that has room and says so in one line: "「工作」用完了，周一 00:00 重置，这次先用「个人」。 / Work is used up until Mon 00:00, so this task uses Personal." The owner may still pick the used-up account; the session then waits for the reset, or is switched automatically when the owner has turned automatic switching on for that tool (decision 33). Details:
  - **Room** here is the first rule below ("One rule for room"): every window below 100%, with no reserve.
  - An account that is not signed in is passed over the same way as a used-up one, and the line says "没有登录 / is not signed in" instead. An account that needs the owner's attention (decision 34) is different: nothing is pre-selected in its place. The default stays shown with "需要你处理 / needs your attention", it cannot be started on, and the owner either deals with it or picks another account by hand.
  - No account has room: the default stays selected, and the line gives its own reset and the earliest reset; nothing moves the task at the earlier one. "所有账号都用完了。「工作」周一 00:00 重置；最早重置的是「个人」，15:45。 / Every account is used up. Work resets Mon 00:00; the first to reset is Personal, at 15:45."
  - For a Codex task, this line and the line shown when the owner picks a used-up account add: "Codex 不会自己继续，到时要你点继续。 / Codex does not continue by itself; continue it then."
  - The repository's default is not changed by any of this. The next task starts from it again.
- **One rule for room** (used by decision 15, Auto-switch below, design 5.6 and 5.15).
  - Room for a session the owner started (the New task sheet, the switch menu, "Continue on B", and auto-switch of that session): every window below 100%; an unknown reading counts as room. The queue's reserve (4.17) is not subtracted: it exists to keep usage for exactly this kind of work.
  - Room for a queued ticket (the 4.17 check, and auto-switch of a ticket's session): below 100% minus the reserve.
- **16. Switch any time, in both directions, as often as wanted.** The account chip on the session bar is a menu, "换账号 / Switch account". It lists the tool's other accounts with their latest reading. Choosing one runs "Switch and resume" below: only between turns, the same confirm, and the same three layers that carry the conversation (decision 22). Going back from B to A is the same action. The limit banner's "Continue on B" stays, as a shortcut into the same flow. The task's row on the AI tasks page offers the same menu. A task that is stopped and can be resumed offers it too; it then resumes on the chosen account.
- **17. An automatic switch is never undone automatically.** After an automatic switch, once a reading shows that the account the session left has room again, the session bar shows one quiet hint: "「工作」又有用量了 · 切回「工作」 / Work has usage again · Switch back to Work". Nothing happens unless the owner clicks it; the click is the switch of decision 16, with its confirm. The hint comes from readings the app takes anyway (no extra reading is made for it), can be dismissed, and is shown once per reset.
- **18. Tickets.** A ticket's account is "跟随仓库 / Follow the repository" (the default) or one fixed account. The queue resolves it when the ticket starts, not when it was queued, then applies its check and auto-switch (4.17).
- **19. Removing an account that is a default somewhere.** The remove preview lists the projects and repositories that use it as their default; they fall back to the level above. Tickets fixed to it go back to "Follow the repository". An account cannot be removed while a session runs under it: the preview names those sessions and asks the owner to stop or switch them first. A stopped or ended task whose account was removed shows "已移除的账号 / Removed account"; Resume first asks for an account through the switch menu and its confirm. **Without links** (decision 2's last point), the newest record of a conversation lives in the folder of the account the session last ran on, so the remove preview lists the tasks whose latest conversation is only in this folder: "会先把对话复制到「工作」 / Their conversations are copied to Work first". Before deleting, the app copies each of them, add-only, into the tool's own account (decision 2), and those tasks resume there. If a copy fails, the folder is not deleted. A hidden repository keeps its setting. The project sheet's lists, the move confirm and the "Default for" line leave hidden repositories out; the remove preview counts them in one line: "另有 1 个已隐藏的仓库也默认用「个人」 / 1 hidden repository also uses Personal by default".
- **20. When the shell decides** (the probe below found `CLAUDE_CONFIG_DIR` or `CODEX_HOME`): the defaults for that tool are shown and can still be edited, but they are not applied. The account row in the New task sheet and the switch menu are disabled and say why, in the words of the "The shell decides" row (design 5.16). A task started while the shell decides records no account; its chip and its rows read "由终端环境决定 / set by your shell".
- **21. Both tools, and nothing else changes.** Claude Code and Codex work the same way here. OpenCode has no accounts, so it has no account row, no chip and no default. Approval and sandbox settings are the same under every account of a tool (`settings.json` and `config.toml` are always linked; the shared parts above). MCP servers the owner added, and folder trust, are kept per account (design 5.16 step 2), so after a switch the tool may ask to trust the folder again. A switch keeps the task's folder, branch, terminal and conversation (decisions 22 and 25).

Example, with the accounts "工作 / Work" (first in the order) and "个人 / Personal":

| Where | Set to | A new Claude Code task starts on |
|---|---|---|
| App | Work is first in the order | - |
| Project acme | follows the app default | - |
| Repository acme_api (in acme) | follows the project | Work |
| Repository acme_agent_portal (in acme) | Personal | Personal |
| Project globex | Personal | - |
| Repository globex_web (in globex) | follows the project | Personal; the owner may pick Work for one task in the sheet |

**The launcher and the shell (decision 3).** The account's variable goes into the task's explicit environment (4.7 point 4). The owner's login shell runs after that and could override it. So at app start a probe runs once in the login shell and reports only **which of these names are set, never a value**: `CLAUDE_CONFIG_DIR`, `CODEX_HOME`, `ANTHROPIC_API_KEY`, `ANTHROPIC_AUTH_TOKEN`, `CLAUDE_CODE_OAUTH_TOKEN`, `OPENAI_API_KEY`, `CODEX_API_KEY`. Results:

| Found in the shell | What the app does and says |
|---|---|
| `CLAUDE_CONFIG_DIR` or `CODEX_HOME` | The shell decides the account, not Codepass. Accounts for that tool are shown but cannot be chosen, and its defaults are shown but not applied (decision 20); one line names the variable and says to remove it from the shell profile. |
| An API key or token variable | The tool would use it instead of the plan sign-in. The meter for that tool reads "unknown"; helper calls in print mode do not run (4.7 point 7); a warn line names the variable. Tasks still start: which credential a tool uses is the owner's choice. |
| None | Nothing is shown. |

**Windows:** there is no login shell (4.7 point 3). The probe reads the names in the app's own environment (user and system variables). `CLAUDE_CONFIG_DIR` or `CODEX_HOME` found there is handled as in decision 20, with the sentence "你的 Windows 环境变量设置了 CLAUDE_CONFIG_DIR，Claude Code 用哪个账号由它决定，Codepass 选不了。 / A Windows environment variable sets CLAUDE_CONFIG_DIR, so it decides which account Claude Code uses. Codepass cannot choose." The API key and token names are handled as in the table.

**Reading the usage (decision 4).** Always through the tool, in the account's folder. Never a credential, never an undocumented endpoint.

| Tool | How | What comes back |
|---|---|---|
| Claude Code | `claude -p "/usage"` with JSON output, run in an empty folder under the app's data folder; the structured `usage_report` on the reply [U R1] | per window: kind (session, weekly for all models, weekly for one model), percent used, reset time; whether usage credits are on. The type is marked experimental by Anthropic. |
| Codex | a short-lived `codex app-server` (stdio), `initialize`, then `account/rateLimits/read`, then exit [U R5a] | two windows (told apart by their length in minutes), percent used, reset time, plan, credits |

- **When:** at most once every 5 minutes per account while a meter is on screen; once before each queued ticket (4.17); once after a limit signal. A tool's own "rate limited until" answer is respected and shown as the reading's age.
- **Not confirmed yet:** that either call works as described and spends no model request (R1-M1, R5-M1). **If a call fails or is not available,** the meter says "未知，运行过一次会话后才知道 / Unknown until a session has run" for Claude Code (then the last limit signal is all the app knows) and "未知 / Unknown" for Codex; nothing else changes. One exception: an authentication error from the usage call is an account signal (decision 34). A call that fails for any other reason, a network or proxy error included, never marks the account.
- A reading is a snapshot with its time. The app never counts tokens itself and never estimates.
- The numbers are the account's, not the app's: use in the tool's other apps counts too [U R1].

**When a limit is reached in a running session (decision 10).**

| Tool | Signal | Reset time known from |
|---|---|---|
| Claude Code | hook `StopFailure` with the error kind `rate_limit`; later `Notification` types `quota_auto_resume_fired`, `_stale`, `_disabled` [U R1] | a usage reading taken right after the signal |
| Codex | no hook. After each turn end or exit, and every 5 minutes while a session runs, the app takes a reading; a window at 100% or a "limit reached" flag while the session is not working means the limit [U R5a] | the reading |

- The session becomes `waiting` with a limit note (4.8). Nothing switches by itself unless the owner turned automatic switching on (decision 33). The session bar shows a banner: which limit, when it resets, and two actions: "在 B 上继续 / Continue on B" when another account of that tool has room (a shortcut into the switch of decision 16), and "等待 / Wait".
- **Claude Code continues by itself.** Since 2.1.234 an interactive session waits and continues after the reset, up to three times (`autoContinueAtUsageLimit`, on by default) [U R1]. The app reads the effective setting, read-only, and the banner says "Claude Code 会在 3:45 自己继续 / Claude Code continues by itself at 3:45". The app does not send a second "continue". Only the owner can change that setting, in the tool.
- **Codex does not.** "Wait" for a Codex session means: the app resumes it at the reset with `codex resume <id>` and the message "continue", if the owner ticked "到时自动继续 / Continue automatically" in the banner (off for an interactive session; the queue always does it, 4.17).
- **Usage credits.** With credits on, neither tool stops at the limit: it keeps going and bills at API rates [U R1, R5a]. The meter shows "已开启用量额度 / Usage credits are on" beside that account. An interactive session is the owner's own business and is not stopped. The queue's rule is in 4.17.
- **A limit is not an account problem.** An authentication, billing, suspension, disabled-account or policy error is handled by decision 34, never by this banner: it offers no other account.

**Switch and resume (decision 9).** Moving a session from account A to account B. It is one flow with three entries: the "换账号 / Switch account" menu on the session bar, the same menu on the task's row on the AI tasks page, and "Continue on B" on the limit banner (decision 16). It needs no limit: the owner may switch while A still has room, in either direction, and as often as wanted. Every switch follows the same steps:

1. Only between turns. A working session stops its turn inside the same confirm (design 5.6); there is no second Stop confirm.
2. The confirm says what comes along and what it costs: "对话会带过去 / The conversation comes along", then "B 上的第一轮会把整段对话重新发送一遍，会用掉 B 的一部分用量。 / The first turn on B sends the whole conversation again and uses part of B's allowance." [U R2, R8]. It adds the lines of decision 24 when they apply (another model; a conversation longer than B's window; a turn cut off at the limit).
3. **B is checked before anything is stopped:** `claude auth status` or `codex login status` in B's folder, and no "Needs your attention" mark on B. If the check fails, nothing is stopped, the session stays as it is on A, and the bar says why ("「个人」没有登录，没有换。 / Personal is not signed in, so nothing was switched." or the words of decision 34). Then the switch is journalled (below), and the tool is stopped the way Stop stops it (4.8): programs the session started end too. The confirm says so when the process tracker (4.8b) reports any. **Nothing starts under B until the tracker confirms that the tool's process under A is gone.** A session never runs under two accounts at once: two processes on one conversation record lose messages [U R8].
4. The tool starts again in the same task folder and the same terminal tab with B's variable, and the conversation is carried by the three layers of decision 22, tried in order without asking: the same session (`claude --resume <id> --model <the session's model>` or `codex resume <id>`), then a repaired copy, then a handoff note. The first message follows decision 23. The task keeps its record; `aiAccountId`, `toolSessionId` and the layer change only once the session runs under B; the session bar, the AI tasks page and the session switcher show the new account. Each switch is recorded on the task (from, to, when, by the owner or automatic, at a limit or not, and the layer that carried the conversation), and the owner is told which layer it was (decision 26).
5. **If no session can start on B at all** (B is not signed in, the tool refuses B's account, the tool does not start): the switch did not happen. The task stays on A, stopped, with its conversation untouched and "继续 / Resume" on A; the bar says so and shows the tool's own message. **No path starts a session on B without the conversation.** The earlier fallback, a new session from "the current state of the branch", is gone, and so is the optional summary written by the account the session leaves: the handoff note needs no usage of A, and at a limit A has none to give.
6. A switch changes this task only. The repository's default (decision 14) stays as it is.

**A switch is journalled on the task, so an app quit or crash in the middle loses nothing.**

- Before step 3 stops anything, the app writes the switch record (`pendingSwitch`, 3.2) with the state in-progress, the target account, the planned new session ID and every path it will create, and only then writes files. A next layer adds its paths to the record before it writes them.
- `aiAccountId`, `toolSessionId` and the layer change only once the session runs under B: the tool's `SessionStart` reported the expected session ID (Codex: its first hook or `notify`).
- **At start-up an in-progress switch is rolled back.** The task stays on its old account, stopped, with "继续 / Resume" there and "换账号 / Switch account" to retry. Its recorded files stay listed for "Free up space" (4.8a). One line on the bar: "上次换账号没有完成，会话还在「工作」上。 / The last switch did not finish. The session is still on Work."
- "The last turn was cut off" is kept on the task (`lastTurnCutOff`, 3.2), so the first message of decision 23 is still sent when the owner resumes or retries after a restart.
- **A layer-3 session gets an ID the app makes** (`--session-id` for Claude Code), recorded in the journal before the session starts. Codex gives its ID in its first hook or `notify`; until then Resume runs layer 3 again from the same note.
- **Resume never starts a new session by itself.** Resume looks for the conversation under every ID in `sessionIds`, newest first, in the task's account folder and then in every other account's folder of that tool. Found in another account's folder, it is a switch: the confirm and the three layers apply. Found nowhere, no session starts: "找不到这个任务的对话记录，没有开新的会话。 / This task's conversation record could not be found, so no new session was started." with "开一个新会话 / Start a new session" as the owner's explicit choice (design 5.6, "App restarted"). The same rule applies after a folder rename (4.4a).
- **When an automatic switch ends in step 5,** the tool's process under A is gone, and with it Claude Code's own wait-and-continue. The app then owns "continue at the reset" for that session, as it does for Codex: `claude --resume <id>` with the decision-23 message once a fresh reading shows room. The banner says "Codepass 会在重置后让它继续 / Codepass continues it after the reset". Step 3's check makes this rare: it is left for a B that passes the check and still does not start.

Cross-account resume is **not verified for either tool** (R2-M2, R5-M4, R8-M1 to M10). What the research shows so far [U R8]: for Claude Code, messages, tool calls and results are plain data and carry over, and Claude Code already strips stale reasoning signatures after a login switch (changelog 2.1.152); for Codex the session file records the account that created it, a check that refuses another account exists in the source but is switched off, and public reports about its encrypted reasoning under another account disagree. Until the checks pass, the builder treats layers 2 and 3 as likely paths, not as edge cases.

**Switching keeps the conversation (decisions 22 to 28).** The owner's statement S1. These seven choices are the planner's, for the owner to correct.

- **22. The hard rule, and three layers.** A switch never starts the work over without its conversation. Three layers are tried in order, automatically, and the owner is told which one carried the conversation.

  | Layer | What happens | What does not come along |
  |---|---|---|
  | **1. The same session** | The session is resumed on the other account from the shared conversation folders (or from the add-only copy, decision 2). If the tool refuses, one more try with the tool's own copy command (`claude --resume <id> --fork-session`; `codex fork <id>`): the tool writes the same history under a new session ID and leaves the original unchanged. The second try stays in the build only if R8-M3 and R8-M9 show that it works where the plain resume does not. | Nothing the owner can see. |
  | **2. A repaired copy** | The app writes a copy of the session under a new session ID, with only the parts removed that the research shows are tied to the account it left, and resumes that copy. The original is never changed. | The removed parts, named to the owner (decision 26). |
  | **3. A handoff note** | The app builds a note on this computer from the conversation record and from git, **with no AI call**, and saves it as a file. A new session on the other account starts with a short fixed first message that names the note's path and tells the AI to read it in full. | The conversation as a conversation. Its content is in the note, and the full record stays on disk for the new session to read. |

  - **"Refused"** is a closed list of signals, so a refused conversation is never taken for an account problem (decision 34).
    - Claude Code: the tool exits within a few seconds without a turn; or `StopFailure` in the first turn after a switch carries the conversation mark `conversation` (4.8: one of the four fixed strings, among them "Invalid `signature` in `thinking` block" [U R8], matched locally; the text is never sent). "Prompt is too long" after a switch goes straight to layer 3 (decision 24): a copy of the same length would not fit either.
    - Codex: the tool exits within a few seconds of `codex resume` or `codex fork`, and its last output line, matched locally and never stored, contains `invalid_encrypted_content`, "Encrypted content organization_id did not match" or the account-check sentence ("This thread was created with a different ChatGPT account or workspace" [U R8]). These read like account errors and are not: they are about the conversation. The account-check sentence skips layer 2 and goes to layer 3 (decision 28).
    - A usage limit on B is not a refusal (the limit banner follows), and an account error on B is decision 34.
  - **Plan mode is kept.** When the last permission mode reported for the session (4.8) was `plan`, every launch after a switch passes `--permission-mode plan` (Claude Code), in every layer, and the result line adds "仍在计划模式 / Still in plan mode". Reason: the tool's own copy command does not restore plan mode, and a new session starts in the owner's default mode, which may edit files without approval [U R8].
  - **Layer 2 for Claude Code** [U R8]. Removed: every `thinking` and `redacted_thinking` block, all of them and never only some (the service refuses a history with some left out in the middle). A line left empty is dropped and the lines that pointed to it point to its parent. A tool call at the very end that has no result is removed: it never finished, and the first message of decision 23 says where the work stopped. Changed: the session ID in every line, to the new ID, which is also the file name. Copied: the session's sub-agent and tool-result folder, to the new ID. Kept unchanged and in order: every message of the owner, every visible reply, every tool call with its result, compaction summaries and what follows them. **Text, tool inputs and tool outputs are never touched.**
  - **Layer 2 is behind a version check.** Both tools call this file format internal. The reader knows the shapes of the tool versions it was tested with (fixtures per version). A file it does not fully understand is not rewritten: layer 2 is skipped and layer 3 is used.
  - **Layer 3, the note.** It holds, in this order: one line that says what it is (a handoff from an earlier session of the same task); the ticket or task text, word for word; **every message of the owner, in full and in order**; the AI's visible replies (the last one in full); each tool call with its target and its outcome; the files the session changed; the last plan or to-do list the AI wrote, with each item's state; the cut-off point (the last call and whether it finished, any question nobody answered, the programs that were stopped at the switch); the branch, `git status`, the diff summary and the recent commits at the moment of the note; the path of the full conversation record, with the sentence that it may be read when a detail is missing; and the closing instruction, which follows decision 23: at a limit or after a cut-off, its sentences; between turns with nothing cut off, "Read this, check the workspace, then wait for the owner's next message."; when the AI's last message was a question to the owner, "Read this, check the workspace, then wait for the owner's answer to the question above." For a queued ticket the note ends with the queue note, word for word (4.17). This mirrors what both tools keep in their own summaries [U R8].
  - **The note is a file, never a command-line argument.** It stays at `sessions/<task-id>/handoff-<n>.md` (mode 0600). The new session's first message is a short fixed English text with the note's absolute path: "This session continues an earlier session of the same task. Before anything else, read the handoff note at <path> in full. It holds every message of the owner, the work so far and where it stopped." The closing instruction above is repeated after it, and for a queued ticket the queue note after that, so the last lines the AI sees are the same in the note and in the message. Reason: a first message travels as an argument (4.7 point 3); Windows limits a whole command line to 32,767 characters and macOS limits arguments and environment together to 1 MiB, and a real note is longer. If the tool asks for permission to read the file, that is the tool's own question: the session is `waiting`.
  - **When the note is longer than the other account's window allows:** the size rule applies to the note file only; the AI reads it with its file tool, in parts if needed. Old tool outcomes are shortened first, then older replies of the AI. **The owner's messages are never shortened.** The full note stays on disk and the shortened one names its path.
  - **One limit that no layer can lift, said plainly.** The AI's hidden reasoning (the "thinking" it does not show) is tied to the account that produced it for the newest Claude models: in another account the service drops it, with no error [U R8]. For Codex the same question is open. Neither Codepass nor the tool can carry it. Everything the owner can see in the conversation carries over: every message of the owner word for word, and the replies, tool calls and results in full (layers 1 and 2) or in the note, which points to the full record (layer 3). The confirm has one link that says this (design 5.6); it is shown only for a Codex session or a Claude model on R8's list.
  - **What the tool may still ask in its own screen** after a resume [U R8]: "Resume this conversation?" after a long break or on another model, a usage-credits question when the model is billed differently on B, or a notice that the organisation's settings replaced the model. The app never answers these. The session is `waiting`, and a ticket is "needs you". The usage-credits question is the owner's alone (decision 32).

- **23. The first message after a switch.**
  - **At a limit** (the turn was cut off): "The previous turn stopped because this account reached a usage limit. The session now continues on another account. Check the workspace first (`git status`), then continue exactly where the work stopped. Do not redo steps that are finished." The text is fixed, in English, visible in the terminal like any message, and is the app's only addition.
  - **Cut off by the owner** ("停下并换账号 / Stop and switch" ended a working turn): "The previous turn was stopped before it finished. Check the workspace first (`git status`), then wait for the owner's next message." Without it nothing would say where the work stopped: layer 2 removes the unfinished last tool call.
  - **Between turns, with nothing cut off:** no message in layers 1 and 2. The session waits for the owner, as before the switch.
  - **With layer 3** there is always a first message, the short fixed text that names the note's path (decision 22). The sentences above follow it and close the note. Between turns with nothing cut off, the closing instruction is "Read this, check the workspace, then wait for the owner's next message.", or "wait for the owner's answer" when the AI's last message was a question, so the new session never acts past a question it had asked.
  - Which case applies is read from `lastTurnCutOff` on the task (3.2), so it survives a restart.
  - A queued ticket's session keeps its queue note from its first message in layers 1 and 2 (4.17); nothing is added to it. In layer 3 the conversation is new, so the note and the first message end with the queue note, word for word. Otherwise every later turn end would read `none` and the ticket would become "needs you".
- **24. What the confirm says before the switch, when it applies.**
  - **Another model.** The app knows that the other account's plan lacks a model only from what the tool reported under that account before (`modelsReplaced`, 3.2: the model that was replaced, the model the tool used in its place, and a date; nothing else). When `modelsReplaced` has an entry for B and the session's model, resume passes that replacement with `--model`, and the confirm names both models. Otherwise resume passes the session's own model, so the tool does not silently fall back to its default [U R8], and the confirm says nothing: the app does not promise a model it cannot know. If B's tool then replaces the model or asks about credits, that is the tool's own screen (decision 22), and the result line names the model if it changed.
  - **A conversation longer than the other account's window.** The tool compacts it on the first turn (its own summary of the earlier part), or, if it cannot, layer 3 is used. The confirm says so when the conversation's last size is above the standard window; after the switch the result line says it when the tool reports a compaction. **Sources:** the size is the token counts of the last assistant entry in the transcript, read locally by the conversation reader (task T8-11) and never sent anywhere. A compaction is `SessionStart` with the source `compact`, forwarded as a fixed value (4.8). When compaction cannot fit the conversation, the tool's "Prompt is too long" is the signal for layer 3 (decision 22).
  - **A turn cut off at the limit.** One line: the AI will check the workspace first and continue where it stopped. For a turn the owner stops inside the confirm ("Stop and switch"): the AI will check the workspace first and then wait for the owner.
- **25. The same task.** A switch keeps the task record, the workspace, the branch and the terminal tab. Only the account changes, and with layer 1's second try or layer 2 the session ID (all IDs stay on the task, 3.2). Each switch records the layer that carried the conversation.
- **26. Honest result lines, one per layer** (design 5.6). The line never says more than the layer delivered.
  - Layer 1: "对话完整带过去了 / The whole conversation came along". It is shown only once the tool's `SessionStart` reports the expected session ID with the source `resume` (4.8); until then the chip keeps "正在换到「个人」… / Switching to Personal...".
  - **A refusal that shows only in the first turn** (after a between-turns switch no message is sent, so it comes after the owner has typed): the line changes to the next layer's, and that layer carries the owner's unanswered last message, which is by then in the refused session's record. The owner does not type it again.
  - **The tool started a new conversation instead** (the owner chose "Start a new conversation" in the tool's own "Resume this conversation?" screen, so the tool reports a session ID other than the one resumed): "Claude Code 开了一个新对话，原来的对话没有接上。 / Claude Code started a new conversation; the earlier one was not continued." with "换账号 / Switch account" to retry. The earlier conversation is untouched, and the new ID is added to `sessionIds`.
  - Every line adds "仍在计划模式 / Still in plan mode" when decision 22's plan-mode rule applied.
  - Layer 2: "对话带过去了，只去掉了只属于「工作」的部分 / The conversation came along, without the parts that belong only to Work", followed by exactly what was removed. Claude Code: "AI 的思考记录 / the AI's reasoning records". Codex: "AI 的推理记录和加密的摘要 / the AI's reasoning records and encrypted summaries". When a last, unfinished tool call was removed, the line says that too.
  - Layer 3: "对话以交接笔记的形式带过去了 / The conversation came along as a handoff note", with "查看笔记 / View note".
  - No layer could start a session: "没有换成 / The switch did not happen", and the session is still on the account it was on (step 5).
- **27. Files.** Copies and notes are always **new** files. The app never changes a file of a tool. With the owner's consent it adds new files in two cases only, the repaired copy here and the add-only copy of decision 2. It deletes a tool's file only through "Free up space" (4.8a): the exact paths of an ended task's own session IDs and recorded copies, after the preview.
  - A repaired copy is a new conversation record under a new session ID, written where the tool looks for sessions: the shared conversation folder, which for the tool's own account is inside the tool's own folder. This is one of the two cases in which the app adds a file to a tool's folder (decision 2), and the owner consents to it once, in the add-account sheet (design 5.16). The new ID is random and is checked against the sessions that exist. The copy is written with mode 0600 and its folders 0700, whatever the original's mode. The copy's exact paths are recorded on the task before the copy is written ("Switch and resume").
  - A handoff note lives in the task's data folder: `sessions/<task-id>/handoff-<n>.md`, mode 0600.
  - Both are listed and deleted by "Free up space" for that task only, by exact path (4.8a). Neither is ever in Codepass's backups, an export or the log, and no conversation text reaches an error message. `sessions/` is excluded from Time Machine, like the worktrees folder (4.7 point 2); a repaired copy sits in the tool's folder, and Time Machine copies it like the tool's own transcripts.
- **28. Codex: the same three layers.** Layer 1: `codex resume <id>` by ID under B's `CODEX_HOME` (never "the last session": Codex's own listing can hide another account's sessions [U R8]), then `codex fork <id>`. Layer 2 removes exactly what R8 names: every `reasoning` item; every `compaction` and `context_compaction` item, with the items before it kept in its place so the visible history is whole again (Codex compacts by itself if it is then too long); and the `encrypted_content` field of message and call items. Changed: only `id` and `session_id`, to the new ID. Kept: every message of the owner and of the AI, every call with its output, the folder, the base instructions, and **the record of which account created the conversation** (`creator_user_id`, `creator_account_id` and `account_scoped` in the first line stay exactly as they are). When Codex refuses with its account-check sentence ("This thread was created with a different ChatGPT account or workspace. Sign in with the original account to resume it." [U R8]), layer 2 is skipped and layer 3 is used: a new session that B creates itself. Codepass never rewrites that record to get past the check (decision 31). Layer 3 is the same note. None of this was run (R8-M8, R8-M9): whether Codex finds a file the app wrote is not known, so for Codex layer 3 may be the main path at first.

**Several accounts within the terms (decisions 29 to 36).** The owner's statements S2 and S3. The quotes are from [U R9] (Anthropic's pages read directly on 2026-11-16; OpenAI's from archive copies of 2026-11-14 to 2026-11-16) and are not legal advice. These eight choices are the planner's, for the owner to correct.

What the published text says, sorted [U R9]:

| | Anthropic (Claude Code) | OpenAI (Codex) |
|---|---|---|
| Several of the owner's own accounts signed in on one computer, side by side | **Documented.** "To stay signed in to multiple accounts at once, such as work and personal accounts, give each account its own configuration directory." `CLAUDE_CONFIG_DIR` is "Useful for running multiple accounts side by side". | Not forbidden, not described. A help article: "If you want to use ChatGPT under a different account, you'll need to manage the subscription separately". |
| One person holding several paid subscriptions | No clause for or against. "We only allow a maximum of three Claude accounts to be verified with the same phone number." | No clause for or against. |
| Switching a task **by hand** to the owner's second account at a limit | Not named. Bearing on it: "Advertised usage limits for Pro, Max, and Ultra plans assume ordinary, individual usage". | Not named. Bearing on it, and nearer the line: "Interfere with or disrupt our Services, including circumvent any rate limits or restrictions or bypass any protective measures or safety mitigations". |
| The same switch done **by an app** at the moment of the limit | Less supported than by hand: also "to access the Services through automated or non-human means, whether through a bot, script, or otherwise" (not allowed without an API key or explicit permission). | The same sentence as above, and nothing in the docs describes it. |
| Sharing, reselling, proxying | **Not allowed.** "You may not share your Account login information, Anthropic API key, or Account credentials with anyone else. You also may not make your Account available to anyone else." "Resell, proxy, or otherwise provide access to Claude through unauthorized means, including services that route requests through consumer subscriptions or misrepresent the product or client being used". | **Not allowed.** "You may not share your account credentials or make your account available to anyone else". |
| An app that holds a sign-in, or a changed tool | **Not allowed.** "developers may not collect, store, or intermediate Claude.ai credentials or session tokens — sign-in to a Claude account must complete through Anthropic's own flow." "The Claude Code binary must not be modified." Allowed: "signing in to the unmodified Claude Code binary with their own Claude subscription". | Codex docs: "Treat `~/.codex/auth.json` like a password". |
| A second account after a ban | **Not allowed.** "Circumvent a ban through the use of a different account, such as the creation of a new account, use of an existing account, or providing access to a person or entity that was previously banned". | No sentence on a second account was found. Usage Policies: "breaking or circumventing our rules and safeguards may mean you lose access to our systems or experience other penalties." |

No statement by either company was found that names one person with two paid accounts as a reason for a ban, and none that rules it out [U R9].

- **29. Codepass supports several tasks at once on different accounts** (S3). Task A on "工作 / Work" and task B on "个人 / Personal" at the same time is the normal case of decision 13: every task has its own account, and sessions under different accounts run side by side. For Claude Code it is the use Anthropic documents ("multiple accounts at once, such as work and personal accounts"); for Codex nothing forbids it and nothing describes it. One condition: a Team or Enterprise seat follows its organisation's rules about which work may run on it, and an organisation can refuse other accounts on a managed computer [U R9]. The add-account sheet says so in one line (design 5.16).
- **30. Personal use only.** Every AI account in Codepass is the owner's own: paid by the owner and signed in by the owner, through the tool's own sign-in. No sharing with other people, no reselling, no running work for others, no proxy and no API for others. No feature lets another person use an account. An export never includes an AI account folder or anything signed in (3.2).
- **31. Unmodified official tools.** Codepass starts the official `claude` and `codex` programs the owner installed, in a normal terminal, with their own sign-in. It never patches them, never calls a private endpoint, never reads or stores a token, never automates sign-up or sign-in, never changes device identifiers, proxies or network settings per account, never hides that the accounts are used on one computer, and never removes or rewrites a tool's record of which account created a conversation.
- **32. Each account stays inside its own plan.** The usage is read through the tool (decision 4). The queue respects each account's limits and the reserve (4.17). Usage credits are used only when the owner allows it. One queue session runs at a time. The same ticket never runs on two accounts at once, and a session never runs under two accounts at once (step 3).
- **33. Automatic switching is off by default.** This changes decision 9 and every place that said "on by default once a tool has two accounts". Reason: an automatic hop to the next account at the moment of a limit is the pattern closest to OpenAI's "circumvent any rate limits or restrictions" and to Anthropic's "automated or non-human means"; it is the part of this section with the least support in the published text [U R9]. Turning it on, per tool, shows a short confirm once, with those sentences, the links to the terms, and the sentence of decision 36. With it off nothing is lost in convenience that matters: the limit banner and the switch menu still switch with one click and a confirm, and the queue pauses and sends its notice with the button "在「个人」上继续 / Continue on Personal" when another account has room (no button otherwise; 4.17).
- **34. Never route around a ban, a suspension or a lost sign-in.** When a tool reports an authentication, billing, suspension, disabled-account or policy error for an account, Codepass stops using that account: no automatic switch for that session, no retry, and the queue pauses. The account is marked "需要你处理 / Needs your attention" with the tool's own message, and the decision is the owner's.
  - **Signals, a closed list.** The account rule covers only these:
    - Claude Code: `StopFailure` with the kind `authentication_failed` or `billing_error`; `StopFailure` with the kind `other` and the conversation mark `none` (4.8) at a turn end; `claude auth status` exiting non-zero for an account that was signed in; an authentication error from the usage call.
    - Codex: a failed `codex login status`; an authentication or account error from the usage call or at a turn end, unless it is one of the refusal signals of decision 22.
    - **Never an account problem:** a conversation error (the mark `conversation`, or a Codex refusal of decision 22), which moves the switch to the next layer; `rate_limit` (decision 10); `server_error` and `overloaded`; a usage call that fails for any other reason; a network or proxy error. Those keep "未知 / Unknown" on the meter or "等待你 / needs you" on the session, and mark nothing.
    - Within the list, stopping is the safe side: an `other` error at a turn end that is not about the conversation marks the account, with the kind `unknown`.
  - **While the mark is set:** the account cannot be started on or picked as the target of a switch, and when it is the default of a new task the app pre-selects no other account in its place (decision 15); its sessions are not switched to another account by the app, neither automatically nor from the menu; a ticket that would run on it pauses the queue. Sessions already running under it are left alone: the tool shows its own error there.
  - **Only the owner clears it.** For the sign-in kind, "重新检查 / Check again" decides: it asks the tool for its status (or the owner signs in again through the tool). Signed in: the mark goes. Not signed in: the account is a normal "未登录 / Not signed in" account, and a session can be moved by the owner like any other. For the kinds billing, refused and unknown the status call cannot tell (a suspended or unpaid account still reads as signed in): Check again shows what the tool says now, and the mark is cleared only when the owner confirms "我已处理好 / I have dealt with it". The next account error marks it again.
  - The tool's message is shown as text and kept in memory only; the file stores the kind and the time (3.1). It is never logged and never put in a notification.
  - Why: Anthropic's Usage Policy forbids to "Circumvent a ban through the use of a different account, such as the creation of a new account, use of an existing account". No matching sentence was found for OpenAI; the same rule applies to Codex.
- **35. Terms checkpoint.** Before milestone 8 is built the owner reads [U R3], [U R5d] and [U R9] and confirms how several subscriptions and the queue are used. The plan records the date and the decision here: **not confirmed yet** (date: none; decision: none). Until then the plan assumes this section as written. Settings carries one line, "Codepass 只用于你自己的账号和你自己的工作。 / Codepass is for your own accounts and your own work.", with links to both companies' terms (design 5.14). The release checklist gets "re-read both companies' AI terms" (section 10).
- **36. What the app cannot promise, said once and plainly.** Codepass cannot guarantee that an account will not be limited or suspended, and how several subscriptions are used is the owner's decision. The sentence appears in two places only, the step that adds a second account and the confirm that turns automatic switching on (design 5.16): "Codepass 不能保证账号不会被限制或停用。几个订阅怎么用，由你决定。 / Codepass cannot guarantee that an account will not be limited or suspended. How you use several subscriptions is your decision." No warning colour, no repeated reminders.

**Auto-switch** is a setting per tool, **off by default** (decision 33). It is stored per tool in `ai-accounts.json` (3.1, 3.2) and set on the AI accounts page (design 5.16), not in the queue's settings, because it applies to the owner's own sessions and to the queue alike. A tool's second account does not turn it on; the first time the owner turns it on for a tool, the confirm of decision 33 is shown. With it on: at a limit, the app picks the first account in the owner's order that has room (room as defined after decision 15) and does steps 1 to 5 without the switch confirm; the three layers and the first message are the same (decisions 22 and 23). Step 3 applies here too: programs the session started end, and the banner says so when there were any. It tells the owner by a banner on the session bar that names the layer and, for a queued ticket, a notification. When an automatic switch ends in step 5 (no session could start on the other account), the app continues the session on the account it was on after the reset, and the banner says "Codepass 会在重置后让它继续 / Codepass continues it after the reset" ("Switch and resume", the journal's last point). It never switches a session that is working, never switches twice within one turn, and never switches back by itself: when the account it left has room again, the session bar only offers "切回「工作」 / Switch back to Work" (decision 17), and the owner decides. Auto-switch acts only at a usage limit. It never acts on an account error (decision 34), never moves a session because of a default, and does not act when the shell decides the account. With it off, every switch is the owner's click.

**What the UI shows** (design 5.16, 5.6): an AI accounts group on the Accounts page, where the first account of a tool is marked as the app default and each account's details list the projects and repositories it is the default for; a usage chip in the sidebar with a popover that lists every account of both tools; on the session bar the account chip, which is the "Switch account" menu with its confirm (what comes along, the cost, and the lines of decision 24), the result line that names the layer and "View note" (decision 26), the limit banner and the switch-back hint; the automatic-switching switch per tool, off, with its one-time confirm (decision 33); the "Needs your attention" state of an account with the tool's message (decision 34); the consent sentence for conversation copies, the Team or Enterprise line and the sentence of decision 36 in the add-account sheet; the personal-use line in Settings (decision 35); copies and notes in "Free up space" (4.8a); the task's account and the same menu on the AI tasks page rows; the account in the session switcher rows; the account row in the New task sheet, pre-selected from the defaults; the two default rows ("This repository uses") in the Repository settings sheet (design 4.2) and "AI accounts" in the project section's menu (design 5.2); the account field of a ticket (design 5.15). The account chip and its menu, the switch-back hint, the account on AI tasks and switcher rows, the New task account row, the default rows, the project's "AI accounts" item and the ticket's account field appear only for a tool with more than one account; the accounts group, the usage chip and the limit banner appear with one account too.

**Terms.** The quotes, the links, what was not found and what could not be read live are in [U R3], [U R5d] and [U R9]; the table and decisions 29 to 36 above are what this plan does about them. In short: every sign-in stays inside the unmodified official tool and the app holds no credential (decision 31); the accounts are the owner's own, for the owner's own work (decision 30); nothing switches by itself unless the owner turned it on after reading the sentences that bear on it (decision 33); a refused or suspended account is never routed around (decision 34). No clause was found, for either company, about one person holding several subscriptions, and neither company was asked: only a written answer from them would settle the grey cases (R9-M4 is the owner's choice). Whether and how to use several subscriptions is the owner's decision (decisions 35 and 36).

### 4.17 Tickets and the queue

Owner's requirement (2026-11-16): today, ideas for later are typed into a text editor while the weekly usage is nearly used up. Codepass gets tickets with the owner's three states, 编辑中 (editing), 暂不执行 (on hold) and 待执行 (queued). Queued tickets run one at a time. Each must finish completely before the next starts. Before each one the app checks the usage, and the queue pauses when the limit is reached.

Research: [U R4, R5c]. As in 4.16, nothing here has been run, and the numbered choices are the planner's for the owner to correct.

**A ticket (decision 11).**

- Text in Markdown (shown with the app's safe renderer; raw HTML off). A repository and a tool: both may be empty while editing and are required to queue. Where it runs: own workspace by default.
- **Account (decision 18 in 4.16).** "跟随仓库 / Follow the repository" by default, or one fixed account of the ticket's tool. "Follow the repository" means the repository's default for that tool (repository > project > app, 4.16 decision 14), resolved when the ticket starts. Shown only when the tool has more than one account.
- **Quick capture** from any screen, Cmd/Ctrl+Shift+N: a small sheet with the text field focused. It saves as you type (state: editing). Esc closes and keeps the text.
- **Owner states:** 编辑中 / Editing, 暂不执行 / On hold, 待执行 / Queued. **System states:** running, waiting for reset, needs you, done, failed, skipped. Only the owner moves a ticket between the three owner states; only the queue sets a system state.
- **One queue for the whole app**, across repositories, ordered by drag. One ticket runs at a time.
- A ticket that starts becomes **a normal AI task** (4.7): its own workspace, named by the AI, the same terminal, review and merge (4.9). The ticket links to the task and the task to the ticket. Finishing the ticket does not merge anything.
- Stored in `tickets.json` (3.1). Ticket text is never logged and never put in a notification.

**How a ticket runs (decision 5).** An interactive session in the app's own terminal, started with the ticket text as the first message, with the hooks of 4.8. Not print mode: the owner wants to watch and to step in, permission questions and the tool's own limit wait work as usual, and print mode would treat a key in the environment differently (4.7 point 7) [U R4]. The app adds no flag that skips approvals. A queue runs unattended only as far as the tool's own approval mode allows; with a mode that asks, the ticket stops at the first question and is "needs you".

**When is a ticket finished? (decision 6)** No signal separates "done" from "the AI ended its turn with a question in plain text": both are a normal turn end [U R4]. So:

- The queue appends a short **queue note** to the ticket's first message. It is visible and editable in Settings and shown under the ticket text before queueing: "When everything in this ticket is finished, end your last message with the single line CODEPASS-DONE. If you need an answer from me first, end with the single line CODEPASS-QUESTION."
- At each turn end, report mode compares the last line of the last assistant message with those two words, locally, and sends only `done`, `question` or `none` (4.8).

| What happened | Ticket becomes | Queue |
|---|---|---|
| Turn end with `done`, no background work pending | done | starts the next ticket after its check |
| Turn end with `question` or `none`; the AskUserQuestion tool; a permission prompt; an input request from a connected server; idle after a turn | needs you | **waits**. One notification. The owner answers in the terminal, or presses "算完成 / Mark done" or "跳过 / Skip" |
| The owner types into the session | stays running | nothing; the next turn end is judged again |
| Limit reached | waiting for reset | see below |
| A turn ended by another error (overloaded, server error): | running, then needs you | the tool retries by itself first; if the turn ends, "needs you" with the error kind |
| The tool exits non-zero | failed | **waits**; "重试 / Retry" or "跳过 / Skip". A failed ticket never lets the next one start by itself |
| The tool reports an authentication, billing, suspension, disabled-account or policy error for the account (4.16 decision 34) | needs you, with the account marked "需要你处理 / Needs your attention" | **pauses, whatever automatic switching is set to.** No retry, and the ticket is not moved to another account. One notification. The queue goes on only after the owner cleared the mark, or took the ticket out |
| OpenCode | needs you at every turn end | OpenCode sends no last message to compare (not verified); the owner presses "Mark done" |

The owner's rule "each ticket finishes completely before the next starts" is why every unclear case waits instead of moving on.

**The check before each ticket (decision 8).**

1. A usage reading for the account the ticket will run under: its fixed account, or the repository's default when it follows the repository (4.16).
2. **Room** means: the session window and the weekly window (and the weekly window of the model in use, when known) are below 100% minus the reserve. The reserve, "给自己留 / Keep for myself", is a percentage the queue leaves untouched for the owner's own interactive work. Default 0%.
3. No room on that account and another account of the tool has room:
   - **Automatic switching off (the default, 4.16 decision 33):** the queue is paused for the limit until that account's reset, and its notice and the queue bar carry the button "在「个人」上继续 / Continue on Personal", naming the first account in the owner's order that has room. One click starts this ticket on that account, this once; nothing is remembered for the next ticket. This holds for a fixed account too.
   - **Automatic switching on:** the ticket starts on the first account in the owner's order that has room, also when its account is fixed.
   - The ticket and its task say which account was used and why.
4. No room anywhere: the queue is **paused for the limit** until the earliest reset among the windows that block it. One notification, with no button. At that time it reads again; it does not start on a guess.
5. Reading unknown: the ticket starts, and the limit signal of 4.16 is the safety net. The queue bar says the check could not be made.
6. **Usage credits.** The queue never starts a ticket, and never keeps one going, on credits unless the owner allows it ("允许队列使用用量额度 / Let the queue use usage credits", off by default). With credits on for the account and not allowed: a window at 100% counts as "no room" even though the tool would continue; and when a running ticket crosses 100%, the queue stops the session at the next turn end and treats the ticket as waiting for reset. The app cannot see a crossing in the middle of a turn before the turn ends, so a single long turn can still spend credits; the setting's help line says so.
7. OpenCode: no check. The ticket starts.
8. With "每张票开始前先问我 / Ask me before each ticket" on (default off), the queue stops before every start and waits for "开始 / Start".
9. **An account that needs the owner's attention** (4.16 decision 34) is never used and never routed around: a ticket that resolves to it pauses the queue with that reason, with automatic switching on or off, and the notice names the account. The button "Continue on Personal" is not offered in this case.
10. **Never the same ticket on two accounts at once**, and one queue session at a time (4.16 decision 32).

**A limit during a ticket: one owner of "continue" (decision 7).**

| Case | Who continues |
|---|---|
| Automatic switching on (off by default) and another account has room | The queue: switch and resume (4.16), at the turn boundary the limit made, with the conversation carried by the three layers (4.16 decision 22) and the first message of decision 23. Notification "continued on another account". |
| Automatic switching off and another account has room | **The owner, with one click.** The ticket is "waiting for reset"; the notice "paused for the limit" and the queue bar carry "在「个人」上继续 / Continue on Personal", which opens the switch confirm for that session (4.16 decision 16). Without the click, the rows below apply: the session waits for the reset. |
| Claude Code, its own automatic continue on (the default) | **Claude Code.** The queue sends nothing. The ticket reads "等待重置 · Claude Code 会在 3:45 自己继续 / Waiting for reset · Claude Code continues by itself at 3:45". |
| Claude Code, automatic continue off, or it gave up (`quota_auto_resume_disabled`), or the Mac slept through the reset (`quota_auto_resume_stale`) | The queue: at the reset, after a fresh reading shows room, it resumes the session (`claude --resume <id>` with the message "continue"). |
| Codex | Always the queue, the same way (`codex resume <id>`, "continue"). |
| OpenCode | Nobody. A failed turn is "needs you". |

The queue never types "continue" into a session that the tool is about to continue by itself.

**Sleep.** Keep-awake (4.10) holds while a ticket runs. While the queue only waits for a reset, the Mac may sleep; on wake the queue takes a reading and carries on. A reset that passed during sleep is handled by the table above.

**Stopping and editing.** "暂停队列 / Pause queue" stops after the current ticket; "现在停止 / Stop now" stops the running session like any task (its conversation is kept, the ticket goes back to queued with a note). A queued ticket can be edited until it starts. Removing a ticket never removes its task or workspace.

**Notifications** go through the one service (4.15), one button at most, and never contain ticket text: needs you; paused for the limit (with the reset time, and with the button "在「个人」上继续 / Continue on Personal" when automatic switching is off and another account has room); continued on another account (only with automatic switching on); queue finished (with counts). An account that needs the owner's attention has its own notice (4.16 decision 34). Texts are in design 6.

**What the UI shows** (design 5.15): the Tickets page with the owner's three columns, the queue and the finished list; the editor; quick capture; the queue bar with what the queue is doing and why.

**Terms (decision 12).** The queue starts AI sessions by itself, one after another. Published sentences about that kind of use [U R3, R9]: Anthropic's Consumer Terms list as not allowed, "Except when you are accessing our Services via an Anthropic API Key or where we otherwise explicitly permit it", "to access the Services through automated or non-human means, whether through a bot, script, or otherwise". Anthropic's Claude Code documentation describes scripted and unattended runs on a subscription sign-in, and its legal page says the advertised limits "assume ordinary, individual usage". For Codex [U R9]: "`codex exec` reuses saved CLI authentication by default", "Schedule recurring tasks to run in the background", and at a limit "subject to fair use limits"; its docs recommend an API key for CI. So an unattended local queue that starts the official tool one ticket at a time is allowed in form for both and grey in amount. What the queue does to stay on the safe side (4.16 decisions 30 to 34): the owner's own accounts and work only; the official tool in a normal terminal; one session at a time; each account's limits and the reserve respected; no usage credits unless allowed; no automatic hop to another account unless the owner turned it on; a full stop at any account error. **The owner reads [U R3], [U R5d] and [U R9] and decides before the queue task is built** (the checkpoint of 4.16 decision 35). Assumed until then: the queue starts tickets by itself; "Ask me before each ticket" exists for the owner who prefers a person in the loop.

---

## 5. Technology choices and engineering rules

| Topic | Choice | Reason |
|---|---|---|
| Terminal UI | **xterm.js** (`@xterm/xterm` 6.x), DOM renderer first | The standard; handles full-screen terminal apps. Needs inline styles, which the CSP already allows [S19]. |
| PTY | macOS: `github.com/creack/pty`. Windows: ConPTY behind the app's own `internal/pty` interface, as **an own wrapper over `golang.org/x/sys/windows`** (CreatePseudoConsole, `PROC_THREAD_ATTRIBUTE_JOB_LIST`, a Job Object with `KILL_ON_JOB_CLOSE`), derived from go-pty's MIT code [S20] and not imported. `ClosePseudoConsole` runs off the caller's goroutine. | go-pty has no Job Object and closes the thread handle, so a child cannot be put in a job before it can start a grandchild; it also pulls in SSH helper modules. **The wrapper compiles and has never run: it is the first Windows VM task** [M0 S0-01]. |
| **Terminal data path** | The PTY reader **only appends to a per-session byte log with offsets**. The event to the page is a tiny "new data up to offset N" ping, sent from its own goroutine. The page pulls bytes by offset through a bound method and re-syncs on gaps. Only visible terminals pull (per terminal, not per window). Measured: 50 MB with nobody reading never blocks the child; the rejected design stalls after 512 KB [M0 S0-01]. The log is a ring of fixed chunks (default cap 16 MB per session), trimmed only between escape sequences and UTF-8 characters. The reader keeps a small table of terminal modes (alt screen, mouse, bracketed paste, focus, cursor keys, cursor visibility); a read whose offset was trimmed returns `gap` and a mode `prologue` as of the new start of the log. The page then resets xterm, writes the prologue and the data, and nudges the child with a size change (cols-1, 120 ms, back), at most once every 2 seconds. **While replaying history (attach, reload, after a gap) the page drops everything xterm emits on `onData` and `onBinary`:** replayed queries would otherwise be answered into the live program as typed input. | Wails makes a background emitter wait once 64 events are queued for a window, and parks payloads over 8 KB for 30 seconds before dropping them. If the reader sent data as events it would stall whenever the UI is slow or hidden, the AI tool would block on output, and the task would pause exactly when the owner walks away [R6]. |
| Terminal instances | Kept mounted across page, tab and folder switches | That is the rule. A reload replays a **full** log to the identical screen (verified for vim, opencode and the claude first-run screen). Replay after **trimming** is verified for vim only and failed once for opencode, so it is the recovery path, not the normal one [R-S1] [M0 review A]. |
| Terminal details | Shift+Enter sends ESC+CR; Unicode 11 / grapheme width addon; xterm's `windowsPty` option; screen-reader mode toggle; OSC 52 clipboard writes off; links through the https-only opener, `http://localhost` after a one-time confirm | [R-S1] |
| Code editor | **CodeMirror 6** with `@codemirror/merge` for diffs and conflicts | No web workers, so `worker-src 'none'` stays; a few hundred kB with lazily loaded languages; viewport rendering for large files; a nonce option if the CSP is tightened [S21]. Monaco needs workers and is about 100 MB unpacked. |
| Language servers | `@codemirror/lsp-client` in the page; servers are child processes of Go; the transport is pull-by-offset over a **message log** (not a byte log). Responses and requests are never dropped; above a per-session cap (16 MB) progress notes, log messages and superseded diagnostics are compacted, and a hard limit restarts the server. Go answers the server requests a hidden page cannot answer. The ping never blocks the reader; the send path never drops a message. | One non-blocking design for both. A hidden or frozen page did not stall gopls; a reader that waits on a 64-event queue stalled it for 37 seconds [M0 S0-06]. |
| File watching | FSEvents on macOS, `ReadDirectoryChangesW` on Windows; ignored folders skipped; bursts debounced | Go's usual fsnotify uses kqueue on macOS: one file handle per file and no recursive watching [R-S4]. |
| Find in files | `git grep` (tracked and untracked, ignores respected) with the read-only flags from 4.6 | Already installed. |
| Diff data | `git diff` parsed in Go, rendered by the CodeMirror merge view | One source for Changes, History, task review and pull request files. |
| Updater | Wails `pkg/updater` for download, extract, swap, relaunch and rollback; **every trust decision is the app's own**, made on the staged copy before the swap (4.14) | The swap is the hard part and already exists [S17]. It is not a trust boundary, and for a `.app` it is remove-then-rename, not atomic [M0 S0-02]. |
| Secrets | Security.framework on macOS, Credential Manager on Windows, file fake in sandbox mode | 3.3 |
| HTTP | One client that reads the system proxy; ETag cache for GitHub and Bitbucket | [R-S11] |
| Process control | macOS: kill by terminal session (every group whose `getsid()` is the task's), plus a process tracker for what leaves the session. Windows: Job Objects. | [R-S5] [M0 review A, X1]; 4.8, 4.8b |
| Repository identity | A random ID per repository record; the path is a field | 3.2, 4.4a |
| CSP | **Unchanged** | Every choice was made so `frontend/scripts/vite-plugins.ts` needs no loosening. |

**Rules for every implementer and reviewer:**

1. No real names in the repository. Mock scenarios, tests, fixtures, the tutorial and screenshots use acme / globex / initech, `111122223333`, `@example.com`. The publish check blocks the rest; its word list is changed to allow the product strings it must now name (Claude Code, the `claude` command, hook settings) while still blocking AI attribution lines [R3].
2. Commands are argument lists, never shell strings.
3. Automatic git reads use the read-only flags (4.6).
4. Text from other people is never markup: Markdown with raw HTML off, no remote images, links through the https-only opener. `dangerouslySetInnerHTML` stays at zero; a test enforces it. The page can now write to a terminal, so an HTML injection would be code execution [R-S3]. The language-server client library renders server Markdown through `innerHTML`: it gets a real sanitizer or raw HTML off, with a test that an `<img onerror>` in a hover stays inert [M0 S0-06].
5. Terminal output, language-server traffic, hook payloads, tokens and keys never reach the log. Language servers send log messages of their own (the native TypeScript server one per request); they are dropped, never logged.
6. Every new Go service has a typed mock (`frontend/src/api/mock`); the mocks are typed against the generated bindings. Two are scripted: a terminal and a fake language server.
7. Every config write is backup-first. Backups never contain private keys or tokens.
8. Every milestone builds and smoke-tests on Windows.
9. A privileged or elevated child process gets an explicit minimal environment and absolute tool paths; nothing is inherited.
10. No developer-tool stub runs on macOS before `xcode-select -p` succeeds (4.1).
11. Nothing is deleted from disk unless its exact path was listed first and it belongs to one known task or session ID (4.8a).

---

## 6. Scope of v1.0.0, and what moves first

**In 1.0.0:** everything in sections 2 and 4. The owner decided on 2026-11-16 that nothing moves to 1.1 (decision 4); the list at the end of this section is kept for the record only.

**Not in 1.0.0:** interactive rebase; inline pull request line comments; nested projects; GitHub Enterprise and Bitbucket Data Center hosts; GitLab and others; SSH key passphrases created by the app; code intelligence beyond Go, TS/JS and PHP; rename symbol and format on save (shown when a server offers them, not required); debugger, run configurations, extensions; a chat UI of the app's own; Linux; telemetry of any kind; extending the Git global guard to SSH hosts.

**The order that was proposed in case the date slipped (not in force: decision 4):**

1. Tags and remotes screens (remotes stay editable in the repository settings sheet).
2. The cross-repository Pull requests page (the per-repository tab stays).
3. Line-level staging (hunk staging stays).
4. Split editor.
5. Blame and file history.
6. OpenCode state detection beyond the fallback.
7. The TypeScript language server (Go stays).

---

## 7. Milestones

Internal only. Each milestone ends with a signed build on the test channel, a Windows build and smoke test, and the gate in `tasks.md`. Sizes are in weeks of the implement-and-review loop and are estimates.

| # | Milestone | Contents | Weeks |
|---|---|---|---|
| 0 | **Spikes** (finished 2026-11-16) | Terminal in WKWebView (resize, pull-by-offset, reload); manifest-signed update; Claude Code and OpenCode signals; Homebrew.pkg checks; signing and notarization; language-server transport; GitHub token reads. Not run: anything on Windows, clean VMs, Chinese input, Codex, Bitbucket with a token (`manual-checks.md`) | 2 |
| 1 | **Foundation** | Rename and module path; data files; **stable repository IDs**; custom menu and window behaviour; old-app handoff; signed updater (macOS and the Windows gate) and release pipeline; tools gate; repository engine for any git repository; **clone under a custom folder name, two names shown**; read-only git layer and file watcher; projects; author identity; motion tokens; scrub check; **the AWS SSO Hub 0.0.2 fixes (merge of `main`); system notifications and the sign-in expiry notices** | 7 |
| 2 | **AI terminal and keep-awake** | Workspace frame; PTY and session log; terminal panel; AI tasks in the main workspace for three tools; install on first use; task states and notifications; keep-awake; kill by terminal session; **process tracker and Force stop; the quit dialog; the Cmd+J session switcher; Open repository (Cmd+O) by folder name** | 5 |
| 3 | **Worktree tasks, Changes, commit** | Own workspace per task; working-tree switcher; Changes with hunk and line staging; commit composer; discard with recovery; merge / push / discard a task; AI tasks page; **Free up space; Rename local folder** | 5 |
| 4 | **Git client** | Sync with safe force push; branches; compare; merge and conflict screen; rebase; cherry-pick; revert; reset; stash; history with graph; tags; remotes; blame; file history | 5 |
| 5 | **Editor and code intelligence** | File tree; tabs and split; CodeMirror; quick open; find in files; outline; recovery; language servers for Go, TS/JS **and PHP** with install consent | 6 |
| 6 | **GitHub and Bitbucket** | Accounts; SSH keys and host keys; token store; binding and probe; clone sources; pull request list, create, detail, merge | 4 |
| 7 | **Windows parity, polish, release** | Windows pass on every screen; accessibility; performance on large repositories; tutorial and guide; README, SECURITY and notices; rc1 to rc2 update test; v1.0.0 | 3 |
| 8 | **Ticket queue, AI accounts, usage** (runs after 3, before 4) | Tickets (editing, on hold, queued); a queue that runs one ticket at a time as an AI task, checks the usage first, pauses at the limit and continues after the reset, or on another account when the owner says so (automatic switching is off by default); several accounts per tool, several tasks on different accounts at once, and a switch that never starts over without the conversation (the same session, a repaired copy, or a handoff note); the terms checkpoint; the usage meter. Claude Code and Codex alike (4.16, 4.17) | 8 |

Total: about 45 weeks (the reviewer's range was 28 to 32 before the ten tasks added at the milestone-0 gate; 35 before the owner's second batch; 43 before the owner's third batch). Run order: 0, 1, 2, 3, 8, 4, 5, 6, 7. The order follows dependency and daily value: updates first so every later build arrives with one click; then AI coding, the owner's biggest daily win; then the git client it needs for review; then the editor; then the new providers, which the owner's CodeCommit repositories do not depend on. Milestone 8 has the last number but runs right after milestone 3: it needs AI tasks (2) and task workspaces (3), and the owner reaches the weekly usage limit today, so the queue and the account switch pay off before the git client does.

---

## 8. Risks

| Risk | Mitigation |
|---|---|
| Scope: a git client, an AI task runner, an editor with code intelligence and a pull request client in one release | The cut order in section 6; milestones that each give the owner a usable build; spikes first. |
| An update installs something malicious or bricks the app | Signed manifest, version rules, code-signature requirement, atomic swap with rollback, keys off CI, rc test on clean VMs (4.14). |
| The signing identity is a company certificate | Gatekeeper shows "<owner company>" as the developer. Only the Account Holder can create a new certificate. Task: export the certificate and key as a password-protected `.p12`, stored in two places the owner controls. The updater's code-signature requirement pins the Team ID, so a move to another team needs one release signed by the old team whose manifest announces the new one; the second pinned ed25519 key exists for the same reason. |
| The Homebrew install gives root to a swapped file, or the caller's environment changes what root runs | The script is an argument, never a file; fixed minimal environment and `env -i`; bounded copy of a regular file from one descriptor into a root-only folder; verify and install that copy; refuse an existing `/opt/homebrew` (4.1). The privileged half has never run as root and no clean VM has been used yet: acceptance for F1-12. |
| Homebrew's package is signed by one person's Developer ID; if the signer changes, every new install fails | The Team ID is pinned as a constant; a different signer gets its own message ("update Codepass"); a normal release changes the value. |
| Real names leak to the public branch | Generic data everywhere; the publish check runs in every gate. |
| AI tasks overwrite each other | Worktree per task by default; warning when a second task starts in the main workspace. |
| The UI overstates what isolation protects | Plain copy: a task runs as the user. No badge that implies a safeguard. No option to skip permission prompts. |
| The AI pauses when the window is hidden | Pull-by-offset terminal path; App Nap activity; keep-awake (section 5, 4.10). |
| Background git breaks an AI's commit or runs repository programs | Read-only flags on every automatic read; watcher-driven refresh (4.6). |
| Worktrees pile up | The task list is the only place they are created and removed; size shown per task; "Clean up finished tasks"; orphan detection at startup; excluded from Time Machine. |
| Language servers eat memory (measured: gopls 0.3 to 1.3 GB, typescript-language-server 0.9 GB, native TypeScript 0.3 GB) | Idle and no-pull shutdown; 2 live servers by default (4 on large-memory machines); the native TypeScript server first; a hard limit on the message log; diagnostics held in fresh workspaces. |
| State detection is wrong | Hooks first; the fallback only says alive or exited; a tool without hooks shows "running (no signal)"; hard ceiling on keep-awake; states are advisory. |
| Programs a session started keep running unseen (CPU, battery, ports) | Kill by terminal session; the process tracker; "closed, still running" rows with Force stop; the quit dialog (4.8b). |
| Clean-up deletes something that is not the session's | Exact paths built from the stored session ID, previewed with sizes, re-checked before deletion; running sessions never eligible; OpenCode removes its own data; unmerged commits kept 14 days (4.8a). |
| Renaming a folder loses the repository's records or breaks task workspaces | Stable IDs; one journalled transaction that renames back on any failure; `git worktree repair`; same-disk moves only (4.4a). |
| Provider APIs and token rules change | Scopes measured for GitHub reads; ETag polling; token features degrade to "open in browser". |
| A GitHub organisation blocks classic tokens or requires SSO authorisation | Its own error and sentence; that organisation's pull requests open in the browser; the owner checks each organisation's policy (`manual-checks.md` A12). |
| Bitbucket allows about 1,000 requests an hour per personal token, with no "remaining" header | The client counts its own calls; "mine" per workspace; ETag; 15 minutes off screen. |
| The update swap is remove-then-rename; a crash in that millisecond window leaves no app | Accepted; `Codepass.app.bak` is the manual recovery, documented in the release notes (4.14). |
| Windows: no signing certificate, corporate machines without the Store, Codex's native Windows mode. **Nothing ran on Windows in milestone 0**: the ConPTY wrapper, the update gate, the three AI tools, winget and the language servers are all unproven there | Decision 3; a clear failure state for winget; a Windows 11 on Arm VM on the owner's Mac (amd64 under emulation) before milestone 1's gate and before A2-01; Windows 10 1809 on an Intel PC or a cloud instance (`manual-checks.md` B8 to B12) [S27]. |
| Upgrade from 0.0.1 loses data or hands off to the old app | Separate data files; old-app detection before the single-instance handoff (4.2). |
| Corporate proxy | System proxy for Go and for child processes (4.1). |
| The AI companies' terms may not allow what the queue or several subscriptions do | The quotes are in [U R3], [U R5d] and [U R9]; no clause on several subscriptions was found, and neither company was asked. The owner reads them and confirms before milestone 8 is built; the date and the decision are recorded (4.16 decision 35). Every sign-in stays inside the tool; the app never holds a credential (4.16 decision 31). "Ask me before each ticket" keeps a person in the loop. The release checklist re-reads the terms (section 10). |
| An account is limited or suspended because of how several accounts are used (the owner's worry, S2). Codepass cannot rule this out: the terms are silent on several subscriptions, both companies say they may suspend an account they judge in breach, Anthropic without notice, and press reports describe automatic filters [U R9] | What is in the app's hands (4.16 decisions 29 to 36): the owner's own accounts and work only, no sharing, reselling or proxy; unmodified official tools with their own sign-in, nothing hidden or disguised; each account inside its own plan; automatic switching off by default, turned on only after the owner read the sentences that bear on it; a refused or suspended account is never routed around; the app says once, plainly, that it cannot guarantee anything. What is not: how much the owner uses, and what the companies decide. |
| The app helps go around a ban without meaning to (an account is suspended and the next click moves its work to another account) | An account error stops everything for that account: no automatic switch, no retry, the queue pauses, the account is marked "Needs your attention", its sessions cannot be moved by the app until the owner cleared the mark, and an unclassified error at a turn end that is not about the conversation counts as an account error (4.16 decision 34). The signals are a closed list, so a refused conversation, a failed usage call or a network error never marks an account. |
| The usage reading breaks: Claude Code's `usage_report` is marked experimental, `/usage` in print mode is documented only for the SDK, and the Codex call was never run | One small adapter per tool with a recorded sample as its test; an unreadable answer is "unknown", never a guess; the limit signal works without the meter; R1-M1 and R5-M1 are acceptance (4.16). |
| A conversation does not continue under another account (not verified for either tool; the service drops Claude's hidden reasoning in another account; a Codex session file records its creator account and its encrypted parts may not open there) | Three layers, tried in order: the same session, a repaired copy, a handoff note built locally. No path starts over without the conversation; if no session can start on the other account, the switch does not happen (4.16 decisions 22 to 28). The owner is told which layer carried it and exactly what was left out. R2-M2, R5-M4 and R8-M1 to M10 are acceptance. |
| The owner expects "nothing lost" to include what the AI was thinking | Said plainly before the switch: the hidden reasoning of the newest models belongs to the account that produced it and no tool can move it; everything visible carries (4.16 decision 22). The result line never says more than the layer delivered (decision 26). |
| A repaired copy is written in a file format both tools call internal; a tool update changes it, and a bad copy confuses the tool | A version check and fixtures per tool version; a file the reader does not fully understand is never rewritten (the handoff note is used instead); the copy is always a new file under a new ID and the original is never changed, so the worst case is one unused file that "Free up space" lists (4.16 decisions 22 and 27). |
| A handoff note or a conversation copy leaks conversation text | Mode 0600 (folders 0700) whatever the original's mode; the note travels as a file path, never as a command-line argument; every path recorded before it is written; never in Codepass's backups, an export, the log, a notification or an error message; `sessions/` excluded from Time Machine; listed and removed with the task's data (4.8a, 4.16 decision 27). |
| Two processes write one conversation (a session under A and under B at once) | Nothing starts under B until the process tracker confirms the tool under A is gone; one queue session at a time; never the same ticket on two accounts (4.16 step 3 and decision 32). |
| Two things continue the same ticket after a reset (Claude Code's own automatic continue and the queue) | One owner of "continue" per case; the app reads the tool's setting and never sends a second "continue" (4.17). |
| The queue spends money: usage credits keep a tool running past the limit | Off for the queue by default; the meter shows "credits are on"; the queue stops at the next turn end; the limit of that is stated (4.17). |
| A ticket is judged finished too early, or never | The marker line plus "anything unclear waits for the owner"; "Mark done" and "Skip" are always there (4.17). |
| The owner's shell profile overrides the account, or moves helper calls to API billing | The names-only probe at start and the lines it produces (4.16, 4.7). |
| Codex hooks need a trust review and may not be suppliable per launch | `manual-checks.md` A8; without hooks a Codex ticket is judged by turn-complete and the exit code, and more of them end in "needs you" (4.8, 4.17). |

---

## 9. Decisions

**Settled by the owner:** name; platforms; Homebrew and winget mandatory; three AI tools installed on first use; worktree per task by default; the full git list; pull requests in the app; code intelligence for Go and TS/JS, and for PHP (added 2026-11-16); motion; Developer ID signing with the <owner company> team (confirmed 2026-11-16); one release from the `codepass` branch.

**Defaults this plan assumes unless the owner says otherwise:**

- Each AI tool always starts in its own approval or sandbox mode; the app has no option to skip permissions. The "start with this repository's AWS account selected" setting exists and is off by default.
- The Mac may sleep after a task has waited 15 minutes for the owner, and `caffeinate -d` (on by default, as asked) keeps the screen from locking while it is held.
- Users of 0.0.1 on Intel Macs or macOS 13-14 stay on 0.0.1.

**Decided by the owner (2026-11-16):** the four decisions, in `tasks.md` "Owner decisions (FINAL)". **Still open:** the six questions listed right below them, and the manual checks in `manual-checks.md`.

---

## 10. Documents to rewrite before release

- **README (en and zh-CN):** the lines "no update checks", "nothing secret is stored", "runs only aws, git...", "talks only to AWS and your repositories" are no longer true. New text: checks GitHub for a new version once a day (can be turned off); tokens are stored in the system keychain and SSH keys the app creates in `~/.ssh`; the app runs aws, git, ssh, brew or winget, the AI tools and language servers the user installs, and programs the user starts in a terminal; it talks to AWS, the user's git hosts, GitHub for updates, and the GitHub and Bitbucket APIs. Also: the 0.0.1 note for Intel and macOS 13-14, the Documents permission prompt, and that Homebrew's analytics is turned off for commands the app runs.
- **SECURITY.md:** supported versions (1.x) and scope (what an AI task can do; what the updater verifies).
- **THIRD_PARTY_NOTICES:** xterm.js and its addons, CodeMirror packages, the PTY libraries (creack/pty; go-pty's licence for the code the ConPTY wrapper is derived from), the Markdown renderer, the LSP client and what it brings (`marked`, `vscode-languageserver-protocol`, `vscode-jsonrpc`), the sanitizer.
- **Tutorial and illustrated guide:** rewritten for Codepass with generic data.
- **AI terms (release checklist, every release that ships milestone 8 or changes it):** re-read both companies' AI terms on the live pages (the six pages and the sentences of [U] R9-M1), note each page's date, and compare with the quotes in 4.16 "Several accounts within the terms". A changed or missing sentence stops the release until 4.16, the automatic-switching confirm and the add-account sheet are brought up to date. The date of the reading goes into the release notes file.
- **README and guide, AI accounts:** one paragraph in the words of 4.16 decisions 30 and 31: Codepass is for the owner's own accounts and the owner's own work; it starts the official tools with their own sign-in and stores no AI sign-in; it cannot guarantee that an account will not be limited or suspended.
- **`docs/design/ui-spec.md`:** section 9's "no entrance animations" is replaced by the motion system; new sections from `ui-design.md`.

---

## 11. Sources

Primary sources fetched 2026-11-15 unless marked.

- [S1] Atlassian, "Using API tokens" (Bitbucket Cloud): git over HTTPS with the Bitbucket username or the static user names `x-bitbucket-api-token-auth` / `x-token-auth`; scopes; REST with Basic or Bearer. https://support.atlassian.com/bitbucket-cloud/docs/using-api-tokens/
- [S2] Atlassian blog: app passwords stop working 2026-06-09. https://www.atlassian.com/blog/bitbucket/bitbucket-cloud-transitions-to-api-tokens-enhancing-security-with-app-password-deprecation
- [S3] Atlassian, "Managing multiple Bitbucket user SSH keys on one device". https://support.atlassian.com/bitbucket-cloud/docs/managing-multiple-bitbucket-user-ssh-keys-on-one-device/
- [S4] GitHub Docs, "Managing multiple accounts": `ssh -i <key> -o IdentitiesOnly=yes`. https://docs.github.com/en/account-and-profile/how-tos/account-management/managing-multiple-accounts
- [S5] GitHub Docs, device flow for OAuth apps. https://docs.github.com/en/apps/oauth-apps/building-oauth-apps/authorizing-oauth-apps
- [S6] Homebrew, "Installation": the `.pkg` is Apple silicon only; macOS 15 minimum; no sudo after setup; non-admin install into another folder. https://docs.brew.sh/Installation
- [S7] Homebrew `install.sh`: aborts on Intel macOS; `MACOS_OLDEST_SUPPORTED="15.0"`. https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh
- [S8] GitHub API, Homebrew/brew latest release: 7.0.13, `Homebrew.pkg`, 154,292,112 bytes. https://api.github.com/repos/Homebrew/brew/releases/latest
- [S9] Atlassian developer community: cross-workspace APIs removed 2026-04-14. https://community.developer.atlassian.com/t/bitbucket-cloud-announcing-end-of-life-for-cross-workspace-apis-timeline-next-steps-and-instructions-for-connect-apps/99972
- [S10] Claude Code docs, "Advanced setup": install methods, requirements. https://code.claude.com/docs/en/setup
- [S11] Claude Code docs, "Run parallel sessions with worktrees": `--worktree`, `.worktreeinclude`, manual `git worktree add`. https://code.claude.com/docs/en/worktrees
- [S12] OpenAI Codex CLI docs. https://learn.chatgpt.com/docs/codex/cli . `notify` (one event, `agent-turn-complete`, payload as an argument), `-c key=value`: https://learn.chatgpt.com/docs/config-file/config-advanced . Lifecycle hooks (14 events, a trust step): https://learn.chatgpt.com/docs/hooks . Read 2026-11-16, not run.
- [S13] Claude Code docs, "Hooks": `Notification`, `Stop`, `SessionStart`, `SessionEnd`, `UserPromptSubmit`; `--settings`. https://code.claude.com/docs/en/hooks
- [S14] `man caffeinate` on the owner's Mac: `-d`, `-i`, `-w pid`.
- [S15] Microsoft Learn, `SetThreadExecutionState`. https://learn.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-setthreadexecutionstate
- [S16] GitHub API, `rudian/aws_sso_hub` latest release: v0.0.1 and its assets; 60 unauthenticated requests per hour. https://api.github.com/repos/rudian/aws_sso_hub/releases/latest
- [S17] Wails v3.1.0 source in the local module cache, `pkg/updater` and `pkg/application` (files listed under [R]). Docs: https://v3.wails.io/guides/updater
- [S19] xterm.js issue #4445: needs `style-src 'unsafe-inline'`. https://github.com/xtermjs/xterm.js/issues/4445
- [S20] `github.com/aymanbagabas/go-pty`. https://github.com/aymanbagabas/go-pty
- [S21] CodeMirror `EditorView.cspNonce`. https://discuss.codemirror.net/t/csp-nonce-value-check/7200
- [S22] Apple, Developer ID: only the Account Holder creates Developer ID certificates. https://developer.apple.com/support/developer-id/
- [S23] GitHub Docs, managing personal access tokens: a fine-grained token covers one user or organization; approval; SAML. https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/managing-your-personal-access-tokens
- [S24] GitHub Docs, permissions required for fine-grained tokens: merging a pull request needs Contents: write. https://docs.github.com/en/rest/authentication/permissions-required-for-fine-grained-personal-access-tokens
- [S25] git-push documentation, `--force-with-lease` and `--force-if-includes` (git 2.30+): a background fetch defeats a lease without an expected value. https://git-scm.com/docs/git-push
- [S26] OpenCode plugins. https://opencode.ai/docs/plugins . OpenCode 2.x (run 2026-11-16, version 2.3.0): config key `plugins`, `Plugin.define({id, setup})`, `ctx.event.subscribe()`; per-launch through `OPENCODE_CONFIG_CONTENT`; launched with `--standalone`. Observed events: `session.execution.started` and `.failed`; `session.idle` and `permission.asked` are names in the binary, not observed.
- [S27] Codex on Windows. https://developers.openai.com/codex/app/windows . Third-party sources disagree on stability; not verified in milestone 0 (`manual-checks.md` B10).
- [U] `docs/research/claude-usage-accounts.md` (2026-11-16): usage, several accounts and the queue for Claude Code and Codex. Its section names (R1 to R9) and check names (R1-M1 and so on) are what 4.16 and 4.17 point to. R8 (carrying a conversation to another account) and R9 (terms and ban risk, with full-sentence quotes) were added for the owner's third batch. Main pages behind it: Claude Code docs on the status line, hooks, authentication, environment variables, the `.claude` directory, sessions, interactive mode, errors, headless mode, the Agent SDK TypeScript reference (https://code.claude.com/docs/en/...) and legal and compliance (https://code.claude.com/docs/en/legal-and-compliance); Anthropic's Consumer Terms and Usage Policy; the help-centre articles on usage limits and on the Agent SDK with a plan; Codex docs on the app-server, authentication, configuration, hooks and non-interactive mode (https://learn.chatgpt.com/docs/...) and the `openai/codex` source. For R8 and R9 also: Anthropic's API page on preserved thinking, the Claude Code changelog, Anthropic's help-centre articles on the Team plan, phone verification and appeals, the `openai/codex` source at one commit and four of its public issues, and two press reports (marked as third-party in the note). Nothing in it was run; Codex was not installed; OpenAI's policy pages could not be read directly and were read from Internet Archive copies dated 2026-11-14 to 2026-11-16.
- [N] `docs/research/notifications.md` (2026-11-16): Wails 3.1.0 notifications service read from source, Apple UserNotifications pages, Microsoft toast pages; section and limit numbers in 4.15 point into it.

**[R] Review evidence** (the principal engineer's findings; numbers are the review's item numbers):

- Repository: `main.go` (lines 29-43 run before `application.New`; 70 `ApplicationShouldTerminateAfterLastWindowClosed`; 76-77 single-instance ID), `internal/settings/settings.go` (122, 160, 294), `internal/app/runtimeguard.go` (21, 45: 4 MB cap, no chunked bodies), `internal/gitx/bind.go` (400-420: connection tests run outside the repository with `-c` overrides), `internal/envcheck/envcheck.go` (21, 24: hard minimums), `docs/progress/T-scrub.md`.
- Wails v3.1.0, under `pkg/`: `updater/providers/github/github.go` (131, 168-178), `updater/verify.go` (88, 134), `updater/validate.go` (367-388), `application/application.go` (786-788), `application/webview_window.go` (1542-1580), `application/event_payload_store.go` (41, 46), `application/roles.go` (68, 80, 156-160), `application/menuitem_roles.go` (179-198), `application/single_instance_darwin.go` (42).
- Checked locally: a "Developer ID Application" identity for the team is in the login keychain; the five brew packages in 4.1 exist (2026-11-16).

**[M0] Milestone-0 evidence:** `docs/research/spike-terminal.md` (S0-01), `spike-update.md` (S0-02), `spike-signals.md` (S0-03), `spike-installers.md` (S0-04), `spike-signing.md` (S0-05), `spike-lsp.md` (S0-06), `spike-providers.md` (S0-07), and the reviews `spike-review-a.md` and `spike-review-b.md`. Code: local branches `spike/terminal`, `spike/update`, `spike/signals`, `spike/installers`, `spike/signing`, `spike/lsp`, `spike/providers` (never merged, never pushed).
