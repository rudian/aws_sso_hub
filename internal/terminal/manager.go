package terminal

import (
	"os"
	"regexp"
	"sync"
	"time"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/netx"
	"github.com/rudian/codepass/internal/pty"
)

const (
	defaultReadMax       = 256 << 10
	hardReadMax          = 1 << 20
	defaultScrollbackCap = 4 << 20
	defaultPingEvery     = 15 * time.Millisecond
	forceEndAfter        = 3 * time.Second
	maxDim               = 1000
)

var idRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

// Config is what a Manager needs from the app. Zero values are sensible defaults.
type Config struct {
	// DataDir is the app's data folder; an ended session's scrollback goes to <DataDir>/sessions/<id>/scrollback.
	// Empty turns scrollback files off.
	DataDir string
	// Emit sends an event to the page. It may block (Wails parks emitters); only a session's pinger waits on it.
	Emit func(name string, data any)
	// LogCap is the byte-log cap per session (default 16 MB). ScrollbackCap is the cap of the file (default 4 MB).
	LogCap, ScrollbackCap int
	// ScrollbackEnabled is the setting "keep the last output of ended sessions" (nil = on).
	ScrollbackEnabled func() bool
	// BaseEnv is the environment children inherit from (default os.Environ). ExtraEnv is added to every child
	// (the sandbox's variables in a development build).
	BaseEnv  func() []string
	ExtraEnv []string
	// ProxyEnv returns the proxy variables of the proxy the app detected (netx Config.ChildEnv), evaluated at
	// each Start. When it returns any, the proxy variables of BaseEnv are dropped and these replace them: an app
	// started from Finder, the Dock or the Start menu has none of its own. Nil or empty leaves the inherited ones.
	ProxyEnv func() []string
	// Start creates the terminal (default pty.Start; tests replace it).
	Start func(pty.Options) (pty.Session, error)
	// Grace is the wait between the polite stop and the forced kill (0 = pty.DefaultGrace).
	Grace time.Duration
	// PingEvery is the shortest time between two pings of a session (default 15 ms).
	PingEvery time.Duration
	// LookPath resolves a program for the batch-file check (default exec.LookPath).
	LookPath func(string) (string, error)
	// GOOS "" means the running OS (environment rules).
	GOOS string
}

// StartOptions describes one session.
type StartOptions struct {
	ID   string // lowercase letters, digits, "_", "-" (no case or dot variants of one folder on the common file systems): it names the scrollback folder (the task ID)
	Argv []string
	Dir  string
	// Env adds to the explicit child environment (for example CODEPASS_TASK, an account's CLAUDE_CONFIG_DIR).
	Env        []string
	Cols, Rows uint16
}

// Manager owns the sessions. It is the Go-side API of the terminal; the bound TerminalService is a thin layer
// over Read, Write and Resize.
type Manager struct {
	cfg Config

	mu       sync.Mutex
	sessions map[string]*Session
	archives map[string]*Log
	// archOrder lists the loaded archives, least recently read first (at most maxArchives).
	archOrder []string
	// starting holds the IDs whose Start is in flight, so two Starts cannot both pass the check.
	starting map[string]bool
	// tails is the end offset of each released session, so a reused ID continues its offsets.
	tails map[string]int64
}

// NewManager makes a manager.
func NewManager(cfg Config) *Manager {
	if cfg.LogCap <= 0 {
		cfg.LogCap = DefaultLogCap
	}
	if cfg.ScrollbackCap <= 0 {
		cfg.ScrollbackCap = defaultScrollbackCap
	}
	if cfg.Start == nil {
		cfg.Start = pty.Start
	}
	if cfg.BaseEnv == nil {
		cfg.BaseEnv = os.Environ
	}
	if cfg.PingEvery <= 0 {
		cfg.PingEvery = defaultPingEvery
	}
	return &Manager{cfg: cfg, sessions: map[string]*Session{}, archives: map[string]*Log{}, starting: map[string]bool{}, tails: map[string]int64{}}
}

func invalid(field string) error {
	return api.NewError(api.ErrInvalidInput, "invalid %s", field).WithParam("field", field)
}

// ValidID reports whether id can name a session.
func ValidID(id string) bool { return idRe.MatchString(id) }

// Start launches a session. It returns at once; output is read in the background.
func (m *Manager) Start(o StartOptions) (*Session, error) {
	if !ValidID(o.ID) {
		return nil, invalid("session")
	}
	if len(o.Argv) == 0 || o.Argv[0] == "" {
		return nil, invalid("argv")
	}
	if err := CheckProgram(m.cfg.GOOS, o.Argv[0], m.cfg.LookPath); err != nil {
		return nil, err
	}
	dir := o.Dir
	if dir == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			return nil, api.NewError(api.ErrInternal, "cannot determine the home directory").WithDetail(err.Error())
		}
		dir = h
	}
	cols, rows := o.Cols, o.Rows
	if cols == 0 {
		cols = 120
	}
	if rows == 0 {
		rows = 30
	}
	m.mu.Lock()
	if old := m.sessions[o.ID]; (old != nil && !old.Ended()) || m.starting[o.ID] {
		m.mu.Unlock()
		return nil, api.NewError(api.ErrInvalidInput, "session %s is already running", o.ID).WithParam("field", "session")
	}
	m.starting[o.ID] = true
	// offsets of an ID keep growing across runs: the new log begins where the previous one (live, released or
	// archived) ended, so a page holding an older offset gets a gap and never other bytes
	tail := m.tails[o.ID]
	if old := m.sessions[o.ID]; old != nil {
		tail = max(tail, old.log.Tail())
	}
	m.dropArchiveLocked(o.ID)
	m.mu.Unlock()
	if tail == 0 {
		tail = archiveTail(ScrollbackPath(m.cfg.DataDir, o.ID))
	}
	reserved := true
	defer func() {
		if reserved {
			m.mu.Lock()
			delete(m.starting, o.ID)
			m.mu.Unlock()
		}
	}()

	base := m.cfg.BaseEnv()
	extra := append([]string(nil), m.cfg.ExtraEnv...)
	if m.cfg.ProxyEnv != nil {
		if pe := m.cfg.ProxyEnv(); len(pe) > 0 {
			base = netx.StripProxyEnv(base)
			extra = append(extra, pe...)
		}
	}
	env := BuildEnv(base, m.cfg.GOOS, append(extra, o.Env...))
	p, err := m.cfg.Start(pty.Options{Argv: o.Argv, Dir: dir, Env: env, Cols: cols, Rows: rows, Grace: m.cfg.Grace})
	if err != nil {
		return nil, api.NewError(api.ErrInternal, "cannot start the terminal").WithDetail(err.Error())
	}
	s := newSession(o.ID, p, newLogAt(m.cfg.LogCap, chunkSize, tail, defaultModes()))
	m.mu.Lock()
	m.sessions[o.ID] = s
	delete(m.starting, o.ID)
	m.mu.Unlock()
	reserved = false
	go m.pinger(s)
	go m.after(s)
	return s, nil
}

// pinger turns the reader's coalesced notification into a tiny event. It is its own goroutine: if Emit blocks
// (Wails parks emitters once 64 events are queued), only this goroutine waits, never the reader.
func (m *Manager) pinger(s *Session) {
	t := time.NewTimer(time.Hour)
	t.Stop()
	for {
		select {
		case <-s.ping:
			m.emit(api.TerminalPing{Session: s.id, Tail: s.log.Tail()})
			t.Reset(m.cfg.PingEvery)
			select {
			case <-t.C:
			case <-s.ended:
				t.Stop()
			}
		case <-s.ended:
			m.emit(api.TerminalPing{Session: s.id, Tail: s.log.Tail(), Done: true})
			return
		}
	}
}

func (m *Manager) emit(p api.TerminalPing) {
	if m.cfg.Emit != nil {
		m.cfg.Emit(api.EventTerminalPing, p)
	}
}

// after runs when the terminal is finished: it releases the console and writes the scrollback.
func (m *Manager) after(s *Session) {
	<-s.ended
	_ = s.p.Close()
	m.saveScrollback(s)
}

func (m *Manager) get(id string) *Session {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sessions[id]
}

// Session returns a session by name.
func (m *Manager) Session(id string) (*Session, bool) {
	s := m.get(id)
	return s, s != nil
}

// ReadReply is Read's answer before encoding.
type ReadReply struct {
	Result
	Done bool
}

// Read returns bytes from offset on. A session the manager does not know is looked up in the scrollback files
// (an ended task after a restart); one that is not there either is reported as done and empty.
func (m *Manager) Read(id string, offset int64, max int) ReadReply {
	if max <= 0 {
		max = defaultReadMax
	}
	if max > hardReadMax {
		max = hardReadMax
	}
	if offset < 0 {
		offset = 0
	}
	if s := m.get(id); s != nil {
		r := s.log.Read(offset, max)
		return ReadReply{Result: r, Done: s.Ended() && r.Next >= r.Tail}
	}
	if l := m.archive(id); l != nil {
		r := l.Read(offset, max)
		return ReadReply{Result: r, Done: r.Next >= r.Tail}
	}
	return ReadReply{Done: true}
}

// Write sends keystrokes. Input to an unknown or ended session is dropped.
func (m *Manager) Write(id string, b []byte) error {
	if s := m.get(id); s != nil {
		return s.Write(b)
	}
	return nil
}

// Resize changes a session's terminal size. Unknown sessions and closed consoles are not errors.
func (m *Manager) Resize(id string, cols, rows int) error {
	if cols < 1 || cols > maxDim {
		return invalid("cols")
	}
	if rows < 1 || rows > maxDim {
		return invalid("rows")
	}
	if s := m.get(id); s != nil {
		return s.Resize(uint16(cols), uint16(rows))
	}
	return nil
}

// Stop ends a session and everything it started in its terminal session (macOS) or job (Windows). It can
// block for the grace time plus a few seconds: never call it on a UI thread. The terminal's scrollback is
// written once the output has ended.
func (m *Manager) Stop(id string) error {
	s := m.get(id)
	if s == nil {
		return nil
	}
	err := s.p.KillSession()
	select {
	case <-s.ended:
	case <-time.After(forceEndAfter):
		// Something that left the session (setsid) still holds the terminal: report the session as ended and
		// drop what it prints from now on. The process tracker (A2-13) deals with the leftovers.
		s.forced.Store(true)
		s.finish()
	}
	_ = s.p.Close()
	return err
}

// Release forgets an ended session and frees its log (its scrollback file, if any, stays readable).
func (m *Manager) Release(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.sessions[id]
	if s == nil {
		m.dropArchiveLocked(id)
		return nil
	}
	if !s.Ended() {
		return api.NewError(api.ErrInvalidInput, "session %s is still running", id).WithParam("field", "session")
	}
	m.tails[id] = max(m.tails[id], s.log.Tail())
	delete(m.sessions, id)
	return nil
}

// Status is a session's state for the task list and for measurements.
type Status struct {
	Found     bool
	Ended     bool // the terminal is finished (end of output)
	RootDone  bool // the root process has exited (the terminal may still be open)
	ExitCode  int  // -1 until RootDone
	Pid       int
	Base      int64
	Tail      int64
	Trims     int64
	TotalRead int64 // bytes read from the terminal since the start (for bytes per hour without code: run the tool under `script -q <file> <tool>` in Terminal.app and `wc -c <file>`)
	// LongestAppend is the longest time the reader spent appending and notifying (a measure of "never blocks").
	LongestAppend time.Duration
}

// StatusOf reports a session's state.
func (m *Manager) StatusOf(id string) Status {
	s := m.get(id)
	if s == nil {
		return Status{ExitCode: -1}
	}
	st := Status{Found: true, Ended: s.Ended(), ExitCode: s.p.ExitCode(), Pid: s.p.Pid(),
		Base: s.log.Base(), Tail: s.log.Tail(), Trims: s.log.Trims(), TotalRead: s.totalRead.Load(),
		LongestAppend: time.Duration(s.maxAppend.Load())}
	select {
	case <-s.p.Done():
		st.RootDone = true
	default:
	}
	return st
}

// Shutdown stops every running session (in parallel) and waits at most limit. Call it when the app quits.
func (m *Manager) Shutdown(limit time.Duration) {
	m.mu.Lock()
	var run []string
	for id, s := range m.sessions {
		if !s.Ended() {
			run = append(run, id)
		}
	}
	m.mu.Unlock()
	var wg sync.WaitGroup
	for _, id := range run {
		wg.Add(1)
		go func() { defer wg.Done(); _ = m.Stop(id) }()
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(limit):
	}
}
