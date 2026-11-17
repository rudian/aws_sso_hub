package taskstate

import (
	"sync"
	"testing"
	"time"
)

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func newT(t *testing.T) (*Tracker, *[]Info, *clock) {
	t.Helper()
	var ch []Info
	c := &clock{t: time.Unix(1000, 0)}
	return NewTracker(Options{Now: c.now}, func(i Info) { ch = append(ch, i) }), &ch, c
}

var seq int64

func ev(task string, tool Tool, event string, f func(*Report)) Report {
	seq++
	r := Report{Task: task, Tool: tool, Event: event, At: seq}
	if f != nil {
		f(&r)
	}
	return r
}

func state(t *testing.T, tr *Tracker, task string) Info {
	t.Helper()
	i, ok := tr.Get(task)
	if !ok {
		t.Fatal("no task")
	}
	return i
}

func TestClaudeEventToState(t *testing.T) {
	tr, changes, _ := newT(t)
	tr.Register("a", ToolClaude, ModeHooks)
	step := func(r Report, st State, reason Reason) {
		t.Helper()
		tr.Apply(r)
		if i := state(t, tr, "a"); i.State != st || i.Reason != reason {
			t.Fatalf("%s: %s/%s want %s/%s", r.Event, i.State, i.Reason, st, reason)
		}
	}
	step(ev("a", ToolClaude, "SessionStart", func(r *Report) { r.Session = "s1"; r.Source = "startup" }), StateStarting, ReasonNone)
	step(ev("a", ToolClaude, "UserPromptSubmit", func(r *Report) { r.Mode = "bypassPermissions" }), StateRunning, ReasonNone)
	step(ev("a", ToolClaude, "PermissionRequest", nil), StateWaiting, ReasonPermission)
	step(ev("a", ToolClaude, "UserPromptSubmit", nil), StateRunning, ReasonNone)
	step(ev("a", ToolClaude, "PreToolUse", nil), StateWaiting, ReasonQuestion)
	step(ev("a", ToolClaude, "Notification", func(r *Report) { r.NotificationType = "elicitation_dialog" }), StateWaiting, ReasonQuestion)
	step(ev("a", ToolClaude, "UserPromptSubmit", nil), StateRunning, ReasonNone)
	step(ev("a", ToolClaude, "Stop", nil), StateWaiting, ReasonTurnEnd)
	step(ev("a", ToolClaude, "Notification", func(r *Report) { r.NotificationType = "idle_prompt" }), StateWaiting, ReasonIdle)
	step(ev("a", ToolClaude, "UserPromptSubmit", nil), StateRunning, ReasonNone)
	step(ev("a", ToolClaude, "StopFailure", func(r *Report) { r.ErrorKind = "rate_limit"; r.ConvMark = "none" }), StateWaiting, ReasonLimit)
	if i := state(t, tr, "a"); i.ErrorKind != "rate_limit" || i.Session != "s1" || i.PermissionMode != "bypassPermissions" || i.SessionSource != "startup" {
		t.Fatalf("%+v", i)
	}
	// idle_prompt after a limit must not hide the limit.
	step(ev("a", ToolClaude, "Notification", func(r *Report) { r.NotificationType = "idle_prompt" }), StateWaiting, ReasonLimit)
	step(ev("a", ToolClaude, "Notification", func(r *Report) { r.NotificationType = "quota_auto_resume_fired" }), StateRunning, ReasonNone)
	step(ev("a", ToolClaude, "StopFailure", func(r *Report) { r.ErrorKind = "server_error" }), StateWaiting, ReasonError)
	step(ev("a", ToolClaude, "SessionEnd", nil), StateWaiting, ReasonError) // /clear, resume, logout: the tool runs on
	tr.ProcessExited("a")
	step(ev("a", ToolClaude, "Stop", nil), StateEnded, ReasonNone) // late event of an ended session
	if len(*changes) == 0 {
		t.Fatal("no change callbacks")
	}
}

func TestEndMarkerAndWaitingCount(t *testing.T) {
	tr, _, _ := newT(t)
	tr.Register("a", ToolClaude, ModeHooks)
	tr.Register("b", ToolClaude, ModeHooks)
	tr.Apply(ev("a", ToolClaude, "Stop", func(r *Report) { r.EndMarker = "done" }))
	tr.Apply(ev("b", ToolClaude, "UserPromptSubmit", nil))
	if state(t, tr, "a").EndMarker != "done" || tr.WaitingCount() != 1 {
		t.Fatal("marker or count")
	}
	tr.Apply(ev("a", ToolClaude, "UserPromptSubmit", nil))
	if state(t, tr, "a").EndMarker != "" || tr.WaitingCount() != 0 {
		t.Fatal("marker not cleared")
	}
}

func TestOpenCodeMapping(t *testing.T) {
	tr, _, _ := newT(t)
	tr.Register("o", ToolOpenCode, ModeHooks)
	cases := []struct {
		e  string
		st State
		rs Reason
	}{
		{"session.execution.started", StateRunning, ReasonNone},
		{"permission.asked", StateWaiting, ReasonPermission},
		{"permission.replied", StateRunning, ReasonNone},
		{"session.execution.succeeded", StateWaiting, ReasonTurnEnd},
		{"session.execution.started", StateRunning, ReasonNone},
		{"session.execution.interrupted", StateWaiting, ReasonTurnEnd},
		{"session.execution.started", StateRunning, ReasonNone},
		{"session.execution.failed", StateWaiting, ReasonError},
		{"session.idle", StateWaiting, ReasonTurnEnd},
	}
	for _, c := range cases {
		tr.Apply(ev("o", ToolOpenCode, c.e, func(r *Report) { r.Session = "ses_1" }))
		if i := state(t, tr, "o"); i.State != c.st || i.Reason != c.rs {
			t.Fatalf("%s: %s/%s", c.e, i.State, i.Reason)
		}
	}
}

func TestCodexTurnComplete(t *testing.T) {
	tr, _, _ := newT(t)
	tr.Register("c", ToolCodex, ModeTurnOnly)
	tr.Observe("c", Alive)
	if state(t, tr, "c").State != StateNoSignal {
		t.Fatal("notify-only tool alive must read no signal")
	}
	tr.Apply(ev("c", ToolCodex, EventCodexTurnComplete, func(r *Report) { r.Session = "th" }))
	if i := state(t, tr, "c"); i.State != StateWaiting || i.Reason != ReasonTurnEnd || i.Session != "th" {
		t.Fatalf("%+v", i)
	}
	tr.Register("h", ToolCodex, ModeHooks)
	tr.Apply(ev("h", ToolCodex, "UserPromptSubmit", nil))
	tr.Apply(ev("h", ToolCodex, "PermissionRequest", nil))
	if i := state(t, tr, "h"); i.State != StateWaiting || i.Reason != ReasonPermission {
		t.Fatalf("%+v", i)
	}
}

func TestOutOfOrderUnknownAndForeignToolReportsAreIgnored(t *testing.T) {
	tr, _, _ := newT(t)
	tr.Register("a", ToolClaude, ModeHooks)
	tr.Apply(Report{Task: "a", Tool: ToolClaude, Event: "Stop", At: 100})
	if tr.Apply(Report{Task: "a", Tool: ToolClaude, Event: "UserPromptSubmit", At: 50}) {
		t.Fatal("older report applied")
	}
	if state(t, tr, "a").State != StateWaiting {
		t.Fatal("state moved")
	}
	if tr.Apply(Report{Task: "zz", Tool: ToolClaude, Event: "Stop", At: 1}) {
		t.Fatal("unknown task")
	}
	if tr.Apply(Report{Task: "a", Tool: ToolCodex, Event: "UserPromptSubmit", At: 200}) {
		t.Fatal("other tool")
	}
}

func TestFirstPromptIsHandedOnceAndNeverStored(t *testing.T) {
	tr, _, _ := newT(t)
	tr.Register("a", ToolClaude, ModeHooks)
	var (
		mu  sync.Mutex
		got []string
	)
	tr.ExpectFirstPrompt("a", func(p string) { mu.Lock(); got = append(got, p); mu.Unlock() })
	tr.Apply(ev("a", ToolClaude, "UserPromptSubmit", func(r *Report) { r.Prompt = "first" }))
	tr.Apply(ev("a", ToolClaude, "UserPromptSubmit", func(r *Report) { r.Prompt = "second" }))
	waitUntil(t, func() bool { mu.Lock(); defer mu.Unlock(); return len(got) >= 1 })
	time.Sleep(20 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 1 || got[0] != "first" {
		t.Fatalf("%v", got)
	}
	// A prompt for a task that was not armed goes nowhere.
	tr.Register("b", ToolClaude, ModeHooks)
	tr.Apply(ev("b", ToolClaude, "UserPromptSubmit", func(r *Report) { r.Prompt = "x" }))
}

func waitUntil(t *testing.T, f func() bool) {
	t.Helper()
	for i := 0; i < 200; i++ {
		if f() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timeout")
}

func TestFallbackObserveAndExit(t *testing.T) {
	tr, _, c := newT(t)
	tr.Register("n", ToolOpenCode, ModeNone)
	tr.Observe("n", Alive)
	if state(t, tr, "n").State != StateNoSignal {
		t.Fatal("no-signal tool")
	}
	tr.Observe("n", Exited)
	if state(t, tr, "n").State != StateNoSignal {
		t.Fatal("one miss must not end it")
	}
	tr.Observe("n", Alive)
	tr.Observe("n", Alive) // FU-A2-05a: two alive verdicts in a row before exited verdicts count
	tr.Observe("n", Exited)
	tr.Observe("n", Exited)
	if state(t, tr, "n").State != StateEnded {
		t.Fatal("two misses end it")
	}
	tr.ProcessExited("n")
	tr.Apply(ev("n", ToolOpenCode, "session.execution.started", nil))
	if state(t, tr, "n").State != StateEnded {
		t.Fatal("exit is final")
	}
	// A hook task shows no signal only after the grace.
	tr.Register("h", ToolClaude, ModeHooks)
	tr.Observe("h", Alive)
	if state(t, tr, "h").State != StateStarting {
		t.Fatal("too early")
	}
	c.t = c.t.Add(21 * time.Second)
	tr.Observe("h", Alive)
	if state(t, tr, "h").State != StateNoSignal {
		t.Fatal("grace passed")
	}
	tr.Apply(ev("h", ToolClaude, "UserPromptSubmit", nil))
	if state(t, tr, "h").State != StateRunning {
		t.Fatal("a hook beats no-signal")
	}
	tr.Observe("h", Unknown)
}

func TestExitedBeforeAliveDoesNotEndATaskWhileTheShellStarts(t *testing.T) {
	tr, _, _ := newT(t)
	tr.Register("a", ToolClaude, ModeHooks)
	tr.Observe("a", Exited)
	tr.Observe("a", Exited)
	tr.Observe("a", Exited)
	tr.Apply(ev("a", ToolClaude, "SessionStart", nil))
	tr.Apply(ev("a", ToolClaude, "UserPromptSubmit", nil))
	if state(t, tr, "a").State != StateRunning {
		t.Fatalf("%+v", state(t, tr, "a"))
	}
}

func TestFallbackEndIsNotFinalButProcessExitIs(t *testing.T) {
	tr, _, _ := newT(t)
	tr.Register("a", ToolClaude, ModeHooks)
	tr.Apply(ev("a", ToolClaude, "SessionStart", nil)) // signaled
	tr.Observe("a", Exited)
	tr.Observe("a", Exited)
	if state(t, tr, "a").State != StateEnded {
		t.Fatal("two verdicts after a signal end it")
	}
	tr.Apply(ev("a", ToolClaude, "UserPromptSubmit", nil))
	if state(t, tr, "a").State != StateRunning {
		t.Fatal("a later hook revives it")
	}
	tr.ProcessExited("a")
	tr.Apply(ev("a", ToolClaude, "UserPromptSubmit", nil))
	if state(t, tr, "a").State != StateEnded {
		t.Fatal("process exit is final")
	}
}

func TestSignaledTaskIsNeverNoSignalAndSessionEndKeepsIt(t *testing.T) {
	tr, changes, c := newT(t)
	tr.Register("a", ToolClaude, ModeHooks)
	tr.Apply(ev("a", ToolClaude, "SessionStart", nil))
	c.t = c.t.Add(21 * time.Second)
	tr.Observe("a", Alive)
	if state(t, tr, "a").State != StateStarting {
		t.Fatalf("hooks work: %+v", state(t, tr, "a"))
	}
	// /clear: SessionEnd then SessionStart, never "ended".
	tr.Apply(ev("a", ToolClaude, "UserPromptSubmit", nil))
	n := len(*changes)
	tr.Apply(ev("a", ToolClaude, "SessionEnd", nil))
	tr.Apply(ev("a", ToolClaude, "SessionStart", func(r *Report) { r.Source = "clear" }))
	for _, i := range (*changes)[n:] {
		if i.State == StateEnded {
			t.Fatal("SessionEnd must not end the task")
		}
	}
}

func TestOpenCodePluginLoadedCountsAsSignal(t *testing.T) {
	tr, _, c := newT(t)
	tr.Register("o", ToolOpenCode, ModeHooks)
	tr.Apply(ev("o", ToolOpenCode, EventPluginLoaded, nil))
	c.t = c.t.Add(30 * time.Second)
	tr.Observe("o", Alive)
	if state(t, tr, "o").State != StateStarting {
		t.Fatal("loaded plugin is a signal")
	}
}

func TestOwnerInput(t *testing.T) {
	tr, _, _ := newT(t)
	tr.Register("a", ToolClaude, ModeHooks)
	tr.Apply(ev("a", ToolClaude, "PermissionRequest", nil))
	tr.OwnerInput("a")
	if i := state(t, tr, "a"); i.State != StateRunning || i.Reason != ReasonNone {
		t.Fatalf("%+v", i)
	}
	tr.Apply(ev("a", ToolClaude, "Stop", nil))
	tr.OwnerInput("a") // a finished turn stays waiting for a hook tool
	if state(t, tr, "a").Reason != ReasonTurnEnd {
		t.Fatal("turn end must stay")
	}
	tr.Register("c", ToolCodex, ModeTurnOnly)
	tr.Apply(ev("c", ToolCodex, EventCodexTurnComplete, nil))
	tr.OwnerInput("c")
	if state(t, tr, "c").State != StateNoSignal {
		t.Fatalf("%+v", state(t, tr, "c"))
	}
}

func TestChangesAreDeliveredInOrderEvenWithABlockingPromptCallback(t *testing.T) {
	var mu sync.Mutex
	var last Info
	tr := NewTracker(Options{}, func(i Info) { mu.Lock(); last = i; mu.Unlock() })
	tr.Register("a", ToolClaude, ModeHooks)
	release := make(chan struct{})
	tr.ExpectFirstPrompt("a", func(string) { <-release })
	done := make(chan struct{})
	go func() {
		tr.Apply(ev("a", ToolClaude, "UserPromptSubmit", func(r *Report) { r.Prompt = "p" }))
		close(done)
	}()
	<-done // Apply returned although the callback blocks
	tr.Apply(ev("a", ToolClaude, "Stop", nil))
	mu.Lock()
	got := last
	mu.Unlock()
	if want := state(t, tr, "a"); got.State != want.State || got.Reason != want.Reason {
		t.Fatalf("delivered %s/%s, tracker %s/%s", got.State, got.Reason, want.State, want.Reason)
	}
	close(release)
}

func TestConcurrentChangesEndWithTheTrackersState(t *testing.T) {
	for n := 0; n < 300; n++ {
		var mu sync.Mutex
		var last Info
		tr := NewTracker(Options{}, func(i Info) { mu.Lock(); last = i; mu.Unlock() })
		tr.Register("a", ToolClaude, ModeHooks)
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); tr.Apply(Report{Task: "a", Tool: ToolClaude, Event: "UserPromptSubmit"}) }()
		go func() { defer wg.Done(); tr.ProcessExited("a") }()
		wg.Wait()
		mu.Lock()
		got := last
		mu.Unlock()
		if want := state(t, tr, "a"); got.State != want.State {
			t.Fatalf("delivered %s, tracker %s", got.State, want.State)
		}
	}
}

// FU-A2-05a: helpers that run while the login shell starts must not make a task look ended, then "running (no
// signal)".
func TestShellStartupHelpersDoNotEndOrMisleadAHooksTask(t *testing.T) {
	tr, _, c := newT(t)
	tr.Register("a", ToolClaude, ModeHooks)
	tr.Observe("a", Alive) // path_helper in the foreground
	tr.Observe("a", Exited)
	tr.Observe("a", Exited)
	tr.Observe("a", Exited)
	if st := state(t, tr, "a").State; st != StateStarting {
		t.Fatalf("a hooks task that sent nothing is never ended by the fallback, got %s", st)
	}
	c.t = c.t.Add(25 * time.Second)
	tr.Observe("a", Alive)
	if st := state(t, tr, "a").State; st != StateNoSignal {
		t.Fatalf("after the grace: %s", st)
	}
	tr.Apply(ev("a", ToolClaude, "SessionStart", nil)) // a late SessionStart
	if st := state(t, tr, "a").State; st != StateStarting {
		t.Fatalf("SessionStart moves running (no signal) back to starting, got %s", st)
	}
	tr.Observe("a", Alive)
	if st := state(t, tr, "a").State; st != StateStarting {
		t.Fatalf("a task with a report is not shown as no signal again, got %s", st)
	}

	tr.Register("o", ToolOpenCode, ModeHooks)
	c.t = c.t.Add(25 * time.Second)
	tr.Observe("o", Alive)
	if st := state(t, tr, "o").State; st != StateNoSignal {
		t.Fatalf("%s", st)
	}
	tr.Apply(ev("o", ToolOpenCode, EventPluginLoaded, nil))
	if st := state(t, tr, "o").State; st != StateStarting {
		t.Fatalf("plugin.loaded moves it back to starting, got %s", st)
	}
}

func TestOneAliveVerdictIsNotEnoughForATaskWithoutHooks(t *testing.T) {
	for _, mode := range []SignalMode{ModeNone, ModeTurnOnly} {
		tr, _, _ := newT(t)
		tr.Register("n", ToolCodex, mode)
		tr.Observe("n", Alive) // one helper caught
		tr.Observe("n", Exited)
		tr.Observe("n", Exited)
		tr.Observe("n", Exited)
		if st := state(t, tr, "n").State; st == StateEnded {
			t.Fatalf("mode %d: a single alive verdict must not arm the exit verdicts", mode)
		}
		tr.Observe("n", Alive)
		tr.Observe("n", Alive)
		tr.Observe("n", Exited)
		tr.Observe("n", Exited)
		if st := state(t, tr, "n").State; st != StateEnded {
			t.Fatalf("mode %d: two alive verdicts in a row arm them, got %s", mode, st)
		}
	}
}

// FU-A2-05b: any accepted report revives a fallback end, and a revived task needs its verdicts again.
func TestAnyReportRevivesAFallbackEnd(t *testing.T) {
	for _, event := range []string{"PermissionRequest", "Stop", "StopFailure", "PreToolUse", "Notification"} {
		tr, _, _ := newT(t)
		tr.Register("a", ToolClaude, ModeHooks)
		tr.Apply(ev("a", ToolClaude, "SessionStart", nil))
		tr.Observe("a", Exited)
		tr.Observe("a", Exited)
		if state(t, tr, "a").State != StateEnded {
			t.Fatalf("%s: setup", event)
		}
		tr.Apply(ev("a", ToolClaude, event, func(r *Report) { r.NotificationType = "permission_prompt" }))
		if st := state(t, tr, "a").State; st != StateWaiting {
			t.Fatalf("%s after a fallback end: %s", event, st)
		}
	}
	tr, _, _ := newT(t)
	tr.Register("a", ToolClaude, ModeHooks)
	tr.Apply(ev("a", ToolClaude, "SessionStart", nil))
	tr.Observe("a", Exited)
	tr.Observe("a", Exited)
	tr.Apply(ev("a", ToolClaude, "PermissionRequest", nil))
	if i := state(t, tr, "a"); i.State != StateWaiting || i.Reason != ReasonPermission {
		t.Fatalf("%+v", i)
	}
}

func TestARevivedTaskNeedsItsExitVerdictsAgain(t *testing.T) {
	tr, _, _ := newT(t)
	tr.Register("a", ToolClaude, ModeHooks)
	tr.Apply(ev("a", ToolClaude, "SessionStart", nil))
	tr.Observe("a", Exited)
	tr.Observe("a", Exited) // ended
	tr.Apply(ev("a", ToolClaude, "UserPromptSubmit", nil))
	tr.Observe("a", Exited) // one verdict only
	if st := state(t, tr, "a").State; st != StateRunning {
		t.Fatalf("one exited verdict after a revival must not end it, got %s", st)
	}
	tr.Observe("a", Exited)
	if st := state(t, tr, "a").State; st != StateEnded {
		t.Fatalf("two do, got %s", st)
	}
}

func TestProcessExitStaysFinalForEveryReport(t *testing.T) {
	tr, _, _ := newT(t)
	tr.Register("a", ToolClaude, ModeHooks)
	tr.ProcessExited("a")
	for _, e := range []string{"SessionStart", "UserPromptSubmit", "PermissionRequest", "Stop"} {
		tr.Apply(ev("a", ToolClaude, e, nil))
		if st := state(t, tr, "a").State; st != StateEnded {
			t.Fatalf("%s revived an exited process: %s", e, st)
		}
	}
}

// FU-A2-05c
func TestCanAnswer(t *testing.T) {
	yes := []string{"\r", "1", "y2", "\x1b", "ok\r", "\x1b[Ayes\r", "\x1bx3"}
	no := []string{"", "\x1b[I", "\x1b[O", "\x1b[12;40R", "\x1b[?62;4c", "\x1b]11;rgb:0000/0000/0000\x07", "\x1b]11;rgb:0000/0000/0000\x1b\\", "\x1b[A", "\x1b[B", "\x1bOA", "abc", "\x1b[I\x1b[O", "\x1b[<0;10;5M"}
	for _, s := range yes {
		if !CanAnswer([]byte(s)) {
			t.Errorf("%q should be an answer", s)
		}
	}
	for _, s := range no {
		if CanAnswer([]byte(s)) {
			t.Errorf("%q cannot answer anything", s)
		}
	}
}

func TestIsTyping(t *testing.T) {
	yes := []string{"a", "\r", "\x1b", "\x1b[A", "\x1b[3~", "\x1bOA", "x\x1b[I", "\x1bx"}
	no := []string{"", "\x1b[I", "\x1b[O", "\x1b[12;40R", "\x1b[?62;4c", "\x1b]11;rgb:0000/0000/0000\x07", "\x1b]11;x\x1b\\", "\x1b[<0;10;5M", "\x1b[I\x1b[O", "\x1b[>0;276;0c"}
	for _, s := range yes {
		if !IsTyping([]byte(s)) {
			t.Errorf("%q is typing", s)
		}
	}
	for _, s := range no {
		if IsTyping([]byte(s)) {
			t.Errorf("%q is not typing", s)
		}
	}
}
