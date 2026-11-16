package repos

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/codecommit"
	"github.com/rudian/codepass/internal/gitx"
)

var reRepoName = regexp.MustCompile(`^[\w.-]{1,100}$`)

// ListRemote lists the repositories of the account behind profile in region.
func (e *Engine) ListRemote(ctx context.Context, profile, region string) ([]api.RemoteRepo, error) {
	if !gitx.RegionRE.MatchString(region) {
		return nil, api.NewError(api.ErrInvalidRegion, "invalid region")
	}
	p, err := e.profileFor(ctx, profile)
	if err != nil {
		return nil, err
	}
	if !p.Usable || p.Provider == nil {
		portal := p.NotSignedInPortal
		if portal == "" {
			portal = p.Portal
		}
		return nil, api.NewError(api.ErrNotSignedIn, "profile is not signed in").WithParam("portal", portal)
	}
	if e.d.CodeCommit == nil {
		return nil, api.NewError(api.ErrInternal, "no CodeCommit client configured")
	}
	metas, err := e.d.CodeCommit(p.Provider, region).ListRepositories(ctx)
	if err != nil {
		return nil, mapAWSError(err, p)
	}
	sort.Slice(metas, func(i, j int) bool { return metas[i].Name < metas[j].Name })
	out := make([]api.RemoteRepo, 0, len(metas))
	for _, m := range metas {
		rr := api.RemoteRepo{Name: m.Name, Description: m.Description, RepositoryID: m.RepositoryID, CloneURL: m.CloneURLHTTP}
		if rr.CloneURL == "" {
			rr.CloneURL = "https://" + hostFor(region) + "/v1/repos/" + m.Name
		}
		if !m.LastModified.IsZero() {
			rr.LastModified = m.LastModified.UTC().Format(time.RFC3339)
		}
		out = append(out, rr)
	}
	return out, nil
}

func mapAWSError(err error, p ProfileInfo) error {
	switch {
	case ctxErr(err) != nil:
		return ctxErr(err)
	case codecommit.IsNotSignedIn(err):
		portal := p.NotSignedInPortal
		if portal == "" {
			portal = p.Portal
		}
		return api.NewError(api.ErrNotSignedIn, "profile is not signed in").WithParam("portal", portal)
	case codecommit.IsAccessDenied(err):
		return api.NewError(api.ErrAWSAccessDenied, "access denied").WithDetail("codecommit:ListRepositories")
	}
	return api.NewError(api.ErrAWSAPI, "AWS returned an error").WithDetail(err.Error())
}

func ctxErr(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return nil
}

func (e *Engine) cloneTarget(req api.CloneRequest) (dest, url string, err error) {
	dest, url, _, err = e.cloneTargetNamed(req)
	return dest, url, err
}

// cloneTargetNamed is cloneTarget that also returns the folder name used: the request's, or the remote name when
// it was left empty.
func (e *Engine) cloneTargetNamed(req api.CloneRequest) (dest, url, folder string, err error) {
	if !gitx.RegionRE.MatchString(req.Region) {
		return "", "", "", api.NewError(api.ErrInvalidRegion, "invalid region")
	}
	if !reRepoName.MatchString(req.RepoName) {
		return "", "", "", api.NewError(api.ErrInvalidInput, "invalid repository name").WithParam("field", "repoName")
	}
	f := req.FolderName
	if f == "" {
		f = req.RepoName // the optional folder name: the remote's own name
	}
	if verr := ValidateFolderName(f); verr != nil {
		return "", "", "", verr
	}
	if !filepath.IsAbs(req.ParentDir) {
		return "", "", "", api.NewError(api.ErrInvalidInput, "parent folder must be an absolute path").WithParam("field", "parentDir")
	}
	if st, serr := os.Stat(req.ParentDir); serr != nil || !st.IsDir() {
		return "", "", "", api.NewError(api.ErrInvalidInput, "parent folder does not exist").WithParam("field", "parentDir")
	}
	dest = filepath.Join(req.ParentDir, f)
	url = "https://" + hostFor(req.Region) + "/v1/repos/" + req.RepoName
	return dest, url, f, nil
}

// PreviewClone validates a clone request and shows the destination and command.
func (e *Engine) PreviewClone(ctx context.Context, req api.CloneRequest) (api.ClonePreview, error) {
	dest, url, folder, err := e.cloneTargetNamed(req)
	if err != nil {
		return api.ClonePreview{}, err
	}
	_, statErr := os.Lstat(dest)
	// The command names the folder as well, so "Command to run" and "Clones into" say the same (a custom
	// folder name is part of it).
	return api.ClonePreview{Dest: dest, Exists: statErr == nil, FolderName: folder, Command: "git clone " + url + " " + shellArg(dest, e.d.GOOS), HTTPSURL: url}, nil
}

// shellArg writes a path as one argument of a command line for the terminal a person would paste it in: as it
// is when it has only plain characters, otherwise in single quotes (macOS) or double quotes (Windows).
func shellArg(s, goos string) string {
	plain := s != ""
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("_@%+=:,./-\\", r)) {
			plain = false
			break
		}
	}
	switch {
	case plain:
		return s
	case goos == "windows":
		return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// Clone clones a CodeCommit repository with the binding already applied.
func (e *Engine) Clone(ctx context.Context, req api.CloneRequest) (api.BindOutcome, error) {
	aws, err := e.awsPath()
	if err != nil {
		return api.BindOutcome{}, err
	}
	g, err := e.git(ctx)
	if err != nil {
		return api.BindOutcome{}, err
	}
	p, err := e.profileFor(ctx, req.Profile)
	if err != nil {
		return api.BindOutcome{}, err
	}
	if !p.Usable {
		portal := p.NotSignedInPortal
		if portal == "" {
			portal = p.Portal
		}
		return api.BindOutcome{}, api.NewError(api.ErrNotSignedIn, "profile is not signed in").WithParam("portal", portal)
	}
	dest, canonical, err := e.cloneTarget(req)
	if err != nil {
		return api.BindOutcome{}, err
	}
	if _, serr := os.Lstat(dest); serr == nil {
		return api.BindOutcome{}, api.NewError(api.ErrDestExists, "destination exists").WithParam("path", dest)
	}
	url := canonical
	if e.d.RemoteURLFor != nil {
		url = e.d.RemoteURLFor(req.Region, req.RepoName)
	}
	spec := gitx.CloneSpec{URL: url, Dest: dest, Bind: gitx.BindSpec{
		Hosts: []string{hostFor(req.Region)}, Profile: p.Name, AccountID: accountOf(p), AWSPath: aws,
		Lang: e.d.Store.Lang(), GOOS: e.d.GOOS,
	}}
	if _, err := gitx.CloneArgs(spec); err != nil {
		return api.BindOutcome{}, err
	}
	if err := g.Clone(ctx, spec, func(line string) { e.d.Emit(api.EventCloneProgress, api.CloneProgress{Line: line}) }); err != nil {
		return api.BindOutcome{}, api.AsAppError(err)
	}
	if url != canonical {
		// Tests clone from a local bare repository; point origin at the canonical URL like a real clone.
		if err := g.SetRemoteURL(ctx, dest, "origin", canonical, false); err != nil {
			return api.BindOutcome{}, api.AsAppError(err)
		}
	}
	if !e.underScanRoot(dest) {
		if err := e.d.Store.AddExtraRepo(dest); err != nil {
			return api.BindOutcome{}, api.AsAppError(err)
		}
	}
	sr, ok, err := g.Inspect(ctx, dest)
	if err != nil {
		return api.BindOutcome{}, api.AsAppError(err)
	}
	if !ok {
		return api.BindOutcome{}, api.NewError(api.ErrNotCodeCommit, "the cloned repository has no CodeCommit remote")
	}
	id, err := e.idFor(sr.Path)
	if err != nil {
		return api.BindOutcome{}, err
	}
	e.mu.Lock()
	e.recs[id] = &rec{id: id, sr: sr, cls: e.classify(sr), owner: emptyOwner()}
	e.mu.Unlock()
	o := e.outcome(ctx, id, "")
	e.emitState(ctx)
	return o, nil
}
