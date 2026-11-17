package app

import (
	"context"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/proctrack"
)

// ProctrackService is the bound service of the process tracker (plan 4.8b, design 5.6a): the list of programs
// that closed sessions left running, and Force stop. Every call except ForceStop and ForceStopAll answers from
// the tracker's cache. ForceStop and ForceStopAll wait up to about ten seconds: Wails runs a bound call on its
// own goroutine, so the window stays free, but Go code must not call them from the UI thread.
//
// Registration (A2-W): add application.RegisterEvent[[]api.SessionLeftovers](api.EventProctrackChanged) to
// events.go, build the tracker with proctrack.New(proctrack.Options{DataDir: c.Paths.AppDataDir}), hand it to
// terminal sessions (Tracker.Track / MarkClosed) and to the keep-awake manager (Tracker.Active), and list
// NewProctrackService(tracker, c.emit) among the application's services.
type ProctrackService struct {
	t    *proctrack.Tracker
	emit func(name string, data any)

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

// NewProctrackService wires the tracker's change callback to the proctrack:changed event.
func NewProctrackService(t *proctrack.Tracker, emit func(string, any)) *ProctrackService {
	s := &ProctrackService{t: t, emit: emit}
	t.SetOnChange(s.changed)
	return s
}

// tracker is for the Go side of the app (terminal sessions, keep-awake, the quit dialog); it is not bound
// because the type has no JSON form.
func (s *ProctrackService) tracker() *proctrack.Tracker { return s.t }

func (s *ProctrackService) changed() {
	if s.emit != nil {
		s.emit(api.EventProctrackChanged, toAPILeftovers(s.t.All()))
	}
}

// ServiceStartup lists what survived a crash (leftovers.json) and starts the ticker.
func (s *ProctrackService) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	_, _ = s.t.Restore() // a damaged or newer file is handled inside; the app starts either way
	rctx, cancel := context.WithCancel(ctx)
	s.mu.Lock()
	s.cancel, s.done = cancel, make(chan struct{})
	done := s.done
	s.mu.Unlock()
	go func() {
		defer close(done)
		s.t.Run(rctx)
	}()
	return nil
}

// ServiceShutdown stops the ticker. It does not stop any program: that is "Stop all and quit" (ForceStopAll).
func (s *ProctrackService) ServiceShutdown() error {
	s.mu.Lock()
	cancel, done := s.cancel, s.done
	s.cancel, s.done = nil, nil
	s.mu.Unlock()
	if cancel != nil {
		cancel()
		<-done
	}
	return nil
}

// List is every session that has tracked programs (live ones too), cached.
func (s *ProctrackService) List() ([]api.SessionLeftovers, error) {
	return nn(toAPILeftovers(s.t.All())), nil
}

// Leftovers is one session's set, cached; Count is 0 when nothing is tracked.
func (s *ProctrackService) Leftovers(session string) (api.SessionLeftovers, error) {
	return nn(toAPISession(s.t.Leftovers(session))), nil
}

// ForceStop ends every program the session started (SIGTERM, about 5 seconds, SIGKILL; Windows: the job is
// terminated) and answers with what was stopped and what survived. It returns an error only when nothing is
// known (the process table could not be read); otherwise Error in the result says what went wrong and
// Survivors lists what is left.
func (s *ProctrackService) ForceStop(session string) (api.ForceStopResult, error) {
	res, err := s.t.ForceStop(session)
	if err != nil && res.Found == 0 && len(res.Survivors) == 0 {
		return nn(toAPIStop(res)), fail(err)
	}
	return nn(toAPIStop(res)), nil
}

// ForceStopAll is "Force stop all" on the AI tasks page and the stop step of "Stop all and quit". The sessions
// are stopped at the same time. Every result is returned; one that failed has Error set.
func (s *ProctrackService) ForceStopAll() ([]api.ForceStopResult, error) {
	rs, _ := s.t.ForceStopAll()
	out := make([]api.ForceStopResult, 0, len(rs))
	for _, r := range rs {
		out = append(out, toAPIStop(r))
	}
	return nn(out), nil
}

// Commands reads the full command lines of a session's tracked programs with one system call. The page asks
// when the person expands the list, not on a timer. The lines are not cached, saved or logged.
func (s *ProctrackService) Commands(session string) ([]api.ProcessCommand, error) {
	cs, err := s.t.Commands(session)
	out := make([]api.ProcessCommand, 0, len(cs))
	for _, c := range cs {
		out = append(out, api.ProcessCommand{PID: c.PID, Command: c.Line})
	}
	return nn(out), fail(err)
}

func toAPIProcs(ps []proctrack.Process) []api.LeftoverProcess {
	out := make([]api.LeftoverProcess, 0, len(ps))
	for _, p := range ps {
		out = append(out, api.LeftoverProcess{PID: p.PID, PPID: p.PPID, Name: p.Name, Command: p.Command, CPU: p.CPU, Memory: p.RSS})
	}
	return out
}

func toAPISession(r proctrack.Report) api.SessionLeftovers {
	o := api.SessionLeftovers{Session: r.Session, Leader: r.Leader, Live: r.Live, Count: r.Count, CPU: r.CPU, Memory: r.RSS, Procs: toAPIProcs(r.Procs)}
	if !r.ClosedAt.IsZero() {
		o.ClosedAt = r.ClosedAt.UTC().Format(time.RFC3339)
	}
	return o
}

func toAPILeftovers(rs []proctrack.Report) []api.SessionLeftovers {
	out := make([]api.SessionLeftovers, 0, len(rs))
	for _, r := range rs {
		out = append(out, toAPISession(r))
	}
	return out
}

func toAPIStop(r proctrack.StopResult) api.ForceStopResult {
	o := api.ForceStopResult{Session: r.Session, Found: r.Found, Stopped: r.Stopped, Names: r.Names, Survivors: toAPIProcs(r.Survivors)}
	if r.Err != nil {
		o.Error = "the stop was not clean"
	}
	return o
}
