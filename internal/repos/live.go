package repos

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/gitx"
	"github.com/rudian/codepass/internal/redact"
)

// WatchPort is the file watcher the engine uses (internal/watch, F1-05; the app adapts *watch.Watcher). Subscribe
// reports "something changed in this folder" (already debounced) until the returned stop function is called.
//
// After WatchRemoved or WatchStopped the subscription delivers nothing more (internal/watch: until Subscribe is
// called again), so the engine drops it and the next syncWatches subscribes again.
type WatchPort interface {
	Subscribe(dir string, fn func(WatchEvent)) (stop func(), err error)
}

// WatchEvent is what a subscription reports.
type WatchEvent int

const (
	WatchChanged WatchEvent = iota // something in the folder changed
	WatchRemoved                   // the folder is gone or was renamed away
	WatchStopped                   // watching failed and stopped
)

// watchStop is what the engine remembers of a repository whose watch stopped: when it last did, and how many
// times in a row it did so quickly (a watch that keeps dying must not cost a status read every few seconds).
type watchStop struct {
	resubscribed time.Time // when the watch was (or will be) subscribed again after the last stop
	n            int       // quick stops in a row
}

const (
	watchStableAfter = time.Minute     // a watch that lived this long starts the count again
	watchRetryMax    = 5 * time.Minute // the longest wait before subscribing again
)

// watchRetryDelay records a stop of id's watch and returns how long to wait before subscribing again: nothing
// for the first stop, then base, 2*base, 4*base ... (at most watchRetryMax) while the stops follow each other
// within a minute.
func (e *Engine) watchRetryDelay(id string) time.Duration {
	base := e.d.WatchRetryBase
	if base <= 0 {
		base = 5 * time.Second
	}
	now := time.Now()
	e.mu.Lock()
	defer e.mu.Unlock()
	st := e.watchStops[id]
	if st.resubscribed.IsZero() || now.Sub(st.resubscribed) > watchStableAfter {
		st.n = 0 // the last subscription lived long enough: this is a first stop again
	}
	st.n++
	var wait time.Duration
	if st.n > 1 {
		wait = base
		for i := 2; i < st.n && wait < watchRetryMax; i++ {
			wait *= 2
		}
		wait = min(wait, watchRetryMax)
	}
	st.resubscribed = now.Add(wait)
	e.watchStops[id] = st
	return wait
}

type watchSub struct {
	path  string
	stop  func()
	token uint64 // tells a late event of an old subscription from the current one
}

// flaggedIDs returns the IDs the registry marks hidden or waiting for the review. It takes no engine lock.
func (e *Engine) flaggedIDs() map[string]bool {
	flag := map[string]bool{}
	for _, r := range e.reg.Snapshot().Repos {
		if r.Hidden || r.Pending {
			flag[r.ID] = true
		}
	}
	return flag
}

// shownIDsLocked is the set of IDs of e.recs that background work may touch. The caller holds e.mu.
func (e *Engine) shownIDsLocked() map[string]bool {
	flag := e.flaggedIDs()
	out := make(map[string]bool, len(e.recs))
	for id := range e.recs {
		if !flag[id] {
			out[id] = true
		}
	}
	return out
}

// shown returns the folder of every repository that is in the list: not hidden, not waiting for the review. It
// is the one definition of "background work may touch this repository": status reads, watching and fetch all
// start from it, so a hidden repository is excluded from every one of them.
func (e *Engine) shown() map[string]string {
	flag := e.flaggedIDs()
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make(map[string]string, len(e.recs)+len(e.others))
	for id, r := range e.recs {
		if !flag[id] {
			out[id] = r.sr.Path
		}
	}
	for id, o := range e.others {
		if !flag[id] {
			out[id] = o.sr.Path
		}
	}
	return out
}

// RefreshStatus re-reads the git status of listed repositories (ids nil = all of them) through the read-only
// git layer, then emits the state. Hidden and not yet reviewed repositories are skipped.
func (e *Engine) RefreshStatus(ctx context.Context, ids []string) (api.ReposState, error) {
	if err := e.readStatus(ctx, ids); err != nil {
		return api.ReposState{}, err
	}
	return e.emitState(ctx), nil
}

func (e *Engine) readStatus(ctx context.Context, ids []string) error {
	g, err := e.git(ctx)
	if err != nil {
		return err
	}
	targets := e.shown()
	if ids != nil {
		keep := map[string]string{}
		for _, id := range ids {
			if p, ok := targets[id]; ok {
				keep[id] = p
			}
		}
		targets = keep
	}
	rd := gitx.NewReader(g)
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for id, path := range targets {
		wg.Add(1)
		sem <- struct{}{}
		go func(id, path string) {
			defer func() { <-sem; wg.Done() }()
			st, err := rd.Status(ctx, path)
			if err != nil {
				return // keep the last good reading
			}
			s := toRepoStatus(st, e.now())
			e.mu.Lock()
			e.status[id] = s
			e.mu.Unlock()
		}(id, path)
	}
	wg.Wait()
	return ctx.Err()
}

func toRepoStatus(st gitx.Status, at string) api.RepoStatus {
	return api.RepoStatus{
		Branch: st.Head, Detached: st.Detached, Unborn: st.OID == "", Upstream: st.Upstream, SyncKnown: st.HasAB,
		Ahead: st.Ahead, Behind: st.Behind, Staged: st.Staged, Unstaged: st.Unstaged, Untracked: st.Untracked,
		Conflicted: st.Conflicted, Changes: st.Changes(), Unverified: len(st.Unverified), RenamesSkipped: st.RenamesSkipped, At: at,
	}
}

// syncWatches makes the set of watched folders equal to the set of listed repositories: it starts watching a
// repository that appeared or was unhidden and stops watching one that was hidden, removed or moved.
func (e *Engine) syncWatches() {
	if e.d.Watch == nil {
		return
	}
	want := e.shown()
	var stops []func()
	e.mu.Lock()
	for id, sub := range e.watches {
		if p, ok := want[id]; !ok || p != sub.path {
			stops = append(stops, sub.stop)
			delete(e.watches, id)
		}
	}
	var add []string
	for id := range want {
		if _, ok := e.watches[id]; !ok {
			add = append(add, id)
		}
	}
	e.mu.Unlock()
	for _, stop := range stops {
		stop()
	}
	sort.Strings(add)
	for _, id := range add {
		id, path := id, want[id]
		e.mu.Lock()
		e.watchSeq++
		token := e.watchSeq
		e.mu.Unlock()
		stop, err := e.d.Watch.Subscribe(path, func(ev WatchEvent) { e.onWatch(id, token, ev) })
		if err != nil {
			continue
		}
		e.mu.Lock()
		if _, dup := e.watches[id]; dup {
			e.mu.Unlock()
			stop()
			continue
		}
		e.watches[id] = watchSub{path: path, stop: stop, token: token}
		e.mu.Unlock()
	}
}

// onWatch runs when the watcher reports on a listed repository's folder. A removed or stopped subscription
// delivers nothing more, so it is dropped here and the next syncWatches (after a scan, at once for a stop)
// subscribes again; a stop also reads the status once, because changes may have been missed.
func (e *Engine) onWatch(id string, token uint64, ev WatchEvent) {
	if e.bgCtx.Err() != nil {
		return
	}
	if ev != WatchChanged {
		var stop func()
		e.mu.Lock()
		if sub, ok := e.watches[id]; ok && sub.token == token {
			stop = sub.stop
			delete(e.watches, id)
		}
		e.mu.Unlock()
		if stop != nil {
			stop()
		}
		if ev == WatchRemoved {
			return // the next scan sees a folder that is gone (or back); a status read would only fail
		}
		if d := e.watchRetryDelay(id); d > 0 {
			// it keeps stopping: neither subscribe nor read the status now, do both once after the wait
			time.AfterFunc(d, func() {
				if e.bgCtx.Err() != nil {
					return
				}
				e.syncWatches()
				if err := e.readStatus(e.bgCtx, []string{id}); err == nil {
					e.emitState(e.bgCtx)
				}
			})
			return
		}
		e.syncWatches()
	}
	if err := e.readStatus(e.bgCtx, []string{id}); err == nil {
		e.emitState(e.bgCtx)
	}
}

// Close stops watching and cancels background reads. The engine can still answer calls afterwards.
func (e *Engine) Close() {
	e.bgCancel()
	e.mu.Lock()
	subs := e.watches
	e.watches = map[string]watchSub{}
	e.mu.Unlock()
	for _, s := range subs {
		s.stop()
	}
}

// ---- background fetch ----

// MarkOpened records that the person opened a repository in this run; it then takes part in background fetch.
func (e *Engine) MarkOpened(id string) error {
	if !e.knownRepo(id) {
		return api.NewError(api.ErrRepoNotFound, "repository not found").WithParam("path", id)
	}
	e.mu.Lock()
	e.opened[id] = true
	e.mu.Unlock()
	return nil
}

// fetchTargets returns the listed repositories that may be fetched in the background: ones the person bound
// (a CodeCommit binding, or an account binding on the record) or opened. A repository that is hidden, waiting
// for review or neither bound nor opened is never in it: the rule is "background fetch runs only for
// repositories the user bound or opened" (plan 4.6).
func (e *Engine) fetchTargets() map[string]fetchTarget {
	shown := e.shown()
	recs := map[string]bool{}
	for _, r := range e.reg.Snapshot().Repos {
		recs[r.ID] = r.Binding != nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	out := map[string]fetchTarget{}
	for id, path := range shown {
		if r, ok := e.recs[id]; ok {
			if e.opened[id] || boundProfile(r) != "" {
				name, _, _ := primary(r.sr)
				out[id] = fetchTarget{path: path, remote: name}
			}
			continue
		}
		if o, ok := e.others[id]; ok && (e.opened[id] || recs[id]) {
			if rem, ok := originOrPrimary(o.sr); ok {
				out[id] = fetchTarget{path: path, remote: rem.Name}
			}
		}
	}
	return out
}

type fetchTarget struct{ path, remote string }

// FetchTargets lists, sorted, the IDs background fetch may run for.
func (e *Engine) FetchTargets() []string {
	t := e.fetchTargets()
	out := make([]string, 0, len(t))
	for id := range t {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// Fetch fetches one repository's remote. It refuses a repository that is not a background-fetch target.
func (e *Engine) Fetch(ctx context.Context, id string) error {
	t, ok := e.fetchTargets()[id]
	if !ok {
		return api.NewError(api.ErrInvalidInput, "background fetch runs only for repositories that are bound or opened").WithParam("field", "repoId")
	}
	if !gitx.ValidRemoteName(t.remote) {
		return api.NewError(api.ErrInvalidInput, "invalid remote name").WithParam("field", "remote")
	}
	g, err := e.git(ctx)
	if err != nil {
		return err
	}
	// No prompt, no maintenance run (gc and maintenance start hooks), no submodule recursion, no tags.
	// Nothing may prompt (terminal, credential manager, askpass, ssh askpass) and no repository hook runs: hooks
	// are for actions the person started (plan 4.6).
	res, err := g.WithEnv("GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=", "SSH_ASKPASS=", "SSH_ASKPASS_REQUIRE=never").Run(ctx, t.path, nil,
		"-c", "credential.interactive=false", "-c", "core.askPass=", "-c", "core.hooksPath="+e.emptyHooksDir(),
		"-c", "core.fsmonitor=false", "-c", "gc.auto=0", "-c", "maintenance.auto=false",
		"fetch", "--quiet", "--no-tags", "--no-recurse-submodules", t.remote)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return api.NewError(api.ErrGitFailed, "git could not run").WithDetail(err.Error())
	}
	if res.ExitCode != 0 {
		return api.NewError(api.ErrGitFailed, "git fetch failed (exit %d)", res.ExitCode).WithDetail(redact.Tail(string(res.Stderr), 3))
	}
	return nil
}

// BackgroundFetch fetches every target in turn and returns how many succeeded and failed. A failure never
// stops the others. The app calls it on its own schedule.
func (e *Engine) BackgroundFetch(ctx context.Context) (ok, failed int) {
	for _, id := range e.FetchTargets() {
		if ctx.Err() != nil {
			break
		}
		if err := e.Fetch(ctx, id); err != nil {
			failed++
		} else {
			ok++
		}
	}
	return ok, failed
}

// emptyHooksDir is an app-owned folder that stays empty; git is pointed at it so no hook of the repository runs.
func (e *Engine) emptyHooksDir() string {
	if e.d.Paths.AppDataDir != "" {
		dir := filepath.Join(e.d.Paths.AppDataDir, "no-hooks")
		if os.MkdirAll(dir, 0o700) == nil {
			return dir
		}
	}
	return os.DevNull
}
