//go:build !windows

package execx

import (
	"os"
	"os/exec"
	"runtime"
	"syscall"
)

func hideWindow(cmd *exec.Cmd) {}

func showNewConsole(cmd *exec.Cmd) {}

// ownedByUserOrRoot reports whether the file belongs to the current user or to root.
func ownedByUserOrRoot(st os.FileInfo) bool {
	sys, ok := st.Sys().(*syscall.Stat_t)
	if !ok {
		return false
	}
	return int(sys.Uid) == os.Getuid() || sys.Uid == 0
}

// groupTrusted reports whether members of the group are treated like the owner of a folder: root's group
// (wheel) everywhere, and the admin group (gid 80) on macOS, whose members can already become root with sudo.
func groupTrusted(gid uint32, goos string) bool {
	return gid == 0 || (goos == "darwin" && gid == 80)
}

// folderTrusted reports whether a folder holding a program can only be changed by the current user, root or
// the administrators. World-writable folders never qualify; group-writable ones qualify when the folder
// belongs to the current user or root and the group is a trusted one (see groupTrusted). The standard
// Homebrew prefix (/opt/homebrew/bin, owned by the user and group admin, mode 0775) is such a folder.
func folderTrusted(st os.FileInfo) bool {
	perm := st.Mode().Perm()
	if perm&0o002 != 0 {
		return false
	}
	if perm&0o020 == 0 {
		return true
	}
	sys, ok := st.Sys().(*syscall.Stat_t)
	if !ok {
		return false
	}
	return ownedByUserOrRoot(st) && groupTrusted(sys.Gid, runtime.GOOS)
}
