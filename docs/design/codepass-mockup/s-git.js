/* Git screens: history, branches, stash, merge conflict, pull requests, PR detail */

/* ---------------------------------------------------------------- History */
SCREENS['repo-history'] = () => {
  const A = 'var(--text-3)', B = 'var(--info)';
  const seg = {
    a: `<path d="M16 0V44" stroke="${A}"/>`, aTop: `<path d="M16 0V22" stroke="${A}"/>`, aBot: `<path d="M16 22V44" stroke="${A}"/>`,
    b: `<path d="M36 0V44" stroke="${B}"/>`,
    bDown: `<path d="M36 22C36 36 16 30 16 44" stroke="${B}"/>`, aFork: `<path d="M16 22C16 36 36 30 36 44" stroke="${B}"/>`, bJoin: `<path d="M36 0C36 20 16 24 16 44" stroke="${B}"/>`,
  };
  const node = (lane, head) => `<circle cx="${lane ? 36 : 16}" cy="22" r="${head ? 5 : 4}" fill="${head ? 'var(--surface)' : lane ? B : A}" stroke="${head ? 'var(--text)' : 'var(--surface)'}" stroke-width="${head ? 2.5 : 2}"/>`;
  const lane = (parts, ln, head) => `<svg class="lane" viewBox="0 0 56 44" fill="none" stroke-width="2">${parts.map((p) => seg[p]).join('')}${node(ln, head)}</svg>`;
  const ref = (cls, icon, name) => `<span class="ref ${cls}">${icon ? I(icon) : ''}${name}</span>`;
  const C = [
    [['bDown'], 1, 0, ref('ref--ai', 'bot', 'ai/fix-settlement-rounding'), 'settlement: integer fee math', 'K', ME, L('12 分钟前', '12 min ago'), 'e19b4a7'],
    [['aBot'], 0, 1, ref('ref--head', 'branch', 'main') + `<span class="sync"><b class="up">↑2</b></span>`, 'payout: copy pending slice before unlock', 'K', ME, L('40 分钟前', '40 min ago'), 'a41c9e2'],
    [['a'], 0, 0, '', 'webhook: retry with idempotency key', 'K', ME, L('1 小时前', '1 h ago'), 'b7d2f10', 1],
    [['a', 'aFork'], 0, 0, ref('ref--remote', '', 'origin/main'), 'Merge branch ‘feat/payout-batch’', 'W', 'Wei Jie', L('3 小时前', '3 h ago'), '9c03e5a'],
    [['a', 'b'], 1, 0, ref('', 'branch', 'feat/payout-batch'), 'payout: batch size from config', 'W', 'Wei Jie', L('3 小时前', '3 h ago'), '5e8a1d4'],
    [['a', 'b'], 1, 0, '', 'payout: add Batcher', 'W', 'Wei Jie', L('昨天', 'Yesterday'), '0fd27c6'],
    [['a', 'bJoin'], 0, 0, ref('ref--tag', 'tag', 'v2.8.0'), 'settlement: expose Report totals', 'A', 'Aina', L('昨天', 'Yesterday'), '77b90e3'],
    [['a'], 0, 0, '', 'ledger: index by merchant and day', 'K', ME, L('2 天前', '2 days ago'), 'd2c4f81'],
    [['a'], 0, 0, '', 'ci: cache the Go build between jobs', 'A', 'Aina', L('2 天前', '2 days ago'), '31ae6b0'],
    [['a'], 0, 0, '', 'settlement: reconcile one merchant per day', 'K', ME, L('3 天前', '3 days ago'), 'c8e15f2'],
    [['a'], 0, 0, ref('ref--tag', 'tag', 'v2.7.3'), 'webhook: verify signature before parsing', 'W', 'Wei Jie', L('4 天前', '4 days ago'), '6b1d9aa'],
    [['a'], 0, 0, '', 'chore: bump aws-sdk-go-v2', 'A', 'Aina', L('5 天前', '5 days ago'), 'f40c7d5'],
    [['a'], 0, 0, '', 'payout: reject negative amounts', 'K', ME, L('6 天前', '6 days ago'), '18a3e6c'],
    [['a'], 0, 0, '', 'docs: settlement runbook', 'W', 'Wei Jie', L('上周', 'Last week'), 'a9f2b74'],
    [['a'], 0, 0, '', 'ledger: postings are append-only', 'K', ME, L('上周', 'Last week'), '4d07e2b'],
    [['a'], 0, 0, ref('ref--tag', 'tag', 'v2.7.2'), 'release: 2.7.2', 'A', 'Aina', L('上周', 'Last week'), 'be6c31f'],
    [['a'], 0, 0, '', 'webhook: log the delivery id', 'W', 'Wei Jie', L('2 周前', '2 weeks ago'), '9a5f0c8'],
  ];
  const rows = C.map(([parts, ln, head, refs, msg, ini, who, when, hash, on]) => `<div class="commit${on ? ' is-active' : ''}">${lane(parts, ln, head)}
      <div style="min-width:0"><div class="commit__msg">${refs}<span class="m">${msg}</span></div></div>
      <div class="commit__r"><span style="display:inline-flex;gap:6px;align-items:center;justify-content:flex-end"><span class="who">${ini}</span>${who} · ${when}</span><span class="mono">${hash}</span></div></div>`).join('');
  const fi = (name, dir, add, del, st, on) => `<li class="${on ? 'is-active' : ''}">${I('file')}<span class="nm">${name}</span><span class="dir">${dir}</span><span class="stat">${add ? `<span class="plus">+${add}</span>` : ''}${del ? `<span class="minus">−${del}</span>` : ''}</span><span class="st g-${st.toLowerCase()}">${st}</span></li>`;
  const body = `
    <div class="pane" style="border-right:1px solid var(--border)">
      <div class="pane__head" style="height:44px;gap:var(--sp-2);padding:0 var(--sp-3)">
        <div class="search" style="flex:1">${I('search')}<input class="input" style="height:28px" placeholder="${L('搜索说明、作者或提交号', 'Search message, author or hash')}"></div>
        <div class="segmented"><button class="segmented__item is-active">${L('全部分支', 'All branches')}</button><button class="segmented__item">main</button></div>
        <button class="icon-btn icon-btn--sm" title="${L('按作者或文件筛选', 'Filter by author or file')}">${I('filter')}</button>
      </div>
      <div class="pane__scroll" style="overflow:hidden">${rows}</div>
    </div>
    <div class="pane">
      <div class="cdetail">
        <div class="cdetail__title">webhook: retry with idempotency key</div>
        <div class="cdetail__body">${L('每次重试带同一个 key，接收方会丢弃重复的请求，不再重复入账。', 'Every attempt carries the same key, so the receiver drops duplicates instead of posting twice.')}</div>
        <div class="cluster" style="--gap:10px;font-size:12px;color:var(--text-3)"><span class="who who--lg">K</span><span style="color:var(--text)">${ME} <span class="mono muted">&lt;${ID_PROJ}&gt;</span></span><span>2026-11-15 21:42</span><span class="mono">b7d2f10</span><button class="icon-btn icon-btn--sm" title="${L('复制提交号', 'Copy hash')}">${I('copy')}</button></div>
        <div class="cluster"><button class="btn btn--sm">${I('branch')}${L('从这里建分支', 'Branch from here')}</button><button class="btn btn--sm">${L('拣选到 main', 'Cherry-pick onto main')}</button><button class="btn btn--sm">${I('undo')}${L('还原这次提交', 'Revert this commit')}</button><button class="btn btn--sm btn--ghost">${I('tag')}${L('打标签', 'Tag')}</button><button class="btn btn--sm btn--ghost">${L('重置 main 到这里', 'Reset main to here')}</button></div>
      </div>
      <div class="pane__head">${L('改动的文件', 'Files changed')}<span class="count">3 · <span class="plus">+48</span> <span class="minus">−9</span></span></div>
      <ul class="fl" style="flex:none;border-bottom:1px solid var(--border-subtle)">${fi('retry.go', 'internal/webhook', 9, 1, 'M', 1)}${fi('retry_test.go', 'internal/webhook', 36, 0, 'A')}${fi('client.go', 'internal/webhook', 3, 8, 'M')}</ul>
      <div class="diff">
        <div class="hunk">@@ -22,8 +22,14 @@ func (r *Retrier) Do(ctx context.Context, ev Event) error {</div>
        ${dl([[' ', 22, 22, '	for attempt := 1; attempt <= r.max; attempt++ {'], ['-', 23, '', '		err := r.post(ctx, ev)'],
          ['+', '', 23, '		// Same key on every attempt: the receiver drops duplicates'], ['+', '', 24, '		// instead of posting the entry twice.'], ['+', '', 25, '		err := r.post(ctx, ev, WithIdempotencyKey(ev.ID))'],
          [' ', 24, 26, '		if err == nil {'], [' ', 25, 27, '			return nil'], [' ', 26, 28, '		}'],
          ['+', '', 29, '		if !retryable(err) {'], ['+', '', 30, '			return err'], ['+', '', 31, '		}'],
          [' ', 27, 32, '		time.Sleep(backoff(attempt))'], [' ', 28, 33, '	}'], [' ', 29, 34, '	return ErrGaveUp'], [' ', 30, 35, '}']])}
      </div>
    </div>`;
  return ws({ tab: 'repo-history', grid: 'grid-template-columns:minmax(0,1fr) minmax(0,1.05fr)', status: `<span class="st">${I('branch')}main</span><span class="st">${L('1,284 个提交', '1,284 commits')}</span>` }, body);
};

/* ---------------------------------------------------------------- Branches */
SCREENS['repo-branches'] = () => {
  const li = (d, icon, name, extra, cls) => `<li style="--d:${d}" class="${cls || ''}">${I(icon)}<span class="nm">${name}</span>${extra || ''}</li>`;
  const sync = (u, dn) => `<span class="sync">${u ? `<b class="up">↑${u}</b>` : ''}${dn ? `<b>↓${dn}</b>` : ''}</span>`;
  const body = `
    <div class="pane pane--side" style="border-right:1px solid var(--border)">
      <div class="pane__head">${L('分支和标签', 'Branches and tags')}<span class="spacer"></span><button class="btn btn--sm">${I('plus')}${L('新建分支', 'New branch')}</button></div>
      <div class="pane__filter"><div class="search">${I('search')}<input class="input" placeholder="${L('筛选', 'Filter')}"></div></div>
      <div class="pane__scroll"><ul class="refs">
        <li class="grp">${I('chevD')}${L('本地', 'Local')} <span style="font-weight:400">5</span></li>
        ${li(1, 'check', 'main', sync(2, 0) + `<span class="badge badge--neutral" style="height:18px;font-size:11px;margin-left:6px">${L('当前', 'Current')}</span>`, 'cur')}
        ${li(1, 'branch', 'feat/payout-batch', sync(3, 1), 'is-active')}
        ${li(1, 'branch', 'hotfix/webhook-retry', sync(0, 0))}
        ${li(1, 'bot', 'ai/fix-settlement-rounding', sync(2, 0))}
        ${li(1, 'bot', 'ai/payout-batch-tests', sync(1, 0))}
        <li class="grp">${I('chevD')}${L('远程', 'Remote')} · origin <span style="font-weight:400">4</span></li>
        ${li(1, 'cloud', 'origin/main')}${li(1, 'cloud', 'origin/develop')}${li(1, 'cloud', 'origin/feat/payout-batch')}${li(1, 'cloud', 'origin/release/2.8')}
        <li class="grp">${I('chevD')}${L('标签', 'Tags')} <span style="font-weight:400">12</span></li>
        ${li(1, 'tag', 'v2.8.0')}${li(1, 'tag', 'v2.7.3')}${li(1, 'tag', 'v2.7.2')}${li(1, 'tag', 'v2.7.1')}
        <li class="grp">${I('chevR')}${L('远程仓库', 'Remotes')} <span style="font-weight:400">1</span></li>
      </ul></div>
    </div>
    <div class="pane" style="background:var(--bg);overflow:auto">
      <div style="max-width:880px;padding:var(--sp-6) var(--sp-8);display:flex;flex-direction:column;gap:var(--sp-5)">
        <div class="cluster" style="--gap:var(--sp-3)">
          <span class="portal__icon">${I('branch')}</span>
          <div style="flex:1;min-width:0"><div class="mono" style="font-size:18px;font-weight:600;color:var(--text)">feat/payout-batch</div><div class="portal__meta"><span>${L('跟踪', 'Tracks')} <span class="mono">origin/feat/payout-batch</span></span><span>${L('最后提交：Wei Jie，3 小时前', 'Last commit: Wei Jie, 3 h ago')}</span></div></div>
          <button class="btn btn--primary">${L('切换到这个分支', 'Switch to this branch')}</button><button class="icon-btn icon-btn--bordered" title="${L('重命名 / 推送 / 删除', 'Rename / Push / Delete')}">${I('more')}</button>
        </div>
        <div class="stack" style="--gap:var(--sp-3)">
          <div class="cluster"><div class="section-label">${L('和 main 比较', 'Compared with main')}</div><span class="spacer"></span><button class="btn btn--ghost btn--sm">${L('换一个比较对象', 'Compare with another')}${I('chevD')}</button></div>
          <div class="cmp">
            <div class="cmp__side"><div class="cmp__n" style="color:var(--info)">3</div><div class="cmp__k">${L('这里有、main 还没有的提交', 'commits here that main does not have')}</div></div>
            <div class="cmp__side"><div class="cmp__n">1</div><div class="cmp__k">${L('main 有、这里还没有的提交', 'commit on main that is not here')}</div></div>
          </div>
          <div class="cluster">
            <button class="btn">${I('merge')}${L('合并到 main', 'Merge into main')}</button><button class="btn">${L('变基到 main 之上', 'Rebase onto main')}</button><button class="btn">${L('把 main 合并进来', 'Merge main into this')}</button><span class="spacer"></span><span class="hint">${L('这个仓库在 CodeCommit，拉取请求请在 AWS 控制台里创建', 'This repository is on CodeCommit. Create pull requests in the AWS console')}</span>
          </div>
        </div>
        <div class="card">
          <div class="card__header" style="padding:10px var(--sp-4)"><div class="segmented"><button class="segmented__item is-active">${L('提交', 'Commits')} <span class="nav__count">3</span></button><button class="segmented__item">${L('改动的文件', 'Files changed')} <span class="nav__count">6</span></button></div><span class="spacer"></span><span class="mono" style="font-size:12px"><span class="plus">+214</span> <span class="minus">−37</span></span></div>
          <ul class="mini-commits">
            <li><span class="who">W</span><span class="m">payout: batch size from config</span><span class="mono">5e8a1d4 · ${L('3 小时前', '3 h ago')}</span></li>
            <li><span class="who">W</span><span class="m">payout: add Batcher</span><span class="mono">0fd27c6 · ${L('昨天', 'Yesterday')}</span></li>
            <li><span class="who">W</span><span class="m">payout: split send from flush</span><span class="mono">2ab6e09 · ${L('昨天', 'Yesterday')}</span></li>
          </ul>
        </div>
        <div class="banner banner--warn" style="padding:12px 16px">${I('alert', 'banner__icon')}<div class="banner__body"><div class="banner__title">${L('合并时 1 个文件可能冲突', '1 file may conflict when merging')}</div><span class="mono">internal/payout/batch.go</span> ${L('在两边都改过。合并时会带你一处一处解决。', 'was changed on both sides. Merging walks you through each spot.')}</div><div class="banner__actions"><a class="btn btn--sm" href="${href('repo-merge-conflict')}">${L('预览冲突', 'Preview conflicts')}</a></div></div>
      </div>
    </div>`;
  return ws({ tab: 'repo-branches', grid: 'grid-template-columns:310px minmax(0,1fr)', status: `<span class="st">${I('branch')}main</span><span class="st">${L('刚刚获取过远程更新', 'Fetched just now')}</span>` }, body);
};

/* ---------------------------------------------------------------- Stash */
SCREENS['repo-stash'] = () => {
  const fi = (name, dir, add, del, st, on) => `<li class="${on ? 'is-active' : ''}">${I('file')}<span class="nm">${name}</span><span class="dir">${dir}</span><span class="stat">${add ? `<span class="plus">+${add}</span>` : ''}${del ? `<span class="minus">−${del}</span>` : ''}</span><span class="st g-${st.toLowerCase()}">${st}</span></li>`;
  const body = `${changesLeft('stash')}
    <div class="pane">
      <div class="task-head">
        <span class="portal__icon">${I('archive')}</span>
        <div style="min-width:0"><div class="task-head__title">${L('WIP：payout 指标埋点', 'WIP: payout metrics')}</div><div class="task-head__sub"><span class="mono">stash@{0}</span><span>${L('在 main 上贮藏 · 昨天 18:40 · 包含未跟踪的文件', 'stashed on main · yesterday 18:40 · includes untracked files')}</span></div></div>
        <span class="spacer"></span>
        <button class="btn btn--sm btn--primary">${L('取出并删除', 'Apply and remove')}</button><button class="btn btn--sm">${L('取出，保留贮藏', 'Apply and keep')}</button><button class="btn btn--sm">${I('branch')}${L('建成分支', 'Make a branch')}</button><button class="btn btn--sm btn--danger-ghost">${I('trash')}${L('删除', 'Delete')}</button>
      </div>
      <div class="pane__head">${L('贮藏里的文件', 'Files in this stash')}<span class="count">3 · <span class="plus">+41</span> <span class="minus">−2</span></span></div>
      <ul class="fl" style="flex:none;border-bottom:1px solid var(--border-subtle)">${fi('metrics.go', 'internal/payout', 28, 0, 'A', 1)}${fi('batch.go', 'internal/payout', 9, 2, 'M')}${fi('main.go', 'cmd/api', 4, 0, 'M')}</ul>
      <div class="diff">
        <div class="hunk">@@ -0,0 +1,28 @@</div>
        ${dl([['+', '', 1, 'package payout'], ['+', '', 2, ''], ['+', '', 3, 'import "github.com/prometheus/client_golang/prometheus"'], ['+', '', 4, ''],
          ['+', '', 5, '// Metrics counts what the batcher does. Register it once at start-up.'], ['+', '', 6, 'type Metrics struct {'], ['+', '', 7, '	Sent     prometheus.Counter'], ['+', '', 8, '	Timeouts prometheus.Counter'], ['+', '', 9, '	Pending  prometheus.Gauge'], ['+', '', 10, '}'], ['+', '', 11, ''],
          ['+', '', 12, 'func NewMetrics(reg prometheus.Registerer) *Metrics {'], ['+', '', 13, '	m := &Metrics{'], ['+', '', 14, '		Sent: prometheus.NewCounter(prometheus.CounterOpts{'], ['+', '', 15, '			Name: "payout_batch_sent_total",'], ['+', '', 16, '		}),'], ['+', '', 17, '		Timeouts: prometheus.NewCounter(prometheus.CounterOpts{'], ['+', '', 18, '			Name: "payout_batch_timeouts_total",'], ['+', '', 19, '		}),']])}
      </div>
    </div>`;
  return ws({ tab: 'repo-commit', grid: 'grid-template-columns:340px minmax(0,1fr)', status: `<span class="st">${I('branch')}main</span><span class="st">${L('2 个贮藏', '2 stashes')}</span>` }, body);
};

/* ---------------------------------------------------------------- Merge conflict */
SCREENS['repo-merge-conflict'] = () => {
  const fi = (name, dir, st, on, ok) => `<li class="${on ? 'is-active' : ''}">${ok ? `<span style="color:var(--ok);display:inline-grid">${I('circleCheck')}</span>` : I('file')}<span class="nm">${name}</span><span class="dir">${dir}</span><span class="st ${ok ? '' : 'g-c'}" style="margin-left:auto">${st}</span></li>`;
  const common = (n) => [
    '', '// Flush sends what is pending.', 'func (b *Batcher) Flush(ctx context.Context) error {', '	b.mu.Lock()', '	defer b.mu.Unlock()', '	if len(b.pending) == 0 {', '		return nil', '	}',
  ];
  const ours = codeLines([['const maxBatch = 200', { cls: 'ours' }], ...common(), ['	batch := append([]Payout(nil), b.pending...)', { cls: 'ours' }], ['	b.pending = b.pending[:0]', { cls: 'ours' }], '	return b.send(ctx, batch)', '}'], 'go', 36);
  const theirs = codeLines([['// Batch size comes from config (payout.batch_size).', { cls: 'theirs' }], ...common(), ['	batch := b.take(b.cfg.BatchSize)', { cls: 'theirs' }], '	return b.send(ctx, batch)', '}'], 'go', 36);
  const blk = (done, n, label, btns) => `<div class="cf__block${done ? ' is-done' : ''}"><b>${L(`冲突 ${n}`, `Conflict ${n}`)}</b><span>${label}</span><span class="spacer"></span>${btns}</div>`;
  const result = codeLines([
    ['', { html: blk(1, 1, L('已采用对方的', 'took theirs'), `<button class="btn btn--sm btn--ghost">${I('undo')}${L('撤销', 'Undo')}</button>`) }],
    ['// Batch size comes from config (payout.batch_size).', { cls: 'res' }], ...common(),
    ['', { html: blk(0, 2, L('两边都改了这 2 行', 'both sides changed these 2 lines'), `<button class="btn btn--sm">${L('用我的', 'Take mine')}</button><button class="btn btn--sm">${L('用对方的', 'Take theirs')}</button><button class="btn btn--sm">${L('两边都要', 'Take both')}</button>`) }],
    ['	batch := append([]Payout(nil), b.pending...)', { cls: 'ours' }], ['	b.pending = b.pending[:0]', { cls: 'ours' }],
    ['	batch := b.take(b.cfg.BatchSize)', { cls: 'theirs', no: '' }],
    '	return b.send(ctx, batch)', '}',
  ], 'go', 36);
  const body = `
    <div class="opbar" style="grid-column:1/-1">${I('merge')}<span>${L('正在把 <b>feat/payout-batch</b> 合并到 <b>main</b>', 'Merging <b>feat/payout-batch</b> into <b>main</b>')}</span><span>·</span><span>${L('3 个文件有冲突，已解决 1 个', '3 files have conflicts, 1 resolved')}</span><div class="progress"><div class="progress__bar" style="--pct:33%;background:var(--warn)"></div></div><span class="spacer"></span><button class="btn btn--sm btn--danger-ghost">${L('放弃这次合并', 'Abort merge')}</button><button class="btn btn--sm btn--primary" disabled title="${L('先解决全部冲突', 'Resolve every conflict first')}">${L('完成合并', 'Finish merge')}</button></div>
    <div class="pane pane--side" style="border-right:1px solid var(--border)">
      <div class="pane__scroll">
        <div class="fl__group">${I('chevD')}${L('还有冲突', 'Still in conflict')} <span style="font-weight:400">2</span></div>
        <ul class="fl">${fi('batch.go', 'internal/payout', '2', 1)}${fi('config.go', 'internal/config', '1')}</ul>
        <div class="fl__group">${I('chevD')}${L('已解决', 'Resolved')} <span style="font-weight:400">1</span></div>
        <ul class="fl">${fi('batch_test.go', 'internal/payout', '', 0, 1)}</ul>
        <div class="fl__group">${I('chevR')}${L('自动合并，没有冲突', 'Merged automatically')} <span style="font-weight:400">4</span></div>
      </div>
      <div class="composer" style="gap:6px"><div class="composer__who">${I('info')}<span>${L('合并开始前这里没有未提交的改动，所以放弃后 main 会回到合并前的样子。', 'There were no uncommitted changes when this merge started, so aborting puts main back exactly as it was before the merge. Nothing is lost.')}</span></div></div>
    </div>
    <div class="cf">
      <div class="diff__file" style="position:static">${I('file')}batch.go <span>internal/payout</span><span class="badge badge--warn" style="height:18px;font-size:11px">${L('2 处冲突，已解决 1 处', '2 conflicts, 1 resolved')}</span><span class="spacer"></span><button class="btn btn--sm btn--ghost">${L('上一处', 'Previous')}</button><button class="btn btn--sm btn--ghost">${L('下一处', 'Next')} ${kbd('F7')}</button><a class="btn btn--sm btn--ghost" href="${href('workspace-code')}">${I('code')}${L('在编辑器中打开', 'Open in editor')}</a></div>
      <div class="cf__top">
        <div class="cf__side"><div class="cf__label"><span class="dot" style="background:var(--info)"></span><b>${L('我的', 'Mine')}</b> main<span class="spacer"></span><button class="btn btn--sm">${L('整个文件用我的', 'Use mine for the whole file')}</button></div><div class="ed__code">${ours}</div></div>
        <div class="cf__side"><div class="cf__label"><span class="dot" style="background:var(--acct-3)"></span><b>${L('对方的', 'Theirs')}</b> feat/payout-batch<span class="spacer"></span><button class="btn btn--sm">${L('整个文件用对方的', 'Use theirs for the whole file')}</button></div><div class="ed__code">${theirs}</div></div>
      </div>
      <div class="cf__side"><div class="cf__label">${I('pencil')}<b>${L('合并结果', 'Result')}</b> ${L('可以直接修改', 'you can edit it directly')}<span class="spacer"></span><button class="btn btn--sm" disabled>${L('标记为已解决', 'Mark as resolved')}</button></div><div class="ed__code">${result}</div></div>
    </div>`;
  return ws({ tab: 'repo-commit', grid: 'grid-template-columns:300px minmax(0,1fr);grid-template-rows:auto minmax(0,1fr)', status: `<span class="st st--warn">${I('merge')}${L('合并进行中', 'Merge in progress')}</span>` }, body);
};

/* ---------------------------------------------------------------- Pull requests (GitHub repo) */
const PRS = [
  { n: 128, st: 'open', title: 'Onboarding: verify phone before KYC upload', from: 'feat/phone-verify', by: 'alex-dev', me: 1, checks: ['ok', L('6 项通过', '6 passed')], rv: [['A', 'ok'], ['W', '']], when: L('20 分钟前', '20 min ago'), c: 3 },
  { n: 127, st: 'open', title: 'Fix remittance quote rounding for IDR', from: 'ai/quote-rounding', by: 'alex-dev', me: 1, ai: 1, checks: ['run', L('3 / 6 运行中', '3 of 6 running')], rv: [['A', ''], ['W', '']], when: L('1 小时前', '1 h ago'), c: 0 },
  { n: 125, st: 'open', title: 'Rate table: cache by corridor', from: 'feat/rate-cache', by: 'aina-r', review: 1, checks: ['ok', L('6 项通过', '6 passed')], rv: [['K', '']], when: L('3 小时前', '3 h ago'), c: 5 },
  { n: 121, st: 'draft', title: 'Upgrade Expo SDK to 55', from: 'chore/expo-55', by: 'weijie-t', checks: ['fail', L('1 项失败', '1 failed')], rv: [], when: L('昨天', 'Yesterday'), c: 1 },
  { n: 119, st: 'merged', title: 'Receipt PDF: Malay translation', from: 'feat/receipt-ms', by: 'aina-r', checks: ['ok', L('6 项通过', '6 passed')], rv: [['K', 'ok'], ['W', 'ok']], when: L('2 天前', '2 days ago'), c: 8 },
];
const prIcon = (st) => `<span class="pr-icon ${st === 'merged' ? 'pr-icon--merged' : st === 'draft' ? 'pr-icon--draft' : ''}">${I(st === 'merged' ? 'merge' : 'pr')}</span>`;
const checkIcon = { ok: 'circleCheck', fail: 'circleX', run: 'circleDot' };
SCREENS['repo-prs'] = () => {
  const row = (p) => `<a class="pr-row" href="${href('pr-detail')}" style="color:inherit">${prIcon(p.st)}
      <div style="min-width:0"><div class="pr-row__title">${p.title} <span class="muted" style="font-weight:400">#${p.n}</span></div>
        <div class="pr-row__sub"><span class="ref">${p.from}</span>${I('arrowR')}<span class="ref">main</span><span>${p.me ? L('我创建的', 'by me') : p.by}</span>${p.ai ? `<span class="ref ref--ai">${I('bot')}${L('来自 AI 任务', 'from an AI task')}</span>` : ''}${p.st === 'draft' ? `<span class="badge badge--neutral" style="height:18px;font-size:11px">${L('草稿', 'Draft')}</span>` : ''}${p.st === 'merged' ? `<span class="badge badge--neutral" style="height:18px;font-size:11px">${L('已合并', 'Merged')}</span>` : ''}${p.review ? `<span class="badge badge--warn" style="height:18px;font-size:11px">${L('等你评审', 'Needs your review')}</span>` : ''}</div></div>
      <span class="checks checks--${p.checks[0]}">${I(checkIcon[p.checks[0]])}${p.checks[1]}</span>
      <span style="display:flex;align-items:center;gap:8px;font-size:12px;color:var(--text-3)">${p.rv.length ? `<span class="avatars">${p.rv.map(([i, s]) => `<span class="who ${s}">${i}</span>`).join('')}</span>${p.rv.filter((r) => r[1] === 'ok').length} / ${p.rv.length} ${L('批准', 'approved')}` : L('还没有评审人', 'No reviewers')}</span>
      <span class="muted" style="font-size:12px">${p.when}</span>
      <span class="muted" style="font-size:12px;display:inline-flex;align-items:center;gap:5px;justify-content:flex-end">${I('msg')}${p.c}</span></a>`;
  const body = `<div class="pr-wrap"><div class="pr-page">
      <div class="toolbar">
        <div class="segmented"><button class="segmented__item">${L('我创建的', 'Mine')} <span class="nav__count">2</span></button><button class="segmented__item">${L('等我评审', 'To review')} <span class="nav__count nav__count--warn">1</span></button><button class="segmented__item is-active">${L('全部', 'All')} <span class="nav__count">5</span></button></div>
        <div class="segmented"><button class="segmented__item is-active">${L('打开和最近的', 'Open and recent')}</button><button class="segmented__item">${L('已合并', 'Merged')}</button><button class="segmented__item">${L('已关闭', 'Closed')}</button></div>
        <div class="search">${I('search')}<input class="input" placeholder="${L('搜索标题或编号', 'Search title or number')}"></div>
        <button class="btn btn--primary">${I('plus')}${L('新建拉取请求', 'New pull request')}</button>
      </div>
      <div class="table" style="overflow:visible">
        <div class="table__head" style="--cols:22px minmax(0,1fr) 150px 130px 110px 84px;position:static"><span></span><span>${L('拉取请求', 'Pull request')}</span><span>${L('检查', 'Checks')}</span><span>${L('评审', 'Reviews')}</span><span>${L('更新时间', 'Updated')}</span><span style="text-align:right">${L('评论', 'Comments')}</span></div>
        ${PRS.map(row).join('')}
      </div>
      <div class="hint" style="display:flex;align-items:center;gap:8px">${prov('github')}${L('通过 alex-dev 的 API 令牌读取 github.com/alex-dev/globex_app · 1 分钟前更新', 'Read from github.com/alex-dev/globex_app with alex-dev’s API token · updated 1 min ago')} <span class="link">${L('刷新', 'Refresh')}</span></div>
    </div></div>`;
  return ws({ tab: 'repo-prs', repo: REPO_APP, rail: 'repo-prs', status: `<span class="st">${I('branch')}feat/phone-verify</span>` }, body);
};

SCREENS['pr-detail'] = () => {
  const check = (name, t) => `<div class="rv"><span class="checks checks--ok">${I('circleCheck')}</span>${name}<span class="st">${t}</span></div>`;
  const body = `<div class="pr-wrap"><div class="pr-page">
      <a class="back-link" href="${href('repo-prs')}" style="margin:0">${I('chevL')}${L('拉取请求', 'Pull requests')}</a>
      <div class="cluster" style="align-items:flex-start;flex-wrap:nowrap">
        <div style="flex:1;min-width:0"><h1 class="pr-title">Onboarding: verify phone before KYC upload <span>#128</span></h1>
          <div class="pr-meta"><span class="badge badge--ok">${I('pr')}${L('打开', 'Open')}</span><span class="ref">feat/phone-verify</span>${I('arrowR')}<span class="ref">main</span><span class="who">A</span><span>alex-dev · ${L('2 天前创建 · 3 个提交', 'opened 2 days ago · 3 commits')}</span></div></div>
        <button class="btn">${I('ext')}${L('在 GitHub 上打开', 'Open on GitHub')}</button>
      </div>
      <div class="tabs"><button class="tab is-active">${L('概览', 'Overview')}</button><button class="tab">${L('提交', 'Commits')} <span class="tab__n">3</span></button><button class="tab">${L('改动的文件', 'Files changed')} <span class="tab__n">7</span></button><span class="spacer"></span><span class="mono" style="font-size:12px;align-self:center"><span class="plus">+212</span> <span class="minus">−48</span></span></div>
      <div class="pr-detail">
        <div class="stack" style="--gap:var(--sp-4)">
          <div class="card"><div class="card__body pr-desc">
            <h4>${L('做了什么', 'What')}</h4>
            <p>${L('上传证件前先用短信验证码确认手机号。没验证的用户会停在新的一步，不能直接进入 KYC。', 'Confirm the phone number with an SMS code before any document upload. Users who are not verified stop at the new step and cannot enter KYC.')}</p>
            <h4>${L('怎么验证', 'How to check')}</h4>
            <ul><li>${L('新用户注册，走完手机验证，再上传证件', 'Sign up as a new user, verify the phone, then upload a document')}</li><li>${L('验证码输错 5 次后，等 10 分钟才能重发', 'After 5 wrong codes, resending waits 10 minutes')}</li></ul>
          </div></div>
          <div class="card"><div class="card__body" style="padding:var(--sp-3) var(--sp-5)"><ul class="tl">
            <li><span class="ico">${I('commit')}</span><span><b>alex-dev</b> ${L('推送了 3 个提交', 'pushed 3 commits')}</span><span class="when">${L('2 天前', '2 days ago')}</span></li>
            <li><span class="ico">${I('msg')}</span><span><b>aina-r</b> ${L('留下了 3 条逐行评论', 'left 3 line comments')} · <span class="link">${L('在 GitHub 上查看和回复', 'View and reply on GitHub')}</span></span><span class="when">${L('昨天', 'Yesterday')}</span></li>
            <li><span class="ico">${I('commit')}</span><span><b>alex-dev</b> ${L('推送了 1 个提交', 'pushed 1 commit')} · <span class="mono muted">otp: lock resend after 5 failures</span></span><span class="when">${L('1 小时前', '1 h ago')}</span></li>
            <li><span class="ico ok">${I('check')}</span><span><b>aina-r</b> ${L('批准了这些改动', 'approved these changes')}</span><span class="when">${L('20 分钟前', '20 min ago')}</span></li>
          </ul></div></div>
        </div>
        <div class="stack" style="--gap:var(--sp-4)">
          <div class="card side-card mergebox">
            <div class="mergebox__head">${I('circleCheck')}${L('可以合并', 'Ready to merge')}</div>
            <div class="hint" style="font-size:12.5px">${L('检查全部通过 · 1 个批准 · 和 main 没有冲突', 'All checks passed · 1 approval · no conflicts with main')}</div>
            <select class="select"><option>${L('压缩成一个提交再合并', 'Squash and merge')}</option></select>
            <label style="display:flex;align-items:center;gap:8px;font-size:12.5px;color:var(--text-2)"><input type="checkbox" class="checkbox" checked>${L('合并后删除 feat/phone-verify', 'Delete feat/phone-verify after merging')}</label>
            <button class="btn btn--primary btn--block">${I('merge')}${L('合并拉取请求', 'Merge pull request')}</button>
            <button class="btn btn--danger-ghost btn--block">${L('关闭，不合并', 'Close without merging')}</button>
          </div>
          <div class="card side-card"><div class="side-card__t">${L('评审人', 'Reviewers')}<span class="spacer"></span><span class="link" style="font-weight:400">${L('添加', 'Add')}</span></div>
            <div class="rv"><span class="who who--lg ok">A</span>aina-r<span class="st ok">${I('check')}${L('已批准', 'Approved')}</span></div>
            <div class="rv"><span class="who who--lg">W</span>weijie-t<span class="st">${I('clock')}${L('还没评审', 'Not reviewed yet')}</span></div>
          </div>
          <div class="card side-card"><div class="side-card__t">${L('检查', 'Checks')}<span class="spacer"></span><span style="font-weight:400;color:var(--ok)">${L('6 项全部通过', 'All 6 passed')}</span></div>
            ${check('build', '1m 42s')}${check('unit tests', '3m 05s')}${check('lint', '38s')}${check('e2e · ios', '9m 12s')}${check('e2e · android', '8m 47s')}${check('preview deploy', '2m 10s')}
          </div>
        </div>
      </div>
    </div></div>`;
  return ws({ tab: 'repo-prs', repo: REPO_APP, rail: 'repo-prs', status: `<span class="st">${I('branch')}feat/phone-verify</span>` }, body);
};
