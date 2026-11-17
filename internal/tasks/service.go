package tasks

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/rudian/codepass/internal/aitools"
	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/keepawake"
	"github.com/rudian/codepass/internal/notify"
	"github.com/rudian/codepass/internal/proctrack"
	"github.com/rudian/codepass/internal/taskstate"
	"github.com/rudian/codepass/internal/terminal"
)

// DefaultSoftCap is the number of live tasks from which starting another asks first (plan 4.8: "a soft cap
// (default 6)"). The Settings value replaces it through Deps.SoftCap.
const DefaultSoftCap = 6

const (
	defaultPollEvery = 3 * time.Second
	namePromptBudget = 6 * time.Second
	maxTitleRunes    = 80
	fallbackRunes    = 40
	// keepEnded is how many ended tasks keep their terminal log in memory; older ones are released when their
	// scrollback file exists (the page then reads the file).
	keepEnded = 8
)

// Terminals is the part of *terminal.Manager the service uses.
type Terminals interface {
	Start(terminal.StartOptions) (*terminal.Session, error)
	// Stop ends a session and everything it started in its terminal session. It can take seconds.
	Stop(id string) error
	Release(id string) error
	Shutdown(limit time.Duration)
}

// Awake is the part of *keepawake.Manager the service feeds.
type Awake interface {
	SetState(id string, kind keepawake.Kind, st keepawake.TaskState)
	Touch(id string)
	Forget(id string)
	Refresh()
}

// Procs is the part of *proctrack.Tracker the service uses.
type Procs interface {
	Track(id string, g proctrack.Group)
	MarkClosed(id string)
	Forget(id string)
	Leftovers(id string) proctrack.Report
	Active() int
	// Tick takes one snapshot now: finalize calls it right after MarkClosed so that the programs that died with
	// the session are gone from the set before the task is shown.
	Tick() error
	ForceStopAll() ([]proctrack.StopResult, error)
}

// RepoRef is what the service needs to know about a repository.
type RepoRef struct {
	ID   string
	Path string // the main workspace
	Name string // the local folder name
}

// Repos finds a repository by its stable ID (F1-18).
type Repos interface {
	Lookup(id string) (RepoRef, bool)
}

// Naming is a task's name as the tool gave it (aitools.Namer), or the quiet fallback.
type Naming struct {
	Title  string
	Slug   string
	FromAI bool
}

// Deps is everything a Service needs. Terminals, Tracker, Launch and DataDir are required.
type Deps struct {
	DataDir   string
	Terminals Terminals
	Tracker   *taskstate.Tracker
	// Relay is the onChange target the tracker was built with; New connects it. Nil when the caller passes
	// Service.OnState to the tracker itself.
	Relay *Relay
	// Endpoint is the hook socket's address (taskstate Server.Endpoint) and Exe the app's own binary, which the
	// hooks run in report mode.
	Endpoint string
	Exe      string

	Awake  Awake
	Procs  Procs
	Notify *notify.Service
	Repos  Repos

	// Launch builds the launch (AIToolService.buildLaunch fills the login shell, the OS and the program path).
	Launch func(aitools.LaunchRequest) (aitools.Launch, error)
	// Name asks the tool for a title and a slug (AIToolService.NameTask). It never fails in practice; an error
	// gives the quiet fallback.
	Name func(ctx context.Context, tool aitools.ID, description string, n int) (Naming, error)

	Lang    func() string // "zh-CN" or "en"
	Focused func() bool   // the window is in front; nil counts as not
	// ShowWindow brings the window forward (a click on a notification).
	ShowWindow func()
	Emit       func(name string, data any)
	Log        *slog.Logger
	Now        func() time.Time
	SoftCap    func() int // nil = DefaultSoftCap
	// Probe is the fallback of plan 4.8 for one live terminal (taskstate.Probe with the system's process table).
	// Nil turns the fallback off.
	Probe     func(term taskstate.Terminal) taskstate.Verdict
	PollEvery time.Duration
	// ExcludeBackup keeps <data>/sessions out of Time Machine; nil does nothing (ExcludeFromBackup is the
	// real one).
	ExcludeBackup func(dir string) error
	// NewID and NewSessionID are for tests.
	NewID        func() string
	NewSessionID func(tool aitools.ID) (string, error)
}

// jobs counts short background jobs. Unlike a WaitGroup, Add may run while a Wait is in progress.
type jobs struct {
	mu sync.Mutex
	n  int
	c  *sync.Cond
}

func (j *jobs) cond() *sync.Cond {
	if j.c == nil {
		j.c = sync.NewCond(&j.mu)
	}
	return j.c
}

func (j *jobs) Add(d int) {
	j.mu.Lock()
	j.n += d
	if j.n <= 0 {
		j.cond().Broadcast()
	}
	j.mu.Unlock()
}

func (j *jobs) Done() { j.Add(-1) }

func (j *jobs) Wait() {
	j.mu.Lock()
	c := j.cond()
	for j.n > 0 {
		c.Wait()
	}
	j.mu.Unlock()
}

type busyKind uint8

const (
	busyLaunch   busyKind = iota + 1 // Create or Resume is between its checks and the stored run
	busyFinalize                     // a run's end is being recorded
)

// run is one start of one task (a resume is a new run).
type run struct {
	id       string
	sess     *terminal.Session
	stopping atomic.Bool
	once     sync.Once
}

// Service is the task lifecycle. Every method is safe for concurrent use.
type Service struct {
	d    Deps
	rs   *recStore
	ctx  context.Context
	stop context.CancelFunc
	wg   sync.WaitGroup // the long-lived goroutines: the emitter and one watcher per run
	bg   jobs           // short jobs: notifications, files

	saveMu sync.Mutex

	mu       sync.Mutex
	recs     map[string]*record
	order    []string
	data     fileData // the counters and the flag; Tasks is rebuilt on save
	runs     map[string]*run
	info     map[string]taskstate.Info
	awake    map[string]keepawake.TaskState // the last state reported for a task; absent = none
	notified map[string]bool                // a "waiting" notification was sent for the task and not taken back
	pending  int                            // Creates and Resumes between the cap check and the start
	busy     map[string]busyKind            // a launch or a finalize of this task is in progress: no second one starts
	tailHook func()                         // tests: runs in Create's and Resume's tail, after the run is stored
	idle     *sync.Cond                     // on mu: a busy mark was cleared

	nq     sync.Mutex // the notification queue: one goroutine sends and takes back in order
	nqueue []func()
	nkick  chan struct{}

	running atomic.Bool // cached: a task runs or a closed session left programs (the window-close hook reads it)

	kick    chan struct{}
	emitted atomic.Int64
	kicked  atomic.Int64
}

// New builds the service: it loads tasks.json (a task that was live when the app went away is stopped now),
// registers the notification kinds and connects the relay. Nothing starts until a task is created.
func New(d Deps) *Service {
	if d.Log == nil {
		d.Log = slog.New(slog.NewTextHandler(discard{}, nil))
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Emit == nil {
		d.Emit = func(string, any) {}
	}
	if d.Lang == nil {
		d.Lang = func() string { return "en" }
	}
	if d.PollEvery <= 0 {
		d.PollEvery = defaultPollEvery
	}
	if d.SoftCap == nil {
		d.SoftCap = func() int { return DefaultSoftCap }
	}
	if d.NewID == nil {
		d.NewID = newTaskID
	}
	if d.NewSessionID == nil {
		d.NewSessionID = aitools.NewSessionID
	}
	s := &Service{d: d, recs: map[string]*record{}, runs: map[string]*run{}, info: map[string]taskstate.Info{},
		awake: map[string]keepawake.TaskState{}, notified: map[string]bool{}, busy: map[string]busyKind{},
		kick: make(chan struct{}, 1), nkick: make(chan struct{}, 1)}
	s.ctx, s.stop = context.WithCancel(context.Background())
	s.idle = sync.NewCond(&s.mu)
	var loaded fileData
	s.rs, loaded = openRecords(d.DataDir, d.Now, d.Log)
	s.data = loaded
	for _, r := range loaded.Tasks {
		r := r
		if !terminal.ValidID(r.ID) {
			continue // a record this app did not write
		}
		if r.Status == statusLive || r.Status == "" {
			r.Status = statusStopped // the app went away while it ran
			if r.EndedAt.IsZero() {
				r.EndedAt = r.StartedAt
			}
		}
		s.recs[r.ID] = &r
		s.order = append(s.order, r.ID)
	}
	if d.Relay != nil {
		d.Relay.Connect(s.OnState)
	}
	if d.Notify != nil {
		spec := notify.KindSpec{OnClick: s.onClick}
		d.Notify.RegisterKind(KindTaskWaiting, spec)
		d.Notify.RegisterKind(KindStillRunning, spec)
	}
	s.wg.Add(2)
	go s.emitter()
	go s.notifier()
	s.excludeSessions()
	s.recompute()
	s.save()
	return s
}

type discard struct{}

func (discard) Write(b []byte) (int, error) { return len(b), nil }

func newTaskID() string {
	var b [6]byte
	_, _ = rand.Read(b[:])
	return "t-" + hex.EncodeToString(b[:])
}

// sessionsDir is <data>/sessions, the parent of every task's folder.
func (s *Service) sessionsDir() string { return filepath.Join(s.d.DataDir, "sessions") }

// taskDir is <data>/sessions/<task-id>. The ID is validated by every caller (terminal.ValidID: no separator,
// no dot), so the path can only name one task's own folder.
func (s *Service) taskDir(id string) string { return filepath.Join(s.sessionsDir(), id) }

// excludeSessions creates <data>/sessions and keeps it out of Time Machine (plan 3.1). A failure is logged and
// otherwise ignored.
func (s *Service) excludeSessions() {
	if s.d.DataDir == "" {
		return
	}
	dir := s.sessionsDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		s.d.Log.Warn("cannot create the sessions folder", "err", err)
		return
	}
	if s.d.ExcludeBackup != nil {
		s.async(func() {
			if err := s.d.ExcludeBackup(dir); err != nil {
				s.d.Log.Warn("cannot keep the sessions folder out of the backup", "err", err)
			}
		})
	}
}

// Close stops the background goroutines (the pollers, the event emitter). It does not stop any task: that is
// Stop and StopAll, and the terminal manager's own shutdown is the last resort.
func (s *Service) Close() {
	s.stop()
	done := make(chan struct{})
	go func() { s.wg.Wait(); s.bg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second): // an event emit that is parked must not hold the quit
	}
}

func (s *Service) async(f func()) {
	if s.ctx.Err() != nil {
		go f() // closing: not waited for
		return
	}
	s.bg.Add(1)
	go func() {
		defer s.bg.Done()
		f()
	}()
}

// ---- views ------------------------------------------------------------------------------------------------

func invalid(field string) error {
	return api.NewError(api.ErrInvalidInput, "invalid value for %s", field).WithParam("field", field)
}

func toolID(t api.AIToolID) aitools.ID { return aitools.ID(t) }

var shellNames = map[string]bool{"sh": true, "bash": true, "zsh": true, "fish": true, "dash": true, "ksh": true, "csh": true, "tcsh": true}

func isShell(name string) bool {
	n := strings.ToLower(filepath.Base(strings.ReplaceAll(name, `\`, "/")))
	n = strings.TrimPrefix(n, "-")
	n = strings.TrimSuffix(n, ".exe")
	return shellNames[n]
}

// started counts the programs the tool started. The leader is either the login shell, which runs the tool as
// its child (the tool is a shell function, or the shell does not exec it): then the leader and its direct
// children are not counted. Or it is the tool itself, because the shell exec'd it: then only the leader is not
// counted. A leader that is not in the set any more (it died) is taken as the shell. On Windows PPID is 0, so
// the count includes the tool (FU-A2-13a).
func started(r proctrack.Report) int {
	leaderIsTool := false
	for _, p := range r.Procs {
		if p.PID == r.Leader && r.Leader != 0 {
			leaderIsTool = !isShell(p.Name)
			break
		}
	}
	n := 0
	for _, p := range r.Procs {
		if p.PID == r.Leader {
			continue
		}
		if !leaderIsTool && p.PPID == r.Leader {
			continue
		}
		n++
	}
	return n
}

func iso(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func (s *Service) leftovers(id string) proctrack.Report {
	if s.d.Procs == nil {
		return proctrack.Report{Session: id}
	}
	return s.d.Procs.Leftovers(id)
}

// derive is the one place that maps the pieces onto the task's state (plan 4.8).
func derive(rec *record, live bool, info taskstate.Info, haveInfo bool, left proctrack.Report) (api.TaskState, api.TaskWait) {
	if !live && rec.Status == statusLive {
		// launch is in progress (New turns every live record without a run into stopped, so this state exists
		// only between the record's creation and the run's storing)
		return api.TaskStarting, ""
	}
	if live {
		if !haveInfo {
			return api.TaskStarting, ""
		}
		switch info.State {
		case taskstate.StateRunning:
			return api.TaskRunning, ""
		case taskstate.StateWaiting:
			return api.TaskWaiting, api.TaskWait(info.Reason)
		case taskstate.StateNoSignal:
			return api.TaskNoSignal, ""
		case taskstate.StateEnded:
			// the fallback thinks the tool is gone but the process lives: advisory, the terminal is one click away
			return api.TaskNoSignal, ""
		}
		return api.TaskStarting, ""
	}
	if left.Count > 0 {
		return api.TaskClosedRunning, ""
	}
	switch rec.Status {
	case statusDone:
		return api.TaskDone, ""
	case statusFailed:
		return api.TaskFailed, ""
	}
	return api.TaskStopped, ""
}

func (s *Service) view(rec *record, live bool, info taskstate.Info, haveInfo bool, left proctrack.Report) api.Task {
	state, reason := derive(rec, live, info, haveInfo, left)
	t := api.Task{
		ID: rec.ID, Repo: rec.Repo, Tool: api.AIToolID(rec.Tool), Title: rec.Title, Slug: rec.Slug,
		NamedBy: api.TaskNameSource(rec.NamedBy), Number: rec.Number, Where: api.TaskWhere(rec.Where), Dir: rec.Dir,
		State: state, Reason: reason, CreatedAt: iso(rec.CreatedAt), StartedAt: iso(rec.StartedAt),
		EndedAt: iso(rec.EndedAt), Live: live, Resumed: rec.Resumed,
		Resumable: !live && rec.Status != statusLive && len(rec.SessionIDs) > 0,
	}
	if live || rec.Status == statusLive {
		t.EndedAt = ""
	}
	if rec.ExitCode != nil && !live && rec.Status != statusLive {
		c := *rec.ExitCode
		t.ExitCode = &c
	}
	if haveInfo {
		t.PermissionMode = info.PermissionMode
	}
	if left.Count > 0 {
		t.Leftovers = api.TaskLeftovers{Count: left.Count, Others: started(left), CPU: left.CPU, Memory: left.RSS}
		if !left.Live {
			t.Leftovers.ClosedAt = iso(left.ClosedAt)
		}
	}
	return t
}

// Snapshot is the whole list with the counts. It reads the tracker's and the process tracker's cached state only.
func (s *Service) Snapshot() api.TasksState {
	s.mu.Lock()
	type item struct {
		rec  record
		live bool
		info taskstate.Info
		have bool
	}
	items := make([]item, 0, len(s.order))
	for _, id := range s.order {
		rec := s.recs[id]
		in, have := s.info[id]
		items = append(items, item{rec: *rec, live: s.runs[id] != nil, info: in, have: have})
	}
	s.mu.Unlock()
	st := api.TasksState{Tasks: make([]api.Task, 0, len(items)), Cap: s.d.SoftCap()}
	for i := range items {
		it := &items[i]
		t := s.view(&it.rec, it.live, it.info, it.have, s.leftovers(it.rec.ID))
		st.Tasks = append(st.Tasks, t)
		if t.Live {
			st.Live++
		}
		switch t.State {
		case api.TaskStarting, api.TaskRunning, api.TaskNoSignal:
			st.Running++
		case api.TaskWaiting:
			st.Waiting++
		case api.TaskClosedRunning:
			st.Closed++
			st.Programs += t.Leftovers.Count
		}
	}
	return st
}

// Get returns one task.
func (s *Service) Get(id string) (api.Task, bool) {
	s.mu.Lock()
	rec, ok := s.recs[id]
	if !ok {
		s.mu.Unlock()
		return api.Task{}, false
	}
	cp := *rec
	live := s.runs[id] != nil
	in, have := s.info[id]
	s.mu.Unlock()
	return s.view(&cp, live, in, have, s.leftovers(id)), true
}

// LiveSummary feeds the quit dialog (design 5.6b): every task whose tool runs and every closed session that
// still has programs, with the number of programs the tool started.
func (s *Service) LiveSummary() api.LiveSummary {
	out := api.LiveSummary{Sessions: []api.LiveSession{}}
	for _, t := range s.Snapshot().Tasks {
		if !t.Live && t.State != api.TaskClosedRunning {
			continue
		}
		name := t.Repo
		if s.d.Repos != nil {
			if r, ok := s.d.Repos.Lookup(t.Repo); ok && r.Name != "" {
				name = r.Name
			}
		}
		out.Sessions = append(out.Sessions, api.LiveSession{Task: t.ID, Repo: t.Repo, RepoName: name, Tool: t.Tool,
			Title: t.Title, State: t.State, Programs: t.Leftovers.Others})
		out.Programs += t.Leftovers.Others
		if t.Live {
			out.Live++
		} else {
			out.Closed++
		}
	}
	return out
}

// TasksRunning is the window-close hook (Lifecycle.SetTasksRunning): a task runs, or a closed session left
// programs. It reads one cached flag, because the hook runs on the UI thread.
func (s *Service) TasksRunning() bool { return s.running.Load() }

func (s *Service) recompute() {
	s.mu.Lock()
	live := len(s.runs) > 0
	s.mu.Unlock()
	run := live || (s.d.Procs != nil && s.d.Procs.Active() > 0)
	s.running.Store(run)
}

// ---- events -----------------------------------------------------------------------------------------------

func (s *Service) kickEmit() {
	s.kicked.Add(1)
	select {
	case s.kick <- struct{}{}:
	default:
	}
}

// emitter sends tasks:changed from its own goroutine: the event system can block (Wails parks emitters once its
// queue is full), and the tracker's delivery lock must never wait for it.
func (s *Service) emitter() {
	defer s.wg.Done()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-s.kick:
		}
		target := s.kicked.Load()
		s.d.Emit(api.EventTasksChanged, s.Snapshot())
		s.emitted.Store(target)
	}
}

// Flush waits until every change so far was sent as an event and the background work (notifications) is done.
// For tests and for a clean shutdown.
func (s *Service) Flush() {
	for i := 0; i < 5000 && s.emitted.Load() < s.kicked.Load(); i++ {
		time.Sleep(time.Millisecond)
	}
	s.bg.Wait()
}

func (s *Service) changed() {
	s.recompute()
	s.kickEmit()
}

func (s *Service) save() {
	s.saveMu.Lock()
	defer s.saveMu.Unlock()
	s.mu.Lock()
	d := s.data
	d.Counters = make(map[string]int, len(s.data.Counters))
	for k, v := range s.data.Counters {
		d.Counters[k] = v
	}
	d.Tasks = make([]record, 0, len(s.order))
	for _, id := range s.order {
		r := *s.recs[id]
		r.SessionIDs = slices.Clone(r.SessionIDs)
		d.Tasks = append(d.Tasks, r)
	}
	s.mu.Unlock()
	s.rs.save(d)
}

// ---- tracker and process tracker callbacks ----------------------------------------------------------------

// Sink is the taskstate.Sink the hook socket's server feeds: it applies the report and counts it as an event
// for keep-awake's hard ceiling (the ceiling is judged from the last hook event or keystroke).
func (s *Service) Sink() taskstate.Sink { return sink{s} }

type sink struct{ s *Service }

func (k sink) Apply(r taskstate.Report) bool {
	ok := k.s.d.Tracker.Apply(r)
	if k.s.d.Awake != nil {
		k.s.d.Awake.Touch(r.Task)
	}
	return ok
}

func awakeFor(st taskstate.State) (keepawake.TaskState, bool) {
	switch st {
	case taskstate.StateRunning, taskstate.StateNoSignal:
		return keepawake.StateRunning, true
	case taskstate.StateWaiting:
		return keepawake.StateWaiting, true
	}
	return 0, false // starting does not count; ended is reported by the exit, not by the fallback
}

// OnState is the tracker's onChange. It must be quick and must not call back into the tracker (its delivery
// lock is not reentrant): it updates memory, feeds keep-awake, and hands notifications and files to goroutines.
func (s *Service) OnState(in taskstate.Info) {
	s.mu.Lock()
	rec := s.recs[in.Task]
	if rec == nil {
		s.mu.Unlock()
		return
	}
	prev, hadPrev := s.info[in.Task]
	s.info[in.Task] = in
	live := s.runs[in.Task] != nil
	dirty := false
	// The tool's own session ID, as its hooks report it (the cross-check of the ID the app created, and the only
	// source for Codex). Claude Code changes it on /clear. OpenCode's child sessions would pollute the list, so
	// its IDs stay the ones the app created.
	if in.Session != "" && rec.Tool != string(aitools.OpenCode) && !slices.Contains(rec.SessionIDs, in.Session) && validSession(rec.Tool, in.Session) {
		rec.SessionIDs = append(rec.SessionIDs, in.Session)
		dirty = true
	}
	// Resumed is per run: "resume" says the tool picked the conversation up, "startup" that it began a new one
	// (clear and compact leave it as it is).
	switch in.SessionSource {
	case "resume":
		if !rec.Resumed {
			rec.Resumed, dirty = true, true
		}
	case "startup":
		if rec.Resumed {
			rec.Resumed, dirty = false, true
		}
	}
	var send aitools.ID
	var repo string
	var clear, feed bool
	var want keepawake.TaskState
	if live {
		if w, ok := awakeFor(in.State); ok {
			if cur, set := s.awake[in.Task]; !set || cur != w {
				s.awake[in.Task] = w
				want, feed = w, true
			}
		}
		if in.State == taskstate.StateWaiting && (!hadPrev || prev.State != taskstate.StateWaiting) {
			send, repo = toolID(api.AIToolID(rec.Tool)), rec.Repo
		}
		if hadPrev && prev.State == taskstate.StateWaiting && in.State != taskstate.StateWaiting {
			clear = true
		}
	}
	s.mu.Unlock()
	if feed {
		s.feedAwake(in.Task, want) // only a state change: every SetState restarts keep-awake's hard ceiling
	}
	if dirty {
		s.async(s.save)
	}
	if send != "" {
		s.enqueue(func() { s.notifyWaiting(in.Task, repo, send) })
	}
	if clear {
		s.enqueue(func() { s.retractWaiting(in.Task) })
	}
	s.changed()
}

func (s *Service) feedAwake(id string, st keepawake.TaskState) {
	if s.d.Awake != nil {
		s.d.Awake.SetState(id, keepawake.KindAI, st)
	}
}

func validSession(tool, id string) bool {
	return len(id) > 0 && len(id) <= 128 && !strings.ContainsAny(id, "\x00\r\n")
}

func (s *Service) notifyWaiting(task, repoID string, tool aitools.ID) {
	if s.d.Notify == nil {
		return
	}
	if s.d.Focused != nil && s.d.Focused() {
		return // the person is looking at the app: the sidebar count says it
	}
	name := repoID
	if s.d.Repos != nil {
		if r, ok := s.d.Repos.Lookup(repoID); ok && r.Name != "" {
			name = r.Name
		}
	}
	// The task may have been answered or ended while Focused ran: look again, and mark the notice as sent in the
	// same critical section, so that a retract queued after this job sees it.
	s.mu.Lock()
	if s.runs[task] == nil || s.info[task].State != taskstate.StateWaiting {
		s.mu.Unlock()
		return
	}
	s.notified[task] = true
	s.mu.Unlock()
	s.d.Notify.Send(WaitingNotice(s.d.Lang(), task, repoID, name, tool))
}

// retractWaiting takes a delivered "waiting" notification back, if one was sent for the task.
func (s *Service) retractWaiting(task string) {
	s.mu.Lock()
	was := s.notified[task]
	delete(s.notified, task)
	s.mu.Unlock()
	if was && s.d.Notify != nil {
		s.d.Notify.RemoveDelivered(waitingID(task))
	}
}

// enqueue puts a notification job on the queue of the one goroutine that sends and takes back notifications,
// so that a task's send and its retract can never swap places.
func (s *Service) enqueue(f func()) {
	if s.ctx.Err() != nil {
		return
	}
	s.bg.Add(1)
	s.nq.Lock()
	s.nqueue = append(s.nqueue, f)
	s.nq.Unlock()
	select {
	case s.nkick <- struct{}{}:
	default:
	}
}

func (s *Service) popJob() func() {
	s.nq.Lock()
	defer s.nq.Unlock()
	if len(s.nqueue) == 0 {
		return nil
	}
	f := s.nqueue[0]
	s.nqueue[0] = nil
	s.nqueue = s.nqueue[1:]
	return f
}

func (s *Service) notifier() {
	defer s.wg.Done()
	for {
		select {
		case <-s.ctx.Done():
			for s.popJob() != nil {
				s.bg.Done()
			}
			return
		case <-s.nkick:
		}
		for f := s.popJob(); f != nil; f = s.popJob() {
			if s.ctx.Err() == nil {
				f()
			}
			s.bg.Done()
		}
	}
}

// onClick handles a press on either notification: show the window; for a task, tell the page to open its terminal.
func (s *Service) onClick(c notify.Click) {
	task := c.Data["task"]
	repo := c.Data["repo"]
	s.async(func() {
		if s.d.ShowWindow != nil {
			s.d.ShowWindow()
		}
		if c.Kind == KindTaskWaiting && task != "" {
			s.d.Emit(api.EventTaskOpen, api.TaskOpen{Task: task, Repo: repo})
		}
	})
}

// ProcChanged is proctrack's OnChange: the programs of a closed session came or went. It recomputes the cached
// flag, tells keep-awake about a closed session that has programs (after a restart nobody else did), and sends
// the new list. It never blocks.
func (s *Service) ProcChanged() {
	var feed []string
	s.mu.Lock()
	for _, id := range s.order {
		if s.runs[id] != nil {
			continue
		}
		if st, ok := s.awake[id]; ok && st == keepawake.StateEnded {
			continue
		}
		feed = append(feed, id)
	}
	s.mu.Unlock()
	for _, id := range feed {
		if s.leftovers(id).Count == 0 {
			continue
		}
		s.mu.Lock()
		s.awake[id] = keepawake.StateEnded
		s.mu.Unlock()
		s.feedAwake(id, keepawake.StateEnded)
	}
	if s.d.Awake != nil {
		s.d.Awake.Refresh()
	}
	s.changed()
}

// Input is told about every write to a task's terminal (terminal.Manager.SetOnInput). The terminal's own replies
// and focus reports count for nothing: they would turn a waiting task into a running one before the person
// answered (FU-A2-05c) and keep a stuck task awake for the whole ceiling. Typing renews keep-awake's ceiling;
// only keys that can answer a prompt (Enter, Esc, a digit) also change the task's state. It is quick: the
// tracker and keep-awake only update memory.
func (s *Service) Input(id string, data []byte) {
	typing := taskstate.IsTyping(data)
	if !typing {
		return
	}
	s.mu.Lock()
	live := s.runs[id] != nil
	s.mu.Unlock()
	if !live {
		return
	}
	if taskstate.CanAnswer(data) {
		s.d.Tracker.OwnerInput(id)
	}
	if s.d.Awake != nil {
		s.d.Awake.Touch(id)
	}
}

// ---- create, start, resume --------------------------------------------------------------------------------

// checkCap reserves a start slot, or asks the soft-cap question.
func (s *Service) checkCap(confirmed bool) (sl *slot, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cap := s.d.SoftCap()
	n := len(s.runs) + s.pending
	if !confirmed && cap > 0 && n >= cap {
		return nil, api.NewError(api.ErrInvalidInput, "%d tasks are already running", n).WithParam("field", api.TaskFieldCap).
			WithParam("running", itoa(n)).WithParam("cap", itoa(cap))
	}
	s.pending++
	return &slot{s: s}, nil
}

// slot is one reservation of a start. launch gives it back in the critical section that stores the run, so a
// task is never counted twice (as a run and as pending); the deferred release covers every other return.
type slot struct {
	s    *Service
	done bool // guarded by s.mu
}

// releaseLocked gives the slot back; s.mu is held.
func (sl *slot) releaseLocked() {
	if sl != nil && !sl.done {
		sl.done = true
		sl.s.pending--
	}
}

func (sl *slot) release() {
	sl.s.mu.Lock()
	sl.releaseLocked()
	sl.s.mu.Unlock()
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for ; n > 0; n /= 10 {
		i--
		b[i] = byte('0' + n%10)
	}
	return string(b[i:])
}

// Create makes a task and starts its tool in the repository's main workspace (milestone 3 adds the own
// workspace). With a description the tool names the task first (up to 5 seconds, then the quiet fallback);
// without one the task is "New task <n>" until the first message the person types names its label.
func (s *Service) Create(ctx context.Context, req api.CreateTaskRequest) (api.Task, error) {
	tool := toolID(req.Tool)
	if !tool.Valid() {
		return api.Task{}, invalid("tool")
	}
	if req.Where != "" && req.Where != api.TaskWhereMain {
		return api.Task{}, invalid("where") // the own workspace is milestone 3
	}
	if len(req.Description) > aitools.MaxMessageBytes || strings.ContainsRune(req.Description, 0) {
		return api.Task{}, invalid("description")
	}
	if s.d.Repos == nil {
		return api.Task{}, api.NewError(api.ErrInternal, "no repository list")
	}
	repo, ok := s.d.Repos.Lookup(req.Repo)
	if !ok || repo.Path == "" {
		return api.Task{}, api.NewError(api.ErrRepoNotFound, "unknown repository").WithParam("path", req.Repo)
	}
	if st, err := os.Stat(repo.Path); err != nil || !st.IsDir() {
		return api.Task{}, api.NewError(api.ErrRepoNotFound, "the repository folder is gone").WithParam("path", repo.Path)
	}
	sl, err := s.checkCap(req.Confirmed)
	if err != nil {
		return api.Task{}, err
	}
	defer sl.release()

	s.mu.Lock()
	s.data.Counters[req.Repo]++
	n := s.data.Counters[req.Repo]
	s.mu.Unlock()

	desc := strings.TrimSpace(req.Description)
	lang := s.d.Lang()
	title, slug, by := newTaskName(lang, n), "new-task-"+itoa(n), api.TaskNamedPending
	if desc != "" {
		title, slug, by = s.nameFromText(ctx, tool, desc, n)
		if err := ctx.Err(); err != nil {
			return api.Task{}, api.NewError(api.ErrCancelled, "cancelled")
		}
	}
	sid, err := s.d.NewSessionID(tool)
	if err != nil {
		return api.Task{}, api.NewError(api.ErrInternal, "cannot make a session ID").WithDetail(err.Error())
	}
	now := s.d.Now()
	rec := &record{
		ID: s.d.NewID(), Repo: req.Repo, Tool: string(tool), Title: title, Slug: slug, NamedBy: string(by), Number: n,
		Where: string(api.TaskWhereMain), Dir: repo.Path, Status: statusLive, CreatedAt: now, StartedAt: now,
	}
	if sid != "" {
		rec.SessionIDs = []string{sid}
	}
	s.mu.Lock()
	s.recs[rec.ID] = rec
	s.order = append(s.order, rec.ID)
	s.busy[rec.ID] = busyLaunch
	s.mu.Unlock()
	launched := false // launch clears the mark itself once the run is stored; a failure leaves it to this return
	defer func() {
		if !launched {
			s.unbusy(rec.ID, busyLaunch)
		}
	}()
	first := req.Description
	if desc == "" {
		first = "" // blank is no first message
	}
	if err := s.launch(rec.ID, false, first, sl); err != nil {
		s.mu.Lock()
		delete(s.recs, rec.ID)
		s.order = slices.DeleteFunc(s.order, func(x string) bool { return x == rec.ID })
		s.mu.Unlock()
		s.removeTaskDir(rec.ID)
		s.changed()
		return api.Task{}, err
	}
	launched = true
	if s.tailHook != nil {
		s.tailHook()
	}
	s.save()
	t, _ := s.Get(rec.ID)
	return t, nil
}

// unbusy clears the task's busy mark only if it is still the caller's kind: a launcher whose run ended meanwhile
// must leave finalize's mark alone.
func (s *Service) unbusy(id string, kind busyKind) {
	s.mu.Lock()
	if s.busy[id] == kind {
		delete(s.busy, id)
	}
	s.idle.Broadcast()
	s.mu.Unlock()
}

func newTaskName(lang string, n int) string {
	if isZH(lang) {
		return "新任务 " + itoa(n)
	}
	return "New task " + itoa(n)
}

// nameFromText names a task from text (the description, or the first message) through the tool, with the quiet
// fallback of plan 4.7: the text's first line cut to 40 characters, and the slug "task-<n>".
func (s *Service) nameFromText(ctx context.Context, tool aitools.ID, text string, n int) (title, slug string, by api.TaskNameSource) {
	if s.d.Name != nil {
		if r, err := s.d.Name(ctx, tool, text, n); err == nil && strings.TrimSpace(r.Title) != "" && r.Slug != "" {
			by = api.TaskNamedFallback
			if r.FromAI {
				by = api.TaskNamedByAI
			}
			return r.Title, r.Slug, by
		}
	}
	return fallbackTitle(text), "task-" + itoa(n), api.TaskNamedFallback
}

// fallbackTitle is the first non-empty line, cut to 40 characters.
func fallbackTitle(text string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if utf8.RuneCountInString(line) > fallbackRunes {
			return strings.TrimSpace(string([]rune(line)[:fallbackRunes]))
		}
		return line
	}
	return ""
}

func mapLaunchError(err error) error {
	switch {
	case errors.Is(err, aitools.ErrBadMessage):
		return invalid("description")
	case errors.Is(err, aitools.ErrBadTool):
		return invalid("tool")
	case errors.Is(err, aitools.ErrBadSessionID):
		return invalid("session")
	case errors.Is(err, aitools.ErrBadDir):
		return invalid("dir")
	}
	var ae *api.AppError
	if errors.As(err, &ae) {
		return err
	}
	return api.NewError(api.ErrInternal, "cannot start the task").WithDetail(err.Error())
}

func mergeEnv(lists ...[]string) []string {
	idx := map[string]int{}
	var out []string
	for _, l := range lists {
		for _, kv := range l {
			k, _, _ := strings.Cut(kv, "=")
			if i, ok := idx[k]; ok {
				out[i] = kv
				continue
			}
			idx[k] = len(out)
			out = append(out, kv)
		}
	}
	return out
}

// launch builds the launch, registers the task with the tracker, starts the terminal and the watcher.
//
// The caller holds the task's busy mark (Create, Resume), so nothing else launches, finalizes or deletes it
// meanwhile. The record is "live" with no run from here until the run is stored: that is the "starting" state.
func (s *Service) launch(id string, resume bool, firstMessage string, sl *slot) (err error) {
	s.mu.Lock()
	real := s.recs[id]
	if real == nil {
		s.mu.Unlock()
		return invalid("task")
	}
	cp := *real
	cp.SessionIDs = slices.Clone(cp.SessionIDs)
	prev := *real
	real.Status = statusLive
	real.Resumed = false // per run: the new run's SessionStart says whether it really resumed
	delete(s.info, id)
	s.mu.Unlock()
	defer func() {
		if err == nil {
			return
		}
		s.mu.Lock()
		if r := s.recs[id]; r != nil {
			r.Status, r.Resumed = prev.Status, prev.Resumed
		}
		s.mu.Unlock()
		s.changed() // the page saw "starting": show the restored state
	}()
	rec := &cp // read-only copy: hooks may add session IDs to the real record meanwhile
	tool := aitools.ID(rec.Tool)
	dir := s.taskDir(rec.ID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return api.NewError(api.ErrFileIO, "cannot create the task's folder").WithParam("file", dir).WithDetail(err.Error())
	}
	sid := ""
	if n := len(rec.SessionIDs); n > 0 {
		sid = rec.SessionIDs[n-1]
	}
	req := aitools.LaunchRequest{Tool: tool, Dir: rec.Dir, TaskID: rec.ID, SessionID: sid, Resume: resume, FirstMessage: firstMessage}
	switch tool {
	case aitools.Claude:
		p, err := taskstate.WriteClaudePlugin(dir, s.d.Exe)
		if err != nil {
			return api.NewError(api.ErrFileIO, "cannot write the task's hook plugin").WithDetail(err.Error())
		}
		req.PluginDirs = []string{p}
	case aitools.OpenCode:
		p, err := taskstate.WriteOpenCodePlugin(dir, s.d.Exe)
		if err != nil {
			return api.NewError(api.ErrFileIO, "cannot write the task's hook plugin").WithDetail(err.Error())
		}
		cfg, err := aitools.OpenCodeConfigContent(p, nil)
		if err != nil {
			return mapLaunchError(err)
		}
		req.OpenCodeConfig = cfg
	case aitools.Codex:
		req.CodexNotify = taskstate.CodexNotifyArgs(s.d.Exe)
	}
	l, err := s.d.Launch(req)
	if err != nil {
		return mapLaunchError(err)
	}
	mode := taskstate.ModeHooks
	if tool == aitools.Codex {
		mode = taskstate.ModeTurnOnly
		if l.NotifySkipped {
			mode = taskstate.ModeNone // the person's own notify program stays; no signal reaches the app
		}
	}
	namePrompt := rec.NamedBy == string(api.TaskNamedPending) && firstMessage == ""
	st := taskstate.Tool(tool)
	s.d.Tracker.Register(rec.ID, st, mode)
	if namePrompt {
		id, n := rec.ID, rec.Number
		s.d.Tracker.ExpectFirstPrompt(id, func(prompt string) { s.nameFromFirstPrompt(id, tool, n, prompt) })
	}
	env := mergeEnv(taskstate.TaskEnv(rec.ID, s.d.Endpoint, namePrompt, false), l.Env)
	sess, err := s.d.Terminals.Start(terminal.StartOptions{ID: rec.ID, Argv: l.Argv, Dir: l.Dir, Env: env})
	if err != nil {
		// this call registered the task and holds the busy mark, so nothing else owns the registration
		s.d.Tracker.Forget(rec.ID)
		s.mu.Lock()
		delete(s.info, rec.ID)
		s.mu.Unlock()
		return err
	}
	r := &run{id: rec.ID, sess: sess}
	now := s.d.Now()
	s.mu.Lock()
	s.runs[rec.ID] = r
	sl.releaseLocked() // the run now counts; not also as pending
	if s.busy[rec.ID] == busyLaunch {
		delete(s.busy, rec.ID) // from here on the run's end (finalize) and a Resume are told apart by the run
		s.idle.Broadcast()
	}
	delete(s.awake, rec.ID)
	if real := s.recs[rec.ID]; real != nil {
		real.Status, real.StartedAt = statusLive, now
		real.EndedAt, real.ExitCode = time.Time{}, nil
	}
	s.mu.Unlock()
	if s.d.Procs != nil {
		s.d.Procs.Track(rec.ID, sess.PTY())
	}
	s.wg.Add(1)
	go s.watch(r)
	s.changed()
	return nil
}

// nameFromFirstPrompt names an empty-description task's label from the first message the tool reported. It
// runs on its own goroutine. The branch (milestone 3) keeps its name; only the label changes, and only while
// the person has not renamed the task.
func (s *Service) nameFromFirstPrompt(id string, tool aitools.ID, n int, prompt string) {
	ctx, cancel := context.WithTimeout(s.ctx, namePromptBudget)
	defer cancel()
	title, _, by := s.nameFromText(ctx, tool, prompt, n)
	if title == "" {
		return
	}
	s.mu.Lock()
	rec := s.recs[id]
	if rec == nil || rec.NamedBy != string(api.TaskNamedPending) {
		s.mu.Unlock()
		return
	}
	rec.Title, rec.NamedBy = title, string(by)
	s.mu.Unlock()
	s.save()
	s.changed()
}

// Resume starts a stopped task again in its own conversation: the newest session ID the task has had, never
// "the latest in this folder".
func (s *Service) Resume(id string, confirmed bool) (api.Task, error) {
	if !terminal.ValidID(id) {
		return api.Task{}, invalid("task")
	}
	s.mu.Lock()
	rec := s.recs[id]
	var cp record
	if rec != nil {
		cp = *rec
	}
	// A run's end being recorded is short (a snapshot, two calls, a save): wait for it, then the task is
	// stopped. A run, or a launch in progress, makes this a second start of the same task.
	for s.busy[id] == busyFinalize {
		s.idle.Wait()
	}
	rec = s.recs[id]
	if rec != nil {
		cp = *rec
	}
	live := s.runs[id] != nil || s.busy[id] != 0
	if rec != nil && !live {
		s.busy[id] = busyLaunch // reserved until launch stored the run or failed
	}
	s.mu.Unlock()
	if rec == nil {
		return api.Task{}, invalid("task")
	}
	if live {
		return api.Task{}, invalid("run")
	}
	launched := false
	defer func() {
		if !launched {
			s.unbusy(id, busyLaunch)
		}
	}()
	if len(cp.SessionIDs) == 0 {
		return api.Task{}, invalid("session") // Codex's ID was never reported: the page offers a new session
	}
	if st, err := os.Stat(cp.Dir); err != nil || !st.IsDir() {
		return api.Task{}, api.NewError(api.ErrRepoNotFound, "the task's folder is gone").WithParam("path", cp.Dir)
	}
	sl, err := s.checkCap(confirmed)
	if err != nil {
		return api.Task{}, err
	}
	defer sl.release()
	if err := s.launch(id, true, "", sl); err != nil {
		return api.Task{}, err
	}
	launched = true
	if s.tailHook != nil {
		s.tailHook()
	}
	s.save()
	t, _ := s.Get(id)
	return t, nil
}

// ---- watching and ending ----------------------------------------------------------------------------------

// watch is the fallback of plan 4.8 and the exit watcher of one run.
func (s *Service) watch(r *run) {
	defer s.wg.Done()
	t := time.NewTicker(s.d.PollEvery)
	defer t.Stop()
	for {
		select {
		case <-r.sess.PTY().Done():
			s.finalize(r)
			return
		case <-s.ctx.Done():
			return
		case <-t.C:
			if s.d.Probe != nil {
				s.d.Tracker.Observe(r.id, s.d.Probe(r.sess.PTY()))
			}
		}
	}
}

// finalize ends a run exactly once, whichever of the exit watcher and Stop gets here first: it records how the
// task ended, tells the tracker (the final end), the process tracker (the session is closed: its programs are
// now "closed, still running") and keep-awake.
func (s *Service) finalize(r *run) {
	r.once.Do(func() {
		code := -1
		select {
		case <-r.sess.PTY().Done():
			code = r.sess.PTY().ExitCode()
		default:
		}
		stopped := r.stopping.Load()
		now := s.d.Now()
		// The session is closed for the process tracker first: from the moment the list says "not live" its
		// programs (if any) already count as "closed, still running". It may wait for a tick in progress.
		if s.d.Procs != nil {
			s.d.Procs.MarkClosed(r.id)
			// MarkClosed keeps the dead shell, tool and children in the set until the next tick: take it now, or
			// a task that just ended would show "closed, still running" for a moment.
			_ = s.d.Procs.Tick()
		}
		s.mu.Lock()
		rec := s.recs[r.id]
		if s.runs[r.id] == r {
			delete(s.runs, r.id)
			s.busy[r.id] = busyFinalize // no Resume between the removal of the run and its final end
		}
		if rec != nil {
			switch {
			case stopped || code < 0:
				rec.Status, rec.ExitCode = statusStopped, nil
			case code == 0:
				c := 0
				rec.Status, rec.ExitCode = statusDone, &c
			default:
				rec.Status, rec.ExitCode = statusFailed, &code
			}
			rec.EndedAt = now
		}
		s.awake[r.id] = keepawake.StateEnded
		s.mu.Unlock()
		defer s.unbusy(r.id, busyFinalize)
		s.d.Tracker.ProcessExited(r.id)
		s.feedAwake(r.id, keepawake.StateEnded)
		if s.d.Awake != nil {
			s.d.Awake.Refresh()
		}
		s.enqueue(func() { s.retractWaiting(r.id) })
		s.save()
		s.changed()
		s.async(s.releaseOld)
	})
}

// releaseOld frees the terminal logs of ended tasks beyond the newest few, once their scrollback file exists (the
// page then reads the file). A task whose file is missing, for example with the setting off, is kept.
func (s *Service) releaseOld() {
	s.mu.Lock()
	var ended []record
	for _, id := range s.order {
		if rec := s.recs[id]; s.runs[id] == nil && rec.Status != statusLive {
			ended = append(ended, *rec)
		}
	}
	s.mu.Unlock()
	slices.SortFunc(ended, func(a, b record) int { return b.EndedAt.Compare(a.EndedAt) })
	for i, rec := range ended {
		if i < keepEnded {
			continue
		}
		if p := terminal.ScrollbackPath(s.d.DataDir, rec.ID); p != "" {
			if _, err := os.Stat(p); err == nil {
				_ = s.d.Terminals.Release(rec.ID)
			}
		}
	}
}

// Stop ends a task: the tool and everything it started in its terminal session. It blocks for the polite stop,
// the grace time and the forced one (seconds): call it off the UI thread. Programs that left the terminal
// session stay and show as "closed, still running" (Force stop is the process tracker's).
func (s *Service) Stop(id string) (api.Task, error) {
	s.mu.Lock()
	r := s.runs[id]
	s.mu.Unlock()
	if r == nil {
		t, ok := s.Get(id)
		if !ok {
			return api.Task{}, invalid("task")
		}
		return t, nil
	}
	r.stopping.Store(true)
	err := s.d.Terminals.Stop(id)
	s.finalize(r)
	t, _ := s.Get(id)
	if err != nil {
		return t, api.NewError(api.ErrInternal, "the task was stopped, but a program may be left").WithDetail(err.Error())
	}
	return t, nil
}

// StopAll is "Stop all and quit" (design 5.6b), without the quit: every task is stopped (in parallel), then
// every other session of the terminal manager (the bottom panel's shells), then every program that survived is
// force-stopped, and the leftovers file is updated by the process tracker. It blocks for up to about a quarter
// of a minute: call it off the UI thread. The quit dialog (A2-14) calls it and then exits the app.
func (s *Service) StopAll() api.StopAllResult {
	s.mu.Lock()
	rs := make([]*run, 0, len(s.runs))
	for _, r := range s.runs {
		r.stopping.Store(true)
		rs = append(rs, r)
	}
	s.mu.Unlock()
	var wg sync.WaitGroup
	for _, r := range rs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = s.d.Terminals.Stop(r.id)
			s.finalize(r)
		}()
	}
	wg.Wait()
	s.d.Terminals.Shutdown(10 * time.Second)
	res := api.StopAllResult{Stopped: len(rs)}
	if s.d.Procs != nil {
		results, err := s.d.Procs.ForceStopAll()
		for _, r := range results {
			res.Survivors += len(r.Survivors)
		}
		if err != nil {
			res.Error = "force-stop"
		}
	}
	s.changed()
	return res
}

// ---- rename, the notice ---------------------------------------------------------------------------

// Rename changes the label only (the branch keeps its name: the AI may already use it).
func (s *Service) Rename(id, title string) (api.Task, error) {
	title = strings.TrimSpace(title)
	if title == "" || utf8.RuneCountInString(title) > maxTitleRunes || strings.ContainsFunc(title, unicode.IsControl) {
		return api.Task{}, invalid("title")
	}
	s.mu.Lock()
	rec := s.recs[id]
	if rec == nil {
		s.mu.Unlock()
		return api.Task{}, invalid("task")
	}
	rec.Title, rec.NamedBy = title, string(api.TaskNamedByPerson)
	s.mu.Unlock()
	s.save()
	s.changed()
	t, _ := s.Get(id)
	return t, nil
}

func (s *Service) removeTaskDir(id string) {
	if s.d.DataDir == "" || !terminal.ValidID(id) {
		return
	}
	dir := s.taskDir(id)
	if filepath.Dir(dir) != s.sessionsDir() {
		return
	}
	_ = os.RemoveAll(dir)
}

// WindowHidden is called when closing the window hid it while tasks run (Lifecycle.CloseAction gave CloseHide).
// The first time ever it sends "Codepass is still running" (design 4.4). It reports whether it sent one. It
// returns at once: the persistence and the notification run on their own goroutine.
func (s *Service) WindowHidden() bool {
	s.mu.Lock()
	n := len(s.runs)
	if n == 0 || s.data.StillRunningNoticed || s.d.Notify == nil {
		s.mu.Unlock()
		return false
	}
	s.data.StillRunningNoticed = true
	s.mu.Unlock()
	s.async(func() {
		s.save()
		s.d.Notify.Send(StillRunningNotice(s.d.Lang(), n))
	})
	return true
}
