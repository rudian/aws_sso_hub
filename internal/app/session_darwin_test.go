//go:build darwin

package app

import "testing"

// FU-03b: the power-off observer is registered from NewLifecycle, once, not in a package init.
func TestPowerOffObserverRegistersOnceFromNewLifecycle(t *testing.T) {
	NewLifecycle()
	NewLifecycle()
	if n := sessionWatchCount(); n != 1 {
		t.Fatalf("the observer was registered %d times", n)
	}
}
