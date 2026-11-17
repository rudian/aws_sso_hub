package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rudian/codepass/internal/aitools"
	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/keepawake"
	"github.com/rudian/codepass/internal/notify"
	"github.com/rudian/codepass/internal/taskstate"
)

func TestCreateStartsClaudeWithItsOwnSessionIDAndHooks(t *testing.T) {
	r := newRig(t)
	tk := r.create(api.AIToolClaude, "Fix the rounding in settlement totals")
	st := r.lastStart()
	argv := st.opts.Argv
	if st.opts.ID != tk.ID || st.opts.Dir != r.repo {
		t.Fatalf("session %q dir %q", st.opts.ID, st.opts.Dir)
	}
	sid := argAfter(argv, "--session-id")
	if len(sid) != 36 {
		t.Fatalf("the app creates the session ID: %v", argv)
	}
	if plug := argAfter(argv, "--plugin-dir"); plug != filepath.Join(r.data, "sessions", tk.ID, "claude-plugin") {
		t.Fatalf("plugin dir %q", plug)
	}
	if _, err := os.Stat(filepath.Join(r.data, "sessions", tk.ID, "claude-plugin", ".claude-plugin", "plugin.json")); err != nil {
		t.Fatalf("the plugin is written: %v", err)
	}
	if argv[len(argv)-2] != "--" || argv[len(argv)-1] != "Fix the rounding in settlement totals" {
		t.Fatalf("the description is the first message, after --: %v", argv)
	}
	if v, _ := envOf(st.opts.Env, "CODEPASS_TASK"); v != tk.ID {
		t.Fatalf("env %v", st.opts.Env)
	}
	if v, _ := envOf(st.opts.Env, "CODEPASS_SOCK"); v != "/tmp/test.sock" {
		t.Fatalf("env %v", st.opts.Env)
	}
	if _, ok := envOf(st.opts.Env, "CODEPASS_NAME_PROMPT"); ok {
		t.Fatal("a named task does not ask for its first prompt")
	}
	if tk.State != api.TaskStarting || !tk.Live || tk.Title != "Fix rounding" || tk.Slug != "fix-rounding" || tk.NamedBy != api.TaskNamedByAI {
		t.Fatalf("%+v", tk)
	}
	info, ok := r.tracker.Get(tk.ID)
	if !ok || info.Mode != taskstate.ModeHooks || info.Tool != taskstate.ToolClaude {
		t.Fatalf("tracker: %+v %v", info, ok)
	}
	if _, ok := r.procs.tracked[tk.ID]; !ok {
		t.Fatal("the process tracker follows the session")
	}
	// the record never holds the description
	b, _ := os.ReadFile(filepath.Join(r.data, "tasks.json"))
	if strings.Contains(string(b), "settlement") || !strings.Contains(string(b), sid) {
		t.Fatalf("tasks.json: %s", b)
	}
}

func TestOpenCodeAndCodexLaunchForms(t *testing.T) {
	r := newRig(t)
	oc := r.create(api.AIToolOpenCode, "Add a retry")
	st := r.lastStart()
	if !hasArg(st.opts.Argv, "--standalone") || !strings.HasPrefix(argAfter(st.opts.Argv, "--session"), "ses_") || !hasArg(st.opts.Argv, "--prompt=Add a retry") {
		t.Fatalf("%v", st.opts.Argv)
	}
	cfg, ok := envOf(st.opts.Env, "OPENCODE_CONFIG_CONTENT")
	if !ok || !strings.Contains(cfg, filepath.Join(r.data, "sessions", oc.ID, "opencode-plugin")) {
		t.Fatalf("env %v", st.opts.Env)
	}
	if i, _ := r.tracker.Get(oc.ID); i.Mode != taskstate.ModeHooks {
		t.Fatalf("%+v", i)
	}
	cx := r.create(api.AIToolCodex, "Write tests")
	st = r.lastStart()
	if hasArg(st.opts.Argv, "--session") || hasArg(st.opts.Argv, "--session-id") {
		t.Fatalf("Codex's ID is captured, not assigned: %v", st.opts.Argv)
	}
	if !strings.Contains(strings.Join(st.opts.Argv, " "), "notify=[") {
		t.Fatalf("Codex gets the notify override: %v", st.opts.Argv)
	}
	if i, _ := r.tracker.Get(cx.ID); i.Mode != taskstate.ModeTurnOnly {
		t.Fatalf("%+v", i)
	}
	if got := r.task(cx.ID); got.Resumable {
		t.Fatal("a live task is not resumable")
	}
}

func TestCodexWithItsOwnNotifyIsModeNone(t *testing.T) {
	r := newRig(t, func(d *Deps) {
		d.Launch = func(q aitools.LaunchRequest) (aitools.Launch, error) {
			q.GOOS, q.Shell, q.CodexHasNotify = "darwin", "/bin/zsh", true
			return aitools.BuildLaunch(q)
		}
	})
	cx := r.create(api.AIToolCodex, "x")
	if i, _ := r.tracker.Get(cx.ID); i.Mode != taskstate.ModeNone {
		t.Fatalf("the person's own notify stays; no signal: %+v", i)
	}
}

func TestNamingFallbackAndPendingName(t *testing.T) {
	r := newRig(t)
	r.mu.Lock()
	r.nameErr = errors.New("no answer")
	r.mu.Unlock()
	long := "Make the settlement report handle refunds that arrive after month end without double counting anything\nsecond line"
	tk := r.create(api.AIToolClaude, long)
	if tk.NamedBy != api.TaskNamedFallback || tk.Slug != "task-1" || len([]rune(tk.Title)) > 40 || !strings.HasPrefix(long, tk.Title) {
		t.Fatalf("%+v", tk)
	}
	r.mu.Lock()
	r.nameErr = nil
	r.nameResp = Naming{Title: "Quiet fallback", Slug: "task-2", FromAI: false}
	r.mu.Unlock()
	if tk := r.create(api.AIToolClaude, "something"); tk.NamedBy != api.TaskNamedFallback || tk.Title != "Quiet fallback" {
		t.Fatalf("a fallback from the namer is still a fallback: %+v", tk)
	}

	before := r.names.Load()
	empty := r.create(api.AIToolClaude, "  ")
	if r.names.Load() != before {
		t.Fatal("an empty description skips the naming call")
	}
	if empty.Title != "New task 3" || empty.Slug != "new-task-3" || empty.NamedBy != api.TaskNamedPending || empty.Number != 3 {
		t.Fatalf("%+v", empty)
	}
	st := r.lastStart()
	if v, _ := envOf(st.opts.Env, "CODEPASS_NAME_PROMPT"); v != "1" {
		t.Fatalf("the first prompt is handed over: %v", st.opts.Env)
	}
	if st.opts.Argv[len(st.opts.Argv)-1] == "" || hasArg(st.opts.Argv, "--") {
		t.Fatalf("no first message: %v", st.opts.Argv)
	}
	r.setLang("zh-CN")
	if zh := r.create(api.AIToolClaude, ""); zh.Title != "新任务 4" {
		t.Fatalf("%+v", zh)
	}

	// the first message names the label only
	r.mu.Lock()
	r.nameResp = Naming{Title: "Add retries", Slug: "add-retries", FromAI: true}
	r.mu.Unlock()
	r.report(empty.ID, taskstate.ToolClaude, "UserPromptSubmit", func(p *taskstate.Report) { p.Prompt = "add retries to the uploader" })
	eventually(t, "the label", func() bool { return r.task(empty.ID).Title == "Add retries" })
	got := r.task(empty.ID)
	if got.Slug != "new-task-3" || got.NamedBy != api.TaskNamedByAI {
		t.Fatalf("the slug (the branch) keeps its name: %+v", got)
	}
}

func TestRenameChangesTheLabelOnly(t *testing.T) {
	r := newRig(t)
	tk := r.create(api.AIToolClaude, "x")
	got, err := r.svc.Rename(tk.ID, "  My name ")
	if err != nil || got.Title != "My name" || got.Slug != "fix-rounding" || got.NamedBy != api.TaskNamedByPerson {
		t.Fatalf("%+v %v", got, err)
	}
	for _, bad := range []string{"", "   ", "a\nb", strings.Repeat("x", 81)} {
		if _, err := r.svc.Rename(tk.ID, bad); err == nil {
			t.Fatalf("%q must be refused", bad)
		}
	}
	if _, err := r.svc.Rename("nope", "x"); err == nil {
		t.Fatal("unknown task")
	}
}

func TestCreateRefusesBadRequests(t *testing.T) {
	r := newRig(t)
	cases := map[string]api.CreateTaskRequest{
		"tool":        {Repo: "repo-1", Tool: "vim"},
		"where":       {Repo: "repo-1", Tool: api.AIToolClaude, Where: api.TaskWhereOwn},
		"description": {Repo: "repo-1", Tool: api.AIToolClaude, Description: strings.Repeat("x", aitools.MaxMessageBytes+1)},
	}
	for field, req := range cases {
		_, err := r.svc.Create(context.Background(), req)
		var ae *api.AppError
		if !errors.As(err, &ae) || ae.Params["field"] != field {
			t.Fatalf("%s: %v", field, err)
		}
	}
	if _, err := r.svc.Create(context.Background(), api.CreateTaskRequest{Repo: "unknown", Tool: api.AIToolClaude}); err == nil {
		t.Fatal("unknown repository")
	}
	if r.startCount() != 0 || len(r.svc.Snapshot().Tasks) != 0 {
		t.Fatal("nothing was created")
	}
}

func TestAFailedStartLeavesNothingBehind(t *testing.T) {
	r := newRig(t, func(d *Deps) {
		d.Launch = func(aitools.LaunchRequest) (aitools.Launch, error) { return aitools.Launch{}, aitools.ErrBadMessage }
	})
	_, err := r.svc.Create(context.Background(), api.CreateTaskRequest{Repo: "repo-1", Tool: api.AIToolClaude, Description: "x"})
	var ae *api.AppError
	if !errors.As(err, &ae) || ae.Params["field"] != "description" {
		t.Fatalf("%v", err)
	}
	if len(r.svc.Snapshot().Tasks) != 0 {
		t.Fatal("no record")
	}
	entries, _ := os.ReadDir(filepath.Join(r.data, "sessions"))
	if len(entries) != 0 {
		t.Fatalf("the task's folder was removed: %v", entries)
	}
}

// ---- states, notifications, keep-awake -------------------------------------------------------------------

func TestHookStatesReachKeepAwakeAndTheList(t *testing.T) {
	r := newRig(t)
	tk := r.create(api.AIToolClaude, "x")
	if r.awake.count("set:"+tk.ID) != 0 {
		t.Fatal("starting does not count as activity")
	}
	r.report(tk.ID, taskstate.ToolClaude, "UserPromptSubmit", nil)
	if got := r.task(tk.ID); got.State != api.TaskRunning {
		t.Fatalf("%+v", got)
	}
	if st, _ := r.awake.last(tk.ID); st != keepawake.StateRunning {
		t.Fatalf("%v", r.awake.list())
	}
	r.report(tk.ID, taskstate.ToolClaude, "PreToolUse", nil)
	if got := r.task(tk.ID); got.State != api.TaskWaiting || got.Reason != api.TaskWaitQuestion {
		t.Fatalf("%+v", got)
	}
	if st, _ := r.awake.last(tk.ID); st != keepawake.StateWaiting {
		t.Fatalf("%v", r.awake.list())
	}
	// an event that changes nothing visible still renews the hard ceiling
	n := r.awake.count("touch:")
	r.report(tk.ID, taskstate.ToolClaude, "PreToolUse", nil)
	if r.awake.count("touch:") != n+1 {
		t.Fatal("Touch for every hook event")
	}
	// keep-awake hears of a state only when it changes (every call restarts its ceiling)
	sets := r.awake.count("set:")
	r.report(tk.ID, taskstate.ToolClaude, "Notification", func(p *taskstate.Report) { p.NotificationType = "elicitation_dialog" })
	if r.awake.count("set:") != sets {
		t.Fatalf("repeated state reported: %v", r.awake.list())
	}
	r.svc.Flush()
	ev, _ := r.ev.last(api.EventTasksChanged)
	ts := ev.(api.TasksState)
	if ts.Waiting != 1 || ts.Running != 0 || ts.Live != 1 || ts.Tasks[0].State != api.TaskWaiting {
		t.Fatalf("%+v", ts)
	}
}

func TestWaitingNotificationOnlyWhenTheWindowIsNotFocused(t *testing.T) {
	r := newRig(t)
	tk := r.create(api.AIToolClaude, "x")
	r.setFocused(true)
	r.report(tk.ID, taskstate.ToolClaude, "PermissionRequest", nil)
	r.svc.Flush()
	if len(r.backend.Sent()) != 0 {
		t.Fatal("the window is in front: no notification")
	}
	r.report(tk.ID, taskstate.ToolClaude, "UserPromptSubmit", nil)
	r.setFocused(false)
	r.report(tk.ID, taskstate.ToolClaude, "PermissionRequest", nil)
	r.svc.Flush()
	sent := r.backend.Sent()
	if len(sent) != 1 {
		t.Fatalf("%+v", sent)
	}
	n := sent[0]
	if n.Kind != KindTaskWaiting || n.Title != "acme_api" || n.Body != "Claude Code is waiting for your answer" || n.Silent || n.HasAction {
		t.Fatalf("%+v", n)
	}
	if n.Data["task"] != tk.ID || n.Data["repo"] != "repo-1" {
		t.Fatalf("%+v", n.Data)
	}
	// still waiting, another reason: no second notification
	r.report(tk.ID, taskstate.ToolClaude, "Stop", nil)
	r.svc.Flush()
	if len(r.backend.Sent()) != 1 {
		t.Fatalf("one notification per change to waiting: %d", len(r.backend.Sent()))
	}
	// leaving waiting takes the delivered one back
	r.report(tk.ID, taskstate.ToolClaude, "UserPromptSubmit", nil)
	r.svc.Flush()
	if rm := r.backend.Removed(); len(rm) != 1 || rm[0] != waitingID(tk.ID) {
		t.Fatalf("%v", rm)
	}
	// Chinese
	r.setLang("zh-CN")
	r.report(tk.ID, taskstate.ToolClaude, "Stop", nil)
	r.svc.Flush()
	if s := r.backend.Sent(); len(s) != 2 || s[1].Body != "Claude Code 在等你回答" {
		t.Fatalf("%+v", s)
	}
	for _, s := range r.backend.Sent() {
		if strings.Contains(s.Title+s.Body, r.repo) || strings.Contains(s.Title+s.Body, "/") {
			t.Fatalf("no path in a notification: %+v", s)
		}
	}
}

func TestClickOnTheWaitingNoticeShowsTheWindowAndOpensTheTask(t *testing.T) {
	r := newRig(t)
	tk := r.create(api.AIToolClaude, "x")
	r.report(tk.ID, taskstate.ToolClaude, "PermissionRequest", nil)
	r.svc.Flush()
	if err := r.backend.Respond(waitingID(tk.ID), notify.DefaultActionID); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the click", func() bool { _, ok := r.ev.last(api.EventTaskOpen); return ok })
	r.svc.Flush()
	ev, _ := r.ev.last(api.EventTaskOpen)
	if ev.(api.TaskOpen).Task != tk.ID || ev.(api.TaskOpen).Repo != "repo-1" || r.shown.Load() != 1 {
		t.Fatalf("%+v shown %d", ev, r.shown.Load())
	}
}

func TestInputThatCannotAnswerLeavesTheTaskWaiting(t *testing.T) {
	r := newRig(t)
	tk := r.create(api.AIToolClaude, "x")
	r.report(tk.ID, taskstate.ToolClaude, "PermissionRequest", nil)
	touches := r.awake.count("touch:")
	for _, b := range []string{"\x1b[I", "\x1b[O", "\x1b[12;40R", "\x1b[?62;4c", "\x1b]11;rgb:0000/0000/0000\x07", "\x1b[<0;10;5M"} {
		r.svc.Input(tk.ID, []byte(b))
	}
	if got := r.task(tk.ID); got.State != api.TaskWaiting {
		t.Fatalf("focus reports and terminal replies do not answer: %+v", got)
	}
	if r.awake.count("touch:") != touches {
		t.Fatal("nor do they renew keep-awake's ceiling")
	}
	r.svc.Input(tk.ID, []byte("\x1b[B")) // an arrow key is the person's, but it answers nothing
	if got := r.task(tk.ID); got.State != api.TaskWaiting {
		t.Fatalf("an arrow key does not answer: %+v", got)
	}
	if r.awake.count("touch:") != touches+1 {
		t.Fatal("but it is input from the owner for the ceiling")
	}
	touches++
	r.svc.Input(tk.ID, []byte("1"))
	if got := r.task(tk.ID); got.State != api.TaskRunning {
		t.Fatalf("a digit answers: %+v", got)
	}
	if r.awake.count("touch:") != touches+1 {
		t.Fatal("the answer renews the ceiling")
	}
	r.svc.Input("unknown", []byte("\r")) // no panic
}

func TestTerminalWritesReachInput(t *testing.T) {
	r := newRig(t)
	r.mgr.SetOnInput(r.svc.Input)
	tk := r.create(api.AIToolClaude, "x")
	r.report(tk.ID, taskstate.ToolClaude, "PermissionRequest", nil)
	_ = r.mgr.Write(tk.ID, []byte("\x1b[I"))
	if r.task(tk.ID).State != api.TaskWaiting {
		t.Fatal("a focus report")
	}
	_ = r.mgr.Write(tk.ID, []byte("\r"))
	if r.task(tk.ID).State != api.TaskRunning {
		t.Fatal("Enter")
	}
}

func TestSessionIDsAreCrossCheckedAndCodexIsCaptured(t *testing.T) {
	r := newRig(t)
	cl := r.create(api.AIToolClaude, "x")
	first := argAfter(r.lastStart().opts.Argv, "--session-id")
	r.report(cl.ID, taskstate.ToolClaude, "SessionStart", func(p *taskstate.Report) { p.Session = first; p.Source = "startup" })
	r.report(cl.ID, taskstate.ToolClaude, "SessionStart", func(p *taskstate.Report) { p.Session = "cafe0001-1111-4222-8333-0000000000b1"; p.Source = "clear" })
	b, _ := os.ReadFile(filepath.Join(r.data, "tasks.json"))
	eventually(t, "the new ID on disk", func() bool {
		b, _ = os.ReadFile(filepath.Join(r.data, "tasks.json"))
		return strings.Contains(string(b), "cafe0001-1111-4222-8333-0000000000b1")
	})
	var d fileData
	_ = json.Unmarshal(b, &d)
	if len(d.Tasks[0].SessionIDs) != 2 || d.Tasks[0].SessionIDs[0] != first {
		t.Fatalf("%+v", d.Tasks[0].SessionIDs)
	}

	oc := r.create(api.AIToolOpenCode, "x")
	r.report(oc.ID, taskstate.ToolOpenCode, "session.execution.started", func(p *taskstate.Report) { p.Session = "ses_child" })
	r.svc.Flush()
	r.svc.mu.Lock()
	ids := append([]string(nil), r.svc.recs[oc.ID].SessionIDs...)
	r.svc.mu.Unlock()
	if len(ids) != 1 || ids[0] == "ses_child" {
		t.Fatalf("OpenCode keeps the ID the app created: %v", ids)
	}

	cx := r.create(api.AIToolCodex, "x")
	if r.task(cx.ID).Resumable {
		t.Fatal("not live-resumable")
	}
	r.report(cx.ID, taskstate.ToolCodex, taskstate.EventCodexTurnComplete, func(p *taskstate.Report) { p.Session = "cafe0001-1111-4222-8333-0000000000b2" })
	if _, err := r.svc.Stop(cx.ID); err != nil {
		t.Fatal(err)
	}
	if !r.task(cx.ID).Resumable {
		t.Fatal("the captured ID makes it resumable")
	}
	if _, err := r.svc.Resume(cx.ID, false); err != nil {
		t.Fatal(err)
	}
	argv := r.lastStart().opts.Argv
	if argAfter(argv, "resume") != "cafe0001-1111-4222-8333-0000000000b2" {
		t.Fatalf("%v", argv)
	}
}

// ---- ending ----------------------------------------------------------------------------------------------

func TestExitCodesGiveDoneAndFailed(t *testing.T) {
	r := newRig(t)
	ok := r.create(api.AIToolClaude, "x")
	r.lastStart().pty.exit(0)
	eventually(t, "done", func() bool { return r.task(ok.ID).State == api.TaskDone })
	bad := r.create(api.AIToolClaude, "y")
	r.lastStart().pty.exit(2)
	eventually(t, "failed", func() bool { return r.task(bad.ID).State == api.TaskFailed })
	got := r.task(bad.ID)
	if got.Live || got.ExitCode == nil || *got.ExitCode != 2 || got.EndedAt == "" || !got.Resumable {
		t.Fatalf("%+v", got)
	}
	if info, _ := r.tracker.Get(bad.ID); info.State != taskstate.StateEnded {
		t.Fatalf("the tracker's end is final: %+v", info)
	}
	if r.procs.closed[bad.ID] != 1 {
		t.Fatal("the process tracker is told the session closed, once")
	}
	if st, _ := r.awake.last(bad.ID); st != keepawake.StateEnded {
		t.Fatalf("%v", r.awake.list())
	}
}

func TestStopMarksTheTaskStopped(t *testing.T) {
	r := newRig(t)
	tk := r.create(api.AIToolClaude, "x")
	f := r.lastStart().pty
	got, err := r.svc.Stop(tk.ID)
	if err != nil || got.State != api.TaskStopped || got.Live || got.ExitCode != nil {
		t.Fatalf("%+v %v", got, err)
	}
	if f.killed.Load() != 1 {
		t.Fatal("the terminal session is killed")
	}
	if again, err := r.svc.Stop(tk.ID); err != nil || again.State != api.TaskStopped {
		t.Fatalf("Stop is idempotent: %+v %v", again, err)
	}
	if _, err := r.svc.Stop("nope"); err == nil {
		t.Fatal("unknown task")
	}
}

func TestClosedStillRunningFollowsTheProcessTracker(t *testing.T) {
	r := newRig(t)
	tk := r.create(api.AIToolClaude, "x")
	r.lastStart().pty.exit(0)
	eventually(t, "done", func() bool { return r.task(tk.ID).State == api.TaskDone })
	r.procs.setLeftovers(tk.ID, 4, false)
	r.svc.ProcChanged()
	got := r.task(tk.ID)
	if got.State != api.TaskClosedRunning || got.Leftovers.Count != 4 || got.Leftovers.Others != 2 || got.Leftovers.Memory != 1<<20 || got.Leftovers.ClosedAt == "" {
		t.Fatalf("%+v", got)
	}
	if !r.svc.TasksRunning() {
		t.Fatal("a closed session with programs keeps the window-close hook on hide")
	}
	ls := r.svc.LiveSummary()
	if ls.Closed != 1 || ls.Live != 0 || ls.Programs != 2 || len(ls.Sessions) != 1 || ls.Sessions[0].RepoName != "acme_api" || ls.Sessions[0].State != api.TaskClosedRunning {
		t.Fatalf("%+v", ls)
	}
	st := r.svc.Snapshot()
	if st.Closed != 1 || st.Programs != 4 {
		t.Fatalf("%+v", st)
	}
	r.procs.setLeftovers(tk.ID, 0, false)
	r.svc.ProcChanged()
	if got := r.task(tk.ID); got.State != api.TaskDone {
		t.Fatalf("back to its own end: %+v", got)
	}
	if r.svc.TasksRunning() {
		t.Fatal("nothing runs")
	}
	if _, err := r.svc.Stop(tk.ID); err != nil {
		t.Fatal(err)
	}
}

func TestRestartedAppTellsKeepAwakeAboutRestoredLeftovers(t *testing.T) {
	r := newRig(t)
	tk := r.create(api.AIToolClaude, "x")
	r.lastStart().pty.exit(0)
	eventually(t, "done", func() bool { return r.task(tk.ID).State == api.TaskDone })

	r.awake, r.procs = &fakeAwake{}, newFakeProcs()
	r.build() // the app restarts; the process tracker restored a survivor from leftovers.json
	r.procs.setLeftovers(tk.ID, 3, false)
	r.svc.ProcChanged()
	if r.task(tk.ID).State != api.TaskClosedRunning {
		t.Fatal("a stopped task with a surviving program is closed, still running")
	}
	if st, ok := r.awake.last(tk.ID); !ok || st != keepawake.StateEnded {
		t.Fatalf("keep-awake counts the survivors: %v", r.awake.list())
	}
	if !r.svc.TasksRunning() {
		t.Fatal("cached flag")
	}
}

// Races between exit, stop and state events: whichever wins, a run ends once and the record is consistent.
func TestExitStopAndStateEventsRace(t *testing.T) {
	for i := 0; i < 60; i++ {
		r := newRig(t)
		tk := r.create(api.AIToolClaude, "x")
		f := r.lastStart().pty
		var wg sync.WaitGroup
		wg.Add(4)
		go func() { defer wg.Done(); f.exit(0) }()
		go func() { defer wg.Done(); _, _ = r.svc.Stop(tk.ID) }()
		go func() {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				r.report(tk.ID, taskstate.ToolClaude, "PermissionRequest", nil)
				r.report(tk.ID, taskstate.ToolClaude, "UserPromptSubmit", nil)
			}
		}()
		go func() { defer wg.Done(); r.svc.Input(tk.ID, []byte("\r")); _ = r.svc.Snapshot() }()
		wg.Wait()
		eventually(t, "the end", func() bool {
			st, _ := r.awake.last(tk.ID)
			return !r.task(tk.ID).Live && st == keepawake.StateEnded
		})
		// finalize has returned once its busy mark is gone: it may still queue work until then
		eventually(t, "finalize returned", func() bool {
			r.svc.mu.Lock()
			defer r.svc.mu.Unlock()
			return r.svc.busy[tk.ID] == 0
		})
		r.svc.Flush()
		got := r.task(tk.ID)
		if got.State != api.TaskDone && got.State != api.TaskStopped {
			t.Fatalf("iteration %d: %+v", i, got)
		}
		if r.procs.closed[tk.ID] != 1 {
			t.Fatalf("iteration %d: closed %d times", i, r.procs.closed[tk.ID])
		}
		if info, _ := r.tracker.Get(tk.ID); info.State != taskstate.StateEnded {
			t.Fatalf("iteration %d: tracker %+v", i, info)
		}
		if st, _ := r.awake.last(tk.ID); st != keepawake.StateEnded {
			t.Fatalf("iteration %d: a late state event must not revive keep-awake: %v", i, r.awake.list())
		}
		if r.svc.TasksRunning() {
			t.Fatalf("iteration %d: still running", i)
		}
	}
}

func TestLateStateEventsAfterTheEndAreIgnored(t *testing.T) {
	r := newRig(t)
	tk := r.create(api.AIToolClaude, "x")
	r.report(tk.ID, taskstate.ToolClaude, "UserPromptSubmit", nil)
	r.lastStart().pty.exit(0)
	eventually(t, "done", func() bool { return r.task(tk.ID).State == api.TaskDone })
	eventually(t, "keep-awake told", func() bool { st, _ := r.awake.last(tk.ID); return st == keepawake.StateEnded })
	sets := r.awake.count("set:")
	r.report(tk.ID, taskstate.ToolClaude, "PermissionRequest", nil) // a hook process that outlived the tool
	r.svc.OnState(taskstate.Info{Task: tk.ID, Tool: taskstate.ToolClaude, State: taskstate.StateWaiting, Reason: taskstate.ReasonPermission})
	if r.awake.count("set:") != sets || r.task(tk.ID).State != api.TaskDone {
		t.Fatalf("%v %+v", r.awake.list(), r.task(tk.ID))
	}
	r.svc.Flush()
	if len(r.backend.Sent()) != 0 {
		t.Fatal("no notification for a task that ended")
	}
}

// ---- cap -------------------------------------------------------------------------------------------------

func TestSoftCapAsksBeforeAnotherTask(t *testing.T) {
	r := newRig(t)
	r.cap.Store(2)
	r.create(api.AIToolClaude, "a")
	r.create(api.AIToolClaude, "b")
	_, err := r.svc.Create(context.Background(), api.CreateTaskRequest{Repo: "repo-1", Tool: api.AIToolClaude, Description: "c"})
	var ae *api.AppError
	if !errors.As(err, &ae) || ae.Code != api.ErrInvalidInput || ae.Params["field"] != api.TaskFieldCap || ae.Params["running"] != "2" || ae.Params["cap"] != "2" {
		t.Fatalf("%v", err)
	}
	if r.startCount() != 2 {
		t.Fatal("nothing started")
	}
	c, err := r.svc.Create(context.Background(), api.CreateTaskRequest{Repo: "repo-1", Tool: api.AIToolClaude, Description: "c", Confirmed: true})
	if err != nil || !c.Live || r.svc.Snapshot().Live != 3 || r.svc.Snapshot().Cap != 2 {
		t.Fatalf("%+v %v", c, err)
	}
	// an ended task does not count; a resume counts
	if _, err := r.svc.Stop(c.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.Resume(c.ID, false); err == nil {
		t.Fatal("two run, the cap is two: a resume asks as well")
	}
	if _, err := r.svc.Resume(c.ID, true); err != nil {
		t.Fatal(err)
	}
}

func TestSoftCapHoldsUnderConcurrentStarts(t *testing.T) {
	r := newRig(t)
	r.cap.Store(3)
	var ok, asked atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := r.svc.Create(context.Background(), api.CreateTaskRequest{Repo: "repo-1", Tool: api.AIToolClaude, Description: "x"})
			if err == nil {
				ok.Add(1)
			} else {
				asked.Add(1)
			}
		}()
	}
	wg.Wait()
	if ok.Load() != 3 || asked.Load() != 9 {
		t.Fatalf("started %d asked %d", ok.Load(), asked.Load())
	}
}

func TestDefaultSoftCapIsSix(t *testing.T) {
	r := newRig(t, func(d *Deps) { d.SoftCap = nil })
	for i := 0; i < 6; i++ {
		r.create(api.AIToolClaude, "x")
	}
	if _, err := r.svc.Create(context.Background(), api.CreateTaskRequest{Repo: "repo-1", Tool: api.AIToolClaude}); err == nil {
		t.Fatal("the seventh asks")
	}
	if r.svc.Snapshot().Cap != DefaultSoftCap || DefaultSoftCap != 6 {
		t.Fatal("cap")
	}
}

// ---- restart recovery ------------------------------------------------------------------------------------

func TestRestartRecoveryAndResumeIntoTheOwnSession(t *testing.T) {
	r := newRig(t)
	a := r.create(api.AIToolClaude, "first")
	sidA := argAfter(r.lastStart().opts.Argv, "--session-id")
	b := r.create(api.AIToolOpenCode, "second")
	sidB := argAfter(r.lastStart().opts.Argv, "--session")
	r.report(a.ID, taskstate.ToolClaude, "UserPromptSubmit", nil)
	r.svc.Flush()
	// the app goes away without stopping anything (a crash): the file says "live"
	r.build()
	if r.tracker == nil {
		t.Fatal("rebuilt")
	}
	list := r.svc.Snapshot()
	if len(list.Tasks) != 2 || list.Live != 0 || list.Running != 0 {
		t.Fatalf("%+v", list)
	}
	for _, tk := range list.Tasks {
		if tk.State != api.TaskStopped || tk.Live || !tk.Resumable {
			t.Fatalf("tasks reappear as stopped: %+v", tk)
		}
	}
	if r.svc.TasksRunning() {
		t.Fatal("nothing runs after a restart")
	}
	got, err := r.svc.Resume(a.ID, false)
	if err != nil || !got.Live {
		t.Fatalf("%+v %v", got, err)
	}
	argv := r.lastStart().opts.Argv
	if argAfter(argv, "--resume") != sidA || hasArg(argv, "--session-id") || hasArg(argv, "--") {
		t.Fatalf("resume into its own conversation, with no new first message: %v", argv)
	}
	if r.lastStart().opts.ID != a.ID {
		t.Fatal("the same terminal session ID")
	}
	if _, err := r.svc.Resume(b.ID, false); err != nil {
		t.Fatal(err)
	}
	if argAfter(r.lastStart().opts.Argv, "--session") != sidB {
		t.Fatalf("%v", r.lastStart().opts.Argv)
	}
	if _, err := r.svc.Resume(b.ID, false); err == nil {
		t.Fatal("a running task is not resumed")
	}
	if info, _ := r.tracker.Get(a.ID); info.State != taskstate.StateStarting {
		t.Fatalf("registered again: %+v", info)
	}
	// numbering goes on
	if c := r.create(api.AIToolClaude, ""); c.Number != 3 {
		t.Fatalf("%+v", c)
	}
}

func TestResumeKeepsTheNewestSessionID(t *testing.T) {
	r := newRig(t)
	tk := r.create(api.AIToolClaude, "x")
	r.report(tk.ID, taskstate.ToolClaude, "SessionStart", func(p *taskstate.Report) { p.Session = "cafe0001-1111-4222-8333-0000000000aa"; p.Source = "clear" })
	if _, err := r.svc.Stop(tk.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.Resume(tk.ID, false); err != nil {
		t.Fatal(err)
	}
	if argAfter(r.lastStart().opts.Argv, "--resume") != "cafe0001-1111-4222-8333-0000000000aa" {
		t.Fatalf("%v", r.lastStart().opts.Argv)
	}
	r.report(tk.ID, taskstate.ToolClaude, "SessionStart", func(p *taskstate.Report) { p.Session = "cafe0001-1111-4222-8333-0000000000aa"; p.Source = "resume" })
	if got := r.task(tk.ID); !got.Resumed {
		t.Fatalf("the tool said it really resumed: %+v", got)
	}
}

func TestDamagedTasksFileIsMovedAsideAndNewerOneIsLeftAlone(t *testing.T) {
	r := newRig(t)
	r.create(api.AIToolClaude, "x")
	r.svc.Close()
	path := filepath.Join(r.data, "tasks.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	r.build()
	if len(r.svc.Snapshot().Tasks) != 0 {
		t.Fatal("a fresh start")
	}
	entries, _ := os.ReadDir(r.data)
	moved := false
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "tasks.json.") {
			moved = true
		}
	}
	if !moved {
		t.Fatalf("the damaged file is kept aside: %v", entries)
	}
	// a record this app did not write is skipped
	newer := `{"schemaVersion":1,"tasks":[{"id":"../evil","repo":"r","tool":"claude","status":"live"},{"id":"t-ok","repo":"r","tool":"claude","status":"live","sessionIds":["s"]}],"counters":{}}`
	if err := os.WriteFile(path, []byte(newer), 0o600); err != nil {
		t.Fatal(err)
	}
	r.build()
	ts := r.svc.Snapshot().Tasks
	if len(ts) != 1 || ts[0].ID != "t-ok" || ts[0].State != api.TaskStopped {
		t.Fatalf("%+v", ts)
	}
}

// ---- stop all --------------------------------------------------------------------------------------------

func TestStopAllStopsEveryTaskThenForceStopsLeftovers(t *testing.T) {
	r := newRig(t)
	var order []string
	r.procs.orderLog = &order
	var ids []string
	var ptys []*fakePTY
	for i := 0; i < 3; i++ {
		tk := r.create(api.AIToolClaude, "x")
		ids = append(ids, tk.ID)
		ptys = append(ptys, r.lastStart().pty)
	}
	res := r.svc.StopAll()
	if res.Stopped != 3 || res.Survivors != 0 || res.Error != "" {
		t.Fatalf("%+v", res)
	}
	for i, f := range ptys {
		if f.killed.Load() != 1 {
			t.Fatalf("task %d: killed %d times", i, f.killed.Load())
		}
		if got := r.task(ids[i]); got.State != api.TaskStopped || got.Live {
			t.Fatalf("%+v", got)
		}
	}
	if r.procs.forceAll != 1 {
		t.Fatalf("ForceStopAll ran %d times", r.procs.forceAll)
	}
	if r.svc.TasksRunning() {
		t.Fatal("nothing runs")
	}
	b, _ := os.ReadFile(filepath.Join(r.data, "tasks.json"))
	var d fileData
	_ = json.Unmarshal(b, &d)
	for _, tk := range d.Tasks {
		if tk.Status != statusStopped {
			t.Fatalf("saved as %s", tk.Status)
		}
	}
	// running again is possible
	if _, err := r.svc.Resume(ids[0], false); err != nil {
		t.Fatal(err)
	}
}

func TestStopAllReportsSurvivors(t *testing.T) {
	r := newRig(t)
	r.create(api.AIToolClaude, "x")
	r.procs.survivor = 2
	if res := r.svc.StopAll(); res.Survivors != 2 {
		t.Fatalf("%+v", res)
	}
}

func TestStopAllWithNothingRunning(t *testing.T) {
	r := newRig(t)
	if res := r.svc.StopAll(); res.Stopped != 0 {
		t.Fatalf("%+v", res)
	}
}

// ---- the window-close hook and notice --------------------------------------------------------------------

func TestTasksRunningIsACachedFlag(t *testing.T) {
	r := newRig(t)
	if r.svc.TasksRunning() {
		t.Fatal("nothing runs")
	}
	tk := r.create(api.AIToolClaude, "x")
	if !r.svc.TasksRunning() {
		t.Fatal("a task runs")
	}
	r.lastStart().pty.exit(0)
	eventually(t, "ended", func() bool { return !r.svc.TasksRunning() })
	_ = tk
	r.procs.setLeftovers(tk.ID, 2, false)
	r.svc.ProcChanged()
	if !r.svc.TasksRunning() {
		t.Fatal("programs left behind")
	}
}

func TestStillRunningNoticeIsSentOnceEver(t *testing.T) {
	r := newRig(t)
	if r.svc.WindowHidden() {
		t.Fatal("no task runs: closing the window quits")
	}
	r.create(api.AIToolClaude, "x")
	r.create(api.AIToolCodex, "y")
	if !r.svc.WindowHidden() {
		t.Fatal("first time")
	}
	r.svc.Flush()
	s := r.backend.Sent()
	if len(s) != 1 || s[0].Kind != KindStillRunning || s[0].Title != "Codepass is still running" || s[0].Body != "2 AI tasks were not interrupted" || !s[0].Silent || s[0].HasAction {
		t.Fatalf("%+v", s)
	}
	if r.svc.WindowHidden() {
		t.Fatal("once")
	}
	if err := r.backend.Respond("app.stillrunning", notify.DefaultActionID); err != nil {
		t.Fatal(err)
	}
	eventually(t, "show", func() bool { return r.shown.Load() == 1 })
	// ever: also after a restart
	r.build()
	r.create(api.AIToolClaude, "z")
	if r.svc.WindowHidden() {
		t.Fatal("once ever, across restarts")
	}
}

func TestStillRunningNoticeTexts(t *testing.T) {
	zh := StillRunningNotice("zh-CN", 2)
	if zh.Title != "Codepass 还在运行" || zh.Body != "2 个 AI 任务没有中断" || !zh.Silent {
		t.Fatalf("%+v", zh)
	}
	if one := StillRunningNotice("en", 1); one.Body != "1 AI task was not interrupted" {
		t.Fatalf("%+v", one)
	}
	w := WaitingNotice("zh-CN", "t", "r", "acme_api", aitools.Codex)
	if w.Title != "acme_api" || w.Body != "Codex 在等你回答" || w.Silent {
		t.Fatalf("%+v", w)
	}
}

// ---- the fallback poller ---------------------------------------------------------------------------------

func TestFallbackProbeFeedsTheTracker(t *testing.T) {
	var verdict atomic.Int32
	var probes atomic.Int32
	r := newRig(t, func(d *Deps) {
		d.PollEvery = 2 * time.Millisecond
		d.Probe = func(taskstate.Terminal) taskstate.Verdict { probes.Add(1); return taskstate.Verdict(verdict.Load()) }
	})
	verdict.Store(int32(taskstate.Alive))
	opencode := r.create(api.AIToolOpenCode, "x")
	_ = opencode
	eventually(t, "probes", func() bool { return probes.Load() > 3 })
	// a hooks task that sent nothing shows no signal only after the grace (NoSignalAfter is an hour here)
	if got := r.task(opencode.ID); got.State != api.TaskStarting {
		t.Fatalf("%+v", got)
	}
	// the shell exits with the tool: the exit watcher ends the task
	r.lastStart().pty.exit(0)
	eventually(t, "done", func() bool { return r.task(opencode.ID).State == api.TaskDone })
}

func TestSnapshotCountsAndEvents(t *testing.T) {
	r := newRig(t)
	a := r.create(api.AIToolClaude, "a")
	b := r.create(api.AIToolClaude, "b")
	c := r.create(api.AIToolCodex, "c")
	r.report(a.ID, taskstate.ToolClaude, "UserPromptSubmit", nil)
	r.report(b.ID, taskstate.ToolClaude, "Stop", nil)
	_ = c
	st := r.svc.Snapshot()
	if st.Running != 2 || st.Waiting != 1 || st.Live != 3 || len(st.Tasks) != 3 {
		t.Fatalf("%+v", st)
	}
	r.svc.Flush()
	if _, ok := r.ev.last(api.EventTasksChanged); !ok {
		t.Fatal("tasks:changed")
	}
}

func TestRelayDropsChangesBeforeItIsConnected(t *testing.T) {
	var rl Relay
	rl.OnChange(taskstate.Info{}) // no panic
	var got atomic.Int32
	rl.Connect(func(taskstate.Info) { got.Add(1) })
	rl.OnChange(taskstate.Info{})
	if got.Load() != 1 {
		t.Fatal("connected")
	}
}

func TestSessionsFolderIsExcludedFromBackup(t *testing.T) {
	var dir atomic.Value
	r := newRig(t, func(d *Deps) { d.ExcludeBackup = func(p string) error { dir.Store(p); return nil } })
	r.svc.Flush()
	if dir.Load() != filepath.Join(r.data, "sessions") {
		t.Fatalf("%v", dir.Load())
	}
	if st, err := os.Stat(filepath.Join(r.data, "sessions")); err != nil || st.Mode().Perm() != 0o700 {
		t.Fatalf("%v %v", st, err)
	}
}

func TestNoTerminalBytesOrDescriptionInTheLog(t *testing.T) {
	var buf strings.Builder
	r := newRig(t, func(d *Deps) { d.Log = newTestLogger(&buf) })
	tk := r.create(api.AIToolClaude, "super secret description")
	r.lastStart().pty.feed("secret terminal text")
	_, _ = r.svc.Stop(tk.ID)
	r.svc.Flush()
	if strings.Contains(buf.String(), "secret") {
		t.Fatalf("log: %s", buf.String())
	}
}

type procCount struct{ p *fakeProcs }

func (c procCount) Count(session string) int { return c.p.Leftovers(session).Count }

// Accept: keep-awake is released only when the last task and the last leftover process are gone.
func TestKeepAwakeIsReleasedOnlyWhenTheLastTaskAndTheLastLeftoverAreGone(t *testing.T) {
	h, clk := &keepawake.FakeHolder{}, keepawake.NewFakeClock()
	procs := newFakeProcs()
	r := newRig(t, func(d *Deps) {
		m := keepawake.New(keepawake.Options{Holders: []keepawake.Holder{h}, Clock: clk, Leftovers: procCount{procs}, Config: keepawake.DefaultConfig()})
		t.Cleanup(m.Close)
		d.Awake, d.Procs = m, procs
	})
	held := func() bool { b, _ := h.Held(); return b }
	a := r.create(api.AIToolClaude, "a")
	fa := r.lastStart().pty
	b := r.create(api.AIToolClaude, "b")
	fb := r.lastStart().pty
	r.report(a.ID, taskstate.ToolClaude, "UserPromptSubmit", nil)
	r.report(b.ID, taskstate.ToolClaude, "UserPromptSubmit", nil)
	if !held() {
		t.Fatal("two tasks run: held")
	}
	fa.exit(0)
	eventually(t, "a ended", func() bool { return r.task(a.ID).State == api.TaskDone })
	if !held() {
		t.Fatal("one task still runs")
	}
	procs.setLeftovers(b.ID, 3, true) // its dev server
	fb.exit(0)
	eventually(t, "b ended", func() bool { return !r.task(b.ID).Live })
	procs.setLeftovers(b.ID, 3, false)
	r.svc.ProcChanged()
	if !held() {
		t.Fatal("the last task ended but its programs run: still held")
	}
	procs.setLeftovers(b.ID, 0, false)
	r.svc.ProcChanged()
	eventually(t, "released", func() bool { return !held() })
}
