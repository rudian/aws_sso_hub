package tasks

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rudian/codepass/internal/aitools"
	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/keepawake"
	"github.com/rudian/codepass/internal/notify"
	"github.com/rudian/codepass/internal/proctrack"
	"github.com/rudian/codepass/internal/pty"
	"github.com/rudian/codepass/internal/taskstate"
	"github.com/rudian/codepass/internal/terminal"
)

// fakePTY is a pty.Session the test drives: output with feed, the end with exit.
type fakePTY struct {
	out    chan []byte
	done   chan struct{}
	once   sync.Once
	code   atomic.Int32
	killed atomic.Int32
	mu     sync.Mutex
	in     []byte
	pid    int
}

func newFakePTY(pid int) *fakePTY {
	f := &fakePTY{out: make(chan []byte, 256), done: make(chan struct{}), pid: pid}
	f.code.Store(-1)
	return f
}

func (f *fakePTY) feed(s string) { f.out <- []byte(s) }

// exit ends the root process and the terminal with a code.
func (f *fakePTY) exit(code int) {
	f.once.Do(func() {
		f.code.Store(int32(code))
		close(f.done)
		close(f.out)
	})
}

func (f *fakePTY) Read(p []byte) (int, error) {
	b, ok := <-f.out
	if !ok {
		return 0, io.EOF
	}
	return copy(p, b), nil
}
func (f *fakePTY) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.in = append(f.in, p...)
	return len(p), nil
}
func (f *fakePTY) Resize(uint16, uint16) error { return nil }
func (f *fakePTY) Pid() int                    { return f.pid }
func (f *fakePTY) Done() <-chan struct{}       { return f.done }
func (f *fakePTY) ExitCode() int               { return int(f.code.Load()) }
func (f *fakePTY) Foreground() (int, error)    { return 0, pty.ErrClosed }
func (f *fakePTY) Members() ([]int, error)     { return nil, nil }
func (f *fakePTY) KillSession() error          { f.killed.Add(1); f.exit(143); return nil }
func (f *fakePTY) Close() error                { return nil }
func (f *fakePTY) input() string               { f.mu.Lock(); defer f.mu.Unlock(); return string(f.in) }

type startRec struct {
	opts terminal.StartOptions
	pty  *fakePTY
}

// fakeAwake records what keep-awake was told.
type fakeAwake struct {
	mu    sync.Mutex
	calls []string
	state map[string]keepawake.TaskState
	// onSet runs before a SetState is recorded, outside the lock (a test parks the call there).
	onSet func(id string, st keepawake.TaskState)
}

func (a *fakeAwake) SetState(id string, k keepawake.Kind, st keepawake.TaskState) {
	a.mu.Lock()
	f := a.onSet
	a.mu.Unlock()
	if f != nil {
		f(id, st)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.state == nil {
		a.state = map[string]keepawake.TaskState{}
	}
	a.state[id] = st
	a.calls = append(a.calls, "set:"+id+":"+stateName(st))
}
func (a *fakeAwake) setOnSet(f func(string, keepawake.TaskState)) {
	a.mu.Lock()
	a.onSet = f
	a.mu.Unlock()
}
func (a *fakeAwake) Touch(id string) {
	a.mu.Lock()
	a.calls = append(a.calls, "touch:"+id)
	a.mu.Unlock()
}
func (a *fakeAwake) Forget(id string) {
	a.mu.Lock()
	a.calls = append(a.calls, "forget:"+id)
	a.mu.Unlock()
}
func (a *fakeAwake) Refresh() { a.mu.Lock(); a.calls = append(a.calls, "refresh"); a.mu.Unlock() }
func (a *fakeAwake) list() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return slices.Clone(a.calls)
}
func (a *fakeAwake) count(prefix string) int {
	n := 0
	for _, c := range a.list() {
		if len(c) >= len(prefix) && c[:len(prefix)] == prefix {
			n++
		}
	}
	return n
}
func (a *fakeAwake) last(id string) (keepawake.TaskState, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	st, ok := a.state[id]
	return st, ok
}

func stateName(st keepawake.TaskState) string {
	switch st {
	case keepawake.StateRunning:
		return "running"
	case keepawake.StateWaiting:
		return "waiting"
	case keepawake.StateEnded:
		return "ended"
	}
	return "idle"
}

// fakeProcs is a process tracker with programs the test sets.
type fakeProcs struct {
	mu       sync.Mutex
	tracked  map[string]proctrack.Group
	closed   map[string]int
	reports  map[string]proctrack.Report
	forgot   []string
	forceAll int
	survivor int
	orderLog *[]string
	// onTick, when set, runs inside Tick (a test makes the dead programs of a closed session vanish there).
	onTick func(p *fakeProcs)
	ticks  int
}

func newFakeProcs() *fakeProcs {
	return &fakeProcs{tracked: map[string]proctrack.Group{}, closed: map[string]int{}, reports: map[string]proctrack.Report{}}
}
func (p *fakeProcs) Track(id string, g proctrack.Group) {
	p.mu.Lock()
	p.tracked[id] = g
	p.mu.Unlock()
}
func (p *fakeProcs) MarkClosed(id string) { p.mu.Lock(); p.closed[id]++; p.mu.Unlock() }
func (p *fakeProcs) Forget(id string)     { p.mu.Lock(); p.forgot = append(p.forgot, id); p.mu.Unlock() }
func (p *fakeProcs) Leftovers(id string) proctrack.Report {
	p.mu.Lock()
	defer p.mu.Unlock()
	if r, ok := p.reports[id]; ok {
		return r
	}
	return proctrack.Report{Session: id}
}
func (p *fakeProcs) Active() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	n := 0
	for _, r := range p.reports {
		if !r.Live {
			n += r.Count
		}
	}
	return n
}
func (p *fakeProcs) Tick() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.ticks++
	if p.onTick != nil {
		p.onTick(p) // under p.mu: the callback may touch the reports
	}
	return nil
}
func (p *fakeProcs) ForceStopAll() ([]proctrack.StopResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.forceAll++
	if p.orderLog != nil {
		*p.orderLog = append(*p.orderLog, "forcestopall")
	}
	var out []proctrack.StopResult
	if p.survivor > 0 {
		out = append(out, proctrack.StopResult{Session: "x", Found: p.survivor, Survivors: make([]proctrack.Process, p.survivor)})
	}
	for id := range p.reports {
		delete(p.reports, id)
	}
	return out, nil
}

// setLeftovers makes a closed session have n programs: the shell (leader 100), the tool (pid 101, child of the
// shell) and n-2 programs the tool started.
func (p *fakeProcs) setLeftovers(id string, n int, live bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if n == 0 {
		delete(p.reports, id)
		return
	}
	r := proctrack.Report{Session: id, Leader: 100, Live: live, Count: n, CPU: 1.5, RSS: 1 << 20}
	if !live {
		r.ClosedAt = time.Unix(2000, 0)
	}
	for i := 0; i < n; i++ {
		pp := 100
		if i >= 2 {
			pp = 101
		}
		name := "program"
		switch i {
		case 0:
			name = "zsh"
		case 1:
			name = "claude"
		}
		r.Procs = append(r.Procs, proctrack.Process{PID: 100 + i, PPID: pp, Name: name})
	}
	p.reports[id] = r
}

type fakeRepos map[string]RepoRef

func (f fakeRepos) Lookup(id string) (RepoRef, bool) { r, ok := f[id]; return r, ok }

type events struct {
	mu sync.Mutex
	l  []emitted
}
type emitted struct {
	name string
	data any
}

func (e *events) emit(name string, data any) {
	e.mu.Lock()
	e.l = append(e.l, emitted{name, data})
	e.mu.Unlock()
}
func (e *events) last(name string) (any, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for i := len(e.l) - 1; i >= 0; i-- {
		if e.l[i].name == name {
			return e.l[i].data, true
		}
	}
	return nil, false
}

// rig is a Service wired to a real terminal manager (with fake terminals), a real tracker, the fake backend of
// internal/notify and the fakes above.
type rig struct {
	t        *testing.T
	data     string
	svc      *Service
	mgr      *terminal.Manager
	tracker  *taskstate.Tracker
	relay    *Relay
	awake    *fakeAwake
	procs    *fakeProcs
	notifier *notify.Service
	backend  *notify.Fake
	ev       *events
	repo     string

	mu       sync.Mutex
	starts   []startRec
	focused  bool
	lang     string
	shown    atomic.Int32
	nameResp Naming
	nameErr  error
	names    atomic.Int32
	pidSeq   atomic.Int32
	cap      atomic.Int32
	idSeq    atomic.Int32
	opts     func(*Deps)
}

func newRig(t *testing.T, opts ...func(*Deps)) *rig {
	t.Helper()
	r := &rig{t: t, data: t.TempDir(), repo: t.TempDir(), awake: &fakeAwake{}, procs: newFakeProcs(), ev: &events{}, lang: "en"}
	r.cap.Store(6)
	r.nameResp = Naming{Title: "Fix rounding", Slug: "fix-rounding", FromAI: true}
	r.build(opts...)
	return r
}

// build (re)creates everything except the data folder: a second call is a restart of the app.
func (r *rig) build(opts ...func(*Deps)) {
	r.t.Helper()
	r.mgr = terminal.NewManager(terminal.Config{
		DataDir: r.data,
		Start: func(o pty.Options) (pty.Session, error) {
			f := newFakePTY(5000 + int(r.pidSeq.Add(1)))
			r.mu.Lock()
			r.starts = append(r.starts, startRec{pty: f})
			r.mu.Unlock()
			return f, nil
		},
		LookPath: func(s string) (string, error) { return s, nil },
		GOOS:     "darwin",
	})
	r.relay = &Relay{}
	r.tracker = taskstate.NewTracker(taskstate.Options{NoSignalAfter: time.Hour}, r.relay.OnChange)
	r.backend = notify.NewFake()
	r.notifier = notify.New(notify.Options{
		Backend: r.backend, Lang: func() string { return r.getLang() }, Go: func(f func()) { f() },
		Toast: func(api.NotifyToast) {}, Visible: func() bool { return true },
	})
	r.notifier.Start(context.Background())
	d := Deps{
		DataDir: r.data, Terminals: &recTerminals{Manager: r.mgr, r: r}, Tracker: r.tracker, Relay: r.relay,
		Endpoint: "/tmp/test.sock", Exe: "/Applications/Codepass.app/Contents/MacOS/codepass",
		Awake: r.awake, Procs: r.procs, Notify: r.notifier, Repos: fakeRepos{"repo-1": {ID: "repo-1", Path: r.repo, Name: "acme_api"}},
		Launch: func(q aitools.LaunchRequest) (aitools.Launch, error) {
			q.GOOS, q.Shell = "darwin", "/bin/zsh"
			return aitools.BuildLaunch(q)
		},
		Name: func(ctx context.Context, tool aitools.ID, desc string, n int) (Naming, error) {
			r.names.Add(1)
			r.mu.Lock()
			defer r.mu.Unlock()
			return r.nameResp, r.nameErr
		},
		Lang:       func() string { return r.getLang() },
		Focused:    func() bool { r.mu.Lock(); defer r.mu.Unlock(); return r.focused },
		ShowWindow: func() { r.shown.Add(1) },
		Emit:       r.ev.emit,
		SoftCap:    func() int { return int(r.cap.Load()) },
		PollEvery:  time.Hour,
		NewID:      func() string { return fmt.Sprintf("t-%010d", r.idSeq.Add(1)) },
	}
	for _, o := range opts {
		o(&d)
	}
	r.svc = New(d)
	r.t.Cleanup(r.svc.Close)
}

func (r *rig) getLang() string   { r.mu.Lock(); defer r.mu.Unlock(); return r.lang }
func (r *rig) setLang(l string)  { r.mu.Lock(); r.lang = l; r.mu.Unlock() }
func (r *rig) setFocused(b bool) { r.mu.Lock(); r.focused = b; r.mu.Unlock() }

// recTerminals records the options of every Start next to the fake terminal it made.
type recTerminals struct {
	*terminal.Manager
	r *rig
}

func (t *recTerminals) Start(o terminal.StartOptions) (*terminal.Session, error) {
	s, err := t.Manager.Start(o)
	if err == nil {
		t.r.mu.Lock()
		t.r.starts[len(t.r.starts)-1].opts = o
		t.r.mu.Unlock()
	}
	return s, err
}

func (r *rig) lastStart() startRec {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.starts) == 0 {
		r.t.Fatal("no terminal was started")
	}
	return r.starts[len(r.starts)-1]
}

func (r *rig) startCount() int { r.mu.Lock(); defer r.mu.Unlock(); return len(r.starts) }

func (r *rig) create(tool api.AIToolID, desc string) api.Task {
	r.t.Helper()
	tk, err := r.svc.Create(context.Background(), api.CreateTaskRequest{Repo: "repo-1", Tool: tool, Where: api.TaskWhereMain, Description: desc})
	if err != nil {
		r.t.Fatalf("Create: %v", err)
	}
	return tk
}

func (r *rig) report(task string, tool taskstate.Tool, event string, f func(*taskstate.Report)) {
	r.t.Helper()
	rep := taskstate.Report{Task: task, Tool: tool, Event: event, At: time.Now().UnixNano()}
	if f != nil {
		f(&rep)
	}
	r.svc.Sink().Apply(rep)
}

func (r *rig) task(id string) api.Task {
	r.t.Helper()
	tk, ok := r.svc.Get(id)
	if !ok {
		r.t.Fatalf("no task %s", id)
	}
	return tk
}

func eventually(t *testing.T, what string, f func() bool) {
	t.Helper()
	for end := time.Now().Add(5 * time.Second); time.Now().Before(end); time.Sleep(2 * time.Millisecond) {
		if f() {
			return
		}
	}
	t.Fatalf("timed out waiting for %s", what)
}

func hasArg(argv []string, a string) bool { return slices.Contains(argv, a) }

func argAfter(argv []string, a string) string {
	i := slices.Index(argv, a)
	if i < 0 || i+1 >= len(argv) {
		return ""
	}
	return argv[i+1]
}

func envOf(env []string, key string) (string, bool) {
	for i := len(env) - 1; i >= 0; i-- {
		if k, v, ok := cutEnv(env[i]); ok && k == key {
			return v, true
		}
	}
	return "", false
}

func cutEnv(kv string) (string, string, bool) {
	for i := 0; i < len(kv); i++ {
		if kv[i] == '=' {
			return kv[:i], kv[i+1:], true
		}
	}
	return kv, "", false
}

var _ = filepath.Join

func newTestLogger(w io.Writer) *slog.Logger {
	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: slog.LevelDebug}))
}
