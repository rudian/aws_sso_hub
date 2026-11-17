package keepawake

import (
	"fmt"
	"sync"
)

// ActivityAPI is the macOS NSProcessInfo activity (beginActivityWithOptions / endActivity). The real one lives in
// activity_darwin.go (cgo); tests use a fake.
type ActivityAPI interface {
	// Begin starts an activity with the given reason and returns a handle for End.
	Begin(reason string) (handle uint64, err error)
	End(handle uint64)
}

// ActivityReason is the text the system shows in `pmset -g assertions` and Activity Monitor.
const ActivityReason = "Codepass: AI task running"

// Activity holds an NSProcessInfo activity so App Nap does not throttle a hidden window while a task counts
// (plan 4.10, R6). It is not a sleep assertion (it allows idle system sleep: caffeinate holds that), so the Manager
// wants it whenever a task counts, whatever the keep-awake setting says, and leaves it out of State.Held. The
// screen option makes no difference to it.
type Activity struct {
	API ActivityAPI

	mu     sync.Mutex
	handle uint64
	held   bool
}

func (a *Activity) Name() string { return "activity" }

// TaskScoped: see taskScoped.
func (a *Activity) TaskScoped() bool { return true }

func (a *Activity) Acquire(bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.held {
		return nil
	}
	h, err := a.API.Begin(ActivityReason)
	if err != nil {
		return fmt.Errorf("starting the activity: %w", err)
	}
	a.handle, a.held = h, true
	return nil
}

func (a *Activity) Release() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.held {
		return
	}
	a.API.End(a.handle)
	a.held = false
}
