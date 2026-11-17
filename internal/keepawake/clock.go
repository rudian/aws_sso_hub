package keepawake

import "time"

// Clock is the manager's time. The real one strips the monotonic reading, which stands still while the Mac sleeps:
// after a wake every deadline is judged by the wall clock.
type Clock interface {
	Now() time.Time
	// AfterFunc calls f in its own goroutine after d. Stop prevents it if it has not started.
	AfterFunc(d time.Duration, f func()) Timer
}

// Timer is the part of time.Timer the package uses.
type Timer interface{ Stop() bool }

type realClock struct{}

func (realClock) Now() time.Time { return time.Now().Round(0) }
func (realClock) AfterFunc(d time.Duration, f func()) Timer {
	return time.AfterFunc(d, f)
}

// RealClock is the wall clock.
func RealClock() Clock { return realClock{} }
