package proctrack

import (
	"context"
	"errors"
	"os"
	"sort"
	"sync"
	"syscall"
	"time"
)

// Group is what the tracker needs from a terminal session: internal/pty's Session satisfies it. Members and
// KillSession can take a while; the tracker only calls them from its own goroutines.
type Group interface {
	Pid() int
	Members() ([]int, error)
	Done() <-chan struct{}
	KillSession() error
}

// Process is one tracked program as the UI shows it.
type Process struct {
	PID     int     `json:"pid"`
	PPID    int     `json:"ppid"`  // parent at the last snapshot (0 on Windows); lets the UI tell a program the tool started from the shell and the tool itself
	Start   int64   `json:"start"` // Unix seconds
	Name    string  `json:"name"`
	Command string  `json:"command,omitempty"` // as the system prints it; display only, never logged or saved
	CPU     float64 `json:"cpu"`
	RSS     int64   `json:"rss"`
}

// Report is the tracked set of one session.
type Report struct {
	Session  string
	Leader   int       // PID of the terminal's session leader (the shell); 0 for a session restored after a crash
	Live     bool      // the session's terminal is open and its tool still runs
	ClosedAt time.Time // zero while Live
	Count    int
	CPU      float64 // percent of one core, summed
	RSS      int64   // bytes, summed
	Procs    []Process
}

// StopResult is the outcome of ForceStop.
type StopResult struct {
	Session   string
	Found     int       // programs in the set when the stop began
	Stopped   int       // Found minus the survivors (never negative)
	Names     []string  // distinct names of what was in the set
	Survivors []Process // still alive after the forced step and one more snapshot
	Err       error     // why the stop was not clean (the same error ForceStop returns); Survivors still says what is left
}

// Options configures a Tracker. Zero values take the defaults.
type Options struct {
	Source   Source
	DataDir  string        // the data folder; leftovers.json lives in <DataDir>/sessions. "" keeps nothing on disk
	Live     time.Duration // tick while a session is live (default 2 s)
	Idle     time.Duration // tick when only closed sessions remain (default 10 s)
	Grace    time.Duration // between SIGTERM and SIGKILL (default 5 s)
	Hard     time.Duration // how long the forced step repeats (default 5 s)
	Poll     time.Duration // gap between checks while a stop waits (default 100 ms)
	Signal   func(pid int, sig syscall.Signal) error
	UseJob   *bool // Windows: end a session through Group.KillSession (default: true on Windows only)
	Now      func() time.Time
	OnChange func() // the set of programs or a session's live state changed; never called with a tracker or stop lock held, so it may call back into the tracker, ForceStop and ForceStopAll included
	// Commands reads full command lines for display (default: one `ps -ww` call on Unix, none on Windows).
	Commands func(pids []int) ([]CommandLine, error)
}

type entry struct {
	Process
	pgid int
}

type session struct {
	id       string
	g        Group // nil for a session restored from disk
	entries  map[int]*entry
	closedAt time.Time // zero while live
	stopMu   sync.Mutex
}

// CommandLine is one program's full command line, read on demand for display only.
type CommandLine struct {
	PID   int
	Start int64
	Line  string
}

func (s *session) live() bool { return s.closedAt.IsZero() }

// Tracker keeps the sets. Safe for concurrent use.
type Tracker struct {
	o      Options
	tickMu sync.Mutex // one tick or stop-step at a time: ticks are cheap, and this keeps persistence ordered
	mu     sync.Mutex // guards sessions
	sess   map[string]*session
	saved  string // signature of what leftovers.json holds
	disk   *diskFile
	wake   chan struct{} // Track pokes Run so a new session is seen at once
}

// New returns a Tracker. Call Run to start the ticker, or Tick by hand.
func New(o Options) *Tracker {
	if o.Source == nil {
		o.Source = NewSource()
	}
	if o.Live <= 0 {
		o.Live = 2 * time.Second
	}
	if o.Idle <= 0 {
		o.Idle = 10 * time.Second
	}
	if o.Grace <= 0 {
		o.Grace = 5 * time.Second
	}
	if o.Hard <= 0 {
		o.Hard = 5 * time.Second
	}
	if o.Poll <= 0 {
		o.Poll = 100 * time.Millisecond
	}
	if o.Signal == nil {
		o.Signal = defaultSignal
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	t := &Tracker{o: o, sess: map[string]*session{}, wake: make(chan struct{}, 1)}
	t.disk = openDisk(o.DataDir)
	return t
}

func (t *Tracker) useJob() bool {
	if t.o.UseJob != nil {
		return *t.o.UseJob
	}
	return defaultUseJob
}

// Track starts tracking a session's terminal. The set grows from the session's members and from every
// descendant of a tracked program. Tracking the same id again replaces the group.
func (t *Tracker) Track(id string, g Group) {
	t.mu.Lock()
	if s := t.sess[id]; s != nil {
		s.g = g
		s.closedAt = time.Time{}
	} else {
		t.sess[id] = &session{id: id, g: g, entries: map[int]*entry{}}
	}
	t.mu.Unlock()
	select {
	case t.wake <- struct{}{}:
	default:
	}
}

// MarkClosed records that the session's terminal was closed. Its set stays until the last program is gone.
func (t *Tracker) MarkClosed(id string) {
	t.tickMu.Lock()
	t.mu.Lock()
	changed := false
	if s := t.sess[id]; s != nil && s.live() {
		s.closedAt = t.o.Now()
		changed = true
	}
	snap := t.snapshotLocked()
	t.mu.Unlock()
	if changed {
		t.persist(snap)
	}
	t.tickMu.Unlock()
	if changed {
		t.changed()
	}
}

// Forget drops a session's set without signalling anything, and from leftovers.json too.
func (t *Tracker) Forget(id string) {
	t.tickMu.Lock()
	t.mu.Lock()
	delete(t.sess, id)
	snap := t.snapshotLocked()
	t.mu.Unlock()
	t.persist(snap)
	t.tickMu.Unlock()
	t.changed()
}

func (t *Tracker) changed() {
	if t.o.OnChange != nil {
		t.o.OnChange()
	}
}

func reportOf(s *session) Report {
	r := Report{Session: s.id, Live: s.live(), ClosedAt: s.closedAt}
	if s.g != nil {
		r.Leader = s.g.Pid()
	}
	for _, e := range s.entries {
		r.Count++
		r.CPU += e.CPU
		r.RSS += e.RSS
		r.Procs = append(r.Procs, e.Process)
	}
	sort.Slice(r.Procs, func(i, j int) bool { return r.Procs[i].PID < r.Procs[j].PID })
	return r
}

// Leftovers is the cached set of one session; it never touches the system, so a UI call may use it.
func (t *Tracker) Leftovers(id string) Report {
	t.mu.Lock()
	defer t.mu.Unlock()
	if s := t.sess[id]; s != nil {
		return reportOf(s)
	}
	return Report{Session: id}
}

// All lists every session that has at least one tracked program, sorted by session id. Cached, like Leftovers.
func (t *Tracker) All() []Report {
	t.mu.Lock()
	defer t.mu.Unlock()
	var out []Report
	for _, s := range t.sess {
		if len(s.entries) > 0 {
			out = append(out, reportOf(s))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Session < out[j].Session })
	return out
}

// Active is the number of programs that belong to a closed session (the "closed, still running" ones). The
// keep-awake manager counts them as activity (A2-03 through A2-W).
func (t *Tracker) Active() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	n := 0
	for _, s := range t.sess {
		if !s.live() {
			n += len(s.entries)
		}
	}
	return n
}

// Run ticks until ctx ends: every Live while a session is live, every Idle while only closed ones remain, and
// not at all when nothing is tracked.
func (t *Tracker) Run(ctx context.Context) {
	for {
		wait := t.o.Idle
		t.mu.Lock()
		for _, s := range t.sess {
			if s.live() {
				wait = t.o.Live
				break
			}
		}
		t.mu.Unlock()
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-t.wake:
		case <-timer.C:
		}
		timer.Stop()
		_, _ = t.tick()
	}
}

// Tick takes one snapshot now.
func (t *Tracker) Tick() error {
	_, err := t.tick()
	return err
}

// tick reads the members of the live sessions, then the process table once, folds both into the sets and
// returns the table it used. A failed listing leaves every set as it was.
func (t *Tracker) tick() (map[int]Proc, error) {
	t.tickMu.Lock()
	table, changed, err := t.tickLocked()
	t.tickMu.Unlock()
	if changed {
		t.changed() // after the lock: the callback may call back into the tracker
	}
	return table, err
}

// tickQuiet is tick without the OnChange call: it sets *dirty when the set changed, and the caller calls
// t.changed() once after it released every lock it holds (ForceStop holds a session's stop lock).
func (t *Tracker) tickQuiet(dirty *bool) (map[int]Proc, error) {
	t.tickMu.Lock()
	table, changed, err := t.tickLocked()
	t.tickMu.Unlock()
	if changed {
		*dirty = true
	}
	return table, err
}

// tickLocked is tick with tickMu held by the caller. It reports whether the set changed and leaves the
// OnChange call to the caller.
func (t *Tracker) tickLocked() (map[int]Proc, bool, error) {

	type pending struct {
		s       *session
		g       Group
		live    bool
		members []int
		ok      bool
	}
	t.mu.Lock()
	var ps []*pending
	var want []int
	for _, s := range t.sess {
		p := &pending{s: s, g: s.g, live: s.live()}
		for pid := range s.entries {
			want = append(want, pid)
		}
		ps = append(ps, p)
	}
	t.mu.Unlock()
	if len(ps) == 0 {
		return nil, false, nil
	}
	exited := map[*session]bool{}
	for _, p := range ps {
		if p.g == nil {
			continue
		}
		select {
		case <-p.g.Done():
			exited[p.s] = true // the tool exited: from now on it is a closed session
		default:
		}
		if m, err := p.g.Members(); err == nil { // a listing error adds nothing this tick
			p.members, p.ok = m, true
			want = append(want, m...)
		}
	}
	list, err := t.o.Source.Read(want)
	if err != nil {
		return nil, false, err
	}
	table := make(map[int]Proc, len(list))
	for _, p := range list {
		table[p.PID] = p
	}

	t.mu.Lock()
	now := t.o.Now()
	owner := map[int]string{}
	for _, s := range t.sess {
		for pid := range s.entries {
			owner[pid] = s.id
		}
	}
	changed := false
	for _, p := range ps {
		s := p.s
		if t.sess[s.id] != s {
			continue // forgotten meanwhile
		}
		if exited[s] && s.live() {
			s.closedAt = now
			changed = true
		}
		if advance(s, table, p.members, owner) {
			changed = true
		}
		// A session whose last program is gone and whose terminal is over leaves the tracker.
		if len(s.entries) == 0 && !s.live() {
			delete(t.sess, s.id)
			changed = true
		}
	}
	snap := t.snapshotLocked()
	t.mu.Unlock()
	t.persist(snap)
	return table, changed, nil
}

// advance folds one process table into a session's set and reports whether the set (not CPU or memory) changed.
// It is a pure function of its arguments apart from s and owner, and the tests drive it with fake tables.
//
//   - A tracked program stays while the table has its PID with the same start time. A reused PID has a later
//     start time, so the old entry ends (and the newcomer is added only if it is really a descendant).
//   - New members: the PIDs the session itself reports (same terminal session), and every program whose parent
//     is tracked and started no later than the child. Re-parenting to launchd or a setsid call changes neither
//     the PID nor the start time, so what is already in the set stays in it.
//   - A program that another session tracks is never taken over.
func advance(s *session, table map[int]Proc, members []int, owner map[int]string) bool {
	changed := false
	for pid, e := range s.entries {
		p, ok := table[pid]
		if !ok || p.Start != e.Start {
			delete(s.entries, pid)
			delete(owner, pid)
			changed = true
			continue
		}
		e.CPU, e.RSS, e.pgid, e.PPID = p.CPU, p.RSS, p.PGID, p.PPID
		if p.Comm != "" {
			e.Command = p.Comm
		}
	}
	add := func(p Proc) {
		if p.PID <= 1 || p.PID == os.Getpid() {
			return
		}
		if o, taken := owner[p.PID]; taken && o != s.id {
			return
		}
		if _, have := s.entries[p.PID]; have {
			return
		}
		s.entries[p.PID] = &entry{Process: Process{PID: p.PID, PPID: p.PPID, Start: p.Start, Name: p.Name(), Command: p.Comm, CPU: p.CPU, RSS: p.RSS}, pgid: p.PGID}
		owner[p.PID] = s.id
		changed = true
	}
	for _, pid := range members {
		if p, ok := table[pid]; ok {
			add(p)
		}
	}
	// Descendants, to a fixed point: a child may appear in the table before its parent in any order.
	var kids map[int][]Proc
	if len(s.entries) > 0 {
		kids = map[int][]Proc{}
		for _, p := range table {
			kids[p.PPID] = append(kids[p.PPID], p)
		}
	}
	queue := make([]int, 0, len(s.entries))
	for pid := range s.entries {
		queue = append(queue, pid)
	}
	for len(queue) > 0 {
		pid := queue[0]
		queue = queue[1:]
		parent := s.entries[pid]
		if parent == nil {
			continue
		}
		for _, c := range kids[pid] {
			if c.Start < parent.Start {
				continue // cannot be its child: the number was reused
			}
			if _, have := s.entries[c.PID]; have {
				continue
			}
			add(c)
			if _, ok := s.entries[c.PID]; ok {
				queue = append(queue, c.PID)
			}
		}
	}
	return changed
}

var errUnreadable = errors.New("proctrack: the process table could not be read")

// SetOnChange sets the change callback. Call it before Run and before any session is tracked.
func (t *Tracker) SetOnChange(fn func()) { t.o.OnChange = fn }

// Commands reads the full command lines of a session's tracked programs, for the expanded list in the UI.
// One call to the system per request (never per tick); a line is returned only for a program whose PID and
// start time still match the tracked one. The lines are display only: never cached, saved or logged.
func (t *Tracker) Commands(id string) ([]CommandLine, error) {
	t.mu.Lock()
	s := t.sess[id]
	var pids []int
	start := map[int]int64{}
	if s != nil {
		for pid, e := range s.entries {
			pids = append(pids, pid)
			start[pid] = e.Start
		}
	}
	t.mu.Unlock()
	if len(pids) == 0 {
		return nil, nil
	}
	sort.Ints(pids)
	read := t.o.Commands
	if read == nil {
		read = defaultCommands
	}
	all, err := read(pids)
	if err != nil {
		return nil, err
	}
	var out []CommandLine
	for _, c := range all {
		if st, ok := start[c.PID]; ok && st == c.Start {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PID < out[j].PID })
	return out, nil
}
