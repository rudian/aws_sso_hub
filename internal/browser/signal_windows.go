//go:build windows

package browser

import (
	"os"
	"os/exec"
	"sync"

	"golang.org/x/sys/windows"

	"github.com/rudian/codepass/internal/execx"
)

// held keeps a handle to each browser this app started. While a handle is open, Windows does not give the
// process id to another process, so Close can never end an unrelated program that got a reused id.
var (
	heldMu sync.Mutex
	held   = map[int]windows.Handle{}
)

// signalPID ends a process. Windows has no polite signal for a GUI program; Close uses taskkill instead.
func signalPID(pid int, _ bool) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Kill()
}

// startProcess launches a program with the sanitised environment, keeps a handle to it and returns its id.
func startProcess(name string, args []string) (int, error) {
	cmd := exec.Command(name, args...)
	cmd.Env = execx.ChildEnv()
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	pid := cmd.Process.Pid
	// Opened before Wait can release the process, so the id still belongs to it.
	if h, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid)); err == nil {
		heldMu.Lock()
		held[pid] = h
		heldMu.Unlock()
	}
	go func() { _ = cmd.Wait() }()
	return pid, nil
}

// processRunning reports whether a process started by startProcess has not exited yet.
func processRunning(pid int) bool {
	heldMu.Lock()
	h, ok := held[pid]
	heldMu.Unlock()
	if !ok {
		return false
	}
	ev, err := windows.WaitForSingleObject(h, 0)
	return err == nil && ev == uint32(windows.WAIT_TIMEOUT)
}

// releaseProcess closes the kept handle; the id may be reused afterwards.
func releaseProcess(pid int) {
	heldMu.Lock()
	h, ok := held[pid]
	delete(held, pid)
	heldMu.Unlock()
	if ok {
		_ = windows.CloseHandle(h)
	}
}
