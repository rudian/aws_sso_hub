package terminal

import (
	"io"
	"sync"
	"sync/atomic"

	"github.com/rudian/codepass/internal/pty"
)

// fakePTY is a pty.Session driven by the test: output is pushed with feed, input is collected.
type fakePTY struct {
	out       chan []byte
	closeOnce sync.Once
	done      chan struct{}

	mu       sync.Mutex
	in       []byte
	cols     uint16
	rows     uint16
	blockIn  chan struct{} // when non-nil, Write waits for it
	resizeEr error
	killed   atomic.Int32
	closed   atomic.Bool
}

func newFakePTY() *fakePTY { return &fakePTY{out: make(chan []byte, 1024), done: make(chan struct{})} }

func (f *fakePTY) feed(b string) { f.out <- []byte(b) }
func (f *fakePTY) eof()          { f.closeOnce.Do(func() { close(f.out); close(f.done) }) }

func (f *fakePTY) Read(p []byte) (int, error) {
	b, ok := <-f.out
	if !ok {
		return 0, io.EOF
	}
	return copy(p, b), nil
}
func (f *fakePTY) Write(p []byte) (int, error) {
	if f.blockIn != nil {
		<-f.blockIn
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.in = append(f.in, p...)
	return len(p), nil
}
func (f *fakePTY) Resize(c, r uint16) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cols, f.rows = c, r
	return f.resizeEr
}
func (f *fakePTY) Pid() int                 { return 4242 }
func (f *fakePTY) Done() <-chan struct{}    { return f.done }
func (f *fakePTY) ExitCode() int            { return 0 }
func (f *fakePTY) Foreground() (int, error) { return 0, pty.ErrClosed }
func (f *fakePTY) Members() ([]int, error)  { return nil, nil }
func (f *fakePTY) KillSession() error       { f.killed.Add(1); f.eof(); return nil }
func (f *fakePTY) Close() error             { f.closed.Store(true); return nil }
func (f *fakePTY) input() string            { f.mu.Lock(); defer f.mu.Unlock(); return string(f.in) }
func (f *fakePTY) size() (uint16, uint16)   { f.mu.Lock(); defer f.mu.Unlock(); return f.cols, f.rows }

type event struct {
	name string
	data any
}

// fakeManager returns a manager whose terminals are fakePTYs; the last one started is sent on the channel.
func fakeManager(cfg Config) (*Manager, chan *fakePTY) {
	ch := make(chan *fakePTY, 8)
	cfg.Start = func(pty.Options) (pty.Session, error) {
		f := newFakePTY()
		ch <- f
		return f, nil
	}
	return NewManager(cfg), ch
}
