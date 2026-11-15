package update

import (
	"os"
	"path/filepath"
	"strings"
)

// installTarget returns what the helper replaces: the enclosing .app bundle on macOS, otherwise the executable.
// It matches Wails' own rule, so the folder checked here is the folder Wails stages in.
func installTarget(exe, goos string) string {
	if goos != "darwin" {
		return exe
	}
	parts := strings.Split(filepath.Clean(exe), string(os.PathSeparator))
	for i, p := range parts {
		if strings.HasSuffix(p, ".app") {
			return string(os.PathSeparator) + filepath.Join(parts[1:i+1]...)
		}
	}
	return exe
}

// Preflight runs before any byte is downloaded. It returns a *PreflightError when this copy of the app cannot
// replace itself: macOS "App Translocation" (a quarantined app opened from Downloads runs from a read-only copy
// under a random path), or a folder the current user cannot write to (Wails stages next to the app and the swap is a
// rename). The error names the folder so the message can say where to fix it.
func Preflight(exe, goos string) error {
	target := installTarget(exe, goos)
	if strings.Contains(target, "/AppTranslocation/") {
		return &PreflightError{Kind: BlockTranslocated, Folder: filepath.Dir(target),
			Detail: "the app is running from a temporary read-only copy; move it to Applications and open it from there"}
	}
	dir := filepath.Dir(target)
	probe, err := os.MkdirTemp(dir, ".codepass-writecheck-")
	if err != nil {
		return &PreflightError{Kind: BlockNotWritable, Folder: dir, Detail: err.Error()}
	}
	_ = os.Remove(probe)
	// The swap also deletes the old bundle (RemoveAll before the rename), which needs write access to every folder
	// inside it, not only to the folder that holds it. A bundle another account installed passes the probe above.
	if goos == "darwin" {
		if bad := firstUnwritableDir(target); bad != "" {
			return &PreflightError{Kind: BlockNotWritable, Folder: target, Detail: bad + " is not writable by this user"}
		}
	}
	return nil
}
