# Milestone 0 spike review B: S0-04, S0-05, S0-06, S0-07

Date 2026-11-16. Reviewer: Opus (review B). Read: `spike-installers.md`, `spike-signing.md`, `spike-lsp.md`, `spike-providers.md`, the four `spike/*` branches, and `docs/plan/codepass/product-plan.md` and `tasks.md` (milestone 0, F1/E5/P6 tasks, owner decisions). Nothing in the repository was changed except this file. Nothing ran as root, no password dialog, no install, no provider token.

## What I checked myself

- **Homebrew.pkg** (the spike's download in the session scratchpad): `shasum`, `pkgutil --check-signature`, `spctl -a -vv -t install`, `stapler validate` against the saved release JSON. All match the note: 7.0.13, 154,292,112 bytes, sha256 `f5dae55e...314e1`, Team ID `927JGANW46`, notarized, not stapled (stapler exit 65).
- **`spike/installers/test-verify.sh`** re-run: the same 8 rows and exit codes as the note (0, 15, 13, then 12 for flipped payload, flipped header, unsigned re-wrap, truncated file, HTML page).
- **`spike/installers/test-osascript.sh`** re-run: hostile path arrives intact, no injected file created, exit code arrives as `execution error: to-stderr (13)`, stdout is lost on failure, the administrator clause compiles.
- **`spike/installers/classify`**: `go vet`, `go test`, `GOOS=windows go build` pass.
- **New tests of the `osascript` -> `do shell script` -> `/bin/bash -c` path** (S0-04 issue 1).
- **Signed spike app** (`/private/tmp/claude-501/spk/`): `codesign -dvv`, `codesign -d -r-`, entitlements (none), the plan 4.14 requirement with `XC34P8TNZ6` (rc 0) and `WRONGTEAM1` (rc 3), the full designated requirement as `-R` (rc 0), `spctl -a -vvv -t exec` (Notarized Developer ID), `stapler validate`, `vtool -show-build` (minos 13.0 from today's Taskfile, SDK 27.2), notary JSON (Accepted, no issues). The OU of all three signing identities in the keychain (Developer ID Application, Apple Distribution, Apple Development) is `XC34P8TNZ6`.
- **lsp-client 6.3.0 source** as the spike installed it: Markdown goes through `marked.parse` and is assigned with `innerHTML` in three places; `sanitizeHTML` is optional; default request timeout 3000 ms; only `publishDiagnostics`, no `textDocument/diagnostic`.
- **LSP `initialize` against `frontend/`** (TypeScript 7.0.2 in `node_modules`): typescript-language-server 6.1.0 fails with "provides no tsserver.js. No other valid TypeScript installation was found. Exiting."; `tsc --lsp -stdio` from 7.0.2 starts and advertises `diagnosticProvider` (pull).
- **File counts**: 2,003 `.go` files in the spike's `go.work` (1,291 x/tools + 712 x/net); 138 in `internal/`; 184 `.ts/.tsx` in `frontend/src`.
- **The spike's bottle closure script** re-run: 27 formulae behind git + awscli, bottles for `arm64_sequoia`, `arm64_tahoe` and `arm64_golden_gate` on all of them, no post-install step.
- **formulae.brew.sh API**: `typescript-language-server` 6.1.0 depends on `node` and `typescript`; `typescript` is 7.0.2 with no runtime dependency (go and node are build-only); `gopls` 0.24.0 has no runtime dependency; casks `claude-code` and `codex` have only `binary`/`zap` artifacts.
- **winget manifest `Amazon.AWSCLI` 2.40.2** (public raw file): the same MSI URL is listed twice, once `Scope: machine`, once `Scope: user`.
- **Bitbucket `swagger.json`** the spike downloaded: `GET /user/workspaces` and `GET /workspaces/{workspace}/pullrequests/{selected_user}` exist and are not deprecated (S0-07 issue 1).
- **Leftovers**: the own-folder brew test folder is gone; the owner's real Homebrew cache and logs hold no spike entries; `$TMPDIR/lsp-spike-*.stderr` are still there; the spike branches are not on `origin`.

## Verdicts

| Spike | Verdict | In one line |
|---|---|---|
| S0-04 Installers | **Partly verified** | Every local claim reproduces. The acceptance run (clean macOS VMs without CLT, Windows) has not happened, and the privileged step has gaps the spike missed (environment, lost installer text, unbounded copy). |
| S0-05 Signing | **Solid** | Built, signed, notarized, stapled, verified; every recorded value re-checks. Clean-Mac double-click, a launch of the hardened build and the `.p12` export remain manual. |
| S0-06 Language servers | **Solid (macOS)** | Transport, backpressure, memory and TS-server findings are backed by runs; the two claims I re-ran reproduce. Windows install path, the real Wails IPC and WKWebView are open. |
| S0-07 Provider tokens | **Partly verified** | GitHub reads and ETag measured. Every write, every Bitbucket scope and all authenticated Bitbucket behaviour are documentation only, and the "no cross-workspace endpoint" conclusion is wrong. |

---

## S0-04 Tool installers: partly verified

### Three facts
1. Homebrew.pkg 7.0.13 is signed with an individual Homebrew maintainer's Developer ID, Team ID `927JGANW46`, notarized but not stapled. The certificate changed between 5.1.13 and 7.0.13; the Team ID did not. Pin the Team ID.
2. Copy from one open descriptor into a root-only `mktemp -d` folder under `/var/root` (0750 root:wheel, wheel = root only; checked), verify the copy, install the copy: no file-swap window. Damaged, re-wrapped, wrong-team, truncated and HTML downloads all stop before `installer` (re-run).
3. Own-folder Homebrew does not do what decision 1 hoped: a prefix longer than 13 bytes is Tier 3; git, openssl, gettext, pkgconf, ca-certificates and python were compiled from source (build logs recorded by the spike), which on a clean Mac needs Command Line Tools and so an administrator. With `/opt/homebrew`, all 27 formulae behind git + awscli have bottles for macOS 15, 26 and 27 and none has a post-install step (I re-ran the spike's closure script).

### Claims against evidence
- Reproduced or backed: the pkg facts, the verification exit codes, the `osascript` transport (no streaming, stdout lost on failure, code in the trailing parentheses), proxy failure strings (scripts kept), the own-folder compile, the brew names and dependencies, the winget manifest scopes.
- Stated carefully as not run: the real administrator prompt, `open -W`, a clean machine, a standard account, Windows, MDM, disk full, the `-60005`/`-60007` codes (from `Authorization.h`; the test strings for them are synthetic).
- Overstated in the summary only: "wrote and tested the privileged script". Only the verify half ran, unprivileged. The install half (`installer` as root) never ran. The note itself is accurate.
- The acceptance item "macOS 15 and 26 VMs without Command Line Tools install git and awscli with no developer tools dialog" is **not met**. It is the whole point of S0-04 and must run before F1-12 is accepted.

### Issues the spike missed (all for F1-12)
1. **The environment is a second way to change what root runs.** `do shell script` runs `/bin/sh -c`, which runs `/bin/bash -c '<script>'`. With the unprivileged form of the exact call I confirmed:
   - `BASH_ENV=<file>` on the `osascript` process: the file is sourced before the script's own `unset BASH_ENV`.
   - an exported function `BASH_FUNC_grep%%='() { return 0; }'`: the script's `grep` is replaced, so every signature check would pass.
   - `SHELLOPTS=xtrace` with a `PS4` holding `$(...)`: the command runs inside `/bin/sh` itself, even with `/usr/bin/env -i` in front of bash.
   Any process of the user can plant such variables for the next app launch (`launchctl setenv`). Whether `with administrator privileges` keeps the caller's environment was not tested (it needs the dialog); assume it does until the VM shows otherwise. Fix: start `osascript` with an explicit, minimal environment (`cmd.Env = ["PATH=/usr/bin:/bin:/usr/sbin:/sbin"]`, nothing inherited), run `/usr/bin/env -i /bin/bash --noprofile --norc -c` inside, and call every tool by absolute path. This is the same class of attack as the file swap the review question asks about.
2. **Installer text is lost on failure.** The script runs `installer ... 2>&1`, which puts it on stdout, and stdout is dropped when the script fails (the spike's own finding 4). `classify.macInstallerText` (disk full, "restricted", "blocked") will never see real text; its test case is synthetic. Send installer output to stderr, or put its last lines into the `fail` message.
3. **The copy is unbounded and accepts any file type.** Root opens whatever the path points to at that moment. A symlink to `/dev/zero` fills the disk under `/var/root`; a FIFO hangs the step. Copy at most expected size + 1 bytes, refuse a non-regular file (check the open descriptor), and make the size argument mandatory (the script accepts `-`).
4. **Gatekeeper turned off.** With assessments disabled, `spctl -a` is expected to answer `accepted` with `override=security disabled` and no `origin=` line, so the script would exit 14 ("not notarized") on a good package. Decide: `pkgutil` (signature, Team ID, notarization) is the gate; this `spctl` answer gets its own message or is skipped. Verify on the VM.
5. **Digest.** The note's plan edit says size and digest come from the API, but the script checks size only. Pass the sha256 in, or state that the digest is checked unprivileged before the prompt. If the GitHub API is unavailable (60 requests/hour per IP behind a shared NAT), fall back to the `releases/latest/download` URL and the signature check.
6. **The `xcode-select` rule exists already for git.** `internal/execx.FindGit` refuses `/usr/bin/git` unless `xcode-select -p` succeeds. Keep it, tighten it to "prints an existing path other than `/`", test it, and apply it to any other stub (`python3`, `make`, `clang`, `xcrun`).
7. **The pinned Team ID belongs to one person.** If Homebrew changes signer, every new install fails until Codepass ships an update. Add a risk row and a specific message ("Homebrew's package is signed by a developer this version of Codepass does not know; update Codepass"). Keep the value as a constant changed by a normal release; a separately updatable config adds a path for no gain.
8. **Own-folder nuance.** `/Users/<name>/h` fits in 13 bytes for user names of four characters or fewer. Not tested and not a general path; "not offered" stands.
9. **AWS CLI per user.** Both manifest scopes point to the same MSI, so a per-user install is doubtful. Keep "machine-wide, needs an administrator" until the Windows run shows `--scope user` works without UAC.
10. **The plan has no Windows install path for the AI tools.** 4.1 lists brew commands only. Add `winget install --id Anthropic.ClaudeCode -e` and `--id OpenAI.Codex -e`. OpenCode has no winget package (the note says "the plan routes it elsewhere"; it does not); choose its Windows source.

### Decisions
Privileged script embedded in the app, passed to `osascript` as an argument; `open -W` only as a fallback; own-folder Homebrew not offered; a standard account installs the pkg with an administrator's name and password; Team ID pinned, not the certificate fingerprint; "verify signature" shown as an unprivileged step before the prompt, because the privileged step cannot stream. I endorse all of them with issues 1-5 fixed. I would not build `open -W` at all in 1.0: its exit status cannot tell cancel from success and its swap safety is unknown. The failure text can say "open Homebrew.pkg yourself" instead.

### Rules
Kept. No root, no dialog, no `open -W`. The own-folder test used its own `HOMEBREW_CACHE` and `HOMEBREW_LOGS` inside a cache folder that is now deleted. GitHub and winget reads were anonymous. Nothing pushed, no secrets. Left in the session scratchpad: the 154 MB pkg, an expanded copy and a refused first prefix attempt.

---

## S0-05 Signing and notarization: solid

### Three facts
1. The arm64 build was signed with hardened runtime and a secure timestamp, notarized ("Accepted", "Ready for distribution", no issues), stapled and accepted by `spctl` as Notarized Developer ID; about 69 s end to end (notarization 27 s). Re-checked.
2. Team ID `XC34P8TNZ6`. The designated requirement is recorded verbatim. The plan 4.14 string passes with the Team ID filled in and fails (rc 3) with a wrong one. Re-checked.
3. No entitlements are needed to notarize. A launch of the hardened build was not verified (the single-instance guard handed off to the installed AWS SSO Hub), and "Open in terminal" (`internal/terminal` drives Terminal and iTerm through `osascript`) probably needs `com.apple.security.automation.apple-events` plus `NSAppleEventsUsageDescription`, which `build/darwin/Info.plist` lacks.

### Claims against evidence
Everything claimed was run and reproduces. The note marks the launch, the clean-Mac double-click and the `.p12` export as not done. One weak proxy: the "quarantined copy" check adds nothing, because `spctl --assess` answers the same with or without the attribute. The browser download on a clean VM is the real test.

### Review notes
1. **Use the full designated requirement in the updater, not the short string.** `anchor apple generic and identifier ... and certificate leaf[subject.OU] = "XC34P8TNZ6"` is also met by the team's Apple Distribution and Apple Development certificates, and the keychain holds both (OU `XC34P8TNZ6`, checked). The plan relies on `spctl -a -t exec` to reject such a bundle, but `spctl` answers "accepted" when Gatekeeper is off. This string passes on the notarized app (checked) and closes both gaps; review A reached the same conclusion from S0-02:
   `anchor apple generic and identifier "com.rudian.awsssohub" and certificate 1[field.1.2.840.113635.100.6.2.6] exists and certificate leaf[field.1.2.840.113635.100.6.1.13] exists and certificate leaf[subject.OU] = "XC34P8TNZ6"`
2. **Launch smoke test before F1-10 is accepted**: quit the installed app, open the signed copy, check the window, browser sign-in, keychain reads and "Open in terminal" (an Automation prompt should appear). If P6-01 picks the data-protection keychain, it needs `keychain-access-groups` and an embedded Developer ID provisioning profile; re-run notarization then. The file-based keychain item the plan describes needs no entitlement.
3. **Bundle ID**: already decided. Plan 1.1 keeps `com.rudian.awsssohub` and F1-01 pins it with a test. No new decision needed.
4. **For F1-10**: the spike script hard-codes the owner's home path (`WAILS3` default) and the company identity name; the release script takes both from the environment or a git-ignored local file (the scrub check would flag them). Parse `notary.json` without splicing a path into Python code. Give `notarytool --wait` a timeout.

### Decisions
Re-sign the Taskfile output with `--force --options runtime --timestamp`, executable first, then the bundle; `--deep` only to verify; notarize a `ditto` zip; staple the `.app`; build the release archive from the stapled app; no entitlements by default. Endorsed.

### Rules
Kept. Identity and notary profile referenced by name only; no secret in any file; nothing pushed. Two outward effects, both expected for this task: one notarization submission under the team, and a launch that handed off to the running AWS SSO Hub (it may have raised its window).

---

## S0-06 Language-server transport: solid (macOS)

### Three facts
1. Pull-by-offset works for LSP. With the page hidden or frozen, Go kept reading, the server answered probes in 0-4 ms, and on show the page caught up in 143-192 ms with the same final diagnostics as the visible control. A reader that blocks at 64 queued events (the Wails emitter behaviour, emulated) stalled gopls for 37 s.
2. typescript-language-server 6.1.0 does not start in a TypeScript 7 workspace (reproduced). The TS 7 native server `tsc --lsp -stdio` works with lsp-client at about 290 MB instead of about 930 MB, but reports diagnostics only by pull, which lsp-client 6.3.0 does not implement (confirmed in its source).
3. Memory: gopls 0.3-1.3 GB depending on cache state (1.1 GB on 2,003 files), typescript-language-server with TS 5.9 0.93 GB, native TS 0.29 GB. lsp-client renders server Markdown with `marked` and `innerHTML`, so a sanitizer (or raw HTML off) is mandatory.

### Claims against evidence
The tables have no saved raw output; the evidence is the note plus reproducible drivers (`spike/lsp/test/*.mjs`, run lines in `build.sh`). The `$TMPDIR/lsp-spike-*.stderr` logs confirm the runs and the tsls fallback. Where I re-ran or read source, every claim held. The accept items are met for Go (2,003 files), TS (184-file repository; the task does not ask for 2,000 TS files) and memory; "Windows install path confirmed" is open. "brew install typescript-language-server would be broken out of the box" is true for TS 7 workspaces and for workspaces without `node_modules`, not for projects that carry TS 5 or 6 in `node_modules`.

### Review notes
1. **Endorse the native TS server, with a selection rule.** Workspace `node_modules/typescript` at 7 or above, or none (a fresh task workspace) -> `tsc --lsp -stdio`. TS 5.x or 6.x -> typescript-language-server, which then uses the workspace's own `tsserver.js`. On macOS `brew install typescript` gives the native server with no runtime dependency (formula API), so the TS server needs no Node on macOS; typescript-language-server (and node) is installed only when a TS 5/6 project is opened.
2. **The cap is not a hard bound.** Compaction drops only `$/progress`, log messages and superseded diagnostics. Newest diagnostics per file, responses and unanswered server requests stay, so many files with errors or an unanswered `window/showMessageRequest` grow past it. E5-03 needs a hard limit with a defined action (restart the server, re-open documents).
3. **The send path must never drop.** The spike's `write()` drops a client message when its 4,096-slot queue is full; a lost `didChange` desynchronises the server silently. Block the bound method or fail the session.
4. **The ping must never block the reader.** In Wails the "new data up to N" ping is itself an event; send it from its own goroutine, coalesced, with at most one pending. The SSE stand-in did this by construction, so the spike does not prove it for Wails.
5. **Requests the hidden page cannot answer.** Besides the four the spike answers in Go, decide `window/showMessageRequest` (gopls uses it, for example, for its telemetry opt-in; the plan has no telemetry) and `workspace/diagnostic/refresh` (native TS).
6. Delete `$TMPDIR/lsp-spike-*.stderr`; they hold server log text with local paths.

### Decisions
Pull-by-offset as a message log with offsets (not a byte log), cap and compaction, `gap` and `exited` flags, Go-side answers, serial batched send queue; native TS server first; idle shutdown mandatory and the cap of 4 only with it (default 2, 4 on large-RAM machines); lsp-client usable with its own Workspace class, pull diagnostics for TS and a real sanitizer. Endorsed with notes 1-5.

### Rules
Kept. gopls and npm packages in temporary prefixes, `frontend/node_modules` in the worktree only (git-ignored), headless Chrome with a temporary profile, nothing pushed.

---

## S0-07 Provider tokens: partly verified

### Three facts
1. One classic PAT with `repo` + `read:org` covers every GitHub endpoint the app needs, checks included (check runs plus combined status, or GraphQL `statusCheckRollup`). `workflow` is not needed because git goes over SSH. Measured on reads only; private check runs were seen only as an empty list.
2. GitHub ETag: `If-None-Match` returned 304 on the PR list and detail without using quota. Search (30/min) and GraphQL have no ETag. A missing classic scope returns 404 "Not Found"; the only clue is `X-OAuth-Scopes` against `X-Accepted-OAuth-Scopes`. A bad token returns 401 "Bad credentials".
3. Bitbucket: Bearer API token; personal tokens get about 1,000 requests/hour with no remaining-count header. The five scopes (`read:user`, `read:workspace`, `read:repository`, `read:pullrequest`, `write:pullrequest`) are the spike's own mapping from swagger OAuth names, never tried on a real token.

### Claims against evidence
GitHub read rows, ETag and error shapes were observed (headers only, no token printed). Write rows, fine-grained rows, rate-limit exhaustion and all Bitbucket scope rows come from documentation, as the table says. The summary states the Bitbucket scopes and "write:pullrequest covers close_source_branch" as facts; the note correctly calls them unconfirmed. Treat them as unverified until manual step 3.

### Review notes
1. **Bitbucket has the cross-workspace endpoints the app needs.** The spike's own `swagger.json` lists `GET /user/workspaces` ("an object for each workspace accessible to the caller") and `GET /workspaces/{workspace}/pullrequests/{selected_user}` (all pull requests a user authored in a workspace, with `state` and `q` filters); neither is deprecated. Only `/workspaces` and `/user/permissions/workspaces` are gone. So the app can list the user's workspaces from the token instead of asking for slugs (pre-fill from remotes stays as a fallback), and "my pull requests" is one call per workspace, not per repository. "Waiting for my review" stays per repository (`q=reviewers.uuid="{uuid}"`, to verify). This changes plan 4.3, design 5.11, P6-04, P6-07 and the 1,000/h budget.
2. **Organisations can block classic tokens.** GitHub lets an organisation restrict access by personal access tokens (classic); requests for its repositories then fail with 403 (documented by GitHub; not reproduced). Under decision 2 that organisation can only open in the browser. Add it to the error rules, the add-account text and the risk table; the owner should check the policy of each organisation he works in. SAML organisations need "Configure SSO" per token (403 with `X-GitHub-SSO`), already in the note.
3. Atlassian API tokens are created with an expiry date; make "token expired, paste a new one" a routine state.
4. The note's "product-plan.md 5.11" is `ui-design.md` 5.11. The product-plan text is 4.3 (lines 217-218), 4.13 (line 329) and the risk row at line 437.
5. Method: the spike used the owner's own signed-in `gh` session (a classic token with `admin:org`, `repo`, `workflow`) on private organisation repositories, not a test account. Read-only, nothing named in the note, no token printed. Acceptable; the owner should know.

### Decisions
Decision 2 workable; fine-grained tokens dropped; checks from check runs plus combined status; polling with ETag where available; Bitbucket five scopes pending confirmation; missing-scope detection from the scope headers. Endorsed with notes 1-3.

### Rules
Kept. GET requests only; Bitbucket anonymous; no token created, printed or stored; nothing created, merged or deleted on either provider.

---

## Cross-cutting

- **Decision 3.** Same observation as review A (X4): the relayed answer "1 a, 2 a, 3 a, 4 b" matches the FINAL block for 1, 2 and 4, but in `tasks.md`'s own lettering "3 a" means "buy a code-signing certificate", while the FINAL block says "ship unsigned". Confirm. If a certificate is bought, F1-10 needs a Windows signing step that no spike has tested.
- **macOS 27 belongs in the test matrix.** The owner's Mac runs macOS 27.2.1 (bottle tag `arm64_golden_gate`); plan 4.1 lists only macOS 15 and 26 VMs.
- **Nothing in S0-04 to S0-07 ran on Windows.** Same as review A (X3).
- **Clean-up after the gate** (I removed only my own test files): the session scratchpad holds about 1 GB of spike material (Homebrew.pkg, expanded pkg, prefix attempt, npm prefixes, a gopls binary, the x/tools + x/net copy); `$TMPDIR/lsp-spike-*.stderr`; `/private/tmp/claude-501/spk/` (signed spike app and zip; keep it until the launch check below).

## What the plan must change

### product-plan.md
- **4.1 line 193** (pkg flow): the verified design. Script text embedded in the app and passed to `osascript` as an argument; `osascript` started with a fixed minimal environment and `/usr/bin/env -i` inside; tools by absolute path; an unprivileged pre-check of size, digest, signature and Team ID shown as "verify" before the prompt; as root: bounded copy of a regular file from one descriptor into `mktemp -d` under `/var/root`, verify the copy (`pkgutil` with Team ID `927JGANW46` and notarization; `spctl` as a second opinion with a defined answer for Gatekeeper off), `installer` on the copy, refuse if `/opt/homebrew` exists, all reasons on stderr. Pin the Team ID, not the certificate. Drop `open -W`.
- **4.1 line 194**: add `HOMEBREW_NO_AUTO_UPDATE=1` for the setup run; set proxy variables in upper and lower case.
- **4.1 line 195**: keep "machine-wide, needs an administrator" for the AWS CLI until the Windows test of `--scope user` passes. Add the Windows AI tool installs: `Anthropic.ClaudeCode`, `OpenAI.Codex`; a chosen source for OpenCode.
- **4.1 line 196** and decision 1 facts (`tasks.md` line 653): own-folder Homebrew is not offered (Tier 3, compiles from source, needs CLT); a standard account installs the pkg with an administrator's name and password; "not an administrator" is detected before the prompt (`id -Gn`).
- **4.1 line 199**: names confirmed (two are casks). Language servers: `brew install go` before `gopls`; `brew install typescript` (native server, no Node) for TS 7 and fresh workspaces; `typescript-language-server` only for TS 5/6 projects; drop the "typescript-language-server 6.1.0" fact.
- **4.1 detection**: never run a developer-tool stub before `xcode-select -p` returns an existing path other than `/` (as `execx.FindGit` already does for git).
- **4.1 line 200**: add macOS 27 to the VM matrix.
- **4.3 line 217**: classic token with `repo` + `read:org` only; fine-grained tokens not supported in 1.0; an organisation that blocks classic tokens or needs SSO authorisation gets "open in browser".
- **4.3 line 218**: Bitbucket scopes as listed (confirm names on the token screen); Bearer; workspaces from `GET /user/workspaces`, remote pre-fill as fallback; closing the source branch is the `close_source_branch` flag; tokens expire.
- **4.12 lines 318-320**: TS server choice and selection rule; on Windows Node is needed only to run `npm -g`, and the app starts the native `tsc.exe` directly; default 2 live servers (4 on large-RAM machines), idle and no-pull shutdown, memory as ranges.
- **4.13 line 329**: polling: GitHub per-repository lists and details with ETag (304 is free); "waiting for my review" through search or GraphQL without ETag, at most every 60 s while visible; Bitbucket: count own calls against 1,000/h per token, "mine" per workspace, "to review" per repository, 15 minutes off screen. Error mapping: 401 expired; 404 with the scope headers; 403 SSO or classic-blocked; 403/429 limits with `retry-after`.
- **4.14 step 4 (line 341)**: the full designated requirement (S0-05 note 1); `spctl` is a second check, never the only one.
- **4.14 step 7 (line 344)**: `--options runtime --timestamp`, executable then bundle; notarize a zip; fail on any status other than Accepted and keep `notarytool log`; staple; build the release archive from the stapled app.
- **Section 5 line 361** (language servers): a message log with offsets; never drop responses or requests; compact progress, log messages and superseded diagnostics above a per-session cap (16 MB), with a hard-limit action; Go answers the server requests the page cannot; the ping never blocks the reader; the send path never drops.
- **Rules (lines 376-377)**: lsp-client Markdown is rendered through a sanitizer or with raw HTML off; language-server log messages (one per request with native TS) never reach the app log.
- **Risk table**: Homebrew row (428) to the verified design including the environment; new rows: Homebrew's signer changes Team ID; Bitbucket 1,000 requests/hour per token; a GitHub organisation blocks classic tokens; language-server memory with the measured ranges (435).
- **Section 10 line 462**: THIRD_PARTY_NOTICES adds `marked`, `vscode-languageserver-protocol`, `vscode-jsonrpc` and the sanitizer.

### tasks.md
- **S0-04**: names and IDs confirmed; VM and Windows clauses open (the note's section 11 list plus: environment seen by `with administrator privileges`, Gatekeeper off, AWS CLI `--scope user` as a standard user, macOS 27). Remove "Homebrew into a folder the user owns" from Build.
- **S0-05**: Team ID and requirement recorded; clean-Mac double-click, the launch check and the `.p12` export stay with the owner.
- **S0-06**: done on macOS; Windows install path open; server choice changed.
- **S0-07**: reads done; manual steps 1-3 plus the Bitbucket workspace endpoints and the GitHub organisation policy check before P6-03/P6-04 sign-off.
- **Standing checklist**: add "an elevated or privileged child process gets an explicit minimal environment and absolute tool paths" and "no developer-tool stub runs before `xcode-select -p` succeeds".
- **F1-01**: `NSAppleEventsUsageDescription` (both languages) and the apple-events entitlement if the launch check needs them.
- **F1-09**: requirement = full designated requirement; `spctl` never the sole gate.
- **F1-10**: start from `spike/signing/sign-notarize.sh`; identity and tool paths from the environment, never the repository; fail on any status other than Accepted; staple, then archive; Accept adds a launch smoke test with any installed copy quit.
- **F1-12**: start from `homebrew-pkg-install.sh` and `classify`; fix S0-04 issues 1-5; exit codes 0/10/11/12/13/14/15/20/21/30/31; Tests: the `test-verify.sh` cases without root, plus environment injection (BASH_ENV, an exported function, SHELLOPTS/PS4) has no effect; Review adds "osascript gets a fixed environment; the script text is never read from disk; every argument through `quoted form of`".
- **F1-15 and design 5.1**: remove the "Install into my own folder" button; add rows "Prefix exists", "Download damaged" (size or digest, retry) and "Homebrew signer not recognised" (update Codepass); "verify signature" runs before the password prompt, and the privileged step is one indeterminate state that shows only its final error line.
- **E5-03**: the S0-06 Build items plus notes 2-5; Accept "a hidden page does not stall a fake server that floods output"; Windows Job Object; request timeout 15-30 s; paths compared case-insensitively on macOS and Windows.
- **E5-09**: pull diagnostics for the native TS server; own Workspace class; a test that `<img onerror>` in a hover stays inert (the task already demands raw HTML off).
- **P6-03**: scopes `repo` + `read:org`; tests with recorded 401, 404 with scope headers, 403 SSO, 403 classic-blocked, secondary-limit 403/429, 304.
- **P6-04**: `/user/workspaces`; `/workspaces/{ws}/pullrequests/{me}`; count calls under 1,000/h; async merge (202 and task status), 400/409/555; `close_source_branch` on merge.
- **P6-07**: the polling rules above; "to review" works without ETag.
- **P6-08 and design 5.11**: the token step names the exact scopes per provider and only the classic GitHub token; Bitbucket workspaces come from the token after step 4 (chips to confirm), not typed in step 1; verify calls `GET /user` and compares granted scopes.
- **Windows tasks (R7-01 and the milestone 1 Windows gate)**: winget installs of Claude Code and Codex; OpenCode's source; no orphan `gopls.exe`, `tsc.exe` or `node.exe` after quit.

## Manual checks for the owner (S0-04 to S0-07)

S0-04 (clean macOS 15, 26 and 27 VMs without Command Line Tools; Windows 10 1809 and 11)
1. The note's section 11 list, items 1-8: script run through the real `osascript` prompt; `brew install git awscli` with only "Pouring" lines and no developer-tools dialog; `brew update` with no dialog; `/usr/bin/git --version` does raise the dialog; a standard account with an administrator's credentials (cancel once, wrong password once); offline notarization check; MDM; disk full; winget on both Windows versions including `Amazon.AWSCLI --scope user` as a standard user; a real proxy and a PAC file.
2. From a process carrying a marker variable, run `do shell script "/usr/bin/env" with administrator privileges` and see whether the marker arrives (decides how serious S0-04 issue 1 is; the fix is needed either way).
3. With Gatekeeper turned off, run the script in `verify` mode and record the `spctl` answer.

S0-05
4. Quit AWS SSO Hub, open `/private/tmp/claude-501/spk/AWS SSO Hub.app`: window, browser sign-in, keychain read, "Open in terminal" and its Automation prompt.
5. On a clean VM, download the zip in Safari, double-click: no Gatekeeper warning, also offline.
6. Export the Developer ID certificate and key as a password-protected `.p12` to two places you control.

S0-06
7. Windows 11 VM: `winget install GoLang.Go`, `go install gopls`; `winget install OpenJS.NodeJS.LTS`, `npm install -g typescript`, then start the native `tsc.exe --lsp -stdio`; check `gopls` is found from a GUI-launched process.
8. Windows: quitting or killing the app leaves no `gopls.exe`, `tsc.exe` or `node.exe`.
9. Your largest Go and TS repositories: memory with four task folders open (milestone 5 gate).

S0-07
10. GitHub: a classic token with only `repo` + `read:org` on a throwaway repository: create, merge, delete the branch; then revoke it.
11. In each GitHub organisation you work in: Settings > Personal access tokens: is classic-token access allowed? For SAML organisations, authorise the token and record the 403 before you do.
12. Bitbucket: an API token with the five scopes: `GET /user/workspaces`, `GET /workspaces/{ws}/pullrequests/{you}`, list, create with `close_source_branch`, merge, decline; remove `write:pullrequest` and record the 403; note the token's expiry; delete the token.
