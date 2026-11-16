//go:build windows || (darwin && cgo)

package watch

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// These tests use the real operating-system backend.

func nativeOpts() Options { return Options{Debounce: 150 * time.Millisecond, MaxWait: time.Second} }

func waitEvent(t *testing.T, ch chan Event, d time.Duration) (Event, bool) {
	t.Helper()
	select {
	case e := <-ch:
		return e, true
	case <-time.After(d):
		return Event{}, false
	}
}

func drain(ch chan Event, d time.Duration) []Event { return collect(ch, d) }

// withoutScanHints drops the events a loaded machine legitimately sends (FSEvents' MustScanSubDirs or dropped
// events arrive as an Overflow with no path): the "silent" tests fail on an event that names a path, not on
// a request to rescan.
func withoutScanHints(evs []Event) []Event {
	var out []Event
	for _, e := range evs {
		if e.Overflow && len(e.Paths) == 0 {
			continue
		}
		out = append(out, e)
	}
	return out
}

func startNative(t *testing.T, dir string) (*Watcher, chan Event) {
	t.Helper()
	w := New(nativeOpts())
	t.Cleanup(w.Close)
	ch := make(chan Event, 1000)
	if _, err := w.Subscribe(dir, func(e Event) { ch <- e }); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond) // let the stream start
	return w, ch
}

func TestNativeWriteIsReported(t *testing.T) {
	dir := t.TempDir()
	_, ch := startNative(t, dir)
	os.MkdirAll(filepath.Join(dir, "src"), 0o755)
	target := filepath.Join(dir, "src", "main.go")
	os.WriteFile(target, []byte("package main"), 0o644)
	deadline := time.After(6 * time.Second)
	for {
		select {
		case ev := <-ch:
			for _, p := range ev.Paths {
				if p == target {
					return
				}
			}
		case <-deadline:
			t.Fatalf("no event naming %s", target)
		}
	}
}

func TestNativeBurstOf10000Writes(t *testing.T) {
	dir := t.TempDir()
	_, ch := startNative(t, dir)
	start := time.Now()
	for i := 0; i < 10000; i++ {
		os.WriteFile(filepath.Join(dir, fmt.Sprintf("f%d.txt", i%100)), []byte("x"), 0o644)
	}
	t.Logf("10000 writes took %v", time.Since(start))
	evs := drain(ch, 2500*time.Millisecond)
	if len(evs) == 0 {
		t.Fatal("no events")
	}
	// One event per second of burst, plus the tail.
	limit := int(time.Since(start)/time.Second) + 3
	if len(evs) > limit {
		t.Fatalf("%d events for one burst (limit %d)", len(evs), limit)
	}
}

func TestNativeNodeModulesIsSilent(t *testing.T) {
	dir := t.TempDir()
	nm := filepath.Join(dir, "node_modules", "pkg")
	os.MkdirAll(nm, 0o755)
	os.MkdirAll(filepath.Join(dir, ".git", "objects", "ab"), 0o755)
	_, ch := startNative(t, dir)
	drain(ch, 600*time.Millisecond) // setup events that FSEvents delivered late
	for i := 0; i < 500; i++ {
		os.WriteFile(filepath.Join(nm, fmt.Sprintf("f%d.js", i)), []byte("x"), 0o644)
		os.WriteFile(filepath.Join(dir, ".git", "objects", "ab", fmt.Sprintf("o%d", i)), []byte("x"), 0o644)
	}
	if evs := withoutScanHints(drain(ch, 1200*time.Millisecond)); len(evs) != 0 {
		t.Fatalf("got %d events: %+v", len(evs), evs[0])
	}
}

func TestNativeRenameInside(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.txt")
	os.WriteFile(a, []byte("x"), 0o644)
	time.Sleep(300 * time.Millisecond)
	_, ch := startNative(t, dir)
	b := filepath.Join(dir, "b.txt")
	os.Rename(a, b)
	ev, ok := waitEvent(t, ch, 5*time.Second)
	if !ok {
		t.Fatal("no event")
	}
	if ev.Count < 1 {
		t.Fatalf("event = %+v", ev)
	}
}

func TestNativeFolderRemoved(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "repo")
	os.Mkdir(dir, 0o755)
	_, ch := startNative(t, dir)
	os.RemoveAll(dir)
	deadline := time.After(6 * time.Second)
	for {
		select {
		case ev := <-ch:
			if ev.Removed {
				return
			}
		case <-deadline:
			t.Fatal("no Removed event")
		}
	}
}

func TestNativeFolderRenamedAwayWhilePaused(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "repo")
	moved := filepath.Join(parent, "repo2")
	os.Mkdir(dir, 0o755)
	w, ch := startNative(t, dir)
	if err := w.Pause(dir); err != nil {
		t.Fatal(err)
	}
	// The watcher opens with FILE_SHARE_DELETE, so this rename also succeeds with
	// a handle left open; native_windows_test.go checks the handle itself.
	if err := os.Rename(dir, moved); err != nil {
		t.Fatal(err)
	}
	if err := w.Relocate(dir, moved); err != nil {
		t.Fatal(err)
	}
	if err := w.Resume(moved); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	drain(ch, 400*time.Millisecond)
	os.WriteFile(filepath.Join(moved, "x.txt"), []byte("x"), 0o644)
	ev, ok := waitEvent(t, ch, 5*time.Second)
	if !ok || ev.Folder != moved {
		t.Fatalf("event = %+v ok=%v", ev, ok)
	}
}

func TestNativeBoundedHandles(t *testing.T) {
	count := func() int {
		n, ok := openHandles()
		if !ok {
			t.Skip("no handle counter on this platform")
		}
		return n
	}
	w := New(nativeOpts())
	defer w.Close()
	// Warm up: the first watch creates the Watcher's own long-lived resources
	// (on Windows the completion port and its dispatcher thread, with the
	// runtime handles that thread holds). They are not a leak and not per folder.
	for i := 0; i < 5; i++ {
		un, err := w.Subscribe(t.TempDir(), func(Event) {})
		if err != nil {
			t.Fatal(err)
		}
		un()
	}
	time.Sleep(300 * time.Millisecond)
	before := count()
	var unsubs []func()
	for i := 0; i < 30; i++ {
		d := t.TempDir()
		for j := 0; j < 50; j++ {
			sub := filepath.Join(d, fmt.Sprintf("d%d", j))
			os.Mkdir(sub, 0o755)
			os.WriteFile(filepath.Join(sub, "f"), []byte("x"), 0o644)
		}
		un, err := w.Subscribe(d, func(Event) {})
		if err != nil {
			t.Fatal(err)
		}
		unsubs = append(unsubs, un)
	}
	time.Sleep(500 * time.Millisecond)
	during := count()
	t.Logf("%s handles before=%d during=%d (30 folders, 1500 files)", runtime.GOOS, before, during)
	// Windows: one directory handle per folder, all on one completion port
	// drained by one thread, so no per-folder events or threads.
	limit := 10
	if runtime.GOOS == "windows" {
		limit = 30 + 10
	}
	if during-before > limit {
		t.Fatalf("handle growth %d for 30 watched folders (limit %d)", during-before, limit)
	}
	for _, un := range unsubs {
		un()
	}
	time.Sleep(300 * time.Millisecond)
	if after := count(); after-before > 3 {
		t.Fatalf("handles leaked after unsubscribe: before=%d after=%d", before, after)
	}
}

func TestNativeSubscribeUnsubscribeCycleLeaksNothing(t *testing.T) {
	dir := t.TempDir()
	w := New(nativeOpts())
	defer w.Close()
	cycle := func() {
		for i := 0; i < 100; i++ {
			un, err := w.Subscribe(dir, func(Event) {})
			if err != nil {
				t.Fatal(err)
			}
			if i%2 == 0 {
				w.Pause(dir)
				w.Resume(dir)
			}
			un()
		}
	}
	cycle() // warm up lazily created runtime handles
	before, ok := openHandles()
	cycle()
	if after, _ := openHandles(); ok && after-before > 3 {
		t.Fatalf("handles grew over 100 cycles: before=%d after=%d", before, after)
	}
}

// The folder is moved away (as the Trash does), comes back, and a fresh
// Subscribe must watch it again.
func TestNativeSubscribeAfterMoveAwayAndBack(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "repo")
	away := filepath.Join(parent, "away")
	os.Mkdir(dir, 0o755)
	w, ch := startNative(t, dir)
	if err := os.Rename(dir, away); err != nil {
		t.Fatal(err)
	}
	w.Check() // Windows reports nothing when the watched folder itself moves
	deadline := time.After(6 * time.Second)
wait:
	for {
		select {
		case ev := <-ch:
			if ev.Removed {
				break wait
			}
		case <-deadline:
			t.Fatal("no Removed event")
		}
	}
	if err := os.Rename(away, dir); err != nil {
		t.Fatal(err)
	}
	ch2 := make(chan Event, 1000)
	if _, err := w.Subscribe(dir, func(e Event) { ch2 <- e }); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	drain(ch2, 400*time.Millisecond)
	target := filepath.Join(dir, "x.txt")
	os.WriteFile(target, []byte("x"), 0o644)
	deadline = time.After(6 * time.Second)
	for {
		select {
		case ev := <-ch2:
			for _, p := range ev.Paths {
				if p == target {
					return
				}
			}
		case <-deadline:
			t.Fatal("no event after the folder came back")
		}
	}
}

// Renaming an ancestor is invisible to the stream; Check finds it.
func TestNativeAncestorRenamedIsFoundByCheck(t *testing.T) {
	parent := t.TempDir()
	outer := filepath.Join(parent, "outer")
	dir := filepath.Join(outer, "repo")
	os.MkdirAll(dir, 0o755)
	w, ch := startNative(t, dir)
	if err := os.Rename(outer, filepath.Join(parent, "moved")); err != nil {
		t.Fatal(err)
	}
	w.Check()
	deadline := time.After(6 * time.Second)
	for {
		select {
		case ev := <-ch:
			if ev.Removed {
				return
			}
		case <-deadline:
			t.Fatal("no Removed event for a moved ancestor")
		}
	}
}
