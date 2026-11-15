//go:build windows

package appguard

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Windows has no Trash folder to rename into. 0.0.1 shipped as a portable zip (AWS SSO Hub.exe, no installer,
// no uninstall entry), so the whole old app is that one exe: it goes to the Recycle Bin through the shell.
func trashDir() string { return "" }

const canRecycle = true

var shFileOperation = syscall.NewLazyDLL("shell32.dll").NewProc("SHFileOperationW")

// shFileOpStruct is SHFILEOPSTRUCTW (packed on 32-bit, natural alignment on 64-bit, as Go lays it out).
type shFileOpStruct struct {
	Hwnd                 uintptr
	Func                 uint32
	From                 *uint16
	To                   *uint16
	Flags                uint16
	AnyOperationsAborted int32
	NameMappings         uintptr
	ProgressTitle        *uint16
}

// recycleFile moves exactly one file to the Recycle Bin (FOF_ALLOWUNDO), without confirmation or progress UI.
func recycleFile(path string) error {
	const foDelete, fofSilent, fofNoConfirmation, fofAllowUndo, fofNoErrorUI = 0x3, 0x4, 0x10, 0x40, 0x400
	if strings.ContainsAny(path, "*?\x00") {
		return syscall.EINVAL // SHFileOperation expands wildcards: only a literal path is acceptable
	}
	from, err := syscall.UTF16FromString(path)
	if err != nil {
		return err
	}
	from = append(from, 0) // the list ends with a double NUL
	op := shFileOpStruct{Func: foDelete, From: &from[0], Flags: fofSilent | fofNoConfirmation | fofAllowUndo | fofNoErrorUI}
	r, _, _ := shFileOperation.Call(uintptr(unsafe.Pointer(&op)))
	if r != 0 {
		return syscall.Errno(r)
	}
	if op.AnyOperationsAborted != 0 {
		return syscall.ERROR_OPERATION_ABORTED
	}
	return nil
}

// createNoWindow keeps the GUI exe from flashing a console for each helper tool (as internal/execx does).
const createNoWindow = 0x08000000

func hideConsole(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
}

// system32 returns the absolute path of a tool in %SystemRoot%\System32: never resolved through PATH.
func system32(tool string) string {
	root := os.Getenv("SystemRoot")
	if root == "" {
		root = `C:\Windows`
	}
	return filepath.Join(root, "System32", tool)
}

type sysLister struct{}

// List lists the current user's processes with their full executable path. A process that cannot be opened
// (another user's, or protected) is skipped: it is never the user's own old app.
func (sysLister) List() ([]Proc, error) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(snap)
	me := currentUserSID()
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	var procs []Proc
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		if p, ok := imagePath(e.ProcessID, me); ok {
			procs = append(procs, Proc{PID: int(e.ProcessID), Path: p})
		}
	}
	return procs, nil
}

func currentUserSID() *windows.SID {
	u, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil
	}
	return u.User.Sid
}

// imagePath returns the full executable path of pid when it belongs to the user with SID me.
func imagePath(pid uint32, me *windows.SID) (string, bool) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return "", false
	}
	defer windows.CloseHandle(h)
	if me != nil {
		var tok windows.Token
		if windows.OpenProcessToken(h, windows.TOKEN_QUERY, &tok) != nil {
			return "", false
		}
		defer tok.Close()
		u, err := tok.GetTokenUser()
		if err != nil || !windows.EqualSid(u.User.Sid, me) {
			return "", false
		}
	}
	buf := make([]uint16, 32768)
	n := uint32(len(buf))
	if windows.QueryFullProcessImageName(h, 0, &buf[0], &n) != nil {
		return "", false
	}
	return windows.UTF16ToString(buf[:n]), true
}

type sysTerm struct{}

// Quit asks the process to close (no /F, so never a kill). It posts a close request to the app's windows.
func (sysTerm) Quit(pid int) error {
	cmd := exec.Command(system32("taskkill.exe"), "/PID", strconv.Itoa(pid))
	hideConsole(cmd)
	return cmd.Run()
}

func (sysTerm) Alive(pid int) bool {
	cmd := exec.Command(system32("tasklist.exe"), "/FI", "PID eq "+strconv.Itoa(pid), "/FO", "CSV", "/NH")
	hideConsole(cmd)
	out, err := cmd.Output()
	return err == nil && strings.Contains(string(out), `"`+strconv.Itoa(pid)+`"`)
}

type sysPrompter struct{}

func newSysPrompter() Prompter { return sysPrompter{} }

var messageBox = syscall.NewLazyDLL("user32.dll").NewProc("MessageBoxW")

func box(text string, flags uintptr) int {
	t, _ := syscall.UTF16PtrFromString(text)
	c, _ := syscall.UTF16PtrFromString("Codepass")
	r, _, _ := messageBox.Call(0, uintptr(unsafe.Pointer(t)), uintptr(unsafe.Pointer(c)), flags)
	return int(r)
}

// Message boxes cannot rename their buttons, so the text says what OK does (design 5.13 copy).
func (sysPrompter) AskQuit(_ []Instance, lang string) bool {
	const mbOKCancel, mbIconWarning, idOK = 0x1, 0x30, 1
	return box(tr(lang,
		"旧版 AWS SSO Hub 还在运行。请先退出它，再打开 Codepass。\n\n点「确定」：退出旧版并继续。",
		"The old AWS SSO Hub is still running. Quit it first, then open Codepass.\n\nClick OK to quit the old app and continue."),
		mbOKCancel|mbIconWarning) == idOK
}

// AskTrash is a Yes/No box naming the exact exe that will go to the Recycle Bin.
func (sysPrompter) AskTrash(targets []string, lang string) bool {
	const mbYesNo, mbIconQuestion, idYes = 0x4, 0x20, 6
	return box(tr(lang, "把旧版移到回收站？\n\n", "Move the old app to the Recycle Bin?\n\n")+strings.Join(targets, "\n")+"\n\n"+
		tr(lang, "「是」：移到回收站。「否」：保留。", "Yes moves it to the Recycle Bin. No keeps it."), mbYesNo|mbIconQuestion) == idYes
}

func (sysPrompter) Tell(msg string) { box(msg, 0x30) }
