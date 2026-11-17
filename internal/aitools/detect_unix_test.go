//go:build !windows

package aitools

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// A tool that hangs is ended at the time budget, with everything it started, and detection says "timeout".
func TestDetectTimeoutKillsWhatItStarted(t *testing.T) {
	h := newFakeHome(t)
	pidFile := filepath.Join(h.Home, "pid")
	h.tool("claude", `echo $$ > `+pidFile+`; sleep 60`)
	start := time.Now()
	d := DetectOne(context.Background(), DetectOptions{GOOS: "darwin", Env: h.Env(), Timeout: 1500 * time.Millisecond}, Claude)
	if d.Installed || d.Note != "timeout" {
		t.Fatalf("%+v", d)
	}
	if el := time.Since(start); el > 6*time.Second {
		t.Fatalf("took %v", el)
	}
	b, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	pid := 0
	for _, c := range strings.TrimSpace(string(b)) {
		pid = pid*10 + int(c-'0')
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if syscall.Kill(pid, 0) != nil {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	syscall.Kill(pid, syscall.SIGKILL) // a process this test started
	t.Fatal("the fake tool survived the timeout")
}

// The naming call ends a tool that is too slow, with everything it started, and leaves no folder behind.
func TestNamerKillsATooSlowTool(t *testing.T) {
	h := newFakeHome(t)
	pidFile := filepath.Join(h.Home, "pid")
	h.tool("codex", `echo $$ > `+pidFile+`; sleep 60`)
	nm := Namer{GOOS: "darwin", Shell: "/bin/zsh", WorkRoot: filepath.Join(h.Home, "naming"), Budget: 1500 * time.Millisecond,
		BaseEnv: func() []string { return h.Env() }}
	start := time.Now()
	got := nm.Name(context.Background(), Codex, "Fix it", 1)
	if got.Reason != "timeout" || got.Title != "Fix it" || time.Since(start) > 6*time.Second {
		t.Fatalf("%+v %v", got, time.Since(start))
	}
	entries, _ := os.ReadDir(filepath.Join(h.Home, "naming"))
	if len(entries) != 0 {
		t.Fatal("the working folder is left behind")
	}
	b, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if syscall.Kill(pid, 0) != nil {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	syscall.Kill(pid, syscall.SIGKILL) // a process this test started
	t.Fatal("the fake tool survived the naming budget")
}
