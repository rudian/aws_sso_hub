package aitools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func detectAll(t *testing.T, h *fakeHome, timeout time.Duration) map[ID]Detection {
	t.Helper()
	m := map[ID]Detection{}
	for _, d := range Detect(context.Background(), DetectOptions{GOOS: "darwin", Env: h.Env(), Timeout: timeout}) {
		m[d.ID] = d
	}
	return m
}

func TestDetectFindsToolsThroughTheLoginShell(t *testing.T) {
	h := newFakeHome(t)
	h.tool("claude", `echo "2.1.325 (Claude Code)"`)
	h.tool("codex", `echo "codex-cli 0.31.2"`)
	h.tool("opencode", `echo "2.3.0"`)
	m := detectAll(t, h, 20*time.Second)
	for id, ver := range map[ID]string{Claude: "2.1.325", Codex: "0.31.2", OpenCode: "2.3.0"} {
		d := m[id]
		if !d.Installed || d.Version != ver || d.Via != "file" || d.Path != filepath.Join(h.Bin, string(id)) {
			t.Errorf("%s: %+v", id, d)
		}
	}
	// only --version, never `opencode auth list` (it starts OpenCode's shared background service)
	for _, c := range h.calls() {
		if !strings.HasSuffix(c, " [--version]") {
			t.Errorf("detection ran something other than --version: %q", c)
		}
	}
}

func TestDetectMissingToolIsNotAnError(t *testing.T) {
	h := newFakeHome(t)
	h.tool("claude", `echo "2.1.325"`)
	m := detectAll(t, h, 20*time.Second)
	if !m[Claude].Installed {
		t.Fatal("claude")
	}
	for _, id := range []ID{Codex, OpenCode} {
		if d := m[id]; d.Installed || d.Path != "" || d.Version != "" {
			t.Errorf("%s: %+v", id, d)
		}
	}
	ins, _ := InstallFor(Codex, "darwin")
	if ins.Display() != "brew install --cask codex" {
		t.Fatal(ins.Display())
	}
}

// A `claude` shell function of the person's own still applies (plan 4.7): its flags come first, then ours.
func TestDetectSeesAShellFunction(t *testing.T) {
	h := newFakeHome(t)
	h.tool("claude", `echo "2.1.325 (Claude Code)"`)
	h.zshrc(`claude() { command claude --model opus --effort max "$@"; }`)
	d := detectAll(t, h, 20*time.Second)[Claude]
	if !d.Installed || d.Via != "function" || d.Path != "" || d.Version != "2.1.325" {
		t.Fatalf("%+v", d)
	}
	if got := h.calls()[0]; got != "claude [--model] [opus] [--effort] [max] [--version]" {
		t.Fatalf("the function's own flags must come before ours: %q", got)
	}
}

func TestDetectUsesProfileChosenPathOnly(t *testing.T) {
	h := newFakeHome(t)
	other := filepath.Join(h.Home, "tools")
	os.MkdirAll(other, 0o755)
	os.WriteFile(filepath.Join(other, "codex"), []byte("#!/bin/sh\necho 'codex-cli 0.9.0'\n"), 0o755)
	h.zshrc(`export PATH=` + other + `:$PATH`)
	d := detectAll(t, h, 20*time.Second)[Codex]
	if !d.Installed || d.Path != filepath.Join(other, "codex") {
		t.Fatalf("%+v", d)
	}
}

func TestDetectBrokenToolAndNoVersion(t *testing.T) {
	h := newFakeHome(t)
	h.tool("claude", `echo "no number here"; exit 0`)
	h.tool("codex", `echo "2.0.0"; exit 3`)
	m := detectAll(t, h, 20*time.Second)
	for _, id := range []ID{Claude, Codex} {
		if d := m[id]; d.Installed || d.Note == "" {
			t.Errorf("%s: %+v", id, d)
		}
	}
}

func TestDetectWindowsRunsTheProgramDirectly(t *testing.T) {
	var ran [][]string
	run := func(ctx context.Context, argv []string, dir string, env []string) ([]byte, int, error) {
		ran = append(ran, argv)
		return []byte("codex-cli 0.31.2\r\n"), 0, nil
	}
	lookup := func(name string) (string, error) {
		if name == "codex" {
			return `C:\Tools\codex.exe`, nil
		}
		return "", os.ErrNotExist
	}
	ds := Detect(context.Background(), DetectOptions{GOOS: "windows", Env: []string{"PATH=x"}, Run: run, Lookup: lookup})
	if len(ran) != 1 || strings.Join(ran[0], "|") != `C:\Tools\codex.exe|--version` {
		t.Fatalf("%q", ran)
	}
	for _, d := range ds {
		if (d.ID == Codex) != d.Installed {
			t.Errorf("%+v", d)
		}
	}
}

func TestParseVersion(t *testing.T) {
	for in, want := range map[string]string{
		"2.1.325 (Claude Code)": "2.1.325", "codex-cli 0.31.2": "0.31.2", "2.3.0\n": "2.3.0", "v1.4.0-beta.2 x": "1.4.0-beta.2", "none": "", "": "",
	} {
		if got := ParseVersion(in); got != want {
			t.Errorf("%q: %q want %q", in, got, want)
		}
	}
}

// An alias is never expanded by `"$@"`, so a launch could not use it: it is reported as not installed, with a
// note (the fake tool itself is not on PATH, so only the alias exists).
func TestDetectAliasOnlyIsNotInstalled(t *testing.T) {
	h := newFakeHome(t)
	h.zshrc(`alias claude='/nonexistent/claude --model opus'`)
	d := detectAll(t, h, 20*time.Second)[Claude]
	if d.Installed || d.Via != "" || !strings.Contains(d.Note, "alias") {
		t.Fatalf("%+v", d)
	}
}

func TestLoginShellRule(t *testing.T) {
	for _, c := range []struct {
		shell string
		env   []string
		want  string
	}{
		{"", []string{"SHELL=/bin/bash"}, "/bin/bash"},
		{"", []string{"SHELL=/opt/homebrew/bin/zsh"}, "/opt/homebrew/bin/zsh"},
		{"", []string{"SHELL=/usr/bin/fish"}, "/bin/zsh"},
		{"", []string{"SHELL=/opt/homebrew/bin/nu"}, "/bin/zsh"},
		{"", []string{"SHELL=bash"}, "/bin/zsh"}, // not an absolute path
		{"", nil, "/bin/zsh"},
		{"/bin/dash", []string{"SHELL=/bin/bash"}, "/bin/dash"},
		{"/usr/bin/fish", []string{"SHELL=/bin/bash"}, "/bin/zsh"},
	} {
		if got := LoginShell(c.shell, c.env); got != c.want {
			t.Errorf("%q %q: %q want %q", c.shell, c.env, got, c.want)
		}
	}
}

// An alias with the program behind it on PATH: the shell prints the alias for `command -v`, but `"$@"` runs the
// program, so the tool is installed (the alias's flags do not apply).
func TestDetectAliasOverARealProgramIsInstalled(t *testing.T) {
	h := newFakeHome(t)
	h.tool("claude", `echo "2.1.5 (Claude Code)"`)
	h.zshrc(`alias claude='claude --dangerously-skip-permissions'`)
	d := detectAll(t, h, 20*time.Second)[Claude]
	if !d.Installed || d.Version != "2.1.5" || !strings.Contains(d.Note, "alias") {
		t.Fatalf("%+v", d)
	}
	for _, c := range h.calls() {
		if strings.Contains(c, "dangerously") {
			t.Errorf("the alias's flag reached the program: %q", c)
		}
	}
}
