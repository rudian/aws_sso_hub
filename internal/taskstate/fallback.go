package taskstate

import (
	"errors"
	"path/filepath"
	"strings"

	"github.com/rudian/codepass/internal/pty"
)

// Verdict is the fallback's answer when no hook exists (plan 4.8): the foreground process of the terminal is
// the tool itself, so the only things it can say are "a tool is alive" and "back at the shell or gone".
type Verdict int

const (
	Unknown Verdict = iota // the process table could not be read; change nothing
	Alive
	Exited
)

// Process is one row of the process table.
type Process struct {
	PID  int
	PGID int // 0 where unknown (Windows)
	Name string
}

// Table lists the live processes with one call.
type Table interface {
	List() ([]Process, error)
}

// Terminal is the part of internal/pty.Session the fallback reads.
type Terminal interface {
	Foreground() (int, error)
	Members() ([]int, error)
}

// Probe decides alive or exited. macOS: the foreground process group of the terminal (TIOCGPGRP) and one
// process listing; "alive" when a member of that group is not a shell. Windows (Foreground is unsupported):
// the job's members, alive when one of them is not a shell or console host.
func Probe(term Terminal, tbl Table) Verdict {
	members, merr := term.Members()
	if merr == nil && len(members) == 0 {
		return Exited
	}
	fg, ferr := term.Foreground()
	useGroup := ferr == nil
	if ferr != nil && !errors.Is(ferr, errors.ErrUnsupported) {
		// A closed terminal is an ended one; any other error means the answer is unknown.
		if errors.Is(ferr, pty.ErrClosed) {
			return Exited
		}
		return Unknown
	}
	if !useGroup && merr != nil {
		return Unknown
	}
	procs, err := tbl.List()
	if err != nil {
		return Unknown
	}
	inJob := map[int]bool{}
	for _, m := range members {
		inJob[m] = true
	}
	for _, p := range procs {
		if useGroup && p.PGID != fg {
			continue
		}
		if !useGroup && !inJob[p.PID] {
			continue
		}
		if !isShellName(p.Name) {
			return Alive
		}
	}
	return Exited
}

var shellNames = map[string]bool{
	"sh": true, "bash": true, "zsh": true, "fish": true, "dash": true, "ksh": true, "csh": true, "tcsh": true,
	"login": true, "cmd": true, "powershell": true, "pwsh": true, "conhost": true, "openconsole": true,
}

func isShellName(comm string) bool {
	n := strings.ToLower(filepath.Base(strings.ReplaceAll(comm, `\`, "/")))
	n = strings.TrimPrefix(n, "-")
	n = strings.TrimSuffix(n, ".exe")
	return shellNames[n]
}
