// Package keepawake keeps the computer awake while AI work is going on (plan 4.10).
//
// One Manager counts activity: AI tasks that run, AI tasks that wait for the owner (for the grace time), and
// closed sessions whose programs are still running (plan 4.8b, through the Leftovers interface). While the count
// is above zero and the setting is on, the Manager holds the operating system's assertions through its Holders
// (macOS: caffeinate and an NSProcessInfo activity; Windows: SetThreadExecutionState on one locked thread).
// A plain shell never counts. After the hard ceiling without any event or input, a session stops counting even if
// it still looks busy, programs left behind included.
//
// The package starts no process and takes no assertion by itself: the Holders do, and every test uses fakes.
package keepawake

import (
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/rudian/codepass/internal/api"
)

// DefaultPollInterval is how often the Manager asks Leftovers while a closed session is counted, so a program
// that ended is noticed even when nobody calls Refresh.
const DefaultPollInterval = 5 * time.Second

// retryInterval is how soon a failed assertion is tried again while it is wanted.
const retryInterval = 30 * time.Second

// Options configures New.
type Options struct {
	Holders   []Holder
	Clock     Clock     // nil = the wall clock
	Leftovers Leftovers // nil = none
	Config    api.KeepAwakeConfig
	// Save stores the configuration after SetConfig (A2-W writes it into settings.json; nil = nowhere).
	Save         func(api.KeepAwakeConfig) error
	Log          *slog.Logger // nil = discard
	PollInterval time.Duration
	// OnChange is called after the state changed, never under a lock.
	OnChange func(api.KeepAwakeState)
}

type session struct {
	kind  Kind
	state TaskState
	since time.Time // when the state was entered (the grace time counts from here)
	last  time.Time // the last hook event or input (the hard ceiling counts from here)
}

// Manager is the keep-awake counter. Safe for concurrent use.
type Manager struct {
	opts  Options
	clock Clock
	log   *slog.Logger

	evalMu sync.Mutex // serialises evaluation and every holder call
	// owned by evalMu
	ok     []bool // holder i was acquired and holds (a holder that reports for itself is asked as well)
	screen []bool // holder i was acquired with the screen option on
	tried  []bool // Acquire was called since the last Release (so a release is due even when ok is false)

	emitMu sync.Mutex         // serialises delivery of the state to OnChange
	sent   api.KeepAwakeState // the last state delivered (under emitMu)
	saveMu sync.Mutex         // one SetConfig at a time from assigning to saving

	mu       sync.Mutex
	cfg      api.KeepAwakeConfig
	sessions map[string]*session
	st       api.KeepAwakeState
	timer    Timer
	closed   bool
}

// New builds a Manager. Nothing is held until the first activity.
func New(o Options) *Manager {
	if o.Clock == nil {
		o.Clock = RealClock()
	}
	if o.Log == nil {
		o.Log = slog.New(slog.DiscardHandler)
	}
	if o.PollInterval <= 0 {
		o.PollInterval = DefaultPollInterval
	}
	if err := ValidateConfig(o.Config); err != nil {
		o.Config = DefaultConfig()
	}
	m := &Manager{opts: o, clock: o.Clock, log: o.Log, cfg: o.Config, sessions: map[string]*session{},
		ok: make([]bool, len(o.Holders)), screen: make([]bool, len(o.Holders)), tried: make([]bool, len(o.Holders))}
	m.st.Config = o.Config
	m.sent = m.st
	for _, h := range o.Holders {
		if n, ok := h.(changeNotifier); ok {
			n.SetNotify(m.reevaluate)
		}
	}
	return m
}

// ---- input --------------------------------------------------------------------------------------------------

// SetState reports a session's state. A plain shell (KindShell) is ignored: it never holds the computer awake.
// Every call counts as an event for the hard ceiling, so a feeder that polls reports a state only when it changed
// (a poll that reported "running (no signal)" every few seconds would restart the ceiling every time). See
// TaskState for the mapping of the task states.
func (m *Manager) SetState(id string, kind Kind, st TaskState) {
	if id == "" {
		return
	}
	now := m.clock.Now()
	m.mu.Lock()
	if kind == KindShell {
		delete(m.sessions, id)
		m.mu.Unlock()
		m.reevaluate()
		return
	}
	s := m.sessions[id]
	if s == nil {
		s = &session{kind: kind, state: st, since: now}
		m.sessions[id] = s
	} else if s.state != st {
		s.state, s.since = st, now
	}
	s.last = now
	m.mu.Unlock()
	m.reevaluate()
}

// Touch records a hook event or input from the owner for a session: it restarts the hard ceiling and nothing else.
// Call it for hook events and for the owner's own input (keys and paste from the terminal page's key handler),
// never for the terminal's automatic replies (cursor position, device attributes, focus and mouse reports): a TUI
// that queries the terminal would keep a stuck task alive forever.
//
// It is cheap on the input path: a Running session inside its ceiling only gets a later time (the pending timer
// judges the later deadline when it fires), with no holder call and no Leftovers call.
func (m *Manager) Touch(id string) {
	now := m.clock.Now()
	m.mu.Lock()
	s := m.sessions[id]
	if s == nil {
		m.mu.Unlock()
		return
	}
	cheap := s.state == StateRunning && now.Before(s.last.Add(m.ceilingDuration()))
	s.last = now
	m.mu.Unlock()
	if !cheap {
		m.reevaluate()
	}
}

// Forget removes a session's record (the task was deleted).
func (m *Manager) Forget(id string) {
	m.mu.Lock()
	delete(m.sessions, id)
	m.mu.Unlock()
	m.reevaluate()
}

// Refresh asks again: Leftovers changed, or the computer woke up. Call it without holding a lock that
// Leftovers.Count takes.
func (m *Manager) Refresh() { m.reevaluate() }

// SetOnChange sets the function called after the state changed (never under a lock). The service sets it at start.
func (m *Manager) SetOnChange(f func(api.KeepAwakeState)) {
	m.mu.Lock()
	m.opts.OnChange = f
	m.mu.Unlock()
}

// Config returns the settings.
func (m *Manager) Config() api.KeepAwakeConfig {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cfg
}

// SetConfig validates, applies and saves the settings. The new values apply even when saving fails; the error
// (E_FILE_IO) says so.
func (m *Manager) SetConfig(c api.KeepAwakeConfig) error {
	if err := ValidateConfig(c); err != nil {
		return err
	}
	m.saveMu.Lock() // two quick changes are applied and saved in the same order
	defer m.saveMu.Unlock()
	m.mu.Lock()
	m.cfg = c
	m.mu.Unlock()
	m.reevaluate()
	if m.opts.Save == nil {
		return nil
	}
	if err := m.opts.Save(c); err != nil {
		m.log.Warn("keepawake: saving the settings failed", "err", err)
		if api.AsAppError(err) != nil && api.AsAppError(err).Code != api.ErrInternal {
			return err
		}
		return api.NewError(api.ErrFileIO, "cannot write the keep-awake settings").WithDetail(err.Error())
	}
	return nil
}

// State is the current state.
func (m *Manager) State() api.KeepAwakeState {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.st
}

// Close releases everything and stops the timer. It is the last call; later input is ignored.
func (m *Manager) Close() {
	m.evalMu.Lock()
	m.mu.Lock()
	m.closed = true
	if m.timer != nil {
		m.timer.Stop()
		m.timer = nil
	}
	m.mu.Unlock()
	m.releaseAll()
	m.mu.Lock()
	m.st.Held, m.st.Screen, m.st.Error = false, false, ""
	m.st.Active, m.st.Running, m.st.Waiting, m.st.Leftover, m.st.Programs, m.st.ReleaseAt = 0, 0, 0, 0, 0, 0
	m.mu.Unlock()
	m.evalMu.Unlock()
	m.deliver()
}

// deliver sends the latest state to OnChange when it differs from the last one sent. One at a time and always the
// newest, so two evaluations that change the state cannot arrive in the opposite order.
func (m *Manager) deliver() {
	m.emitMu.Lock()
	defer m.emitMu.Unlock()
	m.mu.Lock()
	st, cb := m.st, m.opts.OnChange
	m.mu.Unlock()
	if cb == nil || st == m.sent {
		return
	}
	m.sent = st
	cb(st)
}

// ---- evaluation ---------------------------------------------------------------------------------------------

// reevaluate recounts, applies the holders and sets the timer for the next deadline.
func (m *Manager) reevaluate() {
	m.evaluate()
	m.deliver()
}

func (m *Manager) evaluate() {
	m.evalMu.Lock()
	defer m.evalMu.Unlock()
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	var ended []string
	for id, s := range m.sessions {
		if s.state == StateEnded {
			ended = append(ended, id)
		}
	}
	m.mu.Unlock()
	sort.Strings(ended)
	// Leftovers.Count runs outside the lock. A session that turns Ended after this snapshot was not asked: it is
	// not judged by a count it never got.
	asked := make(map[string]bool, len(ended))
	counts := make(map[string]int, len(ended))
	for _, id := range ended {
		asked[id] = true
		if m.opts.Leftovers != nil {
			counts[id] = m.opts.Leftovers.Count(id)
		}
	}

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	now := m.clock.Now()
	cfg := m.cfg
	grace, forever := m.graceDuration()
	ceiling := m.ceilingDuration()
	var running, waiting, leftover, programs int
	var ceilingHit, hasEnded bool
	var wake time.Time
	var releaseAt time.Time
	note := func(t time.Time) {
		if wake.IsZero() || t.Before(wake) {
			wake = t
		}
	}
	for id, s := range m.sessions {
		ceilAt := s.last.Add(ceiling)
		var deadline time.Time
		counted := false
		switch s.state {
		case StateRunning:
			counted = true
			deadline = ceilAt
		case StateWaiting:
			counted = true
			deadline = ceilAt
			if !forever {
				if g := s.since.Add(grace); g.Before(deadline) {
					deadline = g
				}
			}
		case StateEnded:
			if asked[id] && counts[id] == 0 {
				delete(m.sessions, id) // nothing left: the record is done
				continue
			}
			counted = true
			deadline = ceilAt
			if !asked[id] {
				note(now) // turned Ended during this round: it counts for now and is asked at once
			}
		}
		if !counted {
			continue
		}
		if !now.Before(deadline) {
			if !now.Before(ceilAt) {
				ceilingHit = true
			}
			continue
		}
		switch s.state {
		case StateRunning:
			running++
		case StateWaiting:
			waiting++
		case StateEnded:
			leftover++
			programs += counts[id]
			hasEnded = true
		}
		note(deadline)
		if s.state != StateRunning {
			if deadline.After(releaseAt) {
				releaseAt = deadline
			}
		}
	}
	active := running + waiting + leftover
	wantSleep := cfg.Enabled && active > 0
	wantNap := active > 0
	if hasEnded {
		note(now.Add(m.opts.PollInterval))
	}
	m.mu.Unlock()

	held, screen, errCode, retry := m.apply(wantSleep, wantNap, cfg.ScreenToo)
	if retry {
		note(now.Add(retryInterval))
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return
	}
	next := api.KeepAwakeState{Held: held, Active: active, Running: running, Waiting: waiting, Leftover: leftover,
		Programs: programs, Screen: screen, CeilingHit: ceilingHit, Error: errCode, Config: cfg}
	if active > 0 && running == 0 && !releaseAt.IsZero() {
		next.ReleaseAt = releaseAt.UnixMilli()
	}
	m.st = next
	if m.timer != nil {
		m.timer.Stop()
		m.timer = nil
	}
	if !wake.IsZero() {
		d := wake.Sub(now)
		if d < 0 {
			d = 0
		}
		m.timer = m.clock.AfterFunc(d, m.reevaluate)
	}
}

// apply brings the holders to the wanted state. Called under evalMu only. wantSleep is for the sleep assertions,
// wantNap for the task-scoped holders (the App Nap activity). It returns what is really held (a holder that can
// report for itself is asked), the error code ("" = none) and whether a failed holder should be tried again.
func (m *Manager) apply(wantSleep, wantNap, screen bool) (held, screenHeld bool, errCode string, retry bool) {
	failedSleep := 0
	for i, h := range m.opts.Holders {
		scoped := isTaskScoped(h)
		want := wantSleep
		if scoped {
			want = wantNap
		}
		if !want {
			m.release(i)
			continue
		}
		if m.ok[i] && m.screen[i] == screen && holding(h, true) {
			continue
		}
		m.tried[i] = true
		if err := h.Acquire(screen); err != nil {
			m.log.Warn("keepawake: could not hold", "holder", h.Name(), "err", err)
			retry = true
			if !scoped {
				failedSleep++
			}
			// A holder that reports for itself may still hold what it held before (a change that failed keeps the
			// old hold); any other holder holds nothing now.
			if !holding(h, false) {
				m.ok[i] = false
			}
			continue
		}
		m.ok[i], m.screen[i] = true, screen
	}
	for i, h := range m.opts.Holders {
		if isTaskScoped(h) {
			continue
		}
		if !holding(h, m.ok[i]) {
			continue
		}
		held = true
		if sr, ok := h.(screenReporter); ok {
			screenHeld = screenHeld || sr.HoldsScreen()
		} else if m.screen[i] {
			screenHeld = true
		}
	}
	switch {
	case failedSleep == 0:
	case held:
		errCode = api.KeepAwakeErrorPartial
	default:
		errCode = api.KeepAwakeErrorNone
	}
	return held, screenHeld, errCode, retry
}

// holding asks a holder that reports for itself; any other holder gets the fallback.
func holding(h Holder, fallback bool) bool {
	if r, ok := h.(holdingReporter); ok {
		return r.Holding()
	}
	return fallback
}

func isTaskScoped(h Holder) bool {
	t, ok := h.(taskScoped)
	return ok && t.TaskScoped()
}

func (m *Manager) release(i int) {
	if m.ok[i] || m.tried[i] {
		m.opts.Holders[i].Release()
	}
	m.ok[i], m.screen[i], m.tried[i] = false, false, false
}

func (m *Manager) releaseAll() {
	for i := range m.opts.Holders {
		m.release(i)
	}
}
