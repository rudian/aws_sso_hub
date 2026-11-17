package taskstate

import (
	"sync"
	"time"
)

// State is what the task list shows for one task (plan 4.8). "closed, still running", "done / failed" and
// "stopped" belong to the process tracker and the task service; this package only knows the ones below.
type State string

const (
	StateStarting State = "starting"
	StateRunning  State = "running"
	StateWaiting  State = "waiting"
	// StateNoSignal is "running (no signal)": the tool is alive but sends no events (plan 4.8 fallback).
	StateNoSignal State = "running-no-signal"
	// StateEnded: the tool's process is gone (ProcessExited is final; the fallback's verdict can be undone by a
	// later report or an alive observation).
	StateEnded State = "ended"
)

// Reason says why a task is waiting.
type Reason string

const (
	ReasonNone       Reason = ""
	ReasonPermission Reason = "permission"
	ReasonQuestion   Reason = "question" // AskUserQuestion or an MCP elicitation
	ReasonIdle       Reason = "idle"
	ReasonTurnEnd    Reason = "turn_end"
	ReasonLimit      Reason = "limit" // StopFailure rate_limit: "limit reached"; ErrorKind is "rate_limit"
	ReasonError      Reason = "error" // StopFailure with another kind, or an OpenCode failed turn
)

// SignalMode says which signals a task's tool can send.
type SignalMode int

const (
	ModeHooks    SignalMode = iota // full hooks: running and waiting are known
	ModeTurnOnly                   // Codex notify: only "turn complete"
	ModeNone                       // no hook: the fallback only
)

// Info is the state of one task.
type Info struct {
	Task             string
	Tool             Tool
	Mode             SignalMode
	State            State
	Reason           Reason
	Session          string // the tool's own session ID as last reported (cross-check of the ID the app created)
	PermissionMode   string // as of the last event that carried it
	ErrorKind        string
	NotificationType string
	EndMarker        string
	ConvMark         string
	SessionSource    string // SessionStart source: "resume" says a resume really resumed
	Updated          time.Time
}

// Options configures a Tracker.
type Options struct {
	Now func() time.Time
	// NoSignalAfter is how long a task with hooks may stay "starting" while the tool is alive before it is shown
	// as "running (no signal)". Zero means 20 s.
	NoSignalAfter time.Duration
	// ExitedConfirm is how many consecutive "exited" observations of the fallback end a task. Zero means 2: the
	// login shell may be the foreground for a moment before it starts the tool.
	ExitedConfirm int
}

// Tracker holds the state of every registered task. Methods are safe for concurrent use. onChange is called
// without the state lock held, once per change of the visible fields, and a task's changes are delivered in
// the order they happened: a snapshot older than the last one delivered for that task is dropped. Deliveries
// are serialized, so onChange must be quick and must not call Register, Forget, Apply, Observe, OwnerInput or
// ProcessExited (the delivery lock is not reentrant).
type Tracker struct {
	opt      Options
	onChange func(Info)

	mu    sync.Mutex
	tasks map[string]*taskEntry
	ver   uint64 // last version handed out, under mu

	emitMu    sync.Mutex
	delivered map[string]uint64 // task -> version last delivered, under emitMu
}

// snap is one change to deliver: the state and the version it got under mu.
type snap struct {
	info Info
	ver  uint64
}

type taskEntry struct {
	info       Info
	lastAt     int64
	registered time.Time
	exited     bool // the process is gone (ProcessExited): final
	signaled   bool // a report was accepted: the tool's hooks work
	seenAlive  bool // the fallback saw the tool alive (twice in a row for a task without hooks): only then do exited verdicts count
	alive      int  // consecutive alive verdicts
	missed     int
	prompt     func(string) // armed first-prompt receiver
}

// NewTracker returns a Tracker; onChange may be nil.
func NewTracker(o Options, onChange func(Info)) *Tracker {
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.NoSignalAfter <= 0 {
		o.NoSignalAfter = 20 * time.Second
	}
	if o.ExitedConfirm <= 0 {
		o.ExitedConfirm = 2
	}
	return &Tracker{opt: o, onChange: onChange, tasks: map[string]*taskEntry{}, delivered: map[string]uint64{}}
}

// Register starts tracking a task in state "starting". Registering again resets it (a resumed task).
func (t *Tracker) Register(task string, tool Tool, mode SignalMode) {
	now := t.opt.Now()
	t.mu.Lock()
	t.tasks[task] = &taskEntry{info: Info{Task: task, Tool: tool, Mode: mode, State: StateStarting, Updated: now}, registered: now}
	s := t.snapLocked(t.tasks[task])
	t.mu.Unlock()
	t.emit(task, s)
}

// snapLocked takes the next version for e's current state. Call with mu held.
func (t *Tracker) snapLocked(e *taskEntry) snap {
	t.ver++
	return snap{info: e.info, ver: t.ver}
}

// Forget stops tracking a task.
func (t *Tracker) Forget(task string) {
	t.mu.Lock()
	delete(t.tasks, task)
	t.mu.Unlock()
	t.emitMu.Lock()
	delete(t.delivered, task)
	t.emitMu.Unlock()
}

// ExpectFirstPrompt arms the one-shot naming hand-over: the next UserPromptSubmit report of the task calls fn
// once with the prompt, on its own goroutine after the state change was delivered (fn may take its time; it must
// not be waited on). The prompt is not stored and never leaves this call. The launcher must also set
// EnvNamePrompt, or report mode does not forward it.
func (t *Tracker) ExpectFirstPrompt(task string, fn func(prompt string)) {
	t.mu.Lock()
	if e := t.tasks[task]; e != nil {
		e.prompt = fn
	}
	t.mu.Unlock()
}

// Get returns the state of a task.
func (t *Tracker) Get(task string) (Info, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	e := t.tasks[task]
	if e == nil {
		return Info{}, false
	}
	return e.info, true
}

// All returns every task's state.
func (t *Tracker) All() []Info {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]Info, 0, len(t.tasks))
	for _, e := range t.tasks {
		out = append(out, e.info)
	}
	return out
}

// WaitingCount is the number of tasks in state waiting (the sidebar badge).
func (t *Tracker) WaitingCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	n := 0
	for _, e := range t.tasks {
		if e.info.State == StateWaiting {
			n++
		}
	}
	return n
}

func (t *Tracker) emit(task string, s snap) {
	if t.onChange == nil {
		return
	}
	t.emitMu.Lock()
	defer t.emitMu.Unlock()
	if s.ver <= t.delivered[task] {
		return // a newer state of this task was delivered already
	}
	t.delivered[task] = s.ver
	t.onChange(s.info)
}

// Apply feeds one report (already normalized) into the state machine. It reports whether the task's visible
// state changed. Reports for an unknown task, of another tool than registered, older than the last applied one
// (hooks are separate processes and can arrive out of order), or after the process exited are ignored.
func (t *Tracker) Apply(r Report) bool {
	t.mu.Lock()
	e := t.tasks[r.Task]
	if e == nil || e.exited || (r.Tool != "" && r.Tool != e.info.Tool) {
		t.mu.Unlock()
		return false
	}
	if r.At != 0 && r.At < e.lastAt {
		t.mu.Unlock()
		return false
	}
	if r.At != 0 {
		e.lastAt = r.At
	}
	var handPrompt func(string)
	prompt := r.Prompt
	if r.Event == "UserPromptSubmit" && e.prompt != nil && prompt != "" {
		handPrompt, e.prompt = e.prompt, nil
	}
	r.Prompt = "" // never stored
	e.signaled = true
	e.missed = 0 // FU-A2-05b: a report is proof of life; a fallback end needs its verdicts again
	before := e.info
	t.transition(e, r)
	e.info.Updated = t.opt.Now()
	changed := visibleChange(before, e.info)
	var s snap
	if changed {
		s = t.snapLocked(e)
	}
	t.mu.Unlock()

	if changed {
		t.emit(r.Task, s)
	}
	if handPrompt != nil {
		go handPrompt(prompt)
	}
	return changed
}

func visibleChange(a, b Info) bool {
	a.Updated, b.Updated = time.Time{}, time.Time{}
	return a != b
}

func (t *Tracker) transition(e *taskEntry, r Report) {
	i := &e.info
	if r.Session != "" {
		i.Session = r.Session
	}
	if r.Mode != "" {
		i.PermissionMode = r.Mode
	}
	// FU-A2-05b: no guard for a fallback end. Apply refuses reports of a task whose process exited
	// (ProcessExited is final), so any report that gets here proves the tool is alive and revives the task.
	wait := func(reason Reason) { i.State, i.Reason = StateWaiting, reason }
	run := func() {
		i.State, i.Reason = StateRunning, ReasonNone
		i.ErrorKind, i.NotificationType, i.EndMarker, i.ConvMark = "", "", "", ""
	}
	switch r.Event {
	case "SessionStart":
		i.SessionSource = r.Source
		if i.State == StateEnded || (i.State == StateNoSignal && i.Mode == ModeHooks) {
			i.State, i.Reason = StateStarting, ReasonNone
		}
	case "UserPromptSubmit", "session.execution.started", "permission.replied":
		run()
	case "PermissionRequest", "permission.asked":
		wait(ReasonPermission)
	case "PreToolUse":
		wait(ReasonQuestion)
	case "Notification":
		i.NotificationType = r.NotificationType
		switch r.NotificationType {
		case "permission_prompt":
			wait(ReasonPermission)
		case "elicitation_dialog":
			wait(ReasonQuestion)
		case "idle_prompt":
			// A limit or an error is the stronger reason; keep it.
			if i.State != StateWaiting || (i.Reason != ReasonLimit && i.Reason != ReasonError) {
				wait(ReasonIdle)
			}
		case "quota_auto_resume_fired":
			// The tool continues by itself after the reset.
			i.State, i.Reason, i.ErrorKind = StateRunning, ReasonNone, ""
		}
	case "Stop", EventCodexTurnComplete, "session.execution.succeeded", "session.execution.interrupted", "session.idle":
		wait(ReasonTurnEnd)
		if r.EndMarker != "" {
			i.EndMarker = r.EndMarker
		}
	case "StopFailure", "session.execution.failed":
		i.ErrorKind, i.ConvMark = r.ErrorKind, r.ConvMark
		if r.ErrorKind == "rate_limit" {
			wait(ReasonLimit)
		} else {
			wait(ReasonError)
		}
	case EventPluginLoaded:
		// A signal that the plugin works. A task that was shown as "running (no signal)" before the tool had
		// loaded it goes back to starting (FU-A2-05a).
		if i.State == StateNoSignal && i.Mode == ModeHooks {
			i.State, i.Reason = StateStarting, ReasonNone
		}
	case "SessionEnd":
		// No state change: Claude Code sends it for /clear, resume and logout while the tool keeps running.
		// The tool's exit is ProcessExited's job.
	}
}

// ProcessExited ends a task for good: the tool's process is gone (exit code in the task service, A2-08).
func (t *Tracker) ProcessExited(task string) {
	t.mu.Lock()
	e := t.tasks[task]
	if e == nil || e.exited {
		t.mu.Unlock()
		return
	}
	e.exited = true
	changed := e.info.State != StateEnded || e.info.Reason != ReasonNone
	e.info.State, e.info.Reason = StateEnded, ReasonNone
	e.info.Updated = t.opt.Now()
	var s snap
	if changed {
		s = t.snapLocked(e)
	}
	t.mu.Unlock()
	if changed {
		t.emit(task, s)
	}
}

// OwnerInput tells the tracker that the person typed into the task's terminal. No hook fires when a permission
// or question is answered, so a task waiting for that answer goes back to running; a notify-only task (Codex)
// that was waiting at a turn end goes to "running (no signal)" because it cannot say the next turn began.
// Other states are left alone.
func (t *Tracker) OwnerInput(task string) {
	t.mu.Lock()
	e := t.tasks[task]
	if e == nil || e.exited || e.info.State != StateWaiting {
		t.mu.Unlock()
		return
	}
	i := &e.info
	switch {
	case i.Reason == ReasonPermission || i.Reason == ReasonQuestion:
		i.State, i.Reason = StateRunning, ReasonNone
	case i.Reason == ReasonTurnEnd && i.Mode == ModeTurnOnly:
		i.State, i.Reason = StateNoSignal, ReasonNone
	default:
		t.mu.Unlock()
		return
	}
	i.Updated = t.opt.Now()
	s := t.snapLocked(e)
	t.mu.Unlock()
	t.emit(task, s)
}

// Observe feeds one fallback verdict (Probe). Alive keeps or sets "running (no signal)" where no hook says
// better. Exited, seen ExitedConfirm times in a row after the tool was seen alive (or sent a report), shows the
// task as ended; that verdict is not final (a later report revives the task): only ProcessExited is.
func (t *Tracker) Observe(task string, v Verdict) {
	t.mu.Lock()
	e := t.tasks[task]
	if e == nil || e.exited {
		t.mu.Unlock()
		return
	}
	switch v {
	case Alive:
		e.missed = 0
		e.alive++
		// FU-A2-05a: a helper that runs while the login shell starts (path_helper, tty, brew shellenv's ps,
		// starship, zoxide, conda, nvm) is a non-shell process in the foreground for a moment. For a task
		// without hooks one alive verdict is not enough to say the tool ran; two in a row are. A task with hooks
		// never counts verdicts for its end: only a report (signaled) lets the fallback end it.
		if e.info.Mode != ModeHooks && e.alive >= 2 {
			e.seenAlive = true
		}
		noSignal := e.info.Mode != ModeHooks || (!e.signaled && t.opt.Now().Sub(e.registered) >= t.opt.NoSignalAfter)
		if e.info.State == StateEnded {
			// the fallback ended it by mistake (ProcessExited would have set exited). A task with hooks that
			// never sent a report goes back to starting, with the grace time still running.
			noSignal = e.info.Mode != ModeHooks || e.signaled || noSignal
			if !noSignal {
				e.info.State, e.info.Reason = StateStarting, ReasonNone
				e.info.Updated = t.opt.Now()
				s := t.snapLocked(e)
				t.mu.Unlock()
				t.emit(task, s)
				return
			}
		}
		if (e.info.State == StateStarting || e.info.State == StateEnded) && noSignal {
			e.info.State = StateNoSignal
			e.info.Updated = t.opt.Now()
			s := t.snapLocked(e)
			t.mu.Unlock()
			t.emit(task, s)
			return
		}
	case Exited:
		e.alive = 0
		// While the login shell sources its startup files only the shell is in the foreground: that is not an
		// exit. A task with hooks is ended by the fallback only after it has sent a report; any other task only
		// after the tool was seen alive.
		if e.info.Mode == ModeHooks {
			if !e.signaled {
				break
			}
		} else if !e.seenAlive && !e.signaled {
			break
		}
		e.missed++
		if e.missed >= t.opt.ExitedConfirm && (e.info.State != StateEnded || e.info.Reason != ReasonNone) {
			e.info.State, e.info.Reason = StateEnded, ReasonNone
			e.info.Updated = t.opt.Now()
			s := t.snapLocked(e)
			t.mu.Unlock()
			t.emit(task, s)
			return
		}
	}
	t.mu.Unlock()
}
