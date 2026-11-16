/* Sidebar pages: setup, projects, ai-tasks, accounts, add-account, identity, update, settings */

/* ---------------------------------------------------------------- setup */
SCREENS.setup = () => {
  const st = STATE || 'running';
  const win = OS === 'win';
  const mgr = win ? 'winget' : 'Homebrew';
  const ok = (v) => `<span class="ok" style="display:inline-flex;align-items:center;gap:6px">${I('circleCheck')}${L('已安装', 'Installed')} <span class="mono muted">${v}</span></span>`;
  const queued = `<span class="muted">${L('排队中', 'Queued')}</span>`;
  const missing = `<span class="muted">${L('未安装', 'Not installed')}</span>`;
  const rows = [
    { ic: 'package', name: mgr, why: win ? L('Windows 自带的安装工具，用来安装和更新下面两项', 'The installer built into Windows. It installs and updates the two tools below') : L('安装和更新下面两项，以及之后用到的工具', 'Installs and updates the two tools below, and anything needed later'), v: win ? '1.12.350' : '7.0.13' },
    { ic: 'branch', name: win ? 'Git for Windows' : 'Git', why: L('克隆、提交、推送', 'Clone, commit, push'), v: '2.52.0', cmd: win ? 'winget install --id Git.Git -e' : 'brew install git' },
    { ic: 'cloud', name: 'AWS CLI v2', why: L('登录 AWS，访问 CodeCommit', 'Sign in to AWS and reach CodeCommit'), v: '2.40.3', cmd: win ? 'winget install --id Amazon.AWSCLI -e' : 'brew install awscli' },
  ];
  const stateOf = (i) => {
    if (st === 'done') return ok(rows[i].v);
    if (st === 'idle') return i === 0 && win ? ok(rows[0].v) : missing;
    if (st === 'noadmin') return i === 0 ? `<span class="bad" style="display:inline-flex;align-items:center;gap:6px">${I('circleX')}${L('没有装上', 'Not installed')}</span>` : queued;
    if (i === 0) return ok(rows[0].v);
    if (i === 1) return st === 'failed' ? `<span class="bad" style="display:inline-flex;align-items:center;gap:6px">${I('circleX')}${L('没有装上', 'Not installed')}</span>` : `<span style="display:inline-flex;align-items:center;gap:8px;color:var(--info)"><span class="spinner"></span>${L('正在安装', 'Installing')}</span>`;
    return queued;
  };
  const row = (r, i) => `<div class="tool-row${st === 'running' && i === 1 ? ' is-current' : ''}">
      <span class="tool-row__icon">${I(r.ic)}</span>
      <div><div class="tool-row__name">${r.name}</div><div class="tool-row__why">${r.why}</div></div>
      <div class="tool-row__state">${stateOf(i)}</div>
      ${st === 'running' && i === 1 ? `<div class="tool-row__progress"><div class="progress"><div class="progress__bar" style="--pct:71%"></div></div><span><span class="mono">${r.cmd}</span> · ${L('正在下载，71%', 'downloading, 71%')}</span></div>` : ''}
    </div>`;
  const log = st === 'running' ? `<pre class="setup__log"><span class="g">✓</span> ${win ? 'winget 1.12.350' : L('Homebrew.pkg 签名有效，已安装到 /opt/homebrew', 'Homebrew.pkg signature is valid, installed to /opt/homebrew')}
<span class="d">$</span> ${rows[1].cmd}
${win ? '' : '<span class="d">==&gt;</span> '}${win ? 'Found Git [Git.Git] Version 2.52.0' : 'Fetching git'}
${win ? '' : '<span class="d">==&gt;</span> '}${win ? 'Downloading https://github.com/git-for-windows/git/releases/…/Git-2.52.0-64-bit.exe' : 'Downloading https://ghcr.io/v2/homebrew/core/git/blobs/sha256:9f2c…'}
<span class="d">${win ? '  ██████████████████████▒▒▒▒▒▒▒▒  46.1 MB / 64.9 MB' : '################################################              71.4%'}</span></pre>`
    : st === 'failed' ? `<pre class="setup__log"><span class="d">$</span> ${rows[1].cmd}
${win ? '' : '<span class="d">==&gt;</span> '}${win ? 'Downloading Git-2.52.0-64-bit.exe' : 'Downloading https://ghcr.io/v2/homebrew/core/git/blobs/sha256:9f2c…'}
<span class="r">curl: (28) Connection timed out after 30002 milliseconds</span>
<span class="r">Error: git: Failed to download resource "git"</span></pre>` : '';
  const noadmin = st === 'noadmin' ? `<div class="banner banner--danger setup__fail">${I('lock', 'banner__icon')}<div class="banner__body"><div class="banner__title">${L('Homebrew 没有装上：这个账号不是管理员', 'Homebrew was not installed: this account is not an administrator')}</div>${L('安装 Homebrew 需要管理员。请管理员在 macOS 的窗口里输入他的用户名和密码，然后重试。这台 Mac 没有任何改动。', 'Installing Homebrew needs an administrator. Ask one to enter their name and password in the macOS window, then try again. Nothing on this Mac was changed.')}</div></div>` : '';
  const fail = st === 'failed' ? `<div class="banner banner--danger setup__fail">${I('alert', 'banner__icon')}<div class="banner__body"><div class="banner__title">${L('Git 没有装上：下载超时', 'Git was not installed: the download timed out')}</div>${L('已经装好的不受影响。检查网络或代理后再试一次，会从这一步继续。', 'What is already installed is untouched. Check the network or proxy and try again; it continues from this step.')}</div></div>` : '';
  const primary = { idle: L('一键安装', 'Install everything'), running: L('正在安装', 'Installing'), failed: L('重试', 'Try again'), noadmin: L('重试', 'Try again'), done: L('进入 Codepass', 'Open Codepass') }[st];
  const doneN = { idle: win ? 1 : 0, running: 1, failed: 1, noadmin: 0, done: 3 }[st];
  return `<div class="setup">
    <div class="setup__bar">${win ? '' : '<div class="traffic" aria-hidden="true"><i></i><i></i><i></i></div>'}</div>
    <div class="setup__wrap page-enter">
      <div class="setup__intro">
        <img class="setup__mark" src="appicon.svg" alt="">
        <h1 class="setup__title">${win ? L('先把这台电脑准备好', 'Let’s get this PC ready') : L('先把这台 Mac 准备好', 'Let’s get this Mac ready')}</h1>
        <p class="setup__lead">${win
          ? L('Codepass 用 winget 安装和更新它需要的工具。点一次，其余的自动完成。', 'Codepass uses winget to install and update the tools it needs. One click, and the rest happens by itself.')
          : L('Codepass 用 Homebrew 安装和更新它需要的工具。点一次，其余的自动完成。', 'Codepass uses Homebrew to install and update the tools it needs. One click, and the rest happens by itself.')}</p>
        <ul class="setup__notes">
          <li>${I('lock')}<span>${win
            ? L('安装时 Windows 会弹出「用户账户控制」确认，每个工具一次。', 'Windows asks for permission with a User Account Control prompt, once per tool.')
            : L('安装 Homebrew 时 macOS 会要你的密码。密码直接交给系统，Codepass 看不到。', 'macOS asks for your password to install Homebrew. It goes straight to macOS; Codepass never sees it.')}</span></li>
          <li>${I('shield')}<span>${win
            ? L('只用 winget 官方源里的安装包：Git.Git 和 Amazon.AWSCLI。', 'Only packages from the official winget source: Git.Git and Amazon.AWSCLI.')
            : L('只用官方安装包：Homebrew.pkg（先验证签名），再用 brew 安装 git 和 awscli。Codepass 运行 brew 时会关闭它的匿名统计。', 'Only official installers: Homebrew.pkg (signature checked first), then git and awscli through brew. Codepass turns off Homebrew’s analytics for the commands it runs.')}</span></li>
          <li>${I('bot')}<span>${L('Claude Code、Codex 和 OpenCode 不在这里安装，第一次用到时再装。', 'Claude Code, Codex and OpenCode are not installed here. Each one is installed the first time you use it.')}</span></li>
        </ul>
      </div>
      <div class="card setup__card">
        <div class="card__header"><div class="card__title">${L('需要的工具', 'Required tools')}</div><span class="spacer"></span><span class="muted" style="font-size:12px">${L(`3 项中已完成 ${doneN} 项`, `${doneN} of 3 ready`)}</span></div>
        ${rows.map(row).join('')}
        ${fail}${noadmin}${log}
        <div class="setup__foot">
          <span>${st === 'running' ? L('大约还需要 2 分钟。可以随时取消，已装好的会保留。', 'About 2 minutes left. You can cancel at any time; what is installed stays.') : st === 'done' ? L('都准备好了。以后可以在「安全与环境」里更新它们。', 'Everything is ready. You can update these later in Health.') : L('大约需要 3 到 6 分钟，取决于网速。', 'Takes about 3 to 6 minutes, depending on your connection.')}</span>
          <span class="spacer"></span>
          ${st === 'done' ? '' : `<button class="btn btn--ghost">${st === 'running' ? L('取消', 'Cancel') : L('我自己安装', 'I’ll install them myself')}</button>`}
          ${st === 'failed' || st === 'noadmin' ? `<button class="btn">${I('copy')}${L('复制日志', 'Copy log')}</button>` : ''}
          <a class="btn btn--primary btn--lg${st === 'running' ? ' is-loading' : ''}" href="${href(st === 'done' ? 'projects' : 'setup', { state: st === 'idle' || st === 'failed' || st === 'noadmin' ? 'running' : 'done' })}" style="min-width:132px">${primary}</a>
        </div>
      </div>
    </div>
  </div>`;
};

/* ---------------------------------------------------------------- projects */
const repoRow = (r) => `<a class="row" href="${href('workspace-code')}" style="color:inherit">
    <span class="row__cell"><input type="checkbox" class="checkbox" aria-label="${r.name}"></span>
    <span class="row__cell repo-cell">${I('grip', 'grip')}${prov(r.p)}<span class="repo-cell__text"><span class="row__primary truncate">${r.name}</span><span class="row__secondary truncate">~/Documents/code/${r.name}</span></span></span>
    <span class="row__cell">${r.acct ? chip(r.acct) : `<span class="acct-chip acct-chip--none">${L('未绑定', 'Not bound')}</span>`}</span>
    <span class="row__cell" style="gap:10px"><span class="branch">${I('branch')}<span class="truncate">${r.branch}</span></span><span class="sync">${r.sync || `<span>${L('干净', 'Clean')}</span>`}</span></span>
    <span class="row__cell">${r.status}</span>
    <span class="row__cell row__actions">${r.ai || ''}${r.action || `<button class="icon-btn icon-btn--sm" title="${L('更多', 'More')}">${I('more')}</button>`}</span>
  </a>`;
const okStatus = (when) => `<span class="dot dot--ok"></span><span class="hint"><span class="hint__label" style="color:var(--text-2)">${L('连接正常', 'Connection OK')}</span><span class="hint__detail">${when}</span></span>`;
const project = (p) => `<section class="proj${p.collapsed ? ' is-collapsed' : ''}${p.drop ? ' is-drop' : ''}">
    <div class="proj__head">
      <button class="portal__toggle" aria-expanded="${!p.collapsed}" onclick="var s=this.closest('.proj');s.classList.toggle('is-collapsed');this.setAttribute('aria-expanded',!s.classList.contains('is-collapsed'))">${I('chevR', 'portal__chevron')}<span class="portal__icon">${I(p.icon || 'layers')}</span></button>
      <div class="proj__id"><div class="portal__title">${p.name}</div><div class="portal__meta"><span>${p.meta}</span>${p.peek ? `<span class="portal__peek">${p.peek}</span>` : ''}</div></div>
      ${p.ident ? `<span class="proj__ident" title="${L('这个项目的作者身份', 'Author identity of this project')}">${I('user')}<b>${ME}</b> &lt;${p.ident}&gt;</span>` : ''}
      ${p.nomenu ? '' : `<button class="icon-btn icon-btn--sm" title="${L('项目菜单', 'Project menu')}">${I('more')}</button>`}
    </div>
    <div class="proj__body"><div><div class="table proj-table">
      <div class="table__head"><span></span><span>${L('仓库', 'Repository')}</span><span>${L('使用的账号', 'Account used')}</span><span>${L('分支和改动', 'Branch and changes')}</span><span>${L('状态', 'Status')}</span><span></span></div>
      ${(p.repos || []).map(repoRow).join('')}
    </div></div></div>
  </section>`;
function projectsBody(top) {
  const up = (n) => `<b class="up">↑${n}</b>`, dn = (n) => `<b>↓${n}</b>`, chg = (n) => `<span class="chg">${L(`${n} 个改动`, `${n} changed`)}</span>`;
  const drag = STATE === 'drag';
  const P = [
    { name: 'acme', meta: L('4 个仓库 · 2 个 AI 任务运行中', '4 repositories · 2 AI tasks running'), ident: ID_PROJ, repos: [
      { name: 'acme_api', p: 'aws', acct: 'acme-prod', branch: 'main', sync: `${up(2)} · ${chg(5)}`, status: okStatus(L('2 分钟前检查过', 'checked 2 min ago')), ai: `<span class="ai-badge ai-badge--wait" title="${L('1 个任务在等你回答，1 个在运行', '1 task is waiting for you, 1 is running')}">${I('bot')}${L('1 等待你', '1 waiting')}</span>` },
      { name: 'acme_agent_portal', p: 'aws', acct: 'acme-prod', branch: 'feat/agent-kyc', sync: `${up(1)} ${dn(3)}`, status: okStatus(L('2 分钟前检查过', 'checked 2 min ago')) },
      { name: 'acme_cs_portal', p: 'aws', acct: 'acme-prod', branch: 'main', status: okStatus(L('2 分钟前检查过', 'checked 2 min ago')), ai: `<span class="ai-badge" title="${L('1 个任务在运行', '1 task running')}"><span class="dot dot--busy"></span>${L('1 运行中', '1 running')}</span>` },
      { name: 'acme_merchant_portal_v4', p: 'aws', branch: 'release/4.2', sync: chg(1), status: `<span class="dot dot--warn"></span><span class="hint"><span class="hint__label" style="color:var(--text-2)">${L('acme-prod 中有这个仓库', 'Found in acme-prod')}</span><span class="hint__detail">${L('已用 3 个账号查过', 'checked with 3 accounts')}</span></span>`, action: `<button class="btn btn--primary btn--sm">${L('绑定 acme-prod', 'Bind acme-prod')}</button>` },
    ] },
    { name: 'globex', drop: drag, meta: L('2 个仓库', '2 repositories'), repos: [
      { name: 'globex_backend', p: 'aws', acct: 'globex', branch: 'develop', status: okStatus(L('2 分钟前检查过', 'checked 2 min ago')) },
      { name: 'globex_app', p: 'github', acct: 'alex-dev', branch: 'main', sync: dn(4), status: `<span class="dot dot--ok"></span><span class="hint"><span class="hint__label" style="color:var(--text-2)">${L('有 4 个新提交可以拉取', '4 new commits to pull')}</span><span class="hint__detail">${L('1 个拉取请求等你评审', '1 pull request to review')}</span></span>` },
    ] },
    { name: 'umbrella', collapsed: true, meta: L('1 个仓库', '1 repository'), peek: chip('alex_acme') },
  ];
  const U = { name: L('未分组', 'Ungrouped'), icon: 'folder', nomenu: true, meta: L('5 个仓库', '5 repositories'), repos: [
    { name: 'qr_middleware', p: 'aws', acct: 'initech', branch: 'main', status: `<span class="dot dot--warn"></span><span class="hint"><span class="hint__label" style="color:var(--warn)">${L('登录还剩 48 分钟', 'Sign-in ends in 48 min')}</span><span class="hint__detail">${L('过期后推送会失败', 'pushes fail after that')}</span></span>`, action: `<button class="btn btn--sm">${L('重新登录', 'Sign in again')}</button>` },
    { name: 'initech_document', p: 'aws', acct: 'initech', branch: 'main', status: okStatus(L('2 分钟前检查过', 'checked 2 min ago')) },
    { name: 'initech_gateway', p: 'aws', acct: 'initech', branch: 'hotfix/webhook-retry', sync: `${up(1)}`, status: okStatus(L('2 分钟前检查过', 'checked 2 min ago')) },
  ] };
  return `<div class="page__header"><div><h1 class="page__title">${L('项目', 'Projects')}</h1><p class="page__subtitle">${L('3 个项目 · 12 个仓库 · 刚刚扫描 ~/Documents/code', '3 projects · 12 repositories · scanned ~/Documents/code just now')}</p></div>
      <div class="page__actions"><button class="icon-btn icon-btn--bordered" title="${L('重新扫描', 'Rescan')} (${mod}R)">${I('refresh')}</button><button class="btn">${I('download')}${L('克隆仓库', 'Clone repository')}</button><button class="btn btn--primary">${I('plus')}${L('新建项目', 'New project')}</button></div></div>
    <div class="page__body" style="gap:var(--sp-5);position:relative">
      ${top || ''}
      ${STATE === 'found' ? `<div class="suggest" style="border-style:solid">${I('info')}<span>${L('又发现了 <b>23 个仓库</b>（Codepass 现在会列出所有 Git 仓库，不只是 CodeCommit）。它们还没有显示在列表里。', 'Found <b>23 more repositories</b> (Codepass now lists every Git repository, not only CodeCommit). They are not in the list yet.')}</span><span class="spacer"></span><button class="btn btn--sm btn--primary">${L('挑选要显示的', 'Choose which to show')}</button><button class="btn btn--ghost btn--sm">${L('全部隐藏', 'Hide them all')}</button></div>` : ''}
      <div class="toolbar">
        <div class="segmented" role="radiogroup"><button class="segmented__item is-active">${L('全部', 'All')} <span class="nav__count">12</span></button><button class="segmented__item">${L('待处理', 'To fix')} <span class="nav__count nav__count--warn">2</span></button><button class="segmented__item">${L('有改动', 'With changes')} <span class="nav__count">3</span></button></div>
        <button class="btn btn--ghost">${I('filter')}${L('全部来源', 'All providers')}</button>
        <div class="search">${I('search')}<input class="input" placeholder="${L('搜索仓库或项目', 'Search repositories or projects')}"></div>
      </div>
      ${project(P[0])}${project(P[1])}${project(P[2])}
      <div class="suggest">${I('sparkle')}<span>${L('发现 3 个以 <b>initech_</b> 开头的仓库，要放进同一个项目吗？', 'Found 3 repositories that start with <b>initech_</b>. Put them in one project?')}</span><span class="spacer"></span><button class="btn btn--sm">${L('创建项目 initech', 'Create project initech')}</button><button class="btn btn--ghost btn--sm">${L('不用了', 'No thanks')}</button></div>
      ${project(U)}
      ${drag ? `<div class="drag-ghost" style="left:300px;top:468px">${prov('aws')}initech_gateway <span class="badge badge--info">${L('移到 globex', 'Move to globex')}</span></div>` : ''}
    </div>`;
}
SCREENS.projects = () => shell('projects', projectsBody());

/* ---------------------------------------------------------------- update */
SCREENS.update = () => {
  const banner = `<div class="update-banner"><span class="update-banner__mark">${I('rocket')}</span><span><b>${L('Codepass 1.1.0 可以更新', 'Codepass 1.1.0 is available')}</b> · ${L('你现在用的是 1.0.0', 'you are on 1.0.0')}</span><span class="spacer"></span><button class="btn btn--ghost btn--sm">${L('更新内容', 'What’s new')}</button><button class="btn btn--primary btn--sm">${L('更新', 'Update')}</button></div>`;
  const modal = `<div class="modal-backdrop"><div class="modal" role="dialog" aria-modal="true" aria-labelledby="up-t" style="width:min(560px,100%)">
    <div class="modal__header"><h2 class="modal__title" id="up-t">${L('更新 Codepass', 'Update Codepass')}</h2><span class="spacer"></span><button class="icon-btn icon-btn--sm" title="${L('关闭', 'Close')}">${I('x')}</button></div>
    <div class="modal__body">
      <div class="ver"><div class="ver__box"><div class="ver__n muted">1.0.0</div><div class="ver__k">${L('当前版本', 'Installed')}</div></div>${I('arrowR')}<div class="ver__box"><div class="ver__n">1.1.0</div><div class="ver__k">${L('2026 年 11 月 12 日发布', 'Released 12 Nov 2026')}</div></div></div>
      <ul class="notes">
        <li>${L('历史页可以按文件筛选，并显示每一行最后是谁改的', 'History can filter by file and show who last changed each line')}</li>
        <li>${L('AI 任务结束时可以直接创建拉取请求', 'Create a pull request straight from a finished AI task')}</li>
        <li>${L('修复：Bitbucket 账号偶尔需要重新验证密钥', 'Fixed: Bitbucket accounts sometimes asked to verify the key again')}</li>
      </ul>
      <div style="margin-top:var(--sp-5);display:flex;flex-direction:column;gap:8px">
        <div style="display:flex;justify-content:space-between;font-size:13px"><span style="color:var(--text);font-weight:600">${L('正在下载', 'Downloading')}</span><span class="num muted">31.2 / 48.6 MB · ${L('还需约 20 秒', 'about 20 s left')}</span></div>
        <div class="progress" role="progressbar" aria-valuenow="64" aria-valuemin="0" aria-valuemax="100"><div class="progress__bar" style="--pct:64%"></div></div>
        <ul class="usteps"><li class="is-now"><span class="spinner"></span>${L('下载', 'Download')}</li><li>${L('校验签名和版本', 'Verify signature and version')}</li><li>${L('替换应用', 'Replace app')}</li><li>${L('重新打开', 'Reopen')}</li></ul>
      </div>
      <div class="banner banner--warn" style="margin-top:var(--sp-5);padding:12px 14px">${I('bot', 'banner__icon')}<div class="banner__body"><div class="banner__title">${L('2 个 AI 任务还在运行', '2 AI tasks are still running')}</div>${L('重新打开应用会结束它们。分支和改动都会保留，之后可以从原来的会话继续。', 'Reopening the app ends them. Their branches and changes stay, and each task can resume its session afterwards.')}</div></div>
    </div>
    <div class="modal__footer"><button class="link-button" style="margin-right:auto;font-size:13px">${L('改为在浏览器中下载', 'Download in the browser instead')}</button><button class="btn">${L('现在就重新打开', 'Reopen now')}</button><button class="btn btn--primary">${L('等没有任务在运行时再重新打开', 'Reopen when no task is running')}</button></div>
  </div></div>`;
  return shell('projects', projectsBody(banner), STATE === 'banner' ? '' : modal);
};

/* ---------------------------------------------------------------- AI tasks (all repos) */
const TASKS = [
  { id: 1, tool: 'claude', title: L('修复结算对账的四舍五入误差', 'Fix rounding error in settlement reconcile'), branch: 'ai/fix-settlement-rounding', repo: 'acme_api', st: 'wait', time: '14:08', files: 4, where: 'wt' },
  { id: 2, tool: 'codex', title: L('给 payout 批处理补单元测试', 'Add unit tests for payout batching'), branch: 'ai/payout-batch-tests', repo: 'acme_api', st: 'run', time: '06:41', files: 2, where: 'wt' },
  { id: 3, tool: 'opencode', title: L('把商户列表改成虚拟滚动', 'Virtualise the merchant list'), branch: 'ai/merchant-list-virtual', repo: 'acme_cs_portal', st: 'run', time: '21:17', files: 7, where: 'wt' },
  { id: 4, tool: 'claude', title: L('解释 webhook 重试为什么会重复入账', 'Explain why webhook retries double-post'), branch: 'main', repo: 'acme_api', st: 'done', time: '03:12', files: 0, where: 'main' },
  { id: 5, tool: 'claude', title: L('升级 Go 到 1.25 并修复告警', 'Upgrade Go to 1.25 and fix warnings'), branch: 'ai/go-1-25', repo: 'globex_backend', st: 'fail', time: '00:48', files: 1, where: 'wt' },
];
/* local folder name first, the remote name as secondary text when they differ */
const repoName = (local, remote) => `${local}${remote && remote !== local ? ` <span class="remote-name">${L('远程', 'remote')} ${remote}</span>` : ''}`;
const LOCAL = { acme_cs_portal: 'cs-portal' };
/* opts: { leftover: show the "closed but still running" section } */
function aiTasksBody(opts) {
  const o = opts || {};
  const row = (t) => `<a class="row" href="${href('workspace-ai')}" style="color:inherit">
      <span class="row__cell">${tool(t.tool)}</span>
      <span class="row__cell repo-name"><span class="row__primary truncate" style="max-width:100%">${t.title}</span><span class="row__secondary truncate" style="max-width:100%">${t.branch}</span></span>
      <span class="row__cell" style="font-size:12px;color:var(--text-2)">${t.where === 'wt' ? L('独立工作区', 'Own workspace') : L('主工作区', 'Main workspace')}</span>
      <span class="row__cell">${state(t.st)}</span>
      <span class="row__cell num muted" style="font-size:12px">${t.time}</span>
      <span class="row__cell row__actions">${t.st === 'wait' ? `<span class="btn btn--primary btn--sm">${L('去回答', 'Answer')}</span>` : t.files ? `<span class="btn btn--sm">${L(`查看 ${t.files} 个改动`, `Review ${t.files} files`)}</span>` : `<span class="btn btn--ghost btn--sm">${L('打开', 'Open')}</span>`}</span>
    </a>`;
  const group = (repo, p) => `<div class="stack" style="--gap:8px"><div class="group-label">${prov(p)}${o.leftover ? repoName(LOCAL[repo] || repo, repo) : repo} <span>· ${L(`${TASKS.filter((t) => t.repo === repo).length} 个任务`, `${TASKS.filter((t) => t.repo === repo).length} tasks`)}</span></div>
    <div class="table tasks-table"><div class="table__head"><span></span><span>${L('任务', 'Task')}</span><span>${L('在哪里运行', 'Runs in')}</span><span>${L('状态', 'State')}</span><span>${L('已用时', 'Elapsed')}</span><span></span></div>${TASKS.filter((t) => t.repo === repo).map(row).join('')}</div></div>`;
  const lrow = (l) => `<div class="row${l.open ? ' is-open' : ''}">
      <span class="row__cell">${tool(l.tool)}</span>
      <span class="row__cell repo-name"><span class="row__primary truncate" style="max-width:100%">${l.title}</span><span class="row__secondary truncate" style="max-width:100%">${repoName(l.local, l.remote)} · ${l.where}</span></span>
      <span class="row__cell"><span class="state state--left"><span class="dot dot--warn"></span>${L(`${l.n} 个进程`, `${l.n} processes`)}</span></span>
      <span class="row__cell num" style="font-size:12px;color:var(--text-2)">CPU ${l.cpu}% · ${l.mem}</span>
      <span class="row__cell" style="font-size:12px;color:var(--text-3)">${l.since}</span>
      <span class="row__cell row__actions"><button class="btn btn--ghost btn--sm">${l.open ? L('收起', 'Hide') : L('查看进程', 'Show processes')}</button><button class="btn btn--danger-ghost btn--sm">${I('stop')}${L('强制停止', 'Force stop')}</button></span>
    </div>${l.open ? `<ul class="procs">${l.procs.map((p) => `<li><span class="mono">${p[0]}</span><span class="procs__cmd mono">${p[1]}</span><span class="num">${p[2]}</span><span class="num">${p[3]}</span></li>`).join('')}</ul>` : ''}`;
  const LEFT = [
    { tool: 'claude', title: L('解释 webhook 重试为什么会重复入账', 'Explain why webhook retries double-post'), local: 'acme_api', remote: 'acme_api', where: L('主工作区', 'main workspace'), n: 3, cpu: 41, mem: '612 MB', since: L('会话 12 分钟前关闭', 'session closed 12 min ago'), open: true,
      procs: [['48213', 'node ./node_modules/.bin/vite --port 5173', '38%', '402 MB'], ['48240', 'esbuild --service=0.25.4 --ping', '3%', '96 MB'], ['48102', 'sh -c "npm run dev"', '0%', '4 MB']] },
    { tool: 'opencode', title: L('给导出接口加分页', 'Add paging to the export endpoint'), local: 'cs-portal', remote: 'acme_cs_portal', where: 'ai/export-paging', n: 1, cpu: 0, mem: '48 MB', since: L('会话 2 小时前关闭', 'session closed 2 h ago') },
  ];
  const leftover = o.leftover ? `<section class="leftover"><div class="leftover__head">${I('alert')}<div><b>${L('2 个会话已经关闭，但它们启动的程序还在运行', '2 sessions are closed, but programs they started are still running')}</b><span>${L('这些程序还在占用 CPU 和内存，电脑也因此保持唤醒。', 'They still use CPU and memory, and they keep this Mac awake.')}</span></div><span class="spacer"></span><button class="btn btn--sm">${L('全部强制停止', 'Force stop all')}</button></div>
      <div class="table tasks-table tasks-table--left"><div class="table__head"><span></span><span>${L('会话', 'Session')}</span><span>${L('还在运行', 'Still running')}</span><span>${L('占用', 'Using')}</span><span>${L('关闭时间', 'Closed')}</span><span></span></div>${LEFT.map(lrow).join('')}</div></section>` : '';
  return `<div class="page__header"><div><h1 class="page__title">${L('AI 任务', 'AI tasks')}</h1><p class="page__subtitle">${o.leftover ? L('5 个任务 · 2 个运行中 · 1 个等待你 · 2 个已关闭仍在运行', '5 tasks · 2 running · 1 waiting for you · 2 closed but still running') : L('5 个任务 · 2 个运行中 · 1 个等待你', '5 tasks · 2 running · 1 waiting for you')}</p></div>
      <div class="page__actions"><span class="btn btn--ghost" title="${L('从任何页面切换到一个会话', 'Jump to a session from any screen')}">${L('切换会话', 'Switch session')} ${kbd(mod, 'J')}</span><a class="btn" href="${href('free-space')}">${I('trash')}${L('释放空间', 'Free up space')} <span class="num muted">6.4 GB</span></a><a class="btn btn--primary" href="${href('new-task')}">${I('plus')}${L('新任务', 'New task')}</a></div></div>
    <div class="page__body">
      ${leftover}
      <div class="banner banner--ok" style="padding:12px 16px">${I('coffee', 'banner__icon')}<div class="banner__body"><div class="banner__title">${L('这台 Mac 正保持唤醒', 'This Mac is being kept awake')}</div>${L('有 2 个任务在运行。最后一个结束后，电脑会恢复正常的睡眠设置。合上盖子仍然会睡眠。', '2 tasks are running. When the last one ends, the normal sleep settings return. Closing the lid still puts it to sleep.')}</div><div class="banner__actions"><a class="btn btn--ghost btn--sm" href="${href('settings')}">${L('设置', 'Settings')}</a></div></div>
      ${group('acme_api', 'aws')}${group('acme_cs_portal', 'aws')}${group('globex_backend', 'aws')}
    </div>`;
}
SCREENS['ai-tasks'] = () => shell('ai-tasks', aiTasksBody({ leftover: STATE === 'leftover' }));

/* ---------------------------------------------------------------- accounts */
function accountsBody() {
  const pass = (name, role, facts, pct, left, low) => `<article class="pass pass--active" data-acct="${ACCT[name].c}"><span class="pass__stripe"></span>
      <div class="pass__head">${tile(name)}<div style="min-width:0"><span class="pass__name">${name}</span><div class="pass__role">${role}</div></div></div>
      <dl class="pass__facts">${facts.map((f) => `<div class="pass__fact"><dd>${f}</dd></div>`).join('')}</dl>
      <div class="countdown${low ? ' countdown--low' : ''}"><div class="pass__state"><span class="dot dot--${low ? 'warn' : 'ok'}"></span>${left}</div><div class="countdown__track"><div class="countdown__fill" style="--pct:${pct}%"></div></div></div>
      <div class="pass__actions"><button class="btn btn--sm">${L('重新登录', 'Sign in again')}</button><button class="btn btn--ghost btn--sm">${L('复制配置名', 'Copy profile name')}</button><button class="icon-btn icon-btn--sm">${I('more')}</button></div></article>`;
  const gitPass = (name, sub, repos, fp, tok) => `<article class="pass pass--active" data-acct="${ACCT[name].c}"><span class="pass__stripe"></span>
      <div class="pass__head"><span class="tile-wrap">${tile(name)}<span class="prov-corner">${I(PROV[ACCT[name].p])}</span></span><div style="min-width:0"><span class="pass__name">${name}</span><div class="pass__role">${sub}</div></div></div>
      <dl class="pass__facts"><div class="pass__fact"><dd class="pass__repos"><span class="link">${repos}</span></dd></div></dl>
      <div class="pass__block">
        <div class="keyline">${I('key')}<span style="color:var(--ok);font-weight:500">${L('SSH 密钥已验证', 'SSH key verified')}</span><span class="mono">${fp}</span></div>
        <div class="keyline">${I('pr')}<span>${tok}</span></div>
      </div>
      <div class="pass__actions"><button class="btn btn--sm">${L('验证', 'Verify')}</button><button class="btn btn--ghost btn--sm">${I('copy')}${L('复制公钥', 'Copy public key')}</button><button class="icon-btn icon-btn--sm">${I('more')}</button></div></article>`;
  const add = (t, s) => `<button class="pass-add"><span class="icon-btn icon-btn--bordered">${I('plus')}</span><strong>${t}</strong>${s}</button>`;
  const sec = (icon, title, meta, pill, grid) => `<section class="portal-panel"><div class="portal__header"><button class="portal__toggle" aria-expanded="true">${I('chevR', 'portal__chevron')}<span class="portal__icon prov-section__icon">${I(icon)}</span></button><div class="portal__id"><div class="portal__title">${title}</div><div class="portal__meta">${meta}</div></div><div class="portal__tools">${pill}<button class="icon-btn icon-btn--sm">${I('more')}</button></div></div><div class="portal__grid">${grid}</div></section>`;
  return `<div class="page__header"><div><h1 class="page__title">${L('账号', 'Accounts')}</h1><p class="page__subtitle">${L('3 个 AWS 账号 · 1 个 GitHub 账号 · 1 个 Bitbucket 账号', '3 AWS accounts · 1 GitHub account · 1 Bitbucket account')}</p></div>
      <div class="page__actions"><button class="btn">${L('导出', 'Export')}</button><span class="splitbtn"><a class="btn btn--primary" href="${href('add-account')}">${I('plus')}${L('添加账号', 'Add account')}</a><button class="btn btn--primary" title="AWS / GitHub / Bitbucket" style="border-left:1px solid color-mix(in srgb,var(--on-ink) 25%,transparent)">${I('chevD')}</button></span></div></div>
    <div class="page__body">
      <div class="section-label" style="display:flex;align-items:center;gap:8px">${prov('aws')}AWS · IAM Identity Center</div>
      ${sec('globe', 'acme-prod', `<span class="mono">d-1234567890.awsapps.com/start</span><span>ap-southeast-1</span>`, `<span class="session-pill"><span class="dot dot--ok"></span>${L('剩余', 'Left')} <b>6:12</b></span>`,
        pass('acme-prod', 'AdministratorAccess', ['1111 2222 3333', 'ap-southeast-1', L('4 个仓库', '4 repositories')], 78, L('已登录，还剩 6 小时 12 分', 'Signed in, 6 h 12 min left')) +
        pass('globex', 'PowerUserAccess', ['4444 5555 6666', 'ap-southeast-1', L('1 个仓库', '1 repository')], 78, L('已登录，还剩 6 小时 12 分', 'Signed in, 6 h 12 min left')) + add(L('添加账号', 'Add an account'), L('从这个入口里选择', 'Pick from this portal')))}
      ${sec('globe', 'initech', `<span class="mono">initech.awsapps.com/start</span><span>ap-southeast-5</span>`, `<span class="session-pill session-pill--warn"><span class="dot dot--warn"></span>${L('剩余', 'Left')} <b>48m</b></span>`,
        pass('initech', 'DeveloperAccess', ['7777 8888 9999', 'ap-southeast-5', L('4 个仓库', '4 repositories')], 10, L('还剩 48 分钟', '48 min left'), true))}
      <div class="section-label" style="display:flex;align-items:center;gap:8px;margin-top:var(--sp-2)">${prov('github')}GitHub</div>
      <div class="portal__grid">${gitPass('alex-dev', 'github.com', L('2 个仓库', '2 repositories'), 'SHA256:Qm3x…f9Kc', L('API 令牌有效，可以管理拉取请求', 'API token valid, pull requests enabled'))}${add(L('添加 GitHub 账号', 'Add a GitHub account'), L('每个账号一把自己的密钥', 'One key of its own per account'))}</div>
      <div class="section-label" style="display:flex;align-items:center;gap:8px;margin-top:var(--sp-2)">${prov('bitbucket')}Bitbucket</div>
      <div class="portal__grid">${gitPass('alex_acme', L('工作区 umbrella', 'Workspace umbrella'), L('1 个仓库', '1 repository'), 'SHA256:v81T…a0Zp', `<span style="color:var(--warn)">${L('还没有 API 令牌，拉取请求暂不可用', 'No API token yet, pull requests are off')}</span> <span class="link">${L('添加', 'Add')}</span>`)}${add(L('添加 Bitbucket 账号', 'Add a Bitbucket account'), L('SSH 密钥 + API 令牌', 'SSH key + API token'))}</div>
    </div>`;
}
SCREENS.accounts = () => shell('accounts', accountsBody());

SCREENS['add-account'] = () => {
  const step = (cls, label) => `<li class="stepper__step ${cls}"><span class="stepper__marker"></span><span class="stepper__label">${label}</span></li>`;
  const modal = `<div class="modal-backdrop"><div class="modal modal--lg" role="dialog" aria-modal="true" aria-labelledby="aa-t" style="width:min(720px,100%)">
    <div class="modal__header"><span class="prov prov--lg" style="margin-top:2px;color:var(--text)">${I('github')}</span><h2 class="modal__title" id="aa-t">${L('添加 GitHub 账号', 'Add a GitHub account')}</h2><span class="spacer"></span><button class="icon-btn icon-btn--sm" title="${L('关闭', 'Close')}">${I('x')}</button></div>
    <div class="modal__body" style="display:flex;flex-direction:column;gap:var(--sp-5)">
      <ol class="stepper stepper--h">${step('is-done', L('账号', 'Account'))}${step('is-done', L('SSH 密钥', 'SSH key'))}${step('is-current', L('交给 GitHub', 'Give it to GitHub'))}${step('', L('API 令牌', 'API token'))}</ol>
      <div>
        <div style="font-size:16px;font-weight:600;color:var(--text)">${L('把公钥添加到你的 GitHub 账号', 'Add the public key to your GitHub account')}</div>
        <p style="font-size:13px;margin-top:4px">${L('Codepass 为 alex-dev 新建了一把密钥。私钥留在这台 Mac 上，只有公钥需要交给 GitHub。', 'Codepass created a new key for alex-dev. The private key stays on this Mac; only the public key goes to GitHub.')}</p>
      </div>
      <div class="field">
        <span class="field__label">${L('公钥', 'Public key')}</span>
        <div class="pubkey"><span>ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIPq8mX2vK0cT4nYw7hJd1uRzB6eLs9oGfA3tVxQy5MNk codepass-alex-dev@github</span><button class="btn btn--sm">${I('copy')}${L('复制', 'Copy')}</button></div>
        <span class="field__hint">${L('私钥保存在', 'Private key saved at')} <span class="mono">~/.ssh/codepass_github_alex-dev_ed25519</span></span>
      </div>
      <ol class="howto">
        <li><span>${L('点 <b>打开 GitHub 的 SSH 设置</b>，用 alex-dev 这个账号登录。', 'Choose <b>Open GitHub SSH settings</b> and sign in as alex-dev.')}</span></li>
        <li><span>${L('标题随意，类型选 <b>Authentication Key</b>，把公钥粘贴进去。', 'Use any title, pick <b>Authentication Key</b>, and paste the public key.')}</span></li>
        <li><span>${L('保存后回到这里，点 <b>验证</b>。', 'Save, come back here, and choose <b>Verify</b>.')}</span></li>
      </ol>
      <div class="verify is-ok">${I('circleCheck')}<span><b>${L('GitHub 认出了这把密钥', 'GitHub recognised this key')}</b> · ${L('账号是', 'the account is')} <span class="mono">alex-dev</span></span><span class="spacer"></span><button class="btn btn--ghost btn--sm">${L('重新验证', 'Verify again')}</button></div>
    </div>
    <div class="modal__footer"><button class="btn btn--ghost" style="margin-right:auto">${I('chevL')}${L('上一步', 'Back')}</button><button class="btn">${I('ext')}${L('打开 GitHub 的 SSH 设置', 'Open GitHub SSH settings')}</button><button class="btn btn--primary">${L('下一步：API 令牌', 'Next: API token')}</button></div>
  </div></div>`;
  return shell('accounts', accountsBody(), modal);
};

/* ---------------------------------------------------------------- identity (Git settings) */
SCREENS.identity = () => {
  const who = (name, mail) => `<span class="ident">${name} <span>&lt;${mail}&gt;</span></span>`;
  const proj = (name, n, identHtml, tag) => `<div class="row is-proj"><span class="row__cell"><span class="row__primary">${I('layers')}${name} <span class="muted" style="font-weight:400;font-size:12px">${L(`${n} 个仓库`, `${n} repositories`)}</span></span></span><span class="row__cell">${identHtml}</span><span class="row__cell">${tag}</span><span class="row__cell row__actions"><button class="btn btn--sm">${L('设置', 'Set')}</button></span></div>`;
  const repo = (name, p, identHtml, tag) => `<div class="row is-child"><span class="row__cell" style="gap:10px">${prov(p)}<span class="truncate" style="font-size:13px;color:var(--text)">${name}</span></span><span class="row__cell">${identHtml}</span><span class="row__cell">${tag}</span><span class="row__cell row__actions"><button class="icon-btn icon-btn--sm" title="${L('修改', 'Edit')}">${I('pencil')}</button></span></div>`;
  const tProj = `<span class="src-tag src-tag--proj">${L('来自项目', 'From project')}</span>`;
  const tGlobal = `<span class="src-tag">${L('来自全局', 'From global')}</span>`;
  const tRepo = `<span class="src-tag src-tag--repo">${L('仓库自己的', 'Repository’s own')}</span>`;
  const body = `<div class="page__header"><div><h1 class="page__title">${L('Git 设置', 'Git settings')}</h1><p class="page__subtitle">${L('提交时用谁的名字和邮箱，以及 Git 全局保护', 'Whose name and email go on your commits, and the Git global guard')}</p></div></div>
    <div class="page__body">
      <div class="card"><div class="card__body ident-hero">
        <div class="stack" style="--gap:var(--sp-4)">
          <div><div class="card__title">${L('全局作者身份', 'Global author identity')}</div><div class="field__hint" style="margin-top:2px">${L('没有自己设置的项目和仓库都用它。现在有 6 个仓库在用。', 'Used by every project and repository without its own. 6 repositories use it now.')}</div></div>
          <div class="ident-fields"><label class="field"><span class="field__label">${L('名字', 'Name')}</span><input class="input" value="${ME}"></label><label class="field"><span class="field__label">${L('邮箱', 'Email')}</span><input class="input" value="${ID_GLOBAL}"></label><button class="btn" disabled>${L('保存', 'Save')}</button></div>
        </div>
        <div class="stack" style="--gap:8px;align-items:flex-end"><div class="ladder"><span>${L('仓库', 'Repository')}</span>${I('chevR')}<span>${L('项目', 'Project')}</span>${I('chevR')}<span>${L('全局', 'Global')}</span></div><span class="field__hint">${L('越具体的越优先', 'The most specific one wins')}</span></div>
      </div></div>
      <div class="stack" style="--gap:var(--sp-3)">
        <div class="section-label">${L('项目和仓库实际使用的身份', 'What each project and repository actually uses')}</div>
        <div class="table ident-table">
          <div class="table__head"><span>${L('项目 / 仓库', 'Project / repository')}</span><span>${L('生效的作者身份', 'Effective author identity')}</span><span>${L('来源', 'Source')}</span><span></span></div>
          ${proj('acme', 4, who(ME, ID_PROJ), `<span class="src-tag src-tag--proj">${L('项目设置', 'Project setting')}</span>`)}
          ${repo('acme_api', 'aws', who(ME, ID_PROJ), tProj)}
          ${repo('acme_agent_portal', 'aws', who(ME, ID_PROJ), tProj)}
          ${repo('acme_cs_portal', 'aws', who(ME, ID_PROJ), tProj)}
          ${repo('acme_merchant_portal_v4', 'aws', who('alex-work', 'alex@old.example.com'), `${tRepo}`)}
          ${proj('globex', 2, `<span class="ident is-inherit">${L('跟随全局', 'Follows global')}</span>`, '')}
          ${repo('globex_backend', 'aws', who(ME, ID_GLOBAL), tGlobal)}
          ${repo('globex_app', 'github', who('alex-dev', 'alex-dev@noreply.example.com'), tRepo)}
          ${proj('umbrella', 1, `<span class="ident is-inherit">${L('跟随全局', 'Follows global')}</span>`, '')}
          ${repo('umbrella', 'bitbucket', who(ME, ID_GLOBAL), tGlobal)}
        </div>
      </div>
    </div>`;
  const sheet = `<div class="sheet-backdrop"></div><aside class="sheet" role="dialog" aria-modal="true" aria-labelledby="id-t">
    <div class="sheet__head"><span class="portal__icon">${I('layers')}</span><div><div class="sheet__title" id="id-t">${L('acme 的作者身份', 'Author identity for acme')}</div><div class="sheet__sub">${L('这个项目里的仓库提交时使用', 'Used for commits in this project’s repositories')}</div></div><span class="spacer"></span><button class="icon-btn icon-btn--sm">${I('x')}</button></div>
    <div class="sheet__body">
      <div class="stack" style="--gap:8px">
        <label class="opt"><input type="radio" class="radio" name="src"><span class="opt__title">${L('跟随全局', 'Follow global')}</span><span class="opt__text">${ME} &lt;${ID_GLOBAL}&gt;</span></label>
        <label class="opt is-on"><input type="radio" class="radio" name="src" checked><span class="opt__title">${L('这个项目用自己的', 'This project has its own')}</span><span class="opt__text">${L('会写进每个仓库自己的设置，关掉 Codepass 也有效。', 'Written into each repository’s own config, so it also works with Codepass closed.')}</span></label>
      </div>
      <div class="stack"><label class="field"><span class="field__label">${L('名字', 'Name')}</span><input class="input" value="${ME}"></label><label class="field"><span class="field__label">${L('邮箱', 'Email')}</span><input class="input" value="${ID_PROJ}"></label></div>
      <div class="stack" style="--gap:8px">
        <div class="field__label">${L('保存后会改动 3 个仓库', 'Saving changes 3 repositories')}</div>
        <ul class="affect">
          <li>${prov('aws')}acme_api<span class="spacer"></span><span class="was">${ID_GLOBAL}</span></li>
          <li>${prov('aws')}acme_agent_portal<span class="spacer"></span><span class="was">${ID_GLOBAL}</span></li>
          <li>${prov('aws')}acme_cs_portal<span class="spacer"></span><span class="was">${ID_GLOBAL}</span></li>
          <li style="background:var(--surface-2)">${prov('aws')}<span class="muted">acme_merchant_portal_v4</span><span class="spacer"></span><span class="src-tag src-tag--repo">${L('保留自己的', 'Keeps its own')}</span></li>
        </ul>
        <span class="field__hint">${L('每个仓库的 .git/config 会先备份，可以撤销。', 'Each repository’s .git/config is backed up first, and you can undo.')}</span>
      </div>
    </div>
    <div class="sheet__foot"><button class="btn">${L('取消', 'Cancel')}</button><button class="btn btn--primary">${L('保存并应用到 3 个仓库', 'Save and apply to 3 repositories')}</button></div>
  </aside>`;
  return shell('identity', body, STATE === 'page' ? '' : sheet);
};

/* ---------------------------------------------------------------- settings */
SCREENS.settings = () => {
  const sw = (on) => `<button class="switch" role="switch" aria-checked="${on}"><span class="switch__thumb"></span></button>`;
  const toolRow = (t, name, d, right) => `<div class="set-row set-row--tool">${tool(t, true)}<div><div class="set-row__t">${name}</div><div class="set-row__d">${d}</div></div><div class="set-row__c">${right}</div></div>`;
  const inst = (v) => `<span class="badge badge--ok">${I('check')}${v}</span>`;
  const seg = (items, on) => `<div class="segmented">${items.map((x, i) => `<button class="segmented__item${i === on ? ' is-active' : ''}">${x}</button>`).join('')}</div>`;
  const row = (t, d, c) => `<div class="set-row"><div><div class="set-row__t">${t}</div>${d ? `<div class="set-row__d">${d}</div>` : ''}</div><div class="set-row__c">${c}</div></div>`;
  return shell('settings', `<div class="page__header"><div><h1 class="page__title">${L('设置', 'Settings')}</h1><p class="page__subtitle">Codepass 1.0.0 · ${L('macOS 15 或更高，Apple 芯片', 'macOS 15 or later, Apple silicon')}</p></div></div>
    <div class="set-grid">
      <div class="stack" style="--gap:var(--sp-5)">
        <div class="card"><div class="card__header"><div class="card__title">${L('AI 编码工具', 'AI coding tools')}</div></div>
          ${toolRow('claude', 'Claude Code', L('命令 <span class="mono">claude</span> · 自己会保持最新', 'Command <span class="mono">claude</span> · keeps itself up to date'), inst('2.4.11'))}
          ${toolRow('codex', 'Codex', L('命令 <span class="mono">codex</span> · 通过 Homebrew 安装', 'Command <span class="mono">codex</span> · installed with Homebrew'), `${inst('0.169.0')}<button class="btn btn--sm">${L('更新', 'Update')}</button>`)}
          ${toolRow('opencode', 'OpenCode', L('还没有安装。第一次新建任务时会问你要不要装。', 'Not installed yet. You are asked the first time you start a task with it.'), `<button class="btn btn--sm">${I('download')}${L('安装', 'Install')}</button>`)}
          ${row(L('新任务在哪里运行', 'Where a new task runs'), L('独立工作区：每个任务有自己的文件夹和分支，互不影响。', 'Own workspace: each task gets its own folder and branch, so tasks cannot disturb each other.'), seg([L('独立工作区', 'Own workspace'), L('主工作区', 'Main workspace')], 0))}
          ${row(L('同时运行的任务上限', 'Tasks running at once'), L('所有任务共用同一个账号的用量，开得越多用得越快。', 'All tasks share one account’s usage allowance; more tasks spend it faster.'), `<select class="select" style="width:84px"><option>6</option></select>`)}
        </div>
        <div class="card"><div class="card__header"><div class="card__title">${L('编辑器和代码智能', 'Editor and code intelligence')}</div></div>
          ${row('Go · gopls', L('跳到定义、查找引用、悬停说明、补全、错误提示', 'Go to definition, find references, hover, completion, diagnostics'), inst('0.21.0'))}
          ${row('TypeScript / JavaScript', L('typescript-language-server · 打开对应文件时自动安装', 'typescript-language-server · installed automatically when you open such a file'), inst('5.1.3'))}
          ${row(L('字号', 'Font size'), '', seg(['12', '13', '14', '15'], 1))}
          ${row(L('自动换行', 'Wrap long lines'), '', sw(false))}
        </div>
      </div>
      <div class="stack" style="--gap:var(--sp-5)">
        <div class="card"><div class="card__header">${I('coffee')}<div class="card__title">${L('保持唤醒', 'Keep awake')}</div><span class="spacer"></span><span class="badge badge--ok"><span class="dot dot--ok" style="width:6px;height:6px;box-shadow:none"></span>${L('现在生效 · 2 个任务', 'Active now · 2 tasks')}</span></div>
          ${row(L('AI 任务运行时不让电脑睡眠', 'Keep the computer awake while AI tasks run'), L('只在有任务运行时生效，最后一个任务结束就恢复。普通终端不算。', 'Only while a task is running; it ends with the last task. A plain terminal does not count.'), sw(true))}
          ${row(L('屏幕也保持常亮', 'Keep the screen on too'), L('关掉后屏幕可以熄灭并锁定，AI 照常工作。在办公室更安全。', 'Turn off to let the screen sleep and lock while the AI keeps working. Safer in an office.'), sw(true))}
          ${row(L('任务等你回答时，继续保持', 'While a task waits for your answer, stay awake for'), '', seg([L('5 分钟', '5 min'), L('15 分钟', '15 min'), L('1 小时', '1 hour'), L('一直', 'Always')], 1))}
          <div style="padding:0 var(--sp-5) var(--sp-4)"><ul class="limits"><li>${L('合上 MacBook 的盖子仍然会睡眠，除非接着电源和外接显示器。', 'Closing a MacBook’s lid still puts it to sleep, unless it is on power with an external display.')}</li><li>${L('屏幕常亮时，电脑不会自动锁屏。离开座位请手动锁定。', 'While the screen stays on, the computer does not lock by itself. Lock it by hand when you leave.')}</li></ul></div>
        </div>
        <div class="card"><div class="card__header"><div class="card__title">${L('更新', 'Updates')}</div></div>
          ${row(L('已是最新版本 1.0.0', 'Version 1.0.0 is the latest'), L('10 分钟前检查过 · 从 github.com/rudian/codepass 的发布页获取', 'Checked 10 minutes ago · from the releases of github.com/rudian/codepass'), `<button class="btn btn--sm">${L('现在检查', 'Check now')}</button>`)}
          ${row(L('每天自动检查一次', 'Check automatically once a day'), L('只读取版本号，不上传任何信息。下载的更新会先校验签名。', 'Reads the version number only and sends nothing. Downloads are signature-checked before install.'), sw(true))}
        </div>
        <div class="card"><div class="card__header"><div class="card__title">${L('外观', 'Appearance')}</div></div>
          ${row(L('主题', 'Theme'), '', seg([L('跟随系统', 'System'), L('浅色', 'Light'), L('深色', 'Dark')], 0))}
          ${row(L('语言', 'Language'), '', seg(['中文', 'English'], zh ? 0 : 1))}
          ${row(L('动效', 'Motion'), L('「减少」会去掉滑动和淡入，只保留必要的进度提示。', '“Reduced” removes slides and fades and keeps only essential progress cues.'), seg([L('跟随系统', 'System'), L('完整', 'Full'), L('减少', 'Reduced')], 0))}
        </div>
      </div>
    </div>`);
};

SCREENS._other = (id) => shell(id, `<div class="empty" style="margin-top:120px">${I('info', 'empty__art')}<div class="empty__title">${L('这一页沿用现有设计', 'This page keeps its current design')}</div><div class="empty__text">${L('本次样稿只画了有变化的页面。', 'This mockup only covers the screens that change.')}</div></div>`);
