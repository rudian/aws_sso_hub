package proctrack

import (
	"sort"
	"sync"
	"syscall"
	"testing"
	"time"
)

// fakeSys is a process table with signals: a program dies on SIGTERM unless it is in ignoreTerm, and always
// on SIGKILL. Group signals reach every program of the group.
type fakeSys struct {
	mu         sync.Mutex
	procs      map[int]Proc
	ignoreTerm map[int]bool
	reads      int
	signals    []sent
}

type sent struct {
	pid int
	sig syscall.Signal
}

func newFakeSys(ps ...Proc) *fakeSys {
	f := &fakeSys{procs: map[int]Proc{}, ignoreTerm: map[int]bool{}}
	for _, p := range ps {
		f.procs[p.PID] = p
	}
	return f
}

func (f *fakeSys) Read(_ []int) ([]Proc, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reads++
	out := make([]Proc, 0, len(f.procs))
	for _, p := range f.procs {
		out = append(out, p)
	}
	return out, nil
}

func (f *fakeSys) set(p Proc)  { f.mu.Lock(); f.procs[p.PID] = p; f.mu.Unlock() }
func (f *fakeSys) del(pid int) { f.mu.Lock(); delete(f.procs, pid); f.mu.Unlock() }

func (f *fakeSys) signal(pid int, sig syscall.Signal) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.signals = append(f.signals, sent{pid, sig})
	if sig == sigCont {
		return nil
	}
	hit := func(p Proc) {
		if sig == syscall.SIGKILL || !f.ignoreTerm[p.PID] {
			delete(f.procs, p.PID)
		}
	}
	if pid < 0 {
		for _, p := range f.procs {
			if p.PGID == -pid {
				hit(p)
			}
		}
		return nil
	}
	if p, ok := f.procs[pid]; ok {
		hit(p)
	}
	return nil
}

func (f *fakeSys) signalled(pid int) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, s := range f.signals {
		if s.pid == pid || s.pid == -pid {
			return true
		}
	}
	return false
}

// fakeGroup is a terminal session whose members the test sets.
type fakeGroup struct {
	mu      sync.Mutex
	members []int
	done    chan struct{}
	killed  int
}

func newFakeGroup(members ...int) *fakeGroup {
	return &fakeGroup{members: members, done: make(chan struct{})}
}
func (g *fakeGroup) Pid() int { return 100 }
func (g *fakeGroup) Members() ([]int, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]int(nil), g.members...), nil
}
func (g *fakeGroup) Done() <-chan struct{} { return g.done }
func (g *fakeGroup) KillSession() error    { g.mu.Lock(); g.killed++; g.mu.Unlock(); return nil }
func (g *fakeGroup) set(m ...int)          { g.mu.Lock(); g.members = m; g.mu.Unlock() }

func pr(pid, ppid, pgid int, start int64, comm string) Proc {
	return Proc{PID: pid, PPID: ppid, PGID: pgid, Start: start, Comm: comm, CPU: 1, RSS: 1024}
}

func newTestTracker(t *testing.T, f *fakeSys, dir string) *Tracker {
	t.Helper()
	no := false
	return New(Options{Source: f, DataDir: dir, Signal: f.signal, UseJob: &no,
		Grace: 300 * time.Millisecond, Hard: 300 * time.Millisecond, Poll: 5 * time.Millisecond})
}

func pids(r Report) []int {
	var out []int
	for _, p := range r.Procs {
		out = append(out, p.PID)
	}
	sort.Ints(out)
	return out
}
