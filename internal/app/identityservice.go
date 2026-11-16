package app

import (
	"context"
	"os"
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/identity"
	"github.com/rudian/codepass/internal/projects"
)

// Thin layer over identity.Engine (author identity at three levels, plan 3.4). Every write is a preview
// followed by an apply that carries the preview's hash; every apply returns an UndoID for Undo.

// IdentityService is a bound Wails service.
type IdentityService struct {
	c *Container
	e *identity.Engine
}

// NewIdentityService wires the service. reg is the same projects.Store the container passes to the
// repository engine (the service must not open a second store on projects.json: its snapshots would go stale).
func NewIdentityService(c *Container, reg *projects.Store) *IdentityService {
	e := identity.New(identity.Deps{
		Paths:    c.Paths,
		Git:      c.git,
		Projects: reg,
		Backup:   c.Backup,
		GOOS:     runtime.GOOS,
		Emit:     c.emit,
		OnWrite:  c.noteIdentityWrite,
		// the repository engine keeps the moved-folder questions; a repository that got a project or an identity
		// of its own through this engine must lose its question
		OnRecordData: func(ids ...string) {
			if c.Repos != nil {
				c.Repos.ForgetMovedOffers(ids...)
			}
		},
	})
	return &IdentityService{c: c, e: e}
}

// noteIdentityWrite tells the file watcher about a global config write. content nil with the file gone (an
// undo that removed a file the app had created) records the watcher's "missing" state, not the hash of an
// empty file, so the next poll does not report the app's own change.
func (c *Container) noteIdentityWrite(path string, content []byte) {
	if content == nil {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			c.files.noteRemoved(path)
			return
		}
	}
	c.files.note(path, content)
}

func (s *IdentityService) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	return nil
}

func (s *IdentityService) ServiceShutdown() error { return nil }

// GetOverview: the effective identity table (global card, projects with repositories, ungrouped).
func (s *IdentityService) GetOverview(ctx context.Context) (api.IdentityOverview, error) {
	o, err := s.e.Overview(ctx)
	return nn(o), fail(err)
}

// GetTerminalEnv: identity variables a shell profile exports; refresh forces a new probe.
func (s *IdentityService) GetTerminalEnv(ctx context.Context, refresh bool) (api.TerminalEnvOverride, error) {
	return nn(s.e.TerminalEnv(ctx, refresh)), nil
}

func (s *IdentityService) PreviewGlobal(ctx context.Context, a api.Author) (api.GlobalIdentityPreview, error) {
	p, err := s.e.PreviewGlobal(ctx, a)
	return nn(p), fail(err)
}

func (s *IdentityService) ApplyGlobal(ctx context.Context, a api.Author, baseHash string) (api.IdentityChangeResult, error) {
	r, err := s.e.ApplyGlobal(ctx, a, baseHash)
	return nn(r), fail(err)
}

func (s *IdentityService) PreviewProject(ctx context.Context, req api.ProjectIdentityRequest) (api.ProjectIdentityPreview, error) {
	p, err := s.e.PreviewProject(ctx, req)
	return nn(p), fail(err)
}

func (s *IdentityService) ApplyProject(ctx context.Context, req api.ProjectIdentityRequest, planHash string) (api.IdentityChangeResult, error) {
	r, err := s.e.ApplyProject(ctx, req, planHash)
	return nn(r), fail(err)
}

func (s *IdentityService) PreviewRepo(ctx context.Context, req api.RepoIdentityRequest) (api.RepoIdentityPreview, error) {
	p, err := s.e.PreviewRepo(ctx, req)
	return nn(p), fail(err)
}

func (s *IdentityService) ApplyRepo(ctx context.Context, req api.RepoIdentityRequest, planHash string) (api.IdentityChangeResult, error) {
	r, err := s.e.ApplyRepo(ctx, req, planHash)
	return nn(r), fail(err)
}

// PreviewMove: the identity side of moving a repository into toProjectID ("" = out of every project).
func (s *IdentityService) PreviewMove(ctx context.Context, repoID, toProjectID string) (api.MoveIdentityPreview, error) {
	p, err := s.e.PreviewMove(ctx, repoID, toProjectID)
	return nn(p), fail(err)
}

// ApplyMove: writes the identity change and moves the repository in projects.json.
func (s *IdentityService) ApplyMove(ctx context.Context, repoID, toProjectID, planHash string) (api.IdentityChangeResult, error) {
	r, err := s.e.ApplyMove(ctx, repoID, toProjectID, planHash)
	return nn(r), fail(err)
}

// Undo reverses the apply that returned undoId. The engine keeps what it needs; the page cannot supply paths.
func (s *IdentityService) Undo(ctx context.Context, undoID string) (api.IdentityChangeResult, error) {
	r, err := s.e.Undo(ctx, undoID)
	return nn(r), fail(err)
}
