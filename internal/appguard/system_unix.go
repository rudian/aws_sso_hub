//go:build !windows

package appguard

import (
	"os"
	"os/exec"
	"syscall"
)

type sysLister struct{}

func (sysLister) List() ([]Proc, error) {
	// Only this user's processes: -x lists the user's own, parsePS keeps rows whose uid is ours.
	out, err := runOut("/bin/ps", "-x", "-o", "pid=,uid=,comm=")
	if err != nil {
		return nil, err
	}
	return parsePS(out, os.Getuid()), nil
}

type sysTerm struct{}

// Quit sends SIGTERM, the polite request. It never sends SIGKILL.
func (sysTerm) Quit(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Signal(syscall.SIGTERM)
}

func (sysTerm) Alive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return p.Signal(syscall.Signal(0)) == nil
}

func hideConsole(*exec.Cmd) {}
