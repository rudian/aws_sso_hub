package app

import (
	"context"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/rudian/aws_sso_hub/internal/api"
)

// Thin layer over migrate.Engine; the engine emits migration:progress and accounts:changed itself.

// MigrationService is the bound Wails service for the static-key to SSO migration wizard.
type MigrationService struct{ c *Container }

// NewMigrationService wires the service to the container.
func NewMigrationService(c *Container) *MigrationService { return &MigrationService{c: c} }

func (s *MigrationService) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	return nil
}

func (s *MigrationService) ServiceShutdown() error { return nil }

func (s *MigrationService) Start(ctx context.Context, profile string) (api.MigrationState, error) {
	st, err := s.c.Migrate.Start(ctx, profile)
	return nn(st), fail(err)
}

func (s *MigrationService) Get(profile string) (api.MigrationState, error) {
	st, err := s.c.Migrate.Get(profile)
	return nn(st), fail(err)
}

func (s *MigrationService) ChoosePortal(profile string, portal string) (api.MigrationState, error) {
	st, err := s.c.Migrate.ChoosePortal(profile, portal)
	return nn(st), fail(err)
}

func (s *MigrationService) ChooseAccount(profile string, choice api.MigrationAccountChoice) (api.MigrationState, error) {
	st, err := s.c.Migrate.ChooseAccount(profile, choice)
	return nn(st), fail(err)
}

func (s *MigrationService) Preview(profile string) (api.MigrationState, error) {
	st, err := s.c.Migrate.Preview(profile)
	return nn(st), fail(err)
}

func (s *MigrationService) Apply(ctx context.Context, profile string, baseHash string) (api.MigrationState, error) {
	st, err := s.c.Migrate.Apply(ctx, profile, baseHash)
	return nn(st), fail(err)
}

func (s *MigrationService) RetryVerify(ctx context.Context, profile string) (api.MigrationState, error) {
	st, err := s.c.Migrate.RetryVerify(ctx, profile)
	return nn(st), fail(err)
}

func (s *MigrationService) RestoreConfig(profile string) (api.MigrationState, error) {
	st, err := s.c.Migrate.RestoreConfig(profile)
	return nn(st), fail(err)
}

func (s *MigrationService) PreviewKeyRemoval(profile string) (api.MigrationState, error) {
	st, err := s.c.Migrate.PreviewKeyRemoval(profile)
	return nn(st), fail(err)
}

func (s *MigrationService) RemoveKey(profile string, baseHash string) (api.MigrationState, error) {
	st, err := s.c.Migrate.RemoveKey(profile, baseHash)
	return nn(st), fail(err)
}

func (s *MigrationService) KeepKey(profile string) (api.MigrationState, error) {
	st, err := s.c.Migrate.KeepKey(profile)
	return nn(st), fail(err)
}

func (s *MigrationService) Deactivate(ctx context.Context, profile string) (api.MigrationState, error) {
	st, err := s.c.Migrate.Deactivate(ctx, profile)
	return nn(st), fail(err)
}

func (s *MigrationService) Finish(profile string) (api.MigrationState, error) {
	st, err := s.c.Migrate.Finish(profile)
	return nn(st), fail(err)
}

func (s *MigrationService) Discard(profile string) error {
	s.c.Migrate.Discard(profile)
	return nil
}
