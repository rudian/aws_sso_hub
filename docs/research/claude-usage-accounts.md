# Research: usage, several accounts and the ticket queue (Claude Code and Codex)

Date: 2026-11-16. macOS arm64. Claude Code 2.1.325 and OpenCode 2.3.0 are installed; Codex is not.
Method: public pages only, plus `claude --help`, `claude auth --help`, `claude auth status --help`, `claude auth login --help`, `claude setup-token --help`. No session was started, nobody signed in, no credential or tool data file was read.

How to read the tags:
- **[doc]** an official page, read on 2026-11-16. Link follows.
- **[local]** checked locally: `claude --help`, version 2.1.325.
- **[3p]** only a third-party tool, blog or user report says so. Treat as a hint.
- Quotes from anthropic.com and support.claude.com are short fragments (a limit of the reading tool). Read the full clause at the link before relying on it.

## 0. Summary and recommendations

| Question | Recommendation |
|---|---|
| R1 Usage | Ask Claude Code itself: send `/usage` as the prompt of a print-mode call and read the structured `usage_report` (session, weekly all models, weekly per model, each with a percentage and a reset time). Codepass never touches a credential. Add the `StopFailure` hook (`error: rate_limit`) as the "limit reached" signal. Do not call the undocumented OAuth usage endpoint. |
| R2 Several accounts | (a) One config folder per account, chosen with `CLAUDE_CONFIG_DIR` in the task's environment. This is the route Anthropic documents. Share `projects/` and `file-history/` between the folders so `--resume <id>` finds the conversation under account B. Resume across accounts is **not verified**; it needs a 10-minute manual check with two accounts. |
| R3 Terms | Quotes only, in section R3. Three texts matter: the Consumer Terms clause on automated access, "ordinary, individual usage", and "developers may not collect, store, or intermediate Claude.ai credentials or session tokens". No clause was found about one person holding several subscriptions. |
| R4 Queue | (a) An interactive session in the app's PTY, ticket text as the first message, hooks as today plus three more signals. Print mode gives cleaner machine signals but no terminal to step into. |
| R5 Codex (same three features) | Usage: ask Codex itself through `codex app-server` (`account/rateLimits/read`). Accounts: one `CODEX_HOME` per account. Queue: the TUI in the app's PTY with Codex hooks, plus the app-server call as the limit gate. Codex is not installed here, so every Codex fact is from docs or source code, none from a run. |
| OpenCode | Offers a ChatGPT-plan sign-in but shows no usage or limits. Tickets can run; no meter, no gate. |

Biggest open risks:
1. Whether `claude -p "/usage"` returns `usage_report` from the CLI, and whether it costs a model request, is documented only for the Agent SDK. Manual check R1-M1.
2. Whether a conversation started under account A resumes cleanly under account B. Nothing official says yes or no. Manual check R2-M2.
3. Since 2.1.234 an interactive session **waits and continues by itself** after a limit resets. A queue must know this, or two things will try to continue the same ticket.
4. With usage credits (pay-as-you-go) turned on, a session does not stop at the limit. It keeps going and bills at API rates.

## R1. Usage

### Facts

**Status line input**
- The status line command receives JSON on stdin with `rate_limits.five_hour.used_percentage`, `rate_limits.seven_day.used_percentage` ("from 0 to 100") and `resets_at` ("Unix epoch seconds") for each. [doc] https://code.claude.com/docs/en/statusline
- "`rate_limits`: appears only for claude.ai Pro, Max, and Ultra subscribers ... Even then, it appears only after the first API response in the session." Each window "may be independently absent". [doc] same page
- No per-model weekly field is on that page. The page gives no "since version" for the two basic windows.
- The command runs at session start, on each new assistant message, when a rate-limit window reaches `resets_at`, and on an optional `refreshInterval`. Print mode is not mentioned. [doc] same page
- A status line is a `statusLine` key in settings. The plan's injection route is a session-only plugin, and passing a second `--settings` drops the owner's own (spike-signals.md, section 2.2). Whether a plugin can supply a status line was not found in the docs.

**The `/usage` command and `usage_report`**
- "When you send `/usage` as a prompt, Claude Code delivers the report as an assistant message whose `message.content` holds the text. It attaches `usage_report` ... only when ... The session authenticates with a claude.ai credential; The credential shows a known plan type or carries the `user:profile` scope; The account isn't on usage-based billing". Requires Agent SDK v0.3.273 or later. The type is "experimental: its shape may change". [doc] https://code.claude.com/docs/en/agent-sdk/typescript
- `usage_report.rate_limits.limits[]` has `kind` ("such as `session`, `weekly_all`, or `weekly_scoped`"), `percent` ("0-100"), `resets_at` ("ISO 8601 timestamp"), `scope.model.display_name`, `severity`, `is_active`. `extra_usage` has `is_enabled`, `monthly_limit`, `used_credits`, `utilization`. [doc] same page
- "A `claude setup-token` token passed as `CLAUDE_CODE_OAUTH_TOKEN` doesn't qualify by default, because it carries only the `user:inference` scope." [doc] same page
- Claude Code "runs some commands immediately without interrupting the response, such as `/status`, `/tasks`, and `/usage`." `/cost` and `/stats` are aliases of `/usage`. [doc] https://code.claude.com/docs/en/commands
- The headless page's list of commands that work with `-p` does not name `/usage`. It says "Built-in commands that only run in the terminal interface, such as `/login`, aren't available." [doc] https://code.claude.com/docs/en/headless
- "A message such as `rate limited until 3:14am` in `/usage` means Claude Code can't fetch your plan limits again until then". So the fetch itself is throttled; poll rarely. [doc] https://code.claude.com/docs/en/costs
- There is no `claude usage` subcommand. [local]
- `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC` also turns off the plan-usage fetch. [3p] issue title, https://github.com/anthropics/claude-code/issues/108146

**Print-mode stream**
- `stream-json` can carry `rate_limit_event` with `rate_limit_info`: `status` (`allowed`, `allowed_warning`, `rejected`), `resetsAt` (number), `utilization`, `errorCode` (`credits_required`), `overageEnabled`. "Emitted when the session encounters a rate limit." [doc] https://code.claude.com/docs/en/agent-sdk/typescript
- Assistant messages can carry `error: 'rate_limit'`. The `result` message has `is_error`, `api_error_status` ("the HTTP status code of the API error that terminated the conversation") and `terminal_reason`. [doc] same page
- `system/api_retry` events report `attempt`, `max_retries`, `retry_delay_ms`, `error_status`, `error` (same categories, including `rate_limit`). [doc] https://code.claude.com/docs/en/headless
- "Claude Code exits with code 0 on success and a non-zero code when the run fails". No specific code for a usage limit is documented. [doc] same page

**Hooks**
- `StopFailure` "Runs instead of Stop when the turn ends due to an API error." Its `error` field is one of `rate_limit`, `overloaded`, `authentication_failed`, `billing_error`, `server_error`, and others. It also has `error_details` (example: `429 Too Many Requests`) and `last_assistant_message` ("the API error string itself"). Output and exit code are ignored. [doc] https://code.claude.com/docs/en/hooks
- `Stop`: "API errors fire StopFailure instead." [doc] same page
- `Notification` types include `quota_auto_resume_fired`, `quota_auto_resume_stale`, `quota_auto_resume_disabled` (v2.1.234 or later). [doc] same page
- No hook payload with usage percentages was found. About half of the hooks page was read word for word.

**What happens at a limit, interactive**
- On-screen: `You've hit your session limit · resets 3:45pm`, `You've hit your weekly limit · resets Mon 12:00am`, and per-model lines (`Opus`, `Sonnet`; the changelog for 2.1.323 also names a weekly Fable limit). A warning such as `You've used 85% of your session limit · resets 3:45pm` can come earlier. [doc] https://code.claude.com/docs/en/errors
- "The session and weekly limits are shared across all models, so switching models doesn't restore access." A per-model limit is escaped by `/model`. [doc] same page
- "When a claude.ai usage limit stops Claude mid-task, Claude Code waits in the open session and continues the task on its own after the limit resets. Automatic continue is on by default in interactive sessions signed in with a claude.ai subscription. Requires Claude Code v2.1.234 or later." [doc] https://code.claude.com/docs/en/interactive-mode
- While waiting: `Usage limit reached · limit resets 3:45pm` and `Continuing automatically at 3:45pm · Esc to cancel`. At the reset Claude Code "prompts Claude to pick the task up where it stopped. It doesn't resend your last message." [doc] same page
- "After three automatic continuations, automatic continue stops". After a sleep of more than about 30 minutes it waits for Enter instead. [doc] same page
- Turn it off: `autoContinueAtUsageLimit: false` in user settings, or `/config`. "`/config autoContinueAtUsageLimit=false` also works, including with `-p`". [doc] same page
- "Background sessions and `-p` runs: the menu row isn't available." So print mode does not wait by itself. [doc] same page
- `idle_prompt` is not sent while Claude Code waits for a limit reset. [doc] https://code.claude.com/docs/en/hooks
- The limits are shared: "all activity in both tools counts against the same limits" (Claude apps and Claude Code). [doc] https://support.claude.com/en/articles/11145838-using-claude-code-with-your-pro-or-max-plan

**Usage credits ("extra usage")**
- "With usage credits enabled, your work continues on usage credits instead of stopping." [doc] https://support.claude.com/en/articles/11647753-understanding-usage-and-length-limits
- "Usage will be billed at standard API rates". [doc] https://support.claude.com/en/articles/11145838-using-claude-code-with-your-pro-or-max-plan
- 2.1.323 added "an options menu (stop, switch to usage credits, upgrade) when a model needs usage credits". [doc] https://github.com/anthropics/claude-code/blob/main/CHANGELOG.md

**The undocumented endpoint**
- Several open-source tools call `https://api.anthropic.com/api/oauth/usage` with a bearer token and the header `anthropic-beta: oauth-2025-04-20`. They read `five_hour`, `seven_day`, optional `seven_day_opus` / `seven_day_sonnet` and `extra_usage`, each with `utilization` and an ISO `resets_at`. [3p] https://docs.rs/crate/claude-usage/0.2.3/source/README.md and similar crates
- No official page documents it. The token is Claude Code's own sign-in, so a caller must read Claude Code's Keychain item or `.credentials.json`. This research did not do that.
- A feature request asks for a supported way "so tools don't need to read OAuth credentials". [3p] issue title, https://github.com/anthropics/claude-code/issues/107376

### Not verified (manual checks)

**R1-M1. Does `/usage` work in print mode, and is it free?** (3 minutes)
1. `mkdir -p ~/.codepass-dev/usage-probe && cd ~/.codepass-dev/usage-probe`
2. `command claude -p "/usage" --output-format stream-json --verbose > out.jsonl; echo "exit $?"`
3. Look in `out.jsonl` for `"usage_report"`. Note the `kind` values and whether `resets_at` is filled.
4. In the `result` line, read `num_turns`, `usage` and `total_cost_usd`. All zero means no model request was spent.
5. Open Claude Desktop, Settings > Usage, and compare the percentages.
6. Repeat with `--output-format json`.

**R1-M2. Can a plugin supply a status line?** (5 minutes) Put `"statusLine": {"type":"command","command":"cat > /tmp/sl.json"}` in a test plugin's `settings.json`, start `claude --plugin-dir <dir>`, send one message, and check that the file holds `rate_limits`. If it does not, the status-line route needs `--settings` and is out.

**R1-M3. What exactly arrives when the limit is hit?** This can only be seen when a limit is really reached. When it next happens in an app-launched session, save the hook event names (not the payload text): expect `StopFailure` with `error: rate_limit`, then no `idle_prompt`, then `Notification` `quota_auto_resume_fired` at the reset. For print mode, run `claude -p "say ok" --output-format stream-json --verbose; echo $?` while limited and keep the exit code, the `rate_limit_event` line and the `result` line.

**R1-M4. Do the status-line windows match `/usage`?** With any status line that prints its stdin, compare `rate_limits` with the `/usage` screen.

### Options and recommendation

| Option | Spends a request | Per-model weekly | Reset time | Touches credentials | Verdict |
|---|---|---|---|---|---|
| A. `claude -p "/usage"`, read `usage_report` | probably none (M1) | yes (`weekly_scoped`) | ISO | no | **Recommended** |
| B. Status line `rate_limits` | no, but only inside a live session after its first reply | no | epoch seconds | no | Backup; needs M2 |
| C. `rate_limit_event` in a print-mode stream | only seen during a real run | no | number | no | Use it if print mode is ever used |
| D. Call the OAuth usage endpoint | no | yes | ISO | **yes: reads Claude Code's sign-in** | Reject |

Recommendation: A for the numbers (the usage panel, and the check before each queued ticket), run once per configured account, at most every few minutes and once before each ticket. `StopFailure` with `error: rate_limit` is the signal that a running session hit a limit. Show `extra_usage.is_enabled` next to the bars, because with credits on a queue will not stop at the limit. If M1 fails, fall back to B and show "unknown until a session has run".

## R2. Several accounts

### Facts

**Config folder per account**
- "To stay signed in to multiple accounts at once, such as work and personal accounts, give each account its own configuration directory. ... set the `CLAUDE_CONFIG_DIR` environment variable ... Each directory has its own settings, session history, and claude.ai login or API key." [doc] https://code.claude.com/docs/en/authentication
- `CLAUDE_CONFIG_DIR`: "All settings, session history, and plugins are stored under this path. ... Useful for running multiple accounts side by side". [doc] https://code.claude.com/docs/en/env-vars
- "every `~/.claude` path on this page lives under that directory instead. The same applies to `.claude.json`". So `.claude.json` moves too. It holds "App state, account details, UI toggles, personal MCP servers" and per-project trust. [doc] https://code.claude.com/docs/en/claude-directory and https://code.claude.com/docs/en/settings
- Moves with the folder: `settings.json`, `CLAUDE.md`, `agents/`, `skills/`, `plugins/`, `projects/<project>/<session>.jsonl` (transcripts), `projects/<project>/memory/`, `file-history/<session>/`, `tasks/`. `todos/` is "Legacy ... No longer written." [doc] https://code.claude.com/docs/en/claude-directory
- Keychain: Claude Code "keys the macOS Keychain entry to that directory too, so a session with a different `CLAUDE_CONFIG_DIR` reads a different entry." The item's name is not documented. [doc] https://code.claude.com/docs/en/authentication
- Older blog posts say the opposite (one global Keychain item). They predate the sentence above. [3p]
- Setting `CLAUDE_CONFIG_DIR` to the default path once selected a different Keychain entry than leaving it unset (issue closed 2026-11-10). So leave it unset for the default account. [3p] https://github.com/anthropics/claude-code/issues/92252
- Claude Desktop's bundled Claude Code has been reported to overwrite the CLI's Keychain item. [3p] issue title, https://github.com/anthropics/claude-code/issues/101990

**Commands**
- `claude auth login` (`--claudeai`, `--console`, `--email`, `--sso`), `claude auth logout`, `claude auth status` (`--json` default, `--text`) exist. [local]
- `claude auth status`: "Exits with code 0 if logged in, 1 if not. The JSON includes a `configDirectory` field". `authMethod` is one of `none`, `claude.ai`, `oauth_token`, `api_key`, `api_key_helper`, `third_party`. Other keys are not documented. [doc] https://code.claude.com/docs/en/cli-reference
- `claude auth status` creates `.claude.json` if it is missing. [doc] https://code.claude.com/docs/en/claude-directory

**Long-lived token**
- `claude setup-token` makes "a one-year OAuth token". "It does not save the token anywhere". "It can only make model requests, so it can't establish Remote Control sessions or fetch claude.ai connectors." [doc] https://code.claude.com/docs/en/authentication
- Its scope is `user:inference` only. [doc] https://code.claude.com/docs/en/agent-sdk/typescript
- `CLAUDE_CODE_OAUTH_TOKEN` is place 5 in the precedence list, above the `/login` sign-in (place 7). It is not limited to print mode. `--bare` does not read it. [doc] https://code.claude.com/docs/en/authentication
- 2.1.323: "Fixed `/usage` hiding limit bars for setup tokens and `CLAUDE_CODE_OAUTH_TOKEN`". So the on-screen bars may show; the structured `usage_report` does not "by default". [doc] changelog
- A CLI holding an env credential has been reported to still renew the stored claude.ai login. [3p] issue title, https://github.com/anthropics/claude-code/issues/104359

**Precedence pitfalls**
- Order: cloud provider variables, `ANTHROPIC_AUTH_TOKEN`, `ANTHROPIC_API_KEY`, `apiKeyHelper`, `CLAUDE_CODE_OAUTH_TOKEN`, Anthropic profile, then `/login`. [doc] https://code.claude.com/docs/en/authentication
- `ANTHROPIC_API_KEY`: "In non-interactive mode (`-p`), the key is always used when present." Interactive mode asks once. A key in the shell profile silently moves a print-mode call (the naming call, the `/usage` call) to API billing. [doc] same page
- "If you run `/login` while the variable is set, Claude Code switches the current session to the new login." So `/login` inside a session changes the account of that session and of the stored sign-in in that config folder. [doc] same page

**Sessions and resume**
- Transcripts are local JSONL files: `~/.claude/projects/<project>/<session-id>.jsonl`. "Claude Code stores it locally as you work". [doc] https://code.claude.com/docs/en/sessions
- "You can run `claude --resume <session-id>` from any directory"; the lookup works when exactly one other project holds that ID. [doc] same page
- Files older than `cleanupPeriodDays` (default 30) are deleted. [doc] same page
- No page says a session belongs to an account or an organisation, and none says it does not.
- Resume failures with "Invalid signature in thinking block" exist, but every report found is about damaged transcripts, not about a change of account. [3p] https://claudeissues.com/issue/66204-bug-resume-command-fails-with-invalid-signature-in-thinking-block-error-when-pre
- The status line input has a `prompt_cache` block (`warm`, `hit_ratio`, `miss_recache_tokens`). It can show what a switch cost. [doc] https://code.claude.com/docs/en/statusline
- The prompt cache is kept per organisation on Anthropic's API. This is from the API's prompt-caching page and was not re-read today. If true, the first turn under B sends the whole conversation uncached.

### Not verified (manual checks)

**R2-M1. Set up a second account folder.** (5 minutes)
1. `CLAUDE_CONFIG_DIR=~/.claude-b command claude auth login` and sign in as account B.
2. `CLAUDE_CONFIG_DIR=~/.claude-b command claude auth status` and `command claude auth status`. Check: two different accounts, `configDirectory` differs, and note which keys name the account and the plan (never paste the output into the repository).
3. Start `command claude` without the variable. Check that account A is still signed in.

**R2-M2. Resume A's conversation under B.** (10 minutes)
1. In a scratch folder: `command claude --session-id <new uuid> --model haiku`, say "Remember the word walnut. Then run: echo hi", then quit.
2. Copy that session to B: `mkdir -p ~/.claude-b/projects/<slug> && cp ~/.claude/projects/<slug>/<uuid>.jsonl ~/.claude-b/projects/<slug>/` (the slug is the scratch folder's path with non-alphanumeric characters replaced by `-`).
3. In the same scratch folder: `CLAUDE_CONFIG_DIR=~/.claude-b command claude --resume <uuid>`, ask "Which word?".
4. Pass: it answers "walnut" with no API error. Note any error text about signatures or thinking.
5. Repeat once with a session that used extended thinking (`--effort high`) and a tool call, since that is the risky case.
6. Repeat step 3 with `~/.claude-b/projects` as a symlink to `~/.claude/projects` instead of a copy.

**R2-M3. Does the owner's `claude` shell function or shell profile set `CLAUDE_CONFIG_DIR`, `ANTHROPIC_API_KEY` or `CLAUDE_CODE_OAUTH_TOKEN`?** Run `env | grep -c -E '^(CLAUDE_CONFIG_DIR|ANTHROPIC_API_KEY|CLAUDE_CODE_OAUTH_TOKEN)='` in a new terminal. Anything but `0` means the login shell can override what the app sets.

**R2-M4. Do `--chrome` and `--channels` (used by the owner's function) work under the second folder?** Start through the function with `CLAUDE_CONFIG_DIR=~/.claude-b` and watch for errors.

### Options and recommendation

| | (a) One config folder per account | (b) One folder, a token per launch | (c) Swap the Keychain item |
|---|---|---|---|
| Documented by Anthropic | yes, for exactly this | token is documented "for CI pipelines and scripts" | no |
| Codepass stores a credential | no | **yes** (a one-year token per account) | reads and writes Claude Code's own item |
| Running sessions when switching | untouched | untouched | **all of them change account** |
| Resume under B | needs the transcript visible in B's folder (copy or link) | works as is | works as is |
| Settings, skills, plugins, MCP servers | per folder: link the shared ones; user-scope MCP servers and folder trust live in `.claude.json` and must be set up once per folder | shared | shared |
| Usage numbers for B | yes (`/usage` in B's folder) | not in `usage_report` (`user:inference` only) | only for the active account |
| Remote Control, claude.ai connectors | yes | no | yes |

Recommendation: **(a)**. Codepass keeps a list of accounts, each one a label plus a config folder path (the default account has no path and the variable stays unset). The launcher adds `CLAUDE_CONFIG_DIR` to the task's explicit environment. The owner signs in with `claude auth login` in the app's terminal. To share a conversation, link `projects/` and `file-history/` of each extra folder to the default folder's, set up once with the owner's consent; if M2 step 6 fails, copy the one session's files on switch instead. Link `settings.json`, `CLAUDE.md`, `agents/`, `skills/` the same way if the owner wants identical behaviour.

Switching a running ticket from A to B: stop the session on A, then start `claude --resume <id>` with B's folder in the same task folder, with a first message such as "continue". Cost: the first turn under B re-reads the whole conversation, which counts against B's allowance.

(b) is the fallback if M2 fails for linked folders but not for a shared folder. (c) is rejected.

## R3. Terms (quotes only)

**(a) One person, several subscriptions**
- Consumer Terms, section 2: "You may not share your Account login information, Anthropic API key, or Account credentials with anyone else." and "You also may not make your Account available to anyone else." https://www.anthropic.com/legal/consumer-terms (effective 2025-10-08)
- Usage Policy, "Do Not Abuse Our Platform": "Coordinate malicious activity across multiple accounts, or create multiple accounts, to avoid detection" (the bullet continues with circumventing product guardrails). Also: "Circumvent a ban through the use of a different account". Also: "Intentionally bypass or interfere with capabilities, restrictions, or guardrails established within our products." https://www.anthropic.com/legal/aup (effective 2026-11-12)
- Usage limits article: "We may adjust usage limits at our discretion to manage capacity and ensure fair access to all users." https://support.claude.com/en/articles/11647753-understanding-usage-and-length-limits
- Claude Code docs: "To stay signed in to multiple accounts at once, such as work and personal accounts, give each account its own configuration directory." https://code.claude.com/docs/en/authentication
- Not found in any page read: a clause that allows or forbids one person paying for more than one subscription.

**(b) Unattended or scripted use on a subscription**
- Consumer Terms, section 3, a listed prohibition: "Except when you are accessing our Services via an Anthropic API Key or where we otherwise explicitly permit it" ... "to access the Services through automated or non-human means, whether through a bot, script, or otherwise". https://www.anthropic.com/legal/consumer-terms
- Claude Code legal page: "Claude Code usage is subject to the Anthropic Usage Policy. Advertised usage limits for Pro, Max, and Ultra plans assume ordinary, individual usage of Claude Code and the Agent SDK." https://code.claude.com/docs/en/legal-and-compliance
- Authentication page, on `claude setup-token`: "For CI pipelines, scripts, or other environments where interactive browser login isn't available, generate a one-year OAuth token ... This token authenticates with your Claude subscription". https://code.claude.com/docs/en/authentication
- Help centre, update of 2026-06-15: "We've paused the previously-announced changes to Claude Agent SDK usage. For now, nothing has changed." and "Claude Agent SDK, `claude -p`, and third-party app usage still draw from your subscription limits." Update of 2026-10-07: "when you're signed in with your Claude plan, the Claude Agent SDK and `claude -p` still draw from your subscription limits." https://support.claude.com/en/articles/15036540-use-the-claude-agent-sdk-with-your-claude-plan
- Interactive-mode page: `autoContinueAtUsageLimit` cannot be turned on with `/config key=value` "because the setting grants unattended execution". https://code.claude.com/docs/en/interactive-mode

**(c) A third-party app and a subscription's credentials**
- Claude Code legal page, "Authentication and credential use": "**OAuth authentication** is intended exclusively for purchasers of Claude Free, Pro, Max, Ultra, Team, and Enterprise subscription plans and is designed to support ordinary use of Claude Code and other native Anthropic applications."
- Same section: "Anthropic does not permit third-party developers to offer Claude.ai login into their own applications, or to route requests through Free, Pro, Max, or Ultra plan credentials on behalf of their users. Moreover, developers may not collect, store, or intermediate Claude.ai credentials or session tokens — sign-in to a Claude account must complete through Anthropic's own flow."
- Same section: "Nor does it prevent an end user from signing in to the unmodified Claude Code binary with their own Claude subscription".
- Same section: "Anthropic reserves the right to take measures to enforce these restrictions and may do so without prior notice." https://code.claude.com/docs/en/legal-and-compliance
- Agent SDK overview: "Unless previously approved, Anthropic does not allow third party developers to offer claude.ai login or rate limits for their products, including agents built on the Claude Agent SDK." https://code.claude.com/docs/en/agent-sdk/overview
- Usage Policy: "Resell, proxy, or otherwise provide access to Claude through unauthorized means", including "services that route requests through consumer subscriptions". https://www.anthropic.com/legal/aup
- Not found: any official statement about the usage endpoint, for or against third-party use.

## R4. Unattended queue

### Facts

**Shared**
- `--session-id <uuid>` and `--resume <id>` work in both modes; `--fork-session` makes a new ID on resume. [local]
- "Resuming one that is still running opens that session in this terminal through `claude attach`, and a prompt you pass on the command line goes to it as its next turn." [doc] https://code.claude.com/docs/en/cli-reference

**(a) Interactive session in the PTY, with hooks**
- Turn ended normally: `Stop`. Its payload has `last_assistant_message`, `background_tasks`, `session_crons`. It does not fire on a user interrupt. [doc] https://code.claude.com/docs/en/hooks
- Turn ended by an API error or a limit: `StopFailure` with `error`. [doc] same page
- Permission prompt: `PermissionRequest` at once; `Notification` `permission_prompt` "only after the prompt has waited about six seconds". [doc] same page
- A question through the `AskUserQuestion` tool: no event of its own. `PreToolUse` can be matched to the tool name `AskUserQuestion`. "`AskUserQuestion` and `ExitPlanMode` require user interaction." [doc] same page
- An MCP server asking for input: `Notification` `elicitation_dialog`. [doc] same page
- Idle: "Expect `idle_prompt` about 60 seconds after Claude finishes responding, and only if you haven't typed since and no background agent ... is still running." [doc] same page
- A plain-text question at the end of a turn looks exactly like a finished turn: both are `Stop`. No signal separates them.
- Limit wait and continue are built in (R1). Hook signals: `quota_auto_resume_fired`, `_stale`, `_disabled`.
- Tool exit: `SessionEnd` (`reason`: `clear`, `resume`, `logout`, `prompt_input_exit`, `other`) and the process exit code.

**(b) Print mode**
- Finished: the last stream line is a `result` message; `subtype: success`, `is_error`, `terminal_reason` (`completed`, `api_error`, `max_turns`, ...), `permission_denials`; exit code 0 or non-zero. [doc] https://code.claude.com/docs/en/agent-sdk/typescript and https://code.claude.com/docs/en/headless
- Permission prompts: "a call that needs approval is denied unless something approves it, such as a `PermissionRequest` hook or a `--permission-prompt-tool`." `--permission-prompts none` denies them and removes `AskUserQuestion`. [doc] https://code.claude.com/docs/en/headless and [local]
- `AskUserQuestion` reaches an SDK host as a `canUseTool` call; the answer goes back in `updatedInput.answers`. The raw stdin format for a plain `claude -p` host is not documented. [doc] https://code.claude.com/docs/en/agent-sdk/user-input
- More messages can be queued with `--input-format stream-json`. [doc] https://code.claude.com/docs/en/agent-sdk/streaming-vs-single-mode
- `session_state_changed` messages (`idle`, `running`, `requires_action`) exist when `CLAUDE_CODE_EMIT_SESSION_STATE_EVENTS=1` is set. The docs do not say they appear in CLI output. [doc] https://code.claude.com/docs/en/agent-sdk/typescript
- Limit: `rate_limit_event`, assistant `error: 'rate_limit'`, `api_error_status`. No built-in wait in `-p`. `CLAUDE_CODE_RETRY_WATCHDOG=1` is described as waiting out a usage limit for unattended runs (read from a summary of the env-vars page, not word for word). [doc] https://code.claude.com/docs/en/env-vars
- "The workspace trust dialog is skipped when Claude is run in non-interactive mode". An `ANTHROPIC_API_KEY` in the environment is always used. [local] and [doc]
- There is no terminal screen. The owner cannot type into it; the app would have to draw its own chat view from the JSON stream.

### How each state is recognised

| State | (a) PTY + hooks | (b) print mode |
|---|---|---|
| Finished | `Stop` with empty `background_tasks`. Cannot tell "done" from "asked in plain text" without help | `result` `success`, exit 0. Same blind spot for a plain-text question |
| Waiting: permission | `PermissionRequest` | denied, listed in `permission_denials`, unless the app speaks the host protocol |
| Waiting: AskUserQuestion | `PreToolUse` matched to `AskUserQuestion` | host callback only; otherwise the tool is removed or denied |
| Waiting: idle | `Notification` `idle_prompt` after about 60 s | does not apply (the process exits) |
| Usage limit | `StopFailure` `error: rate_limit`; then the built-in wait | `rate_limit_event` `rejected` with `resetsAt`; non-zero exit (code not documented) |
| Failed | `StopFailure` other `error`; exit code; `SessionEnd` | `result` error subtype, `is_error`, exit code |
| Continue after reset | built in (up to three times), or `claude --resume <id> -- "continue"` | new process: `claude -p --resume <id> "continue"` |
| Continue on account B | stop, then `--resume <id>` with B's config folder | the same |
| Owner can step in | yes, it is a normal terminal | no |

### Not verified (manual checks)

**R4-M1. Hooks in a real interactive run.** With the app's plugin plus `StopFailure` and a `PreToolUse` hook matched to `AskUserQuestion`: start `claude --permission-mode manual`, ask it to "ask me one multiple-choice question, then run echo hi". Expect `PreToolUse` (AskUserQuestion), `PermissionRequest`, then `Stop`. Wait 70 seconds and expect `idle_prompt`.

**R4-M2. Continue by resume.** After a normal `Stop`, quit, then `claude --resume <id> -- "continue"`. Check that the first message is taken as a prompt and `SessionStart` says `resume`.

**R4-M3. The "done" marker.** Add one line to a ticket: "End your last message with the single line CODEPASS-DONE when everything is finished, or CODEPASS-QUESTION if you need an answer." Run five small tickets and count how often the last line is right.

### Options and recommendation

Recommendation: **(a)**. It is the same launch the plan already has, the owner can watch and type, permission prompts and questions work as usual, and the limit wait is built in. It needs three more hook signals than the plan lists: `StopFailure`, `PreToolUse` limited to the matcher `AskUserQuestion`, and the `Notification` type.

Design points for the next phase (decisions, not facts):
- "Finished completely" cannot be read from a signal alone. Either the owner confirms each ticket, or the ticket text asks for a marker line and report mode checks the last line of `last_assistant_message` locally and forwards only a yes/no (R4-M3).
- Pick one owner of "continue after a limit": Claude Code's automatic continue (on by default, stops after three) or the queue. If the queue owns it, `autoContinueAtUsageLimit` must be off, and only the owner can change that setting.
- The queue only runs unattended because the owner's own default is `bypassPermissions`. With any other default a ticket waits at the first prompt.
- Print mode stays in use for short helper calls (naming, `/usage`).

## R5. Codex (ChatGPT plans): the same three features

Codex is not installed on this machine, so nothing below was run. Extra tags in this section:
- **[src]** read in the open-source repository `openai/codex`, branch `main`, on 2026-11-16. Line marked "(summary)" means the reading tool summarised that part of the file; the field names still need a look at the real file.
- **[doc-s]** an official docs page whose relevant part was only summarised by the reading tool, not read word for word.
- **[search]** only a search-result snippet. OpenAI's policy pages (`openai.com/policies/...`, `help.openai.com`) refused automated reading (HTTP 403).

### R5a. Usage

**Facts**
- The app-server is Codex's documented interface for client programs. Start: "`codex app-server` (default stdio transport)", "newline-delimited JSON". "Clients must send a single `initialize` request per transport connection before invoking any other method". [doc] https://learn.chatgpt.com/docs/app-server
- It has `account/rateLimits/read` ("fetch ChatGPT rate limits") and a notification `account/rateLimits/updated` ("emitted whenever a user's ChatGPT rate limits change"). Fields: `rateLimits.primary` and `.secondary`, each with `usedPercent`, `windowDurationMins`, `resetsAt` ("a Unix timestamp (seconds) for the next reset"); `planType`; `credits`; `rateLimitReachedType`; `rateLimitsByLimitId`. [doc-s] same page
- `account/read` returns `account.type` (`chatgpt`, `apiKey`, `amazonBedrock`), `email` and `planType`. [doc-s] same page
- The team lead's field names are out of date. Today the internal snapshot is `RateLimitSnapshot { primary, secondary, credits, plan_type, rate_limit_reached_type, ... }` with `RateLimitWindow { used_percent, window_minutes, resets_at }`. `resets_at` is a Unix timestamp; `resets_in_seconds` appears nowhere in the files read. [src] (summary) https://github.com/openai/codex/blob/main/codex-rs/protocol/src/protocol.rs
- The snapshot rides inside the `token_count` event (`TokenCountEvent { info, rate_limits }`). [src] (summary) same file
- `token_count` events are written into the session's rollout file. [src] https://github.com/openai/codex/blob/main/codex-rs/rollout/src/policy.rs So the last-seen numbers of each session sit on disk, but they are only as fresh as that session's last reply.
- The numbers come from response headers of every model reply: `x-codex-primary-used-percent`, `x-codex-primary-window-minutes`, `x-codex-primary-reset-at`, the same for `secondary`, and `x-codex-credits-*`. [src] https://github.com/openai/codex/blob/main/codex-rs/codex-api/src/rate_limits.rs
- Codex itself also has a plain GET that needs no model request: `{base}/wham/usage` (ChatGPT sign-in; full path `/backend-api/wham/usage`) or `{base}/api/codex/usage`. [src] https://github.com/openai/codex/blob/main/codex-rs/backend-client/src/client/rate_limit_resets.rs It is not in the docs. Whether `account/rateLimits/read` uses it was not traced.
- Which window is which is not stated in the docs. Guides say primary is the 5-hour window and secondary the weekly one. [3p] `windowDurationMins` (300 or 10080) settles it at run time.
- TUI: "In the Codex CLI, you can run `/status` to see remaining limits." `/usage`: "View account token usage or use a rate-limit reset." The web dashboard is `chatgpt.com/codex/settings/usage`. [doc-s] https://learn.chatgpt.com/docs/pricing and https://learn.chatgpt.com/docs/cli/slash-commands
- `codex exec --json` events are `thread.started`, `turn.started`, `turn.completed` (with token `usage`), `turn.failed` (with `error`), `item.started/updated/completed`, `error`. None of them carries the rate-limit snapshot. [src] https://github.com/openai/codex/blob/main/codex-rs/exec/src/exec_events.rs

**When the limit is hit**
- Message text: "You’ve hit your usage limit." plus a plan-specific part (upgrade, or "Visit https://chatgpt.com/settings/usage to purchase more credits") plus " Try again at {time}." or " Try again later." The time is local, human-formatted text. [src] https://github.com/openai/codex/blob/main/codex-rs/protocol/src/error.rs
- App-server: the turn ends with `turn.status` `failed` and `error.codexErrorInfo` = `usageLimitExceeded`. [doc] https://learn.chatgpt.com/docs/app-server
- `codex exec` exits with code 1 after any error or failed turn. There is no special code for a limit. [src] (summary) https://github.com/openai/codex/blob/main/codex-rs/exec/src/lib.rs
- No "wait and continue after the reset" feature was found in docs or source.
- "After you reach your included limits, available credits let you continue working." And a turn already in progress may be allowed to finish. [doc-s] https://learn.chatgpt.com/docs/pricing So, as with Claude, credits mean the queue does not stop.
- No hook fires for a limit. `PostModelRequest` reports completed, failed or cancelled; the docs do not say what a limit looks like there, nor whether `Stop` fires after a failed turn. [doc] https://learn.chatgpt.com/docs/hooks

**Not verified (manual checks; install first: `npm i -g --prefix ~/.codepass-dev/codex @openai/codex`, then the owner runs `codex login`)**

*R5-M1. Usage without a request.* (5 minutes)
1. `codex app-server generate-json-schema --out ~/.codepass-dev/codex-schema` and read the schema of `account/rateLimits/read` (this is the exact truth for the installed version).
2. Start `codex app-server`, type the `initialize` request, the `initialized` notification, then `{"method":"account/rateLimits/read","id":2}`.
3. Check: `primary` and `secondary` with `usedPercent`, `windowDurationMins`, `resetsAt`. Compare with `chatgpt.com/codex/settings/usage`.
4. Call it twice a minute apart with no session running. Unchanged percentages mean it spends nothing.

*R5-M2. Limit reached.* Only possible when it happens. Keep: the TUI text, `codex exec --json "say ok"; echo $?` output, and which hooks fired (names only).

**Options and recommendation**

| Option | Fresh | Spends a request | Touches credentials | Verdict |
|---|---|---|---|---|
| A. `codex app-server` + `account/rateLimits/read`, one short-lived process per account | yes | probably none (M1) | no | **Recommended** |
| B. Read the last `token_count` line of the newest rollout file | only as fresh as the last reply | no | no, but reads Codex's session files | Backup |
| C. Call `/backend-api/wham/usage` directly | yes | no | **yes: reads `auth.json` or the keyring** | Reject |

### R5b. Several accounts

**Facts**
- "Codex stores its local state under `CODEX_HOME` (defaults to `~/.codex`)." [doc-s] https://learn.chatgpt.com/docs/config-file/config-advanced
- Sign-in is cached "in a plaintext file at `~/.codex/auth.json` or in your OS-specific credential store". `cli_auth_credentials_store` is `file`, `keyring`, `auto` or `ephemeral`. [doc-s] https://learn.chatgpt.com/docs/auth
- Keyring: service name `Codex Auth`, entry name `cli|<first 16 hex of SHA-256 of the canonical CODEX_HOME path>`. So each `CODEX_HOME` has its own entry. After a keyring save, `auth.json` is deleted. [src] https://github.com/openai/codex/blob/main/codex-rs/login/src/auth/storage.rs
- `auth.json` fields: `auth_mode`, `OPENAI_API_KEY`, `tokens` (`id_token`, `access_token`, `refresh_token`, `account_id`), `last_refresh`. [src] same file and `token_data.rs`
- Under `CODEX_HOME` by the docs: `config.toml`, profile files `<name>.config.toml`, `hooks.json`, `auth.json`, logs. The `sessions/` path and `history.jsonl` were not found in the pages read; the plan (4.8a) already assumes `~/.codex/sessions/`. [doc-s]
- `codex login` ("Authenticate the CLI with a ChatGPT account, API key, or access token"), `--with-api-key` (key on stdin), `--device-auth`, `codex login status` ("Print the active authentication mode and exit with 0 when logged in"), `codex logout`. [doc-s] https://learn.chatgpt.com/docs/cli/reference
- An API-key login "bills at standard API rates instead of your plan's included usage". It has no plan windows to show. [search] https://developers.openai.com/codex/auth
- Nothing official describes several accounts. The auth page says "the CLI and extension share one cached login." Profiles (`--profile`) are "named configuration layers"; nothing says a profile can hold its own login. [doc-s]
- `forced_chatgpt_workspace_id` and `forced_login_method` can log a user out when the credentials do not match. [doc-s] https://learn.chatgpt.com/docs/auth
- Resume: `codex resume <SESSION_ID>`, `codex resume --last`, `codex exec resume <SESSION_ID> [prompt]`, `codex fork`. [doc-s] https://learn.chatgpt.com/docs/cli/reference
- **A session records who made it.** `SessionMeta` has `creator_user_id` ("ChatGPT user that created this thread"), `creator_account_id` ("ChatGPT account selected when this thread was created. Never updated on resume.") and `account_scoped: bool`. [src] (summary) `protocol.rs` Whether resume refuses another account was not traced in the source.
- Reasoning items are stored in the rollout with `encrypted_content` and are persisted. [src] `rollout/src/policy.rs`, `protocol/src/models.rs` Whether another account can use them on resume is unknown. If not, the visible conversation survives but the model's earlier reasoning is lost, or the request fails.
- A changed model provider is refused: "`thread/resume` and new input return `-32600` with `data.type` set to `modelProviderMismatch`." [doc] https://learn.chatgpt.com/docs/app-server So ChatGPT sign-in to API key may count as a mismatch; not checked.

**Not verified (manual checks)**

*R5-M3. Second account.* `CODEX_HOME=~/.codex-b codex login`, then `CODEX_HOME=~/.codex-b codex login status` and plain `codex login status`. Check: two accounts, the first one untouched.

*R5-M4. Resume under the other account.* (10 minutes)
1. Under the default home: `codex exec --json --skip-git-repo-check "Remember the word walnut. Then run: echo hi"` in a scratch folder. Note the `thread_id` from `thread.started`.
2. Find that session's file under the default home's `sessions/` and copy it to the same relative path under `~/.codex-b/sessions/`.
3. `CODEX_HOME=~/.codex-b codex exec resume <id> "Which word?"`.
4. Pass: "walnut", exit code 0. Keep any error text (account, scope, encrypted content, provider mismatch).
5. Repeat with `~/.codex-b/sessions` as a symlink to the default home's `sessions`, and once with the interactive `codex resume <id>`.

**Options and recommendation**

| | (a) One `CODEX_HOME` per account | (b) One home, swap `auth.json` / keyring entry |
|---|---|---|
| Documented | the variable is; the multi-account use is not | no |
| Codepass handles a credential | no | **yes** |
| Running sessions when switching | untouched | all change account at their next token refresh |
| Resume under B | session file must be reachable in B's home (link or copy); account tag in the file is a risk | file is already there; same account-tag risk |
| Config, hooks, skills | per home; link `config.toml` if wanted | shared |

Recommendation: **(a)**, the same shape as Claude Code. The default account leaves `CODEX_HOME` unset. Note that spike-signals.md said "`CODEX_HOME` would lose auth and history, so not recommended"; that was about hook injection. For accounts, a separate home with its own sign-in is exactly the point. R5-M4 decides whether "switch and keep the context" is possible at all for Codex. If it fails, the fallback is a new session on B that starts from a summary the app asks A's session to write before the switch (lossy, and only possible while A still has allowance).

### R5c. Unattended queue

**Facts**

*TUI in the PTY, with hooks or `notify`*
- Hook events: `SessionStart`, `UserPromptSubmit`, `PreToolUse`, `PermissionRequest`, `PostToolUse`, `PreModelRequest`, `PostModelRequest`, `Stop`, `Interrupt`, `SessionEnd`, and compaction and subagent events. `Stop` carries `turn_id`, `stop_hook_active`, `last_assistant_message`. `PermissionRequest` "runs when Codex is about to ask for approval". [doc] and [doc-s] https://learn.chatgpt.com/docs/hooks
- "Non-managed hooks must be reviewed and trusted before they run." Trust is tied to the hook's hash. Hooks load from `hooks.json` or `[hooks]` in `config.toml` (user or project) or from an installed plugin. A `-c` route for hooks was not found. [doc] same page
- `notify` reports only `agent-turn-complete`. [doc-s] https://learn.chatgpt.com/docs/config-file/config-advanced
- No hook for "the AI asked the person a question" and none for a limit. The app-server has an experimental `tool/requestUserInput` request ("1-3 short questions"), so such a tool exists. [doc] https://learn.chatgpt.com/docs/app-server

*`codex exec`*
- "streams progress to `stderr` and prints only the final agent message to `stdout`"; `--json` gives the events listed in R5a; `-o` / `--output-last-message <path>` writes the final message to a file. [doc] https://learn.chatgpt.com/codex/non-interactive-mode
- "By default, `codex exec` runs in a read-only sandbox." A ticket that edits files needs a wider sandbox from the user's own `config.toml` or a flag. [doc-s] same page
- `--approve-for-me` routes approvals to an automatic reviewer. It is on the plan's never-pass list. [doc-s] https://learn.chatgpt.com/docs/cli/reference
- Exit code 1 after an error or a failed or interrupted turn, else 0. [src] (summary)
- Typing into a running `codex exec` is not documented. Continue with `codex exec resume <id> "continue"`. [doc-s]

*App-server as the host (third route)*
- `turn/completed` with `turn.status` `completed`, `interrupted` or `failed`, and `codexErrorInfo` (`usageLimitExceeded`, `unauthorized`, `sandboxError`, ...). Approvals and questions arrive as requests the client must answer. `account/rateLimits/updated` arrives live. [doc] https://learn.chatgpt.com/docs/app-server
- There is no terminal screen. The app would draw its own chat view.

**How each state is recognised**

| State | TUI + hooks | `codex exec --json` | app-server host |
|---|---|---|---|
| Finished | `Stop` (or `notify`). Same blind spot for a plain-text question | `turn.completed`, exit 0 | `turn/completed` `completed` |
| Waiting: approval | `PermissionRequest` | cannot be answered; the turn fails or the action is refused | a request to the client |
| Waiting: question | no signal | does not apply | `tool/requestUserInput` |
| Usage limit | **no hook**. Poll `account/rateLimits/read`, or see `rateLimitReachedType` | `turn.failed` / `error` with the "usage limit" text, exit 1 | `codexErrorInfo: usageLimitExceeded` |
| Failed | `PostModelRequest` failed (not confirmed), process exit | `turn.failed`, exit 1 | `turn/completed` `failed` |
| Continue after reset | no built-in wait; `codex resume <id>` with "continue" | `codex exec resume <id> "continue"` | `thread/resume` |
| Owner can step in | yes | no | only in a chat view the app builds |

**Not verified**: R5-M5, the existing `manual-checks.md` A8 (can Codex hooks be supplied per launch without a trust prompt), extended with: does `Stop` fire after a failed turn, and what does `PostModelRequest` carry on a failure.

**Recommendation**: the TUI in the PTY with hooks, as for Claude Code, plus the R5a app-server call before each ticket and after each `Stop` or process exit to tell "limit" from "done". If per-launch hooks are impossible (A8 fails), the queue falls back to `notify` (turn complete only) and the exit code. `codex exec` is the choice only for a "no terminal needed" queue mode, and then the sandbox comes from the user's own config, never from a flag the app adds.

### R5d. Terms (quotes only)

OpenAI's policy pages could not be read directly. The quotes below are from search-result snippets of `openai.com/policies/row-terms-of-use/` and must be checked on the live page.
- (a) "You may not share your account credentials or make your account available to anyone else and are responsible for all activities that occur under your account." [search]
- (a) Among the things a user may not do: "Interfere with or disrupt our Services, including circumvent any rate limits or restrictions or bypass any protective measures or safety mitigations". [search]
- (a) Not found: a clause about one person holding several ChatGPT subscriptions.
- (b) "Automatically or programmatically extract data or Output". [search]
- (b) Codex docs describe automation with a ChatGPT sign-in as an advanced case ("Maintain Codex account auth in CI/CD (advanced)": create `auth.json` once with `codex login`, "run Codex and persist the updated auth.json"), and say the recommended way for automation is an API key (`CODEX_API_KEY`). [search] https://developers.openai.com/codex/auth/ci-cd-auth and https://developers.openai.com/codex/noninteractive
- (c) "`~/.codex/auth.json` contains access tokens"; treat it as a credential. [search] same pages
- (c) The app-server is documented for building clients, and its `account/*` methods include sign-in and rate limits. [doc] https://learn.chatgpt.com/docs/app-server Not found: a statement for or against third-party apps reading a ChatGPT plan's credentials or calling `/wham/usage` themselves.

## R6. OpenCode (short)

OpenCode signs in to several providers. It offers a ChatGPT-plan sign-in today: "Use a ChatGPT Plus or Pro plan instead of an OpenAI API key." and "Usage counts against your plan, so these models show no cost." GitHub Copilot uses device sign-in. The providers page has no Claude subscription sign-in. [doc] https://opencode.ai/docs/providers It shows no usage or limits for a plan signed in this way, has no usage command in its CLI reference, and documents no rate-limit event. Its own plan, OpenCode Go, has 5-hour, weekly and monthly limits and reports each response's cost in `opencode.allowance`; current usage is on its web console. [doc] https://opencode.ai/docs/go The only failure signal seen so far is `session.execution.failed` with an `error.type` (spike-signals.md). Several accounts per provider are hinted at ("whenever the active Copilot account changes") but not described. Result: tickets can run in OpenCode; there is no meter and no limit gate, and a failed turn means "stopped, look at it".

## R7. Does one design fit all three tools?

| Design part | Claude Code | Codex | OpenCode |
|---|---|---|---|
| Account = a label + the tool's home folder variable | `CLAUDE_CONFIG_DIR` (documented for this) | `CODEX_HOME` (works by construction; not documented for this) | not available in a documented way |
| Usage meter per account | `claude -p "/usage"` → `usage_report` | `codex app-server` → `account/rateLimits/read` | none |
| Windows shown | session, weekly all models, weekly per model | primary and secondary (told apart by `windowDurationMins`), credits | none |
| Queue gate before each ticket | the same call | the same call | none |
| "Limit reached" during a run | hook `StopFailure` `rate_limit` | no hook; poll the meter, or parse the failure | failed turn only |
| Waits and continues by itself | yes, built in (default on) | no | no |
| Switch account and resume | link or copy the transcript; nothing in the docs ties it to an account | link or copy the rollout; **the file names its creator account**, so it may be refused | no |
| Credits keep it running past the limit | yes | yes | does not apply |

## Conflicts with the plan and the owner's wishes

1. **"The app stores no AI credentials."** Holds with the recommendations. It breaks with option R2 (b) (the app would keep a one-year token) and with option R1 D (the app would read Claude Code's sign-in). Both are rejected here.
2. **"Report mode forwards only task ID, event, session ID and permission mode."** Too narrow now. The queue also needs the `error` type of `StopFailure`, the `notification_type`, and perhaps a done/question flag. Still no prompt or answer text.
3. **"Only six events are subscribed; no tool-call hooks."** The queue needs `StopFailure` and one `PreToolUse` hook limited to `AskUserQuestion`.
4. **"Hooks injected per launch, never written into the user's settings."** Holds. But a status line cannot be injected this way as far as the docs show, so the status-line route is only a backup. Linking folders between config folders (R2) writes into the tool's own folders once; that needs the owner's consent and a line in the plan.
5. **"Launch through the login shell."** The owner's profile or `claude` function can set `CLAUDE_CONFIG_DIR`, `ANTHROPIC_API_KEY` or `CLAUDE_CODE_OAUTH_TOKEN` after the app's environment and so pick another account or API billing (R2-M3).
6. **`~/.claude` is written as a fixed path** in plan sections 4.8 (byte-identical test) and 4.8a (session data to delete). With several accounts it is "the account's config folder".
7. **Plan 4.8, soft cap: "all sessions of one tool share one sign-in and one usage allowance."** No longer true with several accounts.
8. **Wish 1, "check the weekly limit before each ticket".** Possible only if R1-M1 passes. The numbers also include the owner's use of the Claude apps.
9. **Wish 1, "pause the queue at the limit".** Claude Code already waits and continues by itself, and with usage credits on it never stops. The queue must show both settings.
10. **Wish 2, "continue on B with the context intact".** Not documented and not verified (R2-M2). The first turn on B re-sends the whole conversation, so the switch costs a visible share of B's 5-hour window.
11. **Wish 3, "show what Claude Desktop shows".** `usage_report` covers it, including the per-model weekly bar. The status line does not have the per-model bar.
12. **"Never a permission-skipping flag."** Holds. The queue is unattended only because of the owner's own default mode.
13. **Terms.** The quotes in R3 (b) and (c) apply to a queue and to any credential handling. The owner should read them before deciding.
14. **Codex hooks need a trust review** and no per-launch route is documented. "Hooks injected per launch, never written into the user's settings" may be impossible for Codex (`manual-checks.md` A8).
15. **Codex has no limit hook and no built-in wait.** The queue itself must poll the meter and must do the "continue after reset" for Codex, while Claude Code does it alone. The two tools need different queue logic here.
16. **Codex session files name the account that created them** (`creator_account_id`, `account_scoped`). "Switch to B and keep the context" may be refused for Codex (R5-M4).
17. **`codex exec` is read-only by default.** A no-terminal queue mode for Codex would need a sandbox setting. The plan's rule is about approval-skipping flags; a sandbox flag is a new case the plan does not cover.
18. **spike-signals.md advises against `CODEX_HOME`.** That advice was about hook injection and must be narrowed, since accounts need it.
19. **Codex was not installed**, so the whole Codex design rests on docs and source reading. OpenAI's terms pages could not be read directly.
