//go:build !windows

package proctrack

import (
	"syscall"
)

// defaultSignal sends sig to pid; a negative pid is a whole process group.
func defaultSignal(pid int, sig syscall.Signal) error { return syscall.Kill(pid, sig) }

const defaultUseJob = false

const sigCont = syscall.SIGCONT

func ownPgrp() int { return syscall.Getpgrp() }
