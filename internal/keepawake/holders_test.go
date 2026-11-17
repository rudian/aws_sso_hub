package keepawake

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

func newCaff() (*Caffeinate, *FakeRunner, *FakeClock) {
	run, clk := &FakeRunner{}, NewFakeClock()
	return &Caffeinate{Runner: run, Clock: clk, PID: 777}, run, clk
}

func TestCaffeinateRestartsWhenKilledFromOutside(t *testing.T) {
	c, run, clk := newCaff()
	if err := c.Acquire(true); err != nil {
		t.Fatal(err)
	}
	defer c.Release()
	for i := 1; i <= 3; i++ {
		clk.Jump(time.Minute) // it ran for a while each time: no back-off
		run.Proc(i - 1).Exit()
		eventually(t, "restart", func() bool { return run.Starts() == i+1 })
		if got := run.Argv(i); got[1] != "-d" || got[len(got)-1] != "777" {
			t.Fatalf("restart argv %v", got)
		}
	}
	if run.Alive() != 1 {
		t.Fatalf("alive = %d", run.Alive())
	}
}

func TestCaffeinateBacksOffWhenItDiesAtOnce(t *testing.T) {
	c, run, clk := newCaff()
	if err := c.Acquire(false); err != nil {
		t.Fatal(err)
	}
	defer c.Release()
	run.Proc(0).Exit()
	eventually(t, "timer", func() bool { return clk.Pending() == 1 })
	if run.Starts() != 1 {
		t.Fatal("restarted before the back-off")
	}
	clk.Advance(time.Second)
	eventually(t, "second start", func() bool { return run.Starts() == 2 })
	run.Proc(1).Exit()
	eventually(t, "timer 2", func() bool { return clk.Pending() == 1 })
	clk.Advance(time.Second)
	if run.Starts() != 2 {
		t.Fatal("the delay should have doubled to 2 s")
	}
	clk.Advance(time.Second)
	eventually(t, "third start", func() bool { return run.Starts() == 3 })
}

func TestCaffeinateReleaseKillsOnlyItsOwnProcessAndStopsRestarts(t *testing.T) {
	c, run, _ := newCaff()
	if err := c.Acquire(true); err != nil {
		t.Fatal(err)
	}
	p := run.Proc(0)
	c.Release()
	if !p.KilledByUs() || run.Alive() != 0 {
		t.Fatal("not killed")
	}
	time.Sleep(20 * time.Millisecond)
	if run.Starts() != 1 {
		t.Fatalf("restarted after Release: %d", run.Starts())
	}
	c.Release() // twice is fine
}

func TestCaffeinateScreenOptionRestartsWithNewArguments(t *testing.T) {
	c, run, _ := newCaff()
	defer c.Release()
	if err := c.Acquire(true); err != nil {
		t.Fatal(err)
	}
	if err := c.Acquire(true); err != nil || run.Starts() != 1 {
		t.Fatalf("same option started again: %d", run.Starts())
	}
	if err := c.Acquire(false); err != nil {
		t.Fatal(err)
	}
	if run.Starts() != 2 || run.Alive() != 1 {
		t.Fatalf("starts %d alive %d", run.Starts(), run.Alive())
	}
	for _, a := range run.Argv(1) {
		if a == "-d" {
			t.Fatalf("-d with screen off: %v", run.Argv(1))
		}
	}
	if got := run.Argv(1); got[0] != CaffeinatePath || got[1] != "-i" {
		t.Fatalf("argv %v", got)
	}
}

func TestCaffeinateStartFailureIsReported(t *testing.T) {
	c, run, _ := newCaff()
	run.SetFail(errors.New("no such file"))
	if err := c.Acquire(true); err == nil {
		t.Fatal("expected an error")
	}
	run.SetFail(nil)
	if err := c.Acquire(true); err != nil {
		t.Fatal(err)
	}
	c.Release()
}

func TestManagerRestartsCaffeinateKilledFromOutsideWhileTasksRun(t *testing.T) {
	c, run, clk := newCaff()
	m := New(Options{Holders: []Holder{c}, Clock: clk, Config: DefaultConfig()})
	defer m.Close()
	m.SetState("a", KindAI, StateRunning)
	clk.Jump(10 * time.Second)
	run.Proc(0).Exit()
	eventually(t, "restart", func() bool { return run.Starts() == 2 && run.Alive() == 1 })
	eventually(t, "held again after the restart", func() bool { return m.State().Held })
	m.SetState("a", KindAI, StateEnded) // the last task killed from outside
	if run.Alive() != 0 || m.State().Held {
		t.Fatalf("alive %d", run.Alive())
	}
}

func TestActivityHolder(t *testing.T) {
	api := &FakeActivity{}
	a := &Activity{API: api}
	if err := a.Acquire(true); err != nil {
		t.Fatal(err)
	}
	_ = a.Acquire(false)
	if api.Active() != 1 || api.Begun() != 1 {
		t.Fatalf("active %d begun %d", api.Active(), api.Begun())
	}
	a.Release()
	a.Release()
	if api.Active() != 0 {
		t.Fatal("still active")
	}
	api.fail = errors.New("no")
	if err := a.Acquire(true); err == nil {
		t.Fatal("expected error")
	}
}

func TestExecStateFlagsAndOneThread(t *testing.T) {
	var mu sync.Mutex
	var calls []uint32
	e := &ExecState{Set: func(f uint32) error { mu.Lock(); calls = append(calls, f); mu.Unlock(); return nil }}
	if err := e.Acquire(true); err != nil {
		t.Fatal(err)
	}
	if err := e.Acquire(true); err != nil { // same value: no second call
		t.Fatal(err)
	}
	if err := e.Acquire(false); err != nil {
		t.Fatal(err)
	}
	e.Release()
	e.Release()
	e.Close()
	want := []uint32{esContinuous | esSystemRequired | esDisplayRequired, esContinuous | esSystemRequired, esContinuous}
	mu.Lock()
	defer mu.Unlock()
	if len(calls) != len(want) {
		t.Fatalf("calls %x", calls)
	}
	for i := range want {
		if calls[i] != want[i] {
			t.Fatalf("calls %x, want %x", calls, want)
		}
	}
	if err := e.Acquire(true); err == nil {
		t.Fatal("Acquire after Close")
	}
}

func TestExecStateRunsEveryCallOnTheSameGoroutine(t *testing.T) {
	ids := map[string]bool{}
	var mu sync.Mutex
	e := &ExecState{Set: func(uint32) error { mu.Lock(); ids[goroutineID()] = true; mu.Unlock(); return nil }}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = e.Acquire(i%2 == 0)
			e.Release()
		}(i)
	}
	wg.Wait()
	e.Close()
	if len(ids) != 1 {
		t.Fatalf("calls ran on %d goroutines", len(ids))
	}
}

func TestExecStateFailure(t *testing.T) {
	e := &ExecState{Set: func(uint32) error { return errors.New("denied") }}
	defer e.Close()
	if err := e.Acquire(true); err == nil {
		t.Fatal("expected error")
	}
	e.Release() // nothing held: no call, no panic
}
