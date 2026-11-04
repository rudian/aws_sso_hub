package app

import (
	"context"
	"path/filepath"
	"sync"
	"time"

	"github.com/rudian/aws_sso_hub/internal/api"
)

// fileWatcher polls the AWS config, the credentials file and the global git config. Changes the app made itself
// are reported through note() and ignored.
type fileWatcher struct {
	c *Container

	mu    sync.Mutex
	known map[string]string // cleaned path -> sha256 ("" = missing)

	// Undo bookkeeping. A backup is taken right before the app writes a file; the hash of what the app then
	// wrote is what an undo of that backup expects to find. Unlike known, it is never changed by poll(), so an
	// external edit stays visible to the undo check after the watcher has seen it.
	snapped   map[string]string // cleaned path -> id of the backup taken for the write in progress
	afterEdit map[string]string // backup id -> hash of the file as the app left it
	editOrder []string          // ids in afterEdit, oldest first

	settle time.Duration // re-read delay before a change is reported (own writes call note() right after writing)
}

func newFileWatcher(c *Container) *fileWatcher {
	return &fileWatcher{c: c, known: map[string]string{}, snapped: map[string]string{}, afterEdit: map[string]string{}, settle: 250 * time.Millisecond}
}

type watched struct{ kind, path string }

func (w *fileWatcher) list() []watched {
	return []watched{
		{"aws-config", filepath.Clean(w.c.Paths.AWSConfigFile)},
		{"aws-credentials", filepath.Clean(w.c.Paths.AWSCredentialsFile)},
		{"git-global", filepath.Clean(w.c.gitGlobalPath())},
	}
}

// note records the content the app just wrote (OnWrite).
func (w *fileWatcher) note(path string, content []byte) {
	p, h := filepath.Clean(path), hashBytes(content)
	w.mu.Lock()
	w.known[p] = h
	if id, ok := w.snapped[p]; ok {
		delete(w.snapped, p)
		w.afterEdit[id] = h
		w.editOrder = append(w.editOrder, id)
		if len(w.editOrder) > maxUndoRecords {
			delete(w.afterEdit, w.editOrder[0])
			w.editOrder = w.editOrder[1:]
		}
	}
	w.mu.Unlock()
}

// maxUndoRecords bounds how many backups remember the content the app wrote after them.
const maxUndoRecords = 256

// onSnapshot is called for every backup taken; the next note() for the same file belongs to it.
func (w *fileWatcher) onSnapshot(e api.BackupEntry) {
	if e.ID == "" || e.Path == "" {
		return
	}
	w.mu.Lock()
	w.snapped[filepath.Clean(e.Path)] = e.ID
	w.mu.Unlock()
}

// watches reports whether path is one of the files the watcher tracks.
func (w *fileWatcher) watches(path string) bool {
	p := filepath.Clean(path)
	for _, f := range w.list() {
		if f.path == p {
			return true
		}
	}
	return false
}

// writtenAfter returns the hash of the file as the app left it after the write that backup id was taken for.
func (w *fileWatcher) writtenAfter(id string) (string, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	h, ok := w.afterEdit[id]
	return h, ok
}

func (w *fileWatcher) get(path string) (string, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	h, ok := w.known[path]
	return h, ok
}

func (w *fileWatcher) set(path, h string) {
	w.mu.Lock()
	w.known[path] = h
	w.mu.Unlock()
}

// prime records the current hashes without reporting anything.
func (w *fileWatcher) prime() {
	for _, f := range w.list() {
		if _, ok := w.get(f.path); !ok {
			w.set(f.path, hashFile(f.path))
		}
	}
}

// poll compares every watched file with the known hash and returns the kinds that changed outside the app.
func (w *fileWatcher) poll() []watched {
	var changed []watched
	for _, f := range w.list() {
		known, ok := w.get(f.path)
		cur := hashFile(f.path)
		if !ok {
			w.set(f.path, cur)
			continue
		}
		if cur == known {
			continue
		}
		time.Sleep(w.settle) // an own write calls note() right after the rename
		known2, _ := w.get(f.path)
		cur = hashFile(f.path)
		if cur == known2 {
			continue
		}
		w.set(f.path, cur)
		changed = append(changed, f)
	}
	return changed
}

// pollFiles runs one watcher pass: emits files:external-change and refreshes accounts and repos.
func (c *Container) pollFiles(ctx context.Context) {
	for _, f := range c.files.poll() {
		c.emit(api.EventExternalChange, api.ExternalChange{Kind: f.kind, File: c.Paths.Display(f.path)})
		if ctx.Err() != nil {
			return
		}
		c.Accounts.EmitState(ctx)
		c.Repos.Invalidate(ctx)
		if st, err := c.Repos.State(ctx); err == nil {
			c.emit(api.EventReposChanged, st)
		}
	}
}
