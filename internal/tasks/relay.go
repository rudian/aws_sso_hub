package tasks

import (
	"sync/atomic"

	"github.com/rudian/codepass/internal/taskstate"
)

// Relay breaks the construction cycle between the tracker and the service: the tracker takes its onChange
// function when it is built, and the service needs the tracker. Build the tracker with relay.OnChange, build
// the service with Deps.Relay set to the same relay, and the service connects itself.
type Relay struct {
	f atomic.Pointer[func(taskstate.Info)]
}

// OnChange is the tracker's onChange. Changes that arrive before Connect are dropped (no task exists yet).
func (r *Relay) OnChange(i taskstate.Info) {
	if f := r.f.Load(); f != nil {
		(*f)(i)
	}
}

// Connect sets the receiver.
func (r *Relay) Connect(f func(taskstate.Info)) { r.f.Store(&f) }
