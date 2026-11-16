//go:build !windows

package watch

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// FU-05a: the folder check never opens the path. A FIFO would block an open until a writer appears.
func TestSubscribeToAFifoFailsAtOnce(t *testing.T) {
	fifo := filepath.Join(t.TempDir(), "pipe")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Skip("no FIFOs here:", err)
	}
	w := newWatcher(fastOpts(), newFake())
	t.Cleanup(w.Close)
	done := make(chan error, 1)
	go func() { _, err := w.Subscribe(fifo, func(Event) {}); done <- err }()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "not a folder") {
			t.Fatalf("%v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Subscribe on a FIFO hangs")
	}
	if _, err := os.Stat(fifo); err != nil {
		t.Fatal(err)
	}
}
