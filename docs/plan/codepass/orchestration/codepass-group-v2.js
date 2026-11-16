export const meta = {
  name: 'codepass-group-v2',
  description: 'Run one Codepass task group: Sonnet implements each task in its own worktree, Opus does one full review, then Sonnet fixes and Opus verifies the fixes (scoped, up to 3 rounds); Sonnet merges the approved tasks into codepass, runs the gate and pushes',
  phases: [
    { title: 'Implement', model: 'sonnet' },
    { title: 'Review', model: 'opus' },
    { title: 'Fix', model: 'sonnet' },
    { title: 'Verify', model: 'opus' },
    { title: 'Merge', model: 'sonnet' },
  ],
}

// args: { repo, wt, scratch, scrubList, portBase, attr, tasks: [{id, title}], groupNote, notes: {id: text}, keepHistory: [id] }
// keepHistory (optional): tasks whose branch must reach codepass with its history (F1-21 merges main; a squash would drop main as a parent).
const A = args
const R = A.repo
const WT = A.wt
// The commit trailer lines come from the running session's attribution reminder (they change per session).
if (!A.attr) throw new Error('args.attr is required: the two commit trailer lines of the current session, joined by \\n')
const ATTR = A.attr
const KEEP = new Set(A.keepHistory || [])
const SHARED = 'main.go, internal/app/container.go, internal/app/events.go, frontend/src/api/services.ts, frontend/src/api/mock/index.ts, frontend/src/App.tsx, frontend/src/shell/Sidebar.tsx, frontend/src/i18n/index.ts, frontend/src/styles/*.css, go.mod, go.sum, frontend/package.json, frontend/package-lock.json, .gitignore'

const others = (t) => A.tasks.filter((x) => x.id !== t.id).map((x) => `${x.id} (${x.title})`).join(', ') || 'none'
const keepNote = (t) => (KEEP.has(t.id) ? `\nHistory rule for ${t.id}: this branch reaches codepass with a --no-ff merge, never a squash, so every commit you make on task/${t.id} stays in codepass's history. End every commit message on it with exactly these two lines (this replaces "no AI attribution lines"):\n${ATTR}\n` : '')
const notes = (t) => (A.notes && A.notes[t.id] ? `\nOrchestrator notes for ${t.id}:\n${A.notes[t.id]}\n` : '') + (A.groupNote ? `\nGroup note: ${A.groupNote}\n` : '') + keepNote(t)

const READ = (t) => `Read first: ${R}/docs/plan/codepass/tasks.md: "How the work runs", "Owner decisions (FINAL)" (and the open-questions block right after it: the assumed answers apply), the standing review checklist, and your entry ${t.id} (Build / Owns / Deps / Accept / Tests / Review). Then every section it cites in ${R}/docs/plan/codepass/product-plan.md ("plan N.N") and ui-design.md ("design N.N"), the findings notes in ${R}/docs/research/ it relies on, and for UI work the named mockup screens in ${R}/docs/design/codepass-mockup/ (PNG shots in shots/; mockup.css is the draft of the new CSS). Spike code to port is on local branches spike/* (git -C ${R} show spike/<name>:<path>).`
const READ_SHORT = (t) => `Your entry: ${R}/docs/plan/codepass/tasks.md, entry ${t.id}, plus "Owner decisions (FINAL)" and the standing review checklist; open the cited plan / design sections only as needed.`

const LINKS = `If frontend/node_modules is a symlink into ${R}, NEVER run npm ci / install / prune / update / uninstall in the worktree and never rm -rf through it: that changes the main checkout (it happened once and emptied it). If your task changes frontend/package.json, first remove only the link (rm frontend/node_modules, no trailing slash), then run npm ci in frontend/.`
const GATE = `go build ./... && go vet ./... && go test ./... ; GOOS=windows GOARCH=amd64 go build ./... (plus GOOS=windows GOARCH=amd64 go vet on packages with Windows-only files) ; cd frontend && npm run typecheck && npm test && npm run build && npm run check:dist`
const RULES = (t, port) => `${t.id === 'F1-W' || /^F\d-W$/.test(t.id) ? `You are the wiring task: you own the shared files (${SHARED}) plus the files your entry and the orchestrator notes name.` : `Edit only the files your entry Owns (plus new tests); shared files (${SHARED}) belong to the wiring task: put a needed registration or dependency in notesForWiring.`} After a frontend build, git ls-tree HEAD frontend/dist/ must still list .gitkeep. Stage files by name (never git add -A or git add .); never commit .tools or node_modules. Commit on task/${t.id} with a clear message and no AI attribution lines. Do not push and do not merge into codepass. UI checks: your own Vite server on port ${port} (npm run dev -- --port ${port} --strictPort in frontend/, background), screenshots via ${R}/docs/plan/codepass/shoturl.sh <out.png> 1440 900 "http://127.0.0.1:${port}/?<mock query>" into ${WT}/${t.id}-shots/, stop the server when done; never port 9245.`

const implPrompt = (t, i) => `You are the Sonnet engineer for Codepass task ${t.id} "${t.title}".
Main checkout: ${R} (branch codepass) - never edit it. Your worktree: ${WT}/${t.id} on branch task/${t.id}.

Setup first:
1. If ${WT}/${t.id} does not exist: git -C ${R} worktree add ${WT}/${t.id} -b task/${t.id} codepass (if branch task/${t.id} already exists: git -C ${R} worktree add ${WT}/${t.id} task/${t.id}).
   Copy these commands exactly: every path is absolute. Several engineers before you typed a relative path and created the worktree inside the main checkout. Afterwards git -C ${R} worktree list must show ${WT}/${t.id}, and ${R}/.codepass-wt must not exist.
2. cd ${WT}/${t.id}; ln -sfn ${R}/.tools .tools; ln -sfn ${R}/frontend/node_modules frontend/node_modules. ${LINKS}

${READ(t)}
${notes(t)}
Other tasks run at the same time, each in its own worktree: ${others(t)}.
Regenerate frontend/bindings only if your task owns Go API types and you need the TS types (.tools/bin/wails3 generate bindings -clean=true -ts -i), and commit the result.
${RULES(t, A.portBase + i)}
Verify in your worktree (all must pass): ${GATE}.
Return: summary, filesChanged, tests (one line per gate command), deviations (with reasons), notesForWiring, manualChecks (what only the owner can verify).`

const reviewPrompt = (t, i, impl) => `You are the Opus reviewer for Codepass task ${t.id} "${t.title}". This is the one full review; later rounds only verify fixes, so report everything real now. Read-only: do not edit tracked files or commit; probes go under ${A.scratch}/review-${t.id}/ (go test -overlay works well) and are deleted when you finish. UI checks: own Vite server on port ${A.portBase + 50 + i}; never 9245.
Worktree: ${WT}/${t.id} (branch task/${t.id}, based on codepass). Review \`git -C ${WT}/${t.id} diff codepass...HEAD\` and \`git -C ${WT}/${t.id} status\`.
${READ(t)}
${notes(t)}
Implementer report: ${JSON.stringify(impl)}

Check: (1) the Build is complete; (2) Owns respected (name every file edited outside it; shared files: ${SHARED}); (3) every Accept item, verified by running where possible; (4) tests present and meaningful, not tautological; (5) the entry's Review focus; (6) the standing checklist; (7) security and failure handling; (8) Windows build and vet; (9) UI: screenshots against the named mockup shots; (10) no regressions: the full gate (${GATE}); frontend/dist/.gitkeep still tracked.
Report only real problems, each with file, line, problem and a concrete fix. Severity: blocker, major, minor, nit. verdict=approve only when there is no blocker, major or minor.`

const fixPrompt = (t, i, items, round) => `You are the Sonnet engineer fixing review findings on Codepass task ${t.id} "${t.title}" (fix round ${round}) in your worktree ${WT}/${t.id} (branch task/${t.id}). ${LINKS}
${READ_SHORT(t)}
${notes(t)}
Items:
${JSON.stringify(items, null, 2)}
Fix every blocker, major and minor item, with the tests it asks for. Fix a nit when it is quick and safe; otherwise put it under deferred with a reason. Verify each item before changing code; if one is wrong, change nothing for it and put it under declined with evidence (command output, file:line).
${RULES(t, A.portBase + i)}
Gate in your worktree, all must pass: ${GATE}.
Return: fixed, declined, deferred, tests (one line per gate command), notesForWiring (new ones only), head (commit hash).`

const verifyPrompt = (t, i, round, base, findings, fixes, prevOpen) => `You are the Opus verifier for Codepass task ${t.id} "${t.title}", verification round ${round}. Read-only: do not edit tracked files or commit; probes go under ${A.scratch}/verify-${t.id}/ and are deleted when you finish. UI checks: own Vite server on port ${A.portBase + 50 + i}; never 9245.
This is NOT a fresh full review. The full review found:
${JSON.stringify(findings)}
${prevOpen.length ? `The previous verification left these items open: ${JSON.stringify(prevOpen)}` : ''}
Fixer reports so far: ${JSON.stringify(fixes)}
${READ_SHORT(t)}
${notes(t)}
Do exactly this:
(A) For each blocker, major and minor finding (and each previously open item): fixed (confirm by reading the code AND running the test or a probe), declined-ok (the fixer's evidence holds), or open (state exactly what is still wrong and the fix).
(B) Nits the fixer says it fixed: check nothing broke. Deferred nits are acceptable.
(C) Read the fix diff: git -C ${WT}/${t.id} diff ${base}..HEAD. Look for regressions the fixes introduced and for edits outside the entry's Owns (shared files: ${SHARED}).
(D) Run the gate in the worktree: ${GATE}. Then git status (clean apart from the untracked .tools / node_modules links) and git ls-tree HEAD frontend/dist/ (must list .gitkeep).
(E) A NEW problem outside (A)-(C) goes to open only if it is a blocker or major. A new minor or nit goes to followUps: it is scheduled later and does not block this merge.
verdict=approve only when nothing is open and the gate passes.
Return: verdict, open (severity, file, line, problem, fix), resolved (one line each), declinedOk, followUps (severity, file, line, problem, fix), gate.`

const FINDING = { type: 'object', properties: { severity: { type: 'string', enum: ['blocker', 'major', 'minor', 'nit'] }, file: { type: 'string' }, line: { type: 'number' }, problem: { type: 'string' }, fix: { type: 'string' } }, required: ['severity', 'file', 'problem', 'fix'] }
const IMPL_SCHEMA = { type: 'object', properties: { summary: { type: 'string' }, filesChanged: { type: 'array', items: { type: 'string' } }, tests: { type: 'string' }, deviations: { type: 'array', items: { type: 'string' } }, notesForWiring: { type: 'array', items: { type: 'string' } }, manualChecks: { type: 'array', items: { type: 'string' } }, head: { type: 'string' } }, required: ['summary', 'filesChanged', 'tests', 'deviations', 'notesForWiring', 'manualChecks', 'head'] }
const REVIEW_SCHEMA = { type: 'object', properties: { verdict: { type: 'string', enum: ['approve', 'changes'] }, findings: { type: 'array', items: FINDING }, checked: { type: 'string' }, reviewedHead: { type: 'string' } }, required: ['verdict', 'findings', 'checked', 'reviewedHead'] }
const FIX_SCHEMA = { type: 'object', properties: { fixed: { type: 'array', items: { type: 'string' } }, declined: { type: 'array', items: { type: 'string' } }, deferred: { type: 'array', items: { type: 'string' } }, tests: { type: 'string' }, notesForWiring: { type: 'array', items: { type: 'string' } }, head: { type: 'string' } }, required: ['fixed', 'declined', 'deferred', 'tests', 'notesForWiring', 'head'] }
const VERIFY_SCHEMA = { type: 'object', properties: { verdict: { type: 'string', enum: ['approve', 'changes'] }, open: { type: 'array', items: FINDING }, resolved: { type: 'array', items: { type: 'string' } }, declinedOk: { type: 'array', items: { type: 'string' } }, followUps: { type: 'array', items: FINDING }, gate: { type: 'string' } }, required: ['verdict', 'open', 'resolved', 'declinedOk', 'followUps', 'gate'] }
const MERGE_SCHEMA = { type: 'object', properties: { merged: { type: 'array', items: { type: 'string' } }, gate: { type: 'string' }, integrationFixes: { type: 'array', items: { type: 'string' } }, pushed: { type: 'boolean' }, problems: { type: 'array', items: { type: 'string' } } }, required: ['merged', 'gate', 'integrationFixes', 'pushed', 'problems'] }

const results = await pipeline(
  A.tasks,
  (t, _orig, i) => agent(implPrompt(t, i), { label: `impl:${t.id}`, phase: 'Implement', model: 'sonnet', schema: IMPL_SCHEMA }),
  async (impl, t, i) => {
    const review = await agent(reviewPrompt(t, i, impl), { label: `review:${t.id}`, phase: 'Review', model: 'opus', schema: REVIEW_SCHEMA })
    const fixes = []
    let ver = null
    let approved = !!review && review.verdict === 'approve'
    if (review && !approved) {
      const base = review.reviewedHead || impl?.head || 'HEAD'
      let items = review.findings
      for (let round = 1; round <= 3; round++) {
        fixes.push(await agent(fixPrompt(t, i, items, round), { label: `fix:${t.id}:f${round}`, phase: 'Fix', model: 'sonnet', schema: FIX_SCHEMA }))
        ver = await agent(verifyPrompt(t, i, round, base, review.findings, fixes, ver?.open ?? []), { label: `verify:${t.id}:v${round}`, phase: 'Verify', model: 'opus', schema: VERIFY_SCHEMA })
        if (!ver || ver.verdict === 'approve') break
        items = (ver.open ?? []).length ? ver.open : [{ severity: 'major', file: '(gate)', problem: `The verifier did not approve. Gate: ${ver.gate}`, fix: 'Make every gate command pass.' }]
      }
      approved = !!ver && ver.verdict === 'approve'
    }
    log(`${t.id}: ${approved ? 'approved' : 'NOT approved'} after ${fixes.length} fix round(s)`)
    return { id: t.id, title: t.title, impl, review, fixes, ver, approved }
  },
)

const done = results.filter(Boolean)
const approved = done.filter((r) => r.approved)
const blocked = done.filter((r) => !r.approved)
const wiring = (r) => [...(r.impl?.notesForWiring ?? []), ...r.fixes.flatMap((f) => f?.notesForWiring ?? [])]
let merge = null
if (approved.length) {
  phase('Merge')
  merge = await agent(
    `You are the Sonnet release engineer. Merge these approved Codepass tasks into branch codepass of the main checkout ${R}, in this order: ${approved.map((r) => `${r.id} (${r.title})`).join(', ')}.
Preconditions: ${R} is on branch codepass with a clean tree (check; if not, stop and report). The owner's installed AWS SSO Hub app may be running: never quit, kill, move or trash it.
For each task: ${approved.some((r) => KEEP.has(r.id)) ? `for ${approved.filter((r) => KEEP.has(r.id)).map((r) => r.id).join(', ')} (keep history): git -C ${R} merge --no-ff --no-commit task/<id>, never --squash (the branch holds a merge with main that must stay in codepass's history; afterwards git -C ${R} log codepass..main must print nothing); for every other task: ` : ''}git -C ${R} merge --squash task/<id>; resolve any conflict (generated files under frontend/bindings/: take either side, then regenerate after all merges); make sure frontend/dist/.gitkeep stays tracked and that no .tools or node_modules link is staged; commit with the message "codepass(<id>): <title>", a blank line, then exactly these two lines:
${ATTR}
Wiring notes (for your information; implement nothing from them): ${JSON.stringify(approved.map((r) => ({ id: r.id, notesForWiring: wiring(r) })))}
After all merges: if frontend/package.json or package-lock.json changed, run npm ci in ${R}/frontend; if any Go API type or service changed, run .tools/bin/wails3 generate bindings -clean=true -ts -i in ${R} and commit "codepass: regenerate bindings" (same two trailer lines) if anything changed. Then run the gate in ${R}: go vet ./... && go test ./... ; GOOS=windows GOARCH=amd64 go build ./... ; cd frontend && npm run typecheck && npm test && npm run build && npm run check:dist ; then CODEPASS_SCRUB_LIST=${A.scrubList} bash scripts/scrub-check.sh --tracked (must exit 0; never open, print or quote the list file or the matched text). An integration failure between tasks: fix it minimally and commit "codepass: integrate <ids>" (same trailer); if it is not small, stop and report without pushing. Then git -C ${R} push origin codepass. Finally remove each merged task's worktree (git -C ${R} worktree remove --force ${WT}/<id>; rm -rf ${WT}/<id>-shots) and keep the task branches.
Return: merged (id + commit hash each), gate (results), integrationFixes, pushed, problems.`,
    { label: 'merge', phase: 'Merge', model: 'sonnet', schema: MERGE_SCHEMA },
  )
}

return {
  approved: approved.map((r) => ({ id: r.id, fixRounds: r.fixes.length, manualChecks: r.impl?.manualChecks ?? [], deviations: r.impl?.deviations ?? [], notesForWiring: wiring(r), deferred: r.fixes.flatMap((f) => f?.deferred ?? []), declined: r.fixes.flatMap((f) => f?.declined ?? []), followUps: [...(r.ver?.followUps ?? []), ...((r.fixes.length ? [] : r.review?.findings) ?? []).filter((f) => f.severity === 'nit')] })),
  blocked: blocked.map((r) => ({ id: r.id, open: r.ver?.open ?? r.review?.findings ?? [], gate: r.ver?.gate ?? '', followUps: r.ver?.followUps ?? [], fixes: r.fixes, impl: r.impl })),
  merge,
}
