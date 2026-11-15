//go:build windows

package envcheck

import "golang.org/x/sys/windows"

// freeBytes returns the space the current user can still use on the volume that holds dir.
func freeBytes(dir string) (int64, error) {
	p, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return -1, err
	}
	var avail, total, free uint64
	if err := windows.GetDiskFreeSpaceEx(p, &avail, &total, &free); err != nil {
		return -1, err
	}
	return int64(avail), nil
}
