//go:build !windows

package toolinstall

import (
	"errors"
	"os"
	"os/exec"
)

func hideConsole(*exec.Cmd) {}

func asExit(err error, target **exec.ExitError) bool { return errors.As(err, target) }

// softStop asks a child to stop with SIGINT; WaitDelay is the hard stop.
func softStop(p *os.Process) error { return p.Signal(os.Interrupt) }
