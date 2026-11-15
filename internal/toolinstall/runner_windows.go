//go:build windows

package toolinstall

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

func hideConsole(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
}

func asExit(err error, target **exec.ExitError) bool { return errors.As(err, target) }

// softStop: Windows has no way to send an interrupt to a console-less child; WaitDelay does not apply to a
// killed process, so the child is ended at once.
func softStop(p *os.Process) error { return p.Kill() }
