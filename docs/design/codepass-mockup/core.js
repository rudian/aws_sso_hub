/* Codepass mockup - helpers, icons, shell and workspace frame. Static: no backend, no network. */
const Q = new URLSearchParams(location.search);
const SCREEN = Q.get('screen') || 'projects';
const THEME = Q.get('theme') === 'dark' ? 'dark' : 'light';
const LANG = Q.get('lang') === 'en' ? 'en' : 'zh-CN';
const OS = Q.get('os') === 'win' ? 'win' : 'mac';
const STATE = Q.get('state') || '';
const PANEL = Q.get('panel') || '';
const zh = LANG !== 'en';
const L = (a, b) => (zh ? a : b);
const mod = OS === 'win' ? 'Ctrl' : '⌘';
document.documentElement.dataset.theme = THEME;
document.documentElement.lang = LANG;
if (OS === 'win') document.documentElement.dataset.platform = 'windows';

const href = (screen, extra) => {
  const p = new URLSearchParams({ screen, theme: THEME, lang: LANG });
  if (OS === 'win') p.set('os', 'win');
  if (Q.get('bar') === '0') p.set('bar', '0');
  for (const k in extra || {}) p.set(k, extra[k]);
  return 'index.html?' + p.toString();
};

/* lucide icons (24x24, stroke 2), inline */
const IC = {
  layers: '<path d="M12.83 2.18a2 2 0 0 0-1.66 0L2.6 6.08a1 1 0 0 0 0 1.83l8.58 3.91a2 2 0 0 0 1.66 0l8.58-3.9a1 1 0 0 0 0-1.83z"/><path d="M2 12a1 1 0 0 0 .58.91l8.6 3.91a2 2 0 0 0 1.65 0l8.58-3.9A1 1 0 0 0 22 12"/><path d="M2 17a1 1 0 0 0 .58.91l8.6 3.91a2 2 0 0 0 1.65 0l8.58-3.9A1 1 0 0 0 22 17"/>',
  bot: '<path d="M12 8V4H8"/><rect width="16" height="12" x="4" y="8" rx="2"/><path d="M2 14h2"/><path d="M20 14h2"/><path d="M15 13v2"/><path d="M9 13v2"/>',
  idcard: '<path d="M16 10h2"/><path d="M16 14h2"/><path d="M6.17 15a3 3 0 0 1 5.66 0"/><circle cx="9" cy="11" r="2"/><rect x="2" y="5" width="20" height="14" rx="2"/>',
  pr: '<circle cx="18" cy="18" r="3"/><circle cx="6" cy="6" r="3"/><path d="M13 6h3a2 2 0 0 1 2 2v7"/><line x1="6" x2="6" y1="9" y2="21"/>',
  merge: '<circle cx="18" cy="18" r="3"/><circle cx="6" cy="6" r="3"/><path d="M6 21V9a9 9 0 0 0 9 9"/>',
  branch: '<line x1="6" x2="6" y1="3" y2="15"/><circle cx="18" cy="6" r="3"/><circle cx="6" cy="18" r="3"/><path d="M18 9a9 9 0 0 1-9 9"/>',
  commit: '<circle cx="12" cy="12" r="3"/><line x1="3" x2="9" y1="12" y2="12"/><line x1="15" x2="21" y1="12" y2="12"/>',
  sliders: '<line x1="21" x2="14" y1="4" y2="4"/><line x1="10" x2="3" y1="4" y2="4"/><line x1="21" x2="12" y1="12" y2="12"/><line x1="8" x2="3" y1="12" y2="12"/><line x1="21" x2="16" y1="20" y2="20"/><line x1="12" x2="3" y1="20" y2="20"/><line x1="14" x2="14" y1="2" y2="6"/><line x1="8" x2="8" y1="10" y2="14"/><line x1="16" x2="16" y1="18" y2="22"/>',
  shield: '<path d="M20 13c0 5-3.5 7.5-7.66 8.95a1 1 0 0 1-.67-.01C7.5 20.5 4 18 4 13V6a1 1 0 0 1 1-1c2 0 4.5-1.2 6.24-2.72a1.17 1.17 0 0 1 1.52 0C14.51 3.81 17 5 19 5a1 1 0 0 1 1 1z"/><path d="m9 12 2 2 4-4"/>',
  book: '<path d="M12 7v14"/><path d="M3 18a1 1 0 0 1-1-1V4a1 1 0 0 1 1-1h5a4 4 0 0 1 4 4 4 4 0 0 1 4-4h5a1 1 0 0 1 1 1v13a1 1 0 0 1-1 1h-6a3 3 0 0 0-3 3 3 3 0 0 0-3-3z"/>',
  settings: '<path d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z"/><circle cx="12" cy="12" r="3"/>',
  terminal: '<path d="m7 11 2-2-2-2"/><path d="M11 13h4"/><rect width="18" height="18" x="3" y="3" rx="2"/>',
  chevR: '<path d="m9 18 6-6-6-6"/>', chevD: '<path d="m6 9 6 6 6-6"/>', chevL: '<path d="m15 18-6-6 6-6"/>',
  check: '<path d="M20 6 9 17l-5-5"/>', x: '<path d="M18 6 6 18"/><path d="m6 6 12 12"/>',
  plus: '<path d="M5 12h14"/><path d="M12 5v14"/>', minus: '<path d="M5 12h14"/>',
  search: '<circle cx="11" cy="11" r="8"/><path d="m21 21-4.3-4.3"/>',
  file: '<path d="M15 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7Z"/><path d="M14 2v4a2 2 0 0 0 2 2h4"/>',
  folder: '<path d="M20 20a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.9a2 2 0 0 1-1.69-.9L9.6 3.9A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2Z"/>',
  folderOpen: '<path d="m6 14 1.5-2.9A2 2 0 0 1 9.24 10H20a2 2 0 0 1 1.94 2.5l-1.54 6a2 2 0 0 1-1.95 1.5H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h3.9a2 2 0 0 1 1.69.9l.81 1.2a2 2 0 0 0 1.67.9H18a2 2 0 0 1 2 2v2"/>',
  up: '<path d="m5 12 7-7 7 7"/><path d="M12 19V5"/>', down: '<path d="M12 5v14"/><path d="m19 12-7 7-7-7"/>',
  arrowR: '<path d="M5 12h14"/><path d="m12 5 7 7-7 7"/>',
  refresh: '<path d="M3 12a9 9 0 0 1 9-9 9.75 9.75 0 0 1 6.74 2.74L21 8"/><path d="M21 3v5h-5"/><path d="M21 12a9 9 0 0 1-9 9 9.75 9.75 0 0 1-6.74-2.74L3 16"/><path d="M8 16H3v5"/>',
  download: '<path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" x2="12" y1="15" y2="3"/>',
  copy: '<rect width="14" height="14" x="8" y="8" rx="2" ry="2"/><path d="M4 16c-1.1 0-2-.9-2-2V4c0-1.1.9-2 2-2h10c1.1 0 2 .9 2 2"/>',
  key: '<path d="M2.586 17.414A2 2 0 0 0 2 18.828V21a1 1 0 0 0 1 1h3a1 1 0 0 0 1-1v-1a1 1 0 0 1 1-1h1a1 1 0 0 0 1-1v-1a1 1 0 0 1 1-1h.172a2 2 0 0 0 1.414-.586l.814-.814a6.5 6.5 0 1 0-4-4z"/><circle cx="16.5" cy="7.5" r=".5" fill="currentColor"/>',
  coffee: '<path d="M10 2v2"/><path d="M14 2v2"/><path d="M16 8a1 1 0 0 1 1 1v8a4 4 0 0 1-4 4H7a4 4 0 0 1-4-4V9a1 1 0 0 1 1-1h14a4 4 0 1 1 0 8h-1"/><path d="M6 2v2"/>',
  more: '<circle cx="12" cy="12" r="1"/><circle cx="19" cy="12" r="1"/><circle cx="5" cy="12" r="1"/>',
  alert: '<path d="m21.73 18-8-14a2 2 0 0 0-3.48 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3"/><path d="M12 9v4"/><path d="M12 17h.01"/>',
  info: '<circle cx="12" cy="12" r="10"/><path d="M12 16v-4"/><path d="M12 8h.01"/>',
  circleCheck: '<circle cx="12" cy="12" r="10"/><path d="m9 12 2 2 4-4"/>',
  circleX: '<circle cx="12" cy="12" r="10"/><path d="m15 9-6 6"/><path d="m9 9 6 6"/>',
  circleDot: '<circle cx="12" cy="12" r="10"/><circle cx="12" cy="12" r="1"/>',
  ext: '<path d="M15 3h6v6"/><path d="M10 14 21 3"/><path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"/>',
  trash: '<path d="M3 6h18"/><path d="M19 6v14c0 1-1 2-2 2H7c-1 0-2-1-2-2V6"/><path d="M8 6V4c0-1 1-2 2-2h4c1 0 2 1 2 2v2"/>',
  pencil: '<path d="M21.174 6.812a1 1 0 0 0-3.986-3.987L3.842 16.174a2 2 0 0 0-.5.83l-1.321 4.352a.5.5 0 0 0 .623.622l4.353-1.32a2 2 0 0 0 .83-.497z"/><path d="m15 5 4 4"/>',
  play: '<polygon points="6 3 20 12 6 21 6 3"/>', stop: '<rect width="14" height="14" x="5" y="5" rx="2"/>',
  archive: '<rect width="20" height="5" x="2" y="3" rx="1"/><path d="M4 8v11a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8"/><path d="M10 12h4"/>',
  history: '<path d="M3 12a9 9 0 1 0 9-9 9.75 9.75 0 0 0-6.74 2.74L3 8"/><path d="M3 3v5h5"/><path d="M12 7v5l4 2"/>',
  code: '<polyline points="16 18 22 12 16 6"/><polyline points="8 6 2 12 8 18"/>',
  diff: '<path d="M15 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7Z"/><path d="M9 10h6"/><path d="M12 13V7"/><path d="M9 17h6"/>',
  tag: '<path d="M12.586 2.586A2 2 0 0 0 11.172 2H4a2 2 0 0 0-2 2v7.172a2 2 0 0 0 .586 1.414l8.704 8.704a2.426 2.426 0 0 0 3.42 0l6.58-6.58a2.426 2.426 0 0 0 0-3.42z"/><circle cx="7.5" cy="7.5" r=".5" fill="currentColor"/>',
  cloud: '<path d="M17.5 19H9a7 7 0 1 1 6.71-9h1.79a4.5 4.5 0 1 1 0 9Z"/>',
  grip: '<circle cx="9" cy="12" r="1"/><circle cx="9" cy="5" r="1"/><circle cx="9" cy="19" r="1"/><circle cx="15" cy="12" r="1"/><circle cx="15" cy="5" r="1"/><circle cx="15" cy="19" r="1"/>',
  panelLeft: '<rect width="18" height="18" x="3" y="3" rx="2"/><path d="M9 3v18"/>',
  split: '<rect width="18" height="18" x="3" y="3" rx="2"/><path d="M12 3v18"/>',
  msg: '<path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/>',
  user: '<path d="M19 21v-2a4 4 0 0 0-4-4H9a4 4 0 0 0-4 4v2"/><circle cx="12" cy="7" r="4"/>',
  clock: '<circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/>',
  undo: '<path d="M3 12a9 9 0 1 0 9-9 9.75 9.75 0 0 0-6.74 2.74L3 8"/><path d="M3 3v5h5"/>',
  lock: '<rect width="18" height="11" x="3" y="11" rx="2" ry="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/>',
  globe: '<circle cx="12" cy="12" r="10"/><path d="M12 2a14.5 14.5 0 0 0 0 20 14.5 14.5 0 0 0 0-20"/><path d="M2 12h20"/>',
  package: '<path d="M11 21.73a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73z"/><path d="M12 22V12"/><path d="m3.3 7 8.7 5 8.7-5"/>',
  sparkle: '<path d="M9.937 15.5A2 2 0 0 0 8.5 14.063l-6.135-1.582a.5.5 0 0 1 0-.962L8.5 9.936A2 2 0 0 0 9.937 8.5l1.582-6.135a.5.5 0 0 1 .963 0L14.063 8.5A2 2 0 0 0 15.5 9.937l6.135 1.581a.5.5 0 0 1 0 .964L15.5 14.063a2 2 0 0 0-1.437 1.437l-1.582 6.135a.5.5 0 0 1-.963 0z"/>',
  rocket: '<path d="M12 3v12"/><path d="m7 8 5-5 5 5"/><path d="M5 21h14"/>',
  filter: '<path d="M3 6h18"/><path d="M7 12h10"/><path d="M10 18h4"/>',
  eye: '<path d="M2.062 12.348a1 1 0 0 1 0-.696 10.75 10.75 0 0 1 19.876 0 1 1 0 0 1 0 .696 10.75 10.75 0 0 1-19.876 0"/><circle cx="12" cy="12" r="3"/>',
  /* tool marks: neutral glyphs, not vendor logos */
  tClaude: '<path d="M12 3v18"/><path d="M4.2 7.5l15.6 9"/><path d="M19.8 7.5l-15.6 9"/>',
  tCodex: '<path d="m7 8 4 4-4 4"/><path d="M13 16h4"/>',
  tOpen: '<path d="M9 5H6a1 1 0 0 0-1 1v12a1 1 0 0 0 1 1h3"/><path d="M15 5h3a1 1 0 0 1 1 1v12a1 1 0 0 1-1 1h-3"/><circle cx="12" cy="12" r="1.5"/>',
};
const BRAND = {
  github: '<path fill="currentColor" stroke="none" d="M12 .297c-6.63 0-12 5.373-12 12 0 5.303 3.438 9.8 8.205 11.385.6.113.82-.258.82-.577 0-.285-.01-1.04-.015-2.04-3.338.724-4.042-1.61-4.042-1.61C4.422 18.07 3.633 17.7 3.633 17.7c-1.087-.744.084-.729.084-.729 1.205.084 1.838 1.236 1.838 1.236 1.07 1.835 2.809 1.305 3.495.998.108-.776.417-1.305.76-1.605-2.665-.3-5.466-1.332-5.466-5.93 0-1.31.465-2.38 1.235-3.22-.135-.303-.54-1.523.105-3.176 0 0 1.005-.322 3.3 1.23.96-.267 1.98-.399 3-.405 1.02.006 2.04.138 3 .405 2.28-1.552 3.285-1.23 3.285-1.23.645 1.653.24 2.873.12 3.176.765.84 1.23 1.91 1.23 3.22 0 4.61-2.805 5.625-5.475 5.92.42.36.81 1.096.81 2.22 0 1.606-.015 2.896-.015 3.286 0 .315.21.69.825.57C20.565 22.092 24 17.592 24 12.297c0-6.627-5.373-12-12-12"/>',
  bitbucket: '<path fill="currentColor" stroke="none" d="M.778 1.213a.768.768 0 0 0-.768.892l3.263 19.81c.084.5.515.868 1.022.873H19.95a.772.772 0 0 0 .77-.646l3.27-20.03a.768.768 0 0 0-.768-.891zM14.52 15.53H9.522L8.17 8.466h7.561z"/>',
};
const I = (n, cls) => `<svg ${cls ? `class="${cls}"` : ''} viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${IC[n] || BRAND[n] || ''}</svg>`;

/* ---- data ---- */
const ACCT = {
  'acme-prod': { c: 3, m: 'AP', p: 'aws' }, globex: { c: 2, m: 'GL', p: 'aws' }, initech: { c: 1, m: 'IN', p: 'aws' },
  'alex-dev': { c: 6, m: 'AD', p: 'github' }, alex_acme: { c: 5, m: 'AA', p: 'bitbucket' },
};
const PROV = { aws: 'cloud', github: 'github', bitbucket: 'bitbucket' };
const PROVNAME = { aws: 'AWS CodeCommit', github: 'GitHub', bitbucket: 'Bitbucket' };
const prov = (p, lg) => `<span class="prov${lg ? ' prov--lg' : ''}" title="${PROVNAME[p]}">${I(PROV[p])}</span>`;
const chip = (name) => { const a = ACCT[name]; return `<span class="acct-chip" data-acct="${a.c}"><span class="acct-tile">${a.m}</span><span class="acct-chip__name">${name}</span></span>`; };
const tile = (name, size) => { const a = ACCT[name]; return `<span class="acct-tile${size ? ' acct-tile--' + size : ''}" data-acct="${a.c}">${a.m}</span>`; };
const TOOLS = { claude: ['tClaude', 'Claude Code'], codex: ['tCodex', 'Codex'], opencode: ['tOpen', 'OpenCode'] };
const tool = (t, lg) => `<span class="tool${lg ? ' tool--lg' : ''}" title="${TOOLS[t][1]}">${I(TOOLS[t][0])}</span>`;
const kbd = (...keys) => keys.map((k) => `<span class="kbd">${k}</span>`).join(' ');
const ME = 'Alex';
const ID_PROJ = 'alex@acme.example.com';
const ID_GLOBAL = 'alex@example.com';

const STATES = {
  run: ['state--run', 'dot dot--busy', L('运行中', 'Running')],
  wait: ['state--wait', 'dot dot--warn', L('等待你', 'Waiting for you')],
  done: ['state--done', 'dot dot--ok', L('已完成', 'Done')],
  fail: ['state--fail', 'dot dot--danger', L('失败', 'Failed')],
  stop: ['state--stop', 'dot dot--off', L('已停止', 'Stopped')],
};
const state = (s) => `<span class="state ${STATES[s][0]}"><span class="${STATES[s][1]}"></span>${STATES[s][2]}</span>`;

/* ---- app shell (sidebar pages) ---- */
function sidebar(active) {
  const item = (id, icon, label, count) => `<a class="nav__item${active === id ? ' is-active' : ''}" href="${href(id)}"${active === id ? ' aria-current="page"' : ''}>${I(icon, 'nav__icon')}${label}${count || ''}</a>`;
  const kr = (dot, label, time, warn, p) => `<button type="button" class="keyring__item"><span class="dot dot--${dot}"></span><span class="truncate" style="display:flex;align-items:center;gap:6px">${p ? prov(p) : ''}${label}</span><span class="keyring__time${warn ? ' keyring__time--warn' : ''}">${time}</span></button>`;
  return `<aside class="app__sidebar">
    <div class="app__titlebar">${OS === 'mac' ? '<div class="traffic" aria-hidden="true"><i></i><i></i><i></i></div>' : ''}</div>
    <div class="brand"><img class="brand__mark" src="appicon.svg" alt=""><div><div class="brand__name">Codepass</div><div class="brand__sub">${L('账号 · 仓库 · AI 编码', 'Accounts · repos · AI')}</div></div></div>
    <nav class="nav" aria-label="${L('主导航', 'Main')}">
      ${item('projects', 'layers', L('项目', 'Projects'), `<span class="nav__count nav__count--warn">${L('2 待处理', '2 to fix')}</span>`)}
      ${item('ai-tasks', 'bot', L('AI 任务', 'AI tasks'), `<span class="nav__count nav__count--warn">${L('1 等待你', '1 waiting')}</span>`)}
      ${item('repo-prs', 'pr', L('拉取请求', 'Pull requests'), `<span class="nav__count">${L('2 待评审', '2 review')}</span>`)}
      ${item('accounts', 'idcard', L('账号', 'Accounts'))}
    </nav>
    <div class="nav__section" style="padding-left:24px;margin-top:8px">${L('登录状态', 'Sign-in status')}</div>
    <div class="keyring">
      ${kr('ok', 'acme-prod', '6:12', 0, 'aws')}
      ${kr('warn', 'initech', '48m', 1, 'aws')}
      ${kr('ok', 'alex-dev', L('密钥', 'Key'), 0, 'github')}
      ${kr('ok', 'alex_acme', L('密钥', 'Key'), 0, 'bitbucket')}
    </div>
    <div class="nav__group-gap"></div>
    <nav class="nav">
      ${item('identity', 'sliders', L('Git 设置', 'Git settings'))}
      ${item('health', 'shield', L('安全与环境', 'Health'))}
      ${item('tutorial', 'book', L('教程', 'Tutorial'))}
    </nav>
    <div class="sidebar-foot">
      <a class="awake" href="${href('ai-tasks')}" style="text-decoration:none" title="${L('有 AI 任务在运行，这台 Mac 不会自动睡眠', 'AI tasks are running, so this Mac will not sleep on its own')}">${I('coffee')}<b>${L('保持唤醒中', 'Keeping awake')}</b><span class="awake__n">${L('2 个任务', '2 tasks')}</span></a>
      <nav class="nav">${item('settings', 'settings', L('设置', 'Settings'), SCREEN === 'update' ? '<span class="nav__dot" title="update"></span>' : '')}</nav>
    </div>
  </aside>`;
}
const shell = (active, body, overlay) => `<div class="app">${sidebar(active)}<main class="app__main"><div class="page page-enter">${body}</div></main></div>${overlay || ''}`;

/* ---- workspace frame ---- */
const REPO_API = { proj: 'acme', name: 'acme_api', p: 'aws', acct: 'acme-prod', branch: 'main', up: 2, down: 0, changes: 5, prs: 0, ai: '<span class="tab__n tab__n--warn">1</span>' };
const REPO_APP = { proj: 'globex', name: 'globex_app', p: 'github', acct: 'alex-dev', branch: 'feat/phone-verify', up: 0, down: 0, changes: 0, prs: 4, ai: '' };
const wsTabs = (r) => [
  ['workspace-code', 'code', L('代码', 'Code')],
  ['repo-commit', 'diff', L('改动', 'Changes'), r.changes ? `<span class="tab__n">${r.changes}</span>` : ''],
  ['repo-history', 'history', L('历史', 'History')],
  ['repo-branches', 'branch', L('分支', 'Branches')],
  ['repo-prs', 'pr', L('拉取请求', 'Pull requests'), r.prs ? `<span class="tab__n">${r.prs}</span>` : ''],
  ['workspace-ai', 'bot', L('AI 任务', 'AI tasks'), r.ai],
];
/* opts: { tab, task (bool: a task worktree is selected), status (html), overlay, branch } */
function ws(opts, body) {
  const task = !!opts.task;
  const R = opts.repo || REPO_API;
  const WS_TABS = wsTabs(R);
  const railOn = opts.rail || 'projects';
  const sw = task
    ? `<button class="tree-switch is-task" title="${L('切换工作区', 'Switch working tree')}">${I('bot')}${L('AI 任务', 'AI task')} · <b>ai/fix-settlement-rounding</b>${I('chevD')}</button>`
    : `<button class="tree-switch" title="${L('切换工作区', 'Switch working tree')}">${I('branch')}${L('主工作区', 'Main workspace')} · <b>${opts.branch || R.branch}</b>${I('chevD')}</button>`;
  const rail = (id, icon, title, pip) => `<a class="rail__btn${id === railOn ? ' is-active' : ''}" href="${href(id)}" title="${title}">${I(icon)}${pip || ''}</a>`;
  return `<div class="ws">
    <header class="ws__head${task ? ' is-task' : ''}">
      ${OS === 'mac' ? '<div class="traffic" aria-hidden="true"><i></i><i></i><i></i></div>' : ''}
      <a class="icon-btn icon-btn--sm" href="${href('projects')}" title="${L('返回项目', 'Back to projects')} (${mod}1)">${I('panelLeft')}</a>
      <div class="ws__crumb"><span>${R.proj}</span>${I('chevR')}${prov(R.p)}<b>${R.name}</b></div>
      ${sw}
      <span class="spacer"></span>
      <span class="splitbtn"><button class="btn btn--sm"${task ? ` title="${L('这个任务的分支还没有推送过', 'This task’s branch has not been pushed yet')}"` : ''}>${task ? `${I('up')}${L('发布分支', 'Publish branch')}` : `${I('refresh')}<span class="num">↓${R.down}</span> <span class="num">↑${R.up}</span> ${R.up ? L('推送', 'Push') : L('同步', 'Sync')}`}</button><button class="btn btn--sm" title="${L('更多同步操作', 'More sync actions')}">${I('chevD')}</button></span>
      ${chip(R.acct)}
      <button class="icon-btn icon-btn--sm" title="${L('仓库设置', 'Repository settings')}">${I('settings')}</button>
    </header>
    <nav class="ws__rail" aria-label="${L('主导航', 'Main')}">
      ${rail('projects', 'layers', L('项目', 'Projects'))}
      ${rail('ai-tasks', 'bot', L('AI 任务', 'AI tasks'), '<span class="pip"></span>')}
      ${rail('repo-prs', 'pr', L('拉取请求', 'Pull requests'))}
      ${rail('accounts', 'idcard', L('账号', 'Accounts'))}
      <span class="rail__sep"></span>
      ${rail('identity', 'sliders', L('Git 设置', 'Git settings'))}
      ${rail('health', 'shield', L('安全与环境', 'Health'))}
      <span class="rail__spacer"></span>
      <a class="rail__btn rail__awake" href="${href('ai-tasks')}" title="${L('保持唤醒中 · 2 个任务', 'Keeping awake · 2 tasks')}">${I('coffee')}</a>
      ${rail('settings', 'settings', L('设置', 'Settings'))}
    </nav>
    <div class="ws__main">
      <div class="ws__tabs" role="tablist">
        ${WS_TABS.map(([id, ic, label, n]) => `<a class="tab${opts.tab === id ? ' is-active' : ''}" role="tab" aria-selected="${opts.tab === id}" href="${href(id)}">${I(ic)}${label}${n || ''}</a>`).join('')}
        <span class="ws__quick">${I('search')}${L('搜索文件、符号、命令', 'Search files and symbols')}<span class="keys">${kbd(mod, 'K')}</span></span>
      </div>
      <div class="ws__body" style="${opts.grid || ''}">${body}</div>
    </div>
    <footer class="ws__status">
      <span class="st">${I('terminal')}${L('终端', 'Terminal')} <span class="kbd" style="height:16px;font-size:10px">⌃\`</span></span>
      ${opts.status || ''}
      <span class="spacer"></span>
      ${opts.statusRight || ''}
      <span class="st st--ok">${I('coffee')}${L('保持唤醒 · 2 个任务', 'Keeping awake · 2 tasks')}</span>
    </footer>
  </div>${opts.overlay || ''}`;
}

/* ---- code rendering ---- */
const esc = (s) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
const KW = {
  go: /\b(package|import|func|return|if|else|for|range|var|const|type|struct|interface|nil|defer|go|map|chan|switch|case|default|break|continue)\b/g,
  ts: /\b(import|from|export|interface|type|const|let|async|await|function|return|if|else|new|class|extends|implements|null|undefined|true|false|as)\b/g,
};
const TY = {
  go: /\b(string|int64|int|bool|error|float64|byte|any|context|time|Service|Report|Mismatch|Money|Rate|Context|Time|Entry|Currency|Testing|T)\b/g,
  ts: /\b(string|number|boolean|void|Promise|Report|Mismatch|ReconcileQuery|Money)\b/g,
};
/* tiny highlighter: comments and strings are protected first, then keywords / types / calls / numbers */
function hl(line, lang) {
  const keep = [];
  const stash = (cls) => (m) => { keep.push(`<span class="${cls}">${m}</span>`); return String.fromCharCode(0xE000 + keep.length - 1); };
  let s = esc(line);
  s = s.replace(/\/\/.*$/, stash('c'));
  s = s.replace(/"(?:[^"\\]|\\.)*"|`[^`]*`/g, stash('s'));
  s = s.replace(KW[lang], stash('k'));
  s = s.replace(TY[lang], stash('t'));
  s = s.replace(/\b([A-Za-z_][A-Za-z0-9_]*)(?=\()/g, stash('f'));
  s = s.replace(/\b\d[\d_]*(?:\.\d+)?\b/g, stash('n'));
  return s.replace(/[-]/g, (ch) => keep[ch.charCodeAt(0) - 0xE000]);
}
/* lines: array of strings or [text, {bar:'m'|'a', cur:true, cls:'...', raw:true}] */
function codeLines(lines, lang, start) {
  let n = start || 1;
  return lines.map((ln) => {
    const [text, o] = Array.isArray(ln) ? ln : [ln, {}];
    if (o.html) return o.html;
    const body = o.raw ? text : hl(text, lang);
    return `<div class="ln${o.cur ? ' is-cur' : ''}${o.cls ? ' ' + o.cls : ''}"><i class="${o.bar || ''}"></i><span>${o.no === '' ? '' : n++}</span><span>${body}</span></div>`;
  }).join('');
}
/* diff rows: [kind(' '|'+'|'-'), oldNo, newNo, text] */
const dl = (rows, lang) => rows.map(([k, a, b, t, staged]) => `<div class="dl${k === '+' ? ' dl--add' : k === '-' ? ' dl--del' : ''}${staged ? ' is-staged' : ''}"><span>${a || ''}</span><span>${b || ''}</span><span>${k === ' ' ? '' : k}</span><span>${hl(t, lang || 'go')}</span></div>`).join('');

const SCREENS = {};
