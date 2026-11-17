package app

import (
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/proctrack"
)

type procGroup struct{ members []int }

func (g *procGroup) Pid() int                { return 100 }
func (g *procGroup) Members() ([]int, error) { return g.members, nil }
func (g *procGroup) Done() <-chan struct{}   { c := make(chan struct{}); close(c); return c } // the tool has exited
func (g *procGroup) KillSession() error      { return nil }

func TestProctrackServiceListsAndStops(t *testing.T) {
	var mu sync.Mutex
	table := map[int]proctrack.Proc{
		120: {PID: 120, PPID: 1, PGID: 120, Start: 12, Comm: "/bin/sleep", CPU: 3, RSS: 2048},
	}
	src := proctrack.SourceFunc(func([]int) ([]proctrack.Proc, error) {
		mu.Lock()
		defer mu.Unlock()
		var out []proctrack.Proc
		for _, p := range table {
			out = append(out, p)
		}
		return out, nil
	})
	no := false
	tr := proctrack.New(proctrack.Options{Source: src, UseJob: &no, Grace: time.Second, Hard: time.Second, Poll: 5 * time.Millisecond,
		Signal: func(pid int, sig syscall.Signal) error {
			mu.Lock()
			defer mu.Unlock()
			if sig == syscall.SIGTERM || sig == syscall.SIGKILL {
				delete(table, pid)
				delete(table, -pid)
			}
			return nil
		}})
	var events int
	var emu sync.Mutex
	svc := NewProctrackService(tr, func(name string, data any) {
		emu.Lock()
		defer emu.Unlock()
		if name == api.EventProctrackChanged {
			events++
			if _, ok := data.([]api.SessionLeftovers); !ok {
				t.Errorf("payload %T", data)
			}
		}
	})
	if l, err := svc.List(); err != nil || l == nil || len(l) != 0 {
		t.Fatalf("empty list: %#v %v", l, err)
	}
	tr.Track("s1", &procGroup{members: []int{120}})
	if err := tr.Tick(); err != nil {
		t.Fatal(err)
	}
	l, _ := svc.List()
	if len(l) != 1 || l[0].Live || l[0].Count != 1 || l[0].Procs[0].Name != "sleep" || l[0].Memory != 2048 || l[0].ClosedAt == "" {
		t.Fatalf("list %+v", l)
	}
	emu.Lock()
	if events == 0 {
		t.Error("no proctrack:changed event")
	}
	emu.Unlock()
	res, err := svc.ForceStop("s1")
	if err != nil || res.Stopped != 1 || res.Survivors == nil || len(res.Survivors) != 0 || res.Names[0] != "sleep" {
		t.Fatalf("stop %+v %v", res, err)
	}
	if one, _ := svc.Leftovers("s1"); one.Count != 0 || one.Procs == nil {
		t.Errorf("after stop %+v", one)
	}
}
