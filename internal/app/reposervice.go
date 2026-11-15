package app

import (
	"context"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/repos"
	"github.com/rudian/codepass/internal/watch"
)

// Thin layer over repos.Engine. The engine emits repos:changed and the scan and clone progress events; because
// bindings change the bound-repo counters of the accounts, every mutating call also emits accounts:changed.

// RepoService is a bound Wails service.
type RepoService struct{ c *Container }

// NewRepoService wires the service to the container.
func NewRepoService(c *Container) *RepoService { return &RepoService{c: c} }

func (s *RepoService) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	return nil
}

func (s *RepoService) ServiceShutdown() error {
	if s.c != nil && s.c.Repos != nil {
		s.c.Repos.Close()
	}
	return nil
}

// NewWatchPort adapts the file watcher (internal/watch) to what the repository engine subscribes to. The
// wiring passes it as repos.Deps.Watch.
func NewWatchPort(w *watch.Watcher) repos.WatchPort { return watchPort{w} }

type watchPort struct{ w *watch.Watcher }

func (p watchPort) Subscribe(dir string, fn func(repos.WatchEvent)) (func(), error) {
	return p.w.Subscribe(dir, func(ev watch.Event) {
		switch {
		case ev.Stopped:
			fn(repos.WatchStopped)
		case ev.Removed:
			fn(repos.WatchRemoved)
		default:
			fn(repos.WatchChanged)
		}
	})
}

// accountsChanged refreshes the accounts view after a binding change.
func (s *RepoService) accountsChanged(ctx context.Context) { s.c.Accounts.EmitState(ctx) }

func (s *RepoService) GetState(ctx context.Context) (api.ReposState, error) {
	st, err := s.c.Repos.State(ctx)
	return nn(st), fail(err)
}

func (s *RepoService) Scan(ctx context.Context) (api.ReposState, error) {
	st, err := s.c.Repos.Scan(ctx)
	if err == nil {
		s.accountsChanged(ctx)
	}
	return nn(st), fail(err)
}

func (s *RepoService) DetectOwners(ctx context.Context, repoIDs []string) (api.ReposState, error) {
	st, err := s.c.Repos.DetectOwners(ctx, repoIDs)
	return nn(st), fail(err)
}

func (s *RepoService) CompareCommits(ctx context.Context, repoID string) (api.Repo, error) {
	r, err := s.c.Repos.CompareCommits(ctx, repoID)
	return nn(r), fail(err)
}

func (s *RepoService) PreviewBind(ctx context.Context, repoID string, profile string) (api.BindPreview, error) {
	p, err := s.c.Repos.PreviewBind(ctx, repoID, profile)
	return nn(p), fail(err)
}

func (s *RepoService) Bind(ctx context.Context, repoID string, profile string) (api.BindOutcome, error) {
	o, err := s.c.Repos.Bind(ctx, repoID, profile)
	if err == nil {
		s.accountsChanged(ctx)
	}
	return nn(o), fail(err)
}

func (s *RepoService) BindMany(ctx context.Context, items []api.BindItem) ([]api.BindOutcome, error) {
	o, err := s.c.Repos.BindMany(ctx, items)
	if err == nil {
		s.accountsChanged(ctx)
	}
	return nn(o), fail(err)
}

func (s *RepoService) Unbind(ctx context.Context, repoID string) (api.BindOutcome, error) {
	o, err := s.c.Repos.Unbind(ctx, repoID)
	if err == nil {
		s.accountsChanged(ctx)
	}
	return nn(o), fail(err)
}

func (s *RepoService) TestConnection(ctx context.Context, repoID string) (api.TestResult, error) {
	r, err := s.c.Repos.Test(ctx, repoID)
	return r, fail(err)
}

func (s *RepoService) TestMany(ctx context.Context, repoIDs []string) ([]api.TestResult, error) {
	r, err := s.c.Repos.TestMany(ctx, repoIDs)
	return nn(r), fail(err)
}

func (s *RepoService) ConvertToHTTPS(ctx context.Context, repoID string, profile string) (api.BindOutcome, error) {
	o, err := s.c.Repos.ConvertToHTTPS(ctx, repoID, profile)
	if err == nil {
		s.accountsChanged(ctx)
	}
	return nn(o), fail(err)
}

func (s *RepoService) RepairBindings(ctx context.Context) ([]api.BindOutcome, error) {
	o, err := s.c.Repos.RepairAll(ctx)
	if err == nil {
		s.accountsChanged(ctx)
	}
	return nn(o), fail(err)
}

// AddExisting adds a repository that is already on disk, detects its AWS account and, when exactly one SSO
// profile fits, binds it.
func (s *RepoService) AddExisting(ctx context.Context, path string) (api.AddRepoResult, error) {
	res, err := s.c.Repos.AddExisting(ctx, path)
	if err == nil && res.AutoBound {
		s.accountsChanged(ctx)
	}
	return nn(res), fail(err)
}

// RemoveExtra stops listing a repository that was added by hand. The folder is not touched.
func (s *RepoService) RemoveExtra(ctx context.Context, repoID string) (api.ReposState, error) {
	st, err := s.c.Repos.RemoveExtra(ctx, repoID)
	return nn(st), fail(err)
}

func (s *RepoService) OpenFolder(repoID string) error {
	path, ok := s.c.Repos.RepoPath(repoID)
	if !ok {
		return fail(api.NewError(api.ErrRepoNotFound, "repository not found").WithParam("path", repoID))
	}
	return fail(s.c.RT.RevealPath(s.c.Paths.Expand(path)))
}

func (s *RepoService) ListRemoteRepos(ctx context.Context, profile string, region string) ([]api.RemoteRepo, error) {
	r, err := s.c.Repos.ListRemote(ctx, profile, region)
	return nn(r), fail(err)
}

func (s *RepoService) PreviewClone(req api.CloneRequest) (api.ClonePreview, error) {
	p, err := s.c.Repos.PreviewClone(context.Background(), req)
	return p, fail(err)
}

func (s *RepoService) Clone(ctx context.Context, req api.CloneRequest) (api.BindOutcome, error) {
	o, err := s.c.Repos.Clone(ctx, req)
	if err == nil {
		s.accountsChanged(ctx)
	}
	return nn(o), fail(err)
}

// AddFolder lists any git repository that is on disk (CodeCommit or not). A CodeCommit one is handled like
// AddExisting; any other is shown at once, with no binding. A hidden repository comes back.
func (s *RepoService) AddFolder(ctx context.Context, path string) (api.AddRepoResult, error) {
	res, err := s.c.Repos.AddFolder(ctx, path)
	if err == nil && res.AutoBound {
		s.accountsChanged(ctx)
	}
	return nn(res), fail(err)
}

// ReattachMoved answers "yes" to "Is this <name>, moved here?" for the repository newID: the old record (project,
// hidden flag, identity, binding, test result) takes this folder. The folder itself is never touched.
// Errors: E_MOVED_HAS_DATA, E_MOVED_GONE.
func (s *RepoService) ReattachMoved(ctx context.Context, newID string) (api.ReposState, error) {
	st, err := s.c.Repos.ReattachMoved(ctx, newID)
	if err == nil {
		s.accountsChanged(ctx)
	}
	return nn(st), fail(err)
}

// DeclineMoved answers "no": the folder is a new repository and the question is not asked again.
func (s *RepoService) DeclineMoved(ctx context.Context, newID string) (api.ReposState, error) {
	st, err := s.c.Repos.DeclineMoved(ctx, newID)
	return nn(st), fail(err)
}

// ResolveFound answers the one-time review of newly found repositories: show joins the list, hide hides.
func (s *RepoService) ResolveFound(ctx context.Context, show []string, hide []string) (api.ReposState, error) {
	st, err := s.c.Repos.ResolveFound(ctx, show, hide)
	return nn(st), fail(err)
}

// SetHidden hides a repository (it leaves the list, the counts and all background work) or brings it back.
func (s *RepoService) SetHidden(ctx context.Context, repoID string, hidden bool) (api.ReposState, error) {
	st, err := s.c.Repos.SetHidden(ctx, repoID, hidden)
	return nn(st), fail(err)
}

// MoveToProject puts repositories into a project; an empty projectID takes them out of any project.
func (s *RepoService) MoveToProject(ctx context.Context, repoIDs []string, projectID string) (api.ReposState, error) {
	st, err := s.c.Repos.MoveToProject(ctx, repoIDs, projectID)
	return nn(st), fail(err)
}

func (s *RepoService) CreateProject(ctx context.Context, name string) (api.ReposState, error) {
	st, err := s.c.Repos.CreateProject(ctx, name)
	return nn(st), fail(err)
}

// CreateProjectWith makes a project and moves the repositories into it (a suggestion's "Create project").
func (s *RepoService) CreateProjectWith(ctx context.Context, name string, repoIDs []string) (api.ReposState, error) {
	st, err := s.c.Repos.CreateProjectWith(ctx, name, repoIDs)
	return nn(st), fail(err)
}

func (s *RepoService) RenameProject(ctx context.Context, projectID string, name string) (api.ReposState, error) {
	st, err := s.c.Repos.RenameProject(ctx, projectID, name)
	return nn(st), fail(err)
}

// RemoveProject deletes a project; its repositories become ungrouped. Nothing on disk is touched.
func (s *RepoService) RemoveProject(ctx context.Context, projectID string) (api.ReposState, error) {
	st, err := s.c.Repos.RemoveProject(ctx, projectID)
	return nn(st), fail(err)
}

func (s *RepoService) SetProjectCollapsed(ctx context.Context, projectID string, collapsed bool) (api.ReposState, error) {
	st, err := s.c.Repos.SetProjectCollapsed(ctx, projectID, collapsed)
	return nn(st), fail(err)
}

// DismissSuggestion says "no thanks" to a project suggestion (ReposState.Suggestions[].Key) for good.
func (s *RepoService) DismissSuggestion(ctx context.Context, key string) (api.ReposState, error) {
	st, err := s.c.Repos.DismissSuggestion(ctx, key)
	return nn(st), fail(err)
}

// RefreshStatus re-reads the git status of listed repositories (empty = all). Hidden repositories are skipped.
func (s *RepoService) RefreshStatus(ctx context.Context, repoIDs []string) (api.ReposState, error) {
	var ids []string
	if len(repoIDs) > 0 {
		ids = repoIDs
	}
	st, err := s.c.Repos.RefreshStatus(ctx, ids)
	return nn(st), fail(err)
}

// MarkOpened tells the engine the person opened a repository: it then takes part in background fetch.
func (s *RepoService) MarkOpened(repoID string) error {
	return fail(s.c.Repos.MarkOpened(repoID))
}

// FetchNow fetches one repository's remote (the person asked, or the app's schedule). It refuses a repository
// that is hidden, or neither bound nor opened.
func (s *RepoService) FetchNow(ctx context.Context, repoID string) error {
	return fail(s.c.Repos.Fetch(ctx, repoID))
}
