package update

import (
	"os"
	"strconv"
	"syscall"
	"testing"
	"time"
)

// If a test ever reaches a successful Restart, Wails re-runs this binary as the swap helper. Without this guard the
// helper would run the whole suite again, which starts another helper. Answer "not a helper" at once instead.
func TestMain(m *testing.M) {
	if os.Getenv("WAILS_UPDATER_HELPER") == "1" {
		if os.Getenv("CODEPASS_TEST_HELPER_WAITS") == "1" {
			// A helper that answers "ready" and then waits for the app to exit, like the real one, but never swaps.
			if ready := os.Getenv("WAILS_UPDATER_HELPER_READY"); ready != "" {
				_ = os.WriteFile(ready, []byte("ready"), 0o600)
			}
			pid, _ := strconv.Atoi(os.Getenv("WAILS_UPDATER_HELPER_PID"))
			for i := 0; i < 600 && pid > 0 && alive(pid); i++ {
				time.Sleep(100 * time.Millisecond)
			}
			os.Exit(0)
		}
		if ready := os.Getenv("WAILS_UPDATER_HELPER_READY"); ready != "" {
			_ = os.WriteFile(ready, []byte("error: test binary is not an update helper"), 0o600)
		}
		os.Exit(2)
	}
	os.Exit(m.Run())
}

func alive(pid int) bool {
	p, err := os.FindProcess(pid)
	return err == nil && p.Signal(syscall.Signal(0)) == nil
}
