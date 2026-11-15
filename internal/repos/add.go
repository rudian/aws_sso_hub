package repos

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/gitx"
)

// underScanRoot reports whether a scan of the configured roots would find the repository at path: it lies at
// most MaxDepth levels below a root, no component is hidden or excluded, no directory on the way is itself a
// repository, and no component is a symlink.
func (e *Engine) underScanRoot(path string) bool {
	opt := gitx.DefaultScanOptions
	skip := map[string]bool{}
	for _, x := range opt.Exclude {
		skip[x] = true
	}
	path = filepath.Clean(path)
	for _, root := range e.d.Store.ScanRoots() {
		root = filepath.Clean(root)
		rel, err := filepath.Rel(root, path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		if rel == "." {
			return true
		}
		parts := strings.Split(rel, string(filepath.Separator))
		if len(parts) > opt.MaxDepth {
			continue
		}
		ok := true
		cur := root
		for i, part := range parts {
			if skip[part] || strings.HasPrefix(part, ".") {
				ok = false
				break
			}
			cur = filepath.Join(cur, part)
			st, err := os.Lstat(cur)
			if err != nil || !st.IsDir() {
				ok = false
				break
			}
			if i < len(parts)-1 {
				if _, err := os.Lstat(filepath.Join(cur, ".git")); err == nil {
					ok = false
					break
				}
			}
		}
		if ok {
			return true
		}
	}
	return false
}

// isExtra is true for a repository that is listed only because the user added it by hand.
func (e *Engine) isExtra(path string) bool {
	path = filepath.Clean(path)
	for _, x := range e.d.Store.ExtraRepos() {
		if filepath.Clean(x) == path {
			return !e.underScanRoot(path)
		}
	}
	return false
}

var scpRemoteRE = regexp.MustCompile(`^(?:[^@/]+@)?([^:/\s]+):`)

// remoteHostsOf lists the host names of the repository's remote URLs (no user names, no passwords, no paths).
func remoteHostsOf(urls []string) []string {
	set := map[string]bool{}
	for _, raw := range urls {
		raw = strings.TrimSpace(raw)
		host := ""
		if u, err := url.Parse(raw); err == nil && u.Host != "" {
			host = u.Hostname()
		} else if m := scpRemoteRE.FindStringSubmatch(raw); m != nil {
			host = m[1]
		}
		if host != "" {
			set[strings.ToLower(host)] = true
		}
	}
	out := make([]string, 0, len(set))
	for h := range set {
		out = append(out, h)
	}
	sort.Strings(out)
	return out
}

func samePath(a, b string) bool {
	if a == b {
		return true
	}
	ra, err1 := filepath.EvalSymlinks(a)
	rb, err2 := filepath.EvalSymlinks(b)
	return err1 == nil && err2 == nil && ra == rb
}

// repoTop resolves the top-level directory of the repository containing path, keeping the spelling of path
// (git prints symlink-resolved paths).
func (e *Engine) repoTop(ctx context.Context, g *gitx.Git, path string) (string, error) {
	notRepo := func() error {
		return api.NewError(api.ErrRepoNotFound, "not a git repository").WithParam("path", path)
	}
	if !filepath.IsAbs(path) {
		return "", notRepo()
	}
	path = filepath.Clean(path)
	st, err := os.Stat(path)
	if err != nil || !st.IsDir() {
		return "", notRepo()
	}
	res, err := g.Run(ctx, path, nil, "rev-parse", "--show-toplevel")
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", api.NewError(api.ErrGitFailed, "git could not run").WithDetail(err.Error())
	}
	if res.ExitCode != 0 {
		return "", notRepo()
	}
	top := filepath.Clean(strings.TrimSpace(string(res.Stdout)))
	if top == "" || top == "." {
		return "", notRepo()
	}
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return top, nil
	}
	rel, err := filepath.Rel(top, real)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return top, nil
	}
	if rel == "." {
		return path, nil
	}
	up := path
	for range strings.Split(rel, string(filepath.Separator)) {
		up = filepath.Dir(up)
	}
	return up, nil
}

// AddExisting lists a repository that is already on disk, detects the owning account and, when exactly one SSO
// profile is a safe choice, binds it.
func (e *Engine) AddExisting(ctx context.Context, path string) (api.AddRepoResult, error) {
	g, err := e.git(ctx)
	if err != nil {
		return api.AddRepoResult{}, err
	}
	top, err := e.repoTop(ctx, g, path)
	if err != nil {
		return api.AddRepoResult{}, api.AsAppError(err)
	}
	if _, found, err := g.Inspect(ctx, top); err != nil {
		return api.AddRepoResult{}, api.AsAppError(err)
	} else if !found {
		var urls []string
		if res, rerr := g.Run(ctx, top, nil, "config", "--get-regexp", `^remote\..*\.(url|pushurl)$`); rerr == nil && res.ExitCode == 0 {
			for _, line := range strings.Split(string(res.Stdout), "\n") {
				if _, v, ok := strings.Cut(strings.TrimSpace(line), " "); ok {
					urls = append(urls, v)
				}
			}
		}
		return api.AddRepoResult{}, api.NewError(api.ErrNotCodeCommit, "the repository has no CodeCommit remote").
			WithDetail(strings.Join(remoteHostsOf(urls), ", "))
	}

	// Is it listed already?
	id := repoID(top)
	e.mu.Lock()
	scanned := e.scanned
	listed := false
	if r, ok := e.recs[id]; ok && r != nil {
		listed = true
	} else {
		for _, r := range e.recs {
			if samePath(r.sr.Path, top) {
				id, top, listed = r.id, r.sr.Path, true
				break
			}
		}
	}
	e.mu.Unlock()
	added := false
	if !listed || !scanned {
		listed = e.underScanRoot(top)
		if !listed {
			for _, x := range e.d.Store.ExtraRepos() {
				if samePath(filepath.Clean(x), top) {
					listed = true
					break
				}
			}
		}
	}
	if !listed {
		if err := e.d.Store.AddExtraRepo(top); err != nil {
			return api.AddRepoResult{}, api.AsAppError(err)
		}
		added = true
	}

	if _, err := e.Scan(ctx); err != nil {
		return api.AddRepoResult{}, err
	}
	e.mu.Lock()
	if _, ok := e.recs[id]; !ok {
		for _, r := range e.recs {
			if samePath(r.sr.Path, top) {
				id = r.id
				break
			}
		}
	}
	_, present := e.recs[id]
	e.mu.Unlock()
	if !present {
		return api.AddRepoResult{}, api.NewError(api.ErrRepoNotFound, "repository not found").WithParam("path", top)
	}

	res := api.AddRepoResult{RepoID: id, Added: added}
	if e.d.CodeCommit != nil {
		if _, err := e.DetectOwners(ctx, []string{id}); err != nil {
			return api.AddRepoResult{}, api.AsAppError(err)
		}
		if o, ok := e.ownerOf(id); ok && o.Status == api.OwnerAmbiguous {
			// A failing comparison leaves the repository ambiguous; the user then chooses.
			_, _ = e.CompareCommits(ctx, id)
		}
	}
	if o, ok := e.autoBind(ctx, g, id); ok {
		res.AutoBound, res.Outcome = true, &o
	}
	res.State = e.emitState(ctx)
	return res, nil
}

func (e *Engine) ownerOf(id string) (api.OwnerResult, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	r, ok := e.recs[id]
	if !ok {
		return api.OwnerResult{}, false
	}
	return r.owner, true
}

// autoBind applies the automatic-binding rule to one repository.
func (e *Engine) autoBind(ctx context.Context, g *gitx.Git, id string) (api.BindOutcome, bool) {
	if !e.d.Store.AutoBindOnAdd() || e.d.AWSReady == nil || !e.d.AWSReady() {
		return api.BindOutcome{}, false
	}
	e.mu.Lock()
	r, ok := e.recs[id]
	if !ok {
		e.mu.Unlock()
		return api.BindOutcome{}, false
	}
	_, info, _ := primary(r.sr)
	source := r.cls.Source
	owner := r.owner
	e.mu.Unlock()
	if info.Kind != api.RemoteHTTPS || source == api.BindingLocal {
		return api.BindOutcome{}, false
	}
	var pick *api.OwnerCandidate
	switch {
	case owner.Status == api.OwnerSingle && len(owner.Candidates) == 1:
		pick = &owner.Candidates[0]
	case owner.Status == api.OwnerAmbiguous:
		n := 0
		for i := range owner.Candidates {
			if owner.Candidates[i].HeadMatch {
				pick = &owner.Candidates[i]
				n++
			}
		}
		if n != 1 {
			return api.BindOutcome{}, false
		}
	}
	if pick == nil || pick.Profile == "" {
		return api.BindOutcome{}, false
	}
	p, err := e.profileFor(ctx, pick.Profile)
	if err != nil || p.Kind != api.ProfileKindSSO {
		return api.BindOutcome{}, false
	}
	o, err := e.bindOne(ctx, g, id, p.Name, true, api.ReasonBind)
	if err != nil {
		return api.BindOutcome{}, false
	}
	return o, true
}

// RemoveExtra stops listing a hand-added repository. The folder and its git configuration are not touched.
func (e *Engine) RemoveExtra(ctx context.Context, id string) (api.ReposState, error) {
	r, err := e.rec(id)
	if err != nil {
		return api.ReposState{}, err
	}
	if !e.isExtra(r.sr.Path) {
		return api.ReposState{}, api.NewError(api.ErrInvalidInput, "the repository was not added by hand").WithParam("field", "repoId")
	}
	if err := e.d.Store.RemoveExtraRepo(r.sr.Path); err != nil {
		return api.ReposState{}, api.AsAppError(err)
	}
	return e.Scan(ctx)
}
