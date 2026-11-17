//go:build !windows

package terminal

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// These tests start real children on a real terminal, through the manager. They run plain /bin/sh with a
// small explicit environment: no login shell, so no shell file of the person running the tests is read.

func realManager(t *testing.T, cfg Config) *Manager {
	t.Helper()
	cfg.BaseEnv = func() []string {
		return []string{"PATH=/usr/bin:/bin", "HOME=" + t.TempDir(), "AWSSSOHUB_SANDBOX=/must/not/leak", "WAILS_X=1"}
	}
	cfg.Grace = 300 * time.Millisecond
	m := NewManager(cfg)
	t.Cleanup(func() { m.Shutdown(10 * time.Second) })
	return m
}

func readAll(m *Manager, id string) string {
	var out []byte
	var off int64
	for {
		r := m.Read(id, off, 0)
		out = append(out, r.Data...)
		off = r.Next
		if r.Done {
			return string(out)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestRealChildEnvironmentOutputAndExitCode(t *testing.T) {
	m := realManager(t, Config{})
	_, err := m.Start(StartOptions{ID: "t1", Argv: []string{"/bin/sh", "-c", "env; echo END; exit 3"}, Dir: t.TempDir(), Env: []string{"CODEPASS_TASK=t1"}})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "end", 10*time.Second, func() bool { return m.StatusOf("t1").Ended })
	out := readAll(m, "t1")
	for _, want := range []string{"TERM=xterm-256color", "COLORTERM=truecolor", "LANG=en_US.UTF-8", "CODEPASS_TASK=t1", "END"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "AWSSSOHUB_") || strings.Contains(out, "WAILS_") {
		t.Errorf("the child saw the app's variables:\n%s", out)
	}
	st := m.StatusOf("t1")
	if !st.RootDone || st.ExitCode != 3 {
		t.Fatalf("%+v", st)
	}
}

func TestRealResizeReachesTheChildAndInputIsDelivered(t *testing.T) {
	m := realManager(t, Config{})
	if _, err := m.Start(StartOptions{ID: "t1", Argv: []string{"/bin/sh", "-c", "read x; stty size; echo got-$x"}, Cols: 80, Rows: 24}); err != nil {
		t.Fatal(err)
	}
	if err := m.Resize("t1", 101, 37); err != nil {
		t.Fatal(err)
	}
	if err := m.Write("t1", []byte("ping\n")); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "end", 10*time.Second, func() bool { return m.StatusOf("t1").Ended })
	out := readAll(m, "t1")
	if !strings.Contains(out, "37 101") || !strings.Contains(out, "got-ping") {
		t.Fatalf("%q", out)
	}
	// after the end, the console is closed: resize and write are not errors
	if err := m.Resize("t1", 90, 30); err != nil {
		t.Fatal(err)
	}
	if err := m.Write("t1", []byte("late")); err != nil {
		t.Fatal(err)
	}
}

// The flood: 50 MB from a real child while nobody reads and Emit never returns (the stuck webview). The child
// must finish, the reader must have taken every byte, and a late page must get a gap and a prologue.
func TestRealFloodWithNobodyReadingDoesNotStallTheChild(t *testing.T) {
	block := make(chan struct{})
	defer close(block)
	m := realManager(t, Config{LogCap: 4 << 20, Emit: func(string, any) { <-block }})
	const mb = 50
	script := "printf '\\033[?1049h'; yes xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx | head -c " + strconv.Itoa(mb<<20)
	t0 := time.Now()
	if _, err := m.Start(StartOptions{ID: "t1", Argv: []string{"/bin/sh", "-c", script}}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the flood to finish", 60*time.Second, func() bool { return m.StatusOf("t1").Ended })
	st := m.StatusOf("t1")
	t.Logf("%d MB in %v; longest append+notify %v; trims %d", st.TotalRead>>20, time.Since(t0), st.LongestAppend, st.Trims)
	if st.TotalRead < mb<<20 || st.Tail != st.TotalRead {
		t.Fatalf("%+v", st)
	}
	if sz := st.Tail - st.Base; sz > 4<<20+(4<<20)/8+(4<<20)/16+chunkSize {
		t.Fatalf("log not bounded: %d", sz)
	}
	r := m.Read("t1", 0, 0)
	if r.Gap == 0 || !strings.Contains(string(r.Prologue), "\x1b[?1049h") {
		t.Fatalf("gap %d prologue %q", r.Gap, r.Prologue)
	}
}

func TestRealStopEndsTheWholeSessionAndWritesScrollback(t *testing.T) {
	data := t.TempDir()
	m := realManager(t, Config{DataDir: data})
	// a background job in its own process group (set -m) that ignores SIGTERM, started by the session
	script := "set -m; (trap '' TERM; exec sleep 300) & echo CHILD=$!; echo READY; wait"
	if _, err := m.Start(StartOptions{ID: "t1", Argv: []string{"/bin/sh", "-c", script}}); err != nil {
		t.Fatal(err)
	}
	var pid int
	waitFor(t, "the child pid", 10*time.Second, func() bool {
		out := string(m.Read("t1", 0, 0).Data)
		if i := strings.Index(out, "CHILD="); i >= 0 && strings.Contains(out, "READY") {
			rest := out[i+6:]
			pid, _ = strconv.Atoi(strings.TrimSpace(rest[:strings.IndexAny(rest, "\r\n")]))
			return pid > 0
		}
		return false
	})
	if err := syscall.Kill(pid, 0); err != nil {
		t.Fatalf("the child is not running: %v", err)
	}
	t0 := time.Now()
	if err := m.Stop("t1"); err != nil {
		t.Fatal(err)
	}
	t.Logf("stopped in %v", time.Since(t0))
	waitFor(t, "the child to be gone", 5*time.Second, func() bool { return syscall.Kill(pid, 0) != nil })
	if !m.StatusOf("t1").Ended {
		t.Fatal("not ended")
	}
	path := filepath.Join(data, "sessions", "t1", "scrollback")
	waitFor(t, "scrollback", 5*time.Second, func() bool { _, err := os.Stat(path); return err == nil })
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), "READY") {
		t.Fatalf("%q", b)
	}
	if st, _ := os.Stat(path); st.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", st.Mode().Perm())
	}
}
