// Package watch reports changes under a folder, recursively, without one file
// handle per file: FSEvents on macOS and ReadDirectoryChangesW on Windows.
// Noise (node_modules, .git/objects, lock files) is dropped, bursts are
// debounced, and Pause / Resume close and reopen every handle on a folder so
// the folder can be renamed on Windows.
package watch

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// defaultCheckInterval is how often every watched root is stat'ed. FSEvents does
// not report a renamed or trashed ancestor, so this is the only way to learn
// that a folder has gone away because a parent moved.
const defaultCheckInterval = 30 * time.Second

var (
	// ErrUnsupported is returned on a platform without a native backend.
	ErrUnsupported = errors.New("watch: no native file watcher on this platform")
	// ErrNotWatched is returned by Pause, Resume and Relocate for a folder nobody subscribed to.
	ErrNotWatched = errors.New("watch: folder is not watched")
	// ErrClosed is returned after Close.
	ErrClosed = errors.New("watch: watcher is closed")
)

// MaxPaths caps Event.Paths.
const MaxPaths = 64

// Event is one debounced batch of changes under a folder.
type Event struct {
	Folder string
	// Paths is a sample (at most MaxPaths) of distinct changed paths, absolute, under Folder.
	Paths []string
	// Count is the number of change notifications folded into this event.
	Count int
	// GitChanged is true when something inside a .git folder changed (HEAD, index, refs).
	GitChanged bool
	// Overflow means notifications may have been lost, or watching was paused:
	// rescan the whole folder.
	Overflow bool
	// Removed means the folder is gone or was renamed away (also when an
	// ancestor was). No further events come for it until the folder is back and
	// someone subscribes again (it then joins the same record and watching
	// restarts), or Pause, Relocate and Resume are used. Ancestor moves are
	// noticed by a periodic check (Options.CheckInterval, Check), not by the
	// native stream. On Windows the same holds for the watched folder itself:
	// renaming or moving it (also to the Recycle Bin) is noticed only by Check.
	Removed bool
	// Stopped means the native watch failed for a reason other than removal and
	// could not be recovered. No further events come until Subscribe is called
	// again for the folder; the consumer should rescan and, meanwhile, poll.
	Stopped bool
}

// Options tune a Watcher. Zero values pick the defaults.
type Options struct {
	// Debounce is the quiet time after the last notification before an event is
	// delivered. Default 150 ms.
	Debounce time.Duration
	// MaxWait bounds how long a continuous burst can delay delivery. Default 1 s.
	MaxWait time.Duration
	// ExtraIgnore adds folder or file names to ignore at any depth.
	ExtraIgnore []string
	// CheckInterval is how often watched roots are stat'ed to catch a moved
	// ancestor. Default 30 s.
	CheckInterval time.Duration
}

// backend is the native watcher for one folder.
type backend interface {
	// resolve returns the path the native API needs for root (symlinks
	// resolved) and fails when root is not reachable. It may block on slow
	// file systems, so the Watcher calls it without holding its lock.
	resolve(root string) (string, error)
	// start begins watching root (real is resolve's answer) and returns a stop
	// function. emit may be called from any goroutine. stop must be safe to call
	// without holding locks that emit takes, and no emit call may start after
	// stop returns.
	start(root, real string, emit emitFunc) (stop func(), err error)
}

// emitFunc receives raw notifications. paths are absolute and under the root as
// the caller spelled it. overflow marks lost events; rootGone hints the root
// itself may have been removed or renamed; dead means the native watch failed
// for good while the root still exists.
type emitFunc func(paths []string, overflow, rootGone, dead bool)

// Watcher multiplexes subscribers onto one native watch per folder.
type Watcher struct {
	mu      sync.Mutex
	opts    Options
	ig      *ignorer
	be      backend
	folders map[string]*folder
	// paused counts Pause calls by the folder they were made on. A record is
	// paused while any key here is its root or a parent folder of it.
	paused  map[string]int
	nextSub int
	closed  bool
	nowFn   func() time.Time
	// stopping holds backend stops that run outside the lock, so Pause can wait
	// for handles that a concurrent unsubscribe is still closing.
	stopping map[*stopTask]struct{}
	quit     chan struct{}
}

type stopTask struct {
	root string
	done chan struct{}
}

type pending struct {
	set      map[string]struct{}
	paths    []string
	count    int
	git      bool
	overflow bool
	rootGone bool
	dead     bool
	first    time.Time
}

type folder struct {
	root    string
	subs    map[int]func(Event)
	stop    func()
	info    os.FileInfo // the folder the backend was started on, to notice a swap
	gen     int
	pend    pending
	timer   *time.Timer
	seq     int
	removed bool // no backend because the folder vanished or the watch died
	deliver sync.Mutex
}

// New returns a Watcher using the platform's native backend.
func New(opts Options) *Watcher { return newWatcher(opts, newNativeBackend()) }

func newWatcher(opts Options, be backend) *Watcher {
	if opts.Debounce <= 0 {
		opts.Debounce = 150 * time.Millisecond
	}
	if opts.MaxWait <= 0 {
		opts.MaxWait = time.Second
	}
	if opts.MaxWait < opts.Debounce {
		opts.MaxWait = opts.Debounce
	}
	if opts.CheckInterval <= 0 {
		opts.CheckInterval = defaultCheckInterval
	}
	w := &Watcher{opts: opts, ig: newIgnorer(opts.ExtraIgnore), be: be,
		folders: map[string]*folder{}, paused: map[string]int{}, nowFn: time.Now,
		stopping: map[*stopTask]struct{}{}, quit: make(chan struct{})}
	go w.checkLoop()
	return w
}

func (w *Watcher) checkLoop() {
	t := time.NewTicker(w.opts.CheckInterval)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			w.Check()
		case <-w.quit:
			return
		}
	}
}

// Check stats every active root and reports Removed for those that are gone,
// including roots whose ancestor was renamed or moved to the Trash, which the
// native streams do not report. It runs on a timer; callers may also run it
// after the app regains focus or the machine wakes.
func (w *Watcher) Check() {
	type item struct {
		f    *folder
		gen  int
		root string
		info os.FileInfo
	}
	w.mu.Lock()
	var items []item
	for _, f := range w.folders {
		if f.stop != nil && !f.removed {
			items = append(items, item{f, f.gen, f.root, f.info})
		}
	}
	w.mu.Unlock()
	for _, it := range items {
		st, err := os.Stat(it.root)
		if err == nil {
			if it.info != nil && !os.SameFile(it.info, st) {
				// The watch follows the folder it was opened on: that folder was
				// renamed or trashed and another one now sits at the path.
				w.restart(it.f, it.gen, it.root)
			}
			continue
		}
		w.mu.Lock()
		if it.f.gen == it.gen && !w.coveredLocked(it.root, "") && !it.f.removed && !w.closed {
			it.f.pend.rootGone = true
			w.scheduleLocked(it.f)
		}
		w.mu.Unlock()
	}
}

// coveredLocked reports whether a Pause is outstanding on root or a parent
// folder of it, ignoring the pauses made on skip.
func (w *Watcher) coveredLocked(root, skip string) bool {
	for k, n := range w.paused {
		if n > 0 && k != skip && isUnder(root, k) {
			return true
		}
	}
	return false
}

// restart re-opens the backend of f on the folder now at root, after the
// watched folder was swapped for another one, and tells subscribers to rescan.
func (w *Watcher) restart(f *folder, gen int, root string) {
	real, info, err := w.resolveDir(root)
	w.mu.Lock()
	if f.gen != gen || f.stop == nil || f.root != root || w.closed || w.coveredLocked(root, "") {
		w.mu.Unlock()
		return
	}
	if err != nil {
		f.pend.rootGone = true
		w.scheduleLocked(f)
		w.mu.Unlock()
		return
	}
	stop := w.detachLocked(f)
	if err := w.startLocked(f, real, info); err != nil {
		f.pend.dead = true
	}
	f.pend.overflow = true
	w.scheduleLocked(f)
	w.mu.Unlock()
	if stop != nil {
		stop()
	}
}

// isUnder reports whether p is root or lies beneath it.
func isUnder(p, root string) bool {
	if p == root {
		return true
	}
	prefix := root
	if !strings.HasSuffix(prefix, string(filepath.Separator)) {
		prefix += string(filepath.Separator)
	}
	return strings.HasPrefix(p, prefix)
}

// matchLocked returns every record rooted at dir or beneath it.
func (w *Watcher) matchLocked(dir string) []*folder {
	var out []*folder
	for p, f := range w.folders {
		if isUnder(p, dir) {
			out = append(out, f)
		}
	}
	return out
}

func cleanFolder(p string) (string, error) {
	if p == "" {
		return "", errors.New("watch: empty folder")
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

// resolveDir checks that root is a folder and returns the backend's path for it.
func (w *Watcher) resolveDir(root string) (string, os.FileInfo, error) {
	// Stat through an open handle: on Windows this loads the volume serial and
	// file index now, so a later swap of the folder is detected by SameFile.
	d, err := os.Open(root)
	if err != nil {
		return "", nil, err
	}
	st, err := d.Stat()
	d.Close()
	if err != nil {
		return "", nil, err
	}
	if !st.IsDir() {
		return "", nil, fmt.Errorf("watch: %s is not a folder", root)
	}
	real, err := w.be.resolve(root)
	if err != nil {
		return "", nil, err
	}
	return real, st, nil
}

// Subscribe calls fn for every debounced event under folder. Calls for one
// folder never overlap. The returned function unsubscribes (idempotent); when
// the last subscriber leaves, every handle on the folder is closed.
func (w *Watcher) Subscribe(dir string, fn func(Event)) (func(), error) {
	root, err := cleanFolder(dir)
	if err != nil {
		return nil, err
	}
	// File-system calls happen before taking the lock: a dead network share must
	// not stall every other watch.
	real, info, err := w.resolveDir(root)
	if err != nil {
		return nil, err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil, ErrClosed
	}
	f := w.folders[root]
	switch {
	case f == nil:
		f = &folder{root: root, subs: map[int]func(Event){}}
		// A folder inside a paused one stays closed until the outer Resume.
		if !w.coveredLocked(root, "") {
			if err := w.startLocked(f, real, info); err != nil {
				return nil, err
			}
		}
		w.folders[root] = f
	case f.stop == nil && !w.coveredLocked(root, ""):
		// The record outlived its backend (folder removed, or the watch died) and
		// the folder is back: watch it again and tell the others to rescan.
		if err := w.startLocked(f, real, info); err != nil {
			return nil, err
		}
		f.removed = false
		f.pend.overflow = true
		w.scheduleLocked(f)
	}
	w.nextSub++
	id := w.nextSub
	f.subs[id] = fn
	var once sync.Once
	return func() { once.Do(func() { w.unsubscribe(f, id) }) }, nil
}

func (w *Watcher) unsubscribe(f *folder, id int) {
	w.mu.Lock()
	delete(f.subs, id)
	var stop func()
	if len(f.subs) == 0 {
		stop = w.detachLocked(f)
		// Pauses are kept by folder, so a Subscribe during a pause still opens
		// nothing without this record.
		if w.folders[f.root] == f {
			delete(w.folders, f.root)
		}
	}
	w.mu.Unlock()
	if stop != nil {
		stop()
	}
}

// Pause closes every handle on folder and on every watched folder beneath it,
// and drops pending events. It returns after the handles are closed, including
// those a concurrent unsubscribe is still closing. A Subscribe for the folder
// or beneath it while paused opens nothing until Resume. Pausing a folder
// nobody watches is allowed. Calls nest: the folder is watched again after as
// many Resume calls.
func (w *Watcher) Pause(dir string) error {
	root, err := cleanFolder(dir)
	if err != nil {
		return err
	}
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return ErrClosed
	}
	w.paused[root]++
	var stops []func()
	for _, f := range w.matchLocked(root) {
		f.pend = pending{}
		f.seq++
		if f.timer != nil {
			f.timer.Stop()
		}
		if s := w.detachLocked(f); s != nil {
			stops = append(stops, s)
		}
	}
	var waits []chan struct{}
	for t := range w.stopping {
		if isUnder(t.root, root) {
			waits = append(waits, t.done)
		}
	}
	w.mu.Unlock()
	for _, s := range stops {
		s()
	}
	for _, c := range waits {
		<-c
	}
	return nil
}

// Resume undoes one Pause on folder. When the count on that folder reaches
// zero, every watched folder beneath it that no other Pause still covers is
// watched again and its subscribers get one Overflow event (changes while
// paused are unknown). Resume on a folder that was not paused returns
// ErrNotWatched. If a folder cannot be opened, nothing changes: the pause stays
// and Resume can be retried.
func (w *Watcher) Resume(dir string) error {
	root, err := cleanFolder(dir)
	if err != nil {
		return err
	}
	// Resolve outside the lock.
	w.mu.Lock()
	if w.paused[root] == 0 {
		w.mu.Unlock()
		return ErrNotWatched
	}
	var cand []string
	if w.paused[root] == 1 {
		for _, f := range w.matchLocked(root) {
			if len(f.subs) > 0 && !w.coveredLocked(f.root, root) {
				cand = append(cand, f.root)
			}
		}
	}
	w.mu.Unlock()
	type resolvedDir struct {
		real string
		info os.FileInfo
	}
	resolved := map[string]resolvedDir{}
	missing := map[string]bool{}
	for _, p := range cand {
		real, info, err := w.resolveDir(p)
		if err == nil {
			resolved[p] = resolvedDir{real, info}
			continue
		}
		if p == root {
			return err
		}
		missing[p] = true // a nested repository that is gone
	}

	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return ErrClosed
	}
	switch n := w.paused[root]; {
	case n == 0:
		w.mu.Unlock()
		return ErrNotWatched
	case n > 1:
		w.paused[root]--
		w.mu.Unlock()
		return nil
	}
	var ms []*folder
	for _, f := range w.matchLocked(root) {
		if len(f.subs) > 0 && !w.coveredLocked(f.root, root) {
			ms = append(ms, f)
		}
	}
	type started struct {
		f       *folder
		removed bool
	}
	var done []started
	var stops []func()
	var firstErr error
	for _, f := range ms {
		if missing[f.root] {
			continue
		}
		rd, ok := resolved[f.root]
		if !ok {
			firstErr = errors.New("watch: folders changed during Resume, try again")
			break
		}
		prev := f.removed
		f.removed = false
		if err := w.startLocked(f, rd.real, rd.info); err != nil {
			f.removed = prev
			firstErr = err
			break
		}
		done = append(done, started{f, prev})
	}
	if firstErr != nil {
		for _, d := range done {
			d.f.removed = d.removed
			if s := w.detachLocked(d.f); s != nil {
				stops = append(stops, s)
			}
		}
		w.mu.Unlock()
		for _, s := range stops {
			s()
		}
		return firstErr
	}
	delete(w.paused, root)
	for _, f := range ms {
		if missing[f.root] {
			f.pend.rootGone = true
		} else {
			f.pend.overflow = true
		}
		w.scheduleLocked(f)
	}
	w.mu.Unlock()
	return nil
}

// Relocate re-points a paused folder, and every paused folder beneath it, to a
// new path (after a rename on disk), keeping the subscribers and the pauses.
// Call Resume with the new path afterwards.
func (w *Watcher) Relocate(oldDir, newDir string) error {
	oldRoot, err := cleanFolder(oldDir)
	if err != nil {
		return err
	}
	newRoot, err := cleanFolder(newDir)
	if err != nil {
		return err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	hasPause := false
	for k, n := range w.paused {
		if n > 0 && (isUnder(oldRoot, k) || isUnder(k, oldRoot)) {
			hasPause = true
		}
	}
	if !hasPause {
		if w.folders[oldRoot] == nil {
			return ErrNotWatched
		}
		return errors.New("watch: Relocate needs a paused folder")
	}
	if oldRoot == newRoot {
		return nil
	}
	// Every record that moves must be detached and still covered by a pause
	// after the move (a pause on oldRoot or beneath moves with it; one on an
	// ancestor stays and must cover the new path too).
	for _, g := range w.matchLocked(oldRoot) {
		newPath := newRoot + strings.TrimPrefix(g.root, oldRoot)
		covered := false
		for k, n := range w.paused {
			if n <= 0 || !isUnder(g.root, k) {
				continue
			}
			if isUnder(k, oldRoot) || isUnder(newPath, k) {
				covered = true
			}
		}
		if g.stop != nil || !covered {
			return errors.New("watch: Relocate needs every folder beneath it paused")
		}
	}
	moving := map[string]*folder{}
	inMove := map[*folder]bool{}
	for _, g := range w.matchLocked(oldRoot) {
		moving[newRoot+strings.TrimPrefix(g.root, oldRoot)] = g
		inMove[g] = true
	}
	for np := range moving {
		if existing := w.folders[np]; existing != nil && !inMove[existing] {
			return fmt.Errorf("watch: %s is already watched", np)
		}
	}
	for _, g := range moving {
		delete(w.folders, g.root)
	}
	for np, g := range moving {
		g.root = np
		w.folders[np] = g
	}
	movedKeys := map[string]int{}
	for k, n := range w.paused {
		if isUnder(k, oldRoot) {
			movedKeys[newRoot+strings.TrimPrefix(k, oldRoot)] = n
			delete(w.paused, k)
		}
	}
	for k, n := range movedKeys {
		w.paused[k] += n
	}
	return nil
}

// Close stops every watch. Subscribers get nothing more.
func (w *Watcher) Close() {
	w.mu.Lock()
	first := !w.closed
	if first {
		close(w.quit)
	}
	w.closed = true
	var stops []func()
	for root, f := range w.folders {
		f.seq++
		if f.timer != nil {
			f.timer.Stop()
		}
		if s := w.detachLocked(f); s != nil {
			stops = append(stops, s)
		}
		f.subs = map[int]func(Event){}
		delete(w.folders, root)
	}
	w.mu.Unlock()
	for _, s := range stops {
		s()
	}
	if c, ok := w.be.(interface{ close() }); ok && first {
		c.close() // after every stop: the backend may release shared resources
	}
}

// startLocked opens the backend for f. real is be.resolve(f.root), computed
// without the lock. Caller holds w.mu.
func (w *Watcher) startLocked(f *folder, real string, info os.FileInfo) error {
	f.gen++
	gen := f.gen
	stop, err := w.be.start(f.root, real, func(paths []string, overflow, rootGone, dead bool) {
		w.onRaw(f, gen, paths, overflow, rootGone, dead)
	})
	if err != nil {
		return err
	}
	f.stop = stop
	f.info = info
	return nil
}

// detachLocked invalidates the running backend and returns a function that
// stops it, which the caller runs after releasing w.mu. Until it returns,
// Pause on the folder waits.
func (w *Watcher) detachLocked(f *folder) func() {
	f.gen++
	s := f.stop
	f.stop = nil
	if s == nil {
		return nil
	}
	t := &stopTask{root: f.root, done: make(chan struct{})}
	w.stopping[t] = struct{}{}
	return func() {
		s()
		w.mu.Lock()
		delete(w.stopping, t)
		w.mu.Unlock()
		close(t.done)
	}
}

func (w *Watcher) onRaw(f *folder, gen int, paths []string, overflow, rootGone, dead bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if f.gen != gen || w.closed {
		return
	}
	relevant := overflow || rootGone || dead
	for _, p := range paths {
		rel, err := filepath.Rel(f.root, p)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			f.pend.overflow = true
			relevant = true
			continue
		}
		if w.ig.ignored(rel) {
			continue
		}
		relevant = true
		f.pend.count++
		if rel == ".git" || strings.HasPrefix(filepath.ToSlash(rel), ".git/") || strings.Contains(filepath.ToSlash(rel), "/.git/") {
			f.pend.git = true
		}
		if _, dup := f.pend.set[p]; !dup && len(f.pend.paths) < MaxPaths {
			if f.pend.set == nil {
				f.pend.set = map[string]struct{}{}
			}
			f.pend.set[p] = struct{}{}
			f.pend.paths = append(f.pend.paths, p)
		}
	}
	if !relevant {
		return
	}
	if overflow {
		f.pend.overflow = true
	}
	if rootGone {
		f.pend.rootGone = true
	}
	if dead {
		f.pend.dead = true
	}
	w.scheduleLocked(f)
}

func (w *Watcher) scheduleLocked(f *folder) {
	now := w.nowFn()
	if f.pend.first.IsZero() {
		f.pend.first = now
	}
	delay := w.opts.Debounce
	if limit := f.pend.first.Add(w.opts.MaxWait).Sub(now); limit < delay {
		delay = limit
	}
	if delay < 0 {
		delay = 0
	}
	if f.timer != nil {
		f.timer.Stop()
	}
	f.seq++
	seq := f.seq
	f.timer = time.AfterFunc(delay, func() { w.flush(f, seq) })
}

func (w *Watcher) flush(f *folder, seq int) {
	w.mu.Lock()
	if f.seq != seq || w.coveredLocked(f.root, "") || f.removed {
		w.mu.Unlock()
		return
	}
	root, gen, info := f.root, f.gen, f.info
	w.mu.Unlock()

	// The stat runs without the lock: a dead network share can block it.
	st, statErr := os.Stat(root)
	if statErr == nil && info != nil && !os.SameFile(info, st) {
		// Another folder sits at the path now (the watched one was renamed or
		// trashed and the path was reused): watch the new one, which schedules
		// the next flush with an Overflow.
		w.restart(f, gen, root)
		return
	}

	w.mu.Lock()
	if f.seq != seq || f.gen != gen || w.coveredLocked(f.root, "") || f.removed || w.closed {
		w.mu.Unlock()
		return // something newer is scheduled or the state changed meanwhile
	}
	p := f.pend
	f.pend = pending{}
	ev := Event{Folder: f.root, Paths: p.paths, Count: p.count, GitChanged: p.git, Overflow: p.overflow}
	var stop func()
	if statErr != nil {
		// The folder is gone or renamed away: stop watching it.
		f.removed = true
		ev = Event{Folder: f.root, Removed: true}
		stop = w.detachLocked(f)
	} else if p.dead {
		f.removed = true
		ev = Event{Folder: f.root, Overflow: true, Stopped: true}
		stop = w.detachLocked(f)
	}
	subs := make([]func(Event), 0, len(f.subs))
	for _, fn := range f.subs {
		subs = append(subs, fn)
	}
	w.mu.Unlock()
	if stop != nil {
		stop()
	}
	f.deliver.Lock()
	defer f.deliver.Unlock()
	for _, fn := range subs {
		func() {
			defer func() { _ = recover() }()
			fn(ev)
		}()
	}
}
