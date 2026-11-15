//go:build windows

package watch

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWindowsPauseAndUnsubscribeCloseTheDirectoryHandle(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "repo")
	os.Mkdir(dir, 0o755)
	w := New(nativeOpts())
	defer w.Close()
	un, err := w.Subscribe(dir, func(Event) {})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	if err := w.Pause(dir); err != nil {
		t.Fatal(err)
	}
	assertNoHandleOn(t, dir)
	// The parent can be renamed while the folder is paused and held by a subscriber.
	moved := filepath.Join(filepath.Dir(parent), filepath.Base(parent)+"-moved")
	if err := os.Rename(parent, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(moved, parent); err != nil {
		t.Fatal(err)
	}
	if err := w.Resume(dir); err != nil {
		t.Fatal(err)
	}
	un()
	assertNoHandleOn(t, dir)
}

func TestWindowsNestedRepoHandleClosedByOuterPause(t *testing.T) {
	outer := t.TempDir()
	inner := filepath.Join(outer, "inner")
	os.Mkdir(inner, 0o755)
	w := New(nativeOpts())
	defer w.Close()
	w.Subscribe(outer, func(Event) {})
	w.Subscribe(inner, func(Event) {})
	time.Sleep(200 * time.Millisecond)
	w.Pause(outer)
	assertNoHandleOn(t, inner)
	assertNoHandleOn(t, outer)
	if err := w.Resume(outer); err != nil {
		t.Fatal(err)
	}
}

// A watched folder renamed away and replaced by a new folder at the same path
// (deleting a repository in Explorer and cloning it again): the handle follows
// the old folder, so Check must move the watch to the new one.
func TestWindowsCheckFollowsReplacedFolder(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "repo")
	os.Mkdir(dir, 0o755)
	w := New(nativeOpts())
	defer w.Close()
	ch := make(chan Event, 100)
	if _, err := w.Subscribe(dir, func(e Event) { ch <- e }); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	if err := os.Rename(dir, filepath.Join(parent, "recycled")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	w.Check()
	time.Sleep(300 * time.Millisecond)
	drain(ch, 500*time.Millisecond)
	target := filepath.Join(dir, "new.txt")
	os.WriteFile(target, []byte("x"), 0o644)
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
			t.Fatalf("no event naming %s after the folder was replaced", target)
		}
	}
}
