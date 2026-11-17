package terminal

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/pty"
)

// maxPendingInput bounds the keystrokes waiting for the child to read them (a paste into a child that is not
// reading must not pile up without end, nor block the caller).
const maxPendingInput = 8 << 20

// Session is one child on a terminal. Its reader goroutine does three things only: read from the terminal,
// append to the log (a short lock), and a non-blocking send on the ping channel. Nothing in that path can wait
// for the page, the event system or the disk.
type Session struct {
	id  string
	p   pty.Session
	log *Log

	// ping is a coalescing notification: capacity 1, the reader uses a non-blocking send.
	ping  chan struct{}
	ended chan struct{} // closed when the terminal is finished (end of output) or the session was forced down
	once  sync.Once

	forced    atomic.Bool // set by Stop when the terminal did not end in time: later output is dropped
	totalRead atomic.Int64
	maxAppend atomic.Int64 // longest append plus notify (ns)

	inMu    sync.Mutex
	inQ     [][]byte
	inBytes int
	inSig   chan struct{}
}

func newSession(id string, p pty.Session, log *Log) *Session {
	s := &Session{id: id, p: p, log: log, ping: make(chan struct{}, 1), ended: make(chan struct{}), inSig: make(chan struct{}, 1)}
	go s.readLoop()
	go s.writeLoop()
	return s
}

func (s *Session) finish() { s.once.Do(func() { close(s.ended) }) }

// readLoop is the only reader of the terminal. It keeps draining until the terminal reports the end.
func (s *Session) readLoop() {
	defer s.finish()
	buf := make([]byte, 32<<10)
	for {
		n, err := s.p.Read(buf)
		if n > 0 && !s.forced.Load() {
			t0 := time.Now()
			s.log.Append(buf[:n])
			s.totalRead.Add(int64(n))
			select {
			case s.ping <- struct{}{}:
			default:
			}
			if d := int64(time.Since(t0)); d > s.maxAppend.Load() {
				s.maxAppend.Store(d)
			}
		}
		if err != nil { // io.EOF is the normal end; any other error ends the session the same way
			return
		}
	}
}

// Write queues keystrokes for the child and returns at once; a goroutine delivers them in order. Input to
// an ended session is dropped without error.
func (s *Session) Write(b []byte) error {
	if len(b) == 0 {
		return nil
	}
	select {
	case <-s.ended:
		return nil
	default:
	}
	s.inMu.Lock()
	if s.inBytes+len(b) > maxPendingInput {
		s.inMu.Unlock()
		return api.NewError(api.ErrInternal, "the terminal is not reading its input")
	}
	s.inQ = append(s.inQ, append([]byte(nil), b...))
	s.inBytes += len(b)
	s.inMu.Unlock()
	select {
	case s.inSig <- struct{}{}:
	default:
	}
	return nil
}

func (s *Session) writeLoop() {
	for {
		select {
		case <-s.inSig:
		case <-s.ended:
			return
		}
		for {
			s.inMu.Lock()
			if len(s.inQ) == 0 {
				s.inMu.Unlock()
				break
			}
			b := s.inQ[0]
			s.inQ = s.inQ[1:]
			s.inBytes -= len(b)
			s.inMu.Unlock()
			if _, err := s.p.Write(b); err != nil {
				s.inMu.Lock()
				s.inQ, s.inBytes = nil, 0
				s.inMu.Unlock()
				break
			}
		}
	}
}

// Resize changes the terminal size. A session whose console is already closed is not an error.
func (s *Session) Resize(cols, rows uint16) error {
	if err := s.p.Resize(cols, rows); err != nil && !errors.Is(err, pty.ErrClosed) {
		return err
	}
	return nil
}

// Ended reports whether the terminal is finished.
func (s *Session) Ended() bool {
	select {
	case <-s.ended:
		return true
	default:
		return false
	}
}

// ID is the session's name.
func (s *Session) ID() string { return s.id }

// Log is the session's byte log.
func (s *Session) Log() *Log { return s.log }

// PTY is the process layer, for the process tracker and the state fallback.
func (s *Session) PTY() pty.Session { return s.p }
