//go:build !windows

package browser

import (
	"os/exec"
	"syscall"

	"github.com/rudian/codepass/internal/execx"
)

// signalPID asks a process to quit (SIGTERM) or forces it (SIGKILL).
func signalPID(pid int, force bool) error {
	if force {
		return syscall.Kill(pid, syscall.SIGKILL)
	}
	return syscall.Kill(pid, syscall.SIGTERM)
}

// startProcess launches a program with the sanitised environment and returns its process id.
func startProcess(name string, args []string) (int, error) {
	cmd := exec.Command(name, args...)
	cmd.Env = execx.ChildEnv()
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	pid := cmd.Process.Pid
	go func() { _ = cmd.Wait() }()
	return pid, nil
}

// processRunning and releaseProcess keep track of started processes on Windows only.
func processRunning(int) bool { return false }

func releaseProcess(int) {}
