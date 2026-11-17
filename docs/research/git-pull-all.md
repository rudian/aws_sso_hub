# Git research for the third batch (2026-11-17): branch from any branch, sync counts, Pull all

Written by Fable (design) for plan 4.6a and design 4.2, 5.2, 5.2b and 5.8; the planner applied the design and the task split (tasks G4-01, G4-02, G4-06, G4-07, G4-11, G4-12, G4-13). Section 1 is the source [G3] of plan 4.6a; G4-11 turns its rows into tests.

The design text went into `product-plan.md` 4.6a (with sources S28 to S35 and G3) and into `ui-design.md` (sections 2, 3, 4.2, 5.2, 5.2b, 5.8, 6, 7 and 8).

## 1. Git facts that were checked

Method: the pages of https://git-scm.com/docs were downloaded on 2026-11-17 (they describe git 2.56) and the commands were run on throwaway repositories with **git 2.56.1** on macOS (a bare "server", a seed clone that pushed, a working clone `acme_api`, one extra worktree, and hooks that append to a log). Global and system git config were switched off for the runs. The throwaway repositories were deleted afterwards.

**Not checked on an old git.** The app's minimum is git 2.44 (`internal/gitx` `MinVersion`, FU-04a). The design uses nothing newer than 2.35, but G4-11 must repeat the runs on git 2.44. Two things I deliberately did not rely on because they need 2.41: `git fetch --porcelain` and `%(ahead-behind:<ref>)`. With the 2.44 minimum, `--porcelain` gives the old and new hash per ref in one stable line and is the better way for G4-11 to read results.

| # | Command or situation | What git did (2.56.1) | Documentation |
|---|---|---|---|
| 1 | `git fetch --prune <remote>` | Removed the remote-tracking ref of a branch deleted on the server; updated the others. | "Before fetching, remove any remote-tracking references that no longer exist on the remote." https://git-scm.com/docs/git-fetch |
| 2 | `git for-each-ref --format='%(upstream:track)' refs/heads` after 1 | `[behind 1]`, `[ahead 1]`, `[ahead 1, behind 1]`, empty when equal, `[gone]` for the deleted remote branch, empty upstream for a branch with none. | "`:track` to show "[ahead N, behind M]" ... also prints "[gone]" whenever unknown upstream ref is encountered." https://git-scm.com/docs/git-for-each-ref |
| 3 | `%(upstream:remotename)`, `%(upstream:remoteref)` | `origin`, `refs/heads/feat-a`. | "refer to the name of the remote and the name of the tracked remote ref" (same page) |
| 4 | `%(worktreepath)` | The folder for a branch checked out in the main folder **and** for one checked out in a linked worktree; empty otherwise. The page says "if it is checked out in any linked worktree"; the run shows the main folder is reported too. `%(HEAD)` is `*` only for the branch of the folder the command runs in. | same page |
| 5 | `git fetch . refs/remotes/origin/feat-a:refs/heads/feat-a`, branch not checked out, remote ahead | Fast-forwarded the local branch, exit 0. No working folder touched. | "an attempt is made to update the local ref ... Whether that update is allowed without --force depends on ... whether the update is considered to be a fast-forward." https://git-scm.com/docs/git-fetch |
| 6 | The same when both sides moved | Rejected (`!`), exit 1, branch unchanged. | same passage |
| 7 | The same when only the local side has new commits | Also rejected (`!`), exit 1. So the app must classify with row 2 first and never call this for "ahead only"; otherwise a harmless state looks like an error. | same passage |
| 8 | The same for the branch checked out in the main folder | `fatal: refusing to fetch into branch 'refs/heads/main' checked out at '<path>'`, exit 128. | "By default git fetch refuses to update the head which corresponds to the current branch." (`--update-head-ok`) https://git-scm.com/docs/git-fetch |
| 9 | The same for a branch checked out in **another worktree** | The same fatal message with that worktree's path, exit 128. | Release notes of git 2.35.0: "git fetch without the --update-head-ok option ought to protect a checked out branch ... only checked the branch that was checked out in the current worktree, which has been updated." https://github.com/git/git/blob/master/Documentation/RelNotes/2.35.0.adoc |
| 10 | `git update-ref refs/heads/task-1 <new> <old>` for a branch checked out in another worktree | **Succeeded** (exit 0). That worktree then showed a staged deletion nobody made. So `update-ref` is not a safe way to move branches here. | "stores the <new-oid> in the <ref> ... after verifying that the current value of the <ref> matches <old-oid>" (it checks nothing else). https://git-scm.com/docs/git-update-ref |
| 11 | `git merge --ff-only origin/main` in a clean folder | Fast-forward, files updated, exit 0. | "With --ff-only, resolve the merge as a fast-forward when possible. When not possible, refuse to merge and exit with a non-zero status." https://git-scm.com/docs/git-merge |
| 12 | The same with an uncommitted change to a file the update also changes | `error: Your local changes to the following files would be overwritten by merge: a.txt ... Aborting`, exit 1, nothing changed. | "git pull and git merge will stop without doing anything when local uncommitted changes overlap with files that git pull/git merge may need to update." (same page) |
| 13 | The same with uncommitted or staged changes to other files, and untracked files | Git fast-forwarded anyway, exit 0; the changes stayed. So "leave a folder with changes alone" is the app's rule, not git's (open point 22 of the design). | same passage |
| 14 | `git merge --ff-only` while a merge with conflicts is unfinished | `fatal: Exiting because of an unresolved conflict`, exit 128. | https://git-scm.com/docs/git-merge |
| 15 | `git fetch . <src>:<dst>` for another branch while a merge is unfinished | Worked, exit 0. The design still moves no branch of a busy repository (simpler to explain, and the rebase case was not run). | not stated in the docs; from the run |
| 16 | Detached checkout in the main folder | No branch has `%(HEAD)` or a worktree path; `git fetch . origin/main:main` is then allowed. | from the run |
| 17 | `post-merge` hook on a fast-forward merge | Ran (argument `0`). | "This hook is invoked by git-merge, which happens when a git pull is done on a local repository ... cannot affect the outcome of git merge". https://git-scm.com/docs/githooks |
| 18 | `reference-transaction` hook | Ran on `git fetch <remote>`, on `git fetch . src:dst` and on the merge (preparing / prepared / committed). | "This hook is invoked by any Git command that performs reference updates." (same page) |
| 19 | `-c core.hooksPath=<empty folder>` | No hook ran (same way the background fetch in `internal/repos/live.go` does it). | "the hooks directory ... can be changed via the core.hooksPath configuration variable" (same page) |
| 20 | `git branch new-a origin/feat-a` | "branch 'new-a' set up to track 'origin/feat-a'": the new branch follows its base. | "The default option, true, behaves as though --track=direct were given whenever the start-point is a remote-tracking branch." https://git-scm.com/docs/git-branch |
| 21 | `git branch --no-track new-b origin/feat-a`; `git branch new-c <local branch>` | No upstream in both cases. | `--no-track`, same page |
| 22 | `git merge-base --is-ancestor A B` | Not run; documented: "exit with status 0 if true, or with status 1 if not". Not needed by the design, because `git fetch . src:dst` does the check itself. | https://git-scm.com/docs/git-merge-base |

**Not verified, marked so in the design or left to the task:**

- What `git merge` exits with when `post-merge` fails. The design has a line "the repository's own script reported an error"; the task must find out how to detect it (exit status or the hook's output).
- An untracked file that the update would overwrite: I expect git to refuse as in row 12; not run. The line "a file in the folder is in the way" covers it.
- `git fetch .` with several refspecs in one call when one of them is rejected. Until tested: one call per branch.
- Whether git treats the branch being rebased as "checked out" for row 9. The design avoids the question (a busy repository gets a fetch only).
- Stopping a `git fetch` process half-way. Believed safe; the task tests cancel on a large fetch and checks `git fsck` and that no lock file is left.
- The names git gives for an expired AWS sign-in or a refused key: that classification is G4-01's ("auth classification from recorded errors").
- Bitbucket: the Atlassian page (https://support.atlassian.com/bitbucket-cloud/docs/branch-a-repository/) confirms that the site creates the branch on the server and that one then runs `git fetch && git checkout <feature>`. It does not describe the "from branch" field; that is from the owner's words.

## 2. Mockup screens to draw later (E)

All 1440x900, generic data (section 7 of the design plus the list in block 9).

| Shot | Screen id / state | What it shows |
|---|---|---|
| `projects-counts-light-zh` | `projects`, state `counts` | The Projects page with the pills in every form: `↓4 ↑2 · 5 个改动`, `↑1`, "未发布", "远程分支已删除", "干净", one stale muted `↓3` with the clock mark, one row with "另有 2 个分支可更新", a project header with "3 个仓库有新提交", and the new "拉取全部" button in the header. One tooltip open on `↓4` with the freshness line. |
| `projects-counts-dark-en` | same, dark, English | The same, to check the info pill and the muted pill on slate. |
| `pull-all-start-light-zh` | `pull-all`, state `start` | The start popover under the header button: 12 repositories, 31 branches, the script checkbox "3 个仓库有", the "没算进去" line. |
| `pull-all-light-zh` | `pull-all`, state `running` | The bar "第 5 / 12 个仓库 · 已更新 7 个分支" with Details and Cancel; rows in each state: queued, fetching, updating 3 branches, running the repository's own script, done with "更新了 2 个分支", done with "已是最新", done with "1 个分支需要你看一下"; the header button as "正在拉取… 5/12". |
| `pull-all-dark-en` | same, dark, English | The same. |
| `pull-all-signin-light-zh` | `pull-all`, state `signin` | The bar with two waiting lines (AWS portal `initech` with 5 repositories; SSH key of `alex-dev` with 2) and their buttons; waiting rows read "等待登录". |
| `pull-all-done-light-zh` | `pull-all`, state `done` | The warn summary bar "更新了 9 个分支，5 个需要你看一下" and the results sheet open: Needs a look (both sides moved; uncommitted changes; AI task in the folder; branch in a task's workspace with new commits; unfinished merge; network failure), Updated (one not checked out, one checked out "文件夹里的文件也更新了", one with the script error), Nothing to do collapsed, with the deleted-remote block still visible; the footer "重试没成功的 3 个仓库". |
| `pull-all-done-light-en` | same, English | The same, to check line lengths of the English sentences in a 520px sheet. |
| `pull-all-clean-light-zh` | `pull-all`, state `clean` | The ok summary "全部都是最新的。" with the check, sheet closed. |
| `pull-all-cancelled-light-zh` | `pull-all`, state `cancelled` | The bar "已取消。12 个仓库处理了 5 个，更新的分支保持更新。" |
| `new-branch-light-zh` | `new-branch` | The dialog from a local base: name, "基于 main" with the commit line, both checkboxes, prefix chips. |
| `new-branch-pick-light-zh` | `new-branch`, state `pick` | The picker open with a search text: groups Local / Remote origin / Tags, pills on local branches, ages on the right. |
| `new-branch-remote-light-zh` | `new-branch`, state `remote` | Base `origin/release/2.4` with "3 分钟前获取的 · 先获取", "同时发布到 origin" ticked, primary "创建并发布". |
| `new-branch-behind-light-zh` | `new-branch`, state `behind` | Base local `main` with the warn line "比 origin/main 少 4 个提交" and "改用 origin/main". |
| `repo-branches-light-zh`, `repo-branches-light-en` (re-take) | `repo-branches` | The existing shots with pills per local branch, "未发布" and "远程分支已删除" rows, the hover button "从这个分支新建" on one local and one remote row, the freshness text on the Remote heading. |
| `workspace-sync-light-zh` | `workspace-code`, state `sync` | The header with the Sync button showing `↓4 ↑2 同步`, its tooltip, the caret menu with "拉取全部分支", and the switcher menu open with the two branch items at its foot. |

## 3. Who builds what in milestone 4 (F)

Proposal. Three new tasks; four existing tasks grow a little. My estimate: milestone 4 goes from 5 to about 6.5 weeks (the two Pull all tasks are the bulk). The planner decides.

**Existing tasks**

- **G4-01 · Sync** (backend). Adds: (a) one shared "network action" runner used by fetch, pull, push, publish and Pull all: no-prompt environment, the auth-failure classification, a per-repository lock so one repository never runs two network actions, a no-progress timeout; (b) `Publish(branch, remote)` reused by the New branch dialog; (c) a method to set the per-branch "last integrated remote commit" that Pull all calls for branches it moved (the lease rule of plan 4.6). No Pull all logic here.
- **G4-02 · Refs** (backend). Adds: (a) the branch list carries, per local branch, upstream name, remote name, ahead, behind, gone, and the folder it is checked out in (one `for-each-ref`, read-only runner); remote branches carry the age of their last commit; (b) `CreateBranch(name, base, switch, publishTo)`: base may be a local branch, a remote branch, a tag or a commit; always `--no-track`, except "same name as the remote branch"; name check with git's own rule (`git check-ref-format --branch`, https://git-scm.com/docs/git-check-ref-format; exists on that page, not run by me); publish goes through G4-01; (c) "how far is local X behind its remote" for the dialog's warn line. Tests: the tracking table (rows 20 and 21 above), the same-name exception, create inside a task folder without switching.
- **G4-06 · Sync button** (UI). Uses the shared `SyncCount` pills and tooltip from G4-13; adds "拉取全部分支 / Pull all branches" to the caret menu (calls G4-11 for one repository and shows its result as a toast with "查看结果"); the waiting text while Pull all holds the repository. Accept adds: the tooltip shows the freshness line in each of its four forms.
- **G4-07 · Branches tab** (UI). Adds: pills and the two word states per local branch; "从这个分支新建" on every local and remote row (button, context menu, key N); the New branch dialog with the `.base-pick` picker (it lives here: `frontend/src/workspace/branches/NewBranch*.tsx`, exported so History's "Branch from here" and the header's branch menu open the same dialog); freshness text and Fetch on the Remote heading; "更新到远程最新" on a behind branch. Accept adds: matches `new-branch-*` shots. Review adds: a new branch from a remote base never follows the base.
- **A-something (workspace header, milestone 2 or 3, whoever owns `.tree-switch`)**: the two items at the foot of the switcher menu and pills on its rows. Small; can ride in G4-06 if the planner prefers, with ownership of that one file handed over.

**New tasks**

- **G4-11 · Pull all, backend** (L). Build `internal/gitpullall`: the plan step (which repositories and branches, what is left out, which have a post-merge script), the run (4 repositories at once; per repository: look, fetch with `--prune`, look again, per-branch decision table of plan 4.6a), waiting groups per sign-in or key with resume after sign-in, cancel, per-repository retry, progress events, the result model (one record per repository and per branch with a case from a fixed list, old and new hash, counts). Owns `internal/gitpullall/`, `internal/app/pullallservice.go`, mock `pullall.ts` (scripted to reproduce the `pull-all-*` shots). Deps: G4-01 (runner, classification, lock), G4-02 (branch list). Accept: on a fixture set that reproduces every row of the case table, each branch ends in the expected case and no branch other than the "moved forward" ones changed; a branch checked out in a task workspace is byte-for-byte untouched; nothing is ever pushed (the fixture server's hooks record any push). Tests: rows 5 to 16 of the table above as table tests; the lease value after a run; cancel during a fetch; resume after a simulated sign-in; hooks: the log of a `reference-transaction` hook stays empty for fetch and ref-only moves, and `post-merge` runs once for the checked-out branch and not at all with the switch off. Review: no `update-ref`, no `--force`, no `+` refspec, no `--update-head-ok`, no stash, no `merge` without `--ff-only`, no `rebase`, no `push` anywhere in the package (a grep test enforces it).
- **G4-12 · Pull all, UI** (L). Design 5.2b: header button, start popover, bar with row states, sign-in lines, summary, results sheet, the three other entry points on the Projects page, toast and notification text. "Fable first": draws the `pull-all-*` screens in the mockup, takes the shots, then builds. Owns `frontend/src/pages/projects/pullall/`, locale `pullall.ts`; touches `ProjectsPage.tsx`, `ProjectRow.tsx`, `ProjectSection.tsx` (the task must be given these three files for the run, since milestone 1 built them). Deps: G4-11, G4-13, the notify service (`internal/notify`) for the one new kind. Accept: matches the shots; the whole flow works with the keyboard only; with a screen reader the start, each sign-in line and the end are announced once. Tests: the case table renders every line in both languages; the summary sentence matrix; focus return; one run at a time. Review: no raw git term in any string ("fast-forward", "upstream", "HEAD", "worktree", "hook", "prune" never appear); branch names and git messages render as plain text.
- **G4-13 · Counts and freshness** (M). (a) Backend: `internal/repos/live.go` and `internal/app/loops.go`: last successful fetch time and last failure reason per repository in the status the page reads; the extra fetch triggers of plan 4.6a (window to front, workspace opened, network back, sign-in succeeded) with the 5-minute floor; 4 at once; the "other branches to update" list per repository. The target rule does not change (bound or opened). (b) Frontend: the shared `SyncCount` component (`frontend/src/components/SyncCount.tsx`, class `.sync-n`), its tooltip, the stale style; used by `ProjectRow.tsx`, the project header meta, "Open repository" rows; exported for G4-06 and G4-07. Locale `sync.ts`. "Fable first" for `projects-counts-*`. Accept: with the window in the back for an hour and then brought to the front, one round runs and the pills change without a click; a repository that is neither bound nor opened is never fetched (test with a remote that records fetches); the tooltip is right in all four freshness forms. Tests: trigger debounce; the pill matrix (0/0, n/0, 0/n, n/m, none, gone, detached, 99+, stale). Review: the background fetch still passes the empty hooks folder and the no-prompt settings.
- **G4-W · Wiring** gains the new service and events.

Order: G4-13 (b) can start at once (the Projects row has ahead and behind today). G4-11 after G4-01 and G4-02. G4-12 after G4-11's mock exists.

## 4. Planner decisions the owner may change

These are choices I made so the design is complete. Each is one switch to flip.

1. **Publish on create**: off by default, remembered per repository. Other choice: on by default (closer to Bitbucket).
2. **A new branch never follows its base** (always `--no-track`), except when it takes the remote branch's own name.
3. **Pull all covers every listed repository with a remote**, also ones that are not bound and were never opened. Other choice: only bound or opened ones.
4. **Hidden repositories are left out** of Pull all and named in the popover.
5. **Uncommitted changes stop the update of the checked-out branch**, whatever they touch (the clean-folder rule). Other choice: let git decide (it updates when the changes do not overlap).
6. **A live AI session in the main folder stops the update of the checked-out branch.** Not in the brief; added because files would change under the AI.
7. **A busy repository (unfinished merge and so on) is fetched but no branch of it moves.** Git would allow moving the other branches.
8. **Hooks**: off for the fetch and for branches that are not checked out; `post-merge` runs for the checked-out branch, default on, switch per run. Other choice: never in a bulk run.
9. **Pull all passes `--prune`**; the background fetch passes no pruning flag and follows the owner's own git setting. Effect of prune: remote-tracking refs of deleted remote branches go; local branches never do.
10. **4 repositories at once**, 2 minutes without progress is a failure, 5 minutes between background rounds, 2 minutes for "fetch on open", one hour until a `↓` number is drawn as stale.
11. **A start popover every time** for the list-wide run (it carries the counts and the script switch). Other choice: start at once, explain in the bar.
12. **No undo, no shortcut, no automatic clean-up of deleted branches** (open points 25 to 27 in the design).
13. **The result is not kept after the app quits.** A history of runs is not designed.
14. **Tags follow as with any fetch**; submodules are never updated by Pull all.
15. **`↓` pill in the info colour, `↑` neutral.** If the owner wants both the same, both become neutral.
16. **Wording**: "拉取全部 / Pull all" (the owner's words), "未发布 / Not published", "远程分支已删除 / Remote branch deleted", "需要你看一下 / Needs a look", "仓库自己的脚本 / the repository's own script" (for a hook).

## 5. Things the orchestrator should know

- **The product plan's git minimum** read 2.39.1 in 4.1 and 4.6; the planner changed both to 2.44 (FU-04a) when applying this batch.
- **The header "branch menu"** did not exist as its own control in design 4.2 (only the working-tree switcher). I put the branch items at the foot of the switcher's menu instead of adding a second control to the header.
- **`ProjectRow.tsx` today** renders `↑` before `↓`; the design puts `↓` first (what is waiting for you). G4-13 changes the order.
- **The "behind" status text** (`projects.status.behind`) stays.
- **Plan 4.15** has prose, not a table, for the kinds of notification; block 2 of the plan text is a bullet for that reason. The design table in section 6 gets the row.
- **Estimate**: plan section 0 says "about 45 weeks"; this batch adds about 1.5 weeks to milestone 4 if the planner accepts the three tasks.
