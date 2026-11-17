//go:build windows

package pty

// The ConPTY wrapper is derived from github.com/aymanbagabas/go-pty (MIT, Copyright (c) 2023 Ayman Bagabas;
// see THIRD_PARTY_NOTICES.md). go-pty itself is not imported. What this adds:
//   - the child is created inside a Job Object (KILL_ON_JOB_CLOSE) through PROC_THREAD_ATTRIBUTE_JOB_LIST in
//     the same attribute list as the pseudo console, so there is no window between CreateProcess and
//     AssignProcessToJobObject in which a grandchild can escape, and every descendant dies when the app
//     (the last holder of the job handle) dies, however it dies;
//   - no SSH or u-root dependency tree;
//   - ClosePseudoConsole runs off the caller's goroutine: on older Windows 10 builds it blocks until the
//     output pipe is drained;
//   - the pseudo console is closed only when the job is empty, not when the root exits: closing it sends
//     CTRL_CLOSE_EVENT to every client still attached, which would end a dev server the tool started. This
//     matches macOS, where output ends once every holder of the terminal is gone.
//
// This has not run on a real Windows machine yet (owner's check B8).

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/rudian/codepass/internal/shellguard"
)

const procThreadAttributeJobList = 0x0002000D // not defined in x/sys/windows

type winSession struct {
	hpc       windows.Handle
	in, out   *os.File // our ends: in writes keystrokes, out reads output
	job       windows.Handle
	process   windows.Handle
	pid       uint32
	done      chan struct{}
	exit      int
	quit      chan struct{} // closed by Close: stops the job watcher
	closeMu   sync.Mutex    // guards job, hpc, closed and hpcClosed against use after free
	closed    bool          // Close ran: the job handle is closed
	hpcClosed bool          // ClosePseudoConsole was started: the HPCON must not be used again
}

// Start runs argv on a new pseudo console inside a new Job Object.
func Start(o Options) (Session, error) {
	if len(o.Argv) == 0 {
		return nil, errors.New("pty: empty argv")
	}
	if err := shellguard.CheckTerminal(o.Argv[0], o.Argv[1:], o.Env); err != nil {
		return nil, err // a test binary never starts the owner's login shell
	}
	argv := append([]string(nil), o.Argv...)
	// Resolve the program like os/exec: a failed search is an error (no CreateProcess search of the app's
	// folder or working directory), and a hit in the current directory is refused (ErrDot).
	app, err := exec.LookPath(argv[0])
	if err != nil {
		return nil, err
	}
	argv[0] = app
	cmdline := windows.ComposeCommandLine(argv)
	cols, rows := int16(o.Cols), int16(o.Rows)
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}

	ptyIn, inOurs, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	outOurs, ptyOut, err := os.Pipe()
	if err != nil {
		ptyIn.Close()
		inOurs.Close()
		return nil, err
	}
	var hpc windows.Handle
	err = windows.CreatePseudoConsole(windows.Coord{X: cols, Y: rows}, windows.Handle(ptyIn.Fd()), windows.Handle(ptyOut.Fd()), 0, &hpc)
	// The pseudo console owns its two ends now (or creation failed): drop ours either way.
	ptyIn.Close()
	ptyOut.Close()
	if err != nil {
		inOurs.Close()
		outOurs.Close()
		return nil, fmt.Errorf("CreatePseudoConsole: %w", err)
	}
	fail := func(err error, job windows.Handle) (Session, error) {
		if job != 0 {
			windows.CloseHandle(job)
		}
		go windows.ClosePseudoConsole(hpc)
		inOurs.Close()
		outOurs.Close()
		return nil, err
	}

	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return fail(err, 0)
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		return fail(err, job)
	}

	attrs, err := windows.NewProcThreadAttributeList(2)
	if err != nil {
		return fail(err, job)
	}
	defer attrs.Delete()
	if err := attrs.Update(windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE, *(*unsafe.Pointer)(unsafe.Pointer(&hpc)), unsafe.Sizeof(hpc)); err != nil {
		return fail(err, job)
	}
	if err := attrs.Update(procThreadAttributeJobList, unsafe.Pointer(&job), unsafe.Sizeof(job)); err != nil {
		return fail(err, job)
	}

	si := windows.StartupInfoEx{ProcThreadAttributeList: attrs.List()}
	si.Cb = uint32(unsafe.Sizeof(si))
	appName, err := windows.UTF16PtrFromString(app)
	if err != nil {
		return fail(err, job)
	}
	cl, err := windows.UTF16PtrFromString(cmdline)
	if err != nil {
		return fail(err, job)
	}
	var dirp *uint16
	if o.Dir != "" {
		if dirp, err = windows.UTF16PtrFromString(o.Dir); err != nil {
			return fail(err, job)
		}
	}
	// STARTF_USESTDHANDLES with all three handles left zero: the child's standard handles become NULL, so it
	// binds to the pseudo console. Without the flag Windows duplicates the app's own standard handles into
	// the child, and when the app's stdio is redirected (go test, "wails3 dev") the tool would read and write
	// those pipes instead of the console. Same as go-pty, Windows Terminal and WezTerm.
	si.Flags |= windows.STARTF_USESTDHANDLES
	var envp *uint16
	if o.Env != nil {
		envp = &envBlock(addCriticalEnv(dedupEnv(o.Env)))[0]
	}
	var pi windows.ProcessInformation
	flags := uint32(windows.EXTENDED_STARTUPINFO_PRESENT | windows.CREATE_UNICODE_ENVIRONMENT)
	if err := windows.CreateProcess(appName, cl, nil, nil, false, flags, envp, dirp, &si.StartupInfo, &pi); err != nil {
		return fail(fmt.Errorf("CreateProcess: %w", err), job)
	}
	_ = windows.CloseHandle(pi.Thread)

	s := &winSession{hpc: hpc, in: inOurs, out: outOurs, job: job, process: pi.Process, pid: pi.ProcessId, done: make(chan struct{}), quit: make(chan struct{}), exit: -1}
	go s.wait()
	return s, nil
}

// wait reaps the root process. Once the job is empty (the root and everything it started are gone) the
// pseudo console is closed, off this goroutine, which flushes the last output and lets Read return io.EOF.
func (s *winSession) wait() {
	if ev, err := windows.WaitForSingleObject(s.process, windows.INFINITE); err == nil && ev == windows.WAIT_OBJECT_0 {
		var code uint32
		if windows.GetExitCodeProcess(s.process, &code) == nil {
			s.exit = int(code)
		}
	}
	close(s.done)
	for {
		pids, err := s.Members()
		if err != nil || len(pids) == 0 {
			break
		}
		select {
		case <-s.quit:
		case <-time.After(100 * time.Millisecond):
			continue
		}
		break
	}
	s.closePseudoConsole()
}

// closePseudoConsole closes the HPCON once. The flag is set under closeMu before the close starts, so Resize
// never touches a freed console (ResizePseudoConsole dereferences the HPCON).
func (s *winSession) closePseudoConsole() {
	s.closeMu.Lock()
	defer s.closeMu.Unlock()
	s.closeHPCLocked()
}

func (s *winSession) closeHPCLocked() {
	if s.hpcClosed {
		return
	}
	s.hpcClosed = true
	go windows.ClosePseudoConsole(s.hpc) // may block until the output pipe is drained
}

func (s *winSession) Read(p []byte) (int, error) {
	n, err := s.out.Read(p)
	if err != nil && n == 0 && (errors.Is(err, os.ErrClosed) || errors.Is(err, windows.ERROR_BROKEN_PIPE)) {
		return 0, io.EOF
	}
	return n, err
}

func (s *winSession) Write(p []byte) (int, error) {
	n, err := s.in.Write(p)
	if errors.Is(err, os.ErrClosed) {
		err = ErrClosed
	}
	return n, err
}

func (s *winSession) Resize(cols, rows uint16) error {
	s.closeMu.Lock()
	defer s.closeMu.Unlock()
	if s.closed || s.hpcClosed {
		return ErrClosed
	}
	return windows.ResizePseudoConsole(s.hpc, windows.Coord{X: int16(cols), Y: int16(rows)})
}

func (s *winSession) Pid() int              { return int(s.pid) }
func (s *winSession) Done() <-chan struct{} { return s.done }

func (s *winSession) ExitCode() int {
	select {
	case <-s.done:
		return s.exit
	default:
		return -1
	}
}

// jobPidList is JOBOBJECT_BASIC_PROCESS_ID_LIST with room for 1024 ids.
type jobPidList struct {
	assigned uint32
	listed   uint32
	ids      [1024]uintptr
}

// queryJobLocked reads the job's process list. The caller holds closeMu and has checked closed.
func (s *winSession) queryJobLocked() ([]int, error) {
	var l jobPidList
	err := windows.QueryInformationJobObject(s.job, windows.JobObjectBasicProcessIdList, uintptr(unsafe.Pointer(&l)), uint32(unsafe.Sizeof(l)), nil)
	if err != nil && !errors.Is(err, windows.ERROR_MORE_DATA) { // more than 1024: the first 1024 are filled
		return nil, err
	}
	out := make([]int, 0, l.listed)
	for i := uint32(0); i < l.listed && i < uint32(len(l.ids)); i++ {
		out = append(out, int(l.ids[i]))
	}
	if len(out) == 0 && l.assigned > 0 { // cannot happen with a 1024 buffer, but never report "empty" wrongly
		out = append(out, 0)
	}
	return out, nil
}

// Members lists the processes in the job (empty once the session has ended or Close ran).
func (s *winSession) Members() ([]int, error) {
	s.closeMu.Lock()
	defer s.closeMu.Unlock()
	if s.closed {
		return nil, nil
	}
	return s.queryJobLocked()
}

// Foreground has no Windows counterpart; use Members.
func (s *winSession) Foreground() (int, error) { return 0, errors.ErrUnsupported }

// KillSession ends every process in the job. TerminateJobObject has no polite step and no grace period, and
// it is asynchronous, so this waits (up to 5 s) until the job reports no member.
func (s *winSession) KillSession() error {
	s.closeMu.Lock()
	if s.closed {
		s.closeMu.Unlock()
		return nil // the job handle is closed, which already killed every member
	}
	err := windows.TerminateJobObject(s.job, 1) // under the lock: Close cannot free the handle meanwhile
	s.closeMu.Unlock()
	if err != nil {
		return err
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		pids, err := s.Members() // nil once Close ran, which killed everything
		if err != nil {
			return err
		}
		if len(pids) == 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("pty: %d process(es) still in the job after TerminateJobObject", len(pids))
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// Close kills the tree (closing the last job handle does it, KILL_ON_JOB_CLOSE) and releases the pseudo
// console. The reader must keep reading until Read returns an error, or ClosePseudoConsole can block on
// older Windows 10 builds; that block is off this goroutine.
func (s *winSession) Close() error {
	s.closeMu.Lock()
	defer s.closeMu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	close(s.quit)
	_ = windows.TerminateJobObject(s.job, 1)
	s.closeHPCLocked()
	_ = s.in.Close()
	err := windows.CloseHandle(s.job)
	go func() { // the root has exited by now; the handle is only closed after that so Wait can finish
		<-s.done
		windows.CloseHandle(s.process)
		s.out.Close()
	}()
	return err
}

// dedupEnv keeps the last of any variables whose names are equal ignoring case (Windows variable names are
// case-insensitive: "Path" and "PATH" are one variable) and drops NUL bytes.
func dedupEnv(env []string) []string {
	out := make([]string, 0, len(env))
	seen := map[string]bool{}
	for i := len(env) - 1; i >= 0; i-- {
		kv := strings.ReplaceAll(env[i], "\x00", "")
		if kv == "" {
			continue
		}
		eq := strings.Index(kv, "=")
		if eq == 0 { // "=C:=C:\dir": the name starts after the first character
			eq = strings.Index(kv[1:], "=") + 1
			if eq == 0 {
				eq = -1
			}
		}
		if eq < 0 {
			out = append(out, kv)
			continue
		}
		key := strings.ToUpper(kv[:eq])
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, kv)
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

// addCriticalEnv adds SYSTEMROOT when missing: without it many programs fail to start (os/exec does the same).
func addCriticalEnv(env []string) []string {
	for _, kv := range env {
		if len(kv) >= 11 && strings.EqualFold(kv[:11], "SYSTEMROOT=") {
			return env
		}
	}
	if v := os.Getenv("SYSTEMROOT"); v != "" {
		return append(env, "SYSTEMROOT="+v)
	}
	return env
}

// envBlock builds the UTF-16 environment block: each entry NUL-terminated, then one more NUL. An empty block
// is two NULs.
func envBlock(env []string) []uint16 {
	if len(env) == 0 {
		return []uint16{0, 0}
	}
	var u []uint16
	for _, kv := range env {
		u = append(u, windows.StringToUTF16(kv)...)
	}
	return append(u, 0)
}
