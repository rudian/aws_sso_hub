package app

import (
	"context"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/rudian/aws_sso_hub/internal/api"
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

func (s *RepoService) ServiceShutdown() error { return nil }

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
