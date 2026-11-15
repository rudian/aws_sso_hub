//go:build darwin

package appguard

import (
	"errors"
	"os/exec"
	"strings"
)

func trashDir() string { return homeTrash() }

const canRecycle = false

func recycleFile(string) error { return errors.New("no Recycle Bin on this platform") }

type sysPrompter struct{}

func newSysPrompter() Prompter { return sysPrompter{} }

// Fixed script: the texts arrive as arguments (item N of argv), never spliced into the script.
const script2 = `on run argv
	set r to display dialog (item 1 of argv) with title (item 2 of argv) buttons {(item 3 of argv), (item 4 of argv)} default button (item 4 of argv) cancel button (item 3 of argv) with icon caution
	return button returned of r
end run`

// ask shows a two-button dialog and reports whether the second (default) button was chosen.
func ask(msg, yes, no string) bool {
	out, err := exec.Command("/usr/bin/osascript", "-e", script2, msg, "Codepass", no, yes).Output() // argument list; the text is data
	return err == nil && strings.TrimSpace(string(out)) == yes
}

// AskQuit is design 5.13: "The old AWS SSO Hub is still running. Quit it first, then open Codepass."
func (sysPrompter) AskQuit(_ []Instance, lang string) bool {
	return ask(
		tr(lang, "旧版 AWS SSO Hub 还在运行。请先退出它，再打开 Codepass。", "The old AWS SSO Hub is still running. Quit it first, then open Codepass."),
		tr(lang, "退出旧版并继续", "Quit the old app and continue"),
		tr(lang, "取消", "Cancel"))
}

// AskTrash names the exact bundle path(s) it will move.
func (sysPrompter) AskTrash(bundles []string, lang string) bool {
	msg := tr(lang, "把旧版移到废纸篓？\n\n", "Move the old app to the Trash?\n\n") + strings.Join(bundles, "\n")
	return ask(msg, tr(lang, "把旧版移到废纸篓", "Move the old app to the Trash"), tr(lang, "保留", "Keep it"))
}

func (sysPrompter) Tell(msg string) {
	exec.Command("/usr/bin/osascript", "-e", `on run argv
	display dialog (item 1 of argv) with title "Codepass" buttons {"OK"} default button "OK" with icon caution
end run`, msg).Run()
}
