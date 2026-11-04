//go:build windows

package execx

import (
	"os"
	"os/exec"
	"syscall"
)

func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
}

// showNewConsole gives the child its own visible console window (CREATE_NEW_CONSOLE).
func showNewConsole(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000010}
}

// ownedByUserOrRoot is not meaningful on Windows, where access is governed by ACLs.
func ownedByUserOrRoot(st os.FileInfo) bool { return true }

// folderTrusted is not meaningful on Windows, where access is governed by ACLs.
func folderTrusted(st os.FileInfo) bool { return true }
