package keepawake

import (
	"errors"
	"fmt"
	"runtime"
	"sync"
)

// Flags of SetThreadExecutionState (documented values).
const (
	esSystemRequired  uint32 = 0x00000001
	esDisplayRequired uint32 = 0x00000002
	esContinuous      uint32 = 0x80000000
)

// SetStateFunc is SetThreadExecutionState. It returns an error when the call fails.
type SetStateFunc func(flags uint32) error

// ExecState holds the Windows assertion: SetThreadExecutionState(ES_CONTINUOUS | ES_SYSTEM_REQUIRED
// [| ES_DISPLAY_REQUIRED]). The setting belongs to the calling thread and ends with it, so every call runs on one
// dedicated goroutine locked to one OS thread; ending that goroutine (Close) also clears the setting if Release was
// somehow missed.
type ExecState struct {
	Set SetStateFunc

	mu     sync.Mutex
	reqs   chan execReq
	closed bool
	held   bool
	screen bool
}

type execReq struct {
	flags uint32
	reply chan error
}

func (e *ExecState) Name() string { return "execstate" }

func (e *ExecState) start() {
	e.reqs = make(chan execReq)
	reqs := e.reqs
	go func() {
		runtime.LockOSThread() // never unlocked: when the goroutine returns, the thread ends
		for r := range reqs {
			r.reply <- e.Set(r.flags)
		}
	}()
}

func (e *ExecState) call(flags uint32) error {
	if e.reqs == nil {
		e.start()
	}
	reply := make(chan error, 1)
	e.reqs <- execReq{flags, reply}
	return <-reply
}

func (e *ExecState) Acquire(screen bool) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return errors.New("execstate: closed")
	}
	if e.held && e.screen == screen {
		return nil
	}
	f := esContinuous | esSystemRequired
	if screen {
		f |= esDisplayRequired
	}
	if err := e.call(f); err != nil {
		return fmt.Errorf("SetThreadExecutionState: %w", err)
	}
	e.held, e.screen = true, screen
	return nil
}

func (e *ExecState) Release() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.held {
		return
	}
	_ = e.call(esContinuous) // clears the requirements
	e.held = false
}

// Holding tells whether the execution state is set now.
func (e *ExecState) Holding() bool { e.mu.Lock(); defer e.mu.Unlock(); return e.held }

// HoldsScreen tells whether ES_DISPLAY_REQUIRED is set now.
func (e *ExecState) HoldsScreen() bool { e.mu.Lock(); defer e.mu.Unlock(); return e.held && e.screen }

// Close releases and ends the locked thread. The Manager does not call it (Manager.Close only releases, ES_CONTINUOUS
// is cleared); it is for tests and for an owner that wants the thread gone before the process ends.
func (e *ExecState) Close() {
	e.Release()
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.reqs != nil && !e.closed {
		close(e.reqs)
	}
	e.closed = true
}
