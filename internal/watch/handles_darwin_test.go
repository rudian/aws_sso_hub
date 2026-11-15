//go:build darwin && cgo

package watch

import "os"

// openHandles counts this process's open file descriptors.
func openHandles() (int, bool) {
	es, err := os.ReadDir("/dev/fd")
	if err != nil {
		return 0, false
	}
	return len(es), true
}
