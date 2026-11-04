//go:build !windows

package app

import "os/exec"

func setCmdLine(cmd *exec.Cmd, line string) {}
