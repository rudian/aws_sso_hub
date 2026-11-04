package gitx

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/rudian/aws_sso_hub/internal/api"
)

// Remote is one configured remote of a repository.
type Remote struct {
	Name, URL, PushURL string
	Info               RemoteInfo
	PushInfo           *RemoteInfo
}

// ScannedRepo is a repository that has at least one CodeCommit remote.
type ScannedRepo struct {
	Path    string
	Remotes []Remote // all remotes, sorted by name; non-CodeCommit ones have Info.Kind == api.RemoteOther
	Primary int      // index into Remotes
	Entries []Entry
}

// ScanOptions bounds the repository walk.
type ScanOptions struct {
	MaxDepth    int
	Exclude     []string
	Concurrency int
}

// DefaultScanOptions are the default scan limits.
var DefaultScanOptions = ScanOptions{MaxDepth: 4, Exclude: []string{"node_modules", ".venv", "venv", "vendor", "Library", ".Trash"}, Concurrency: 8}

// FindRepoDirs walks roots to MaxDepth looking for directories that contain .git (a directory or a file).
// Roots that do not exist are skipped. A found repository is not descended into; symlinks are not followed.
// progress is called every 10 repositories per root and once with found = total after the root is done.
func FindRepoDirs(ctx context.Context, roots []string, opt ScanOptions, progress func(root string, found int)) ([]string, error) {
	if opt.MaxDepth <= 0 {
		opt.MaxDepth = DefaultScanOptions.MaxDepth
	}
	skip := map[string]bool{}
	for _, e := range opt.Exclude {
		skip[e] = true
	}
	seen := map[string]bool{}
	out := make([]string, 0)
	for _, root := range roots {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		root = filepath.Clean(root)
		st, err := os.Stat(root)
		found := 0
		if err == nil && st.IsDir() && filepath.IsAbs(root) {
			var walk func(dir string, depth int)
			walk = func(dir string, depth int) {
				if ctx.Err() != nil {
					return
				}
				if _, err := os.Lstat(filepath.Join(dir, ".git")); err == nil {
					if !seen[dir] {
						seen[dir] = true
						out = append(out, dir)
						found++
						if found%10 == 0 && progress != nil {
							progress(root, found)
						}
					}
					return
				}
				if depth >= opt.MaxDepth {
					return
				}
				ents, err := os.ReadDir(dir)
				if err != nil {
					return
				}
				for _, e := range ents {
					if !e.IsDir() || e.Type()&os.ModeSymlink != 0 {
						continue
					}
					n := e.Name()
					if skip[n] || strings.HasPrefix(n, ".") {
						continue
					}
					walk(filepath.Join(dir, n), depth+1)
				}
			}
			walk(root, 0)
		}
		if progress != nil {
			progress(root, found)
		}
	}
	sort.Strings(out)
	return out, ctx.Err()
}

var remoteKeyRE = func(key string) (name, kind string, ok bool) {
	if !strings.HasPrefix(key, "remote.") {
		return "", "", false
	}
	for _, suf := range []string{".pushurl", ".url"} {
		if strings.HasSuffix(key, suf) {
			return key[len("remote.") : len(key)-len(suf)], suf[1:], true
		}
	}
	return "", "", false
}

// Inspect reads the remotes and credential config of a repository. false means no remote is CodeCommit.
func (g *Git) Inspect(ctx context.Context, repoDir string) (ScannedRepo, bool, error) {
	res, err := g.runOK(ctx, repoDir, nil, []int{1}, "config", "--null", "--get-regexp", `^remote\..*\.(url|pushurl)$`)
	if err != nil {
		return ScannedRepo{}, false, err
	}
	if res.ExitCode == 1 {
		return ScannedRepo{}, false, nil
	}
	cfg, perr := ParseNullConfig(res.Stdout, false)
	if perr != nil {
		return ScannedRepo{}, false, api.NewError(api.ErrGitFailed, "cannot read remotes").WithDetail(perr.Error())
	}
	names := map[string]bool{}
	hasPush := map[string]bool{}
	for _, e := range cfg {
		if n, kind, ok := remoteKeyRE(e.Key); ok {
			names[n] = true
			if kind == "pushurl" {
				hasPush[n] = true
			}
		}
	}
	sorted := make([]string, 0, len(names))
	for n := range names {
		sorted = append(sorted, n)
	}
	sort.Strings(sorted)
	repo := ScannedRepo{Path: repoDir, Remotes: make([]Remote, 0, len(sorted)), Primary: -1}
	any := false
	for _, n := range sorted {
		r := Remote{Name: n}
		if !ValidRemoteName(n) {
			// Not passed to git (it could read the name as an option); such a remote is not a CodeCommit one.
			repo.Remotes = append(repo.Remotes, Remote{Name: n, Info: RemoteInfo{Kind: api.RemoteOther}})
			continue
		}
		if out, err := g.runOK(ctx, repoDir, nil, []int{128}, "remote", "get-url", "--", n); err == nil && out.ExitCode == 0 {
			r.URL = strings.TrimSpace(string(out.Stdout))
		}
		if hasPush[n] {
			if out, err := g.runOK(ctx, repoDir, nil, []int{128}, "remote", "get-url", "--push", "--", n); err == nil && out.ExitCode == 0 {
				r.PushURL = strings.TrimSpace(string(out.Stdout))
			}
		}
		if info, ok := ParseRemote(r.URL); ok {
			r.Info = info
			any = true
		} else {
			r.Info = RemoteInfo{Kind: api.RemoteOther}
		}
		if r.PushURL != "" {
			if info, ok := ParseRemote(r.PushURL); ok {
				r.PushInfo = &info
				any = true
			}
		}
		repo.Remotes = append(repo.Remotes, r)
	}
	if !any {
		return ScannedRepo{}, false, nil
	}
	isCC := func(r Remote) bool { return r.Info.Kind != api.RemoteOther || r.PushInfo != nil }
	for i, r := range repo.Remotes {
		if r.Name == "origin" && isCC(r) {
			repo.Primary = i
			break
		}
	}
	if repo.Primary < 0 {
		for i, r := range repo.Remotes {
			if isCC(r) {
				repo.Primary = i
				break
			}
		}
	}
	repo.Entries, err = g.CredentialEntries(ctx, repoDir)
	if err != nil {
		return ScannedRepo{}, false, err
	}
	return repo, true, nil
}

// InspectMany inspects repos concurrently (bounded by concurrency) and returns results in input order.
// Repos without a CodeCommit remote are omitted. Errors for single repositories are returned in errs.
func (g *Git) InspectMany(ctx context.Context, dirs []string, concurrency int) (repos []ScannedRepo, errs map[string]error) {
	if concurrency <= 0 {
		concurrency = DefaultScanOptions.Concurrency
	}
	type item struct {
		r   ScannedRepo
		ok  bool
		err error
	}
	results := make([]item, len(dirs))
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	for i, d := range dirs {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, d string) {
			defer wg.Done()
			defer func() { <-sem }()
			r, ok, err := g.Inspect(ctx, d)
			results[i] = item{r, ok, err}
		}(i, d)
	}
	wg.Wait()
	errs = map[string]error{}
	repos = make([]ScannedRepo, 0, len(dirs))
	for i, it := range results {
		switch {
		case it.err != nil:
			errs[dirs[i]] = it.err
		case it.ok:
			repos = append(repos, it.r)
		}
	}
	return repos, errs
}
