import type { TutorialSection } from './types'

// 中文教程，内容与 en.ts 一一对应。示例：两家公司 acme（账号 111122223333）和 globex（账号 444455556666），
// 各有自己的 AWS 组织和 IAM Identity Center 门户；initech 代表一个已经在用 Identity Center 的组织。
//
// 控制台菜单名称以 AWS 的英文原文为准，AWS 会不定期调整，中文界面里的叫法也可能不同。
// 所以每一步都同时用文字描述了位置，面包屑只是提示，不是精确路径。
export const zh: TutorialSection[] = [
  {
    id: 'why',
    group: 'admin',
    minutes: 3,
    title: 'SSO 是什么，为什么要换',
    lead: '访问密钥是长期有效的：谁拿到一份拷贝，在有人吊销之前都能用。SSO 通过浏览器登录，发给你的是短期凭证，到期自动失效。',
    steps: [
      {
        title: '什么是访问密钥',
        text: '存放在 `~/.aws/credentials` 里的一对值。它在任何电脑、任何时间都能用，直到被停用。备份里、聊天记录里、仓库里的一份拷贝，和原件一样有效。',
      },
      {
        title: 'SSO 改变了什么',
        text: '你在浏览器里登录，每个会话一次（AWS 默认 **8 小时**，管理员可以设为 15 分钟到 90 天）。AWS CLI 和 Git 用这次登录换取短期凭证，电脑上不再保存任何长期密钥。',
      },
      {
        title: '什么保持不变',
        text: '配置文件名、`aws --profile acme ...` 命令，以及绑定到某个配置文件的仓库都照常可用。AWS SSO Hub 迁移配置文件时**保留原来的名字**。',
      },
      {
        title: '应用会怎么动你的文件',
        text: '修改 `~/.aws/config`、`~/.aws/credentials` 或仓库的 `.git/config` 之前，AWS SSO Hub 会先做备份（设置里的“备份”）。备份里没有密钥内容。每一处改动都会先以对比的形式给你看。',
      },
    ],
    callouts: [
      {
        kind: 'info',
        title: '你确认每一步之前，什么都不会改',
        body: '打开应用只会读取文件。每次改动都有预览、备份和确认按钮。',
      },
    ],
  },
  {
    id: 'enable',
    group: 'admin',
    minutes: 10,
    title: '开启 IAM Identity Center',
    lead: 'IAM Identity Center 是 AWS 登录页背后的服务，每个 AWS 组织开通一次，由能登录该组织管理账号的人来做。本教程里每家公司（acme、globex）都有自己的组织和自己的门户，每家公司各做一遍下面的步骤。',
    steps: [
      {
        title: '登录公司的 AWS 账号',
        text: '用将要负责这套设置的账号登录，比如 acme（账号 `111122223333`）。需要管理员权限的用户，或者根用户。',
      },
      {
        title: '确认账号现在的状态',
        crumbs: ['AWS Organizations'],
        text: '在控制台搜索框里打开 **AWS Organizations**。如果页面提示创建组织，说明这个账号是独立账号；如果它显示这个账号是管理账号，说明组织已经存在；如果显示它是另一个组织的成员，就不能在这里开通 Identity Center，请联系那个组织的管理员给你分配权限（见常见问题）。',
        code: { label: '也可以在终端里查看（只读）', text: 'aws organizations describe-organization --profile acme' },
      },
      {
        title: '创建组织（仅限独立账号）',
        text: '选择 **Create an organization**（创建组织）。独立账号会成为新组织的管理账号，账号本身没有其他变化。在免费套餐账号上创建组织，可能会把账号升级为付费方案。',
      },
      {
        title: '启用 IAM Identity Center',
        crumbs: ['IAM Identity Center', 'Enable'],
        text: '搜索并打开 **IAM Identity Center**。**先选好区域**（右上角的区域选择器）：大家会通过这个区域登录，它不必和你部署资源的区域一致，而且以后更改区域需要删除并重新创建实例。然后选择 **Enable**（启用），如果被询问，创建**组织实例**。',
      },
      {
        title: '保留默认的身份源',
        crumbs: ['Settings', 'Identity source'],
        text: '内置的 **Identity Center 目录**最简单：用户和组都存放在 Identity Center 里。以后也可以改接外部身份提供商。',
      },
      {
        title: '添加你自己和同事',
        crumbs: ['Users', 'Add user'],
        text: '在左侧菜单里打开用户列表，为每个人添加用户：用户名、邮箱和姓名，选择发送邮件邀请。也可以建一个 `Developers` 之类的组，把用户加进去，之后给组分配权限更省事。',
      },
      {
        title: '要求使用 MFA',
        crumbs: ['Settings', 'Authentication', 'Multi-factor authentication'],
        text: '在身份验证设置里配置多重身份验证：选择**每次登录都提示**，允许验证器应用和安全密钥，并让还没有设备的用户在登录时注册一个。',
      },
      {
        title: '决定一次登录能用多久',
        crumbs: ['Settings', 'Authentication', 'Session duration'],
        text: '“用户交互式会话”（user interactive sessions）决定大家多久要重新登录一次。默认 8 小时：越长登录越少，越短越安全。AWS SSO Hub 的倒计时就基于这个值。如果你改了它，也要在应用里设成同样的时长（账号页，门户的登录详情）。',
      },
    ],
    callouts: [
      {
        kind: 'warn',
        title: '请使用组织实例，不要用账号实例',
        body: '账号实例只支持应用。按 AWS 的说法，账号实例不支持权限集，因此不能用来访问 AWS 账号，也不能转换成组织实例。',
      },
      {
        kind: 'info',
        title: '两家公司，两个门户',
        body: 'acme 和 globex 各有自己的组织和门户，所以 AWS SSO Hub 里会有两个门户，各自有登录按钮和倒计时。以后如果两家公司合并，可以把两个账号移到同一个组织里，改用一个门户。',
      },
    ],
  },
  {
    id: 'assign',
    group: 'admin',
    minutes: 10,
    title: '给大家分配账号权限',
    lead: '权限集就是登录后可以选用的角色。先创建权限集，再把它分配给用户或组，最后把门户地址发给开发者。',
    steps: [
      {
        title: '创建权限集',
        crumbs: ['Multi-account permissions', 'Permission sets', 'Create permission set'],
        text: '在左侧菜单里管理 AWS 账号访问权限的那一部分，打开权限集并创建一个。可以选预定义的 **AdministratorAccess**（管理员）或 **PowerUserAccess**（开发者）；只需要 Git 的话，也可以建一个自定义权限集，附加 AWS 托管策略 `AWSCodeCommitPowerUser`。设置**会话时长**（默认 1 小时，最长 12 小时），日常使用 Git 建议 8 到 12 小时。',
      },
      {
        title: '可选：允许应用停用旧密钥',
        text: '迁移配置文件的最后一步是在 IAM 里停用旧访问密钥。**AdministratorAccess** 可以在应用里一键完成；**PowerUserAccess** 和 `AWSCodeCommitPowerUser` 不行，因为它们不管理 IAM。想允许的话，给权限集加上下面的内联策略，并填入旧 IAM 用户的名字。没有它，应用会改为给出控制台操作步骤。',
        code: {
          label: '内联策略（替换账号 ID 和用户名）',
          text: '{\n  "Version": "2012-10-17",\n  "Statement": [\n    {\n      "Effect": "Allow",\n      "Action": [\n        "iam:ListAccessKeys",\n        "iam:GetAccessKeyLastUsed",\n        "iam:UpdateAccessKey"\n      ],\n      "Resource": "arn:aws:iam::111122223333:user/acme-dev"\n    }\n  ]\n}',
        },
      },
      {
        title: '把账号分配给大家',
        crumbs: ['Multi-account permissions', 'AWS accounts', 'Assign users or groups'],
        text: '打开组织里的 AWS 账号列表，勾选账号（acme）。选择 **Assign users or groups**（分配用户或组），挑选用户或组，再选择权限集并提交。等预置完成后再去测试。',
      },
      {
        title: '每个人接受邀请',
        text: '每个人按邮件里的邀请设置密码，并注册 MFA 设备。然后在浏览器里打开一次门户，确认能看到账号和角色。',
      },
      {
        title: '复制门户地址和区域',
        crumbs: ['Dashboard', 'Settings summary'],
        text: '在 Identity Center 的仪表板上，从设置摘要里复制 **AWS access portal URL**（AWS 访问门户地址），并记下开通时选的区域：这两项都要填进 AWS SSO Hub。子域名可以自定义一次，默认的样子如下。',
        code: { label: '门户地址的样子', text: 'https://d-xxxxxxxxxx.awsapps.com/start' },
      },
    ],
    callouts: [
      {
        kind: 'info',
        title: '这个账号属于哪个组织？',
        body: '**独立账号：**在这个账号里启用组织实例（上一节），会创建一个新组织。\n**自己组织的管理账号：**直接在那里启用 Identity Center。\n**自己能控制的组织的成员：**使用那个组织的 Identity Center，在那里分配账号。\n**不受自己控制的组织的成员：**请该组织的管理员分配权限集，或者先让账号退出那个组织。',
      },
      {
        kind: 'warn',
        title: '账号实例不能用来访问 AWS 账号',
        body: '如果 Identity Center 是以账号实例的方式在单个账号里开通的，门户里不会列出任何 AWS 账号。请改为开通组织实例。',
      },
    ],
  },
  {
    id: 'portal',
    group: 'dev',
    minutes: 3,
    title: '添加门户并登录',
    lead: '门户就是一个 Identity Center 登录页。添加一次、登录一次，AWS SSO Hub 就会为你选的每个账号和角色创建配置文件。需要上一节里的门户地址和区域。',
    steps: [
      {
        title: '打开“添加 SSO 门户”',
        crumbs: ['账号', '添加 SSO 门户'],
        text: '打开**账号**页，选择**添加 SSO 门户**。',
      },
      {
        title: '填写门户信息',
        text: '粘贴**门户地址**，选择 **SSO 区域**（Identity Center 所在的区域），再给门户起一个简短的名字，只能用字母、数字、`-`、`_`、`.`，比如 `acme`。',
      },
      {
        title: '登录',
        text: '选择**登录并继续**，浏览器会打开；登录后点击**允许访问**，应用会自动继续。如果浏览器打不开，或者连不上应用，改用设备码。',
      },
      {
        title: '选择账号和角色',
        text: '勾选要创建成配置文件的角色，并给它们起名，比如 `acme-admin`。每个配置文件的颜色跟着 AWS 账号走，同一个账号的配置文件看起来一致。',
      },
      {
        title: '检查并写入',
        text: '对话框会显示 `~/.aws/config` 的确切改动，写入前先做备份。之后每个门户卡片上都会显示这次登录还能用多久的倒计时。',
      },
    ],
    callouts: [
      {
        kind: 'info',
        title: '要迁移访问密钥配置文件？',
        body: '这一节可以跳过：迁移向导会替你添加门户，并保留配置文件名。',
      },
    ],
  },
  {
    id: 'migrate',
    group: 'dev',
    minutes: 5,
    title: '把访问密钥迁移到 SSO',
    lead: '迁移后的配置文件名字不变。带 `--profile acme` 的命令，以及绑定到 `acme` 的所有仓库都照常可用，只是改走 SSO。acme 和 globex 各有自己的组织，所以各自需要一个新门户。每个配置文件各做一遍下面的步骤。',
    steps: [
      {
        title: '开始迁移',
        crumbs: ['账号'],
        text: '在**账号**页，仍在使用访问密钥的配置文件会列在单独的面板里。在 `acme` 卡片上选择**迁移到 SSO**。',
      },
      {
        title: '确认密钥归属',
        text: '应用用这把密钥问 AWS“我是谁”，显示账号（`111122223333`）、IAM 用户和打码后的密钥 ID，不做任何修改。如果这是根用户的密钥，仍可以迁移，但之后只能在控制台里停用它。',
      },
      {
        title: '选择门户并登录',
        text: '如果这个账号不在你已添加的任何门户里，向导会预选**添加新门户**：粘贴前面几节里的门户地址和区域，然后登录。如果某个已有门户里列出了这个账号，则会预选那个门户。',
      },
      {
        title: '选同一个账号和一个角色',
        text: '与密钥属于同一个账号的那一项会被预选并标出来。选一个角色即可，配置文件的区域保持原样。',
      },
      {
        title: '检查、写入并验证',
        text: '看一眼 `~/.aws/config` 的对比：只有 `[profile acme]` 会改动，并新增一个 `[sso-session acme]`。写入后，应用会**用 SSO 角色凭证（不是旧密钥）**确认身份，并检查账号是否一致。',
      },
      {
        title: '从电脑上删除旧密钥',
        text: '新配置验证通过后，把 `~/.aws/credentials` 里的 `[acme]` 段删掉。删除前做的备份里没有密钥内容。你也可以选择**先保留**，这时配置文件会显示“旧密钥仍在磁盘上”。',
      },
      {
        title: '在 IAM 中停用旧密钥',
        text: '密钥已经不在电脑上，但在 AWS 里仍然有效。应用会依次尝试：用你的新 SSO 角色；密钥还在内存中时用旧密钥自己；最后引导你在 IAM 控制台里操作。如果在删除和停用之间重启了应用，就只剩新角色和控制台两种方式。',
      },
    ],
    callouts: [
      {
        kind: 'warn',
        title: '先停用，之后再删除',
        body: '停用密钥可以撤销。等一周都没出问题、确认没有别的东西（CI 任务、服务器）在用它，再到 IAM 里删除。',
      },
      {
        kind: 'info',
        title: '接下来',
        body: '对 `globex`（账号 `444455556666`）重复同样的步骤，它会有自己的门户。',
      },
    ],
  },
  {
    id: 'bind',
    group: 'dev',
    minutes: 5,
    title: '把仓库绑定到账号',
    lead: '绑定告诉某个仓库访问 CodeCommit 时用哪个配置文件。它存放在该仓库自己的 `.git/config` 里，所以两份克隆可以用两个账号，电脑上的其他设置不受影响。',
    steps: [
      {
        title: '打开需要处理的仓库',
        crumbs: ['仓库', '待处理'],
        text: '打开**仓库**页，看**待处理**筛选。每一行会告诉你“现在推送会用哪个账号”，应用也会去查每个仓库属于哪个账号。',
      },
      {
        title: '按建议绑定',
        text: '在某一行点**绑定**，或者用**全部按建议绑定**一次确认多个。要添加扫描文件夹之外的仓库，用**添加现有仓库**：它会识别所属账号，如果是 SSO 账号，还可以帮你直接绑定。',
      },
      {
        title: '处理两个账号里的同名仓库',
        text: '两个账号都有同名仓库时，这一行会同时列出两个候选。**比较提交**会检查哪一个和你本地最新的提交一致。',
      },
      {
        title: '把 SSH 远程地址改成 HTTPS',
        text: '只有 HTTPS 远程地址才能用 SSO。**转为 HTTPS** 会先备份 `.git/config`，然后修改 `origin` 并一并完成绑定。',
      },
      {
        title: '删除全局默认',
        crumbs: ['Git 全局设置'],
        text: '如果全局 Git 设置把某一个账号设成了所有 CodeCommit 仓库的默认，请在全部绑定好之后把它删除：这样没有绑定的仓库会明确报错，而不是悄悄用错账号。按地址的规则要先改成本地绑定。',
        code: {
          label: '未绑定的仓库上，Git 会输出这样的提示',
          text: 'AWS SSO Hub: 这个仓库还没有绑定 AWS 账号。打开 AWS SSO Hub > 仓库，绑定后再试。',
        },
      },
      {
        title: '测试连接',
        text: '在某一行点**测试连接**。它会对远程仓库做一次只读检查，告诉你账号是否已登录、有没有访问权限。',
      },
    ],
    callouts: [
      {
        kind: 'info',
        title: '一条绑定里有什么',
        body: '一段针对 CodeCommit 主机的设置：AWS CLI 的完整路径、配置文件名，以及一个说明“未登录时该怎么办”的兜底提示。同一个仓库里的其他远程地址（比如 GitHub）不受影响。',
      },
    ],
  },
  {
    id: 'daily',
    group: 'dev',
    minutes: 2,
    title: '每天的使用方式',
    lead: '登录一次，然后照常工作。Git 和 AWS CLI 用的是和应用同一个登录。',
    steps: [
      {
        title: '登录',
        text: '打开 AWS SSO Hub，在门户卡片上选择 **SSO 登录**，或者点侧边栏里的门户。一次登录覆盖该门户下的所有配置文件，在浏览器里点击允许即可。',
      },
      {
        title: '照常工作',
        text: '在已绑定的仓库里 `git pull`、`git push`，在终端里 `aws ... --profile acme`，都能直接用。',
      },
      {
        title: '留意倒计时',
        text: '卡片上会显示这次登录还能用多久（默认会话 8 小时）。会话结束之前，应用会自动续期短期访问令牌。会话结束后，再选一次 **SSO 登录**。退出应用不会退出登录。',
      },
      {
        title: '在终端里使用',
        text: '“终端助手”页会为你选的配置文件生成下面这些命令。在终端里登录也可以，应用能看到。',
        code: {
          label: '在任意终端里',
          text: 'export AWS_PROFILE=acme-admin\naws sso login --sso-session acme\naws sts get-caller-identity',
        },
      },
      {
        title: '推送前看一眼颜色',
        text: '每个仓库行和每张卡片都用所属 AWS 账号的颜色。如果标签不是你预期的颜色，先检查绑定。',
      },
    ],
    callouts: [
      {
        kind: 'info',
        title: '退出登录和 Git',
        body: '退出登录会删除登录状态和缓存的角色凭证，但已经签发的凭证在 AWS 一侧会继续有效，直到过期（最长为权限集的会话时长）。',
      },
    ],
  },
  {
    id: 'faq',
    group: 'dev',
    minutes: 5,
    title: '常见问题',
    lead: '安装 AWS CLI、系统提示风险时怎么打开应用，以及常见的报错信息。',
    steps: [
      {
        title: '安装 AWS CLI v2',
        text: '登录不依赖它，但绑定仓库和 Git 推送需要 AWS CLI v2，版本 2.22.0 或更新。**macOS：**用 Homebrew，或者 AWS 文档里的官方安装包。**Windows：**用官方 MSI 安装程序。装好后在“安全与环境检查”页点**重新检查**。',
        code: { label: 'macOS，用 Homebrew', text: 'brew install awscli\naws --version' },
      },
      {
        title: '系统提示风险时怎么打开应用',
        text: '发布的安装包没有签名。**macOS：**右键点击应用，选择**打开**，或者去掉隔离标记。**Windows：**在 SmartScreen 提示里选择**更多信息**，再选**仍要运行**。',
        code: { label: 'macOS，只需一次', text: 'xattr -dr com.apple.quarantine "/Applications/AWS SSO Hub.app"' },
      },
    ],
    faq: [
      {
        q: '把仓库从 acme 的密钥切换到 SSO',
        a: '**方式一：原地迁移（推荐）。**用向导迁移这个配置文件。名字不变，所以绑定到 `acme` 的仓库，以及跟随全局默认 `--profile acme` 的仓库，都不用改动，直接改走 SSO。然后删除旧密钥并停用它。\n**方式二：换一个新名字。**先创建一个新的 SSO 配置文件（比如 `acme-sso`），在删除 `acme` 之前，用旧配置文件卡片上的**把仓库改绑到…**把它的仓库移过去。\n**还没出现在列表里的仓库：**在“仓库”页用**添加现有仓库**。应用会找出所属账号，优先选 SSO 配置文件，并且在恰好只有一个账号匹配时帮你绑定。',
      },
      {
        q: 'Git 要我输入用户名和密码。',
        a: '这个仓库没有绑定，或者钥匙串/凭据管理器里存着旧密码。先绑定仓库，再到“Git 全局设置”页清除已保存的凭证。',
      },
      {
        q: '我推到了错误的账号。',
        a: '看一下仓库的账号标签。全局默认或按地址的规则可能会匹配到错误的克隆。请改为本地绑定，并用**测试连接**确认。',
      },
      {
        q: '浏览器没有打开，或者连不上 127.0.0.1。',
        a: '在登录对话框里改用设备码。远程桌面和限制较多的浏览器也适用这个办法。',
      },
      {
        q: 'AWS CLI 提示令牌已过期，或者令牌不存在。',
        a: '在门户卡片上重新登录。应用和 CLI 共用同一个登录。`Token has expired and refresh failed` 表示门户会话已经结束或被撤销。',
      },
      {
        q: '授权没有通过，或者超时了。',
        a: '浏览器页面在 10 分钟内没有确认，或者选了**拒绝**。重试即可。',
      },
      {
        q: '`aws --profile acme` 还在用旧密钥。',
        a: '环境变量的优先级高于配置文件。打开“安全与环境检查”页：它会列出导出 `AWS_ACCESS_KEY_ID` 的 shell 启动文件和环境变量。应用从不修改这些文件，请自己删掉相关行。',
      },
      {
        q: 'git push 返回 403。',
        a: '仓库绑定了错误的账号，或者这个角色没有 CodeCommit 写权限。检查绑定的颜色，用**测试连接**确认，并请管理员分配包含 CodeCommit 的权限集。',
      },
      {
        q: '门户里看不到任何账号。',
        a: '还没有给你的用户分配权限集，或者预置仍在进行。请联系管理员（见“给大家分配账号权限”）。如果 Identity Center 是以账号实例的方式开通的，它列不出 AWS 账号。',
      },
      {
        q: '退出登录之后 Git 还能用。',
        a: '已经签发的凭证在过期前仍然有效。退出登录时 AWS SSO Hub 会删除缓存的角色凭证，AWS 会在权限集的会话时长内让剩下的凭证过期。',
      },
      {
        q: 'AWS CLI 版本太旧。',
        a: '门户登录需要 2.9.0，AWS CLI 默认使用的浏览器授权流程需要 2.22.0 或更新。请升级（见“安装 AWS CLI v2”）。',
      },
      {
        q: '出现时钟偏差的报错。',
        a: '同步系统时间后重试。',
      },
      {
        q: '备份存放在哪里？',
        a: '设置里的“备份”。每次修改 `~/.aws/config`、`~/.aws/credentials`、全局 Git 配置或仓库的 `.git/config` 之前都会先做一份。凭证的备份里没有密钥内容。',
      },
      {
        q: '我的 AWS 账号是别人组织的成员。',
        a: 'Identity Center 属于组织。请管理账号的管理员在你的账号上给你分配权限集，或者让这个账号退出那个组织并创建自己的组织。',
      },
      {
        q: '可以把设置分享给同事或换到另一台电脑吗？',
        a: '可以。“设置”里的“账号配置”中，**导出账号配置…**会把门户和 SSO 配置文件保存成一个不含任何密钥或令牌的 JSON 文件；**导入账号配置…**会先预览再读取这样的文件。仓库绑定和电脑有关：先扫描，再用**全部按建议绑定**。',
      },
    ],
  },
]
