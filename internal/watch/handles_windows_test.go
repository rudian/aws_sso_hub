//go:build windows

package watch

import (
	"errors"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// openHandles counts this process's open kernel handles.
func openHandles() (int, bool) {
	var n uint32
	r, _, _ := procGetProcessHandleCount.Call(uintptr(windows.CurrentProcess()), uintptr(unsafe.Pointer(&n)))
	if r == 0 {
		return 0, false
	}
	return int(n), true
}

var procGetProcessHandleCount = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetProcessHandleCount")

// assertNoHandleOn fails while any handle is open on dir: a directory opened
// with share mode 0 fails with ERROR_SHARING_VIOLATION if another handle exists.
// (FILE_SHARE_DELETE on the watcher's own handle would allow renaming the
// folder itself, so a rename is not a sufficient test.)
func assertNoHandleOn(t *testing.T, dir string) {
	t.Helper()
	p, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		t.Fatal(err)
	}
	h, err := windows.CreateFile(p, windows.GENERIC_READ, 0, nil, windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		if errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
			t.Fatalf("a handle is still open on %s", dir)
		}
		t.Fatal(err)
	}
	windows.CloseHandle(h)
}
