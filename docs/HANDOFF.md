# Codepass 交接文档

更新：2026-11-16。状态：**已暂停**。用户要求 E 组做完后暂停所有任务，等新 session 继续。

这份文档给下一个 session 用。先读完这份，再照第 1 节做。

---

## 0. 30 秒看懂

- **在做什么**：把 AWS SSO Hub 0.0.1 改版成 **Codepass 1.0.0**。功能包括：项目分组、多个 AWS SSO / GitHub / Bitbucket 账号、完整的 git 客户端、作者身份、OTA 更新、多会话的 AI 编码终端（Claude Code / Codex / OpenCode，每个任务一个 worktree）、带 LSP 的简易 IDE、AI 运行时防止睡眠。平台是 macOS arm64（15 及以上）和 Windows。所有功能做完后一次发布 v1.0.0。
- **进度**：约 **25%**，算法见第 8 节。里程碑 0 已完成。里程碑 1 的 22 个任务做完了 21 个，只剩收尾任务 F1-CL。里程碑 1 的关卡还没过（需要用户手动检查）。里程碑 2 到 7 还没开始。
- **代码**：在 `codepass` 分支，已经 push 到 `origin`。`main` 仍是 0.0.1 加上两个 issue 的修复。
- **下一步**：F1-CL，然后过里程碑 1 关卡，然后做里程碑 2。

## 1. 新 session 的第一步

1. **跟用户说话的方式**：用中文，ELI5 风格。短句，只说做了什么、成功没有、接下来做什么。要用户做决定时，最多给两个选项，并说你推荐哪个。
2. **确认状态**：
   - `git status` 应该是干净的。
   - `git log --oneline -3`：最上面是这份文档的 commit，下面是 `dc4a213`。
   - `git worktree list` 只有主目录。
3. **按顺序读**：
   - 本文件；
   - `docs/plan/codepass/tasks.md` 里的「How the work runs」「Owner decisions (FINAL)」和它后面的 open questions；
   - 同一文件的「Follow-ups」、F1-CL 条目，以及里程碑 1 的「Gate」。
4. **跑一次 gate**：`bash docs/plan/codepass/orchestration/gate.sh`。期望每一项都是 0，前端测试是 56 个文件、749 个测试。
5. **等用户说继续，再开始 F1-CL。** 用户没说之前不要开工。

## 2. 文件在哪里

计划、设计稿和调研笔记**都在仓库里**（2026-11-16 按用户要求加入）。

⚠️ **仓库是公开的**：
- 这些文档也在 scrub 检查的范围内，提交前必须跑 scrub。
- 设计稿和文档里只能用假数据：假名字（acme、globex、initech、alex-dev 等）、`example.com` 邮箱、目录 ID `d-1234567890`、公司名写成 `<owner company>`。

**只留在本机、不进 git 的**（`.git/info/exclude` 里列着，里面有真实账号数据，不要 commit）：
- 旧版 0.0.1 的计划和设计：`docs/plan/implementation-plan.md`、`docs/design/ui-spec.md`、`docs/design/mockup.html`、`docs/design/copy.md`、`docs/design/shots/`；
- 旧版的调研：`docs/research/aws-sso.md`、`docs/research/git-codecommit.md`；
- workflow 的原始记录：`docs/plan/codepass/reviews/*.json`；
- `docs/progress/`。

| 路径 | 内容 |
|---|---|
| `docs/plan/codepass/tasks.md` | **唯一的任务清单**。每个任务有 Build / Owns / Deps / Accept / Tests / Review 几项。还有用户决定、Follow-ups（FU-xx）和每组的合并记录。 |
| `docs/plan/codepass/product-plan.md` | 产品方案，文中引用为 "plan N.N" |
| `docs/plan/codepass/ui-design.md` | 设计规范，文中引用为 "design N.N" |
| `docs/plan/codepass/manual-checks.md` | 用户要亲手做的检查（中文）：A1–A18、B1–B18、C1–C18 |
| `docs/plan/codepass/reviews/` | 每组的接线笔记 `group*-wiring.md`，以及当时传给 workflow 的说明 `group*-args.md`（完整的原始结果 `*.json` 只在本机） |
| `docs/plan/codepass/orchestration/` | `codepass-group-v2.js`（每组任务用的 workflow 脚本）和 `gate.sh` |
| `docs/plan/codepass/shoturl.sh` | 截图工具（用 headless Chrome，支持 `file://`） |
| `docs/design/codepass-mockup/` | 静态设计稿，`shots/` 里是对照截图（1440×900） |
| `docs/research/` | 调研笔记和 spike 笔记 |
| 本地分支 `spike/*` | spike 的代码。**永远不要 push。** |
| 仓库外 `~/.codepass-dev/scrub-list.txt` | scrub 屏蔽名单（权限 600）。**绝不打开、打印或引用它。** |
| 仓库外 `~/Documents/code/.codepass-wt/` | 任务 worktree 的根目录（现在是空的） |
| 上一个 session 的完整记录 | `~/.claude/projects/<本仓库对应的目录>/f8739b81-61d7-45bd-80c2-9e9b28fe5875.jsonl` |

**在 git 里的**：代码；`docs/release.md`（发布流程、钥匙仪式、测试频道）；`scripts/scrub-check.sh`；`build/release/`。

## 3. 已经做完的

- **GitHub issue #1、#2**：已修好、已留言、已关闭（`main` 上的 `3717a6f`）。同一个 commit 加了「用无痕窗口登录」，默认开启。
- **Apple 签名**：
  - Developer ID Application 证书（Team ID `XC34P8TNZ6`）已经在钥匙串里。
  - 公证用 keychain profile `codepass-notary`。
  - 密码不在任何文件里。
- **里程碑 0**：7 个 spike 加两份评审，已完成。
- **里程碑 1**（都在 `codepass` 分支上）：

| 组 | 任务 | commit |
|---|---|---|
| A | F1-01：改名、只做 arm64、新图标 | `f868498` |
| B | F1-02…F1-10：版本化存储、`projects.json`、只读 git 层、文件监视、网络/代理、scrub 检查、动效、更新校验、发布脚本、菜单和生命周期 | `9302a4c`..`64b1b64` |
| B2 | F1-18：固定的仓库 ID | `f4176d7` |
| C | F1-11 仓库引擎；F1-12 工具检查和安装；F1-13 作者身份后端；F1-20 Windows 更新校验 | `82ea29e`..`35d16a0` |
| D | F1-W：接线 | `7ede90e`、`e4a4b93` |
| — | 测试 setup（修一个偶发的 unhandled error）；`RepoName` 和 CloneModal 的入口 | `fd36f8b`、`821cf55` |
| E | F1-14 项目页；F1-15 安装页和 Health 行；F1-16 身份界面；F1-17 更新界面；F1-19 克隆时改文件夹名 | `1447a36`..`dc4a213` |

在 `dc4a213` 上跑的最后一次 gate 全部通过：go vet、go test、Windows 编译、前端 typecheck、前端测试（56 个文件、749 个测试）、build、check:dist、scrub。

**用户中途加的需求**，都已经写进计划，详见 `tasks.md` 的「Owner requirements of 2026-11-16」。

最后一个需求（2026-11-16）是：**新建 AI 任务时不再填任务名，由 AI 根据描述起名。**
- 已经改了 plan 4.7、design 5.6、tasks 里的 A2-04 / 05 / 08 / 09 和 C3-01 / 09，并加了检查 A18。
- 设计稿和截图已经更新：`shots/new-task-*.png`。
- 代码还没写，属于里程碑 2 和 3。

## 4. 接下来做什么

1. **F1-CL（收尾）**：任务说明见 `tasks.md` 的 F1-CL 条目。
   - 修所有标了「F1-CL」的 follow-up，从 FU-02b 到 FU-19c，大约 60 条。
   - 把 E 组没挂上的组件挂上：`UpdatesCard`、`UpdateBanner`、`ToolRows`。
   - 删掉 `pages/repos/` 里没人再用的文件。
   - 让 `RepoName` 覆盖所有显示仓库名的地方。
   - 条目太多，建议拆成两个并行任务：F1-CLa（Go）和 F1-CLb（前端），按文件分开。
2. **里程碑 1 关卡**（`tasks.md` 里里程碑 1 的「Gate」）：
   - 跑标准 gate。
   - 重新截所有截图，和设计稿对比。E 组有几张截图在修复之后没有重截。
   - 和用户一起确认设计上的偏差（清单写在 Gate 里）。
   - 用户手动检查：`manual-checks.md` 的 C 组、A9，以及 B1–B3、B5、B9 等。
3. 然后按 `tasks.md` 的顺序做里程碑 2（AI 终端，A2-xx），再做 3 到 7。

**这些事必须由用户本人来做或在场，不要自己做：**
- 更新签名钥匙（ed25519）的钥匙仪式：用户必须在场（见 `docs/release.md`）。
- 测试频道的私有仓库（名字类似 `rudian/codepass-testchannel`）和它的令牌：用户同意后才能建。
- 发布（R7-08）：用户必须在场。步骤是：仓库改名为 `rudian/codepass`；改 `frontend/src/pages/settings/util.ts` 里的 `REPO_URL` 和 origin；合并到 `main`；打 `v1.0.0` 标签。
- 手动检查：见 `manual-checks.md`，要用虚拟机、Windows、真实数据的副本。
- 过关卡时，请用户看两样东西：启动弹窗的措辞（`internal/app/startup.go`）和 E 组的设计偏差。

## 5. 怎么干活（照做）

**模型分工**（用户定的，为了省 token）：
- Fable：UI 设计。改设计稿和 `ui-design.md`。
- Opus：规划、派任务、审查（review 和验证）。
- Sonnet：写代码、修 bug、合并。在 Chrome 里操作也用 Sonnet。
- 不用的 subagent 要及时关掉。

**每组任务跑一个 Workflow**，脚本是 `docs/plan/codepass/orchestration/codepass-group-v2.js`，调用方式是 `Workflow({ scriptPath, args })`。流程：
1. Sonnet 在各自的 worktree 里实现。worktree 在 `~/Documents/code/.codepass-wt/<id>`，分支是 `task/<id>`。
2. Opus 做一次完整的 review。
3. 如果有问题，最多做 3 轮「Sonnet 修，Opus 只验证修复」。新发现的 minor 或 nit 问题记进 follow-up，不挡合并。
4. Sonnet 把通过的任务 squash-merge 进 `codepass`：
   - commit 信息是 `codepass(<id>): <title>`，后面加两行 trailer；
   - 需要时重新生成 bindings；
   - 跑 gate，然后 push；
   - 删掉 worktree，但保留 `task/*` 分支。

args 的格式：

```
{ repo: "<仓库的绝对路径>", wt: "<home>/Documents/code/.codepass-wt",
  scratch: "<本 session 的 scratchpad>", scrubList: "<home>/.codepass-dev/scrub-list.txt",
  portBase: 9700,            // 每组换一个，不要用 9245
  attr: "<本 session 的两行 commit trailer，用 \n 连接>",
  tasks: [{ id: "F1-CL", title: "Milestone 1 clean-up" }],
  groupNote: "<安全红线（第 6 节）和 scrub gate>",
  notes: { "F1-CL": "<这个任务要注意的事>" } }
```

`groupNote` 和 `notes` 的写法，可以参考以前几组：`reviews/group*-args.md` 里有当时传进去的原文。

**每组结束后，orchestrator 要做这几件事：**
1. 把结果存到 `reviews/<组>-final.json`（只在本机），接线笔记写到 `reviews/<组>-wiring.md`，传给 workflow 的说明写到 `reviews/<组>-args.md`。这两个 `.md` 要 commit，commit 前把里面的 home 路径换成 `~`，并跑 scrub。
2. 自己跑 `bash docs/plan/codepass/orchestration/gate.sh`。全部通过才能继续。
3. 把 follow-up 登记到 `tasks.md` 的「Follow-ups」，编号格式是 FU-<任务号><字母>，分给后面的任务或 F1-CL；需要时改相关任务的 Build 和 Owns；新的手动检查用中文写进 `manual-checks.md`。
4. 用中文 ELI5 告诉用户结果。

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

## 6. 安全红线（每个 agent 的 prompt 里都要写进去）

- **不要碰用户装的旧版 AWS SSO Hub**，它可能正在运行：不要运行、退出、kill、移动或删除它。
- **不要碰真实数据**：任何程序或测试都不能指向用户真正的数据文件夹，也不能用真实的 home 配置（`~/.gitconfig`、`~/.aws`、shell 配置、真实仓库）。真要启动 app，只能用开发版，并设置 `AWSSSOHUB_SANDBOX=<新的临时目录>`。
- **scrub 名单不能看**：`~/.codepass-dev/scrub-list.txt` 绝不打开、打印或引用。真实的公司名、人名、账号 ID、仓库文件夹名都不能写进仓库。
- **对外操作要先问用户**：不建 GitHub 仓库、release 或令牌，不上传东西，不生成正式的 ed25519 钥匙。
- **测试和环境**：
  - 签过名的测试包用完就删。
  - spike 分支不 push。
  - 不做需要管理员权限的安装，不改全局配置。F1-12 的安装测试只用假的程序。
- **隐私**：
  - 密码和令牌不能出现在对话或文件里。用剪贴板传，用完清空。
  - 用户的邮箱等个人信息不发给外部服务。
- **commit 规则**：
  - `main` 上的 commit 不用自动关闭 issue 的关键词（比如 `closes #…`）。
  - 每个 commit 的结尾加当前 session 的两行 trailer。

## 7. 坑和经验

- agent 常常用相对路径建 worktree，结果建在了仓库里面。脚本里已经强调要用绝对路径。每组做完后检查 `<repo>/.codepass-wt` 不存在。
- 不要通过 worktree 里 `frontend/node_modules` 的软链接跑 `npm ci`、`npm install` 或 `rm -rf`。它曾经把主目录的 node_modules 清空过。修复办法：在主目录里 `cd frontend && npm ci`。
- `frontend/dist/.gitkeep` 必须一直被 git 跟踪，因为 `//go:embed all:frontend/dist` 需要它。现在有一个 Vite 插件会在每次 build 后把它写回去。
- stage 时按文件名加，不用 `git add -A`。`.tools` 和 `node_modules` 这两个软链接不能进 commit。
- scrub 只用 `--tracked` 模式。`--commits` 模式会把 commit 的 trailer 也当成问题。
- 前端测试偶尔报「window is not defined」：已经由 `frontend/src/test-setup.ts` 修好了。
- 硬盘：现在大约有 18 GB 空闲。Go 的编译缓存会涨到 6 GB 左右，可以用 `go clean -cache` 清掉。
- 本地多余的分支：
  - `worktree-wf_*`（7 个）是里程碑 0 留下的，可以删。
  - `task/F1-*` 合并后保留，作为记录。
- Wails 3.1.0 没法拖住系统注销，所以注销或重启时不弹框，直接全部停止并退出。这一点已经写进 plan 和 design。
- 开发服务器（端口 9245）已经停了。没有正在跑的 workflow 或 subagent。

## 8. 进度

| 里程碑 | 状态 |
|---|---|
| 0 Spikes | ✅ 完成 |
| 1 基础（项目、安装、身份、更新） | 代码 21/22 个任务；剩 F1-CL；关卡还没过 |
| 2 AI 终端和防睡眠 | 未开始 |
| 3 Worktree 任务、改动和提交 | 未开始 |
| 4 Git 客户端 | 未开始 |
| 5 编辑器和代码智能 | 未开始 |
| 6 GitHub 和 Bitbucket | 未开始 |
| 7 Windows、打磨、发布 | 未开始 |

**整体约 25%。**
- 按任务大小算：S 记 1 分、M 记 2 分、L 记 3 分，一共 204 分，已经完成 53 分，约 26%。
- 按计划的周数算：约 22%。

## 9. 建议用的技能（Skill）

- `workflow-authoring`：写或改 workflow 脚本之前加载。
- `superpowers:verification-before-completion`：在说「做完了」之前用。
- `superpowers:systematic-debugging`：gate 失败或测试时好时坏的时候用。
- `superpowers:receiving-code-review`：处理 review 和验证的结论时用。
- `impeccable:impeccable` 或 `frontend-design:frontend-design`：UI 设计和打磨时用（交给 Fable）。
- `anthropic-skills:chrome-browser`：在 Chrome 里操作 GitHub 时用（交给 Sonnet）。
- `mattpocock-skills:handoff`：下次需要交接时用。
