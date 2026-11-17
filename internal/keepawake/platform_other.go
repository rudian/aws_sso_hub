//go:build !darwin && !windows

package keepawake

import "log/slog"

// PlatformHolders: no holder on other systems (the app targets macOS and Windows).
func PlatformHolders(*slog.Logger) []Holder { return nil }
