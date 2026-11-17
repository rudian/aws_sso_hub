//go:build windows

package aitools

import (
	"os/exec"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// prepare runs the program without a console window and inside a Job Object with KILL_ON_JOB_CLOSE (as
// internal/pty does for a terminal): when the context ends, or the call returns, everything the program started
// ends with it. That matters for OpenCode's npm shim, whose node.exe and opencode.exe would otherwise outlive the
// direct child and keep the naming folder from being removed. The program is assigned right after it starts, so
// a grandchild it starts in the first instants could escape; the tools do not do that.
func prepare(cmd *exec.Cmd) (afterStart func(), release func()) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
	var mu sync.Mutex
	var job windows.Handle
	cmd.Cancel = func() error {
		mu.Lock()
		j := job
		mu.Unlock()
		if j != 0 {
			if windows.TerminateJobObject(j, 1) == nil {
				return nil
			}
		}
		if cmd.Process == nil {
			return nil
		}
		return cmd.Process.Kill()
	}
	afterStart = func() {
		if cmd.Process == nil {
			return
		}
		j, err := windows.CreateJobObject(nil, nil)
		if err != nil {
			return
		}
		var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
		info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
		if _, err := windows.SetInformationJobObject(j, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
			windows.CloseHandle(j)
			return
		}
		h, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
		if err != nil {
			windows.CloseHandle(j)
			return
		}
		defer windows.CloseHandle(h)
		if windows.AssignProcessToJobObject(j, h) != nil {
			windows.CloseHandle(j)
			return
		}
		mu.Lock()
		job = j
		mu.Unlock()
	}
	release = func() {
		mu.Lock()
		j := job
		job = 0
		mu.Unlock()
		if j != 0 {
			windows.CloseHandle(j) // KILL_ON_JOB_CLOSE ends what is left
		}
	}
	return afterStart, release
}
