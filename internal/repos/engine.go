// Package repos is the repository engine: it scans disk for CodeCommit clones, works out each clone's effective
// binding, detects the owning AWS account, binds/unbinds/repairs/tests repositories and clones new ones.
// It never writes secrets anywhere; git configuration changes always follow a backup snapshot.
package repos

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/codecommit"
	"github.com/rudian/codepass/internal/gitx"
	"github.com/rudian/codepass/internal/paths"
	"github.com/rudian/codepass/internal/projects"
	"github.com/rudian/codepass/internal/redact"
)

// ProfileInfo is what the repo engine needs to know about an AWS profile (supplied by the AWS side).
type ProfileInfo struct {
	Name, AccountID, Portal string
	Kind                    api.ProfileKind
	ColorSlot               int
	Monogram                string
	Usable                  bool
	NotSignedInPortal       string
	Provider                aws.CredentialsProvider
}

// ProfileSource lists the profiles. It is called often and must be cheap.
type ProfileSource func(ctx context.Context) ([]ProfileInfo, error)

// Store is the slice of settings the engine uses.
type Store interface {
	ScanRoots() []string
	ExtraRepos() []string
	AddExtraRepo(path string) error
	RemoveExtraRepo(path string) error
	AutoBindOnAdd() bool
	// RepoTests are the connection test results 0.0.1 kept in settings.json under the path hash. They are only
	// read, once per record, to fill the registry (F1-18); the engine never writes them back.
	RepoTests() map[string]api.TestResult
	Lang() string
}

// Registry is the repository registry (internal/projects, F1-18): the only place a path becomes a repository
// ID and back. *projects.Store implements it. Deps.Registry is required: the app passes the projects.json store
// and tests pass projects.OpenMemory(). There is no silent in-memory fallback.
type Registry interface {
	IDsForPaths(paths []string, provider projects.Provider, at time.Time) ([]string, error)
	IDsForExisting(paths []string) []string
	IDForPath(path string) (string, error)
	PathOf(id string) (string, bool)
	Snapshot() projects.Data
	LastTests() map[string]api.TestResult
	SetLastTest(id string, r api.TestResult) error
	ImportLegacyTests(legacy map[string]api.TestResult) (int, error)
	SetIdentities(m map[string]projects.RepoIdentity) error
	Reconcile(probe func(path string) projects.Presence, at time.Time) (missing, purged []string, err error)
	MovedFrom(id string) (projects.RepoRecord, bool)
	Reattach(oldID, newID string) error
	DeclineMove(newID, oldID string) error
	// F1-11: repositories that are not CodeCommit, hiding, projects.
	IDsForFound(paths []string, provider projects.Provider, at time.Time, pendingNew bool) (ids []string, created []bool, err error)
	ResolvePending(show, hide []string) error
	SetHidden(id string, hidden bool) error
	SetProject(repoID, projectID string) error
	AddProject(name string) (projects.Project, error)
	RenameProject(id, name string) error
	RemoveProject(id string) error
	SetProjectCollapsed(id string, collapsed bool) error
	DismissSuggestion(key string) error
}

// BackupPort snapshots a file before the engine changes it.
type BackupPort interface {
	Snapshot(path, display string, kind api.BackupKind, reason api.BackupReason, params map[string]string) (api.BackupEntry, error)
}

// Deps wires the engine.
type Deps struct {
	Paths      paths.Paths
	Git        func(ctx context.Context) (*gitx.Git, error)
	AWSPath    func() (string, error)
	AWSReady   func() bool
	Profiles   ProfileSource
	Store      Store
	Registry   Registry
	Backup     BackupPort
	CodeCommit codecommit.Factory
	Emit       func(name string, data any)
	Now        func() time.Time
	GOOS       string
	// RemoteURLFor overrides the URL used by Clone and CompareCommits. nil = the regional HTTPS URL.
	// Tests return file:// bare repositories.
	RemoteURLFor func(region, name string) string
	// Watch, when set, keeps the status of listed repositories fresh: the engine subscribes to the folder of
	// every repository that is shown (never a hidden or not yet reviewed one) and re-reads that repository's
	// status when something in it changes. *watch.Watcher implements it.
	Watch WatchPort
}

type rec struct {
	id    string
	sr    gitx.ScannedRepo
	cls   gitx.Classified
	owner api.OwnerResult
}

// Engine holds the scanned repositories.
type Engine struct {
	d Deps

	scanMu sync.Mutex // serialises scans
	mu     sync.Mutex
	recs   map[string]*rec
	reg    Registry
	moved  map[string]MovedOffer
	lost   map[string]api.OwnerResult // owner results of records that left the list, for a re-attach

	// Repositories that are not CodeCommit (GitHub, Bitbucket, anything else, no remote at all), by ID. They
	// are kept apart from recs so every CodeCommit code path runs on exactly the records it always did.
	others     map[string]*orec
	status     map[string]api.RepoStatus // last status read, by ID
	lastCommit map[string]string         // RFC 3339 of HEAD's commit, for found and hidden rows
	opened     map[string]bool           // repositories the person opened in this run (background fetch)
	warnings   []api.RepoWarning
	leftOut    map[string]bool // folders a scan could not give a record (projects.json not writable)
	watchSeq   uint64
	watches    map[string]watchSub
	bgCtx      context.Context
	bgCancel   context.CancelFunc

	scanned           bool
	scannedAt         time.Time
	scanning          bool
	detecting         bool
	globalHelper      string // first aws helper among plain global/system helpers ("" = none)
	globalHelperFound bool
}

// New creates an engine. Nothing is scanned until State or Scan is called.
func New(d Deps) *Engine {
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Emit == nil {
		d.Emit = func(string, any) {}
	}
	if d.GOOS == "" {
		d.GOOS = goosDefault()
	}
	reg := d.Registry
	if reg == nil {
		// No silent fallback to an in-memory registry: IDs would change at every launch and test results
		// would never be saved. Every registry call fails loudly instead, until the caller passes one.
		reg = noRegistry{}
	}
	bg, cancel := context.WithCancel(context.Background())
	return &Engine{d: d, recs: map[string]*rec{}, reg: reg, moved: map[string]MovedOffer{}, lost: map[string]api.OwnerResult{},
		others: map[string]*orec{}, status: map[string]api.RepoStatus{}, lastCommit: map[string]string{},
		opened: map[string]bool{}, leftOut: map[string]bool{}, watches: map[string]watchSub{}, bgCtx: bg, bgCancel: cancel}
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

func emptyOwner() api.OwnerResult {
	return api.OwnerResult{Status: api.OwnerUnknown, Candidates: []api.OwnerCandidate{}, SkippedPortals: []string{}}
}

// primary returns the primary remote's name, parsed info and URL.
func primary(sr gitx.ScannedRepo) (name string, info gitx.RemoteInfo, url string) {
	r := sr.Remotes[sr.Primary]
	if r.Info.Kind == api.RemoteOther && r.PushInfo != nil {
		return r.Name, *r.PushInfo, r.PushURL
	}
	return r.Name, r.Info, r.URL
}

func (e *Engine) classify(sr gitx.ScannedRepo) gitx.Classified {
	_, info, url := primary(sr)
	meta := gitx.LocalMetaFrom(sr.Entries)
	var eff gitx.Effective
	if info.Kind == api.RemoteHTTPS {
		u, err := gitx.ParseCredURL(url)
		if err != nil {
			u, _ = gitx.ParseCredURL(info.HTTPSURL)
		}
		eff = gitx.Resolve(sr.Entries, u)
	}
	return gitx.Classify(eff, meta, info.Kind, info.Profile, fileExists)
}

func (e *Engine) git(ctx context.Context) (*gitx.Git, error) {
	if e.d.Git == nil {
		return nil, api.NewError(api.ErrGitMissing, "git is not configured")
	}
	g, err := e.d.Git(ctx)
	if err != nil {
		return nil, api.AsAppError(err)
	}
	return g, nil
}

func (e *Engine) profiles(ctx context.Context) []ProfileInfo {
	if e.d.Profiles == nil {
		return nil
	}
	ps, err := e.d.Profiles(ctx)
	if err != nil {
		return nil
	}
	return ps
}

func profileMap(ps []ProfileInfo) map[string]ProfileInfo {
	m := make(map[string]ProfileInfo, len(ps))
	for _, p := range ps {
		m[p.Name] = p
	}
	return m
}

func (e *Engine) now() string { return e.d.Now().UTC().Format(time.RFC3339) }

// ---- scanning ----

// Scan walks the scan roots and extra repositories again. Owner results of repositories that are still present
// are kept.
func (e *Engine) Scan(ctx context.Context) (api.ReposState, error) {
	e.scanMu.Lock()
	defer e.scanMu.Unlock()
	g, err := e.git(ctx)
	if err != nil {
		return api.ReposState{}, err
	}
	e.mu.Lock()
	e.scanning = true
	e.mu.Unlock()
	defer func() {
		e.mu.Lock()
		e.scanning = false
		e.mu.Unlock()
	}()

	scanAt := e.d.Now() // one clock for FirstSeen and MissingSince of this scan
	roots := e.d.Store.ScanRoots()
	dirs, err := gitx.FindRepoDirs(ctx, roots, gitx.DefaultScanOptions, func(root string, found int) {
		e.d.Emit(api.EventScanProgress, api.ScanProgress{Root: e.d.Paths.Display(root), Found: found})
	})
	if err != nil {
		return api.ReposState{}, api.AsAppError(err)
	}
	seen := map[string]bool{}
	for _, d := range dirs {
		seen[d] = true
	}
	for _, x := range e.d.Store.ExtraRepos() {
		x = filepath.Clean(x)
		if seen[x] {
			continue
		}
		if _, err := os.Stat(filepath.Join(x, ".git")); err == nil {
			seen[x] = true
			dirs = append(dirs, x)
		}
	}
	e.d.Emit(api.EventScanProgress, api.ScanProgress{Found: len(dirs), Done: true})

	scanned, inspectErrs := g.InspectMany(ctx, dirs, gitx.DefaultScanOptions.Concurrency)
	if err := ctx.Err(); err != nil {
		return api.ReposState{}, api.AsAppError(err)
	}
	gh, ghFound := e.readGlobalHelper(ctx, g)
	// Every other repository under the roots (GitHub, Bitbucket, no remote at all) is read by the read-only layer.
	otherScanned := e.inspectOthers(ctx, g, otherDirs(dirs, scanned, inspectErrs))
	if err := ctx.Err(); err != nil {
		return api.ReposState{}, api.AsAppError(err)
	}

	ids, err := e.idsFor(scanned, scanAt)
	if err != nil {
		return api.ReposState{}, err
	}
	otherIDs := e.idsForOthers(otherScanned, scanAt)
	e.mu.Lock()
	left := map[string]bool{}
	next := make(map[string]*rec, len(scanned))
	for i, sr := range scanned {
		id := ids[i]
		if id == "" { // no record and projects.json could not be written: left out until the next scan
			left[sr.Path] = true
			continue
		}
		r := &rec{id: id, sr: sr, cls: e.classify(sr), owner: emptyOwner()}
		if old, ok := e.recs[id]; ok {
			r.owner = old.owner
		}
		next[id] = r
	}
	for id, old := range e.recs { // a row that left the list (folder moved by hand) keeps its owner for a re-attach
		if _, ok := next[id]; !ok && old.owner.Status != api.OwnerUnknown {
			e.lost[id] = old.owner
		}
	}
	e.recs = next
	nextOthers := make(map[string]*orec, len(otherScanned))
	for i, sr := range otherScanned {
		if otherIDs[i] == "" {
			left[sr.Path] = true
			continue
		}
		nextOthers[otherIDs[i]] = &orec{id: otherIDs[i], sr: sr, provider: otherProvider(sr)}
	}
	e.others = nextOthers
	e.leftOut = left
	e.warnings = nil
	if len(left) > 0 {
		e.warnings = []api.RepoWarning{{Code: api.WarnProjectsNotWritable, Count: len(left), File: "projects.json"}}
	}
	e.scanned = true
	e.scanning = false
	e.scannedAt = e.d.Now()
	e.globalHelper, e.globalHelperFound = gh, ghFound
	e.mu.Unlock()
	e.afterScan(ctx, g, joinScanned(scanned, otherScanned), joinIDs(ids, otherIDs), scanAt)
	e.fillLastCommits(ctx, g)
	e.syncWatches()

	st := e.buildState(ctx)
	e.d.Emit(api.EventReposChanged, st)
	return st, nil
}

// readGlobalHelper finds the first aws helper among the plain (no subsection) credential.helper entries of the
// system and global files that survives the last plain reset.
func (e *Engine) readGlobalHelper(ctx context.Context, g *gitx.Git) (string, bool) {
	es, err := g.GlobalEntries(ctx)
	if err != nil {
		return "", false
	}
	var list []string
	for _, en := range es {
		if en.Subsection != "" || en.Var != "helper" || en.NoValue {
			continue
		}
		if en.Value == "" {
			list = list[:0]
			continue
		}
		list = append(list, en.Value)
	}
	for _, v := range list {
		if gitx.ParseAWSHelper(v).IsAWS {
			return v, true
		}
	}
	return "", false
}

// State returns the current state, scanning once if nothing was scanned yet.
func (e *Engine) State(ctx context.Context) (api.ReposState, error) {
	e.mu.Lock()
	scanned := e.scanned
	e.mu.Unlock()
	if !scanned {
		return e.Scan(ctx)
	}
	return e.buildState(ctx), nil
}

// Invalidate re-inspects the bindings of known repositories without walking the disk. It does not emit.
func (e *Engine) Invalidate(ctx context.Context) {
	g, err := e.git(ctx)
	if err != nil {
		return
	}
	e.mu.Lock()
	dirs := make([]string, 0, len(e.recs))
	for _, r := range e.recs {
		dirs = append(dirs, r.sr.Path)
	}
	e.mu.Unlock()
	scanned, _ := g.InspectMany(ctx, dirs, gitx.DefaultScanOptions.Concurrency)
	gh, ghFound := e.readGlobalHelper(ctx, g)
	ids, err := e.idsFor(scanned, e.d.Now())
	if err != nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	next := make(map[string]*rec, len(scanned))
	for i, sr := range scanned {
		id := ids[i]
		if id == "" {
			continue
		}
		r := &rec{id: id, sr: sr, cls: e.classify(sr), owner: emptyOwner()}
		if old, ok := e.recs[id]; ok {
			r.owner = old.owner
		}
		next[id] = r
	}
	e.recs = next
	e.globalHelper, e.globalHelperFound = gh, ghFound
}

// refresh re-inspects one repository after a change.
func (e *Engine) refresh(ctx context.Context, g *gitx.Git, id string) error {
	e.mu.Lock()
	old, ok := e.recs[id]
	e.mu.Unlock()
	if !ok {
		return nil
	}
	sr, found, err := g.Inspect(ctx, old.sr.Path)
	if err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if !found {
		delete(e.recs, id)
		return nil
	}
	e.recs[id] = &rec{id: id, sr: sr, cls: e.classify(sr), owner: old.owner}
	return nil
}

// ---- building the DTOs ----

func (e *Engine) buildRepo(r *rec, pm map[string]ProfileInfo, tests map[string]api.TestResult) api.Repo {
	rname, info, url := primary(r.sr)
	meta := gitx.LocalMetaFrom(r.sr.Entries)
	b := api.Binding{
		Source: r.cls.Source, Profile: r.cls.Profile, URLRule: r.cls.URLRule,
		LegacyPlain: r.cls.LegacyPlain, NeedsRepair: r.cls.NeedsRepair, NoProfileFlag: r.cls.NoProfileFlag,
	}
	if p, ok := pm[r.cls.Profile]; ok && r.cls.Profile != "" {
		b.ProfileKnown, b.AccountID, b.ColorSlot, b.Monogram = true, p.AccountID, p.ColorSlot, p.Monogram
	} else if r.cls.Source == api.BindingLocal && meta.AccountID != "" {
		b.AccountID = meta.AccountID
	}
	repo := api.Repo{
		ID: r.id, Path: r.sr.Path, DisplayPath: e.d.Paths.Display(r.sr.Path), Name: info.Repo, Remote: rname,
		RemoteURL: redact.URL(url), RemoteKind: info.Kind, Host: info.Host, Region: info.Region, HTTPSURL: info.HTTPSURL,
		HasUserinfo: info.Kind == api.RemoteHTTPS && info.User != "", Binding: b, Owner: r.owner,
		LocalName: filepath.Base(r.sr.Path), RemoteName: remoteNameFor(r.sr), Provider: api.ProviderCodeCommit,
	}
	repo.Owner.Candidates = append([]api.OwnerCandidate{}, r.owner.Candidates...)
	repo.Owner.SkippedPortals = append([]string{}, r.owner.SkippedPortals...)
	if t, ok := tests[r.id]; ok {
		tt := t
		repo.LastTest = &tt
	}
	repo.Extra = e.isExtra(r.sr.Path)
	repo.ToFix = !((b.Source == api.BindingLocal && !b.LegacyPlain && !b.NeedsRepair && b.ProfileKnown) || b.Source == api.BindingRemoteURL)
	return repo
}

func (e *Engine) buildState(ctx context.Context) api.ReposState {
	pm := profileMap(e.profiles(ctx))
	tests := e.reg.LastTests()
	roots := e.d.Store.ScanRoots()
	data := e.reg.Snapshot()
	recOf := make(map[string]projects.RepoRecord, len(data.Repos))
	for _, r := range data.Repos {
		recOf[r.ID] = r
	}

	e.mu.Lock()
	recs := make([]*rec, 0, len(e.recs))
	for _, r := range e.recs {
		recs = append(recs, r)
	}
	others := make([]*orec, 0, len(e.others))
	for _, o := range e.others {
		others = append(others, o)
	}
	status := make(map[string]api.RepoStatus, len(e.status))
	for k, v := range e.status {
		status[k] = v
	}
	lastCommit := make(map[string]string, len(e.lastCommit))
	for k, v := range e.lastCommit {
		lastCommit[k] = v
	}
	moved := make(map[string]MovedOffer, len(e.moved))
	for k, v := range e.moved {
		moved[k] = v
	}
	warnings := append([]api.RepoWarning{}, e.warnings...)
	scanning, detecting := e.scanning, e.detecting
	scannedAt, scanned := e.scannedAt, e.scanned
	gh, ghFound := e.globalHelper, e.globalHelperFound
	e.mu.Unlock()

	st := api.ReposState{
		Roots: append([]string{}, roots...), RootsDisplay: make([]string, 0, len(roots)),
		Scanning: scanning, Detecting: detecting, Repos: make([]api.Repo, 0, len(recs)+len(others)),
		AWSCLIReady: e.d.AWSReady != nil && e.d.AWSReady(),
		Found:       []api.Repo{}, HiddenRepos: []api.Repo{}, Projects: []api.ProjectInfo{},
		Suggestions: []api.ProjectSuggestion{}, Warnings: warnings,
	}
	known := map[string]bool{}
	for _, p := range data.Projects {
		known[p.ID] = true
	}
	for _, r := range roots {
		st.RootsDisplay = append(st.RootsDisplay, e.d.Paths.Display(r))
	}
	if scanned {
		st.ScannedAt = scannedAt.UTC().Format(time.RFC3339)
	}
	// route puts a built repository into the list, the review set or the hidden set.
	route := func(repo api.Repo, sr gitx.ScannedRepo, provider api.RepoProvider) {
		rr := recOf[repo.ID]
		repo.LocalName = filepath.Base(sr.Path)
		repo.RemoteName = remoteNameFor(sr)
		repo.Provider = provider
		if known[rr.ProjectID] {
			repo.ProjectID = rr.ProjectID
		}
		repo.Hidden, repo.Pending = rr.Hidden, rr.Pending
		if mo, ok := moved[repo.ID]; ok {
			repo.MovedFrom = &api.MovedFrom{OldID: mo.OldID, Name: mo.Name}
		}
		switch {
		case rr.Pending:
			repo.LastCommitAt = lastCommit[repo.ID]
			st.Found = append(st.Found, repo)
		case rr.Hidden:
			repo.LastCommitAt = lastCommit[repo.ID]
			st.HiddenRepos = append(st.HiddenRepos, repo)
		default:
			if s, ok := status[repo.ID]; ok {
				s := s
				repo.Status = &s
			}
			st.Repos = append(st.Repos, repo)
			st.Counts.All++
			if provider == api.ProviderCodeCommit {
				if repo.ToFix {
					st.Counts.ToFix++
				} else {
					st.Counts.Bound++
				}
				if repo.Binding.Source == api.BindingLocal && repo.Binding.NeedsRepair {
					st.NeedsRepair++
				}
			}
		}
	}
	for _, r := range recs {
		route(e.buildRepo(r, pm, tests), r.sr, api.ProviderCodeCommit)
	}
	for _, o := range others {
		route(e.buildOther(o), o.sr, api.RepoProvider(o.provider))
	}
	byName := func(l []api.Repo) {
		sort.Slice(l, func(i, j int) bool {
			if l[i].Name != l[j].Name {
				return l[i].Name < l[j].Name
			}
			return l[i].DisplayPath < l[j].DisplayPath
		})
	}
	byName(st.Repos)
	byName(st.Found)
	byName(st.HiddenRepos)
	st.Counts.Found, st.Counts.Hidden = len(st.Found), len(st.HiddenRepos)
	st.Projects = projectInfos(data, st.Repos)
	st.Suggestions = suggestionsFor(data, st.Repos, roots)
	if ghFound {
		h := gitx.ParseAWSHelper(gh)
		gd := &api.GlobalDefaultInfo{Profile: h.Profile, Helper: redact.String(gh)}
		if p, ok := pm[h.Profile]; ok && h.HasProfile {
			gd.ProfileKind, gd.ColorSlot, gd.Monogram = p.Kind, p.ColorSlot, p.Monogram
		}
		for _, r := range st.Repos {
			if r.Binding.Source == api.BindingGlobalDefault {
				gd.AffectedRepos++
			}
		}
		st.GlobalDefault = gd
	}
	return st
}

// remoteNameFor is the last segment of the origin URL (or of the primary remote's, when there is no origin).
func remoteNameFor(sr gitx.ScannedRepo) string {
	for _, r := range sr.Remotes {
		if r.Name == "origin" {
			if r.URL != "" {
				return remoteNameOf(r.URL)
			}
			return remoteNameOf(r.PushURL)
		}
	}
	if r, ok := originOrPrimary(sr); ok {
		if r.URL != "" {
			return remoteNameOf(r.URL)
		}
		return remoteNameOf(r.PushURL)
	}
	return ""
}

// Snapshot returns the current repositories without scanning.
//
// It returns every CodeCommit repository, hidden or waiting for the review included, and no GitHub, Bitbucket
// or other row: callers such as internal/gitglobal treat it as "all CodeCommit repositories" (URL-rule
// migration, default-change impact, bound counts) and a hidden repository must not escape those. The UI list
// is ReposState.Repos, which is separate.
func (e *Engine) Snapshot() []api.Repo {
	ctx := context.Background()
	pm := profileMap(e.profiles(ctx))
	tests := e.reg.LastTests()
	e.mu.Lock()
	recs := make([]*rec, 0, len(e.recs))
	for _, r := range e.recs {
		recs = append(recs, r)
	}
	status := make(map[string]api.RepoStatus, len(e.status))
	for k, v := range e.status {
		status[k] = v
	}
	e.mu.Unlock()
	out := make([]api.Repo, 0, len(recs))
	for _, r := range recs {
		repo := e.buildRepo(r, pm, tests)
		e.fillRecord(&repo)
		if s, ok := status[repo.ID]; ok && !repo.Hidden && !repo.Pending {
			s := s
			repo.Status = &s
		}
		out = append(out, repo)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].DisplayPath < out[j].DisplayPath
	})
	return out
}

// RepoPath returns the absolute path of a repository.
func (e *Engine) RepoPath(id string) (string, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if r, ok := e.recs[id]; ok {
		return r.sr.Path, true
	}
	if o, ok := e.others[id]; ok {
		return o.sr.Path, true
	}
	return "", false
}

// BoundCounts returns, per profile name, how many repositories currently use it (local binding, global URL
// rule, global default or git-remote-codecommit URL). It implements accounts.RepoStats and takes no
// callbacks, so it is safe to call from the AWS side while computing its own state.
func (e *Engine) BoundCounts() map[string]int {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := map[string]int{}
	for _, r := range e.recs {
		switch r.cls.Source {
		case api.BindingLocal, api.BindingGlobalURL, api.BindingGlobalDefault, api.BindingRemoteURL:
			if r.cls.Profile != "" {
				out[r.cls.Profile]++
			}
		}
	}
	return out
}

// GlobalDefault returns the profile named by the global default helper, the number of repositories that follow
// it, and whether a global default exists.
func (e *Engine) GlobalDefault() (string, int, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.globalHelperFound {
		return "", 0, false
	}
	n := 0
	for _, r := range e.recs {
		if r.cls.Source == api.BindingGlobalDefault {
			n++
		}
	}
	return gitx.ParseAWSHelper(e.globalHelper).Profile, n, true
}

// Hosts returns the distinct CodeCommit host names of all known repositories, sorted.
func (e *Engine) Hosts() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	set := map[string]bool{}
	for _, r := range e.recs {
		for _, rm := range r.sr.Remotes {
			if rm.Info.Host != "" {
				set[rm.Info.Host] = true
			}
			if rm.PushInfo != nil && rm.PushInfo.Host != "" {
				set[rm.PushInfo.Host] = true
			}
		}
	}
	out := make([]string, 0, len(set))
	for h := range set {
		out = append(out, h)
	}
	sort.Strings(out)
	return out
}

func (e *Engine) rec(id string) (*rec, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if r, ok := e.recs[id]; ok {
		return r, nil
	}
	if _, ok := e.others[id]; ok {
		return nil, api.NewError(api.ErrNotCodeCommit, "the repository has no CodeCommit remote")
	}
	return nil, api.NewError(api.ErrRepoNotFound, "repository not found").WithParam("path", id)
}

func (e *Engine) emitState(ctx context.Context) api.ReposState {
	st := e.buildState(ctx)
	e.d.Emit(api.EventReposChanged, st)
	return st
}

func hostFor(region string) string {
	if strings.HasPrefix(region, "cn-") {
		return "git-codecommit." + region + ".amazonaws.com.cn"
	}
	return "git-codecommit." + region + ".amazonaws.com"
}
