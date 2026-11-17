//go:build !windows

package proctrack

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/rudian/codepass/internal/pty"
)

// Real-process tests. Each starts its own terminal session through internal/pty and stops only the programs
// that session started (and the one stranger the test starts itself): never by name, never a PID outside the
// test's own set.

func ptyEnv(dir string) []string {
	return []string{"HOME=" + dir, "ZDOTDIR=" + dir, "PATH=/usr/bin:/bin:/usr/sbin:/sbin", "TERM=xterm-256color", "LANG=en_US.UTF-8"}
}

func alive(pid int) bool {
	if err := syscall.Kill(pid, 0); err != nil {
		return false
	}
	// A zombie still answers signal 0; ask ps for its state.
	out, err := exec.Command("/bin/ps", "-o", "stat=", "-p", strconv.Itoa(pid)).Output()
	return err == nil && !strings.HasPrefix(strings.TrimSpace(string(out)), "Z") && strings.TrimSpace(string(out)) != ""
}

func waitFor(t *testing.T, what string, d time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func readPidFile(t *testing.T, file string) int {
	t.Helper()
	pid := 0
	waitFor(t, file, 10*time.Second, func() bool {
		b, err := os.ReadFile(file)
		if err != nil {
			return false
		}
		n, err := strconv.Atoi(strings.TrimSpace(string(b)))
		pid = n
		return err == nil && n > 1
	})
	return pid
}

// runTracker drives tr with Run at the production cadence (Live 2 s) and stops it with the test.
func runTracker(t *testing.T, tr *Tracker) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); tr.Run(ctx) }()
	t.Cleanup(func() { cancel(); <-done })
}

func startSession(t *testing.T, dir, script string) pty.Session {
	t.Helper()
	s, err := pty.Start(pty.Options{Argv: []string{"/bin/sh", "-c", script}, Dir: dir, Env: append(ptyEnv(dir), "D="+dir), Grace: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.KillSession(); _ = s.Close() })
	go func() { // keep the terminal drained
		buf := make([]byte, 4096)
		for {
			if _, err := s.Read(buf); err != nil {
				return
			}
		}
	}()
	return s
}

func killRecorded(t *testing.T, pids ...int) {
	t.Cleanup(func() { // only the PIDs this test's script recorded
		for _, p := range pids {
			if alive(p) {
				_ = syscall.Kill(p, syscall.SIGKILL)
			}
		}
	})
}

// Accept: a session whose tool started `sh -c "sleep 600 &"` and a server that calls setsid, then exited,
// reports 2 leftover programs; ForceStop leaves none; a program outside the session is never touched. The
// tracker runs at the production cadence (a tick every 2 s, no manual Tick) and the tool lives 4 s after it
// started the programs, so at least one tick sees them while their parent is alive.
func TestRealSessionLeftoversAndForceStop(t *testing.T) {
	if _, err := os.Stat("/usr/bin/perl"); err != nil {
		t.Skip("perl is needed to call setsid")
	}
	dir := t.TempDir()
	stranger := exec.Command("/bin/sleep", "603") // the test's own "user Terminal" program
	if err := stranger.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stranger.Process.Kill(); _, _ = stranger.Process.Wait() })

	script := `trap '' HUP # the tool's helpers survive the hangup that follows the shell's exit
sleep 600 &
echo $! > "$D/bg.pid"
perl -MPOSIX -e 'POSIX::setsid(); exec "sleep", "601"' &
echo $! > "$D/srv.pid"
sleep 4
exit 0`
	s := startSession(t, dir, script)
	bg, srv := readPidFile(t, filepath.Join(dir, "bg.pid")), readPidFile(t, filepath.Join(dir, "srv.pid"))
	killRecorded(t, bg, srv)

	tr := New(Options{Source: NewSource(), Grace: 2 * time.Second, Hard: 3 * time.Second, Poll: 20 * time.Millisecond})
	tr.Track("real", s)
	runTracker(t, tr)
	waitFor(t, "the leader to exit with two programs left", 25*time.Second, func() bool {
		select {
		case <-s.Done():
		default:
			return false
		}
		r := tr.Leftovers("real")
		return !r.Live && r.Count == 2
	})
	r := tr.Leftovers("real")
	if got := pids(r); len(got) != 2 || !(got[0] == bg || got[1] == bg) || !(got[0] == srv || got[1] == srv) {
		t.Fatalf("set %v, want %d and %d", got, bg, srv)
	}
	for _, p := range r.Procs {
		if p.Name != "sleep" {
			t.Errorf("name %q", p.Name)
		}
	}
	res, err := tr.ForceStop("real")
	if err != nil {
		t.Fatal(err)
	}
	if res.Stopped != 2 || len(res.Survivors) != 0 {
		t.Errorf("result %+v", res)
	}
	waitFor(t, "both programs to be gone", 5*time.Second, func() bool { return !alive(bg) && !alive(srv) })
	if !alive(stranger.Process.Pid) {
		t.Error("a program outside the session was stopped")
	}
	if tr.Leftovers("real").Count != 0 || tr.Active() != 0 {
		t.Error("the set was not dropped")
	}
}

// The known limit (plan 4.8b): a program that daemonizes itself (the parent exits right after fork, the child
// calls setsid: nginx, redis --daemonize, ssh -f) is gone from the session and has no tracked parent for longer
// than the few milliseconds the parent lives, so no tick sees it. It is neither listed nor stopped by Force
// stop. The test records that and then ends the program by the PID it wrote itself.
func TestRealDaemonizedProgramIsNotTracked(t *testing.T) {
	if _, err := os.Stat("/usr/bin/perl"); err != nil {
		t.Skip("perl is needed to fork and call setsid")
	}
	dir := t.TempDir()
	script := `trap '' HUP
perl -MPOSIX -e 'exit 0 if fork; POSIX::setsid(); open F, ">$ENV{D}/dm.pid"; print F $$; close F; exec "sleep", "604"'
sleep 4
exit 0`
	s := startSession(t, dir, script)
	dm := readPidFile(t, filepath.Join(dir, "dm.pid"))
	killRecorded(t, dm)

	tr := New(Options{Source: NewSource(), Grace: time.Second, Hard: 2 * time.Second, Poll: 20 * time.Millisecond})
	tr.Track("daemon", s)
	runTracker(t, tr)
	waitFor(t, "the tool to exit", 25*time.Second, func() bool {
		select {
		case <-s.Done():
			return true
		default:
			return false
		}
	})
	time.Sleep(2500 * time.Millisecond) // one more tick after the exit
	for _, p := range tr.Leftovers("daemon").Procs {
		if p.PID == dm {
			t.Fatalf("the daemonized program %d is tracked: the limit in plan 4.8b changed", dm)
		}
	}
	res, err := tr.ForceStop("daemon")
	if err != nil {
		t.Fatal(err)
	}
	if !alive(dm) {
		t.Error("Force stop reached a program the tracker never saw")
	}
	_ = res
	_ = syscall.Kill(dm, syscall.SIGKILL) // the recorded PID: this test's own program
	waitFor(t, "the program to end", 5*time.Second, func() bool { return !alive(dm) })
}

// FU-01b: a nohup child survives when the app dies. A second tracker, started over the same data folder,
// finds it by PID and start time and can end it.
func TestRealCrashLeavesNohupChildAndRestoreEndsIt(t *testing.T) {
	dir := t.TempDir()
	data := filepath.Join(dir, "data")
	script := `nohup sleep 602 >/dev/null 2>&1 &
echo $! > "$D/nohup.pid"
sleep 600`
	s, err := pty.Start(pty.Options{Argv: []string{"/bin/sh", "-c", script}, Dir: dir, Env: append(ptyEnv(dir), "D="+dir), Grace: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.KillSession(); _ = s.Close() })
	// No reader here: a pending Read would hold the master open and Close would not hang up the terminal.
	child := readPidFile(t, filepath.Join(dir, "nohup.pid"))
	t.Cleanup(func() {
		if alive(child) {
			_ = syscall.Kill(child, syscall.SIGKILL) // the one PID this test's script recorded
		}
	})

	tr1 := New(Options{Source: NewSource(), DataDir: data})
	tr1.Track("crash", s)
	waitFor(t, "the nohup child in the set", 10*time.Second, func() bool {
		_ = tr1.Tick()
		for _, p := range tr1.Leftovers("crash").Procs {
			if p.PID == child {
				return true
			}
		}
		return false
	})
	st, err := os.Stat(filepath.Join(data, "sessions", "leftovers.json"))
	if err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("leftovers.json: %v %v", st, err)
	}

	// The "crash": our end of the terminal closes; the kernel hangs up the foreground job only.
	_ = s.Close()
	waitFor(t, "the shell to end", 10*time.Second, func() bool {
		select {
		case <-s.Done():
			return true
		default:
			return false
		}
	})
	if !alive(child) {
		t.Skip("this system also ended the nohup child on hangup")
	}

	tr2 := New(Options{Source: NewSource(), DataDir: data, Grace: 2 * time.Second, Hard: 3 * time.Second, Poll: 20 * time.Millisecond})
	n, err := tr2.Restore()
	if err != nil || n < 1 {
		t.Fatalf("restore: %d %v", n, err)
	}
	r := tr2.Leftovers("crash")
	found := false
	for _, p := range r.Procs {
		found = found || p.PID == child
	}
	if r.Live || !found {
		t.Fatalf("restored %+v, want pid %d", r, child)
	}
	if _, err := tr2.ForceStop("crash"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the nohup child to end", 5*time.Second, func() bool { return !alive(child) })
}
