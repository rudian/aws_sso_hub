package keepawake

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rudian/codepass/internal/api"
)

type rig struct {
	t    *testing.T
	clk  *FakeClock
	h    *FakeHolder
	left *FakeLeftovers
	m    *Manager
	mu   sync.Mutex
	got  []api.KeepAwakeState
}

func newRig(t *testing.T, mod func(*api.KeepAwakeConfig)) *rig {
	t.Helper()
	r := &rig{t: t, clk: NewFakeClock(), h: &FakeHolder{}, left: &FakeLeftovers{}}
	cfg := DefaultConfig()
	if mod != nil {
		mod(&cfg)
	}
	r.m = New(Options{Holders: []Holder{r.h}, Clock: r.clk, Leftovers: r.left, Config: cfg,
		OnChange: func(s api.KeepAwakeState) { r.mu.Lock(); r.got = append(r.got, s); r.mu.Unlock() }})
	t.Cleanup(r.m.Close)
	return r
}

func (r *rig) held() bool { h, _ := r.h.Held(); return h }

func (r *rig) wantHeld(want bool, why string) {
	r.t.Helper()
	if got := r.held(); got != want {
		r.t.Fatalf("%s: held = %v, want %v (state %+v)", why, got, want, r.m.State())
	}
	if st := r.m.State(); st.Held != want {
		r.t.Fatalf("%s: State().Held = %v, want %v", why, st.Held, want)
	}
}

func TestCounterHoldsUntilTheLastTaskStops(t *testing.T) {
	r := newRig(t, nil)
	r.wantHeld(false, "start")
	r.m.SetState("a", KindAI, StateRunning)
	r.wantHeld(true, "one running")
	r.m.SetState("b", KindAI, StateRunning)
	r.m.SetState("a", KindAI, StateIdle)
	r.wantHeld(true, "one of two stopped")
	if st := r.m.State(); st.Running != 1 || st.Active != 1 {
		t.Fatalf("state %+v", st)
	}
	r.m.SetState("b", KindAI, StateEnded)
	r.wantHeld(false, "the last stopped")
	if acq, rel := r.h.Calls(); acq != 1 || rel != 1 {
		t.Fatalf("acquire/release = %d/%d, want 1/1 (one hold for two tasks)", acq, rel)
	}
}

func TestPlainShellNeverHolds(t *testing.T) {
	r := newRig(t, nil)
	for _, st := range []TaskState{StateRunning, StateWaiting} {
		r.m.SetState("sh", KindShell, st)
		r.wantHeld(false, "shell")
	}
	// a session that turns out to be a shell is dropped even if it was counted before
	r.m.SetState("x", KindAI, StateRunning)
	r.wantHeld(true, "ai")
	r.m.SetState("x", KindShell, StateRunning)
	r.wantHeld(false, "reclassified as shell")
	if acq, _ := r.h.Calls(); acq != 1 {
		t.Fatalf("acquire = %d", acq)
	}
}

func TestWaitingKeepsTheHoldForTheGraceTime(t *testing.T) {
	for _, tc := range []struct {
		minutes int
		at      time.Duration
	}{{5, 5 * time.Minute}, {15, 15 * time.Minute}, {60, time.Hour}} {
		r := newRig(t, func(c *api.KeepAwakeConfig) { c.GraceMinutes = tc.minutes })
		r.m.SetState("a", KindAI, StateRunning)
		r.m.SetState("a", KindAI, StateWaiting)
		r.wantHeld(true, "waiting")
		st := r.m.State()
		if st.Waiting != 1 || st.ReleaseAt != r.clk.Now().Add(tc.at).UnixMilli() {
			t.Fatalf("%d min: state %+v", tc.minutes, st)
		}
		r.clk.Advance(tc.at - time.Second)
		r.wantHeld(true, "just before the grace time")
		r.clk.Advance(time.Second)
		r.wantHeld(false, "grace time over")
		if st := r.m.State(); st.CeilingHit || st.Active != 0 {
			t.Fatalf("grace is not the ceiling: %+v", st)
		}
	}
}

func TestGraceStartsWhenTheTaskStartsWaiting(t *testing.T) {
	r := newRig(t, nil)
	r.m.SetState("a", KindAI, StateRunning)
	r.clk.Advance(2 * time.Hour) // running does not use up the grace time (and the hook events keep the ceiling away)
	r.m.Touch("a")
	r.m.SetState("a", KindAI, StateWaiting)
	r.clk.Advance(14 * time.Minute)
	r.wantHeld(true, "14 minutes into the grace")
	r.m.SetState("a", KindAI, StateWaiting) // the same state again: an event, not a new grace time
	r.clk.Advance(2 * time.Minute)
	r.wantHeld(false, "grace over")
	r.m.SetState("a", KindAI, StateRunning) // answered: back to running
	r.wantHeld(true, "running again")
}

func TestNeverReleaseStillHasTheCeiling(t *testing.T) {
	r := newRig(t, func(c *api.KeepAwakeConfig) { c.GraceMinutes = 0; c.CeilingHours = 4 })
	r.m.SetState("a", KindAI, StateWaiting)
	r.clk.Advance(3*time.Hour + 59*time.Minute)
	r.wantHeld(true, "waiting for good")
	r.clk.Advance(time.Minute)
	r.wantHeld(false, "ceiling")
	if !r.m.State().CeilingHit {
		t.Fatal("CeilingHit not set")
	}
}

func TestHardCeilingReleasesARunningTaskWithoutEvents(t *testing.T) {
	r := newRig(t, nil) // 8 hours
	r.m.SetState("a", KindAI, StateRunning)
	r.clk.Advance(7 * time.Hour)
	r.m.Touch("a") // a hook event or the owner typing
	r.clk.Advance(7*time.Hour + 59*time.Minute)
	r.wantHeld(true, "7h59m after the last event")
	r.clk.Advance(time.Minute)
	r.wantHeld(false, "8h after the last event")
	if st := r.m.State(); !st.CeilingHit || st.Active != 0 {
		t.Fatalf("state %+v", st)
	}
	r.m.SetState("a", KindAI, StateRunning) // a new event revives it
	r.wantHeld(true, "revived")
	if r.m.State().CeilingHit {
		t.Fatal("CeilingHit stays after a new event")
	}
}

func TestLeftoverProgramsCountAndTheCeilingStillApplies(t *testing.T) {
	r := newRig(t, nil)
	r.m.SetState("a", KindAI, StateRunning)
	r.left.Set("a", 2)
	r.m.SetState("a", KindAI, StateEnded)
	r.wantHeld(true, "closed session with two programs")
	if st := r.m.State(); st.Leftover != 1 || st.Programs != 2 || st.ReleaseAt == 0 {
		t.Fatalf("state %+v", st)
	}
	r.left.Set("a", 1)
	r.m.Refresh()
	r.wantHeld(true, "one program left")
	if st := r.m.State(); st.Programs != 1 {
		t.Fatalf("state %+v", st)
	}
	r.left.Set("a", 0)
	r.m.Refresh()
	r.wantHeld(false, "the last program is gone")
	if acq, rel := r.h.Calls(); acq != 1 || rel != 1 {
		t.Fatalf("calls %d/%d", acq, rel)
	}

	// without a Refresh the poll notices
	r.left.Set("b", 1)
	r.m.SetState("b", KindAI, StateEnded)
	r.wantHeld(true, "b")
	r.left.Set("b", 0)
	r.clk.Advance(DefaultPollInterval)
	r.wantHeld(false, "poll")

	// the ceiling
	r.left.Set("c", 3)
	r.m.SetState("c", KindAI, StateEnded)
	r.clk.Advance(7*time.Hour + 59*time.Minute)
	r.wantHeld(true, "c just before the ceiling")
	r.clk.Advance(time.Minute)
	r.wantHeld(false, "c at the ceiling")
	if !r.m.State().CeilingHit {
		t.Fatal("CeilingHit")
	}
}

func TestEndedSessionWithNothingLeftIsForgotten(t *testing.T) {
	r := newRig(t, nil)
	r.m.SetState("a", KindAI, StateRunning)
	r.m.SetState("a", KindAI, StateEnded)
	r.wantHeld(false, "no leftovers")
	r.m.mu.Lock()
	n := len(r.m.sessions)
	r.m.mu.Unlock()
	if n != 0 {
		t.Fatalf("%d sessions kept", n)
	}
}

func TestSettingTurnedOffMidTaskReleasesAtOnce(t *testing.T) {
	r := newRig(t, nil)
	r.m.SetState("a", KindAI, StateRunning)
	r.wantHeld(true, "running")
	cfg := r.m.Config()
	cfg.Enabled = false
	if err := r.m.SetConfig(cfg); err != nil {
		t.Fatal(err)
	}
	r.wantHeld(false, "setting off")
	if st := r.m.State(); st.Running != 1 || st.Held {
		t.Fatalf("the count goes on while the hold is off: %+v", st)
	}
	r.m.SetState("b", KindAI, StateRunning)
	r.wantHeld(false, "new task while off")
	cfg.Enabled = true
	if err := r.m.SetConfig(cfg); err != nil {
		t.Fatal(err)
	}
	r.wantHeld(true, "setting on again with tasks running")
}

func TestScreenOptionChangesTheHoldInPlace(t *testing.T) {
	r := newRig(t, nil)
	r.m.SetState("a", KindAI, StateRunning)
	if _, scr := r.h.Held(); !scr || !r.m.State().Screen {
		t.Fatal("screen should be held by default")
	}
	cfg := r.m.Config()
	cfg.ScreenToo = false
	if err := r.m.SetConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if h, scr := r.h.Held(); !h || scr || r.m.State().Screen {
		t.Fatalf("after screenToo=false: held=%v screen=%v state=%+v", h, scr, r.m.State())
	}
	if got := r.h.Log(); len(got) != 2 || got[0] != "acquire:screen" || got[1] != "acquire" {
		t.Fatalf("log %v", got)
	}
}

func TestSleepAndWake(t *testing.T) {
	r := newRig(t, nil)
	r.m.SetState("run", KindAI, StateRunning)
	r.m.SetState("wait", KindAI, StateWaiting)
	// The Mac sleeps for 30 minutes: timers did not run. After the wake the app asks again.
	r.clk.Jump(30 * time.Minute)
	r.m.Refresh()
	r.wantHeld(true, "running task survives the sleep")
	if st := r.m.State(); st.Running != 1 || st.Waiting != 0 {
		t.Fatalf("the grace time ran out during the sleep: %+v", st)
	}
	// Even without a Refresh the timer that was due fires at the first chance
	r2 := newRig(t, nil)
	r2.m.SetState("wait", KindAI, StateWaiting)
	r2.clk.Jump(20 * time.Minute)
	r2.clk.Advance(0)
	r2.wantHeld(false, "timer after wake")
}

func TestAppCrashAndForceKillLeaveNothingBehindByDesign(t *testing.T) {
	// A crash runs no code: the guarantee is in how the assertion is built (checked here), and in the manual check.
	run := &FakeRunner{}
	c := &Caffeinate{Runner: run, Clock: NewFakeClock(), PID: 4242}
	if err := c.Acquire(true); err != nil {
		t.Fatal(err)
	}
	defer c.Release()
	argv := run.Argv(0)
	want := []string{CaffeinatePath, "-d", "-i", "-w", "4242"}
	if len(argv) != len(want) {
		t.Fatalf("argv %v", argv)
	}
	for i := range want {
		if argv[i] != want[i] {
			t.Fatalf("argv %v, want %v", argv, want)
		}
	}
}

func TestEveryExitPathReleases(t *testing.T) {
	r := newRig(t, nil)
	r.m.SetState("a", KindAI, StateRunning)
	r.m.Close()
	r.wantHeld(false, "Close")
	r.m.SetState("b", KindAI, StateRunning)
	r.wantHeld(false, "after Close nothing is taken")
	if r.clk.Pending() != 0 {
		t.Fatal("timer left after Close")
	}
	if acq, rel := r.h.Calls(); acq != 1 || rel != 1 {
		t.Fatalf("calls %d/%d", acq, rel)
	}

	// Forget and Ended release too
	r = newRig(t, nil)
	r.m.SetState("a", KindAI, StateRunning)
	r.m.Forget("a")
	r.wantHeld(false, "Forget")
}

func TestFailedHolderIsReportedAndRetried(t *testing.T) {
	r := newRig(t, nil)
	r.h.SetFail(true)
	r.m.SetState("a", KindAI, StateRunning)
	st := r.m.State()
	if st.Held || st.Error != api.KeepAwakeErrorNone {
		t.Fatalf("state %+v", st)
	}
	r.h.SetFail(false)
	r.clk.Advance(retryInterval)
	r.wantHeld(true, "retry")
	if r.m.State().Error != "" {
		t.Fatalf("error stays: %+v", r.m.State())
	}
}

func TestOneHolderFailingDoesNotStopTheOther(t *testing.T) {
	a, b := &FakeHolder{N: "a"}, &FakeHolder{N: "b"}
	a.SetFail(true)
	m := New(Options{Holders: []Holder{a, b}, Clock: NewFakeClock(), Config: DefaultConfig()})
	defer m.Close()
	m.SetState("x", KindAI, StateRunning)
	st := m.State()
	if h, _ := b.Held(); !h || !st.Held || st.Error != api.KeepAwakeErrorPartial {
		t.Fatalf("state %+v", st)
	}
}

func TestOnChangeOnlyWhenTheStateChanges(t *testing.T) {
	r := newRig(t, nil)
	r.m.SetState("a", KindAI, StateRunning)
	r.m.Touch("a")
	r.m.Touch("a")
	r.mu.Lock()
	n := len(r.got)
	r.mu.Unlock()
	if n != 1 {
		t.Fatalf("%d events, want 1", n)
	}
}

func TestConfigValidationAndSave(t *testing.T) {
	r := newRig(t, nil)
	for _, bad := range []api.KeepAwakeConfig{
		{Enabled: true, GraceMinutes: 7, CeilingHours: 8},
		{Enabled: true, GraceMinutes: 15, CeilingHours: 3},
	} {
		if err := r.m.SetConfig(bad); err == nil {
			t.Fatalf("%+v accepted", bad)
		}
	}
	var saved []api.KeepAwakeConfig
	m := New(Options{Clock: NewFakeClock(), Config: DefaultConfig(), Save: func(c api.KeepAwakeConfig) error { saved = append(saved, c); return nil }})
	defer m.Close()
	want := api.KeepAwakeConfig{Enabled: false, ScreenToo: false, GraceMinutes: 0, CeilingHours: 12}
	if err := m.SetConfig(want); err != nil {
		t.Fatal(err)
	}
	if len(saved) != 1 || saved[0] != want || m.Config() != want {
		t.Fatalf("saved %+v config %+v", saved, m.Config())
	}
}

func TestSaveFailureIsFileIOAndStillApplies(t *testing.T) {
	m := New(Options{Clock: NewFakeClock(), Config: DefaultConfig(), Save: func(api.KeepAwakeConfig) error { return errors.New("disk full") }})
	defer m.Close()
	c := DefaultConfig()
	c.ScreenToo = false
	err := m.SetConfig(c)
	if err == nil || api.AsAppError(err).Code != api.ErrFileIO {
		t.Fatalf("err %v", err)
	}
	if m.Config() != c {
		t.Fatal("not applied")
	}
	// an app error from the saver is passed on as it is
	want := api.NewError(api.ErrFileIO, "x").WithParam("file", "settings.json")
	m2 := New(Options{Clock: NewFakeClock(), Config: DefaultConfig(), Save: func(api.KeepAwakeConfig) error { return want }})
	defer m2.Close()
	if err := m2.SetConfig(c); api.AsAppError(err).Params["file"] != "settings.json" {
		t.Fatalf("err %v", err)
	}
}

func TestTwoQuickConfigChangesAreSavedInTheOrderTheyApply(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var mu sync.Mutex
	var saved []int
	first := true
	m := New(Options{Clock: NewFakeClock(), Config: DefaultConfig(), Save: func(c api.KeepAwakeConfig) error {
		mu.Lock()
		f := first
		first = false
		mu.Unlock()
		if f {
			close(entered)
			<-release
		}
		mu.Lock()
		saved = append(saved, c.GraceMinutes)
		mu.Unlock()
		return nil
	}})
	defer m.Close()
	a := DefaultConfig()
	a.GraceMinutes = 5
	b := DefaultConfig()
	b.GraceMinutes = 60
	done := make(chan struct{}, 2)
	go func() { _ = m.SetConfig(a); done <- struct{}{} }()
	<-entered
	go func() { _ = m.SetConfig(b); done <- struct{}{} }()
	time.Sleep(20 * time.Millisecond) // B must wait for A's save
	close(release)
	<-done
	<-done
	if len(saved) != 2 || saved[0] != 5 || saved[1] != 60 || m.Config().GraceMinutes != 60 {
		t.Fatalf("saved %v, config %+v", saved, m.Config())
	}
}

func TestStoredFormIsZeroSafeAndRoundTrips(t *testing.T) {
	if FromStored(Stored{}) != DefaultConfig() {
		t.Fatalf("zero value is not the defaults: %+v", FromStored(Stored{}))
	}
	for _, c := range []api.KeepAwakeConfig{
		DefaultConfig(),
		{Enabled: false, ScreenToo: false, GraceMinutes: 0, CeilingHours: 12},
		{Enabled: true, ScreenToo: false, GraceMinutes: 60, CeilingHours: 4},
		{Enabled: true, ScreenToo: true, GraceMinutes: 5, CeilingHours: 8},
	} {
		if got := FromStored(ToStored(c)); got != c {
			t.Fatalf("%+v -> %+v", c, got)
		}
	}
	if got := FromStored(Stored{GraceMinutes: 7, CeilingHours: 99}); got.GraceMinutes != 15 || got.CeilingHours != 8 {
		t.Fatalf("values the card does not offer: %+v", got)
	}
}

func TestInvalidInitialConfigFallsBackToDefaults(t *testing.T) {
	m := New(Options{Clock: NewFakeClock(), Config: api.KeepAwakeConfig{}})
	defer m.Close()
	if m.Config() != DefaultConfig() {
		t.Fatalf("%+v", m.Config())
	}
}

// blockingLeftovers blocks Count for one session, once armed, to hold an evaluation between its two sections.
type blockingLeftovers struct {
	inner   *FakeLeftovers
	block   string
	armed   atomic.Bool
	entered chan struct{}
	release chan struct{}
	calls   atomic.Int32
}

func (b *blockingLeftovers) Count(id string) int {
	b.calls.Add(1)
	if id == b.block && b.armed.CompareAndSwap(true, false) {
		b.entered <- struct{}{}
		<-b.release
	}
	return b.inner.Count(id)
}

func TestSessionEndedDuringACountIsNotJudgedByACountItNeverGot(t *testing.T) {
	left := &FakeLeftovers{}
	bl := &blockingLeftovers{inner: left, block: "x", entered: make(chan struct{}), release: make(chan struct{})}
	h := &FakeHolder{}
	m := New(Options{Holders: []Holder{h}, Clock: NewFakeClock(), Leftovers: bl, Config: DefaultConfig()})
	defer m.Close()
	left.Set("x", 1)
	left.Set("y", 2)
	m.SetState("x", KindAI, StateEnded)
	if !m.State().Held {
		t.Fatal("x not held")
	}
	bl.armed.Store(true)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); m.Refresh() }()
	<-bl.entered // the evaluation sits between its two sections, y is not in its snapshot
	go func() { defer wg.Done(); m.SetState("y", KindAI, StateEnded) }()
	eventually(t, "y recorded", func() bool {
		m.mu.Lock()
		defer m.mu.Unlock()
		return m.sessions["y"] != nil
	})
	close(bl.release)
	wg.Wait()
	m.mu.Lock()
	_, kept := m.sessions["y"]
	m.mu.Unlock()
	if !kept {
		t.Fatal("y's record was dropped by an evaluation that never counted its programs")
	}
	if st := m.State(); st.Leftover != 2 || st.Programs != 3 {
		t.Fatalf("state %+v", st)
	}
	left.Set("x", 0)
	m.Refresh()
	if st := m.State(); !st.Held || st.Leftover != 1 || st.Programs != 2 {
		t.Fatalf("x's last program ended, y's two still run: %+v", st)
	}
}

func TestOnChangeDeliversTheLatestStateInOrder(t *testing.T) {
	var mu sync.Mutex
	var got []api.KeepAwakeState
	first := make(chan struct{})
	unblock := make(chan struct{})
	n := 0
	h := &FakeHolder{}
	m := New(Options{Holders: []Holder{h}, Clock: NewFakeClock(), Config: DefaultConfig(), OnChange: func(s api.KeepAwakeState) {
		mu.Lock()
		n++
		k := n
		mu.Unlock()
		if k == 1 {
			close(first)
			<-unblock // the first delivery is slow
		}
		mu.Lock()
		got = append(got, s)
		mu.Unlock()
	}})
	defer m.Close()
	go m.SetState("a", KindAI, StateRunning) // held = true
	<-first
	done := make(chan struct{})
	go func() { m.SetState("a", KindAI, StateEnded); close(done) }() // released, while the first is still being delivered
	eventually(t, "the second change happened", func() bool { return !m.State().Held })
	close(unblock)
	<-done
	eventually(t, "both delivered", func() bool { mu.Lock(); defer mu.Unlock(); return len(got) >= 1 && got[len(got)-1] == m.State() })
	mu.Lock()
	defer mu.Unlock()
	if last := got[len(got)-1]; last.Held || last != m.State() {
		t.Fatalf("the last delivered state %+v differs from State() %+v", last, m.State())
	}
}

func TestTouchOnACountedRunningSessionDoesNoWork(t *testing.T) {
	left := &FakeLeftovers{}
	cnt := &blockingLeftovers{inner: left, block: "none"}
	h := &FakeHolder{}
	clk := NewFakeClock()
	m := New(Options{Holders: []Holder{h}, Clock: clk, Leftovers: cnt, Config: DefaultConfig()})
	defer m.Close()
	m.SetState("a", KindAI, StateRunning)
	acq, rel := h.Calls()
	pending, calls := clk.Pending(), cnt.calls.Load()
	clk.Jump(time.Hour)
	for i := 0; i < 5; i++ {
		m.Touch("a")
	}
	if a2, r2 := h.Calls(); a2 != acq || r2 != rel || cnt.calls.Load() != calls || clk.Pending() != pending {
		t.Fatalf("Touch worked: holder %d/%d, leftovers %d, timers %d", a2, r2, cnt.calls.Load(), clk.Pending())
	}
	// the later time still counts: the ceiling is 8 hours after the last Touch, not after the first event
	clk.Advance(7*time.Hour + 59*time.Minute)
	if !m.State().Held {
		t.Fatal("released before the ceiling that counts from the last Touch")
	}
	clk.Advance(time.Minute)
	if m.State().Held || !m.State().CeilingHit {
		t.Fatalf("state %+v", m.State())
	}
}

func TestActivityIsHeldWhileATaskCountsEvenWithTheSettingOff(t *testing.T) {
	act := &FakeActivity{}
	caf := &FakeHolder{N: "caffeinate"}
	cfg := DefaultConfig()
	cfg.Enabled = false
	m := New(Options{Holders: []Holder{caf, &Activity{API: act}}, Clock: NewFakeClock(), Config: cfg})
	defer m.Close()
	m.SetState("a", KindAI, StateRunning)
	if act.Active() != 1 {
		t.Fatal("App Nap protection missing while a task runs")
	}
	if h, _ := caf.Held(); h || m.State().Held || m.State().Screen {
		t.Fatalf("the setting is off: %+v", m.State())
	}
	cfg.Enabled = true
	if err := m.SetConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if h, _ := caf.Held(); !h || !m.State().Held || act.Active() != 1 || act.Begun() != 1 {
		t.Fatalf("%+v active %d begun %d", m.State(), act.Active(), act.Begun())
	}
	m.SetState("a", KindAI, StateEnded)
	if act.Active() != 0 {
		t.Fatal("activity left after the last task")
	}
}

func TestOnlyTheActivityHoldingIsNotAHold(t *testing.T) {
	run := &FakeRunner{}
	run.SetFail(errors.New("no caffeinate"))
	act := &FakeActivity{}
	clk := NewFakeClock()
	m := New(Options{Holders: []Holder{&Caffeinate{Runner: run, Clock: clk, PID: 1}, &Activity{API: act}}, Clock: clk, Config: DefaultConfig()})
	defer m.Close()
	m.SetState("a", KindAI, StateRunning)
	st := m.State()
	if st.Held || st.Screen || st.Error != api.KeepAwakeErrorNone || act.Active() != 1 {
		t.Fatalf("the activity holds no sleep assertion and no display: %+v", st)
	}
}

func TestFailedScreenChangeKeepsTheOldHoldAndTheStateSaysSo(t *testing.T) {
	run := &FakeRunner{}
	clk := NewFakeClock()
	m := New(Options{Holders: []Holder{&Caffeinate{Runner: run, Clock: clk, PID: 1}}, Clock: clk, Config: DefaultConfig()})
	defer m.Close()
	m.SetState("a", KindAI, StateRunning)
	if st := m.State(); !st.Held || !st.Screen || run.Alive() != 1 {
		t.Fatalf("%+v", st)
	}
	run.SetFail(errors.New("no"))
	cfg := m.Config()
	cfg.ScreenToo = false
	if err := m.SetConfig(cfg); err != nil {
		t.Fatal(err)
	}
	// the old caffeinate (with -d) still runs: the state says what is true
	if st := m.State(); !st.Held || !st.Screen || st.Error != api.KeepAwakeErrorPartial || run.Alive() != 1 {
		t.Fatalf("a failed change keeps the old hold: %+v alive %d", st, run.Alive())
	}
	// flipped back before the retry: nothing to do, nothing wrong
	cfg.ScreenToo = true
	if err := m.SetConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if st := m.State(); !st.Held || !st.Screen || st.Error != "" || run.Starts() != 1 {
		t.Fatalf("%+v starts %d", st, run.Starts())
	}
	// a change that works replaces it: the new one starts before the old one ends
	run.SetFail(nil)
	cfg.ScreenToo = false
	if err := m.SetConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if st := m.State(); !st.Held || st.Screen || run.Alive() != 1 || run.Starts() != 2 {
		t.Fatalf("%+v alive %d starts %d", st, run.Alive(), run.Starts())
	}
}

func TestLostCaffeinateIsSeenAndRetried(t *testing.T) {
	run := &FakeRunner{}
	clk := NewFakeClock()
	m := New(Options{Holders: []Holder{&Caffeinate{Runner: run, Clock: clk, PID: 1}}, Clock: clk, Config: DefaultConfig()})
	defer m.Close()
	m.SetState("a", KindAI, StateRunning)
	run.SetFail(errors.New("no")) // the restart will fail
	run.Proc(0).Exit()            // killed from outside
	eventually(t, "the loss is reported", func() bool {
		st := m.State()
		return !st.Held && !st.Screen && st.Error == api.KeepAwakeErrorNone
	})
	run.SetFail(nil)
	clk.Advance(retryInterval) // the manager's own retry (and caffeinate's back-off) find it working again
	eventually(t, "held again", func() bool { st := m.State(); return st.Held && st.Screen && st.Error == "" })
}
