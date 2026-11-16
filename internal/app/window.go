package app

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// TerminateAfterLastWindowClosed is the value main.go gives Wails for "quit when the last window is gone". It
// is false on purpose: closing the window while tasks run, and "keep running in the background", only hide it
// (S0-01 saw the app quit at Hide() while this was true). The quit decision below ends the app instead.
const TerminateAfterLastWindowClosed = false

// LogoutQuitLimit is the longest the quit dialog may stay open while the system logs out or restarts (design
// 4.4 and 5.6b). Wails 3.1.0 can only answer "quit" or "cancel" to a terminate request (it has no
// NSTerminateLater), so no countdown is possible yet: during a logout the app quits at once, without a dialog.
// The constant is kept only as the documented limit; nothing uses it until Wails can answer NSTerminateLater.
const LogoutQuitLimit = 20 * time.Second

// QuitAnswer is what the quit decision hook answers.
type QuitAnswer int

const (
	QuitNow        QuitAnswer = iota // stop everything and exit
	QuitBackground                   // hide the window, keep the app and its tasks running
	QuitCancel                       // do nothing
)

// CloseAction is what closing the window does.
type CloseAction int

const (
	CloseHide CloseAction = iota // hide the window, the app stays
	CloseQuit                    // go through the quit flow
)

// Lifecycle owns the window-close and quit rules. The hooks are replaced by later tasks: TasksRunning by the
// AI-task engine (A2-05), QuitDecision by the quit dialog (A2-14). Until then no task runs, so a quit goes
// through at once, as before.
type Lifecycle struct {
	mu sync.Mutex
	// tasksRunning reports a live AI task or a program a closed session left running.
	tasksRunning func() bool
	// quitDecision shows the dialog and may take as long as the user needs. It runs on its own goroutine, never
	// on the thread that called ShouldQuit.
	quitDecision func(ctx context.Context) QuitAnswer
	loggingOut   func() bool
	onBackground func()
	quit         func() // asks the application to quit again (Shell sets it to app.Quit)

	sessionEnd atomic.Bool // set inside ShouldQuit when the platform says the session is ending

	approved atomic.Bool // one quit was approved; consumed by the next ShouldQuit
	deciding atomic.Bool // a decision is pending: further quit requests are refused until it ends
}

// NewLifecycle returns a Lifecycle with the default hooks (no tasks running, answer "quit").
func NewLifecycle() *Lifecycle {
	watchSessionEnd()
	return &Lifecycle{
		tasksRunning: func() bool { return false },
		quitDecision: func(context.Context) QuitAnswer { return QuitNow },
		loggingOut:   sessionEnding,
		quit:         func() {},
	}
}

// SetTasksRunning sets the hook that reports a live AI task. ShouldQuit calls it on the UI thread (AppKit's
// applicationShouldTerminate), so the hook must return at once from cached state and must never call into the
// UI thread (no InvokeSync, no blocking waits).
func (l *Lifecycle) SetTasksRunning(f func() bool) { l.mu.Lock(); l.tasksRunning = f; l.mu.Unlock() }

// SetQuitDecision sets the dialog hook (A2-14). It is called on a goroutine and may block.
func (l *Lifecycle) SetQuitDecision(f func(ctx context.Context) QuitAnswer) {
	l.mu.Lock()
	l.quitDecision = f
	l.mu.Unlock()
}

// SetLoggingOut replaces the session-end detector (tests; the default reads the platform).
func (l *Lifecycle) SetLoggingOut(f func() bool) { l.mu.Lock(); l.loggingOut = f; l.mu.Unlock() }

// LoggingOut reports whether the last ShouldQuit saw the system logging out, restarting or shutting down. The
// platform detector is read only inside ShouldQuit (on macOS it is valid only on the main thread during
// applicationShouldTerminate); this accessor is safe from any goroutine. A logout quits at once with no dialog,
// so a quit dialog never has to handle it.
func (l *Lifecycle) LoggingOut() bool { return l.sessionEnd.Load() }

// SetOnBackground registers what "stay in the background" does (the shell hides the window).
func (l *Lifecycle) SetOnBackground(f func()) { l.mu.Lock(); l.onBackground = f; l.mu.Unlock() }

// SetQuit gives the lifecycle the function that quits the application (a QuitNow answer calls it).
func (l *Lifecycle) SetQuit(f func()) { l.mu.Lock(); l.quit = f; l.mu.Unlock() }

// QuitApproved runs run (a quit or a restart) with the quit decision already made, so it does not ask a
// second time (the update dialog's "Reopen now"). The approval is one-shot: ShouldQuit consumes it, and it is
// cleared when run fails. If run returns nil without the app having asked, the approval stays until the next
// ShouldQuit, so run must be a call that quits.
func (l *Lifecycle) QuitApproved(run func() error) error {
	l.approved.Store(true)
	err := run()
	if err != nil {
		l.approved.Store(false)
	}
	return err
}

// CloseAction decides what closing the window does: it hides while tasks run or when the tray setting says to
// keep running, and otherwise goes through the quit flow.
func (l *Lifecycle) CloseAction(keepRunningSetting bool) CloseAction {
	l.mu.Lock()
	f := l.tasksRunning
	l.mu.Unlock()
	if keepRunningSetting || f() {
		return CloseHide
	}
	return CloseQuit
}

// ShouldQuit is the Wails ShouldQuit callback (Cmd+Q, the menu, the Dock, the tray, a session end). Wails
// calls it on the UI thread, so it NEVER blocks:
//   - it returns true at once when a quit was approved, when nothing is live, and always during a logout,
//     restart or shutdown (nothing blocks the shutdown);
//   - otherwise it starts the decision on a goroutine and returns false, which cancels this terminate. The
//     goroutine applies the answer: QuitNow approves once and quits again; QuitBackground hides the window;
//     QuitCancel does nothing.
func (l *Lifecycle) ShouldQuit() bool {
	if l.approved.CompareAndSwap(true, false) {
		return true
	}
	l.mu.Lock()
	running, decide := l.tasksRunning, l.quitDecision
	l.mu.Unlock()
	l.mu.Lock()
	ending := l.loggingOut
	l.mu.Unlock()
	if ending() {
		l.sessionEnd.Store(true)
		return true
	}
	if !running() {
		return true
	}
	if !l.deciding.CompareAndSwap(false, true) {
		return false // a dialog is already open for an earlier quit request
	}
	go func() {
		defer l.deciding.Store(false)
		switch decide(context.Background()) {
		case QuitNow:
			l.mu.Lock()
			quit := l.quit
			l.mu.Unlock()
			l.approved.Store(true)
			quit()
			l.approved.Store(false) // the terminate did not consume it: never leave an approval behind
		case QuitBackground:
			l.mu.Lock()
			f := l.onBackground
			l.mu.Unlock()
			if f != nil {
				f()
			}
		}
	}()
	return false
}

// Windows session-end messages (WM_QUERYENDSESSION, WM_ENDSESSION).
const (
	wmQueryEndSession = 0x0011
	wmEndSession      = 0x0016
)

var sessionEndFlag atomic.Bool

// WndProc is the Wails WindowsOptions.WndProcInterceptor: it records a logout, restart or shutdown and never
// consumes a message, except a cancelled session end: Wails would quit the app for every WM_ENDSESSION, also
// when wParam is FALSE (the shutdown was cancelled), so that one is consumed here.
func WndProc(_ uintptr, msg uint32, wParam, _ uintptr) (uintptr, bool) {
	switch msg {
	case wmQueryEndSession:
		sessionEndFlag.Store(true)
	case wmEndSession:
		if wParam == 0 {
			sessionEndFlag.Store(false)
			return 0, true
		}
		sessionEndFlag.Store(true)
	}
	return 0, false
}
