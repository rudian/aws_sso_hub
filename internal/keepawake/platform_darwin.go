//go:build darwin

package keepawake

import (
	"log/slog"
	"os"
)

// PlatformHolders returns the macOS holders: caffeinate (restarted if killed) and, with cgo, the NSProcessInfo
// activity.
func PlatformHolders(log *slog.Logger) []Holder {
	hs := []Holder{&Caffeinate{Runner: ExecRunner(), Clock: RealClock(), Log: log, PID: os.Getpid()}}
	if api := platformActivityAPI(); api != nil {
		hs = append(hs, &Activity{API: api})
	}
	return hs
}
