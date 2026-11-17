package taskstate

import (
	"errors"
	"testing"

	"github.com/rudian/codepass/internal/pty"
)

type fakeTerm struct {
	fg      int
	fgErr   error
	members []int
	mErr    error
}

func (f fakeTerm) Foreground() (int, error) { return f.fg, f.fgErr }
func (f fakeTerm) Members() ([]int, error)  { return f.members, f.mErr }

type fakeTable struct {
	ps  []Process
	err error
}

func (f fakeTable) List() ([]Process, error) { return f.ps, f.err }

func TestProbeUnix(t *testing.T) {
	tbl := fakeTable{ps: []Process{{10, 10, "-zsh"}, {20, 20, "/opt/homebrew/bin/claude"}, {21, 20, "node"}, {30, 30, "sleep"}}}
	if v := Probe(fakeTerm{fg: 20, members: []int{10, 20, 21}}, tbl); v != Alive {
		t.Fatalf("tool in foreground: %v", v)
	}
	if v := Probe(fakeTerm{fg: 10, members: []int{10}}, tbl); v != Exited {
		t.Fatalf("back at the shell: %v", v)
	}
	if v := Probe(fakeTerm{fg: 10, members: nil}, tbl); v != Exited {
		t.Fatalf("no members: %v", v)
	}
	if v := Probe(fakeTerm{fgErr: pty.ErrClosed, members: []int{10}}, tbl); v != Exited {
		t.Fatalf("closed: %v", v)
	}
	if v := Probe(fakeTerm{fgErr: errors.New("boom"), members: []int{10}}, tbl); v != Unknown {
		t.Fatalf("error: %v", v)
	}
	if v := Probe(fakeTerm{fg: 20, members: []int{20}}, fakeTable{err: errors.New("ps failed")}); v != Unknown {
		t.Fatalf("table error: %v", v)
	}
}

func TestProbeWindowsJob(t *testing.T) {
	tbl := fakeTable{ps: []Process{{PID: 5, Name: "conhost.exe"}, {PID: 6, Name: "pwsh.exe"}, {PID: 7, Name: `C:\tools\opencode.exe`}, {PID: 8, Name: "other.exe"}}}
	un := errors.ErrUnsupported
	if v := Probe(fakeTerm{fgErr: un, members: []int{5, 6, 7}}, tbl); v != Alive {
		t.Fatalf("%v", v)
	}
	if v := Probe(fakeTerm{fgErr: un, members: []int{5, 6}}, tbl); v != Exited {
		t.Fatalf("%v", v)
	}
	if v := Probe(fakeTerm{fgErr: un, mErr: errors.New("x")}, tbl); v != Unknown {
		t.Fatalf("%v", v)
	}
}
