//go:build windows

package keepawake

import (
	"syscall"
)

var procSetThreadExecutionState = syscall.NewLazyDLL("kernel32.dll").NewProc("SetThreadExecutionState")

// NewSetStateFunc is the real SetThreadExecutionState (kernel32). It returns an error when the call returns 0.
func NewSetStateFunc() SetStateFunc {
	return func(flags uint32) error {
		r, _, e := procSetThreadExecutionState.Call(uintptr(flags))
		if r == 0 {
			if e != nil && e != syscall.Errno(0) {
				return e
			}
			return syscall.EINVAL
		}
		return nil
	}
}
