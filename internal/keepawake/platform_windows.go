//go:build windows

package keepawake

import "log/slog"

// PlatformHolders returns the Windows holder: SetThreadExecutionState on one locked thread. Manager.Close releases
// it (ES_CONTINUOUS is set alone, which clears the requirements); the locked thread ends with the process.
func PlatformHolders(*slog.Logger) []Holder {
	return []Holder{&ExecState{Set: NewSetStateFunc()}}
}
