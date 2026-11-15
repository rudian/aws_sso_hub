//go:build darwin && cgo

package watch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Every started FSEvents stream is stopped again by unsubscribe, pause and
// Close: file descriptors cannot show this on macOS, so count the streams.
func TestNativeLiveStreamsReturnToBaseline(t *testing.T) {
	dir := t.TempDir()
	base := liveStreams.Load()
	w := New(nativeOpts())
	for i := 0; i < 50; i++ {
		un, err := w.Subscribe(dir, func(Event) {})
		if err != nil {
			t.Fatal(err)
		}
		if got := liveStreams.Load(); got != base+1 {
			t.Fatalf("live streams %d, want %d", got, base+1)
		}
		w.Pause(dir)
		if got := liveStreams.Load(); got != base {
			t.Fatalf("live streams after Pause %d, want %d", got, base)
		}
		w.Resume(dir)
		un()
		if got := liveStreams.Load(); got != base {
			t.Fatalf("live streams after unsubscribe %d, want %d", got, base)
		}
	}
	if _, err := w.Subscribe(dir, func(Event) {}); err != nil {
		t.Fatal(err)
	}
	w.Close()
	if got := liveStreams.Load(); got != base {
		t.Fatalf("live streams after Close %d, want %d", got, base)
	}
}

// A root spelled with another letter case than on disk must still filter
// node_modules writes and report normal writes with their paths.
func TestNativeDifferentlyCasedRoot(t *testing.T) {
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	disk := filepath.Join(parent, "RepoDir")
	os.MkdirAll(filepath.Join(disk, "node_modules"), 0o755)
	spelled := filepath.Join(parent, strings.ToLower("RepoDir"))
	if _, err := os.Stat(spelled); err != nil {
		t.Skip("case-sensitive volume")
	}
	_, ch := startNative(t, spelled)
	drain(ch, 600*time.Millisecond) // the setup's own mkdir may still be in flight
	for i := 0; i < 20; i++ {
		os.WriteFile(filepath.Join(disk, "node_modules", "f"+string(rune('a'+i))), []byte("x"), 0o644)
	}
	if evs := drain(ch, 800*time.Millisecond); len(evs) != 0 {
		t.Fatalf("node_modules writes were reported: %+v", evs)
	}
	target := filepath.Join(spelled, "main.go")
	os.WriteFile(target, []byte("x"), 0o644)
	deadline := time.After(6 * time.Second)
	for {
		select {
		case ev := <-ch:
			if ev.Overflow && len(ev.Paths) == 0 {
				t.Fatalf("overflow instead of a path: %+v", ev)
			}
			for _, p := range ev.Paths {
				if p == target {
					return
				}
			}
		case <-deadline:
			t.Fatal("no event with the caller's spelling of the path")
		}
	}
}
