package app

import (
	"context"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/keepawake"
)

// KeepAwakeService is the bound service of the keep-awake manager (plan 4.10, A2-03): the state for the chip, the
// AI tasks banner and the Settings card, and the card's settings. The manager itself is fed by Go code (the
// terminal sessions, the task state signals, the process tracker), never by the window, so no feeding method is
// bound here.
type KeepAwakeService struct {
	m  *keepawake.Manager
	rt Runtime
}

// NewKeepAwakeService wires the service to the manager (build it with keepawake.NewDefault).
func NewKeepAwakeService(m *keepawake.Manager, rt Runtime) *KeepAwakeService {
	return &KeepAwakeService{m: m, rt: rt}
}

// manager returns the manager, for tests (the feeders get the *Manager from main.go).
func (s *KeepAwakeService) manager() *keepawake.Manager { return s.m }

// ServiceStartup starts forwarding changes to the window as the keepawake:changed event.
func (s *KeepAwakeService) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	s.m.SetOnChange(func(st api.KeepAwakeState) { s.rt.Emit(api.EventKeepAwakeChanged, st) })
	return nil
}

// ServiceShutdown releases every assertion. It runs on every normal exit; a crash or a force kill is covered by
// caffeinate's -w (macOS) and by the operating system (Windows).
func (s *KeepAwakeService) ServiceShutdown() error {
	s.m.Close()
	return nil
}

// GetState is the current state.
func (s *KeepAwakeService) GetState() (api.KeepAwakeState, error) { return s.m.State(), nil }

// GetConfig is the settings of the Keep awake card.
func (s *KeepAwakeService) GetConfig() (api.KeepAwakeConfig, error) { return s.m.Config(), nil }

// SetConfig applies and saves the settings; a value the card does not offer is E_INVALID_INPUT. The hold changes
// at once (turning it off releases it even with tasks running).
func (s *KeepAwakeService) SetConfig(c api.KeepAwakeConfig) (api.KeepAwakeState, error) {
	if err := s.m.SetConfig(c); err != nil {
		return api.KeepAwakeState{}, fail(err)
	}
	return s.m.State(), nil
}
