package taskstate

import (
	"bytes"
	"encoding/json"
	"io"
	"sync"
	"time"
)

// Sink receives every accepted report. *Tracker is one.
type Sink interface{ Apply(Report) bool }

// Server accepts reports from report mode. One short-lived connection carries one JSON line.
//
// Nothing here logs or returns the text of a message: a problem is reported to onProblem as one of the fixed
// words below, because a payload may hold a prompt.
type Server struct {
	l        listener
	endpoint string

	mu     sync.Mutex
	closed bool
	wg     sync.WaitGroup
}

// Problem words handed to the Serve callback. They never carry message text.
const (
	ProblemTooLarge = "message too large"
	ProblemBadJSON  = "message is not valid"
	ProblemRejected = "message rejected"
)

func newServer(l listener, endpoint string) *Server { return &Server{l: l, endpoint: endpoint} }

// Endpoint is where the server listens; the launcher puts it in EnvSocket.
func (s *Server) Endpoint() string { return s.endpoint }

// Serve accepts connections in the background and feeds the sink. onProblem may be nil.
func (s *Server) Serve(sink Sink, onProblem func(string)) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		for {
			c, err := s.l.Accept()
			if err != nil {
				s.mu.Lock()
				closed := s.closed
				s.mu.Unlock()
				if closed {
					return
				}
				time.Sleep(10 * time.Millisecond) // a transient accept error must not spin
				continue
			}
			s.wg.Add(1)
			go func() {
				defer s.wg.Done()
				handle(c, sink, onProblem)
			}()
		}
	}()
}

// Close stops the server and waits for the handlers.
func (s *Server) Close() error {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	err := s.l.Close()
	s.wg.Wait()
	return err
}

func handle(c io.ReadCloser, sink Sink, problem func(string)) {
	t := time.AfterFunc(readTimeout, func() { c.Close() }) // a silent client cannot hold a handler
	defer t.Stop()
	defer c.Close()
	buf, err := io.ReadAll(io.LimitReader(c, maxWireLen+1))
	if len(buf) > maxWireLen {
		tell(problem, ProblemTooLarge)
		return
	}
	_ = err // a cut connection with a complete line is still usable
	line := bytes.TrimSpace(buf)
	if len(line) == 0 {
		return
	}
	var r Report
	if json.Unmarshal(line, &r) != nil {
		tell(problem, ProblemBadJSON)
		return
	}
	n, ok := Normalize(r, true)
	if !ok {
		tell(problem, ProblemRejected)
		return
	}
	sink.Apply(n)
}

func tell(f func(string), s string) {
	if f != nil {
		f(s)
	}
}
