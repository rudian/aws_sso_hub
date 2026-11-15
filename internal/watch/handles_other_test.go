//go:build !windows && !(darwin && cgo)

package watch

func openHandles() (int, bool) { return 0, false }
