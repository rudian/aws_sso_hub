# Codepass 手动检查清单（里程碑 0 之后）

日期：2026-11-16。作者：planner（Fable）。

七个 spike 和两份评审里，有些事情机器没法替你试：要你的眼睛、你的密码、一台干净的电脑，或者 Windows。全在这里。

- **A 组：现在就能在这台 Mac 上做**，18 项（A13、A16、A17、A18 要等后面的版本做出来）。
- **B 组：需要干净的虚拟机或 Windows**，19 项（B19 是第二批需求加的）。
- **C 组：里程碑 1 做完之后，在这台 Mac 上做**，25 项（C1–C18 是 2026-11-16 B、B2、C、E 组合并后加的；C19–C25 是同一天第二批需求加的：0.0.2 的修复、登录通知、在项目页打字找仓库）。
- **D 组：里程碑 8（票队列、AI 账号、用量）**，30 项。**D1–D4、D13–D19、D22（Claude）和 D9–D12、D20–D21（Codex）现在就能做**，D23–D27（条款）也现在就能做；它们决定里程碑 8 怎么做。D5–D8 和 D28–D30 等后面的版本。

每一项都写了：为什么做、一步一步怎么做、把什么结果告诉我、它卡住哪个任务。
回报时只说编号和结果就行，比如「A1：通过」「A6：换文件夹后接不上」。**不要把令牌、密码贴给我。**

---

## 先做一次：准备工作

打开「终端」，整段复制粘贴。它只是把 spike 的代码取出来放到 `~/codepass-checks`，不会切换分支。

```sh
REPO=~/Documents/code/aws_sso_hub
mkdir -p ~/codepass-checks/{bin,work,work2,out,npm}
cd "$REPO"
git archive spike/terminal spike/terminal | tar -x -C ~/codepass-checks
git archive spike/signals spikes/signals | tar -x -C ~/codepass-checks
git archive spike/installers spike/installers | tar -x -C ~/codepass-checks
chmod +x ~/codepass-checks/spikes/signals/rec.sh

cd ~/codepass-checks/npm && npm init -y >/dev/null
npm install @xterm/xterm@6.0.0 @xterm/addon-fit @xterm/addon-unicode11 @wailsio/runtime@3.1.0 esbuild
cd ~/codepass-checks/spike/terminal && ./build.sh ~/codepass-checks/npm ~/codepass-checks/bin
ls -l ~/codepass-checks/bin/spiketerm
```

最后一行能看到 `spiketerm` 这个文件，就准备好了。看不到，把报错发我。

### 顺手清理（评审让你做的，不算检查）

```sh
pgrep -fl "opencode serve"            # 有输出、而且不是你自己开的：kill <前面那个数字>
rm -f "$TMPDIR"/lsp-spike-*.stderr    # spike 留下的 5 个日志
ls -d ~/.claude/projects/*scratchpad-cc-work*   # spike 留下的对话记录；确认是它之后可以删
```

`/private/tmp/claude-501/spk/` 先别删，A9 要用。

---

## A 组：现在就能在这台 Mac 上做

### A1 · 终端里的中文输入法（最重要）

- 为什么：没人看过候选窗在不在光标旁边。这是你每天都要用的。
- 卡住：A2-07。
- 步骤：
  1. 运行：
     ```sh
     cd ~/codepass-checks/work
     SPIKE_SCENARIO=interactive SPIKE_DIR=$PWD SPIKE_ARGV='["/bin/zsh","-l"]' ~/codepass-checks/bin/spiketerm
     ```
  2. 出来一个窗口，里面是终端。切到拼音，打 `nihao`。
  3. 看三件事：候选窗是不是贴着光标；按空格或数字能不能上屏「你好」；选字过程中按退格，会不会删掉已经上屏的字。
  4. 在这个终端里输入 `vim`，按 `i`，再试一遍。`Esc` 然后 `:q!` 退出。
  5. 输入 `claude`，在它的输入框里再试一遍。输入 `opencode`，再试一遍。
- 回报：四个地方（zsh、vim、claude、opencode）各自「通过」或哪里不对。候选窗跑偏的话，说它跑到哪里。

### A2 · Claude Code 里 Shift+Enter 换行

- 为什么：页面发的是 `ESC CR`，没在登录后的 claude 里验证过。
- 卡住：A2-07。
- 步骤：还在 A1 那个窗口，运行 `claude`（就用你平时的）。输入 `a`，按 Shift+Enter，再输入 `b`。
- 回报：输入框里是「a 换行 b」两行（通过），还是 `a` 被直接发出去了（不通过）。

### A3 · Codex 在终端里的显示

- 为什么：这台 Mac 没装 Codex，spike 一次都没跑过它。
- 卡住：A2-04、A2-07。
- 步骤：
  1. `brew install --cask codex`，然后 `codex login`。
  2. 像 A1 那样开窗口，在里面运行 `codex`。看界面有没有花、能不能打中文。
  3. 退出后运行（它会自己刷新一次页面再比较，60 秒内自动关）：
     ```sh
     cd ~/codepass-checks/work
     SPIKE_SCENARIO=reload SPIKE_OUT=~/codepass-checks/out SPIKE_REPORT=~/codepass-checks/out/rep-codex.jsonl \
     SPIKE_ARGV='["codex"]' SPIKE_SETTLE=5000 SPIKE_LOGCAP=16777216 SPIKE_DIR=$PWD SPIKE_TIMEOUT=60 \
     ~/codepass-checks/bin/spiketerm
     ```
- 回报：第 2 步「正常 / 哪里不对」；第 3 步告诉我「文件 `~/codepass-checks/out/rep-codex.jsonl` 生成了」，我来读。

### A4 · 长对话刷新后还对不对

- 为什么：日志被截短之后再重放，只在 vim 上验证过，opencode 出过一次乱码。
- 卡住：A2-02。
- 步骤：
  1. 用很小的日志上限开窗口：
     ```sh
     cd ~/codepass-checks/work
     SPIKE_SCENARIO=interactive SPIKE_DIR=$PWD SPIKE_LOGCAP=65536 SPIKE_ARGV='["/bin/zsh","-l"]' ~/codepass-checks/bin/spiketerm
     ```
  2. 在里面运行 `claude`，让它干点真活，持续两三分钟（比如让它读几个文件再总结）。
  3. 记住屏幕的样子，按 `⌘R` 刷新。
  4. 换成 `opencode` 再做一遍。
- 回报：刷新前后屏幕「一样 / 不一样（怎么不一样）」。`⌘R` 没反应也告诉我。

### A5 · Claude Code 的钩子，用你自己的 `claude` 函数

- 为什么：只在 `-p` 模式验证过。没看过交互界面会不会弹「是否信任插件」，也没见过权限提示的事件。
- 卡住：A2-05。
- 步骤：
  1. 准备：
     ```sh
     cd ~/codepass-checks && mkdir -p plug/.claude-plugin plug/hooks
     printf '{"name":"codepass-check","version":"0.0.1","description":"check"}\n' > plug/.claude-plugin/plugin.json
     python3 spikes/signals/mk.py plug/hooks/hooks.json P SessionStart UserPromptSubmit Notification PermissionRequest Stop SessionEnd
     python3 spikes/signals/mk.py B.json B SessionStart Stop
     export CP_LOG=~/codepass-checks/out/a5.jsonl CODEPASS_TASK=check-a5
     cd work && claude --plugin-dir ~/codepass-checks/plug --settings ~/codepass-checks/B.json --permission-mode manual
     ```
  2. 留意启动时有没有问「信任 / 启用这个插件吗」。
  3. 对它说：`运行 shell 命令 echo hi`。应该弹出权限提示，选「允许」。
  4. 输入 `/status`，看有没有提到 ultracode。然后 `/exit`。
  5. 运行（只打印事件名和字段名，不打印内容）：
     ```sh
     python3 - <<'PY'
     import json, os
     for l in open(os.path.expanduser('~/codepass-checks/out/a5.jsonl')):
         r = json.loads(l); p = r['payload']
         print(r['label'], p.get('hook_event_name'), p.get('permission_mode'), sorted(p.keys()))
     PY
     rm ~/codepass-checks/out/a5.jsonl     # 里面有你的提问原文，看完就删
     ```
- 回报：有没有信任提示；第 5 步打印出来的那几行；`/status` 里有没有 ultracode（看不出来就写「看不出来」）。

### A6 · `--session-id`、继续对话、对话文件放在哪里

- 为什么：三件事靠它：应用自己定会话 ID；换文件夹后能不能接上（「重命名本地文件夹」要用）；「释放空间」要删的文件到底叫什么。
- 卡住：A2-08、C3-10、C3-12。
- 步骤：
  ```sh
  ID=$(uuidgen | tr A-Z a-z); echo $ID
  cd ~/codepass-checks/work && command claude --session-id $ID -p "只回答一个字：一"
  ls ~/.claude/projects/ | grep codepass-checks
  ls -la ~/.claude/projects/*codepass-checks-work/ | grep $ID
  ls -d ~/.claude/file-history/$ID 2>/dev/null; echo "file-history: $?"
  command claude --resume $ID -p "我上一句让你回答什么？"
  cd ~/codepass-checks/work2 && command claude --resume $ID -p "我上一句让你回答什么？"
  ```
- 回报：第 3、4、5 行各打印了什么（只有文件名，没有内容）；同一个文件夹里继续，它记不记得「一」；换到 `work2` 之后是记得、报错、还是当成新对话。

### A7 · OpenCode 登录后，在 `--standalone` 下的事件

- 为什么：spike 里 OpenCode 的登录是坏的（401），成功、空闲、权限这几个事件都没见到。
- 卡住：A2-04、A2-05。
- 步骤：
  1. `opencode auth login`，登录好。
  2. 运行：
     ```sh
     cd ~/codepass-checks
     export CP_LOG=~/codepass-checks/out/a7.jsonl CODEPASS_TASK=check-a7
     shasum ~/.config/opencode/service.json ~/.config/opencode/opencode.json > out/oc-before.sha
     cd work && OPENCODE_CONFIG_CONTENT='{"plugins":["'$HOME'/codepass-checks/spikes/signals/opencode-plugin"]}' opencode --standalone
     ```
  3. 在界面里说：`运行 shell 命令 echo hi`。有权限提示就允许。等它答完，退出。
  4. 运行：
     ```sh
     pgrep -fl "opencode serve"; echo "serve: $?"        # 期望：没有进程，显示 serve: 1
     shasum ~/.config/opencode/service.json ~/.config/opencode/opencode.json | diff - ~/codepass-checks/out/oc-before.sha && echo SAME
     python3 -c "import json,os,collections;c=collections.Counter(json.loads(l)['ev'] for l in open(os.path.expanduser('~/codepass-checks/out/a7.jsonl')));print('\n'.join(f'{n} {k}' for k,n in sorted(c.items())))"
     opencode plugin list
     rm ~/codepass-checks/out/a7.jsonl
     ```
- 回报：有没有 `opencode serve` 进程；有没有打印 `SAME`；事件名和次数那张表；`plugin list` 里有没有出现 codepass。

### A8 · Codex 的信号

- 为什么：Codex 的全部结论都只来自文档。
- 卡住：A2-05、C3-12。先做完 A3。
- 步骤：
  ```sh
  cd ~/codepass-checks
  printf '#!/bin/sh\nprintf "%%s\\n" "$1" >> "$CP_LOG"\n' > notify.sh && chmod +x notify.sh
  export CP_LOG=~/codepass-checks/out/a8.jsonl
  cd work && codex -c 'notify=["'$HOME'/codepass-checks/notify.sh"]' exec "只回答：ok"
  python3 -c "import json,os;[print(sorted(json.loads(l).keys())) for l in open(os.path.expanduser('~/codepass-checks/out/a8.jsonl'))]"
  find ~/.codex/sessions -type f -mmin -5 | sed "s|$HOME|~|"
  codex resume --last
  rm ~/codepass-checks/out/a8.jsonl
  ```
  然后在 Codex 界面里输入 `/status`，看它怎么称呼「批准方式」和「沙箱」。
- 回报：字段名那一行；`find` 找到的文件名（看 ID 是不是在文件名末尾）；`resume --last` 能不能接上；`/status` 里那两项的原话；过程中有没有任何「信任」提示。

### A9 · 打开公证过的测试版（先退出已安装的那个）

- 为什么：签了名、开了 hardened runtime 的版本从没真正启动过。每次启动都被已经在运行的 AWS SSO Hub 接走了。
- 卡住：F1-10 验收；决定 F1-01 要不要加 Apple Events 权限。
- 步骤：
  1. 退出 AWS SSO Hub（⌘Q）。`pgrep -fl "AWS SSO Hub"` 应该没有输出。
  2. `open "/private/tmp/claude-501/spk/AWS SSO Hub.app"`
  3. 看：窗口正常显示吗；点登录，浏览器会打开吗；账号列表读得出来吗；点「在终端中打开」，系统有没有弹「允许控制终端」的提示，终端开了没有。
  4. 退出它，再打开你平时用的那个。
- 回报：四件事各自「可以 / 不行」。重点是第三步最后一条：**弹了提示并打开 / 没弹也没打开 / 没弹但打开了**。如果第 2 步说找不到文件（电脑重启过会清掉），告诉我，我重新出一份。

### A10 · 备份签名证书（.p12）

- 为什么：只有账号持有人能重新申请 Developer ID 证书。这把钥匙丢了就发不了版本。
- 卡住：不卡任务，但越早越好。
- 步骤：打开「钥匙串访问」→ 左边「登录」→「我的证书」→ 找到 `Developer ID Application: …` → 点开小三角，确认下面有一把私钥 → 右键证书 →「导出」→ 格式选 `.p12` → 设一个强密码 → 存到两个你自己掌控的地方（不要放进这个仓库）。
- 回报：「已备份，两处」。不要告诉我密码和位置。

### A11 · GitHub 经典令牌，只勾 `repo` 和 `read:org`

- 为什么：spike 只做了读取，而且用的是权限更大的令牌。创建、合并、删分支都没试过。
- 卡住：P6-03 签收。
- 步骤：
  1. 在 GitHub 新建一个私有的临时仓库 `codepass-token-check`（勾上「加 README」）。在网页上新建分支 `t1`，在 `t1` 上随便改一行 README 并提交。
  2. Settings → Developer settings → Personal access tokens → **Tokens (classic)** → Generate new token (classic)。只勾 `repo` 和 `read:org`，有效期 7 天。复制。
  3. 终端里（令牌不会显示，也不进历史记录）：
     ```sh
     read -s "T?token: "; echo
     O=<你的 GitHub 用户名>; R=codepass-token-check; H="Authorization: Bearer $T"
     curl -sI -H "$H" https://api.github.com/user | grep -i "^x-oauth-scopes"
     curl -s -o /dev/null -w "orgs %{http_code}\n" -H "$H" https://api.github.com/user/orgs
     curl -sI -H "$H" https://api.github.com/user/emails | grep -iE "^HTTP|^x-accepted-oauth-scopes"
     curl -s -X POST -H "$H" https://api.github.com/repos/$O/$R/pulls -d '{"title":"t","head":"t1","base":"main"}' | grep -E '"number"|"message"' | head -2
     curl -s -X PUT -H "$H" https://api.github.com/repos/$O/$R/pulls/1/merge | grep -E '"merged"|"message"'
     curl -s -o /dev/null -w "delete %{http_code}\n" -X DELETE -H "$H" https://api.github.com/repos/$O/$R/git/refs/heads/t1
     ```
  4. **先别撤销令牌，接着做 A12。** 做完 A12 再：在网页上撤销令牌、删掉临时仓库、`unset T`。
- 回报：六行各自的输出。期望：`x-oauth-scopes: read:org, repo`；`orgs 200`；`HTTP/2 404` 加一行 `x-accepted-oauth-scopes: user, user:email`；`"number": 1`；`"merged": true`；`delete 204`。

### A12 · 你所在的每个 GitHub 组织，允许经典令牌吗

- 为什么：组织可以禁止经典令牌。那样的组织，拉取请求只能在浏览器里处理。
- 卡住：P6-03、P6-08 的文案。
- 步骤：对每个组织（用 A11 那个令牌）：
  ```sh
  ORG=<组织名>
  curl -sI -H "Authorization: Bearer $T" https://api.github.com/orgs/$ORG/repos | grep -iE "^HTTP|^x-github-sso"
  ```
  如果你是组织的所有者，也可以直接看：组织 Settings → Personal access tokens → Settings → 「Personal access tokens (classic)」那一项是允许还是限制。
- 回报：每个组织一行（不用写组织名，写「组织 1、组织 2」就行）：`200`、`403`、或者 `403 + x-github-sso`。有 SSO 的组织，先记下 403，再去令牌页面点「Configure SSO」授权，重跑一次看是不是变成 200。

### A13 · 最大的仓库上，语言服务吃多少内存（等里程碑 5）

- 为什么：spike 量的是别的仓库。你的仓库可能更大。
- 卡住：里程碑 5 的关卡。现在不用做。
- 步骤（到时候在里程碑 5 的版本里）：打开你最大的 Go 仓库和最大的 TS 仓库，各开两个任务文件夹，翻几个文件；然后「活动监视器」里看 `gopls`、`tsc`、`node` 的内存。
- 回报：每个进程的内存，大概多少 GB。

### A14 · Bitbucket API 令牌

- 为什么：Bitbucket 的所有结论都没用真令牌试过。五个权限的名字是 spike 自己从旧名字对过来的。
- 卡住：P6-04 签收、P6-08 的文案。
- 步骤：
  1. 打开 https://id.atlassian.com/manage-profile/security/api-tokens →「Create API token with scopes」→ 选 Bitbucket → 勾五项：`read:user`、`read:workspace`、`read:repository`、`read:pullrequest`、`write:pullrequest`。**记下页面上这五项的确切名字和到期日的选项。**
  2. 在 Bitbucket 建一个临时仓库，建分支 `t1`、`t2`，各改一行。
  3. 终端里：
     ```sh
     read -s "B?token: "; echo
     H="Authorization: Bearer $B"; API=https://api.bitbucket.org/2.0
     WS=<工作区 slug>; R=<临时仓库名>
     curl -s -o /dev/null -w "user %{http_code}\n" -H "$H" $API/user
     curl -s -H "$H" $API/user/workspaces | python3 -c "import sys,json;print('workspaces', len(json.load(sys.stdin).get('values',[])))"
     ME=$(curl -s -H "$H" $API/user | python3 -c "import sys,json;print(json.load(sys.stdin)['uuid'])")
     curl -s -o /dev/null -w "mine %{http_code}\n" -H "$H" "$API/workspaces/$WS/pullrequests/$ME"
     curl -s -o /dev/null -w "list %{http_code}\n" -H "$H" "$API/repositories/$WS/$R/pullrequests?state=OPEN"
     mk() { curl -s -X POST -H "$H" -H "Content-Type: application/json" $API/repositories/$WS/$R/pullrequests -d '{"title":"'$1'","source":{"branch":{"name":"'$1'"}},"close_source_branch":true}' | python3 -c "import sys,json;d=json.load(sys.stdin);print('create', d.get('id'), d.get('error',{}).get('message',''))"; }
     mk t1; mk t2
     curl -s -o /dev/null -w "merge %{http_code}\n" -X POST -H "$H" $API/repositories/$WS/$R/pullrequests/1/merge
     curl -s -o /dev/null -w "decline %{http_code}\n" -X POST -H "$H" $API/repositories/$WS/$R/pullrequests/2/decline
     curl -s -o /dev/null -w "branch t1 after merge %{http_code}\n" -H "$H" $API/repositories/$WS/$R/refs/branches/t1
     ```
  4. 再建一个**不勾** `write:pullrequest` 的令牌，换上它（重新 `read -s "B?token: "` 和 `H=...`），运行：
     ```sh
     curl -s -i -X POST -H "$H" $API/repositories/$WS/$R/pullrequests/2/decline | grep -iE "^HTTP|^x-accepted-oauth-scopes|message"
     ```
  5. 删掉两个令牌和临时仓库，`unset B`。
- 回报：每一行的输出（都是状态码和数字）；第 1 步页面上五个权限的确切名字；令牌到期日有哪些选项；合并后 `t1` 分支是不是 404（说明自动关掉了源分支）。

### A15 · 管理员权限下，环境变量会不会带过去

- 为什么：评审发现环境变量能改变 root 运行的东西。修法已经定了，这一项只是量一下有多严重。
- 卡住：不卡（F1-12 无论如何都修）。
- 步骤：这条命令会弹一次密码窗口，只运行 `/usr/bin/env`，不安装任何东西。
  ```sh
  CP_MARK=hello osascript -e 'do shell script "/usr/bin/env" with administrator privileges' | tr '\r' '\n' | grep -E "^(CP_MARK|PATH|USER|BASH_ENV)="
  ```
- 回报：打印出来的行里有没有 `CP_MARK=hello`，`USER=` 是谁，`PATH=` 是什么。

### A16 · 私有测试频道的令牌（等 F1-09、F1-10 做完）

- 为什么：测试频道是私有仓库，要令牌才能读。spike 用的是假的服务器。
- 卡住：里程碑 1 的关卡。
- 步骤（到时候）：给测试频道那一个仓库建一个只读的 fine-grained 令牌；在应用里填一次；点「检查更新」并下载。
- 回报：能不能发现新版本、能不能下载完；`security find-generic-password -s Codepass 2>&1 | head -3` 在终端里能不能不弹窗就读到令牌（期望：不能）。

### A17 · 慢网下载更新（等 F1-09、F1-10 做完）

- 为什么：Wails 默认 30 秒超时，100 MB 的包在慢网下会被掐断。修法已定，要实测。
- 卡住：里程碑 1 的关卡。
- 步骤（到时候）：装 Xcode 附加工具里的「Network Link Conditioner」，限到 1 Mbps 左右；在应用里更新。
- 回报：下载完成了没有；进度和剩余时间显示得对不对。


### A18 · AI 自己起任务名（等 A2-04 做完）

- 为什么：新建任务不再填任务名，由 AI 根据你写的描述起名。每个工具的「起名」调用要在你的 Mac 上实测。
- 卡住：A2-04 的验收。
- 步骤：用 Claude Code、Codex、OpenCode 各新建一个任务，描述用中文写一句话。再新建一个任务，描述写 `--dangerously-skip-permissions 你好`。
- 回报：每个工具起名用了几秒；起出来的任务名和分支名；工具自己的历史记录（比如 `claude --resume` 的列表）里有没有多出一个「起名」的会话；最后那个任务是不是把整句当成第一句话（期望：是，而且权限模式没变）。
---

## B 组：需要干净的虚拟机或 Windows

**需要的机器**

| 机器 | 用来做 | 怎么来 |
|---|---|---|
| macOS 15、26、27 虚拟机各一台，**没装 Xcode 和 Command Line Tools** | B1–B7、B13 | 这台 Mac 上用 Tart 或 UTM。先打快照，每次试完恢复。 |
| Windows 11 虚拟机（Arm 版，能跑 amd64 程序） | B8–B12 | 这台 Mac 上用 UTM 或 Parallels。 |
| Windows 10 1809 x64（或 Windows Server 2019，同一个内核版本 17763） | B8、B11 | 一台 Intel 电脑，或者一台云主机。 |
| 一个带认证代理或 PAC 的网络 | B14 | 公司网络，或者自己搭一个代理。 |

把 `~/codepass-checks/spike/installers` 这个文件夹共享（或拷贝）进 macOS 虚拟机。下面命令里的 `installers/` 就是它：先 `cd` 到它的上一级文件夹再运行。

### B1 · 干净 Mac 上装 Homebrew、git、awscli，不弹「安装开发者工具」（S0-04 的验收，还没做过）

- 卡住：**F1-12 不能验收**。三个系统版本各做一遍。
- 步骤（在虚拟机里）：
  1. `xcode-select -p` 应该报错；`ls /Library/Developer/CommandLineTools` 应该不存在。不是这样就换一个更干净的镜像。
  2. 下载并量大小：
     ```sh
     cd ~ && curl -LO https://github.com/Homebrew/brew/releases/latest/download/Homebrew.pkg
     SIZE=$(stat -f %z Homebrew.pkg); echo $SIZE
     ```
  3. `sudo bash installers/homebrew-pkg-install.sh ~/Homebrew.pkg 927JGANW46 $SIZE verify; echo rc=$?` → 期望 `rc=0`。
  4. `sudo bash installers/homebrew-pkg-install.sh ~/Homebrew.pkg 927JGANW46 $SIZE install; echo rc=$?` → 期望 `rc=0`，然后 `/opt/homebrew/bin/brew --version` 有输出。
  5. `HOMEBREW_NO_ANALYTICS=1 HOMEBREW_NO_AUTO_UPDATE=1 /opt/homebrew/bin/brew install git awscli`。盯着看：应该只有「Pouring」，**没有编译，没有任何弹窗**。
  6. `/opt/homebrew/bin/git --version`、`/opt/homebrew/bin/aws --version`。
  7. `/opt/homebrew/bin/brew update`：也不应该弹窗。
  8. 最后才做：`/usr/bin/git --version`。这一步**应该**弹「安装开发者工具」。点取消。
  9. 真正的密码窗口（此时 `/opt/homebrew` 已存在，所以期望失败码 20，正好验证错误码能不能传回来）：
     ```sh
     osascript -e 'on run argv' -e 'do shell script "/bin/bash -c " & quoted form of (item 1 of argv) & " codepass-pkg " & quoted form of (item 2 of argv) & " " & quoted form of (item 3 of argv) & " " & quoted form of (item 4 of argv) & " " & quoted form of (item 5 of argv) with prompt "Codepass check" with administrator privileges' -e 'end run' -- "$(cat installers/homebrew-pkg-install.sh)" ~/Homebrew.pkg 927JGANW46 $SIZE install; echo "osascript rc=$?"
     ```
     把最后的 `install` 换成 `verify` 再跑一次。
  10. 等 F1-15 做完：恢复干净快照，直接打开 Codepass 测试版，走一遍首次安装界面（验证签名 → 密码窗口 → 安装 Homebrew → git、awscli）。这一步也顺便验证了：特权脚本在系统自带的 bash 3.2 下能跑，`/var/root` 下复制 150 MB 左右的文件没问题。
- 回报（每个系统版本）：第 3、4 步的 rc；第 5、7 步有没有弹窗、有没有编译；第 8 步弹了没有；第 9 步两次的完整输出（期望第一次结尾是 `(20)`，第二次成功）；第 10 步每一步的结果和最后一行字。

### B2 · 非管理员账号

- 卡住：F1-12、F1-15 的文案。
- 步骤：恢复快照。新建一个「标准」账号并登录。重复 B1 第 2 步，然后跑 B1 第 9 步那条 `osascript`（最后一个参数用 `install`）。
  - 第一次：在密码窗口点「取消」。
  - 第二次：输错密码。
  - 第三次：输入**管理员**的用户名和密码。
  然后 `ls -ld /opt/homebrew`，再 `brew install git awscli`（不加 sudo）。另外运行 `id -Gn` 看里面有没有 `admin`。
- 回报：三次各自的完整报错文字（期望有 `(-128)` 和 `(-60005)`）；`/opt/homebrew` 的所有者是不是这个标准账号；后面的 `brew install` 要不要密码。

### B3 · Gatekeeper 关掉、以及断网时的签名检查

- 卡住：F1-12（`spctl` 这种回答要单独处理）。
- 步骤：恢复快照，下载好 pkg（B1 第 2 步）。
  1. `sudo spctl --global-disable`（新系统可能要在「系统设置 → 隐私与安全性」里确认）。然后跑 B1 第 3 步的 `verify`，再单独跑 `spctl -a -vv -t install ~/Homebrew.pkg`。做完 `sudo spctl --global-enable`。
  2. 断开虚拟机的网络，再跑一次 `verify`，以及 `pkgutil --check-signature ~/Homebrew.pkg`。
- 回报：两种情况下 `verify` 的 rc 和报错；`spctl` 那条的原话；断网时命令是立刻失败、卡住多久、还是通过。

### B4 · 干净 Mac 上，下载后双击就能开

- 卡住：F1-10 验收。
- 步骤：把 `/private/tmp/claude-501/spk/Codepass-spike.zip` 放到一个能从虚拟机里用 **Safari** 下载到的地方（这样才有真正的隔离标记）。在虚拟机里下载、解压、双击。再断网做一遍。
- 回报：有没有任何 Gatekeeper 警告；断网时呢。

### B5 · 干净 Mac 上 rc.1 更新到 rc.2（等 F1-09、F1-10 做完）

- 卡住：里程碑 1 的关卡。
- 步骤：用测试频道的真版本。Safari 下载 rc.1 → 拖进「应用程序」→ 打开一次 → 发布 rc.2 → 在应用里更新。然后运行：
  ```sh
  /usr/bin/codesign --verify --deep --strict -R='anchor apple generic and identifier "com.rudian.awsssohub" and certificate 1[field.1.2.840.113635.100.6.2.6] exists and certificate leaf[field.1.2.840.113635.100.6.1.13] exists and certificate leaf[subject.OU] = "XC34P8TNZ6"' /Applications/Codepass.app; echo rc=$?
  /usr/sbin/spctl -a -t exec -vv /Applications/Codepass.app
  ```
  再做两种摆法：标准账号、应用放在 `~/Applications`；应用放在 `/Applications` 但所有者是管理员、用标准账号运行。
- 回报：有没有横幅、进度、自动重开成 rc.2；有没有 Gatekeeper 弹窗；`rc=0` 吗；`spctl` 说的是不是「Notarized Developer ID」；第三种摆法是不是提示「没法替换自己」而且应用没被动。

### B6 · 同一台虚拟机：坏的更新要被拒绝

- 卡住：里程碑 1 的关卡。
- 步骤：发布一个 rc.3，内容其实是 rc.1 的包加 rc.1 的清单（旧包换新标签）；再发布一个 ad-hoc 签名的版本。每次都在应用里点更新。
- 回报：两次是不是都被拒绝、说了什么理由；应用是不是还在跑 rc.2；`/Applications` 里有没有多出 `wails-update-*` 或 `.bak`。

### B7 · 替换到一半时杀掉助手进程

- 卡住：不卡（已接受这个风险），要写进发布文档。
- 步骤：应用点「重新打开」之后，立刻 `pkill -9 -f WAILS_UPDATER_HELPER`（多试几次，早一点、晚一点）。每次看 `/Applications` 里剩下什么。
- 回报：出现过「`Codepass.app` 没了，只剩 `Codepass.app.bak`」吗；把 `.bak` 改回原名之后能不能正常打开。

### B8 · Windows：WebView2 里的终端和 ConPTY 封装（从没运行过）

- 卡住：**A2-01**。Windows 11 和 Windows 10 1809 都要做。
- 步骤：这一项需要先有一个能在 Windows 上跑的 spike 程序，现在只有 `winpty_windows.go` 这一个文件。**先告诉我你准备好了 Windows 虚拟机，我让实现方把 spike 程序接上这个封装、交叉编译一个 `spiketerm.exe` 给你。** 拿到之后：
  1. 运行它，在里面开 `cmd`，再运行 `cmd /c start /b ping -t localhost`。
  2. 在程序里点「结束」→ 任务管理器里 `PING.EXE` 应该没了。
  3. 重新来一遍，这次从任务管理器直接结束 `spiketerm.exe` → `PING.EXE` 也应该没了。
  4. 灌 50 MB 输出、同时把窗口最小化；拖动改变窗口大小；刷新页面。
  5. 在 Windows 10 1809 上，关闭一个会话，看程序会不会卡住。
- 回报：每一步「通过 / 不通过」；有没有屏幕上多出奇怪字符。

### B9 · Windows 11 上 rc.1 更新到 rc.2（等 F1-20 做完）

- 卡住：里程碑 1 的关卡。
- 步骤：下载 rc.1 的 exe，运行；发布 rc.2；在应用里更新。
  然后试一个「版本号对不上」的更新（我会陪你做）：把 rc.2 的 `codepass.exe` 单独压成 `Codepass-1.0.0-rc.3-windows-amd64.zip`；`relkit manifest -version 1.0.0-rc.3 -archive <zip> -os windows -arch amd64 -out manifest-windows-amd64.json`；`relkit sign -key <测试频道的钥匙> -in manifest-windows-amd64.json -out manifest-windows-amd64.json.sig`；三个文件作为预发布 `v1.0.0-rc.3` 传到测试频道仓库。再在应用里更新。
  最后：更新进行到替换那一步时，应用还开着（exe 被占用），看它怎么处理。
- 回报：有没有替换并重开；SmartScreen 说了什么；杀毒软件有没有拦；应用里显示的版本号对不对。rc.3 那次：是不是显示「已拒绝」（期望代码 `bundle-version`，步骤 verify），`codepass.exe` 的哈希有没有变（期望没变），旁边有没有剩下 `wails-update-*` 文件夹（期望没有）。

### B10 · Windows 上的三个 AI 工具

- 卡住：A2-04、A2-05、R7-01。
- 步骤：
  ```
  winget install --id Anthropic.ClaudeCode -e
  winget install --id OpenAI.Codex -e
  winget install --id OpenJS.NodeJS.LTS -e
  npm install -g opencode-ai
  claude --version & codex --version & opencode --version
  ```
- 回报：每条命令成功没有；`opencode-ai` 这个 npm 包名对不对（不对的话，OpenCode 官网写的 Windows 安装命令是什么）；三个工具各自能不能启动到主界面。钩子命令在 Windows 上怎么加引号，等 A2-05 做出来再试。

### B11 · Windows 上的 winget：git 和 AWS CLI

- 卡住：F1-12、F1-15（Windows 那一半）。
- 步骤：
  1. Windows 10 1809：`winget --version`（期望「不是内部或外部命令」）。从 Microsoft Store 装「应用安装程序」，再试。
  2. 管理员账号：`winget install --id Git.Git -e --accept-package-agreements --accept-source-agreements --silent`，记下有没有 UAC。
  3. **标准账号**：`winget install --id Amazon.AWSCLI -e --scope user --silent`。看：有没有 UAC；装到了 `%LocalAppData%\Programs\Amazon\AWSCLIV2` 吗；新开一个窗口 `aws --version` 找得到吗。
  4. 标准账号：不带 `--scope user` 再来一次，记下 `echo %ERRORLEVEL%`。
  5. 断网跑一次，记下错误码和最后一行。
- 回报：每一步的结果。第 3 步最重要：**标准账号能不能不要管理员就装上 AWS CLI**。

### B12 · Windows 上的语言服务

- 卡住：E5-03。
- 步骤：干净的 Windows 11 虚拟机里：
  ```
  winget install --id GoLang.Go -e
  ```
  新开窗口：
  ```
  go install golang.org/x/tools/gopls@latest
  dir %USERPROFILE%\go\bin\gopls.exe
  winget install --id OpenJS.NodeJS.LTS -e
  ```
  再新开窗口：
  ```
  npm install -g typescript
  tsc --version
  dir /s /b %APPDATA%\npm\node_modules\@typescript\*tsc.exe
  ```
  直接运行找到的那个 `tsc.exe --lsp -stdio`，它应该停在那里等输入（Ctrl+C 退出）。
- 回报：`gopls.exe` 在不在；`tsc --version` 是不是 7.x；原生 `tsc.exe` 的完整路径；它能不能启动。退出后任务管理器里有没有残留的 `gopls.exe`、`tsc.exe`、`node.exe`（这一条等里程碑 5 的版本再看一次）。

### B13 · 被公司管理的 Mac、磁盘满

- 卡住：F1-15 那两行报错的真实文字（现在是猜的）。有机会再做。
- 步骤：
  - 被 MDM 管理、限制安装的 Mac：`profiles status -type enrollment`，然后跑 B1 第 4 步。
  - 磁盘很小的虚拟机：把磁盘填到只剩 100 MB 左右，跑 B1 第 4 步。
- 回报：两种情况下脚本的 rc 和完整报错文字。

### B14 · 真实的代理

- 卡住：F1-06、F1-12。
- 步骤：
  - 带用户名密码的代理：`https_proxy=http://用户:密码@主机:端口 HTTPS_PROXY=$https_proxy /opt/homebrew/bin/brew install git`。
  - 只配了 PAC 文件的网络：`scutil --proxy`，然后直接 `brew install git`。
- 回报：第一种通不通；第二种 `scutil --proxy` 打印了什么（把主机名抹掉），`brew` 报什么错。

### B15 · Windows：旧版交接、回收站、取消关机

- 卡住：里程碑 1 的关卡。
- 步骤：在 Windows 11 虚拟机里解压 0.0.1 的 zip，运行里面的 `AWS SSO Hub.exe`；再打开 Codepass：应该提示先退出旧版，然后问要不要把旧版移到回收站。之后开着 Codepass 关机，在「这些应用阻止关机」的画面点取消。
- 回报：旧版 exe 有没有进回收站（不是直接删掉）；取消关机后 Codepass 还在不在、任务还在不在。

### B16 · Windows：存储和文件监视的测试

- 步骤：虚拟机里装好 Go 以后，我会给你一条命令，跑 `internal/store`、`internal/watch` 的 Windows 测试（被占用的文件改名、一万次写入、监视的文件夹被删）。
- 回报：把最后的 `ok` / `FAIL` 行发我。

---


### B17 · Windows：仓库文件夹名和隐藏仓库

- 卡住：里程碑 1 的关卡。
- 步骤：克隆时把本地文件夹名填成 `CON`、`nul.txt`、`abc.`（结尾有点）；把一个仓库设成隐藏，然后在资源管理器里改它里面的文件。
- 回报：三个名字是不是在 git 运行之前就被拒绝；隐藏后那一行是不是离开列表，改文件后 Codepass 是不是不再刷新它。

### B18 · Windows：全局作者身份

- 卡住：里程碑 1 的关卡。
- 步骤：在 Codepass 的 Git 设置里保存全局名字和邮箱；然后在 PowerShell 里运行 `git config --global --list --show-origin`。
- 回报：Codepass 写的文件是不是 git 报告的那个文件；页面上是不是只显示「进程环境」，没有去读 PowerShell 的配置文件。

### B19 · Windows：登录过期的通知（等 F1-22 做完）

- 卡住：里程碑 1 的关卡（Windows 部分）。
- 步骤：照 `docs/research/notifications.md` 第 8 节的 W1 到 W4 做。重点是 W1：先用不装、直接运行的 exe（没有开始菜单快捷方式），再用安装包装一次。
- 回报：不装的 exe 能不能弹通知；按钮在不在；Codepass 关着时点按钮会怎样；标题和内容在哪里被截断；`start ms-settings:notifications` 能不能打开通知设置。

## C 组：里程碑 1 做完之后，在这台 Mac 上做

这些功能的代码已经合并了，但还没接到界面上（接线是 F1-W）。到里程碑 1 关卡时一起做。

### C1 · 旧版 AWS SSO Hub 还开着时打开 Codepass

- 为什么：两个应用用同一个 ID，不处理的话 Codepass 会悄悄交给旧版然后退出。
- 卡住：里程碑 1 的关卡。
- 步骤：**不要用你装着的那个旧版。** 复制一份旧版到 `~/codepass-checks/`，打开这份副本；再打开 Codepass 测试版。分别试「取消」「退出它」「退出并移到废纸篓」。
- 回报：中英文提示对不对；退出后 Codepass 能不能打开；废纸篓里已有同名应用时，新的名字是不是自动编号。

### C2 · 菜单和快捷键

- 步骤：Cmd+R 不应该刷新页面；输入框里复制、粘贴、撤销、全选都能用；没有任务时关窗口 = 退出；Cmd+Q、程序坞的「退出」、托盘的「退出」都走同一个退出流程。
- 回报：哪一项不对。

### C3 · 动效设置

- 步骤：设置里「动效」选「减少」，切换页面、打开侧边面板、打开菜单：应该一下子出现（只有转圈还在慢慢转）。系统「减弱动态效果」打开时选「完整」，动画仍然要有；选「跟随系统」就没有。
- 回报：哪里不对；暗色下侧边面板和设计稿像不像。

### C4 · 真实的系统代理

- 步骤：在系统设置里设一个真实代理，看 Codepass 能不能认出来、请求能不能走代理；只有 PAC 的网络，要提示「请手动填代理」；带用户名密码的代理，`brew` 要能用，日志里不能出现密码。
- 回报：每一步通过 / 不通过（主机名抹掉再发我）。

### C5 · 仓库刷新

- 步骤：在终端里对一个真实仓库 `git commit` 或 `git checkout`，Codepass 里大约 0.3 秒内要变；仓库设了 fsmonitor 或 diff 驱动时，Codepass 不能去运行它们；AI 工具 `git add` 时不能报 `index.lock` 错。
- 回报：有没有延迟、有没有报错。

### C6 · 注销被取消后，再按 Cmd+Q（等 A2-14 做完）

- 为什么：Codepass 在注销时直接全部停止退出（不弹框）。如果注销被取消，之后的 Cmd+Q 还要正常弹「退出 Codepass？」。
- 步骤：开一个 AI 任务；点「注销」再取消；按 Cmd+Q。另外：开着任务直接重启电脑。
- 回报：取消后按 Cmd+Q 有没有弹框；重启时有没有出现「Codepass 阻止了注销」之类的提示（期望：没有）。

### C7 · 从「下载」文件夹直接打开的测试版

- 步骤：从浏览器下载测试版，不移到「应用程序」，直接打开，点「检查更新」。
- 回报：是不是先提示「先把 Codepass 移到应用程序文件夹」，而且没有开始下载。


### C8 · 用旧数据的「副本」升级（固定仓库 ID）

- 为什么：Codepass 第一次打开时，会把旧版的仓库记录搬进新文件 `projects.json`。旧版的 `settings.json` 必须一个字节都不变，旧版才还能用。
- 步骤：**不要用你真正的数据文件夹。** 把旧版的数据文件夹复制一份到 `~/codepass-checks/data-copy/`，并另存一份对照副本。让测试版用这份副本打开（我会给你命令）。然后用 `cmp` 比较 `settings.json` 和对照副本；再让旧版 0.0.1 用同一份副本打开。
- 回报：13 个仓库还在不在、连接测试结果还在不在；旁边新找到的仓库（fork、GitHub 或 Bitbucket 的克隆、普通仓库）是不是在「新找到」里等你选，而不是直接进列表；`cmp` 有没有输出（期望：没有）；旧版还能不能打开、仓库在不在。

### C9 · 在访达里给仓库文件夹改名，再「接回来」（等 F1-14 做完）

- 步骤：给一个仓库设好项目、隐藏、作者身份；在访达里把它的文件夹改名；回到 Codepass 刷新。那一行应该问「这是移动过来的 acme_api 吗？」，点「是」。
- 回报：有没有出现这个问题；点「是」后项目、隐藏、作者身份还在不在；再试一次点「不是」。另外：一个已经设了项目或已隐藏的新文件夹，问题应该直接消失，而不是点「是」后报错。

### C10 · 同一个仓库，大小写不同的路径

- 步骤：「添加已有仓库」时，选同一个仓库，但路径换一种大小写写法（比如 `~/Code/Repo` 和 `~/code/repo`）。
- 回报：列表里是一行还是两行（期望：一行）。

### C11 · 仓库在外接硬盘上，硬盘拔掉

- 步骤：把一个仓库放到外接硬盘（或 U 盘）上并加进 Codepass；推出硬盘；刷新。
- 回报：那一行是标成「找不到」还是直接消失（期望：标成找不到，30 天内都在）；插回去后是不是自动恢复。


### C12 · 「全部隐藏」和「选择要显示的」（等 F1-14 做完）

- 步骤：在你真实的扫描文件夹上，打开「新找到」的提示，分别试「全部隐藏」和「选择要显示的」；然后退出 Codepass 再打开。
- 回报：和设计稿一样吗；重开后选择还在不在。

### C13 · 克隆时改本地文件夹名（等 F1-19 做完）

- 步骤：克隆一个 CodeCommit 仓库，本地文件夹名改成别的（比如远程 `acme_api` 填成 `api-main`）。再试填 `CON`、`nul.txt`、`abc.`。
- 回报：列表里是不是两个名字都显示；三个怪名字是不是在开始克隆之前就被拦住。

### C14 · 部分克隆里的改名

- 步骤：`git clone --filter=blob:none <一个真实仓库> ~/codepass-checks/partial`；进去 `git mv` 一个文件（暂存一个改名）；加进 Codepass。需要 git 2.44 或更新。
- 回报：那一行是不是正常显示、写着「没有检测改名」，而不是报错。

### C15 · 作者身份从哪里来（等 F1-16 做完）

- 步骤：
  1. 临时在你的 `~/.zshrc` 里加一行 `export GIT_AUTHOR_EMAIL=test@example.com`，打开 Codepass 的 Git 设置页。**做完记得删掉这一行。**
  2. 如果你的 `~/.gitconfig` 里有 `includeIf "gitdir:..."` 规则：看那个文件夹里的仓库，来源是不是显示成这条规则。
  3. 如果你用 oh-my-zsh 或 powerlevel10k：页面是不是 5 秒内出来，而不是卡住。
- 回报：警告里是不是点名了 `~/.zshrc`；每一行的邮箱是不是变成了 `test@example.com`；规则来源对不对；有没有卡住。

### C16 · 给项目统一作者身份，再撤销（等 F1-16 做完）

- 步骤：在 `~/codepass-checks/id-test/` 里 `git init` 两三个练习仓库，其中一个再加一个 `git worktree`；放进同一个项目；给项目设名字和邮箱，先看预览，再应用；然后点「撤销」。
  另外：在项目页把一个仓库从一个有作者身份的项目拖到另一个；再删掉一个有作者身份的项目。
- 回报：预览列表和真正写进去的一样吗；worktree 那个是不是写进了共享的配置；撤销后 `.git/config` 是不是和原来一模一样（可以先复制一份用 `cmp` 比）；拖动和删除项目时，确认框有没有列出会受影响的仓库。


### C17 · 项目页的拖拽、键盘和读屏（等 F1-CL 做完）

- 步骤：在项目页用鼠标拖一个仓库到另一个项目，中途按 Esc 取消一次；从访达拖一个文件进窗口（不应该开始拖仓库）。只用键盘：Tab 到列表，方向键移动，M 移动、空格选择、回车确认。打开 VoiceOver 听一遍。设置里「动效」选「减少」再拖一次。
- 回报：拖动时幽灵块跟不跟手、目标有没有高亮、放下后行有没有移动；键盘能不能完成同样的事；VoiceOver 有没有读出「已拿起 / 移到 / 已移动」；「减少」时有没有飞来飞去的动画。

### C18 · 更新提示和设置（等 F1-CL 做完）

- 步骤：设置里找到「更新」卡片，关掉「每天检查一次」，重开 Codepass，看它有没有自己去检查；再点「现在检查」。有新版本时（测试频道，A16 之后）看项目页顶部的横幅和弹窗。
- 回报：关掉后是不是不自动检查、手动检查还能用；横幅和弹窗的中英文措辞读起来顺不顺。

### C19 · 同一个门户，两个用户先后登录（等 F1-21 做完）

你在 `main`（0.0.2）上修好的问题，要在 Codepass 里再确认一次。用测试频道装的 Codepass 做。

- 步骤：找一个门户（同一个 start URL），它下面有两个 Identity Center 用户。先登录第一个，紧接着登录第二个。注意每次弹出来的浏览器窗口。
- 回报：每次是不是一个全新的无痕窗口（看不到上一个人的登录）；登录成功后窗口有没有自己关掉；两张门户卡片上显示的是不是各自的用户和账户。

### C20 · 两个用户各自打开控制台（等 F1-21 做完）

- 步骤：接着 C19，两个用户都登录着。在两个用户各自的一个账户卡片上点「控制台」。
- 回报：按钮有没有显示「正在打开」；两个控制台是不是各自用对的人登录（右上角的名字）；有没有出现 "No access"。

### C21 · 会话时长由管理员决定（等 F1-21 做完）

- 步骤：
  1. 找一个管理员设成比 8 小时短的门户（比如 1 小时）。退出，再登录。「自动续期登录」要开着。等 2 分钟。
  2. 在账户页和设置里找一找，有没有能改会话时长的地方。
  3. 像 C8 那样，用 0.0.2 用过的数据文件夹的**副本**升级一次。
- 回报：刚登录时倒计时是不是不超过 1 小时，2 分钟内是不是变成管理员设的长度；有没有能改时长的按钮或对话框（应该没有）；副本升级后，倒计时还在不在、对不对。

### C22 · 登录快过期、已过期的系统通知（等 F1-22 做完）

用测试频道装的 Codepass 做。详细步骤在 `docs/research/notifications.md` 第 8 节（M1、M2、M6、M7）。

- 步骤：
  1. 登录一个门户。第一次登录成功时，macOS 会问「允许 Codepass 发通知吗」，选允许。
  2. 设置里「提前多久提醒」选 15 分钟。等到快过期（找一个管理员设得短的门户最快）。
  3. 不要点。等它过期。
  4. 再等下一次过期，这次直接点通知上的「重新登录」。
- 回报：提前提醒有没有出现，标题和内容是不是写着那个门户、用户和账户；过期时是不是只剩一条（「已过期」替换了「快过期」）；「重新登录」是直接一个按钮，还是要把鼠标移上去、或藏在「选项」里；点了之后是不是马上开始登录（弹出无痕窗口）；Codepass 在最前面时通知还会不会出现。

### C23 · 关掉通知、Codepass 没开时点通知（等 F1-22 做完）

详细步骤在研究笔记第 8 节的 M3、M4、M5、M8。

- 步骤：
  1. 让一条「已过期」通知留在通知中心，然后退出 Codepass，再点通知上的「重新登录」。
  2. 系统设置 → 通知 → Codepass，关掉「允许通知」。打开 Codepass 的设置页，点「打开系统设置」。
  3. 把语言换成 English，再触发一次通知。
- 回报：退出后点按钮，Codepass 有没有自己打开并开始登录；关掉通知后，设置页有没有出现提示、按钮能不能打开对的设置页；换语言后按钮文字有没有跟着变。

### C24 · 从 0.0.2 升级后第一次问权限（等 F1-22 做完）

研究笔记第 8 节的 M9。

- 步骤：在一台只用过 AWS SSO Hub 0.0.2、没装过 Codepass 的 Mac（或虚拟机）上装 Codepass，打开「系统设置 → 通知」看一眼，再登录一个门户。
- 回报：登录前通知列表里有没有 Codepass（应该没有）；登录后是不是只问了一次权限。

### C25 · 在项目页打字找仓库（等 F1-CL 做完）

- 步骤：在项目页按 Cmd+F，打你常用仓库文件夹名的前几个字母，再试中间的一个词（比如 `agent` 找 `acme_agent_portal`）。用上下键换一个，按回车。再试一个没有的名字。
- 回报：你要的仓库是不是排在第一个；匹配的字有没有加粗；回车有没有打开对的那个；找不到时的提示读起来顺不顺；整个过程是不是比以前快。

---

## D 组：里程碑 8（票队列、AI 账号、用量）

D1 到 D4 **现在就能做**，不需要 Codepass，越早做越好：它们决定里程碑 8 怎么做。要两个 Claude 账号（A 是你平常用的，B 是第二个）。来源：`docs/research/claude-usage-accounts.md`。
**输出里有账号信息，只看、只回报结论，不要贴进仓库或发给我原文。**

### D1 · 命令行能不能直接问用量，花不花额度（3 分钟）

- 卡住：T8-02（用量从哪里读、多久读一次）。
- 步骤：
  1. `mkdir -p ~/.codepass-dev/usage-probe && cd ~/.codepass-dev/usage-probe`
  2. `command claude -p "/usage" --output-format stream-json --verbose > out.jsonl; echo "exit $?"`
  3. 在 `out.jsonl` 里找 `usage_report`。看有哪些 `kind`，`resets_at` 有没有值。
  4. 看最后的 `result` 那一行：`num_turns`、`usage`、`total_cost_usd` 是不是都是 0（都是 0 = 没花额度）。
  5. 打开 Claude Desktop → 设置 → 用量，对一下百分比。
- 回报：有没有 `usage_report`；花没花额度；百分比和 Claude Desktop 一样不一样。

### D2 · 第二个账号用自己的文件夹（5 分钟）

- 卡住：T8-01。
- 步骤：
  1. `CLAUDE_CONFIG_DIR=~/.claude-b command claude auth login`，用账号 B 登录。
  2. 分别运行 `CLAUDE_CONFIG_DIR=~/.claude-b command claude auth status` 和 `command claude auth status`。
  3. 不带变量打开 `command claude`，看 A 是不是还登录着。
- 回报：两个是不是不同的账号；JSON 里哪几个字段写着账号和套餐（只说字段名，不要贴值）；A 有没有被踢掉。

### D3 · A 的对话能不能在 B 上接着聊（10 分钟）

- 卡住：T8-05（切账号时是共享文件夹就够，还是要复制）。
- 步骤：
  1. 在一个练习文件夹里：`command claude --session-id <新的 uuid> --model haiku`，说「记住单词 walnut，然后运行 echo hi」，退出。
  2. 把这个会话复制给 B：`mkdir -p ~/.claude-b/projects/<slug> && cp ~/.claude/projects/<slug>/<uuid>.jsonl ~/.claude-b/projects/<slug>/`（slug 是练习文件夹的路径，把不是字母数字的字符换成 `-`）。
  3. 在同一个练习文件夹：`CLAUDE_CONFIG_DIR=~/.claude-b command claude --resume <uuid>`，问「刚才是哪个单词？」。
  4. 再试一次：会话里用过深度思考（`--effort high`）和工具调用的那种。
  5. 再试一次：不复制，而是把 `~/.claude-b/projects` 做成指向 `~/.claude/projects` 的软链接。
  6. 来回切：在 B 上说「再记住单词 cedar」，退出；回到 A（不带变量）`command claude --resume <uuid>`，问「两个单词是什么？」。
- 回报：B 能不能答出 walnut；有没有关于 signature 或 thinking 的错误；软链接那次行不行；回到 A 后能不能答出 walnut 和 cedar。

### D4 · 你的 shell 设置会不会盖掉 app 选的账号（3 分钟）

- 卡住：T8-01。
- 步骤：
  1. 开一个新终端：`env | grep -c -E '^(CLAUDE_CONFIG_DIR|ANTHROPIC_API_KEY|CLAUDE_CODE_OAUTH_TOKEN|CODEX_HOME|OPENAI_API_KEY)='`
  2. 用你自己的 `claude` 函数（带 `--chrome`、`--channels` 那个）加上 `CLAUDE_CONFIG_DIR=~/.claude-b` 启动一次，看有没有报错。
- 回报：第 1 步的数字（不是 0 就说明有变量会盖掉 app 的选择）；第 2 步有没有报错。

### D5 · 「做完」标记靠不靠谱（等里程碑 8 开工前做，20 分钟）

- 卡住：T8-04 的验收。
- 步骤：准备 5 个小任务（练习仓库就行）。每个任务的最后加一句：「全部做完时，最后一条消息的最后一行只写 CODEPASS-DONE；需要我回答时，最后一行只写 CODEPASS-QUESTION。」一个一个跑。
- 回报：5 次里有几次最后一行写对了；写错的时候它写了什么（大概说就行）。

### D6 · 真实会话里的信号（等 A2-05 做完）

- 步骤：照研究笔记的 R4-M1、R4-M2：让 AI 先问你一个选择题、再运行一个命令；等 70 秒；然后退出，再用 `--resume <id> -- "continue"` 接着跑。
- 回报：Codepass 里任务状态有没有依次显示「在问你」「等你批准」「这一轮结束」「空闲」；接着跑时是不是接上了原来的对话。

### D7 · 真的撞到上限时（碰到时再做）

- 步骤：下次在 Codepass 里撞到 5 小时或每周上限时，记下：任务状态变成什么、重置时间对不对、到时间后 Claude Code 有没有自己接着做。开了「超额额度」的话，也记下它有没有停。
- 回报：上面几件事。

### D8 · 里程碑 8 做完后：一个晚上跑三张票

- 步骤：
  1. 在练习仓库写三张小票，拖进「待执行」，让其中一张会撞到上限（或在设置里把「给自己留」调高来模拟）。睡前开始，第二天看结果。先在「自动换账号」关着（默认）时跑一次，再打开跑一次。
  2. 同时开两个任务：仓库 A 的任务用账号 A，仓库 B 的任务用账号 B。
  3. 把一个撞到每周上限的会话从 A 换到 B，接着做；再手动 B→A→B 来回切两次。
- 回报：三张票是不是按顺序一个一个跑完；「自动换账号」关着时，队列是不是停下来等、通知里有没有「在 B 上继续」；打开后是不是自己换了；两个账号同时跑有没有问题；每次换账号后 AI 还记不记得前面的对话、能不能接着做；结果提示写的是哪一层（完整带过去 / 修好的副本 / 交接笔记）；用量表和 Claude Desktop / ChatGPT 的用量页对不对得上。

### D9 到 D12 · Codex 的同类检查（里程碑 8 开工前做）

研究时这台 Mac 没装 Codex，所以 Codex 的结论全来自文档和源码，要你实测。步骤在研究笔记的 R5 部分（R5-M1 到 R5-M5）。先把 Codex 装到单独的文件夹：`npm i -g --prefix ~/.codepass-dev/codex @openai/codex`，再用它 `codex login` 登录你平常的 ChatGPT 账号。

- **D9 · 不花额度能不能读到用量（R5-M1，5 分钟）**：用 `codex app-server` 问 `account/rateLimits/read`，隔一分钟再问一次。回报：有没有 5 小时和每周两个窗口、百分比和 chatgpt.com 的用量页对不对得上、两次之间百分比变没变（没变 = 不花额度）。
- **D10 · 第二个 ChatGPT 账号（R5-M3，5 分钟）**：`CODEX_HOME=~/.codex-b codex login` 登录第二个账号，再分别看 `codex login status`。回报：是不是两个账号、第一个有没有被踢掉。
- **D11 · A 的对话能不能在 B 上接着跑（R5-M4，10 分钟）**：照笔记的 5 个步骤做，最后再从 B 切回 A 问一次（像 D3 第 6 步）。回报：B 能不能答出 walnut；如果报错，错误里有没有提到 account、scope 或 encrypted；软链接那次行不行；切回 A 后两个单词都记得吗。**这一项决定 Codex 换账号时用哪一层带过去对话（同一个会话、修好的副本、交接笔记）。**
- **D12 · Codex 的 hooks 能不能每次启动时带上（R5-M5，接着 A8）**：除了 A8 原来的问题，再看一轮失败后 `Stop` 会不会触发。回报：有没有弹出信任确认；`Stop` 触发没有。

条款原文的核对见 D23：研究笔记 R9 已经从网页存档读到了 OpenAI 的原文，但还要你在正常打开的网页上再看一眼。

### D13 到 D22 · 换账号时对话能不能完整带过去（研究笔记 R8，现在就能做）

这几项决定计划 4.16「换账号不丢对话」的三层里，哪一层真正在干活：同一个会话、修好的副本，还是交接笔记。都用练习文件夹 `~/.codepass-dev/carry-probe` 和练习会话，**不碰你真实的对话**（D18 例外，只看类型不看内容）。要用到 D2 建好的 `~/.claude-b`。每一项的命令在研究笔记 R8 的「Not verified」里，照着复制就行。

- **D13 · 带着思考和工具调用，直接在 B 上接着聊（R8-M1，10 分钟）**：卡住 T8-05。这一项也回答「隐藏思考带不过去，任务还能不能接着做」（计划 4.16 decision 22「Why the task still continues」）。照笔记做完第 6 步后，再让 B 做下一步：「把较大的那个数写进 note.txt 的第二行」。回报：B 能不能答对两个问题；下一步有没有直接做对（不用你再解释任务）；有没有 `API Error`；开始前有没有弹出什么对话框；显示的是哪个模型。
- **D14 · 对话记录里有什么（R8-M2，3 分钟）**：卡住 T8-11。回报：有没有 `thinking`、`text`、`tool_use`、`tool_result` 这几种；每行有没有 `sessionId`、`uuid`、`parentUuid`（只说有没有，不要贴内容）。
- **D15 · 工具自己的「复制会话」（R8-M3，3 分钟）**：卡住 T8-05（决定第一层要不要多试一次 `--fork-session`）。回报：有没有多出一个新编号的文件；原文件大小变没变。
- **D16 · 修好的副本（R8-M4，5 分钟）**：卡住 T8-11。回报：新文件里 `thinking` 的数量是不是 0；在 B 上用它接着聊，答得对不对、有没有 `API Error`。
- **D17 · 换个模型接着聊（R8-M5，3 分钟）**：回报：有没有出现「Resume this conversation?」，它给了哪些选项。
- **D18 · 真的撞到上限后，记录里留下了什么（R8-M6，碰到时再做）**：这一项会读你一个真实会话，但命令只打印类型，不打印内容。回报：最后三行的类型；接着聊时第一句回复有没有把已经做完的步骤再做一遍。
- **D19 · 计划模式换账号后还在不在（R8-M7，5 分钟）**：回报：在 B 上是不是还在计划模式、还记得那个计划；如果不记得，复制 `plans/` 之后呢。
- **D20 · Codex 的记录和开关（R8-M8，5 分钟，先做 D9–D12）**：回报：账号检查那个功能是不是关着；记录第一行的几个开关（只说 true / false，不要贴编号）；有几处 `encrypted_content`。
- **D21 · Codex 在 B 上接着跑，不行就用修好的副本（R8-M9，10 分钟，先做 D9–D12）**：副本只改会话编号，**「这个对话是哪个账号建的」那几项原样保留**。回报：直接接着跑行不行；副本行不行；如果 Codex 说「This thread was created with a different ChatGPT account or workspace」，也写下来（那就说明 Codex 只能用交接笔记）。
- **D22 · 手写一份交接笔记试试（R8-M10，5 分钟）**：回报：B 读了笔记后，第一句是不是先检查文件夹、然后接着做，而不是问你任务是什么。

### D23 到 D27 · 条款和防封号（研究笔记 R9，现在就能做）

- **D23 · 在正常网页上核对条款原文（R9-M1，10 分钟）**：卡住里程碑 8 开工，以后每次发布前也要做一次。照笔记的 6 个网页和要找的句子，用 Cmd-F 找。回报：每句话还在不在；每个网页上写的生效日期；OpenAI 打开的是哪个地区的条款页面。
- **D24 · 你的账号里有没有公司的 Team 席位（R9-M2，2 分钟）**：只在屏幕上看，不要把输出贴到任何地方。回报：每个账号是个人的还是公司的。是公司的，就要先问公司能不能在它和个人账号之间搬对话。
- **D25 · 这台 Mac 有没有公司管理的设置（R9-M3，1 分钟）**：在 Claude Code 里输入 `/status`。回报：有没有「managed」或「organization」之类的设置。
- **D26 · 写信问两家公司（R9-M4，可选，你自己决定做不做）**：用笔记里那一句英文问题，从相关的账号去问。Codepass 永远不会替你问。回报：对方的书面回答。只有它能把灰色地带变成「可以」或「不行」。
- **D27 · 哪些设置决定工具能自己做多少事（R9-M5，2 分钟）**：Claude Code 的 `/config` 里「Continue automatically at usage limit」开着还是关着；`/usage` 里超额额度开着没有；Codex 的 `/status` 里额度那一行。回报：这三项的状态。

### D28 到 D30 · 出错时 app 看到的是什么（等 A2-05 做完，用 Codepass 开发版和一个练习账号）

- **D28 · 账号被登出、付款失败时，工具报哪一种错**：只记错误的种类名称，不记内容。回报：两种情况各报什么种类（期望 `authentication_failed`、`billing_error`）。这决定 Codepass 什么时候把账号标成「需要你处理」。
- **D29 · 「对话被拒」和「对话太长」时报哪一种错**：第一种用 D16 的方法做一个故意删掉中间一个 thinking 块的副本去接着聊；第二种用一个很长的练习对话换到窗口小的模型。回报：两种情况各报什么种类。**它们绝不能被当成账号问题**，这一项就是验证这一点。
- **D30 · 工具自己换了模型时，app 看不看得到**：回报：钩子（hook）的信息或对话记录里，能不能看出模型被换了。

---

## 做完之后

全部做完可以删掉 `~/codepass-checks`。A11、A14 的临时仓库和令牌记得都删了。
