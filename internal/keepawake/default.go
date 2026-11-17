package keepawake

import (
	"log/slog"

	"github.com/rudian/codepass/internal/api"
)

// NewDefault builds the manager the app uses: the holders of this platform, the wall clock, the saved settings
// (cfg, read by the caller from settings.json through FromStored) and the function that saves a change (save, which
// writes settings.json; nil = nowhere). leftovers may be nil until A2-13's tracker is connected.
func NewDefault(log *slog.Logger, leftovers Leftovers, cfg api.KeepAwakeConfig, save func(api.KeepAwakeConfig) error) *Manager {
	return New(Options{Holders: PlatformHolders(log), Leftovers: leftovers, Config: Normalize(cfg), Save: save, Log: log})
}
