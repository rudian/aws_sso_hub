package keepawake

// Holder is one kind of assertion the operating system understands. The manager owns a list of them and calls
// Acquire when the first activity starts and Release when the last one ends.
type Holder interface {
	// Name is for the log and for error text ("caffeinate", "activity", "execstate").
	Name() string
	// Acquire takes the assertion. It is called again with another screen value while held (the setting changed):
	// the holder then changes what it holds. Calling it again with the same value changes nothing.
	Acquire(screen bool) error
	// Release gives the assertion back. It never fails the caller and is safe when nothing is held.
	Release()
}

// Optional parts of a Holder. The Manager uses them when a holder has them, so the state never claims more than the
// operating system holds.
type (
	// holdingReporter: Holding tells whether the assertion is really held now (a helper process may have died).
	holdingReporter interface{ Holding() bool }
	// screenReporter: HoldsScreen tells whether the display is really held awake now.
	screenReporter interface{ HoldsScreen() bool }
	// taskScoped marks a holder that is wanted whenever a task counts, even with the setting off, and that is not
	// a sleep assertion (the App Nap activity): it is left out of State.Held and State.Screen.
	taskScoped interface{ TaskScoped() bool }
	// changeNotifier: the holder calls f (in its own goroutine, never under a lock of the Manager) when what it
	// holds changed by itself; the Manager then evaluates again.
	changeNotifier interface{ SetNotify(f func()) }
)

// Leftovers tells how many programs a closed session left running (A2-13's process tracker implements it; A2-W
// connects the two). Count must be cheap and must not call into the Manager: the manager calls it from its own
// goroutines, outside its locks.
type Leftovers interface {
	Count(session string) int
}

// TaskState is what a task reports.
type TaskState int

const (
	// StateIdle: the session is alive with no AI work and nobody waiting on it, for example before the first event.
	// Does not count. It is never used for a finished turn: a finished turn is StateWaiting, which keeps the hold
	// for the grace time (plan 4.10).
	//
	// The mapping of the task states (plan 4.8) for whoever feeds the Manager:
	//   running, running (no signal)                                      -> StateRunning
	//   waiting (Notification, PermissionRequest, Stop, AskUserQuestion,
	//            limit reached)                                           -> StateWaiting
	//   done, failed, stopped, terminal closed                            -> StateEnded
	//   plain shells                                                      -> KindShell, or not reported
	StateIdle TaskState = iota
	// StateRunning: the AI is working. Counts.
	StateRunning
	// StateWaiting: the AI waits for the owner (or its turn is over). Counts for the grace time.
	StateWaiting
	// StateEnded: the tool exited or the terminal was closed. Counts only while Leftovers.Count(session) > 0.
	// Report it after the process tracker's snapshot that follows the exit: a record whose count is 0 is
	// forgotten at once and Refresh does not bring it back.
	StateEnded
)

// Kind separates AI sessions from plain shells. A plain shell never counts.
type Kind int

const (
	KindAI Kind = iota
	KindShell
)
