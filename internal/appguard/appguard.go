// Package appguard handles the old AWS SSO Hub app (same bundle ID as Codepass, so the single-instance check
// would hand the launch to it and exit silently). Before Wails builds the single-instance manager, Run finds a
// running old app and asks the user to quit it or to move it to the Trash. Every system touch (process list,
// signals, dialogs, file moves, clock) sits behind an interface so tests use fakes only.
package appguard

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// Proc is one running process of the current user: its id and the full path of its executable.
type Proc struct {
	PID  int
	Path string
}

// ProcessLister lists running processes.
type ProcessLister interface{ List() ([]Proc, error) }

// Terminator asks a process to exit (never kills) and reports whether it still runs.
type Terminator interface {
	Quit(pid int) error
	Alive(pid int) bool
}

// FileOps is the only file access the guard has. On macOS an old app is a ".app" bundle that is renamed into the
// Trash folder; on Windows it is one .exe (0.0.1 was a portable zip) that goes to the Recycle Bin.
type FileOps interface {
	Exists(path string) bool
	Rename(oldPath, newPath string) error
	// TrashDir is the folder "move to the Trash" puts things in ("" when the platform has none).
	TrashDir() string
	// CanRecycle reports whether Recycle works on this platform (Windows: the Recycle Bin, one exe at a time).
	CanRecycle() bool
	// Recycle moves exactly one file to the Recycle Bin (undoable, no confirmation).
	Recycle(path string) error
}

// Clock lets tests run the wait without sleeping.
type Clock interface {
	Now() time.Time
	Sleep(d time.Duration)
}

// Choice is the user's answer to the prompt.
type Choice int

const (
	ChoiceCancel Choice = iota
	ChoiceQuit
	ChoiceTrash
)

// Prompter asks the user (design 5.13, "Old app still running"; plan 4.2). It shows the texts for Deps.Lang.
type Prompter interface {
	// AskQuit shows "The old AWS SSO Hub is still running. Quit it first, then open Codepass." with the buttons
	// "Quit the old app and continue" and Cancel. It reports whether the user chose to quit it.
	AskQuit(found []Instance, lang string) bool
	// AskTrash is the second prompt, after the old app quit: "Move the old app to the Trash" (macOS: the bundle
	// folders; Windows: "Recycle Bin", the exact .exe files) naming the exact paths, with "Keep it". It reports
	// whether the user chose to move them.
	AskTrash(bundles []string, lang string) bool
	// Tell shows a message (a failure the user must read).
	Tell(msg string)
}

// Instance is one running old app.
type Instance struct {
	PID    int
	Path   string // executable
	Bundle string // the ".app" folder containing it ("" when there is none)
}

// Outcome says what main does next.
type Outcome int

const (
	// Proceed: no old app is running (or it was quit): start normally.
	Proceed Outcome = iota
	// Exit: the user cancelled or the old app could not be stopped: leave without starting.
	Exit
)

// Deps are the seams Run uses.
type Deps struct {
	Lister   ProcessLister
	Term     Terminator
	Files    FileOps
	Clock    Clock
	Prompter Prompter
	SelfPID  int
	// SelfPath is this executable; a process with the same path is never an old app.
	SelfPath string
	// Lang is the saved UI language ("zh-CN" or "en"); the prompts and messages use it.
	Lang string
	// QuitTimeout is how long to wait for the old app to exit (default 10 s).
	QuitTimeout time.Duration
}

const oldBundleName = "AWS SSO Hub.app"

// isOldApp matches the executable of the old product: its file name is "AWS SSO Hub", "aws_sso_hub" or
// "aws-sso-hub" (with or without .exe), compared without case, spaces, dashes and underscores.
func isOldApp(path string) bool {
	base := strings.ToLower(path[strings.LastIndexAny(path, `/\`)+1:])
	base = strings.TrimSuffix(base, ".exe")
	base = strings.NewReplacer(" ", "", "-", "", "_", "").Replace(base)
	return base == "awsssohub"
}

// bundleOf returns the ".app" folder around an executable path, or "".
func bundleOf(path string) string {
	i := strings.Index(path, ".app/")
	if i < 0 {
		return ""
	}
	return path[:i+len(".app")]
}

// Detect returns the running old apps, never this process.
func Detect(l ProcessLister, selfPID int, selfPath string) ([]Instance, error) {
	procs, err := l.List()
	if err != nil {
		return nil, err
	}
	var out []Instance
	for _, p := range procs {
		if p.PID == selfPID || p.PID <= 0 || (selfPath != "" && p.Path == selfPath) || !isOldApp(p.Path) {
			continue
		}
		out = append(out, Instance{PID: p.PID, Path: p.Path, Bundle: bundleOf(p.Path)})
	}
	return out, nil
}

func tr(lang, zh, en string) string {
	if lang == "zh-CN" {
		return zh
	}
	return en
}

// Run implements the handoff. A failure to list processes never blocks the start.
func Run(d Deps) Outcome {
	if d.QuitTimeout <= 0 {
		d.QuitTimeout = 10 * time.Second
	}
	found, err := Detect(d.Lister, d.SelfPID, d.SelfPath)
	if err != nil || len(found) == 0 {
		return Proceed
	}
	if !d.Prompter.AskQuit(found, d.Lang) {
		return Exit
	}
	// The prompt may have stayed open for a long time: look again and signal every old-app process that runs now
	// (Detect already filters by the old executable name and excludes this process, so a PID reused by an
	// unrelated program cannot match; an old app started again during the prompt is quit too).
	running := found
	if again, err := Detect(d.Lister, d.SelfPID, d.SelfPath); err == nil {
		running = again
	}
	if err := quitAll(d, running); err != nil {
		d.Prompter.Tell(err.Error())
		return Exit
	}
	// Quitting can race with a relaunch: if any old app still runs, never move it and never start.
	left, err := Detect(d.Lister, d.SelfPID, d.SelfPath)
	if err == nil && len(left) > 0 {
		d.Prompter.Tell(stillRunningMsg(d.Lang))
		return Exit
	}
	offerTrash(d, append(found, running...))
	return Proceed
}

func stillRunningMsg(lang string) string {
	return tr(lang, "AWS SSO Hub 还在运行，请先手动退出它，再打开 Codepass。", "AWS SSO Hub is still running. Quit it yourself and open Codepass again.")
}

// offerTrash is the second prompt, after the old app has quit. Nothing here can stop Codepass from starting:
// a failure is told to the user with the exact path.
func offerTrash(d Deps, found []Instance) {
	recycle := d.Files.CanRecycle()
	if !recycle && d.Files.TrashDir() == "" {
		return
	}
	var targets []string
	seen := map[string]bool{}
	for _, in := range found {
		t := in.Bundle
		if recycle {
			t = in.Path
		}
		if t == "" || !validTarget(t, recycle) {
			return // an unknown shape: move nothing at all
		}
		if !seen[t] {
			seen[t] = true
			targets = append(targets, t)
		}
	}
	if len(targets) == 0 || !d.Prompter.AskTrash(targets, d.Lang) {
		return
	}
	for _, msg := range trashAll(d, targets) {
		d.Prompter.Tell(msg)
	}
}

// baseName is the last element of a path with either separator (a Windows path may be tested on any OS).
func baseName(p string) string { return p[strings.LastIndexAny(p, `/\`)+1:] }

// validTarget: a bundle named exactly "AWS SSO Hub.app", or (Recycle Bin) an .exe of the old product.
func validTarget(t string, recycle bool) bool {
	if recycle {
		return strings.HasSuffix(strings.ToLower(t), ".exe") && isOldApp(t)
	}
	return baseName(t) == oldBundleName
}

func quitAll(d Deps, found []Instance) error {
	for _, in := range found {
		if err := d.Term.Quit(in.PID); err != nil {
			return fmt.Errorf("%s: %w", tr(d.Lang, "无法让 AWS SSO Hub 退出", "Could not ask AWS SSO Hub to quit"), err)
		}
	}
	deadline := d.Clock.Now().Add(d.QuitTimeout)
	for {
		alive := false
		for _, in := range found {
			if d.Term.Alive(in.PID) {
				alive = true
			}
		}
		if !alive {
			return nil
		}
		if !d.Clock.Now().Before(deadline) {
			return errors.New(stillRunningMsg(d.Lang))
		}
		d.Clock.Sleep(200 * time.Millisecond)
	}
}

// trashAll moves each old app into the Trash (macOS: rename under a free name) or the Recycle Bin (Windows: the
// exe only) and returns one message per item it could not move. Only a folder named exactly "AWS SSO Hub.app" or
// an old-product .exe is ever moved, and only after its process has exited.
func trashAll(d Deps, targets []string) []string {
	var msgs []string
	recycle := d.Files.CanRecycle()
	for _, b := range targets {
		if !validTarget(b, recycle) {
			msgs = append(msgs, fmt.Sprintf(tr(d.Lang, "没有移动 %s：它不是旧版 AWS SSO Hub。", "Did not move %s: it is not the old AWS SSO Hub."), b))
			continue
		}
		if !d.Files.Exists(b) {
			msgs = append(msgs, fmt.Sprintf(tr(d.Lang, "找不到旧版 %s，它可能已经不在了，没有移动。", "The old app %s could not be found, so nothing was moved."), b))
			continue
		}
		if recycle {
			if err := d.Files.Recycle(b); err != nil {
				msgs = append(msgs, fmt.Sprintf(tr(d.Lang, "无法把 %s 移到回收站：%v。请自己删除它。", "Could not move %s to the Recycle Bin: %v. Delete it yourself."), b, err))
			}
			continue
		}
		dst := filepath.Join(d.Files.TrashDir(), oldBundleName)
		for n := 2; d.Files.Exists(dst); n++ {
			dst = filepath.Join(d.Files.TrashDir(), fmt.Sprintf("AWS SSO Hub %d.app", n))
		}
		if err := d.Files.Rename(b, dst); err != nil {
			msgs = append(msgs, fmt.Sprintf(tr(d.Lang, "无法把 %s 移到废纸篓：%v。请自己移动它。", "Could not move %s to the Trash: %v. Move it yourself."), b, err))
		}
	}
	return msgs
}
