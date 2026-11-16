/* Workspace screens: code, changes (task review), commit, AI tasks, new task */

/* ---------------------------------------------------------------- Code */
SCREENS['workspace-code'] = () => {
  const d = (n) => `style="--d:${n}"`;
  const dir = (n, name, open) => `<li ${d(n)}>${I(open ? 'chevD' : 'chevR', 'chev')}${I(open ? 'folderOpen' : 'folder')}<span class="nm">${name}</span></li>`;
  const f = (n, name, st, active, cls) => `<li ${d(n)} class="${active ? 'is-active' : ''} ${cls || ''}"><span style="width:10px"></span>${I('file')}<span class="nm ${st ? 'g-' + st.toLowerCase() : ''}">${name}</span>${st ? `<span class="st g-${st.toLowerCase()}">${st}</span>` : ''}</li>`;
  const tree = `<ul class="tree" role="tree">
    ${dir(0, 'cmd')}${dir(0, 'internal', 1)}${dir(1, 'ledger')}${dir(1, 'payout')}${dir(1, 'settlement', 1)}
    ${f(2, 'money.go', 'M')}${f(2, 'money_test.go', 'A')}${f(2, 'reconcile.go', 'M', 1)}${f(2, 'reconcile_test.go')}${f(2, 'service.go')}
    ${dir(1, 'webhook')}${dir(0, 'migrations')}${dir(0, 'web/admin/src/api', 1)}
    ${f(1, 'http.ts')}${f(1, 'settlement.ts', 'M')}${f(1, 'types.ts')}
    ${f(0, '.worktreeinclude')}${f(0, 'Dockerfile')}${f(0, 'Makefile')}${f(0, 'go.mod')}${f(0, 'README.md')}
  </ul>`;
  const ol = (n, kind, name, active, extra) => `<li ${d(n)} class="${active ? 'is-active' : ''}"><span class="kind">${kind}</span>${name}${extra ? `<span class="muted" style="margin-left:4px">${extra}</span>` : ''}</li>`;
  const outline = `<ul class="outline">${ol(0, 'S', 'Service')}${ol(1, 'f', 'Reconcile', 1, '(ctx, merchantID, day)')}${ol(1, 'f', 'loadRate', 0, '(ctx)')}${ol(0, 'S', 'Report')}${ol(0, 'S', 'Mismatch')}${ol(0, 'f', 'diffEntries', 0, '(a, b)')}</ul>`;

  const peek = `<div class="peek">
      <div class="peek__head">${I('file')}<b>money.go</b><span>internal/settlement</span><span class="muted">· ${L('CalcFee 的定义', 'definition of CalcFee')}</span><span class="spacer"></span><span>${L('打开文件', 'Open file')} ${kbd('⏎')}</span><span>${L('关闭', 'Close')} ${kbd('Esc')}</span></div>
      ${codeLines([
        '// CalcFee returns the fee for amount at rate, in minor units.',
        ['func CalcFee(amount Money, rate Rate) Money {', { cls: 'is-def' }],
        '	fee := amount.Minor * rate.BasisPoints',
        '	q, r := fee/10_000, fee%10_000',
        '	if r*2 >= 10_000 { // round half up',
        '		q++',
      ], 'go', 18)}
    </div>`;
  const goLines = codeLines([
    '	"acme_api/internal/ledger"', ')', '',
    '// Reconcile compares the gateway report with the ledger for one',
    '// merchant and day, and returns the entries that do not match.',
    'func (s *Service) Reconcile(',
    '	ctx context.Context, merchantID string, day time.Time,',
    ') (*Report, error) {',
    ['	entries, err := s.ledger.<span class="sym f">ListByDay</span>(ctx, merchantID, day)', { raw: true }],
    '	if err != nil {',
    '		return nil, fmt.Errorf("list ledger: %w", err)',
    '	}', '',
    '	rate, err := s.loadRate(ctx, merchantID)',
    '	if err != nil {',
    '		return nil, err',
    '	}',
    '	report := &Report{MerchantID: merchantID, Day: day}',
    '	for _, e := range entries {',
    ['		fee := <span class="sym f">CalcFee</span>(e.Amount, rate)<span class="cursor"></span>', { raw: true, cur: true, bar: 'm' }],
    ['', { html: peek }],
    ['		net := e.Amount.Sub(fee)', { bar: 'm' }],
    '		if !net.Equal(e.Settled) {',
    '			report.Mismatches = append(report.Mismatches, Mismatch{',
    '				EntryID: e.ID, Expected: net, Actual: e.Settled,',
    '			})',
    '		}',
    ['		report.Total = report.Total.Add(net)', { bar: 'a' }],
    '	}',
    '	return report, nil',
    '}',
  ], 'go', 8);
  const hover = `<div class="hover-card pop-enter"  style="left:196px;top:${6 + 9 * 20 + 2}px;width:436px" role="tooltip">
      <div class="hover-card__sig">${hl('func (r *Repo) ListByDay(ctx context.Context,', 'go')}
${hl('    merchantID string, day time.Time) ([]Entry, error)', 'go')}</div>
      <div class="hover-card__doc">${L('ListByDay 返回某个商户在指定日期的账本记录，按入账时间排序。', 'ListByDay returns the ledger entries of one merchant for the given day, ordered by posting time.')} <span class="muted">internal/ledger/repo.go</span></div>
      <div class="hover-card__foot"><span>${kbd(mod)} ${L('点击 跳到定义', 'click to go to definition')}</span><span>${kbd('⇧', 'F12')} ${L('12 处引用', '12 references')}</span></div>
    </div>`;

  const tsLines = codeLines([
    'import { http } from "./http";',
    'import type { Report, Mismatch } from "./types";', '',
    'export interface ReconcileQuery {',
    '  merchantId: string;',
    '  day: string; // YYYY-MM-DD',
    '}', '',
    'export async function fetchReport(q: ReconcileQuery) {',
    '  const res = await http.get<Report>("/settlement/report", {',
    '    params: q,',
    '  });',
    '  return res.data;',
    '}', '',
    ['export function totalMismatch(report: Report): number {', { bar: 'm' }],
    ['  return report.mismatches', { bar: 'm' }],
    ['    .map((m: Mismatch) => m.expected - m.actual)', { bar: 'm' }],
    ['    .reduce((a, b) => a + b, 0);', { bar: 'm' }],
    '}', '',
    ['export async function exportCsv(q: ReconcileQuery) {', { bar: 'a' }],
    ['  const report = await fetchReport(q);', { bar: 'a' }],
    ['  <span class="k">return</span> report.<span class="squig">mi</span><span class="cursor"></span>', { raw: true, cur: true, bar: 'a' }],
    ['}', { bar: 'a' }],
  ], 'ts', 1);
  const acItem = (on, kind, name, ty) => `<li class="${on ? 'is-on' : ''}"><span class="kind">${kind}</span><span><b>mi</b>${name}</span><span class="ty">${ty}</span></li>`;
  const ac = `<div class="ac pop-enter" style="left:64px;top:${6 + 24 * 20 + 2}px;grid-template-columns:212px 184px" role="listbox">
      <ul class="ac__list">${acItem(1, 'p', 'smatches', 'Mismatch[]')}${acItem(0, 'p', 'ssingDays', 'string[]')}${acItem(0, 'p', 'nSettled', 'number')}${acItem(0, 'm', 'nBy()', 'Mismatch')}<li style="color:var(--text-3);font:11px/1 var(--font-sans);height:22px">${kbd('⏎')}&nbsp;${L('插入', 'insert')}&nbsp;&nbsp;${kbd('⇥')}&nbsp;${L('替换', 'replace')}</li></ul>
      <div class="ac__doc"><code>(property) Report.mismatches: Mismatch[]</code>${L('与账本对不上的记录。', 'Entries that do not match the ledger.')}</div>
    </div>`;

  const find = PANEL === 'find' ? `<div class="bpanel">
      <div class="bpanel__tabs"><span>${L('终端', 'Terminal')}</span><span>${L('问题', 'Problems')} 1</span><span class="is-active">${L('查找结果', 'Find results')}</span><span class="spacer"></span><span class="search" style="width:260px"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">${IC.search}</svg><input class="input" style="height:24px;font-size:12px" value="CalcFee"></span><span class="muted">${L('3 个文件中有 7 处', '7 results in 3 files')}</span><button class="icon-btn icon-btn--sm">${I('x')}</button></div>
      <ul class="found">
        <li class="file">${I('chevD')}money.go <span>internal/settlement · 2</span></li>
        <li><span class="no">18</span><span>// <mark>CalcFee</mark> returns the fee for amount at rate, in minor units.</span></li>
        <li><span class="no">19</span><span>func <mark>CalcFee</mark>(amount Money, rate Rate) Money {</span></li>
        <li class="file">${I('chevD')}reconcile.go <span>internal/settlement · 1</span></li>
        <li><span class="no">27</span><span>		fee := <mark>CalcFee</mark>(e.Amount, rate)</span></li>
        <li class="file">${I('chevD')}money_test.go <span>internal/settlement · 4</span></li>
      </ul></div>` : '';
  const edTab = (name, on, dirty, st) => `<span class="ed__tab${on ? ' is-active' : ''}">${I('file')}<span class="${st ? 'g-' + st : ''}">${name}</span>${dirty ? '<span class="dirty"></span>' : I('x', 'x')}</span>`;
  const body = `
    <div class="pane pane--side" style="display:grid;grid-template-rows:auto auto minmax(0,1fr) auto auto;border-right:1px solid var(--border)">
      <div class="pane__head">${L('文件', 'Files')}<span class="spacer"></span><button class="icon-btn icon-btn--sm" title="${L('新建文件', 'New file')}">${I('plus')}</button><button class="icon-btn icon-btn--sm" title="${L('只看有改动的文件', 'Show changed files only')}">${I('filter')}</button></div>
      <div class="pane__filter"><div class="search">${I('search')}<input class="input" placeholder="${L('筛选文件', 'Filter files')}"></div></div>
      <div class="pane__scroll">${tree}</div>
      <div class="pane__head" style="border-top:1px solid var(--border-subtle)">${I('chevD')}${L('大纲', 'Outline')}<span class="count">reconcile.go</span></div>
      ${outline}
    </div>
    <div style="display:grid;grid-template-rows:minmax(0,1fr)${find ? ' 196px' : ''};min-width:0;min-height:0">
      <div style="display:grid;grid-template-columns:minmax(0,1.28fr) minmax(0,1fr);min-height:0">
        <div class="ed">
          <div class="ed__tabs">${edTab('reconcile.go', 1, 0, 'm')}${edTab('service.go')}${edTab('money.go', 0, 0, 'm')}<span class="spacer"></span><button class="icon-btn icon-btn--sm" title="${L('拆分编辑器', 'Split editor')}">${I('split')}</button></div>
          <div class="ed__crumbs">internal${I('chevR')}settlement${I('chevR')}<b>reconcile.go</b>${I('chevR')}Service${I('chevR')}<b>Reconcile</b></div>
          <div class="ed__code">${goLines}${hover}</div>
        </div>
        <div class="ed">
          <div class="ed__tabs">${edTab('settlement.ts', 1, 1, 'm')}${edTab('types.ts')}<span class="spacer"></span></div>
          <div class="ed__crumbs">web${I('chevR')}admin${I('chevR')}src${I('chevR')}api${I('chevR')}<b>settlement.ts</b>${I('chevR')}<b>exportCsv</b></div>
          <div class="ed__code">${tsLines}${find ? '' : ac}</div>
        </div>
      </div>
      ${find}
    </div>`;
  return ws({ tab: 'workspace-code', grid: 'grid-template-columns:248px minmax(0,1fr)',
    status: `<span class="st st--danger">${I('circleX')}1</span><span class="st">${I('alert')}0</span><span class="st">${I('search')}${L('在文件中查找', 'Find in files')} <span class="kbd" style="height:16px;font-size:10px">⇧${mod}F</span></span>`,
    statusRight: `<span class="st st--ok">${I('circleCheck')}gopls</span><span class="st st--ok">${I('circleCheck')}tsserver</span><span class="st">${L('行 27，列 32', 'Ln 27, Col 32')}</span><span class="st">UTF-8</span><span class="st">Go</span>` }, body);
};

/* ---------------------------------------------------------------- Changes: reviewing an AI task */
const viewSeg = () => `<div class="segmented" style="transform:scale(.92);transform-origin:right"><button class="segmented__item is-active">${L('统一', 'Unified')}</button><button class="segmented__item">${L('并排', 'Side by side')}</button></div>`;
SCREENS['workspace-changes'] = () => {
  const fi = (name, dir, add, del, st, on) => `<li class="${on ? 'is-active' : ''}">${I('file')}<span class="nm">${name}</span><span class="dir">${dir}</span><span class="stat">${add ? `<span class="plus">+${add}</span>` : ''}${del ? `<span class="minus">−${del}</span>` : ''}</span><span class="st g-${st.toLowerCase()}">${st}</span></li>`;
  const body = `
    <div class="taskbar" style="grid-column:1/-1">${tool('claude')}<span>${L('正在查看任务', 'Reviewing task')} <b>${TASKS[0].title}</b></span><span class="mono muted">ai/fix-settlement-rounding → main</span>${state('wait')}<span class="spacer"></span>
      <a class="btn btn--sm btn--ghost" href="${href('workspace-ai')}">${I('terminal')}${L('回到任务终端', 'Back to its terminal')}</a><button class="btn btn--sm btn--danger-ghost">${L('丢弃任务', 'Discard task')}</button><button class="btn btn--sm">${I('pr')}${L('推送分支', 'Push branch')}</button><button class="btn btn--sm btn--primary">${I('merge')}${L('合并到 main', 'Merge into main')}</button></div>
    <div class="pane pane--side" style="border-right:1px solid var(--border)">
      <div class="pane__head">${L('相对 main 的改动', 'Changes against main')}<span class="count">4 · <span class="plus">+86</span> <span class="minus">−31</span></span><span class="spacer"></span><button class="icon-btn icon-btn--sm" title="${L('筛选', 'Filter')}">${I('filter')}</button></div>
      <div class="pane__scroll">
        <ul class="fl">${fi('money.go', 'internal/settlement', 9, 5, 'M', 1)}${fi('money_test.go', 'internal/settlement', 52, 0, 'A')}${fi('reconcile.go', 'internal/settlement', 12, 21, 'M')}${fi('settlement.ts', 'web/admin/src/api', 13, 5, 'M')}</ul>
        <div class="fl__group">${I('chevD')}${L('任务里的提交', 'Commits in this task')} <span style="font-weight:400">2</span></div>
        <ul class="mini-commits" style="font-size:12.5px">
          <li><span class="who">A</span><span class="m">settlement: integer fee math</span><span class="mono">e19b4a7</span></li>
          <li><span class="who">A</span><span class="m">settlement: table tests for CalcFee</span><span class="mono">c3f08d1</span></li>
        </ul>
        <div class="fl__group">${I('chevD')}${L('还没提交', 'Not committed yet')} <span style="font-weight:400">1</span></div>
        <ul class="fl">${fi('settlement.ts', 'web/admin/src/api', 13, 5, 'M')}</ul>
      </div>
      <div class="composer" style="gap:6px"><div class="composer__who">${I('info')}<span>${L('合并时，还没提交的改动会先提交。提交用', 'Merging commits what is not committed yet, as')} <b>${ME} &lt;${ID_PROJ}&gt;</b></span></div></div>
    </div>
    <div class="pane">
      <div class="diff">
        <div class="diff__file">${I('file')}money.go <span>internal/settlement</span><span class="plus mono" style="font-size:11px">+9</span><span class="minus mono" style="font-size:11px">−5</span><span class="spacer"></span>${viewSeg()}<a class="btn btn--sm btn--ghost" href="${href('workspace-code')}">${I('code')}${L('在编辑器中打开', 'Open in editor')}</a></div>
        <div class="hunk">@@ -1,9 +1,5 @@</div>
        ${dl([[' ', 1, 1, 'package settlement'], [' ', 2, 2, ''], ['-', 3, '', 'import "math"'], ['-', 4, '', ''], [' ', 5, 3, '// Money is an amount in minor units (cents, sen).'], [' ', 6, 4, 'type Money struct {'], [' ', 7, 5, '	Minor    int64']])}
        <div class="hunk">@@ -14,10 +10,15 @@ type Rate struct {</div>
        ${dl([['-', 15, '', '	Percent float64'], ['+', '', 11, '	BasisPoints int64 // 1 bp = 0.01%'], [' ', 16, 12, '}'], [' ', 17, 13, ''],
          [' ', 18, 14, '// CalcFee returns the fee for amount at rate, in minor units.'], [' ', 19, 15, 'func CalcFee(amount Money, rate Rate) Money {'],
          ['-', 20, '', '	fee := float64(amount.Minor) * rate.Percent / 100'], ['-', 21, '', '	return Money{Minor: int64(math.Round(fee)), Currency: amount.Currency}'],
          ['+', '', 16, '	// Integer math only: a float here put 1 sen of drift into'], ['+', '', 17, '	// a settlement run of two million entries.'],
          ['+', '', 18, '	fee := amount.Minor * rate.BasisPoints'], ['+', '', 19, '	q, r := fee/10_000, fee%10_000'], ['+', '', 20, '	if r*2 >= 10_000 { // round half up'], ['+', '', 21, '		q++'], ['+', '', 22, '	}'],
          ['+', '', 23, '	return Money{Minor: q, Currency: amount.Currency}'], [' ', 22, 24, '}']])}
        <div class="diff__gap"></div>
        <div class="diff__file" style="border-top:1px solid var(--border-subtle)">${I('file')}money_test.go <span>internal/settlement</span><span class="badge badge--ok" style="height:18px;font-size:11px">${L('新文件', 'New file')}</span><span class="plus mono" style="font-size:11px">+52</span></div>
        <div class="hunk">@@ -0,0 +1,52 @@</div>
        ${dl([['+', '', 1, 'package settlement'], ['+', '', 2, ''], ['+', '', 3, 'import "testing"'], ['+', '', 4, ''], ['+', '', 5, 'func TestCalcFee(t *testing.T) {'], ['+', '', 6, '	cases := []struct {'], ['+', '', 7, '		name   string'], ['+', '', 8, '		minor  int64'], ['+', '', 9, '		bp     int64'], ['+', '', 10, '		want   int64']])}
      </div>
    </div>`;
  return ws({ tab: 'repo-commit', task: true, grid: 'grid-template-columns:320px minmax(0,1fr);grid-template-rows:auto minmax(0,1fr)',
    status: `<span class="st">${I('folder')}<span class="mono">~/.codepass/worktrees/acme_api/fix-settlement-rounding</span></span>` }, body);
};

/* ---------------------------------------------------------------- Changes: commit composer (main workspace) */
function changesLeft(mode) {
  const fi = (name, dir, st, on, checked) => `<li class="${on ? 'is-active' : ''}"><input type="checkbox" class="checkbox" ${checked ? 'checked' : ''} aria-label="${L('暂存', 'Stage')} ${name}"><span class="nm">${name}</span><span class="dir">${dir}</span><span class="st g-${st === 'U' ? 'a' : st.toLowerCase()}" style="margin-left:auto">${st}</span></li>`;
  const stash = mode === 'stash';
  const sItem = (ref, msg, meta, on) => `<div class="task${on ? ' is-active' : ''}" style="grid-template-columns:18px minmax(0,1fr)"><span style="color:var(--text-3);margin-top:2px">${I('archive')}</span><span class="task__title">${msg}</span><span class="task__meta"><span class="mono">${ref}</span><span>${meta}</span></span></div>`;
  return `<div class="pane pane--side" style="border-right:1px solid var(--border)">
      <div class="pane__scroll">
        <div class="fl__group">${I(stash ? 'chevR' : 'chevD')}${L('已暂存', 'Staged')} <span style="font-weight:400">2</span><span class="spacer"></span>${stash ? '' : `<button class="btn btn--ghost btn--sm">${L('全部取消', 'Unstage all')}</button>`}</div>
        ${stash ? '' : `<ul class="fl">${fi('retry.go', 'internal/webhook', 'M', 0, 1)}${fi('retry_test.go', 'internal/webhook', 'A', 0, 1)}</ul>`}
        <div class="fl__group">${I(stash ? 'chevR' : 'chevD')}${L('未暂存', 'Not staged')} <span style="font-weight:400">3</span><span class="spacer"></span>${stash ? '' : `<button class="btn btn--ghost btn--sm">${L('全部暂存', 'Stage all')}</button>`}</div>
        ${stash ? '' : `<ul class="fl">${fi('batch.go', 'internal/payout', 'M', 1)}${fi('Makefile', '', 'M')}${fi('webhook.md', 'docs', 'U')}</ul>`}
        <div class="fl__group">${I(stash ? 'chevD' : 'chevR')}${stash ? L('贮藏', 'Stashes') : `<a href="${href('repo-stash')}" style="color:inherit">${L('贮藏', 'Stashes')}</a>`} <span style="font-weight:400">2</span><span class="spacer"></span><button class="btn btn--ghost btn--sm">${I('archive')}${L('贮藏当前改动', 'Stash changes')}</button></div>
        ${stash ? sItem('stash@{0}', L('WIP：payout 指标埋点', 'WIP: payout metrics'), L('在 main 上 · 昨天 18:40 · 3 个文件', 'on main · yesterday 18:40 · 3 files'), 1) + sItem('stash@{1}', L('试验：批量大小改成 500', 'Experiment: batch size 500'), L('在 feat/payout-batch 上 · 3 天前 · 1 个文件', 'on feat/payout-batch · 3 days ago · 1 file')) : ''}
      </div>
      <div class="composer">
        <textarea class="textarea" placeholder="${L('提交说明', 'Commit message')}">${stash ? '' : 'webhook: retry with idempotency key\n\n' + L('每次重试带同一个 key，接收方会丢弃重复的请求，不再重复入账。', 'Every attempt carries the same key, so the receiver drops duplicates instead of posting twice.')}</textarea>
        <div class="composer__who">${I('user')}<b class="truncate">${ME} &lt;${ID_PROJ}&gt;</b><span class="src-tag src-tag--proj">${L('来自项目 acme', 'From project acme')}</span></div>
        <div class="composer__row"><label><input type="checkbox" class="checkbox">${L('修订上一次提交', 'Amend')}</label><span class="spacer"></span><span class="splitbtn"><button class="btn btn--primary"${stash ? ' disabled' : ''}>${L('提交 2 个文件', 'Commit 2 files')} <span class="kbd" style="background:transparent;border-color:color-mix(in srgb,var(--on-ink) 35%,transparent);color:var(--on-ink);height:18px">${mod}⏎</span></button><button class="btn btn--primary"${stash ? ' disabled' : ''} title="${L('提交并推送', 'Commit and push')}" style="border-left:1px solid color-mix(in srgb,var(--on-ink) 25%,transparent)">${I('chevD')}</button></span></div>
      </div>
    </div>`;
}
SCREENS['repo-commit'] = () => {
  const hb = `<span class="spacer"></span><button class="btn btn--sm">${L('暂存这一块', 'Stage hunk')}</button><button class="btn btn--sm btn--ghost">${L('丢弃这一块', 'Discard hunk')}</button>`;
  const body = `${changesLeft('commit')}
    <div class="pane">
      <div class="diff">
        <div class="diff__file">${I('file')}batch.go <span>internal/payout</span><span class="plus mono" style="font-size:11px">+7</span><span class="minus mono" style="font-size:11px">−1</span><span class="spacer"></span>${viewSeg()}<button class="btn btn--sm">${I('plus')}${L('暂存文件', 'Stage file')}</button><button class="btn btn--sm btn--ghost">${I('undo')}${L('丢弃改动', 'Discard changes')}</button><button class="icon-btn icon-btn--sm" title="${L('文件历史和逐行追溯', 'File history and blame')}">${I('history')}</button></div>
        <div class="hunk">@@ -41,7 +41,9 @@ func (b *Batcher) Flush(ctx context.Context) error {${hb}</div>
        ${dl([[' ', 41, 41, '	if len(b.pending) == 0 {'], [' ', 42, 42, '		return nil'], [' ', 43, 43, '	}'], ['-', 44, '', '	batch := b.pending'],
          ['+', '', 44, '	// Copy before releasing the lock so a concurrent Add cannot'], ['+', '', 45, '	// append to the slice we are about to send.'], ['+', '', 46, '	batch := append([]Payout(nil), b.pending...)'],
          [' ', 45, 47, '	b.pending = b.pending[:0]'], [' ', 46, 48, '	b.mu.Unlock()'], [' ', 47, 49, '']])}
        <div class="hunk">@@ -63,6 +65,10 @@ func (b *Batcher) send(ctx context.Context, batch []Payout) error {${hb}</div>
        ${dl([[' ', 63, 65, '	resp, err := b.client.CreateBatch(ctx, batch)'], [' ', 64, 66, '	if err != nil {'],
          ['+', '', 67, '		if errors.Is(err, context.DeadlineExceeded) {'], ['+', '', 68, '			b.metrics.Timeouts.Inc()'], ['+', '', 69, '			return fmt.Errorf("payout batch timed out: %w", err)'], ['+', '', 70, '		}'],
          [' ', 65, 71, '		return fmt.Errorf("create batch: %w", err)'], [' ', 66, 72, '	}'], [' ', 67, 73, '	b.metrics.Sent.Add(float64(len(resp.Items)))'], [' ', 68, 74, '	return nil'], [' ', 69, 75, '}']])}
        <div style="padding:14px;font:12px/1.5 var(--font-sans);color:var(--text-3)">${L('拖选几行后可以只暂存选中的行。', 'Drag across lines to stage only the lines you select.')}</div>
      </div>
    </div>`;
  return ws({ tab: 'repo-commit', grid: 'grid-template-columns:340px minmax(0,1fr)',
    status: `<span class="st">${I('branch')}main</span><span class="st">${L('2 个已暂存 · 3 个未暂存', '2 staged · 3 not staged')}</span>` }, body);
};

/* ---------------------------------------------------------------- AI tasks in a repo */
function aiBody() {
  const mine = TASKS.filter((t) => t.repo === 'acme_api');
  const item = (t, on) => `<a class="task${on ? ' is-active' : ''}" href="${href('workspace-ai')}" style="color:inherit">${tool(t.tool)}<span class="task__title">${t.title}</span><span class="num muted" style="font-size:11.5px">${t.time}</span>
      <span class="task__meta">${state(t.st)}<span class="mono">${t.where === 'wt' ? t.branch : L('主工作区', 'main workspace')}</span>${t.files ? `<span>· ${L(`${t.files} 个文件`, `${t.files} files`)}</span>` : ''}</span></a>`;
  const Z = zh;
  const term = `<span class="box"><span class="v">✻</span> <span class="w">Claude Code</span>
  <span class="d">cwd: ~/.codepass/worktrees/acme_api/fix-settlement-rounding</span></span>
<span class="d">&gt;</span> ${Z ? '结算对账有 1 分钱的误差。把手续费改成用最小货币单位的整数来算，并补上测试。' : 'Settlement reconcile is off by 1 sen. Compute the fee with integers in minor units and add tests.'}

<span class="g">⏺</span> ${Z ? '我先看看手续费是怎么算的。' : 'I’ll start by reading how the fee is computed.'}

<span class="g">⏺</span> <span class="w">Read</span>(internal/settlement/money.go)
  <span class="d">⎿  Read 41 lines</span>

<span class="g">⏺</span> ${Z ? 'CalcFee 用 float64 乘百分比再四舍五入。金额一大，浮点误差就会进到结果里。' : 'CalcFee multiplies a float64 by a percentage and rounds. With large amounts the float error reaches the result.'}
  ${Z ? '我改成基点（bp）加整数除法，四舍五入规则保持「五入」。' : 'I’ll switch to basis points with integer division and keep round-half-up.'}

<span class="g">⏺</span> <span class="w">Update</span>(internal/settlement/money.go)
  <span class="d">⎿  Updated with 9 additions and 5 removals</span>
<span class="del">     20 -   fee := float64(amount.Minor) * rate.Percent / 100</span><span class="add">     18 +   fee := amount.Minor * rate.BasisPoints</span><span class="add">     19 +   q, r := fee/10_000, fee%10_000</span>
<span class="g">⏺</span> <span class="w">Write</span>(internal/settlement/money_test.go)
  <span class="d">⎿  Wrote 52 lines</span>

<span class="g">⏺</span> <span class="w">Bash</span>(go test ./internal/settlement/...)
  <span class="d">⎿  </span><span class="g">ok</span><span class="d">   acme_api/internal/settlement   0.412s</span>
<span class="box box--ask"><span class="y">${Z ? '需要你的许可' : 'Permission needed'}</span>   <span class="w">Bash</span>  git commit -am "settlement: integer fee math"

<span class="sel"><span class="b">❯ 1. Yes</span></span>
  2. Yes, and don’t ask again for git commit in this task
  3. No, and tell Claude what to do differently <span class="d">(esc)</span></span>`;
  return `
    <div class="pane pane--side" style="border-right:1px solid var(--border)">
      <div class="pane__head">${L('这个仓库的任务', 'Tasks in this repository')}<span class="spacer"></span><a class="btn btn--primary btn--sm" href="${href('new-task')}">${I('plus')}${L('新任务', 'New task')} <span class="kbd" style="background:transparent;border-color:color-mix(in srgb,var(--on-ink) 35%,transparent);color:var(--on-ink);height:16px;font-size:10px">${mod}T</span></a></div>
      <div class="pane__scroll">
        ${item(mine[0], 1)}${item(mine[1])}
        <div class="fl__group">${I('chevD')}${L('已结束', 'Finished')} <span style="font-weight:400">1</span></div>
        ${item(mine[2])}
      </div>
      <div class="composer" style="gap:6px"><div class="composer__who" style="color:var(--ok)">${I('coffee')}<span>${L('有任务在运行，这台 Mac 保持唤醒。合上盖子仍会睡眠。', 'Tasks are running, so this Mac stays awake. Closing the lid still sleeps it.')}</span></div></div>
    </div>
    <div class="pane">
      <div class="task-head">${tool('claude', true)}
        <div style="min-width:0"><div class="task-head__title truncate">${mine[0].title}</div><div class="task-head__sub"><span class="ref ref--ai">${I('branch')}ai/fix-settlement-rounding</span><span>${L('从 main 创建 · 独立工作区', 'from main · own workspace')}</span><span class="num">14:08</span></div></div>
        ${state('wait')}<span class="spacer"></span>
        <a class="btn btn--sm" href="${href('workspace-changes')}">${I('diff')}${L('查看改动', 'Review changes')} <span class="tab__n">4</span></a>
        <span class="splitbtn"><button class="btn btn--sm">${I('merge')}${L('合并到 main', 'Merge into main')}</button><button class="btn btn--sm" title="${L('推送分支 / 丢弃 / 保留', 'Push branch / Discard / Keep')}">${I('chevD')}</button></span>
        <button class="icon-btn icon-btn--sm" title="${L('停止任务', 'Stop task')}">${I('stop')}</button>
      </div>
      <div class="term-tabs"><span class="is-active">${I('tClaude')}Claude Code</span><span>${I('terminal')}zsh</span><span>${I('plus')}</span><span class="spacer"></span><span class="env">132×38</span></div>
      <div class="term-note">${I('user')}<span>${L('这个任务以你的身份运行，你能读写的它都能读写。', 'This task runs as you. It can read and change whatever you can.')}</span><span class="term-note__sep"></span><span>${L('批准方式', 'Approvals')} <b>${L('每次询问', 'Ask every time')}</b></span><span class="term-note__sep"></span><span>${L('提交作者', 'Commits as')} <b>Alex &lt;alex@acme.example.com&gt;</b></span><span class="spacer"></span><button class="link-button">${L('了解任务能做什么', 'What a task can do')}</button></div>
      <div class="term">${term}</div>
    </div>`;
}
const aiOpts = (overlay) => ({ tab: 'workspace-ai', grid: 'grid-template-columns:330px minmax(0,1fr)', overlay,
  status: `<span class="st st--warn">${I('bot')}${L('1 个任务在等你', '1 task is waiting for you')}</span><span class="st">${L('2 个运行中', '2 running')}</span>` });
SCREENS['workspace-ai'] = () => ws(aiOpts(), aiBody());

SCREENS['new-task'] = () => {
  // STATE: '' = description filled in, 'empty' = nothing typed yet, 'naming' = Start pressed, the AI is naming the task.
  const naming = STATE === 'naming', empty = STATE === 'empty';
  const pick = (t, on, sub) => `<label class="tool-pick__item${on ? ' is-on' : ''}"><span style="display:flex;align-items:center;gap:8px">${tool(t)}${TOOLS[t][1]}</span><small>${sub}</small></label>`;
  const desc = empty ? '' : L('给 webhook 加幂等 key：同一个请求重复发来时，直接返回第一次的结果，不要重复扣款。', 'Add an idempotency key to the webhook: when the same request arrives twice, return the first result instead of charging again.');
  const ph = L('写下要做的事，比如“修复结算金额的四舍五入”。也可以留空，进了终端再说。', 'Describe the job, for example “fix the rounding in settlement totals”. Or leave it empty and say it in the terminal.');
  const slug = (zh, en) => `<span class="mono">ai/&lt;${L(zh, en)}&gt;</span>`;
  const hint = empty
    ? `<span class="field__hint task-desc__hint">${L('留空也可以：任务先叫“新任务 3”，你发出第一条消息后再改成合适的名字。', 'Empty is fine: the task is called “New task 3” until your first message gives it a name.')}<br>${L('分支', 'Branch')} <span class="mono">ai/new-task-3</span>${L('，文件夹同名，之后不变。', ' and a folder of the same name. They keep that name.')}</span>`
    : `<span class="field__hint task-desc__hint">${L('会作为第一条消息发给 Claude Code，任务名也由它根据这段话来起。', 'Sent to Claude Code as its first message. Claude Code also names the task from it.')}<br>${L('分支', 'Branch')} ${slug('英文短名', 'short-name')}${L(' 和文件夹也在开始时命名。', ' and its folder are named when the task starts.')}</span>`;
  const start = naming
    ? `<button class="btn btn--primary is-busy" aria-busy="true"><span class="spinner"></span>${L('正在起名…', 'Naming the task...')}</button>`
    : `<a class="btn btn--primary" href="${href('new-task', { state: 'naming' })}">${I('play')}${L('开始任务', 'Start task')}</a>`;
  const modal = `<div class="modal-backdrop"><div class="modal" role="dialog" aria-modal="true" aria-labelledby="nt-t" style="width:min(580px,100%)">
    <div class="modal__header"><div><h2 class="modal__title" id="nt-t">${L('新建 AI 任务', 'New AI task')}</h2><div class="sheet__sub">acme_api · ${L('现在在 main 分支', 'currently on main')}</div></div><span class="spacer"></span><button class="icon-btn icon-btn--sm">${I('x')}</button></div>
    <div class="modal__body${naming ? ' is-busy' : ''}" style="display:flex;flex-direction:column;gap:var(--sp-5)">
      <div class="field"><span class="field__label">${L('用哪个工具', 'Tool')}</span><div class="tool-pick">${pick('claude', 1, L('已安装 2.4.11', 'Installed 2.4.11'))}${pick('codex', 0, L('已安装 0.169.0', 'Installed 0.169.0'))}${pick('opencode', 0, L('未安装，开始时安装', 'Not installed. Installs on start'))}</div></div>
      <div class="field"><span class="field__label">${L('在哪里运行', 'Where it runs')}</span>
        <label class="opt is-on"><input type="radio" class="radio" name="w" checked${naming ? ' disabled' : ''}><span class="opt__title">${L('独立工作区', 'Own workspace')} <span class="badge badge--neutral" style="height:18px;font-size:11px">${L('推荐', 'Recommended')}</span></span><span class="opt__text">${L('从 main 新建一个分支，放在单独的文件夹里。别的任务和你自己的改动都碰不到它，做完后再合并或丢弃。', 'A new branch from main in a folder of its own. Other tasks and your own edits cannot touch it. Merge or discard it when it is done.')}</span></label>
        <label class="opt"><input type="radio" class="radio" name="w"${naming ? ' disabled' : ''}><span class="opt__title">${L('主工作区', 'Main workspace')}</span><span class="opt__text">${L('直接在仓库文件夹里运行。适合问一个问题或改一处小地方。', 'Runs directly in the repository folder. Good for a question or one small fix.')}</span></label>
      </div>
      <label class="field"><span class="field__label task-desc__label">${L('要 AI 做什么', 'What should the AI do?')}<span class="task-desc__key">${kbd(mod, '⏎')} ${L('开始', 'to start')}</span></span>
        <textarea class="textarea task-desc" rows="3" placeholder="${ph}"${naming ? ' disabled' : ' autofocus'}>${desc}</textarea>
        ${hint}</label>
      <div class="note-line">${I('info')}<span>${L('新工作区是一份干净的代码。.env 这类没进版本库的文件，会按仓库里的 .worktreeinclude 复制过去。', 'The new workspace is a clean copy. Files kept out of git, such as .env, are copied in as listed in the repository’s .worktreeinclude.')}</span></div>
    </div>
    <div class="modal__footer"><span style="margin-right:auto;font-size:12px;color:var(--text-3)">${L('这个仓库已有 2 个任务在运行', '2 tasks are already running in this repository')}</span><a class="btn" href="${href('workspace-ai')}">${L('取消', 'Cancel')}</a>${start}</div>
  </div></div>`;
  return ws(aiOpts(modal), aiBody());
};
