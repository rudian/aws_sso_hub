package app

import (
	"context"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/rudian/codepass/internal/aitools"
	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/keepawake"
	"github.com/rudian/codepass/internal/proctrack"
	"github.com/rudian/codepass/internal/tasks"
	"github.com/rudian/codepass/internal/taskstate"
	"github.com/rudian/codepass/internal/terminal"
)

// TaskService is the bound Wails service of the AI task lifecycle (plan 4.7, 4.8; A2-08): the task list, start,
// stop, resume and rename, and the two calls of the quit path ("what is live" and "stop all"). The
// lifecycle itself is internal/tasks.Service; this is a thin layer over it.
//
// Registration (A2-W): see NewTasksManager for the construction and the lines that connect the pieces, then
//
//	application.NewService(app.NewTaskService(c.Tasks))
//
// after the proctrack service (its ServiceStartup restores leftovers.json first), and in internal/app/events.go
//
//	application.RegisterEvent[api.TasksState](api.EventTasksChanged)
//	application.RegisterEvent[api.TaskOpen](api.EventTaskOpen)
type TaskService struct{ m *tasks.Service }

// NewTaskService wraps a lifecycle service.
func NewTaskService(m *tasks.Service) *TaskService { return &TaskService{m: m} }

// manager returns the lifecycle service (for tests; the Container holds it for the hooks).
func (s *TaskService) manager() *tasks.Service { return s.m }

// ServiceStartup tells keep-awake and the cached "tasks running" flag about programs the process tracker
// restored from leftovers.json (the service starts after the proctrack service, so they are known by now).
func (s *TaskService) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	s.m.ProcChanged()
	return nil
}

// ServiceShutdown stops the background goroutines. It stops no task: the quit path is StopAll, and the
// terminal manager's own shutdown is the last resort.
func (s *TaskService) ServiceShutdown() error {
	s.m.Close()
	return nil
}

// List is every task with the counts the sidebar and the update dialog read.
func (s *TaskService) List() (api.TasksState, error) { return nn(s.m.Snapshot()), nil }

// Create makes a task and starts its tool (naming first when there is a description: up to 5 seconds). When
// the soft cap is reached it fails with E_INVALID_INPUT, field "cap", parameters "running" and "cap"; asking
// again with Confirmed set starts it.
func (s *TaskService) Create(ctx context.Context, req api.CreateTaskRequest) (api.Task, error) {
	t, err := s.m.Create(ctx, req)
	return nn(t), fail(err)
}

// Stop ends the task's tool and everything it started in its terminal session. It returns when that is done.
func (s *TaskService) Stop(id string) (api.Task, error) {
	t, err := s.m.Stop(id)
	return nn(t), fail(err)
}

// Resume starts a stopped task again in its own conversation.
func (s *TaskService) Resume(id string, confirmed bool) (api.Task, error) {
	t, err := s.m.Resume(id, confirmed)
	return nn(t), fail(err)
}

// Rename changes the task's label only.
func (s *TaskService) Rename(id, title string) (api.Task, error) {
	t, err := s.m.Rename(id, title)
	return nn(t), fail(err)
}

// LiveSummary is what the quit dialog lists.
func (s *TaskService) LiveSummary() (api.LiveSummary, error) { return nn(s.m.LiveSummary()), nil }

// StopAll is "Stop all and quit" without the quit: every task and every other session is stopped and what
// survived is force-stopped. It takes up to about a quarter of a minute.
func (s *TaskService) StopAll() (api.StopAllResult, error) { return nn(s.m.StopAll()), nil }

// ---- construction -------------------------------------------------------------------------------------------

// TasksWiring is what the lifecycle service joins. The Container (A2-W) builds each part once.
type TasksWiring struct {
	Terminal *terminal.Manager // the one terminal manager of the app
	Tracker  *taskstate.Tracker
	// Relay is the onChange function the tracker was built with: taskstate.NewTracker(opts, relay.OnChange).
	Relay *tasks.Relay
	// Endpoint is the hook socket's address (taskstate.Server.Endpoint()).
	Endpoint string
	Proc     *proctrack.Tracker
	Awake    *keepawake.Manager
	AITools  *AIToolService
	// Exe is the app's own binary, which the hooks run in report mode ("" = os.Executable()).
	Exe string
	// ExcludeBackup keeps <data>/sessions out of Time Machine ("" = the real one; tests pass a fake).
	ExcludeBackup func(string) error
}

// NewTasksManager builds the lifecycle service from the Container and the parts. Besides building, it
// connects (so A2-W adds nothing else for these):
//   - the tracker's onChange (the relay);
//   - the terminal manager's input hook (taskstate.CanAnswer filters terminal replies from keystrokes).
//
// A2-W still connects, in this order (the hook socket needs the service's Sink, the service needs the socket's
// Endpoint, so the socket is listened on first and served after):
//
//	srv, err := taskstate.Listen(taskstate.Endpoint(p.AppDataDir))
//	c.Tasks = app.NewTasksManager(c, app.TasksWiring{..., Endpoint: srv.Endpoint()})
//	srv.Serve(c.Tasks.Sink(), ...)
//	defer srv.Close()
//	app.ConnectProctrack(proctrackSvc, c.Tasks) // right after NewProctrackService; one callback, both targets
//
// and Lifecycle.SetTasksRunning(c.Tasks.TasksRunning); the close hook's call to c.Tasks.WindowHidden(); the quit
// path to c.Tasks.StopAll.
func NewTasksManager(c *Container, w TasksWiring) *tasks.Service {
	exe := w.Exe
	if exe == "" {
		exe, _ = os.Executable()
	}
	excl := w.ExcludeBackup
	if excl == nil {
		excl = tasks.ExcludeFromBackup
	}
	d := tasks.Deps{
		DataDir: c.Paths.AppDataDir, Terminals: w.Terminal, Tracker: w.Tracker, Relay: w.Relay,
		Endpoint: w.Endpoint, Exe: exe, Awake: w.Awake, Procs: w.Proc, Notify: c.Notify,
		Repos: repoLookup{c},
		Lang:  c.Settings.UILanguage, Focused: c.windowFocused, ShowWindow: c.RT.ShowWindow, Emit: c.emit, Log: c.Log,
		Now: c.now, ExcludeBackup: excl,
		Probe: func(term taskstate.Terminal) taskstate.Verdict { return taskstate.Probe(term, taskstate.PSTable{}) },
	}
	if w.AITools != nil {
		d.Launch = w.AITools.buildLaunch
		d.Name = func(ctx context.Context, tool aitools.ID, desc string, n int) (tasks.Naming, error) {
			r, err := w.AITools.NameTask(ctx, api.AIToolID(tool), desc, n)
			return tasks.Naming{Title: r.Title, Slug: r.Slug, FromAI: r.FromAI}, err
		}
	}
	svc := tasks.New(d)
	if w.Terminal != nil {
		w.Terminal.SetOnInput(svc.Input)
	}
	return svc
}

// repoLookup finds a repository's folder by its stable ID (F1-18).
type repoLookup struct{ c *Container }

func (r repoLookup) Lookup(id string) (tasks.RepoRef, bool) {
	p, ok := r.c.Repos.RepoPath(id)
	if !ok || p == "" {
		return tasks.RepoRef{}, false
	}
	return tasks.RepoRef{ID: id, Path: p, Name: filepath.Base(p)}, true
}

// ConnectProctrack makes the process tracker's one change callback reach both its targets: the proctrack:changed
// event (NewProctrackService set that) and the task lifecycle (the cached "tasks running" flag, keep-awake, the
// closed-running rows). SetOnChange replaces, so setting either one alone would lose the other. Call it right
// after NewProctrackService.
func ConnectProctrack(ps *ProctrackService, t *tasks.Service) {
	ps.t.SetOnChange(func() {
		ps.changed()
		t.ProcChanged()
	})
}
