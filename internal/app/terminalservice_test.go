package app

import (
	"encoding/base64"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/pty"
	"github.com/rudian/codepass/internal/terminal"
)

// termFake is a pty.Session driven by the test.
type termFake struct {
	out  chan []byte
	once sync.Once
	done chan struct{}
	mu   sync.Mutex
	in   []byte
	rerr error
}

func newTermFake() *termFake { return &termFake{out: make(chan []byte, 64), done: make(chan struct{})} }
func (f *termFake) Read(p []byte) (int, error) {
	b, ok := <-f.out
	if !ok {
		return 0, io.EOF
	}
	return copy(p, b), nil
}
func (f *termFake) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.in = append(f.in, p...)
	return len(p), nil
}
func (f *termFake) Resize(uint16, uint16) error { return f.rerr }
func (f *termFake) Pid() int                    { return 1 }
func (f *termFake) Done() <-chan struct{}       { return f.done }
func (f *termFake) ExitCode() int               { return 0 }
func (f *termFake) Foreground() (int, error)    { return 0, pty.ErrClosed }
func (f *termFake) Members() ([]int, error)     { return nil, nil }
func (f *termFake) KillSession() error          { f.end(); return nil }
func (f *termFake) Close() error                { return nil }
func (f *termFake) end()                        { f.once.Do(func() { close(f.out); close(f.done) }) }
func (f *termFake) input() string               { f.mu.Lock(); defer f.mu.Unlock(); return string(f.in) }

func termService(t *testing.T, logCap int) (*TerminalService, *terminal.Manager, *termFake) {
	t.Helper()
	f := newTermFake()
	m := terminal.NewManager(terminal.Config{
		LogCap: logCap,
		Start:  func(pty.Options) (pty.Session, error) { return f, nil },
	})
	if _, err := m.Start(terminal.StartOptions{ID: "t1", Argv: []string{"x"}}); err != nil {
		t.Fatal(err)
	}
	return NewTerminalService(m), m, f
}

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

func TestTerminalServiceReadEncodesAndPages(t *testing.T) {
	s, m, f := termService(t, 0)
	f.out <- []byte("hello world")
	f.end()
	for end := time.Now().Add(5 * time.Second); !m.StatusOf("t1").Ended && time.Now().Before(end); time.Sleep(5 * time.Millisecond) {
	}
	r, err := s.Read("t1", 0, 5)
	if err != nil || r.Data != b64("hello") || r.From != 0 || r.Next != 5 || r.Tail != 11 || r.Gap != 0 || r.Done || r.Prologue != "" {
		t.Fatalf("%+v %v", r, err)
	}
	r, _ = s.Read("t1", r.Next, 100)
	if r.Data != b64(" world") || !r.Done || r.Next != 11 {
		t.Fatalf("%+v", r)
	}
	if r, _ := s.Read("nobody", 0, 10); !r.Done || r.Data != "" {
		t.Fatalf("%+v", r)
	}
}

func TestTerminalServiceGapCarriesAPrologue(t *testing.T) {
	s, m, f := termService(t, 2000)
	f.out <- []byte("\x1b[?1049h")
	for i := 0; i < 200; i++ {
		f.out <- []byte("0123456789012345678901234567890123456789\r\n")
	}
	for end := time.Now().Add(5 * time.Second); m.StatusOf("t1").Tail < 8000 && time.Now().Before(end); time.Sleep(5 * time.Millisecond) {
	}
	// the log is one chunk here, so force a trim by feeding past the chunk size
	for i := 0; i < 40; i++ {
		f.out <- make([]byte, 8192)
	}
	for end := time.Now().Add(5 * time.Second); m.StatusOf("t1").Trims == 0 && time.Now().Before(end); time.Sleep(5 * time.Millisecond) {
	}
	r, err := s.Read("t1", 0, 1000)
	if err != nil || r.Gap == 0 || r.From != r.Gap {
		t.Fatalf("%+v %v", r, err)
	}
	pro, _ := base64.StdEncoding.DecodeString(r.Prologue)
	if !strings.Contains(string(pro), "\x1b[?1049h") || !strings.Contains(string(pro), "\x1b[?25h") {
		t.Fatalf("prologue %q", pro)
	}
}

func TestTerminalServiceWriteResizeValidate(t *testing.T) {
	s, _, f := termService(t, 0)
	if err := s.Write("t1", b64("ls\r")); err != nil {
		t.Fatal(err)
	}
	for end := time.Now().Add(5 * time.Second); f.input() == "" && time.Now().Before(end); time.Sleep(5 * time.Millisecond) {
	}
	if f.input() != "ls\r" {
		t.Fatalf("%q", f.input())
	}
	if err := s.Write("t1", "***not base64***"); err == nil {
		t.Fatal("bad base64 accepted")
	} else if ae, ok := err.(*api.AppError); !ok || ae.Code != api.ErrInvalidInput {
		t.Fatalf("%T %v", err, err)
	}
	if err := s.Write("nobody", b64("x")); err != nil {
		t.Fatal(err)
	}
	f.rerr = pty.ErrClosed
	if err := s.Resize("t1", 100, 40); err != nil {
		t.Fatalf("a closed console is not an error: %v", err)
	}
	if err := s.Resize("t1", -1, 40); err == nil {
		t.Fatal("negative columns")
	}
	if err := s.ServiceShutdown(); err != nil {
		t.Fatal(err)
	}
}
