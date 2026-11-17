package tasks

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/keepawake"
	"github.com/rudian/codepass/internal/proctrack"
	"github.com/rudian/codepass/internal/taskstate"
	"github.com/rudian/codepass/internal/terminal"
)

// gateTerminals parks a Start until the test lets it go.
type gateTerminals struct {
	Terminals
	on      atomic.Bool
	entered chan struct{}
	release chan struct{}
	calls   atomic.Int32
}

func (g *gateTerminals) Start(o terminal.StartOptions) (*terminal.Session, error) {
	if g.on.Load() {
		g.calls.Add(1)
		g.entered <- struct{}{}
		<-g.release
	}
	return g.Terminals.Start(o)
}

func newGate() *gateTerminals {
	return &gateTerminals{entered: make(chan struct{}, 8), release: make(chan struct{})}
}

func (r *rig) endTask(id string) {
	r.t.Helper()
	r.lastStart().pty.exit(0)
	eventually(r.t, "the task ended", func() bool { g := r.task(id); return !g.Live && g.State != api.TaskStarting })
	r.waitSettled(id)
}

// waitSettled waits until finalize has finished with the task (its busy mark is gone), so a Flush after it
// cannot overlap finalize's own background jobs.
func (r *rig) waitSettled(id string) {
	r.t.Helper()
	eventually(r.t, "finalize done", func() bool {
		r.svc.mu.Lock()
		defer r.svc.mu.Unlock()
		return r.svc.busy[id] == 0
	})
}

// ---- Resume is serialized per task -----------------------------------------------------------------------

func TestTwoConcurrentResumesStartOnlyOneRun(t *testing.T) {
	g := newGate()
	r := newRig(t, func(d *Deps) { g.Terminals = d.Terminals; d.Terminals = g })
	tk := r.create(api.AIToolClaude, "x")
	r.endTask(tk.ID)
	r.svc.Flush()
	g.on.Store(true)
	first := make(chan error, 1)
	go func() { _, err := r.svc.Resume(tk.ID, false); first <- err }()
	select {
	case <-g.entered: // the first Resume is parked inside Start
	case err := <-first:
		t.Fatalf("the first Resume did not reach Start: %v", err)
	}
	_, err := r.svc.Resume(tk.ID, false)
	if err == nil {
		t.Fatal("a second Resume while one is starting must be refused")
	}
	if g.calls.Load() != 1 {
		t.Fatalf("the second Resume must not reach the terminal manager: %d starts", g.calls.Load())
	}
	close(g.release)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	r.report(tk.ID, taskstate.ToolClaude, "UserPromptSubmit", nil)
	if got := r.task(tk.ID); got.State != api.TaskRunning {
		t.Fatalf("the live run keeps its registration: %+v", got)
	}
	if _, ok := r.tracker.Get(tk.ID); !ok {
		t.Fatal("the tracker still knows the task")
	}
	if st, _ := r.awake.last(tk.ID); st != keepawake.StateRunning {
		t.Fatalf("keep-awake follows the new run: %v", r.awake.list())
	}
}

func TestResumeInTheGapOfAFinalizeWaitsForTheEnd(t *testing.T) {
	g := newGate()
	r := newRig(t, func(d *Deps) { g.Terminals = d.Terminals; d.Terminals = g })
	tk := r.create(api.AIToolClaude, "x")
	parked, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	r.awake.setOnSet(func(id string, st keepawake.TaskState) {
		if st == keepawake.StateEnded {
			once.Do(func() { close(parked); <-release })
		}
	})
	starts := r.startCount()
	r.lastStart().pty.exit(0)
	<-parked // finalize removed the run and is about to tell keep-awake
	done := make(chan error, 1)
	go func() { _, err := r.svc.Resume(tk.ID, false); done <- err }()
	select {
	case err := <-done:
		t.Fatalf("a Resume between the removal of the run and its final end must wait for it: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	if r.startCount() != starts {
		t.Fatal("no new run before the old one ended")
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatalf("after the end it resumes: %v", err)
	}
	// the old run's final end was not applied to the new registration
	r.report(tk.ID, taskstate.ToolClaude, "UserPromptSubmit", nil)
	if got := r.task(tk.ID); got.State != api.TaskRunning {
		t.Fatalf("%+v", got)
	}
	if st, _ := r.awake.last(tk.ID); st != keepawake.StateRunning {
		t.Fatalf("%v", r.awake.list())
	}
}

func TestAFailedResumeLeavesTheTaskStopped(t *testing.T) {
	g := newGate()
	r := newRig(t, func(d *Deps) { g.Terminals = d.Terminals; d.Terminals = g })
	tk := r.create(api.AIToolClaude, "x")
	r.endTask(tk.ID)
	r.report(tk.ID, taskstate.ToolClaude, "SessionStart", func(p *taskstate.Report) { p.Source = "resume" })
	// a start that fails: the manager refuses an ID it still runs
	g.Terminals = failTerminals{g.Terminals}
	if _, err := r.svc.Resume(tk.ID, false); err == nil {
		t.Fatal("expected the start to fail")
	}
	got := r.task(tk.ID)
	if got.State != api.TaskDone || !got.Resumable || got.Live {
		t.Fatalf("a failed Resume changes nothing: %+v", got)
	}
}

type failTerminals struct{ Terminals }

func (failTerminals) Start(terminal.StartOptions) (*terminal.Session, error) {
	return nil, os.ErrPermission
}

// ---- Stop and exit do not flash "closed, still running" --------------------------------------------------

func TestStopAndExitTickTheProcessTrackerBeforeTheTaskIsShown(t *testing.T) {
	r := newRig(t)
	tk := r.create(api.AIToolClaude, "x")
	// the session's programs are still in the set when the session closes; only the next tick drops the dead ones
	r.procs.setLeftovers(tk.ID, 2, false)
	r.procs.onTick = func(p *fakeProcs) { delete(p.reports, tk.ID) }
	got, err := r.svc.Stop(tk.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != api.TaskStopped || got.Leftovers.Count != 0 {
		t.Fatalf("Stop must not return closed-running for programs that died with the session: %+v", got)
	}
	if r.svc.TasksRunning() {
		t.Fatal("nothing runs")
	}
	if ls := r.svc.LiveSummary(); ls.Closed != 0 {
		t.Fatalf("%+v", ls)
	}
	// the same for an exit
	b := r.create(api.AIToolClaude, "y")
	r.procs.setLeftovers(b.ID, 2, false)
	r.procs.onTick = func(p *fakeProcs) { delete(p.reports, b.ID) }
	r.lastStart().pty.exit(0)
	eventually(t, "done", func() bool { return r.task(b.ID).State == api.TaskDone })
	r.waitSettled(b.ID)
	eventually(t, "nothing runs", func() bool { return !r.svc.TasksRunning() })
	if r.svc.TasksRunning() {
		t.Fatal("nothing runs after an exit")
	}
}

// ---- a new task is never drawn as stopped ----------------------------------------------------------------

func TestNewTaskIsNeverSentAsStopped(t *testing.T) {
	r := newRig(t)
	r.cap.Store(100)
	for i := 0; i < 20; i++ {
		r.create(api.AIToolClaude, "x")
	}
	r.svc.Flush()
	r.ev.mu.Lock()
	defer r.ev.mu.Unlock()
	for _, e := range r.ev.l {
		ts, ok := e.data.(api.TasksState)
		if !ok {
			continue
		}
		for _, tk := range ts.Tasks {
			if tk.State == api.TaskStopped || tk.Resumable {
				t.Fatalf("a task that starts is never stopped or resumable: %+v", tk)
			}
		}
	}
}

func TestStartingRecordWithoutARunIsStartingNotResumable(t *testing.T) {
	rec := &record{ID: "t-1", Status: statusLive, SessionIDs: []string{"s"}}
	st, _ := derive(rec, false, taskstate.Info{}, false, proctrack.Report{})
	if st != api.TaskStarting {
		t.Fatalf("%v", st)
	}
	r := newRig(t)
	if v := r.svc.view(rec, false, taskstate.Info{}, false, proctrack.Report{}); v.Resumable || v.Live {
		t.Fatalf("%+v", v)
	}
}

// ---- programs the tool started ---------------------------------------------------------------------------

func TestStartedCountsByTheLayoutOfTheLeader(t *testing.T) {
	proc := func(pid, ppid int, name string) proctrack.Process {
		return proctrack.Process{PID: pid, PPID: ppid, Name: name}
	}
	// the shell runs the tool as its child (a shell function): shell 10, tool 11, two programs of the tool
	shell := proctrack.Report{Leader: 10, Count: 4, Procs: []proctrack.Process{
		proc(10, 1, "-zsh"), proc(11, 10, "claude"), proc(12, 11, "node"), proc(13, 11, "esbuild"),
	}}
	if n := started(shell); n != 2 {
		t.Fatalf("shell leader: %d", n)
	}
	// zsh exec'd the tool: the leader is the tool, and a program it started has the leader as its parent
	tool := proctrack.Report{Leader: 10, Count: 3, Procs: []proctrack.Process{
		proc(10, 1, "codex"), proc(11, 10, "node"), proc(12, 11, "esbuild"),
	}}
	if n := started(tool); n != 2 {
		t.Fatalf("tool leader: %d", n)
	}
	// the leader died: what is left is what the tool started
	gone := proctrack.Report{Leader: 10, Count: 1, Procs: []proctrack.Process{proc(12, 1, "node")}}
	if n := started(gone); n != 1 {
		t.Fatalf("dead leader: %d", n)
	}
	// Windows: PPID is 0, the leader is a shell, so the tool counts (FU-A2-13a)
	win := proctrack.Report{Leader: 10, Count: 2, Procs: []proctrack.Process{proc(10, 0, "cmd.exe"), proc(11, 0, "claude.exe")}}
	if n := started(win); n != 1 {
		t.Fatalf("windows: %d", n)
	}
}

// ---- the waiting notification and its retraction ---------------------------------------------------------

type gatedFocus struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func newGatedFocus(t *testing.T) *gatedFocus {
	g := &gatedFocus{entered: make(chan struct{}, 4), release: make(chan struct{})}
	t.Cleanup(g.open)
	return g
}
func (g *gatedFocus) open() { g.once.Do(func() { close(g.release) }) }
func (g *gatedFocus) focused() bool {
	g.entered <- struct{}{}
	<-g.release
	return false
}

func TestWaitingNoticeIsNotSentForATaskThatWasAnswered(t *testing.T) {
	g := newGatedFocus(t)
	r := newRig(t, func(d *Deps) { d.Focused = g.focused })
	tk := r.create(api.AIToolClaude, "x")
	r.report(tk.ID, taskstate.ToolClaude, "PermissionRequest", nil)
	<-g.entered // the notification job is parked in Focused
	r.report(tk.ID, taskstate.ToolClaude, "UserPromptSubmit", nil)
	g.open()
	r.svc.Flush()
	if got := r.task(tk.ID); got.State != api.TaskRunning {
		t.Fatalf("%+v", got)
	}
	if sent, removed := len(r.backend.Sent()), len(r.backend.Removed()); sent != removed {
		t.Fatalf("a notice for a task that is no longer waiting stays: sent %d, removed %d", sent, removed)
	}
}

func TestWaitingNoticeIsNotSentForATaskThatEnded(t *testing.T) {
	g := newGatedFocus(t)
	r := newRig(t, func(d *Deps) { d.Focused = g.focused })
	tk := r.create(api.AIToolClaude, "x")
	r.report(tk.ID, taskstate.ToolClaude, "PermissionRequest", nil)
	<-g.entered
	r.lastStart().pty.exit(0)
	eventually(t, "done", func() bool { return r.task(tk.ID).State == api.TaskDone })
	r.waitSettled(tk.ID)
	g.open()
	r.svc.Flush()
	if sent, removed := len(r.backend.Sent()), len(r.backend.Removed()); sent != removed {
		t.Fatalf("sent %d, removed %d", sent, removed)
	}
}

func TestSentWaitingNoticeIsTakenBackWhenAnswered(t *testing.T) {
	r := newRig(t)
	tk := r.create(api.AIToolClaude, "x")
	r.report(tk.ID, taskstate.ToolClaude, "PermissionRequest", nil)
	r.svc.Flush()
	if len(r.backend.Sent()) != 1 {
		t.Fatalf("%+v", r.backend.Sent())
	}
	r.report(tk.ID, taskstate.ToolClaude, "UserPromptSubmit", nil)
	r.svc.Flush()
	if len(r.backend.Removed()) != 1 {
		t.Fatalf("%v", r.backend.Removed())
	}
}

// ---- Resumed is per run ----------------------------------------------------------------------------------

func TestResumedIsPerRun(t *testing.T) {
	r := newRig(t)
	tk := r.create(api.AIToolClaude, "x")
	r.report(tk.ID, taskstate.ToolClaude, "SessionStart", func(p *taskstate.Report) { p.Source = "resume" })
	if !r.task(tk.ID).Resumed {
		t.Fatal("the tool said it resumed")
	}
	r.report(tk.ID, taskstate.ToolClaude, "SessionStart", func(p *taskstate.Report) { p.Source = "compact" })
	if !r.task(tk.ID).Resumed {
		t.Fatal("compact leaves it as it is")
	}
	r.report(tk.ID, taskstate.ToolClaude, "SessionStart", func(p *taskstate.Report) { p.Source = "startup" })
	if r.task(tk.ID).Resumed {
		t.Fatal("startup is a new conversation")
	}
	r.report(tk.ID, taskstate.ToolClaude, "SessionStart", func(p *taskstate.Report) { p.Source = "resume" })
	if _, err := r.svc.Stop(tk.ID); err != nil {
		t.Fatal(err)
	}
	if !r.task(tk.ID).Resumed {
		t.Fatal("an ended task keeps what its last run said")
	}
	if got, err := r.svc.Resume(tk.ID, false); err != nil || got.Resumed {
		t.Fatalf("a new run has not reported anything yet: %+v %v", got, err)
	}
	r.report(tk.ID, taskstate.ToolClaude, "SessionStart", func(p *taskstate.Report) { p.Source = "startup" })
	if r.task(tk.ID).Resumed {
		t.Fatal("the tool started a new conversation")
	}
}

// ---- tasks.json written by a newer app -------------------------------------------------------------------

func TestTasksFileOfANewerAppIsLeftAlone(t *testing.T) {
	r := newRig(t)
	r.svc.Close()
	path := filepath.Join(r.data, "tasks.json")
	newer := []byte(`{"schemaVersion":2,"tasks":[{"id":"t-future","repo":"r","tool":"claude"}],"counters":{},"somethingNew":true}`)
	if err := os.WriteFile(path, newer, 0o600); err != nil {
		t.Fatal(err)
	}
	r.build()
	tk := r.create(api.AIToolClaude, "x")
	r.svc.Flush()
	if _, err := r.svc.Stop(tk.ID); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(newer) {
		t.Fatalf("the newer app's file was changed: %s", got)
	}
	if _, ok := r.svc.Get(tk.ID); !ok {
		t.Fatal("this run still keeps its tasks in memory")
	}
}

// ---- fix round 2 ---------------------------------------------------------------------------------------

func lastTasks(t *testing.T, r *rig) api.TasksState {
	t.Helper()
	ev, ok := r.ev.last(api.EventTasksChanged)
	if !ok {
		t.Fatal("no tasks:changed")
	}
	return ev.(api.TasksState)
}

func TestAFailedResumeSendsTheRestoredTaskToThePage(t *testing.T) {
	g := newGate()
	r := newRig(t, func(d *Deps) { g.Terminals = d.Terminals; d.Terminals = g })
	tk := r.create(api.AIToolClaude, "x")
	r.endTask(tk.ID)
	r.svc.Flush()
	g.Terminals = failTerminals{g.Terminals}
	g.on.Store(true)
	done := make(chan error, 1)
	go func() { _, err := r.svc.Resume(tk.ID, false); done <- err }()
	<-g.entered
	close(g.release)
	if err := <-done; err == nil {
		t.Fatal("expected the start to fail")
	}
	r.svc.Flush()
	ts := lastTasks(t, r)
	if len(ts.Tasks) != 1 || ts.Tasks[0].State != api.TaskDone || !ts.Tasks[0].Resumable {
		t.Fatalf("the page must see the restored task: %+v", ts.Tasks)
	}
}

func TestAFailedCreateIsTakenOffThePage(t *testing.T) {
	r := newRig(t, func(d *Deps) { d.Terminals = failTerminals{d.Terminals} })
	_, err := r.svc.Create(context.Background(), api.CreateTaskRequest{Repo: "repo-1", Tool: api.AIToolClaude, Where: api.TaskWhereMain, Description: "x"})
	if err == nil {
		t.Fatal("expected the start to fail")
	}
	r.svc.Flush()
	if ts := lastTasks(t, r); len(ts.Tasks) != 0 {
		t.Fatalf("the failed task must not stay in the page's list: %+v", ts.Tasks)
	}
}

type toggleFocus struct {
	on      atomic.Bool
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (g *toggleFocus) focused() bool {
	if g.on.Load() {
		g.entered <- struct{}{}
		<-g.release
	}
	return false
}

func TestAnEndedRunsNoticeIsRetractedEvenIfResumedBeforeTheQueueGetsThere(t *testing.T) {
	g := &toggleFocus{entered: make(chan struct{}, 4), release: make(chan struct{})}
	t.Cleanup(func() { g.once.Do(func() { close(g.release) }) })
	r := newRig(t, func(d *Deps) { d.Focused = g.focused })
	a := r.create(api.AIToolClaude, "a")
	ptyA := r.lastStart().pty
	r.report(a.ID, taskstate.ToolClaude, "PermissionRequest", nil)
	r.svc.Flush()
	if len(r.backend.Sent()) != 1 {
		t.Fatalf("A's notice: %+v", r.backend.Sent())
	}
	g.on.Store(true)
	b := r.create(api.AIToolClaude, "b")
	r.report(b.ID, taskstate.ToolClaude, "PermissionRequest", nil)
	<-g.entered // B's notice job is parked in Focused, A's retract will queue behind it
	ptyA.exit(0)
	eventually(t, "A ended", func() bool { return !r.task(a.ID).Live })
	if _, err := r.svc.Resume(a.ID, false); err != nil {
		t.Fatal(err)
	}
	g.once.Do(func() { close(g.release) })
	r.svc.Flush()
	if rm := r.backend.Removed(); len(rm) != 1 {
		t.Fatalf("A's old notice must be removed: sent %d, removed %v", len(r.backend.Sent()), rm)
	}
}

type exitOnStart struct {
	Terminals
	exit func()
}

func (e exitOnStart) Start(o terminal.StartOptions) (*terminal.Session, error) {
	s, err := e.Terminals.Start(o)
	if err == nil {
		e.exit()
	}
	return s, err
}

func TestResumeWaitsWhenTheRunEndedBeforeCreateReturned(t *testing.T) {
	var r *rig
	var exitOnce sync.Once // only the first run ends at once
	r = newRig(t, func(d *Deps) {
		d.Terminals = exitOnStart{d.Terminals, func() { exitOnce.Do(func() { r.lastStart().pty.exit(0) }) }}
	})
	parked, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	r.awake.setOnSet(func(id string, st keepawake.TaskState) {
		if st == keepawake.StateEnded {
			once.Do(func() { close(parked); <-release })
		}
	})
	t.Cleanup(func() {
		once.Do(func() { close(parked) })
		select {
		case <-release:
		default:
			close(release)
		}
	})
	tk := r.create(api.AIToolClaude, "x") // returns while finalize is parked in keep-awake
	<-parked
	done := make(chan error, 1)
	go func() { _, err := r.svc.Resume(tk.ID, false); done <- err }()
	select {
	case err := <-done:
		t.Fatalf("Resume must wait for the end being recorded: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	r.report(tk.ID, taskstate.ToolClaude, "UserPromptSubmit", nil)
	if st, _ := r.awake.last(tk.ID); st != keepawake.StateRunning {
		t.Fatalf("keep-awake must follow the new run: %v", r.awake.list())
	}
}

// ---- round 3: the launch mark and the cap slot end where the run is stored ---------------------------------

// parkFirstTail parks the first Create's or Resume's tail (the run is stored, the call has not returned) until
// the returned func is called.
func parkFirstTail(r *rig) (parked <-chan struct{}, release func()) {
	p, rel := make(chan struct{}), make(chan struct{})
	var first atomic.Bool // not a sync.Once: a second caller must not wait for the first
	r.svc.tailHook = func() {
		if first.CompareAndSwap(false, true) {
			close(p)
			<-rel
		}
	}
	var o sync.Once
	return p, func() { o.Do(func() { close(rel) }) }
}

func TestASecondStartIsNotCountedTwiceWhileTheFirstFinishes(t *testing.T) {
	r := newRig(t)
	r.cap.Store(2)
	parked, release := parkFirstTail(r)
	defer release()
	done := make(chan error, 1)
	go func() {
		_, err := r.svc.Create(context.Background(), api.CreateTaskRequest{Repo: "repo-1", Tool: api.AIToolClaude, Description: "a"})
		done <- err
	}()
	<-parked
	if r.svc.Snapshot().Live != 1 {
		t.Fatalf("one task runs: %+v", r.svc.Snapshot())
	}
	if _, err := r.svc.Create(context.Background(), api.CreateTaskRequest{Repo: "repo-1", Tool: api.AIToolClaude, Description: "b"}); err != nil {
		t.Fatalf("one runs and the cap is two: the second must not ask: %v", err)
	}
	release()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestAResumeInTheGapOfALaunchersTailIsNotLetThroughTwice(t *testing.T) {
	g := newGate()
	r := newRig(t, func(d *Deps) { g.Terminals = d.Terminals; d.Terminals = g })
	parked, release := parkFirstTail(r)
	defer release()
	type res struct {
		t   api.Task
		err error
	}
	created := make(chan res, 1)
	go func() {
		tk, err := r.svc.Create(context.Background(), api.CreateTaskRequest{Repo: "repo-1", Tool: api.AIToolClaude, Description: "a"})
		created <- res{tk, err}
	}()
	<-parked // the run is stored; Create is in its tail
	id := r.lastStart().opts.ID
	r.lastStart().pty.exit(0)
	eventually(t, "the task ended", func() bool { g := r.task(id); return !g.Live && g.State != api.TaskStarting })
	r.waitSettled(id)
	g.on.Store(true)
	first := make(chan error, 1)
	go func() { _, err := r.svc.Resume(id, false); first <- err }()
	select {
	case <-g.entered: // Resume #1 is parked in Start
	case err := <-first:
		t.Fatalf("Resume #1 did not reach Start: %v", err)
	}
	release()
	if c := <-created; c.err != nil {
		t.Fatal(c.err)
	}
	_, err := r.svc.Resume(id, false)
	var ae *api.AppError
	if !errors.As(err, &ae) || ae.Code != api.ErrInvalidInput || ae.Params["field"] != "run" {
		t.Fatalf("Resume #2 must be refused with the run error: %v", err)
	}
	if g.calls.Load() != 1 {
		t.Fatalf("Resume #2 must not reach Start: %d", g.calls.Load())
	}
	close(g.release)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	r.report(id, taskstate.ToolClaude, "UserPromptSubmit", nil)
	if got := r.task(id); got.State != api.TaskRunning {
		t.Fatalf("%+v", got)
	}
	if _, ok := r.tracker.Get(id); !ok {
		t.Fatal("the tracker knows the task")
	}
	if st, _ := r.awake.last(id); st != keepawake.StateRunning {
		t.Fatalf("%v", r.awake.list())
	}
}
