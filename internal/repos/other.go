package repos

import (
	"context"
	"errors"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/gitx"
	"github.com/rudian/codepass/internal/projects"
	"github.com/rudian/codepass/internal/redact"
)

// orec is a repository that is not a CodeCommit clone: GitHub, Bitbucket, any other host, or no remote at all.
// Its remotes are read by the read-only git layer (F1-04); the engine never changes such a repository. There is
// no classification or binding: it follows the machine's own git setup.
type orec struct {
	id       string
	sr       gitx.ScannedRepo // Primary is -1 when there is no remote
	provider projects.Provider
}

// otherDirs lists the folders the CodeCommit inspection did not return and did not fail on.
func otherDirs(dirs []string, cc []gitx.ScannedRepo, failed map[string]error) []string {
	is := make(map[string]bool, len(cc))
	for _, r := range cc {
		is[r.Path] = true
	}
	var out []string
	for _, d := range dirs {
		if !is[d] && failed[d] == nil {
			out = append(out, d)
		}
	}
	return out
}

func joinScanned(a, b []gitx.ScannedRepo) []gitx.ScannedRepo {
	out := make([]gitx.ScannedRepo, 0, len(a)+len(b))
	return append(append(out, a...), b...)
}

func joinIDs(a, b []string) []string {
	out := make([]string, 0, len(a)+len(b))
	return append(append(out, a...), b...)
}

// inspectOthers reads the remotes of every folder, a few at a time. A folder git cannot read is left out.
func (e *Engine) inspectOthers(ctx context.Context, g *gitx.Git, dirs []string) []gitx.ScannedRepo {
	if len(dirs) == 0 {
		return nil
	}
	rd := gitx.NewReader(g)
	res := make([]*gitx.ScannedRepo, len(dirs))
	sem := make(chan struct{}, gitx.DefaultScanOptions.Concurrency)
	var wg sync.WaitGroup
	for i, d := range dirs {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, d string) {
			defer func() { <-sem; wg.Done() }()
			if sr, ok := inspectOther(ctx, rd, d); ok {
				res[i] = &sr
			}
		}(i, d)
	}
	wg.Wait()
	out := make([]gitx.ScannedRepo, 0, len(dirs))
	for _, r := range res {
		if r != nil {
			out = append(out, *r)
		}
	}
	return out
}

// inspectOther reads the remote URLs of one repository. false: git could not read it (not a repository).
func inspectOther(ctx context.Context, rd *gitx.Reader, dir string) (gitx.ScannedRepo, bool) {
	res, err := rd.Run(ctx, dir, "config", "--null", "--get-regexp", `^remote\..*\.(url|pushurl)$`)
	if err != nil || (res.ExitCode != 0 && res.ExitCode != 1) {
		return gitx.ScannedRepo{}, false
	}
	sr := gitx.ScannedRepo{Path: dir, Primary: -1}
	if res.ExitCode == 1 { // no remote at all
		return sr, true
	}
	cfg, perr := gitx.ParseNullConfig(res.Stdout, false)
	if perr != nil {
		return gitx.ScannedRepo{}, false
	}
	urls := map[string]string{}
	push := map[string]string{}
	for _, c := range cfg {
		k := strings.TrimPrefix(c.Key, "remote.")
		switch {
		case strings.HasSuffix(k, ".pushurl"):
			push[strings.TrimSuffix(k, ".pushurl")] = c.Value
		case strings.HasSuffix(k, ".url"):
			urls[strings.TrimSuffix(k, ".url")] = c.Value // the last url wins, as for a single-valued key
		}
	}
	names := map[string]bool{}
	for n := range urls {
		names[n] = true
	}
	for n := range push {
		names[n] = true
	}
	sorted := make([]string, 0, len(names))
	for n := range names {
		sorted = append(sorted, n)
	}
	sort.Strings(sorted)
	for i, n := range sorted {
		sr.Remotes = append(sr.Remotes, gitx.Remote{Name: n, URL: urls[n], PushURL: push[n], Info: gitx.RemoteInfo{Kind: api.RemoteOther}})
		if n == "origin" {
			sr.Primary = i
		}
	}
	if sr.Primary < 0 {
		sr.Primary = 0
	}
	return sr, true
}

// originOrPrimary is the remote the repository's remote name and provider come from: "origin", else the first.
func originOrPrimary(sr gitx.ScannedRepo) (gitx.Remote, bool) {
	if sr.Primary < 0 || sr.Primary >= len(sr.Remotes) {
		return gitx.Remote{}, false
	}
	return sr.Remotes[sr.Primary], true
}

// otherProvider is the provider of a repository without a CodeCommit remote.
func otherProvider(sr gitx.ScannedRepo) projects.Provider {
	r, ok := originOrPrimary(sr)
	if !ok {
		return projects.ProviderOther
	}
	return ProviderFor(r.URL)
}

// idsForOthers gives every folder its record. Folders found for the first time wait in the review set
// (pending). When projects.json cannot be written the folders that already have a record are still listed from
// a snapshot, and the new ones get "" (left out of this scan, with a warning; the next scan retries them).
func (e *Engine) idsForOthers(scanned []gitx.ScannedRepo, at time.Time) []string {
	ids := make([]string, len(scanned))
	byProvider := map[projects.Provider][]int{}
	for i, sr := range scanned {
		p := otherProvider(sr)
		byProvider[p] = append(byProvider[p], i)
	}
	provs := make([]string, 0, len(byProvider))
	for p := range byProvider {
		provs = append(provs, string(p))
	}
	sort.Strings(provs)
	for _, ps := range provs {
		idx := byProvider[projects.Provider(ps)]
		paths := make([]string, len(idx))
		for j, i := range idx {
			paths[j] = scanned[i].Path
		}
		got, _, err := e.reg.IDsForFound(paths, projects.Provider(ps), at, true)
		if err != nil {
			got = e.reg.IDsForExisting(paths)
		}
		for j, i := range idx {
			ids[i] = got[j]
		}
	}
	return ids
}

// buildOther is the DTO of a repository that is not CodeCommit. The caller fills the record's own fields.
func (e *Engine) buildOther(o *orec) api.Repo {
	repo := api.Repo{
		ID: o.id, Path: o.sr.Path, DisplayPath: e.d.Paths.Display(o.sr.Path), Name: filepath.Base(o.sr.Path),
		RemoteKind: api.RemoteOther, Owner: emptyOwner(), Extra: e.isExtra(o.sr.Path),
		Binding: api.Binding{Source: StrategyFor(primaryURL(o.sr)).Binding()},
	}
	repo.Owner.Candidates = []api.OwnerCandidate{}
	if r, ok := originOrPrimary(o.sr); ok {
		repo.Remote = r.Name
		url := r.URL
		if url == "" {
			url = r.PushURL
		}
		// A bare token as the user name (https://<token>@host/...) is masked too, and counts as userinfo.
		repo.RemoteURL = redact.String(redact.URL(url))
		repo.HasUserinfo = urlHasCredential(url)
		repo.RemoteKind = remoteKindOf(url)
		if hs := remoteHostsOf([]string{url}); len(hs) > 0 {
			repo.Host = hs[0]
		}
	}
	return repo
}

// primaryURL is the fetch URL of the primary remote ("" without one).
func primaryURL(sr gitx.ScannedRepo) string {
	if r, ok := originOrPrimary(sr); ok {
		if r.URL != "" {
			return r.URL
		}
		return r.PushURL
	}
	return ""
}

// fillLastCommits reads the date of HEAD's commit for repositories that wait for review or are hidden, so the
// chooser can pre-tick recent ones. Other rows do not need it. Best effort.
func (e *Engine) fillLastCommits(ctx context.Context, g *gitx.Git) {
	snap := e.reg.Snapshot()
	flag := map[string]bool{}
	for _, r := range snap.Repos {
		if r.Pending || r.Hidden {
			flag[r.ID] = true
		}
	}
	type job struct{ id, path string }
	var jobs []job
	e.mu.Lock()
	for id, r := range e.recs {
		if flag[id] && e.lastCommit[id] == "" {
			jobs = append(jobs, job{id, r.sr.Path})
		}
	}
	for id, o := range e.others {
		if flag[id] && e.lastCommit[id] == "" {
			jobs = append(jobs, job{id, o.sr.Path})
		}
	}
	e.mu.Unlock()
	if len(jobs) == 0 {
		return
	}
	rd := gitx.NewReader(g)
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for _, j := range jobs {
		wg.Add(1)
		sem <- struct{}{}
		go func(j job) {
			defer func() { <-sem; wg.Done() }()
			res, err := rd.Run(ctx, j.path, "log", "-1", "--format=%cI")
			if err != nil || res.ExitCode != 0 {
				return
			}
			s := strings.TrimSpace(string(res.Stdout))
			if t, perr := time.Parse(time.RFC3339, s); perr == nil {
				e.mu.Lock()
				e.lastCommit[j.id] = t.UTC().Format(time.RFC3339)
				e.mu.Unlock()
			}
		}(j)
	}
	wg.Wait()
}

// AddFolder lists any git repository that is on disk: CodeCommit ones go through AddExisting (detects the
// account and binds); any other is added to the list as shown (the person asked for it, so it skips the
// review), with no binding. A hidden repository comes back.
func (e *Engine) AddFolder(ctx context.Context, path string) (api.AddRepoResult, error) {
	g, err := e.git(ctx)
	if err != nil {
		return api.AddRepoResult{}, err
	}
	top, err := e.repoTop(ctx, g, path)
	if err != nil {
		return api.AddRepoResult{}, api.AsAppError(err)
	}
	if _, found, ierr := g.Inspect(ctx, top); ierr != nil {
		return api.AddRepoResult{}, api.AsAppError(ierr)
	} else if found {
		res, err := e.AddExisting(ctx, top)
		if err != nil {
			return res, err
		}
		// A hidden or not yet reviewed CodeCommit repository comes back, like any other folder added by hand.
		if err := e.reg.ResolvePending([]string{res.RepoID}, nil); err != nil {
			return api.AddRepoResult{}, fileIOProjects(err)
		}
		if err := e.reg.SetHidden(res.RepoID, false); err != nil {
			return api.AddRepoResult{}, fileIOProjects(err)
		}
		res.State = e.emitState(ctx)
		e.syncWatches()
		return res, nil
	}
	listed := e.underScanRoot(top)
	if !listed {
		for _, x := range e.d.Store.ExtraRepos() {
			if samePath(filepath.Clean(x), top) {
				listed = true
				break
			}
		}
	}
	added := false
	if !listed {
		if err := e.d.Store.AddExtraRepo(top); err != nil {
			return api.AddRepoResult{}, api.AsAppError(err)
		}
		added = true
	}
	if _, err := e.Scan(ctx); err != nil {
		return api.AddRepoResult{}, err
	}
	id, ok := e.idOfPath(top)
	if !ok {
		if e.wasLeftOut(top) {
			return api.AddRepoResult{}, fileIOProjects(errors.New("the new folder could not be recorded"))
		}
		return api.AddRepoResult{}, api.NewError(api.ErrRepoNotFound, "repository not found").WithParam("path", top)
	}
	if err := e.reg.ResolvePending([]string{id}, nil); err != nil {
		return api.AddRepoResult{}, fileIOProjects(err)
	}
	if err := e.reg.SetHidden(id, false); err != nil {
		return api.AddRepoResult{}, fileIOProjects(err)
	}
	st := e.emitState(ctx)
	e.syncWatches()
	return api.AddRepoResult{State: st, RepoID: id, Added: added}, nil
}

func (e *Engine) idOfPath(path string) (string, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for id, r := range e.recs {
		if samePath(r.sr.Path, path) {
			return id, true
		}
	}
	for id, o := range e.others {
		if samePath(o.sr.Path, path) {
			return id, true
		}
	}
	return "", false
}

func (e *Engine) wasLeftOut(path string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	for p := range e.leftOut {
		if samePath(p, path) {
			return true
		}
	}
	return false
}

// fileIOProjects is the error for a registry write that failed: E_FILE_IO naming projects.json.
func fileIOProjects(err error) *api.AppError {
	if ae := (*api.AppError)(nil); errors.As(err, &ae) {
		return ae
	}
	return api.NewError(api.ErrFileIO, "cannot read or write the repository list").
		WithParam("file", "projects.json").WithDetail(err.Error())
}

// urlHasUserinfo reports whether a remote URL carries a user name or password (http, https, ssh or git URLs).
func urlHasUserinfo(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.User == nil {
		return false
	}
	if _, hasPw := u.User.Password(); hasPw {
		return true
	}
	// ssh://git@host/... is the normal way to reach a host over SSH, not a credential.
	return u.Scheme != "ssh" && u.User.Username() != ""
}

// urlHasCredential is the rule for hosts other than CodeCommit: a password or a token-shaped user name counts;
// a plain account name (Bitbucket's https://<user>@bitbucket.org/...) and its static user names do not.
func urlHasCredential(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.User == nil {
		return false
	}
	if _, hasPw := u.User.Password(); hasPw {
		return true
	}
	name := u.User.Username()
	switch strings.ToLower(name) {
	case "x-token-auth", "x-bitbucket-api-token-auth", "":
		return false
	}
	return redact.String(raw) != raw // a token-shaped user name is masked
}
