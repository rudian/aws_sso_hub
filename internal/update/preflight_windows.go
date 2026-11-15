//go:build windows

package update

// firstUnwritableDir is only used for macOS bundles; Windows replaces a single file.
func firstUnwritableDir(root string) string { return "" }
