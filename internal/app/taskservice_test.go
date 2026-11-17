package app

import (
	"context"
	"io"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rudian/codepass/internal/aitools"
	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/keepawake"
	"github.com/rudian/codepass/internal/proctrack"
	"github.com/rudian/codepass/internal/pty"
	"github.com/rudian/codepass/internal/tasks"
	"github.com/rudian/codepass/internal/taskstate"
	"github.com/rudian/codepass/internal/terminal"
)

type taskFakePTY struct {
	out  chan []byte
	done chan struct{}
	once sync.Once
	code atomic.Int32
}

func newTaskFakePTY() *taskFakePTY {
	f := &taskFakePTY{out: make(chan []byte, 8), done: make(chan struct{})}
	f.code.Store(-1)
	return f
}
func (f *taskFakePTY) Read(p []byte) (int, error) {
	b, ok := <-f.out
	if !ok {
		return 0, io.EOF
	}
	return copy(p, b), nil
}
func (f *taskFakePTY) Write(p []byte) (int, error) { return len(p), nil }
func (f *taskFakePTY) Resize(uint16, uint16) error { return nil }
func (f *taskFakePTY) Pid() int                    { return 4242 }
func (f *taskFakePTY) Done() <-chan struct{}       { return f.done }
func (f *taskFakePTY) ExitCode() int               { return int(f.code.Load()) }
func (f *taskFakePTY) Foreground() (int, error)    { return 0, pty.ErrClosed }
func (f *taskFakePTY) Members() ([]int, error)     { return nil, nil }
func (f *taskFakePTY) KillSession() error {
	f.once.Do(func() { f.code.Store(143); close(f.done); close(f.out) })
	return nil
}
func (f *taskFakePTY) Close() error { return nil }

func TestTaskServiceJoinsTheGroupBPieces(t *testing.T) {
	w := newWorld(t)
	ok(w.acc.Login(bg, "initech", api.LoginModeBrowser))
	repoDir := w.outsideRepo("initech_infra", ccURL("initech_infra"))
	added := ok(w.repo.AddExisting(bg, repoDir))

	mgr := terminal.NewManager(terminal.Config{
		DataDir: w.p.AppDataDir, Emit: w.c.emit, LookPath: func(s string) (string, error) { return s, nil }, GOOS: "darwin",
		Start: func(o pty.Options) (pty.Session, error) { return newTaskFakePTY(), nil },
	})
	relay := &tasks.Relay{}
	tracker := taskstate.NewTracker(taskstate.Options{}, relay.OnChange)
	awake := keepawake.New(keepawake.Options{Holders: []keepawake.Holder{&keepawake.FakeHolder{}}, Clock: keepawake.NewFakeClock(), Config: keepawake.DefaultConfig()})
	t.Cleanup(awake.Close)
	proc := proctrack.New(proctrack.Options{DataDir: w.p.AppDataDir})
	ai := NewAIToolServiceWith(AIToolDeps{GOOS: "darwin", Detect: aitools.DetectOptions{Shell: "/bin/zsh", Env: []string{"HOME=/h"}},
		Namer: aitools.Namer{WorkRoot: filepath.Join(w.p.AppDataDir, "naming"),
			Run: func(context.Context, []string, string, []string) ([]byte, int, error) {
				return nil, -1, io.ErrClosedPipe
			}}})
	svc := NewTasksManager(w.c, TasksWiring{Terminal: mgr, Tracker: tracker, Relay: relay, Endpoint: "/tmp/x.sock", Proc: proc, Awake: awake, AITools: ai,
		Exe: "/nonexistent/codepass", ExcludeBackup: func(string) error { return nil }})
	ts := NewTaskService(svc)
	t.Cleanup(func() { _ = ts.ServiceShutdown() })
	if err := ts.ServiceStartup(context.Background(), applicationServiceOptions()); err != nil {
		t.Fatal(err)
	}

	st := ok(ts.List())
	if len(st.Tasks) != 0 || st.Cap != tasks.DefaultSoftCap {
		t.Fatalf("%+v", st)
	}
	// the description is named through the tool's headless call, which fails here: the quiet fallback
	tk := ok(ts.Create(bg, api.CreateTaskRequest{Repo: added.RepoID, Tool: api.AIToolClaude, Where: api.TaskWhereMain, Description: "Fix the settlement rounding"}))
	if tk.Title != "Fix the settlement rounding" || tk.NamedBy != api.TaskNamedFallback || !tk.Live || tk.Dir != repoDir {
		t.Fatalf("%+v", tk)
	}
	if info, found := tracker.Get(tk.ID); !found || info.Mode != taskstate.ModeHooks {
		t.Fatalf("the relay and the tracker: %+v", info)
	}
	if _, found := mgr.Session(tk.ID); !found {
		t.Fatal("started on the one terminal manager")
	}
	// the input hook is connected: a focus report does not answer, Enter does
	svc.Sink().Apply(taskstate.Report{Task: tk.ID, Tool: taskstate.ToolClaude, Event: "PermissionRequest", At: 1})
	_ = mgr.Write(tk.ID, []byte("\x1b[I"))
	if got := ok(ts.List()).Tasks[0]; got.State != api.TaskWaiting {
		t.Fatalf("%+v", got)
	}
	_ = mgr.Write(tk.ID, []byte("\r"))
	if got := ok(ts.List()).Tasks[0]; got.State != api.TaskRunning {
		t.Fatalf("%+v", got)
	}
	if !svc.TasksRunning() || awake.State().Running != 1 {
		t.Fatalf("running %v, keep-awake %+v", svc.TasksRunning(), awake.State())
	}
	ls := ok(ts.LiveSummary())
	if ls.Live != 1 || ls.Sessions[0].RepoName != "initech_infra" {
		t.Fatalf("%+v", ls)
	}
	// events reach the window through the container
	deadline := time.Now().Add(3 * time.Second)
	for w.rt.count(api.EventTasksChanged) == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if w.rt.count(api.EventTasksChanged) == 0 {
		t.Fatal("no tasks:changed event")
	}
	// the quit path
	res := ok(ts.StopAll())
	if res.Stopped != 1 {
		t.Fatalf("%+v", res)
	}
	got := ok(ts.List()).Tasks[0]
	if got.State != api.TaskStopped || svc.TasksRunning() {
		t.Fatalf("%+v", got)
	}
	if e := fails(ts.Create(bg, api.CreateTaskRequest{Repo: "unknown", Tool: api.AIToolClaude})); e.Code != api.ErrRepoNotFound {
		t.Fatalf("%+v", e)
	}
	if e := fails(ts.Resume("nope", false)); e.Code != api.ErrInvalidInput {
		t.Fatalf("%+v", e)
	}
}

func TestConnectProctrackReachesBothTargets(t *testing.T) {
	w := newWorld(t)
	var events atomic.Int32
	proc := proctrack.New(proctrack.Options{DataDir: w.p.AppDataDir})
	ps := NewProctrackService(proc, func(name string, _ any) {
		if name == api.EventProctrackChanged {
			events.Add(1)
		}
	})
	relay := &tasks.Relay{}
	tracker := taskstate.NewTracker(taskstate.Options{}, relay.OnChange)
	var changed atomic.Int32
	svc := tasks.New(tasks.Deps{DataDir: w.p.AppDataDir, Terminals: nil, Tracker: tracker, Relay: relay, Emit: func(name string, _ any) {
		if name == api.EventTasksChanged {
			changed.Add(1)
		}
	}})
	t.Cleanup(svc.Close)
	ConnectProctrack(ps, svc)
	before := changed.Load()
	proc.Forget("t-0000000001") // any change of the tracker calls its callback
	deadline := time.Now().Add(5 * time.Second)
	for (events.Load() == 0 || changed.Load() == before) && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}
	if events.Load() == 0 || changed.Load() == before {
		t.Fatalf("proctrack event %d, tasks event %d -> %d", events.Load(), before, changed.Load())
	}
}
