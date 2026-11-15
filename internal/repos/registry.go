package repos

import (
	"context"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/gitx"
	"github.com/rudian/codepass/internal/projects"
)

// MovedOffer is the question "Is this <Name>, moved here?" (F1-18): a repository found at a new folder whose
// first commit and origin URL match exactly one record that vanished from disk less than 30 days ago.
type MovedOffer struct {
	OldID   string // the record that would be re-attached
	OldPath string // its folder before it disappeared
	Name    string // that folder's name, shown in the question
}

// idsFor maps scanned folders to repository IDs through the registry, creating records for new folders. When
// projects.json cannot be written (full disk, unwritable folder), the folders that already have a record are
// still resolved from a snapshot, which needs no write, and the new folders get "" (left out of this scan; the
// next scan retries them). Only when nothing at all can be listed does it fail, with a file error.
func (e *Engine) idsFor(scanned []gitx.ScannedRepo, at time.Time) ([]string, error) {
	paths := make([]string, len(scanned))
	for i, sr := range scanned {
		paths[i] = sr.Path
	}
	ids, err := e.reg.IDsForPaths(paths, projects.ProviderCodeCommit, at)
	if err == nil {
		return ids, nil
	}
	ids = e.reg.IDsForExisting(paths)
	for _, id := range ids {
		if id != "" {
			return ids, nil
		}
	}
	if len(paths) == 0 {
		return ids, nil
	}
	return nil, api.NewError(api.ErrFileIO, "cannot read or write the repository list").
		WithParam("file", "projects.json").WithDetail(err.Error())
}

// idFor is the ID of one folder, created if new; the record is a CodeCommit repository.
func (e *Engine) idFor(path string) (string, error) {
	ids, err := e.reg.IDsForPaths([]string{path}, projects.ProviderCodeCommit, time.Time{})
	if err != nil {
		return "", api.AsAppError(err)
	}
	return ids[0], nil
}

// afterScan keeps the registry in step with what the scan found. Every step is best effort: a registry that
// cannot be written must not hide the repositories the scan found. at is the scan's start time.
func (e *Engine) afterScan(ctx context.Context, g *gitx.Git, scanned []gitx.ScannedRepo, ids []string, at time.Time) {
	var sc []gitx.ScannedRepo
	var kept []string
	for i, id := range ids {
		if id != "" {
			sc, kept = append(sc, scanned[i]), append(kept, id)
		}
	}
	scanned, ids = sc, kept
	// One-time import of the results 0.0.1 saved in settings.json under the path hash (read only).
	if e.d.Store != nil {
		_, _ = e.reg.ImportLegacyTests(e.d.Store.RepoTests())
	}
	e.recordIdentities(ctx, g, scanned, ids)
	_, _, _ = e.reg.Reconcile(projects.ProbePath, at)
	offers := map[string]MovedOffer{}
	for _, id := range ids {
		if old, ok := e.reg.MovedFrom(id); ok {
			offers[id] = MovedOffer{OldID: old.ID, OldPath: old.Path, Name: filepath.Base(old.Path)}
		}
	}
	e.mu.Lock()
	e.moved = offers
	e.mu.Unlock()
}

// recordIdentities stores the first commit and origin URL of repositories whose record has none yet (read
// through the read-only git layer), so a folder moved by hand can be recognised later.
func (e *Engine) recordIdentities(ctx context.Context, g *gitx.Git, scanned []gitx.ScannedRepo, ids []string) {
	have := map[string]projects.RepoRecord{}
	for _, r := range e.reg.Snapshot().Repos {
		have[r.ID] = r
	}
	rd := gitx.NewReader(g)
	var (
		mu  sync.Mutex
		out = map[string]projects.RepoIdentity{}
		sem = make(chan struct{}, 4)
		wg  sync.WaitGroup
	)
	for i, sr := range scanned {
		id, rec := ids[i], have[ids[i]]
		origin := originURL(sr)
		if rec.FirstCommit != "" && rec.OriginURL == origin {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer func() { <-sem; wg.Done() }()
			ident := projects.RepoIdentity{FirstCommit: rec.FirstCommit, OriginURL: origin}
			if ident.FirstCommit == "" {
				ident.FirstCommit = firstCommit(ctx, rd, sr.Path)
			}
			mu.Lock()
			out[id] = ident
			mu.Unlock()
		}()
	}
	wg.Wait()
	_ = e.reg.SetIdentities(out)
}

// originURL is the "origin" remote's URL with the user and password removed (a token is often the user name),
// or "" without one. Two checkouts that differ only in credentials compare equal.
func originURL(sr gitx.ScannedRepo) string {
	for _, r := range sr.Remotes {
		if r.Name == "origin" {
			return projects.StripURLUserinfo(r.URL)
		}
	}
	return ""
}

// firstCommit is the smallest root commit hash of HEAD ("" for an unborn branch or on any error).
func firstCommit(ctx context.Context, rd *gitx.Reader, dir string) string {
	res, err := rd.Run(ctx, dir, "rev-list", "--max-parents=0", "HEAD")
	if err != nil || res.ExitCode != 0 {
		return ""
	}
	var roots []string
	for _, l := range strings.Split(string(res.Stdout), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			roots = append(roots, l)
		}
	}
	if len(roots) == 0 {
		return ""
	}
	sort.Strings(roots)
	return roots[0]
}

// MovedOffers returns, per repository ID found by the last scan, the missing record it may have been moved
// from. The row shows "Is this <Name>, moved here?"; ReattachMoved answers yes.
func (e *Engine) MovedOffers() map[string]MovedOffer {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make(map[string]MovedOffer, len(e.moved))
	for k, v := range e.moved {
		out[k] = v
	}
	return out
}

// ReattachMoved answers yes for the repository found at a new folder: the old record (project, hidden flag,
// identity override, binding, test result) takes the folder, and the repository list is rebuilt. The folder
// itself is never touched.
func (e *Engine) ReattachMoved(ctx context.Context, newID string) (api.ReposState, error) {
	e.mu.Lock()
	offer, ok := e.moved[newID]
	e.mu.Unlock()
	if !ok {
		return api.ReposState{}, api.NewError(api.ErrRepoNotFound, "repository not found").WithParam("path", newID)
	}
	if err := e.reg.Reattach(offer.OldID, newID); err != nil {
		return api.ReposState{}, api.AsAppError(err)
	}
	// The row keeps its detected owner under the old ID.
	e.mu.Lock()
	if r, ok := e.recs[newID]; ok && r != nil {
		if o, ok := e.lost[offer.OldID]; ok {
			r.owner = o
			delete(e.lost, offer.OldID)
		}
		r.id = offer.OldID
		e.recs[offer.OldID] = r
		delete(e.recs, newID)
	}
	delete(e.moved, newID)
	e.mu.Unlock()
	return e.Scan(ctx)
}

// DeclineMoved answers no for the repository found at a new folder: it is treated as a new repository, and the
// question for this pair is not asked again (it survives rescans and restarts). The folder is never touched.
func (e *Engine) DeclineMoved(ctx context.Context, newID string) (api.ReposState, error) {
	e.mu.Lock()
	offer, ok := e.moved[newID]
	e.mu.Unlock()
	if !ok {
		return api.ReposState{}, api.NewError(api.ErrRepoNotFound, "repository not found").WithParam("path", newID)
	}
	if err := e.reg.DeclineMove(newID, offer.OldID); err != nil {
		return api.ReposState{}, api.AsAppError(err)
	}
	e.mu.Lock()
	delete(e.moved, newID)
	e.mu.Unlock()
	st := e.buildState(ctx)
	e.d.Emit(api.EventReposChanged, st)
	return st, nil
}

// noRegistry is the Registry of an engine that was given none: every call reports it.
type noRegistry struct{}

var errNoRegistry = api.NewError(api.ErrInternal, "repository registry is not configured")

func (noRegistry) IDsForPaths([]string, projects.Provider, time.Time) ([]string, error) {
	return nil, errNoRegistry
}
func (noRegistry) IDsForExisting(p []string) []string                       { return make([]string, len(p)) }
func (noRegistry) IDForPath(string) (string, error)                         { return "", errNoRegistry }
func (noRegistry) PathOf(string) (string, bool)                             { return "", false }
func (noRegistry) Snapshot() projects.Data                                  { return projects.Data{} }
func (noRegistry) LastTests() map[string]api.TestResult                     { return map[string]api.TestResult{} }
func (noRegistry) SetLastTest(string, api.TestResult) error                 { return errNoRegistry }
func (noRegistry) ImportLegacyTests(map[string]api.TestResult) (int, error) { return 0, errNoRegistry }
func (noRegistry) SetIdentities(map[string]projects.RepoIdentity) error     { return errNoRegistry }
func (noRegistry) Reconcile(func(string) projects.Presence, time.Time) ([]string, []string, error) {
	return nil, nil, errNoRegistry
}
func (noRegistry) MovedFrom(string) (projects.RepoRecord, bool) { return projects.RepoRecord{}, false }
func (noRegistry) Reattach(string, string) error                { return errNoRegistry }
func (noRegistry) DeclineMove(string, string) error             { return errNoRegistry }
