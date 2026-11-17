// Package pty runs one child process on a pseudo terminal and stops it with everything it started.
//
// It is only the process layer: no byte log, no events, no environment policy (A2-02 builds terminal
// sessions on it). One interface, two implementations:
//
//   - macOS (and other Unix): creack/pty. The child leads a new terminal session. KillSession signals every
//     process in that session, not one process group, because a tool started through a shell function runs
//     as a job in its own process group and a group kill would miss it (review A, X1).
//   - Windows: an own ConPTY wrapper over golang.org/x/sys/windows (derived from go-pty's MIT code, credited
//     in THIRD_PARTY_NOTICES.md). The child runs inside a Job Object with KILL_ON_JOB_CLOSE, so no
//     descendant escapes and an app crash kills the whole tree.
package pty

import (
	"errors"
	"time"
)

// DefaultGrace is how long KillSession waits after the polite signal before it forces the tree down.
const DefaultGrace = 5 * time.Second

// ErrClosed is returned by Write and Resize after Close.
var ErrClosed = errors.New("pty: session closed")

// Options describes the child. Argv[0] is the program (looked up in the app's own PATH when it has no
// directory part). Env is the complete child environment; nil means "inherit", which callers should not use:
// A2-02 passes an explicit one (plan 4.7).
type Options struct {
	Argv       []string
	Dir        string
	Env        []string
	Cols, Rows uint16

	// Grace is the wait between the polite stop signal and the forced kill in KillSession (macOS). Zero means
	// DefaultGrace. Windows ends the tree at once: a Job Object has no polite step.
	Grace time.Duration
}

// Session is one child on a pseudo terminal.
type Session interface {
	// Read returns what the child printed. It returns io.EOF once the terminal is finished: the root child and
	// every other holder of the terminal are gone (both platforms; on Windows the pseudo console is closed only
	// when the job is empty). On macOS, Close does NOT interrupt a Read that is already waiting: the master is
	// a blocking descriptor and the system closes it only after that Read returns, which happens when the last
	// holder of the terminal is gone (after KillSession). A reader loop must therefore end through KillSession,
	// never through Close alone. After Close, a new Read returns io.EOF.
	Read(p []byte) (int, error)
	// Write sends keystrokes to the child.
	Write(p []byte) (int, error)
	// Resize changes the terminal size; the child sees it (SIGWINCH on Unix). It returns ErrClosed after Close,
	// and on Windows also once the pseudo console has been closed (the job is empty), so a mounted terminal of
	// an ended session can call it safely.
	Resize(cols, rows uint16) error
	// Pid is the process ID of the root child (the session leader on Unix).
	Pid() int
	// Done is closed once the root child has exited and been reaped.
	Done() <-chan struct{}
	// ExitCode is the root child's exit code, 128+n when a signal n ended it (Unix), and -1 until Done is closed.
	ExitCode() int
	// Foreground is the process group that is the terminal's foreground job (TIOCGPGRP) on Unix, for the
	// "what is running now" fallback (A2-05). It returns ErrClosed after Close. Windows has no such thing and
	// returns errors.ErrUnsupported; use Members there.
	Foreground() (int, error)
	// Members lists the live processes of the session (Unix: same terminal session ID; Windows: the job). It
	// is empty once the session has ended. The root child is included while it runs.
	Members() ([]int, error)
	// KillSession stops the root child and everything it started in its terminal session (Unix) or job
	// (Windows): a polite stop, the grace period, then SIGKILL. It returns nil once no member is left. It is
	// safe to call more than once and after the child has exited: once the session has been seen empty (it can
	// never gain members again) every later call returns nil without signalling anything, so a process ID that
	// the system reuses later is never touched.
	KillSession() error
	// Close releases the terminal. It does not wait for the child; call KillSession first to end the tree.
	// On Windows, Close ends the tree at once (the job has KILL_ON_JOB_CLOSE, and the app dying does the same).
	// On Unix it does not: it closes our end of the terminal, and the kernel then sends SIGHUP to the
	// foreground job only, so a descendant that ignores SIGHUP (nohup) or runs as a background job in its own
	// process group (a dev server under "set -m") survives an app crash. See the crash test in
	// pty_unix_test.go; A2-13 must record leftovers and offer cleanup at the next start (plan 4.8b).
	Close() error
}

// LaunchArgv builds the login-shell launch form of plan 4.7: the shell starts as a login and interactive
// shell, so the owner's PATH, version managers and shell functions apply, then runs the tool with its
// arguments passed as separate words ("$@"), never joined into a string.
func LaunchArgv(shell, tool string, args ...string) []string {
	out := []string{shell, "-l", "-i", "-c", `"$@"`, "codepass", tool}
	return append(out, args...)
}
