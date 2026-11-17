//go:build !windows

package aitools

import (
	"os/exec"
	"syscall"
)

// prepare puts the program in a session of its own and, when the context ends, kills the whole group:
// the login shell, the tool it started and anything that tool started. Only processes this call started.
//
// A session, not just a process group: a new session has no controlling terminal, so an interactive zsh never
// turns on job control. With Setpgid alone and the app started from a terminal, zsh -i tries to take that
// terminal from a background group and is stopped (state T) until the budget kills it. The session leader's
// process group ID is its PID, so killing -pid still reaches the whole tree.
func prepare(cmd *exec.Cmd) (afterStart func(), release func()) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
			return cmd.Process.Kill()
		}
		return nil
	}
	return func() {}, func() {}
}
