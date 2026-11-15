package app

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestCloseAction(t *testing.T) {
	l := NewLifecycle()
	if l.CloseAction(false) != CloseQuit {
		t.Error("no tasks, no setting: closing quits")
	}
	if l.CloseAction(true) != CloseHide {
		t.Error("keep-running setting hides")
	}
	l.SetTasksRunning(func() bool { return true })
	if l.CloseAction(false) != CloseHide {
		t.Error("tasks running: closing hides")
	}
}

// waitFor polls until cond holds (the decision runs on a goroutine).
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for i := 0; i < 200; i++ {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func liveLifecycle() *Lifecycle {
	l := NewLifecycle()
	l.SetTasksRunning(func() bool { return true })
	l.SetLoggingOut(func() bool { return false })
	return l
}

func TestQuitAnswers(t *testing.T) {
	for _, tc := range []struct {
		ans        QuitAnswer
		quits      int32
		background bool
	}{{QuitNow, 1, false}, {QuitBackground, 0, true}, {QuitCancel, 0, false}} {
		l := liveLifecycle()
		var quits int32
		var bg atomic.Bool
		answered := make(chan struct{})
		l.SetQuitDecision(func(context.Context) QuitAnswer { defer close(answered); return tc.ans })
		l.SetOnBackground(func() { bg.Store(true) })
		l.SetQuit(func() {
			atomic.AddInt32(&quits, 1)
			if !l.ShouldQuit() { // the terminate Quit triggers is approved, not asked again
				t.Error("approved quit must go through")
			}
		})
		if l.ShouldQuit() {
			t.Fatalf("answer %d: the first call must cancel this terminate", tc.ans)
		}
		<-answered
		waitFor(t, "decision to finish", func() bool { return !l.deciding.Load() })
		if atomic.LoadInt32(&quits) != tc.quits || bg.Load() != tc.background {
			t.Errorf("answer %d: quits=%d background=%v", tc.ans, quits, bg.Load())
		}
	}
}

func TestShouldQuitNeverBlocks(t *testing.T) {
	l := liveLifecycle()
	release := make(chan struct{})
	l.SetQuitDecision(func(context.Context) QuitAnswer { <-release; return QuitNow })
	var quits int32
	l.SetQuit(func() { atomic.AddInt32(&quits, 1); l.ShouldQuit() })
	start := time.Now()
	if l.ShouldQuit() || time.Since(start) > 100*time.Millisecond {
		t.Fatal("ShouldQuit must return false at once while the dialog is pending")
	}
	if l.ShouldQuit() { // a second Cmd+Q while the dialog is open
		t.Fatal("a pending dialog refuses further quits")
	}
	close(release)
	waitFor(t, "the quit", func() bool { return atomic.LoadInt32(&quits) == 1 })
	time.Sleep(20 * time.Millisecond)
	if atomic.LoadInt32(&quits) != 1 {
		t.Fatalf("Quit called %d times", quits)
	}
}

func TestNothingLiveQuitsWithoutAsking(t *testing.T) {
	l := NewLifecycle()
	l.SetLoggingOut(func() bool { return false })
	asked := false
	l.SetQuitDecision(func(context.Context) QuitAnswer { asked = true; return QuitCancel })
	if !l.ShouldQuit() || asked {
		t.Fatal("with no live task the quit goes through and nothing is asked")
	}
}

func TestLogoutIsNeverBlocked(t *testing.T) {
	for _, ans := range []QuitAnswer{QuitNow, QuitBackground, QuitCancel} {
		l := liveLifecycle()
		l.SetLoggingOut(func() bool { return true })
		asked := false
		l.SetQuitDecision(func(context.Context) QuitAnswer { asked = true; return ans })
		if !l.ShouldQuit() || asked || !l.LoggingOut() {
			t.Errorf("answer %d: during logout ShouldQuit must be true at once (asked=%v)", ans, asked)
		}
	}
}

func TestApprovedQuitIsOneShot(t *testing.T) {
	l := liveLifecycle()
	asked := make(chan struct{}, 4)
	l.SetQuitDecision(func(context.Context) QuitAnswer { asked <- struct{}{}; return QuitCancel })
	// a successful restart consumes the approval
	if err := l.QuitApproved(func() error {
		if !l.ShouldQuit() {
			t.Error("approved quit must pass")
		}
		return nil
	}); err != nil || len(asked) != 0 {
		t.Fatalf("err=%v asked=%d", err, len(asked))
	}
	// a failed restart leaves no approval behind: the next quit asks
	if err := l.QuitApproved(func() error { return errors.New("bundle gate refused") }); err == nil {
		t.Fatal("error must be returned")
	}
	if l.ShouldQuit() {
		t.Fatal("the next quit after a failed restart must ask")
	}
	waitFor(t, "the dialog", func() bool { return len(asked) == 1 })
}

func TestWndProcTracksSessionEnd(t *testing.T) {
	sessionEndFlag.Store(false)
	if _, consumed := WndProc(0, wmQueryEndSession, 0, 0); consumed || !sessionEndFlag.Load() {
		t.Fatal("QUERYENDSESSION sets the flag and is not consumed")
	}
	if _, consumed := WndProc(0, wmEndSession, 0, 0); !consumed || sessionEndFlag.Load() { // cancelled
		t.Fatal("a cancelled session end clears the flag and is consumed (Wails would quit on it)")
	}
	if _, consumed := WndProc(0, wmEndSession, 1, 0); consumed || !sessionEndFlag.Load() {
		t.Fatal("a real session end sets the flag and is left to Wails")
	}
}

func TestHidingLastWindowDoesNotEndApp(t *testing.T) {
	if TerminateAfterLastWindowClosed {
		t.Fatal("must be false")
	}
	b, err := os.ReadFile("../../main.go")
	if err != nil {
		t.Fatal(err)
	}
	src := strings.Join(strings.Fields(string(b)), " ")
	if !strings.Contains(src, "ApplicationShouldTerminateAfterLastWindowClosed: app.TerminateAfterLastWindowClosed") ||
		!strings.Contains(src, "DisableQuitOnLastWindowClosed: !app.TerminateAfterLastWindowClosed") {
		t.Fatal("main.go must take both last-window options from app.TerminateAfterLastWindowClosed")
	}
}
