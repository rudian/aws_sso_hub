# Codepass 交接文档

更新：2026-11-17（第八版，里程碑 2 的 B 组合并后）。状态：**进行中**。里程碑 1 的任务全部合并，关卡只剩用户的事（测试频道、手动检查）。里程碑 2：A2-01 和 B 组的 7 个任务已合并（还没接到界面上）；**C 组正在跑：A2-08 和 F1-SG 一起跑，之后单独跑 A2-W**。用户的第三批需求（git）已写进计划，放在里程碑 4。

**2026-11-17 用户决定：程序和界面全部改用 Rust，界面库用 gpui + gpui-component（需求不变）。C 组合并后，不再做 Go 的新组。** 现在：C 组已合并（A2-08、F1-SG），这是最后一组 Go；gpui 小样做完了，报告在 `docs/research/rust-gui-spike.md`；用户亲手试过中文输入和 VoiceOver，都没问题；改造设计稿在 `docs/plan/codepass/rust/`（`design.md`、`ui-native.md`、`crate-facts.md`），用户的答案写在 `design.md` 第 13 节：Windows 用 GitHub Actions 编译，清了缓存腾出硬盘；**等用户批准整份设计**，批准后用 writing-plans 重排 `tasks.md`；第四批需求（AWS 登录后自动拉取）已写进计划（G4-14）。下一步见第 1 节第 5 条。

这份文档给下一个 session 用。用户给新 session 的 prompt 会很短，**所有规则都写在这里**。先读完这份，再照第 1 节做。

---

## 0. 30 秒看懂

- **在做什么**：把 AWS SSO Hub 改版成 **Codepass 1.0.0**。用户的目的：用一个 app 代替 JetBrains IDE（WebStorm、PhpStorm、GoLand）、Warp、Bitbucket 网站、Claude Desktop（只用来看周用量）和 Sublime Text（记以后要做的想法），少开软件、省电脑资源。
- **功能**：项目分组；多个 AWS SSO / GitHub / Bitbucket 账号；完整的 git 客户端；作者身份；OTA 更新；多会话的 AI 编码终端（Claude Code / Codex / OpenCode，每个任务一个 worktree）；带代码提示的编辑器（Go、TS/JS、PHP）；AI 运行时防止睡眠；系统通知；**票队列、多个 AI 账号、用量表（新）**。平台是 macOS arm64（15 及以上）和 Windows。所有功能做完后一次发布 v1.0.0。
- **这个 session 做了什么**：
  - F1-21 合并完成（`2c24815`，`main` 的 0.0.2 已经在 `codepass` 里）；
  - F1-22 合并完成（`f3fc3e2`，登录快过期、已过期的系统通知）；
  - F1-CL 合并完成，按文件拆成两个并行任务：`0ab5df7` F1-CLa（Go、脚本、文档）、`86e82d4` F1-CLb（前端，包括用户要的项目页搜索）；
  - 删掉了多余的本地分支；workflow 以后合并、push 成功后自动删 task 分支。
- **进度**：约 **31%**（第 9 节）。里程碑 0 完成。里程碑 1 的任务全部合并，关卡只剩用户的事。里程碑 2 做了一半（A2-01 和 B 组已合并）。
- **代码**：在 `codepass` 分支，已经 push。`main`（0.0.2）已经用 `--no-ff` 合并进来，`git log codepass..main` 是空的。
- **顺序**：里程碑 1 关卡 → 里程碑 2（最后一组是新加的 A2-CL，清掉 F1-CL 留下的 11 条小问题）→ 3 → **8** → 4 → 5 → 6 → 7。

## 1. 新 session 的第一步

1. **跟用户说话**：中文，ELI5 风格。短句。每做完一组，用简单的话说三件事：**做了什么、成功没有、下一步是什么**。只有要用户做决定或手动检查时才问；最多两个选项，并说你推荐哪个。
2. **确认状态**：
   - `git status` 是干净的。
   - `git log --oneline -3`：最上面是这份文档的 commit。
   - `git worktree list` 只有主目录。
   - `git log --oneline codepass..main` 什么都不打印（F1-21 已合并）。
   - `git branch` 只有 `codepass`、`main` 和 `spike/*`。
3. **按顺序读**：
   - 本文件；
   - `docs/plan/codepass/tasks.md`：「How the work runs」、「Owner decisions (FINAL)」和它下面的两批 owner requirements、open questions、「Follow-ups」；
   - 同一文件里程碑 1 的「Gate」、里程碑 2 最后的 A2-CL 条目、「Follow-ups」最后的 Group H（FU-CLa1–4、FU-CLb1–7）；里程碑 8 先略读（它排在里程碑 3 后面）；
   - `reviews/groupH-wiring.md`（F1-CL 留给关卡和里程碑 2 的话）；`reviews/groupE-wiring.md` 和 `reviews/groupG-wiring.md` 的 deviations（关卡要请用户确认）。
4. **跑一次 gate**：`bash docs/plan/codepass/orchestration/gate.sh`。期望每一项都是 0，前端测试是 81 个文件、1005 个测试（里程碑 2 的 C 组之后）。
5. **然后直接开工**：C 组已合并（2026-11-17）。**不要再跑 Go 的新组**：A2-W 和 D 组取消，用户 2026-11-17 决定改用 Rust + gpui。接下来按顺序：(1) gpui 小样（报告在 `docs/research/rust-gui-spike.md`；用户亲手试中文输入和读屏，结果记下来；小样的代码在旧 session 的临时目录里，没有就照报告重做）；(2) Fable 把界面设计改成原生版（需求不变）；(3) Opus 写改造设计，用户批准；(4) Opus 重排 `tasks.md`：先把已做完的部分用 Rust 重做，再接着做剩下的任务，全部用 Rust；(5) Sonnet 一组一组做。Go 代码先留着当参考，Rust 版追上后再删。细节在第 5 节 4f、4g。用户的新 prompt 就是开工的许可，不用再问。

## 2. 用户定的工作方式（每个 session 都照做）

- **模型分工**（用户定的，为了省 token）：
  - **Fable**：UI 设计和产品研究。改设计稿、`ui-design.md`、`docs/research/` 的调研笔记、`product-plan.md` 里的产品部分。
  - **Opus**：规划、任务分发和分区、review 和验证。
  - **Sonnet**：写代码、修 bug、合并。在 Chrome 里操作也用 Sonnet。
  - **用完的 subagent 要关掉**（`TaskStop`）。
- **每组任务跑一个 Workflow**，脚本是 `docs/plan/codepass/orchestration/codepass-group-v2.js`（第 6 节）。
- **安全红线**见第 7 节。仓库是公开的，**每次 commit 前都要跑 scrub**。
- **commit 和 push**：都在 `codepass` 分支上。workflow 的合并步骤会自己 push；orchestrator 改的文档也要 commit 并 push。
- **汇报**：每组结束后用中文、简单的话告诉用户（见第 1 节第 1 条）。

## 3. 用户的第二批需求（2026-11-16，已写进计划，代码还没写）

| 需求 | 任务 | 在哪里写着 |
|---|---|---|
| `main` 上 0.0.2 的修复要带进 Codepass：多个 Identity Center 用户时的「控制台」、同一个门户两个用户先后登录、每个账号用自己的无痕窗口登录、会话时长由 AWS 管理员决定（app 不能改） | **F1-21** ✅ 已合并（`2c24815`） | tasks F1-21；plan 4.2；检查 C19–C21、C26 |
| AWS 登录快过期、已过期时发系统通知，写明是哪个登录和哪些账户，通知上有「重新登录」按钮 | **F1-22** ✅ 已合并（`f3fc3e2`） | plan 4.15；design 5.14、6；`docs/research/notifications.md`；检查 C22–C24、B19 |
| 票（ticket）：编辑中 / 暂不执行 / 待执行。待执行的排队一个一个跑，跑完一个才开始下一个；开始前检查用量，到上限就暂停 | **里程碑 8** | plan 4.17；tasks 里程碑 8 |
| 多个 Claude 账号：A 的周用量用完就换 B，A 上正在用的会话在 B 上接着用，上下文不丢 | **里程碑 8** | plan 4.16 |
| 在 Codepass 里看 Claude 的用量（代替 Claude Desktop） | **里程碑 8** | plan 4.16 |
| 用量、多账号切换、票队列也要支持 GPT（Codex），因为有的用户不用 Claude | **里程碑 8** | plan 4.16、4.17；`docs/research/claude-usage-accounts.md` |
| PHP 也要代码提示（悬停、补全、跳到定义、引用、诊断） | **E5-11** | plan 4.12；tasks E5-11 |
| 在仓库列表里打文件夹名就能快速找到仓库、马上开始干活 | **F1-CL** ✅ 已合并（`86e82d4`，项目页搜索：按文件夹名排序、上下键选、回车打开）和 **A2-16**（任何页面按 Cmd/Ctrl+O 打开仓库，复用同一个排序） | plan 4.4；design 4.3、5.2、5.7c；检查 C25 |
| 多个 Claude / GPT 账号随时来回切换；仓库 A 默认用账号 A，仓库 B 的任务可以自由选 A 或 B | **T8-01、T8-05、T8-09、T8-10** | plan 4.16「Choosing the account」；design 4.2、5.2、5.6、5.16 |
| A 的周用量用完 → 换 B 接着做，**上下文一点都不能丢** | **T8-05、T8-11**（三层：同一个会话、修好的副本、交接笔记；永远不会从零开始） | plan 4.16「Switching keeps the conversation」；调研 R8 |
| 多账号要符合 Claude 和 OpenAI 的官方条款、别被封号；只给自己用，不转售 | **T8-01、T8-04、T8-05、T8-07** | plan 4.16「Several accounts within the terms」；调研 R3、R5d、R9 |
| TASK A 用账号 A、TASK B 用账号 B，同时跑 | 支持（Anthropic 文档写的「同时登录工作账号和个人账号」就是这种用法） | plan 4.16；调研 R2、R9 |
| **第三批（2026-11-17，git）**：像 Bitbucket 一样，选任何一个分支来新建分支 | **G4-02、G4-07** | plan 4.6a；design 5.8「New branch」 |
| 每个仓库、每个分支显示还没推送的提交数（↑n）和远程有、还没拉取的提交数（↓n），并说明上次检查远程是什么时候 | **G4-13、G4-06、G4-07**（项目页的 ↑↓ 里程碑 1 已经有了） | plan 4.6a；design 4.2、5.2 |
| 「拉取全部」：一个按钮，把每个仓库的每个本地分支都更新到远程最新（只做不用决定的更新；不推送、不合并） | **G4-11、G4-12** | plan 4.6a；design 5.2b；`docs/research/git-pull-all.md` |

**规划时 planner 自己定的事**（用户可以改；都写在 `tasks.md` 里）：
- 全部进 1.0.0（和 decision 4 一致）。
- F1-21、F1-22 在里程碑 1 关卡之前做：它们改的是关卡要检查的账号代码，而且替换 AWS SSO Hub 的那个版本不能丢掉 0.0.2 的修复。F1-CL 挪到最后，顺便清掉 F1-21、F1-22 留下的 follow-up。
- 里程碑 8 紧跟在里程碑 3 后面：它需要 AI 任务（里程碑 2）和每个任务的 worktree（里程碑 3），而用户现在就会撞到周上限。编号是 8，**顺序是 0、1、2、3、8、4、5、6、7**。
- F1-21 要用真正的 merge（不能 squash，也不能 cherry-pick），这样 `main` 成为 `codepass` 的祖先，发布时合并回 `main` 才干净。
- 通知：「已过期」有自己的开关（默认开）；Windows 的免安装版如果不能弹通知（检查 B19），就只用 app 内提示。
- 里程碑 8（细节在 plan 4.16、4.17 和 `tasks.md` 里程碑 8）：
  - 每个 AI 账号一个配置文件夹（Claude 用 `CLAUDE_CONFIG_DIR`，Codex 用 `CODEX_HOME`）。登录用工具自己的登录。**Codepass 不读、不存任何密码或令牌。**对话文件夹用链接共享，这样换账号后还能接着聊。
  - 用量让工具自己报（Claude：`claude -p "/usage"` 读 `usage_report`），不调没公开的接口。
  - 票在 app 的终端里跑普通会话，你能看、能打字。「做完」靠 AI 最后一行写的标记；没把握就停下来等你，并发通知。
  - 撞到上限：Claude Code 自己会等重置后继续，队列不跟它抢。**「自动换账号」默认关**（一撞上限就自动跳到下一个账号，最像「绕过限额」）；按工具分别打开，打开时先给你看两家条款的原文。关着时，限额横幅和通知上都有「在 B 上继续」，一键手动换。超额付费额度默认不让队列用。
  - 选账号：每个任务都能选任何账号；默认账号分三层：仓库 → 项目 → 全局（像作者身份）。会话栏上的账号是一个「换账号」菜单，随时来回切，只在两轮之间切。
  - 换账号时对话不丢：先试同一个会话；不行就用「修好的副本」（只去掉只属于旧账号的部分，原文件不动）；再不行就用 app 自己从对话记录整理的「交接笔记」（不花任何额度）。结果会告诉你用的是哪一层。
  - 最新几种 Claude 模型的「隐藏思考」换账号后带不过去（服务方的规定），但**任务照样能接着做**：隐藏思考只是 AI 每次回答前打的草稿，结论已经写在回答和代码里；换过去后 AI 读完整段对话、重新思考再接着做。Anthropic 文档也说换账号时照常发送完整对话就行。代价是第一轮慢一点、多用一点新账号的额度。说明写在 plan 4.16 decision 22「Why the task still continues」，用检查 D13 验证。
  - 条款：只用你自己的账号；只启动官方原版工具；不碰密码和令牌；不自动注册或登录；每个账号只用自己套餐里的额度；工具说账号被停用、被封或被登出时，马上停用那个账号，绝不自动挪到别的账号。设置里写着「Codepass 只用于你自己的账号和你自己的工作」。
  - OpenCode 的票也能排队，但没有用量检查，也没有多账号。
- **里程碑 8 开工前，用户要做两件事**：读 plan 4.16「Several accounts within the terms」和 4.17「Terms」（原文引用在调研笔记 R3、R5d、R9），确认队列可以自己开下一张票、多个订阅怎么用；做手动检查 D1–D4（Claude，两个账号）、D9–D12（Codex，两个 ChatGPT 账号）和 R8 的检查（D13 起），各大约 20 分钟。
- Codex 和 Claude Code 的三个不同（来自调研，还没实测）：Codex 没有「到上限」的信号，也不会自己等，所以队列自己查用量、自己在重置后继续；Codex 的会话文件记着是哪个账号建的，换账号后同一个会话可能接不上（D11 决定），那就用第二层或第三层，对话照样带过去；Codex 的 hooks 可能要先过信任确认（A8、D12）。

## 4. 文件在哪里

计划、设计稿和调研笔记**都在仓库里**。

⚠️ **仓库是公开的**：
- 文档也在 scrub 检查的范围内，提交前必须跑 scrub。
- 设计稿和文档里只能用假数据：假名字（acme、globex、initech、umbrella、alex-dev 等）、`example.com` 邮箱、账户 ID `111122223333` 这种、目录 ID `d-1234567890`、公司名写成 `<owner company>`、home 路径写成 `~`。

**只留在本机、不进 git 的**（`.git/info/exclude` 里列着，里面有真实账号数据，不要 commit）：
- 旧版 0.0.1 的计划和设计：`docs/plan/implementation-plan.md`、`docs/design/ui-spec.md`、`docs/design/mockup.html`、`docs/design/copy.md`、`docs/design/shots/`；
- 旧版的调研：`docs/research/aws-sso.md`、`docs/research/git-codecommit.md`；
- workflow 的原始记录：`docs/plan/codepass/reviews/*.json`；
- `docs/progress/`。

| 路径 | 内容 |
|---|---|
| `docs/plan/codepass/tasks.md` | **唯一的任务清单**。每个任务有 Build / Owns / Deps / Accept / Tests / Review。还有用户决定、两批需求、open questions、Follow-ups（FU-xx）和每组的合并记录。 |
| `docs/plan/codepass/product-plan.md` | 产品方案，文中引用为 "plan N.N"。新的：4.15 系统通知、4.16 AI 账号和用量、4.17 票队列。 |
| `docs/plan/codepass/ui-design.md` | 设计规范，文中引用为 "design N.N"。新的：第 6 节「System notifications」表、5.14 的提醒设置、里程碑 8 的新页面。 |
| `docs/plan/codepass/manual-checks.md` | 用户要亲手做的检查（中文）：A、B（含 B19）、C（C1–C28）、D（里程碑 8） |
| `docs/plan/codepass/reviews/` | 每组的接线笔记 `group*-wiring.md`、传给 workflow 的说明 `group*-args.md`（原始结果 `*.json` 只在本机） |
| `docs/plan/codepass/orchestration/` | `codepass-group-v2.js`（每组用的 workflow 脚本）和 `gate.sh` |
| `docs/plan/codepass/shoturl.sh` | 截图工具（headless Chrome，支持 `file://`） |
| `docs/design/codepass-mockup/` | 静态设计稿，`shots/` 里是对照截图（1440×900） |
| `docs/research/` | 调研笔记和 spike 笔记。新的：`notifications.md`（Wails 3.1.0 通知）、`claude-usage-accounts.md`（Claude Code 和 Codex 的用量、多账号、无人值守） |
| 本地分支 `spike/*` | spike 的代码，以后的任务要用。**永远不要 push。** |
| 仓库外 `~/.codepass-dev/branch-backups/` | 2026-11-16 删掉的 21 个 task 分支（F1-01 到 F1-20、F1-W）的备份（git bundle），`README.txt` 写了怎么还原。以后的 task 分支合并后直接删，不备份（代码已经在 `codepass` 里）。 |
| 仓库外 `~/.codepass-dev/scrub-list.txt` | scrub 屏蔽名单（权限 600）。**绝不打开、打印或引用它。** |
| 仓库外 `~/Documents/code/.codepass-wt/` | 任务 worktree 的根目录（现在是空的） |

**在 git 里的**：代码；`docs/release.md`（发布流程、钥匙仪式、测试频道）；`scripts/scrub-check.sh`；`build/release/`。

## 5. 接下来做什么

1. **F1-21（F 组）✅ 已完成**（2026-11-16，`2c24815`，`--no-ff` 合并；1 轮修复后通过）。留下 3 条 follow-up（FU-21a 到 FU-21c，都归 F1-CL）和给用户的手动检查 C26。传给 workflow 的说明在 `reviews/groupF-args.md`（`portBase` 用过 9700）。
2. **F1-22（G 组）✅ 已完成**（2026-11-17，`f3fc3e2`，squash 合并；review 找到 3 个 major、4 个 minor，1 轮修复后通过）。系统通知服务 `internal/notify`，AWS 登录快过期 / 已过期的通知和「重新登录」按钮。留下 7 条 follow-up（FU-22a 到 FU-22g，都归 F1-CL）；手动检查 C22–C24、B19 按实际做法改写了。传给 workflow 的说明在 `reviews/groupG-args.md`（`portBase` 用过 9760）。
3. **F1-CL（H 组）✅ 已完成**（2026-11-17）。按文件拆成两个并行任务：`0ab5df7` F1-CLa（Go、脚本、文档）和 `86e82d4` F1-CLb（前端）。各 1 轮修复后通过，合并时没有冲突。
   - 修了大约 80 条 follow-up，做了用户要的项目页搜索，挂上了 `UpdatesCard`、`UpdateBanner`、`ToolRows`。
   - 留下 11 条小问题（FU-CLa1–4、FU-CLb1–7），都归新加的 **A2-CL**（里程碑 2 的最后一组）。
   - 手动检查：C25 按实际做法改写；新加 C27（在 Health 页更新工具）；B11、B14–B16、B19、C17、C18、C23 加了步骤。
   - 传给 workflow 的说明在 `reviews/groupH-args.md`（`portBase` 用过 9820）。
4. **里程碑 1 关卡**（`tasks.md` 里里程碑 1 的「Gate」）：
   - **orchestrator 自己做**：
     - 标准 gate（2026-11-17 已过：63 个文件、841 个测试）。
     - 用 mock 重新截 Gate 里列的所有截图，和 `docs/design/codepass-mockup/shots/` 并排对比。
     - 整理一份偏差清单给用户确认：E 组的偏差（`reviews/groupE-wiring.md`）、FU-17e、F1-22 的设置行和通知文字、F1-CL 的搜索和 Health、启动弹窗的措辞。
   - **要用户来**：
     - 测试频道：私有仓库、测试签名钥匙、只读令牌、`gh auth login`（`docs/release.md`「The test channel」）。有了它，orchestrator 才能出签名、公证过的 rc 版本。
     - 手动检查：C 组（C1–C28）、A9、A16、A17、B1–B3、B5、B9、B15–B19。
   - **2026-11-17 进度**：截图和设计对比做完了（37 张，Fable 对比，用户在页面上定了）。31 处照现在的样子接受，19 处由 F1-GF 修（I 组）。每一条和决定都在 `reviews/m1-gate-review.md`。用户说的两件事：(1) 登录到期发通知、通知上有「重新登录」按钮，这是 F1-22，已经做好；只改「自动续期登录」下面那句说明。(2) 一个门户下可以有多个账号和角色（同一个账号的不同角色，或者不同的账号），已经做好，新加手动检查 C28。D27（设置页两列）放进 A2-CL。剩下要用户来的：测试频道和手动检查；查出来的问题放进 A2-CL。
4a. **I 组 ✅ 已完成**（2026-11-17）：`4a7e6b5` F1-GF（关卡的 19 处修复）、`90f1772` A2-01（终端里程序的启动和停止，`internal/pty`）。各 2 轮修复后通过；gate：64 个文件、863 个测试。留下 7 条 follow-up：FU-GFa（项目页窗口 1200 宽时「更多」按钮被挡住，**F1-GF2**，放进 B 组）、FU-GFb、FU-GFc、FU-A2-01c（A2-CL）、FU-A2-01a、FU-A2-01b（A2-13：崩溃后留下的程序，下次启动时列出来）。说明在 `reviews/groupI-wiring.md`、`reviews/groupI-args.md`（`portBase` 用过 9880）。手动检查：B8 改写了，新加 C29。
4b. **用户的第三批需求（git，2026-11-17）**：Fable 设计，写进 plan 4.6a、design 4.2、5.2、5.2b、5.8，调研在 `docs/research/git-pull-all.md`。新任务 G4-11、G4-12、G4-13；G4-01、02、06、07 各加了一点。里程碑 4 多了约 1.5 周。
4c. **里程碑 2 的 B 组**（2026-11-17 开跑）：A2-02、A2-03、A2-04、A2-05、A2-13、A2-06、A2-07，加上 F1-GF2。传给 workflow 的说明在 `reviews/m2B-args.md`（`portBase` 9940）。这一组的例外：A2-06 可以改 `App.tsx`（加工作区路由）和 `ProjectsPage.tsx` 的 `openRepo`；A2-07 可以改 `frontend/package.json`（加 xterm）；A2-13 可以改 `internal/pty/pty_unix.go`（FU-A2-01a）。A2-02 和 A2-07 用同一个终端接口（照 spike 的 `ReadReply`），写在 args 里。
4d. **里程碑 2 的 B 组 ✅ 已完成**（2026-11-17）：`1d3153b` A2-02 终端会话和数据通道、`ebb8783` A2-03 防睡眠、`a157d4c` A2-04 AI 工具、`cfbad11` A2-05 任务状态信号、`4be855f` A2-13 留下的程序和强制停止、`9048069` F1-GF2 项目页各种窗口宽度、`eb361cf` A2-06 工作区外框、`7f53899` A2-07 终端组件。全部通过（1 到 3 轮修复）；gate：80 个文件、996 个测试。**bindings 故意没重新生成**：新服务还没在 `main.go` 注册，生成器会删掉 A2-04 的文件。A2-W 注册完再生成。
   - 37 条 follow-up（FU-A2-02a 到 FU-A2-07f，FU-GF2a、FU-GF2b）：大部分给 A2-CL，5 条给 A2-08，3 条给 A2-W，1 条给 C3-07，2 条给新任务 F1-SG。说明在 `reviews/m2B-wiring.md`（很长，A2-08 和 A2-W 都要读）和 `reviews/m2B-args.md`（`portBase` 用过 9940）。
   - 手动检查：新加 E 组（E1–E10），等 A2-W、A2-09 做完再做。
4e. **新任务 F1-SG**（2026-11-17）：A2-04 的审查发现，`go test ./...` 会启动用户自己的登录 zsh，读到 `~/.zshenv`、`~/.zprofile`、`~/.zlogin`（`internal/execx` 的 `EnvPATH` 在 macOS 上跑 `$SHELL -l -c`，只有少数测试关掉了它）。这违反安全红线。F1-SG 加一个测试里的守卫（像 `awsguard`），在 C 组和 A2-08 一起跑。
4h. **里程碑 2 的 C 组 ✅ 已完成**（2026-11-17，最后一组 Go）：`63a150c` A2-08 任务的生命周期，`70e3c41` F1-SG 测试不再启动用户的 shell，`72cd499` 合并时在 `terminal.Open` 补了 shell 守卫。F1-SG 修 2 轮、A2-08 修 3 轮后通过；gate：81 个文件、1005 个测试。bindings 还是没重新生成（A2-W 不做了）。6 条小问题（FU-A2-08a、b，FU-SGa 到 d）留给 Rust 版。说明在 `reviews/m2C-wiring.md`，Rust 移植时和 `m2B-wiring.md` 一起读。
4i. **gpui 小样**（2026-11-17）：能编译、能跑。终端每秒 141 MB、列表和打字都顺、空闲 CPU 0.01%；启动 0.32 秒、打开 5 万行文件 0.43 秒、内存 110 到 243 MB，比定的目标差一点（测的时候电脑很忙）。能离屏截图（`Window::render_to_image`，不用录屏权限）；有 AccessKit 读屏代码；中文显示正常；授权没问题。风险：组件库现在叫 GPUI Kit（0.10），每周都有不兼容的更新，依赖 `gpui-pre`（一个人维护的 Zed gpui 快照）；要 Rust 1.100；Windows 版要在 Windows 上编译（要 `fxc.exe`）；终端的选字、输入、改大小要自己做。中文输入和 VoiceOver 要用户亲手试。
4f. **用户的第四批需求（2026-11-17）**：AWS 登录成功后，自动对绑定在这个门户上的仓库做一次「拉取全部」，设置里有开关，默认开。Fable 设计，写进 plan 4.6a 第 4 部分、4.15，design 5.2b「Automatic run」、5.14、第 6 节；新任务 G4-14（里程碑 4 的 D 组，约 0.5 周）。两件事按推荐定了，已告诉用户：自动拉取不跑仓库自己的脚本（有脚本的仓库，检出分支留给用户点一下）；只在登录后拉，打开 app 不拉。
4g. **改用 Rust（2026-11-17 用户决定）**：程序和界面全部用 Rust，目标是速度最快、界面精美好用，产品需求一条不改。界面库选 gpui + gpui-component（用户选的 A）：代码编辑器和终端是最吃性能的两块，gpui 有现成的、也最快；Slint 要从零做这两块。风险：gpui 正式版的读屏还没做完（里程碑 7 要用，到时官方没做好就自己用 AccessKit 补），gpui 还没到 1.0（锁定版本）。分工：Fable 管产品和界面设计，Opus 排任务、审代码，Sonnet 写代码。已做完的大约 31% 要用 Rust 重做；需求、设计、研究、测试清单都保留。里程碑 1 的手动检查和测试频道，等 Rust 版做到那一步再做。
5. 然后按顺序：里程碑 2（AI 终端，A2-xx；最后是 A2-CL）→ 3（C3-xx）→ **8（T8-xx：票队列、AI 账号、用量）** → 4 → 5（含 E5-11 PHP）→ 6 → 7。

**这些事必须由用户本人来做或在场，不要自己做：**
- 更新签名钥匙（ed25519）的钥匙仪式：用户必须在场（见 `docs/release.md`）。
- 测试频道的私有仓库（名字类似 `rudian/codepass-testchannel`）和它的令牌：用户同意后才能建。
- 发布（R7-08）：用户必须在场。步骤：仓库改名为 `rudian/codepass`；改 `frontend/src/pages/settings/util.ts` 里的 `REPO_URL` 和 origin；合并到 `main`；打 `v1.0.0` 标签。
- 手动检查：见 `manual-checks.md`，要用虚拟机、Windows、真实数据的副本、真实的 AI 账号。
- 过关卡时请用户看：启动弹窗的措辞（`internal/app/startup.go`）、E 组的设计偏差、通知的文字。
- 里程碑 8 里用真实的 Claude / Codex 账号登录、切换账号：只能用户自己做。D1–D4、D9–D12 现在就能做，越早越好。

## 6. 怎么干活（workflow）

**每组任务跑一个 Workflow**，调用方式是 `Workflow({ scriptPath, args })`。流程：
1. Sonnet 在各自的 worktree 里实现。worktree 在 `~/Documents/code/.codepass-wt/<id>`，分支是 `task/<id>`。
2. Opus 做一次完整的 review。
3. 有问题的话，最多 3 轮「Sonnet 修，Opus 只验证修复」。新发现的 minor 或 nit 记进 follow-up，不挡合并。
4. Sonnet 把通过的任务合并进 `codepass`：
   - 平时是 squash-merge，commit 信息 `codepass(<id>): <title>`，后面加两行 trailer；
   - **`keepHistory` 里列的任务用 `--no-ff` 合并**，任务分支上每个 commit 都要带 trailer（脚本会告诉 Sonnet）；
   - 需要时重新生成 bindings；跑 gate；push；
   - push 成功后，删掉 worktree 和 `task/<id>` 分支（代码已经在 `codepass` 里；用户 2026-11-16 定的：不留 task 分支，也不备份）。没通过的任务，worktree 和分支都留着，下次接着做。

args 的格式：

```
{ repo: "<仓库的绝对路径>", wt: "<home>/Documents/code/.codepass-wt",
  scratch: "<本 session 的 scratchpad>", scrubList: "<home>/.codepass-dev/scrub-list.txt",
  portBase: 9700,            // 每组换一个，不要用 9245
  attr: "<本 session 的两行 commit trailer，用 \n 连接>",
  tasks: [{ id: "F1-21", title: "Carry over AWS SSO Hub 0.0.2 (merge main)" }],
  keepHistory: ["F1-21"],    // 只有 F1-21 需要
  groupNote: "<安全红线（第 7 节）和 scrub gate>",
  notes: { "F1-21": "<这个任务要注意的事>" } }
```

`groupNote` 和 `notes` 的写法可以参考以前几组：`reviews/group*-args.md` 里有当时传进去的原文。

**每组结束后，orchestrator 要做这几件事：**
1. 把结果存到 `reviews/<组>-final.json`（只在本机），接线笔记写到 `reviews/<组>-wiring.md`，传给 workflow 的说明写到 `reviews/<组>-args.md`。这两个 `.md` 要 commit：commit 前把里面的 home 路径换成 `~`，并跑 scrub。
2. 自己跑 `bash docs/plan/codepass/orchestration/gate.sh`。全部通过才能继续。
3. 把 follow-up 登记到 `tasks.md` 的「Follow-ups」，编号格式 FU-<任务号><字母>（F1-CL 的是 FU-CLa1、FU-CLb1 这样；**里程碑 2 起加上里程碑前缀，比如 FU-A2-02a**，因为 FU-02a 这种编号已经被里程碑 1 用掉了），分给后面的任务或这个里程碑的清理任务（里程碑 2 是 A2-CL）；并行拆分的组合并后，自己查一遍两边的接缝（比如 Go 改了、mock 没跟上；新的错误码前端没处理）；需要时改相关任务的 Build 和 Owns；新的手动检查用中文写进 `manual-checks.md`。
4. 用中文 ELI5 告诉用户结果。
5. 关掉还在跑的 subagent。

**Gate 命令**（`gate.sh` 里就是这些）：

```
go vet ./... && go test ./...
GOOS=windows GOARCH=amd64 go build ./...
cd frontend && npm run typecheck && npm test && npm run build && npm run check:dist
CODEPASS_SCRUB_LIST=~/.codepass-dev/scrub-list.txt bash scripts/scrub-check.sh --tracked
```

**其他常用命令：**
- 重新生成 bindings：`.tools/bin/wails3 generate bindings -clean=true -ts -i`（`.tools` 只在本地，git 忽略它）。
- 截图：`zsh docs/plan/codepass/shoturl.sh out.png 1440 900 "<url>"`。
- 打开设计稿：`file://<repo>/docs/design/codepass-mockup/index.html?screen=<id>&theme=light|dark&lang=zh-CN|en&bar=0`。
- 前端 mock 预览：`cd frontend && npm run dev -- --port <port> --strictPort`。URL 参数写在各个 mock 文件开头的注释里，比如 `?scenario=done`、`?setup=missing`、`?identity=page`、`?update=available`。
- 只改文档时的 scrub：先 `git add` 新文件（scrub 只看被 git 跟踪的文件），再跑上面最后一行。

## 7. 安全红线（每个 agent 的 prompt 里都要写进去）

- **不要碰用户装的旧版 AWS SSO Hub**，它可能正在运行：不要运行、退出、kill、移动或删除它。
- **不要碰真实数据**：任何程序或测试都不能指向用户真正的数据文件夹，也不能用真实的 home 配置（`~/.gitconfig`、`~/.aws`、shell 配置、真实仓库）。真要启动 app，只能用开发版，并设置 `AWSSSOHUB_SANDBOX=<新的临时目录>`。
- **AI 工具的数据和登录（里程碑 8 起特别重要）**：agent 和测试不读 `~/.claude`、`~/.claude.json`、`~/.codex`、`~/.config/opencode` 的内容，不读钥匙串，不用真实的 AI 账号登录、不跑 `setup-token`。测试用假的 home 目录和假的工具。用真实账号试只能由用户在手动检查里做。
- **scrub 名单不能看**：`~/.codepass-dev/scrub-list.txt` 绝不打开、打印或引用。真实的公司名、人名、账号 ID、仓库文件夹名都不能写进仓库。
- **对外操作要先问用户**：不建 GitHub 仓库、release 或令牌，不上传东西，不生成正式的 ed25519 钥匙。
- **测试和环境**：
  - 签过名的测试包用完就删。
  - spike 分支不 push。
  - 不做需要管理员权限的安装，不改全局配置。F1-12 的安装测试只用假的程序。
  - 开发版和已安装的 app 共用 bundle ID：开发时不发真的系统通知（F1-22 的 fake），除非是用户在做手动检查。
  - 测试不能连 AWS：测试里建 `Container` 一定要传假的 `Endpoints`（`awsapitest` 和 `internal/app` 的 fake）。F1-22 的一个草稿测试忘了传，碰到了一次真的 AWS 登录服务（假数据，被拒绝）；FU-22f 会让测试碰到真地址时直接报错。
- **隐私**：
  - 密码和令牌不能出现在对话或文件里。用剪贴板传，用完清空。
  - 用户的邮箱等个人信息不发给外部服务，也不放进搜索词。
- **commit 规则**：
  - `main` 上的 commit 不用自动关闭 issue 的关键词（比如 `closes #…`）。
  - 每个 commit 的结尾加当前 session 的两行 trailer。

## 8. 坑和经验

- agent 常常用相对路径建 worktree，结果建在了仓库里面。脚本里已经强调要用绝对路径。每组做完后检查 `<repo>/.codepass-wt` 不存在。
- 不要通过 worktree 里 `frontend/node_modules` 的软链接跑 `npm ci`、`npm install` 或 `rm -rf`。它曾经把主目录的 node_modules 清空过。修复办法：在主目录里 `cd frontend && npm ci`。
- `frontend/dist/.gitkeep` 必须一直被 git 跟踪，因为 `//go:embed all:frontend/dist` 需要它。现在有一个 Vite 插件会在每次 build 后把它写回去。
- stage 时按文件名加，不用 `git add -A`。`.tools` 和 `node_modules` 这两个软链接不能进 commit。
- scrub 只用 `--tracked` 模式。`--commits` 模式会把 commit 的 trailer 也当成问题。
- 前端测试偶尔报「window is not defined」：已经由 `frontend/src/test-setup.ts` 修好了。
- **F1-21（已完成）**：Codepass 的版本号还是 0.0.1，发布时由 `release.sh` 改成 1.0.0-rc.N。有 4 个 GitHub 链接还用旧名 `rudian/aws_sso_hub`（`frontend/src/pages/settings/util.ts` 的 `REPO_URL`，`internal/app/menu.go` 的三个链接），等发布时仓库改名（R7-08）再改。A2-14 的「全部停止」要走 `app.Quit` 或 `Container.Shutdown`，这样登录用的独立浏览器才会关掉；新的登录页面一律用 `c.openSignIn` 打开。
- **F1-22（已完成）**：真的系统通知只在正式版（`-tags production`）里发；sandbox、测试和开发版都用 fake，fake 会在窗口里显示提示条。开发版设 `AWSSSOHUB_REAL_NOTIFICATIONS=1` 才发真的，但开发版不开 sandbox 会用真实数据，所以用户的手动检查用测试频道的版本。Wails 的通知服务由 `internal/notify` 包一层自己启动，失败时只用窗口里的提示条。权限只在「点登录时窗口在最前面」的那次登录结束后问一次。「重新登录」按钮发 `app:navigate` 的 `login`，在窗口里打开登录框，和通行证上的按钮走同一条路。里程碑 8 的新通知种类用 `Notify.RegisterKind` 注册，按钮最多一个，数据里只放 ID。细节见 `reviews/groupG-wiring.md`。
- **F1-CL（已完成）**：
  - 按文件拆成两个并行任务，合并时没有冲突。但两边互相看不到对方的改动，所以合并后要自己查接缝。这次就漏了两处：Go 的克隆命令改了，mock 没跟上（FU-CLb6）；Go 加了新错误码 `E_CONFIG_SHARED`，前端的弹出框没处理（FU-CLb7）。
  - 测试里所有 AWS 客户端都经过 `internal/awsguard`：连本机以外的地址会直接报错，不会连出去（FU-22f）。以后新的 AWS 客户端也要用 `awsguard.Client()` 或 `awsguard.Transport()`。
  - 窗口外发来的登录请求一律用 `useActions().requestLogin(portal)`：登录框开着时会排队。
  - A2-16 用 `frontend/src/lib/repoSearch.ts` 的排序。mock 加了 `?search=<文字>`，方便截搜索的图。
  - `update-prefs.json` 是 Codepass 自己在 F1-17 时加的文件，0.0.1 和 0.0.2 都没有；现在这个设置放在 `settings.json` 的 `updateCheckOff`。
- 硬盘：2026-11-16 清过（Go 编译缓存；用户同意后，删了其他 repo 的 Claude 临时文件和 2 天前的旧对话），之后大约 53 GB 空闲。Go 的编译缓存会涨到 6 GB 左右，可以用 `go clean -cache` 清掉。
- 分支：本地只留 `codepass`、`main` 和 `spike/*`。`worktree-wf_*` 和 `task/F1-*` 在 2026-11-16 删掉了（task 分支的备份见第 4 节）。workflow 合并、push 成功后会自动删 task 分支。
- 查别的 repo 时用 `git --no-optional-locks status`：普通的 `git status` 会顺手改写那个 repo 的 `.git/index`。
- Wails 3.1.0 没法拖住系统注销，所以注销或重启时不弹框，直接全部停止并退出。这一点已经写进 plan 和 design。
- **zsh 不会把 `$变量` 拆成多个参数。** `F="a b"; git add $F` 会把整串当成一个文件名，结果没暂存，还留下没提交的改动。文件名要直接写出来。
- **测试不能启动用户自己的 shell。** B 组的审查发现 `go test ./...` 会经过 `internal/execx` 启动用户的登录 zsh（读 `~/.zshenv` 等）。F1-SG 修。以后的任务：凡是要开 shell 的测试，都要把 HOME 和 ZDOTDIR 指到临时文件夹。
- **workflow 在跑的时候，主目录里不要留没提交的改动。** 合并那一步一看到不干净就会停（I 组第一次合并就这样停了）。要改文档，就在同一条命令里改完、马上 commit。停了的话：先 commit，再把脚本里合并那段 prompt 改一个字，用 `resumeFromRunId` 重跑，前面的 agent 都会直接用缓存。
- 开发服务器（端口 9245）已经停了。

## 9. 进度

| 里程碑 | 状态 |
|---|---|
| 0 Spikes | ✅ 完成 |
| 1 基础（项目、安装、身份、更新） | 24/24 个任务全部合并；关卡的设计检查做完，F1-GF 已合并；测试频道和手动检查等用户 |
| 2 AI 终端和防睡眠 | Go 版：A2-01、B 组、C 组已合并，还没接到界面；Go 到此为止，改用 Rust 重做 |
| 3 Worktree 任务、改动和提交 | 未开始 |
| 8 票队列、AI 账号、用量（排在 3 后面） | 未开始 |
| 4 Git 客户端 | 未开始 |
| 5 编辑器和代码提示（含 PHP） | 未开始 |
| 6 GitHub 和 Bitbucket | 未开始 |
| 7 Windows、打磨、发布 | 未开始 |

**整体约 31%。**
- 按任务大小算：S 记 1 分、M 记 2 分、L 记 3 分。原来一共 204 分；第二批加了 F1-21（2）、F1-22（2）、E5-11（2）、A2-16（1）和里程碑 8（29：12 个任务，含选账号、换账号不丢对话、条款规则），2026-11-17 又加了 A2-CL（1）、F1-GF（1）、F1-GF2（1）、F1-SG（1）和第三批 git（G4-11 3 分、G4-12 3 分、G4-13 2 分），再加第四批的 G4-14（2 分），现在一共 254 分，已经完成 83 分（Go 版，里程碑 2 的 C 组之后），约 33%。改用 Rust 后，第 9 节的数字要按新任务表重算。
- 按计划的周数算：原来约 35 周，现在约 47 周（里程碑 8 约 8 周；第三批 git 让里程碑 4 多了约 1.5 周，第四批又多了约 0.5 周），完成约 20%。

## 10. 建议用的技能（Skill）

- `workflow-authoring`：写或改 workflow 脚本之前加载。
- `superpowers:verification-before-completion`：在说「做完了」之前用。
- `superpowers:systematic-debugging`：gate 失败或测试时好时坏的时候用。
- `superpowers:receiving-code-review`：处理 review 和验证的结论时用。
- `impeccable:impeccable` 或 `frontend-design:frontend-design`：UI 设计和打磨时用（交给 Fable）。
- `anthropic-skills:chrome-browser`：在 Chrome 里操作 GitHub 时用（交给 Sonnet）。

## 11. 给下一个 session 的 prompt（用户复制这一段就行）

```
继续 Codepass（~/Documents/code/aws_sso_hub，分支 codepass）。读 docs/HANDOFF.md，照第 1 节做，然后开工。
```
