# Spike S0-03: AI tool signals (Claude Code, Codex, OpenCode)

Branch: `spike/signals` (local, not pushed). Code: `spikes/signals/` in the worktree `.claude/worktrees/wf_e2601f60-0f7-3`.
Run date: 2026-11-16, macOS arm64. Claude Code 2.1.325, OpenCode 2.3.0 (Homebrew). Codex is not installed (no `~/.codex`).

## 1. Summary of decisions

1. **Claude Code: two `--settings` do not merge. The last one wins and earlier ones are ignored entirely.** The owner's `claude` zsh function is `command claude --model opus --effort max --settings '{"ultracode":true}' --chrome --channels ... "$@"`. If the app launches `claude --settings <ours>` through that function, ours lands after the function's own, so ours wins and `{"ultracode":true}` is dropped for that launch. If our flag came first, our hooks would be silently lost. The outcome depends on flag order.
2. **Fix that works: deliver the hooks as a session-only plugin (`--plugin-dir <dir>` containing `hooks/hooks.json`) instead of `--settings`.** Verified: with `--settings B.json --plugin-dir plug`, hooks from both fired. Plugin hooks add to whichever settings win, so the user's function keeps working and flag order stops mattering. `--settings` stays as the fallback.
3. **Session IDs.** Claude Code has `--session-id <uuid>` (help text; not run), and `session_id` is in every hook payload; `--resume <id>` keeps the same ID (verified). OpenCode `--session <id>` resumes, and creates the session if absent; a made-up `ses_codepass...` ID was accepted (verified). Codex: `codex resume <id>` / `codex exec resume <id>` (docs only).
4. **OpenCode: a plugin loaded purely through `OPENCODE_CONFIG_CONTENT='{"plugins":["<dir>"]}'` with no file written works (verified), using `opencode run --standalone`.** The default is a shared background service, which would not read a per-launch env var. The interactive TUI under `--standalone` is a manual check.
5. **Codex: `notify` emits only `agent-turn-complete`, cannot be set per project, and the only per-launch route is `-c 'notify=[...]'` (docs say `-c` takes any key; untested).** Codex now also has Claude-style lifecycle hooks (14 events incl. `SessionStart`, `PermissionRequest`, `Stop`), but they involve a trust step and the docs do not show per-launch supply. All Codex runtime checks are manual.
6. **The process-tree fallback works** with `TIOCGPGRP` on the pty master plus one `ps` call (verified, section 5), with a limit on what it can say.
7. **Nothing was written into the user's tool settings.** `~/.claude/settings.json` and `~/.config/opencode/opencode.json` are byte-identical (shasum before/after). `~/.claude.json` changed. That is Claude Code's own runtime state file, rewritten by every running Claude session (including the one executing this spike); none of our flags target it. A strict byte-identical test must not include it (see plan changes).

## 2. Claude Code

4 `claude` invocations: 3 model prompts (haiku) plus one `-p "/context"` that makes no model call. All used `command claude` (bypassing the owner's function), temp settings under the scratchpad, `~/.claude` never edited.

Hook recorder: `spikes/signals/rec.sh LABEL` appends env `CODEPASS_TASK` plus the stdin payload to `$CP_LOG`. Settings built by `spikes/signals/mk.py`.

### 2.1 Events and payloads (observed, T1: `claude -p "Run the shell command: echo hi. Then reply ok." --model haiku --settings A.json`)

| Event | Arrived | Payload fields (observed) |
|---|---|---|
| `SessionStart` | yes | `session_id, transcript_path, cwd, hook_event_name, source` (`startup`; `resume` on resume, T3) |
| `UserPromptSubmit` | yes | common + `prompt_id, permission_mode, prompt` (full prompt text) |
| `PreToolUse` | yes | common + `prompt_id, permission_mode, effort.level, tool_name, tool_input, tool_use_id` |
| `PostToolUse` | yes | same + `tool_response` |
| `Stop` | yes | common + `permission_mode, stop_hook_active, last_assistant_message, background_tasks, session_crons` |
| `SessionEnd` | yes | `session_id, transcript_path, cwd, hook_event_name, reason` (`other` in -p mode) |
| `PermissionRequest`, `Notification` | registered, did not fire | no permission prompt occurs in -p mode with the owner's `defaultMode: bypassPermissions`. Docs (S13) say `Notification` fires on permission prompts and idle; needs a manual interactive check. |

Environment: `CODEPASS_TASK` set by the launcher was visible inside the hook process, so the hook command needs no task-id argument.

Payloads contain the prompt, tool input and last assistant message. They must never be logged (checklist item 5); the report mode should forward only `task id + event + session_id + permission_mode`.

### 2.2 Two `--settings` flags (the owner's function case)

| Test | Flags | Hooks that fired |
|---|---|---|
| T2 | `--settings '{"ultracode":true}' --settings B.json --settings A.json` | only A (SessionStart, UserPromptSubmit, Stop, SessionEnd); B did not |
| T3 | `--settings A.json --settings B.json` (plus `--resume`) | only B (SessionStart, UserPromptSubmit, Stop); A did not |
| T4 | `--plugin-dir plug --settings B.json` (`-p "/context"`) | B and P (plugin) SessionStart both fired |

Conclusion: the last `--settings` replaces earlier ones; plugin hooks are additive. Unverified: whether `ultracode` is truly dropped (no visible effect to observe); the hooks result is certain. Also unverified: that `--plugin-dir` raises no trust/enable prompt in the interactive TUI (manual).

### 2.3 Resume by session ID (verified)

- T2 produced session `f9960b58-...`. T3 `claude -p "what was my previous message?" --resume f9960b58-...` answered from the old conversation, returned the same `session_id`, and fired `SessionStart` with `"source":"resume"`.
- `--session-id <uuid>` exists in `claude --help` ("Use a specific session ID for the conversation"): the app can generate the UUID, store it in `tasks.json` at start, and not depend on a hook to capture it. Not run (prompt budget). Keep the `SessionStart` capture as a cross-check.
- Claude Code stores transcripts per cwd (`~/.claude/projects/<cwd-slug>/`), so resume probably must run in the same folder. T3 used the same cwd; a different-cwd resume is untested.

### 2.4 Approval mode

- Name: **permission mode**. Values from `claude --help`: `acceptEdits, auto, bypassPermissions, manual, dontAsk, plan`.
- Readable: `permission_mode` in `UserPromptSubmit`, `PreToolUse`, `Stop` payloads (observed `bypassPermissions`, inherited from the owner's `permissions.defaultMode`); absent from `SessionStart`/`SessionEnd`. It can change mid-session (Shift+Tab), so show "mode as of last event".

## 3. OpenCode (v2.3.0; signed in to OpenAI but the token returned 401)

OpenCode 2.x is a rewrite. The config key is `plugins` (plural; v1 used `plugin` and a `plugin/` folder, which is what plan [S26] describes), and the plugin API is `Plugin.define({id, setup(ctx)})` with `ctx.event.subscribe()`. Docs: https://opencode.ai/docs/plugins, https://opencode.ai/docs/build/plugins, https://opencode.ai/docs/build/plugins/server, https://opencode.ai/docs/config. The docs pages do not list env vars; names below come from `strings` on the binary: `OPENCODE_CONFIG`, `OPENCODE_CONFIG_CONTENT`, `OPENCODE_CONFIG_DIR`, `OPENCODE_DISABLE_PROJECT_CONFIG`, `OPENCODE_DB`, `OPENCODE_SESSION_ID`.

### 3.1 What ran

`spikes/signals/oc-run.sh` sets `OPENCODE_CONFIG_CONTENT={"plugins":["<abs dir>"]}` and runs `opencode run --standalone --archived --model openai/gpt-4.1-mini "reply ok"` in a scratch dir. The plain-JS plugin (`spikes/signals/opencode-plugin/index.js`: default export `{id, setup}`, no SDK import needed) subscribes to all events and appends them to `$CP_LOG`. Result: `setup` ran (`ver 2.3.0`, `dir` = cwd) and events streamed, with the user's config untouched.

| Event (observed) | Payload |
|---|---|
| `integration/provider/model/agent/command/skill/websearch/reference/plugin.updated` | startup noise, `data:{}` |
| `session.inbox.enqueued` | `data.sessionID, inboxID, item.payload.text` (the user's prompt: never log) |
| `session.inbox.delivered` | `data.sessionID, ...` |
| `session.execution.started` | `data.sessionID` (the "running" signal) |
| `session.execution.failed` | `data.sessionID, error{type:"provider.auth", message:"Request failed: 401"}` |
| `location.shutdown` | server end |

Every event has `id, created, type`; session events also have `durable.aggregateID` (the session ID).

Names present in the binary but not observed (the model call failed with 401): `session.execution.succeeded`, `session.execution.interrupted`, `session.idle`, `session.status`, `session.created`, `session.step.*`, `session.tool.called/success/failed`, `permission.asked`, `permission.replied`, `permission.rejected`, `session.form.*`. Proposed mapping: running = `session.execution.started`; waiting/finished = `session.execution.succeeded|failed|interrupted`; needs approval = `permission.asked`. `session.idle` may be a v1 name: confirm with a working model (manual).

### 3.2 Resume by session ID (verified)

- `opencode run --session ses_e1fd...` on an existing session: events carried the same session ID, `execution.started` fired again.
- `--session ses_codepass00000000000000001` (nonexistent): accepted and created with exactly that ID. So the app can pre-assign IDs (must start `ses_`; length rules unverified).
- TUI: `opencode --session <id>`. `--continue` is ambiguous, as in the plan.
- Both test sessions were removed with `opencode session delete`.

### 3.3 Approval mode

- v1 config key `permission` (the owner's file has `"permission": "allow"`); v2 docs show a `permissions` array of `{action, resource, effect: allow|deny|ask}`. CLI flag `--auto` ("Auto-approve permissions that are not explicitly denied"): never pass it.
- No event I saw reports the effective mode. Read the merged config (`opencode debug config`, not run) or show "unknown". Manual.

## 4. Codex (not installed; docs only)

Sources: https://learn.chatgpt.com/docs/config-file/config-advanced, https://learn.chatgpt.com/docs/hooks, https://learn.chatgpt.com/docs/developer-commands, https://learn.chatgpt.com/docs/config-file/config-reference (old developers.openai.com/codex/... URLs redirect there).

### 4.1 `notify`

- Config: `notify = ["python3", "/path/notify.py"]` in `~/.codex/config.toml`. Machine-local: a project `.codex/config.toml` cannot set it (ignored with a warning).
- One event: `agent-turn-complete`. Payload is one JSON argument (argv, not stdin): `type, thread-id, turn-id, cwd, input-messages, last-assistant-message`. `thread-id` is the session ID.
- So `notify` gives "turn finished" and the session ID, but nothing for "running" or "needs approval". Per-launch route: `-c`/`--config key=value` (TOML-parsed, documented for any key), e.g. `codex -c 'notify=["/path/codepass","report"]'` (inferred, untested). `CODEX_HOME` would lose auth and history, so not recommended.

### 4.2 Hooks (new since the plan; not in it)

Events: `PreToolUse, PermissionRequest, PostToolUse, PreCompact, PostCompact, UserPromptSubmit, SubagentStop, Stop, PreModelRequest, PostModelRequest, Interrupt, SessionStart, SubagentStart, SessionEnd`. Stdin: `session_id, transcript_path, cwd, hook_event_name, model`, plus per event `turn_id`, `permission_mode`, `prompt`, `last_assistant_message`. `SessionStart.source` is `startup|resume|clear|compact|fork`. No `Notification` event. Hooks come from `~/.codex/hooks.json`, `config.toml` `[hooks]`, project `.codex/`, plugins or managed sources; `features.hooks` enables them. Project hooks need a trusted project; there is a trust step with `--dangerously-bypass-hook-trust` as the bypass. Whether inline hooks passed by `-c` avoid the trust prompt is unknown. This is the most promising Codex route (same shape as Claude Code, so one report mode could serve both) but needs a real install.

### 4.3 Resume and mode

- Resume: `codex resume <SESSION_ID>`, `codex exec resume <SESSION_ID> [PROMPT]`, `--last`, `--all`. IDs are UUIDs (or a name).
- Names: **approval policy** (`--ask-for-approval/-a`, `approval_policy`): `on-request | never | {granular=...}` (`untrusted` unsupported, `on-failure` deprecated). **Sandbox mode** (`--sandbox/-s`, `sandbox_mode`): `read-only | workspace-write | danger-full-access`; or a `default_permissions` profile (not combined with `sandbox_mode`). Flags to never produce: `--dangerously-bypass-approvals-and-sandbox` / `--yolo`, `--approve-for-me`. Readable: `permission_mode` in hook payloads (docs); `/status` and `/permissions` in the TUI; otherwise parse `~/.codex/config.toml` read-only.

## 5. Process-tree fallback (verified)

`spikes/signals/proctree.py`: a zsh in a pty; reads the foreground process group with `ioctl(master, TIOCGPGRP)`, lists processes with one `ps -axo pid=,ppid=,pgid=,tpgid=,comm=`, and says "busy" when a member of that group is not a shell.

```
idle shell        -> (25371, ['zsh'], False)
running sleep 4   -> (25646, ['sleep'], True)
after sleep       -> (25371, ['zsh'], False)
pipeline          -> (26318, ['zsh', 'cat', 'sleep'], True)
pipeline finished -> (25371, ['zsh'], False)
```

Notes: pipelines include a shell subshell, so test "any non-shell member". An AI TUI (claude, opencode, codex) is itself the foreground process, always non-shell. So the fallback only answers "a tool is alive in this terminal" versus "it exited back to the shell", not working versus waiting; that still needs the hooks. The plan's wording ("a live child that is not a shell") is wrong as written: the tool is the foreground process, not a child of it. Not tested: Windows ConPTY (no foreground-group ioctl; needs a process-tree walk), manual.

## 6. Needs manual verification

1. Claude Code, interactive TUI, with the owner's real function: launch so that the function's flags are followed by `--plugin-dir <tmp>/plug --settings <tmp>/B.json`. Expect `SessionStart` from both; confirm no trust or enable prompt; trigger a permission prompt (`--permission-mode manual`) and check `Notification` / `PermissionRequest` payloads.
2. Whether `{"ultracode":true}` is dropped when ours is last: run `claude --settings '{"ultracode":true}' --settings B.json`, then `/status`.
3. `--session-id <uuid>` on a fresh run, then `claude --resume <uuid>` from the same and from a different cwd.
4. OpenCode with a working provider (`opencode auth login` by the owner): confirm `session.execution.succeeded`, `permission.asked`, `session.idle` and their payloads; run the TUI with `opencode --standalone` plus the env plugin; `opencode debug config` for the effective permission; confirm the env plugin does not show in `opencode plugin list`.
5. Codex (install into a temp prefix: `npm i -g --prefix $TMP @openai/codex`, then the owner runs `codex login`): `codex -c 'notify=["/tmp/rec.sh"]' exec "reply ok"` and read the argv JSON; try inline hooks with `-c` plus `features.hooks=true` and see if a trust prompt blocks them; `codex resume <id>`; text of `/status`.
6. Windows: all three tools; how a hook command (`codepass.exe report`) is quoted in a plugin or settings file; ConPTY process-tree method.

## 7. What the plan must change

product-plan.md:
- Line ~271 ("Hooks are injected per launch"): replace "a settings file passed with `--settings`" with "a session-only plugin passed with `--plugin-dir` (hooks only); a `--settings` file is the fallback. Two `--settings` flags do not merge, the last wins (S0-03), so the app never relies on flag order." Delete "the spike must show which one wins and whether they merge".
- Same section: hook payloads carry `session_id` and `permission_mode`; forward only `task id + event + session_id + mode`; never the prompt text (it is in `UserPromptSubmit`, `Stop` and OpenCode `session.inbox.enqueued`).
- Line ~273 (resume): the app generates the ID: Claude Code `--session-id <uuid>`, OpenCode `--session ses_<id>` (created if absent), Codex `codex resume <id>` with the ID from the `SessionStart` hook or `notify` `thread-id`. Hook capture is the cross-check.
- Line ~272 (fallback): the foreground process is the tool itself. Reword: the fallback means "tool alive vs exited to the shell", not working vs waiting. State the method (`TIOCGPGRP` + `ps`) and that Windows needs another.
- Lines ~266-267 (state table): OpenCode 2.x signals are `session.execution.started` (running), `session.execution.succeeded|failed|interrupted` and `permission.asked` (waiting); `session.idle` is unconfirmed. Codex: `notify` gives only turn-complete; prefer Codex hooks (`UserPromptSubmit`, `PermissionRequest`, `Stop`) if per-launch supply works (manual item 5), else Codex shows only "waiting after a turn" plus the fallback.
- Source [S26]: OpenCode 2.x uses `plugins`, `Plugin.define`, `ctx.event.subscribe`; per-launch route is `OPENCODE_CONFIG_CONTENT`; launch with `--standalone`. Source [S12]: add the hooks page and `-c`.
- Add the approval vocabulary table below to the AI-tools screen section.
- Byte-identical tests: hash `~/.claude/settings.json`, `~/.codex/config.toml`, `~/.config/opencode/opencode.json`; exclude `~/.claude.json` and tool data directories, which the tools rewrite themselves.

tasks.md:
- A2-05 Build: change "per-launch injection for Claude Code (settings file)" to "(session-only plugin dir with `hooks/hooks.json`; settings file as fallback)"; OpenCode: "`OPENCODE_CONFIG_CONTENT` plus `--standalone`"; Codex: "`-c notify=[...]` or inline hooks per S0-03 manual item 5, else fallback only". Add "launcher sets `CODEPASS_TASK` in the child environment; report mode reads it". Accept: add a macOS test with the real tools besides the VM test; the byte-identical check lists the three files above.
- A2-04: argument builders include `--session-id`, `--resume`, `--plugin-dir`, OpenCode `--standalone --session`, Codex `-c` and `resume`. The test asserts none of `--dangerously-skip-permissions`, `--allow-dangerously-skip-permissions`, `--permission-mode bypassPermissions`, `--auto`, `--yolo`, `--dangerously-bypass-approvals-and-sandbox`, `--approve-for-me` can be produced. A user function's flags come before ours, so a `--settings` from the app would override the function's: use the plugin route.
- A2-08: the session ID is created by the app at task creation.

### Approval/sandbox vocabulary

| Tool | Name | Values | Where to read it |
|---|---|---|---|
| Claude Code | permission mode | `manual, acceptEdits, auto, plan, dontAsk, bypassPermissions` | `permission_mode` in hook payloads (not `SessionStart`); `permissions.defaultMode` in settings (read-only) |
| Codex | approval policy + sandbox mode | `on-request, never`; `read-only, workspace-write, danger-full-access` | `permission_mode` in hook payloads (docs); `/status`; `~/.codex/config.toml` (read-only) |
| OpenCode | permission (v1) / permissions rules (v2) | `allow, ask, deny` per action and resource; `--auto` skips | merged config; no event seen |

### Resume summary

| Tool | Choose ID up front | Capture | Resume |
|---|---|---|---|
| Claude Code | `--session-id <uuid>` (help text, untested) | hook `session_id` (verified) | `--resume <id>` (verified; same ID, `source:"resume"`; same cwd) |
| OpenCode | `--session ses_<name>` (verified; created if absent) | event `data.sessionID` / `durable.aggregateID` (verified) | `--session <id>` (verified); TUI `opencode --session <id>` |
| Codex | not found | `notify` `thread-id`, hook `session_id` (docs) | `codex resume <id>`, `codex exec resume <id>` (docs) |
