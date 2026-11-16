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

## R8. Carrying a conversation to another account

Date: 2026-11-16. Asked by the owner's statement S1: account A reaches its weekly limit, the task goes on under account B, and "nothing in the context may be lost".
Method: official pages, the Claude Code changelog, the `openai/codex` source at commit `75fc67e8f4` (branch `main`, 2026-11-16), public issue reports, and `claude --version` / `claude --help` (2.1.325). No session was started. No transcript, rollout file or credential was opened. Codex is still not installed.

Extra tags in R8 and R9:
- **[src]** read word for word in `openai/codex` at the commit above.
- **[3p-issue]** a public issue report or a comment on one. A hint, even when a maintainer wrote it.
- **[arch]** an OpenAI page read from its Internet Archive copy; the copy's date follows.
- **[inferred]** follows from the facts above it, but no page says it and nothing was run.

### Short answer

| | Claude Code | Codex |
|---|---|---|
| Visible messages, tool calls, tool results | Kept. They are plain data in the transcript. | Kept, when the request is accepted at all. |
| The AI's hidden reasoning from earlier turns | **Lost on purpose for the newest models.** The API drops it for another account, without an error. | Stored encrypted. Reports disagree on whether another account can use it. |
| Does plain resume under B work | The docs describe it as working: "keep sending the full history". Not run. | Works today by ID as far as the source shows. A check that refuses it exists in the source, switched off. Not run. |
| What "nothing lost" can honestly mean | Every visible message, tool call and result. Not the hidden reasoning. Not anything a compaction already summarised. | The same. |

So the wish holds for what the owner can see, and does not hold for the model's private reasoning. The UI should say so in one line:
"对话、工具调用和结果都会带过去。AI 之前没显示的思考带不过去。 / Messages, tool calls and results carry over. The AI's earlier hidden reasoning does not."

### Facts: Claude Code

**What a session is made of**
- Transcript: `projects/<project>/<session-id>.jsonl`. "Each line is a JSON object: a message, a tool use or result, a record of a request Claude Code sent the model, or a metadata entry. The entry format is internal to Claude Code and changes between versions, so scripts that parse these files directly can break on any release." [doc] https://code.claude.com/docs/en/sessions
- Beside the transcript, per session: `projects/<project>/<session>/subagents/` ("Subagent transcripts in the same format as the session transcript"), `projects/<project>/<session>/tool-results/` ("Large tool outputs spilled to separate files"), `projects/<project>/<session>/mcp-tasks/`, and `file-history/<session>/` (snapshots for checkpoint restore). [doc] https://code.claude.com/docs/en/claude-directory
- Outside `projects/` and `file-history/`, in the config folder: `plans/` ("Plan files written during plan mode"), `tasks/` ("Task lists written by the task tools"), `paste-cache/` ("Contents of large pastes"), `session-env/`. [doc] same page
- After a compaction, "The plan Claude wrote in plan mode" is "Re-injected from disk". [doc] https://code.claude.com/docs/en/context-window So a plan is read from `plans/`, which plan 4.16 does not link or copy. [inferred]
- No page says a session belongs to an account or an organisation. (Unchanged from R2.)

**Thinking blocks and their signature**
- "Full thinking content is encrypted and returned in the `signature` field on each thinking block. The API uses the signature to verify that thinking blocks were generated by Claude when you pass them back." [doc] https://platform.claude.com/docs/en/build-with-claude/thinking
- On a thinking block, three things are checked: "What came before it: `system`, `tools`, and every earlier message", "The model", and "The account: blocks from some models are tied to an account". [doc] https://platform.claude.com/docs/en/build-with-claude/preserved-thinking
- **Another account:** "A move to another account or cloud platform works the same way: keep sending the full history. Thinking blocks from Claude Fable 5.2, Claude Mythos 5.2, Claude Sonnet 5.5, and Claude Haiku 5.5 work only in the account that produced them or a linked one. In any other account, the API drops them, with no error." [doc] same page
- "A thinking block that the new model or account can't use never causes an error. The API drops it for that request and doesn't bill it, and Claude answers without that reasoning." [doc] same page
- What an "account" is: for the Claude API, "Your organization". [doc] same page. A personal subscription and a Team seat are separate organisations ("your Team plan creates a separate Claude organization"). [doc] https://support.claude.com/en/articles/9267247-get-started-with-the-team-plan What a "linked" account is, and whether two personal subscriptions can be linked, is not said.
- Models whose blocks are **not** in that list (Opus 5.5, Opus 5, Fable 5.1 and others) are not tied to the account. Under B the model can still read them, unless: "The API can also leave out a thinking block that another account produced, if that account is no longer in good standing." [doc] same page
- The API reports each dropped block when asked: `thinking_dropped` with the reason `organization_binding_mismatch`: "Another account produced the block, so the API dropped it. Not yours to fix." [doc] https://platform.claude.com/docs/en/build-with-claude/preserved-thinking/fix-bound-to-a-different-conversation
- `redacted_thinking` blocks carry encrypted `data` and follow the same rules as thinking blocks. [doc] https://platform.claude.com/docs/en/build-with-claude/thinking
- Which earlier thinking stays in context is per model: Opus 4.5 and later, Sonnet 4.6 and later, Haiku 5.5 and the Fable models "Keep all prior turns". [doc] same page. So on today's models the lost reasoning is all of it, not only the last turn's.

**The prefix check (why a hand-edited transcript can fail)**
- "A prefix check fails if `system`, `tools`, or an earlier message changed after Claude produced a block. It also fails if a thinking block between others was left out or moved." [doc] https://platform.claude.com/docs/en/build-with-claude/preserved-thinking/prefix-checks
- Result of a failure: a 400 error on Fable 5.2, Mythos 5.2 and Mythos 5.1; "Possible 400 error" on Fable 5.1, Opus 5.5, Sonnet 5.5 and Haiku 5.5. [doc] same page
- The error text: "Invalid `signature` in `thinking` block. The block is bound to a different conversation." [doc] https://platform.claude.com/docs/en/build-with-claude/thinking-troubleshooting
- "The prefix check looks only at content, not at sessions, IDs, or users. The same content in a new request or a fork passes it." [doc] prefix-checks page. So a new session ID alone changes nothing for the API.
- "Claude Code, claude.ai, or Claude Managed Agents manages your conversation history" is listed under "You have nothing to change". [doc] same page
- On resume, "the resumed conversation keeps the system prompt it started with". [doc] https://code.claude.com/docs/en/prompt-caching So the folder change does not by itself change the prefix. [inferred]
- The documented repair when thinking must go: "Remove the block the error names, and every `thinking` and `redacted_thinking` block after it ... Keep every other block, and send the request again. Don't put the removed blocks back." And: "Leave out the oldest blocks, the newest, or all of them, but none in between." [doc] fix page and https://platform.claude.com/docs/en/build-with-claude/preserved-thinking/change-tools-or-instructions
- "Thinking can't move to a conversation with a different history: start that conversation from a summary instead." [doc] change-tools page

**Claude Code already handles a sign-in change**
- 2.1.30: "Fixed 400 errors that could occur after running `/login` when the conversation contained thinking blocks". [doc] changelog
- 2.1.152: "Fixed sessions getting stuck after a model or login switch left stale thinking-block signatures in history; now stripped proactively with a retry safety-net". [doc] changelog
- 2.1.282: "Fixed sessions failing on every turn with an \"Invalid `data` in `redacted_thinking` block\" API error; Claude Code now drops the conversation's thinking blocks and retries once". [doc] changelog and https://code.claude.com/docs/en/errors
- No issue was found that reports a failed resume after a change of account on a current version. One open report describes a permanent 400 "thinking blocks cannot be modified" on resume in 2.1.153, with no account change. [3p-issue] https://github.com/anthropics/claude-code/issues/63147

**Other things that change under account B**
- Model: "Resumed sessions ... keep the model they were using when the transcript was saved." "A model you pick for the new launch with `--model` or `ANTHROPIC_MODEL` still takes precedence over the restored model." [doc] https://code.claude.com/docs/en/model-config
- A model that B's plan bills differently stops and asks: "Fable 5.1 now uses usage credits · the prompt to confirm went unanswered · nothing was sent". A model the organisation blocks is replaced with a notice: "Model \"claude-opus-4-8\" is restricted by your organization's settings. Using claude-sonnet-4-6 instead." [doc] https://code.claude.com/docs/en/errors
- 1M context: on Pro, Opus 4.6 with 1M "Requires usage credits"; the error is "Usage credits required for 1M context · run /usage-credits ... or /model to switch to standard context". [doc] model-config and errors pages
- A conversation larger than the new window: `/context` shows "Context exceeds the 200k-token limit by 94k tokens — run /compact or /clear to continue." With auto-compact on (the default): "send your next message. If it still fails with `Prompt is too long`, see that entry." [doc] errors page. So the conversation is compacted, which summarises it: "The summary keeps your requests and key code snippets, but detailed instructions from early in the conversation may be lost." [doc] https://code.claude.com/docs/en/how-claude-code-works
- **A dialog can appear.** "On Pro, Max, and Ultra plans, the dialog can appear when you resume a long conversation after a long break or on a different model." It is titled "Resume this conversation?" and offers "Resume" or "Start a new conversation". [doc] sessions page. An automatic switch must expect it.
- Cost: "When you resume a session, Claude Code sends the whole conversation again". "Caches are isolated between organizations". [doc] prompt-caching page. This confirms the R2 note: the first turn under B is uncached.
- A compacted session resumes from "the last compaction's summary, the messages that compaction left unsummarized, and every message since. The messages that the last compaction summarized aren't loaded again." [doc] sessions page. They stay in the file.
- Not restored by any resume: "Background Bash and monitor tasks aren't"; background work "shows up in the resumed transcript as a note that it didn't finish"; "Allow for this session" grants in a new process. [doc] sessions page
- Restored: plan mode (terminal resume without `--fork-session`), an active goal, session-scoped scheduled tasks. `bypassPermissions` is not restored; the session starts in "the permission mode a new session would start in". [doc] sessions page
- Flags to pass again on every resume: `--plugin-dir`, `--mcp-config`, `--add-dir`, `--settings`, `--fallback-model`. [doc] sessions page. The app's hook plugin is one of them.
- MCP servers and folder trust are per account (R2). Under B a server can be missing. Recorded tool definitions are reused when the transcript has them. [doc] prompt-caching page
- Web search results from the API's own search tool carry `encrypted_content`: "If `encrypted_content` is missing or modified, the request fails with a 400 validation error." [doc] https://platform.claude.com/docs/en/agents-and-tools/tool-use/web-search-tool Whether Claude Code's transcript holds such blocks, and whether they are tied to the account, is not documented.
- Files uploaded to the API are "scoped to the workspace they were uploaded in". [doc] https://platform.claude.com/docs/en/build-with-claude/files Whether a Claude Code transcript ever holds a file ID is not documented.
- Two processes on one session: "both write to one transcript. When you resume that session later, messages from one of them can be missing." [doc] sessions page. With linked `projects/` folders this covers A and B at once.
- A duplicate copy of one session ID in two project folders makes resume by ID "report not-found rather than resume an arbitrary copy". [doc] sessions page

**Fork**
- "Forking with `--fork-session` or `/branch` copies the history into a new session ID, leaving the original unchanged." [doc] how-claude-code-works page. `--fork-session` is "When resuming, create a new session ID". [local]
- With `--fork-session` plan mode is not restored. [doc] sessions page
- No documented command strips thinking blocks from a saved session. `/rewind` steps back to a checkpoint; `/clear` starts empty. [doc] errors page

**A turn cut off by a usage limit**
- The signal is `StopFailure` with `error: rate_limit`; `last_assistant_message` is "the API error string itself" (R1).
- At the reset Claude Code "prompts Claude to pick the task up where it stopped. It doesn't resend your last message." [doc] https://code.claude.com/docs/en/interactive-mode
- "Interrupted tool calls: a tool that was still running when the previous process ended, for example in a crash, doesn't get rerun when you resume." [doc] sessions page
- For errors in the middle of a reply: Claude Code "keeps what Claude completed, runs any tool calls Claude finished, and continues the turn from their results." [doc] errors page. That sentence is about server errors and dropped connections, not about limits.
- `CLAUDE_CODE_RESUME_INTERRUPTED_TURN`: "Set to `1` to automatically resume if the previous session ended mid-turn. Used in SDK mode". The message it sends is `Continue from where you left off.` "some turns resume only within six hours of their last message, such as one that ended with an API error". [doc] https://code.claude.com/docs/en/env-vars
- Old fixes show the two broken shapes that used to occur: "`--resume` failing with \"tool_use ids were found without tool_result blocks\"" (2.1.86) and "`--resume` dropping parallel tool results" (2.1.80). [doc] changelog
- What exactly the transcript holds after a limit (a partial assistant message, an error entry, a tool call without its result) is **not documented**. A limit is answered before a reply starts, so the likely shape is: complete earlier steps, then one error entry. [inferred] Check R8-M6.

### Facts: Codex

**The rollout file**
- First line `session_meta`. Fields that name the creator: `account_scoped` ("Scope discovery to the creator. Absent on threads created before account filtering."), `creator_user_id` ("ChatGPT user that created this thread"), `creator_account_id` ("ChatGPT account selected when this thread was created. Never updated on resume."). Also `id`, `session_id`, `forked_from_id`, `cwd`, `cli_version`, `model_provider`, `base_instructions`. [src] `codex-rs/protocol/src/protocol.rs`
- New threads are always written with `account_scoped` true. [src] `codex-rs/thread-store/src/local/create_thread.rs`
- Saved response items: `message`, `reasoning`, `function_call`, `function_call_output`, `local_shell_call`, `custom_tool_call` and its output, `tool_search_call` and its output, `web_search_call`, `image_generation_call`, `compaction`, `context_compaction`. [src] `codex-rs/rollout/src/policy.rs`
- `encrypted_content` is a field of `reasoning`, and also of `message`, `function_call`, `local_shell_call`, `tool_search_call` and `context_compaction` (optional), and of `compaction` (required: the item is nothing else). [src] `codex-rs/protocol/src/models.rs`

**What resume checks**
- A check exists: "Applies the rollout-gated CLI account check before resuming or attaching to a local thread. Creator tags are immutable; untagged threads and other clients retain existing behavior." It refuses with: "This thread was created with a different ChatGPT account or workspace. Sign in with the original account to resume it." [src] `codex-rs/app-server/src/request_processors/thread_resume_account.rs`
- It runs only when the feature `cli_resume_account_check` is on, the client is the TUI or `codex exec`, the thread has a creator tag, and the sign-in is a ChatGPT one. An API-key sign-in is not compared. [src] same file
- That feature is `Stage::UnderDevelopment`, `default_enabled: false`. [src] `codex-rs/features/src/lib.rs` So today a resume by ID is not refused for the account. This can change in any release.
- Listing is filtered separately: a thread with `account_scoped` shows only when its creator IDs match the current sign-in, when the client asks for the current account only. [src] `codex-rs/thread-store/src/local_history_account.rs` So `codex resume --last` and the picker under B may not show A's sessions. Resume by ID does not go through this filter. [inferred]
- A changed model provider is refused (R5b).

**Encrypted reasoning and another account**
- OpenAI's docs: reasoning items "include an `encrypted_content` property containing encrypted reasoning tokens that you can pass to future calls." They do not say which account or organisation can decrypt it. [doc] https://developers.openai.com/api/docs/guides/reasoning
- A maintainer's comment: "Were you using a different account when you created these threads originally? The encryption key is based on the account that you used to sign in." The error in that report: "Encrypted content organization_id did not match the target organization." [3p-issue] https://github.com/openai/codex/issues/13724 (2026-03-06)
- A later comment on the same report (2026-08-17, CLI 0.147.0): a thread with encrypted reasoning and compactions "continued normally under the second account", also when "copied into the second account's Codex home". [3p-issue] same link
- ChatGPT sign-in to API key in one session failed with "The encrypted content ... could not be verified." [3p-issue] https://github.com/openai/codex/issues/3942
- A session that failed with `invalid_encrypted_content` worked again after its `reasoning` rows and encrypted `compaction` records were removed. [3p-issue] https://github.com/openai/codex/issues/25290
- A user who swapped the sign-in file at a limit and resumed reports that since autumn 2026 the new launch keeps using the old account. [3p-issue] https://github.com/openai/codex/issues/50859 Whether a separate `CODEX_HOME` avoids this was not traced.
- So: undecided. The source and the reports show three outcomes (works, refused by the service, wrong account used).

**At a limit**
- "If you reach your usage limits during an active turn, the agent will be able to continue working on that turn, subject to fair use limits." [doc] https://learn.chatgpt.com/docs/pricing So a Codex limit usually falls between turns, and the rollout ends cleanly.

### The tools' own summaries (reference for a handoff note)

- Codex's compaction prompt, whole: "You are performing a CONTEXT CHECKPOINT COMPACTION. Create a handoff summary for another LLM that will resume the task. Include: - Current progress and key decisions made - Important context, constraints, or user preferences - What remains to be done (clear next steps) - Any critical data, examples, or references needed to continue. Be concise, structured, and focused on helping the next LLM seamlessly continue the work." [src] `codex-rs/prompts/templates/compact/prompt.md`
- The line Codex puts before such a summary: "Another language model started to solve this problem and produced a summary of its thinking process. You also have access to the state of the tools that were used by that language model. Use this to build on the work that has already been done and avoid duplicating work." [src] `codex-rs/prompts/templates/compact/summary_prefix.md`
- Claude Code after a compaction: the summary "keeps your requests and key code snippets"; the plan is "Re-injected from disk"; "Claude Code re-reads up to five" files, "most recently modified first"; project `CLAUDE.md` is "Re-injected from disk"; running background work is listed "so it doesn't start a duplicate". [doc] context-window and how-claude-code-works pages
- A new conversation can read an old one: "start a new conversation and Claude can look up the previous one if you refer to it". [doc] changelog

### Options and recommendation

**Order of attempts when a session moves from A to B** (each step only when the one before fails):

| Step | Claude Code | Codex | What is lost |
|---|---|---|---|
| 1. Resume as it is | `claude --resume <id> --model <the session's model>` in B's folder, with the app's flags again | `codex resume <id>` in B's home | Hidden reasoning (Claude Code: dropped by the API for the newest models). Nothing visible. |
| 2. The tool's own copy | `claude --resume <id> --fork-session` | `codex fork <id>` (not read in the source) | The same. New session ID. The original stays untouched. |
| 3. A repaired copy made by the app | see below | see below | All hidden reasoning. Nothing visible. |
| 4. A new session from a handoff note | see below | see below | The conversation itself. The note and the branch remain. |

Step 3 writes a file in a format both tools call internal. It can break with any release, so it stays behind steps 1 and 2 and behind a version check, and the app keeps the original file unchanged.

**Repaired copy, Claude Code** (a new file `<new-id>.jsonl` beside the original; the field names below are not from the docs, see R8-M2):
- Remove: every `thinking` and `redacted_thinking` block, all of them, never only some. A line whose content is then empty goes away, and lines that pointed to it point to its parent instead.
- Change: the session ID in every line to the new ID. The new ID is also the file name.
- Keep, unchanged and in order: every user message, every assistant `text` block, every `tool_use` with its `id`, every `tool_result` with its `tool_use_id`, compaction summaries and the lines after them, metadata lines.
- Check before use: every `tool_use` has its `tool_result` and the other way round. A `tool_use` at the very end without a result is removed (it never finished).
- Copy too: the folder `<old-id>/` with `subagents/` and `tool-results/` to `<new-id>/`. `file-history/<old-id>/` is only needed for rewind.
- Do not touch: text, tool inputs, tool outputs. Any such change is a change of the conversation.

**Repaired copy, Codex** (a new rollout file under `sessions/`, named with the new ID):
- Remove: every `response_item` of type `reasoning`; every `compaction` and `context_compaction` item (they are only encrypted text). When a compaction is removed, the items before it must be kept in its place, so the copy holds the whole visible history again; Codex then compacts by itself if it is too long.
- Strip: the `encrypted_content` field from `message`, `function_call`, `local_shell_call` and `tool_search_call` items.
- Change in `session_meta`: `id` and `session_id` to the new ID, nothing else. **Keep `creator_user_id`, `creator_account_id` and `account_scoped` exactly as they are.** (Planner's correction after the review, 2026-11-16; plan 4.16 decisions 28 and 31: deleting them would make the thread untagged, so Codex's own account check would no longer apply to it; rewriting a tool's record of who created a conversation to escape a product restriction is the "bypass any protective measures" that OpenAI's terms name. When Codex refuses a copy with its account-check sentence, the copy route is out and the handoff note is used.)
- Keep: `cwd`, `base_instructions`, `model_provider`, every user and assistant message, every call with its `call_id` and every output with the same `call_id`, `turn_context` lines, event lines.
- Not checked: whether Codex accepts a hand-made ID and finds the file without its own index (R8-M9).

**Handoff note** (built by the app from the transcript and from git, no AI call; saved in the task's data folder, sent as the first message of a new session):
1. One line that says what this is: a handoff from an earlier session of the same task, which could not be resumed.
2. The task: the ticket text or the first user message, word for word.
3. Every later user message, word for word, in order. These hold the requirements and the corrections.
4. The latest plan or to-do list the AI wrote, with each item's state.
5. The AI's last messages, word for word (the last one in full, a few before it shortened).
6. The step that was in progress: the last tool call, its input, and the end of its output; or "the last call did not finish".
7. Files the session wrote or edited (from its edit calls), plus the branch, `git status --short` and `git diff --stat` at the moment of the note.
8. The last commands that failed, each with the end of its output.
9. A question the AI asked that nobody answered.
10. Programs the session had started that were stopped at the switch.
11. Where the full old transcript is, as a path, with the sentence that it may be read when a detail is missing.
12. The closing instruction: check the branch state first, do not redo finished steps, then continue.

Left out on purpose: the hidden reasoning (it cannot be read), full tool outputs (the path in point 11 covers them). The note has a size cap; when it is over, older user messages stay and older AI messages go first. This matches what both tools keep in their own summaries: the requests, the progress, the next steps, the references.

Proposed copy for the fallback steps:
- "原样继续 / Continue as it is"
- "换一个会话编号再试 / Try again as a copy"
- "去掉思考记录再试 / Try again without the reasoning records"
- "用交接说明新开会话 / Start a new session from a handoff note"

### What this changes in plan 4.16 (for the planner)

1. Step 5 of "Switch and resume" jumps from a failed resume straight to a new session. Steps 2 and 3 above fit in between. The handoff note replaces "ticket text plus one sentence", and needs no allowance of account A.
2. The confirm text can say what is kept and what is not (the sentence under "Short answer").
3. Shared parts for Claude Code: `plans/` and `tasks/` belong with `projects/` and `file-history/`, or a resumed plan-mode session has no plan file.
4. Resume should pass `--model` with the session's model, and must handle three interruptions: the "Resume this conversation?" dialog, the usage-credits question, and a model replaced by the organisation's settings.
5. A session must never run under A and B at once on linked folders.
6. Codex: the creator tag is only a risk today, behind a switched-off feature. The encrypted items are the open question.

### Not verified (manual checks)

Nothing in R8 was run. Each check uses a scratch folder and a scratch session. `~/.claude-b` is the second folder from R2-M1.

**R8-M1. Plain resume under B, with thinking and a tool call.** (10 minutes)
1. `mkdir -p ~/.codepass-dev/carry-probe && cd ~/.codepass-dev/carry-probe && git init -q`
2. `ID=$(uuidgen | tr 'A-Z' 'a-z'); echo $ID`
3. `command claude --session-id $ID --effort high`. Type: "Think carefully: which is larger, 2^61 or 3^38? Then run: echo walnut > note.txt". Quit with `/exit`.
4. `SLUG=$(pwd -P | sed 's/[^A-Za-z0-9]/-/g')`
5. Copy only this scratch session: `mkdir -p ~/.claude-b/projects/$SLUG && cp ~/.claude/projects/$SLUG/$ID.jsonl ~/.claude-b/projects/$SLUG/`
6. `CLAUDE_CONFIG_DIR=~/.claude-b command claude --resume $ID`. Type: "Without tools: which word went into note.txt, and which number was larger?"
7. Pass: both answers right, no line starting with `API Error`. Write down: any dialog before the prompt, and the model shown.

**R8-M2. What the transcript holds.** (3 minutes, the scratch session of M1 only)
1. `F=~/.claude-b/projects/$SLUG/$ID.jsonl`
2. Line keys: `jq -r 'keys_unsorted | join(",")' $F | sort | uniq -c`
3. Block types: `jq -r '.message.content[]?.type? // empty' $F | sort | uniq -c`
4. Expect `thinking`, `text`, `tool_use`, `tool_result`. Check that the lines have `sessionId`, `uuid` and `parentUuid`. If the names differ, the filter in M4 must be changed.

**R8-M3. The tool's own copy.** (3 minutes)
1. `CLAUDE_CONFIG_DIR=~/.claude-b command claude --resume $ID --fork-session`, ask the question of M1 step 6, quit.
2. `ls ~/.claude-b/projects/$SLUG/`. Pass: a second `.jsonl` with another ID, and the first file's size unchanged.

**R8-M4. A repaired copy without thinking.** (5 minutes)
1. `cd ~/.claude-b/projects/$SLUG && NEW=$(uuidgen | tr 'A-Z' 'a-z'); echo $NEW`
2. Run:
```
jq -c -s --arg new "$NEW" '
  def strip: if (.message.content|type)=="array" then .message.content |= map(select(.type!="thinking" and .type!="redacted_thinking")) else . end;
  map(strip) as $rows
  | ($rows | map(select((.message.content|type)=="array" and (.message.content|length)==0 and (.uuid!=null)) | {key:.uuid, value:.parentUuid}) | from_entries) as $gone
  | def up($p): if $p!=null and ($gone|has($p)) then up($gone[$p]) else $p end;
  $rows[]
  | select(((.uuid // "") as $u | $gone|has($u))|not)
  | if has("parentUuid") then .parentUuid = up(.parentUuid) else . end
  | if has("sessionId") then .sessionId=$new else . end' $ID.jsonl > $NEW.jsonl
```
3. `grep -c '"thinking"' $NEW.jsonl` prints `0`.
4. `cd ~/.codepass-dev/carry-probe && CLAUDE_CONFIG_DIR=~/.claude-b command claude --resume $NEW`, ask the question of M1 step 6.
5. Pass: right answers, no `API Error`. (The filter was tried on a made-up sample only.)

**R8-M5. Another model on resume.** (3 minutes) `CLAUDE_CONFIG_DIR=~/.claude-b command claude --resume $ID --model haiku`, ask the same question. Write down whether "Resume this conversation?" appears and what it offers.

**R8-M6. What a limit leaves behind.** Only when a limit is really reached. This step reads one of the owner's real sessions, but prints types only, no text: `tail -n 6 <that session's .jsonl> | jq -c '{type, role: .message.role, blocks: [.message.content[]?.type?], apiError: .isApiErrorMessage}'`. Write down the last three lines. Then resume and check that the first reply does not repeat a finished step.

**R8-M7. Plan mode across folders.** (5 minutes) In the scratch folder start `command claude --permission-mode plan`, ask for a three-step plan to add a file, quit without accepting. Copy the session as in M1 step 5 and resume under B. Pass: it is in plan mode and still knows the plan. If not, repeat after `cp -R ~/.claude/plans ~/.claude-b/` (the plan files of all sessions; they are small text files).

**R8-M8. Codex: the file and the feature.** (5 minutes, after the install in R5 and one `codex exec` run in a scratch folder)
1. `codex features list | grep -i resume_account` (expect it listed as off; if the command differs, `codex --help`).
2. `R=$(ls -t ~/.codex/sessions/*/*/*/rollout-*.jsonl | head -n 1)` (the scratch run just made).
3. Header, without printing any ID: `head -n 1 $R | jq '.payload | {account_scoped, has_user: (.creator_user_id != null), has_account: (.creator_account_id != null), model_provider, cli_version}'`
4. Kinds: `jq -r '[.type, .payload.type] | join("/")' $R | sort | uniq -c`
5. `grep -c encrypted_content $R`

**R8-M9. Codex: resume under B, then a repaired copy.** (10 minutes; `~/.codex-b` from R5-M3)
1. Do R5-M4 steps 1 to 4 with a run that used reasoning and one command.
2. If it fails, keep the error text and make a copy: `NEW=$(uuidgen | tr 'A-Z' 'a-z')`, then
```
jq -c --arg new "$NEW" '
  select((.type=="response_item" and (.payload.type=="reasoning" or .payload.type=="compaction" or .payload.type=="context_compaction"))|not)
  | if .type=="session_meta" then .payload |= (.id=$new | .session_id=$new) else . end
  | if .type=="response_item" then .payload |= del(.encrypted_content) else . end' $R > "$(dirname $R | sed "s#$HOME/.codex#$HOME/.codex-b#")/rollout-$(date +%Y-%m-%dT%H-%M-%S)-$NEW.jsonl"
```
(create the target folder first with `mkdir -p`).
3. `CODEX_HOME=~/.codex-b codex exec resume $NEW "Which word?"`
4. Pass: "walnut", exit code 0. If Codex does not find the session, write that down: it then needs its own index, and the copy route is out. The account tags in `session_meta` stay untouched (plan 4.16 decision 28); if Codex refuses with "This thread was created with a different ChatGPT account or workspace", write that down too: the copy route is out for Codex and the handoff note (R8-M10) is the way.

**R8-M10. A handoff note by hand.** (5 minutes) In the scratch folder write `note.md` with points 1, 2, 3, 6, 7 and 12 of the list above for the session of M1, then `CLAUDE_CONFIG_DIR=~/.claude-b command claude "$(cat note.md)"`. Pass: the first reply checks the folder and continues without asking what the task is.

## R9. Terms and ban risk (verified quotes)

Date: 2026-11-16. Asked by S2 ("it must follow the official terms; this project is for my own use, not for resale") and S3 (task A on account A and task B on account B at the same time).
This section quotes; it is not legal advice, and no page gives the owner a promise. Where the terms are silent, it says "silent".
Anthropic's pages were read directly today. OpenAI's policy pages still refuse direct reading (HTTP 403), so they were read from Internet Archive copies; each quote names the copy's date. R3 and R5d hold the earlier, shorter quotes; the ones below are full sentences and replace the search-snippet quotes of R5d.

### Anthropic: quotes

**Consumer Terms of Service** (effective 2025-10-08) https://www.anthropic.com/legal/consumer-terms
- "You may not share your Account login information, Anthropic API key, or Account credentials with anyone else. You also may not make your Account available to anyone else."
- A listed prohibition: "Except when you are accessing our Services via an Anthropic API Key or where we otherwise explicitly permit it, to access the Services through automated or non-human means, whether through a bot, script, or otherwise."
- "You also must not abuse, harm, interfere with, or disrupt our Services, including, for example, introducing viruses or malware, spamming or DDoSing Services, or bypassing any of our systems or protective measures."
- A listed prohibition: "To develop any products or services that compete with our Services, including to develop or train any artificial intelligence or machine learning algorithms or models or resell the Services."
- "We may suspend or terminate your access to the Services (including any Subscriptions) at any time without notice to you if we believe that you have breached these Terms, or if we must do so in order to comply with law. If we terminate your access to the Services due to a violation of these Terms and you have a Subscription, you will not be entitled to any refund."
- "We may sometimes add or remove features, increase or decrease capacity limits, offer new Services, or stop offering certain Services."
- Silent on: how many accounts or subscriptions one person may hold.

**Usage Policy** (effective 2026-11-12), section "Do Not Abuse Our Platform" https://www.anthropic.com/legal/aup
- "Coordinate malicious activity across multiple accounts, or create multiple accounts, to avoid detection, circumvent product guardrails, or generate identical or similar inputs that otherwise violate our Usage Policy"
- "Utilize automation in account creation or to engage in spammy behavior"
- "Circumvent a ban through the use of a different account, such as the creation of a new account, use of an existing account, or providing access to a person or entity that was previously banned"
- "Intentionally bypass or interfere with capabilities, restrictions, or guardrails established within our products, including by circumvention of the account sign-up or log-in process or by instructing the model to produce harmful outputs (e.g., jailbreaking or prompt injection), without prior authorization from Anthropic"
- "Resell, proxy, or otherwise provide access to Claude through unauthorized means, including services that route requests through consumer subscriptions or misrepresent the product or client being used"
- Enforcement: "If we suspect that you may have violated our Usage Policy, we may warn you or throttle, limit, suspend, or terminate your access to our products and services."

**Claude Code legal and compliance page** https://code.claude.com/docs/en/legal-and-compliance
- "Claude Code usage is subject to the Anthropic Usage Policy. Advertised usage limits for Pro, Max, and Ultra plans assume ordinary, individual usage of Claude Code and the Agent SDK."
- "OAuth authentication is intended exclusively for purchasers of Claude Free, Pro, Max, Ultra, Team, and Enterprise subscription plans and is designed to support ordinary use of Claude Code and other native Anthropic applications."
- "Anthropic does not permit third-party developers to offer Claude.ai login into their own applications, or to route requests through Free, Pro, Max, or Ultra plan credentials on behalf of their users. Moreover, developers may not collect, store, or intermediate Claude.ai credentials or session tokens — sign-in to a Claude account must complete through Anthropic's own flow."
- "Nor does it prevent an end user from signing in to the unmodified Claude Code binary with their own Claude subscription"
- On products that run Claude Code: "The Claude Code binary must not be modified." and "Customers may not pay for, resell, or intermediate Claude usage on their end users' behalf."
- "Anthropic reserves the right to take measures to enforce these restrictions and may do so without prior notice."
- Which terms apply: "Commercial Terms of Service - for Team, Enterprise, and Claude API users" and "Consumer Terms of Service - for Free, Pro, Max, and Ultra users".

**Claude Code authentication page** https://code.claude.com/docs/en/authentication
- "To stay signed in to multiple accounts at once, such as work and personal accounts, give each account its own configuration directory. When you start `claude`, set the `CLAUDE_CONFIG_DIR` environment variable to the directory for the account you want to use. Each directory has its own settings, session history, and claude.ai login or API key."
- The page's own example: "so that `claude-work` uses your work account while `claude` keeps your personal one".
- An organisation can forbid other accounts on its machines: with `forceLoginOrgUUID`, "Claude Code reports an error for a claude.ai login to any other organization and exits at startup".
- On the long-lived token: "For CI pipelines, scripts, or other environments where interactive browser login isn't available".
- Env-vars page, `CLAUDE_CONFIG_DIR`: "Useful for running multiple accounts side by side". https://code.claude.com/docs/en/env-vars

**Help centre**
- Usage limits: "We may adjust usage limits at our discretion to manage capacity and ensure fair access to all users. For example, we may limit your usage with weekly and monthly caps or model and feature usage. We don't guarantee uninterrupted service." https://support.claude.com/en/articles/11647753-understanding-usage-and-length-limits
- Personal next to Team: "If you already use Claude with a personal email address, your Team is a separate account under your work email address." "If you choose to keep your personal account, your Team plan creates a separate Claude organization. You can switch between your personal account and the Team org ... Data isn't shared between separate accounts." "Once content has been moved into an organization, it can't be moved back to a personal account." https://support.claude.com/en/articles/9267247-get-started-with-the-team-plan
- Number of accounts: "We only allow a maximum of three Claude accounts to be verified with the same phone number." https://support.claude.com/en/articles/8287232-verify-your-phone-number
- Appeals: "If you feel your account has been wrongly suspended or terminated, please go to claude.ai, log in with your banned account, and fill out the appeal form". "If your Free, Pro, Max, or Ultra account has been banned for violating our Usage Policy, you can still log in at claude.ai to export your data or delete your account." https://support.claude.com/en/articles/8241253-safeguards-warnings-and-appeals
- Unattended use on a plan: quoted in R3 (b) (the Agent SDK article and the `autoContinueAtUsageLimit` sentence: "the setting grants unattended execution").
- No help article was found on account sharing or on holding several subscriptions.

### OpenAI: quotes

**Terms of Use** (published and effective 2026-01-01). Two pages with the same sentences: the general page [arch 2026-11-16] https://web.archive.org/web/20261116015406/https://openai.com/policies/terms-of-use/ and the rest-of-world page [arch 2026-11-14] https://web.archive.org/web/20261114155028/https://openai.com/policies/row-terms-of-use/
- "You may not share your account credentials or make your account available to anyone else and are responsible for all activities that occur under your account."
- "What you cannot do. You may not use our Services for any illegal, harmful, or abusive activity. For example, you may not:" ... "Modify, copy, lease, sell or distribute any of our Services." ... "Automatically or programmatically extract data or Output (defined below)." ... "Interfere with or disrupt our Services, including circumvent any rate limits or restrictions or bypass any protective measures or safety mitigations we put on our Services."
- "We reserve the right to suspend or terminate your access to our Services or delete your account if we determine:" "You breached these Terms or our Usage Policies." "We must do so to comply with the law." "Your use of our Services could cause risk or harm to OpenAI, our users, or anyone else."
- "Appeals. If you believe we have suspended or terminated your account in error, you can file an appeal with us by contacting our Support team."
- Silent on: how many accounts or subscriptions one person may hold.

**Usage Policies** (the copy shows the version "Effective: November 20, 2026") [arch 2026-11-15] https://web.archive.org/web/20261115074429/https://openai.com/policies/usage-policies/
- "We hold people accountable for inappropriate use of our services, and breaking or circumventing our rules and safeguards may mean you lose access to our systems or experience other penalties."
- Nothing in it about several accounts, account sharing or rate limits.

**Service Terms** (updated 2026-11-13) [arch 2026-11-14] https://web.archive.org/web/20261114100103/https://openai.com/policies/service-terms/
- Section "Codex and Code Generation" is one sentence, about third-party licences of generated code. Nothing about accounts.
- Workspaces: "ChatGPT Enterprise, ChatGPT Edu, and ChatGPT for Healthcare ... and ChatGPT Business accounts are managed by End Users with administrative privileges (\"Administrators\"). Administrators may be able to add, remove and suspend End Users' access to the organization's workspace."

**Help centre**
- "If you want to use ChatGPT under a different account, you'll need to manage the subscription separately". "Mobile subscriptions can't be transferred between ChatGPT accounts." [arch 2026-03-05] https://web.archive.org/web/20260305131644/https://help.openai.com/en/articles/20001056-why-am-i-seeing-a-message-that-my-subscription-is-associated-with-another-account
- Which terms cover Codex: "When you sign in to Codex using an existing ChatGPT account, the ChatGPT Terms of Use and Privacy Policy—or the corresponding online services agreement for OpenAI API and ChatGPT Enterprise, Education or Business Users—apply". [arch 2026-11-15] https://web.archive.org/web/20261115130255/https://help.openai.com/en/articles/11369540-using-codex-with-your-chatgpt-plan
- No help article was found on holding several accounts.
- The business agreement is said to have: "Customer will not share Account access credentials or individual login credentials between multiple users". [search] https://openai.com/policies/services-agreement/ Not read.

**Codex docs** (read directly today)
- Sign-in: "The CLI and extension share the same cached login details." "When you sign in with ChatGPT, Codex usage follows your ChatGPT workspace" policies. https://learn.chatgpt.com/docs/auth
- Nothing describes several accounts. `CODEX_HOME` is documented as the state folder only (R5b).
- Automation: "`codex exec` reuses saved CLI authentication by default. In CI, it's common to provide credentials explicitly". https://learn.chatgpt.com/docs/non-interactive-mode
- "Use API key authentication for programmatic Codex CLI workflows, such as CI/CD jobs." https://learn.chatgpt.com/docs/auth
- ChatGPT sign-in in CI: "This is an advanced workflow for enterprise and other trusted private automation. API keys are still the recommended option for most CI/CD jobs." "Treat `~/.codex/auth.json` like a password". "Do not share the same file across concurrent jobs or multiple machines." https://learn.chatgpt.com/docs/auth/ci-cd-auth
- Running in the background is a product feature: "Schedule recurring tasks to run in the background." https://developers.openai.com/codex/automations
- At a limit: "If you reach your usage limits during an active turn, the agent will be able to continue working on that turn, subject to fair use limits." "After you reach your included limits, available credits let you continue working." https://learn.chatgpt.com/docs/pricing

### Sorted

**Clearly allowed** (a page says so)
- Several Claude accounts signed in at once on one machine, one config folder each, "such as work and personal accounts". (Anthropic authentication page.)
- Signing in "to the unmodified Claude Code binary with their own Claude subscription". (Anthropic legal page.) This is all Codepass does with a sign-in.
- A personal Claude account next to a Team organisation, and switching between them. (Anthropic Team article.)
- Running Claude Code unattended in the forms Anthropic ships: automatic continue at a limit, `claude -p`, the long-lived token for scripts. (R3 (b).)
- Running `codex exec` with the saved sign-in, and scheduled tasks. (Codex docs.)
- Buying a separate ChatGPT subscription for a different account. (OpenAI help article.)

**Clearly not allowed** (a page says so)
- Giving a sign-in or an account to another person, either company.
- Reselling or proxying access; routing other people's requests through a subscription.
- An app that takes, stores or passes on a Claude sign-in or token, or that offers Claude.ai login itself. A modified Claude Code binary, or a client that pretends to be another one.
- Using a second account to get around a ban.
- Creating accounts by automation. Creating accounts "to avoid detection" or to "circumvent product guardrails".
- OpenAI: to "circumvent any rate limits or restrictions". The sentence stands under "Interfere with or disrupt our Services"; the terms do not say whether a second paid account counts.

**Grey** (the terms are silent, or two sentences pull in different directions)

| Case | Anthropic | OpenAI |
|---|---|---|
| (a) One person, several paid subscriptions of one company | **Grey, leaning allowed.** No clause forbids it. The docs describe several accounts ("work and personal"), and three accounts may share a phone number. No page says a second personal subscription is fine. | **Grey.** No clause forbids it; a help article explains a separate subscription per account. No page says it is fine. |
| (b) Task A on account A and task B on account B at the same time (S3) | **Allowed** as a mechanism: it is the documented use of `CLAUDE_CONFIG_DIR` ("multiple accounts side by side"). The answer to S3 is yes for two accounts that are both the owner's own. | **Grey, leaning allowed.** Nothing forbids two sign-ins on one machine; nothing describes it. |
| (c) Switching a task by hand to the owner's second account when the first is at its limit | **Grey.** Nothing names it. Claude Code handles a sign-in change inside a conversation (R8). Against it: limits "assume ordinary, individual usage", and "bypass ... restrictions ... established within our products" could be read to cover a usage limit. The route Anthropic offers for more usage is a higher plan or usage credits. | **Grey, nearer to the line.** "circumvent any rate limits or restrictions" are the words of the terms. The route OpenAI offers is credits or a higher plan. |
| (d) The same switch, done by an app at the moment of the limit | **Grey, more exposed than (c).** The same sentences, plus the Consumer Terms clause on "automated or non-human means". The app starts the official program and never holds a sign-in, but the pattern (use up one account, move on at once, around the clock) is the one Anthropic named in 2025 (see below). | **Grey, more exposed than (c).** The same sentence, and nothing in the docs that describes it. |
| (e) An unattended local queue that starts the official CLI, one ticket at a time | **Allowed in form** (the shipped unattended features), **grey in amount**: "ordinary, individual usage". | **Allowed in form** (`codex exec` with the saved sign-in; scheduled tasks), **grey in amount** ("fair use limits"). For CI the docs recommend an API key. |
| (f) A Team or Enterprise seat used next to a personal plan | **Allowed** side by side. The seat is under the Commercial Terms and the organisation's admin, who can forbid other accounts on a managed machine. **Moving a conversation between the seat and a personal account is grey**: the two are separate organisations with separate data terms, and the employer's rules decide. | **The same.** The workspace is run by its administrators; workspace policies follow the sign-in. |

Three points follow for the design (recommendations, not facts):
1. The answer to S3 is yes. The plan's "free choice per task" (decision 13) is the documented use.
2. Auto-switch (case d) is the part with the least support in the terms. Plan 4.16 turned it on by default once a tool had two accounts when this was written; the planner has since made it off by default (plan 4.16 decision 33). Off by default, with one sentence where it is turned on, fits S2 better: "条款没有写明能不能在用完时自动换账号。要不要开，由你决定。 / The terms do not say whether switching accounts automatically at a limit is allowed. Turning it on is your decision."
3. For an account that is a Team, Enterprise or Business seat, one line before a switch to a personal account: "「工作」是团队席位。把对话换到个人账号前，先确认你的团队允许。 / Work is a team seat. Check that your team allows it before you move a conversation to a personal account." The app cannot know the kind of account without storing it; the owner can mark it when adding the account.

### Ban-risk factors the companies have named (facts only)

1. **Account sharing and reselling.** When Anthropic announced weekly limits (2025-07-28), its message gave as reasons "policy violations like account sharing and reselling access" and "advanced usage patterns like running Claude 24/7 in the background". [3p] press report quoting the message: https://techcrunch.com/2025/07/28/anthropic-unveils-new-rate-limits-to-curb-claude-code-power-users/ The message itself was not found on an Anthropic page.
2. **Other programs using a subscription sign-in.** Since January 2026 Anthropic blocks third-party tools that send a Claude subscription sign-in as if they were Claude Code; press reports say some accounts were suspended by automatic filters and that Anthropic reversed part of this. [3p] https://www.theregister.com/2026/02/20/anthropic_clarifies_ban_third_party_claude_access The official text is the legal page quoted above, with "may do so without prior notice".
3. **Several accounts used to hide or to get around a ban; automated sign-up.** Usage Policy, quoted above.
4. **A client that is not what it says.** Usage Policy: "misrepresent the product or client being used".
5. **Phone number.** At most three Claude accounts per number (help article above).
6. **Accounts are linked on Anthropic's side.** The API "can also leave out a thinking block that another account produced, if that account is no longer in good standing." [doc] preserved-thinking page. So the service knows which account produced a conversation that another account continues.
7. **OpenAI's grounds for suspension** include "Your use of our Services could cause risk or harm to OpenAI, our users, or anyone else", beside a breach of the terms. (Terms of Use above.)
8. **OpenAI's automatic abuse checks have hit ordinary users.** Search results summarise an OpenAI status notice that its "abuse and fraud prevention systems" were "incorrectly rate limiting certain accounts" in Codex. [search] status.openai.com; the notice was not read.
9. **Codex records who created each session** (`creator_user_id`, `creator_account_id`) and holds a switched-off check that refuses another account. [src] R8.
10. No statement by either company was found that names "one person with two paid accounts" as a reason for a ban, and none that rules it out. User reports of suspensions without a stated reason exist for both. [3p]

What Codepass already does that matches the quotes: the sign-in stays inside the unmodified official program; the app stores no token; nothing is resold or shared; each account is the owner's own; no account is created by the app.

### Not verified

- The OpenAI pages were read from archive copies, not live. The Usage Policies copy shows a version that takes effect on 2026-11-20.
- OpenAI has separate terms for some regions (for example the EEA, Switzerland and the UK). Only the general and the rest-of-world pages were read. Which one binds the owner depends on the owner's country.
- OpenAI's Services Agreement (business accounts) was not read; its one quote is a search summary.
- Anthropic's Commercial Terms were searched for rules on seats and sharing; none was found in the text read. The search was by keyword, not a full reading.
- The 2025 message on weekly limits and the 2026 statements on third-party tools are known only from press reports.
- Neither company was asked. Only a written answer from them would settle cases (a), (c) and (d).

### Manual checks

**R9-M1. Read the live sentences.** (10 minutes) Open each page in a browser and find the words with Cmd-F. A missing or changed sentence means this section is out of date.
1. https://www.anthropic.com/legal/consumer-terms : "make your Account available", "automated or non-human means". Note the effective date at the top.
2. https://www.anthropic.com/legal/aup : "create multiple accounts", "Circumvent a ban", "route requests through consumer subscriptions".
3. https://code.claude.com/docs/en/legal-and-compliance : "ordinary, individual usage", "unmodified Claude Code binary".
4. https://code.claude.com/docs/en/authentication : "multiple accounts at once".
5. https://openai.com/policies/ : open "Terms of use" as shown for the owner's country: "share your account credentials", "circumvent any rate limits". Note which regional page opened and its date.
6. https://openai.com/policies/usage-policies/ : look for "account" and "rate limit". Expected: no rule on several accounts.

**R9-M2. Is any of the accounts a team seat?** (2 minutes) `command claude auth status --text` and, for the second folder, `CLAUDE_CONFIG_DIR=~/.claude-b command claude auth status --text`. Read the plan or organisation line on screen; do not paste the output anywhere. For Codex: `codex login status`, and the profile menu on chatgpt.com shows whether the account is a personal one or a workspace. A seat means case (f): ask the organisation before moving conversations.

**R9-M3. Is a managed policy on this machine?** (1 minute) In Claude Code run `/status` and look for a line about managed or organisation settings. If an organisation requires its own sign-in (`forceLoginOrgUUID`), a second account will be refused at start, and that is the organisation's decision.

**R9-M4. Ask the companies (the owner's choice; Codepass never does this).** One question per company through its help centre, from the account concerned: "I pay for two individual subscriptions myself. May I continue the same coding task in your official command-line tool under my second account when the first one reaches its weekly limit?" Keep the written answer. It is the only thing that turns case (c) from grey into yes or no.

**R9-M5. Settings that decide how "unattended" a session is.** (2 minutes) In Claude Code, `/config`: read "Continue automatically at usage limit". In `/usage`: read whether usage credits are on. In Codex, `/status`: read the credits line. These are the owner's own choices, and they change how much the tools do without the owner (R1, R5a).
