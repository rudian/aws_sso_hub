//go:build windows

package proctrack

import (
	"errors"
	"syscall"
)

// Windows has no signals: the session's job is terminated (Group.KillSession) instead.
func defaultSignal(int, syscall.Signal) error { return errors.ErrUnsupported }

const defaultUseJob = true

const sigCont = syscall.Signal(0x12)

func ownPgrp() int { return 0 }
