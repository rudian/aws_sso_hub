//go:build windows

package pty

// These tests need Windows (owner's check B8 on the Windows VM). They run only the tools they start
// (cmd.exe, ping.exe, powershell.exe from System32) and stop only through KillSession on their own session.

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func winEnv(t *testing.T) []string {
	sys := os.Getenv("SystemRoot")
	if sys == "" {
		sys = `C:\Windows`
	}
	return []string{"SystemRoot=" + sys, "PATH=" + filepath.Join(sys, "System32"), "TEMP=" + t.TempDir()}
}

func drainAll(s Session) {
	go func() {
		b := make([]byte, 4096)
		for {
			if _, err := s.Read(b); err != nil {
				return
			}
		}
	}()
}

func processAlive(pid uint32) bool {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, pid)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	ev, _ := windows.WaitForSingleObject(h, 0)
	return ev == uint32(windows.WAIT_TIMEOUT)
}

// collector reads a session into a buffer.
type collector struct {
	mu  sync.Mutex
	buf strings.Builder
	eof chan struct{}
}

func collect(s Session) *collector {
	c := &collector{eof: make(chan struct{})}
	go func() {
		defer close(c.eof)
		b := make([]byte, 4096)
		for {
			n, err := s.Read(b)
			c.mu.Lock()
			c.buf.Write(b[:n])
			c.mu.Unlock()
			if err != nil {
				return
			}
		}
	}()
	return c
}

func (c *collector) String() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.buf.String()
}

func (c *collector) waitFor(t *testing.T, re *regexp.Regexp, d time.Duration) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if re.MatchString(c.String()) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("output did not match %v within %v; got %q", re, d, c.String())
}

func waitPidsGone(t *testing.T, pids []int, d time.Duration) {
	t.Helper()
	deadline := time.Now().Add(d)
	for _, pid := range pids {
		for processAlive(uint32(pid)) {
			if time.Now().After(deadline) {
				t.Errorf("process %d is still running", pid)
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
}

func TestWindowsSpawnExitCode(t *testing.T) {
	s, err := Start(Options{Argv: []string{"cmd.exe", "/c", "echo hello-conpty & exit 7"}, Env: winEnv(t), Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	c := collect(s)
	select {
	case <-s.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("child did not exit")
	}
	select {
	case <-c.eof:
	case <-time.After(10 * time.Second):
		t.Fatal("Read did not reach EOF after the child exited")
	}
	if s.ExitCode() != 7 {
		t.Errorf("exit code %d, want 7", s.ExitCode())
	}
	if !strings.Contains(c.String(), "hello-conpty") {
		t.Errorf("output %q", c.String())
	}
}

// The child sees the new size, and Resize after the pseudo console is closed is safe and returns ErrClosed.
func TestWindowsResize(t *testing.T) {
	s, err := Start(Options{Argv: []string{"cmd.exe", "/k"}, Env: winEnv(t), Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	c := collect(s)
	if err := s.Resize(100, 30); err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond)
	if _, err := s.Write([]byte("mode con\r\n")); err != nil {
		t.Fatal(err)
	}
	c.waitFor(t, regexp.MustCompile(`Columns:\s+100`), 10*time.Second)
	c.waitFor(t, regexp.MustCompile(`Lines:\s+30`), 10*time.Second)

	if err := s.KillSession(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-s.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("root survived KillSession")
	}
	select {
	case <-c.eof:
	case <-time.After(10 * time.Second):
		t.Fatal("Read did not reach EOF after the job was empty")
	}
	time.Sleep(300 * time.Millisecond) // the pseudo console has been closed by now
	for i := 0; i < 20; i++ {
		if err := s.Resize(90, 20); !errors.Is(err, ErrClosed) {
			t.Fatalf("Resize after the end = %v, want ErrClosed", err)
		}
	}
}

// A grandchild started with "start /b" is in the job and dies with KillSession; the console stays open while
// a descendant lives after the root has exited.
func TestWindowsKillTree(t *testing.T) {
	s, err := Start(Options{Argv: []string{"cmd.exe", "/c", "start /b ping -t 127.0.0.1 > nul & ping -t 127.0.0.1 > nul"}, Env: winEnv(t), Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	drainAll(s)
	var pids []int
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if pids, err = s.Members(); err == nil && len(pids) >= 3 { // cmd.exe and two ping.exe
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil || len(pids) < 2 {
		t.Fatalf("job members %v (%v): want the root and at least one descendant", pids, err)
	}
	if !processAlive(uint32(s.Pid())) {
		t.Fatal("root exited early")
	}
	if err := s.KillSession(); err != nil {
		t.Fatal(err)
	}
	if left, _ := s.Members(); len(left) != 0 {
		t.Errorf("members after KillSession: %v", left)
	}
	waitPidsGone(t, pids, 10*time.Second)
	select {
	case <-s.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("root survived KillSession")
	}
	if err := s.KillSession(); err != nil {
		t.Errorf("second KillSession: %v", err)
	}
}

// App crash: the helper (this test binary) starts a session and is terminated without any cleanup; the job
// handle dies with it and KILL_ON_JOB_CLOSE takes every member down.
func TestWindowsHelperProc(t *testing.T) {
	out := os.Getenv("CODEPASS_PTY_HELPER_OUT")
	if out == "" {
		t.Skip("helper process only")
	}
	s, err := Start(Options{Argv: []string{"cmd.exe", "/c", "start /b ping -t 127.0.0.1 > nul & ping -t 127.0.0.1 > nul"}, Env: winEnv(t), Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	drainAll(s)
	for i := 0; i < 100; i++ {
		if pids, _ := s.Members(); len(pids) >= 3 {
			var b strings.Builder
			for _, p := range pids {
				fmt.Fprintln(&b, p)
			}
			_ = os.WriteFile(out+".tmp", []byte(b.String()), 0o600)
			_ = os.Rename(out+".tmp", out)
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	time.Sleep(60 * time.Second) // the parent terminates this process long before
}

func TestWindowsCrashKillsTree(t *testing.T) {
	out := filepath.Join(t.TempDir(), "pids.txt")
	cmd := exec.Command(os.Args[0], "-test.run=^TestWindowsHelperProc$")
	cmd.Env = append(winEnv(t), "CODEPASS_PTY_HELPER_OUT="+out)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	var pids []int
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) && pids == nil {
		if b, err := os.ReadFile(out); err == nil {
			for _, f := range strings.Fields(string(b)) {
				if p, err := strconv.Atoi(f); err == nil {
					pids = append(pids, p)
				}
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	if len(pids) < 3 {
		t.Fatalf("helper reported %v", pids)
	}
	if err := cmd.Process.Kill(); err != nil { // TerminateProcess: no deferred cleanup runs
		t.Fatal(err)
	}
	waitPidsGone(t, pids, 15*time.Second)
}
