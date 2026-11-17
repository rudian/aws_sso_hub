//go:build !windows

package pty

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	cpty "github.com/creack/pty"
	"golang.org/x/sys/unix"

	"github.com/rudian/codepass/internal/shellguard"
)

type unixSession struct {
	cmd       *exec.Cmd
	f         *os.File
	pid       int // also the session ID: the child calls setsid before exec
	k         *killer
	done      chan struct{}
	exit      int
	closeOnce sync.Once
	mu        sync.RWMutex // Resize and Foreground take RLock, Close takes Lock: no ioctl on a closed or reused fd
	closed    bool
}

// Start runs argv on a new pseudo terminal. The child leads a new terminal session and has the terminal as
// its controlling terminal.
func Start(o Options) (Session, error) {
	if len(o.Argv) == 0 {
		return nil, errors.New("pty: empty argv")
	}
	c := exec.Command(o.Argv[0], o.Argv[1:]...)
	c.Dir = o.Dir
	c.Env = o.Env
	if c.Env == nil {
		c.Env = os.Environ()
	}
	if err := shellguard.CheckTerminal(o.Argv[0], o.Argv[1:], c.Env); err != nil {
		return nil, err // a test binary never starts the owner's login shell
	}
	cols, rows := o.Cols, o.Rows
	if cols == 0 {
		cols = 80
	}
	if rows == 0 {
		rows = 24
	}
	f, err := cpty.StartWithSize(c, &cpty.Winsize{Cols: cols, Rows: rows}) // Setsid + Setctty
	if err != nil {
		return nil, err
	}
	grace := o.Grace
	if grace <= 0 {
		grace = DefaultGrace
	}
	s := &unixSession{cmd: c, f: f, pid: c.Process.Pid, k: newKiller(c.Process.Pid, grace), done: make(chan struct{}), exit: -1}
	go func() { // the only Wait: reaps the root child
		err := c.Wait()
		code := 0
		if err != nil {
			var ee *exec.ExitError
			if errors.As(err, &ee) {
				code = ee.ExitCode()
				if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
					code = 128 + int(ws.Signal())
				}
			} else {
				code = -1
			}
		}
		s.exit = code
		close(s.done)
		// The leader's number can be handed to a new process once the session is empty. Notice that moment
		// early and latch it, so a late KillSession never reaches a stranger's session.
		s.k.watch(s.k.pollEvery)
	}()
	return s, nil
}

func (s *unixSession) Read(p []byte) (int, error) {
	n, err := s.f.Read(p)
	if err != nil && n == 0 && isEnd(err) {
		return 0, io.EOF
	}
	return n, err
}

// isEnd reports the normal end of a pty master: EIO once every slave end is closed (macOS and Linux), or a
// read on a file that Close already closed.
func isEnd(err error) bool {
	return errors.Is(err, syscall.EIO) || errors.Is(err, os.ErrClosed) || errors.Is(err, io.EOF)
}

func (s *unixSession) Write(p []byte) (int, error) {
	n, err := s.f.Write(p)
	if errors.Is(err, os.ErrClosed) {
		err = ErrClosed
	}
	return n, err
}

func (s *unixSession) Resize(cols, rows uint16) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return ErrClosed
	}
	err := cpty.Setsize(s.f, &cpty.Winsize{Cols: cols, Rows: rows})
	if errors.Is(err, os.ErrClosed) {
		err = ErrClosed
	}
	return err
}

// Foreground returns the terminal's foreground process group (TIOCGPGRP on the master).
func (s *unixSession) Foreground() (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return 0, ErrClosed
	}
	rc, err := s.f.SyscallConn()
	if err != nil {
		return 0, err
	}
	var pg int
	var ierr error
	if err := rc.Control(func(fd uintptr) { pg, ierr = unix.IoctlGetInt(int(fd), unix.TIOCGPGRP) }); err != nil {
		return 0, ErrClosed
	}
	return pg, ierr
}

func (s *unixSession) Members() ([]int, error) { return s.k.members() }

func (s *unixSession) Pid() int              { return s.pid }
func (s *unixSession) Done() <-chan struct{} { return s.done }

func (s *unixSession) ExitCode() int {
	select {
	case <-s.done:
		return s.exit
	default:
		return -1
	}
}

func (s *unixSession) Close() error {
	var err error
	s.closeOnce.Do(func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.closed = true
		err = s.f.Close()
	})
	return err
}

// KillSession: see killer.kill.
func (s *unixSession) KillSession() error { return s.k.kill() }

// killer stops one terminal session. sid is the leader's pid: the leader called setsid, so every member has
// getsid == sid, even after the leader itself is gone (the kernel keeps the number reserved while the session
// has members). Once the session is empty the number is free for reuse, so the killer remembers that it has
// seen the session empty (ended) and never acts again, and it also refuses when the process that now holds the
// number has a different start time than the leader had.
type killer struct {
	sid       int
	grace     time.Duration
	pollEvery time.Duration
	hardWait  time.Duration // how long the SIGKILL step repeats
	confirm   time.Duration // gap before a second listing confirms an empty session

	// Injectable for tests.
	list   func() ([]int, error) // live processes of the system
	getsid func(pid int) (int, error)
	signal func(pid int, sig syscall.Signal) error // pid < 0 is a process group
	pgid   func(pid int) (int, error)
	start  func(pid int) (int64, bool)

	startAt   int64
	haveStart bool

	mu    sync.Mutex // guards ended and every process listing; never held across a wait (FU-01a)
	ended bool

	killMu sync.Mutex // serializes kill(): one stop at a time, and Members() never waits for it
}

func newKiller(sid int, grace time.Duration) *killer {
	k := &killer{
		sid: sid, grace: grace, pollEvery: 2 * time.Second, hardWait: 5 * time.Second, confirm: 50 * time.Millisecond,
		list:   listPIDs,
		getsid: unix.Getsid,
		signal: func(pid int, sig syscall.Signal) error { return syscall.Kill(pid, sig) },
		pgid:   unix.Getpgid,
		start:  startTime,
	}
	k.startAt, k.haveStart = k.start(sid)
	return k
}

// raw lists the members of the session. The caller holds k.mu. vanished is true when a pid of the snapshot
// could not be asked for its session (it exited during the pass): a listing is not atomic, so an empty
// result with vanished set may have missed a child that the vanished process forked.
func (k *killer) raw() (out []int, vanished bool, err error) {
	pids, err := k.list()
	if err != nil {
		return nil, false, err
	}
	self := os.Getpid()
	for _, pid := range pids {
		if pid == self {
			continue
		}
		got, err := k.getsid(pid)
		if err != nil {
			vanished = true
			continue
		}
		if got == k.sid {
			out = append(out, pid)
		}
	}
	return out, vanished, nil
}

// listOnce lists members and latches ended only on a confirmed empty session: a pass in which a process
// vanished is inconclusive and is repeated (a few tries), and a conclusive empty pass must be confirmed by a
// second listing k.confirm later. The caller holds k.mu.
func (k *killer) listOnce() ([]int, error) {
	const tries = 4
	for i := 0; i < tries; i++ {
		m, vanished, err := k.raw()
		if err != nil {
			return nil, err
		}
		if len(m) > 0 {
			return m, nil
		}
		if vanished {
			if i < tries-1 {
				time.Sleep(k.confirm / 5)
			}
			continue
		}
		// conclusive empty: confirm once more after a pause
		time.Sleep(k.confirm)
		m2, vanished2, err := k.raw()
		if err != nil {
			return nil, err
		}
		if len(m2) > 0 {
			return m2, nil
		}
		if vanished2 {
			continue
		}
		k.ended = true
		return nil, nil
	}
	return nil, nil // inconclusive: report empty for now, but do not latch
}

func (k *killer) members() ([]int, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.ended {
		return nil, nil
	}
	return k.listOnce()
}

// watch polls the session at a low rate until it is empty, then latches ended. Started once the leader has
// been reaped: from then on its number could be reused by an unrelated process.
func (k *killer) watch(every time.Duration) {
	for {
		k.mu.Lock()
		if k.ended {
			k.mu.Unlock()
			return
		}
		_, _ = k.listOnce() // an error leaves the state as it was; try again next round
		ended := k.ended
		k.mu.Unlock()
		if ended {
			return
		}
		time.Sleep(every)
	}
}

// identityChanged is true when a live process holds the leader's number with a different start time: the
// session this killer belongs to is over and the number now belongs to someone else. The caller holds k.mu.
func (k *killer) identityChanged() bool {
	if !k.haveStart {
		return false
	}
	st, ok := k.start(k.sid)
	return ok && st != k.startAt
}

// kill signals every process whose terminal session is this one: SIGTERM (and SIGCONT, for jobs that are
// stopped) to each process group they lead, a wait of the grace period, then SIGKILL until none is left. A
// process that calls setsid itself has left the session and is not touched (A2-13 tracks those).
//
// kill holds killMu for the whole stop, but k.mu only around the ended check and each single listing, so a
// concurrent members() waits for one listing at most, never for the grace period (FU-01a).
func (k *killer) kill() error {
	k.killMu.Lock()
	defer k.killMu.Unlock()
	k.mu.Lock()
	ended := k.ended
	k.mu.Unlock()
	if ended {
		return nil
	}
	if k.sid <= 1 {
		return fmt.Errorf("pty: refusing to stop session %d", k.sid)
	}
	if mine, err := unix.Getsid(0); err == nil && mine == k.sid {
		return fmt.Errorf("pty: refusing to stop the caller's own session")
	}
	k.mu.Lock()
	changed := k.identityChanged()
	if changed {
		k.ended = true // the number was reused: our session is long gone
	}
	k.mu.Unlock()
	if changed {
		return nil
	}
	list := func() ([]int, error) {
		k.mu.Lock()
		defer k.mu.Unlock()
		if k.ended { // a concurrent members() or the watcher saw the session empty: act no more
			return nil, nil
		}
		m, err := k.listOnce()
		if err != nil { // one retry
			time.Sleep(20 * time.Millisecond)
			m, err = k.listOnce()
		}
		return m, err
	}
	termed := map[int]bool{} // process groups that already got the polite signal
	deadline := time.Now().Add(k.grace)
	for {
		members, err := list()
		if err != nil {
			// The process table is unreadable: do not report success. Stop what is certainly ours, the
			// leader's own process group, and say so.
			_ = k.signal(-k.sid, syscall.SIGTERM)
			_ = k.signal(-k.sid, syscall.SIGCONT)
			return fmt.Errorf("pty: cannot list session %d: %w", k.sid, err)
		}
		if len(members) == 0 {
			return nil
		}
		if time.Now().After(deadline) {
			break
		}
		for _, pgid := range k.groupsOf(members) {
			if !termed[pgid] {
				termed[pgid] = true
				_ = k.signal(-pgid, syscall.SIGTERM)
				_ = k.signal(-pgid, syscall.SIGCONT)
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	// Forced step. New forks can appear between listing and signalling, so repeat until the session is empty.
	hard := time.Now().Add(k.hardWait)
	for time.Now().Before(hard) {
		members, err := list()
		if err != nil {
			_ = k.signal(-k.sid, syscall.SIGKILL)
			return fmt.Errorf("pty: cannot list session %d: %w", k.sid, err)
		}
		if len(members) == 0 {
			return nil
		}
		for _, pgid := range k.groupsOf(members) {
			_ = k.signal(-pgid, syscall.SIGKILL)
		}
		for _, pid := range members { // a member whose group has gone away (rare): signal it directly
			_ = k.signal(pid, syscall.SIGKILL)
		}
		time.Sleep(25 * time.Millisecond)
	}
	k.mu.Lock()
	var left []int
	if !k.ended {
		left, _ = k.listOnce()
	}
	k.mu.Unlock()
	if len(left) > 0 {
		return fmt.Errorf("pty: %d process(es) survived SIGKILL in session %d", len(left), k.sid)
	}
	return nil
}

// groupsOf returns the distinct process groups of members, never the caller's own group and never 0 or 1.
func (k *killer) groupsOf(members []int) []int {
	own := unix.Getpgrp()
	seen := map[int]bool{}
	var out []int
	for _, pid := range members {
		pg, err := k.pgid(pid)
		if err != nil || pg <= 1 || pg == own || seen[pg] {
			continue
		}
		seen[pg] = true
		out = append(out, pg)
	}
	return out
}
