// Package proctrack remembers every program an AI session started, also after its terminal is closed
// (plan 4.8b, A2-13).
//
// A session's terminal is stopped by internal/pty, which kills the terminal session. Two things escape that:
// a process that calls setsid, and (after an app crash) a nohup child or a background job. The tracker keeps,
// per session, the set of every descendant it has seen, each remembered by PID and start time so a reused PID
// is never mistaken for it. It lists the set (count, CPU, memory, names) and force-stops it.
//
// One listing of the process table per tick, never one call per process. Command names (not command lines)
// are all that is saved to disk, and nothing about a process is written to the log.
package proctrack

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Proc is one row of the process table.
type Proc struct {
	PID, PPID, PGID int
	Start           int64   // start time, Unix seconds: with the PID it identifies one process for its whole life
	CPU             float64 // percent of one core
	RSS             int64   // resident memory in bytes
	Comm            string  // the executable as the system prints it (a path on macOS); shown, never logged
}

// Name is the program's own short name, never a command line: see nameOf.
func (p Proc) Name() string { return nameOf(p.Comm) }

// pathExists is a variable so tests can fake the one os.Stat that nameOf makes.
var pathExists = func(p string) bool { _, err := os.Stat(p); return err == nil }

// nameOf derives the program name from comm. On macOS comm is argv[0], and programs rewrite it to carry their
// arguments ("npm exec vite --port 9996", "redis-server 127.0.0.1:6390"), so it can hold a token. An absolute
// path that exists is a program path (it may hold spaces): its base name is the name. Anything else is cut at
// the first white space before the base name is taken, so no argument ever becomes part of the name.
func nameOf(comm string) string {
	comm = strings.TrimPrefix(strings.TrimSpace(comm), "-")
	if comm == "" {
		return ""
	}
	if filepath.IsAbs(comm) && pathExists(comm) {
		return baseOf(comm)
	}
	if i := strings.IndexAny(comm, " \t"); i >= 0 {
		comm = comm[:i]
	}
	return strings.TrimPrefix(baseOf(comm), "-")
}

func baseOf(s string) string {
	if i := strings.LastIndexAny(s, `/\`); i >= 0 && i+1 < len(s) {
		s = s[i+1:]
	}
	return path.Clean(s)
}

// Source lists the process table. A listing is one call whatever the number of processes. Want holds the PIDs
// the caller cares about; the Unix source lists everything and ignores it, the Windows one asks about those only.
type Source interface {
	Read(want []int) ([]Proc, error)
}

// SourceFunc adapts a function to Source (tests, fakes).
type SourceFunc func(want []int) ([]Proc, error)

func (f SourceFunc) Read(want []int) ([]Proc, error) { return f(want) }

// PSArgs is the one command of the Unix source. "sess" is unreliable on macOS (it prints 0), so nothing here
// uses it; the session's own members come from internal/pty.
var PSArgs = []string{"-axo", "pid=,ppid=,pgid=,sess=,lstart=,pcpu=,rss=,comm="}

// ParsePS parses the output of `ps` with PSArgs. lstart ("Mon Nov 17 10:23:45 2026") is read in loc. Rows
// that do not parse are skipped: a process can vanish between the kernel's listing and ps printing it.
func ParsePS(out []byte, loc *time.Location) ([]Proc, error) {
	if loc == nil {
		loc = time.Local
	}
	var procs []Proc
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if p, ok := parsePSLine(line, loc); ok {
			procs = append(procs, p)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return procs, nil
}

// field cuts the first whitespace-separated word off s.
func field(s string) (word, rest string) {
	s = strings.TrimLeft(s, " \t")
	i := strings.IndexAny(s, " \t")
	if i < 0 {
		return s, ""
	}
	return s[:i], strings.TrimLeft(s[i:], " \t")
}

func parsePSLine(line string, loc *time.Location) (Proc, bool) {
	var f [11]string
	rest := line
	for i := range f {
		f[i], rest = field(rest)
		if f[i] == "" {
			return Proc{}, false
		}
	}
	comm := strings.TrimSpace(rest) // the rest of the line: a path may hold spaces
	if comm == "" {
		return Proc{}, false
	}
	var p Proc
	var err error
	if p.PID, err = strconv.Atoi(f[0]); err != nil {
		return Proc{}, false
	}
	if p.PPID, err = strconv.Atoi(f[1]); err != nil {
		return Proc{}, false
	}
	if p.PGID, err = strconv.Atoi(f[2]); err != nil {
		return Proc{}, false
	}
	// f[3] is sess; f[4..8] is lstart: weekday month day time year.
	t, err := time.ParseInLocation("Mon Jan _2 15:04:05 2006", strings.Join(f[4:9], " "), loc)
	if err != nil {
		return Proc{}, false
	}
	p.Start = t.Unix()
	if p.CPU, err = strconv.ParseFloat(f[9], 64); err != nil {
		return Proc{}, false
	}
	kb, err := strconv.ParseInt(f[10], 10, 64)
	if err != nil {
		return Proc{}, false
	}
	p.RSS = kb * 1024
	p.Comm = comm
	return p, true
}

// ParseCommands parses `ps -ww -o pid=,lstart=,args=`. Rows that do not parse are skipped.
func ParseCommands(out []byte, loc *time.Location) []CommandLine {
	var res []CommandLine
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	for sc.Scan() {
		rest := strings.TrimSpace(sc.Text())
		var f [6]string
		ok := true
		for i := range f {
			if f[i], rest = field(rest); f[i] == "" {
				ok = false
				break
			}
		}
		if !ok {
			continue
		}
		pid, err := strconv.Atoi(f[0])
		if err != nil {
			continue
		}
		t, err := time.ParseInLocation("Mon Jan _2 15:04:05 2006", strings.Join(f[1:6], " "), loc)
		if err != nil {
			continue
		}
		res = append(res, CommandLine{PID: pid, Start: t.Unix(), Line: strings.TrimSpace(rest)})
	}
	return res
}

func (p Proc) String() string { return fmt.Sprintf("%d@%d", p.PID, p.Start) }
