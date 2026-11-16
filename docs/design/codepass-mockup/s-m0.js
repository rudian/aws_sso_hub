/* Codepass mockup - screens added at the milestone-0 gate (2026-11-16):
   session switcher, rename local folder, free up space, closed-but-still-running rows, quit dialog. */

/* ---------------------------------------------------------------- session switcher (Cmd/Ctrl+J, any screen) */
const SESSIONS = [
  { g: 'wait', tool: 'claude', title: L('修复结算对账的四舍五入误差', 'Fix rounding error in settlement reconcile'), local: 'acme_api', remote: 'acme_api', p: 'aws', where: 'ai/fix-settlement-rounding', st: 'wait', time: '14:08', note: L('要你批准一条命令', 'needs you to approve a command'), on: true },
  { g: 'run', tool: 'codex', title: L('给 payout 批处理补单元测试', 'Add unit tests for payout batching'), local: 'acme_api', remote: 'acme_api', p: 'aws', where: 'ai/payout-batch-tests', st: 'run', time: '06:41' },
  { g: 'run', tool: 'opencode', title: L('把商户列表改成虚拟滚动', 'Virtualise the merchant list'), local: 'cs-portal', remote: 'acme_cs_portal', p: 'aws', where: 'ai/merchant-list-virtual', st: 'run', time: '21:17' },
  { g: 'run', tool: 'claude', title: L('登录页接入手机号验证', 'Wire phone verification into sign-in'), local: 'globex_app', remote: 'globex_app', p: 'github', where: L('主工作区', 'main workspace') + ' · feat/phone-verify', st: 'run', time: '02:55' },
  { g: 'left', tool: 'claude', title: L('解释 webhook 重试为什么会重复入账', 'Explain why webhook retries double-post'), local: 'acme_api', remote: 'acme_api', p: 'aws', where: L('主工作区', 'main workspace'), left: 3, time: '41%' },
  { g: 'end', tool: 'claude', title: L('升级 Go 到 1.25 并修复告警', 'Upgrade Go to 1.25 and fix warnings'), local: 'globex_backend', remote: 'globex_backend', p: 'aws', where: 'ai/go-1-25', st: 'fail', time: '00:48' },
];
function switcherOverlay() {
  const groups = [['wait', L('等待你', 'Waiting for you')], ['run', L('运行中', 'Running')], ['left', L('已关闭，仍在运行', 'Closed, still running')], ['end', L('刚结束', 'Just ended')]];
  let n = 0;
  const row = (s) => { n += 1; return `<li class="sess${s.on ? ' is-on' : ''}" role="option" aria-selected="${!!s.on}">${tool(s.tool)}
      <span class="sess__main"><span class="sess__title">${s.title}</span><span class="sess__sub">${prov(s.p)}<b>${repoName(s.local, s.remote)}</b><span class="mono">${s.where}</span>${s.note ? `<span class="sess__note">${s.note}</span>` : ''}</span></span>
      ${s.left ? `<span class="state state--left"><span class="dot dot--warn"></span>${L(`${s.left} 个进程`, `${s.left} processes`)}</span><span class="num sess__time">CPU ${s.time}</span>` : `${state(s.st)}<span class="num sess__time">${s.time}</span>`}
      <span class="kbd sess__n">${n}</span></li>`; };
  const list = groups.map(([g, label]) => { const items = SESSIONS.filter((s) => s.g === g); return items.length ? `<li class="sess-group" role="presentation">${label} <span>${items.length}</span></li>${items.map(row).join('')}` : ''; }).join('');
  return `<div class="switcher-backdrop"><div class="switcher" role="dialog" aria-modal="true" aria-label="${L('切换会话', 'Switch session')}">
    <div class="switcher__head">${I('search')}<input class="switcher__input" placeholder="${L('搜索会话、仓库或分支', 'Search sessions, repositories or branches')}" aria-label="${L('搜索会话', 'Search sessions')}"><span class="switcher__count">${L('4 个在运行 · 1 个等待你', '4 live · 1 waiting for you')}</span>${kbd(mod, 'J')}</div>
    <ul class="switcher__list" role="listbox">${list}</ul>
    <div class="switcher__foot"><span>${kbd('↑', '↓')} ${L('选择', 'Select')}</span><span>${kbd('⏎')} ${L('跳到这个会话的终端', 'Go to its terminal')}</span><span>${kbd(mod, 'J')} ${L('下一个', 'Next')}</span><span>${kbd(mod, '⏎')} ${L('查看改动', 'Review changes')}</span><span class="spacer"></span><a class="link-button" href="${href('ai-tasks')}">${L('打开 AI 任务页', 'Open AI tasks')}</a></div>
  </div></div>`;
}
SCREENS['session-switcher'] = () => shell('projects', projectsBody(), switcherOverlay());

/* ---------------------------------------------------------------- rename local folder */
SCREENS['rename-folder'] = () => {
  const li = (icon, main, sub, right) => `<li>${I(icon)}<span class="affect__main"><b>${main}</b>${sub ? `<small>${sub}</small>` : ''}</span><span class="spacer"></span>${right || ''}</li>`;
  const modal = `<div class="modal-backdrop"><div class="modal" role="dialog" aria-modal="true" aria-labelledby="rf-t" style="width:min(640px,100%)">
    <div class="modal__header"><div><h2 class="modal__title" id="rf-t">${L('重命名本地文件夹', 'Rename local folder')}</h2><div class="sheet__sub">${L('只改这台电脑上的文件夹。远程仓库不会变。', 'Only the folder on this computer changes. The remote repository stays as it is.')}</div></div><span class="spacer"></span><button class="icon-btn icon-btn--sm" title="${L('关闭', 'Close')}">${I('x')}</button></div>
    <div class="modal__body" style="display:flex;flex-direction:column;gap:var(--sp-4)">
      <div class="two-name">
        <div class="two-name__box two-name__box--fixed"><span class="two-name__k">${prov('aws')}${L('远程仓库名', 'Remote name')}</span><span class="two-name__v mono">acme_api</span><span class="two-name__h">${L('保持不变', 'Stays the same')}</span></div>
        ${I('arrowR')}
        <label class="two-name__box"><span class="two-name__k">${I('folder')}${L('本地文件夹名', 'Local folder name')}</span><input class="input mono" value="api-main" aria-describedby="rf-path"><span class="two-name__h">${L('现在是 acme_api', 'Now acme_api')}</span></label>
      </div>
      <div class="field" style="gap:6px"><span class="field__label">${L('放在哪里', 'Location')}</span>
        <div class="path-line" id="rf-path"><span class="mono"><s>~/Documents/code/acme_api</s></span>${I('arrowR')}<span class="mono"><b>~/Documents/code/api-main</b></span><span class="spacer"></span><button class="btn btn--sm">${L('换一个上级文件夹…', 'Choose another parent folder…')}</button></div></div>
      <div class="rf-cols">
        <div class="field" style="gap:6px"><span class="field__label">${L('开始前会停止', 'Stopped first')}</span>
          <ul class="affect affect--icons">
            ${li('bot', L('2 个 AI 会话', '2 AI sessions'), L('对话会保留，改名后可以继续', 'Conversations are kept and can be resumed'), state('run'))}
            ${li('terminal', L('1 个终端', '1 terminal'), 'zsh')}
            ${li('code', L('代码智能', 'Code intelligence'), L('gopls、TypeScript，之后自动重启', 'gopls, TypeScript; restart by themselves'))}
            ${li('eye', L('文件变化监视', 'File watching'), L('之后自动恢复', 'Comes back by itself'))}
          </ul></div>
        <div class="field" style="gap:6px"><span class="field__label">${L('会跟着更新', 'Updated to match')}</span>
          <ul class="affect affect--icons">
            ${li('layers', L('项目和仓库记录', 'Project and repository records'), L('绑定的账号、作者身份不变', 'Account binding and author identity are unchanged'))}
            ${li('branch', L('2 个任务的独立工作区', '2 task workspaces'), L('重新连到新位置，文件夹不动', 'Re-linked to the new location; their folders do not move'))}
            ${li('history', L('3 条任务记录、连接检查结果', '3 task records, connection checks'))}
          </ul></div>
      </div>
      <div class="banner banner--warn" style="padding:10px 14px">${I('alert', 'banner__icon')}<div class="banner__body">${L('在主工作区运行过的 1 个 Claude Code 会话可能接不上原来的对话：这个工具按文件夹路径保存对话。', '1 Claude Code session that ran in the main workspace may not find its earlier conversation: that tool files conversations by folder path.')}</div></div>
      <div class="note-line">${I('shield')}<span>${L('任何一步失败，文件夹都会改回原来的名字，记录也恢复原样。', 'If any step fails, the folder gets its old name back and every record is restored.')}</span></div>
    </div>
    <div class="modal__footer"><a class="btn" href="${href('projects')}">${L('取消', 'Cancel')}</a><a class="btn btn--primary" href="${href('projects')}">${L('停止并重命名', 'Stop and rename')}</a></div>
  </div></div>`;
  return shell('projects', projectsBody(), modal);
};

/* ---------------------------------------------------------------- free up space (closed sessions) */
SCREENS['free-space'] = () => {
  const S = [
    { on: 1, tool: 'claude', title: L('升级 Go 到 1.25 并修复告警', 'Upgrade Go to 1.25 and fix warnings'), repo: 'globex_backend', when: L('失败 · 3 天前', 'Failed · 3 days ago'), wt: '1.9 GB', wtn: L('其中构建产物 1.7 GB', '1.7 GB of it is build output'), log: '9 MB', tr: '31 MB', sum: '1.94 GB' },
    { on: 1, tool: 'opencode', title: L('给导出接口加分页', 'Add paging to the export endpoint'), repo: 'cs-portal', remote: 'acme_cs_portal', when: L('已合并 · 6 天前', 'Merged · 6 days ago'), wt: '2.3 GB', wtn: L('其中 node_modules 2.1 GB', '2.1 GB of it is node_modules'), log: '14 MB', tr: L('由 OpenCode 保管', 'kept by OpenCode'), trDim: 1, sum: '2.31 GB' },
    { on: 1, tool: 'claude', title: L('解释 webhook 重试为什么会重复入账', 'Explain why webhook retries double-post'), repo: 'acme_api', when: L('已完成 · 昨天', 'Done · yesterday'), wt: L('没有（主工作区）', 'None (main workspace)'), wtDim: 1, log: '3 MB', tr: '12 MB', sum: '15 MB' },
    { on: 0, warn: 1, tool: 'codex', title: L('重写对账报表的导出', 'Rewrite the reconcile report export'), repo: 'acme_api', when: L('已停止 · 9 天前', 'Stopped · 9 days ago'), wt: '2.1 GB', wtn: L('有 2 个提交没有合并', '2 commits not merged anywhere'), log: '22 MB', tr: '48 MB', sum: '2.17 GB' },
  ];
  const row = (s) => `<div class="row${s.warn ? ' is-warn' : ''}">
      <span class="row__cell"><input type="checkbox" class="checkbox"${s.on ? ' checked' : ''} aria-label="${s.title}"></span>
      <span class="row__cell" style="gap:10px">${tool(s.tool)}<span style="min-width:0"><span class="row__primary truncate" style="display:block">${s.title}</span><span class="row__secondary truncate" style="display:block">${repoName(s.repo, s.remote)} · ${s.when}</span></span></span>
      <span class="row__cell fs-size"><span class="num${s.wtDim ? ' muted' : ''}">${s.wt}</span>${s.wtn ? `<small${s.warn ? ' class="is-warn"' : ''}>${s.wtn}</small>` : ''}</span>
      <span class="row__cell num">${s.log}</span>
      <span class="row__cell${s.trDim ? ' muted' : ' num'}" style="${s.trDim ? 'font-size:11.5px' : ''}">${s.tr}</span>
      <span class="row__cell num" style="font-weight:600;justify-content:flex-end">${s.sum}</span>
    </div>`;
  const P = [
    ['~/.codepass/worktrees/globex_backend/go-1-25/', '1.9 GB', L('任务的独立工作区', 'task workspace')],
    ['~/Library/Application Support/AWS SSO Hub/sessions/t_05/', '9 MB', L('Codepass 的终端记录', 'Codepass terminal log')],
    ['~/.claude/projects/-Users-alex--codepass-worktrees-globex-backend-go-1-25/7c1e52aa-41f0-4c6e-9b1d-2f6a3d0e8b11.jsonl', '27 MB', L('Claude Code 的对话记录', 'Claude Code transcript')],
    ['~/.claude/projects/-Users-alex--codepass-worktrees-globex-backend-go-1-25/7c1e52aa-41f0-4c6e-9b1d-2f6a3d0e8b11/', '4 MB', L('同一会话的子任务记录', 'its sub-agent records')],
    ['~/.codepass/worktrees/acme_cs_portal/export-paging/', '2.3 GB', L('任务的独立工作区', 'task workspace')],
    ['~/Library/Application Support/AWS SSO Hub/sessions/t_08/', '14 MB', L('Codepass 的终端记录', 'Codepass terminal log')],
    ['opencode session delete ses_cp08f3a1…', '', L('由 OpenCode 自己删除这个会话', 'OpenCode removes this session itself'), 1],
    ['~/Library/Application Support/AWS SSO Hub/sessions/t_04/', '3 MB', L('Codepass 的终端记录', 'Codepass terminal log')],
    ['~/.claude/projects/-Users-alex-Documents-code-acme-api/b90d3f17-0a52-4d0b-8e7c-55c1a9e4f2d6.jsonl', '12 MB', L('Claude Code 的对话记录', 'Claude Code transcript')],
  ];
  const modal = `<div class="modal-backdrop"><div class="modal" role="dialog" aria-modal="true" aria-labelledby="fs-t" style="width:min(880px,100%)">
    <div class="modal__header"><div><h2 class="modal__title" id="fs-t">${L('释放空间', 'Free up space')}</h2><div class="sheet__sub">${L('4 个已结束的会话占用 6.4 GB。正在运行的 3 个会话不在这里，也不会被动到。', '4 ended sessions use 6.4 GB. The 3 sessions that are running are not listed and are never touched.')}</div></div><span class="spacer"></span><button class="icon-btn icon-btn--sm" title="${L('关闭', 'Close')}">${I('x')}</button></div>
    <div class="modal__body" style="display:flex;flex-direction:column;gap:var(--sp-4)">
      <div class="table fs-table"><div class="table__head"><span></span><span>${L('会话', 'Session')}</span><span>${L('工作区', 'Workspace')}</span><span>${L('终端记录', 'Terminal log')}</span><span>${L('工具的对话记录', 'Tool transcript')}</span><span style="text-align:right">${L('合计', 'Total')}</span></div>${S.map(row).join('')}</div>
      <label class="fs-extra"><input type="checkbox" class="checkbox"><span><b>${L('也删除「重写对账报表的导出」（2 个提交还没合并）', 'Also delete “Rewrite the reconcile report export” (2 commits not merged)')}</b><small>${L('这 2 个提交 14 天内可以在「最近丢弃」里找回；没提交的文件不能找回。', 'The 2 commits can be brought back from Recently discarded for 14 days. Files that were never committed cannot.')}</small></span></label>
      <div class="field" style="gap:6px"><span class="field__label">${L('将要删除的 9 项，一项不多', 'Exactly these 9 items will be deleted')}</span>
        <ul class="paths">${P.map(([p, sz, what, cmd]) => `<li${cmd ? ' class="is-cmd"' : ''}><span class="paths__p mono">${p}</span><span class="paths__w">${what}</span><span class="num">${sz}</span></li>`).join('')}</ul></div>
    </div>
    <div class="modal__footer"><span style="margin-right:auto;font-size:12px;color:var(--text-3)">${L('已选 3 个会话 · 将释放 4.27 GB · 不经过废纸篓', '3 sessions selected · frees 4.27 GB · does not go through the Trash')}</span><a class="btn" href="${href('ai-tasks')}">${L('取消', 'Cancel')}</a><a class="btn btn--danger" href="${href('ai-tasks')}">${I('trash')}${L('删除并释放 4.27 GB', 'Delete and free 4.27 GB')}</a></div>
  </div></div>`;
  return shell('ai-tasks', aiTasksBody(), modal);
};

/* ---------------------------------------------------------------- closed but still running (AI tasks list) */
SCREENS['ai-leftover'] = () => shell('ai-tasks', aiTasksBody({ leftover: true }));

/* ---------------------------------------------------------------- quit with live AI sessions */
SCREENS['quit-dialog'] = () => {
  const live = [
    ['claude', L('修复结算对账的四舍五入误差', 'Fix rounding error in settlement reconcile'), 'acme_api', 'wait', ''],
    ['codex', L('给 payout 批处理补单元测试', 'Add unit tests for payout batching'), 'acme_api', 'run', L('还启动了 2 个程序', 'started 2 programs')],
    ['opencode', L('把商户列表改成虚拟滚动', 'Virtualise the merchant list'), repoName('cs-portal', 'acme_cs_portal'), 'run', L('还启动了 1 个程序', 'started 1 program')],
  ];
  const modal = `<div class="modal-backdrop"><div class="modal" role="alertdialog" aria-modal="true" aria-labelledby="q-t" aria-describedby="q-d" style="width:min(600px,100%)">
    <div class="modal__header"><div><h2 class="modal__title" id="q-t">${L('退出 Codepass？', 'Quit Codepass?')}</h2><div class="sheet__sub" id="q-d">${L('还有 3 个 AI 会话在运行。', '3 AI sessions are still running.')}</div></div></div>
    <div class="modal__body" style="display:flex;flex-direction:column;gap:var(--sp-4)">
      <ul class="affect affect--icons">${live.map(([t, title, repo, st, extra]) => `<li>${tool(t)}<span class="affect__main"><b>${title}</b><small>${repo}${extra ? ' · ' + extra : ''}</small></span><span class="spacer"></span>${state(st)}</li>`).join('')}</ul>
      <div class="choice">
        <a class="choice__item choice__item--danger" href="${href('projects')}"><span class="choice__t">${I('stop')}${L('全部停止并退出', 'Stop all and quit')}</span><span class="choice__d">${L('结束这 3 个会话和它们启动的 3 个程序。分支、改动和对话都保留，下次可以继续。', 'Ends these 3 sessions and the 3 programs they started. Branches, changes and conversations are kept, so you can resume later.')}</span></a>
        <a class="choice__item is-default" href="${href('projects')}"><span class="choice__t">${I('coffee')}${L('留在后台继续运行', 'Keep running in the background')}</span><span class="choice__d">${L('只关闭窗口。Codepass 留在程序坞里，会话不中断，电脑继续保持唤醒。', 'Only the window closes. Codepass stays in the Dock, the sessions carry on and this Mac stays awake.')}</span></a>
      </div>
    </div>
    <div class="modal__footer"><span style="margin-right:auto;font-size:12px;color:var(--text-3)">${L('每次退出都会问，不会替你记住选择。', 'Codepass asks every time and never remembers a choice for you.')}</span><a class="btn btn--ghost" href="${href('workspace-ai')}">${L('取消', 'Cancel')} ${kbd('Esc')}</a></div>
  </div></div>`;
  return ws(aiOpts(modal), aiBody());
};
