//go:build windows

package browser

import (
	"errors"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32          = windows.NewLazySystemDLL("user32.dll")
	procIsIconic    = user32.NewProc("IsIconic")
	procShowWindow  = user32.NewProc("ShowWindow")
	procGetWindow   = user32.NewProc("GetWindow")
	procSetForeWin  = user32.NewProc("SetForegroundWindow")
	procBringToTop  = user32.NewProc("BringWindowToTop")
	swRestore       = uintptr(9)
	gwOwner         = uintptr(4)
	errNoForeground = errors.New("browser: Windows did not let the app bring the sign-in window forward")
)

// enumCB is created once: Go never frees a Windows callback and allows about 2000 of them. It reads the pid and
// collects the windows through package variables guarded by enumMu.
var (
	enumMu   sync.Mutex
	enumPID  int
	enumWins []windows.HWND
	enumCB   = syscall.NewCallback(func(h windows.HWND, _ uintptr) uintptr {
		var p uint32
		_, _ = windows.GetWindowThreadProcessId(h, &p)
		if int(p) == enumPID && windows.IsWindowVisible(h) {
			if owner, _, _ := procGetWindow.Call(uintptr(h), gwOwner); owner == 0 {
				enumWins = append(enumWins, h)
			}
		}
		return 1
	})
)

// foregroundPID restores the visible, unowned top-level windows of pid and asks Windows to make the first the
// foreground window. It asks for no permission; Windows may still refuse (the foreground lock), which is
// reported as an error.
func foregroundPID(pid int) error {
	enumMu.Lock()
	enumPID, enumWins = pid, nil
	err := windows.EnumWindows(enumCB, unsafe.Pointer(nil))
	wins := enumWins
	enumWins = nil
	enumMu.Unlock()
	if err != nil {
		return err
	}
	if len(wins) == 0 {
		return errors.New("browser: the sign-in browser has no window to bring forward")
	}
	h := wins[0]
	if min, _, _ := procIsIconic.Call(uintptr(h)); min != 0 {
		_, _, _ = procShowWindow.Call(uintptr(h), swRestore)
	}
	_, _, _ = procBringToTop.Call(uintptr(h))
	if ok, _, _ := procSetForeWin.Call(uintptr(h)); ok == 0 {
		return errNoForeground
	}
	return nil
}
