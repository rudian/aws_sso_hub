package app

import (
	"context"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/rudian/aws_sso_hub/internal/api"
)

// Thin layer over gitglobal.Engine. The engine emits repos:changed; changes also move the bound-repo counters,
// so accounts:changed is emitted too.

// GitGlobalService is the bound Wails service for the global git credential configuration.
type GitGlobalService struct{ c *Container }

// NewGitGlobalService wires the service to the container.
func NewGitGlobalService(c *Container) *GitGlobalService { return &GitGlobalService{c: c} }

func (s *GitGlobalService) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	return nil
}

func (s *GitGlobalService) ServiceShutdown() error { return nil }

func (s *GitGlobalService) GetState(ctx context.Context) (api.GitGlobalState, error) {
	st, err := s.c.Global.State(ctx)
	return nn(st), fail(err)
}

func (s *GitGlobalService) PreviewDefaultChange(ctx context.Context, req api.DefaultChangeRequest) (api.DefaultChangePreview, error) {
	p, err := s.c.Global.PreviewDefault(ctx, req)
	return nn(p), fail(err)
}

func (s *GitGlobalService) ApplyDefaultChange(ctx context.Context, req api.DefaultChangeRequest, baseHash string) (api.GlobalChangeResult, error) {
	r, err := s.c.Global.ApplyDefault(ctx, req, baseHash)
	if err == nil {
		s.c.Accounts.EmitState(ctx)
	}
	return nn(r), fail(err)
}

func (s *GitGlobalService) PreviewURLRuleMigration(ctx context.Context, pattern string) (api.URLRulePlan, error) {
	p, err := s.c.Global.PreviewURLRule(ctx, pattern)
	return nn(p), fail(err)
}

func (s *GitGlobalService) MigrateURLRule(ctx context.Context, m api.URLRuleMigration, baseHash string) (api.URLRuleMigrationResult, error) {
	r, err := s.c.Global.MigrateURLRule(ctx, m, baseHash)
	if err == nil {
		s.c.Accounts.EmitState(ctx)
	}
	return nn(r), fail(err)
}

func (s *GitGlobalService) ListSavedCredentials(ctx context.Context) ([]api.SavedCredential, error) {
	r, err := s.c.Global.ListSaved(ctx)
	return nn(r), fail(err)
}

// ClearSavedCredentials: empty ids = all listed
func (s *GitGlobalService) ClearSavedCredentials(ctx context.Context, ids []string) (int, error) {
	n, err := s.c.Global.ClearSaved(ctx, ids)
	return n, fail(err)
}
