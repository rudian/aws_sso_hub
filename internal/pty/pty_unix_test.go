//go:build !windows

package pty

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// Safety rules for every test here:
//   - A shell that is a login or interactive shell gets its own HOME and ZDOTDIR (a new temp dir with its own
//     rc files), an explicit environment without BASH_ENV and ENV, and an explicit shell path. The owner's
//     shell files never run.
//   - A test stops only what it started, through KillSession of the session it created. Never by name, never
//     pkill, killall or kill -1.
//   - After each test every recorded pid must be gone.

// harness holds the temp dir, the pids a test saw and the sessions it started.
type harness struct {
	t    *testing.T
	dir  string // HOME, ZDOTDIR and the place the fake tools write their pid files
	pids []int
	sess []Session
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	h := &harness{t: t, dir: t.TempDir()}
	t.Cleanup(h.cleanup)
	return h
}

func (h *harness) cleanup() {
	for _, s := range h.sess {
		_ = s.KillSession()
		_ = s.Close()
	}
	for _, pid := range h.pids {
		if !waitGone(pid, 5*time.Second) {
			h.t.Errorf("process %d is still running after the test", pid)
			_ = syscall.Kill(pid, syscall.SIGKILL) // a pid this test recorded, nothing else
		}
	}
}

// env is the complete child environment: no BASH_ENV, no ENV, no variable of the real shell setup.
func (h *harness) env() []string {
	return []string{
		"HOME=" + h.dir,
		"ZDOTDIR=" + h.dir,
		"PATH=/usr/bin:/bin:/usr/sbin:/sbin",
		"TERM=xterm-256color",
		"LANG=en_US.UTF-8",
		"PIDDIR=" + h.dir,
		"HISTFILE=" + filepath.Join(h.dir, "hist"),
		"SHELL_SESSIONS_DISABLE=1", // macOS /etc/zshrc: no saved shell sessions
	}
}

func (h *harness) write(name, body string) {
	h.t.Helper()
	if err := os.WriteFile(filepath.Join(h.dir, name), []byte(body), 0o600); err != nil {
		h.t.Fatal(err)
	}
}

// fakeShellFiles writes the rc files of both shells. faketool stands in for a tool the owner starts through
// a shell function: the function runs an external program, which the interactive shell runs as a job in its
// own process group. The program starts a grandchild (a "dev server") and records both pids.
func (h *harness) fakeShellFiles() {
	h.write(".zshenv", "# test\n")
	h.write(".zprofile", "# test\n")
	h.write(".zlogin", "# test\n")
	fn := `faketool() {
  /bin/sh -c 'sleep 300 & echo $! > "$PIDDIR/gc.pid"; echo $$ > "$PIDDIR/tool.pid"; wait'
}
`
	h.write(".zshrc", "PROMPT=''\nunsetopt zle\n"+fn)
	h.write(".bash_profile", "[ -f \"$HOME/.bashrc\" ] && . \"$HOME/.bashrc\"\n")
	h.write(".bashrc", "PS1=''\n"+fn)
}

func (h *harness) start(argv []string, grace time.Duration) Session {
	h.t.Helper()
	s, err := Start(Options{Argv: argv, Dir: h.dir, Env: h.env(), Cols: 80, Rows: 24, Grace: grace})
	if err != nil {
		h.t.Fatal(err)
	}
	h.sess = append(h.sess, s)
	h.pids = append(h.pids, s.Pid())
	return s
}

// readPid waits for a pid file written by a fake tool and records the pid.
func (h *harness) readPid(name string) int {
	h.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if b, err := os.ReadFile(filepath.Join(h.dir, name)); err == nil {
			if pid, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil && pid > 1 {
				h.pids = append(h.pids, pid)
				return pid
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	h.t.Fatalf("pid file %s did not appear", name)
	return 0
}

// waitGone is true once kill(pid, 0) fails (a zombie still answers, so give reaping a moment).
func waitGone(pid int, d time.Duration) bool {
	deadline := time.Now().Add(d)
	for {
		if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// drain reads a session into a buffer so the child never blocks on a full terminal.
type drain struct {
	mu  sync.Mutex
	buf bytes.Buffer
	eof chan struct{}
}

func startDrain(s Session) *drain {
	d := &drain{eof: make(chan struct{})}
	go func() {
		defer close(d.eof)
		b := make([]byte, 4096)
		for {
			n, err := s.Read(b)
			if n > 0 {
				d.mu.Lock()
				d.buf.Write(b[:n])
				d.mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()
	return d
}

func (d *drain) String() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.buf.String()
}

func (d *drain) waitFor(t *testing.T, re *regexp.Regexp, timeout time.Duration) []string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if m := re.FindStringSubmatch(d.String()); m != nil {
			return m
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("output did not match %v within %v; got %q", re, timeout, d.String())
	return nil
}

func waitDone(t *testing.T, s Session, d time.Duration) {
	t.Helper()
	select {
	case <-s.Done():
	case <-time.After(d):
		t.Fatalf("child %d did not exit within %v", s.Pid(), d)
	}
}

func TestSpawnOutputAndExitCodes(t *testing.T) {
	h := newHarness(t)
	for _, tc := range []struct {
		script string
		want   int
	}{
		{"echo hello-pty; exit 0", 0},
		{"echo bye; exit 7", 7},
		{"kill -9 $$", 128 + 9},
	} {
		s := h.start([]string{"/bin/sh", "-c", tc.script}, time.Second)
		if s.ExitCode() != -1 && s.ExitCode() != tc.want {
			t.Errorf("%q: ExitCode before Done = %d", tc.script, s.ExitCode())
		}
		d := startDrain(s)
		waitDone(t, s, 5*time.Second)
		select {
		case <-d.eof:
		case <-time.After(5 * time.Second):
			t.Fatalf("%q: Read did not reach EOF", tc.script)
		}
		if got := s.ExitCode(); got != tc.want {
			t.Errorf("%q: exit code %d, want %d", tc.script, got, tc.want)
		}
		if strings.HasPrefix(tc.script, "echo hello") && !strings.Contains(d.String(), "hello-pty") {
			t.Errorf("output %q lacks hello-pty", d.String())
		}
		if err := s.KillSession(); err != nil { // after exit: nothing to do, no error
			t.Errorf("KillSession after exit: %v", err)
		}
	}
}

func TestSpawnWriteEcho(t *testing.T) {
	h := newHarness(t)
	s := h.start([]string{"/bin/sh", "-c", "read x; echo got:$x; exit 3"}, time.Second)
	d := startDrain(s)
	if _, err := s.Write([]byte("abc\n")); err != nil {
		t.Fatal(err)
	}
	d.waitFor(t, regexp.MustCompile(`got:abc`), 5*time.Second)
	waitDone(t, s, 5*time.Second)
	if s.ExitCode() != 3 {
		t.Errorf("exit %d", s.ExitCode())
	}
}

func TestResizeReachesChild(t *testing.T) {
	h := newHarness(t)
	s := h.start([]string{"/bin/sh", "-c", `stty size; read x; stty size; read y`}, time.Second)
	d := startDrain(s)
	d.waitFor(t, regexp.MustCompile(`(?m)^24 80\r?$`), 5*time.Second)
	if err := s.Resize(132, 43); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Write([]byte("\n")); err != nil {
		t.Fatal(err)
	}
	d.waitFor(t, regexp.MustCompile(`(?m)^43 132\r?$`), 5*time.Second)
	_, _ = s.Write([]byte("\n"))
	waitDone(t, s, 5*time.Second)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.Resize(10, 10); !errors.Is(err, ErrClosed) {
		t.Errorf("Resize after Close = %v, want ErrClosed", err)
	}
	if _, err := s.Write([]byte("x")); !errors.Is(err, ErrClosed) {
		t.Errorf("Write after Close = %v, want ErrClosed", err)
	}
	if _, err := s.Read(make([]byte, 8)); !errors.Is(err, io.EOF) {
		t.Errorf("Read after Close = %v, want EOF", err)
	}
}

func TestLaunchArgv(t *testing.T) {
	got := LaunchArgv("/bin/zsh", "claude", "--resume", "a b")
	want := []string{"/bin/zsh", "-l", "-i", "-c", `"$@"`, "codepass", "claude", "--resume", "a b"}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Errorf("got %q", got)
	}
}

// members returns the live processes of the session whose leader is sid.
func members(t *testing.T, sid int) []int {
	t.Helper()
	k := newKiller(sid, time.Second)
	k.mu.Lock()
	defer k.mu.Unlock()
	m, _, err := k.raw()
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// The real launch form with a shell function. The tool runs as a job in its own process group, which a
// process-group kill misses (review A, X1).
func TestKillTreeLaunchFormShellFunction(t *testing.T) {
	for _, sh := range []string{"/bin/zsh", "/bin/bash"} {
		sh := sh
		t.Run(filepath.Base(sh), func(t *testing.T) {
			if _, err := os.Stat(sh); err != nil {
				t.Skip(sh + " not installed")
			}
			h := newHarness(t)
			h.fakeShellFiles()
			s := h.start(LaunchArgv(sh, "faketool"), 3*time.Second)
			startDrain(s)
			tool := h.readPid("tool.pid")
			gc := h.readPid("gc.pid")

			leaderPg, _ := unix.Getpgid(s.Pid())
			toolPg, _ := unix.Getpgid(tool)
			gcPg, _ := unix.Getpgid(gc)
			t.Logf("leader %d pgid %d; tool %d pgid %d; grandchild %d pgid %d", s.Pid(), leaderPg, tool, toolPg, gc, gcPg)
			if toolPg == leaderPg {
				t.Fatalf("the tool shares the leader's process group: this test would not show the X1 case")
			}
			if sid, _ := unix.Getsid(tool); sid != s.Pid() {
				t.Fatalf("tool is not in the leader's session: %d", sid)
			}
			if n := len(members(t, s.Pid())); n < 3 {
				t.Fatalf("expected at least 3 session members, found %d", n)
			}

			start := time.Now()
			if err := s.KillSession(); err != nil {
				t.Fatal(err)
			}
			t.Logf("KillSession took %v", time.Since(start))
			for _, pid := range []int{s.Pid(), tool, gc} {
				if !waitGone(pid, 3*time.Second) {
					t.Errorf("pid %d survived KillSession", pid)
				}
			}
			waitDone(t, s, 3*time.Second)
			if left := members(t, s.Pid()); len(left) != 0 {
				t.Errorf("session members left: %v", left)
			}
			if err := s.KillSession(); err != nil { // second call is harmless
				t.Errorf("second KillSession: %v", err)
			}
		})
	}
}

// A background job started under "set -m" gets its own process group.
func TestKillTreeJobControlGrandchild(t *testing.T) {
	h := newHarness(t)
	s := h.start([]string{"/bin/sh", "-c", `set -m; sleep 300 & echo $! > "$PIDDIR/gc.pid"; wait`}, 3*time.Second)
	startDrain(s)
	gc := h.readPid("gc.pid")
	leaderPg, _ := unix.Getpgid(s.Pid())
	gcPg, _ := unix.Getpgid(gc)
	if gcPg == leaderPg {
		t.Fatalf("grandchild %d shares the leader's group %d: set -m had no effect", gc, leaderPg)
	}
	if err := s.KillSession(); err != nil {
		t.Fatal(err)
	}
	for _, pid := range []int{s.Pid(), gc} {
		if !waitGone(pid, 3*time.Second) {
			t.Errorf("pid %d survived KillSession", pid)
		}
	}
}

// A child that ignores SIGTERM is gone after the SIGKILL step; the polite step did wait the grace period.
func TestKillIgnoresSIGTERMUntilSIGKILL(t *testing.T) {
	h := newHarness(t)
	grace := 400 * time.Millisecond
	s := h.start([]string{"/bin/sh", "-c", `trap '' TERM; sleep 300 & echo $! > "$PIDDIR/gc.pid"; echo ready; while :; do sleep 1; done`}, grace)
	d := startDrain(s)
	d.waitFor(t, regexp.MustCompile(`ready`), 5*time.Second)
	gc := h.readPid("gc.pid") // sleep inherits the ignored SIGTERM

	// The polite signal alone must not end it: send it to the group ourselves first.
	_ = syscall.Kill(-s.Pid(), syscall.SIGTERM)
	time.Sleep(150 * time.Millisecond)
	if syscall.Kill(s.Pid(), 0) != nil || syscall.Kill(gc, 0) != nil {
		t.Fatal("the fake tool did not ignore SIGTERM")
	}

	start := time.Now()
	if err := s.KillSession(); err != nil {
		t.Fatal(err)
	}
	if el := time.Since(start); el < grace {
		t.Errorf("KillSession returned after %v, before the %v grace period", el, grace)
	}
	for _, pid := range []int{s.Pid(), gc} {
		if !waitGone(pid, 3*time.Second) {
			t.Errorf("pid %d survived the SIGKILL step", pid)
		}
	}
	waitDone(t, s, 3*time.Second)
	if s.ExitCode() != 128+9 {
		t.Errorf("exit code %d, want 137", s.ExitCode())
	}
}

// Nothing outside the session is touched: a bystander in its own session survives.
func TestKillTouchesOnlyTheSession(t *testing.T) {
	h := newHarness(t)
	other := h.start([]string{"/bin/sh", "-c", "sleep 300"}, time.Second) // its own session
	startDrain(other)
	s := h.start([]string{"/bin/sh", "-c", "sleep 300 & wait"}, time.Second)
	startDrain(s)
	time.Sleep(200 * time.Millisecond)
	if err := s.KillSession(); err != nil {
		t.Fatal(err)
	}
	waitDone(t, s, 3*time.Second)
	if syscall.Kill(other.Pid(), 0) != nil {
		t.Fatal("a process of another session was killed")
	}
	// the leader of "other" is ours: stop it through its own session
	if err := other.KillSession(); err != nil {
		t.Fatal(err)
	}
}

func TestKillRefusesBadSessionIDs(t *testing.T) {
	if err := newKiller(0, time.Millisecond).kill(); err == nil {
		t.Error("session 0 accepted")
	}
	if err := newKiller(1, time.Millisecond).kill(); err == nil {
		t.Error("session 1 accepted")
	}
	if mine, err := unix.Getsid(0); err == nil {
		if err := newKiller(mine, time.Millisecond).kill(); err == nil {
			t.Error("the caller's own session accepted")
		}
	}
}

// fakeKiller is a killer whose process table and signals are injected.
type fakeKiller struct {
	*killer
	mu      sync.Mutex
	lists   int
	signals []int
	table   func() ([]int, error)
}

func newFakeKiller(sid int) *fakeKiller {
	f := &fakeKiller{}
	k := newKiller(1<<30, 50*time.Millisecond) // a number no real process has: start time unknown
	k.sid = sid
	k.hardWait = 200 * time.Millisecond
	k.confirm = time.Millisecond
	k.list = func() ([]int, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.lists++
		return f.table()
	}
	k.getsid = func(pid int) (int, error) { return sid, nil }
	k.pgid = func(pid int) (int, error) { return pid, nil }
	k.signal = func(pid int, sig syscall.Signal) error {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.signals = append(f.signals, pid)
		return nil
	}
	f.killer = k
	return f
}

// Review fix: once the session was seen empty, a later KillSession neither lists nor signals, even if the
// number now belongs to a new session leader.
func TestKillAfterEndedNeitherListsNorSignals(t *testing.T) {
	f := newFakeKiller(54321)
	f.table = func() ([]int, error) { return nil, nil } // empty session
	if err := f.kill(); err != nil {
		t.Fatal(err)
	}
	if f.lists != 2 { // the empty listing and its confirmation
		t.Fatalf("first call listed %d times", f.lists)
	}
	f.table = func() ([]int, error) { return []int{777}, nil } // "a new session with the same sid"
	for i := 0; i < 3; i++ {
		if err := f.kill(); err != nil {
			t.Fatal(err)
		}
	}
	if f.lists != 2 || len(f.signals) != 0 {
		t.Errorf("late KillSession listed %d times and sent %v", f.lists, f.signals)
	}
	if m, _ := f.members(); len(m) != 0 {
		t.Errorf("members after end: %v", m)
	}
}

// The watcher latches an emptied session on its own.
func TestWatchLatchesEmptySession(t *testing.T) {
	f := newFakeKiller(54321)
	n := 0
	f.table = func() ([]int, error) {
		n++
		if n < 3 {
			return []int{900}, nil
		}
		return nil, nil
	}
	f.watch(5 * time.Millisecond)
	f.table = func() ([]int, error) { return []int{777}, nil }
	if err := f.kill(); err != nil {
		t.Fatal(err)
	}
	if len(f.signals) != 0 {
		t.Errorf("signals after the watcher latched: %v", f.signals)
	}
}

// A live process with the leader's number but another start time is not ours.
func TestKillRefusesStartTimeMismatch(t *testing.T) {
	f := newFakeKiller(54321)
	f.haveStart, f.startAt = true, 1000
	f.start = func(pid int) (int64, bool) { return 2000, true }
	f.table = func() ([]int, error) { return []int{54321}, nil }
	if err := f.kill(); err != nil {
		t.Fatal(err)
	}
	if f.lists != 0 || len(f.signals) != 0 {
		t.Errorf("listed %d times, signalled %v", f.lists, f.signals)
	}
	// A matching start time proceeds.
	g := newFakeKiller(54321)
	g.haveStart, g.startAt = true, 1000
	g.start = func(pid int) (int64, bool) { return 1000, true }
	left := true
	g.table = func() ([]int, error) {
		if left {
			return []int{54321}, nil
		}
		return nil, nil
	}
	g.signal = func(pid int, sig syscall.Signal) error { left = false; return nil }
	if err := g.kill(); err != nil {
		t.Fatal(err)
	}
}

// An unreadable process table is an error, not success, and the leader's group still gets the signal.
func TestKillListErrorIsNotSuccess(t *testing.T) {
	f := newFakeKiller(54321)
	f.table = func() ([]int, error) { return nil, errors.New("sysctl failed") }
	if err := f.kill(); err == nil {
		t.Fatal("kill reported success with an unreadable process table")
	}
	if len(f.signals) == 0 || f.signals[0] != -54321 {
		t.Errorf("signals %v: want the leader's group first", f.signals)
	}
	if f.lists != 2 {
		t.Errorf("listed %d times, want one retry", f.lists)
	}
}

// Resize racing Close must not trip the race detector or hit a reused descriptor.
func TestResizeCloseRace(t *testing.T) {
	h := newHarness(t)
	for i := 0; i < 20; i++ {
		s := h.start([]string{"/bin/sh", "-c", "sleep 30"}, 500*time.Millisecond)
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				if err := s.Resize(uint16(80+j), 24); err != nil && !errors.Is(err, ErrClosed) {
					t.Errorf("Resize: %v", err)
					return
				}
				if _, err := s.Foreground(); err != nil && !errors.Is(err, ErrClosed) {
					t.Errorf("Foreground: %v", err)
					return
				}
			}
		}()
		go func() { defer wg.Done(); _ = s.Close() }()
		wg.Wait()
		if err := s.KillSession(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestForegroundAndMembers(t *testing.T) {
	h := newHarness(t)
	s := h.start([]string{"/bin/sh", "-c", "sleep 300 & wait"}, time.Second)
	startDrain(s)
	time.Sleep(300 * time.Millisecond)
	fg, err := s.Foreground()
	if err != nil {
		t.Fatal(err)
	}
	if fg != s.Pid() {
		t.Errorf("foreground group %d, want %d", fg, s.Pid())
	}
	m, err := s.Members()
	if err != nil || len(m) < 2 {
		t.Fatalf("members %v, %v", m, err)
	}
	h.pids = append(h.pids, m...)
	if err := s.KillSession(); err != nil {
		t.Fatal(err)
	}
	if m, _ := s.Members(); len(m) != 0 {
		t.Errorf("members after KillSession: %v", m)
	}
	_ = s.Close()
	if _, err := s.Foreground(); !errors.Is(err, ErrClosed) {
		t.Errorf("Foreground after Close = %v", err)
	}
}

// Pins what the app dying does on Unix (the master closes, the kernel hangs up the foreground job): the
// shell and a plain background child end; a nohup child and a "set -m" background group survive. The docs
// and plan 4.8b say so; A2-13 must clean such leftovers up at the next start.
func TestCrashLeavesNohupAndJobControlGroups(t *testing.T) {
	h := newHarness(t)
	script := `sleep 300 & echo $! > "$PIDDIR/plain.pid"
nohup sleep 301 >/dev/null 2>&1 &
echo $! > "$PIDDIR/nohup.pid"
set -m
sleep 302 &
echo $! > "$PIDDIR/jobctl.pid"
wait`
	s := h.start([]string{"/bin/sh", "-c", script}, time.Second)
	plain, nohup, jobctl := h.readPid("plain.pid"), h.readPid("nohup.pid"), h.readPid("jobctl.pid")
	time.Sleep(200 * time.Millisecond)
	_ = s.Close() // what an app crash does to the master (no Read is pending here)
	for _, pid := range []int{s.Pid(), plain} {
		if !waitGone(pid, 3*time.Second) {
			t.Errorf("pid %d survived the hangup", pid)
		}
	}
	if syscall.Kill(nohup, 0) != nil || syscall.Kill(jobctl, 0) != nil {
		t.Errorf("expected the nohup child and the set -m group to survive a crash; nohup alive=%v jobctl alive=%v",
			syscall.Kill(nohup, 0) == nil, syscall.Kill(jobctl, 0) == nil)
	}
	if err := s.KillSession(); err != nil { // the cleanup A2-13 has to do after a real crash
		t.Fatal(err)
	}
}

// An empty listing in which a process vanished is inconclusive: no latch, and the next listing finds the
// members (the unconfirmed-empty race of review round 2).
func TestVanishedPidIsInconclusive(t *testing.T) {
	f := newFakeKiller(54321)
	calls := 0
	f.table = func() ([]int, error) { calls++; return []int{900, 901}, nil }
	f.getsid = func(pid int) (int, error) {
		if calls <= 2 { // the first listings: every pid vanished before getsid
			return 0, syscall.ESRCH
		}
		return 54321, nil
	}
	m, err := f.members()
	if err != nil || len(m) != 2 {
		t.Fatalf("members = %v, %v", m, err)
	}
	if f.ended {
		t.Error("latched on an inconclusive pass")
	}
	// Always-vanishing pids: reported empty for now, never latched.
	f.getsid = func(pid int) (int, error) { return 0, syscall.ESRCH }
	if m, _ := f.members(); len(m) != 0 || f.ended {
		t.Errorf("members %v ended %v", m, f.ended)
	}
}

// An empty pass confirmed non-empty by the second listing does not latch.
func TestEmptyNeedsConfirmation(t *testing.T) {
	f := newFakeKiller(54321)
	n := 0
	f.table = func() ([]int, error) {
		n++
		if n == 1 {
			return nil, nil
		}
		return []int{900}, nil
	}
	m, _ := f.members()
	if len(m) != 1 || f.ended {
		t.Errorf("members %v ended %v", m, f.ended)
	}
}

// Real processes: each generation forks the next one and exits (the last live member forks a child and exits
// between a listing's snapshot and its getsid calls). Polling Members() must never latch while a second
// listing still finds members; KillSession then removes everything.
func TestForkThenExitChainDoesNotLatch(t *testing.T) {
	h := newHarness(t)
	h.write("chain.sh", `trap '' HUP
n=${1:-0}
echo $$ >> "$PIDDIR/chain.pids"
if [ "$n" -lt 250 ]; then
  sleep 0.01
  /bin/sh "$0" $((n+1)) &
  exit 0
fi
sleep 300
`)
	s := h.start([]string{"/bin/sh", "-c", `/bin/sh "$PIDDIR/chain.sh" 0 & wait`}, 3*time.Second)
	startDrain(s)
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		m, err := s.Members()
		if err != nil {
			t.Fatal(err)
		}
		if len(m) == 0 {
			// reported empty: it must really be empty, and then the latch is legitimate
			time.Sleep(60 * time.Millisecond)
			if left := members(t, s.Pid()); len(left) > 0 {
				t.Fatalf("Members() reported an empty session while %v live", left)
			}
			break
		}
	}
	if err := s.KillSession(); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(h.dir, "chain.pids")); err == nil {
		for _, f := range strings.Fields(string(b)) {
			if pid, err := strconv.Atoi(f); err == nil && pid > 1 {
				h.pids = append(h.pids, pid)
			}
		}
	}
	if left := members(t, s.Pid()); len(left) > 0 {
		t.Errorf("members left after KillSession: %v", left)
	}
}
