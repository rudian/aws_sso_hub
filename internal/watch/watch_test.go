package watch

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeBackend lets tests drive raw notifications and count open watches.
type fakeBackend struct {
	mu      sync.Mutex
	open    map[string]emitFunc
	ids     map[string]int
	starts  int
	stops   int
	failFor map[string]error
}

func newFake() *fakeBackend {
	return &fakeBackend{open: map[string]emitFunc{}, ids: map[string]int{}, failFor: map[string]error{}}
}

func (b *fakeBackend) resolve(root string) (string, error) { return root, nil }

func (b *fakeBackend) start(root, _ string, emit emitFunc) (func(), error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.failFor[root]; err != nil {
		return nil, err
	}
	b.starts++
	b.open[root] = emit
	b.ids[root] = b.starts
	id := b.starts
	return func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		b.stops++
		if b.ids[root] == id { // a newer start on the same path stays open
			delete(b.open, root)
		}
	}, nil
}

func (b *fakeBackend) send(root string, overflow, gone bool, rel ...string) {
	b.mu.Lock()
	emit := b.open[root]
	b.mu.Unlock()
	paths := make([]string, len(rel))
	for i, r := range rel {
		paths[i] = filepath.Join(root, r)
	}
	if emit != nil {
		emit(paths, overflow, gone, false)
	}
}

func (b *fakeBackend) openCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.open)
}

func fastOpts() Options {
	return Options{Debounce: 20 * time.Millisecond, MaxWait: 100 * time.Millisecond}
}

func collect(ch chan Event, d time.Duration) []Event {
	var out []Event
	timeout := time.After(d)
	for {
		select {
		case e := <-ch:
			out = append(out, e)
		case <-timeout:
			return out
		}
	}
}

func TestBurstCollapsesToOneEvent(t *testing.T) {
	dir := t.TempDir()
	fb := newFake()
	// Tests debouncing, not the MaxWait cap, so the timing is generous: a loaded
	// machine can take well over 100 ms for the loop.
	w := newWatcher(Options{Debounce: 300 * time.Millisecond, MaxWait: 30 * time.Second}, fb)
	defer w.Close()
	ch := make(chan Event, 100)
	if _, err := w.Subscribe(dir, func(e Event) { ch <- e }); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10000; i++ {
		fb.send(dir, false, false, "src/file.go")
	}
	evs := collect(ch, 900*time.Millisecond)
	if len(evs) != 1 {
		t.Fatalf("want 1 event, got %d", len(evs))
	}
	if evs[0].Count != 10000 || len(evs[0].Paths) != 1 {
		t.Fatalf("event = %+v", evs[0])
	}
}

func TestPathSampleIsCapped(t *testing.T) {
	dir := t.TempDir()
	fb := newFake()
	w := newWatcher(fastOpts(), fb)
	defer w.Close()
	ch := make(chan Event, 10)
	w.Subscribe(dir, func(e Event) { ch <- e })
	for i := 0; i < 500; i++ {
		fb.send(dir, false, false, "f"+string(rune('a'+i%26))+string(rune('a'+i/26%26)))
	}
	evs := collect(ch, 200*time.Millisecond)
	if len(evs) != 1 || len(evs[0].Paths) != MaxPaths {
		t.Fatalf("events = %+v", evs)
	}
}

func TestContinuousBurstStillDelivers(t *testing.T) {
	dir := t.TempDir()
	fb := newFake()
	w := newWatcher(fastOpts(), fb)
	defer w.Close()
	var n atomic.Int32
	w.Subscribe(dir, func(Event) { n.Add(1) })
	stop := time.Now().Add(350 * time.Millisecond)
	for time.Now().Before(stop) {
		fb.send(dir, false, false, "a")
		time.Sleep(5 * time.Millisecond) // quieter than Debounce never happens
	}
	time.Sleep(80 * time.Millisecond)
	if got := n.Load(); got < 2 || got > 6 {
		t.Fatalf("MaxWait should cut the burst into a few events, got %d", got)
	}
}

func TestIgnoredPathsProduceNothing(t *testing.T) {
	dir := t.TempDir()
	fb := newFake()
	w := newWatcher(fastOpts(), fb)
	defer w.Close()
	ch := make(chan Event, 10)
	w.Subscribe(dir, func(e Event) { ch <- e })
	for i := 0; i < 1000; i++ {
		fb.send(dir, false, false, "node_modules/pkg/a.js", ".git/objects/ab/cd", ".git/index.lock")
	}
	if evs := collect(ch, 150*time.Millisecond); len(evs) != 0 {
		t.Fatalf("got %d events", len(evs))
	}
	fb.send(dir, false, false, ".git/HEAD")
	evs := collect(ch, 150*time.Millisecond)
	if len(evs) != 1 || !evs[0].GitChanged {
		t.Fatalf("events = %+v", evs)
	}
}

func TestPathOutsideRootMeansOverflow(t *testing.T) {
	dir := t.TempDir()
	fb := newFake()
	w := newWatcher(fastOpts(), fb)
	defer w.Close()
	ch := make(chan Event, 10)
	w.Subscribe(dir, func(e Event) { ch <- e })
	fb.mu.Lock()
	emit := fb.open[dir]
	fb.mu.Unlock()
	emit([]string{filepath.Join(filepath.Dir(dir), "elsewhere", "x")}, false, false, false)
	evs := collect(ch, 150*time.Millisecond)
	if len(evs) != 1 || !evs[0].Overflow {
		t.Fatalf("events = %+v", evs)
	}
}

func TestTwoSubscribersShareOneWatch(t *testing.T) {
	dir := t.TempDir()
	fb := newFake()
	w := newWatcher(fastOpts(), fb)
	defer w.Close()
	var a, b atomic.Int32
	unA, _ := w.Subscribe(dir, func(Event) { a.Add(1) })
	unB, _ := w.Subscribe(dir, func(Event) { b.Add(1) })
	if fb.starts != 1 {
		t.Fatalf("starts = %d", fb.starts)
	}
	fb.send(dir, false, false, "x")
	time.Sleep(100 * time.Millisecond)
	if a.Load() != 1 || b.Load() != 1 {
		t.Fatalf("a=%d b=%d", a.Load(), b.Load())
	}
	unA()
	unA() // idempotent
	if fb.openCount() != 1 {
		t.Fatal("watch closed while a subscriber remains")
	}
	unB()
	if fb.openCount() != 0 || fb.stops != 1 {
		t.Fatalf("open=%d stops=%d", fb.openCount(), fb.stops)
	}
}

func TestUnsubscribeDropsPendingDelivery(t *testing.T) {
	dir := t.TempDir()
	fb := newFake()
	w := newWatcher(fastOpts(), fb)
	defer w.Close()
	var n atomic.Int32
	un, _ := w.Subscribe(dir, func(Event) { n.Add(1) })
	fb.send(dir, false, false, "x")
	un()
	time.Sleep(100 * time.Millisecond)
	if n.Load() != 0 {
		t.Fatal("delivered after unsubscribe")
	}
}

func TestPauseClosesHandlesAndResumeReportsOverflow(t *testing.T) {
	dir := t.TempDir()
	fb := newFake()
	w := newWatcher(fastOpts(), fb)
	defer w.Close()
	ch := make(chan Event, 10)
	w.Subscribe(dir, func(e Event) { ch <- e })
	fb.send(dir, false, false, "before") // pending, then dropped by Pause
	if err := w.Pause(dir); err != nil {
		t.Fatal(err)
	}
	if fb.openCount() != 0 {
		t.Fatal("handle still open while paused")
	}
	if evs := collect(ch, 100*time.Millisecond); len(evs) != 0 {
		t.Fatalf("event while paused: %+v", evs)
	}
	if err := w.Resume(dir); err != nil {
		t.Fatal(err)
	}
	if fb.openCount() != 1 {
		t.Fatal("handle not reopened")
	}
	evs := collect(ch, 100*time.Millisecond)
	if len(evs) != 1 || !evs[0].Overflow {
		t.Fatalf("events = %+v", evs)
	}
	fb.send(dir, false, false, "after")
	if evs := collect(ch, 100*time.Millisecond); len(evs) != 1 || evs[0].Overflow {
		t.Fatalf("events = %+v", evs)
	}
}

func TestPauseNestsAndUnknownFolder(t *testing.T) {
	dir := t.TempDir()
	fb := newFake()
	w := newWatcher(fastOpts(), fb)
	defer w.Close()
	w.Subscribe(dir, func(Event) {})
	w.Pause(dir)
	w.Pause(dir)
	w.Resume(dir)
	if fb.openCount() != 0 {
		t.Fatal("reopened after the first of two Resume calls")
	}
	w.Resume(dir)
	if fb.openCount() != 1 {
		t.Fatal("not reopened")
	}
	if !errors.Is(w.Resume(dir), ErrNotWatched) { // more Resume calls than Pause calls
		t.Fatal("an extra Resume should fail")
	}
	if fb.openCount() != 1 {
		t.Fatal("an extra Resume changed the watch")
	}
	if !errors.Is(w.Resume(filepath.Join(dir, "nope")), ErrNotWatched) {
		t.Fatal("Resume on an unwatched folder should fail")
	}
}

func TestUnsubscribeWhilePausedLeaksNothing(t *testing.T) {
	dir := t.TempDir()
	fb := newFake()
	w := newWatcher(fastOpts(), fb)
	defer w.Close()
	un, _ := w.Subscribe(dir, func(Event) {})
	w.Pause(dir)
	un()
	if fb.openCount() != 0 || fb.starts != fb.stops {
		t.Fatalf("open=%d starts=%d stops=%d", fb.openCount(), fb.starts, fb.stops)
	}
	if err := w.Resume(dir); err != nil {
		t.Fatalf("err = %v", err)
	}
	if fb.openCount() != 0 || len(w.folders) != 0 {
		t.Fatalf("open=%d records=%d after the last Resume", fb.openCount(), len(w.folders))
	}
}

// (a) Pause, last unsubscribe, Subscribe during the window: no handle opens.
func TestSubscribeDuringPauseOpensNothing(t *testing.T) {
	dir := t.TempDir()
	fb := newFake()
	w := newWatcher(fastOpts(), fb)
	defer w.Close()
	un, _ := w.Subscribe(dir, func(Event) {})
	w.Pause(dir)
	un()
	ch := make(chan Event, 10)
	if _, err := w.Subscribe(dir, func(e Event) { ch <- e }); err != nil {
		t.Fatal(err)
	}
	if fb.openCount() != 0 {
		t.Fatalf("handle opened during the pause window: %d", fb.openCount())
	}
	if err := w.Resume(dir); err != nil {
		t.Fatal(err)
	}
	if fb.openCount() != 1 {
		t.Fatal("not watched after Resume")
	}
	if evs := collect(ch, 100*time.Millisecond); len(evs) != 1 || !evs[0].Overflow {
		t.Fatalf("events = %+v", evs)
	}
}

// (b) Pause on a folder nobody watches still keeps a later Subscribe closed.
func TestPauseUnwatchedFolderHoldsLaterSubscribe(t *testing.T) {
	dir := t.TempDir()
	fb := newFake()
	w := newWatcher(fastOpts(), fb)
	defer w.Close()
	if err := w.Pause(dir); err != nil {
		t.Fatal(err)
	}
	w.Subscribe(dir, func(Event) {})
	if fb.openCount() != 0 {
		t.Fatal("handle opened while paused")
	}
	w.Resume(dir)
	if fb.openCount() != 1 {
		t.Fatal("not watched after Resume")
	}
	// Pause then Resume with nobody subscribed leaves no record behind.
	other := t.TempDir()
	w.Pause(other)
	w.Resume(other)
	if len(w.folders) != 1 {
		t.Fatalf("records = %d", len(w.folders))
	}
}

// (d) Watches rooted inside the paused folder close too, and nested Subscribe is held.
func TestPauseCoversFoldersBeneath(t *testing.T) {
	parent := t.TempDir()
	child := filepath.Join(parent, "inner")
	sibling := parent + "-sibling"
	os.Mkdir(child, 0o755)
	os.Mkdir(sibling, 0o755)
	defer os.Remove(sibling)
	fb := newFake()
	w := newWatcher(fastOpts(), fb)
	defer w.Close()
	w.Subscribe(parent, func(Event) {})
	w.Subscribe(child, func(Event) {})
	w.Subscribe(sibling, func(Event) {}) // shares the prefix text, not the folder
	w.Pause(parent)
	if fb.openCount() != 1 {
		t.Fatalf("open = %d, want only the sibling", fb.openCount())
	}
	grand := filepath.Join(child, "g")
	os.Mkdir(grand, 0o755)
	w.Subscribe(grand, func(Event) {})
	if fb.openCount() != 1 {
		t.Fatal("a folder beneath a paused one opened a handle")
	}
	if err := w.Resume(parent); err != nil {
		t.Fatal(err)
	}
	if fb.openCount() != 4 {
		t.Fatalf("open = %d, want 4", fb.openCount())
	}
}

func TestRelocateMovesFoldersBeneath(t *testing.T) {
	parent := t.TempDir()
	oldDir := filepath.Join(parent, "a")
	newDir := filepath.Join(parent, "b")
	os.MkdirAll(filepath.Join(oldDir, "inner"), 0o755)
	fb := newFake()
	w := newWatcher(fastOpts(), fb)
	defer w.Close()
	w.Subscribe(oldDir, func(Event) {})
	w.Subscribe(filepath.Join(oldDir, "inner"), func(Event) {})
	w.Pause(oldDir)
	os.Rename(oldDir, newDir)
	if err := w.Relocate(oldDir, newDir); err != nil {
		t.Fatal(err)
	}
	if err := w.Resume(newDir); err != nil {
		t.Fatal(err)
	}
	fb.mu.Lock()
	_, ok1 := fb.open[newDir]
	_, ok2 := fb.open[filepath.Join(newDir, "inner")]
	fb.mu.Unlock()
	if !ok1 || !ok2 || fb.openCount() != 2 {
		t.Fatalf("open = %v", fb.open)
	}
}

// (c) Pause returns only after a concurrent unsubscribe finished closing.
func TestPauseWaitsForInFlightStop(t *testing.T) {
	dir := t.TempDir()
	fb := newFake()
	release := make(chan struct{})
	slow := &slowStopBackend{fakeBackend: fb, release: release}
	w := newWatcher(fastOpts(), slow)
	defer w.Close()
	un, _ := w.Subscribe(dir, func(Event) {})
	go un()
	time.Sleep(50 * time.Millisecond) // unsubscribe is now inside stop()
	paused := make(chan struct{})
	go func() { w.Pause(dir); close(paused) }()
	select {
	case <-paused:
		t.Fatal("Pause returned while a stop was still running")
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	select {
	case <-paused:
	case <-time.After(2 * time.Second):
		t.Fatal("Pause never returned")
	}
}

type slowStopBackend struct {
	*fakeBackend
	release chan struct{}
}

func (b *slowStopBackend) start(root, real string, emit emitFunc) (func(), error) {
	stop, err := b.fakeBackend.start(root, real, emit)
	if err != nil {
		return nil, err
	}
	return func() { <-b.release; stop() }, nil
}

// After Removed, the folder coming back and a new Subscribe revives watching.
func TestSubscribeAfterRemovedRevives(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "repo")
	moved := filepath.Join(parent, "gone")
	os.Mkdir(dir, 0o755)
	fb := newFake()
	w := newWatcher(fastOpts(), fb)
	defer w.Close()
	ch1 := make(chan Event, 10)
	w.Subscribe(dir, func(e Event) { ch1 <- e })
	os.Rename(dir, moved)
	fb.send(dir, true, true)
	if evs := collect(ch1, 150*time.Millisecond); len(evs) != 1 || !evs[0].Removed {
		t.Fatalf("events = %+v", evs)
	}
	os.Rename(moved, dir)
	ch2 := make(chan Event, 10)
	if _, err := w.Subscribe(dir, func(e Event) { ch2 <- e }); err != nil {
		t.Fatal(err)
	}
	if fb.openCount() != 1 {
		t.Fatal("watch not restarted")
	}
	if evs := collect(ch1, 150*time.Millisecond); len(evs) != 1 || !evs[0].Overflow {
		t.Fatalf("old subscriber should learn the folder is back: %+v", evs)
	}
	fb.send(dir, false, false, "x")
	if evs := collect(ch2, 150*time.Millisecond); len(evs) == 0 {
		t.Fatal("new subscriber got no event")
	}
}

// A moved ancestor is not reported by the native stream; Check notices it.
func TestCheckReportsMovedAncestor(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "outer", "repo")
	os.MkdirAll(dir, 0o755)
	fb := newFake()
	w := newWatcher(fastOpts(), fb)
	defer w.Close()
	ch := make(chan Event, 10)
	w.Subscribe(dir, func(e Event) { ch <- e })
	w.Check()
	if evs := collect(ch, 100*time.Millisecond); len(evs) != 0 {
		t.Fatalf("healthy folder reported: %+v", evs)
	}
	os.Rename(filepath.Join(parent, "outer"), filepath.Join(parent, "moved"))
	w.Check()
	evs := collect(ch, 150*time.Millisecond)
	if len(evs) != 1 || !evs[0].Removed {
		t.Fatalf("events = %+v", evs)
	}
	if fb.openCount() != 0 {
		t.Fatal("handle kept")
	}
}

func TestPeriodicCheckRuns(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "repo")
	os.Mkdir(dir, 0o755)
	fb := newFake()
	o := fastOpts()
	o.CheckInterval = 30 * time.Millisecond
	w := newWatcher(o, fb)
	defer w.Close()
	ch := make(chan Event, 10)
	w.Subscribe(dir, func(e Event) { ch <- e })
	os.RemoveAll(dir)
	if evs := collect(ch, 400*time.Millisecond); len(evs) != 1 || !evs[0].Removed {
		t.Fatalf("events = %+v", evs)
	}
}

// A native watch that dies while the folder exists ends with Stopped, and a new
// Subscribe restarts it.
func TestDeadWatchReportsStopped(t *testing.T) {
	dir := t.TempDir()
	fb := newFake()
	w := newWatcher(fastOpts(), fb)
	defer w.Close()
	ch := make(chan Event, 10)
	w.Subscribe(dir, func(e Event) { ch <- e })
	fb.mu.Lock()
	emit := fb.open[dir]
	fb.mu.Unlock()
	emit(nil, true, false, true)
	evs := collect(ch, 150*time.Millisecond)
	if len(evs) != 1 || !evs[0].Stopped || evs[0].Removed {
		t.Fatalf("events = %+v", evs)
	}
	if fb.openCount() != 0 {
		t.Fatal("handle kept after the watch died")
	}
	w.Subscribe(dir, func(Event) {})
	if fb.openCount() != 1 {
		t.Fatal("not restarted by Subscribe")
	}
}

func TestResumeFailureKeepsPause(t *testing.T) {
	dir := t.TempDir()
	fb := newFake()
	w := newWatcher(fastOpts(), fb)
	defer w.Close()
	w.Subscribe(dir, func(Event) {})
	w.Pause(dir)
	fb.failFor[dir] = errors.New("boom")
	if err := w.Resume(dir); err == nil {
		t.Fatal("want error")
	}
	delete(fb.failFor, dir)
	if err := w.Resume(dir); err != nil || fb.openCount() != 1 {
		t.Fatalf("retry: err=%v open=%d", err, fb.openCount())
	}
}

func TestRemovedFolder(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "repo")
	os.Mkdir(dir, 0o755)
	fb := newFake()
	w := newWatcher(fastOpts(), fb)
	defer w.Close()
	ch := make(chan Event, 10)
	un, _ := w.Subscribe(dir, func(e Event) { ch <- e })
	os.RemoveAll(dir)
	fb.send(dir, true, true)
	evs := collect(ch, 150*time.Millisecond)
	if len(evs) != 1 || !evs[0].Removed {
		t.Fatalf("events = %+v", evs)
	}
	if fb.openCount() != 0 {
		t.Fatal("handle kept after the folder vanished")
	}
	un()
	if fb.starts != fb.stops {
		t.Fatalf("leak: starts=%d stops=%d", fb.starts, fb.stops)
	}
}

func TestRelocateAfterRename(t *testing.T) {
	parent := t.TempDir()
	oldDir := filepath.Join(parent, "acme_api")
	newDir := filepath.Join(parent, "acme_service")
	os.Mkdir(oldDir, 0o755)
	fb := newFake()
	w := newWatcher(fastOpts(), fb)
	defer w.Close()
	ch := make(chan Event, 10)
	w.Subscribe(oldDir, func(e Event) { ch <- e })
	if err := w.Relocate(oldDir, newDir); err == nil {
		t.Fatal("Relocate must require a paused folder")
	}
	w.Pause(oldDir)
	if err := os.Rename(oldDir, newDir); err != nil {
		t.Fatal(err)
	}
	if err := w.Relocate(oldDir, newDir); err != nil {
		t.Fatal(err)
	}
	if err := w.Resume(newDir); err != nil {
		t.Fatal(err)
	}
	collect(ch, 100*time.Millisecond) // the resync event
	fb.send(newDir, false, false, "a.go")
	evs := collect(ch, 100*time.Millisecond)
	if len(evs) != 1 || evs[0].Folder != newDir {
		t.Fatalf("events = %+v", evs)
	}
}

func TestSubscribeErrorsAndClose(t *testing.T) {
	fb := newFake()
	w := newWatcher(fastOpts(), fb)
	if _, err := w.Subscribe(filepath.Join(t.TempDir(), "missing"), func(Event) {}); err == nil {
		t.Fatal("want error for a missing folder")
	}
	f := filepath.Join(t.TempDir(), "file")
	os.WriteFile(f, nil, 0o600)
	if _, err := w.Subscribe(f, func(Event) {}); err == nil {
		t.Fatal("want error for a file")
	}
	dir := t.TempDir()
	w.Subscribe(dir, func(Event) {})
	w.Close()
	if fb.openCount() != 0 {
		t.Fatal("Close left a handle open")
	}
	if _, err := w.Subscribe(dir, func(Event) {}); !errors.Is(err, ErrClosed) {
		t.Fatalf("err = %v", err)
	}
}

func TestSubscriberPanicDoesNotKillOthers(t *testing.T) {
	dir := t.TempDir()
	fb := newFake()
	w := newWatcher(fastOpts(), fb)
	defer w.Close()
	var n atomic.Int32
	w.Subscribe(dir, func(Event) { panic("bad subscriber") })
	w.Subscribe(dir, func(Event) { n.Add(1) })
	fb.send(dir, false, false, "x")
	time.Sleep(100 * time.Millisecond)
	if n.Load() != 1 {
		t.Fatal("second subscriber missed the event")
	}
}

// Pause(outer), Pause(inner), Resume(outer): inner stays paused until Resume(inner).
func TestNestedPausesKeepInnerClosed(t *testing.T) {
	outer := t.TempDir()
	inner := filepath.Join(outer, "inner")
	os.Mkdir(inner, 0o755)
	fb := newFake()
	w := newWatcher(fastOpts(), fb)
	defer w.Close()
	if err := w.Pause(outer); err != nil {
		t.Fatal(err)
	}
	if err := w.Pause(inner); err != nil {
		t.Fatal(err)
	}
	if err := w.Resume(outer); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Subscribe(inner, func(Event) {}); err != nil {
		t.Fatal(err)
	}
	if fb.openCount() != 0 {
		t.Fatal("inner opened while its own Pause is outstanding")
	}
	if err := w.Resume(inner); err != nil {
		t.Fatal(err)
	}
	if fb.openCount() != 1 {
		t.Fatal("inner not watched after its Resume")
	}
	if !errors.Is(w.Resume(inner), ErrNotWatched) {
		t.Fatal("a second Resume(inner) should fail")
	}
}

// Subscribe(inner), Pause(inner), Resume(outer): outer was never paused.
func TestResumeOfUnpausedParentDoesNotReopenInner(t *testing.T) {
	outer := t.TempDir()
	inner := filepath.Join(outer, "inner")
	os.Mkdir(inner, 0o755)
	fb := newFake()
	w := newWatcher(fastOpts(), fb)
	defer w.Close()
	w.Subscribe(inner, func(Event) {})
	w.Pause(inner)
	if !errors.Is(w.Resume(outer), ErrNotWatched) {
		t.Fatal("Resume on a folder that was never paused should fail")
	}
	if fb.openCount() != 0 {
		t.Fatal("inner reopened by Resume(outer)")
	}
	if err := w.Resume(inner); err != nil || fb.openCount() != 1 {
		t.Fatalf("Resume(inner): err=%v open=%d", err, fb.openCount())
	}
}

// Resume(inner) cannot reopen a folder whose parent is still paused.
func TestResumeInnerWhileOuterPaused(t *testing.T) {
	outer := t.TempDir()
	inner := filepath.Join(outer, "inner")
	os.Mkdir(inner, 0o755)
	fb := newFake()
	w := newWatcher(fastOpts(), fb)
	defer w.Close()
	w.Pause(outer)
	w.Subscribe(inner, func(Event) {})
	if !errors.Is(w.Resume(inner), ErrNotWatched) || fb.openCount() != 0 {
		t.Fatal("inner reopened or accepted a Resume while outer is paused")
	}
	if err := w.Resume(outer); err != nil || fb.openCount() != 1 {
		t.Fatalf("Resume(outer): err=%v open=%d", err, fb.openCount())
	}
}

// A rootGone + dead emit with the folder present ends the watch as Stopped, and
// a new Subscribe revives it.
func TestDeadWatchWithFolderPresentIsStoppedAndRevivable(t *testing.T) {
	dir := t.TempDir()
	fb := newFake()
	w := newWatcher(fastOpts(), fb)
	defer w.Close()
	ch := make(chan Event, 10)
	w.Subscribe(dir, func(e Event) { ch <- e })
	fb.mu.Lock()
	emit := fb.open[dir]
	fb.mu.Unlock()
	emit(nil, true, true, true)
	evs := collect(ch, 150*time.Millisecond)
	if len(evs) != 1 || !evs[0].Stopped {
		t.Fatalf("events = %+v", evs)
	}
	if fb.openCount() != 0 {
		t.Fatal("dead watch still open")
	}
	w.Subscribe(dir, func(Event) {})
	if fb.openCount() != 1 {
		t.Fatal("Subscribe did not revive the watch")
	}
}

// The folder at the path is replaced by another one: Check restarts the watch.
func TestCheckRestartsWatchOnSwappedFolder(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "repo")
	os.Mkdir(dir, 0o755)
	fb := newFake()
	w := newWatcher(fastOpts(), fb)
	defer w.Close()
	ch := make(chan Event, 10)
	w.Subscribe(dir, func(e Event) { ch <- e })
	if err := os.Rename(dir, filepath.Join(parent, "old")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	w.Check()
	evs := collect(ch, 150*time.Millisecond)
	if len(evs) != 1 || !evs[0].Overflow || evs[0].Removed {
		t.Fatalf("events = %+v", evs)
	}
	fb.mu.Lock()
	starts, stops := fb.starts, fb.stops
	fb.mu.Unlock()
	if starts != 2 || stops != 1 || fb.openCount() != 1 {
		t.Fatalf("starts=%d stops=%d open=%d", starts, stops, fb.openCount())
	}
	w.Check() // the same folder again: nothing more happens
	if collect(ch, 100*time.Millisecond) != nil {
		t.Fatal("second Check produced an event")
	}
}

// Only a nested folder paused: the live record at the top must not be re-keyed.
func TestRelocateRefusesWhenOnlyNestedFolderPaused(t *testing.T) {
	parent := t.TempDir()
	oldDir := filepath.Join(parent, "a")
	os.MkdirAll(filepath.Join(oldDir, "inner"), 0o755)
	w := newWatcher(fastOpts(), newFake())
	defer w.Close()
	w.Subscribe(oldDir, func(Event) {})
	w.Subscribe(filepath.Join(oldDir, "inner"), func(Event) {})
	w.Pause(filepath.Join(oldDir, "inner"))
	if err := w.Relocate(oldDir, filepath.Join(parent, "b")); err == nil {
		t.Fatal("Relocate must refuse while a live record would move")
	}
	w.mu.Lock()
	_, kept := w.folders[oldDir]
	w.mu.Unlock()
	if !kept {
		t.Fatal("the live record was re-keyed")
	}
}

// Paused through an ancestor that does not cover the new path: refuse.
func TestRelocateRefusesOutOfPausedAncestor(t *testing.T) {
	parent := t.TempDir()
	outer := filepath.Join(parent, "outer")
	oldDir := filepath.Join(outer, "a")
	os.MkdirAll(oldDir, 0o755)
	w := newWatcher(fastOpts(), newFake())
	defer w.Close()
	w.Subscribe(oldDir, func(Event) {})
	w.Pause(outer)
	if err := w.Relocate(oldDir, filepath.Join(parent, "b")); err == nil {
		t.Fatal("Relocate must refuse a move out of the paused ancestor")
	}
	if err := w.Relocate(oldDir, filepath.Join(outer, "b")); err != nil {
		t.Fatalf("a move inside the paused ancestor must work: %v", err)
	}
}
