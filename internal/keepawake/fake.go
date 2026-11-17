package keepawake

import (
	"errors"
	"sort"
	"sync"
	"time"
)

// The fakes of this file are for tests, here and in other packages: no test may hold a real assertion.

// FakeClock is a clock that moves only when Advance is called.
type FakeClock struct {
	mu     sync.Mutex
	now    time.Time
	timers []*fakeTimer
	seq    int
}

type fakeTimer struct {
	at      time.Time
	f       func()
	seq     int
	stopped bool
	fired   bool
	c       *FakeClock
}

func (t *fakeTimer) Stop() bool {
	t.c.mu.Lock()
	defer t.c.mu.Unlock()
	was := !t.stopped && !t.fired
	t.stopped = true
	return was
}

// NewFakeClock starts at a fixed time.
func NewFakeClock() *FakeClock {
	return &FakeClock{now: time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)}
}

func (c *FakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *FakeClock) AfterFunc(d time.Duration, f func()) Timer {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.seq++
	t := &fakeTimer{at: c.now.Add(d), f: f, seq: c.seq, c: c}
	c.timers = append(c.timers, t)
	return t
}

// Advance moves time forward and runs, one after the other and on the calling goroutine, every timer that falls
// due on the way (a timer set by a timer is run too when it falls inside the span).
func (c *FakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	target := c.now.Add(d)
	c.mu.Unlock()
	for {
		c.mu.Lock()
		var due []*fakeTimer
		for _, t := range c.timers {
			if !t.stopped && !t.fired && !t.at.After(target) {
				due = append(due, t)
			}
		}
		if len(due) == 0 {
			c.now = target
			c.mu.Unlock()
			return
		}
		sort.Slice(due, func(i, j int) bool {
			if !due[i].at.Equal(due[j].at) {
				return due[i].at.Before(due[j].at)
			}
			return due[i].seq < due[j].seq
		})
		t := due[0]
		if t.at.After(c.now) {
			c.now = t.at
		}
		t.fired = true
		c.mu.Unlock()
		t.f()
	}
}

// Jump moves time forward without running timers, like a Mac that slept: nothing ran meanwhile. A later Advance(0)
// or any call into the manager then sees the new time.
func (c *FakeClock) Jump(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

// Pending is the number of timers that have not fired and were not stopped.
func (c *FakeClock) Pending() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, t := range c.timers {
		if !t.stopped && !t.fired {
			n++
		}
	}
	return n
}

// FakeHolder records Acquire and Release calls.
type FakeHolder struct {
	N string // name; default "fake"

	mu      sync.Mutex
	held    bool
	screen  bool
	acquire int
	release int
	// FailAcquire makes Acquire return an error while set.
	failAcquire bool
	log         []string
}

func (h *FakeHolder) Name() string {
	if h.N == "" {
		return "fake"
	}
	return h.N
}

func (h *FakeHolder) Acquire(screen bool) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.acquire++
	if h.failAcquire {
		h.log = append(h.log, "acquire-failed")
		return errors.New("refused")
	}
	h.held, h.screen = true, screen
	if screen {
		h.log = append(h.log, "acquire:screen")
	} else {
		h.log = append(h.log, "acquire")
	}
	return nil
}

func (h *FakeHolder) Release() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.release++
	h.held = false
	h.log = append(h.log, "release")
}

func (h *FakeHolder) SetFail(v bool) { h.mu.Lock(); h.failAcquire = v; h.mu.Unlock() }

// Held reports whether the fake currently holds and with which screen option.
func (h *FakeHolder) Held() (held, screen bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.held, h.screen
}

// Calls returns the number of Acquire and Release calls so far.
func (h *FakeHolder) Calls() (acquire, release int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.acquire, h.release
}

// Log returns the calls in order.
func (h *FakeHolder) Log() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.log...)
}

// FakeLeftovers is a Leftovers with counts a test sets.
type FakeLeftovers struct {
	mu sync.Mutex
	n  map[string]int
}

func (f *FakeLeftovers) Set(session string, n int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.n == nil {
		f.n = map[string]int{}
	}
	f.n[session] = n
}

func (f *FakeLeftovers) Count(session string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.n[session]
}

// FakeRunner is a Runner whose processes only exist as records. It never starts a real process.
type FakeRunner struct {
	mu      sync.Mutex
	procs   []*FakeProc
	argvs   [][]string
	failErr error
}

// FakeProc is one fake process; Exit ends it as if killed from outside.
type FakeProc struct {
	done   chan struct{}
	once   sync.Once
	mu     sync.Mutex
	killed bool
}

func (r *FakeRunner) Start(argv []string) (Proc, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.failErr != nil {
		return nil, r.failErr
	}
	p := &FakeProc{done: make(chan struct{})}
	r.procs = append(r.procs, p)
	r.argvs = append(r.argvs, append([]string(nil), argv...))
	return p, nil
}

// SetFail makes Start fail with err (nil = work again).
func (r *FakeRunner) SetFail(err error) { r.mu.Lock(); r.failErr = err; r.mu.Unlock() }

// Starts is the number of processes started so far.
func (r *FakeRunner) Starts() int { r.mu.Lock(); defer r.mu.Unlock(); return len(r.procs) }

// Argv is the argument list of start number i.
func (r *FakeRunner) Argv(i int) []string { r.mu.Lock(); defer r.mu.Unlock(); return r.argvs[i] }

// Proc is start number i.
func (r *FakeRunner) Proc(i int) *FakeProc { r.mu.Lock(); defer r.mu.Unlock(); return r.procs[i] }

// Alive is the number of started processes that have not ended.
func (r *FakeRunner) Alive() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, p := range r.procs {
		select {
		case <-p.done:
		default:
			n++
		}
	}
	return n
}

func (p *FakeProc) Wait() error { <-p.done; return nil }
func (p *FakeProc) Kill() error {
	p.mu.Lock()
	p.killed = true
	p.mu.Unlock()
	p.once.Do(func() { close(p.done) })
	return nil
}

// Exit ends the process as if something else killed it.
func (p *FakeProc) Exit() { p.once.Do(func() { close(p.done) }) }

// KilledByUs reports whether Kill was called.
func (p *FakeProc) KilledByUs() bool { p.mu.Lock(); defer p.mu.Unlock(); return p.killed }

// FakeActivity is an ActivityAPI that counts begun and ended activities.
type FakeActivity struct {
	mu     sync.Mutex
	next   uint64
	active map[uint64]bool
	begun  int
	fail   error
}

func (a *FakeActivity) Begin(string) (uint64, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.fail != nil {
		return 0, a.fail
	}
	if a.active == nil {
		a.active = map[uint64]bool{}
	}
	a.next++
	a.active[a.next] = true
	a.begun++
	return a.next, nil
}

func (a *FakeActivity) End(h uint64) { a.mu.Lock(); delete(a.active, h); a.mu.Unlock() }

// Active is the number of activities begun and not ended.
func (a *FakeActivity) Active() int { a.mu.Lock(); defer a.mu.Unlock(); return len(a.active) }

// Begun is the total number of Begin calls that succeeded.
func (a *FakeActivity) Begun() int { a.mu.Lock(); defer a.mu.Unlock(); return a.begun }
