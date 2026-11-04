//go:build windows

package app

import (
	"os/exec"
	"syscall"
)

// setCmdLine replaces Go's argument quoting with the exact command line explorer expects.
func setCmdLine(cmd *exec.Cmd, line string) {
	if line != "" {
		cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: line}
	}
}
