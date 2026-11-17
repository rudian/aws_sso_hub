package aitools

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fakeHome is a temp HOME that doubles as ZDOTDIR, with its own shell files and a folder of fake tools. The
// person's own shell config never runs, and no real claude, codex or opencode ever starts.
type fakeHome struct {
	t    *testing.T
	Home string
	Bin  string
	Log  string // every fake appends its name and arguments here
}

func newFakeHome(t *testing.T) *fakeHome {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("login shell tests are for Unix")
	}
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh not installed")
	}
	home := t.TempDir()
	h := &fakeHome{t: t, Home: home, Bin: filepath.Join(home, "bin"), Log: filepath.Join(home, "calls.log")}
	if err := os.MkdirAll(h.Bin, 0o755); err != nil {
		t.Fatal(err)
	}
	h.zshrc("")
	return h
}

// zshrc writes .zshrc: PATH first, then extra lines.
func (h *fakeHome) zshrc(extra string) {
	body := "export PATH=" + h.Bin + ":/usr/bin:/bin:/usr/sbin:/sbin\n" + extra + "\n"
	if err := os.WriteFile(filepath.Join(h.Home, ".zshrc"), []byte(body), 0o644); err != nil {
		h.t.Fatal(err)
	}
}

// tool installs a fake program: it logs its arguments, then runs body (a shell snippet; $LOG is the call log).
func (h *fakeHome) tool(name, body string) {
	// one write per call: the three tools run at once and must not interleave their lines
	script := "#!/bin/sh\nLOG=" + h.Log + "\nL=\"" + name + "\"\nfor a in \"$@\"; do L=\"$L [$a]\"; done\nprintf '%s\\n' \"$L\" >> \"$LOG\"\n" + body + "\n"
	if err := os.WriteFile(filepath.Join(h.Bin, name), []byte(script), 0o755); err != nil {
		h.t.Fatal(err)
	}
}

func (h *fakeHome) Env() []string { return HomeEnv(h.Home, "/usr/bin:/bin") }

func (h *fakeHome) calls() []string {
	b, _ := os.ReadFile(h.Log)
	return strings.Split(strings.TrimSpace(string(b)), "\n")
}
