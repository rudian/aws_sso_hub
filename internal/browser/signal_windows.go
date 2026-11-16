//go:build windows

package browser

import (
	"os"
	"os/exec"

	"github.com/rudian/aws_sso_hub/internal/execx"
)

// signalPID ends a process. Windows has no polite signal for a GUI program; Close uses taskkill instead.
func signalPID(pid int, _ bool) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Kill()
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
