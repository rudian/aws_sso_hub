//go:build !windows

package aitools

import (
	"context"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"
)

// When the app is started from a terminal it has a controlling terminal. An interactive login shell started in a
// background process group would be stopped by that terminal; RealRun starts it in a session of its own, so
// detection answers well inside its budget. The test runs itself under a pseudo terminal that is its controlling
// terminal (creack/pty makes the child a session leader with the terminal as its own).
func TestDetectWorksWithAControllingTerminal(t *testing.T) {
	if os.Getenv("CODEPASS_TTY_CHILD") == "" {
		h := newFakeHome(t)
		h.tool("claude", `echo "2.1.5 (Claude Code)"`)
		cmd := exec.Command(os.Args[0], "-test.run=^TestDetectWorksWithAControllingTerminal$", "-test.v")
		cmd.Env = append(os.Environ(), "CODEPASS_TTY_CHILD="+h.Home)
		f, err := pty.Start(cmd)
		if err != nil {
			t.Skip("no pseudo terminal: " + err.Error())
		}
		defer f.Close()
		outc := make(chan string, 1)
		go func() { b, _ := io.ReadAll(f); outc <- string(b) }()
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case err := <-done:
			out := <-outc
			if err != nil {
				t.Fatalf("under a terminal: %v\n%s", err, out)
			}
		case <-time.After(30 * time.Second):
			cmd.Process.Kill() // the process this test started
			t.Fatal("detection hung under a controlling terminal")
		}
		return
	}
	home := os.Getenv("CODEPASS_TTY_CHILD")
	env := HomeEnv(home, "/usr/bin:/bin")
	d := DetectOne(context.Background(), DetectOptions{GOOS: "darwin", Env: env, Shell: "/bin/zsh", Timeout: 15 * time.Second}, Claude)
	if !d.Installed || d.Version != "2.1.5" || strings.Contains(d.Note, "timeout") {
		t.Fatalf("%+v", d)
	}
}
