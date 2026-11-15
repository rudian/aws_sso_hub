//go:build !windows

package envcheck

import "syscall"

// freeBytes returns the space a normal user can still use on the volume that holds dir.
func freeBytes(dir string) (int64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err != nil {
		return -1, err
	}
	return int64(st.Bavail) * int64(st.Bsize), nil
}
